package tui

import (
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/modrinth"
)

// addBackend is what the add screen needs of the pack and of Modrinth: looking for a project, working out what adding one
// would do, and doing it. Each reads the pack when it is called, as the other backends do.
type addBackend interface {
	// search looks for projects of a kind that suit the pack by what they are called.
	search(query, kind string) (*modrinth.SearchResults, error)
	// planAdd works out what adding the project that ref says would do, as "packwiz modrinth add" does before it asks. ref
	// is the address of a page of the project, or its slug or ID; releaseType, if it isn't empty, is the least stable kind of
	// version to accept.
	planAdd(ref, releaseType string) (*modrinth.AddPlan, error)
	// applyAdd adds the project, or updates it if the pack has it, and with dependencies its dependencies too.
	applyAdd(plan *modrinth.AddPlan, dependencies bool) (*modrinth.AddResult, error)
}

func (packBackend) search(query, kind string) (*modrinth.SearchResults, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return nil, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return nil, err
	}
	return modrinth.Search(pack, index, query, kind)
}

func (packBackend) planAdd(ref, releaseType string) (*modrinth.AddPlan, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return nil, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return nil, err
	}
	return modrinth.PlanAdd(pack, index, ref, modrinth.AddOptions{ReleaseType: releaseType})
}

func (packBackend) applyAdd(plan *modrinth.AddPlan, dependencies bool) (*modrinth.AddResult, error) {
	return plan.Apply(dependencies)
}
