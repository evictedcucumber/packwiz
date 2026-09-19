package git

import (
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"strings"

	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/viper"
)

// commitStep is one commit that packwiz git commit makes.
type commitStep struct {
	message string
	// change is the mod change the commit is for. The last commit, which takes everything else that changed, is for
	// none.
	change *changelog.Change
}

// planCommits works out the commits for changes to a pack: one for each mod that was added, updated or removed, in
// path order, and then one for everything else if there is any. There is something else if any other file changed, or
// if rest says so, as it does for changes that aren't in changes, such as pinning a mod.
func planCommits(changes []changelog.Change, rest bool) []commitStep {
	var steps []commitStep
	var others []changelog.Change
	for _, c := range changes {
		if !c.IsMod() {
			others = append(others, c)
			continue
		}
		c := c
		steps = append(steps, commitStep{message: Message([]changelog.Change{c}), change: &c})
	}
	if len(others) > 0 || rest {
		steps = append(steps, commitStep{message: Message(others)})
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
		packPath: filepath.ToSlash(filepath.Base(viper.GetString("pack-file"))),
	}

	hasCommits, err := r.hasCommits()
	if err != nil {
		return committer{}, nil, err
	}
	if !hasCommits {
		// The repository's first commit takes the whole pack: there is nothing before it for the pack to have changed from
		return c, []commitStep{{message: InitialMessage}}, nil
	}

	c.head, err = r.packAt("HEAD", indexPath, c.packPath)
	if err != nil {
		return committer{}, nil, fmt.Errorf("failed to read the pack as of the last commit: %w", err)
	}
	changes := changelog.Diff(c.head.Snapshot, current)
	rest, err := c.changedBeyond(changes)
	if err != nil {
		return committer{}, nil, err
	}
	return c, planCommits(changes, rest), nil
}

// runCommit commits every change under the pack root: one commit for each mod that was added, updated or removed, and
// then one for everything else. With dryRun, it only prints the commits it would make.
func runCommit(dryRun bool) error {
	c, steps, err := prepareCommit(dryRun)
	if err != nil {
		return err
	}
	if !dryRun {
		return c.run(steps)
	}

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
}

// modPath is the path of a mod's metadata file relative to the pack root, given the path the index knows it by.
func (c committer) modPath(indexPath string) string {
	return path.Join(path.Dir(c.indexPath), indexPath)
}

// changedBeyond reports whether anything other than the mod commits for changes would be left to commit: files that
// changed but weren't classified as changes, like a mod being pinned, or that are about to, like a version being saved
// to a mod that hasn't otherwise changed. (The index and pack.toml don't count if there are mod commits, as they
// change with each of them.)
func (c committer) changedBeyond(changes []changelog.Change) (bool, error) {
	dirty, err := c.r.dirtyPaths()
	if err != nil {
		return false, err
	}
	// A commit for each of these takes its mod file along with the index and pack.toml
	planned := make(map[string]bool)
	for _, change := range changes {
		if change.IsMod() {
			planned[c.modPath(change.Path)] = true
		}
	}
	for versioned := range c.w.Versions {
		dirty = append(dirty, c.modPath(versioned))
	}

	for _, p := range dirty {
		if planned[p] || (len(planned) > 0 && (p == c.indexPath || p == c.packPath)) {
			continue
		}
		return true, nil
	}
	return false, nil
}

// run makes the commits. Each commit is a pack that is consistent by itself, so that any commit can be checked out
// and used, or found to be the one that broke something: it holds the mod's file and an index and pack.toml that
// describe the pack as it is in that commit, which is the last commit's pack with only that mod changed.
func (c committer) run(steps []commitStep) error {
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
		printCommitted(step.message)
	}

	if err := c.writeIndexAndPack(c.w.Index.Files, c.w.Pack); err != nil {
		return err
	}
	dirty, err := c.r.dirty()
	if err != nil {
		return err
	}
	if !dirty {
		if made == 0 {
			ui.Info.Println("Nothing to commit.")
		}
		return nil
	}

	// Everything that's left, which is all of it if there were no mods to commit. There may be no step for it if
	// nothing had changed but the committed index was out of date, which isn't known until it has been rewritten.
	message := OtherMessage
	if n := len(steps); n > 0 && steps[n-1].change == nil {
		message = steps[n-1].message
	}
	if err := c.r.commitAll(message); err != nil {
		return fmt.Errorf("committed %d of %d %s, but couldn't commit %q: %w\nFix that and run \"packwiz git commit\" again to commit the rest",
			made, len(steps), plural(len(steps), "commit"), firstLine(message), err)
	}
	printCommitted(message)
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
