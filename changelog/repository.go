package changelog

import "errors"

// Repository is the version control a pack is released from. A release is made from the commits since the last one,
// so it needs to see them, and to make sure the pack's changes are among them first. The git package implements it,
// and registers itself in OpenRepository, which is how this package reaches it without importing it.
type Repository interface {
	// Head is the hash of the current commit.
	Head() (string, error)
	// Log lists the commits made after since, oldest first, up to the current commit. An empty since means all of them.
	// It is an error if since isn't a commit in the repository.
	Log(since string) ([]Commit, error)
	// LastChangedIn is the hash of the last commit to change a file, given relative to the pack root, or "" if there
	// hasn't been one.
	LastChangedIn(file string) (string, error)
	// CommitPending commits every change to the pack that hasn't been committed yet, as "packwiz git commit" does.
	CommitPending() error
	// PendingCommits describes the commits CommitPending would make, in order, without making any.
	PendingCommits() ([]Commit, error)
}

// OpenRepository opens the repository the pack is in, or fails if it isn't in one.
var OpenRepository = func() (Repository, error) {
	return nil, errors.New("no version control has been set up for changelogs")
}
