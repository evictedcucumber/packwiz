package tui

import (
	"fmt"

	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
)

// updateOffer is a new version that a mod can be updated to, as the screens show it.
type updateOffer struct {
	name string
	// path is where the mod's metadata file is
	path string
	// change says what the update is, usually "old -> new"
	change string
	offer  cmd.UpdateOffer
}

// updateFailure is a check for an update that couldn't be made.
type updateFailure struct {
	name string
	err  string
}

// updatesFound is what looking for updates found.
type updatesFound struct {
	offers   []updateOffer
	failures []updateFailure
	// failedChecks is how many mods the failures are for, which can be all of those of a source that failed as a whole
	failedChecks int
	// pinned are the names of the mods that have an update that isn't offered, as they are pinned
	pinned []string
	// unsupported are the names of the mods that nothing can update
	unsupported []string
	// upToDate is how many mods were checked and have nothing newer
	upToDate int
	// notices are what the lookups had to say as they went, in plain text
	notices []string
}

// updateOutcome is what updating mods did.
type updateOutcome struct {
	updated []string
	// updatedPaths are where the metadata files of those are
	updatedPaths []string
	// failed says why what wasn't updated wasn't
	failed []string
}

// updatesBackend is what the updates screen needs of the pack: looking for what can be updated, and updating it. Each
// reads the pack when it is called, as the other backends do.
type updatesBackend interface {
	// findUpdates looks for an update to every mod in the pack, as "packwiz update --all" does. progress says which source
	// is being asked, as that takes the time.
	findUpdates(progress func(string)) (updatesFound, error)
	// checkUpdate looks for an update to one mod, as "packwiz update <mod>" does.
	checkUpdate(path string) (updatesFound, error)
	// applyUpdates updates the mods that the offers are for, each one as it is in the pack now.
	applyUpdates(offers []updateOffer) (updateOutcome, error)
}

func (packBackend) findUpdates(progress func(string)) (updatesFound, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return updatesFound{}, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return updatesFound{}, err
	}
	mods, err := index.LoadAllMods()
	if err != nil {
		return updatesFound{}, fmt.Errorf("failed to read the pack's mods: %w", err)
	}
	// The index lists them in no order, and what is found is shown in the order they were asked about
	cmd.SortMods(mods)
	return searchUpdates(pack, mods, progress), nil
}

func (packBackend) checkUpdate(path string) (updatesFound, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return updatesFound{}, err
	}
	mod, err := core.LoadMod(path)
	if err != nil {
		return updatesFound{}, err
	}
	return searchUpdates(pack, []*core.Mod{&mod}, nil), nil
}

// searchUpdates looks for updates to mods, and describes what it found for the screens.
func searchUpdates(pack core.Pack, mods []*core.Mod, progress func(string)) updatesFound {
	var search cmd.UpdateSearch
	notices := notice.Collect(func() { search = cmd.FindUpdates(pack, mods, progress) })

	found := updatesFound{failedChecks: search.FailedChecks(), upToDate: max(search.Checked-len(search.Offers)-search.FailedChecks()-len(search.Pinned), 0)}
	for _, n := range notices {
		if n.Level != notice.Muted {
			found.notices = append(found.notices, n.Text)
		}
	}
	for _, o := range search.Offers {
		found.offers = append(found.offers, updateOffer{name: o.Mod.Name, path: o.Mod.GetFilePath(), change: o.Change, offer: o})
	}
	for _, f := range search.Failures {
		found.failures = append(found.failures, updateFailure{name: f.Name, err: f.Err.Error()})
	}
	for _, m := range search.Pinned {
		found.pinned = append(found.pinned, m.Name)
	}
	for _, m := range search.Unsupported {
		found.unsupported = append(found.unsupported, m.Name)
	}
	return found
}

func (packBackend) applyUpdates(offers []updateOffer) (updateOutcome, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return updateOutcome{}, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return updateOutcome{}, err
	}

	var outcome updateOutcome
	var apply []cmd.UpdateOffer
	for _, o := range offers {
		// The mod as it is now: it may have been pinned, or changed in some other way, since the update was found
		mod, err := core.LoadMod(o.path)
		if err != nil {
			outcome.failed = append(outcome.failed, fmt.Sprintf("%s: %v", o.name, err))
			continue
		}
		if mod.Pin {
			outcome.failed = append(outcome.failed, o.name+" is pinned")
			continue
		}
		o.offer.Mod = &mod
		apply = append(apply, o.offer)
	}
	if len(apply) == 0 {
		return outcome, nil
	}

	var updated []*core.Mod
	var errs []error
	notice.Collect(func() { updated, errs = cmd.ApplyUpdates(&index, apply) })
	for _, m := range updated {
		outcome.updated = append(outcome.updated, m.Name)
		outcome.updatedPaths = append(outcome.updatedPaths, m.GetFilePath())
	}
	for _, e := range errs {
		outcome.failed = append(outcome.failed, e.Error())
	}

	// What was updated is saved even if something else wasn't, so that the index matches the files
	if len(updated) > 0 {
		if err := pack.SaveIndex(index); err != nil {
			return outcome, err
		}
	}
	return outcome, nil
}
