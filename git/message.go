// Package git commits and releases a pack following one standard: conventional commits (conventionalcommits.org),
// whose types are tied to how far a change raises the pack's version.
//
//	Change                               Version   Commit
//	server or both-side mod: any change  major     feat(mods)!: add Lithium 0.12.0 (server)
//	client-only mod: added or removed    minor     feat(mods): add Sodium 0.5.7 (client)
//	client-only mod: updated             patch     fix(mods): update Iris 1.7.0 -> 1.7.1 (client)
//	config or other file: any change     patch     fix(config): change config/sodium.json
//	anything else (pins, pack.toml, ...) none      chore(pack): update pack files
//
// The scope of a mod's commit is the kind of content, going by the folder its metadata file is in: mods, resourcepacks,
// shaderpacks or datapacks (so a resource pack is "feat(resourcepacks): add Faithful 1.21 (client)").
//
// packwiz git commit makes one commit for each mod, resource pack, shader pack, data pack or config file that was added,
// updated, changed or removed, then one for anything else in the pack's own files, such as a mod being pinned. Files it
// doesn't recognise are left uncommitted. A release is committed as "chore(release): X.Y.Z" and tagged "vX.Y.Z".
package git

import (
	"fmt"
	"strings"

	"github.com/evictedcucumber/packwiz/changelog"
)

const (
	// InitialMessage is the message for a pack's first commit, which has nothing before it to be a change to
	InitialMessage = "chore(pack): initial commit"
	// OtherMessage is the message for changes that don't change what the pack contains, such as pinning a mod
	OtherMessage = "chore(pack): update pack files"

	breakingFooter = "BREAKING CHANGE: changes mods that run on the server; servers must be updated to match"
)

// Message describes changes to a pack as a conventional commit message. The commit's type and "!" marker are
// the bump the changes make: feat! for major, feat for minor and fix for patch.
func Message(changes []changelog.Change) string {
	if len(changes) == 0 {
		return OtherMessage
	}

	bump := changelog.HighestBump(changes)
	commitType := "fix"
	if bump >= changelog.BumpMinor {
		commitType = "feat"
	}
	breaking := bump == changelog.BumpMajor

	header := commitType + "(" + scope(changes) + ")"
	if breaking {
		header += "!"
	}
	header += ": " + subject(changes)

	paragraphs := []string{header}
	if len(changes) > 1 {
		lines := make([]string, len(changes))
		for i, c := range changes {
			lines[i] = "- " + line(c)
		}
		paragraphs = append(paragraphs, strings.Join(lines, "\n"))
	}
	if breaking {
		paragraphs = append(paragraphs, breakingFooter)
	}
	return strings.Join(paragraphs, "\n\n")
}

// CategoryMessage is the message for a file in a category (see core.FileCategories), which is a chore as it doesn't change
// what the pack contains: verb is "add", "change" or "remove".
func CategoryMessage(category, verb, path string) string {
	return "chore(" + category + "): " + verb + " " + path
}

// ReleaseMessage is the message for the commit that records a release.
func ReleaseMessage(version string) string {
	return "chore(release): " + version
}

// TagName is the name of the tag for a release.
func TagName(version string) string {
	return "v" + version
}

// scope is the kind of content (see changelog.ContentKind) if all the changes are to that kind, "mods" if they are to
// several, "config" if they are all to files, and "pack" if they are both.
func scope(changes []changelog.Change) string {
	kind, files := "", false
	for _, c := range changes {
		switch {
		case !c.IsMod():
			files = true
		case kind == "":
			kind = changelog.ContentKind(c.Path)
		case kind != changelog.ContentKind(c.Path):
			kind = "mods"
		}
	}
	switch {
	case kind != "" && files:
		return "pack"
	case kind != "":
		return kind
	default:
		return "config"
	}
}

// subject is the description in a commit's header: the change itself if there is only one, and a count of
// each kind of change otherwise.
func subject(changes []changelog.Change) string {
	if len(changes) == 1 {
		return line(changes[0])
	}

	var added, removed, updated, files int
	for _, c := range changes {
		switch c.Kind {
		case changelog.ModAdded:
			added++
		case changelog.ModRemoved:
			removed++
		case changelog.ModUpdated:
			updated++
		default:
			files++
		}
	}
	var parts []string
	for _, p := range []struct {
		verb  string
		count int
		noun  string
	}{
		{"add", added, "mod"},
		{"remove", removed, "mod"},
		{"update", updated, "mod"},
		{"update", files, "config file"},
	} {
		if p.count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d %s", p.verb, p.count, plural(p.count, p.noun)))
		}
	}
	return strings.Join(parts, ", ")
}

func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// line describes one change, e.g. "update Iris 1.7.0 -> 1.7.1 (client)".
func line(c changelog.Change) string {
	switch c.Kind {
	case changelog.ModAdded:
		return modLine("add", c.Name, c.To, c.Side)
	case changelog.ModRemoved:
		return modLine("remove", c.Name, c.From, c.Side)
	case changelog.ModUpdated:
		return modLine("update", c.Name, c.From+" -> "+c.To, c.Side)
	case changelog.FileAdded:
		return "add " + c.Path
	case changelog.FileRemoved:
		return "remove " + c.Path
	default:
		return "change " + c.Path
	}
}

func modLine(verb, name, version, side string) string {
	if side == "" {
		side = "both"
	}
	parts := []string{verb, name}
	if version != "" {
		parts = append(parts, version)
	}
	parts = append(parts, "("+side+")")
	return strings.Join(parts, " ")
}
