package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

const DependencyTreeFileName = "dependencies.toml"

// DependencyNode represents a node in the dependency tree
type DependencyNode struct {
	Name         string            `toml:"name"`
	File         string            `toml:"file"`
	Dependencies []*DependencyNode `toml:"dependencies,omitempty"`
}

// PackDependencies represents the root structure of the saved dependency tree file
type PackDependencies struct {
	Mods          []*DependencyNode `toml:"mods,omitempty"`
	ResourcePacks []*DependencyNode `toml:"resourcepacks,omitempty"`
	ShaderPacks   []*DependencyNode `toml:"shaderpacks,omitempty"`
}

// GenerateDependencyTree builds the dependency tree for the entire modpack
func (in Index) GenerateDependencyTree() (PackDependencies, error) {
	allMods, err := in.LoadAllMods()
	if err != nil {
		return PackDependencies{}, err
	}

	modsMap := make(map[string]*Mod)
	for _, m := range allMods {
		relPath, err := in.RelIndexPath(m.GetFilePath())
		if err != nil {
			continue
		}
		modsMap[relPath] = m
	}

	// Find root nodes: mods that are NOT marked as dependencies
	var roots []*Mod
	for _, m := range allMods {
		if m.Option == nil || !m.Option.Dependency {
			roots = append(roots, m)
		}
	}

	// Fallback if all mods are marked as dependencies (unlikely) or none are (e.g. old pack format)
	if len(roots) == 0 || len(roots) == len(allMods) {
		roots = nil
		referenced := make(map[string]bool)
		for _, m := range allMods {
			for _, depPath := range m.Dependencies {
				depPathClean, err := in.ToIndexRelativePath(depPath)
				if err != nil {
					depPathClean = filepath.ToSlash(filepath.Clean(depPath))
				}
				referenced[depPathClean] = true
			}
		}
		for _, m := range allMods {
			relPath, _ := in.RelIndexPath(m.GetFilePath())
			if !referenced[relPath] {
				roots = append(roots, m)
			}
		}
	}

	// Final fallback: if still empty, use all mods
	if len(roots) == 0 {
		roots = allMods
	}

	// Sort roots by name to ensure deterministic output
	slices.SortFunc(roots, func(a, b *Mod) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})

	var packDeps PackDependencies
	visited := make(map[string]bool)

	for _, root := range roots {
		node := in.buildDependencyNode(root, modsMap, visited)
		category := modCategory(in, root)
		switch strings.ToLower(category) {
		case "mods":
			packDeps.Mods = append(packDeps.Mods, node)
		case "resourcepacks":
			packDeps.ResourcePacks = append(packDeps.ResourcePacks, node)
		case "shaderpacks":
			packDeps.ShaderPacks = append(packDeps.ShaderPacks, node)
		default:
			// Fallback to mods for uncategorized
			packDeps.Mods = append(packDeps.Mods, node)
		}
	}

	return packDeps, nil
}

// buildDependencyNode recursively builds a dependency node for a given mod
func (in Index) buildDependencyNode(mod *Mod, modsMap map[string]*Mod, visited map[string]bool) *DependencyNode {
	relPath, _ := in.RelIndexPath(mod.GetFilePath())
	node := &DependencyNode{
		Name: mod.Name,
		File: relPath,
	}

	// Cycle detection
	if visited[relPath] {
		node.Name = node.Name + " [Cycle]"
		return node
	}

	visited[relPath] = true
	defer func() { visited[relPath] = false }()

	// Sort dependencies to ensure deterministic output
	deps := append([]string(nil), mod.Dependencies...)
	slices.Sort(deps)

	for _, depPath := range deps {
		depPathClean, err := in.ToIndexRelativePath(depPath)
		if err != nil {
			depPathClean = filepath.ToSlash(filepath.Clean(depPath))
		}
		if depMod, ok := modsMap[depPathClean]; ok {
			childNode := in.buildDependencyNode(depMod, modsMap, visited)
			node.Dependencies = append(node.Dependencies, childNode)
		} else {
			node.Dependencies = append(node.Dependencies, &DependencyNode{
				Name: "Unknown/Untracked Mod",
				File: depPathClean,
			})
		}
	}

	return node
}

// DependencyTreePath returns the path to the dependencies.toml file
func (in Index) DependencyTreePath() string {
	return filepath.Join(in.packRoot, DependencyTreeFileName)
}

// WriteDependencyTree writes the dependency tree to dependencies.toml and registers it in the index
func (in *Index) WriteDependencyTree() error {
	packDeps, err := in.GenerateDependencyTree()
	if err != nil {
		return err
	}

	path := in.DependencyTreePath()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	enc.Indent = ""
	if err := enc.Encode(packDeps); err != nil {
		return err
	}
	_ = f.Close()

	hashFormat, hash, err := hashFile(path)
	if err != nil {
		return err
	}
	return in.RefreshFileWithHash(path, hashFormat, hash, false)
}

// LoadDependencyTree attempts to read the dependency tree from dependencies.toml
func (in Index) LoadDependencyTree() (PackDependencies, error) {
	var packDeps PackDependencies
	path := in.DependencyTreePath()
	if _, err := toml.DecodeFile(path, &packDeps); err != nil {
		return PackDependencies{}, err
	}
	return packDeps, nil
}

// ValidateDependencyTree checks if dependencies.toml is up to date and contains no untracked dependencies
func (in Index) ValidateDependencyTree() error {
	expectedTree, err := in.GenerateDependencyTree()
	if err != nil {
		return err
	}

	// 1. Check for untracked/missing dependencies in the generated tree
	var checkUntracked func(node *DependencyNode) []string
	checkUntracked = func(node *DependencyNode) []string {
		var missing []string
		if node.Name == "Unknown/Untracked Mod" {
			missing = append(missing, node.File)
		}
		for _, child := range node.Dependencies {
			missing = append(missing, checkUntracked(child)...)
		}
		return missing
	}

	var allMissing []string
	for _, node := range expectedTree.Mods {
		allMissing = append(allMissing, checkUntracked(node)...)
	}
	for _, node := range expectedTree.ResourcePacks {
		allMissing = append(allMissing, checkUntracked(node)...)
	}
	for _, node := range expectedTree.ShaderPacks {
		allMissing = append(allMissing, checkUntracked(node)...)
	}

	if len(allMissing) > 0 {
		slices.Sort(allMissing)
		allMissing = slices.Compact(allMissing)
		return fmt.Errorf("missing or untracked dependency files: %s", strings.Join(allMissing, ", "))
	}

	// 2. Load stored tree and compare
	actualBytes, err := os.ReadFile(in.DependencyTreePath())
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("dependencies.toml is missing; run 'packwiz dependencies generate'")
		}
		return err
	}

	var expectedBuf strings.Builder
	enc := toml.NewEncoder(&expectedBuf)
	enc.Indent = ""
	if err := enc.Encode(expectedTree); err != nil {
		return err
	}

	if strings.TrimSpace(string(actualBytes)) != strings.TrimSpace(expectedBuf.String()) {
		return fmt.Errorf("dependencies.toml is out of date; run 'packwiz dependencies generate'")
	}

	return nil
}

// SyncDepsOpts controls dependency metadata maintenance after pack changes.
type SyncDepsOpts struct {
	// NormalizeAll rewrites dependency paths to index-relative form for every mod.
	NormalizeAll bool
	// RefreshMods re-resolves dependencies from provider APIs for the given mods.
	RefreshMods []*Mod
	// RefreshAll re-resolves dependencies for every mod in the pack.
	RefreshAll bool
	// RemovedModPath is the absolute path of a removed mod metadata file.
	RemovedModPath string
	// ForceDependencyTree always regenerates dependencies.toml, even when not yet tracked.
	ForceDependencyTree bool
}

// ToIndexRelativePath converts an absolute or index-relative path to an index-relative path.
func (in Index) ToIndexRelativePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("empty path")
	}

	clean := filepath.ToSlash(filepath.Clean(path))
	if !filepath.IsAbs(filepath.FromSlash(clean)) {
		if _, ok := in.Files[clean]; ok {
			return clean, nil
		}
		abs := in.ResolveIndexPath(clean)
		if _, err := os.Stat(abs); err == nil {
			return in.RelIndexPath(abs)
		}
		return clean, nil
	}

	return in.RelIndexPath(path)
}

// DependencyTreeTracked reports whether dependencies.toml is tracked in the index.
func (in Index) DependencyTreeTracked() bool {
	_, ok := in.Files[DependencyTreeFileName]
	return ok
}

// ShouldSyncDependencyTree reports whether dependencies.toml should be maintained automatically.
func (in Index) ShouldSyncDependencyTree() bool {
	if in.DependencyTreeTracked() {
		return true
	}

	allMods, err := in.LoadAllMods()
	if err != nil {
		return false
	}
	for _, m := range allMods {
		if len(m.Dependencies) > 0 {
			return true
		}
	}
	return false
}

func dependencySlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (in *Index) normalizeModDependencyPaths(mod *Mod) (bool, error) {
	if len(mod.Dependencies) == 0 {
		return false, nil
	}

	normalized := make([]string, 0, len(mod.Dependencies))
	seen := make(map[string]struct{}, len(mod.Dependencies))
	for _, depPath := range mod.Dependencies {
		relPath, err := in.ToIndexRelativePath(depPath)
		if err != nil {
			relPath = filepath.ToSlash(filepath.Clean(depPath))
		}
		if _, ok := seen[relPath]; ok {
			continue
		}
		seen[relPath] = struct{}{}
		normalized = append(normalized, relPath)
	}
	slices.Sort(normalized)

	if dependencySlicesEqual(mod.Dependencies, normalized) {
		return false, nil
	}

	mod.Dependencies = normalized
	format, hash, err := mod.Write()
	if err != nil {
		return false, fmt.Errorf("failed to write updated mod %s: %w", mod.Name, err)
	}
	if err := in.RefreshFileWithHash(mod.GetFilePath(), format, hash, true); err != nil {
		return false, err
	}
	return true, nil
}

// NormalizeAllModDependencyPaths rewrites dependency paths to index-relative form.
func (in *Index) NormalizeAllModDependencyPaths() error {
	allMods, err := in.LoadAllMods()
	if err != nil {
		return err
	}
	for _, m := range allMods {
		if _, err := in.normalizeModDependencyPaths(m); err != nil {
			return err
		}
	}
	return nil
}

func (in *Index) resolveModDependencies(pack Pack, allMods []*Mod, mod *Mod) (bool, error) {
	var resolvedDeps []string
	var resolved bool

	for updateSystem := range mod.Update {
		resolver, ok := DependencyResolvers[updateSystem]
		if !ok {
			continue
		}
		deps, err := resolver.ResolveDependencies(mod, allMods, *in, pack)
		if err != nil {
			fmt.Printf("Warning: failed to resolve dependencies for mod %s using %s: %v\n", mod.Name, updateSystem, err)
			continue
		}
		resolvedDeps = append(resolvedDeps, deps...)
		resolved = true
	}

	if !resolved {
		return false, nil
	}

	slices.Sort(resolvedDeps)
	resolvedDeps = slices.Compact(resolvedDeps)

	if dependencySlicesEqual(mod.Dependencies, resolvedDeps) {
		return false, nil
	}

	mod.Dependencies = resolvedDeps
	format, hash, err := mod.Write()
	if err != nil {
		return false, fmt.Errorf("failed to write updated mod %s: %w", mod.Name, err)
	}
	if err := in.RefreshFileWithHash(mod.GetFilePath(), format, hash, true); err != nil {
		return false, err
	}
	return true, nil
}

func (in *Index) refreshModDependencies(pack Pack, mods []*Mod) error {
	allMods, err := in.LoadAllMods()
	if err != nil {
		return err
	}

	targets := mods
	if len(targets) == 0 {
		targets = allMods
	}

	targetPaths := make(map[string]struct{}, len(targets))
	for _, mod := range targets {
		targetPaths[mod.GetFilePath()] = struct{}{}
	}

	for _, mod := range allMods {
		if _, ok := targetPaths[mod.GetFilePath()]; !ok {
			continue
		}
		if _, err := in.resolveModDependencies(pack, allMods, mod); err != nil {
			return err
		}
	}
	return nil
}

// PruneDependencyReference removes references to a deleted mod from all metadata files.
func (in *Index) PruneDependencyReference(removedModPath string) error {
	removedRel, err := in.ToIndexRelativePath(removedModPath)
	if err != nil {
		return err
	}

	allMods, err := in.LoadAllMods()
	if err != nil {
		return err
	}

	for _, mod := range allMods {
		if len(mod.Dependencies) == 0 {
			continue
		}

		changed := false
		pruned := make([]string, 0, len(mod.Dependencies))
		for _, depPath := range mod.Dependencies {
			depRel, err := in.ToIndexRelativePath(depPath)
			if err != nil {
				depRel = filepath.ToSlash(filepath.Clean(depPath))
			}
			if depRel == removedRel {
				changed = true
				continue
			}
			pruned = append(pruned, depRel)
		}
		if !changed {
			continue
		}

		mod.Dependencies = pruned
		format, hash, err := mod.Write()
		if err != nil {
			return fmt.Errorf("failed to write updated mod %s: %w", mod.Name, err)
		}
		if err := in.RefreshFileWithHash(mod.GetFilePath(), format, hash, true); err != nil {
			return err
		}
	}

	return nil
}

// SyncDependencyMetadata updates dependency references and optionally regenerates dependencies.toml.
func (in *Index) SyncDependencyMetadata(pack Pack, opts SyncDepsOpts) error {
	if opts.RemovedModPath != "" {
		if err := in.PruneDependencyReference(opts.RemovedModPath); err != nil {
			return err
		}
	}

	if opts.RefreshAll {
		if err := in.refreshModDependencies(pack, nil); err != nil {
			return err
		}
	} else if len(opts.RefreshMods) > 0 {
		if err := in.refreshModDependencies(pack, opts.RefreshMods); err != nil {
			return err
		}
	}

	if opts.NormalizeAll {
		if err := in.NormalizeAllModDependencyPaths(); err != nil {
			return err
		}
	}

	if opts.ForceDependencyTree || in.ShouldSyncDependencyTree() {
		return in.WriteDependencyTree()
	}
	return nil
}

// FixDependencies walks all mods, resolves their dependencies using registered DependencyResolvers,
// and updates their .pw.toml files on disk.
func (in *Index) FixDependencies(pack Pack) error {
	return in.SyncDependencyMetadata(pack, SyncDepsOpts{RefreshAll: true})
}

// FormatDependencyTree returns a formatted ASCII tree string of the dependencies
func (packDeps PackDependencies) FormatDependencyTree() string {
	var sb strings.Builder

	formatSection := func(title string, nodes []*DependencyNode) {
		if len(nodes) == 0 {
			return
		}
		sb.WriteString(fmt.Sprintf("%s:\n", title))
		for i, node := range nodes {
			sb.WriteString(formatTreeNode(node, "", i == len(nodes)-1))
		}
		sb.WriteString("\n")
	}

	formatSection("Mods", packDeps.Mods)
	formatSection("Resource Packs", packDeps.ResourcePacks)
	formatSection("Shader Packs", packDeps.ShaderPacks)

	return strings.TrimSpace(sb.String())
}

// formatTreeNode helper constructs the ASCII representation of a tree node
func formatTreeNode(node *DependencyNode, prefix string, isLast bool) string {
	var sb strings.Builder
	marker := "├── "
	if isLast {
		marker = "└── "
	}

	sb.WriteString(prefix + marker + fmt.Sprintf("%s (%s)\n", node.Name, node.File))

	childPrefix := prefix
	if isLast {
		childPrefix += "    "
	} else {
		childPrefix += "│   "
	}

	for i, child := range node.Dependencies {
		sb.WriteString(formatTreeNode(child, childPrefix, i == len(node.Dependencies)-1))
	}
	return sb.String()
}
