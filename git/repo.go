package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/evictedcucumber/packwiz/changelog"
)

// repo runs git commands from a directory inside a repository, which is the pack's root. Paths given to git are
// relative to that directory, and commands that take a pathspec are limited to it.
type repo struct {
	dir string
}

// gitError is a failed git command, carrying what git said about it.
type gitError struct {
	args   []string
	stderr string
	err    error
}

func (e *gitError) Error() string {
	msg := strings.TrimSpace(e.stderr)
	if msg == "" {
		msg = e.err.Error()
	}
	return fmt.Sprintf("git %s: %s", e.args[0], msg)
}

func (e *gitError) Unwrap() error { return e.err }

// openRepo checks that git is installed and that dir is inside a working tree. If git isn't installed, or there is no
// repository, the error is a changelog.ErrNoRepository, as a changelog can be made without one but the commands that
// use git can't. A repository that git refuses to use (because of who owns it, say) is some other error: it has a log,
// so that isn't the same as there being none.
func openRepo(dir string) (repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return repo{}, changelog.NoRepository("git isn't installed, or isn't on your PATH")
	}
	r := repo{dir}
	// Git exits the same way for all of these, so what it says is all that tells them apart, and that is translated
	if _, err := r.runWith([]string{"LC_ALL=C"}, "", "rev-parse", "--is-inside-work-tree"); err != nil {
		var gitErr *gitError
		if !errors.As(err, &gitErr) || !strings.Contains(gitErr.stderr, "not a git repository") {
			return repo{}, err
		}
		// "." says nothing about where it is
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
		return repo{}, changelog.NoRepository(fmt.Sprintf("%s isn't inside a git repository; run \"git init\" first", dir))
	}
	return r, nil
}

// run runs git with the given arguments, feeding it stdin if that isn't empty, and returns what it printed.
func (r repo) run(stdin string, args ...string) ([]byte, error) {
	return r.runWith(nil, stdin, args...)
}

// runWith is run with more environment variables set, given as "NAME=value".
func (r repo) runWith(env []string, stdin string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, &gitError{args, stderr.String(), err}
	}
	return out, nil
}

// head is the hash of the current commit.
func (r repo) head() (string, error) {
	out, err := r.run("", "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// log lists the commits made after since, oldest first, up to the current commit; all of them if since is empty.
func (r repo) log(since string) ([]changelog.Commit, error) {
	// Something that starts with "-" would be taken for an option rather than a commit
	if strings.HasPrefix(since, "-") {
		return nil, fmt.Errorf("%q isn't a commit", since)
	}
	args := []string{"log", "--reverse", "--no-merges", "--format=%s%x1f%b%x1e"}
	if since != "" {
		args = append(args, since+"..HEAD")
	}
	out, err := r.run("", args...)
	if err != nil {
		return nil, err
	}

	var commits []changelog.Commit
	for _, record := range strings.Split(string(out), "\x1e") {
		// Each commit ends in a newline, which comes after the separator
		subject, body, _ := strings.Cut(strings.TrimLeft(record, "\n"), "\x1f")
		if subject == "" && strings.TrimSpace(body) == "" {
			continue
		}
		commits = append(commits, changelog.Commit{Subject: subject, Body: strings.TrimSpace(body)})
	}
	return commits, nil
}

// lastChangedIn is the hash of the last commit to change a file, given relative to the pack root, or "" if there
// hasn't been one.
func (r repo) lastChangedIn(file string) (string, error) {
	out, err := r.run("", "log", "-1", "--format=%H", "--", file)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// hasCommits reports whether the repository has a commit yet; a new one doesn't.
func (r repo) hasCommits() (bool, error) {
	_, err := r.run("", "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err == nil {
		return true, nil
	}
	// --verify --quiet exits with 1, printing nothing, for a revision that doesn't exist
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

// dirty reports whether anything under the pack root has uncommitted changes, including new files that aren't
// ignored by git.
func (r repo) dirty() (bool, error) {
	out, err := r.run("", "status", "--porcelain", "--", ".")
	if err != nil {
		return false, err
	}
	return len(bytes.TrimSpace(out)) > 0, nil
}

// dirtyPaths lists the files under the pack root that differ from HEAD, or are new and not ignored by git, relative to
// the pack root. It needs a commit to compare with.
func (r repo) dirtyPaths() ([]string, error) {
	changed, err := r.run("", "diff", "--name-only", "--relative", "-z", "HEAD", "--", ".")
	if err != nil {
		return nil, err
	}
	untracked, err := r.run("", "ls-files", "--others", "--exclude-standard", "-z", "--", ".")
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, name := range strings.Split(string(changed)+string(untracked), "\x00") {
		if name != "" {
			paths = append(paths, name)
		}
	}
	return paths, nil
}

// commitAll commits every change under the pack root, and nothing outside it, even if something else was staged.
func (r repo) commitAll(message string) error {
	return r.commit(message, ".")
}

// commitPaths commits the given files, given relative to the pack root, as they are on disk, and nothing else, even
// if something else was staged. A file that has been deleted is committed as deleted.
func (r repo) commitPaths(message string, paths ...string) error {
	if err := r.commit(message, paths...); err != nil {
		// Don't leave what was staged for a commit that didn't happen for someone else's commit to pick up
		_, _ = r.run("", append([]string{"reset", "--quiet", "--"}, paths...)...)
		return err
	}
	return nil
}

func (r repo) commit(message string, paths ...string) error {
	if _, err := r.run("", append([]string{"add", "--all", "--"}, paths...)...); err != nil {
		return err
	}
	// The message goes in on stdin so it can't be mistaken for an option, whatever it contains
	_, err := r.run(message, append([]string{"commit", "--file=-", "--"}, paths...)...)
	return err
}

// tag creates an annotated tag on the current commit.
func (r repo) tag(name, message string) error {
	_, err := r.run("", "tag", "--annotate", "--message="+message, name)
	return err
}
