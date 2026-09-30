package tui

import (
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/modrinth"
)

// depsBackend is what the dependencies screen needs of the pack: reporting what its mods depend on, and saving what was
// looked up. Each reads the pack when it is called, as the other backends do.
type depsBackend interface {
	// loadDependencies reports what the pack's mods depend on, as "packwiz modrinth deps" does. With refresh, the
	// dependencies of every mod are looked up again, not only of those that record none.
	loadDependencies(refresh bool) (*modrinth.DependencyReport, error)
	// saveDependencies saves the dependencies that were looked up into the pack, and says what couldn't be saved.
	saveDependencies(report *modrinth.DependencyReport) (failures []string, err error)
}

func (packBackend) loadDependencies(refresh bool) (*modrinth.DependencyReport, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return nil, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return nil, err
	}
	return modrinth.Dependencies(pack, index, refresh)
}

func (packBackend) saveDependencies(report *modrinth.DependencyReport) ([]string, error) {
	failures, err := report.Save()
	texts := make([]string, len(failures))
	for i, f := range failures {
		texts[i] = f.Error()
	}
	return texts, err
}
