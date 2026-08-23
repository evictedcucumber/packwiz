package modrinth

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"

	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/core"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/unascribed/FlexVer/go/flexver"
)

var modrinthCmd = &cobra.Command{
	Use:     "modrinth",
	Aliases: []string{"mr"},
	Short:   "Manage modrinth-based mods",
}

var mrDefaultClient = modrinthApi.NewClient(&http.Client{})

func init() {
	cmd.Add(modrinthCmd)
	core.Updaters["modrinth"] = mrUpdater{}

	mrDefaultClient.UserAgent = core.UserAgent
}

// "Loaders" that are supported regardless of the configured mod loaders
var defaultMRLoaders = []string{
	// TODO: check if Canvas/Iris/Optifine are installed? suggest installing them?
	"canvas",
	"iris",
	"optifine",
	"vanilla",   // Core shaders
	"minecraft", // Resource packs
}

var withDatapackPathMRLoaders = []string{
	"canvas",
	"iris",
	"optifine",
	"vanilla",   // Core shaders
	"minecraft", // Resource packs
	// TODO: check if a datapack loader is installed; suggest installing one?
	"datapack", // Datapacks (requires a datapack loader)
}

var loaderFolders = map[string]string{
	"neoforge":   "mods",
	"modloader":  "mods",
	"rift":       "mods",
	"bukkit":     "plugins",
	"spigot":     "plugins",
	"paper":      "plugins",
	"purpur":     "plugins",
	"sponge":     "plugins",
	"bungeecord": "plugins",
	"waterfall":  "plugins",
	"velocity":   "plugins",
	"canvas":     "resourcepacks",
	"iris":       "shaderpacks",
	"optifine":   "shaderpacks",
	"vanilla":    "resourcepacks",
}

// Preference list for loader types, for comparing files where the version is the same - more preferred is lower
var loaderPreferenceList = []string{
	"neoforge",
	"modloader",
	"rift",
	// Prefer mods to plugins
	"sponge",
	// Prefer newer Bukkit forks
	"purpur",
	"paper",
	"spigot",
	"bukkit",
	"velocity",
	// Prefer newer BungeeCord forks
	"waterfall",
	"bungeecord",
	// Prefer Canvas shaders to Iris shaders to Optifine shaders to core shaders
	"canvas",
	"iris",
	"optifine",
	"vanilla",
	// Prefer mods to datapacks
	"datapack",
	// Prefer mods to resource packs?! Idk this is just here for completeness
	"minecraft",
}

// Groups of loaders that should be treated the same as the key, if both versions support the key
// i.e. the key is a more "generic" loader; support for it implies support for the whole group
// This is useful when authors forget to add Purpur etc. to all versions
// TODO: make abstracted from source backend
var loaderCompatGroups = map[string][]string{
	"bukkit":     {"purpur", "paper", "spigot"},
	"bungeecord": {"waterfall"},
}

func getProjectTypeFolder(projectType string, fileLoaders []string, packLoaders []string) (string, error) {
	if projectType == "modpack" {
		return "", errors.New("this command should not be used to add Modrinth modpacks, and importing of Modrinth modpacks is not yet supported")
	} else if projectType == "resourcepack" {
		return "resourcepacks", nil
	} else if projectType == "shader" {
		bestLoaderIdx := math.MaxInt
		for _, v := range fileLoaders {
			idx := slices.Index(loaderPreferenceList, v)
			if idx != -1 && idx < bestLoaderIdx {
				bestLoaderIdx = idx
			}
		}
		if bestLoaderIdx > -1 && bestLoaderIdx < math.MaxInt {
			return loaderFolders[loaderPreferenceList[bestLoaderIdx]], nil
		}
		return "shaderpacks", nil
	} else if projectType == "mod" {
		// Look up pack loaders in the list of loaders (note this is currently filtered to neoforge)
		bestLoaderIdx := math.MaxInt
		for _, v := range fileLoaders {
			if slices.Contains(packLoaders, v) {
				idx := slices.Index(loaderPreferenceList, v)
				if idx != -1 && idx < bestLoaderIdx {
					bestLoaderIdx = idx
				}
			}
		}
		if bestLoaderIdx > -1 && bestLoaderIdx < math.MaxInt {
			return loaderFolders[loaderPreferenceList[bestLoaderIdx]], nil
		}

		// Datapack loader is "datapack"
		if slices.Contains(fileLoaders, "datapack") {
			if viper.GetString("datapack-folder") != "" {
				return viper.GetString("datapack-folder"), nil
			} else {
				return "", errors.New("set the datapack-folder option to use datapacks")
			}
		}
		// Default to "mods" for mod type
		return "mods", nil
	} else {
		return "", fmt.Errorf("unknown project type %s", projectType)
	}
}

var urlRegexes = [...]*regexp.Regexp{
	// Slug/version number regex from https://github.com/modrinth/labrinth/blob/1679a3f844497d756d0cf272c5374a5236eabd42/src/util/validate.rs#L8
	regexp.MustCompile("^https?://(www.)?modrinth\\.com/(?P<urlCategory>[^/]+)/(?P<slug>[a-zA-Z0-9!@$()`.+,_\"-]{3,64})(?:/version/(?P<version>[a-zA-Z0-9!@$()`.+,_\"-]{1,32}))?"),
	// Version/project IDs are more restrictive: [a-zA-Z0-9]+ (base62)
	regexp.MustCompile("^https?://cdn\\.modrinth\\.com/data/(?P<slug>[a-zA-Z0-9]+)/versions/(?P<versionID>[a-zA-Z0-9]+)/(?P<filename>[^/]+)$"),
}

var urlCategories = []string{
	"mod", "plugin", "datapack", "shader", "resourcepack", "modpack",
}

func parseUrl(input string, slug *string, version *string, versionID *string, filename *string) (err error) {
	for _, r := range urlRegexes {
		matches := r.FindStringSubmatch(input)
		if matches != nil {
			if i := r.SubexpIndex("urlCategory"); i >= 0 {
				if !slices.Contains(urlCategories, matches[i]) {
					return errors.New("unknown project type: " + matches[i])
				}
			}
			if i := r.SubexpIndex("slug"); i >= 0 {
				*slug = matches[i]
			}
			if i := r.SubexpIndex("version"); i >= 0 {
				*version = matches[i]
			}
			if i := r.SubexpIndex("versionID"); i >= 0 {
				*versionID = matches[i]
			}
			if i := r.SubexpIndex("filename"); i >= 0 {
				parsed, err := url.PathUnescape(matches[i])
				if err != nil {
					return err
				}
				*filename = parsed
			}
			return nil
		}
	}
	return errors.New("not a valid Modrinth URL")
}

func compareLoaderLists(a []string, b []string) int32 {
	var compat []string
	for k, v := range loaderCompatGroups {
		if slices.Contains(a, k) && slices.Contains(b, k) {
			// Prerequisite loader is in both lists; add compat group
			compat = append(compat, v...)
		}
	}
	// Prefer loaders; principally mods over datapacks (Modrinth backend handles filtering)
	minIdxA := math.MaxInt
	for _, v := range a {
		if slices.Contains(compat, v) {
			// Ignore loaders in compat groups for comparison
			continue
		}
		idx := slices.Index(loaderPreferenceList, v)
		if idx != -1 && idx < minIdxA {
			minIdxA = idx
		}
	}
	minIdxB := math.MaxInt
	for _, v := range b {
		if slices.Contains(compat, v) {
			// Ignore loaders in compat groups for comparison
			continue
		}
		idx := slices.Index(loaderPreferenceList, v)
		if idx != -1 && idx < minIdxA {
			return 1 // B has more preferable loaders
		}
		if idx != -1 && idx < minIdxB {
			minIdxB = idx
		}
	}
	if minIdxA < minIdxB {
		return -1 // A has more preferable loaders
	}
	return 0
}

// filterVersionsByReleaseType returns only the versions whose release type is at least as stable as releaseType
func filterVersionsByReleaseType(versions []*modrinthApi.Version, releaseType string) []*modrinthApi.Version {
	var filtered []*modrinthApi.Version
	for _, v := range versions {
		fileReleaseType := core.ReleaseTypeRelease
		if v.VersionType != nil && *v.VersionType != "" {
			fileReleaseType = *v.VersionType
		}
		if core.ReleaseTypeAccepts(releaseType, fileReleaseType) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func findLatestVersion(versions []*modrinthApi.Version, gameVersions []string, useFlexVer bool) *modrinthApi.Version {
	latestValidVersion := versions[0]
	bestGameVersion := core.HighestSliceIndex(gameVersions, versions[0].GameVersions)
	for _, v := range versions[1:] {
		gameVersionIdx := core.HighestSliceIndex(gameVersions, v.GameVersions)

		var compare int32
		if useFlexVer {
			// Use FlexVer to compare versions
			compare = flexver.Compare(*v.VersionNumber, *latestValidVersion.VersionNumber)
		}

		if compare == 0 {
			// Prefer later specified game versions (main version specified last)
			compare = int32(gameVersionIdx - bestGameVersion)
		}
		if compare == 0 {
			compare = compareLoaderLists(latestValidVersion.Loaders, v.Loaders)
		}
		if compare == 0 {
			// Other comparisons are equal, compare date instead
			if v.DatePublished.After(*latestValidVersion.DatePublished) {
				compare = 1
			}
		}
		if compare > 0 {
			latestValidVersion = v
			bestGameVersion = gameVersionIdx
		}
	}

	return latestValidVersion
}

// getLatestVersion finds the latest version of a project that is compatible with the pack, and whose
// release type is at least as stable as releaseType. If releaseType is empty, the pack's default
// release type is used (see Pack.GetReleaseType).
func getLatestVersion(projectID string, name string, pack core.Pack, releaseType string) (*modrinthApi.Version, error) {
	if releaseType == "" {
		releaseType = pack.GetReleaseType()
	}

	gameVersions, err := pack.GetSupportedMCVersions()
	if err != nil {
		return nil, err
	}
	var loaders []string
	if viper.GetString("datapack-folder") != "" {
		loaders = append(pack.GetCompatibleLoaders(), withDatapackPathMRLoaders...)
	} else {
		loaders = append(pack.GetCompatibleLoaders(), defaultMRLoaders...)
	}

	result, err := mrDefaultClient.Versions.ListVersions(projectID, modrinthApi.ListVersionsOptions{
		GameVersions: gameVersions,
		Loaders:      loaders,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest version: %w", err)
	}
	if len(result) == 0 {
		// TODO: retry with datapack specified, to determine what the issue is? or just request all and filter afterwards
		return nil, errors.New("no valid versions found\n\tUse the 'packwiz settings acceptable-versions' command to accept more game versions\n\tTo use datapacks, add a datapack loader mod and specify the datapack-folder option with the folder this mod loads datapacks from")
	}

	result = filterVersionsByReleaseType(result, releaseType)
	if len(result) == 0 {
		return nil, fmt.Errorf("no versions found matching release type %q or more stable\n\tUse the 'packwiz settings release-type' command to change the pack default, or the --release-type flag to override it for this mod", releaseType)
	}

	// TODO: option to always compare using flexver?
	// TODO: ask user which one to use?
	flexverLatest := findLatestVersion(result, gameVersions, true)
	releaseDateLatest := findLatestVersion(result, gameVersions, false)
	if flexverLatest != releaseDateLatest && releaseDateLatest.VersionNumber != nil && flexverLatest.VersionNumber != nil {
		fmt.Printf("Warning: Modrinth versions for %s inconsistent between latest version number and newest release date (%s vs %s)\n", name, *flexverLatest.VersionNumber, *releaseDateLatest.VersionNumber)
	}

	return releaseDateLatest, nil
}

func getSide(mod *modrinthApi.Project) string {
	server := mod.ServerSide != nil && shouldDownloadOnSide(*mod.ServerSide)
	client := mod.ClientSide != nil && shouldDownloadOnSide(*mod.ClientSide)

	if server && client {
		return core.UniversalSide
	} else if server {
		return core.ServerSide
	} else if client {
		return core.ClientSide
	} else {
		return ""
	}
}

func shouldDownloadOnSide(side string) bool {
	return side == "required" || side == "optional"
}

func getBestHash(v *modrinthApi.File) (string, string) {
	// Try preferred hashes first; SHA1 is required for Modrinth pack exporting, but
	// so is SHA512, so we can't win with the current one-hash format
	val, exists := v.Hashes["sha512"]
	if exists {
		return "sha512", val
	}
	val, exists = v.Hashes["sha256"]
	if exists {
		return "sha256", val
	}
	val, exists = v.Hashes["sha1"]
	if exists {
		return "sha1", val
	}
	val, exists = v.Hashes["murmur2"] // (not defined in Modrinth pack spec, use with caution)
	if exists {
		return "murmur2", val
	}

	//none of the preferred hashes are present, just get the first one
	for key, val := range v.Hashes {
		return key, val
	}

	//No hashes were present
	return "", ""
}

func getInstalledProjectIDs(index *core.Index) []string {
	var installedProjects []string
	// Get modids of all mods
	mods, err := index.LoadAllMods()
	if err != nil {
		fmt.Printf("Failed to determine existing projects: %v\n", err)
	} else {
		for _, mod := range mods {
			data, ok := mod.GetParsedUpdateData("modrinth")
			if ok {
				updateData, ok := data.(mrUpdateData)
				if ok {
					if len(updateData.ProjectID) > 0 {
						installedProjects = append(installedProjects, updateData.ProjectID)
					}
				}
			}
		}
	}
	return installedProjects
}

func resolveVersion(project *modrinthApi.Project, version string) (*modrinthApi.Version, error) {
	// If it exists in the version list, it is already a version ID (and doesn't need querying further)
	if slices.Contains(project.Versions, version) {
		versionData, err := mrDefaultClient.Versions.Get(version)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch version %s: %v", version, err)
		}
		return versionData, nil
	}

	// Look up all versions
	// TODO: PR a version number filter to Modrinth?
	versionsList, err := mrDefaultClient.Versions.ListVersions(*project.ID, modrinthApi.ListVersionsOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch version list for %s: %v", *project.ID, err)
	}
	// Traverse in reverse order: Modrinth knossos always gives the oldest file precedence over having the version number path
	for i := len(versionsList) - 1; i >= 0; i-- {
		if *versionsList[i].VersionNumber == version {
			return versionsList[i], nil
		}
	}
	return nil, fmt.Errorf("unable to find version %s", version)
}

// buildDependencyList converts a version's raw dependency list into the persisted core.ModDependency form,
// resolving dependencies that are only specified by version ID into their project ID.
func buildDependencyList(version *modrinthApi.Version) []core.ModDependency {
	if len(version.Dependencies) == 0 {
		return nil
	}

	var versionIDsToResolve []string
	for _, dep := range version.Dependencies {
		if dep.ProjectID == nil && dep.VersionID != nil {
			versionIDsToResolve = append(versionIDsToResolve, *dep.VersionID)
		}
	}

	resolvedProjectIDs := make(map[string]string) // version ID -> project ID
	if len(versionIDsToResolve) > 0 {
		versions, err := mrDefaultClient.Versions.GetMultiple(versionIDsToResolve)
		if err != nil {
			fmt.Printf("Warning: failed to resolve dependency version IDs: %v\n", err)
		} else {
			for _, v := range versions {
				if v.ID != nil && v.ProjectID != nil {
					resolvedProjectIDs[*v.ID] = *v.ProjectID
				}
			}
		}
	}

	var deps []core.ModDependency
	seen := make(map[string]bool)
	for _, dep := range version.Dependencies {
		var projectID string
		if dep.ProjectID != nil {
			projectID = *dep.ProjectID
		} else if dep.VersionID != nil {
			projectID = resolvedProjectIDs[*dep.VersionID]
		}
		if projectID == "" || seen[projectID] {
			continue
		}
		seen[projectID] = true

		depType := "required"
		if dep.DependencyType != nil && *dep.DependencyType != "" {
			depType = *dep.DependencyType
		}

		deps = append(deps, core.ModDependency{
			ID:   projectID,
			Type: depType,
		})
	}
	return deps
}
