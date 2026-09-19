package changelog

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/evictedcucumber/packwiz/core"
)

// Snapshot is the contents of a pack at a point in time, for describing a pack that has no log to read its changes from:
// the first release is the difference between one and nothing, and a release made without a repository stores one, so
// the next release is the difference between that snapshot and the pack as it is then.
type Snapshot struct {
	// Mods holds every metadata file (mods, resource packs, etc.), keyed by its path relative to the index
	Mods map[string]SnapshotMod `toml:"mods"`
	// Files holds the SHA-256 hash of every other tracked file (configs, etc.), keyed by its path relative to the index.
	// These are hashed from the files themselves rather than taken from the index, which has no hashes in
	// no-internal-hashes mode.
	Files map[string]string `toml:"files"`
}

// SnapshotMod is the part of a metadata file that a changelog cares about.
type SnapshotMod struct {
	Name string `toml:"name"`
	// Side is always explicit here: metadata files with no side are recorded as core.UniversalSide
	Side string `toml:"side"`
	// Version is core.Mod.DisplayVersion, so it is never empty for a well-formed metadata file. It is the mod's file
	// name if its version isn't known, which Diff allows for.
	Version string `toml:"version"`
	// File is the name of the file the mod installs. It is only known for a snapshot of the pack as it is, not for
	// one read back from the history, which doesn't store it.
	File string `toml:"-"`
}

// NewSnapshotMod summarises a mod for a snapshot.
func NewSnapshotMod(mod core.Mod) SnapshotMod {
	side := mod.Side
	if side == core.EmptySide {
		side = core.UniversalSide
	}
	return SnapshotMod{Name: mod.Name, Side: side, Version: mod.DisplayVersion(), File: mod.FileName}
}

// OpenFunc opens a pack file for reading, given its path relative to the index. It must return an error wrapping
// fs.ErrNotExist for a file that doesn't exist.
type OpenFunc func(indexPath string) (io.ReadCloser, error)

// TakeSnapshot reads every file tracked by the index from disk. The index should be up to date (see Index.Refresh),
// as it decides which files exist and which of them are metadata files. versions are versions found for mods that
// don't record one (see Index.ResolveMissingVersions), by path relative to the index, and may be nil.
func TakeSnapshot(index core.Index, versions map[string]string) (Snapshot, error) {
	return TakeSnapshotFrom(index, func(path string) (io.ReadCloser, error) {
		return os.Open(index.ResolveIndexPath(path))
	}, versions)
}

// TakeSnapshotFrom is TakeSnapshot for a pack that isn't (only) on disk, such as an old version of it in version
// control. Files listed by the index but missing from open are treated as not being part of the pack, as an index
// can be out of date.
func TakeSnapshotFrom(index core.Index, open OpenFunc, versions map[string]string) (Snapshot, error) {
	snap := Snapshot{
		Mods:  make(map[string]SnapshotMod),
		Files: make(map[string]string),
	}
	for path, entry := range index.Files {
		if entry.IsMetaFile() {
			mod, err := readMod(open, path)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return Snapshot{}, fmt.Errorf("failed to read metadata file %s: %w", path, err)
			}
			if version, ok := versions[path]; ok && mod.Version == "" {
				mod.Version = version
			}
			snap.Mods[path] = NewSnapshotMod(mod)
			continue
		}
		hash, err := hashFile(open, path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return Snapshot{}, fmt.Errorf("failed to hash %s: %w", path, err)
		}
		snap.Files[path] = hash
	}
	return snap, nil
}

func readMod(open OpenFunc, path string) (core.Mod, error) {
	r, err := open(path)
	if err != nil {
		return core.Mod{}, err
	}
	defer func() { _ = r.Close() }()

	data, err := io.ReadAll(r)
	if err != nil {
		return core.Mod{}, err
	}
	return core.DecodeMod(data)
}

func hashFile(open OpenFunc, path string) (string, error) {
	r, err := open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()

	h, err := core.GetHashImpl("sha256")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return h.HashToString(h.Sum(nil)), nil
}
