package core

import (
	"slices"
	"strings"
)

// ModConfigFiles is a mod and the config files it claims (see Mod.ConfigFiles) that are actually tracked in the pack.
type ModConfigFiles struct {
	Mod *Mod
	// Files are the paths, relative to the pack and sorted, that Mod's ConfigFiles claims and the index tracks.
	Files []string
}

// ConfigFileTree groups the pack's tracked files that aren't a mod's own metadata or destination file - normally what
// is in its config/ folder - by the mod that claims each one.
type ConfigFileTree struct {
	// Mods are the mods that claim at least one tracked file, in alphabetical order of their name. A file that more
	// than one mod's ConfigFiles claims is listed under each of them.
	Mods []ModConfigFiles
	// Unclaimed are the tracked files (sorted) that no mod's ConfigFiles claims: e.g. left behind by a mod that has
	// since been removed, or never linked to the mod that installed it.
	Unclaimed []string
}

// claimsPath reports whether an entry from a mod's ConfigFiles accounts for a path relative to the pack. An entry
// ending in "/" claims everything under that folder; any other entry claims only that exact path.
func claimsPath(entry, p string) bool {
	if strings.HasSuffix(entry, "/") {
		return strings.HasPrefix(p, entry)
	}
	return entry == p
}

// ConfigFileTree builds a ConfigFileTree of the pack's tracked config-like files, by the mod that claims each one.
//
// Entries in a mod's ConfigFiles are written as if the pack kept its files at the root of the game directory, as it
// normally does. A pack that has a mod with a ConfigDirResolver (e.g. Configured Defaults) instead keeps all of its
// files in that mod's folder, so entries are resolved against it too: "config/sodium.json" claims
// "configureddefaults/config/sodium.json" when Configured Defaults is installed, the same as without it.
func (in Index) ConfigFileTree(mods []*Mod) (ConfigFileTree, error) {
	sorted := slices.Clone(mods)
	slices.SortFunc(sorted, func(a, b *Mod) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})

	prefix := ""
	if dir := ConfigDirOfMods(mods); dir != "" {
		prefix = dir + "/"
	}

	dests := make(map[string]bool, len(mods))
	for _, mod := range mods {
		if mod.FileName == "" {
			continue
		}
		dest, err := in.RelIndexPath(mod.GetDestFilePath())
		if err != nil {
			return ConfigFileTree{}, err
		}
		dests[dest] = true
	}

	var paths []string
	for p, file := range in.Files {
		if file.IsMetaFile() || dests[p] {
			continue
		}
		paths = append(paths, p)
	}
	slices.Sort(paths)

	claimed := make(map[string]bool, len(paths))
	var tree ConfigFileTree
	for _, mod := range sorted {
		if mod.ConfigFiles == nil || len(*mod.ConfigFiles) == 0 {
			continue
		}
		claims := make([]string, len(*mod.ConfigFiles))
		for i, cf := range *mod.ConfigFiles {
			claims[i] = prefix + cf
		}

		var files []string
		for _, p := range paths {
			if slices.ContainsFunc(claims, func(entry string) bool { return claimsPath(entry, p) }) {
				claimed[p] = true
				files = append(files, p)
			}
		}
		if len(files) > 0 {
			tree.Mods = append(tree.Mods, ModConfigFiles{Mod: mod, Files: files})
		}
	}

	for _, p := range paths {
		if !claimed[p] {
			tree.Unclaimed = append(tree.Unclaimed, p)
		}
	}
	return tree, nil
}
