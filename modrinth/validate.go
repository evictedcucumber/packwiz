package modrinth

import (
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
	"github.com/spf13/cobra"
)

// validateCmd represents the validate command
var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check that the mods are valid, have their dependencies, and are on the sides they need to be",
	Long: `Check the pack, without changing it, for what would go wrong once it is exported or played:

  - every mod's metadata file can be read and has what it needs: a name, a file name, a download and its hash, a side,
    and the Modrinth project and version it came from
  - no two mods are the same Modrinth project, or install to the same file
  - every required dependency is in the pack, and nothing in it is incompatible with something else
  - every mod that a mod on the client requires is on the client too (see 'packwiz modrinth add')
  - pack.toml has a Minecraft version, a NeoForge version and a version for the pack
  - every tracked config file is claimed by a mod's config-files (see 'packwiz config list --state invalid')

What a mod depends on is what its metadata records. For a mod that records nothing it is looked up on Modrinth, which
needs the network; if that fails, those mods' dependencies aren't checked, and it says so.

The command fails if it finds errors. Warnings are about things that don't stop the pack from working, and don't.

'packwiz modrinth fix' fixes what can be fixed of what this finds.`,
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

		ui.Muted.Println("Checking the pack...")
		v := validatePack(pack, index)
		v.print()
		if v.errors() > 0 {
			os.Exit(1)
		}
	},
}

type severity int

const (
	severityWarning severity = iota
	severityError
)

// problem is something wrong with a mod, or with the pack
type problem struct {
	severity severity
	message  string
}

// subject is a metadata file, or the pack, and what is wrong with it
type subject struct {
	name     string
	path     string
	problems []problem
}

// entry is a metadata file the pack has: where it is in the index, and what is in it, if it can be read
type entry struct {
	path string
	name string
	mod  *core.Mod
}

// packEntry is what problems with the pack as a whole are about, rather than one of its mods
var packEntry = entry{name: "The pack"}

// dependencyNeed is a mod that requires a project that the pack doesn't have
type dependencyNeed struct {
	by entry
	id string
}

// validation is what was found wrong with a pack. Nothing is saved: the mods it reads are only changed in memory, to
// have what Modrinth says they depend on.
type validation struct {
	// mods is how many metadata files the pack has, however many of them could be read
	mods int
	// subjects are the metadata files, and the pack under "", that have anything wrong with them, by path
	subjects map[string]*subject
	// entries are the metadata files that could be read, in the order of their paths, with the dependencies that
	// were looked up, and missing is what they require that isn't in the pack: what fixing the pack goes by
	entries []entry
	missing []dependencyNeed
}

// validatePack checks a pack for problems, looking up on Modrinth what mods that record no dependencies depend on
func validatePack(pack core.Pack, index core.Index) *validation {
	v := &validation{subjects: make(map[string]*subject)}
	v.checkPack(pack)

	entries := v.readMods(index)
	v.entries = entries
	for _, e := range entries {
		v.checkMod(e)
	}
	v.checkDuplicates(entries)
	v.checkDependencies(pack, entries)
	v.checkConfigFiles(index, entries)
	return v
}

func (v *validation) add(e entry, sev severity, format string, args ...any) {
	s := v.subjects[e.path]
	if s == nil {
		s = &subject{name: e.name, path: e.path}
		v.subjects[e.path] = s
	}
	s.problems = append(s.problems, problem{severity: sev, message: fmt.Sprintf(format, args...)})
}

func (v *validation) errorf(e entry, format string, args ...any) {
	v.add(e, severityError, format, args...)
}

func (v *validation) warnf(e entry, format string, args ...any) {
	v.add(e, severityWarning, format, args...)
}

// count is how many problems of a severity there are
func (v *validation) count(sev severity) int {
	n := 0
	for _, s := range v.subjects {
		for _, p := range s.problems {
			if p.severity == sev {
				n++
			}
		}
	}
	return n
}

func (v *validation) errors() int   { return v.count(severityError) }
func (v *validation) warnings() int { return v.count(severityWarning) }

func (v *validation) checkPack(pack core.Pack) {
	if _, err := pack.GetMCVersion(); err != nil {
		v.errorf(packEntry, "has no Minecraft version (versions.minecraft in pack.toml)")
	}
	if len(pack.GetLoaders()) == 0 {
		v.errorf(packEntry, "has no NeoForge version (versions.neoforge in pack.toml), so an exported pack has no mod loader")
	}
	if pack.Version == "" {
		v.warnf(packEntry, "has no version (version in pack.toml), which a Modrinth pack needs")
	}
}

// readMods reads the metadata files the index lists, in the order of their paths, reporting the ones that can't be read
func (v *validation) readMods(index core.Index) []entry {
	var entries []entry
	for _, path := range slices.Sorted(maps.Keys(index.Files)) {
		if !index.Files[path].IsMetaFile() {
			continue
		}
		v.mods++
		mod, err := core.LoadMod(index.ResolveIndexPath(path))
		if err != nil {
			v.errorf(entry{path: path, name: path}, "can't be read: %v", err)
			continue
		}
		name := mod.Name
		if name == "" {
			name = path
		}
		entries = append(entries, entry{path: path, name: name, mod: &mod})
	}
	return entries
}

// checkMod checks that a mod's metadata has what the pack needs of it
func (v *validation) checkMod(e entry) {
	mod := e.mod

	if mod.Name == "" {
		v.warnf(e, "has no name")
	}
	if mod.FileName == "" {
		v.errorf(e, "has no file name")
	} else if !filepath.IsLocal(filepath.FromSlash(mod.FileName)) {
		v.errorf(e, "file name %q would put the mod outside the folder its metadata is in", mod.FileName)
	}

	switch mod.Download.Mode {
	case "", core.ModeURL:
		if mod.Download.URL == "" {
			v.errorf(e, "has no download URL")
		} else if u, err := url.Parse(mod.Download.URL); err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			v.errorf(e, "download URL %q isn't a web address", mod.Download.URL)
		}
	default:
		v.errorf(e, "download mode %q isn't supported, only %q is", mod.Download.Mode, core.ModeURL)
	}
	if mod.Download.HashFormat == "" {
		v.errorf(e, "has no hash format")
	} else if _, err := core.GetHashImpl(mod.Download.HashFormat); err != nil {
		v.errorf(e, "hash format %q isn't known", mod.Download.HashFormat)
	}
	if mod.Download.Hash == "" {
		v.errorf(e, "has no hash")
	}

	switch mod.Side {
	case core.ClientSide, core.ServerSide, core.UniversalSide:
	case core.EmptySide:
		v.warnf(e, "has no side, so it is on both")
	default:
		v.errorf(e, "side %q isn't one of %s, %s or %s", mod.Side, core.ClientSide, core.ServerSide, core.UniversalSide)
	}

	if data, ok := modrinthUpdateData(mod); !ok {
		v.warnf(e, "isn't from Modrinth, so it can't be updated and its dependencies can't be checked")
	} else {
		if data.ProjectID == "" {
			v.errorf(e, "has no Modrinth project ID (mod-id)")
		}
		if data.InstalledVersion == "" {
			v.errorf(e, "has no Modrinth version ID (version)")
		}
	}

	if mod.Version == "" {
		v.warnf(e, "doesn't record its version; 'packwiz modrinth fix' and 'packwiz git commit' save it")
	}
}

// checkDuplicates finds mods that are the same Modrinth project as an earlier one, or install to the same file
func (v *validation) checkDuplicates(entries []entry) {
	projects := make(map[string]entry)
	files := make(map[string]entry)
	for _, e := range entries {
		if data, ok := modrinthUpdateData(e.mod); ok && data.ProjectID != "" {
			if first, exists := projects[data.ProjectID]; exists {
				v.errorf(e, "is the same Modrinth project as %s", first.path)
			} else {
				projects[data.ProjectID] = e
			}
		}
		if e.mod.FileName == "" {
			continue
		}
		dest := e.mod.GetDestFilePath()
		if first, exists := files[dest]; exists {
			v.errorf(e, "installs to the same file as %s", first.path)
		} else {
			files[dest] = e
		}
	}
}

// checkDependencies finds required dependencies that aren't in the pack, mods that are incompatible with others in it,
// and mods that are only on the server but are needed on the client
func (v *validation) checkDependencies(pack core.Pack, entries []entry) {
	// Only the mods that come from Modrinth have dependencies
	var managed []entry
	for _, e := range entries {
		if projectIDOf(e.mod) != "" {
			managed = append(managed, e)
		}
	}
	installed := installedProjects(pack, entries)

	v.lookUpDependencies(managed)

	var missing []dependencyNeed
	var missingIDs []string
	for _, e := range managed {
		for _, dep := range e.mod.Dependencies {
			name, present := installed[dep.ID]
			switch {
			case dep.Type == "required" && !present:
				missing = append(missing, dependencyNeed{by: e, id: dep.ID})
				missingIDs = append(missingIDs, dep.ID)
			case dep.Type == "incompatible" && present:
				v.errorf(e, "is incompatible with %q, which is in the pack", name)
			}
		}
	}
	v.missing = missing
	names := projectNames(missingIDs)
	for _, m := range missing {
		v.errorf(m.by, "requires %q, which isn't in the pack", names[m.id])
	}

	mods := make([]*core.Mod, len(managed))
	byMod := make(map[*core.Mod]entry, len(managed))
	for i, e := range managed {
		mods[i] = e.mod
		byMod[e.mod] = e
	}
	for _, p := range sidePromotions(mods) {
		v.errorf(byMod[p.mod], "is only on the server, but %q needs it on the client; its side should be %q", p.neededBy.Name, core.UniversalSide)
	}
}

// checkConfigFiles warns about tracked files that no mod's config-files claims: normally something in the pack's
// config/ folder left behind by a mod that has since been removed, or never linked to the mod that installed it.
func (v *validation) checkConfigFiles(index core.Index, entries []entry) {
	mods := make([]*core.Mod, len(entries))
	for i, e := range entries {
		mods[i] = e.mod
	}
	tree, err := index.ConfigFileTree(mods)
	if err != nil {
		v.warnf(packEntry, "config files couldn't be checked: %v", err)
		return
	}

	if len(tree.Unclaimed) == 0 {
		return
	}
	noun, verb := "files", "aren't"
	if len(tree.Unclaimed) == 1 {
		noun, verb = "file", "isn't"
	}
	v.warnf(packEntry, "%d %s %s claimed by any mod's config-files: %s", len(tree.Unclaimed), noun, verb, listNames(tree.Unclaimed, 5))
}

// installedProjects is the projects the pack has, by ID, with the name of the mod that is each. Forgified Fabric API
// takes the place of Fabric API, so a pack that runs Fabric mods has that too.
func installedProjects(pack core.Pack, entries []entry) map[string]string {
	installed := make(map[string]string)
	for _, e := range entries {
		if id := projectIDOf(e.mod); id != "" {
			installed[id] = e.name
		}
	}
	if runsFabricMods(pack, slices.Collect(maps.Keys(installed))) {
		installed[fabricAPIProjectID] = installed[forgifiedFabricAPIProjectID]
	}
	return installed
}

// lookUpDependencies gives the mods that record no dependencies what Modrinth says their installed version depends on,
// as a mod that depends on nothing can't be told from one that was added before dependencies were recorded. Those that
// can't be looked up are reported together, as there is usually one reason for all of them.
func (v *validation) lookUpDependencies(entries []entry) {
	var unrecorded []entry
	var versionIDs []string
	for _, e := range entries {
		if data, _ := modrinthUpdateData(e.mod); len(e.mod.Dependencies) == 0 && data.InstalledVersion != "" {
			unrecorded = append(unrecorded, e)
			versionIDs = append(versionIDs, data.InstalledVersion)
		}
	}
	if len(unrecorded) == 0 {
		return
	}
	slices.Sort(versionIDs)
	versionIDs = slices.Compact(versionIDs)

	versions := make(map[string]*modrinthApi.Version, len(versionIDs))
	var lookupErr error
	for batch := range slices.Chunk(versionIDs, versionLookupBatchSize) {
		found, err := mrDefaultClient.Versions.GetMultiple(batch)
		if err != nil {
			lookupErr = err
			continue
		}
		for _, version := range found {
			if version != nil && version.ID != nil {
				versions[*version.ID] = version
			}
		}
	}

	var unchecked []string
	for _, e := range unrecorded {
		data, _ := modrinthUpdateData(e.mod)
		version, ok := versions[data.InstalledVersion]
		if !ok {
			unchecked = append(unchecked, e.name)
			continue
		}
		e.mod.Dependencies = buildDependencyList(version)
	}
	if len(unchecked) > 0 {
		reason := ""
		if lookupErr != nil {
			reason = fmt.Sprintf(" (%v)", lookupErr)
		}
		v.warnf(packEntry, "dependencies not checked for %d %s, which couldn't be looked up on Modrinth%s: %s",
			len(unchecked), plural(len(unchecked), "mod"), reason, listNames(unchecked, 5))
	}
}

// listNames lists names, up to limit of them, and how many more there are
func listNames(names []string, limit int) string {
	if len(names) <= limit {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:limit], ", "), len(names)-limit)
}

// projectNames finds the names of Modrinth projects, for showing instead of their IDs. A project it can't find the name
// of is shown by its ID.
func projectNames(ids []string) map[string]string {
	slices.Sort(ids)
	ids = slices.Compact(ids)
	names := make(map[string]string, len(ids))
	for _, id := range ids {
		names[id] = id
	}
	if len(ids) == 0 {
		return names
	}
	if projects, err := mrDefaultClient.Projects.GetMultiple(ids); err == nil {
		for _, p := range projects {
			if p.ID != nil && p.Title != nil {
				names[*p.ID] = *p.Title
			}
		}
	}
	return names
}

// print says what was found, by mod, and how much of it
func (v *validation) print() {
	for _, path := range slices.Sorted(maps.Keys(v.subjects)) {
		s := v.subjects[path]
		heading := ui.Bold.Sprint(s.name)
		if s.path != "" && s.path != s.name {
			heading += " " + ui.Muted.Sprintf("(%s)", s.path)
		}
		fmt.Println(heading + ":")

		// Errors first, then warnings, each in the order they were found
		for _, sev := range []severity{severityError, severityWarning} {
			for _, p := range s.problems {
				if p.severity != sev {
					continue
				}
				if sev == severityError {
					ui.Error.Printf("  error: %s\n", p.message)
				} else {
					ui.Warning.Printf("  warning: %s\n", p.message)
				}
			}
		}
		fmt.Println()
	}

	errs, warns := v.errors(), v.warnings()
	switch {
	case errs > 0:
		ui.Error.Printf("Found %d %s and %d %s in %d %s.\n", errs, plural(errs, "error"), warns, plural(warns, "warning"), v.mods, plural(v.mods, "mod"))
	case warns > 0:
		ui.Warning.Printf("No errors, but %d %s.\n", warns, plural(warns, "warning"))
	case v.mods == 0:
		ui.Info.Println("The pack has no mods to check.")
	case v.mods == 1:
		ui.Success.Println("The mod is valid!")
	default:
		ui.Success.Printf("All %d mods are valid!\n", v.mods)
	}
}

func init() {
	modrinthCmd.AddCommand(validateCmd)
}
