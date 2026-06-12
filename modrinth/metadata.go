package modrinth

import (
	"github.com/evictedcucumber/packwiz/core"
)

type mrMetadataFixer struct{}

func (f mrMetadataFixer) FillMissingMetadata(mod *core.Mod) (bool, error) {
	rawData, ok := mod.GetParsedUpdateData("modrinth")
	if !ok {
		return false, nil
	}

	data := rawData.(mrUpdateData)
	changed := false

	if mod.PageURL == "" && data.ProjectID != "" {
		project, err := mrDefaultClient.Projects.Get(data.ProjectID)
		if err != nil {
			return changed, err
		}
		pageURL := getProjectPageURL(project)
		if pageURL != "" {
			mod.PageURL = pageURL
			changed = true
		}
	}

	if mod.Version == "" && data.InstalledVersion != "" {
		version, err := mrDefaultClient.Versions.Get(data.InstalledVersion)
		if err != nil {
			return changed, err
		}
		versionLabel := getModrinthVersionLabel(version)
		if versionLabel != "" {
			mod.Version = versionLabel
			changed = true
		}
	}

	return changed, nil
}
