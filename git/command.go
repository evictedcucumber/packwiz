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
	previewFlag        bool
	releaseVersionFlag string
	sinceFlag          string
)

// gitCmd represents the base command when called without any subcommands
var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Release the pack, committing and tagging the result",
}

// commitCmd represents the commit command, which is registered on the root command: committing is the everyday command
// for a pack, where releasing is occasional
var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Commit each mod and config file that changed on its own, with conventional commit messages",
	Long: fmt.Sprintf(`With --preview, prints the commits it would make, in order, and makes none: nothing is committed or saved, so it is
safe to run first. Without it, refreshes the index, then commits every change to the files the pack tracks, making one commit for each mod, resource
pack, shader pack, data pack or config file that was added, updated, changed or removed, and one for what else changed in pack.toml and the index (a
mod being pinned, say). Each commit has a conventional commit message, whose type follows how far the change raises
the pack's version: feat! for a change to a mod that runs on the server, feat for adding or removing a client-only
mod, and fix for a client-only mod update or a config change. Every commit also holds an index and pack.toml that
describe the pack as it is in that commit, so each one is a valid pack (though files go in alphabetical order, so a
mod can come before one it depends on).

Files that packwiz doesn't track can be sorted into categories in .packwizfiles.toml next to pack.toml, which gives
each category the patterns of its files, written like those of .gitignore and relative to the pack's directory:

    [categories]
    dev = ["flake.nix", "lefthook.yml", ".envrc"]
    docs = ["docs/"]

Each file in a category that changed is committed on its own, as "chore(dev): change flake.nix" (or add, or remove),
after the mods and config files. They are chores, so they never change the pack's version or its changelog. A file in
two categories is in the first by name, and a file the index tracks is never in one. The pack's README.md and LICENSE
are in docs, and everything in serverconfig/ (the server pack's own files) is in server, without being listed, unless
.packwizfiles.toml puts them in another category. These are the only
categories:

%s
Other files packwiz doesn't recognise (anything in the pack's directory that isn't tracked by the index, isn't in a
category, isn't the index or pack.toml, and isn't the pack's changelog, list of mods, .packwizignore or
.packwizfiles.toml) are left uncommitted, and are listed. Commit them yourself with git; "packwiz changelog release" won't release while they are there.`, categoryList()),
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runCommit(previewFlag); err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
	},
}

// categoryList is the categories a file can be put in, one to a line with what goes in it, for the help and the manual.
func categoryList() string {
	var b strings.Builder
	for _, c := range core.FileCategoryList {
		fmt.Fprintf(&b, "    %-7s %s\n", c.Name, c.Description)
	}
	return b.String()
}

// releaseCmd represents the git release command
var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Record a release with \"packwiz changelog release\", then commit and tag it",
	Long: `Records a release (see "packwiz changelog release", which needs the pack's changes committed first with
"packwiz commit"), commits the result as "chore(release): X.Y.Z" and tags it "vX.Y.Z".`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runRelease(releaseVersionFlag, sinceFlag); err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	gitCmd.AddCommand(releaseCmd)

	commitCmd.Flags().BoolVar(&previewFlag, "preview", false, "Print the commits that would be made, without making them")
	releaseCmd.Flags().StringVar(&releaseVersionFlag, "version", "", "Release this version instead of the one worked out from the commits; it must be greater than the last release")
	releaseCmd.Flags().StringVar(&sinceFlag, "since", "", "Read the commits made after this one (a hash, tag or branch), rather than after the last release")

	cmd.Add(commitCmd)
	cmd.Add(gitCmd)
}

// packFile is the path of pack.toml relative to the pack root, always with forward slashes as git uses them
func packFile() string {
	return filepath.ToSlash(filepath.Base(viper.GetString("pack-file")))
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

	// This fails if the pack has changes that aren't committed, so the log the release is made from has them all
	release, released, err := changelog.RunRelease(versionOverride, since)
	if err != nil {
		return err
	}
	if !released {
		// Nothing was released, but that can still change the changelog, and this command isn't going to commit it
		if dirty, err := r.dirty(); err == nil && dirty {
			ui.Info.Println(`The pack changed; commit it with "packwiz commit".`)
		}
		return nil
	}

	tag, err := commitRelease(r, release)
	if err != nil {
		return err
	}
	ui.Success.Printf("Committed and tagged %s\n", ui.Bold.Sprint(tag))
	return nil
}

// commitRelease commits a release that is on disk, and tags the commit, and returns the tag. From here the release is made,
// so failing to commit or tag it has to say how to finish the job by hand.
func commitRelease(r repo, release changelog.Release) (tag string, err error) {
	tag = TagName(release.Version)
	// Only what a release writes: anything else in the pack's directory isn't part of it
	if err := r.commitPaths(ReleaseMessage(release.Version), changelog.HistoryFile, changelog.MarkdownFile, packFile()); err != nil {
		return "", fmt.Errorf("released %s, but couldn't commit it: %w\nCommit the changed files yourself, then tag the commit %s", release.Version, err, tag)
	}
	if err := r.tag(tag, "Release "+release.Version); err != nil {
		return "", fmt.Errorf("released and committed %s, but couldn't tag it: %w\nTag the commit %s yourself", release.Version, err, tag)
	}
	return tag, nil
}

// Released is what releasing the pack did.
type Released struct {
	// Made is whether a release was made: it isn't if there is nothing to release
	Made    bool
	Release changelog.Release
	// Tag is the tag the release was committed with
	Tag string
	// NoChanges says why nothing was released, if nothing was
	NoChanges string
	// Dirty is whether the pack has changes that aren't committed, which is so after nothing was released if that still
	// changed the changelog
	Dirty bool
	// Notices are what was said along the way, in plain text
	Notices []string
}

// ReleaseAndTag records a release, commits it and tags it, as "packwiz git release" does, without asking whether to and
// without saying anything on the terminal: whoever calls it has asked. See runRelease for versionOverride and since.
func ReleaseAndTag(versionOverride, since string) (Released, error) {
	r, err := openRepo(packRoot())
	if err != nil {
		return Released{}, err
	}
	made, err := changelog.MakeRelease(versionOverride, since)
	if err != nil {
		return Released{}, err
	}
	result := Released{Made: made.Made, Release: made.Release, NoChanges: made.NoChanges, Notices: made.Notices}
	if !made.Made {
		if dirty, err := r.dirty(); err == nil {
			result.Dirty = dirty
		}
		return result, nil
	}
	if result.Tag, err = commitRelease(r, made.Release); err != nil {
		return result, err
	}
	return result, nil
}

func firstLine(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	return first
}
