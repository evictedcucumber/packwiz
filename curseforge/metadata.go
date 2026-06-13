package curseforge

import (
	"fmt"

	"github.com/evictedcucumber/packwiz/core"
)

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

func (f cfMetadataFixer) ValidateMetadata(mod *core.Mod) ([]string, error) {
	issues := make([]string, 0)

	rawData, ok := mod.GetParsedUpdateData("curseforge")
	if !ok {
		return issues, nil
	}

	data := rawData.(cfUpdateData)
	if data.ProjectID == 0 || data.FileID == 0 {
		return issues, nil
	}

	// Check if the project exists
	modInfo, err := cfDefaultClient.getModInfo(data.ProjectID)
	if err != nil {
		issues = append(issues, fmt.Sprintf("failed to fetch project from CurseForge: %v", err))
		return issues, nil
	}

	// Check if the file exists
	fileInfo, err := cfDefaultClient.getFileInfo(data.ProjectID, data.FileID)
	if err != nil {
		issues = append(issues, fmt.Sprintf("failed to fetch file from CurseForge: %v", err))
		return issues, nil
	}

	// Check if the file belongs to the project
	if fileInfo.ModID != data.ProjectID {
		issues = append(issues, "file ID does not belong to the project")
	}

	// Check if the version matches the metadata
	if mod.Version != "" && fileInfo.FriendlyName != "" && mod.Version != fileInfo.FriendlyName {
		issues = append(issues, fmt.Sprintf("version mismatch: metadata says '%s' but file indicates '%s'", mod.Version, fileInfo.FriendlyName))
	}

	// Check if the page URL is correct
	expectedPageURL := getModPageURL(modInfo)
	if mod.PageURL != "" && mod.PageURL != expectedPageURL {
		issues = append(issues, fmt.Sprintf("page URL mismatch: metadata says '%s' but should be '%s'", mod.PageURL, expectedPageURL))
	}

	return issues, nil
}

func (f cfMetadataFixer) FixMetadata(mod *core.Mod) (bool, error) {
	rawData, ok := mod.GetParsedUpdateData("curseforge")
	if !ok {
		return false, nil
	}

	data := rawData.(cfUpdateData)
	if data.ProjectID == 0 || data.FileID == 0 {
		return false, nil
	}

	changed := false

	// Fetch the current file from CurseForge
	fileInfo, err := cfDefaultClient.getFileInfo(data.ProjectID, data.FileID)
	if err != nil {
		return false, err
	}

	// Update version to match what CurseForge says
	if fileInfo.FriendlyName != "" && mod.Version != fileInfo.FriendlyName {
		mod.Version = fileInfo.FriendlyName
		changed = true
	}

	// Fetch the project from CurseForge
	modInfo, err := cfDefaultClient.getModInfo(data.ProjectID)
	if err != nil {
		return false, err
	}

	// Update page URL to match what CurseForge says
	expectedPageURL := getModPageURL(modInfo)
	if mod.PageURL != expectedPageURL {
		mod.PageURL = expectedPageURL
		changed = true
	}

	return changed, nil
}
