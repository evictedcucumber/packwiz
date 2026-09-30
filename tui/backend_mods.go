package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/modrinth"
)

// modDep is a dependency of a mod, as the mod's metadata records it.
type modDep struct {
	// name is what the pack calls the mod that it is, if the pack has it, and else its ID at its source: only the pack
	// is asked, so nothing needs the network
	name string
	// kind is how it is needed: "required", "optional", "incompatible" and so on
	kind   string
	inPack bool
}

// modRow is a mod of the pack, as the mods screen shows it: what its metadata file says, and what is worked out from it.
type modRow struct {
	// path is where the mod's metadata file is, which is what identifies it
	path string
	// slug is the name of that file without its extension
	slug string
	name string
	// version is the mod's version if it records it, and else the name of its file (see core.Mod.DisplayVersion)
	version string
	// file is the file that the mod installs
	file string
	// side is where the mod runs, which is never empty: a mod with no side is on both
	side       string
	pinned     bool
	dependency bool
	optional   bool
	// updaters are the sources that can update it
	updaters []string
	// project is the ID of the project the mod is from at its source, if it is from Modrinth
	project string
	// configEntries is how many entries its config-files has
	configEntries int
	deps          []modDep
}

// modsData is what the mods screen shows.
type modsData struct {
	// pack is how the pack is named on screen: its name and version
	pack string
	// rows are the mods, in alphabetical order of their names
	rows []modRow
}

// modsBackend is what the mods screen needs of the pack it works on, as configBackend is for the config screen: each
// method reads the pack when it is called, so it works on the pack as it is then.
type modsBackend interface {
	// loadMods reads the pack's mods, as "packwiz list" does.
	loadMods() (modsData, error)
	// setPinned pins or unpins a mod, as "packwiz pin" and "packwiz unpin" do.
	setPinned(path string, pinned bool) error
	// setDependency marks a mod as a dependency of others, or as a main mod, as "packwiz mark-dependency" and
	// "packwiz unmark-dependency" do.
	setDependency(path string, dependency bool) error
	// saveList writes the markdown list of the mods, as "packwiz list --save" does, and says where it wrote it.
	saveList() (string, error)
	// refresh brings the index up to date with the files on disk, as "packwiz refresh" does (see configBackend).
	refresh() (notices []string, err error)
	updatesBackend
}

func (packBackend) loadMods() (modsData, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return modsData{}, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return modsData{}, err
	}
	mods, err := index.LoadAllMods()
	if err != nil {
		return modsData{}, err
	}
	cmd.SortMods(mods)

	// What the pack calls the projects that it has, for the dependencies of the mods that need them
	names := make(map[string]string, len(mods))
	for _, m := range mods {
		if id := modrinth.ProjectID(m); id != "" {
			names[id] = m.Name
		}
	}

	rows := make([]modRow, len(mods))
	for i, m := range mods {
		rows[i] = newModRow(m, names)
	}
	return modsData{pack: packTitle(pack), rows: rows}, nil
}

// packTitle is how a pack is named on screen: its name and version.
func packTitle(pack core.Pack) string {
	return strings.TrimSpace(pack.Name + " " + pack.Version)
}

func newModRow(m *core.Mod, projectNames map[string]string) modRow {
	side := m.Side
	if side == core.EmptySide {
		side = core.UniversalSide
	}
	row := modRow{
		path: m.GetFilePath(), slug: slugOf(m), name: m.Name, version: m.DisplayVersion(), file: m.FileName, side: side,
		pinned: m.Pin, dependency: m.AddedAsDependency, optional: m.Option != nil && m.Option.Optional,
		updaters: slices.Sorted(maps.Keys(m.Update)), project: modrinth.ProjectID(m), configEntries: len(m.ConfigEntries()),
	}
	if row.name == "" {
		row.name = row.slug
	}
	for _, d := range m.Dependencies {
		name, inPack := projectNames[d.ID]
		if !inPack {
			name = d.ID
		}
		row.deps = append(row.deps, modDep{name: name, kind: d.Type, inPack: inPack})
	}
	return row
}

// changeMod reads a mod's metadata file, has change make its change to it, and saves it with the index and pack that
// follow from it, as the commands that change one mod do.
func changeMod(path string, change func(*core.Mod)) error {
	pack, err := core.LoadPack()
	if err != nil {
		return err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return err
	}
	mod, err := core.LoadMod(path)
	if err != nil {
		return err
	}
	change(&mod)
	if err := index.SaveMod(&mod); err != nil {
		return err
	}
	return pack.SaveIndex(index)
}

func (packBackend) setPinned(path string, pinned bool) error {
	return changeMod(path, func(m *core.Mod) { m.Pin = pinned })
}

func (packBackend) setDependency(path string, dependency bool) error {
	return changeMod(path, func(m *core.Mod) { m.AddedAsDependency = dependency })
}

func (packBackend) saveList() (string, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return "", err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return "", err
	}
	mods, err := index.LoadAllMods()
	if err != nil {
		return "", err
	}
	cmd.SortMods(mods)
	path, err := cmd.SaveModList(pack, index, mods)
	if err != nil {
		return "", fmt.Errorf("failed to write the list: %w", err)
	}
	return path, nil
}
