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

// committedPack is a pack as it was at a commit.
type committedPack struct {
	// Snapshot describes its contents in the same terms as changelog.TakeSnapshot describes them now, so the two can be
	// compared with changelog.Diff. It is empty for a commit from before the pack existed.
	Snapshot changelog.Snapshot
	// Index is the index as it was committed, with no files if the commit had none.
	Index core.Index
	// Pack is the pack file as it was committed, if the commit had one.
	Pack *core.Pack
}

// packAt reads the pack as it was at a commit. indexFile and packFile are the paths of the index and of pack.toml
// relative to the pack root.
//
// Nothing is checked out: the pack file, the index and each file the index lists are read straight out of the commit.
func (r repo) packAt(rev, indexFile, packFile string) (committedPack, error) {
	// With no --full-name these are relative to the pack root, which is where every other path here is from
	out, err := r.run("", "ls-tree", "-r", "--name-only", "-z", rev)
	if err != nil {
		return committedPack{}, err
	}
	present := make(map[string]bool)
	for _, name := range strings.Split(string(out), "\x00") {
		present[name] = true
	}

	var committed committedPack
	if present[packFile] {
		data, err := r.show(rev, packFile)
		if err != nil {
			return committedPack{}, err
		}
		pack, err := core.ParsePack(data)
		if err != nil {
			return committedPack{}, fmt.Errorf("failed to read %s at %s: %w", packFile, rev, err)
		}
		committed.Pack = &pack
	}
	if !present[indexFile] {
		return committed, nil
	}

	indexData, err := r.show(rev, indexFile)
	if err != nil {
		return committedPack{}, err
	}
	index, err := core.ParseIndex(indexData)
	if err != nil {
		return committedPack{}, fmt.Errorf("failed to read %s at %s: %w", indexFile, rev, err)
	}
	committed.Index = index

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
	committed.Snapshot, err = changelog.TakeSnapshotFrom(index, open, nil)
	if err != nil {
		return committedPack{}, err
	}
	return committed, nil
}

// show returns the contents of a file, given its path relative to the pack root, as it was at a commit.
func (r repo) show(rev, file string) ([]byte, error) {
	// The "./" makes the path relative to the pack root rather than to the top of the repository
	return r.run("", "show", rev+":./"+file)
}
