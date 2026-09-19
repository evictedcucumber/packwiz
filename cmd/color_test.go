package cmd

import (
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// setColorFlag gives --color a value as the command line would, for the duration of the test. (A viper override can't be
// taken back, and would stay in force for the tests that follow.)
func setColorFlag(t *testing.T, value string) {
	t.Helper()
	flag := rootCmd.PersistentFlags().Lookup("color")
	old, oldChanged := flag.Value.String(), flag.Changed
	if err := rootCmd.PersistentFlags().Set("color", value); err != nil {
		t.Fatalf("failed to set --color: %v", err)
	}
	t.Cleanup(func() {
		_ = flag.Value.Set(old)
		flag.Changed = oldChanged
	})
}

func TestApplyColorSetsTheMode(t *testing.T) {
	for value, want := range map[string]ui.Mode{
		"always": ui.Always,
		"never":  ui.Never,
		"auto":   ui.Auto,
	} {
		t.Run(value, func(t *testing.T) {
			// Start from a mode other than the one expected, so that the mode being right means it was set
			start := ui.Never
			if want == ui.Never {
				start = ui.Always
			}
			cmdtest.SetColor(t, start)
			setColorFlag(t, value)

			if err := applyColor(); err != nil {
				t.Fatalf("applyColor() returned error: %v", err)
			}
			if got := ui.SetMode(ui.Auto); got != want {
				t.Errorf("mode = %v, want %v", got, want)
			}
		})
	}
}

func TestApplyColorRejectsAnUnknownMode(t *testing.T) {
	cmdtest.SetColor(t, ui.Never)
	setColorFlag(t, "rainbow")

	err := applyColor()
	if err == nil {
		t.Fatal("applyColor() returned no error for an unknown mode")
	}
	if !strings.Contains(err.Error(), "rainbow") {
		t.Errorf("error = %q, want it to name the mode it doesn't know", err)
	}
	if got := ui.SetMode(ui.Never); got != ui.Never {
		t.Errorf("mode = %v, want it left alone when the value is invalid", got)
	}
}

func TestColourIsAnotherNameForTheColorFlag(t *testing.T) {
	flags := rootCmd.PersistentFlags()
	color := flags.Lookup("color")
	if color == nil {
		t.Fatal("the root command has no --color flag")
	}
	if got := flags.Lookup("colour"); got != color {
		t.Errorf("--colour = %v, want the same flag as --color", got)
	}
	if color.DefValue != "auto" {
		t.Errorf("--color defaults to %q, want auto", color.DefValue)
	}
}

func TestListColourOnlyAddsToItsOutput(t *testing.T) {
	setUpListFixture(t)
	setListFlag(t, "version", "true")
	setListFlag(t, "show-kind", "true")

	plain, coloured := cmdtest.AssertColourOnlyAdds(t, func() { listCmd.Run(listCmd, nil) })

	if want := "Alpha Mod (alpha.jar) [main]\nBeta Mod (beta.jar) [dependency]\nGamma Mod (gamma.jar) [main]\n"; plain != want {
		t.Errorf("plain output = %q, want %q", plain, want)
	}
	for name, want := range map[string]string{
		"a file name fades back":         ui.Muted.Sprint("(alpha.jar)"),
		"a main mod stands out":          ui.Info.Sprint("[main]"),
		"a dependency fades back":        ui.Muted.Sprint("[dependency]"),
		"a name is left as it is":        "Alpha Mod " + ui.Muted.Sprint("(alpha.jar)"),
		"the dependency kind is applied": "Beta Mod " + ui.Muted.Sprint("(beta.jar)") + " " + ui.Muted.Sprint("[dependency]"),
	} {
		if !strings.Contains(coloured, want) {
			t.Errorf("%s: output missing %q:\n%q", name, want, coloured)
		}
	}
}

func TestPinReportsSuccessInGreenWithTheNameInBold(t *testing.T) {
	setUpListFixture(t)
	cmdtest.SetColor(t, ui.Always)

	out := cmdtest.CaptureStdout(t, func() { pinCmd.Run(pinCmd, []string{"alpha"}) })

	if want := ui.Success.Sprintf("%s pinned successfully!", ui.Bold.Sprint("alpha")); !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%q", want, out)
	}
	if want := ui.Muted.Sprint("Loading modpack..."); !strings.Contains(out, want) {
		t.Errorf("output missing progress %q:\n%q", want, out)
	}
}

func TestStyleUpdate(t *testing.T) {
	cmdtest.SetColor(t, ui.Always)
	tests := []struct {
		name, update, want string
	}{
		{"old and new", "sodium-0.5.jar -> sodium-0.6.jar", ui.Transition("sodium-0.5.jar", "sodium-0.6.jar")},
		{"no arrow", "updated to 1.2", "updated to 1.2"},
		{"only the first arrow splits", "a -> b -> c", ui.Transition("a", "b -> c")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := styleUpdate(tt.update)
			if got != tt.want {
				t.Errorf("styleUpdate(%q) = %q, want %q", tt.update, got, tt.want)
			}
			if ui.Strip(got) != tt.update {
				t.Errorf("styleUpdate(%q) changed its text to %q", tt.update, ui.Strip(got))
			}
		})
	}
}
