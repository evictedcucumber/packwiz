package changelog

import (
	"cmp"
	"slices"

	"github.com/evictedcucumber/packwiz/core"
)

// Kind is the kind of change made to a pack.
type Kind string

const (
	ModAdded    Kind = "mod-added"
	ModRemoved  Kind = "mod-removed"
	ModUpdated  Kind = "mod-updated"
	FileAdded   Kind = "file-added"
	FileRemoved Kind = "file-removed"
	FileChanged Kind = "file-changed"
	// Note is a commit that says something other than what a mod or a file did, such as one written by hand
	Note Kind = "note"
)

// Change is one thing that changed in a pack: a difference between two snapshots of it, or what a commit said.
type Change struct {
	Kind Kind `toml:"kind"`
	// Path is the path of the metadata file or config file, relative to the index. Changes read from commits know
	// the paths of config files but not of mods.
	Path string `toml:"path,omitempty"`

	// The rest only apply to mods (Kind is ModAdded, ModRemoved or ModUpdated):
	Name string `toml:"name,omitempty"`
	// Side is the mod's current side, or its last side if it was removed
	Side string `toml:"side,omitempty"`
	// From is the version before the change; empty for an added mod
	From string `toml:"from,omitempty"`
	// To is the version after the change; empty for a removed mod
	To string `toml:"to,omitempty"`

	// The rest only apply to notes (Kind is Note), which are conventional commits:
	// Type is the type of the commit, such as "feat" or "fix"
	Type string `toml:"type,omitempty"`
	// Scope is what the commit says it is about, such as "config"; it may be empty
	Scope string `toml:"scope,omitempty"`
	// Text is what the commit says was done
	Text string `toml:"text,omitempty"`
	// Breaking is whether the commit says it breaks compatibility
	Breaking bool `toml:"breaking,omitempty"`
}

// IsMod reports whether this is a change to a mod (as opposed to a config or other file, or a note).
func (c Change) IsMod() bool {
	return c.Kind == ModAdded || c.Kind == ModRemoved || c.Kind == ModUpdated
}

// Bump is how far this change raises the pack's version:
//
//   - Any change to a mod that runs on the server (server or both) is major, as servers must be updated to match.
//   - Adding or removing a client-only mod is minor.
//   - Updating a client-only mod is patch.
//   - Any change to a config or other file is patch.
//
// Each level lines up with a conventional commit type: major is a breaking change (feat!), minor is a feature (feat)
// and patch is a fix (fix). A note, which is a commit that wasn't written by packwiz, is bumped by its own type in
// the same way, and only if it is one of those; chores, documentation and the like don't make a release.
func (c Change) Bump() Bump {
	switch c.Kind {
	case Note:
		switch {
		case c.Breaking:
			return BumpMajor
		case c.Type == "feat":
			return BumpMinor
		case c.Type == "fix" || c.Type == "perf":
			return BumpPatch
		}
		return BumpNone
	case ModAdded, ModRemoved:
		if RunsOnServer(c.Side) {
			return BumpMajor
		}
		return BumpMinor
	case ModUpdated:
		if RunsOnServer(c.Side) {
			return BumpMajor
		}
		return BumpPatch
	default:
		return BumpPatch
	}
}

// RunsOnServer reports whether a mod with the given side is loaded by servers. Anything that isn't explicitly
// client-only is treated as running on the server, including an empty side (which packwiz treats as "both").
func RunsOnServer(side string) bool {
	return side != core.ClientSide
}

// HighestBump returns the largest bump of any of the changes, or BumpNone if there are none.
func HighestBump(changes []Change) Bump {
	highest := BumpNone
	for _, c := range changes {
		highest = max(highest, c.Bump())
	}
	return highest
}

// Diff describes how to get from the old snapshot to the new one, ordered by path. Changes to a mod's pin or
// dependency flags aren't reported, as they don't change what the pack contains, and neither is a version that has
// only just become known (see modUpdated).
func Diff(oldSnap, newSnap Snapshot) []Change {
	var changes []Change

	for path, n := range newSnap.Mods {
		o, existed := oldSnap.Mods[path]
		switch {
		case !existed:
			changes = append(changes, Change{Kind: ModAdded, Path: path, Name: n.Name, Side: n.Side, To: n.Version})
		case modUpdated(o, n):
			changes = append(changes, Change{Kind: ModUpdated, Path: path, Name: n.Name, Side: n.Side, From: o.Version, To: n.Version})
		}
	}
	for path, o := range oldSnap.Mods {
		if _, exists := newSnap.Mods[path]; !exists {
			changes = append(changes, Change{Kind: ModRemoved, Path: path, Name: o.Name, Side: o.Side, From: o.Version})
		}
	}

	for path, newHash := range newSnap.Files {
		oldHash, existed := oldSnap.Files[path]
		switch {
		case !existed:
			changes = append(changes, Change{Kind: FileAdded, Path: path})
		case oldHash != newHash:
			changes = append(changes, Change{Kind: FileChanged, Path: path})
		}
	}
	for path := range oldSnap.Files {
		if _, exists := newSnap.Files[path]; !exists {
			changes = append(changes, Change{Kind: FileRemoved, Path: path})
		}
	}

	// Map iteration order is random; sort so the same pack always produces the same changelog
	slices.SortFunc(changes, func(a, b Change) int { return cmp.Compare(a.Path, b.Path) })
	return changes
}

// modUpdated reports whether a mod has been updated since it was recorded as o. A mod whose version wasn't known is
// recorded by its file name, so if that is all that has changed, because the mod is still on that file and its
// version has since been looked up, the mod hasn't been updated.
func modUpdated(o, n SnapshotMod) bool {
	return o.Version != n.Version && o.Version != n.File
}
