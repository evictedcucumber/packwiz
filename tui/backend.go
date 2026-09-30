package tui

import (
	"slices"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
)

// configData is what the config screen shows: the pack's config files by the mod that claims each, and every mod that
// could claim one.
type configData struct {
	// pack is how the pack is named on screen: its name and version
	pack string
	tree core.ConfigFileTree
	// mods is every mod in the pack, in alphabetical order of their names
	mods []*core.Mod
	// configDir is the folder the pack keeps its files in (see core.ConfigDirResolver), or "" if it has none
	configDir string
}

// relateResult says what relating config files to mods changed.
type relateResult struct {
	// added is how many entries were newly recorded, counting one for each mod they were recorded in
	added int
	// existing is how many were already recorded, counted the same way
	existing int
	// mods is how many mods' metadata files were changed
	mods int
}

// configBackend is what the config screen needs of the pack it works on. It is an interface so the screen can be
// tested without one, but the pack is the one in the current directory, as for every other command (see packBackend).
//
// Mods are given by the path of their metadata file (core.Mod.GetFilePath), and config files by the entries of their
// ConfigFiles (see core.ConfigEntry). Each method reads what it needs from disk when it is called, rather than keeping
// what an earlier one read, so it works on the pack as it is now, whoever else has changed it.
type configBackend interface {
	// load reads the pack's config files, as "packwiz config list" does.
	load() (configData, error)
	// relate records that each of the mods owns each of the entries, as "packwiz config relate" does.
	relate(modPaths, entries []string) (relateResult, error)
	// unrelate takes each of the entries out of the mod's config-files.
	unrelate(modPath string, entries []string) error
	// refresh brings the index up to date with the files on disk, as "packwiz refresh" does, so files that have been
	// added or deleted show up. It writes nothing to the terminal: what the command would say of it comes back as
	// notices, in plain text.
	refresh() (notices []string, err error)
}

// packBackend is the configBackend for the pack in the current directory. It does its work with core, the same as the
// commands do, so what it writes is what they write.
type packBackend struct{}

func (packBackend) load() (configData, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return configData{}, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return configData{}, err
	}
	mods, err := index.LoadAllMods()
	if err != nil {
		return configData{}, err
	}
	tree, err := index.ConfigFileTree(mods)
	if err != nil {
		return configData{}, err
	}

	sorted := slices.Clone(mods)
	slices.SortFunc(sorted, func(a, b *core.Mod) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return configData{
		pack:      strings.TrimSpace(pack.Name + " " + pack.Version),
		tree:      tree,
		mods:      sorted,
		configDir: core.ConfigDirOfMods(mods),
	}, nil
}

func (packBackend) relate(modPaths, entries []string) (relateResult, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return relateResult{}, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return relateResult{}, err
	}

	var result relateResult
	var saveErr error
	for _, path := range modPaths {
		mod, err := core.LoadMod(path)
		if err != nil {
			saveErr = err
			break
		}
		added := 0
		for _, entry := range entries {
			if mod.ClaimConfigFile(entry) {
				added++
			} else {
				result.existing++
			}
		}
		if added == 0 {
			continue
		}
		if err := index.SaveMod(&mod); err != nil {
			saveErr = err
			break
		}
		result.added += added
		result.mods++
	}

	// The mods that were saved are in the index, so it is written even if a later one failed: otherwise it would hold
	// hashes of their files that are out of date
	if result.mods > 0 {
		if err := pack.SaveIndex(index); err != nil && saveErr == nil {
			saveErr = err
		}
	}
	return result, saveErr
}

func (packBackend) unrelate(modPath string, entries []string) error {
	pack, err := core.LoadPack()
	if err != nil {
		return err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return err
	}
	mod, err := core.LoadMod(modPath)
	if err != nil {
		return err
	}

	removed := false
	for _, entry := range entries {
		if mod.UnclaimConfigFile(entry) {
			removed = true
		}
	}
	if !removed {
		// Someone else took them out since the screen was drawn, which is what was asked for
		return nil
	}
	if err := index.SaveMod(&mod); err != nil {
		return err
	}
	return pack.SaveIndex(index)
}

func (packBackend) refresh() ([]string, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return nil, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return nil, err
	}
	notices, err := index.RefreshQuietly()
	if err != nil {
		return nil, err
	}
	return notices, pack.SaveIndex(index)
}
