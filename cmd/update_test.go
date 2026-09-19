package cmd

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// fakeUpdater is an updater that says what a test tells it to about each mod.
type fakeUpdater struct {
	// checks are what it says of each mod, by its name; a mod that isn't in it is up to date
	checks map[string]core.UpdateCheck
	// err is what checking fails with, for all the mods, if it does
	err error
}

func (fakeUpdater) ParseUpdate(data map[string]interface{}) (interface{}, error) { return data, nil }

func (u fakeUpdater) CheckUpdate(mods []*core.Mod, _ core.Pack) ([]core.UpdateCheck, error) {
	if u.err != nil {
		return nil, u.err
	}
	checks := make([]core.UpdateCheck, len(mods))
	for i, mod := range mods {
		checks[i] = u.checks[mod.Name]
	}
	return checks, nil
}

func (fakeUpdater) DoUpdate([]*core.Mod, []interface{}) error { return nil }

// setUpUpdateFixture makes a pack that has a mod of each name, updated by the updater, in a fresh directory.
func setUpUpdateFixture(t *testing.T, updater fakeUpdater, names ...string) {
	t.Helper()
	cmdtest.Chdir(t)
	cmdtest.WritePackFile(t, core.Pack{
		Name: "Test Pack", PackFormat: core.CurrentPackFormat,
		Versions: map[string]string{"minecraft": "1.20.1"},
	})
	if err := os.MkdirAll("mods", 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}

	index := "hash-format = \"sha256\"\n"
	for _, name := range names {
		file := strings.ToLower(name)
		mod := "name = \"" + name + "\"\nfilename = \"" + file + ".jar\"\n\n[download]\nhash-format = \"sha256\"\nhash = \"h\"\n\n[update]\n[update.fake]\nid = \"" + file + "\"\n"
		if err := os.WriteFile("mods/"+file+".pw.toml", []byte(mod), 0644); err != nil {
			t.Fatalf("failed to write mod fixture %s: %v", name, err)
		}
		index += "\n[[files]]\nfile = \"mods/" + file + ".pw.toml\"\nhash = \"irrelevant\"\nmetafile = true\n"
	}
	if err := os.WriteFile("index.toml", []byte(index), 0644); err != nil {
		t.Fatalf("failed to write index.toml fixture: %v", err)
	}

	core.Updaters["fake"] = updater
	t.Cleanup(func() { delete(core.Updaters, "fake") })
}

// setUpdateAll gives update --all, as the command line would, for the duration of the test.
func setUpdateAll(t *testing.T) {
	t.Helper()
	flag := UpdateCmd.Flags().Lookup("all")
	old, oldChanged := flag.Value.String(), flag.Changed
	if err := UpdateCmd.Flags().Set("all", "true"); err != nil {
		t.Fatalf("failed to set --all: %v", err)
	}
	t.Cleanup(func() {
		_ = flag.Value.Set(old)
		flag.Changed = oldChanged
	})
}

func updateAll(t *testing.T) string {
	t.Helper()
	setUpdateAll(t)
	return cmdtest.CaptureStdout(t, func() { UpdateCmd.Run(UpdateCmd, nil) })
}

func TestUpdateAllDoesNotSayEverythingIsUpToDateWhenNoCheckWorked(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{checks: map[string]core.UpdateCheck{
		"Alpha": {Error: errors.New("boom a")},
		"Beta":  {Error: errors.New("boom b")},
	}}, "Alpha", "Beta")

	out := updateAll(t)

	for _, want := range []string{
		"Failed to check updates for Alpha: boom a\n",
		"Failed to check updates for Beta: boom b\n",
		"No updates found, but 2 files could not be checked.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "up to date") {
		t.Errorf("output says the files are up to date although they couldn't be checked:\n%s", out)
	}
}

func TestUpdateAllCountsOnlyTheFilesThatCouldNotBeChecked(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{checks: map[string]core.UpdateCheck{
		"Alpha": {Error: errors.New("boom")},
	}}, "Alpha", "Beta")

	out := updateAll(t)

	if want := "No updates found, but 1 file could not be checked.\n"; !strings.Contains(out, want) {
		t.Errorf("output missing %q (one file, so not \"files\"):\n%s", want, out)
	}
	if strings.Contains(out, "up to date") {
		t.Errorf("output says the files are up to date although one couldn't be checked:\n%s", out)
	}
}

func TestUpdateAllCountsEveryFileOfAnUpdaterThatFailedAsAWhole(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{err: errors.New("no network")}, "Alpha", "Beta", "Gamma")

	out := updateAll(t)

	for _, want := range []string{
		"Failed to check updates for fake: no network\n",
		"No updates found, but 3 files could not be checked.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "up to date") {
		t.Errorf("output says the files are up to date although none could be checked:\n%s", out)
	}
}

func TestUpdateAllSaysEverythingIsUpToDateWhenItIs(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{}, "Alpha", "Beta")
	setUpdateAll(t)

	plain, coloured := cmdtest.AssertColourOnlyAdds(t, func() { UpdateCmd.Run(UpdateCmd, nil) })

	if want := "All files are up to date!\n"; !strings.HasSuffix(plain, want) {
		t.Errorf("output = %q, want it to end with %q", plain, want)
	}
	if strings.Contains(plain, "could not be checked") {
		t.Errorf("output = %q, want nothing said of checks that failed, as none did", plain)
	}
	if want := ui.Success.Sprint("All files are up to date!") + "\n"; !strings.HasSuffix(coloured, want) {
		t.Errorf("coloured output = %q, want it to end with %q", coloured, want)
	}
}

func TestUpdateAllSaysWhatCouldNotBeCheckedInColour(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{checks: map[string]core.UpdateCheck{
		"Alpha": {Error: errors.New("boom")},
	}}, "Alpha")
	setUpdateAll(t)

	_, coloured := cmdtest.AssertColourOnlyAdds(t, func() { UpdateCmd.Run(UpdateCmd, nil) })

	for name, want := range map[string]string{
		"what failed":         ui.Error.Sprint("Failed to check updates for Alpha: boom") + "\n",
		"what that leaves us": ui.Warning.Sprint("No updates found, but 1 file could not be checked.") + "\n",
	} {
		if !strings.Contains(coloured, want) {
			t.Errorf("%s: output missing %q:\n%q", name, want, coloured)
		}
	}
}

// A check that failed for one file doesn't stop the others being updated
func TestUpdateAllOffersTheUpdatesThatWereFoundEvenWhenACheckFailed(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{checks: map[string]core.UpdateCheck{
		"Alpha": {UpdateAvailable: true, UpdateString: "alpha-1.jar -> alpha-2.jar"},
		"Beta":  {Error: errors.New("boom")},
	}}, "Alpha", "Beta")
	cmdtest.SetStdin(t, "n\n")

	out := updateAll(t)

	for _, want := range []string{
		"Failed to check updates for Beta: boom\n",
		"Updates found:\nAlpha: alpha-1.jar -> alpha-2.jar\n",
		"Do you want to update? [Y/n]: ",
		"Cancelled!\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "No updates found") || strings.Contains(out, "up to date") {
		t.Errorf("output says nothing was found although there is an update:\n%s", out)
	}
}

func TestUpdateOneFileStillSaysWhenItIsUpToDate(t *testing.T) {
	setUpUpdateFixture(t, fakeUpdater{}, "Alpha")

	out := cmdtest.CaptureStdout(t, func() { UpdateCmd.Run(UpdateCmd, []string{"alpha"}) })

	if want := "\"Alpha\" is already up to date!\n"; !strings.HasSuffix(out, want) {
		t.Errorf("output = %q, want it to end with %q", out, want)
	}
}

// Checking one file that fails is a failure of the command, as everything else that goes wrong in it is, and the file
// isn't said to be up to date. That ends the command, so it is run again as a process to see how it ends.
func TestUpdateOneFileFailsWhenItsCheckFails(t *testing.T) {
	if os.Getenv("PACKWIZ_TEST_UPDATE_ONE_FILE") == "1" {
		setUpUpdateFixture(t, fakeUpdater{checks: map[string]core.UpdateCheck{
			"Alpha": {Error: errors.New("boom")},
		}}, "Alpha")
		UpdateCmd.Run(UpdateCmd, []string{"alpha"})
		return // The command didn't end, so the process doesn't fail, which is what the test looks for
	}

	process := exec.Command(os.Args[0], "-test.run=^TestUpdateOneFileFailsWhenItsCheckFails$")
	process.Env = append(os.Environ(), "PACKWIZ_TEST_UPDATE_ONE_FILE=1")
	out, err := process.CombinedOutput()

	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("the command ended with %v, want it to fail with exit status 1\noutput: %s", err, out)
	}
	if want := "Failed to check updates for Alpha: boom\n"; !strings.Contains(string(out), want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	if strings.Contains(string(out), "up to date") {
		t.Errorf("output says the file is up to date although it couldn't be checked:\n%s", out)
	}
}
