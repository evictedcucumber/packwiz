package cmd

import (
	"maps"
	"slices"

	"github.com/evictedcucumber/packwiz/core"
)

// UpdateOffer is a new version of a mod, which it can be updated to.
type UpdateOffer struct {
	// Mod is the mod as it is now
	Mod *core.Mod
	// Source is what found the update, the name of an updater in core.Updaters, and does it
	Source string
	// Change says what the update is, usually "old -> new" (see core.UpdateCheck)
	Change string
	// state is what the updater wants back to do the update
	state interface{}
}

// UpdateFailure is a check for an update that couldn't be made.
type UpdateFailure struct {
	// Name is what couldn't be checked: a mod's name, or the name of a source if it failed for all of its mods
	Name string
	Err  error
	// Count is how many mods this is for: one, or all of the mods of a source that failed as a whole
	Count int
}

// UpdateSearch is what looking for updates to some mods found.
type UpdateSearch struct {
	// Offers are the updates that can be made, in the order of their sources and then of the mods they were given in
	Offers []UpdateOffer
	// Failures are the checks that couldn't be made, which isn't the same as there being no update
	Failures []UpdateFailure
	// Pinned are the mods that have an update that isn't offered, as they are pinned
	Pinned []*core.Mod
	// Unsupported are the mods that no source that the pack has can update
	Unsupported []*core.Mod
	// Checked is how many checks were made, which is one for each mod and source that can update it
	Checked int
}

// FailedChecks is how many mods there is no answer for.
func (s UpdateSearch) FailedChecks() int {
	n := 0
	for _, f := range s.Failures {
		n += f.Count
	}
	return n
}

// FindUpdates looks for an update to each of the mods, by asking their sources, as "packwiz update --all" does. It
// changes nothing, and says nothing: what it finds is returned, for the caller to show. progress, if it isn't nil, is
// told which source is being asked, as that is what takes the time.
func FindUpdates(pack core.Pack, mods []*core.Mod, progress func(source string)) UpdateSearch {
	var search UpdateSearch

	bySource := make(map[string][]*core.Mod)
	for _, mod := range mods {
		supported := false
		for name := range mod.Update {
			if _, ok := core.Updaters[name]; !ok {
				continue
			}
			supported = true
			bySource[name] = append(bySource[name], mod)
		}
		if !supported {
			search.Unsupported = append(search.Unsupported, mod)
		}
	}

	for _, source := range slices.Sorted(maps.Keys(bySource)) {
		group := bySource[source]
		if progress != nil {
			progress(source)
		}
		checks, err := core.Updaters[source].CheckUpdate(group, pack)
		if err != nil {
			search.Failures = append(search.Failures, UpdateFailure{Name: source, Err: err, Count: len(group)})
			search.Checked += len(group)
			continue
		}
		for i, check := range checks {
			search.Checked++
			switch {
			case check.Error != nil:
				search.Failures = append(search.Failures, UpdateFailure{Name: group[i].Name, Err: check.Error, Count: 1})
			case !check.UpdateAvailable:
			case group[i].Pin:
				search.Pinned = append(search.Pinned, group[i])
			default:
				search.Offers = append(search.Offers, UpdateOffer{Mod: group[i], Source: source, Change: check.UpdateString, state: check.CachedState})
			}
		}
	}
	return search
}

// ApplyUpdates updates the mods of the offers, each by its source, and writes their metadata files and puts them in the
// index, which is for the caller to write along with the pack (see core.Pack.SaveIndex). It returns the mods that were
// updated and the errors of what wasn't: an update that fails doesn't stop the others.
func ApplyUpdates(index *core.Index, offers []UpdateOffer) (updated []*core.Mod, errs []error) {
	bySource := make(map[string][]UpdateOffer)
	for _, offer := range offers {
		bySource[offer.Source] = append(bySource[offer.Source], offer)
	}
	for _, source := range slices.Sorted(maps.Keys(bySource)) {
		group := bySource[source]
		mods := make([]*core.Mod, len(group))
		states := make([]interface{}, len(group))
		for i, offer := range group {
			mods[i], states[i] = offer.Mod, offer.state
		}
		if err := core.Updaters[source].DoUpdate(mods, states); err != nil {
			errs = append(errs, err)
			continue
		}
		for _, mod := range mods {
			if err := index.SaveMod(mod); err != nil {
				errs = append(errs, err)
				continue
			}
			updated = append(updated, mod)
		}
	}
	return updated, errs
}
