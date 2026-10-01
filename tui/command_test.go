package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

func TestInterfaceIsWhatPackwizDoesWithNoCommand(t *testing.T) {
	root := cmd.Root()
	if root.Run == nil {
		t.Fatal("packwiz with no command does nothing")
	}
	if root.Long != help {
		t.Error("the root command's help doesn't describe the interface")
	}
	if c, _, err := root.Find([]string{"tui"}); err == nil && c != root {
		t.Error("there is still a tui command")
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
		if want := fmt.Sprintf("  %d %s ", i+1, s.title()); !strings.Contains(help, want) {
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
