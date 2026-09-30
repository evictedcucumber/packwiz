package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func TestTuiCommandIsRegisteredOnTheRootAndTakesNoArguments(t *testing.T) {
	if tuiCmd.Parent() == nil {
		t.Fatal("the tui command isn't a subcommand of anything")
	}
	if tuiCmd.Parent().Name() != "packwiz" {
		t.Errorf("the tui command's parent is %q, want the root command", tuiCmd.Parent().Name())
	}
	if tuiCmd.Short == "" || tuiCmd.Long == "" {
		t.Error("the tui command has no help")
	}
	if err := tuiCmd.Args(tuiCmd, []string{"extra"}); err == nil {
		t.Error("the tui command accepts an argument")
	}
	if err := tuiCmd.Args(tuiCmd, nil); err != nil {
		t.Errorf("the tui command rejects having none: %v", err)
	}
}

// The interface can only be drawn on a terminal, and what a program that isn't on one says should be a sentence rather
// than the escape sequences that it would otherwise be written
func TestRunNeedsATerminalAndSaysSoBeforeReadingThePack(t *testing.T) {
	// Not a pack, and stdin a file: the terminal is what is checked first
	cmdtest.Chdir(t)
	cmdtest.SetStdin(t, "")
	err := run()
	if err == nil {
		t.Fatal("run() returned no error when it isn't on a terminal")
	}
	if !strings.Contains(err.Error(), "needs a terminal") {
		t.Errorf("run() returned %q, want it to say that a terminal is needed", err)
	}
}

// The help says which number goes to which screen, so it has to follow the order they are made in
func TestTuiHelpListsTheScreensInTheOrderTheyAreGoneToWith(t *testing.T) {
	screens := newScreens(packBackend{}, configData{})
	for i, s := range screens {
		if want := fmt.Sprintf("  %d %s ", i+1, s.title()); !strings.Contains(tuiCmd.Long, want) {
			t.Errorf("the help doesn't have %q, want every screen listed by its number", want)
		}
	}
	// Every screen says what it is for, which is what the overview lists
	for _, s := range screens {
		if _, ok := s.(describer); !ok && s.title() != "Overview" {
			t.Errorf("the %s screen doesn't say what it is for", s.title())
		}
	}
	if len(screens) > 9 {
		t.Errorf("there are %d screens, but only the numbers 1 to 9 go to one", len(screens))
	}
}
