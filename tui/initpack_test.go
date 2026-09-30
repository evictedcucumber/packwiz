package tui

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// fakeInit is an initBackend that says what a test tells it to, and records what it was asked to make.
type fakeInit struct {
	name         string
	versions     minecraftVersions
	versionsErr  error
	loaderLatest string
	loaderErr    error
	createErr    error

	loaderAsked []string
	created     []cmd.NewPack
}

func (f *fakeInit) defaultName() string { return f.name }

func (f *fakeInit) minecraftVersions() (minecraftVersions, error) { return f.versions, f.versionsErr }

func (f *fakeInit) loaderVersion(loader, mcVersion, chosen string) (string, error) {
	f.loaderAsked = append(f.loaderAsked, loader+"|"+mcVersion+"|"+chosen)
	if f.loaderErr != nil {
		return "", f.loaderErr
	}
	if chosen == "" {
		return f.loaderLatest, nil
	}
	return chosen, nil
}

func (f *fakeInit) createPack(pack cmd.NewPack) error {
	f.created = append(f.created, pack)
	return f.createErr
}

func newFakeInit() *fakeInit {
	return &fakeInit{
		name:         "Demo Pack",
		loaderLatest: "21.1.100",
		versions: minecraftVersions{
			latest: "1.21.1", latestSnapshot: "25w01a",
			valid: func(v string) bool { return v == "1.21.1" || v == "1.20.1" },
		},
	}
}

func initOn(t *testing.T, f *fakeInit) *initScreen {
	t.Helper()
	cmdtest.SetColor(t, ui.Never)
	s := newInitScreen(f)
	s.setSize(100, 24)
	feed(t, s, s.activate()())
	return s
}

func TestInitAsksWhatThePackIsWithWhatItCanSuggest(t *testing.T) {
	s := initOn(t, newFakeInit())
	out := strings.Join(body(t, s), "\n")
	for _, want := range []string{
		"No pack.toml is here", "> Name", "Demo Pack", "Author", "Version", "1.0.0", "Minecraft",
		"the latest release is 1.21.1", "Mod loader", "[NeoForge] none", "NeoForge version", "[ Create the pack ]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the screen doesn't say %q:\n%s", want, out)
		}
	}
	if !s.modal() {
		t.Error("the screen isn't modal, so a q that is typed would quit")
	}
}

func TestInitMakesThePackWithWhatWasTypedAndTheLatestOfWhatWasNot(t *testing.T) {
	f := newFakeInit()
	s := initOn(t, f)
	press(t, s, "ctrl+u")
	typeText(t, s, "My Pack")
	press(t, s, "enter")
	typeText(t, s, "Me")
	press(t, s, "ctrl+s")

	if len(f.created) != 1 {
		t.Fatalf("the pack was made %d times, want once", len(f.created))
	}
	want := cmd.NewPack{Name: "My Pack", Author: "Me", Version: "1.0.0", MCVersion: "1.21.1", Loader: "neoforge", LoaderVersion: "21.1.100", IndexFile: "index.toml"}
	if f.created[0] != want {
		t.Errorf("the pack was made with %+v, want %+v", f.created[0], want)
	}
	if len(f.loaderAsked) != 1 || f.loaderAsked[0] != "neoforge|1.21.1|" {
		t.Errorf("the loader version was asked for with %v, want the latest for the Minecraft version that was used", f.loaderAsked)
	}
}

func TestInitSendsTheAppOnToThePackOnceItIsMade(t *testing.T) {
	f := newFakeInit()
	s := initOn(t, f)
	_, next := s.update(keyMsg(t, "ctrl+s"))
	msg := next()
	for {
		res, cmd, mine := s.receive(msg)
		if !mine {
			break
		}
		if cmd == nil {
			msg = res
			break
		}
		msg = cmd()
	}
	_, done := s.update(msg)
	if done == nil {
		t.Fatal("the screen has nothing to say once the pack is made")
	}
	if _, ok := done().(packCreatedMsg); !ok {
		t.Errorf("the screen said %#v, want that there is a pack now", done())
	}
}

func TestInitTakesWhatWasChosenForTheVersionsAndTheLoader(t *testing.T) {
	f := newFakeInit()
	s := initOn(t, f)
	// Down to Minecraft, which is typed; then the loader, which is chosen from, and then its version
	press(t, s, "down", "down", "down")
	typeText(t, s, "1.20.1")
	press(t, s, "down", "right")
	if out := strings.Join(body(t, s), "\n"); !strings.Contains(out, "NeoForge [none]") || strings.Contains(out, "version") {
		t.Errorf("the screen doesn't show no loader is chosen, without a field for its version:\n%s", out)
	}
	press(t, s, "left", "down")
	typeText(t, s, "21.1.5")
	press(t, s, "ctrl+s")

	if len(f.created) != 1 || f.created[0].MCVersion != "1.20.1" || f.created[0].Loader != "neoforge" || f.created[0].LoaderVersion != "21.1.5" {
		t.Errorf("the pack was made with %+v, want what was chosen", f.created)
	}
}

func TestInitMakesAPackWithoutAModLoader(t *testing.T) {
	f := newFakeInit()
	s := initOn(t, f)
	press(t, s, "down", "down", "down", "down", "right", "ctrl+s")
	if len(f.created) != 1 || f.created[0].Loader != "" || f.created[0].LoaderVersion != "" {
		t.Errorf("the pack was made with %+v, want no mod loader", f.created)
	}
	if len(f.loaderAsked) != 0 {
		t.Errorf("the loader versions were asked for (%v) though there is no loader", f.loaderAsked)
	}
}

func TestInitSkipsTheLoaderVersionWhenThereIsNoLoader(t *testing.T) {
	s := initOn(t, newFakeInit())
	press(t, s, "down", "down", "down", "down", "right")
	press(t, s, "down")
	if s.focus != fieldCreate {
		t.Errorf("the focus is on field %d, want it to skip the version of a loader that isn't there and go to the button", s.focus)
	}
	press(t, s, "up")
	if s.focus != fieldLoader {
		t.Errorf("the focus is on field %d, want it to skip the version going back too", s.focus)
	}
}

func TestInitEnterMovesOnAndMakesThePackFromTheButton(t *testing.T) {
	f := newFakeInit()
	s := initOn(t, f)
	for range fieldCreate {
		press(t, s, "enter")
	}
	if len(f.created) != 0 {
		t.Fatal("the pack was made before the button was reached")
	}
	if s.focus != fieldCreate {
		t.Fatalf("the focus is on field %d, want the button", s.focus)
	}
	press(t, s, "enter")
	if len(f.created) != 1 {
		t.Errorf("the pack was made %d times, want once", len(f.created))
	}
}

func TestInitNeedsAName(t *testing.T) {
	f := newFakeInit()
	f.name = ""
	s := initOn(t, f)
	press(t, s, "ctrl+s")
	if got := statusOf(s); got != "The pack needs a name" || len(f.created) != 0 {
		t.Errorf("the status line is %q and %d packs were made, want it to ask for a name", got, len(f.created))
	}
}

func TestInitSaysWhatIsWrongWithAMinecraftVersion(t *testing.T) {
	f := newFakeInit()
	s := initOn(t, f)
	press(t, s, "down", "down", "down")
	typeText(t, s, "9.9.9")
	if out := strings.Join(body(t, s), "\n"); !strings.Contains(out, "isn't a version of Minecraft") {
		t.Errorf("the screen doesn't say the version is no good:\n%s", out)
	}
	press(t, s, "ctrl+s")
	if got := statusOf(s); got != `"9.9.9" isn't a version of Minecraft` || len(f.created) != 0 {
		t.Errorf("the status line is %q and %d packs were made, want the reason and none made", got, len(f.created))
	}
}

func TestInitSaysWhenTheVersionsOfMinecraftCannotBeLookedUp(t *testing.T) {
	f := newFakeInit()
	f.versionsErr = errors.New("no network")
	s := initOn(t, f)

	if got := statusOf(s); got != "Couldn't look up the versions of Minecraft: no network" {
		t.Errorf("the status line is %q, want the reason", got)
	}
	if out := strings.Join(body(t, s), "\n"); !strings.Contains(out, "can't look up the versions (no network)") {
		t.Errorf("the screen doesn't say by the field that the versions can't be looked up:\n%s", out)
	}
	press(t, s, "ctrl+s")
	if got := statusOf(s); !strings.Contains(got, "can't check the Minecraft version") || len(f.created) != 0 {
		t.Errorf("the status line is %q and %d packs were made, want it not to make a pack it can't check", got, len(f.created))
	}
}

func TestInitSaysWhenTheLoaderVersionIsNoGood(t *testing.T) {
	f := newFakeInit()
	f.loaderErr = errors.New(`"9" isn't a version of NeoForge for Minecraft 1.21.1`)
	s := initOn(t, f)
	press(t, s, "ctrl+s")
	if got := statusOf(s); got != `"9" isn't a version of NeoForge for Minecraft 1.21.1` || len(f.created) != 0 {
		t.Errorf("the status line is %q and %d packs were made, want the reason and none made", got, len(f.created))
	}
}

func TestInitSaysWhenThePackCannotBeMade(t *testing.T) {
	f := newFakeInit()
	f.createErr = errors.New("permission denied")
	s := initOn(t, f)
	press(t, s, "ctrl+s")
	if got := statusOf(s); got != "permission denied" {
		t.Errorf("the status line is %q, want the reason", got)
	}
}

func TestInitTakesTextThatIsPastedIn(t *testing.T) {
	s := initOn(t, newFakeInit())
	press(t, s, "down")
	feed(t, s, tea.PasteMsg{Content: "Someone Else"})
	if got := s.author.String(); got != "Someone Else" {
		t.Errorf("the author is %q, want what was pasted", got)
	}
}

// The real backend, in a folder that has no pack, with the versions of Minecraft and the loader that a test gives it.
func TestInitMakesARealPackThatTheTUICanThenOpen(t *testing.T) {
	cmdtest.Chdir(t)
	cmdtest.SetColor(t, ui.Never)
	if err := os.MkdirAll("config", 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}
	if err := os.WriteFile("config/options.json", []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	f := newFakeInit()
	real := realCreate{fakeInit: f}
	a := newApp("", newInitScreen(real))
	a.opened = func() (string, []screen, error) {
		data, err := packBackend{}.load()
		if err != nil {
			return "", nil, err
		}
		return data.pack, newScreens(packBackend{}, data), nil
	}
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	appFeed(t, a, a.Init()())
	if a.screen.title() != "New pack" {
		t.Fatalf("the app opened on %s, want it to ask for a pack, as there is none", a.screen.title())
	}

	out := cmdtest.CaptureStdout(t, func() { appPress(t, a, "ctrl+s") })
	if out != "" {
		t.Errorf("making the pack wrote %q to the terminal", out)
	}
	if a.screen.title() != "Overview" || len(a.screens) < 4 {
		t.Fatalf("the app is on %s with %d screens, want it opened on the pack that was made", a.screen.title(), len(a.screens))
	}
	if header := lines(a.render())[0]; header != "packwiz  Demo Pack 1.0.0" {
		t.Errorf("the header is %q, want the pack that was made", header)
	}
	pack, err := core.LoadPack()
	if err != nil || pack.Versions["neoforge"] != "21.1.100" || pack.Versions["minecraft"] != "1.21.1" {
		t.Errorf("the pack is %+v (%v), want what was chosen", pack, err)
	}
	// What was in the folder is in the pack's index
	if out := a.render(); !strings.Contains(out, "tracking 1 file") {
		t.Errorf("the overview doesn't say the index tracks the config file that was there:\n%s", out)
	}
}

// realCreate is a fakeInit that really makes the pack, which is what a test that follows the TUI into it needs.
type realCreate struct{ *fakeInit }

func (r realCreate) createPack(pack cmd.NewPack) error { return cmd.CreatePack(pack) }

func TestAppOpenFailureIsSaidOnTheScreenThatMadeThePack(t *testing.T) {
	cmdtest.SetColor(t, ui.Never)
	cmdtest.Chdir(t)
	a := newApp("", newInitScreen(newFakeInit()))
	a.opened = func() (string, []screen, error) { return "", nil, errors.New("pack.toml is unreadable") }
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	appFeed(t, a, packCreatedMsg{})
	if a.screen.title() != "New pack" {
		t.Errorf("the app is on %s, want it to stay on the screen it was on", a.screen.title())
	}
	if out := a.render(); !strings.Contains(out, "the pack was made, but couldn't be opened: pack.toml is unreadable") {
		t.Errorf("the screen doesn't say the pack couldn't be opened:\n%s", out)
	}
}

func TestAppIgnoresAPackBeingMadeWhenItHasNoWayToOpenOne(t *testing.T) {
	a := newNavigableApp(t, 100, 30)
	before := a.screen.title()
	appFeed(t, a, packCreatedMsg{})
	if a.screen.title() != before {
		t.Errorf("the app went to %s, want it to stay on %s", a.screen.title(), before)
	}
}
