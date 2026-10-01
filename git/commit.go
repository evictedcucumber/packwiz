package git

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// commitStep is one commit that packwiz git commit makes.
type commitStep struct {
	message string
	// change is the change the commit is for: a mod or a config file. The last commit, which takes what changed
	// without being a change to either, is for none.
	change *changelog.Change
}

// planCommits works out the commits for changes to a pack: one for each mod or file that was added, updated, changed
// or removed, in path order, and then one for what else changed in the pack's own files if there is any. There is
// something else if rest says so, as it does for changes that aren't in changes, such as pinning a mod.
func planCommits(changes []changelog.Change, rest bool) []commitStep {
	var steps []commitStep
	// Mods come first, each group in path order
	for _, mods := range []bool{true, false} {
		for _, c := range changes {
			if c.IsMod() != mods {
				continue
			}
			c := c
			steps = append(steps, commitStep{message: Message([]changelog.Change{c}), change: &c})
		}
	}
	if rest {
		steps = append(steps, commitStep{message: OtherMessage})
	}
	return steps
}

// prepareCommit works out the commits that packwiz git commit would make. With dryRun, failing to look up the versions
// of mods that don't record one isn't an error, as they are only needed to describe commits that won't be made.
func prepareCommit(dryRun bool) (committer, []commitStep, error) {
	// Before anything else, as there is no point loading the pack, which can need the network, to say there's nowhere
	// to commit it
	r, err := openRepo(packRoot())
	if err != nil {
		return committer{}, nil, err
	}
	// Committing saves the versions looked up for mods that don't record one, so a failure to look them up mustn't
	// be papered over
	w, err := changelog.LoadWorking(!dryRun)
	if err != nil {
		return committer{}, nil, err
	}
	current, err := w.Snapshot()
	if err != nil {
		return committer{}, nil, err
	}
	indexPath, err := indexFile(w.Pack)
	if err != nil {
		return committer{}, nil, err
	}
	c := committer{
		r: r, w: w, indexPath: indexPath,
		packPath: packFile(),
	}
	if c.unclaimed, err = unclaimedConfig(w); err != nil {
		return committer{}, nil, err
	}

	hasCommits, err := r.hasCommits()
	if err != nil {
		return committer{}, nil, err
	}
	if !hasCommits {
		// The repository's first commit takes the whole pack: there is nothing before it for the pack to have changed from
		if _, c.unknown, err = c.dirtyFiles(nil); err != nil {
			return committer{}, nil, err
		}
		return c, []commitStep{{message: InitialMessage}}, nil
	}

	c.head, err = r.packAt("HEAD", indexPath, c.packPath)
	if err != nil {
		return committer{}, nil, fmt.Errorf("failed to read the pack as of the last commit: %w", err)
	}
	changes := changelog.Diff(c.head.Snapshot, current)
	rest, unknown, err := c.changedBeyond(changes)
	if err != nil {
		return committer{}, nil, err
	}
	c.unknown = unknown
	return c, planCommits(changes, rest), nil
}

// runCommit commits every change to the files the pack tracks: one commit for each mod or file that was added, updated,
// changed or removed, and then one for what else changed in the pack's own files. Files packwiz doesn't recognise are
// left alone. With dryRun, it only prints the commits it would make.
func runCommit(dryRun bool) error {
	c, steps, err := prepareCommit(dryRun)
	if err != nil {
		return err
	}
	if !dryRun {
		c.warnUnknown()
		return c.run(steps, printCommitted)
	}

	c.warnUnknown()
	if len(steps) == 0 {
		ui.Info.Println("Nothing to commit.")
		return nil
	}
	messages := make([]string, len(steps))
	for i, step := range steps {
		messages[i] = styleMessage(step.message)
	}
	fmt.Println(strings.Join(messages, "\n"+ui.Muted.Sprint("---")+"\n"))
	return nil
}

// PlanCommit works out the commits that "packwiz git commit" would make, as their messages, without making any and without
// saying anything on the terminal. There are none if there is nothing to commit. It fails if the pack isn't in a repository.
func PlanCommit() (messages []string, notices []string, err error) {
	collected := notice.Collect(func() {
		var c committer
		var steps []commitStep
		if c, steps, err = prepareCommit(true); err != nil {
			return
		}
		c.warnUnknown()
		for _, step := range steps {
			messages = append(messages, step.message)
		}
	})
	return messages, plainNotices(collected), err
}

// CommitAll commits the pack as "packwiz git commit" does, without saying anything on the terminal, and returns the messages
// of the commits that were made.
func CommitAll() (committed []string, notices []string, err error) {
	collected := notice.Collect(func() {
		var c committer
		var steps []commitStep
		if c, steps, err = prepareCommit(false); err != nil {
			return
		}
		c.warnUnknown()
		err = c.run(steps, func(message string) { committed = append(committed, message) })
	})
	return committed, plainNotices(collected), err
}

// plainNotices are what was said that is worth reading, as text.
func plainNotices(collected []notice.Notice) []string {
	var texts []string
	for _, n := range collected {
		if n.Level != notice.Muted {
			texts = append(texts, n.Text)
		}
	}
	return texts
}

// styleMessage picks out the subject of a commit message, which is its first line
func styleMessage(message string) string {
	subject, body, hasBody := strings.Cut(message, "\n")
	if !hasBody {
		return ui.Bold.Sprint(subject)
	}
	return ui.Bold.Sprint(subject) + "\n" + body
}

// printCommitted says that a commit was made, showing the first line of its message
func printCommitted(message string) {
	fmt.Println(ui.Success.Sprint("Committed:"), firstLine(message))
}

// pendingCommits describes the commits that runCommit would make, as the changelog reads them, without making any.
func pendingCommits() ([]changelog.Commit, error) {
	_, steps, err := prepareCommit(true)
	if err != nil {
		return nil, err
	}
	commits := make([]changelog.Commit, len(steps))
	for i, step := range steps {
		subject, body, _ := strings.Cut(step.message, "\n")
		commits[i] = changelog.Commit{Subject: subject, Body: strings.TrimSpace(body)}
	}
	return commits, nil
}

// committer makes the commits for a pack.
type committer struct {
	r repo
	w changelog.Working
	// indexPath and packPath are the paths of the index and of pack.toml, relative to the pack root
	indexPath, packPath string
	// head is the pack as of the last commit, if there is one
	head committedPack
	// unknown are the files under the pack root that have changed but that packwiz doesn't recognise, which are left
	// alone: they aren't part of the pack, so it isn't for packwiz to say what they are. Paths are relative to the
	// pack root.
	unknown []string
	// unclaimed are the config files that the index tracks but that no mod, the mod loader or the pack claims. They are
	// committed, as they are part of the pack, but the pack is in a bad state with them, so it can't be released.
	unclaimed []string
}

// unclaimedConfig are the tracked config files that nothing claims, as paths relative to the index.
func unclaimedConfig(w changelog.Working) ([]string, error) {
	mods, err := w.Index.LoadAllMods()
	if err != nil {
		return nil, err
	}
	tree, err := w.Index.ConfigFileTree(mods, w.Pack)
	if err != nil {
		return nil, fmt.Errorf("failed to work out who owns the config files: %w", err)
	}
	return tree.Unclaimed, nil
}

// modPath is the path of a mod's metadata file or of a config file relative to the pack root, given the path the index
// knows it by.
func (c committer) modPath(indexPath string) string {
	return path.Join(path.Dir(c.indexPath), indexPath)
}

// known are the paths, relative to the pack root, of the files that packwiz recognises: the ones the index lists (now
// and as of the last commit, so a file that has been removed is still known), the index and pack.toml, and the files
// that packwiz writes about the pack (its changelog and its list of mods).
func (c committer) known() map[string]bool {
	known := map[string]bool{
		c.indexPath:            true,
		c.packPath:             true,
		changelog.HistoryFile:  true,
		changelog.MarkdownFile: true,
		core.ModListFile:       true,
		core.IgnoreFile:        true,
	}
	for p := range c.w.Index.Files {
		known[c.modPath(p)] = true
	}
	for p := range c.head.Index.Files {
		known[c.modPath(p)] = true
	}
	return known
}

// dirtyFiles are the files under the pack root that differ from the last commit, divided into those packwiz recognises
// and those it doesn't. The files that versions were looked up for are among the ones recognised, as they are about to
// differ.
func (c committer) dirtyFiles(versions map[string]string) (known, unknown []string, err error) {
	dirty, err := c.r.dirtyPaths()
	if err != nil {
		return nil, nil, err
	}
	for versioned := range versions {
		dirty = append(dirty, c.modPath(versioned))
	}
	recognised := c.known()
	for _, p := range dirty {
		if recognised[p] {
			known = append(known, p)
		} else {
			unknown = append(unknown, p)
		}
	}
	slices.Sort(unknown)
	return slices.Compact(known), slices.Compact(unknown), nil
}

// warnUnknown says which files are being left alone because packwiz doesn't recognise them, and which config files
// nothing claims.
func (c committer) warnUnknown() {
	if len(c.unknown) > 0 {
		notice.Warnf("Leaving %d %s that packwiz doesn't recognise uncommitted: %s",
			len(c.unknown), plural(len(c.unknown), "file"), strings.Join(c.unknown, ", "))
	}
	if len(c.unclaimed) > 0 {
		notice.Warnf("%d config %s claimed by any mod, the mod loader or the pack: %s",
			len(c.unclaimed), claimedVerb(len(c.unclaimed)), strings.Join(c.unclaimed, ", "))
	}
}

// errBadState is the error for a release when the pack is in a state it can't be released from: there are files that
// packwiz doesn't recognise, which would be left out of it and which it can't say don't belong in it, or config files
// that nothing claims, which are part of the pack but belong to nothing. It is nil if there are none.
func (c committer) errBadState() error {
	var problems []string
	if n := len(c.unknown); n > 0 {
		problems = append(problems, fmt.Sprintf("the pack's folder has %d %s that packwiz doesn't recognise: %s\nCommit %s with git, or ignore %s in .gitignore",
			n, plural(n, "file"), strings.Join(c.unknown, ", "), pronoun(n, "it", "them"), pronoun(n, "it", "them")))
	}
	if n := len(c.unclaimed); n > 0 {
		problems = append(problems, fmt.Sprintf("%d config %s claimed by any mod, the mod loader or the pack: %s\nClaim %s with \"packwiz config relate\", or delete %s",
			n, claimedVerb(n), strings.Join(c.unclaimed, ", "), pronoun(n, "it", "them"), pronoun(n, "it", "them")))
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("can't release: %s\nThen release again", strings.Join(problems, "\n\n"))
}

// claimedVerb is "file isn't" or "files aren't", as the start of a sentence about config files that nothing claims.
func claimedVerb(n int) string {
	if n == 1 {
		return "file isn't"
	}
	return "files aren't"
}

func pronoun(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// changedBeyond reports whether anything other than the commits for changes would be left to commit among the files
// packwiz recognises: files that changed but weren't classified as changes, like a mod being pinned, or that are about
// to, like a version being saved to a mod that hasn't otherwise changed. (The index and pack.toml don't count if there
// are commits for changes, as they change with each of them.) It also returns the files that changed that packwiz
// doesn't recognise.
func (c committer) changedBeyond(changes []changelog.Change) (rest bool, unknown []string, err error) {
	dirty, unknown, err := c.dirtyFiles(c.w.Versions)
	if err != nil {
		return false, nil, err
	}
	// A commit for each of these takes its file along with the index and pack.toml
	planned := make(map[string]bool)
	for _, change := range changes {
		planned[c.modPath(change.Path)] = true
	}

	for _, p := range dirty {
		if planned[p] || (len(planned) > 0 && (p == c.indexPath || p == c.packPath)) {
			continue
		}
		return true, unknown, nil
	}
	return false, unknown, nil
}

// run makes the commits. Each commit is a pack that is consistent by itself, so that any commit can be checked out
// and used, or found to be the one that broke something: it holds the mod's file and an index and pack.toml that
// describe the pack as it is in that commit, which is the last commit's pack with only that mod changed.
func (c committer) run(steps []commitStep, report func(message string)) error {
	// Versions belong in the commit of the mod they were looked up for, so they're in the files before any commit
	if err := c.w.Index.RecordVersions(c.w.Versions); err != nil {
		return err
	}

	files := maps.Clone(c.head.Index.Files)
	if files == nil {
		files = make(core.IndexFiles)
	}
	basePack := c.w.Pack
	if c.head.Pack != nil {
		// pack.toml keeps what it had, apart from the index it describes: anything else that was changed in it is
		// committed with everything else, not with whichever mod happens to come first
		basePack = *c.head.Pack
		basePack.Index.File = c.w.Pack.Index.File
	}

	made := 0
	for _, step := range steps {
		if step.change == nil {
			continue
		}
		change := *step.change

		if entry, ok := c.w.Index.Files[change.Path]; ok {
			files[change.Path] = entry
		} else {
			delete(files, change.Path)
		}
		err := c.writeIndexAndPack(files, basePack)
		if err == nil {
			err = c.r.commitPaths(step.message, c.modPath(change.Path), c.indexPath, c.packPath)
		}
		if err != nil {
			// Leave the index and pack.toml as they should end up, not as they were for this commit
			_ = c.writeIndexAndPack(c.w.Index.Files, c.w.Pack)
			return fmt.Errorf("committed %d of %d %s, but couldn't commit %q: %w\nFix that and run \"packwiz git commit\" again to commit the rest",
				made, len(steps), plural(len(steps), "commit"), firstLine(step.message), err)
		}
		made++
		report(step.message)
	}

	if err := c.writeIndexAndPack(c.w.Index.Files, c.w.Pack); err != nil {
		return err
	}
	left, _, err := c.dirtyFiles(nil)
	if err != nil {
		return err
	}
	if len(left) == 0 {
		if made == 0 {
			notice.Infof("Nothing to commit.")
		}
		return nil
	}

	// What's left of the files packwiz recognises, which is all of them if there were no changes to commit. There may be
	// no step for it if nothing had changed but the committed index was out of date, which isn't known until it has
	// been rewritten.
	message := OtherMessage
	if n := len(steps); n > 0 && steps[n-1].change == nil {
		message = steps[n-1].message
	}
	if err := c.r.commitPaths(message, left...); err != nil {
		return fmt.Errorf("committed %d of %d %s, but couldn't commit %q: %w\nFix that and run \"packwiz git commit\" again to commit the rest",
			made, len(steps), plural(len(steps), "commit"), firstLine(message), err)
	}
	report(message)
	return nil
}

// writeIndexAndPack writes the index with the given files, and the pack file with the hash of that index.
func (c committer) writeIndexAndPack(files core.IndexFiles, pack core.Pack) error {
	index := c.w.Index
	index.Files = files
	if err := index.Write(); err != nil {
		return err
	}
	if err := pack.UpdateIndexHash(); err != nil {
		return err
	}
	return pack.Write()
}
