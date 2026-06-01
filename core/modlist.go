package core

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/viper"
)

const ModListFileName = "modlist.md"

func (in Index) GenerateModListMarkdown() (string, error) {
	return in.GenerateModListMarkdownWithSide(UniversalSide)
}

func (in Index) GenerateModListMarkdownWithSide(side string) (string, error) {
	allMods, err := in.LoadAllMods()
	if err != nil {
		return "", err
	}

	// Separate dependency mods from regular mods
	var deps []*Mod
	var mods []*Mod
	for _, m := range allMods {
		if m.Option != nil && m.Option.Dependency {
			deps = append(deps, m)
			continue
		}
		mods = append(mods, m)
	}

	// Filter regular mods by side (dependencies are listed separately regardless of side)
	if side != UniversalSide {
		i := 0
		for _, mod := range mods {
			if mod.Side == side || mod.Side == UniversalSide {
				mods[i] = mod
				i++
			}
		}
		mods = mods[:i]
	}

	// Filter dependencies by side as well when a specific side is requested
	if side != UniversalSide {
		j := 0
		for _, d := range deps {
			if d.Side == side || d.Side == UniversalSide {
				deps[j] = d
				j++
			}
		}
		deps = deps[:j]
	}

	slices.SortFunc(mods, func(a, b *Mod) int {
		categoryCompare := strings.Compare(modCategory(in, a), modCategory(in, b))
		if categoryCompare != 0 {
			return categoryCompare
		}
		nameCompare := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		if nameCompare != 0 {
			return nameCompare
		}
		return strings.Compare(a.GetFilePath(), b.GetFilePath())
	})

	var out strings.Builder
	out.WriteString("# Mods List\n\n")
	if len(mods) == 0 {
		out.WriteString("No mods have been added yet.\n")
		return out.String(), nil
	}

	currentCategory := ""
	replacer := strings.NewReplacer("[", "\\[", "]", "\\]")
	for _, mod := range mods {
		category := modCategory(in, mod)
		if category != currentCategory {
			if currentCategory != "" {
				out.WriteString("\n")
			}
			out.WriteString("## ")
			// Map common categories to the requested display titles
			var title string
			switch strings.ToLower(category) {
			case "mods":
				title = "Mods"
			case "resourcepacks":
				title = "Resource Packs"
			case "shaderpacks":
				title = "Shader Packs"
			default:
				title = strings.Title(category)
			}
			out.WriteString(title)
			out.WriteString("\n\n")
			currentCategory = category
		}
		out.WriteString("- [")
		out.WriteString(replacer.Replace(mod.Name))
		out.WriteString(" - (")
		out.WriteString(mod.Version)
		out.WriteString(")")
		out.WriteString("](")
		out.WriteString(mod.PageURL)
		out.WriteString(")\n")
	}

	// Append Dependencies section
	if len(deps) > 0 {
		// Sort dependencies by name
		slices.SortFunc(deps, func(a, b *Mod) int {
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		})

		out.WriteString("\n## Dependencies\n\n")
		for _, d := range deps {
			out.WriteString("- [")
			out.WriteString(replacer.Replace(d.Name))
			out.WriteString(" - (")
			out.WriteString(d.Version)
			out.WriteString(")](")
			out.WriteString(d.PageURL)
			out.WriteString(")\n")
		}
	}

	return out.String(), nil
}
func (in Index) ModListPath() string {
	return filepath.Join(in.packRoot, ModListFileName)
}
func modCategory(in Index, mod *Mod) string {
	if mod.Category != "" {
		return mod.Category
	}
	modPath := filepath.Dir(mod.GetFilePath())
	relDir, err := filepath.Rel(in.packRoot, modPath)
	if err != nil || relDir == "." {
		return "uncategorized"
	}
	parts := strings.Split(filepath.ToSlash(relDir), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "uncategorized"
	}
	return parts[0]
}

func (in *Index) WriteModList() error {
	markdown, err := in.GenerateModListMarkdownWithSide(UniversalSide)
	if err != nil {
		return err
	}

	path := in.ModListPath()
	if err := os.WriteFile(path, []byte(markdown), 0644); err != nil {
		return err
	}

	hashFormat, hash, err := hashFile(path)
	if err != nil {
		return err
	}
	return in.RefreshFileWithHash(path, hashFormat, hash, false)
}

func (in *Index) WriteModListWithSide(side string) error {
	markdown, err := in.GenerateModListMarkdownWithSide(side)
	if err != nil {
		return err
	}

	path := in.ModListPath()
	if err := os.WriteFile(path, []byte(markdown), 0644); err != nil {
		return err
	}

	hashFormat, hash, err := hashFile(path)
	if err != nil {
		return err
	}
	return in.RefreshFileWithHash(path, hashFormat, hash, false)
}

func (in Index) ValidateModList() error {
	expected, err := in.GenerateModListMarkdownWithSide(UniversalSide)
	if err != nil {
		return err
	}

	actual, err := os.ReadFile(in.ModListPath())
	if err != nil {
		return err
	}

	if !bytes.Equal(actual, []byte(expected)) {
		return fmt.Errorf("modlist.md is out of date")
	}
	return nil
}

func (in Index) ValidateModListWithSide(side string) error {
	expected, err := in.GenerateModListMarkdownWithSide(side)
	if err != nil {
		return err
	}

	actual, err := os.ReadFile(in.ModListPath())
	if err != nil {
		return err
	}

	if !bytes.Equal(actual, []byte(expected)) {
		return fmt.Errorf("modlist.md is out of date")
	}
	return nil
}

func hashFile(path string) (string, string, error) {
	if viper.GetBool("no-internal-hashes") {
		return "sha256", "", nil
	}

	f, err := os.Open(path)
	if err != nil {
		return "sha256", "", err
	}
	defer func() { _ = f.Close() }()

	h, err := GetHashImpl("sha256")
	if err != nil {
		return "", "", err
	}
	if _, err := io.Copy(h, f); err != nil {
		return "", "", err
	}
	return "sha256", h.HashToString(h.Sum(nil)), nil
}
