package core

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"github.com/unascribed/FlexVer/go/flexver"
)

type MavenMetadata struct {
	XMLName    xml.Name `xml:"metadata"`
	GroupID    string   `xml:"groupId"`
	ArtifactID string   `xml:"artifactId"`
	Versioning struct {
		Release  string `xml:"release"`
		Latest   string `xml:"latest"`
		Versions struct {
			Version []string `xml:"version"`
		} `xml:"versions"`
		LastUpdated string `xml:"lastUpdated"`
	} `xml:"versioning"`
}

type ModLoaderVersions struct {
	// All Versions of this modloader
	Versions []string
	// The Latest/preferred version for this modloader
	Latest string
}

type ModLoaderComponent struct {
	// An identifier for the modloader
	Name string
	// A user-friendly name
	FriendlyName string
	// Retrieves the list of all modloader versions. Modloader versions are always filtered to those compatible
	// with a specific minecraft version.
	VersionListGetter func(q VersionListQuery) (*ModLoaderVersions, error)
}

var modLoadersList = []ModLoaderComponent{
	{
		Name:              "neoforge",
		FriendlyName:      "NeoForge",
		VersionListGetter: fetchForNeoForge,
	},
}

// A map containing information about all supported modloaders.
// Can be indexed by the [ModLoaderComponent]'s name, which serves as an identifier.
var ModLoaders = createModloaderMap(modLoadersList)

func createModloaderMap(input []ModLoaderComponent) map[string]ModLoaderComponent {
	var mlMap = make(map[string]ModLoaderComponent)
	for _, loader := range input {
		mlMap[loader.Name] = loader
	}
	return mlMap
}

type QueryType int

const (
	// The Latest field will contain the last released loader version
	Latest QueryType = iota
	// The Latest field will contain the loader version recommended for use
	Recommended
)

type VersionListQuery struct {
	// Which loader to query versions for
	Loader ModLoaderComponent
	// Which minecraft version the returned loader versions should be compatible with
	McVersion string
	// Determines how the latest version is determined
	QueryType QueryType
}

func MakeQuery(loader ModLoaderComponent, mcVersion string) VersionListQuery {
	return VersionListQuery{
		Loader:    loader,
		McVersion: mcVersion,
		QueryType: Latest,
	}
}

func (in VersionListQuery) WithQueryType(queryType QueryType) VersionListQuery {
	return VersionListQuery{
		Loader:    in.Loader,
		McVersion: in.McVersion,
		QueryType: queryType,
	}
}

// Queries the versions of a modloader
func DoQuery(q VersionListQuery) (*ModLoaderVersions, error) {
	return q.Loader.VersionListGetter(q)
}

func fetchForgeStyle(q VersionListQuery, url string) (*ModLoaderVersions, error) {
	// Forge style:
	// each version is formatted like `mcVersion-forgeVersion`
	// eg: `1.18.1-39.0.18`
	return fetchMavenWithFilterMap(q, url, func(version string) *string {
		before, after, f := strings.Cut(version, "-")
		if !f {
			// The version didn't have a dash? Lets just reject it entirely
			return nil
		}
		if before != q.McVersion {
			// The part before the dash should match the mc version we're looking for
			return nil
		}
		// The part after the dash is the actual version, and the part we care about
		return &after
	})
}

func fetchNeoForgeStyle(q VersionListQuery, url string) (*ModLoaderVersions, error) {
	// NeoForge style, for mc versions above 26.1:
	// If minecraft versions are in the form of year.major.minor-(pre-release),
	// then neoforge versions are in the form of year.major.minor.x(nf-pre-release)+(pre-release)
	// Eg, for minecraft 26.1-snapshot-6, neoforge has versions 26.1.0.0-alpha.9+snapshot-6 and 26.1.0.0-alpha.10+snapshot-6

	var mcSplit = strings.SplitN(q.McVersion, ".", 3)

	if len(mcSplit) < 2 {
		// This does not appear to be a minecraft version that's formatted in a way that neoforge's scheme supports
		return nil, fmt.Errorf("packwiz cannot detect compatible %s versions for this Minecraft version (%s)", q.Loader.FriendlyName, q.McVersion)
	}

	var year = mcSplit[0]
	var major = mcSplit[1]
	var minor = "0"
	var prerelease = ""

	if len(mcSplit) == 3 {
		minor, prerelease, _ = strings.Cut(mcSplit[2], "-")
	} else {
		major, prerelease, _ = strings.Cut(mcSplit[1], "-")
	}

	var requiredPrefix = year + "." + major + "." + minor
	var requiredSuffix = prerelease

	return fetchMavenWithFilterMap(q, url, func(version string) *string {
		if strings.HasPrefix(version, requiredPrefix) && strings.HasSuffix(version, requiredSuffix) {
			return &version
		}
		return nil
	})
}

func fetchOldNeoForgeStyle(q VersionListQuery, url string) (*ModLoaderVersions, error) {
	// NeoForge style, for mc versions below 1.21.11:
	// If minecraft versions are in the form of 1.a.b, then neoforge versions are in the form of a.b.x
	// Eg, for minecraft 1.20.6, neoforge version 20.6.2 and 20.6.83-beta would both be valid versions
	// for minecraft 1.20.2, neoforge version 20.2.23-beta
	// for minecraft 1.21, neoforge version 21.0.143 would be valid

	var mcSplit = strings.Split(q.McVersion, ".")
	if len(mcSplit) < 2 {
		// This does not appear to be a minecraft version that's formatted in a way that neoforge's scheme supports
		return nil, fmt.Errorf("packwiz cannot detect compatible %s versions for this Minecraft version (%s)", q.Loader.FriendlyName, q.McVersion)
	}
	var mcMajor = mcSplit[1]
	var mcMinor = "0"
	if len(mcSplit) > 2 {
		mcMinor = mcSplit[2]
	}
	// Note that the period at the end is significant, we don't want to match `21.10.43` as being for 1.21.1 (instead of 1.21.10)
	var requiredPrefix = mcMajor + "." + mcMinor + "."

	return fetchMavenWithFilterMap(q, url, func(version string) *string {
		if !strings.HasPrefix(version, requiredPrefix) {
			// Reject NeoForge versions that don't have the right prefix for this mc version
			return nil
		}
		return &version
	})
}

// Retrieves all versions through maven metadata, and then processes the using the provided `filterMap` function.
// When `filterMap` returns a string, the version will be renamed to the provided string. If `nil` is returned, the
// version is marked as invalid and will not be considered in the result.
func fetchMavenWithFilterMap(q VersionListQuery, url string, filterMap func(version string) *string) (*ModLoaderVersions, error) {
	res, err := GetWithUA(url, "application/xml")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	dec := xml.NewDecoder(res.Body)
	out := MavenMetadata{}
	err = dec.Decode(&out)
	if err != nil {
		return nil, err
	}

	// Pass all of the versions listed in the maven through our filterMap
	versions := make([]string, 0, len(out.Versioning.Versions.Version))
	for _, v := range out.Versioning.Versions.Version {
		mappedV := filterMap(v)
		if mappedV != nil {
			versions = append(versions, *mappedV)
		}
	}

	if len(versions) == 0 {
		return nil, errors.New("no " + q.Loader.FriendlyName + " versions available for " + q.McVersion)
	}

	// Determine the latest release
	var latestRelease = ""
	release := filterMap(out.Versioning.Release)
	latest := filterMap(out.Versioning.Latest)
	if release != nil {
		latestRelease = *release
	} else if latest != nil {
		latestRelease = *latest
	} else {
		// Maven was useless, just rely on flexver sorting
		flexver.VersionSlice(versions).Sort()
		latestRelease = versions[len(versions)-1]
	}
	return &ModLoaderVersions{versions, latestRelease}, nil
}

func fetchForNeoForge(q VersionListQuery) (*ModLoaderVersions, error) {
	// NeoForge reused Forge's versioning scheme for 1.20.1, but moved to their own versioning scheme for 1.20.2 and above
	if q.McVersion == "1.20.1" {
		return fetchForgeStyle(q, "https://maven.neoforged.net/releases/net/neoforged/forge/maven-metadata.xml")
	} else {
		// Mojang changed versioning schemes between 1.21.11 and 26.1
		// The old versioning scheme was 1.major.minor, which changed to year.major(.patch)
		// With snapshot releases for 26.1 being eg 26.1-snapshot.1

		// NeoForge's versioning scheme changed with that. Luckily all versions using the old versioning
		// scheme start with "1.". Well, some things don't (alpha versions, snapshot versions, etc) but NeoForge
		// doesn't support any of those either way.
		if strings.HasPrefix(q.McVersion, "1.") {
			return fetchOldNeoForgeStyle(q, "https://maven.neoforged.net/releases/net/neoforged/neoforge/maven-metadata.xml")
		} else {
			return fetchNeoForgeStyle(q, "https://maven.neoforged.net/releases/net/neoforged/neoforge/maven-metadata.xml")
		}
	}
}

func ComponentToFriendlyName(component string) string {
	if component == "minecraft" {
		return "Minecraft"
	}
	loader, ok := ModLoaders[component]
	if ok {
		return loader.FriendlyName
	} else {
		return component
	}
}

// HighestSliceIndex returns the highest index of the given values in the slice (-1 if no value is found in the slice)
func HighestSliceIndex(slice []string, values []string) int {
	highest := -1
	for _, val := range values {
		for i, v := range slice {
			if v == val && i > highest {
				highest = i
			}
		}
	}
	return highest
}
