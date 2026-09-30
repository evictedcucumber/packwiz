package core

import (
	"maps"
	"slices"
	"strings"
)

// ModConfigFiles is a mod and the config files it claims (see Mod.ConfigFiles): those that are tracked in the pack, and
// the entries of its ConfigFiles that nothing tracked matches.
type ModConfigFiles struct {
	Mod *Mod
	// Files are the paths, relative to the pack and sorted, that Mod's ConfigFiles claims and the index tracks.
	Files []string
	// Missing are the entries of Mod's ConfigFiles, sorted and written as they are in the mod's metadata file (so
	// without the pack's config folder, see ConfigFileTree), that no tracked file matches: a path that isn't tracked,
	// or a folder with no tracked file under it. It is what has to be taken out of ConfigFiles to remove the claim.
	Missing []string
}

// OwnerConfigFiles is the pack as a whole, or one of its mod loaders, and the config files it claims (see
// Pack.ConfigFiles): the same as ModConfigFiles is for a mod.
type OwnerConfigFiles struct {
	// Owner is who it is, as it is in Pack.ConfigFiles: ConfigOwnerPack, or the name of a mod loader.
	Owner string
	// Name is Owner as it is written for people (see Pack.ConfigOwnerName).
	Name string
	// Entries are all of the owner's entries in Pack.ConfigFiles, as they are written there.
	Entries []string
	// Files are the paths, relative to the pack and sorted, that the owner claims and the index tracks.
	Files []string
	// Missing are the owner's entries, sorted and as they are written in Pack.ConfigFiles, that no tracked file matches.
	Missing []string
}

// ConfigFileTree groups the pack's tracked files that aren't a mod's own metadata or destination file - normally what
// is in its config/ folder - by who claims each one.
type ConfigFileTree struct {
	// Owners are the pack as a whole and the mod loaders that claim at least one tracked file, or have an entry that
	// matches none: the pack first, then the loaders in alphabetical order. A file that an owner and a mod both claim
	// is listed under each of them.
	Owners []OwnerConfigFiles
	// Mods are the mods that claim at least one tracked file, or have an entry in their ConfigFiles that matches none,
	// in alphabetical order of their name. A file that more than one mod's ConfigFiles claims is listed under each of
	// them.
	Mods []ModConfigFiles
	// Unclaimed are the tracked files (sorted) that nothing claims: e.g. left behind by a mod that has since been
	// removed, or never linked to the mod that installed it.
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

// ConfigEntry turns a path relative to the pack, as the index has it, into an entry for a mod's ConfigFiles: a folder
// (isDir) is given a trailing "/", which claims everything under it, and the pack's config folder (configDir, from
// ConfigDirOfMods, or "" if it has none) is taken back out, so it reads the same as any other entry does. It is "" for
// the config folder itself, which no entry can be written for.
func ConfigEntry(rel string, isDir bool, configDir string) string {
	if isDir && !strings.HasSuffix(rel, "/") {
		rel += "/"
	}
	if configDir != "" {
		rel = strings.TrimPrefix(rel, configDir+"/")
	}
	return rel
}

// EntryClaims reports whether entry, an entry of a mod's ConfigFiles written as it is in the mod's metadata file,
// claims path, a path relative to the pack as the index has it. configDir is the pack's config folder, as for
// ConfigEntry.
func EntryClaims(entry, configDir, path string) bool {
	if configDir != "" {
		entry = configDir + "/" + entry
	}
	return claimsPath(entry, path)
}

// ClaimingEntries returns the entries, of a mod's ConfigFiles or of an owner's in Pack.ConfigFiles, that claim path, a
// tracked path relative to the pack as the index has it: at most the one that names it and the folders it is under,
// each once, written as they are. It is what has to be taken out for their owner to stop claiming path. configDir is the
// pack's config folder, as for ConfigEntry.
func ClaimingEntries(entries []string, configDir, path string) []string {
	var claiming []string
	for _, entry := range entries {
		if EntryClaims(entry, configDir, path) && !slices.Contains(claiming, entry) {
			claiming = append(claiming, entry)
		}
	}
	return claiming
}

// resolveClaims finds what entries, written as if the pack kept its files at the root of the game directory, claim of
// the tracked paths: the paths they claim, which are marked in claimed, and the entries that claim none, sorted and
// without repeats. prefix is the pack's config folder and a "/" (or nothing), which the entries are resolved against.
func resolveClaims(entries []string, prefix string, paths []string, claimed map[string]bool) (files, missing []string) {
	claims := make([]string, len(entries))
	for i, entry := range entries {
		claims[i] = prefix + entry
	}
	for _, p := range paths {
		if slices.ContainsFunc(claims, func(claim string) bool { return claimsPath(claim, p) }) {
			claimed[p] = true
			files = append(files, p)
		}
	}
	for i, claim := range claims {
		if !slices.ContainsFunc(paths, func(p string) bool { return claimsPath(claim, p) }) {
			missing = append(missing, entries[i])
		}
	}
	slices.Sort(missing)
	return files, slices.Compact(missing)
}

// ConfigFileTree builds a ConfigFileTree of the pack's tracked config-like files, by who claims each one: the pack as a
// whole and its mod loaders (see Pack.ConfigFiles), and the mods.
//
// Entries are written as if the pack kept its files at the root of the game directory, as it normally does. A pack that
// has a mod with a ConfigDirResolver (e.g. Configured Defaults) instead keeps all of its files in that mod's folder, so
// entries are resolved against it too: "config/sodium.json" claims "configureddefaults/config/sodium.json" when
// Configured Defaults is installed, the same as without it.
//
// A file that is claimed is only there if the index tracks it, so an entry that matches no tracked file is reported in
// its owner's Missing: normally a config file that has since been deleted or renamed. It goes by the index, as
// everything else about the pack does, so a file that was only just deleted from disk is still tracked until "packwiz
// refresh".
func (in Index) ConfigFileTree(mods []*Mod, pack Pack) (ConfigFileTree, error) {
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

	// The pack comes first, then the rest, which is whichever loaders have entries, in order: an owner that the pack
	// doesn't have (see Pack.IsConfigOwner) is here too, so that what it claims isn't shown as if nothing did
	owners := slices.Sorted(maps.Keys(pack.ConfigFiles))
	if i := slices.Index(owners, ConfigOwnerPack); i > 0 {
		owners = slices.Insert(slices.Delete(owners, i, i+1), 0, ConfigOwnerPack)
	}
	for _, owner := range owners {
		entries := pack.ConfigFiles[owner]
		files, missing := resolveClaims(entries, prefix, paths, claimed)
		if len(files) > 0 || len(missing) > 0 {
			tree.Owners = append(tree.Owners, OwnerConfigFiles{
				Owner: owner, Name: pack.ConfigOwnerName(owner), Entries: slices.Clone(entries), Files: files, Missing: missing,
			})
		}
	}

	for _, mod := range sorted {
		files, missing := resolveClaims(mod.ConfigEntries(), prefix, paths, claimed)
		if len(files) > 0 || len(missing) > 0 {
			tree.Mods = append(tree.Mods, ModConfigFiles{Mod: mod, Files: files, Missing: missing})
		}
	}

	for _, p := range paths {
		if !claimed[p] {
			tree.Unclaimed = append(tree.Unclaimed, p)
		}
	}
	return tree, nil
}
