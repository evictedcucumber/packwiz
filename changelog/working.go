package changelog

import (
	"fmt"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
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
	if err := refreshIndex(&index); err != nil {
		return Working{}, err
	}

	versions, err := index.ResolveMissingVersions()
	if err != nil {
		if strict {
			return Working{}, fmt.Errorf("couldn't look up the versions of mods that don't record one: %w", err)
		}
		notice.Warnf("Warning: couldn't look up the versions of mods that don't record one (%v); they are shown by file name.", err)
	}
	return Working{pack, index, versions}, nil
}

// refreshIndex brings the index up to date with the files, showing how it is getting on and what it finds on the terminal,
// unless whatever is running has the terminal to itself (see notice.Collecting), which is told of what it finds instead.
func refreshIndex(index *core.Index) error {
	if !notice.Collecting() {
		return index.Refresh()
	}
	notices, err := index.RefreshQuietly()
	for _, n := range notices {
		notice.Infof("%s", n)
	}
	return err
}

// Snapshot describes the pack as it is, with the versions that were looked up.
func (w Working) Snapshot() (Snapshot, error) {
	return TakeSnapshot(w.Index, w.Versions)
}
