package modrinth

import (
	"fmt"
	"slices"

	"github.com/evictedcucumber/packwiz/core"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
)

// Sinytra Connector lets NeoForge load mods that are only made for Fabric. Those mods expect Fabric API, which Forgified
// Fabric API stands in for: the real one has no NeoForge version, and mustn't be added next to it.
const (
	connectorProjectID          = "u58R1TMW"
	forgifiedFabricAPIProjectID = "Aqlf1Shp"
	fabricAPIProjectID          = "P7dR8mSH"
)

// runsFabricMods reports whether a pack can run mods that only have a Fabric version, given the projects it has added:
// it has to be a NeoForge pack with both Sinytra Connector and Forgified Fabric API.
func runsFabricMods(pack core.Pack, installedProjects []string) bool {
	return slices.Contains(pack.GetCompatibleLoaders(), "neoforge") &&
		slices.Contains(installedProjects, connectorProjectID) &&
		slices.Contains(installedProjects, forgifiedFabricAPIProjectID)
}

// packRunsFabricMods is runsFabricMods for the projects the pack has added on disk
func packRunsFabricMods(pack core.Pack) (bool, error) {
	index, err := pack.LoadIndex()
	if err != nil {
		return false, err
	}
	return runsFabricMods(pack, getInstalledProjectIDs(&index)), nil
}

// needsConnector reports whether a version is for Fabric and none of the loaders of the pack, so that the pack only
// runs it through Sinytra Connector
func needsConnector(version *modrinthApi.Version, pack core.Pack) bool {
	packLoaders := pack.GetCompatibleLoaders()
	return slices.Contains(version.Loaders, "fabric") &&
		!slices.ContainsFunc(version.Loaders, func(loader string) bool { return slices.Contains(packLoaders, loader) })
}

// noticeFabricMod tells the user when a mod that is being added is one for Fabric, as it doesn't run by itself
func noticeFabricMod(name string, version *modrinthApi.Version, pack core.Pack) {
	if needsConnector(version, pack) {
		fmt.Printf("Notice: %s is a Fabric mod; it runs on NeoForge through Sinytra Connector\n", name)
	}
}
