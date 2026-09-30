package tui

import (
	"fmt"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/modrinth"
)

// checkBackend is what the check screen needs of the pack: checking it, working out what could be fixed and fixing it.
// Each reads the pack when it is called, as the other backends do.
type checkBackend interface {
	// checkPack checks the pack as "packwiz validate" does, which asks Modrinth about mods that record no dependencies.
	checkPack() (*modrinth.Check, error)
	// planFixes works out what to change to fix what the check found, as "packwiz fix" does, which also needs the network.
	planFixes(check *modrinth.Check) (*modrinth.Fixes, error)
	// applyFixes makes the changes, to the pack as it is now, and says how many files it changed.
	applyFixes(fixes *modrinth.Fixes) (changed int, err error)
}

func (packBackend) checkPack() (*modrinth.Check, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return nil, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return nil, err
	}
	return modrinth.CheckPack(pack, index), nil
}

func (packBackend) planFixes(check *modrinth.Check) (*modrinth.Fixes, error) {
	return check.PlanFixes(), nil
}

func (packBackend) applyFixes(fixes *modrinth.Fixes) (int, error) {
	changed, err := fixes.Apply()
	if err != nil {
		return changed, fmt.Errorf("%w", err)
	}
	return changed, nil
}
