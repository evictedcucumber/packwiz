package core

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	gitignore "github.com/sabhiram/go-gitignore"
)

// FileCategoriesFile is the file in the pack's folder that sorts the files packwiz doesn't track, such as flake.nix, into
// categories. A file in a category is committed on its own by packwiz commit, as a
// chore in the scope of its category, instead of being left alone as one packwiz doesn't recognise. Like .packwizignore,
// it isn't tracked by the index, so it isn't distributed with the pack.
//
//	[categories]
//	dev = ["flake.nix", "flake.lock", ".envrc"]
//	docs = ["docs/"]
//
// A category is one of FileCategoryList, whose name is the scope of its commits, and the patterns of the files in it,
// written like those of .gitignore and relative to the pack's folder. The pack's README.md and LICENSE are in docs without
// being listed (see builtinDocs).
const FileCategoriesFile = ".packwizfiles.toml"

// FileCategory is a kind of file that isn't part of a pack but lives alongside it.
type FileCategory struct {
	// Name is what the category is called in FileCategoriesFile, and the scope of the commits for its files.
	Name string
	// Description says what goes in it, for people choosing one.
	Description string
}

// FileCategoryList are the categories a file can be put in, which are the only ones: a commit's scope says what kind of
// change it is, so it comes from a fixed set that the changelog and anyone reading the log can rely on. The manual for
// "packwiz commit" lists them, from this.
var FileCategoryList = []FileCategory{
	{"dev", "development tooling: flake.nix, lefthook.yml, .envrc"},
	{"docs", "documentation: README.md, LICENSE, docs/"},
	{"ci", "continuous integration: .github/workflows/, .gitlab-ci.yml"},
	{"build", "building and packaging: scripts, Makefile, Dockerfile"},
	{"assets", "images and media: icon.png, screenshots/"},
	{"misc", "anything else kept with the pack"},
}

// FileCategoryNames are the names of FileCategoryList, in that order.
func FileCategoryNames() []string {
	names := make([]string, len(FileCategoryList))
	for i, c := range FileCategoryList {
		names[i] = c.Name
	}
	return names
}

// DefaultFileCategories is the FileCategoriesFile that "packwiz init" writes: every category, with the files that usually
// belong in it, for the pack's author to change.
const DefaultFileCategories = `# Sorts the files in this folder that packwiz doesn't track into categories, so that
# "packwiz commit" commits each one on its own, as "chore(dev): change flake.nix".
# The categories are dev, docs, ci, build, assets and misc (see "man packwiz-git-commit").
# Patterns are written like those of .gitignore, relative to this folder. Files in no
# category are left uncommitted, and "packwiz changelog release" won't release while
# they have changes. README.md and LICENSE are always docs, unless you put them in a
# category here, so they needn't be listed.
[categories]
dev = ["flake.nix", "flake.lock", "lefthook.yml", ".envrc", ".editorconfig", ".gitignore", ".gitattributes"]
docs = ["docs/"]
ci = [".github/", ".gitlab-ci.yml"]
build = ["Makefile", "Dockerfile", "scripts/"]
assets = ["icon.png", "screenshots/"]
misc = []
`

// FileCategories sorts files into the categories of a pack's FileCategoriesFile.
type FileCategories struct {
	names    []string
	patterns map[string]*gitignore.GitIgnore
}

type fileCategoriesFile struct {
	Categories map[string][]string `toml:"categories"`
}

// LoadFileCategories reads the categories of the pack whose folder is packRoot. A pack with no such file has none.
func LoadFileCategories(packRoot string) (FileCategories, error) {
	path := filepath.Join(packRoot, FileCategoriesFile)
	var file fileCategoriesFile
	if _, err := toml.DecodeFile(path, &file); err != nil {
		if os.IsNotExist(err) {
			return FileCategories{}, nil
		}
		return FileCategories{}, fmt.Errorf("failed to read %s: %w", FileCategoriesFile, err)
	}
	categories := FileCategories{patterns: make(map[string]*gitignore.GitIgnore, len(file.Categories))}
	// In name order, so which category a file in two of them is in doesn't depend on the order of a map
	for _, name := range slices.Sorted(maps.Keys(file.Categories)) {
		if !slices.Contains(FileCategoryNames(), name) {
			return FileCategories{}, fmt.Errorf("%s: %q isn't a category; they are %s", FileCategoriesFile, name, strings.Join(FileCategoryNames(), ", "))
		}
		categories.names = append(categories.names, name)
		categories.patterns[name] = gitignore.CompileIgnoreLines(file.Categories[name]...)
	}
	return categories, nil
}

// builtinDocs are the files in the pack's folder that are documentation whether or not the pack's FileCategoriesFile says
// so (or there is one), so a pack's README and licence are committed as "chore(docs)" without being listed anywhere. They
// are DocFiles but for the changelog, which packwiz writes itself and so already recognises. A category the pack gives
// them in its own file wins.
var builtinDocs = []string{"README.md", "LICENSE"}

// Of is the category of a file, given by its path relative to the pack's folder, or false if it is in none. A file that
// is in more than one is in the first of them by name. The README and licence in the pack's folder are in docs unless the
// pack's file puts them elsewhere.
func (c FileCategories) Of(path string) (string, bool) {
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	for _, name := range c.names {
		if c.patterns[name].MatchesPath(path) {
			return name, true
		}
	}
	if slices.Contains(builtinDocs, path) {
		return "docs", true
	}
	return "", false
}
