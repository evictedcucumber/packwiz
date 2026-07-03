package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/evictedcucumber/packwiz/core"
)

func resolveModTargetPath(index core.Index, input string) (string, error) {
	// 1) If the argument points to a real file, ensure it's a tracked metafile.
	if stat, err := os.Stat(input); err == nil && !stat.IsDir() {
		absPath, err := filepath.Abs(input)
		if err != nil {
			return "", err
		}
		packRoot, err := filepath.Abs(index.ResolveIndexPath("."))
		if err != nil {
			return "", err
		}
		relPath, err := filepath.Rel(packRoot, absPath)
		if err != nil {
			return "", err
		}
		relPath = filepath.ToSlash(relPath)
		if file, ok := index.Files[relPath]; ok && file.IsMetaFile() {
			return absPath, nil
		}
		return "", fmt.Errorf("%q is not a tracked metadata file in index.toml", input)
	}

	// 2) If the argument is an index-relative path, resolve it.
	relInput := filepath.ToSlash(filepath.Clean(input))
	if file, ok := index.Files[relInput]; ok && file.IsMetaFile() {
		return index.ResolveIndexPath(relInput), nil
	}

	// 3) Fall back to slug lookup.
	if modPath, ok := index.FindMod(input); ok {
		return modPath, nil
	}

	return "", fmt.Errorf("no mod matched %q", input)
}

func resolveTrackedModMetaPath(index core.Index, input string) (string, error) {
	// 1) If the argument points to a real file, ensure it's a tracked metafile.
	if stat, err := os.Stat(input); err == nil && !stat.IsDir() {
		absPath, err := filepath.Abs(input)
		if err != nil {
			return "", err
		}
		packRoot, err := filepath.Abs(index.ResolveIndexPath("."))
		if err != nil {
			return "", err
		}
		relPath, err := filepath.Rel(packRoot, absPath)
		if err != nil {
			return "", err
		}
		relPath = filepath.ToSlash(relPath)
		if file, ok := index.Files[relPath]; ok && file.IsMetaFile() {
			return absPath, nil
		}
		return "", fmt.Errorf("%q is not a tracked metadata file in index.toml", input)
	}

	// 2) If the argument is an index-relative path, resolve it.
	relInput := filepath.ToSlash(filepath.Clean(input))
	if file, ok := index.Files[relInput]; ok && file.IsMetaFile() {
		return index.ResolveIndexPath(relInput), nil
	}

	return "", fmt.Errorf("pin only accepts a tracked .pw.toml file, not %q", input)
}
