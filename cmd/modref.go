package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// modRefHelp says how a mod is given to the commands that take one, for their help.
const modRefHelp = `The mod is given by its slug (the name of its .pw.toml file without the extension), that file name, or a path to it. If
that is no mod's, what was given is searched for in the names and slugs of the mods, fuzzily as fzf does, so a part of a
name is enough ("sdm" is Sodium, and "sod ex" is Sodium Extra): if exactly one mod matches it is used, and you are told
which, and if several do none is, and they are listed. A path that isn't there is never searched for.`

// maxCandidates is how many mods an error lists when what was typed matches several, before saying how many more
const maxCandidates = 5

// resolveMod finds the mod that a command was given, as "packwiz mr pin" and the others that take one are: by its slug, its
// .pw.toml file name or a path to that file, as Index.FindMod does. A mod that is found that way is always the one.
//
// If there is none, what was typed is searched for, fuzzily, in the names and slugs of the mods (see
// Index.SearchMods), so that a part of a name is enough: "sdm" is Sodium, and "sod ex" is Sodium Extra. If it matches
// exactly one mod that is the mod, and matched says so, for the command to tell whoever typed it which was meant. If it
// matches several none is chosen, as acting on the wrong mod is worse than asking again, and the error lists them.
//
// A reference that has a folder in it is a path, and a path that isn't there isn't a name to look for, so it is never
// searched for.
func resolveMod(index core.Index, ref string) (path string, matched *core.ModMatch, err error) {
	if path, ok := index.FindMod(ref); ok {
		return path, nil, nil
	}
	notFound := fmt.Errorf("Can't find %q; please ensure you have run packwiz refresh and specify its slug, its .pw.toml file name, or a path to that file", ref)
	if strings.TrimSpace(ref) == "" || strings.ContainsAny(ref, `/\`) {
		return "", nil, notFound
	}

	// Its file name with its extension is also what someone might shorten
	matches, err := index.SearchMods(strings.TrimSuffix(strings.TrimSuffix(ref, core.MetaExtension), core.MetaExtensionOld))
	if err != nil {
		return "", nil, err
	}
	switch len(matches) {
	case 0:
		return "", nil, notFound
	case 1:
		return matches[0].Path, &matches[0], nil
	}

	candidates := make([]string, 0, maxCandidates+1)
	for i, m := range matches {
		if i == maxCandidates {
			candidates = append(candidates, fmt.Sprintf("and %d more", len(matches)-maxCandidates))
			break
		}
		candidates = append(candidates, fmt.Sprintf("%s (%s)", m.Name, m.Slug))
	}
	return "", nil, fmt.Errorf("%q matches several mods, and it isn't clear which is meant: %s; specify its slug, its .pw.toml file name, or a path to that file", ref, strings.Join(candidates, ", "))
}

// findMod is resolveMod for a command that goes on with the mod it finds: it says which mod a part of a name was taken
// for, and if there is none ends the command with the reason.
func findMod(index core.Index, ref string) string {
	path, matched, err := resolveMod(index, ref)
	if err != nil {
		ui.Error.Println(err)
		os.Exit(1)
	}
	if matched != nil {
		ui.Info.Printf("%s isn't the name of a mod, so using %s (%s), which it matches\n", ui.Bold.Sprint(ref), ui.Bold.Sprint(matched.Name), matched.Slug)
	}
	return path
}
