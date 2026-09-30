package modrinth

import (
	"slices"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func TestCheckPackFindsWhatValidateFinds(t *testing.T) {
	pack, index := validatablePack(t)
	addMod(t, &index, validMod("good"))
	broken := validMod("broken")
	broken.fileName = ""
	broken.side = ""
	addMod(t, &index, broken)

	check := CheckPack(pack, index)
	v, _ := validate(t, pack, index)

	if check.Mods != v.mods || check.Errors != v.errors() || check.Warnings != v.warnings() {
		t.Errorf("the check has %d mods, %d errors and %d warnings, want what validate has: %d, %d and %d",
			check.Mods, check.Errors, check.Warnings, v.mods, v.errors(), v.warnings())
	}
	if len(check.Findings) != 1 {
		t.Fatalf("the findings are %v, want only those for the broken mod", check.Findings)
	}
	f := check.Findings[0]
	if f.Name != "broken" || f.Path != "mods/broken.pw.toml" {
		t.Errorf("the finding is for %q at %q, want the broken mod", f.Name, f.Path)
	}
	want := []Problem{
		{Error: true, Message: "has no file name"},
		{Error: false, Message: "has no side, so it is on both"},
	}
	if !slices.Equal(f.Problems, want) {
		t.Errorf("the problems are %v, want %v: the errors first, then the warnings", f.Problems, want)
	}
}

func TestCheckPackPutsThePackFirst(t *testing.T) {
	pack, index := validatablePack(t)
	pack.Version = ""
	broken := validMod("aaa")
	broken.fileName = ""
	addMod(t, &index, broken)

	check := CheckPack(pack, index)
	if len(check.Findings) != 2 || check.Findings[0].Path != "" || check.Findings[0].Name != "The pack" {
		t.Errorf("the findings are %v, want the pack's first, as validate prints them", check.Findings)
	}
}

func TestCheckPackSaysNothingOnTheTerminal(t *testing.T) {
	pack, index := validatablePack(t)
	// A mod that records no dependencies is looked up, which can have things to say
	plain := validMod("alpha")
	plain.deps = nil
	addMod(t, &index, plain)

	out := cmdtest.CaptureStdout(t, func() {
		check := CheckPack(pack, index)
		check.PlanFixes()
	})
	if out != "" {
		t.Errorf("checking the pack wrote %q to the terminal", out)
	}
}

func TestCheckPackReportsWhatLookupsSaidAsNotices(t *testing.T) {
	pack, index := validatablePack(t)
	plain := validMod("alpha")
	plain.deps = nil
	addMod(t, &index, plain)
	// Modrinth can't be reached, which is only a warning: the pack is still checked
	check := CheckPack(pack, index)

	if check.Errors != 0 {
		t.Errorf("the check has %d errors, want a lookup that failed to be only a warning", check.Errors)
	}
	joined := ""
	for _, f := range check.Findings {
		for _, p := range f.Problems {
			joined += p.Message + "\n"
		}
	}
	if !strings.Contains(joined, "dependencies not checked") {
		t.Errorf("the findings don't say that dependencies weren't checked:\n%s", joined)
	}
}

func TestPlanFixesDescribesWhatWouldBeChangedAndChangesNothing(t *testing.T) {
	pack, index, libPath, _ := fixPackWithAServerModAModNeeds(t)
	before := readFile(t, libPath)

	fixes := CheckPack(pack, index).PlanFixes()
	if fixes.Empty() {
		t.Fatal("the fixes are empty, want the library's side to change")
	}
	if len(fixes.Files) != 1 || fixes.Files[0].Path != libPath {
		t.Fatalf("the files are %v, want only the library", fixes.Files)
	}
	if want := []string{`side: server -> both, as "alpha" needs it on the client`}; !slices.Equal(stripAll(fixes.Files[0].Lines), want) {
		t.Errorf("the lines are %v, want %v", fixes.Files[0].Lines, want)
	}
	if readFile(t, libPath) != before {
		t.Error("planning the fixes changed the library")
	}
}

func stripAll(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimSpace(l)
	}
	return out
}

// saveFixture writes the pack and index of a fixture to disk, as they are in a pack that is used, which Apply reads them
// back from.
func saveFixture(t *testing.T, pack core.Pack, index core.Index) {
	t.Helper()
	if err := index.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	if err := pack.UpdateIndexHash(); err != nil {
		t.Fatalf("UpdateIndexHash() returned error: %v", err)
	}
	if err := pack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
}

func TestFixesApplyMakesTheChanges(t *testing.T) {
	pack, index, libPath, alphaPath := fixPackWithAServerModAModNeeds(t)
	saveFixture(t, pack, index)
	alphaBefore := readFile(t, alphaPath)

	fixes := CheckPack(pack, index).PlanFixes()
	var changed int
	var err error
	out := cmdtest.CaptureStdout(t, func() { changed, err = fixes.Apply() })

	if err != nil || changed != 1 {
		t.Fatalf("Apply() = %d, %v, want 1 file changed", changed, err)
	}
	if out != "" {
		t.Errorf("Apply() wrote %q to the terminal", out)
	}
	if got := loadModAt(t, libPath).Side; got != core.UniversalSide {
		t.Errorf("library side = %q, want %q", got, core.UniversalSide)
	}
	if readFile(t, alphaPath) != alphaBefore {
		t.Error("alpha changed, want only the library to")
	}
	assertIndexHasHashOf(t, libPath)

	// Checking it again, with what is on disk now, finds nothing
	pack, err = core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	index, err = pack.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	if after := CheckPack(pack, index); after.Errors != 0 || len(after.Findings) != 0 {
		t.Errorf("the pack still has findings after it was fixed: %v", after.Findings)
	}
}

func TestFixesApplyKeepsWhatWasChangedInThePackSinceTheyWerePlanned(t *testing.T) {
	pack, index, libPath, _ := fixPackWithAServerModAModNeeds(t)
	saveFixture(t, pack, index)
	fixes := CheckPack(pack, index).PlanFixes()

	// Something else changes pack.toml before the fixes are made
	changedPack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	changedPack.Description = "written in the meantime"
	if err := changedPack.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	if _, err := fixes.Apply(); err != nil {
		t.Fatalf("Apply() returned error: %v", err)
	}
	if got := loadModAt(t, libPath).Side; got != core.UniversalSide {
		t.Errorf("library side = %q, want the fix made", got)
	}
	after, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	if after.Description != "written in the meantime" {
		t.Errorf("pack.toml's description is %q, want the change made since the plan kept", after.Description)
	}
}

func TestPlanFixesOfAValidPackIsEmpty(t *testing.T) {
	pack, index := validatablePack(t)
	alpha := validMod("alpha")
	alpha.configFiles = []string{"config/alpha.json"}
	addMod(t, &index, alpha)
	if fixes := CheckPack(pack, index).PlanFixes(); !fixes.Empty() {
		t.Errorf("the fixes are %v, want none for a valid pack", fixes.Files)
	}
}
