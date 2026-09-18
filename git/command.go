package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	dryRunFlag         bool
	releaseVersionFlag string
)

// gitCmd represents the base command when called without any subcommands
var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Commit and release the pack following packwiz's conventional commit standard",
}

// commitCmd represents the git commit command
var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Commit every change to the pack, with a conventional commit message describing what changed",
	Long: `Refreshes the index, then commits every change under the pack's directory (except what git ignores) with a
generated message. The message's type follows how far the changes raise the pack's version: feat! for changes to
mods that run on the server, feat for client-only mods being added or removed, and fix for config changes and
client-only mod updates.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runCommit(dryRunFlag); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

// releaseCmd represents the git release command
var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Record a release with \"packwiz changelog release\", then commit and tag it",
	Long: `Records a release (see "packwiz changelog release"), commits the result as "chore(release): X.Y.Z" and tags it
"vX.Y.Z". The pack must have no uncommitted changes, so the release commit contains only the release itself;
use "packwiz git commit" first.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runRelease(releaseVersionFlag); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	gitCmd.AddCommand(commitCmd)
	gitCmd.AddCommand(releaseCmd)

	commitCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Print the commit message without committing")
	releaseCmd.Flags().StringVar(&releaseVersionFlag, "version", "", "Release this version instead of the one worked out from the changes; it must be greater than the last release")

	cmd.Add(gitCmd)
}

func packRoot() string {
	return filepath.Dir(viper.GetString("pack-file"))
}

// indexFile is the path of the pack's index relative to the pack root, always with forward slashes as git uses them
func indexFile(pack core.Pack) (string, error) {
	if filepath.IsAbs(pack.Index.File) {
		return "", fmt.Errorf("the index file %s is given as an absolute path, so it can't be looked up in git", pack.Index.File)
	}
	return filepath.ToSlash(filepath.Clean(pack.Index.File)), nil
}

// runCommit commits every change under the pack root. With dryRun, it only prints the message it would commit with.
func runCommit(dryRun bool) error {
	// Committing saves the versions looked up for mods that don't record one, so a failure to look them up mustn't
	// be papered over; describing a commit that won't be made can carry on without them
	w, err := changelog.LoadWorking(!dryRun)
	if err != nil {
		return err
	}
	r, err := openRepo(packRoot())
	if err != nil {
		return err
	}
	current, err := w.Snapshot()
	if err != nil {
		return err
	}

	message := InitialMessage
	hasCommits, err := r.hasCommits()
	if err != nil {
		return err
	}
	changed := hasCommits
	if hasCommits {
		indexPath, err := indexFile(w.Pack)
		if err != nil {
			return err
		}
		previous, err := r.snapshotAt("HEAD", indexPath)
		if err != nil {
			return fmt.Errorf("failed to read the pack as of the last commit: %w", err)
		}
		changes := changelog.Diff(previous, current)
		message = Message(changes)
		changed = len(changes) > 0
	}

	if dryRun {
		dirty, err := r.dirty()
		if err != nil {
			return err
		}
		// A real commit would also save any versions that were looked up
		if !dirty && !changed && len(w.Versions) == 0 {
			fmt.Println("Nothing to commit.")
			return nil
		}
		fmt.Println(message)
		return nil
	}

	// Commit the refreshed index too, so the pack that gets committed is one that is consistent
	if err := w.Index.RecordVersions(w.Versions); err != nil {
		return err
	}
	if err := w.Index.Write(); err != nil {
		return err
	}
	if err := w.Pack.UpdateIndexHash(); err != nil {
		return err
	}
	if err := w.Pack.Write(); err != nil {
		return err
	}
	dirty, err := r.dirty()
	if err != nil {
		return err
	}
	if !dirty {
		fmt.Println("Nothing to commit.")
		return nil
	}
	if err := r.commitAll(message); err != nil {
		return err
	}
	fmt.Println("Committed: " + firstLine(message))
	return nil
}

// runRelease records a release, commits it and tags it. A non-empty versionOverride replaces the version that would
// otherwise be worked out from the changes.
func runRelease(versionOverride string) error {
	r, err := openRepo(packRoot())
	if err != nil {
		return err
	}
	hasCommits, err := r.hasCommits()
	if err != nil {
		return err
	}
	if !hasCommits {
		return errors.New("the pack has no commits yet; run \"packwiz git commit\" first")
	}
	dirty, err := r.dirty()
	if err != nil {
		return err
	}
	if dirty {
		return errors.New("the pack has uncommitted changes; run \"packwiz git commit\" first, so the release commit contains only the release")
	}

	release, released, err := changelog.RunRelease(versionOverride)
	if err != nil {
		return err
	}
	if !released {
		// Nothing was released, but a release also saves what it looked up for mods that don't record a version,
		// and this command isn't going to commit that
		if dirty, err := r.dirty(); err == nil && dirty {
			fmt.Println(`The pack changed; commit it with "packwiz git commit".`)
		}
		return nil
	}

	// From here the release is on disk, so failing to commit or tag it has to say how to finish the job by hand
	tag := TagName(release.Version)
	if err := r.commitAll(ReleaseMessage(release.Version)); err != nil {
		return fmt.Errorf("released %s, but couldn't commit it: %w\nCommit the changed files yourself, then tag the commit %s", release.Version, err, tag)
	}
	if err := r.tag(tag, "Release "+release.Version); err != nil {
		return fmt.Errorf("released and committed %s, but couldn't tag it: %w\nTag the commit %s yourself", release.Version, err, tag)
	}
	fmt.Printf("Committed and tagged %s\n", tag)
	return nil
}

func firstLine(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	return first
}
