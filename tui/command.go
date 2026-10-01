// Package tui is packwiz's interactive terminal interface (packwiz, run with no command), built on Bubble Tea.
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
	"io/fs"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

// help is what "packwiz --help" says: the interface is what packwiz does when it is given no command, so it is described
// with the root command's own help
const help = `Run with no command to open an interactive interface for the pack in the current directory: what the commands
do, on screens. Go between them with tab and shift+tab, or with the number of a screen:

  1 Overview   what the pack is and what is in it
  2 Mods       the pack's mods, as "packwiz list" has them: search them, pin one, mark one as a dependency, update one, write
               the list to MODS.md, and see what the pack knows of it
  3 Add        add a mod, resource pack or shader from Modrinth, as "packwiz modrinth add" does: search for it by name, or
               give the address of its page or its slug, and see what it needs before you say yes
  4 Updates    look for new versions of the mods, pick the ones to update, and update them, as "packwiz update --all" does
  5 Check      what "packwiz validate" finds wrong with the pack, and "packwiz fix" for what can be fixed
  6 Deps       what each mod needs and whether the pack has it, as "packwiz modrinth deps" does, and saving what was looked up
  7 Config     the pack's config files, as "packwiz config list" has them, and who owns each, as "packwiz config relate" says
  8 Export     export the pack as a .mrpack for Modrinth, as "packwiz modrinth export" does
  9 Release    the release that the pack's changes would make ("packwiz changelog"), committing what hasn't been ("packwiz
               commit"), releasing ("packwiz changelog release", "packwiz git release") and writing CHANGELOG.md

In a folder that has no pack it opens on a screen that makes one, as "packwiz init" does.

Everything it changes it changes as the commands do, so what it writes is what they write, and it asks before it changes
anything that isn't undone by pressing a key again. Each screen lists its keys at the bottom, and ? lists all of them. Keys
like j and k move as the arrow keys do, / searches (fuzzily, as fzf does: "sdm" finds Sodium) and esc leaves a search or a box.
What asks the network (Modrinth, Mojang) does so when you ask it to, and shows that it is working; q waits for what is
changing the pack to finish, and ctrl+c quits at once.

The config screen, as an example of what a screen has:

  r        relate the file the cursor is on (or every file that is marked) to the pack, its mod loader or mods
  x        stop an owner claiming a file, or take out an entry that matches no file
  space    mark a file, to relate several together; on a group, every file in it
  /        search the files, fuzzily: only those that match are shown, and the cursor goes to the best
  f        show all files, then only the valid, the invalid or the missing ones
  R        refresh the index, so that files added or deleted since show up

Relating writes what "packwiz config relate" does, and asks who to give the files to: the pack as a whole (options.txt,
say), its mod loader (NeoForge's own config files) and then the mods (type / to search them), and whether to claim each file
or the folder it is in.

It needs a terminal to run in.`

// launch opens the interface, and exits with a message if it can't be
func launch() {
	if err := run(); err != nil {
		ui.Error.Println(err)
		os.Exit(1)
	}
}

// run opens the interface on the pack in the current directory, or if there is none on a screen that makes one. The pack is
// read first, so that a directory that has a pack that can't be read is an error on the terminal as it is for any other
// command, rather than something the interface has to show.
func run() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("packwiz needs a terminal to open its interface when given no command: its input and its output must both be one")
	}

	backend := packBackend{}
	open := func() (string, []screen, error) {
		data, err := backend.load()
		if err != nil {
			return "", nil, err
		}
		return data.pack, newScreens(backend, data), nil
	}

	var a *app
	if _, err := os.Stat(viper.GetString("pack-file")); errors.Is(err, fs.ErrNotExist) {
		a = newApp("", newInitScreen(backend))
		a.opened = open
	} else {
		pack, screens, err := open()
		if err != nil {
			return err
		}
		a = newApp(pack, screens...)
	}
	_, err := tea.NewProgram(a).Run()
	return err
}

// newScreens makes the screens of the interface, in the order they are reached in: the number a screen is gone to with is
// its place in this list.
func newScreens(backend packBackend, data configData) []screen {
	overview := newOverviewScreen(backend)
	screens := []screen{
		overview,
		newModsScreen(backend),
		newAddScreen(backend),
		newUpdatesScreen(backend),
		newCheckScreen(backend),
		newDepsScreen(backend),
		newConfigScreen(backend, data),
		newExportScreen(backend),
		newReleaseScreen(backend),
	}
	overview.guide = guideFor(screens)
	return screens
}

func init() {
	cmd.SetDefault(help, launch)
}
