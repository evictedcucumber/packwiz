package modrinth

import (
	modrinthApi "codeberg.org/jmansfield/go-modrinth/modrinth"
	"errors"
	"fmt"
	"slices"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/mitchellh/mapstructure"
)

type mrUpdateData struct {
	// TODO(format): change to "project-id"
	ProjectID string `mapstructure:"mod-id"`
	// TODO(format): change to "version-id"
	InstalledVersion string `mapstructure:"version"`
}

func (u mrUpdateData) ToMap() (map[string]interface{}, error) {
	newMap := make(map[string]interface{})
	err := mapstructure.Decode(u, &newMap)
	return newMap, err
}

type mrUpdater struct{}

func (u mrUpdater) ParseUpdate(updateUnparsed map[string]interface{}) (interface{}, error) {
	var updateData mrUpdateData
	err := mapstructure.Decode(updateUnparsed, &updateData)
	return updateData, err
}

type cachedStateStore struct {
	ProjectID string
	Version   *modrinthApi.Version
}

func (u mrUpdater) CheckUpdate(mods []*core.Mod, pack core.Pack) ([]core.UpdateCheck, error) {
	results := make([]core.UpdateCheck, len(mods))

	for i, mod := range mods {
		rawData, ok := mod.GetParsedUpdateData("modrinth")
		if !ok {
			results[i] = core.UpdateCheck{Error: errors.New("failed to parse update metadata")}
			continue
		}

		data := rawData.(mrUpdateData)

		allowedChannel := pack.GetAllowedChannel(mod)
		newVersion, err := getLatestVersion(data.ProjectID, mod.Name, pack, allowedChannel)
		if err != nil {
			results[i] = core.UpdateCheck{Error: fmt.Errorf("failed to get latest version: %v", err)}
			continue
		}

		if len(newVersion.Files) == 0 {
			results[i] = core.UpdateCheck{Error: errors.New("new version doesn't have any files")}
			continue
		}

		newFilename := newVersion.Files[0].Filename
		// Prefer the primary file
		for _, v := range newVersion.Files {
			if *v.Primary {
				newFilename = v.Filename
			}
		}

		// Check if version is different
		updateAvailable := *newVersion.ID != data.InstalledVersion
		updateString := mod.FileName + " -> " + *newFilename

		// If already on latest version, still return version info for requery mode
		if !updateAvailable {
			results[i] = core.UpdateCheck{
				UpdateAvailable: false,
				UpdateString:    updateString,
				CachedState:     cachedStateStore{data.ProjectID, newVersion},
			}
			continue
		}

		results[i] = core.UpdateCheck{
			UpdateAvailable: true,
			UpdateString:    updateString,
			CachedState:     cachedStateStore{data.ProjectID, newVersion},
		}
	}

	return results, nil
}

func (u mrUpdater) DoUpdate(mods []*core.Mod, cachedState []interface{}) error {
	for i, mod := range mods {
		modState := cachedState[i].(cachedStateStore)
		var version = modState.Version

		var file = version.Files[0]
		// Prefer the primary file
		for _, v := range version.Files {
			if *v.Primary {
				file = v
			}
		}

		algorithm, hash := getBestHash(file)
		if algorithm == "" {
			return errors.New("file for project " + mod.Name + " doesn't have a valid hash")
		}

		mod.FileName = *file.Filename
		mod.Version = getModrinthVersionLabel(version)
		mod.Download = core.ModDownload{
			URL:        *file.URL,
			HashFormat: algorithm,
			Hash:       hash,
		}
		mod.Update["modrinth"]["version"] = version.ID
	}

	return nil
}

type mrResolver struct{}

func (r mrResolver) ResolveDependencies(mod *core.Mod, allMods []*core.Mod, index core.Index, pack core.Pack) ([]string, error) {
	rawData, ok := mod.GetParsedUpdateData("modrinth")
	if !ok {
		return nil, errors.New("failed to parse update metadata")
	}

	data := rawData.(mrUpdateData)
	if data.InstalledVersion == "" {
		return nil, nil
	}

	var depVersion *modrinthApi.Version
	err := retryWithBackoff("fetch version metadata from Modrinth", 3, func() error {
		var retryErr error
		depVersion, retryErr = mrDefaultClient.Versions.Get(data.InstalledVersion)
		return retryErr
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get version metadata from Modrinth: %w", err)
	}

	var depPaths []string
	isQuilt := slices.Contains(pack.GetCompatibleLoaders(), "quilt")
	mcVersion, err := pack.GetMCVersion()
	if err != nil {
		return nil, err
	}

	// Helper map to quickly find installed mods by Modrinth Project ID or Version ID
	installedByProjectID := make(map[string]*core.Mod)
	installedByVersionID := make(map[string]*core.Mod)
	for _, m := range allMods {
		mRaw, ok := m.GetParsedUpdateData("modrinth")
		if !ok {
			continue
		}
		mData := mRaw.(mrUpdateData)
		if mData.ProjectID != "" {
			installedByProjectID[mData.ProjectID] = m
		}
		if mData.InstalledVersion != "" {
			installedByVersionID[mData.InstalledVersion] = m
		}
	}

	for _, dep := range depVersion.Dependencies {
		if dep == nil {
			continue
		}
		if dep.DependencyType == nil || *dep.DependencyType != "required" {
			continue
		}

		var foundMod *core.Mod
		if dep.VersionID != nil {
			if m, found := installedByVersionID[*dep.VersionID]; found {
				foundMod = m
			}
		}
		if foundMod == nil && dep.ProjectID != nil {
			projectIDClean := mapDepOverride(*dep.ProjectID, isQuilt, mcVersion)
			if m, found := installedByProjectID[projectIDClean]; found {
				foundMod = m
			}
		}

		if foundMod != nil {
			relPath, err := index.RelIndexPath(foundMod.GetFilePath())
			if err == nil {
				depPaths = append(depPaths, relPath)
			}
		}
	}

	slices.Sort(depPaths)
	depPaths = slices.Compact(depPaths)
	return depPaths, nil
}
