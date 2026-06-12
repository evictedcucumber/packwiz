package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// FixModMetadataOpts controls how mod metadata is repaired.
type FixModMetadataOpts struct {
	Delete bool
}

var obsoleteModOptionKeys = map[string]struct{}{
	"optional": {},
	"default":  {},
}

// FixModMetadata ensures mod metadata complies with packwiz validation rules.
func (in *Index) FixModMetadata(pack Pack, opts FixModMetadataOpts) (int, error) {
	mods, err := in.LoadAllMods()
	if err != nil {
		return 0, err
	}

	fixed := 0
	for _, mod := range mods {
		before, err := os.ReadFile(mod.GetFilePath())
		if err != nil {
			return fixed, fmt.Errorf("failed to read mod %s: %w", mod.Name, err)
		}

		changed := false
		if mod.Option == nil {
			mod.EnsureOptionDefaults()
			changed = true
		}

		for updateSystem := range mod.Update {
			fixer, ok := MetadataFixers[updateSystem]
			if !ok {
				continue
			}
			metadataChanged, err := fixer.FillMissingMetadata(mod)
			if err != nil {
				fmt.Printf("Warning: failed to fill metadata for mod %s using %s: %v\n", mod.Name, updateSystem, err)
				continue
			}
			if metadataChanged {
				changed = true
			}
		}

		if opts.Delete {
			if in.pruneInvalidDependencies(mod) {
				changed = true
			}
			if modHasRemovableKeys(mod.GetFilePath()) {
				changed = true
			}
		}

		if !changed {
			continue
		}

		format, hash, err := mod.Write()
		if err != nil {
			return fixed, fmt.Errorf("failed to write mod %s: %w", mod.Name, err)
		}

		after, err := os.ReadFile(mod.GetFilePath())
		if err != nil {
			return fixed, fmt.Errorf("failed to read mod %s after write: %w", mod.Name, err)
		}
		if !bytes.Equal(before, after) {
			fixed++
		}

		if err := in.RefreshFileWithHash(mod.GetFilePath(), format, hash, true); err != nil {
			return fixed, err
		}
	}

	return fixed, nil
}

func modHasRemovableKeys(modPath string) bool {
	var raw map[string]interface{}
	if _, err := toml.DecodeFile(modPath, &raw); err != nil {
		return false
	}

	for key := range raw {
		if !isAllowedModKey(key) {
			return true
		}
	}

	optionVal, ok := raw["option"].(map[string]interface{})
	if !ok {
		return false
	}
	for key := range optionVal {
		if _, obsolete := obsoleteModOptionKeys[key]; obsolete {
			return true
		}
	}
	return false
}

func isAllowedModKey(key string) bool {
	switch key {
	case "name", "filename", "version", "page-url", "category", "side", "pin", "download", "update", "dependencies", "option":
		return true
	default:
		return false
	}
}

func (in *Index) pruneInvalidDependencies(mod *Mod) bool {
	if len(mod.Dependencies) == 0 {
		return false
	}

	changed := false
	valid := make([]string, 0, len(mod.Dependencies))
	seen := make(map[string]struct{}, len(mod.Dependencies))

	for _, dependencyPath := range mod.Dependencies {
		if dependencyPath == "" {
			changed = true
			continue
		}

		relPath, err := in.ToIndexRelativePath(dependencyPath)
		if err != nil {
			relPath = filepath.ToSlash(filepath.Clean(dependencyPath))
		}

		absPath := in.ResolveIndexPath(relPath)
		if _, err := os.Stat(absPath); err != nil {
			changed = true
			continue
		}

		if file, ok := in.Files[relPath]; !ok || !file.IsMetaFile() {
			changed = true
			continue
		}

		if _, ok := seen[relPath]; ok {
			changed = true
			continue
		}
		seen[relPath] = struct{}{}
		valid = append(valid, relPath)
	}

	if !changed {
		return false
	}

	mod.Dependencies = valid
	return true
}
