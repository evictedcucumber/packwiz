package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCategories(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileCategoriesFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFileCategoriesSortFilesByPattern(t *testing.T) {
	dir := writeCategories(t, `
[categories]
dev = ["flake.nix", "flake.lock", "*.yml", "nix/"]
docs = ["README.md", "!docs/draft.md", "docs/"]
`)
	categories, err := LoadFileCategories(dir)
	if err != nil {
		t.Fatalf("LoadFileCategories() returned error: %v", err)
	}
	for path, want := range map[string]string{
		"flake.nix":      "dev",
		"lefthook.yml":   "dev",
		"nix/shell.nix":  "dev",
		"README.md":      "docs",
		"docs/guide.md":  "docs",
		"sub/flake.lock": "dev", // a pattern without a slash matches in any folder, as in .gitignore
		"./flake.nix":    "dev",
	} {
		if got, ok := categories.Of(path); !ok || got != want {
			t.Errorf("Of(%q) = %q, %v, want %q", path, got, ok, want)
		}
	}
	for _, path := range []string{"options.txt", "config/a.json", "docs2.md"} {
		if got, ok := categories.Of(path); ok {
			t.Errorf("Of(%q) = %q, want no category", path, got)
		}
	}
}

func TestAFileInTwoCategoriesIsInTheFirstByName(t *testing.T) {
	categories, err := LoadFileCategories(writeCategories(t, "[categories]\nmisc = [\"a.txt\"]\nbuild = [\"a.txt\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := categories.Of("a.txt"); got != "build" {
		t.Errorf("Of() = %q, want build", got)
	}
}

func TestNoFileCategoriesWithoutTheFile(t *testing.T) {
	categories, err := LoadFileCategories(t.TempDir())
	if err != nil {
		t.Fatalf("LoadFileCategories() returned error: %v", err)
	}
	if got, ok := categories.Of("flake.nix"); ok {
		t.Errorf("Of() = %q, want no category", got)
	}
}

func TestFileCategoriesAcceptEveryPredefinedCategoryAndNoOther(t *testing.T) {
	for _, c := range FileCategoryList {
		if _, err := LoadFileCategories(writeCategories(t, "[categories]\n"+c.Name+" = [\"a\"]\n")); err != nil {
			t.Errorf("LoadFileCategories() with category %q returned error: %v", c.Name, err)
		}
		if c.Description == "" {
			t.Errorf("category %q has no description, which the manual shows", c.Name)
		}
	}
	for _, name := range []string{"Dev", "my dev", "tools", "dev_files", "config", "mods", ""} {
		_, err := LoadFileCategories(writeCategories(t, "[categories]\n\""+name+"\" = [\"a\"]\n"))
		if err == nil || !strings.Contains(err.Error(), FileCategoriesFile) || !strings.Contains(err.Error(), "dev, docs, ci") {
			t.Errorf("LoadFileCategories() with category %q = %v, want an error naming the file and the categories there are", name, err)
		}
	}
	if _, err := LoadFileCategories(writeCategories(t, "[categories\n")); err == nil || !strings.Contains(err.Error(), FileCategoriesFile) {
		t.Errorf("LoadFileCategories() with invalid TOML = %v, want an error naming the file", err)
	}
}

func TestTheDefaultCategoriesAreValidAndHaveEveryCategory(t *testing.T) {
	categories, err := LoadFileCategories(writeCategories(t, DefaultFileCategories))
	if err != nil {
		t.Fatalf("the default %s doesn't load: %v", FileCategoriesFile, err)
	}
	for _, name := range FileCategoryNames() {
		if !strings.Contains(DefaultFileCategories, "\n"+name+" = [") {
			t.Errorf("the default %s has no %q", FileCategoriesFile, name)
		}
	}
	for path, want := range map[string]string{"flake.nix": "dev", "README.md": "docs", ".github/workflows/ci.yml": "ci", "scripts/build.sh": "build", "icon.png": "assets"} {
		if got, ok := categories.Of(path); !ok || got != want {
			t.Errorf("Of(%q) = %q, %v, want %q", path, got, ok, want)
		}
	}
}

func TestTheReadmeAndLicenseAreDocsWithoutBeingListedAnywhere(t *testing.T) {
	// With no file of categories at all, and with one that doesn't mention them
	for name, dir := range map[string]string{
		"no file":      t.TempDir(),
		"another file": writeCategories(t, "[categories]\ndev = [\"flake.nix\"]\n"),
	} {
		categories, err := LoadFileCategories(dir)
		if err != nil {
			t.Fatalf("%s: LoadFileCategories() returned error: %v", name, err)
		}
		for _, path := range []string{"README.md", "LICENSE", "./README.md"} {
			if got, ok := categories.Of(path); !ok || got != "docs" {
				t.Errorf("%s: Of(%q) = %q, %v, want docs", name, path, got, ok)
			}
		}
		// Only the pack's own: one in a folder is some other file, as it isn't what is exported
		for _, path := range []string{"sub/README.md", "docs/LICENSE", "README.txt", "LICENSE.md", "readme.md"} {
			if got, ok := categories.Of(path); ok {
				t.Errorf("%s: Of(%q) = %q, want no category", name, path, got)
			}
		}
	}
}

func TestTheReadmeIsInTheCategoryThePackPutsItIn(t *testing.T) {
	categories, err := LoadFileCategories(writeCategories(t, "[categories]\nmisc = [\"README.md\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := categories.Of("README.md"); got != "misc" {
		t.Errorf("Of(README.md) = %q, want misc, as the pack's file says", got)
	}
	if got, _ := categories.Of("LICENSE"); got != "docs" {
		t.Errorf("Of(LICENSE) = %q, want docs, as the pack's file doesn't say", got)
	}
}

func TestTheFileOfCategoriesPackwizWritesParses(t *testing.T) {
	categories, err := LoadFileCategories(writeCategories(t, DefaultFileCategories))
	if err != nil {
		t.Fatalf("LoadFileCategories() returned error: %v", err)
	}
	for path, want := range map[string]string{"docs/guide.md": "docs", "README.md": "docs", "flake.nix": "dev"} {
		if got, _ := categories.Of(path); got != want {
			t.Errorf("Of(%q) = %q, want %q", path, got, want)
		}
	}
}
