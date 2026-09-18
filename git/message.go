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
// A commit covering several changes takes the type of the most significant one. A release is committed as
// "chore(release): X.Y.Z" and tagged "vX.Y.Z".
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

// ReleaseMessage is the message for the commit that records a release.
func ReleaseMessage(version string) string {
	return "chore(release): " + version
}

// TagName is the name of the tag for a release.
func TagName(version string) string {
	return "v" + version
}

// scope is "mods" or "config" if all the changes are to one or the other, and "pack" if it is both.
func scope(changes []changelog.Change) string {
	mods, files := false, false
	for _, c := range changes {
		if c.IsMod() {
			mods = true
		} else {
			files = true
		}
	}
	switch {
	case mods && files:
		return "pack"
	case mods:
		return "mods"
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
