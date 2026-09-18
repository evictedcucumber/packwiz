package changelog

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	// HistoryFile is the machine-readable release history, stored next to pack.toml. It is the source of truth for
	// what the pack has released; MarkdownFile is rendered from it.
	HistoryFile = "changelog.toml"
	// MarkdownFile is the human-readable changelog, stored next to pack.toml. Both this and HistoryFile are excluded
	// from the pack's index by default (see core.ignoreDefaults), so they aren't distributed with the pack.
	MarkdownFile = "CHANGELOG.md"
)

// Release is one entry in a pack's release history.
type Release struct {
	Version string `toml:"version"`
	// Date is the day of the release, as YYYY-MM-DD
	Date string `toml:"date"`
	// Bump is how far this release raised the version; BumpNone for the first release, which has nothing to bump from
	Bump Bump `toml:"bump"`
	// Commit is the commit the release was made at. The next release covers the commits made after it.
	Commit  string   `toml:"commit,omitempty"`
	Changes []Change `toml:"change"`
}

// IsInitial reports whether this was the pack's first release, which has nothing to bump from.
func (r Release) IsInitial() bool {
	return r.Bump == BumpNone
}

// History is the contents of the history file.
type History struct {
	// Releases are in the order they were made, oldest first
	Releases []Release `toml:"release"`
}

// Latest returns the most recent release, if there has been one.
func (h History) Latest() (Release, bool) {
	if len(h.Releases) == 0 {
		return Release{}, false
	}
	return h.Releases[len(h.Releases)-1], true
}

// UpgradeVersions replaces versions that were recorded as a file name, because the mod's version wasn't known at
// the time, with the version the mod has in current. This is only done for a mod that is still on that file, where
// the version is certain. It returns how many lines of the releases were replaced.
func (h *History) UpgradeVersions(current Snapshot) int {
	upgraded := 0
	for i := range h.Releases {
		changes := h.Releases[i].Changes
		for j := range changes {
			c := &changes[j]
			cur, ok := current.Mods[c.Path]
			if !ok || !c.IsMod() || !knownVersion(cur) || c.To != cur.File {
				continue
			}
			c.To = cur.Version
			upgraded++
		}
	}
	return upgraded
}

// knownVersion reports whether a mod in a snapshot has a real version, rather than being described by its file name.
func knownVersion(m SnapshotMod) bool {
	return m.File != "" && m.Version != m.File
}

// LoadHistory reads the history file. A file that doesn't exist yet isn't an error: it is a pack with no releases.
func LoadHistory(path string) (History, error) {
	var h History
	if _, err := toml.DecodeFile(path, &h); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return History{}, nil
		}
		return History{}, err
	}
	return h, nil
}

// Write saves the history file.
func (h History) Write(path string) error {
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	// Disable indentation
	enc.Indent = ""
	if err := enc.Encode(h); err != nil {
		return err
	}
	return writeFileAtomic(path, buf.Bytes())
}

// writeFileAtomic replaces path with data, so a failure part way through can't leave a truncated file behind.
func writeFileAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// CreateTemp makes the file private; match what os.Create would have given a normal pack file
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
