package modrinth

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
)

// fixCmd represents the fix command
var fixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Check the pack, then fix what can be fixed, after showing the changes and asking",
	Long: `Check the pack as 'packwiz modrinth validate' does, then work out which of what it found can be fixed, show the
changes that would fix them, and ask before making any:

  - a mod that is only on the server, but that a mod on the client requires, is put on both sides
  - a required dependency that isn't in the pack is added, at its latest version, as 'packwiz modrinth add' would add
    it (and put on both sides if a mod on the client requires it)
  - a mod that has no side is given one (both, which is what it is treated as having)
  - a mod that doesn't record its version has it recorded
  - a mod that doesn't have a config-files gets an empty one, ready for 'packwiz config relate' to fill in

Everything else that is found needs a decision that only you can make, so it is left alone and reported. Once the
changes are made the pack is checked again, and the command fails if any errors are left.

The changes are always shown. With --yes they are made without asking.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pack, err := core.LoadPack()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		index, err := pack.LoadIndex()
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}

		remaining, err := runFix(pack, index)
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		if remaining.errors() > 0 {
			os.Exit(1)
		}
	},
}

// sideChange is a mod that is put on another side, and why
type sideChange struct {
	from, to string
	// neededBy is the mod that needs it on the client, or "" if it is only being given the side it was treated as having
	neededBy string
}

// fileChange is what fixing the pack does to one metadata file
type fileChange struct {
	// path is where the file is, as the index has it
	path, name string
	// create is a file to make, rather than one to change, and requiredBy the mod that requires the project it is of
	create     *core.Mod
	requiredBy string
	side       *sideChange
	// version is the version to record
	version string
	// addConfigFiles is whether to give the mod an empty config-files, as it doesn't have one yet
	addConfigFiles bool
}

// fixPlan is what fixing a pack would do
type fixPlan struct {
	// changes are the files it would make or change, in the order of their paths
	changes []*fileChange
	// skipped is what it could have done but couldn't, and why
	skipped []string
}

func (p *fixPlan) empty() bool {
	return len(p.changes) == 0
}

// runFix checks the pack, shows what could be done about what it finds, and makes those changes if it is told to. It
// returns what was found wrong with the pack after that: what it was to begin with, if nothing was changed.
func runFix(pack core.Pack, index core.Index) (*validation, error) {
	ui.Muted.Println("Checking the pack...")
	v := validatePack(pack, index)
	v.print()

	plan := planFixes(pack, index, v)
	if plan.empty() {
		for _, skipped := range plan.skipped {
			ui.Warning.Println(skipped)
		}
		if len(v.subjects) > 0 {
			ui.Info.Println("Nothing that was found can be fixed automatically.")
		}
		return v, nil
	}

	plan.print()
	if !cmdshared.PromptYesNo("Would you like to make these changes? [Y/n]: ") {
		ui.Warning.Println("Cancelled!")
		return v, nil
	}

	changed, err := plan.apply(&pack, &index)
	if changed > 0 {
		ui.Success.Printf("Changed %d %s.\n", changed, plural(changed, "file"))
		fmt.Println()
	}
	if err != nil {
		return v, err
	}

	index, err = pack.LoadIndex()
	if err != nil {
		return v, err
	}
	ui.Muted.Println("Checking the pack again...")
	after := validatePack(pack, index)
	after.print()
	return after, nil
}

// planFixes works out what to change to fix what validation found, which needs the network: to find what dependencies
// to add, and the versions of mods that don't record theirs. It changes nothing.
func planFixes(pack core.Pack, index core.Index, v *validation) *fixPlan {
	plan := &fixPlan{}
	changes := make(map[string]*fileChange)
	changeOf := func(path, name string) *fileChange {
		c := changes[path]
		if c == nil {
			c = &fileChange{path: path, name: name}
			changes[path] = c
			plan.changes = append(plan.changes, c)
		}
		return c
	}

	// It is worked out on copies, so that what validation read is as it was
	mods := make([]*core.Mod, len(v.entries))
	for i, e := range v.entries {
		mods[i] = e.mod
	}
	working, _ := copyMods(mods)
	entryOf := make(map[*core.Mod]entry, len(working))
	for i, mod := range working {
		entryOf[mod] = v.entries[i]
	}

	// Dependencies that are added join the mods that decide which sides things need to be on, as they may be needed on
	// the client too, and need in turn what isn't there
	added := plan.dependenciesToAdd(pack, index, v)
	all := append(slices.Clone(working), added...)
	for _, mod := range added {
		path, err := index.RelIndexPath(mod.GetFilePath())
		if err != nil {
			path = mod.GetFilePath()
		}
		c := changeOf(path, mod.Name)
		c.create = mod
		if requirer := requirerOf(mod, all); requirer != nil {
			c.requiredBy = requirer.Name
		}
	}

	natural := make(map[*core.Mod]string, len(added))
	for _, mod := range added {
		natural[mod] = mod.Side
	}
	for _, p := range widenSides(all) {
		neededBy := p.neededBy.Name
		if e, ok := entryOf[p.neededBy]; ok {
			neededBy = e.name
		}
		change := &sideChange{from: core.ServerSide, to: core.UniversalSide, neededBy: neededBy}
		if e, ok := entryOf[p.mod]; ok {
			changeOf(e.path, e.name).side = change
			continue
		}
		path, err := index.RelIndexPath(p.mod.GetFilePath())
		if err != nil {
			path = p.mod.GetFilePath()
		}
		change.from = natural[p.mod]
		changeOf(path, p.mod.Name).side = change
	}

	for _, e := range v.entries {
		if e.mod.Side == core.EmptySide {
			changeOf(e.path, e.name).side = &sideChange{from: core.EmptySide, to: core.UniversalSide}
		}
	}

	for _, e := range v.entries {
		if e.mod.ConfigFiles == nil {
			changeOf(e.path, e.name).addConfigFiles = true
		}
	}

	plan.planVersions(v, changeOf)

	slices.SortFunc(plan.changes, func(a, b *fileChange) int { return strings.Compare(a.path, b.path) })
	return plan
}

// requirerOf is the first of mods that requires the project mod is of
func requirerOf(mod *core.Mod, mods []*core.Mod) *core.Mod {
	id := projectIDOf(mod)
	for _, other := range mods {
		if other == mod {
			continue
		}
		for _, dep := range other.Dependencies {
			if dep.Type == "required" && dep.ID == id {
				return other
			}
		}
	}
	return nil
}

// dependenciesToAdd finds the metadata of the required dependencies that the pack doesn't have, and of what those
// require in turn, as adding them with 'packwiz modrinth add' would make it. What can't be found, or would replace a
// file that is there, is said in skipped and left out.
func (p *fixPlan) dependenciesToAdd(pack core.Pack, index core.Index, v *validation) []*core.Mod {
	var ids []string
	for _, need := range v.missing {
		ids = append(ids, need.id)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 {
		return nil
	}

	ui.Muted.Println("Finding dependencies to add...")
	installed := slices.Collect(maps.Keys(installedProjects(pack, v.entries)))
	found, err := findDependencies(pack, ids, nil, installed, runsFabricMods(pack, installed))
	if err != nil {
		p.skipped = append(p.skipped, fmt.Sprintf("Couldn't work out which dependencies to add: %v", err))
		return nil
	}

	var added []*core.Mod
	for _, d := range found {
		mod, err := newFileMeta(d.projectInfo, d.versionInfo, d.fileInfo, pack, "", true)
		if err != nil {
			p.skipped = append(p.skipped, fmt.Sprintf("Couldn't add %s: %v", *d.projectInfo.Title, err))
			continue
		}
		// Adding a project overwrites whatever is at its path, which is only right for a file that is that project's
		if _, err := os.Stat(mod.GetFilePath()); err == nil {
			p.skipped = append(p.skipped, fmt.Sprintf("Won't add %s: %s is already there", mod.Name, mod.GetFilePath()))
			continue
		}
		added = append(added, &mod)
	}
	return added
}

// planVersions plans recording the version of each mod that doesn't record its own, asking Modrinth for them
func (p *fixPlan) planVersions(v *validation, changeOf func(path, name string) *fileChange) {
	var unversioned []entry
	var mods []*core.Mod
	for _, e := range v.entries {
		if data, ok := modrinthUpdateData(e.mod); ok && e.mod.Version == "" && data.InstalledVersion != "" {
			unversioned = append(unversioned, e)
			mods = append(mods, e.mod)
		}
	}
	if len(mods) == 0 {
		return
	}

	versions, err := mrUpdater{}.ResolveVersions(mods)
	if err != nil {
		p.skipped = append(p.skipped, fmt.Sprintf("Couldn't look up the versions of %d %s: %v", len(mods), plural(len(mods), "mod"), err))
		return
	}
	for i, version := range versions {
		if version != "" {
			changeOf(unversioned[i].path, unversioned[i].name).version = version
		}
	}
}

// describe says what is done to a file, a line for each thing
func (c *fileChange) describe() []string {
	var lines []string
	if c.create != nil {
		what := c.create.FileName
		if c.create.Version != "" {
			what = fmt.Sprintf("version %s (%s)", c.create.Version, c.create.FileName)
		}
		line := "add: " + what
		if c.requiredBy != "" {
			line += fmt.Sprintf(", which %q requires", c.requiredBy)
		}
		lines = append(lines, line)
	}
	if c.side != nil {
		from := c.side.from
		if from == core.EmptySide {
			from = "(none)"
		}
		line := "side: " + ui.Transition(from, c.side.to)
		if c.side.neededBy != "" {
			line += fmt.Sprintf(", as %q needs it on the client", c.side.neededBy)
		} else {
			line += ", which is what a mod with no side is treated as having"
		}
		lines = append(lines, line)
	}
	if c.version != "" {
		lines = append(lines, "version: "+ui.Transition("(not recorded)", c.version))
	}
	if c.addConfigFiles {
		lines = append(lines, "config-files: added, empty")
	}
	return lines
}

// print shows the changes, by file, and what couldn't be planned
func (p *fixPlan) print() {
	for _, skipped := range p.skipped {
		ui.Warning.Println(skipped)
	}
	ui.Bold.Println("Changes to make:")
	for _, c := range p.changes {
		fmt.Println(ui.Bold.Sprint(c.name) + " " + ui.Muted.Sprintf("(%s)", c.path) + ":")
		for _, line := range c.describe() {
			fmt.Println("  " + line)
		}
	}
	fmt.Println()
}

// apply makes the changes, each file written once with everything that is done to it, and saves the index and the
// pack. A file that can't be changed doesn't stop the others from being; it returns how many were changed, and the
// errors of those that weren't.
func (p *fixPlan) apply(pack *core.Pack, index *core.Index) (int, error) {
	var errs []error
	changed := 0
	for _, c := range p.changes {
		mod := c.create
		if mod == nil {
			loaded, err := core.LoadMod(index.ResolveIndexPath(c.path))
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to read %s: %w", c.path, err))
				continue
			}
			mod = &loaded
			if c.side != nil {
				mod.Side = c.side.to
			}
			if c.version != "" && mod.Version == "" {
				mod.Version = c.version
			}
			if c.addConfigFiles {
				mod.EnsureConfigFiles()
			}
		}

		format, hash, err := mod.Write()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to save %s: %w", c.path, err))
			continue
		}
		if err := index.RefreshFileWithHash(mod.GetFilePath(), format, hash, true); err != nil {
			errs = append(errs, fmt.Errorf("failed to update %s in the index: %w", c.path, err))
			continue
		}
		changed++
	}

	// What was changed is saved even if something else wasn't, so that the index matches the files
	if changed > 0 {
		if err := index.Write(); err != nil {
			errs = append(errs, err)
		} else if err := pack.UpdateIndexHash(); err != nil {
			errs = append(errs, err)
		} else if err := pack.Write(); err != nil {
			errs = append(errs, err)
		}
	}
	return changed, errors.Join(errs...)
}

func init() {
	modrinthCmd.AddCommand(fixCmd)
}
