package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

// git runs a git command in the current directory, failing the test if it fails, and returns what it printed.
func git(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		var stderr []byte
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = exitErr.Stderr
		}
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, stderr)
	}
	return strings.TrimRight(string(out), "\n")
}

// setUpRepo makes an empty git repository in a fresh temp directory the working directory. It is isolated from the
// user's own git configuration, so signing, hooks and templates configured there can't affect (or be affected by)
// the tests.
func setUpRepo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))

	dir := cmdtest.Chdir(t)
	// Never treat a repository around the temp directory as this one
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	git(t, "init", "-q")
	for k, v := range map[string]string{
		"user.name": "Test", "user.email": "test@example.com", "commit.gpgsign": "false", "tag.gpgsign": "false",
	} {
		git(t, "config", k, v)
	}
}

// testPack is a pack whose root is root, relative to the repository; empty if the pack is at the top of it.
type testPack struct{ root string }

// setUpPack writes an empty pack at root, and points packwiz at it with prompts auto-accepted.
func setUpPack(t *testing.T, root, version string) testPack {
	t.Helper()
	if root != "" {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", root, err)
		}
	}
	cmdtest.SetViper(t, "pack-file", filepath.Join(root, "pack.toml"))
	cmdtest.SetViperBool(t, "non-interactive", true)

	pack := core.Pack{Name: "Test Pack", Version: version, PackFormat: core.CurrentPackFormat, Versions: map[string]string{"minecraft": "1.21"}}
	if err := pack.Write(); err != nil {
		t.Fatalf("failed to write pack.toml: %v", err)
	}
	p := testPack{root}
	p.write(t, "index.toml", "hash-format = \"sha256\"\n")
	return p
}

func (p testPack) path(rel string) string { return filepath.Join(p.root, filepath.FromSlash(rel)) }

func (p testPack) write(t *testing.T, rel, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p.path(rel)), 0o755); err != nil {
		t.Fatalf("failed to create directory for %s: %v", rel, err)
	}
	if err := os.WriteFile(p.path(rel), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", rel, err)
	}
}

func (p testPack) remove(t *testing.T, rel string) {
	t.Helper()
	if err := os.Remove(p.path(rel)); err != nil {
		t.Fatalf("failed to remove %s: %v", rel, err)
	}
}

func (p testPack) mod(t *testing.T, name, side, version string) {
	t.Helper()
	p.modFile(t, core.Mod{
		Name: name, FileName: strings.ToLower(name) + "-" + version + ".jar", Version: version, Side: side,
		Download: core.ModDownload{HashFormat: "sha256", Hash: "hash-" + version},
	})
}

func (p testPack) modFile(t *testing.T, mod core.Mod) {
	t.Helper()
	mod.SetMetaPath(p.path("mods/" + strings.ToLower(mod.Name) + core.MetaExtension))
	if _, _, err := mod.Write(); err != nil {
		t.Fatalf("failed to write mod %s: %v", mod.Name, err)
	}
}

func commit(t *testing.T) string {
	t.Helper()
	var err error
	out := cmdtest.CaptureStdout(t, func() { err = runCommit(false) })
	if err != nil {
		t.Fatalf("runCommit() returned error: %v\noutput: %s", err, out)
	}
	return out
}

func release(t *testing.T, override string) string {
	t.Helper()
	var err error
	out := cmdtest.CaptureStdout(t, func() { err = runRelease(override) })
	if err != nil {
		t.Fatalf("runRelease() returned error: %v\noutput: %s", err, out)
	}
	return out
}

func headMessage(t *testing.T) string {
	t.Helper()
	return git(t, "log", "-1", "--format=%B")
}

func commitCount(t *testing.T) int {
	t.Helper()
	if err := exec.Command("git", "rev-parse", "--verify", "--quiet", "HEAD").Run(); err != nil {
		return 0 // no commits yet
	}
	n := git(t, "rev-list", "--count", "HEAD")
	count := 0
	for _, c := range n {
		count = count*10 + int(c-'0')
	}
	return count
}

func requireClean(t *testing.T) {
	t.Helper()
	if status := git(t, "status", "--porcelain"); status != "" {
		t.Errorf("working tree isn't clean after committing:\n%s", status)
	}
}

func TestCommitDescribesEachKindOfChange(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	p.write(t, "config/sodium.json", "{}")

	commit(t)
	if got := headMessage(t); got != "chore(pack): initial commit" {
		t.Errorf("first commit message = %q, want the initial commit message", got)
	}
	requireClean(t)

	steps := []struct {
		name   string
		change func()
		want   string
	}{
		{"client mod added", func() { p.mod(t, "Iris", core.ClientSide, "1.0") }, "feat(mods): add Iris 1.0 (client)"},
		{"client mod updated", func() { p.mod(t, "Iris", core.ClientSide, "1.1") }, "fix(mods): update Iris 1.0 -> 1.1 (client)"},
		{"config changed", func() { p.write(t, "config/sodium.json", `{"a": 1}`) }, "fix(config): change config/sodium.json"},
		{"config added", func() { p.write(t, "config/iris.json", "{}") }, "fix(config): add config/iris.json"},
		{"config removed", func() { p.remove(t, "config/iris.json") }, "fix(config): remove config/iris.json"},
		{"client mod removed", func() { p.remove(t, "mods/iris.pw.toml") }, "feat(mods): remove Iris 1.1 (client)"},
		{
			"server mod added",
			func() { p.mod(t, "Lithium", core.ServerSide, "0.12.0") },
			"feat(mods)!: add Lithium 0.12.0 (server)\n\n" + wantBreakingFooter,
		},
		{
			"server mod updated",
			func() { p.mod(t, "Lithium", core.ServerSide, "0.12.1") },
			"feat(mods)!: update Lithium 0.12.0 -> 0.12.1 (server)\n\n" + wantBreakingFooter,
		},
		{
			"pinning a mod",
			func() {
				p.modFile(t, core.Mod{
					Name: "Sodium", FileName: "sodium-0.5.7.jar", Version: "0.5.7", Side: core.ClientSide, Pin: true,
					Download: core.ModDownload{HashFormat: "sha256", Hash: "hash-0.5.7"},
				})
			},
			"chore(pack): update pack files",
		},
		{
			"mods and config together",
			func() {
				p.mod(t, "Zoom", core.ClientSide, "2.0")
				p.write(t, "config/zoom.json", "{}")
			},
			// Listed in path order, as changes always are
			"feat(pack): add 1 mod, update 1 config file\n\n- add config/zoom.json\n- add Zoom 2.0 (client)",
		},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			before := commitCount(t)
			step.change()

			out := commit(t)

			if got := headMessage(t); got != step.want {
				t.Errorf("commit message =\n%s\nwant\n%s", got, step.want)
			}
			if got := commitCount(t); got != before+1 {
				t.Errorf("commit count = %d, want %d", got, before+1)
			}
			if want := "Committed: " + strings.SplitN(step.want, "\n", 2)[0]; !strings.Contains(out, want) {
				t.Errorf("output = %q, want it to report %q", out, want)
			}
			requireClean(t)
		})
	}
}

func TestCommitIncludesTheRefreshedIndex(t *testing.T) {
	// The index on disk knows nothing about this config file, as if "packwiz refresh" hadn't been run
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)

	p.write(t, "config/late.json", "{}")
	commit(t)

	if index := git(t, "show", "HEAD:index.toml"); !strings.Contains(index, "config/late.json") {
		t.Errorf("the committed index.toml doesn't list config/late.json:\n%s", index)
	}
	requireClean(t)
}

func TestCommitDryRunChangesNothing(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")
	before := commitCount(t)
	indexBefore, err := os.ReadFile("index.toml")
	if err != nil {
		t.Fatalf("failed to read index.toml: %v", err)
	}

	var runErr error
	out := cmdtest.CaptureStdout(t, func() { runErr = runCommit(true) })
	if runErr != nil {
		t.Fatalf("runCommit(dryRun) returned error: %v", runErr)
	}

	if want := "feat(mods)!: add Lithium 0.12.0 (server)"; !strings.Contains(out, want) {
		t.Errorf("dry run output = %q, want it to contain %q", out, want)
	}
	if got := commitCount(t); got != before {
		t.Errorf("commit count = %d, want %d; a dry run must not commit", got, before)
	}
	if git(t, "status", "--porcelain") == "" {
		t.Error("working tree is clean after a dry run; the change should still be pending")
	}
	if indexAfter, _ := os.ReadFile("index.toml"); string(indexAfter) != string(indexBefore) {
		t.Errorf("a dry run rewrote index.toml:\n%s", indexAfter)
	}
}

func TestCommitWithNothingToCommit(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	before := commitCount(t)

	for name, run := range map[string]func() error{
		"commit":  func() error { return runCommit(false) },
		"dry run": func() error { return runCommit(true) },
	} {
		t.Run(name, func(t *testing.T) {
			var err error
			out := cmdtest.CaptureStdout(t, func() { err = run() })
			if err != nil {
				t.Fatalf("returned error: %v", err)
			}
			if !strings.Contains(out, "Nothing to commit.") {
				t.Errorf("output = %q, want Nothing to commit.", out)
			}
		})
	}
	if got := commitCount(t); got != before {
		t.Errorf("commit count = %d, want %d", got, before)
	}
}

func TestCommitPackInSubdirectory(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "pack", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	p.write(t, "config/sodium.json", "{}")
	// Files elsewhere in the repository, staged and not, aren't part of the pack
	if err := os.WriteFile("README.md", []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	git(t, "add", "README.md")
	if err := os.WriteFile("notes.txt", []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to write notes: %v", err)
	}

	commit(t)

	// The README was already staged, but it isn't in the pack, so it isn't in the pack's commit
	if committed := git(t, "show", "--name-only", "--format=", "HEAD"); strings.Contains(committed, "README.md") || strings.Contains(committed, "notes.txt") {
		t.Errorf("the commit included files outside the pack:\n%s", committed)
	}
	if staged := git(t, "diff", "--cached", "--name-only"); staged != "README.md" {
		t.Errorf("staged files = %q, want the README to still be staged for the user's own commit", staged)
	}

	// Changes are described relative to the pack, not the repository
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")
	commit(t)
	if got, want := headMessage(t), "feat(mods)!: add Lithium 0.12.0 (server)\n\n"+wantBreakingFooter; got != want {
		t.Errorf("commit message =\n%s\nwant\n%s", got, want)
	}
}

func TestSnapshotAtAgreesWithTheWorkingTree(t *testing.T) {
	for name, root := range map[string]string{"pack at top of repository": "", "pack in subdirectory": "pack"} {
		t.Run(name, func(t *testing.T) {
			setUpRepo(t)
			p := setUpPack(t, root, "1.0.0")
			p.mod(t, "Sodium", core.ClientSide, "0.5.7")
			p.mod(t, "Lithium", core.ServerSide, "0.12.0")
			p.write(t, "config/nested/sodium.json", `{"a": 1}`)
			commit(t)

			// The same content read from disk and read from git must describe the pack identically...
			_, index, err := changelog.LoadRefreshed()
			if err != nil {
				t.Fatalf("LoadRefreshed() returned error: %v", err)
			}
			onDisk, err := changelog.TakeSnapshot(index)
			if err != nil {
				t.Fatalf("TakeSnapshot() returned error: %v", err)
			}
			r, err := openRepo(packRoot())
			if err != nil {
				t.Fatalf("openRepo() returned error: %v", err)
			}
			fromGit, err := r.snapshotAt("HEAD", "index.toml")
			if err != nil {
				t.Fatalf("snapshotAt() returned error: %v", err)
			}
			if !reflect.DeepEqual(fromGit, onDisk) {
				t.Errorf("snapshot from git =\n%+v\nsnapshot from disk =\n%+v", fromGit, onDisk)
			}
			if len(fromGit.Mods) != 2 || len(fromGit.Files) != 1 {
				t.Errorf("snapshot = %+v, want 2 mods and 1 file", fromGit)
			}

			// ...and HEAD keeps describing what was committed, whatever has happened to the files since
			p.mod(t, "Sodium", core.ClientSide, "0.6.0")
			p.remove(t, "config/nested/sodium.json")
			again, err := r.snapshotAt("HEAD", "index.toml")
			if err != nil {
				t.Fatalf("snapshotAt() returned error: %v", err)
			}
			if !reflect.DeepEqual(again, fromGit) {
				t.Errorf("snapshot of HEAD changed with the working tree:\n%+v\nwant\n%+v", again, fromGit)
			}
		})
	}
}

func TestSnapshotAtBeforeThePackExisted(t *testing.T) {
	setUpRepo(t)
	// README.md is one of the files packwiz ignores by default, so it isn't part of the pack that comes later
	if err := os.WriteFile("README.md", []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	git(t, "add", "README.md")
	git(t, "commit", "-q", "-m", "before the pack")
	r, err := openRepo(".")
	if err != nil {
		t.Fatalf("openRepo() returned error: %v", err)
	}

	snap, err := r.snapshotAt("HEAD", "index.toml")
	if err != nil {
		t.Fatalf("snapshotAt() returned error: %v", err)
	}
	if len(snap.Mods) != 0 || len(snap.Files) != 0 {
		t.Errorf("snapshot = %+v, want empty for a commit without a pack", snap)
	}

	// The pack's first commit in an existing repository is described as adding everything
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	if got := headMessage(t); got != "feat(mods): add Sodium 0.5.7 (client)" {
		t.Errorf("commit message = %q, want the pack's contents described as added", got)
	}
}

func TestSnapshotAtToleratesAStaleIndex(t *testing.T) {
	// A commit whose index lists a file that wasn't committed still gives a snapshot of what was
	setUpRepo(t)
	setUpPack(t, "", "1.0.0")
	if err := os.WriteFile("index.toml", []byte("hash-format = \"sha256\"\n\n[[files]]\nfile = \"config/ghost.json\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write index: %v", err)
	}
	git(t, "add", "index.toml", "pack.toml")
	git(t, "commit", "-q", "-m", "stale index")
	r, err := openRepo(".")
	if err != nil {
		t.Fatalf("openRepo() returned error: %v", err)
	}

	snap, err := r.snapshotAt("HEAD", "index.toml")
	if err != nil {
		t.Fatalf("snapshotAt() returned error: %v", err)
	}
	if len(snap.Files) != 0 {
		t.Errorf("Files = %v, want the missing file left out", snap.Files)
	}
}

func TestOpenRepoOutsideARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't installed")
	}
	dir := cmdtest.Chdir(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "gitconfig"))

	_, err := openRepo(".")
	if err == nil || !strings.Contains(err.Error(), "git init") {
		t.Errorf("openRepo() error = %v, want one telling the user to run git init", err)
	}
}

func TestRepoHasCommitsAndDirty(t *testing.T) {
	setUpRepo(t)
	r, err := openRepo(".")
	if err != nil {
		t.Fatalf("openRepo() returned error: %v", err)
	}

	if has, err := r.hasCommits(); err != nil || has {
		t.Errorf("hasCommits() = %v, %v, want false for a new repository", has, err)
	}
	if dirty, err := r.dirty(); err != nil || dirty {
		t.Errorf("dirty() = %v, %v, want false for an empty repository", dirty, err)
	}

	if err := os.WriteFile("a.txt", []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if dirty, err := r.dirty(); err != nil || !dirty {
		t.Errorf("dirty() = %v, %v, want true with an untracked file", dirty, err)
	}

	if err := r.commitAll("chore: add a"); err != nil {
		t.Fatalf("commitAll() returned error: %v", err)
	}
	if has, err := r.hasCommits(); err != nil || !has {
		t.Errorf("hasCommits() = %v, %v, want true after a commit", has, err)
	}
	if dirty, err := r.dirty(); err != nil || dirty {
		t.Errorf("dirty() = %v, %v, want false after committing everything", dirty, err)
	}
}

func TestReleaseCommitsAndTags(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	p.write(t, "config/sodium.json", "{}")
	commit(t)

	out := release(t, "")

	if !strings.Contains(out, "Committed and tagged v1.0.0") {
		t.Errorf("output = %q, want it to report the tag", out)
	}
	if got := headMessage(t); got != "chore(release): 1.0.0" {
		t.Errorf("release commit message = %q", got)
	}
	// An annotated tag, on the release commit
	if kind := git(t, "cat-file", "-t", "v1.0.0"); kind != "tag" {
		t.Errorf("v1.0.0 is a %q object, want an annotated tag", kind)
	}
	if tagged, head := git(t, "rev-parse", "v1.0.0^{commit}"), git(t, "rev-parse", "HEAD"); tagged != head {
		t.Errorf("v1.0.0 points at %s, want the release commit %s", tagged, head)
	}
	changed := git(t, "show", "--name-only", "--format=", "HEAD")
	for _, want := range []string{"CHANGELOG.md", "changelog.toml"} {
		if !strings.Contains(changed, want) {
			t.Errorf("release commit doesn't include %s:\n%s", want, changed)
		}
	}
	requireClean(t)

	// A later release continues from it
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")
	commit(t)
	release(t, "")
	if got := headMessage(t); got != "chore(release): 2.0.0" {
		t.Errorf("second release commit message = %q, want 2.0.0 for a server mod", got)
	}
	if tags := git(t, "tag", "--list"); tags != "v1.0.0\nv2.0.0" {
		t.Errorf("tags = %q, want v1.0.0 and v2.0.0", tags)
	}
	requireClean(t)
}

func TestReleaseRefusesUncommittedChanges(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	p.mod(t, "Sodium", core.ClientSide, "0.5.8") // not committed
	before := commitCount(t)

	var err error
	cmdtest.CaptureStdout(t, func() { err = runRelease("") })

	if err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("runRelease() error = %v, want one about uncommitted changes", err)
	}
	if got := commitCount(t); got != before {
		t.Errorf("commit count = %d, want %d", got, before)
	}
	if tags := git(t, "tag", "--list"); tags != "" {
		t.Errorf("tags = %q, want none", tags)
	}
	for _, name := range []string{changelog.HistoryFile, changelog.MarkdownFile} {
		if _, statErr := os.Stat(name); statErr == nil {
			t.Errorf("%s was written although the release was refused", name)
		}
	}
}

func TestReleaseRefusesARepositoryWithNoCommits(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")

	var err error
	cmdtest.CaptureStdout(t, func() { err = runRelease("") })

	if err == nil || !strings.Contains(err.Error(), "no commits") {
		t.Errorf("runRelease() error = %v, want one saying there are no commits", err)
	}
}

func TestReleaseWithNothingToRelease(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	release(t, "")
	before := commitCount(t)

	out := release(t, "")

	if !strings.Contains(out, "No changes since the last release (1.0.0).") {
		t.Errorf("output = %q, want a no-changes message", out)
	}
	if got := commitCount(t); got != before {
		t.Errorf("commit count = %d, want %d; there was nothing to release", got, before)
	}
	if tags := git(t, "tag", "--list"); tags != "v1.0.0" {
		t.Errorf("tags = %q, want only v1.0.0", tags)
	}
}

func TestReleaseTellsYouHowToFinishWhenTheTagAlreadyExists(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	git(t, "tag", "v1.0.0") // in the way of the first release

	var err error
	cmdtest.CaptureStdout(t, func() { err = runRelease("") })

	if err == nil {
		t.Fatal("runRelease() succeeded although the tag already existed")
	}
	// The release itself was made and committed, so the message must say what is left to do
	for _, want := range []string{"released and committed 1.0.0", "couldn't tag", "yourself"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
	if got := headMessage(t); got != "chore(release): 1.0.0" {
		t.Errorf("HEAD message = %q, want the release to have been committed", got)
	}
}
