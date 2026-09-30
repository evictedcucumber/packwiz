// Package tui is packwiz's interactive terminal interface (packwiz tui), built on Bubble Tea.
//
// It is a self-registering command package, like modrinth and changelog: importing it for its side effects adds the
// command. It works through core, the same as the commands do, so what it writes to a pack is what they write: it has
// no way of changing a pack of its own.
//
// The app (app.go) is a header, a footer, and one screen between them. A screen is a page of the interface, and
// knows nothing of the others; for now there is the one, for config files (config.go).
package tui

import (
	"errors"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// tuiCmd represents the tui command
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Open the interactive terminal interface",
	Long: `Open an interactive interface for the pack in the current directory.

It has the pack's config files: the same tree as "packwiz config list", of the pack, its mod loader or the mod that claims
each file, with the files nothing claims under "Invalid". Move through it with the arrow keys (or j and k), fold a group
of files with the left and right arrow keys, and change who owns what:

  r        relate the file the cursor is on (or every file that is marked) to the pack, its mod loader or mods
  x        stop an owner claiming a file, or take out an entry that matches no file
  space    mark a file, to relate several together; on a group, every file in it
  /        search the files, fuzzily: only those that match are shown, and the cursor goes to the best
  f        show all files, then only the valid, the invalid or the missing ones
  R        refresh the index, so that files added or deleted since show up

Relating writes what "packwiz config relate" does, and asks who to give the files to: the pack as a whole (options.txt,
say), its mod loader (NeoForge's own config files) and then the mods (type / to search them, fuzzily, as fzf does: "sdm"
finds Sodium), and whether to claim each file or the folder it is in. Press ? for all of the keys, and q to quit.

It needs a terminal to run in.`,
	Args: cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
		if err := run(); err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
	},
}

// run reads the pack, and opens the interface on it. The pack is read first, so that a directory that isn't one is an
// error on the terminal as it is for any other command, rather than something the interface has to show.
func run() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("packwiz tui needs a terminal: its input and its output must both be one")
	}

	backend := packBackend{}
	data, err := backend.load()
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(newApp(data.pack, newConfigScreen(backend, data))).Run()
	return err
}

func init() {
	cmd.Add(tuiCmd)
}
