package tui

import (
	"slices"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

func TestNewPickerWorksOutWhatWouldBeClaimed(t *testing.T) {
	tests := []struct {
		name         string
		subjects     []string
		configDir    string
		files        []string
		folders      []string
		canChooseDir bool
	}{
		{
			name:         "a file in a folder",
			subjects:     []string{"config/sodium/options.json"},
			files:        []string{"config/sodium/options.json"},
			folders:      []string{"config/sodium/"},
			canChooseDir: true,
		},
		{
			name:         "files in one folder are claimed as it once",
			subjects:     []string{"config/sodium/a.json", "config/sodium/b.json", "config/other.json"},
			files:        []string{"config/sodium/a.json", "config/sodium/b.json", "config/other.json"},
			folders:      []string{"config/sodium/", "config/"},
			canChooseDir: true,
		},
		{
			name:     "a file that isn't in a folder is claimed itself, so there is nothing to choose",
			subjects: []string{"options.txt"},
			files:    []string{"options.txt"},
			folders:  []string{"options.txt"},
		},
		{
			name:         "some of them not in a folder",
			subjects:     []string{"options.txt", "config/a.json"},
			files:        []string{"options.txt", "config/a.json"},
			folders:      []string{"options.txt", "config/"},
			canChooseDir: true,
		},
		{
			name:         "entries are written without the config dir",
			subjects:     []string{"configureddefaults/config/sodium/a.json"},
			configDir:    "configureddefaults",
			files:        []string{"config/sodium/a.json"},
			folders:      []string{"config/sodium/"},
			canChooseDir: true,
		},
		{
			// The config dir itself can't be claimed, as no entry is written for it
			name:      "a file directly in the config dir is claimed itself",
			subjects:  []string{"configureddefaults/options.txt"},
			configDir: "configureddefaults",
			files:     []string{"options.txt"},
			folders:   []string{"options.txt"},
		},
		{
			name:     "the same file twice is claimed once",
			subjects: []string{"config/a.json", "config/a.json"},
			files:    []string{"config/a.json"},
			folders:  []string{"config/"},
			// The folder is another entry than the file, so it can be chosen
			canChooseDir: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPicker(tt.subjects, nil, tt.configDir)
			if !slices.Equal(p.files, tt.files) {
				t.Errorf("files = %v, want %v", p.files, tt.files)
			}
			if !slices.Equal(p.folders, tt.folders) {
				t.Errorf("folders = %v, want %v", p.folders, tt.folders)
			}
			if p.canChooseScope() != tt.canChooseDir {
				t.Errorf("canChooseScope() = %v, want %v", p.canChooseScope(), tt.canChooseDir)
			}
		})
	}
}

func TestPickerTabDoesNothingWhenThereIsNoFolderToChoose(t *testing.T) {
	p := newPicker([]string{"options.txt"}, []owner{fakeOwner("Alpha", "alpha")}, "")
	p.setSize(80, 20)
	p.update(keyMsg(t, "tab"))
	if p.folder {
		t.Error("tab switched to folders when the file has none")
	}
	if !slices.Equal(p.entries(), []string{"options.txt"}) {
		t.Errorf("entries() = %v", p.entries())
	}
	for _, b := range p.keys() {
		if b.Help().Key == "tab" {
			t.Error("tab is listed as a key when it does nothing")
		}
	}
}

func TestPickerResultIsInTheOrderOfTheList(t *testing.T) {
	mods := []owner{fakeOwner("Alpha", "alpha"), fakeOwner("Beta", "beta"), fakeOwner("Gamma", "gamma")}
	p := newPicker([]string{"config/a.json"}, mods, "")
	p.setSize(80, 20)
	// Chosen from the bottom up
	p.update(keyMsg(t, "j"))
	p.update(keyMsg(t, "j"))
	p.update(keyMsg(t, "space"))
	p.update(keyMsg(t, "k"))
	p.update(keyMsg(t, "k"))
	p.update(keyMsg(t, "space"))

	owners, entries := p.result()
	if got := namesOf(owners); !slices.Equal(got, []string{"Alpha", "Gamma"}) {
		t.Errorf("result() = %v", got)
	}
	if !slices.Equal(entries, []string{"config/a.json"}) {
		t.Errorf("entries = %v", entries)
	}
}

func TestPickerSelectionSurvivesFiltering(t *testing.T) {
	mods := []owner{fakeOwner("Alpha", "alpha"), fakeOwner("Beta", "beta")}
	p := newPicker([]string{"config/a.json"}, mods, "")
	p.setSize(80, 20)
	p.update(keyMsg(t, "space")) // Alpha
	p.update(keyMsg(t, "/"))
	for _, r := range "beta" {
		p.update(keyMsg(t, string(r)))
	}
	p.update(keyMsg(t, "enter"))
	p.update(keyMsg(t, "space")) // Beta
	owners, _ := p.result()
	if got := namesOf(owners); !slices.Equal(got, []string{"Alpha", "Beta"}) {
		t.Errorf("the mods chosen are %v, want the one chosen before filtering as well", got)
	}
}

func TestClaimsAll(t *testing.T) {
	bare := fakeOwner("Bare", "bare")
	some := fakeOwner("Some", "some", "config/a.json")
	both := fakeOwner("Both", "both", "config/a.json", "config/b.json", "config/c/")
	entries := []string{"config/a.json", "config/b.json"}
	for _, tt := range []struct {
		owner owner
		want  bool
	}{{bare, false}, {some, false}, {both, true}} {
		if got := claimsAll(tt.owner, entries); got != tt.want {
			t.Errorf("claimsAll(%s) = %v, want %v", tt.owner.name, got, tt.want)
		}
	}
}

func TestSlugOfIsTheNameOfTheMetadataFileWithoutItsExtension(t *testing.T) {
	m := &core.Mod{}
	m.SetMetaPath("mods/sodium-extra.pw.toml")
	if got := slugOf(m); got != "sodium-extra" {
		t.Errorf("slugOf() = %q", got)
	}
	m.SetMetaPath("mods/legacy.toml")
	if got := slugOf(m); got != "legacy" {
		t.Errorf("slugOf() of a file with the old extension = %q", got)
	}
}

// pickerFor makes a picker of mods with these names, which have slugs of their own, with a filter typed into it.
func pickerFor(filter string, owners ...owner) *picker {
	p := newPicker([]string{"config/a.json"}, owners, "")
	p.setSize(100, 30)
	p.filter.insert(filter)
	p.refilter()
	return p
}

func visibleNames(p *picker) []string {
	names := make([]string, len(p.visible))
	for i, e := range p.visible {
		names[i] = e.owner.name
	}
	return names
}

func TestPickerFilterIsFuzzy(t *testing.T) {
	mods := []owner{fakeOwner("Alpha Mod", "alpha"), fakeOwner("Beta Mod", "beta"), fakeOwner("Sodium", "sodium"), fakeOwner("Sodium Extra", "sodium-extra")}

	// Characters found in order, not together
	if got := visibleNames(pickerFor("sdm", mods...)); !slices.Equal(got, []string{"Sodium", "Sodium Extra"}) {
		t.Errorf("sdm finds %v, want the two Sodiums and not the mods that say Mod", got)
	}
	if got := visibleNames(pickerFor("btmd", mods...)); !slices.Equal(got, []string{"Beta Mod"}) {
		t.Errorf("btmd finds %v, want Beta Mod", got)
	}
	// The case typed doesn't matter
	if got := visibleNames(pickerFor("SODX", mods...)); !slices.Equal(got, []string{"Sodium Extra"}) {
		t.Errorf("SODX finds %v, want Sodium Extra", got)
	}
	// Nothing typed is every mod, as they are
	if got := visibleNames(pickerFor("", mods...)); !slices.Equal(got, []string{"Alpha Mod", "Beta Mod", "Sodium", "Sodium Extra"}) {
		t.Errorf("no filter shows %v, want every mod in order", got)
	}
	if got := visibleNames(pickerFor("   ", mods...)); len(got) != 4 {
		t.Errorf("a filter of spaces shows %v, want every mod", got)
	}
}

func TestPickerFilterNeedsEveryWordToMatchInAnyOrder(t *testing.T) {
	mods := []owner{fakeOwner("Sodium", "sodium"), fakeOwner("Sodium Extra", "sodium-extra"), fakeOwner("Lexicon", "lexicon")}
	if got := visibleNames(pickerFor("extra sod", mods...)); !slices.Equal(got, []string{"Sodium Extra"}) {
		t.Errorf("'extra sod' finds %v, want Sodium Extra: both words, in either order", got)
	}
	if got := visibleNames(pickerFor("sod ex", mods...)); !slices.Equal(got, []string{"Sodium Extra"}) {
		t.Errorf("'sod ex' finds %v, want Sodium Extra, not Lexicon (which has no sod) nor Sodium (no ex)", got)
	}
}

// The best match is first, so that enter takes it, rather than the order that the mods are in
func TestPickerFilterPutsTheBestMatchFirst(t *testing.T) {
	// Alphabetical order has Amod first, but mod is the start of a word in the other
	mods := []owner{fakeOwner("Amod", "amod"), fakeOwner("Zed Mod", "zed")}
	if got := visibleNames(pickerFor("mod", mods...)); !slices.Equal(got, []string{"Zed Mod", "Amod"}) {
		t.Errorf("mod finds %v, want the match at the start of a word first", got)
	}

	// Matches that are as good are in the order of the shorter name, as fzf has it, though the longer is first in the list
	mods = []owner{fakeOwner("A Sodium Plus", "a"), fakeOwner("B Sodium", "b")}
	if got := visibleNames(pickerFor("sodium", mods...)); !slices.Equal(got, []string{"B Sodium", "A Sodium Plus"}) {
		t.Errorf("sodium finds %v, want the shorter name first when the matches are as good", got)
	}

	// And of those that are as good and as long, the order they are in
	mods = []owner{fakeOwner("Sodium A", "a"), fakeOwner("Sodium B", "b")}
	if got := visibleNames(pickerFor("sodium", mods...)); !slices.Equal(got, []string{"Sodium A", "Sodium B"}) {
		t.Errorf("sodium finds %v, want them in their order", got)
	}
}

func TestPickerFilterMatchesTheSlugWhenTheNameDoesNot(t *testing.T) {
	mods := []owner{fakeOwner("Plain", "zzz-long-slug"), fakeOwner("Other", "other")}
	p := pickerFor("longslug", mods...)
	if got := visibleNames(p); !slices.Equal(got, []string{"Plain"}) {
		t.Fatalf("longslug finds %v, want the mod with that slug", got)
	}
	if e := p.visible[0]; len(e.namePositions) != 0 || len(e.slugPositions) == 0 {
		t.Errorf("the match has name positions %v and slug positions %v, want it in the slug", e.namePositions, e.slugPositions)
	}

	// A word that the name has is matched there, and one it hasn't in the slug, and both are needed
	p = pickerFor("plain slug", mods...)
	if got := visibleNames(p); !slices.Equal(got, []string{"Plain"}) {
		t.Errorf("'plain slug' finds %v, want Plain: one word in its name and one in its slug", got)
	}
	if e := p.visible[0]; len(e.namePositions) == 0 || len(e.slugPositions) == 0 {
		t.Errorf("the match has name positions %v and slug positions %v, want both", e.namePositions, e.slugPositions)
	}
}

func TestPickerEnterTakesTheBestMatch(t *testing.T) {
	mods := []owner{fakeOwner("Amod", "amod"), fakeOwner("Zed Mod", "zed")}
	p := newPicker([]string{"config/a.json"}, mods, "")
	p.setSize(100, 30)
	p.update(keyMsg(t, "/"))
	for _, r := range "mod" {
		p.update(keyMsg(t, string(r)))
	}
	p.update(keyMsg(t, "enter")) // done typing
	if _, result := p.update(keyMsg(t, "enter")); result != overlayConfirmed {
		t.Fatalf("enter didn't choose, the result is %v", result)
	}
	if owners, _ := p.result(); !slices.Equal(namesOf(owners), []string{"Zed Mod"}) {
		t.Errorf("enter chose %v, want the best match, which is first, rather than the first mod", namesOf(owners))
	}
}

func namesOf(owners []owner) []string {
	names := make([]string, len(owners))
	for i, o := range owners {
		names[i] = o.name
	}
	return names
}
