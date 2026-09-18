package changelog

import (
	"errors"
	"fmt"
	"testing"
)

// fakeRepo is a repository whose history the test writes, standing in for git.
type fakeRepo struct {
	// commits are the commits made, oldest first
	commits []fakeCommit
	// pending are the commits that CommitPending will make. It makes an initial commit if there is nothing at all.
	pending []Commit
	// commitErr, if set, is what CommitPending fails with
	commitErr error
	// touched is the hash of the last commit to change each file
	touched map[string]string
	// events records what was asked of it, in order
	events []string
	// logSince records what each call to Log was asked for
	logSince []string
}

type fakeCommit struct {
	hash string
	Commit
}

// newFakeRepo makes a repository that changelogs are made from for the duration of the test.
func newFakeRepo(t *testing.T) *fakeRepo {
	t.Helper()
	repo := &fakeRepo{touched: make(map[string]string)}
	old := OpenRepository
	OpenRepository = func() (Repository, error) { return repo, nil }
	t.Cleanup(func() { OpenRepository = old })
	return repo
}

// commit adds a commit to the history, and returns its hash.
func (r *fakeRepo) commit(subject string, body ...string) string {
	hash := fmt.Sprintf("%040x", len(r.commits)+1)
	r.commits = append(r.commits, fakeCommit{hash, commit(subject, body...)})
	return hash
}

func (r *fakeRepo) Head() (string, error) {
	r.events = append(r.events, "Head")
	if len(r.commits) == 0 {
		return "", errors.New("the repository has no commits")
	}
	return r.commits[len(r.commits)-1].hash, nil
}

func (r *fakeRepo) Log(since string) ([]Commit, error) {
	r.events = append(r.events, "Log")
	r.logSince = append(r.logSince, since)
	start := 0
	if since != "" {
		start = -1
		for i, c := range r.commits {
			if c.hash == since {
				start = i + 1
			}
		}
		if start < 0 {
			return nil, fmt.Errorf("unknown revision %s", since)
		}
	}
	var commits []Commit
	for _, c := range r.commits[start:] {
		commits = append(commits, c.Commit)
	}
	return commits, nil
}

func (r *fakeRepo) LastChangedIn(file string) (string, error) {
	r.events = append(r.events, "LastChangedIn")
	return r.touched[file], nil
}

func (r *fakeRepo) CommitPending() error {
	r.events = append(r.events, "CommitPending")
	if r.commitErr != nil {
		return r.commitErr
	}
	if len(r.commits) == 0 && len(r.pending) == 0 {
		r.commit("chore(pack): initial commit")
	}
	for _, c := range r.pending {
		r.commit(c.Subject, c.Body)
	}
	r.pending = nil
	return nil
}

func (r *fakeRepo) PendingCommits() ([]Commit, error) {
	r.events = append(r.events, "PendingCommits")
	return append([]Commit(nil), r.pending...), nil
}

// asked reports whether the repository was asked for something.
func (r *fakeRepo) asked(event string) bool {
	for _, e := range r.events {
		if e == event {
			return true
		}
	}
	return false
}
