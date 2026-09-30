package tui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
)

// versionLine is one of the versions that a pack is for: Minecraft, or a mod loader.
type versionLine struct {
	name, version string
}

// overviewData is what the overview screen shows of the pack.
type overviewData struct {
	name, author, version, description string
	// versions are what the pack is for: Minecraft, then each mod loader, by the name they are written with for people
	versions  []versionLine
	indexFile string
	// files is how many files the index tracks
	files int
	// mods is how many of the files are metadata files, and of those how many were added as dependencies, how many are
	// pinned and how many run on each side
	mods, dependencies, pinned int
	sides                      map[string]int
	// config is how many config files are claimed, nothing claims, and are claimed but don't exist
	config stateCounts
}

// overviewBackend is what the overview screen needs of the pack.
type overviewBackend interface {
	// loadOverview reads what the screen shows, as "packwiz list" and "packwiz config list" do for their parts of it.
	loadOverview() (overviewData, error)
	// refresh brings the index up to date with the files on disk, as "packwiz refresh" does (see configBackend).
	refresh() (notices []string, err error)
}

func (packBackend) loadOverview() (overviewData, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return overviewData{}, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return overviewData{}, err
	}
	mods, err := index.LoadAllMods()
	if err != nil {
		return overviewData{}, fmt.Errorf("failed to read the pack's mods: %w", err)
	}
	tree, err := index.ConfigFileTree(mods, pack)
	if err != nil {
		return overviewData{}, err
	}

	data := overviewData{
		name: pack.Name, author: pack.Author, version: pack.Version, description: pack.Description,
		indexFile: pack.Index.File, files: len(index.Files), mods: len(mods),
		sides: make(map[string]int), config: countStates(tree),
	}
	for _, m := range mods {
		if m.AddedAsDependency {
			data.dependencies++
		}
		if m.Pin {
			data.pinned++
		}
		side := m.Side
		if side == core.EmptySide {
			side = core.UniversalSide
		}
		data.sides[side]++
	}

	// Minecraft first, as it is what everything else is for, then the rest in alphabetical order
	for _, name := range slices.SortedFunc(maps.Keys(pack.Versions), func(a, b string) int {
		if (a == "minecraft") != (b == "minecraft") {
			if a == "minecraft" {
				return -1
			}
			return 1
		}
		return cmp.Compare(a, b)
	}) {
		label := core.LoaderName(name)
		if name == "minecraft" {
			label = "Minecraft"
		}
		data.versions = append(data.versions, versionLine{label, pack.Versions[name]})
	}
	return data, nil
}

// describeMods says what mods the pack has: how many, and how they are made up.
func (d overviewData) describeMods() string {
	if d.mods == 0 {
		return "none yet"
	}
	text := fmt.Sprintf("%d", d.mods)
	var parts []string
	if main := d.mods - d.dependencies; main > 0 {
		parts = append(parts, fmt.Sprintf("%d main", main))
	}
	if d.dependencies > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", d.dependencies, pluralWord(d.dependencies, "dependency", "dependencies")))
	}
	if d.pinned > 0 {
		parts = append(parts, fmt.Sprintf("%d pinned", d.pinned))
	}
	if len(parts) > 0 {
		text += " (" + strings.Join(parts, ", ") + ")"
	}
	return text
}

// describeSides says where the mods run.
func (d overviewData) describeSides() string {
	var parts []string
	for _, side := range []string{core.UniversalSide, core.ClientSide, core.ServerSide} {
		if n := d.sides[side]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, side))
		}
	}
	return strings.Join(parts, ", ")
}
