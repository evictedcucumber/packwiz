package changelog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
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
	saveFlag           bool
)

// changelogCmd represents the changelog command. On its own it previews the next release.
var changelogCmd = &cobra.Command{
	Use:   "changelog",
	Short: "Show the changes since the last release and the version they would produce",
	Long: `Reads the commits made since the last release, which are conventional commits (see "packwiz git commit"), and shows
the release they would make and the version it would have. Nothing is committed or saved.

A pack that isn't in a git repository has no commits to read, so its changes are found by comparing it with the pack as
it was at the last release instead.

With --save, nothing is previewed: CHANGELOG.md is written from the releases recorded in changelog.toml, with the changes
not released yet listed above them as Unreleased.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if saveFlag {
			if err := runSave(); err != nil {
				ui.Error.Println(err)
				os.Exit(1)
			}
			return
		}
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

The first release describes the pack as it is, and keeps the version already in pack.toml.

A pack that isn't in a git repository can be released too, without the commits to read: nothing is committed, and the
release lists what has changed in the pack since the last one, which it keeps a record of for the next release.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if _, _, err := RunRelease(releaseVersionFlag, sinceFlag); err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
	},
}

// unreleasedChanges are the changes made since the last release, which CHANGELOG.md lists first as unreleased. Nothing is
// released or recorded for them. A pack that isn't in a repository can have them only if what it was at the last release is
// known, and if it isn't they aren't listed, which is said.
func unreleasedChanges(history History) ([]Change, error) {
	repo, err := openRepository()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		if err := checkWithoutRepository(history, ""); err != nil {
			notice.Infof("Not listing unreleased changes: %s.", err)
			return nil, nil
		}
	}
	p, err := loadPending(repo, false, "", true)
	if err != nil {
		return nil, err
	}
	return p.changes, nil
}

// saveMarkdown writes CHANGELOG.md from the releases recorded in the history, with the changes that aren't released yet
// listed above them, without releasing anything. What it says goes through notice.
func saveMarkdown() error {
	history, err := LoadHistory(historyPath())
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", HistoryFile, err)
	}
	unreleased, err := unreleasedChanges(history)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(markdownPath(), []byte(RenderMarkdownWithUnreleased(history.Releases, unreleased))); err != nil {
		return fmt.Errorf("failed to write %s: %w", MarkdownFile, err)
	}
	return nil
}

// SaveMarkdown writes CHANGELOG.md as "packwiz changelog --save" does, and returns where it wrote it and what it said along
// the way, in plain text. It says nothing on the terminal.
func SaveMarkdown() (path string, notices []string, err error) {
	collected := notice.Collect(func() { err = saveMarkdown() })
	if err != nil {
		return "", nil, err
	}
	for _, n := range collected {
		if n.Level != notice.Muted {
			notices = append(notices, n.Text)
		}
	}
	return markdownPath(), notices, nil
}

// runSave writes CHANGELOG.md from the releases recorded in the history, without releasing anything
func runSave() error {
	if err := saveMarkdown(); err != nil {
		return err
	}
	ui.Success.Printf("Wrote %s.\n", ui.Bold.Sprint(MarkdownFile))
	return nil
}

// Preview is the release that the pack's changes would make, for showing: nothing is committed or saved.
type Preview struct {
	// InRepository is whether the pack is in a repository, so that its changes are read from the log. If it isn't, Reason
	// says why, and they are found by comparing the pack with what it was at the last release.
	InRepository bool
	Reason       string
	// Last is the version of the last release, if there has been one
	Last string
	// HasChanges is whether there is anything to release. If there isn't, NoChanges says so, and Release is empty.
	HasChanges bool
	NoChanges  string
	Release    Release
	// Notes are things to know about how the version was worked out, such as that pack.toml has another version than the
	// last release
	Notes []string
	// Notices are what loading the pack had to say as it went, in plain text
	Notices []string

	release releasePlan
}

// releasePlan is what text of a release needs besides the release itself.
type releasePlan struct {
	last    string
	hasLast bool
	// packVersionNote is the note that pack.toml has a different version than the last release, if it does
	packVersionNote string
}

// Text is the release as "packwiz changelog" prints it: what version it would be, then what is in it, styled for the
// terminal it is shown on (see package ui).
func (p Preview) Text() string {
	if !p.HasChanges {
		return p.NoChanges + "\n"
	}
	return planText(p.Release, p.release)
}

// LoadPreview works out the release that the pack's changes would make, and says nothing on the terminal. since is the
// commit to read the log from, if it isn't where the last release was made, and version a version to use instead of the one
// that is worked out from the changes.
func LoadPreview(since, version string) (Preview, error) {
	var preview Preview
	var err error
	notices := notice.Collect(func() { preview, err = loadPreview(since, version) })
	if err != nil {
		return Preview{}, err
	}
	for _, n := range notices {
		// A pack that isn't in a repository is said by Reason
		if n.Level != notice.Muted && n.Text != notReadingLog(errors.New(preview.Reason)) {
			preview.Notices = append(preview.Notices, n.Text)
		}
	}
	return preview, nil
}

// notReadingLog is what is said when the pack isn't in a repository, so there is no log to read.
func notReadingLog(reason error) string {
	return fmt.Sprintf("Not reading the git log (%v).", reason)
}

func loadPreview(since, version string) (Preview, error) {
	repo, err := OpenRepository()
	preview := Preview{InRepository: err == nil}
	switch {
	case errors.Is(err, ErrNoRepository):
		notice.Infof("%s", notReadingLog(err))
		preview.Reason = err.Error()
		repo = nil
	case err != nil:
		return Preview{}, err
	}
	p, err := loadPending(repo, false, since, true)
	if err != nil {
		return Preview{}, err
	}
	preview.Last, preview.release.hasLast = "", false
	if last, ok := p.history.Latest(); ok {
		preview.Last, preview.release.last, preview.release.hasLast = last.Version, last.Version, true
	}
	if len(p.changes) == 0 {
		preview.NoChanges = p.noChangesMessage()
		return preview, nil
	}
	release, err := p.plan(version)
	if err != nil {
		return Preview{}, err
	}
	preview.HasChanges, preview.Release = true, release
	if preview.release.hasLast && p.pack.Version != "" && p.pack.Version != preview.release.last {
		preview.release.packVersionNote = fmt.Sprintf("Note: pack.toml has version %s, but the last release was %s; using the last release as the base.", p.pack.Version, preview.release.last)
		preview.Notes = append(preview.Notes, preview.release.packVersionNote)
	}
	return preview, nil
}

// runPreview prints the release the pack's changes would make, including those that haven't been committed yet,
// without changing anything. since is the commit to read the log from, if it isn't where the last release was made.
func runPreview(since string) error {
	preview, err := loadPreview(since, "")
	if err != nil {
		return err
	}
	if !preview.HasChanges {
		ui.Info.Println(preview.NoChanges)
		return nil
	}
	fmt.Print(preview.Text())
	return nil
}

// releaseHooks are what the command line does at points of making a release, which an interface of another kind has no use
// for: it asks its questions before it makes one, and has other ways of showing what was done.
type releaseHooks struct {
	// show is given the release before it is made
	show func(p pending, release Release)
	// confirm is asked whether to make it, and if it says no nothing is made. Nil is a yes.
	confirm func(release Release) bool
	// committed, if it isn't nil, is told of each commit made first, which are otherwise printed
	committed func(message string)
	// nothing, if it isn't nil, is told why there is nothing to release, which is otherwise printed
	nothing func(message string)
}

// RunRelease records the release that the commits made since the last one make, once the user confirms it. First it
// commits any changes to the pack that aren't committed yet, so they are in the log. A non-empty versionOverride
// replaces the version that would otherwise be worked out from the commits, and a non-empty since is the commit to
// read the log from, if it isn't where the last release was made. It returns the release and true, or false if no
// release was made because there was nothing to release or the user declined.
//
// A pack that isn't in a repository has no log, so nothing is committed and the release is what has changed in the
// pack since the last one, which the release keeps a record of.
func RunRelease(versionOverride, since string) (Release, bool, error) {
	release, released, err := makeRelease(versionOverride, since, releaseHooks{
		show: func(p pending, release Release) { fmt.Print(planText(release, p.describe())) },
		confirm: func(release Release) bool {
			if !cmdshared.PromptYesNo(fmt.Sprintf("Release %s? [Y/n]: ", release.Version)) {
				ui.Warning.Println("Cancelled!")
				return false
			}
			return true
		},
	})
	if released {
		ui.Success.Printf("Released %s!\n", ui.Bold.Sprint(release.Version))
	}
	return release, released, err
}

// Released is what making a release did.
type Released struct {
	// Made is whether a release was made: it isn't if there is nothing to release
	Made    bool
	Release Release
	// Committed are the messages of the commits that were made first, so that they are in the log the release was made from
	Committed []string
	// NoChanges says why nothing was released, if nothing was
	NoChanges string
	// Notices are what making it said along the way, in plain text
	Notices []string
}

// MakeRelease records the release that the pack's changes make, as "packwiz changelog release" does, without asking whether
// to: whoever calls it has. It says nothing on the terminal. See RunRelease for what it does and for versionOverride and since.
func MakeRelease(versionOverride, since string) (Released, error) {
	var result Released
	var err error
	notices := notice.Collect(func() {
		result.Release, result.Made, err = makeRelease(versionOverride, since, releaseHooks{
			committed: func(message string) { result.Committed = append(result.Committed, message) },
			nothing:   func(message string) { result.NoChanges = message },
		})
	})
	if err != nil {
		return Released{}, err
	}
	for _, n := range notices {
		if n.Level != notice.Muted {
			result.Notices = append(result.Notices, n.Text)
		}
	}
	return result, nil
}

func makeRelease(versionOverride, since string, hooks releaseHooks) (Release, bool, error) {
	repo, err := openRepository()
	if err != nil {
		return Release{}, false, err
	}
	if repo != nil {
		if reporter, ok := repo.(CommitReporter); ok && hooks.committed != nil {
			err = reporter.CommitPendingReporting(hooks.committed)
		} else {
			err = repo.CommitPending()
		}
		if err != nil {
			return Release{}, false, err
		}
	}
	p, err := loadPending(repo, true, since, false)
	if err != nil {
		return Release{}, false, err
	}
	if len(p.changes) == 0 {
		if hooks.nothing != nil {
			hooks.nothing(p.noChangesMessage())
		} else {
			notice.Infof("%s", p.noChangesMessage())
		}
		// Nothing to release, but past releases can still be made to show versions where they showed file names
		if p.upgraded > 0 {
			if err := p.save(p.history); err != nil {
				return Release{}, false, fmt.Errorf("failed to update the changelog: %w", err)
			}
			notice.Successf("Updated %d %s in the changelog.", p.upgraded, plural(p.upgraded, "line"))
		}
		return Release{}, false, nil
	}
	release, err := p.plan(versionOverride)
	if err != nil {
		return Release{}, false, err
	}
	if hooks.show != nil {
		hooks.show(p, release)
	}
	if hooks.confirm != nil && !hooks.confirm(release) {
		return Release{}, false, nil
	}
	if err := p.apply(release); err != nil {
		return Release{}, false, fmt.Errorf("failed to release: %w", err)
	}
	return release, true, nil
}

func init() {
	changelogCmd.AddCommand(releaseCmd)
	changelogCmd.PersistentFlags().StringVar(&sinceFlag, "since", "", "Read the commits made after this one (a hash, tag or branch), rather than after the last release")
	changelogCmd.Flags().BoolVar(&saveFlag, "save", false, "Write CHANGELOG.md from the recorded releases instead of previewing the next one")
	changelogCmd.MarkFlagsMutuallyExclusive("save", "since")
	releaseCmd.Flags().StringVar(&releaseVersionFlag, "version", "", "Release this version instead of the one worked out from the commits; it must be greater than the last release")

	cmd.Add(changelogCmd)
}

// openRepository opens the repository the pack is in. If it isn't in one it says so and returns nil, as a changelog is
// still made then, from the pack alone. Anything else that stops the repository being opened is an error.
func openRepository() (Repository, error) {
	repo, err := OpenRepository()
	if errors.Is(err, ErrNoRepository) {
		notice.Infof("%s", notReadingLog(err))
		return nil, nil
	}
	return repo, err
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
	pack  core.Pack
	index core.Index
	// history is the past releases, with any versions upgraded (see upgraded)
	history History
	// changes are what the commits since the last release changed, or without a repository what differs from the pack as
	// it was then; for the first release, they are what the pack contains
	changes []Change
	// current is the pack as it is now, which a release made without a repository keeps for the next one to compare with
	current Snapshot
	// versions are the versions found for mods that don't record one. A repository has them saved by committing (see
	// Repository.CommitPending), so without one a release saves them.
	versions map[string]string
	// inRepo is whether the pack is in a repository, and so whether changes were read from its log
	inRepo bool
	// head is the commit a release would be made at. It is only known when the changes have all been committed.
	head string
	// upgraded is how many lines of past releases showed a file name that has been replaced by a real version
	upgraded int
}

// loadPending reads the pack, and what has changed in it since its last release: the commits made since, or if repo is
// nil, because the pack isn't in a repository, how it differs from the pack as of that release. strict is whether
// failing to look up the versions of mods that don't record one is an error (see LoadWorking). withPending is whether
// to include the commits that "packwiz git commit" would make as well as those that have been made, for a preview.
func loadPending(repo Repository, strict bool, since string, withPending bool) (pending, error) {
	history, err := LoadHistory(historyPath())
	if err != nil {
		return pending{}, fmt.Errorf("failed to read %s: %w", HistoryFile, err)
	}
	// This can be told before the pack is loaded, which can need the network
	if repo == nil {
		if err := checkWithoutRepository(history, since); err != nil {
			return pending{}, err
		}
	}

	w, err := LoadWorking(strict)
	if err != nil {
		return pending{}, err
	}
	current, err := w.Snapshot()
	if err != nil {
		return pending{}, err
	}
	// Anything released before a mod's version was known was recorded by file name. That's only fixed in memory
	// here; it is written if a release is.
	upgraded := history.UpgradeVersions(current)
	p := pending{
		pack: w.Pack, index: w.Index, history: history, current: current, versions: w.Versions,
		inRepo: repo != nil, upgraded: upgraded,
	}

	switch _, released := history.Latest(); {
	case !released:
		// The first release describes the pack as it is. Its history begins with whatever was committed first, which
		// doesn't list the mods that were in it.
		p.changes = Diff(Snapshot{}, current)
	case repo == nil:
		p.changes = Diff(*history.Snapshot, current)
	default:
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

	if repo != nil && !withPending {
		if p.head, err = repo.Head(); err != nil {
			return pending{}, err
		}
	}
	return p, nil
}

// checkWithoutRepository fails if a pack that isn't in a repository can't have its changes worked out. There is no log
// to read, so a release after the first is compared with the snapshot the last one kept; a release made from the log
// didn't keep one, as there is nothing to say what the pack was like then.
func checkWithoutRepository(history History, since string) error {
	if since != "" {
		return errors.New("--since is a commit in the git log, but the pack isn't in a git repository")
	}
	if last, released := history.Latest(); released && history.Snapshot == nil {
		return fmt.Errorf("can't tell what has changed since release %s, which was made from the git log, as the pack isn't in a git repository", last.Version)
	}
	return nil
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

// describe is what the text of a release needs to know of what is pending.
func (p pending) describe() releasePlan {
	plan := releasePlan{}
	if last, ok := p.history.Latest(); ok {
		plan.last, plan.hasLast = last.Version, true
		if p.pack.Version != "" && p.pack.Version != last.Version {
			plan.packVersionNote = fmt.Sprintf("Note: pack.toml has version %s, but the last release was %s; using the last release as the base.", p.pack.Version, last.Version)
		}
	}
	return plan
}

// planText is what "packwiz changelog" says of a release before it is made: what version it would be and then what is in it.
func planText(release Release, plan releasePlan) string {
	var b strings.Builder
	if plan.hasLast {
		fmt.Fprintf(&b, "Changes since %s; next version is %s %s\n", plan.last, ui.Bold.Sprint(release.Version), ui.Muted.Sprintf("(%s bump)", release.Bump))
		if plan.packVersionNote != "" {
			b.WriteString(ui.Info.Sprint(plan.packVersionNote) + "\n")
		}
	} else {
		fmt.Fprintf(&b, "First release; version is %s\n", ui.Bold.Sprint(release.Version))
	}
	b.WriteString("\n")
	b.WriteString(renderRelease(release, style{colour: true}) + "\n")
	return b.String()
}

// apply records the release, as made at the commit that was current when it was worked out.
func (p pending) apply(release Release) error {
	release.Commit = p.head
	history := p.history
	history.Releases = append(slices.Clone(history.Releases), release)
	// The next release is read from the log if the pack is in a repository, so what it was like isn't kept. Otherwise it
	// is, and any snapshot that was there is out of date.
	history.Snapshot = nil
	if !p.inRepo {
		snapshot := p.current
		history.Snapshot = &snapshot
	}
	p.pack.Version = release.Version
	return p.save(history)
}

// save writes pack.toml and the release history, and without a repository the versions that were found and the index.
// The history file is written last because it is what marks a release as made: if any earlier step fails, running the
// release again works out and redoes the same release rather than skipping it.
func (p pending) save(history History) error {
	if !p.inRepo {
		// Nothing has committed these, as it does with a repository. The index was refreshed to find the changes being
		// released, and consumers of the pack (which see the pack.toml version) need it to match.
		if err := p.index.RecordVersions(p.versions); err != nil {
			return err
		}
		if err := p.index.Write(); err != nil {
			return err
		}
	}
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
