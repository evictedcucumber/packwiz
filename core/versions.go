package core

import (
	"errors"
	"fmt"
	"maps"
	"slices"
)

// ResolveMissingVersions asks the updater of each mod in the index that doesn't record its version for it, and
// returns what it found by path relative to the index. A mod whose updater can't look versions up, or doesn't know
// the mod's version, is left out. Nothing is written; RecordVersions saves what this finds.
//
// If an updater fails, the versions found by the others are still returned, alongside the error.
func (in Index) ResolveMissingVersions() (map[string]string, error) {
	type unversioned struct {
		path string
		mod  *Mod
	}
	byUpdater := make(map[string][]unversioned)

	paths := make([]string, 0, len(in.Files))
	for p, entry := range in.Files {
		if entry.IsMetaFile() {
			paths = append(paths, p)
		}
	}
	// A stable order, so the same pack always asks its updaters the same question
	slices.Sort(paths)

	for _, p := range paths {
		mod, err := LoadMod(in.ResolveIndexPath(p))
		if err != nil {
			return nil, fmt.Errorf("failed to read metadata file %s: %w", p, err)
		}
		if mod.Version != "" {
			continue
		}
		// The first of the mod's updaters that can look versions up
		for _, name := range slices.Sorted(maps.Keys(mod.Update)) {
			if _, ok := Updaters[name].(VersionResolver); ok {
				byUpdater[name] = append(byUpdater[name], unversioned{p, &mod})
				break
			}
		}
	}

	found := make(map[string]string)
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(byUpdater)) {
		group := byUpdater[name]
		mods := make([]*Mod, len(group))
		for i, u := range group {
			mods[i] = u.mod
		}

		versions, err := Updaters[name].(VersionResolver).ResolveVersions(mods)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		if len(versions) != len(group) {
			errs = append(errs, fmt.Errorf("%s: returned %d versions for %d mods", name, len(versions), len(group)))
			continue
		}
		for i, v := range versions {
			if v != "" {
				found[group[i].path] = v
			}
		}
	}
	return found, errors.Join(errs...)
}

// RecordVersions saves versions, as found by ResolveMissingVersions, into the metadata files that lack them, and
// updates their entries in the index (which still needs to be written). A mod that has gained a version in the
// meantime keeps the one it has.
func (in *Index) RecordVersions(versions map[string]string) error {
	for _, p := range slices.Sorted(maps.Keys(versions)) {
		path := in.ResolveIndexPath(p)
		mod, err := LoadMod(path)
		if err != nil {
			return fmt.Errorf("failed to read metadata file %s: %w", p, err)
		}
		if mod.Version != "" {
			continue
		}
		mod.Version = versions[p]

		format, hash, err := mod.Write()
		if err != nil {
			return fmt.Errorf("failed to save %s: %w", p, err)
		}
		if err := in.RefreshFileWithHash(path, format, hash, true); err != nil {
			return fmt.Errorf("failed to update %s in the index: %w", p, err)
		}
	}
	return nil
}
