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

// ErrNoRepository is what the error from OpenRepository is when the pack isn't in a repository, or there is nothing to
// open one with. That isn't a failure for a changelog: it is made without a log to read, from the pack alone. Anything
// else that stops a repository being opened is one.
var ErrNoRepository = errors.New("the pack isn't in a repository")

// NoRepository is an error with the given reason that is ErrNoRepository, for OpenRepository to return.
func NoRepository(reason string) error {
	return noRepositoryError(reason)
}

type noRepositoryError string

func (e noRepositoryError) Error() string { return string(e) }

func (e noRepositoryError) Is(target error) bool { return target == ErrNoRepository }

// OpenRepository opens the repository the pack is in, or fails with an error that is ErrNoRepository if it isn't in one.
var OpenRepository = func() (Repository, error) {
	return nil, NoRepository("no version control has been set up for changelogs")
}
