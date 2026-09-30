package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
)

// configData is what the config screen shows: the pack's config files by who claims each, and everyone who could claim one.
type configData struct {
	// pack is how the pack is named on screen: its name and version
	pack string
	tree core.ConfigFileTree
	// owners is everyone who can claim config files: the pack, then its mod loaders, then every mod in alphabetical order
	// of their names
	owners []owner
	// configDir is the folder the pack keeps its files in (see core.ConfigDirResolver), or "" if it has none
	configDir string
}

// relateResult says what relating config files to owners changed.
type relateResult struct {
	// added is how many entries were newly recorded, counting one for each owner they were recorded for
	added int
	// existing is how many were already recorded, counted the same way
	existing int
	// owners is how many owners had something recorded for them
	owners int
}

// configBackend is what the config screen needs of the pack it works on. It is an interface so the screen can be
// tested without one, but the pack is the one in the current directory, as for every other command (see packBackend).
//
// Config files are given by the entries of a config-files (see core.ConfigEntry). Each method reads what it needs from
// disk when it is called, rather than keeping what an earlier one read, so it works on the pack as it is now, whoever
// else has changed it.
type configBackend interface {
	// load reads the pack's config files, as "packwiz config list" does.
	load() (configData, error)
	// relate records that each of the owners owns each of the entries, as "packwiz config relate" does.
	relate(owners []owner, entries []string) (relateResult, error)
	// unrelate takes each of the entries out of the owner's config-files.
	unrelate(o owner, entries []string) error
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
	tree, err := index.ConfigFileTree(mods, pack)
	if err != nil {
		return configData{}, err
	}

	var owners []owner
	for _, key := range pack.ConfigOwners() {
		owners = append(owners, packOwner(pack, key))
	}
	sorted := slices.Clone(mods)
	slices.SortFunc(sorted, func(a, b *core.Mod) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	for _, m := range sorted {
		owners = append(owners, modOwner(m))
	}
	return configData{
		pack:      strings.TrimSpace(pack.Name + " " + pack.Version),
		tree:      tree,
		owners:    owners,
		configDir: core.ConfigDirOfMods(mods),
	}, nil
}

func (packBackend) relate(owners []owner, entries []string) (relateResult, error) {
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
	modsChanged, packChanged := false, false
	for _, o := range owners {
		added := 0
		switch o.kind {
		case ownerPack, ownerLoader:
			for _, entry := range entries {
				if pack.ClaimConfigFile(o.id, entry) {
					added++
				} else {
					result.existing++
				}
			}
			packChanged = packChanged || added > 0
		case ownerMod:
			mod, err := core.LoadMod(o.id)
			if err != nil {
				saveErr = err
			} else {
				for _, entry := range entries {
					if mod.ClaimConfigFile(entry) {
						added++
					} else {
						result.existing++
					}
				}
				if added > 0 {
					if err := index.SaveMod(&mod); err != nil {
						saveErr = err
						added = 0
					} else {
						modsChanged = true
					}
				}
			}
		default:
			saveErr = fmt.Errorf("%q can't own config files", o.name)
		}
		if saveErr != nil {
			break
		}
		if added > 0 {
			result.added += added
			result.owners++
		}
	}

	// The mods that were saved are in the index, so it is written even if a later one failed: otherwise it would hold
	// hashes of their files that are out of date. The pack is written with it, as it records the index's hash, and what
	// the pack and its loader own is only in the pack, which isn't in the index
	switch {
	case modsChanged:
		if err := pack.SaveIndex(index); err != nil && saveErr == nil {
			saveErr = err
		}
	case packChanged:
		if err := pack.Write(); err != nil && saveErr == nil {
			saveErr = err
		}
	}
	return result, saveErr
}

func (packBackend) unrelate(o owner, entries []string) error {
	pack, err := core.LoadPack()
	if err != nil {
		return err
	}

	switch o.kind {
	case ownerPack, ownerLoader:
		removed := false
		for _, entry := range entries {
			if pack.UnclaimConfigFile(o.id, entry) {
				removed = true
			}
		}
		if !removed {
			// Someone else took them out since the screen was drawn, which is what was asked for
			return nil
		}
		return pack.Write()
	case ownerMod:
		index, err := pack.LoadIndex()
		if err != nil {
			return err
		}
		mod, err := core.LoadMod(o.id)
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
			return nil
		}
		if err := index.SaveMod(&mod); err != nil {
			return err
		}
		return pack.SaveIndex(index)
	}
	return fmt.Errorf("%q can't own config files", o.name)
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
