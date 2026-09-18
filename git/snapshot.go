package git

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/core"
)

// snapshotAt describes the pack as it was at a commit, in the same terms as changelog.TakeSnapshot describes it now,
// so the two can be compared with changelog.Diff. indexFile is the path of the index relative to the pack root.
//
// Nothing is checked out: the index and each file it lists are read straight out of the commit. A commit from before
// the pack existed gives an empty snapshot.
func (r repo) snapshotAt(rev, indexFile string) (changelog.Snapshot, error) {
	// With no --full-name these are relative to the pack root, which is where every other path here is from
	out, err := r.run("", "ls-tree", "-r", "--name-only", "-z", rev)
	if err != nil {
		return changelog.Snapshot{}, err
	}
	present := make(map[string]bool)
	for _, name := range strings.Split(string(out), "\x00") {
		present[name] = true
	}
	if !present[indexFile] {
		return changelog.Snapshot{}, nil
	}

	indexData, err := r.show(rev, indexFile)
	if err != nil {
		return changelog.Snapshot{}, err
	}
	index, err := core.ParseIndex(indexData)
	if err != nil {
		return changelog.Snapshot{}, fmt.Errorf("failed to read %s at %s: %w", indexFile, rev, err)
	}

	// Paths in the index are relative to the index file, which needn't be in the pack root
	base := path.Dir(indexFile)
	open := func(p string) (io.ReadCloser, error) {
		full := path.Join(base, p)
		if !present[full] {
			return nil, fmt.Errorf("%s at %s: %w", full, rev, fs.ErrNotExist)
		}
		data, err := r.show(rev, full)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	// Old commits are read as they were committed: nothing is looked up for a mod that didn't record its version
	return changelog.TakeSnapshotFrom(index, open, nil)
}

// show returns the contents of a file, given its path relative to the pack root, as it was at a commit.
func (r repo) show(rev, file string) ([]byte, error) {
	// The "./" makes the path relative to the pack root rather than to the top of the repository
	return r.run("", "show", rev+":./"+file)
}
