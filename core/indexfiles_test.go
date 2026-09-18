package core

import "testing"

func TestIndexFileStateTransitions(t *testing.T) {
	f := &indexFile{File: "mods/foo.pw.toml"}

	if f.markedFound() {
		t.Error("markedFound() = true before markFound() was called")
	}
	f.markFound()
	if !f.markedFound() {
		t.Error("markedFound() = false after markFound() was called")
	}

	if f.IsMetaFile() {
		t.Error("IsMetaFile() = true before markMetaFile() was called")
	}
	f.markMetaFile()
	if !f.IsMetaFile() {
		t.Error("IsMetaFile() = false after markMetaFile() was called")
	}

	f.updateHash("abc123", "sha256")
	if f.Hash != "abc123" || f.HashFormat != "sha256" {
		t.Errorf("got Hash=%q HashFormat=%q, want Hash=abc123 HashFormat=sha256", f.Hash, f.HashFormat)
	}
}

func TestIndexFileMultipleAliasStateTransitions(t *testing.T) {
	m := indexFileMultipleAlias{
		"":    indexFile{File: "mods/foo.pw.toml"},
		"bar": indexFile{File: "mods/foo.pw.toml", Alias: "bar"},
	}

	if m.markedFound() {
		t.Error("markedFound() = true before markFound() was called")
	}
	m.markFound()
	for alias, entry := range m {
		if !entry.markedFound() {
			t.Errorf("entry %q: markedFound() = false after markFound() was called", alias)
		}
	}

	if m.IsMetaFile() {
		t.Error("IsMetaFile() = true before markMetaFile() was called")
	}
	m.markMetaFile()
	for alias, entry := range m {
		if !entry.IsMetaFile() {
			t.Errorf("entry %q: IsMetaFile() = false after markMetaFile() was called", alias)
		}
	}

	m.updateHash("def456", "md5")
	for alias, entry := range m {
		if entry.Hash != "def456" || entry.HashFormat != "md5" {
			t.Errorf("entry %q: got Hash=%q HashFormat=%q, want Hash=def456 HashFormat=md5", alias, entry.Hash, entry.HashFormat)
		}
	}
}

func TestIndexFileMultipleAliasPanicsWhenEmpty(t *testing.T) {
	m := indexFileMultipleAlias{}

	assertPanics := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: expected a panic on an empty indexFileMultipleAlias, got none", name)
			}
		}()
		fn()
	}

	assertPanics("markedFound", func() { m.markedFound() })
	assertPanics("IsMetaFile", func() { m.IsMetaFile() })
}

func TestUpdateFileEntryCreatesNewEntry(t *testing.T) {
	var files IndexFiles
	files.updateFileEntry("mods/foo.pw.toml", "sha256", "abc123", true)

	entry, ok := files["mods/foo.pw.toml"].(*indexFile)
	if !ok {
		t.Fatalf("expected *indexFile, got %T", files["mods/foo.pw.toml"])
	}
	if entry.Hash != "abc123" || entry.HashFormat != "sha256" {
		t.Errorf("got Hash=%q HashFormat=%q, want Hash=abc123 HashFormat=sha256", entry.Hash, entry.HashFormat)
	}
	if !entry.MetaFile {
		t.Error("MetaFile = false, want true")
	}
	if !entry.markedFound() {
		t.Error("new entry should be marked found")
	}
}

func TestUpdateFileEntryUpdatesExistingEntryWithoutResettingMetaFile(t *testing.T) {
	files := IndexFiles{
		"config/bar.txt": &indexFile{File: "config/bar.txt", Hash: "old", MetaFile: true},
	}

	// markAsMetaFile=false on an existing metafile must not clear MetaFile.
	files.updateFileEntry("config/bar.txt", "md5", "new", false)

	entry := files["config/bar.txt"].(*indexFile)
	if entry.Hash != "new" || entry.HashFormat != "md5" {
		t.Errorf("got Hash=%q HashFormat=%q, want Hash=new HashFormat=md5", entry.Hash, entry.HashFormat)
	}
	if !entry.MetaFile {
		t.Error("MetaFile = false, want existing metafile status to be preserved")
	}
	if !entry.markedFound() {
		t.Error("updated entry should be marked found")
	}
}
