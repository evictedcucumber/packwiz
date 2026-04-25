package core

import (
	"path/filepath"

	"github.com/spf13/viper"
)

// GetMetaFolderBaseForLoader returns meta-folder-base joined with the loader root folder.
func GetMetaFolderBaseForLoader(loader string) string {
	base := viper.GetString("meta-folder-base")
	root := GetRootFolderForLoader(loader)
	if root == "" || root == "." {
		return base
	}
	return filepath.Join(base, root)
}

func GetRootFolderForLoader(loader string) string {
	root := viper.GetString("root-folder." + loader)
	if root == "" {
		root = viper.GetString(loader + ".root-folder")
	}
	return root
}
