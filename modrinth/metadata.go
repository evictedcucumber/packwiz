package modrinth

import (
	"fmt"

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

func (f mrMetadataFixer) ValidateMetadata(mod *core.Mod) ([]string, error) {
	issues := make([]string, 0)

	rawData, ok := mod.GetParsedUpdateData("modrinth")
	if !ok {
		return issues, nil
	}

	data := rawData.(mrUpdateData)
	if data.ProjectID == "" || data.InstalledVersion == "" {
		return issues, nil
	}

	// Check if the project exists
	project, err := mrDefaultClient.Projects.Get(data.ProjectID)
	if err != nil {
		issues = append(issues, fmt.Sprintf("failed to fetch project from Modrinth: %v", err))
		return issues, nil
	}

	// Check if the version exists
	version, err := mrDefaultClient.Versions.Get(data.InstalledVersion)
	if err != nil {
		issues = append(issues, fmt.Sprintf("failed to fetch version from Modrinth: %v", err))
		return issues, nil
	}

	// Check if the version belongs to the project
	if version.ProjectID == nil || *version.ProjectID != data.ProjectID {
		issues = append(issues, "version ID does not belong to the project")
	}

	// Check if the version matches the metadata
	expectedVersion := getModrinthVersionLabel(version)
	if mod.Version != "" && mod.Version != expectedVersion {
		issues = append(issues, fmt.Sprintf("version mismatch: metadata says '%s' but version ID indicates '%s'", mod.Version, expectedVersion))
	}

	// Check if the page URL is correct
	expectedPageURL := getProjectPageURL(project)
	if mod.PageURL != "" && mod.PageURL != expectedPageURL {
		issues = append(issues, fmt.Sprintf("page URL mismatch: metadata says '%s' but should be '%s'", mod.PageURL, expectedPageURL))
	}

	return issues, nil
}

func (f mrMetadataFixer) FixMetadata(mod *core.Mod) (bool, error) {
	rawData, ok := mod.GetParsedUpdateData("modrinth")
	if !ok {
		return false, nil
	}

	data := rawData.(mrUpdateData)
	if data.ProjectID == "" || data.InstalledVersion == "" {
		return false, nil
	}

	changed := false

	// Fetch the current version from Modrinth
	version, err := mrDefaultClient.Versions.Get(data.InstalledVersion)
	if err != nil {
		return false, err
	}

	// Update version to match what Modrinth says
	expectedVersion := getModrinthVersionLabel(version)
	if mod.Version != expectedVersion {
		mod.Version = expectedVersion
		changed = true
	}

	// Fetch the project from Modrinth
	project, err := mrDefaultClient.Projects.Get(data.ProjectID)
	if err != nil {
		return false, err
	}

	// Update page URL to match what Modrinth says
	expectedPageURL := getProjectPageURL(project)
	if mod.PageURL != expectedPageURL {
		mod.PageURL = expectedPageURL
		changed = true
	}

	return changed, nil
}
