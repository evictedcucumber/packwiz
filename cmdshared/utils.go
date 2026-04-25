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

func ResolveExportOutput(_ string, fileName string) string {
	if fileName == "" {
		return fileName
	}
	if filepath.IsAbs(fileName) || strings.ContainsAny(fileName, "/\\") {
		return fileName
	}
	baseFolder := viper.GetString("meta-folder-base")
	if baseFolder == "" {
		baseFolder = "."
	}
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

func FilterModsByAllowedPrefixes(mods []*core.Mod, index *core.Index, allowedPrefixes []string) []*core.Mod {
	if len(allowedPrefixes) == 0 {
		return mods
	}

	filtered := mods[:0]
	for _, mod := range mods {
		modPath, err := index.RelIndexPath(mod.GetFilePath())
		if err != nil {
			filtered = append(filtered, mod)
			continue
		}
		if PathHasAnyPrefix(modPath, allowedPrefixes) {
			filtered = append(filtered, mod)
		}
	}
	return filtered
}

func LoaderRootPrefix(loader string, index *core.Index) string {
	root := core.GetRootFolderForLoader(loader)
	if root == "" || root == "." {
		return ""
	}

	base := viper.GetString("meta-folder-base")
	if base == "" {
		base = "."
	}

	basePath := base
	if !filepath.IsAbs(basePath) {
		basePath = filepath.Join(index.PackRoot(), basePath)
	}

	rootPath := filepath.Join(basePath, root)
	relPath, err := index.RelIndexPath(rootPath)
	if err != nil {
		return ""
	}

	relPath = filepath.ToSlash(filepath.Clean(relPath))
	if relPath == "." {
		return ""
	}
	return relPath
}

func LoaderRootPrefixes(index *core.Index) []string {
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

	prefixes := make([]string, 0, len(rootFolders))
	for _, root := range rootFolders {
		if root == "" || root == "." {
			continue
		}

		rootPath := filepath.Join(basePath, root)
		relPath, err := index.RelIndexPath(rootPath)
		if err != nil {
			continue
		}

		relPath = filepath.ToSlash(filepath.Clean(relPath))
		if relPath == "." {
			continue
		}
		prefixes = append(prefixes, relPath)
	}

	if len(prefixes) == 0 {
		return nil
	}
	return prefixes
}

func ExportPathForLoader(relPath string, loader string, index *core.Index) string {
	prefix := LoaderRootPrefix(loader, index)
	if prefix == "" {
		return filepath.ToSlash(filepath.Clean(relPath))
	}

	cleanPath := filepath.ToSlash(filepath.Clean(relPath))
	cleanPrefix := filepath.ToSlash(filepath.Clean(prefix))
	cleanPrefix = strings.TrimSuffix(cleanPrefix, "/")
	if cleanPrefix == "" || cleanPrefix == "." {
		return cleanPath
	}
	if cleanPath == cleanPrefix {
		return cleanPath
	}
	if strings.HasPrefix(cleanPath, cleanPrefix+"/") {
		return strings.TrimPrefix(cleanPath, cleanPrefix+"/")
	}
	return cleanPath
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
