package changelog

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// now is replaced in tests so release dates are deterministic
var now = time.Now

// firstVersion is the version of a pack's first release when neither pack.toml nor --version give one
const firstVersion = "1.0.0"

var releaseVersionFlag string

// changelogCmd represents the changelog command. On its own it previews the next release.
var changelogCmd = &cobra.Command{
	Use:   "changelog",
	Short: "Show the changes since the last release and the version they would produce",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runPreview(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

// releaseCmd represents the changelog release command
var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Record a release: bump the pack version, update CHANGELOG.md and remember the pack's contents",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if _, _, err := RunRelease(releaseVersionFlag); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

// runPreview prints the release the pack's pending changes would make, without changing anything.
func runPreview() error {
	p, err := loadPending()
	if err != nil {
		return err
	}
	if len(p.changes) == 0 {
		fmt.Println(p.noChangesMessage())
		return nil
	}
	release, err := p.plan("")
	if err != nil {
		return err
	}
	p.printPlan(release)
	return nil
}

// RunRelease records the release the pack's pending changes make, once the user confirms it. A non-empty
// versionOverride replaces the version that would otherwise be worked out from the changes. It returns the release
// and true, or false if no release was made because there was nothing to release or the user declined.
func RunRelease(versionOverride string) (Release, bool, error) {
	p, err := loadPending()
	if err != nil {
		return Release{}, false, err
	}
	if len(p.changes) == 0 {
		fmt.Println(p.noChangesMessage())
		return Release{}, false, nil
	}
	release, err := p.plan(versionOverride)
	if err != nil {
		return Release{}, false, err
	}
	p.printPlan(release)

	if !cmdshared.PromptYesNo(fmt.Sprintf("Release %s? [Y/n]: ", release.Version)) {
		fmt.Println("Cancelled!")
		return Release{}, false, nil
	}
	if err := p.apply(release); err != nil {
		return Release{}, false, fmt.Errorf("failed to release: %w", err)
	}
	fmt.Printf("Released %s!\n", release.Version)
	return release, true, nil
}

func init() {
	changelogCmd.AddCommand(releaseCmd)
	releaseCmd.Flags().StringVar(&releaseVersionFlag, "version", "", "Release this version instead of the one worked out from the changes; it must be greater than the last release")

	cmd.Add(changelogCmd)
}

func packRoot() string {
	return filepath.Dir(viper.GetString("pack-file"))
}

func historyPath() string {
	return filepath.Join(packRoot(), HistoryFile)
}

func markdownPath() string {
	return filepath.Join(packRoot(), MarkdownFile)
}

// pending is a pack, and how it differs from its last release.
type pending struct {
	pack    core.Pack
	index   core.Index
	history History
	// current is the pack as it is now, which becomes the history's snapshot if it is released
	current Snapshot
	changes []Change
}

// LoadRefreshed loads the pack and its index, with the index refreshed in memory so files added or edited since the
// last "packwiz refresh" are noticed. Nothing is written to disk.
func LoadRefreshed() (core.Pack, core.Index, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return core.Pack{}, core.Index{}, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return core.Pack{}, core.Index{}, err
	}
	if err := index.Refresh(); err != nil {
		return core.Pack{}, core.Index{}, err
	}
	return pack, index, nil
}

// loadPending reads the pack and compares it to its last release.
func loadPending() (pending, error) {
	pack, index, err := LoadRefreshed()
	if err != nil {
		return pending{}, err
	}
	history, err := LoadHistory(historyPath())
	if err != nil {
		return pending{}, fmt.Errorf("failed to read %s: %w", HistoryFile, err)
	}
	current, err := TakeSnapshot(index)
	if err != nil {
		return pending{}, err
	}
	return pending{pack, index, history, current, Diff(history.Snapshot, current)}, nil
}

func (p pending) noChangesMessage() string {
	if last, ok := p.history.Latest(); ok {
		return fmt.Sprintf("No changes since the last release (%s).", last.Version)
	}
	return "The pack has no files to release yet."
}

// plan works out the release the pending changes would make. A non-empty versionOverride replaces the version that
// would otherwise be worked out from the changes.
func (p pending) plan(versionOverride string) (Release, error) {
	release := Release{Date: now().Format("2006-01-02"), Changes: p.changes}

	var override *Version
	if versionOverride != "" {
		v, err := ParseVersion(versionOverride)
		if err != nil {
			return Release{}, err
		}
		override = &v
	}

	last, hasLast := p.history.Latest()
	if !hasLast {
		// Nothing to bump from, so the first release just takes the version the pack already has
		version, err := p.firstVersion(override)
		if err != nil {
			return Release{}, err
		}
		release.Version = version.String()
		return release, nil
	}

	base, err := ParseVersion(last.Version)
	if err != nil {
		return Release{}, fmt.Errorf("the last release in %s has an invalid version: %w", HistoryFile, err)
	}
	release.Bump = HighestBump(p.changes)
	next := base.Bump(release.Bump)
	if override != nil {
		if override.Compare(base) <= 0 {
			return Release{}, fmt.Errorf("version %s must be greater than the last release (%s)", override, base)
		}
		next = *override
	}
	release.Version = next.String()
	return release, nil
}

func (p pending) firstVersion(override *Version) (Version, error) {
	if override != nil {
		return *override, nil
	}
	packVersion := p.pack.Version
	if packVersion == "" {
		packVersion = firstVersion
	}
	v, err := ParseVersion(packVersion)
	if err != nil {
		return Version{}, fmt.Errorf("the version in pack.toml can't be used for the first release (%w); pass --version to choose one", err)
	}
	return v, nil
}

func (p pending) printPlan(release Release) {
	last, hasLast := p.history.Latest()
	if hasLast {
		fmt.Printf("Changes since %s; next version is %s (%s bump)\n", last.Version, release.Version, release.Bump)
		if p.pack.Version != "" && p.pack.Version != last.Version {
			fmt.Printf("Note: pack.toml has version %s, but the last release was %s; using the last release as the base.\n", p.pack.Version, last.Version)
		}
	} else {
		fmt.Printf("First release; version is %s\n", release.Version)
	}
	fmt.Println()
	fmt.Println(RenderRelease(release))
}

// apply records the release. The history file is written last because it is what marks the release as made: if any
// earlier step fails, running the release again works out and redoes the same release rather than skipping it.
func (p pending) apply(release Release) error {
	history := p.history
	history.Releases = append(slices.Clone(history.Releases), release)
	history.Snapshot = p.current

	// The index is written because it was refreshed to find the changes being released, and consumers of the
	// pack (which see the pack.toml version) need it to match
	if err := p.index.Write(); err != nil {
		return err
	}
	p.pack.Version = release.Version
	if err := p.pack.UpdateIndexHash(); err != nil {
		return err
	}
	if err := p.pack.Write(); err != nil {
		return err
	}
	if err := writeFileAtomic(markdownPath(), []byte(RenderMarkdown(history.Releases))); err != nil {
		return err
	}
	return history.Write(historyPath())
}
