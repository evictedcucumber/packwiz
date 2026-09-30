package tui

import (
	"fmt"
	"slices"

	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
)

// minecraftVersions are the versions of Minecraft that a pack can be for.
type minecraftVersions struct {
	// latest is the newest release, and latestSnapshot the newest snapshot
	latest, latestSnapshot string
	valid                  func(version string) bool
}

// initBackend is what the screen that makes a pack needs: what versions there are to choose from, and making it. It works
// in the current directory.
type initBackend interface {
	// defaultName is what the pack would be called, going by the folder it is in.
	defaultName() string
	// minecraftVersions looks up what versions of Minecraft there are, as "packwiz init" does.
	minecraftVersions() (minecraftVersions, error)
	// loaderVersion works out what version of a mod loader to use for a Minecraft version: the one that was chosen if it is
	// one there is, else the latest if none was. It fails if the one that was chosen isn't.
	loaderVersion(loader, mcVersion, chosen string) (string, error)
	// createPack makes the pack, as "packwiz init" does.
	createPack(pack cmd.NewPack) error
}

func (packBackend) defaultName() string { return cmd.DefaultPackName() }

func (packBackend) minecraftVersions() (minecraftVersions, error) {
	manifest, err := cmdshared.GetValidMCVersions()
	if err != nil {
		return minecraftVersions{}, err
	}
	return minecraftVersions{latest: manifest.Latest.Release, latestSnapshot: manifest.Latest.Snapshot, valid: manifest.IsValid}, nil
}

func (packBackend) loaderVersion(loader, mcVersion, chosen string) (string, error) {
	versions, err := cmd.LoaderVersions(loader, mcVersion)
	if err != nil {
		return "", fmt.Errorf("couldn't look up the versions of %s: %w", core.LoaderName(loader), err)
	}
	if chosen == "" {
		return versions.Latest, nil
	}
	if !slices.Contains(versions.Versions, cmd.LoaderVersion(core.ModLoaders[loader], mcVersion, chosen)) {
		return "", fmt.Errorf("%q isn't a version of %s for Minecraft %s", chosen, core.LoaderName(loader), mcVersion)
	}
	return chosen, nil
}

func (packBackend) createPack(pack cmd.NewPack) error {
	return cmd.CreatePack(pack)
}
