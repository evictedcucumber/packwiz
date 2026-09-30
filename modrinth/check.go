package modrinth

import (
	"maps"
	"slices"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
)

// The commands validate and fix print what they find and ask questions as they go, which an interface that has the terminal
// to itself can't do. What follows is the same work with what it finds returned instead: Check is what "packwiz validate"
// finds, and Fixes what "packwiz fix" would do about it, and neither prints anything or asks anything.

// Problem is something wrong with a mod, or with the pack.
type Problem struct {
	// Error is whether it makes the pack fail to work, where a warning is about something that doesn't stop it working
	Error   bool
	Message string
}

// Finding is what is wrong with one metadata file, or with the pack as a whole.
type Finding struct {
	// Name is what the mod is called, or "The pack"
	Name string
	// Path is where the mod's metadata file is in the index, or "" for the pack
	Path string
	// Problems are the errors first and then the warnings, each in the order they were found
	Problems []Problem
}

// Check is what validating a pack found.
type Check struct {
	// Mods is how many metadata files the pack has, however many of them could be read
	Mods int
	// Findings are the mods, and the pack, that have anything wrong with them, in the order of their paths, the pack first
	Findings         []Finding
	Errors, Warnings int
	// Notices are what the lookups said as they went, in plain text, such as that a mod was added at its newest version
	// although another has a higher number
	Notices []string

	pack  core.Pack
	index core.Index
	v     *validation
}

// CheckPack checks the pack as "packwiz validate" does, looking up on Modrinth what mods that record no dependencies
// depend on, and changes nothing.
func CheckPack(pack core.Pack, index core.Index) *Check {
	var v *validation
	notices := notice.Collect(func() { v = validatePack(pack, index) })
	return newCheck(pack, index, v, notices)
}

func newCheck(pack core.Pack, index core.Index, v *validation, notices []notice.Notice) *Check {
	c := &Check{Mods: v.mods, Errors: v.errors(), Warnings: v.warnings(), pack: pack, index: index, v: v}
	for _, n := range notices {
		if n.Level != notice.Muted {
			c.Notices = append(c.Notices, n.Text)
		}
	}
	for _, path := range slices.Sorted(maps.Keys(v.subjects)) {
		s := v.subjects[path]
		f := Finding{Name: s.name, Path: s.path}
		// Errors first, then warnings, each in the order they were found
		for _, sev := range []severity{severityError, severityWarning} {
			for _, p := range s.problems {
				if p.severity == sev {
					f.Problems = append(f.Problems, Problem{Error: sev == severityError, Message: p.message})
				}
			}
		}
		c.Findings = append(c.Findings, f)
	}
	return c
}

// FixedFile is what fixing the pack does to one metadata file.
type FixedFile struct {
	Name string
	// Path is where the file is in the index
	Path string
	// Lines say what is done to it, a line for each thing
	Lines []string
}

// Fixes is what fixing a pack would do about what was found in it, which isn't done until Apply is called.
type Fixes struct {
	// Files are the metadata files that would be made or changed, in the order of their paths
	Files []FixedFile
	// Pack are the changes to pack.toml, a line for each
	Pack []string
	// Skipped is what could have been fixed but couldn't, and why
	Skipped []string
	// Notices are what the lookups said as they went, in plain text
	Notices []string

	plan *fixPlan
}

// PlanFixes works out what to change to fix what the check found, as "packwiz fix" does, which needs the network: to
// find what dependencies to add, and the versions of mods that don't record theirs. It changes nothing.
func (c *Check) PlanFixes() *Fixes {
	var plan *fixPlan
	notices := notice.Collect(func() { plan = planFixes(c.pack, c.index, c.v) })

	f := &Fixes{Skipped: plan.skipped, plan: plan}
	for _, n := range notices {
		if n.Level != notice.Muted {
			f.Notices = append(f.Notices, n.Text)
		}
	}
	for _, change := range plan.changes {
		f.Files = append(f.Files, FixedFile{Name: change.name, Path: change.path, Lines: change.describe()})
	}
	for _, change := range plan.packChanges {
		for _, entry := range change.remove {
			f.Pack = append(f.Pack, "config-files: remove "+entry+" from "+change.name+", which matches no file in the pack")
		}
	}
	return f
}

// Empty is whether there is nothing to do.
func (f *Fixes) Empty() bool { return f.plan.empty() }

// Apply makes the changes, each file written once with everything that is done to it, and saves the index and the pack.
// It works on the pack as it is on disk now, not as it was when the changes were planned, so what has been changed
// since is kept. A file that can't be changed doesn't stop the others from being: it returns how many were changed, and
// the errors of those that weren't. Check the pack again afterwards, with what is then on disk.
func (f *Fixes) Apply() (changed int, err error) {
	pack, err := core.LoadPack()
	if err != nil {
		return 0, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return 0, err
	}
	notice.Collect(func() { changed, err = f.plan.apply(&pack, &index) })
	return changed, err
}
