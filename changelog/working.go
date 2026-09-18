package changelog

import (
	"fmt"

	"github.com/evictedcucumber/packwiz/core"
)

// Working is the pack as it is on disk now.
type Working struct {
	Pack  core.Pack
	Index core.Index
	// Versions are the versions found for mods that don't record one, by path relative to the index. Nothing has been
	// written, so a command that saves the pack should save these too, with Index.RecordVersions.
	Versions map[string]string
}

// LoadWorking loads the pack and its index, refreshed in memory so files added or edited since the last
// "packwiz refresh" are noticed, and looks up the versions of any mods that don't record one (which, for Modrinth
// mods, needs the network).
//
// If the versions can't be looked up, strict decides whether that is an error. If it isn't, it is only a warning, and
// the mods are described by their file names instead. A command that is going to save what it finds should be
// strict, so file names aren't saved in place of versions.
func LoadWorking(strict bool) (Working, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return Working{}, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return Working{}, err
	}
	if err := index.Refresh(); err != nil {
		return Working{}, err
	}

	versions, err := index.ResolveMissingVersions()
	if err != nil {
		if strict {
			return Working{}, fmt.Errorf("couldn't look up the versions of mods that don't record one: %w", err)
		}
		fmt.Printf("Warning: couldn't look up the versions of mods that don't record one (%v); they are shown by file name.\n", err)
	}
	return Working{pack, index, versions}, nil
}

// Snapshot describes the pack as it is, with the versions that were looked up.
func (w Working) Snapshot() (Snapshot, error) {
	return TakeSnapshot(w.Index, w.Versions)
}
