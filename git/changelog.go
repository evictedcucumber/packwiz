package git

import "github.com/evictedcucumber/packwiz/changelog"

// gitRepository is the pack's repository, as a changelog reads it.
type gitRepository struct {
	r repo
}

var _ changelog.Repository = gitRepository{}

func init() {
	changelog.OpenRepository = func() (changelog.Repository, error) {
		r, err := openRepo(packRoot())
		if err != nil {
			return nil, err
		}
		return gitRepository{r}, nil
	}
}

func (g gitRepository) Head() (string, error) {
	return g.r.head()
}

func (g gitRepository) Log(since string) ([]changelog.Commit, error) {
	return g.r.log(since)
}

func (g gitRepository) LastChangedIn(file string) (string, error) {
	return g.r.lastChangedIn(file)
}

// CommitPending is "packwiz git commit"
func (g gitRepository) CommitPending() error {
	return runCommit(false)
}

func (g gitRepository) PendingCommits() ([]changelog.Commit, error) {
	return pendingCommits()
}
