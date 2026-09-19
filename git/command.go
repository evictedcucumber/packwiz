package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	dryRunFlag         bool
	releaseVersionFlag string
	sinceFlag          string
)

// gitCmd represents the base command when called without any subcommands
var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Commit and release the pack following packwiz's conventional commit standard",
}

// commitCmd represents the git commit command
var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Commit each mod that changed on its own, then everything else, with conventional commit messages",
	Long: `Refreshes the index, then commits every change under the pack's directory (except what git ignores), making one
commit for each mod that was added, updated or removed, and one for everything else. Each commit has a conventional
commit message, whose type follows how far the change raises the pack's version: feat! for a change to a mod that
runs on the server, feat for adding or removing a client-only mod, and fix for a client-only mod update or a config
change. Every commit also holds an index and pack.toml that describe the pack as it is in that commit, so each one is a
valid pack (though mods go in alphabetical order, so one can come before a mod it depends on).`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runCommit(dryRunFlag); err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
	},
}

// releaseCmd represents the git release command
var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Record a release with \"packwiz changelog release\", then commit and tag it",
	Long: `Records a release (see "packwiz changelog release", which first commits any changes to the pack), commits the
result as "chore(release): X.Y.Z" and tags it "vX.Y.Z".`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runRelease(releaseVersionFlag, sinceFlag); err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	gitCmd.AddCommand(commitCmd)
	gitCmd.AddCommand(releaseCmd)

	commitCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Print the commit messages without committing")
	releaseCmd.Flags().StringVar(&releaseVersionFlag, "version", "", "Release this version instead of the one worked out from the commits; it must be greater than the last release")
	releaseCmd.Flags().StringVar(&sinceFlag, "since", "", "Read the commits made after this one (a hash, tag or branch), rather than after the last release")

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

// runRelease records a release, commits it and tags it. A non-empty versionOverride replaces the version that would
// otherwise be worked out from the commits, and a non-empty since is the commit to read the log from, if it isn't where
// the last release was made.
func runRelease(versionOverride, since string) error {
	r, err := openRepo(packRoot())
	if err != nil {
		return err
	}

	// This commits any changes to the pack first, so they are in the log the release is made from
	release, released, err := changelog.RunRelease(versionOverride, since)
	if err != nil {
		return err
	}
	if !released {
		// Nothing was released, but that can still change the changelog, and this command isn't going to commit it
		if dirty, err := r.dirty(); err == nil && dirty {
			ui.Info.Println(`The pack changed; commit it with "packwiz git commit".`)
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
	ui.Success.Printf("Committed and tagged %s\n", ui.Bold.Sprint(tag))
	return nil
}

func firstLine(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	return first
}
