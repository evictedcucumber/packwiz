package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
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

// openRepo checks that git is installed and that dir is inside a working tree.
func openRepo(dir string) (repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return repo{}, errors.New("git isn't installed, or isn't on your PATH")
	}
	r := repo{dir}
	if _, err := r.run("", "rev-parse", "--is-inside-work-tree"); err != nil {
		return repo{}, fmt.Errorf("%s isn't inside a git repository; run \"git init\" first", dir)
	}
	return r, nil
}

// run runs git with the given arguments, feeding it stdin if that isn't empty, and returns what it printed.
func (r repo) run(stdin string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
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

// commitAll commits every change under the pack root, and nothing outside it, even if something else was staged.
func (r repo) commitAll(message string) error {
	if _, err := r.run("", "add", "--all", "--", "."); err != nil {
		return err
	}
	// The message goes in on stdin so it can't be mistaken for an option, whatever it contains
	_, err := r.run(message, "commit", "--file=-", "--", ".")
	return err
}

// tag creates an annotated tag on the current commit.
func (r repo) tag(name, message string) error {
	_, err := r.run("", "tag", "--annotate", "--message="+message, name)
	return err
}
