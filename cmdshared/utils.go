package cmdshared

import (
	"path/filepath"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/viper"
)

func GetRawForgeVersion(version string) string {
	var wantedVersion string
	// Check if we have a "-" in the version
	if strings.Contains(version, "-") {
		// We have a mcVersion-loaderVersion format
		// Strip the mcVersion
		wantedVersion = strings.Split(version, "-")[1]
	} else {
		wantedVersion = version
	}
	return wantedVersion
}

func ResolveExportOutput(loader string, fileName string) string {
	if fileName == "" {
		return fileName
	}
	if filepath.IsAbs(fileName) || strings.ContainsAny(fileName, "/\\") {
		return fileName
	}
	baseFolder := core.GetMetaFolderBaseForLoader(loader)
	return filepath.Join(baseFolder, fileName)
}

func OtherLoaderIgnorePrefixes(loader string, index *core.Index) []string {
	rootFolders := configuredRootFolders()
	if len(rootFolders) == 0 {
		return nil
	}

	base := viper.GetString("meta-folder-base")
	if base == "" {
		base = "."
	}

	basePath := base
	if !filepath.IsAbs(basePath) {
		basePath = filepath.Join(index.PackRoot(), basePath)
	}

	ignorePrefixes := make([]string, 0)
	for name, root := range rootFolders {
		if name == loader {
			continue
		}
		if root == "" || root == "." {
			continue
		}

		otherPath := filepath.Join(basePath, root)
		relPath, err := index.RelIndexPath(otherPath)
		if err != nil {
			continue
		}

		relPath = filepath.ToSlash(filepath.Clean(relPath))
		if relPath == "." {
			continue
		}
		ignorePrefixes = append(ignorePrefixes, relPath)
	}

	if len(ignorePrefixes) == 0 {
		return nil
	}
	return ignorePrefixes
}

func FilterModsByIgnorePrefixes(mods []*core.Mod, index *core.Index, ignorePrefixes []string) []*core.Mod {
	if len(ignorePrefixes) == 0 {
		return mods
	}

	filtered := mods[:0]
	for _, mod := range mods {
		modPath, err := index.RelIndexPath(mod.GetFilePath())
		if err != nil {
			filtered = append(filtered, mod)
			continue
		}
		if !PathHasAnyPrefix(modPath, ignorePrefixes) {
			filtered = append(filtered, mod)
		}
	}
	return filtered
}

func PathHasAnyPrefix(relPath string, prefixes []string) bool {
	cleanPath := filepath.ToSlash(filepath.Clean(relPath))
	for _, prefix := range prefixes {
		cleanPrefix := filepath.ToSlash(filepath.Clean(prefix))
		cleanPrefix = strings.TrimSuffix(cleanPrefix, "/")
		if cleanPrefix == "" || cleanPrefix == "." {
			continue
		}
		if cleanPath == cleanPrefix || strings.HasPrefix(cleanPath, cleanPrefix+"/") {
			return true
		}
	}
	return false
}

func configuredRootFolders() map[string]string {
	rootFolders := viper.GetStringMapString("root-folder")
	if rootFolders == nil {
		rootFolders = map[string]string{}
	}

	knownLoaders := []string{"modrinth", "curseforge"}
	for _, loader := range knownLoaders {
		if _, exists := rootFolders[loader]; exists {
			continue
		}
		root := viper.GetString(loader + ".root-folder")
		if root != "" {
			rootFolders[loader] = root
		}
	}

	return rootFolders
}
