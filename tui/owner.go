package tui

import (
	"path/filepath"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
)

// ownerKind is what can own config files.
type ownerKind int

const (
	// ownerNone is nothing: what the files that nobody claims are given as their owner
	ownerNone ownerKind = iota
	// ownerMod is a mod, which keeps what it owns in its metadata file
	ownerMod
	// ownerPack is the pack as a whole, for what belongs to no mod or loader, such as options.txt
	ownerPack
	// ownerLoader is the pack's mod loader, such as NeoForge, for its own config files
	ownerLoader
)

// owner is who a config file can belong to: a mod, the pack as a whole or its mod loader. The last two keep what they own
// in pack.toml, where a mod keeps it in its own file, so the screen treats all three alike and leaves that to the backend.
type owner struct {
	kind ownerKind
	// id says which it is: the path of a mod's metadata file, or the key that the pack or a loader has in pack.toml's
	// config-files (core.ConfigOwnerPack, or the loader's name)
	id string
	// name is how it is written for people
	name string
	// slug is what it is searched for by as well as its name: the name of a mod's metadata file without its extension,
	// or the key in pack.toml
	slug string
	// entries are what it claims now, as they are written
	entries []string
}

// key identifies the owner among the others, which no two share whatever they are: a mod's id is a path to a file with an
// extension, and the others' are words.
func (o owner) key() string {
	if o.kind == ownerMod {
		return o.id
	}
	return "(owner:" + o.id + ")"
}

// note says what kind of owner it is when that isn't the obvious, which it is for a mod.
func (o owner) note() string {
	switch o.kind {
	case ownerPack:
		return "the pack as a whole"
	case ownerLoader:
		return "mod loader"
	}
	return ""
}

// modOwner is a mod as an owner.
func modOwner(m *core.Mod) owner {
	return owner{kind: ownerMod, id: m.GetFilePath(), name: m.Name, slug: slugOf(m), entries: m.ConfigEntries()}
}

// slugOf is the name of a mod's metadata file without its extension, which is what most people call it.
func slugOf(m *core.Mod) string {
	name := filepath.Base(m.GetFilePath())
	return strings.TrimSuffix(strings.TrimSuffix(name, core.MetaExtension), core.MetaExtensionOld)
}

// packOwner is the pack as a whole, or one of its mod loaders, as an owner: key is what it is in pack.toml's config-files
// (see core.Pack.ConfigOwners).
func packOwner(pack core.Pack, key string) owner {
	kind := ownerLoader
	if key == core.ConfigOwnerPack {
		kind = ownerPack
	}
	return owner{kind: kind, id: key, name: pack.ConfigOwnerName(key), slug: key, entries: pack.ConfigFiles[key]}
}
