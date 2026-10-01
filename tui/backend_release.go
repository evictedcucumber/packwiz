package tui

import (
	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/git"
)

// releaseData is what the release screen shows: the release that the pack's changes would make, and what hasn't been
// committed yet.
type releaseData struct {
	preview changelog.Preview
	// pending are the messages of the commits that committing would make, if the pack is in a repository
	pending []string
	// notices are what reading all this said along the way, in plain text
	notices []string
}

// releaseOutcome is what making a release did.
type releaseOutcome struct {
	// made is whether a release was made: it isn't if there was nothing to release
	made    bool
	version string
	// tag is the tag the release was committed with, if it was
	tag string
	// noChanges says why nothing was released, if nothing was
	noChanges string
	notices   []string
}

// releaseBackend is what the release screen needs of the pack and of its repository. Each reads the pack when it is called,
// as the other backends do.
type releaseBackend interface {
	// loadRelease works out the next release, as "packwiz changelog" does, and what "packwiz git commit" would commit if the
	// pack is in a repository. version, if it isn't empty, is used for the release instead of the one that is worked out.
	loadRelease(version string) (releaseData, error)
	// commit commits the pack, as "packwiz git commit" does, and says which commits it made.
	commit() (committed []string, notices []string, err error)
	// release records a release, as "packwiz changelog release" does, which fails if the pack has changes that aren't
	// committed. With tag it
	// also commits the release and tags it, as "packwiz git release" does.
	release(version string, tag bool) (releaseOutcome, error)
	// saveChangelog writes CHANGELOG.md from the releases recorded so far, with what isn't released yet above them, as
	// "packwiz changelog --save" does, and says where and what it had to say along the way.
	saveChangelog() (path string, notices []string, err error)
}

func (packBackend) loadRelease(version string) (releaseData, error) {
	preview, err := changelog.LoadPreview("", version)
	if err != nil {
		return releaseData{}, err
	}
	data := releaseData{preview: preview, notices: preview.Notices}
	if preview.InRepository {
		pending, notices, err := git.PlanCommit()
		if err != nil {
			return releaseData{}, err
		}
		data.pending = pending
		data.notices = append(data.notices, notices...)
	}
	return data, nil
}

func (packBackend) commit() ([]string, []string, error) {
	return git.CommitAll()
}

func (packBackend) release(version string, tag bool) (releaseOutcome, error) {
	if tag {
		released, err := git.ReleaseAndTag(version, "")
		if err != nil {
			return releaseOutcome{}, err
		}
		return releaseOutcome{
			made: released.Made, version: released.Release.Version, tag: released.Tag,
			noChanges: released.NoChanges, notices: released.Notices,
		}, nil
	}
	released, err := changelog.MakeRelease(version, "")
	if err != nil {
		return releaseOutcome{}, err
	}
	return releaseOutcome{
		made: released.Made, version: released.Release.Version,
		noChanges: released.NoChanges, notices: released.Notices,
	}, nil
}

func (packBackend) saveChangelog() (string, []string, error) {
	return changelog.SaveMarkdown()
}
