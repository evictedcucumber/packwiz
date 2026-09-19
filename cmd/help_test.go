package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
)

// syntheticTree makes a command tree that uses every part of what cobra writes about how a command is used: groups of
// subcommands and ones that aren't in one, aliases, examples, flags of its own and ones it inherits, a subcommand that
// is hidden and one that is deprecated, one with subcommands of its own, and a help topic.
func syntheticTree() *cobra.Command {
	run := func(*cobra.Command, []string) {}

	root := &cobra.Command{
		Use:   "tool",
		Short: "A tool for testing",
		// The line "Flags:" is only a line of the description, and isn't to be taken for a heading
		Long: "A tool for testing how help is written.  \n\nFlags:\n  this is part of the description\n\n",
	}
	root.PersistentFlags().String("config", "cfg.toml", "The config file to use")
	root.PersistentFlags().BoolP("yes", "y", false, "Say yes to everything")
	root.AddGroup(&cobra.Group{ID: "main", Title: "Main Commands:"})

	add := &cobra.Command{
		Use: "add [name]", Short: "Add a thing", Aliases: []string{"a", "new"}, GroupID: "main", Run: run,
		Example: "  tool add sodium\n  tool add lithium --count 2",
	}
	add.Flags().IntP("count", "c", 3, "How many to add")
	add.Flags().Bool("force", false, "Add it even if it is there")

	sub := &cobra.Command{Use: "sub", Short: "Has subcommands of its own", GroupID: "main"}
	sub.AddCommand(&cobra.Command{Use: "leaf", Short: "A leaf", Run: run})

	root.AddCommand(
		add,
		sub,
		&cobra.Command{Use: "list", Short: "List things", Run: run},
		&cobra.Command{Use: "secret", Short: "Hidden", Hidden: true, Run: run},
		&cobra.Command{Use: "old", Short: "Deprecated", Deprecated: "use list", Run: run},
		&cobra.Command{Use: "topic", Short: "About a topic"},
	)
	root.InitDefaultHelpCmd()
	return root
}

// eachCommand calls fn for a command and everything under it.
func eachCommand(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		eachCommand(sub, fn)
	}
}

// whatIsWritten is the usage and the help of every command in the tree, by the command and which of them it is.
func whatIsWritten(root *cobra.Command) map[string]string {
	written := make(map[string]string)
	eachCommand(root, func(c *cobra.Command) {
		var help bytes.Buffer
		c.SetOut(&help)
		_ = c.Help()
		c.SetOut(nil)

		written[c.CommandPath()+" usage"] = c.UsageString()
		written[c.CommandPath()+" help"] = help.String()
	})
	return written
}

// The text is cobra's. It is made here so that it can be coloured for the stream it goes to, so it has to say just what
// cobra's own does, and this makes a change in cobra's templates that isn't followed here a test that fails.
func TestUsageAndHelpAreWhatCobraWrites(t *testing.T) {
	cmdtest.SetColor(t, ui.Never)

	t.Run("a tree that uses every part of usage", func(t *testing.T) {
		cobraWrites := whatIsWritten(syntheticTree())

		ours := syntheticTree()
		ours.SetUsageFunc(usageFunc)
		ours.SetHelpFunc(helpFunc)
		weWrite := whatIsWritten(ours)

		requireSameText(t, cobraWrites, weWrite)

		// Nothing to compare if what cobra writes leaves parts out
		all := ""
		for _, text := range cobraWrites {
			all += text
		}
		for _, part := range []string{
			"Usage:", "Aliases:", "Examples:", "Main Commands:", "Additional Commands:", "Additional help topics:",
			"Flags:", "Global Flags:", "(default ", `Use "tool [command] --help" for more information about a command.`,
		} {
			if !strings.Contains(all, part) {
				t.Errorf("the tree doesn't make cobra write %q, which is a part of usage that is to be compared", part)
			}
		}
		for _, hidden := range []string{"secret", "Deprecated"} {
			if strings.Contains(cobraWrites["tool usage"], hidden) {
				t.Errorf("cobra lists %q, which is hidden, in the usage of the tool", hidden)
			}
		}
	})

	t.Run("the commands of packwiz", func(t *testing.T) {
		ours := whatIsWritten(rootCmd)

		rootCmd.SetUsageFunc(nil)
		rootCmd.SetHelpFunc(nil)
		t.Cleanup(func() {
			rootCmd.SetUsageFunc(usageFunc)
			rootCmd.SetHelpFunc(helpFunc)
		})
		cobraWrites := whatIsWritten(rootCmd)

		if len(ours) < 10 {
			t.Fatalf("only %d things were written by the commands of packwiz, want its commands to have been found", len(ours))
		}
		requireSameText(t, cobraWrites, ours)
	})
}

// cobraTypos are how cobra's own way of writing usage (Go code that is meant to be the same as its template) is
// different from its template, which is what is used here.
var cobraTypos = strings.NewReplacer("Additional help topcis:", "Additional help topics:")

func requireSameText(t *testing.T, cobraWrites, weWrite map[string]string) {
	t.Helper()
	for what, text := range cobraWrites {
		cobraWrites[what] = cobraTypos.Replace(text)
	}
	if len(cobraWrites) != len(weWrite) {
		t.Errorf("cobra writes %d things and we write %d", len(cobraWrites), len(weWrite))
	}
	for what, want := range cobraWrites {
		if got, ok := weWrite[what]; !ok {
			t.Errorf("%s: not written", what)
		} else if got != want {
			t.Errorf("%s: written differently from how cobra writes it\n--- cobra\n%s\n--- written\n%s", what, want, got)
		}
	}
}

func TestStyleUsagePicksOutTheParts(t *testing.T) {
	cmdtest.SetColor(t, ui.Always)
	tree := syntheticTree()
	add, _, err := tree.Find([]string{"add"})
	if err != nil {
		t.Fatalf("Find() returned error: %v", err)
	}

	rootHelp, err := helpText(tree)
	if err != nil {
		t.Fatalf("helpText() returned error: %v", err)
	}
	addUsage, err := usageText(add)
	if err != nil {
		t.Fatalf("usageText() returned error: %v", err)
	}
	styledRoot, styledAdd := styleUsage(rootHelp, os.Stdout, groupTitles(tree)), styleUsage(addUsage, os.Stdout, groupTitles(add))

	// Colour is only added to the text
	if ui.Strip(styledRoot) != rootHelp || ui.Strip(styledAdd) != addUsage {
		t.Errorf("colouring changed the text:\n%q\nto\n%q", rootHelp, ui.Strip(styledRoot))
	}

	for name, want := range map[string]string{
		"the usage heading":                  ui.Bold.Sprint("Usage:") + "\n  tool [command]",
		"the heading of a group of commands": ui.Bold.Sprint("Main Commands:"),
		"a heading for the rest":             ui.Bold.Sprint("Additional Commands:"),
		"the help topics":                    ui.Bold.Sprint("Additional help topics:"),
		"the flags heading":                  ui.Bold.Sprint("Flags:") + "\n      " + ui.Info.Sprint("--config"),
		"a command in a group":               "  " + ui.Info.Sprint("sub") + " ",
		"the name of a command":              "  " + ui.Info.Sprint("add") + " ",
		"the name of another command":        "  " + ui.Info.Sprint("list") + " ",
		"a flag with a shorthand":            ui.Info.Sprint("-y, --yes"),
		"what a flag defaults to":            ui.Muted.Sprint(`(default "cfg.toml")`),
		"the hint at the end":                ui.Muted.Sprint(`Use "tool [command] --help" for more information about a command.`),
		// A heading is only one of usage, and what comes before it is the description of the command
		"the description is left as it is": "\n\nFlags:\n  this is part of the description\n\n" + ui.Bold.Sprint("Usage:"),
	} {
		if !strings.Contains(styledRoot, want) {
			t.Errorf("%s: missing %q:\n%q", name, want, styledRoot)
		}
	}

	for name, want := range map[string]string{
		"the aliases heading":       ui.Bold.Sprint("Aliases:"),
		"the examples heading":      ui.Bold.Sprint("Examples:"),
		"a flag with no shorthand":  ui.Info.Sprint("--force"),
		"a flag with a shorthand":   ui.Info.Sprint("-c, --count"),
		"the default of a flag":     ui.Muted.Sprint("(default 3)"),
		"the heading of the global": ui.Bold.Sprint("Global Flags:"),
	} {
		if !strings.Contains(styledAdd, want) {
			t.Errorf("%s: missing %q:\n%q", name, want, styledAdd)
		}
	}
	// The text of examples and aliases has no names or flags in it to pick out
	if strings.Contains(styledAdd, ui.Info.Sprint("tool")) {
		t.Errorf("the text of the examples was picked out:\n%q", styledAdd)
	}
}

func TestStyleUsageLeavesTextAloneWithoutColour(t *testing.T) {
	cmdtest.SetColor(t, ui.Never)
	text, err := helpText(syntheticTree())
	if err != nil {
		t.Fatalf("helpText() returned error: %v", err)
	}
	if got := styleUsage(text, os.Stdout, []string{"Main Commands:"}); got != text {
		t.Errorf("styleUsage() changed the text with colour off:\n%q", got)
	}
}

func TestDestination(t *testing.T) {
	if got := destination(&bytes.Buffer{}, os.Stderr); got != os.Stderr {
		t.Errorf("destination(buffer, stderr) = %v, want the fallback: cobra writes usage to a buffer to print it to stderr", got)
	}
	if got := destination(os.Stdout, os.Stderr); got != os.Stdout {
		t.Errorf("destination(stdout, stderr) = %v, want the file that is written to", got)
	}
}

func TestHelpAndUsageAreColouredForTheCommandsOfPackwiz(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		_, coloured := cmdtest.AssertColourOnlyAdds(t, func() { _ = listCmd.Help() })

		if want := ui.Info.Sprint("--show-kind"); !strings.Contains(coloured, want) {
			t.Errorf("help missing the flag %q:\n%q", want, coloured)
		}
	})
	t.Run("usage after an error", func(t *testing.T) {
		_, coloured := cmdtest.AssertColourOnlyAdds(t, func() { fmt.Print(listCmd.UsageString()) })

		if want := ui.Bold.Sprint("Usage:"); !strings.Contains(coloured, want) {
			t.Errorf("usage missing %q:\n%q", want, coloured)
		}
	})
	t.Run("the commands of the tool", func(t *testing.T) {
		_, coloured := cmdtest.AssertColourOnlyAdds(t, func() { _ = rootCmd.Help() })

		if want := ui.Info.Sprint("list"); !strings.Contains(coloured, want) {
			t.Errorf("help missing the command %q:\n%q", want, coloured)
		}
	})
}

// A command writes help to stdout and usage to stderr, and each is a terminal or not on its own: colour for one isn't
// for the other.
func TestHelpFollowsStdoutAndUsageFollowsStderr(t *testing.T) {
	terminal := cmdtest.OpenTerminal(t)
	cmdtest.SetColor(t, ui.Auto)
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")

	t.Run("only stderr is a terminal", func(t *testing.T) {
		oldErr := os.Stderr
		os.Stderr = terminal
		t.Cleanup(func() { os.Stderr = oldErr })

		// Stdout is what CaptureStdout gives it: a pipe
		help := cmdtest.CaptureStdout(t, func() { _ = listCmd.Help() })
		if ui.Strip(help) != help {
			t.Errorf("help is coloured although stdout isn't a terminal: %q", help)
		}
		usage := listCmd.UsageString()
		if ui.Strip(usage) == usage {
			t.Errorf("usage isn't coloured although stderr is a terminal: %q", usage)
		}
	})

	t.Run("only stdout is a terminal", func(t *testing.T) {
		oldOut := os.Stdout
		os.Stdout = terminal
		t.Cleanup(func() { os.Stdout = oldOut })

		var usage string
		cmdtest.CaptureStderr(t, func() { usage = listCmd.UsageString() })
		if ui.Strip(usage) != usage {
			t.Errorf("usage is coloured although stderr isn't a terminal: %q", usage)
		}
	})
}

func TestFlagErrorAppliesTheColorFlagAndReturnsTheError(t *testing.T) {
	cmdtest.SetColor(t, ui.Never)
	setColorFlag(t, "always")
	bad := errors.New("unknown flag: --bogus")

	got := rootCmd.FlagErrorFunc()(listCmd, bad)

	if got != bad {
		t.Errorf("the flag error func returned %v, want the error it was given", got)
	}
	if mode := ui.SetMode(ui.Never); mode != ui.Always {
		t.Errorf("mode = %v, want Always: --color came before the flag that couldn't be parsed", mode)
	}
}

func TestErrorPrefixFollowsTheColorMode(t *testing.T) {
	t.Cleanup(func() { rootCmd.SetErrPrefix(errPrefix()) })
	cmdtest.SetColor(t, ui.Never)

	setColorFlag(t, "always")
	if err := applyColor(); err != nil {
		t.Fatalf("applyColor() returned error: %v", err)
	}
	if got, want := rootCmd.ErrPrefix(), ui.Error.Sprint("Error:"); got != want {
		t.Errorf("ErrPrefix() = %q, want %q", got, want)
	}

	setColorFlag(t, "never")
	if err := applyColor(); err != nil {
		t.Fatalf("applyColor() returned error: %v", err)
	}
	if got := rootCmd.ErrPrefix(); got != "Error:" {
		t.Errorf("ErrPrefix() = %q, want the plain prefix cobra has", got)
	}
}
