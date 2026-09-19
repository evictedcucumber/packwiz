package modrinth

import (
	"errors"
	"fmt"
	"slices"

	"github.com/evictedcucumber/packwiz/core"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
	"github.com/mitchellh/mapstructure"
)

type mrUpdateData struct {
	// TODO(format): change to "project-id"
	ProjectID string `mapstructure:"mod-id"`
	// TODO(format): change to "version-id"
	InstalledVersion string `mapstructure:"version"`
	// ReleaseType overrides the pack's default release type for this mod. Empty means "use the pack default"
	ReleaseType string `mapstructure:"release-type,omitempty"`
}

func (u mrUpdateData) ToMap() (map[string]interface{}, error) {
	newMap := make(map[string]interface{})
	err := mapstructure.Decode(u, &newMap)
	return newMap, err
}

type mrUpdater struct{}

// mrUpdater can look up the readable version numbers of the mods it manages
var _ core.VersionResolver = mrUpdater{}

// versionLookupBatchSize is how many versions are looked up in one request. They are listed in the URL, so a pack
// with a great many mods mustn't put them all in one.
const versionLookupBatchSize = 100

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

		newVersion, err := getLatestVersion(data.ProjectID, mod.Name, pack, data.ReleaseType)
		if err != nil {
			results[i] = core.UpdateCheck{Error: fmt.Errorf("failed to get latest version: %v", err)}
			continue
		}

		if *newVersion.ID == data.InstalledVersion { //The latest version from the site is the same as the installed one
			results[i] = core.UpdateCheck{UpdateAvailable: false}
			continue
		}

		if len(newVersion.Files) == 0 {
			results[i] = core.UpdateCheck{Error: errors.New("new version doesn't have any files")}
			continue
		}

		newFilename := newVersion.Files[0].Filename
		// Prefer the primary file
		for _, v := range newVersion.Files {
			if isPrimary(v) {
				newFilename = v.Filename
			}
		}

		results[i] = core.UpdateCheck{
			UpdateAvailable: true,
			UpdateString:    mod.FileName + " -> " + *newFilename,
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
			if isPrimary(v) {
				file = v
			}
		}

		if err := applyVersion(mod, version, file); err != nil {
			return err
		}
	}

	return nil
}

// applyVersion points a mod at file, one of the files of version, recording where to download it from and which
// version it is. The rest of the mod (its name, side, pin, and so on) is left as it was.
func applyVersion(mod *core.Mod, version *modrinthApi.Version, file *modrinthApi.File) error {
	algorithm, hash := getBestHash(file)
	if algorithm == "" {
		return errors.New("file for project " + mod.Name + " doesn't have a valid hash")
	}

	mod.FileName = *file.Filename
	// Overwrite rather than keep the old value, so a version without a number doesn't leave a stale one behind
	mod.Version = versionNumberOf(version)
	mod.Download = core.ModDownload{
		URL:        *file.URL,
		HashFormat: algorithm,
		Hash:       hash,
	}
	mod.Update["modrinth"]["version"] = version.ID
	mod.Dependencies = buildDependencyList(version)
	return nil
}

// modrinthUpdateData returns the Modrinth update data of a mod, and whether it has any (i.e. is managed by Modrinth)
func modrinthUpdateData(mod *core.Mod) (mrUpdateData, bool) {
	raw, ok := mod.GetParsedUpdateData("modrinth")
	if !ok {
		return mrUpdateData{}, false
	}
	data, ok := raw.(mrUpdateData)
	return data, ok
}

// ResolveVersions implements core.VersionResolver, looking up the version number of the version each mod has
// installed.
func (u mrUpdater) ResolveVersions(mods []*core.Mod) ([]string, error) {
	installed := make([]string, len(mods))
	var unique []string
	seen := make(map[string]bool)
	for i, mod := range mods {
		rawData, ok := mod.GetParsedUpdateData("modrinth")
		if !ok {
			continue
		}
		data, ok := rawData.(mrUpdateData)
		if !ok || data.InstalledVersion == "" {
			continue
		}
		installed[i] = data.InstalledVersion
		if !seen[data.InstalledVersion] {
			seen[data.InstalledVersion] = true
			unique = append(unique, data.InstalledVersion)
		}
	}

	numbers := make(map[string]string, len(unique))
	for batch := range slices.Chunk(unique, versionLookupBatchSize) {
		versions, err := mrDefaultClient.Versions.GetMultiple(batch)
		if err != nil {
			return nil, fmt.Errorf("failed to look up versions: %w", err)
		}
		for _, v := range versions {
			if v != nil && v.ID != nil {
				numbers[*v.ID] = versionNumberOf(v)
			}
		}
	}

	results := make([]string, len(mods))
	for i, id := range installed {
		results[i] = numbers[id]
	}
	return results, nil
}
