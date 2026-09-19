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
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// now is replaced in tests so release dates are deterministic
var now = time.Now

// firstVersion is the version of a pack's first release when neither pack.toml nor --version give one
const firstVersion = "1.0.0"

var (
	releaseVersionFlag string
	sinceFlag          string
)

// changelogCmd represents the changelog command. On its own it previews the next release.
var changelogCmd = &cobra.Command{
	Use:   "changelog",
	Short: "Show the changes since the last release and the version they would produce",
	Long: `Reads the commits made since the last release, which are conventional commits (see "packwiz git commit"), and shows
the release they would make and the version it would have. Nothing is committed or saved.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runPreview(sinceFlag); err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
	},
}

// releaseCmd represents the changelog release command
var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Commit any changes, then record a release: bump the pack version and update CHANGELOG.md",
	Long: `Commits any changes to the pack that aren't committed yet, as "packwiz git commit" does, so that the git log is up
to date. Then it reads the commits made since the last release, works out the version they make from their types (a
breaking change is major, a feature is minor and a fix is patch), updates the version in pack.toml and adds the
release to CHANGELOG.md.

Besides what "packwiz git commit" writes for mods and config files, any conventional commit that is a feature, a fix
or breaking is listed in the release in its own words, and counts towards the version.

The first release describes the pack as it is, and keeps the version already in pack.toml.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if _, _, err := RunRelease(releaseVersionFlag, sinceFlag); err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
	},
}

// runPreview prints the release the pack's changes would make, including those that haven't been committed yet,
// without changing anything. since is the commit to read the log from, if it isn't where the last release was made.
func runPreview(since string) error {
	repo, err := OpenRepository()
	if err != nil {
		return err
	}
	p, err := loadPending(repo, false, since, true)
	if err != nil {
		return err
	}
	if len(p.changes) == 0 {
		ui.Info.Println(p.noChangesMessage())
		return nil
	}
	release, err := p.plan("")
	if err != nil {
		return err
	}
	p.printPlan(release)
	return nil
}

// RunRelease records the release that the commits made since the last one make, once the user confirms it. First it
// commits any changes to the pack that aren't committed yet, so they are in the log. A non-empty versionOverride
// replaces the version that would otherwise be worked out from the commits, and a non-empty since is the commit to
// read the log from, if it isn't where the last release was made. It returns the release and true, or false if no
// release was made because there was nothing to release or the user declined.
func RunRelease(versionOverride, since string) (Release, bool, error) {
	repo, err := OpenRepository()
	if err != nil {
		return Release{}, false, err
	}
	if err := repo.CommitPending(); err != nil {
		return Release{}, false, err
	}
	p, err := loadPending(repo, true, since, false)
	if err != nil {
		return Release{}, false, err
	}
	if len(p.changes) == 0 {
		ui.Info.Println(p.noChangesMessage())
		// Nothing to release, but past releases can still be made to show versions where they showed file names
		if p.upgraded > 0 {
			if err := p.save(p.history); err != nil {
				return Release{}, false, fmt.Errorf("failed to update the changelog: %w", err)
			}
			ui.Success.Printf("Updated %d %s in the changelog.\n", p.upgraded, plural(p.upgraded, "line"))
		}
		return Release{}, false, nil
	}
	release, err := p.plan(versionOverride)
	if err != nil {
		return Release{}, false, err
	}
	p.printPlan(release)

	if !cmdshared.PromptYesNo(fmt.Sprintf("Release %s? [Y/n]: ", release.Version)) {
		ui.Warning.Println("Cancelled!")
		return Release{}, false, nil
	}
	if err := p.apply(release); err != nil {
		return Release{}, false, fmt.Errorf("failed to release: %w", err)
	}
	ui.Success.Printf("Released %s!\n", ui.Bold.Sprint(release.Version))
	return release, true, nil
}

func init() {
	changelogCmd.AddCommand(releaseCmd)
	changelogCmd.PersistentFlags().StringVar(&sinceFlag, "since", "", "Read the commits made after this one (a hash, tag or branch), rather than after the last release")
	releaseCmd.Flags().StringVar(&releaseVersionFlag, "version", "", "Release this version instead of the one worked out from the commits; it must be greater than the last release")

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

// pending is a pack, and what has changed in it since its last release.
type pending struct {
	pack    core.Pack
	history History
	// changes are what the commits since the last release changed; for the first release, they are what the pack contains
	changes []Change
	// head is the commit a release would be made at. It is only known when the changes have all been committed.
	head string
	// upgraded is how many lines of past releases showed a file name that has been replaced by a real version
	upgraded int
}

// loadPending reads the pack, and the commits made since its last release. strict is whether failing to look up the
// versions of mods that don't record one is an error (see LoadWorking). withPending is whether to include the commits
// that "packwiz git commit" would make as well as those that have been made, for a preview.
func loadPending(repo Repository, strict bool, since string, withPending bool) (pending, error) {
	w, err := LoadWorking(strict)
	if err != nil {
		return pending{}, err
	}
	history, err := LoadHistory(historyPath())
	if err != nil {
		return pending{}, fmt.Errorf("failed to read %s: %w", HistoryFile, err)
	}
	current, err := w.Snapshot()
	if err != nil {
		return pending{}, err
	}
	// Anything released before a mod's version was known was recorded by file name. That's only fixed in memory
	// here; it is written if a release is.
	p := pending{pack: w.Pack, history: history, upgraded: history.UpgradeVersions(current)}

	if _, released := history.Latest(); !released {
		// The first release describes the pack as it is. Its history begins with whatever was committed first, which
		// doesn't list the mods that were in it.
		p.changes = Diff(Snapshot{}, current)
	} else {
		base, err := releaseBase(repo, history, since)
		if err != nil {
			return pending{}, err
		}
		commits, err := repo.Log(base)
		if err != nil {
			return pending{}, fmt.Errorf("couldn't read the commits made since %s: %w\nUse --since to say which commit to read them from", base, err)
		}
		if withPending {
			uncommitted, err := repo.PendingCommits()
			if err != nil {
				return pending{}, err
			}
			commits = append(commits, uncommitted...)
		}
		p.changes = ChangesFromCommits(commits)
	}

	if !withPending {
		if p.head, err = repo.Head(); err != nil {
			return pending{}, err
		}
	}
	return p, nil
}

// releaseBase is the commit that the next release is made from the commits after: the one given, or else the one the
// last release was made at.
func releaseBase(repo Repository, history History, since string) (string, error) {
	if since != "" {
		return since, nil
	}
	last, _ := history.Latest()
	if last.Commit != "" {
		return last.Commit, nil
	}
	// A release from before the commit was recorded was committed along with the history file, so the last commit to
	// change that file is where it was made
	commit, err := repo.LastChangedIn(HistoryFile)
	if err != nil {
		return "", err
	}
	if commit == "" {
		return "", fmt.Errorf("can't tell where release %s was made in the history of the pack; use --since to say which commit to read the log from", last.Version)
	}
	return commit, nil
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
		fmt.Printf("Changes since %s; next version is %s %s\n", last.Version, ui.Bold.Sprint(release.Version), ui.Muted.Sprintf("(%s bump)", release.Bump))
		if p.pack.Version != "" && p.pack.Version != last.Version {
			ui.Info.Printf("Note: pack.toml has version %s, but the last release was %s; using the last release as the base.\n", p.pack.Version, last.Version)
		}
	} else {
		fmt.Printf("First release; version is %s\n", ui.Bold.Sprint(release.Version))
	}
	fmt.Println()
	fmt.Println(renderRelease(release, style{colour: true}))
}

// apply records the release, as made at the commit that was current when it was worked out.
func (p pending) apply(release Release) error {
	release.Commit = p.head
	history := p.history
	history.Releases = append(slices.Clone(history.Releases), release)
	p.pack.Version = release.Version
	return p.save(history)
}

// save writes pack.toml and the release history. The history file is written last because it is what marks a release
// as made: if any earlier step fails, running the release again works out and redoes the same release rather than
// skipping it.
func (p pending) save(history History) error {
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
