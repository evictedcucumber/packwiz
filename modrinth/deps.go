package modrinth

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Dependency is something that a mod needs, or can't be used with, and whether the pack has it.
type Dependency struct {
	// Name is what the pack calls it if it has it, else what Modrinth calls it, else its project ID
	Name string
	// Kind is how it is needed: "required", "optional", "incompatible" and so on
	Kind   string
	InPack bool
}

// DependencyGroup is the dependencies of one mod.
type DependencyGroup struct {
	Mod          string
	Dependencies []Dependency
}

// DependencyReport is what the pack's mods depend on, and whether the pack has it.
type DependencyReport struct {
	// Mods is how many of the pack's mods come from Modrinth, which is what has dependencies
	Mods int
	// Groups are the mods that have dependencies, in alphabetical order of their names
	Groups []DependencyGroup
	// MissingRequired is how many required dependencies the pack lacks, counted for each mod that needs one
	MissingRequired int
	// Fetched is how many mods had their dependencies looked up on Modrinth, as they had none recorded, and haven't had
	// them saved to the pack yet (see Save)
	Fetched int
	// FetchFailed are the names of the mods whose dependencies couldn't be looked up
	FetchFailed []string
	// Notices are what the lookups said as they went, in plain text
	Notices []string

	scan *dependencyScan
}

// dependencyScan is the pack's Modrinth mods, with the dependencies of those that recorded none fetched from Modrinth.
type dependencyScan struct {
	pack core.Pack
	// mods are the pack's mods that come from Modrinth, in alphabetical order of their names
	mods []*core.Mod
	// installed is the name of the mod that is each Modrinth project the pack has, by project ID
	installed map[string]string
	// fetched are the mods that had their dependencies fetched, which aren't saved, and failed those that couldn't be
	fetched []*core.Mod
	failed  []string
}

// scanDependencies reads the pack's mods, and fetches the dependencies of those that record none (or of all of them, with
// refresh) from Modrinth. Nothing is saved.
func scanDependencies(pack core.Pack, index core.Index, refresh bool) (*dependencyScan, error) {
	mods, err := index.LoadAllMods()
	if err != nil {
		return nil, err
	}

	// installedNames maps installed project IDs to the mod's local name
	scan := &dependencyScan{pack: pack, installed: make(map[string]string)}
	for _, mod := range mods {
		data, ok := mod.GetParsedUpdateData("modrinth")
		if !ok {
			continue
		}
		updateData := data.(mrUpdateData)
		if updateData.ProjectID == "" {
			continue
		}
		scan.installed[updateData.ProjectID] = mod.Name
		scan.mods = append(scan.mods, mod)
	}
	if len(scan.mods) == 0 {
		return scan, nil
	}

	// Forgified Fabric API takes the place of Fabric API, so what needs Fabric API has it
	if runsFabricMods(pack, slices.Collect(maps.Keys(scan.installed))) {
		scan.installed[fabricAPIProjectID] = scan.installed[forgifiedFabricAPIProjectID]
	}

	// Fetch dependency data for any mod that doesn't have it stored yet (or all mods, with refresh)
	for _, mod := range scan.mods {
		if len(mod.Dependencies) > 0 && !refresh {
			continue
		}
		data, _ := mod.GetParsedUpdateData("modrinth")
		updateData := data.(mrUpdateData)
		version, err := mrDefaultClient.Versions.Get(updateData.InstalledVersion)
		if err != nil {
			scan.failed = append(scan.failed, mod.Name)
			continue
		}
		mod.Dependencies = buildDependencyList(version)
		scan.fetched = append(scan.fetched, mod)
	}
	return scan, nil
}

// save writes the dependencies that were fetched into the metadata files of the mods they are of, and the index and pack
// that follow from that. It works on the pack as it is on disk now. A mod that can't be saved doesn't stop the others:
// what went wrong with each is returned, as failures, and err is what went wrong with the index or the pack.
func (s *dependencyScan) save() (failures []error, err error) {
	pack, err := core.LoadPack()
	if err != nil {
		return nil, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return nil, err
	}
	for _, fetched := range s.fetched {
		mod, err := core.LoadMod(fetched.GetFilePath())
		if err != nil {
			failures = append(failures, fmt.Errorf("Failed to save dependency data for %q: %v", fetched.Name, err))
			continue
		}
		mod.Dependencies = fetched.Dependencies
		format, hash, err := mod.Write()
		if err != nil {
			failures = append(failures, fmt.Errorf("Failed to save dependency data for %q: %v", mod.Name, err))
			continue
		}
		if err := index.RefreshFileWithHash(mod.GetFilePath(), format, hash, true); err != nil {
			failures = append(failures, fmt.Errorf("Failed to update index for %q: %v", mod.Name, err))
		}
	}
	return failures, pack.SaveIndex(index)
}

// report says what the mods depend on, looking up on Modrinth what the dependencies that the pack doesn't have are
// called.
func (s *dependencyScan) report() *DependencyReport {
	// Resolve display names for dependencies that aren't already in the pack
	var unresolvedIDs []string
	for _, mod := range s.mods {
		for _, dep := range mod.Dependencies {
			if _, ok := s.installed[dep.ID]; !ok {
				unresolvedIDs = append(unresolvedIDs, dep.ID)
			}
		}
	}
	slices.Sort(unresolvedIDs)
	unresolvedIDs = slices.Compact(unresolvedIDs)

	depNames := make(map[string]string)
	if len(unresolvedIDs) > 0 {
		projects, err := mrDefaultClient.Projects.GetMultiple(unresolvedIDs)
		if err != nil {
			notice.Warnf("Warning: failed to resolve dependency project names: %v", err)
		} else {
			for _, p := range projects {
				if p.ID != nil && p.Title != nil {
					depNames[*p.ID] = *p.Title
				}
			}
		}
	}

	mods := slices.Clone(s.mods)
	sort.Slice(mods, func(i, j int) bool {
		return strings.ToLower(mods[i].Name) < strings.ToLower(mods[j].Name)
	})

	report := &DependencyReport{Mods: len(mods), Fetched: len(s.fetched), FetchFailed: s.failed, scan: s}
	for _, mod := range mods {
		if len(mod.Dependencies) == 0 {
			continue
		}
		group := DependencyGroup{Mod: mod.Name}
		for _, dep := range mod.Dependencies {
			name, installed := s.installed[dep.ID]
			if !installed {
				// Only a dependency that is required is a problem when it is missing
				if dep.Type == "required" {
					report.MissingRequired++
				}
				if resolved, ok := depNames[dep.ID]; ok {
					name = resolved
				} else {
					name = dep.ID
				}
			}
			group.Dependencies = append(group.Dependencies, Dependency{Name: name, Kind: dep.Type, InPack: installed})
		}
		report.Groups = append(report.Groups, group)
	}
	return report
}

// Dependencies reports what the pack's Modrinth mods depend on, and whether the pack has each, as "packwiz modrinth deps"
// does. The dependencies of mods that record none are looked up on Modrinth, which needs the network, but nothing is
// saved: that is what Save does. It says nothing on the terminal.
func Dependencies(pack core.Pack, index core.Index, refresh bool) (*DependencyReport, error) {
	var report *DependencyReport
	var err error
	notices := notice.Collect(func() {
		var scan *dependencyScan
		if scan, err = scanDependencies(pack, index, refresh); err != nil {
			return
		}
		report = scan.report()
	})
	if err != nil {
		return nil, err
	}
	for _, n := range notices {
		if n.Level != notice.Muted {
			report.Notices = append(report.Notices, n.Text)
		}
	}
	return report, nil
}

// Save writes the dependencies that were looked up into the pack, so that they needn't be looked up again: what checks
// the pack and what puts mods on the sides they need to be on goes by them. It returns what couldn't be saved, as failures.
func (r *DependencyReport) Save() (failures []error, err error) {
	if r.scan == nil || len(r.scan.fetched) == 0 {
		return nil, nil
	}
	failures, err = r.scan.save()
	if err == nil {
		r.Fetched = 0
		r.scan.fetched = nil
	}
	return failures, err
}

// depsCmd represents the deps command
var depsCmd = &cobra.Command{
	Use:   "deps",
	Short: "List the dependencies of Modrinth-managed mods, and whether they are already added to the pack",
	Args:  cobra.NoArgs,
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

		scan, err := scanDependencies(pack, index, viper.GetBool("deps.refresh"))
		if err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
		if len(scan.mods) == 0 {
			ui.Info.Println("No Modrinth-managed mods found.")
			return
		}

		for _, name := range scan.failed {
			ui.Warning.Printf("Warning: failed to fetch dependency data for %q\n", name)
		}

		if len(scan.fetched) > 0 {
			ui.Info.Printf("Fetched dependency data for %d mod(s) from Modrinth.\n", len(scan.fetched))
			if cmdshared.PromptYesNo("Save this dependency data to the pack for faster future reports? [Y/n]: ") {
				failures, err := scan.save()
				for _, failure := range failures {
					ui.Error.Println(failure)
				}
				if err != nil {
					ui.Error.Println(err)
					os.Exit(1)
				}
			}
			fmt.Println()
		}

		report := scan.report()
		for _, group := range report.Groups {
			ui.Bold.Println(group.Mod + ":")
			for _, dep := range group.Dependencies {
				status := ui.Success.Sprint("already in pack")
				if !dep.InPack {
					// Only a dependency that is required is a problem when it is missing
					missing := ui.Warning
					if dep.Kind == "required" {
						missing = ui.Error
					}
					status = missing.Sprint("missing")
				}
				fmt.Printf("  %s %s (%s)\n", styleDependencyType(dep.Kind), dep.Name, status)
			}
		}

		if report.MissingRequired > 0 {
			ui.Warning.Printf("\n%d required dependencies are missing from the pack. Use 'packwiz mr add' to install them.\n", report.MissingRequired)
		} else {
			ui.Success.Println("\nAll required dependencies are already added to the pack!")
		}
	},
}

// styleDependencyType shows how a dependency is needed: one that is required as it is, one that can't be used with the
// mod stands out, and one that isn't needed fades back
func styleDependencyType(depType string) string {
	tag := "[" + depType + "]"
	switch depType {
	case "required":
		return tag
	case "incompatible":
		return ui.Error.Sprint(tag)
	}
	return ui.Muted.Sprint(tag)
}

func init() {
	modrinthCmd.AddCommand(depsCmd)

	depsCmd.Flags().BoolP("refresh", "r", false, "Re-fetch dependency data from Modrinth even for mods that already have it cached")
	_ = viper.BindPFlag("deps.refresh", depsCmd.Flags().Lookup("refresh"))
}
