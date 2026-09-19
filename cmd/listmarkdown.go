package cmd

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/viper"
)

// markdownEntry is a file in the pack that goes in the markdown list
type markdownEntry struct {
	name string
	// path is its metadata file, in forward slash format relative to the pack's folder
	path string
}

// markdownSections are the sections of the list for the folders that packwiz puts files in, in the order they are
// listed in. Files in any other folder get a section of their own, named after the folder.
var markdownSections = []struct{ folder, title string }{
	{"mods", "Mods"},
	{"resourcepacks", "Resource Packs"},
	{"shaderpacks", "Shader Packs"},
	{"datapacks", "Data Packs"},
}

// otherSection is where a file that isn't in a folder goes, after all the others
const otherSection = "Other"

// sectionOf returns the title of the section that a metadata file is listed under, going by its folders: the first one
// that packwiz puts a kind of file in, so that a subfolder of mods/, or a pack that keeps everything in a folder of its
// own (--meta-folder-base), is still listed with the mods
func sectionOf(file string) string {
	dir := path.Dir(file)
	if dir == "." {
		return otherSection
	}
	folders := strings.Split(dir, "/")
	for _, folder := range folders {
		for _, section := range markdownSections {
			if strings.EqualFold(folder, section.folder) {
				return section.title
			}
		}
	}
	name := folders[len(folders)-1]
	r, size := utf8.DecodeRuneInString(name)
	return string(unicode.ToUpper(r)) + name[size:]
}

// compareSections orders section titles as they are listed: the ones for the folders packwiz knows, then any others by
// name, then "Other"
func compareSections(a, b string) int {
	rank := func(title string) int {
		if title == otherSection {
			return len(markdownSections) + 1
		}
		for i, section := range markdownSections {
			if section.title == title {
				return i
			}
		}
		return len(markdownSections)
	}
	if c := cmp.Compare(rank(a), rank(b)); c != 0 {
		return c
	}
	return strings.Compare(strings.ToLower(a), strings.ToLower(b))
}

// renderMarkdownList makes the markdown list of what is in the pack: its name and description, then the names of its
// files under a heading for each kind, in alphabetical order. That is all it says of them: no versions, and nothing of
// what was added as a dependency.
func renderMarkdownList(pack core.Pack, entries []markdownEntry) string {
	sections := make(map[string][]markdownEntry)
	for _, e := range entries {
		title := sectionOf(e.path)
		sections[title] = append(sections[title], e)
	}

	var b strings.Builder
	heading := pack.Name
	if strings.TrimSpace(heading) == "" {
		heading = "Modpack"
	}
	b.WriteString("# " + escapeMarkdown(heading) + "\n")
	if description := escapeMarkdown(pack.Description); description != "" {
		b.WriteString("\n" + description + "\n")
	}

	for _, title := range slices.SortedFunc(maps.Keys(sections), compareSections) {
		b.WriteString("\n## " + escapeMarkdown(title) + "\n\n")
		files := sections[title]
		slices.SortFunc(files, func(a, b markdownEntry) int {
			if c := strings.Compare(strings.ToLower(entryName(a)), strings.ToLower(entryName(b))); c != 0 {
				return c
			}
			// So that the same pack always gives the same file
			return strings.Compare(a.path, b.path)
		})
		for _, e := range files {
			b.WriteString("- " + escapeMarkdown(entryName(e)) + "\n")
		}
	}
	if len(entries) == 0 {
		b.WriteString("\n_Nothing to list._\n")
	}
	return b.String()
}

// entryName is how a file is listed: by its name, or by the name of its metadata file if it has none
func entryName(e markdownEntry) string {
	if strings.TrimSpace(e.name) != "" {
		return e.name
	}
	return strings.TrimSuffix(path.Base(e.path), ".pw.toml")
}

var (
	// Characters that mean something in markdown wherever they are in a line
	markdownSpecials = regexp.MustCompile("[\\\\`*_~\\[\\]<>]")
	// An ampersand that would be read as the start of a character reference, like &amp;
	markdownReference = regexp.MustCompile(`&([#A-Za-z0-9]+;)`)
	// What would make a line of text a numbered list item, or a heading or a bullet (a quote's > is in markdownSpecials)
	markdownNumbered = regexp.MustCompile(`^(\d+)([.)])`)
	markdownBlock    = regexp.MustCompile(`^[#+-]`)
)

// escapeMarkdown makes text that is on a line of its own (a name, a description) show as it is written rather than as
// markdown, on one line. Only what would change how it looks is escaped, so an ordinary name is left as it is.
func escapeMarkdown(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	text = markdownSpecials.ReplaceAllString(text, `\$0`)
	text = markdownReference.ReplaceAllString(text, `\&$1`)
	text = markdownNumbered.ReplaceAllString(text, `$1\$2`)
	return markdownBlock.ReplaceAllString(text, `\$0`)
}

// markdownListPath is where the markdown list is written: where --output says, or else in the pack's folder
func markdownListPath() string {
	if out := viper.GetString("list.output"); out != "" {
		return out
	}
	return filepath.Join(filepath.Dir(viper.GetString("pack-file")), core.ModListFile)
}

// writeMarkdownList makes the markdown list of the mods and writes it to dest, or to stdout if that is "-"
func writeMarkdownList(dest string, pack core.Pack, index core.Index, mods []*core.Mod) error {
	entries := make([]markdownEntry, len(mods))
	for i, mod := range mods {
		file, err := index.RelIndexPath(mod.GetFilePath())
		if err != nil {
			return err
		}
		entries[i] = markdownEntry{name: mod.Name, path: file}
	}
	text := renderMarkdownList(pack, entries)

	if dest == "-" {
		fmt.Print(text)
		return nil
	}
	if err := os.WriteFile(dest, []byte(text), 0o644); err != nil {
		return err
	}
	ui.Success.Printf("Wrote %s\n", dest)
	return nil
}
