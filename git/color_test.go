package git

import (
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

func TestStyleMessagePicksOutTheSubject(t *testing.T) {
	cmdtest.SetColor(t, ui.Always)
	const subject = "feat(mods)!: add Lithium 0.12.0 (server)"

	for name, message := range map[string]string{
		"a subject alone":  subject,
		"a body as well":   subject + "\n\n- Added **Lithium**\n\n" + breakingFooter,
		"a trailing break": subject + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			got := styleMessage(message)
			_, rest, _ := strings.Cut(message, "\n")
			if !strings.HasPrefix(got, ui.Bold.Sprint(subject)) {
				t.Errorf("styleMessage() = %q, want it to start with the subject in bold", got)
			}
			if !strings.HasSuffix(got, rest) {
				t.Errorf("styleMessage() = %q, want the rest of the message left as it is", got)
			}
			if ui.Strip(got) != message {
				t.Errorf("styleMessage() changed the text of the message: %q", ui.Strip(got))
			}
		})
	}
}

func TestCommitDryRunColourOnlyAdds(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	p.mod(t, "Lithium", core.ServerSide, "0.12.0")
	p.write(t, "config/a.json", "{}")

	_, coloured := cmdtest.AssertColourOnlyAdds(t, func() {
		if err := runCommit(true); err != nil {
			t.Fatalf("runCommit(dryRun) returned error: %v", err)
		}
	})

	for name, want := range map[string]string{
		"the subject of a commit":                  ui.Bold.Sprint("feat(mods)!: add Lithium 0.12.0 (server)"),
		"the line between two commits":             "\n" + ui.Muted.Sprint("---") + "\n",
		"the subject of the other one":             ui.Bold.Sprint("fix(config): add config/a.json"),
		"the rest of the message is left as it is": ui.Bold.Sprint("feat(mods)!: add Lithium 0.12.0 (server)") + "\n\n" + breakingFooter + "\n",
	} {
		if !strings.Contains(coloured, want) {
			t.Errorf("%s: dry run missing %q:\n%q", name, want, coloured)
		}
	}
}

func TestCommitSaysWhatItCommittedInGreen(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	cmdtest.SetColor(t, ui.Always)

	out := commit(t)

	want := ui.Success.Sprint("Committed:") + " chore(pack): initial commit\n"
	if !strings.HasSuffix(cmdtest.WithoutProgress(out), want) {
		t.Errorf("output = %q, want it to end with %q", out, want)
	}
	// Colour is for the terminal: what was committed has none of it
	if message := headMessage(t); strings.Contains(message, "\x1b") {
		t.Errorf("the commit message has escape sequences in it: %q", message)
	}
}

func TestNothingToCommitIsAnInfoNotice(t *testing.T) {
	setUpRepo(t)
	p := setUpPack(t, "", "1.0.0")
	p.mod(t, "Sodium", core.ClientSide, "0.5.7")
	commit(t)
	cmdtest.SetColor(t, ui.Always)

	out := commit(t)

	if want := ui.Info.Sprint("Nothing to commit.") + "\n"; !strings.HasSuffix(cmdtest.WithoutProgress(out), want) {
		t.Errorf("output = %q, want it to end with %q", out, want)
	}
}
