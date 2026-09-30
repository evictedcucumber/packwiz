package core

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/BurntSushi/toml"
	"github.com/spf13/viper"
)

// Pack stores the modpack metadata, usually in pack.toml
type Pack struct {
	Name        string `toml:"name"`
	Author      string `toml:"author,omitempty"`
	Version     string `toml:"version,omitempty"`
	Description string `toml:"description,omitempty"`
	PackFormat  string `toml:"pack-format"`
	Index       struct {
		// Path is stored in forward slash format relative to pack.toml
		File       string `toml:"file"`
		HashFormat string `toml:"hash-format"`
		Hash       string `toml:"hash,omitempty"`
	} `toml:"index"`
	Versions map[string]string                 `toml:"versions"`
	Export   map[string]map[string]interface{} `toml:"export"`
	Options  map[string]interface{}            `toml:"options"`

	// ConfigFiles lists the config files that no mod owns, by who owns them: ConfigOwnerPack for the pack as a whole
	// (options.txt, say), or the name of a mod loader the pack has, as it is in Versions, for the loader's own
	// (neoforge's config/neoforge-common.toml). What a mod owns is in its own metadata file (see Mod.ConfigFiles); the
	// entries are written the same way, a path or a path ending in "/" to claim everything under it, and "packwiz
	// config relate" adds to them. An owner with nothing to claim is left out, rather than kept with an empty list.
	ConfigFiles map[string][]string `toml:"config-files,omitempty"`
}

// ConfigOwnerPack is the owner of the config files that belong to the pack as a whole, rather than to a mod or to its
// mod loader (see Pack.ConfigFiles): options.txt, for one.
const ConfigOwnerPack = "pack"

// LoaderName is how a mod loader is written for people, given its name as it is in a pack's versions.
func LoaderName(loader string) string {
	if loader == "neoforge" {
		return "NeoForge"
	}
	return loader
}

// ConfigOwners lists who can own config files that no mod does (see Pack.ConfigFiles): the pack, then each mod loader it
// has.
func (pack Pack) ConfigOwners() []string {
	return append([]string{ConfigOwnerPack}, pack.GetLoaders()...)
}

// IsConfigOwner reports whether owner is one that can own config files in this pack: the pack itself, or a mod loader
// that it has.
func (pack Pack) IsConfigOwner(owner string) bool {
	return slices.Contains(pack.ConfigOwners(), owner)
}

// ConfigOwnerName is how an owner of config files is written for people: "Pack", the name of a loader, or an owner
// that isn't one of these as it is written in the pack (see IsConfigOwner).
func (pack Pack) ConfigOwnerName(owner string) string {
	if owner == ConfigOwnerPack {
		return "Pack"
	}
	return LoaderName(owner)
}

// ClaimConfigFile adds path to what owner's config files are, and reports whether it did: false if it was there.
func (pack *Pack) ClaimConfigFile(owner, path string) bool {
	if slices.Contains(pack.ConfigFiles[owner], path) {
		return false
	}
	if pack.ConfigFiles == nil {
		pack.ConfigFiles = make(map[string][]string)
	}
	pack.ConfigFiles[owner] = append(pack.ConfigFiles[owner], path)
	return true
}

// UnclaimConfigFile takes path out of owner's config files, however many times it is there, and reports whether it was
// there. An owner left with none is taken out, so pack.toml doesn't keep an empty list for it.
func (pack *Pack) UnclaimConfigFile(owner, path string) bool {
	if !slices.Contains(pack.ConfigFiles[owner], path) {
		return false
	}
	pack.ConfigFiles[owner] = slices.DeleteFunc(pack.ConfigFiles[owner], func(entry string) bool { return entry == path })
	if len(pack.ConfigFiles[owner]) == 0 {
		delete(pack.ConfigFiles, owner)
	}
	return true
}

// CurrentPackFormat identifies packs created by this fork of packwiz. It is
// intentionally distinct from upstream packwiz's "packwiz:x.x.x" format, so
// packs are only compatible with this fork's own tooling.
const CurrentPackFormat = "evictedcucumber-packwiz:1.0.0"

// LoadPack loads the modpack metadata to a Pack struct
func LoadPack() (Pack, error) {
	data, err := os.ReadFile(viper.GetString("pack-file"))
	if err != nil {
		return Pack{}, err
	}
	modpack, err := ParsePack(data)
	if err != nil {
		return Pack{}, err
	}

	if modpack.PackFormat != CurrentPackFormat {
		return Pack{}, fmt.Errorf("pack-format field %q is not supported; this pack must be created with this fork of packwiz (expected %q)", modpack.PackFormat, CurrentPackFormat)
	}

	// Read options into viper
	if modpack.Options != nil {
		err := viper.MergeConfigMap(modpack.Options)
		if err != nil {
			return Pack{}, err
		}
	}
	return modpack, nil
}

// ParsePack parses the contents of a pack file that isn't necessarily the one in use (e.g. a committed version of it).
// Unlike LoadPack it doesn't check the pack's format, and doesn't merge its options into the global configuration.
func ParsePack(data []byte) (Pack, error) {
	var modpack Pack
	if _, err := toml.Decode(string(data), &modpack); err != nil {
		return Pack{}, err
	}
	if len(modpack.Index.File) == 0 {
		modpack.Index.File = "index.toml"
	}
	return modpack, nil
}

// LoadIndex attempts to load the index file of this modpack
func (pack Pack) LoadIndex() (Index, error) {
	if filepath.IsAbs(pack.Index.File) {
		return LoadIndex(pack.Index.File)
	}
	fileNative := filepath.FromSlash(pack.Index.File)
	return LoadIndex(filepath.Join(filepath.Dir(viper.GetString("pack-file")), fileNative))
}

// UpdateIndexHash recalculates the hash of the index file of this modpack
func (pack *Pack) UpdateIndexHash() error {
	if viper.GetBool("no-internal-hashes") {
		pack.Index.HashFormat = "sha256"
		pack.Index.Hash = ""
		return nil
	}

	fileNative := filepath.FromSlash(pack.Index.File)
	indexFile := filepath.Join(filepath.Dir(viper.GetString("pack-file")), fileNative)

	f, err := os.Open(indexFile)
	if err != nil {
		return err
	}

	// Hash usage strategy (may change):
	// Just use SHA256, overwrite existing hash regardless of what it is
	// May update later to continue using the same hash that was already being used
	h, err := GetHashImpl("sha256")
	if err != nil {
		_ = f.Close()
		return err
	}
	if _, err := io.Copy(h, f); err != nil {
		_ = f.Close()
		return err
	}
	hashString := h.HashToString(h.Sum(nil))

	pack.Index.HashFormat = "sha256"
	pack.Index.Hash = hashString
	return f.Close()
}

// SaveIndex writes the index, brings the pack's record of the index's hash up to date and writes the pack: everything
// that has to follow a change to the index.
func (pack *Pack) SaveIndex(index Index) error {
	if err := index.Write(); err != nil {
		return err
	}
	if err := pack.UpdateIndexHash(); err != nil {
		return err
	}
	return pack.Write()
}

// Write saves the pack file
func (pack Pack) Write() error {
	f, err := os.Create(viper.GetString("pack-file"))
	if err != nil {
		return err
	}

	enc := toml.NewEncoder(f)
	// Disable indentation
	enc.Indent = ""
	err = enc.Encode(pack)
	if err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// GetMCVersion gets the version of Minecraft this pack uses, if it has been correctly specified
func (pack Pack) GetMCVersion() (string, error) {
	mcVersion, ok := pack.Versions["minecraft"]
	if !ok {
		return "", errors.New("no minecraft version specified in modpack")
	}
	return mcVersion, nil
}

// GetSupportedMCVersions gets the versions of Minecraft this pack allows in downloaded mods, ordered by preference (highest = most desirable)
func (pack Pack) GetSupportedMCVersions() ([]string, error) {
	mcVersion, ok := pack.Versions["minecraft"]
	if !ok {
		return nil, errors.New("no minecraft version specified in modpack")
	}
	allVersions := append(append([]string(nil), viper.GetStringSlice("acceptable-game-versions")...), mcVersion)
	// Deduplicate values
	allVersionsDeduped := []string(nil)
	for i, v := range allVersions {
		// If another copy of this value exists past this point in the array, don't insert
		// (i.e. prefer a later copy over an earlier copy, so the main version is last)
		if !slices.Contains(allVersions[i+1:], v) {
			allVersionsDeduped = append(allVersionsDeduped, v)
		}
	}
	return allVersionsDeduped, nil
}

// GetReleaseType gets the minimum acceptable release type for this pack (release, beta or alpha),
// defaulting to "release" if not configured
func (pack Pack) GetReleaseType() string {
	releaseType := viper.GetString("release-type")
	if !IsValidReleaseType(releaseType) {
		return ReleaseTypeRelease
	}
	return releaseType
}

func (pack Pack) GetPackName() string {
	if pack.Name == "" {
		return "export"
	} else if pack.Version == "" {
		return pack.Name
	} else {
		return pack.Name + "-" + pack.Version
	}
}

func (pack Pack) GetCompatibleLoaders() (loaders []string) {
	if _, hasNeoForge := pack.Versions["neoforge"]; hasNeoForge {
		loaders = append(loaders, "neoforge")
	}
	return
}

func (pack Pack) GetLoaders() (loaders []string) {
	if _, hasNeoForge := pack.Versions["neoforge"]; hasNeoForge {
		loaders = append(loaders, "neoforge")
	}
	return
}
