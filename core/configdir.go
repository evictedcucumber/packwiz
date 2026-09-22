package core

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/evictedcucumber/packwiz/internal/ui"
)

// ConfigDirResolver is optionally implemented by an Updater that can tell that a mod reads a pack's files from a
// folder of its own, as Configured Defaults does from its configureddefaults folder. A pack that has such a mod keeps
// its files there, so Index.Refresh tracks only that folder and the metadata files (mods, etc.).
type ConfigDirResolver interface {
	// ConfigDir returns the folder that the mod reads the pack's files from, relative to the index and written with
	// forward slashes, or "" if the mod isn't one that does. It is called for mods that this updater handles.
	ConfigDir(*Mod) string
}

// configDir finds the folder that a pack keeps its files in, given the paths of its metadata files: the folder of the
// first mod, in the order given, that has one. It is "" if no mod does, which leaves the pack free to keep its files
// anywhere.
func configDir(metaFiles []string) (string, error) {
	mods := make([]*Mod, len(metaFiles))
	for i, p := range metaFiles {
		mod, err := LoadMod(p)
		if err != nil {
			return "", fmt.Errorf("failed to read metadata file %s: %w", p, err)
		}
		mods[i] = &mod
	}
	return ConfigDirOfMods(mods), nil
}

// ConfigDirOfMods is configDir for mods already loaded, so a caller that has them (e.g. ConfigFileTree, or
// "packwiz config relate") doesn't read the metadata files again: the folder of the first mod, in the order given,
// that has one, or "" if no mod does.
func ConfigDirOfMods(mods []*Mod) string {
	for _, mod := range mods {
		for _, name := range slices.Sorted(maps.Keys(mod.Update)) {
			if resolver, ok := Updaters[name].(ConfigDirResolver); ok {
				if dir := resolver.ConfigDir(mod); dir != "" {
					return dir
				}
			}
		}
	}
	return ""
}

// inDir reports whether a path relative to the index, written with forward slashes, is inside a folder of it
func inDir(rel, dir string) bool {
	return strings.HasPrefix(rel, dir+"/")
}

// hasMetaExtension reports whether a path is a metadata file's, going by its extension
func hasMetaExtension(path string) bool {
	return strings.HasSuffix(filepath.Base(path), MetaExtension)
}

// keepConfigDirFiles takes the files found in the pack, as paths on disk, and drops the ones that a pack with a
// config folder doesn't track: everything but its metadata files and what is in that folder. A pack without one
// keeps them all. It says so when files that the index has are dropped, as they leave it.
func (in *Index) keepConfigDirFiles(found []string) ([]string, error) {
	rels := make([]string, len(found))
	isMeta := make([]bool, len(found))
	var metaFiles []string
	for i, p := range found {
		rel, err := in.RelIndexPath(p)
		if err != nil {
			return nil, err
		}
		rels[i] = rel
		// A file that the index has as a metadata file stays one, whatever it is called (see updateFileEntry)
		entry, tracked := in.Files[rel]
		isMeta[i] = hasMetaExtension(p) || (tracked && entry.IsMetaFile())
		if isMeta[i] {
			metaFiles = append(metaFiles, p)
		}
	}

	dir, err := configDir(metaFiles)
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return found, nil
	}

	kept := make([]string, 0, len(found))
	left := 0
	for i, p := range found {
		switch {
		case isMeta[i] || inDir(rels[i], dir):
			kept = append(kept, p)
		default:
			if _, tracked := in.Files[rels[i]]; tracked {
				left++
			}
		}
	}
	if left > 0 {
		noun, verb := "files", "are"
		if left == 1 {
			noun, verb = "file", "is"
		}
		ui.Info.Printf("Notice: this pack keeps its files in %s, so %d %s outside it %s no longer tracked\n",
			ui.Bold.Sprint(dir+"/"), left, noun, verb)
	}
	return kept, nil
}
