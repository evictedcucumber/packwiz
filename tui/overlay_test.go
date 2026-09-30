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
	p := newPicker([]string{"options.txt"}, []*core.Mod{fakeMod("Alpha", "alpha")}, "")
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
	mods := []*core.Mod{fakeMod("Alpha", "alpha"), fakeMod("Beta", "beta"), fakeMod("Gamma", "gamma")}
	p := newPicker([]string{"config/a.json"}, mods, "")
	p.setSize(80, 20)
	// Chosen from the bottom up
	p.update(keyMsg(t, "j"))
	p.update(keyMsg(t, "j"))
	p.update(keyMsg(t, "space"))
	p.update(keyMsg(t, "k"))
	p.update(keyMsg(t, "k"))
	p.update(keyMsg(t, "space"))

	paths, names, entries := p.result()
	if !slices.Equal(paths, []string{"mods/alpha.pw.toml", "mods/gamma.pw.toml"}) || !slices.Equal(names, []string{"Alpha", "Gamma"}) {
		t.Errorf("result() = %v, %v", paths, names)
	}
	if !slices.Equal(entries, []string{"config/a.json"}) {
		t.Errorf("entries = %v", entries)
	}
}

func TestPickerSelectionSurvivesFiltering(t *testing.T) {
	mods := []*core.Mod{fakeMod("Alpha", "alpha"), fakeMod("Beta", "beta")}
	p := newPicker([]string{"config/a.json"}, mods, "")
	p.setSize(80, 20)
	p.update(keyMsg(t, "space")) // Alpha
	p.update(keyMsg(t, "/"))
	for _, r := range "beta" {
		p.update(keyMsg(t, string(r)))
	}
	p.update(keyMsg(t, "enter"))
	p.update(keyMsg(t, "space")) // Beta
	_, names, _ := p.result()
	if !slices.Equal(names, []string{"Alpha", "Beta"}) {
		t.Errorf("the mods chosen are %v, want the one chosen before filtering as well", names)
	}
}

func TestClaimsAll(t *testing.T) {
	bare := fakeMod("Bare", "bare")
	some := fakeMod("Some", "some", "config/a.json")
	both := fakeMod("Both", "both", "config/a.json", "config/b.json", "config/c/")
	entries := []string{"config/a.json", "config/b.json"}
	for _, tt := range []struct {
		mod  *core.Mod
		want bool
	}{{bare, false}, {some, false}, {both, true}} {
		if got := claimsAll(tt.mod, entries); got != tt.want {
			t.Errorf("claimsAll(%s) = %v, want %v", tt.mod.Name, got, tt.want)
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
