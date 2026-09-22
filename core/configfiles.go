package core

import (
	"slices"
	"strings"
)

// ConfigFile is one of the pack's tracked files that isn't a mod's own metadata or destination file - normally
// something in its config/ folder, or anything else installed by hand rather than by a mod.
type ConfigFile struct {
	// Path is relative to the pack, written with forward slashes, as the index stores it.
	Path string
	// Claimed is whether a mod's ConfigFiles names this path, either directly or by claiming a folder it is in.
	Claimed bool
}

// claimsPath reports whether an entry from a mod's ConfigFiles accounts for a path relative to the pack. An entry
// ending in "/" claims everything under that folder; any other entry claims only that exact path.
func claimsPath(entry, p string) bool {
	if strings.HasSuffix(entry, "/") {
		return strings.HasPrefix(p, entry)
	}
	return entry == p
}

// ConfigFiles lists the pack's tracked files that aren't a mod's own metadata or destination file, sorted by path,
// saying which of them a mod's ConfigFiles claims. A file that nothing claims may be left behind by a mod that has
// since been removed, or simply never linked to the mod that installed it.
//
// ConfigFiles is written as if the pack kept its files at the root of the game directory, as it normally does. A
// pack that has a mod with a ConfigDirResolver (e.g. Configured Defaults) instead keeps all of its files in that
// mod's folder, so entries are resolved against it too: "config/sodium.json" claims
// "configureddefaults/config/sodium.json" when Configured Defaults is installed, the same as without it.
func (in Index) ConfigFiles(mods []*Mod) ([]ConfigFile, error) {
	prefix := ""
	if dir := configDirOfMods(mods); dir != "" {
		prefix = dir + "/"
	}

	dests := make(map[string]bool, len(mods))
	var claims []string
	for _, mod := range mods {
		if mod.FileName != "" {
			dest, err := in.RelIndexPath(mod.GetDestFilePath())
			if err != nil {
				return nil, err
			}
			dests[dest] = true
		}
		for _, cf := range mod.ConfigFiles {
			claims = append(claims, prefix+cf)
		}
	}

	var files []ConfigFile
	for p, file := range in.Files {
		if file.IsMetaFile() || dests[p] {
			continue
		}
		claimed := slices.ContainsFunc(claims, func(entry string) bool { return claimsPath(entry, p) })
		files = append(files, ConfigFile{Path: p, Claimed: claimed})
	}
	slices.SortFunc(files, func(a, b ConfigFile) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}
