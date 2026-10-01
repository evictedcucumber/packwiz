package git

import "github.com/evictedcucumber/packwiz/changelog"

// gitRepository is the pack's repository, as a changelog reads it.
type gitRepository struct {
	r repo
}

var _ changelog.Repository = gitRepository{}
var _ changelog.CommitReporter = gitRepository{}

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

// CommitPending is "packwiz git commit", except that it fails, before committing anything, if the pack is in a bad state:
// there are files that packwiz doesn't recognise, which a release can't say belong in it, or config files that nothing
// claims.
func (g gitRepository) CommitPending() error {
	c, steps, err := prepareCommit(false)
	if err != nil {
		return err
	}
	if err := c.errBadState(); err != nil {
		return err
	}
	return c.run(steps, printCommitted)
}

// CommitPendingReporting is CommitPending, telling report the message of each commit instead of printing it
func (g gitRepository) CommitPendingReporting(report func(message string)) error {
	c, steps, err := prepareCommit(false)
	if err != nil {
		return err
	}
	if err := c.errBadState(); err != nil {
		return err
	}
	return c.run(steps, report)
}

func (g gitRepository) PendingCommits() ([]changelog.Commit, error) {
	return pendingCommits()
}
