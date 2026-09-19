package git

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

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

// unversioned writes a mod as ones added before versions were recorded are: with no version, but with a source that
// can look it up.
func (p testPack) unversioned(t *testing.T, src *cmdtest.VersionSource, name, side, versionID string) {
	t.Helper()
	p.modFile(t, core.Mod{
		Name: name, FileName: strings.ToLower(name) + "-" + versionID + ".jar", Side: side,
		Download: core.ModDownload{HashFormat: "sha256", Hash: "hash-" + versionID},
		Update:   src.UpdateData(versionID),
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
	out := cmdtest.CaptureStdout(t, func() { err = runRelease(override, "") })
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(data)
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

// A pack with a mod that reads its files from a folder keeps them there: they are what commits and the changelog call
// config, whatever they are (they needn't be in config/), and files outside the folder aren't tracked at all.
func TestCommitAndReleaseFollowTheConfigDir(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	src := cmdtest.RegisterConfigDirSource(t, "testdefaults", "configureddefaults")
	p.modFile(t, core.Mod{
		Name: "Defaults", FileName: "defaults-1.0.jar", Version: "1.0", Side: core.ClientSide,
		Download: core.ModDownload{HashFormat: "sha256", Hash: "hash-1.0"},
		Update:   src.UpdateData(),
	})
	p.write(t, "configureddefaults/config/sodium.json", "{}")
	p.write(t, "config/sodium.json", "{}")
	release(t, "")

	if index := git(t, "show", "HEAD:index.toml"); strings.Contains(index, `"config/sodium.json"`) ||
		!strings.Contains(index, `"configureddefaults/config/sodium.json"`) {
		t.Errorf("the committed index should list only the file in configureddefaults/:\n%s", index)
	}

	steps := []struct {
		name   string
		change func()
		want   string
	}{
		{
			"config in the folder changed",
			func() { p.write(t, "configureddefaults/config/sodium.json", `{"a": 1}`) },
			"fix(config): change configureddefaults/config/sodium.json",
		},
		{
			"a file in the folder that isn't in config/ added",
			func() { p.write(t, "configureddefaults/options.txt", "fov:90") },
			"fix(config): add configureddefaults/options.txt",
		},
		{
			"a file outside the folder changed, which the pack doesn't have",
			func() { p.write(t, "config/sodium.json", `{"a": 2}`) },
			"chore(pack): update pack files",
		},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			step.change()

			commit(t)

			if got := headMessage(t); got != step.want {
				t.Errorf("commit message =\n%s\nwant\n%s", got, step.want)
			}
			requireClean(t)
		})
	}

	release(t, "")

	changelogMD := git(t, "show", "HEAD:CHANGELOG.md")
	for _, want := range []string{
		"### Config\n\n",
		"- Changed `configureddefaults/config/sodium.json`",
		"- Added `configureddefaults/options.txt`",
	} {
		if !strings.Contains(changelogMD, want) {
			t.Errorf("the committed changelog is missing %q:\n%s", want, changelogMD)
		}
	}
	if strings.Contains(changelogMD, "`config/sodium.json`") {
		t.Errorf("the committed changelog mentions a file that is outside the folder:\n%s", changelogMD)
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
			w, err := changelog.LoadWorking(true)
			if err != nil {
				t.Fatalf("LoadWorking() returned error: %v", err)
			}
			onDisk, err := w.Snapshot()
			if err != nil {
				t.Fatalf("TakeSnapshot() returned error: %v", err)
			}
			r, err := openRepo(packRoot())
			if err != nil {
				t.Fatalf("openRepo() returned error: %v", err)
			}
			fromGitPack, err := r.packAt("HEAD", "index.toml", "pack.toml")
			if err != nil {
				t.Fatalf("packAt() returned error: %v", err)
			}
			fromGit := fromGitPack.Snapshot
			if !reflect.DeepEqual(fromGit, onDisk) {
				t.Errorf("snapshot from git =\n%+v\nsnapshot from disk =\n%+v", fromGit, onDisk)
			}
			if len(fromGit.Mods) != 2 || len(fromGit.Files) != 1 {
				t.Errorf("snapshot = %+v, want 2 mods and 1 file", fromGit)
			}

			// ...and HEAD keeps describing what was committed, whatever has happened to the files since
			p.mod(t, "Sodium", core.ClientSide, "0.6.0")
			p.remove(t, "config/nested/sodium.json")
			againPack, err := r.packAt("HEAD", "index.toml", "pack.toml")
			if err != nil {
				t.Fatalf("packAt() returned error: %v", err)
			}
			again := againPack.Snapshot
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

	snapPack, err := r.packAt("HEAD", "index.toml", "pack.toml")
	if err != nil {
		t.Fatalf("packAt() returned error: %v", err)
	}
	snap := snapPack.Snapshot
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

	snapPack, err := r.packAt("HEAD", "index.toml", "pack.toml")
	if err != nil {
		t.Fatalf("packAt() returned error: %v", err)
	}
	snap := snapPack.Snapshot
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
		t.Fatalf("openRepo() error = %v, want one telling the user to run git init", err)
	}
	// A changelog goes on without a repository, but not because of anything else that went wrong with git
	if !errors.Is(err, changelog.ErrNoRepository) {
		t.Errorf("openRepo() error = %v, want it to be changelog.ErrNoRepository", err)
	}
	// It says where, rather than "."
	if where, _, _ := strings.Cut(err.Error(), " isn't inside"); !filepath.IsAbs(where) {
		t.Errorf("openRepo() error = %q, want it to start with the directory it looked in, in full", err)
	}
}

func TestOpenRepoWithoutGitInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := openRepo(".")

	if err == nil || !strings.Contains(err.Error(), "git isn't installed") {
		t.Fatalf("openRepo() error = %v, want one saying git isn't installed", err)
	}
	// There is no git log to read without it, which is all a changelog needs to know
	if !errors.Is(err, changelog.ErrNoRepository) {
		t.Errorf("openRepo() error = %v, want it to be changelog.ErrNoRepository", err)
	}
}

func TestOpenRepoOfARepositoryGitWontUseIsNotAMissingOne(t *testing.T) {
	// Git exits the same way when it refuses a repository, because of who owns it say, as when there isn't one. Taking
	// the first for the second would make a changelog without the log the pack has, and tell the user to git init.
	setUpRepo(t)
	git(t, "config", "core.repositoryformatversion", "1")
	git(t, "config", "extensions.bogus", "true")
	if err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Run(); err == nil {
		t.Skip("this git uses a repository with an extension it doesn't know")
	}
	setUpPack(t, "", "1.0.0")

	_, err := openRepo(".")

	if err == nil {
		t.Fatal("openRepo() of a repository git won't use returned no error")
	}
	if errors.Is(err, changelog.ErrNoRepository) || strings.Contains(err.Error(), "git init") {
		t.Errorf("openRepo() error = %v, want git's own reason rather than there being no repository", err)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("openRepo() error = %v, want it to say what git said", err)
	}

	// So nothing is released, rather than released the way a pack outside a repository is
	cmdtest.CaptureStdout(t, func() { _, _, err = changelog.RunRelease("", "") })
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("RunRelease() error = %v, want the reason git gave", err)
	}
	if _, statErr := os.Stat(changelog.HistoryFile); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("a release was made: %v", statErr)
	}
}

// setUpPackOutsideARepo makes a pack in a fresh temp directory that isn't in a git repository, though git is installed.
func setUpPackOutsideARepo(t *testing.T) testPack {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't installed")
	}
	dir := cmdtest.Chdir(t)
	// Never treat a repository around the temp directory as this one
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "gitconfig"))
	return setUpPack(t, "", "1.0.0")
}

func TestGitCommandsRefuseToRunOutsideARepositoryBeforeLoadingAnything(t *testing.T) {
	// Loading the pack looks up the versions of mods that don't record one, which needs the network and can fail, so
	// that must not be what a pack that isn't in a repository is told
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7"})
	src.Err = errors.New("network is down")
	p := setUpPackOutsideARepo(t)
	p.unversioned(t, src, "Sodium", core.ClientSide, "id-a")
	files := []string{"pack.toml", "index.toml", "mods/sodium.pw.toml"}
	before := make(map[string]string)
	for _, f := range files {
		before[f] = readFile(t, f)
	}

	commands := map[string]func() error{
		"commit":           func() error { return runCommit(false) },
		"commit --dry-run": func() error { return runCommit(true) },
		"release":          func() error { return runRelease("", "") },
	}
	for name, run := range commands {
		t.Run(name, func(t *testing.T) {
			src.Calls = 0
			var err error
			out := cmdtest.CaptureStdout(t, func() { err = run() })

			if err == nil || !strings.Contains(err.Error(), "isn't inside a git repository") || !strings.Contains(err.Error(), "git init") {
				t.Errorf("error = %v, want one saying the pack isn't inside a git repository", err)
			}
			if out != "" {
				t.Errorf("printed %q before refusing", out)
			}
			if src.Calls != 0 {
				t.Errorf("%d versions were looked up before it refused", src.Calls)
			}
			for _, f := range files {
				if got := readFile(t, f); got != before[f] {
					t.Errorf("%s changed:\n--- before\n%s\n--- after\n%s", f, before[f], got)
				}
			}
			if _, err := os.Stat(".git"); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a repository was made where there was none: %v", err)
			}
		})
	}
}

func TestChangelogIsMadeOutsideARepositoryWithoutTheGitLog(t *testing.T) {
	p := setUpPackOutsideARepo(t)
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	p.write(t, "config/sodium.json", "{}")

	var err error
	out := cmdtest.CaptureStdout(t, func() { _, _, err = changelog.RunRelease("", "") })
	if err != nil {
		t.Fatalf("RunRelease() returned error: %v\noutput: %s", err, out)
	}
	for _, want := range []string{"Not reading the git log", "isn't inside a git repository", "Released 1.0.0!"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	// It only reads the pack: nothing was committed, or made to commit into
	if _, err := os.Stat(".git"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a repository was made where there was none: %v", err)
	}
	history, err := changelog.LoadHistory(changelog.HistoryFile)
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	if latest, _ := history.Latest(); latest.Version != "1.0.0" || latest.Commit != "" || history.Snapshot == nil {
		t.Errorf("history = %+v, want 1.0.0 made at no commit, with a snapshot of the pack", history)
	}

	// And the release after it is what changed in the pack since
	p.mod(t, "Sodium", core.ClientSide, "0.5.8")
	out = cmdtest.CaptureStdout(t, func() { _, _, err = changelog.RunRelease("", "") })
	if err != nil {
		t.Fatalf("second RunRelease() returned error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "next version is 1.0.1") || !strings.Contains(out, "**Sodium** 0.5.7 → 0.5.8 (client)") {
		t.Errorf("second release output should say what changed in Sodium:\n%s", out)
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

	// Nothing has been committed, in a repository that has no commits at all: releasing does that first
	out := release(t, "")

	if !strings.Contains(out, "Committed and tagged v1.0.0") {
		t.Errorf("output = %q, want it to report the tag", out)
	}
	if got := lastMessages(t, 2); !reflect.DeepEqual(got, []string{"chore(pack): initial commit", "chore(release): 1.0.0"}) {
		t.Errorf("commit messages = %q, want the initial commit and then the release", got)
	}
	// An annotated tag, on the release commit
	if kind := git(t, "cat-file", "-t", "v1.0.0"); kind != "tag" {
		t.Errorf("v1.0.0 is a %q object, want an annotated tag", kind)
	}
	if tagged, head := git(t, "rev-parse", "v1.0.0^{commit}"), git(t, "rev-parse", "HEAD"); tagged != head {
		t.Errorf("v1.0.0 points at %s, want the release commit %s", tagged, head)
	}
	// The first release keeps the version pack.toml already had, so that isn't part of it
	if changed := commitFiles(t, "HEAD"); !reflect.DeepEqual(changed, []string{"CHANGELOG.md", "changelog.toml"}) {
		t.Errorf("release commit changed %v, want the changelog and the history", changed)
	}
	requireClean(t)

	// A later release commits what hasn't been committed, then releases what the commits since the last one say
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")
	release(t, "")
	if got := lastMessages(t, 2); !reflect.DeepEqual(got, []string{"feat(mods)!: add Lithium 0.12.0 (server)\n\n" + wantBreakingFooter, "chore(release): 2.0.0"}) {
		t.Errorf("commit messages = %q, want the mod committed and then a release that is major for a server mod", got)
	}
	if tags := git(t, "tag", "--list"); tags != "v1.0.0\nv2.0.0" {
		t.Errorf("tags = %q, want v1.0.0 and v2.0.0", tags)
	}
	// This one raised the version, so pack.toml is part of the release
	if changed := commitFiles(t, "HEAD"); !reflect.DeepEqual(changed, []string{"CHANGELOG.md", "changelog.toml", "pack.toml"}) {
		t.Errorf("release commit changed %v, want the changelog, the history and pack.toml with the new version", changed)
	}
	requireClean(t)
}

func TestReleaseCommitsEachModAndThenReleasesWhatTheLogSays(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	p.mod(t, "Iris", core.ClientSide, "1.0")
	p.write(t, "config/sodium.json", "{}")
	release(t, "")

	// Everything that has changed since, none of it committed
	p.mod(t, "Sodium", core.ClientSide, "0.5.8")   // updated
	p.remove(t, "mods/iris.pw.toml")               // removed
	p.mod(t, "Lithium", core.ServerSide, "0.12.0") // added
	p.write(t, "config/sodium.json", `{"a": 1}`)   // changed
	release(t, "")

	// One command, and a commit for each of them, then the release
	want := []string{
		"feat(mods): remove Iris 1.0 (client)",
		"feat(mods)!: add Lithium 0.12.0 (server)\n\n" + wantBreakingFooter,
		"fix(mods): update Sodium 0.5.7 -> 0.5.8 (client)",
		"fix(config): change config/sodium.json",
		"chore(release): 2.0.0",
	}
	if got := lastMessages(t, 5); !reflect.DeepEqual(got, want) {
		t.Errorf("commit messages =\n%q\nwant\n%q", got, want)
	}

	changelogMD := git(t, "show", "HEAD:CHANGELOG.md")
	for _, want := range []string{
		"## 2.0.0 - ",
		"Server update required",
		"### Added\n\n- **Lithium** 0.12.0 (server)",
		"### Updated\n\n- **Sodium** 0.5.7 → 0.5.8 (client)",
		"### Removed\n\n- **Iris** 1.0 (client)",
		"### Config\n\n- Changed `config/sodium.json`",
	} {
		if !strings.Contains(changelogMD, want) {
			t.Errorf("the committed changelog is missing %q:\n%s", want, changelogMD)
		}
	}

	// The release is recorded at the commit before the release commit: the last of the commits it was made from
	history, err := changelog.LoadHistory("changelog.toml")
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	latest, _ := history.Latest()
	if before := git(t, "rev-parse", "HEAD~1"); latest.Version != "2.0.0" || latest.Commit != before {
		t.Errorf("latest release = %s at %s, want 2.0.0 at %s", latest.Version, latest.Commit, before)
	}
	requireClean(t)
}

func TestReleaseIncludesCommitsWrittenByHand(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	release(t, "")

	// Ordinary git commits with conventional messages, not made by packwiz
	p.write(t, "config/general.json", `{"particles": 1}`)
	git(t, "add", "-A")
	git(t, "commit", "-q", "-m", "fix(config): lower the particle count")
	git(t, "commit", "-q", "--allow-empty", "-m", "feat!: update to Minecraft 1.21.4")
	git(t, "commit", "-q", "--allow-empty", "-m", "docs: explain the settings")
	git(t, "commit", "-q", "--allow-empty", "-m", "Fix a typo")

	release(t, "")

	changelogMD := git(t, "show", "HEAD:CHANGELOG.md")
	for _, want := range []string{"- lower the particle count", "- **Breaking:** update to Minecraft 1.21.4"} {
		if !strings.Contains(changelogMD, want) {
			t.Errorf("the changelog is missing %q:\n%s", want, changelogMD)
		}
	}
	for _, unwanted := range []string{"explain the settings", "typo"} {
		if strings.Contains(changelogMD, unwanted) {
			t.Errorf("the changelog has %q, which isn't a change to the pack:\n%s", unwanted, changelogMD)
		}
	}
	if got := headMessage(t); got != "chore(release): 2.0.0" {
		t.Errorf("release commit message = %q, want 2.0.0 for a breaking commit", got)
	}
}

func TestReleaseFindsWhereTheLastOneWasMadeWhenItWasNotRecorded(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	release(t, "")

	// A history from before the commit was recorded: the same, without it
	history, err := changelog.LoadHistory("changelog.toml")
	if err != nil {
		t.Fatalf("LoadHistory() returned error: %v", err)
	}
	history.Releases[0].Commit = ""
	if err := history.Write("changelog.toml"); err != nil {
		t.Fatalf("failed to write history: %v", err)
	}
	git(t, "commit", "-q", "-a", "-m", "chore: history from before commits were recorded")
	p.mod(t, "Iris", core.ClientSide, "1.0")

	out := release(t, "")

	if got := headMessage(t); got != "chore(release): 1.1.0" {
		t.Errorf("release commit message = %q, want 1.1.0 for the mod added since; output:\n%s", got, out)
	}
	if changelogMD := git(t, "show", "HEAD:CHANGELOG.md"); strings.Count(changelogMD, "**Sodium**") != 1 {
		t.Errorf("the changelog lists Sodium more than once, so the first release was read again:\n%s", changelogMD)
	}
}

func TestReleaseSinceReadsFromTheCommitGiven(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	release(t, "")
	p.mod(t, "Iris", core.ClientSide, "1.0")
	release(t, "")
	p.mod(t, "Zoom", core.ClientSide, "2.0")

	var err error
	cmdtest.CaptureStdout(t, func() { err = runRelease("", "v1.0.0") })
	if err != nil {
		t.Fatalf("runRelease() returned error: %v", err)
	}

	// Everything since the first release, so Iris is listed again as well as Zoom
	changelogMD := git(t, "show", "HEAD:CHANGELOG.md")
	newest := changelogMD[strings.Index(changelogMD, "## 1.2.0"):strings.Index(changelogMD, "## 1.1.0")]
	if !strings.Contains(newest, "**Iris** 1.0 (client)") || !strings.Contains(newest, "**Zoom** 2.0 (client)") {
		t.Errorf("the newest release should list what was added since v1.0.0:\n%s", newest)
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
	cmdtest.CaptureStdout(t, func() { err = runRelease("", "") })

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

func TestCommitSavesLookedUpVersions(t *testing.T) {
	setUpRepo(t)
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-a": "0.5.7"})
	p := setUpPack(t, "", "1.0.0")
	p.unversioned(t, src, "Sodium", core.ClientSide, "id-a")

	commit(t)

	if committed := git(t, "show", "HEAD:mods/sodium.pw.toml"); !strings.Contains(committed, `version = "0.5.7"`) {
		t.Errorf("the committed mod doesn't record its version:\n%s", committed)
	}
	// The committed index matches the committed mod, or a client checking the file against it would reject it
	requireClean(t)
	before := git(t, "show", "HEAD:index.toml")
	commit(t)
	if after := git(t, "show", "HEAD:index.toml"); after != before {
		t.Errorf("committing again changed the index; it wasn't up to date with the mod:\n%s", after)
	}
}

func TestCommitDescribesModsByVersionNotFileName(t *testing.T) {
	setUpRepo(t)
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-b": "0.12.0"})
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)

	p.unversioned(t, src, "Lithium", core.ServerSide, "id-b")
	commit(t)

	if got, want := headMessage(t), "feat(mods)!: add Lithium 0.12.0 (server)\n\n"+wantBreakingFooter; got != want {
		t.Errorf("commit message =\n%s\nwant\n%s", got, want)
	}
}

func TestCommitDoesNotTakeRecordingAVersionForAnUpdate(t *testing.T) {
	setUpRepo(t)
	// The first commit can't find the version, so the mod goes in with only its file name to go by
	src := cmdtest.RegisterVersionSource(t, "testsource", nil)
	p := setUpPack(t, "", "1.0.0")
	p.unversioned(t, src, "Lithium", core.ServerSide, "id-b")
	commit(t)
	committed, err := core.DecodeMod([]byte(git(t, "show", "HEAD:mods/lithium.pw.toml")))
	if err != nil || committed.Version != "" {
		t.Fatalf("the fixture should start with a mod that records no version, got %q (%v)", committed.Version, err)
	}
	before := commitCount(t)

	// Now it can. The mod hasn't changed, only what is known about it, so this isn't the "feat!" that a server mod
	// being updated would be.
	src.Versions = map[string]string{"id-b": "0.12.0"}
	commit(t)

	if got := headMessage(t); got != "chore(pack): update pack files" {
		t.Errorf("commit message = %q, want a chore; nothing about the pack changed", got)
	}
	if got := commitCount(t); got != before+1 {
		t.Errorf("commit count = %d, want %d", got, before+1)
	}
	if committed := git(t, "show", "HEAD:mods/lithium.pw.toml"); !strings.Contains(committed, `version = "0.12.0"`) {
		t.Errorf("the version wasn't committed:\n%s", committed)
	}
}

func TestCommitDryRunLooksUpVersionsButSavesNothing(t *testing.T) {
	setUpRepo(t)
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-b": "0.12.0"})
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	p.unversioned(t, src, "Lithium", core.ServerSide, "id-b")
	modBefore, indexBefore := readFile(t, "mods/lithium.pw.toml"), readFile(t, "index.toml")

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = runCommit(true) })
	if err != nil {
		t.Fatalf("runCommit(dryRun) returned error: %v", err)
	}

	if want := "feat(mods)!: add Lithium 0.12.0 (server)"; !strings.Contains(out, want) {
		t.Errorf("dry run output = %q, want the looked-up version in %q", out, want)
	}
	if readFile(t, "mods/lithium.pw.toml") != modBefore || readFile(t, "index.toml") != indexBefore {
		t.Error("a dry run modified the pack; it must only look versions up")
	}
}

func TestCommitFailsWhenVersionsCannotBeLookedUp(t *testing.T) {
	setUpRepo(t)
	src := cmdtest.RegisterVersionSource(t, "testsource", nil)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	p.unversioned(t, src, "Lithium", core.ServerSide, "id-b")
	src.Err = errors.New("network is down")
	before := commitCount(t)
	modBefore := readFile(t, "mods/lithium.pw.toml")

	t.Run("commit", func(t *testing.T) {
		var err error
		cmdtest.CaptureStdout(t, func() { err = runCommit(false) })
		// Committing a file name where a version belongs would leave it in the history
		if err == nil || !strings.Contains(err.Error(), "network is down") {
			t.Errorf("runCommit() error = %v, want one saying the versions couldn't be looked up", err)
		}
		if got := commitCount(t); got != before {
			t.Errorf("commit count = %d, want %d", got, before)
		}
		if readFile(t, "mods/lithium.pw.toml") != modBefore {
			t.Error("the mod was modified although the commit failed")
		}
	})

	t.Run("dry run carries on", func(t *testing.T) {
		var err error
		out := cmdtest.CaptureStdout(t, func() { err = runCommit(true) })
		if err != nil {
			t.Fatalf("runCommit(dryRun) returned error: %v", err)
		}
		for _, want := range []string{"Warning: couldn't look up the versions", "add Lithium lithium-id-b.jar (server)"} {
			if !strings.Contains(out, want) {
				t.Errorf("dry run output missing %q:\n%s", want, out)
			}
		}
	})
}

func TestReleaseCommitsTheVersionsItSavesBeforeReleasing(t *testing.T) {
	setUpRepo(t)
	src := cmdtest.RegisterVersionSource(t, "testsource", nil)
	p := setUpPack(t, "", "1.0.0")
	p.unversioned(t, src, "Sodium", core.ClientSide, "id-a")
	commit(t) // committed while its version couldn't be found
	src.Versions = map[string]string{"id-a": "0.5.7"}

	release(t, "")

	// Committing what is pending is what records the version, and that is a commit of its own before the release's
	if got := lastMessages(t, 2); !reflect.DeepEqual(got, []string{"chore(pack): update pack files", "chore(release): 1.0.0"}) {
		t.Errorf("commit messages = %q", got)
	}
	revs := lastCommits(t, 2)
	if changed := commitFiles(t, revs[0]); !slices.Contains(changed, "mods/sodium.pw.toml") {
		t.Errorf("the commit before the release changed %v, want the mod that had its version recorded", changed)
	}
	if changelogMD := git(t, "show", "HEAD:CHANGELOG.md"); !strings.Contains(changelogMD, "**Sodium** 0.5.7 (client)") {
		t.Errorf("the committed changelog doesn't show the version:\n%s", changelogMD)
	}
	requireClean(t)
}

func TestCommitDryRunSaysWhenOnlyVersionsWouldBeCommitted(t *testing.T) {
	setUpRepo(t)
	src := cmdtest.RegisterVersionSource(t, "testsource", nil)
	p := setUpPack(t, "", "1.0.0")
	p.unversioned(t, src, "Sodium", core.ClientSide, "id-a")
	commit(t) // committed while its version couldn't be found
	src.Versions = map[string]string{"id-a": "0.5.7"}
	before := commitCount(t)

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = runCommit(true) })
	if err != nil {
		t.Fatalf("runCommit(dryRun) returned error: %v", err)
	}

	// The tree is clean and nothing has changed, but a real commit would save the version, so a dry run mustn't
	// claim there is nothing to do
	if strings.Contains(out, "Nothing to commit") || !strings.Contains(out, "chore(pack): update pack files") {
		t.Errorf("dry run output = %q, want the message the real commit would use", out)
	}
	if got := commitCount(t); got != before {
		t.Errorf("commit count = %d, want %d; a dry run mustn't commit", got, before)
	}

	commit(t)
	if got := commitCount(t); got != before+1 {
		t.Errorf("the real commit made %d new commits, want the 1 the dry run described", got-before)
	}
}

func TestReleaseWithNothingNewToldToCommitWhatItFixed(t *testing.T) {
	setUpRepo(t)
	src := cmdtest.RegisterVersionSource(t, "testsource", nil)
	p := setUpPack(t, "", "1.0.0")
	p.unversioned(t, src, "Sodium", core.ClientSide, "id-a")
	commit(t)
	release(t, "") // the first release, while the version couldn't be found: it lists the file name
	src.Versions = map[string]string{"id-a": "0.5.7"}
	before := commitCount(t)

	out := release(t, "")

	// The version was recorded, in a commit that doesn't make a release, and the old changelog lines were fixed
	if !strings.Contains(out, "No changes since the last release (1.0.0).") || !strings.Contains(out, "Updated 1 line in the changelog.") {
		t.Errorf("output = %q, want it to say nothing was released and what was fixed", out)
	}
	if !strings.Contains(out, `packwiz git commit`) {
		t.Errorf("output = %q, want it to say how to commit what changed", out)
	}
	if got := commitCount(t); got != before+1 {
		t.Errorf("commit count = %d, want %d: the one that recorded the version", got, before+1)
	}
	if git(t, "status", "--porcelain") == "" {
		t.Error("the tree is clean, but the changelog was rewritten")
	}
	if got := readFile(t, "CHANGELOG.md"); !strings.Contains(got, "**Sodium** 0.5.7 (client)") || strings.Contains(got, "sodium-id-a.jar") {
		t.Errorf("CHANGELOG.md still shows the file name:\n%s", got)
	}
}

func TestReleaseWithNothingToDoDoesNotMentionCommitting(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	release(t, "")

	out := release(t, "")

	if strings.Contains(out, "packwiz git commit") {
		t.Errorf("output = %q; there was nothing to commit, so it shouldn't say to", out)
	}
}

// gitBytes is git for output that must be kept exactly, such as a file's contents.
func gitBytes(t *testing.T, args ...string) []byte {
	t.Helper()
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		t.Fatalf("git %s failed: %v", strings.Join(args, " "), err)
	}
	return out
}

// lastMessages returns the messages of the last n commits, oldest first.
func lastMessages(t *testing.T, n int) []string {
	t.Helper()
	out := git(t, "log", "-n", strconv.Itoa(n), "--reverse", "--format=%B%x1e")
	var messages []string
	for _, m := range strings.Split(out, "\x1e") {
		if m = strings.TrimSpace(m); m != "" {
			messages = append(messages, m)
		}
	}
	return messages
}

// lastCommits returns the last n commits, oldest first.
func lastCommits(t *testing.T, n int) []string {
	t.Helper()
	return strings.Fields(git(t, "log", "-n", strconv.Itoa(n), "--reverse", "--format=%H"))
}

// commitFiles lists the files a commit changed, sorted.
func commitFiles(t *testing.T, rev string) []string {
	t.Helper()
	files := strings.Fields(git(t, "show", "--name-only", "--format=", rev))
	slices.Sort(files)
	return files
}

// withoutProgress removes the progress bar that refreshing the index draws, from output that is compared exactly.
func withoutProgress(out string) string {
	var kept []string
	for _, line := range strings.SplitAfter(out, "\n") {
		if !strings.Contains(line, "Refreshing index") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "")
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// requireConsistentPack fails unless the pack in a commit can be used by itself: its index lists the metadata files in
// the commit, and nothing else in the pack, with the hashes they have there, and pack.toml has the hash of that index.
// root is where the pack is in the repository.
func requireConsistentPack(t *testing.T, root, rev string) {
	t.Helper()
	at := func(rel string) string { return path.Join(root, rel) }

	indexText := gitBytes(t, "show", rev+":"+at("index.toml"))
	var index struct {
		Files []struct {
			File     string `toml:"file"`
			Hash     string `toml:"hash"`
			MetaFile bool   `toml:"metafile"`
		} `toml:"files"`
	}
	if _, err := toml.Decode(string(indexText), &index); err != nil {
		t.Fatalf("index.toml at %s isn't valid: %v", rev[:8], err)
	}

	listed := make(map[string]bool)
	for _, f := range index.Files {
		listed[f.File] = true
		// git show fails, failing the test, for a file the index lists that isn't in the commit
		if got := sha256Hex(gitBytes(t, "show", rev+":"+at(f.File))); f.Hash != got {
			t.Errorf("at %s the index has %s as %s, but the file hashes to %s", rev[:8], f.File, f.Hash, got)
		}
	}
	dir := root
	if dir == "" {
		dir = "." // git wants a real pathspec, and this is all of the repository
	}
	for _, name := range strings.Fields(git(t, "ls-tree", "-r", "--name-only", rev, "--", dir)) {
		rel := strings.TrimPrefix(strings.TrimPrefix(name, root), "/")
		if strings.HasSuffix(rel, core.MetaExtension) && !listed[rel] {
			t.Errorf("at %s the index doesn't list %s, which is in the commit", rev[:8], rel)
		}
	}

	var pack struct {
		Index struct {
			Hash string `toml:"hash"`
		} `toml:"index"`
	}
	if _, err := toml.Decode(string(gitBytes(t, "show", rev+":"+at("pack.toml"))), &pack); err != nil {
		t.Fatalf("pack.toml at %s isn't valid: %v", rev[:8], err)
	}
	if want := sha256Hex(indexText); pack.Index.Hash != want {
		t.Errorf("at %s pack.toml has the index as %s, but it hashes to %s", rev[:8], pack.Index.Hash, want)
	}
}

// rejectCommitsMentioning makes git refuse any commit whose message contains word, as a commit-msg hook can.
func rejectCommitsMentioning(t *testing.T, word string) (remove func()) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("hooks are shell scripts")
	}
	hook := filepath.Join(".git", "hooks", "commit-msg")
	script := "#!/bin/sh\nif grep -q " + word + " \"$1\"; then echo 'rejected by test hook' >&2; exit 1; fi\n"
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write hook: %v", err)
	}
	return func() {
		if err := os.Remove(hook); err != nil {
			t.Fatalf("failed to remove hook: %v", err)
		}
	}
}

func TestCommitMakesOneCommitPerMod(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	p.mod(t, "Iris", core.ClientSide, "1.0")
	commit(t)
	before := commitCount(t)

	p.mod(t, "Sodium", core.ClientSide, "0.5.8")    // updated
	p.remove(t, "mods/iris.pw.toml")                // removed
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")  // added
	p.mod(t, "Fabric", core.UniversalSide, "0.100") // added
	out := commit(t)

	// Each mod on its own, in path order, each with the type its change deserves
	want := []string{
		"feat(mods)!: add Fabric 0.100 (both)\n\n" + wantBreakingFooter,
		"feat(mods): remove Iris 1.0 (client)",
		"feat(mods)!: add Lithium 0.12.0 (server)\n\n" + wantBreakingFooter,
		"fix(mods): update Sodium 0.5.7 -> 0.5.8 (client)",
	}
	if got := commitCount(t); got != before+4 {
		t.Fatalf("commit count = %d, want %d: one for each mod", got, before+4)
	}
	if got := lastMessages(t, 4); !reflect.DeepEqual(got, want) {
		t.Errorf("commit messages =\n%q\nwant\n%q", got, want)
	}
	for _, m := range want {
		if line := "Committed: " + strings.SplitN(m, "\n", 2)[0]; !strings.Contains(out, line) {
			t.Errorf("output = %q, want it to report %q", out, line)
		}
	}

	// Each commit is that mod's file, and the index and pack.toml that go with it, and nothing else
	files := [][]string{
		{"index.toml", "mods/fabric.pw.toml", "pack.toml"},
		{"index.toml", "mods/iris.pw.toml", "pack.toml"},
		{"index.toml", "mods/lithium.pw.toml", "pack.toml"},
		{"index.toml", "mods/sodium.pw.toml", "pack.toml"},
	}
	for i, rev := range lastCommits(t, 4) {
		if got := commitFiles(t, rev); !reflect.DeepEqual(got, files[i]) {
			t.Errorf("commit %d changed %v, want %v", i+1, got, files[i])
		}
		requireConsistentPack(t, "", rev)
	}
	requireClean(t)
}

func TestModCommitsComeBeforeTheCommitForEverythingElse(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	p.write(t, "config/sodium.json", "{}")
	commit(t)
	before := commitCount(t)

	p.write(t, "config/sodium.json", `{"a": 1}`) // a config change...
	p.write(t, "config/new.json", "{}")          // ...another...
	p.mod(t, "Zoom", core.ClientSide, "2.0")     // ...and a mod
	p.modFile(t, core.Mod{                       // ...and pinning one, which is neither
		Name: "Sodium", FileName: "sodium-0.5.7.jar", Version: "0.5.7", Side: core.ClientSide, Pin: true,
		Download: core.ModDownload{HashFormat: "sha256", Hash: "hash-0.5.7"},
	})
	commit(t)

	want := []string{
		"feat(mods): add Zoom 2.0 (client)",
		// The mod first, then everything that isn't a mod change together, however small
		"fix(config): update 2 config files\n\n- add config/new.json\n- change config/sodium.json",
	}
	if got := commitCount(t); got != before+2 {
		t.Fatalf("commit count = %d, want %d", got, before+2)
	}
	if got := lastMessages(t, 2); !reflect.DeepEqual(got, want) {
		t.Errorf("commit messages =\n%q\nwant\n%q", got, want)
	}
	mods, rest := lastCommits(t, 2)[0], lastCommits(t, 2)[1]
	if got := commitFiles(t, mods); !reflect.DeepEqual(got, []string{"index.toml", "mods/zoom.pw.toml", "pack.toml"}) {
		t.Errorf("the mod's commit changed %v; the config and the pin are not part of it", got)
	}
	// The pin, a change to a mod that isn't an add, update or removal, goes in the last commit
	if got := commitFiles(t, rest); !slices.Contains(got, "mods/sodium.pw.toml") || !slices.Contains(got, "config/new.json") {
		t.Errorf("the last commit changed %v, want the config files and the pinned mod", got)
	}
	requireConsistentPack(t, "", mods)
	requireConsistentPack(t, "", rest)
	requireClean(t)
}

func TestPinningAModAlongsideAModChangeIsItsOwnChoreCommit(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	before := commitCount(t)

	p.mod(t, "Iris", core.ClientSide, "1.0")
	p.modFile(t, core.Mod{
		Name: "Sodium", FileName: "sodium-0.5.7.jar", Version: "0.5.7", Side: core.ClientSide, Pin: true,
		Download: core.ModDownload{HashFormat: "sha256", Hash: "hash-0.5.7"},
	})
	commit(t)

	want := []string{"feat(mods): add Iris 1.0 (client)", "chore(pack): update pack files"}
	if got := commitCount(t); got != before+2 {
		t.Fatalf("commit count = %d, want %d", got, before+2)
	}
	if got := lastMessages(t, 2); !reflect.DeepEqual(got, want) {
		t.Errorf("commit messages = %q, want %q", got, want)
	}
	requireClean(t)
}

func TestPackTomlEditsAreNotBlamedOnAMod(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)

	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	pack.Description = "A description that isn't about any one mod"
	if err := pack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	p.mod(t, "Iris", core.ClientSide, "1.0")
	commit(t)

	want := []string{"feat(mods): add Iris 1.0 (client)", "chore(pack): update pack files"}
	if got := lastMessages(t, 2); !reflect.DeepEqual(got, want) {
		t.Fatalf("commit messages = %q, want %q", got, want)
	}
	revs := lastCommits(t, 2)
	if modsPack := git(t, "show", revs[0]+":pack.toml"); strings.Contains(modsPack, "A description") {
		t.Errorf("the mod's commit has the description in pack.toml:\n%s", modsPack)
	}
	if lastPack := git(t, "show", revs[1]+":pack.toml"); !strings.Contains(lastPack, "A description") {
		t.Errorf("the last commit doesn't have the description in pack.toml:\n%s", lastPack)
	}
	requireConsistentPack(t, "", revs[0])
	requireConsistentPack(t, "", revs[1])
}

func TestFirstCommitOfARepositoryIsASingleCommit(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	p.mod(t, "Iris", core.ClientSide, "1.0")
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")
	p.write(t, "config/sodium.json", "{}")

	commit(t)

	// There is nothing before it to be a change to, so the pack goes in as it is
	if got := commitCount(t); got != 1 {
		t.Errorf("commit count = %d, want the pack in one initial commit", got)
	}
	if got := headMessage(t); got != "chore(pack): initial commit" {
		t.Errorf("commit message = %q", got)
	}
	requireConsistentPack(t, "", "HEAD")
	requireClean(t)
}

func TestPackAddedToAnExistingRepositoryCommitsEachMod(t *testing.T) {
	setUpRepo(t)
	if err := os.WriteFile("notes.txt", []byte("not the pack"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	// A commit from before the pack, in a directory of its own so the pack doesn't pick the file up
	if err := os.MkdirAll("docs", 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.Rename("notes.txt", filepath.Join("docs", "notes.txt")); err != nil {
		t.Fatalf("failed to move file: %v", err)
	}
	git(t, "add", "docs")
	git(t, "commit", "-q", "-m", "before the pack")
	p := setUpPack(t, "pack", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	p.mod(t, "Iris", core.ClientSide, "1.0")

	commit(t)

	want := []string{"feat(mods): add Iris 1.0 (client)", "feat(mods): add Sodium 0.5.7 (client)"}
	if got := commitCount(t); got != 3 {
		t.Fatalf("commit count = %d, want the existing commit and one for each mod", got)
	}
	if got := lastMessages(t, 2); !reflect.DeepEqual(got, want) {
		t.Errorf("commit messages = %q, want %q", got, want)
	}
	revs := lastCommits(t, 2)
	// The pack file and the index come into being with the first of them
	if got := commitFiles(t, revs[0]); !reflect.DeepEqual(got, []string{"pack/index.toml", "pack/mods/iris.pw.toml", "pack/pack.toml"}) {
		t.Errorf("the first mod's commit changed %v", got)
	}
	for _, rev := range revs {
		requireConsistentPack(t, "pack", rev)
	}
	requireClean(t)
}

func TestPerModCommitsInAPackInASubdirectory(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "pack", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	// Staged files elsewhere in the repository aren't the pack's, and aren't swept into its commits
	if err := os.WriteFile("README.md", []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	git(t, "add", "README.md")
	before := commitCount(t)

	p.mod(t, "Iris", core.ClientSide, "1.0")
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")
	commit(t)

	if got := commitCount(t); got != before+2 {
		t.Fatalf("commit count = %d, want %d", got, before+2)
	}
	for i, rev := range lastCommits(t, 2) {
		mod := []string{"pack/mods/iris.pw.toml", "pack/mods/lithium.pw.toml"}[i]
		if got := commitFiles(t, rev); !reflect.DeepEqual(got, []string{"pack/index.toml", mod, "pack/pack.toml"}) {
			t.Errorf("commit %d changed %v", i+1, got)
		}
		requireConsistentPack(t, "pack", rev)
	}
	if staged := git(t, "diff", "--cached", "--name-only"); staged != "README.md" {
		t.Errorf("staged files = %q, want the README left for the user's own commit", staged)
	}
}

func TestVersionsRecordedForUnchangedModsAreOneCommitNotOnePerMod(t *testing.T) {
	setUpRepo(t)
	src := cmdtest.RegisterVersionSource(t, "testsource", nil)
	p := setUpPack(t, "", "1.0.0")
	for _, name := range []string{"Sodium", "Iris", "Zoom"} {
		p.unversioned(t, src, name, core.ClientSide, "id-"+strings.ToLower(name))
	}
	commit(t) // committed while none of their versions could be found
	src.Versions = map[string]string{"id-sodium": "0.5.7", "id-iris": "1.0", "id-zoom": "2.0"}
	before := commitCount(t)

	commit(t)

	// None of the mods changed, so recording what is known about them isn't a commit for each
	if got := commitCount(t); got != before+1 {
		t.Errorf("commit count = %d, want %d: one commit for all of them", got, before+1)
	}
	if got := headMessage(t); got != "chore(pack): update pack files" {
		t.Errorf("commit message = %q", got)
	}
	requireConsistentPack(t, "", "HEAD")
	requireClean(t)
}

func TestAModsLookedUpVersionIsInItsOwnCommit(t *testing.T) {
	setUpRepo(t)
	src := cmdtest.RegisterVersionSource(t, "testsource", map[string]string{"id-b": "0.12.0"})
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)

	p.unversioned(t, src, "Lithium", core.ServerSide, "id-b")
	p.mod(t, "Zoom", core.ClientSide, "2.0")
	commit(t)

	revs := lastCommits(t, 2)
	if got := lastMessages(t, 2)[0]; got != "feat(mods)!: add Lithium 0.12.0 (server)\n\n"+wantBreakingFooter {
		t.Errorf("first commit message = %q", got)
	}
	if committed := git(t, "show", revs[0]+":mods/lithium.pw.toml"); !strings.Contains(committed, `version = "0.12.0"`) {
		t.Errorf("the mod's own commit doesn't record its version:\n%s", committed)
	}
	for _, rev := range revs {
		requireConsistentPack(t, "", rev)
	}
}

func TestCommitDryRunListsEveryCommit(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	before := commitCount(t)

	p.mod(t, "Zoom", core.ClientSide, "2.0")
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")
	p.write(t, "config/new.json", "{}")

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = runCommit(true) })
	if err != nil {
		t.Fatalf("runCommit(dryRun) returned error: %v", err)
	}

	want := "feat(mods)!: add Lithium 0.12.0 (server)\n\n" + wantBreakingFooter + "\n---\n" +
		"feat(mods): add Zoom 2.0 (client)\n---\n" +
		"fix(config): add config/new.json\n"
	if out = withoutProgress(out); out != want {
		t.Errorf("dry run output =\n%q\nwant\n%q", out, want)
	}
	if got := commitCount(t); got != before {
		t.Errorf("commit count = %d, want %d; a dry run must not commit", got, before)
	}
}

func TestCommitDryRunForASingleCommitPrintsJustItsMessage(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	p.mod(t, "Zoom", core.ClientSide, "2.0")

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = runCommit(true) })
	if err != nil {
		t.Fatalf("runCommit(dryRun) returned error: %v", err)
	}
	if out = withoutProgress(out); out != "feat(mods): add Zoom 2.0 (client)\n" {
		t.Errorf("dry run output = %q, want only the message", out)
	}
}

func TestCommitDryRunPredictsTheCommitForAPin(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	p.mod(t, "Iris", core.ClientSide, "1.0")
	p.modFile(t, core.Mod{
		Name: "Sodium", FileName: "sodium-0.5.7.jar", Version: "0.5.7", Side: core.ClientSide, Pin: true,
		Download: core.ModDownload{HashFormat: "sha256", Hash: "hash-0.5.7"},
	})

	var err error
	out := cmdtest.CaptureStdout(t, func() { err = runCommit(true) })
	if err != nil {
		t.Fatalf("runCommit(dryRun) returned error: %v", err)
	}

	if want := "feat(mods): add Iris 1.0 (client)\n---\nchore(pack): update pack files\n"; withoutProgress(out) != want {
		t.Errorf("dry run output = %q, want %q", withoutProgress(out), want)
	}
}

func TestCommitCanBeRunAgainAfterOneOfTheCommitsFails(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	before := commitCount(t)

	p.mod(t, "Alpha", core.ClientSide, "1.0")
	p.mod(t, "Lithium", core.ServerSide, "0.12.0") // the second of three, and the one that will be refused
	p.mod(t, "Zoom", core.ClientSide, "2.0")
	removeHook := rejectCommitsMentioning(t, "Lithium")

	var err error
	cmdtest.CaptureStdout(t, func() { err = runCommit(false) })

	if err == nil {
		t.Fatal("runCommit() succeeded although a commit was refused")
	}
	for _, want := range []string{"committed 1 of 3 commits", "Lithium", "rejected by test hook", `run "packwiz git commit" again`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
	if got := commitCount(t); got != before+1 {
		t.Errorf("commit count = %d, want %d: only the mod before the refused one", got, before+1)
	}
	// The files are left as they should end up, not as they were for the commit that failed...
	index := readFile(t, "index.toml")
	for _, mod := range []string{"alpha", "lithium", "zoom"} {
		if !strings.Contains(index, "mods/"+mod+".pw.toml") {
			t.Errorf("index.toml doesn't list %s after the failure; it was left as it was for one commit:\n%s", mod, index)
		}
	}
	// ...and nothing is left staged for someone else's commit to pick up
	if staged := git(t, "diff", "--cached", "--name-only"); staged != "" {
		t.Errorf("files were left staged: %q", staged)
	}

	// Once the problem is gone, running it again picks up where it stopped
	removeHook()
	commit(t)

	want := []string{
		"feat(mods): add Alpha 1.0 (client)",
		"feat(mods)!: add Lithium 0.12.0 (server)\n\n" + wantBreakingFooter,
		"feat(mods): add Zoom 2.0 (client)",
	}
	if got := commitCount(t); got != before+3 {
		t.Errorf("commit count = %d, want %d", got, before+3)
	}
	if got := lastMessages(t, 3); !reflect.DeepEqual(got, want) {
		t.Errorf("commit messages =\n%q\nwant\n%q", got, want)
	}
	for _, rev := range lastCommits(t, 3) {
		requireConsistentPack(t, "", rev)
	}
	requireClean(t)
}

func TestCommitForEverythingElseFailingLeavesTheModCommitsMade(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	before := commitCount(t)
	p.mod(t, "Iris", core.ClientSide, "1.0")
	p.write(t, "config/new.json", "{}")
	removeHook := rejectCommitsMentioning(t, "config")

	var err error
	cmdtest.CaptureStdout(t, func() { err = runCommit(false) })

	if err == nil || !strings.Contains(err.Error(), "committed 1 of 2 commits") {
		t.Errorf("error = %v, want one saying the mod was committed and the rest wasn't", err)
	}
	if got := commitCount(t); got != before+1 {
		t.Errorf("commit count = %d, want %d", got, before+1)
	}
	removeHook()
	commit(t)
	if got := headMessage(t); got != "fix(config): add config/new.json" {
		t.Errorf("commit message = %q, want the config committed when run again", got)
	}
	requireClean(t)
}

func TestPlanCommits(t *testing.T) {
	add := func(name string) changelog.Change {
		return changelog.Change{Kind: changelog.ModAdded, Path: "mods/" + name + ".pw.toml", Name: name, Side: core.ClientSide, To: "1"}
	}
	config := changelog.Change{Kind: changelog.FileChanged, Path: "config/a.json"}
	messages := func(steps []commitStep) []string {
		var out []string
		for _, s := range steps {
			out = append(out, s.message)
		}
		return out
	}

	t.Run("one commit for each mod, in the order given", func(t *testing.T) {
		steps := planCommits([]changelog.Change{add("a"), add("b"), add("c")}, false)
		if want := []string{"feat(mods): add a 1 (client)", "feat(mods): add b 1 (client)", "feat(mods): add c 1 (client)"}; !reflect.DeepEqual(messages(steps), want) {
			t.Errorf("messages = %q, want %q", messages(steps), want)
		}
		for i, s := range steps {
			if s.change == nil || s.change.Name != []string{"a", "b", "c"}[i] {
				t.Errorf("step %d is for %+v, want its own mod's change (not the loop's last)", i, s.change)
			}
		}
	})
	t.Run("other files come last, together", func(t *testing.T) {
		steps := planCommits([]changelog.Change{config, add("a")}, false)
		if want := []string{"feat(mods): add a 1 (client)", "fix(config): change config/a.json"}; !reflect.DeepEqual(messages(steps), want) {
			t.Errorf("messages = %q, want %q", messages(steps), want)
		}
		if steps[1].change != nil {
			t.Error("the last step is for a mod, but it takes everything else")
		}
	})
	t.Run("nothing to commit", func(t *testing.T) {
		if steps := planCommits(nil, false); len(steps) != 0 {
			t.Errorf("steps = %+v, want none", steps)
		}
	})
	t.Run("something else changed that isn't a change", func(t *testing.T) {
		steps := planCommits(nil, true)
		if want := []string{"chore(pack): update pack files"}; !reflect.DeepEqual(messages(steps), want) {
			t.Errorf("messages = %q, want %q", messages(steps), want)
		}
	})
}

func TestCommitFixesAStaleCommittedIndex(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	p.write(t, "config/sodium.json", "{}")
	commit(t)

	// A config edit committed with plain git, without "packwiz refresh": the tree is clean, but the committed index
	// no longer describes the file
	p.write(t, "config/sodium.json", `{"edited": "by hand"}`)
	git(t, "add", "-A")
	git(t, "commit", "-q", "-m", "edit config by hand")
	requireClean(t)
	before := commitCount(t)

	// Nothing has changed since the last commit, so there are no changes to describe, but the index is wrong
	commit(t)

	if got := commitCount(t); got != before+1 {
		t.Fatalf("commit count = %d, want %d: one commit to fix the index", got, before+1)
	}
	if got := headMessage(t); got != "chore(pack): update pack files" {
		t.Errorf("commit message = %q", got)
	}
	requireConsistentPack(t, "", "HEAD")
	requireClean(t)
}

// plainCommit makes a commit the way a person would with git, with a message of their own and nothing staged
func plainCommit(t *testing.T, subject string, body ...string) {
	t.Helper()
	args := []string{"commit", "-q", "--allow-empty", "-m", subject}
	for _, paragraph := range body {
		args = append(args, "-m", paragraph)
	}
	git(t, args...)
}

func TestLogListsCommitsOldestFirstWithTheirBodies(t *testing.T) {
	setUpRepo(t)
	plainCommit(t, "first")
	plainCommit(t, "feat(mods)!: second", "A paragraph.\n\nWith two lines in it.", "BREAKING CHANGE: something")
	plainCommit(t, "third")
	r, err := openRepo(".")
	if err != nil {
		t.Fatalf("openRepo() returned error: %v", err)
	}

	got, err := r.log("")
	if err != nil {
		t.Fatalf("log() returned error: %v", err)
	}

	want := []changelog.Commit{
		{Subject: "first"},
		{Subject: "feat(mods)!: second", Body: "A paragraph.\n\nWith two lines in it.\n\nBREAKING CHANGE: something"},
		{Subject: "third"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("log() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestLogSinceListsOnlyTheCommitsAfterIt(t *testing.T) {
	setUpRepo(t)
	plainCommit(t, "one")
	second := func() string { plainCommit(t, "two"); return git(t, "rev-parse", "HEAD") }()
	plainCommit(t, "three")
	plainCommit(t, "four")
	git(t, "tag", "v-two", second)
	r, err := openRepo(".")
	if err != nil {
		t.Fatalf("openRepo() returned error: %v", err)
	}
	subjects := func(commits []changelog.Commit) []string {
		var out []string
		for _, c := range commits {
			out = append(out, c.Subject)
		}
		return out
	}

	for name, since := range map[string]string{"a hash": second, "a tag": "v-two", "a branch-like name": "HEAD~2"} {
		got, err := r.log(since)
		if err != nil {
			t.Fatalf("%s: log() returned error: %v", name, err)
		}
		if want := []string{"three", "four"}; !reflect.DeepEqual(subjects(got), want) {
			t.Errorf("%s: log() = %v, want %v", name, subjects(got), want)
		}
	}

	if got, err := r.log("HEAD"); err != nil || len(got) != 0 {
		t.Errorf("log(HEAD) = %v, %v, want nothing after the current commit", got, err)
	}
	if _, err := r.log("no-such-revision"); err == nil {
		t.Error("log() of a revision that doesn't exist returned no error")
	}
	// Something that looks like an option is never handed to git as one
	if _, err := r.log("--output=/tmp/should-not-exist"); err == nil || !strings.Contains(err.Error(), "isn't a commit") {
		t.Errorf("log() of an option = %v, want it refused as not being a commit", err)
	}
}

func TestLogSkipsMergeCommits(t *testing.T) {
	setUpRepo(t)
	plainCommit(t, "base")
	git(t, "checkout", "-q", "-b", "side")
	plainCommit(t, "on the side")
	git(t, "checkout", "-q", "-")
	plainCommit(t, "on the main line")
	git(t, "merge", "-q", "--no-ff", "-m", "Merge branch 'side'", "side")
	r, err := openRepo(".")
	if err != nil {
		t.Fatalf("openRepo() returned error: %v", err)
	}

	got, err := r.log("")
	if err != nil {
		t.Fatalf("log() returned error: %v", err)
	}
	for _, c := range got {
		if strings.HasPrefix(c.Subject, "Merge") {
			t.Errorf("log() has the merge commit %q, which says nothing about the pack", c.Subject)
		}
	}
	if len(got) != 3 {
		t.Errorf("log() has %d commits, want the 3 that aren't merges", len(got))
	}
}

func TestHeadAndLastChangedIn(t *testing.T) {
	setUpRepo(t)
	if err := os.WriteFile("a.txt", []byte("a"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	git(t, "add", "a.txt")
	git(t, "commit", "-q", "-m", "add a")
	added := git(t, "rev-parse", "HEAD")
	plainCommit(t, "changes nothing")
	r, err := openRepo(".")
	if err != nil {
		t.Fatalf("openRepo() returned error: %v", err)
	}

	if head, err := r.head(); err != nil || head != git(t, "rev-parse", "HEAD") {
		t.Errorf("head() = %q, %v, want the current commit", head, err)
	}
	if got, err := r.lastChangedIn("a.txt"); err != nil || got != added {
		t.Errorf("lastChangedIn(a.txt) = %q, %v, want the commit that changed it, %q", got, err, added)
	}
	if got, err := r.lastChangedIn("never-existed.txt"); err != nil || got != "" {
		t.Errorf("lastChangedIn(never-existed.txt) = %q, %v, want nothing", got, err)
	}
}

func TestPendingCommitsAreWhatCommitWouldMake(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")
	p.write(t, "config/a.json", "{}")
	before := commitCount(t)

	var got []changelog.Commit
	var err error
	cmdtest.CaptureStdout(t, func() { got, err = pendingCommits() })
	if err != nil {
		t.Fatalf("pendingCommits() returned error: %v", err)
	}

	want := []changelog.Commit{
		{Subject: "feat(mods)!: add Lithium 0.12.0 (server)", Body: wantBreakingFooter},
		{Subject: "fix(config): add config/a.json"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pendingCommits() =\n%+v\nwant\n%+v", got, want)
	}
	if now := commitCount(t); now != before {
		t.Errorf("commit count = %d, want %d; describing the commits isn't making them", now, before)
	}

	// And they are the ones that get made
	commit(t)
	if made := lastMessages(t, 2); !reflect.DeepEqual(made, []string{
		"feat(mods)!: add Lithium 0.12.0 (server)\n\n" + wantBreakingFooter, "fix(config): add config/a.json",
	}) {
		t.Errorf("committed %q, want what was described", made)
	}
}

func TestChangelogRepositoryIsRegisteredAndSaysWhenThereIsNoGitRepository(t *testing.T) {
	setUpRepo(t)
	setUpPack(t, "", "1.0.0")
	if repo, err := changelog.OpenRepository(); err != nil || repo == nil {
		t.Errorf("OpenRepository() inside a repository = %v, %v, want one", repo, err)
	}

	// Somewhere that isn't in one
	dir := cmdtest.Chdir(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	cmdtest.SetViper(t, "pack-file", "pack.toml")
	_, err := changelog.OpenRepository()
	if err == nil || !strings.Contains(err.Error(), "git init") {
		t.Errorf("OpenRepository() outside a repository error = %v, want one telling the user to run git init", err)
	}
	if !errors.Is(err, changelog.ErrNoRepository) {
		t.Errorf("OpenRepository() outside a repository error = %v, want it to be changelog.ErrNoRepository so a changelog is made without one", err)
	}
}

// Everything packwiz git commit writes must be read back by a changelog as the change it described, and as the same
// size of change, or the version a release comes to would depend on how it was worked out
func TestCommitMessagesAreReadBackAsTheChangesTheyDescribe(t *testing.T) {
	mods := []changelog.Change{
		{Kind: changelog.ModAdded, Name: "Sodium", Side: core.ClientSide, To: "0.5.7"},
		{Kind: changelog.ModRemoved, Name: "Sodium", Side: core.ClientSide, From: "0.5.7"},
		{Kind: changelog.ModUpdated, Name: "Iris", Side: core.ClientSide, From: "1.7.0", To: "1.7.1"},
		{Kind: changelog.ModAdded, Name: "Lithium", Side: core.ServerSide, To: "0.12.0"},
		{Kind: changelog.ModUpdated, Name: "Lithium", Side: core.ServerSide, From: "0.12.0", To: "0.12.1"},
		{Kind: changelog.ModRemoved, Name: "Fabric API", Side: core.UniversalSide, From: "0.100.0+1.21.1"},
		{Kind: changelog.ModAdded, Name: "Iris Shaders", Side: core.ClientSide, To: "1.8.12+1.21.1-neoforge"},
		{Kind: changelog.ModUpdated, Name: "Roughly Enough Items (REI)", Side: core.UniversalSide, From: "16.0.799", To: "16.0.800"},
		{Kind: changelog.ModAdded, Name: "Xaero's Minimap", Side: core.ClientSide, To: "25.2.10_Fabric_1.21"},
		{Kind: changelog.ModUpdated, Name: "Sodium", Side: core.ClientSide, From: "sodium-0.5.7.jar", To: "sodium-0.5.8.jar"},
	}
	files := []changelog.Change{
		{Kind: changelog.FileAdded, Path: "config/sodium.json"},
		{Kind: changelog.FileChanged, Path: "config/iris.properties"},
		{Kind: changelog.FileRemoved, Path: "options.txt"},
	}

	read := func(changes []changelog.Change) []changelog.Change {
		message := Message(changes)
		subject, body, _ := strings.Cut(message, "\n")
		return changelog.ChangesFromCommits([]changelog.Commit{{Subject: subject, Body: strings.TrimSpace(body)}})
	}

	t.Run("each change in a commit of its own", func(t *testing.T) {
		for _, c := range append(append([]changelog.Change(nil), mods...), files...) {
			got := read([]changelog.Change{c})
			if len(got) != 1 || got[0] != c {
				t.Errorf("Message(%+v) was read back as %+v", c, got)
			}
			if changelog.HighestBump(got) != c.Bump() {
				t.Errorf("Message(%+v) was read back as a %v change, want %v", c, changelog.HighestBump(got), c.Bump())
			}
		}
	})

	t.Run("several files in one commit", func(t *testing.T) {
		if got := read(files); !reflect.DeepEqual(got, files) {
			t.Errorf("the config commit was read back as\n%+v\nwant\n%+v", got, files)
		}
	})

	t.Run("mods and files together, as commits once were", func(t *testing.T) {
		together := []changelog.Change{files[0], mods[3], mods[2]}
		got := read(together)
		if len(got) != 3 || changelog.HighestBump(got) != changelog.HighestBump(together) {
			t.Errorf("read back %+v, want the same three changes and bump", got)
		}
	})
}

func TestReadingCommitMessagesBackNeverChangesTheBumpOfAnyCombination(t *testing.T) {
	kinds := []changelog.Change{
		{Kind: changelog.ModAdded, Name: "A", Side: core.ClientSide, To: "1"},
		{Kind: changelog.ModRemoved, Name: "B", Side: core.ClientSide, From: "1"},
		{Kind: changelog.ModUpdated, Name: "C", Side: core.ClientSide, From: "1", To: "2"},
		{Kind: changelog.ModAdded, Name: "D", Side: core.ServerSide, To: "1"},
		{Kind: changelog.ModUpdated, Name: "E", Side: core.UniversalSide, From: "1", To: "2"},
		{Kind: changelog.FileChanged, Path: "config/f.json"},
		{Kind: changelog.FileAdded, Path: "config/g.json"},
	}
	for mask := 1; mask < 1<<len(kinds); mask++ {
		var changes []changelog.Change
		for i, c := range kinds {
			if mask&(1<<i) != 0 {
				changes = append(changes, c)
			}
		}
		// As one commit, and as a commit for each mod and one for the files, which is what is actually made
		var commits []changelog.Commit
		for _, step := range planCommits(changes, false) {
			subject, body, _ := strings.Cut(step.message, "\n")
			commits = append(commits, changelog.Commit{Subject: subject, Body: strings.TrimSpace(body)})
		}
		got := changelog.ChangesFromCommits(commits)
		if want := changelog.HighestBump(changes); changelog.HighestBump(got) != want {
			t.Fatalf("the commits for %+v were read back as %+v, a %v change; want %v", changes, got, changelog.HighestBump(got), want)
		}
		if len(got) != len(changes) {
			t.Fatalf("the commits for %+v were read back as %d changes, want %d: %+v", changes, len(got), len(changes), got)
		}
	}
}
