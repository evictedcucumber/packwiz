package curseforge

import "github.com/evictedcucumber/packwiz/core"

type cfMetadataFixer struct{}

func (f cfMetadataFixer) FillMissingMetadata(mod *core.Mod) (bool, error) {
	rawData, ok := mod.GetParsedUpdateData("curseforge")
	if !ok {
		return false, nil
	}

	data := rawData.(cfUpdateData)
	if data.ProjectID == 0 {
		return false, nil
	}

	changed := false

	if mod.PageURL == "" {
		modInfoData, err := cfDefaultClient.getModInfo(data.ProjectID)
		if err != nil {
			return changed, err
		}
		pageURL := getModPageURL(modInfoData)
		if pageURL != "" {
			mod.PageURL = pageURL
			changed = true
		}
	}

	if mod.Version == "" && data.FileID != 0 {
		fileInfoData, err := cfDefaultClient.getFileInfo(data.ProjectID, data.FileID)
		if err != nil {
			return changed, err
		}
		if fileInfoData.FriendlyName != "" {
			mod.Version = fileInfoData.FriendlyName
			changed = true
		}
	}

	return changed, nil
}
