package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestIndexWriteLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.toml")

	idx := Index{
		HashFormat: "sha256",
		Files: IndexFiles{
			"mods/foo.pw.toml": &indexFile{File: "mods/foo.pw.toml", Hash: "abc123", MetaFile: true},
			"config/bar.txt":   &indexFile{File: "config/bar.txt", Hash: "def456", HashFormat: "md5"},
		},
		indexFile: indexPath,
		packRoot:  dir,
	}

	if err := idx.Write(); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}

	loaded, err := LoadIndex(indexPath)
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}

	if loaded.HashFormat != "sha256" {
		t.Errorf("loaded.HashFormat = %q, want %q", loaded.HashFormat, "sha256")
	}

	fooEntry, ok := loaded.Files["mods/foo.pw.toml"].(*indexFile)
	if !ok {
		t.Fatalf("expected mods/foo.pw.toml entry to be *indexFile, got %T", loaded.Files["mods/foo.pw.toml"])
	}
	if fooEntry.Hash != "abc123" {
		t.Errorf("foo hash = %q, want %q", fooEntry.Hash, "abc123")
	}
	if !fooEntry.MetaFile {
		t.Error("expected foo entry to be marked as MetaFile")
	}

	barEntry, ok := loaded.Files["config/bar.txt"].(*indexFile)
	if !ok {
		t.Fatalf("expected config/bar.txt entry to be *indexFile, got %T", loaded.Files["config/bar.txt"])
	}
	if barEntry.Hash != "def456" {
		t.Errorf("bar hash = %q, want %q", barEntry.Hash, "def456")
	}
	if barEntry.HashFormat != "md5" {
		t.Errorf("bar hash format = %q, want %q", barEntry.HashFormat, "md5")
	}
}

func TestIndexResolveAndRelPath(t *testing.T) {
	dir := t.TempDir()
	idx := Index{packRoot: dir}

	resolved := idx.ResolveIndexPath("mods/foo.jar")
	want := filepath.Join(dir, "mods", "foo.jar")
	if resolved != want {
		t.Errorf("ResolveIndexPath() = %q, want %q", resolved, want)
	}

	rel, err := idx.RelIndexPath(want)
	if err != nil {
		t.Fatalf("RelIndexPath() returned error: %v", err)
	}
	if rel != "mods/foo.jar" {
		t.Errorf("RelIndexPath() = %q, want %q", rel, "mods/foo.jar")
	}
}

func TestIndexRemoveFile(t *testing.T) {
	dir := t.TempDir()
	idx := Index{
		packRoot: dir,
		Files: IndexFiles{
			"mods/foo.jar": &indexFile{File: "mods/foo.jar"},
		},
	}

	fullPath := idx.ResolveIndexPath("mods/foo.jar")
	if err := idx.RemoveFile(fullPath); err != nil {
		t.Fatalf("RemoveFile() returned error: %v", err)
	}

	if _, ok := idx.Files["mods/foo.jar"]; ok {
		t.Error("expected mods/foo.jar to be removed from index")
	}
}

func TestIndexRefreshFileWithHashAddsNewEntry(t *testing.T) {
	dir := t.TempDir()
	idx := Index{HashFormat: "sha256", packRoot: dir, Files: IndexFiles{}}

	fullPath := idx.ResolveIndexPath("mods/foo.jar")
	if err := idx.RefreshFileWithHash(fullPath, "sha256", "somehash", false); err != nil {
		t.Fatalf("RefreshFileWithHash() returned error: %v", err)
	}

	entry, ok := idx.Files["mods/foo.jar"].(*indexFile)
	if !ok {
		t.Fatalf("expected new entry to be *indexFile, got %T", idx.Files["mods/foo.jar"])
	}
	if entry.Hash != "somehash" {
		t.Errorf("Hash = %q, want %q", entry.Hash, "somehash")
	}
	// Same format as index HashFormat should be normalised to ""
	if entry.HashFormat != "" {
		t.Errorf("HashFormat = %q, want empty (matches index-level format)", entry.HashFormat)
	}
	if !entry.markedFound() {
		t.Error("expected new entry to be marked found")
	}
	if entry.IsMetaFile() {
		t.Error("expected new entry not to be a meta file")
	}
}

func TestIndexRefreshFileWithHashUpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	idx := Index{HashFormat: "sha256", packRoot: dir, Files: IndexFiles{}}
	fullPath := idx.ResolveIndexPath("mods/foo.jar")

	if err := idx.RefreshFileWithHash(fullPath, "sha256", "hash1", false); err != nil {
		t.Fatalf("RefreshFileWithHash() returned error: %v", err)
	}
	if err := idx.RefreshFileWithHash(fullPath, "sha256", "hash2", false); err != nil {
		t.Fatalf("RefreshFileWithHash() returned error: %v", err)
	}

	entry := idx.Files["mods/foo.jar"].(*indexFile)
	if entry.Hash != "hash2" {
		t.Errorf("Hash = %q, want %q", entry.Hash, "hash2")
	}
}

func TestIndexRefreshFileWithHashMarkAsMetaFileSticky(t *testing.T) {
	dir := t.TempDir()
	idx := Index{HashFormat: "sha256", packRoot: dir, Files: IndexFiles{}}
	fullPath := idx.ResolveIndexPath("mods/foo.pw.toml")

	if err := idx.RefreshFileWithHash(fullPath, "sha256", "hash1", true); err != nil {
		t.Fatalf("RefreshFileWithHash() returned error: %v", err)
	}
	entry := idx.Files["mods/foo.pw.toml"].(*indexFile)
	if !entry.IsMetaFile() {
		t.Fatal("expected entry to be marked as meta file")
	}

	// Calling again with markAsMetaFile=false must not unset the existing meta status
	if err := idx.RefreshFileWithHash(fullPath, "sha256", "hash2", false); err != nil {
		t.Fatalf("RefreshFileWithHash() returned error: %v", err)
	}
	entry = idx.Files["mods/foo.pw.toml"].(*indexFile)
	if !entry.IsMetaFile() {
		t.Error("expected meta file status to remain set after update with markAsMetaFile=false")
	}
}

func TestIndexRefreshFileWithHashRespectsNoInternalHashes(t *testing.T) {
	old := viper.GetBool("no-internal-hashes")
	viper.Set("no-internal-hashes", true)
	t.Cleanup(func() { viper.Set("no-internal-hashes", old) })

	dir := t.TempDir()
	idx := Index{HashFormat: "sha256", packRoot: dir, Files: IndexFiles{}}
	fullPath := idx.ResolveIndexPath("mods/foo.jar")

	if err := idx.RefreshFileWithHash(fullPath, "sha256", "somehash", false); err != nil {
		t.Fatalf("RefreshFileWithHash() returned error: %v", err)
	}

	entry := idx.Files["mods/foo.jar"].(*indexFile)
	if entry.Hash != "" {
		t.Errorf("Hash = %q, want empty when no-internal-hashes is set", entry.Hash)
	}
}

func TestIndexFindMod(t *testing.T) {
	dir := t.TempDir()
	idx := Index{
		packRoot: dir,
		Files: IndexFiles{
			"mods/foo.pw.toml": &indexFile{File: "mods/foo.pw.toml", MetaFile: true},
			"mods/bar.toml":    &indexFile{File: "mods/bar.toml", MetaFile: true},
			"mods/baz.jar":     &indexFile{File: "mods/baz.jar", MetaFile: false},
		},
	}

	path, found := idx.FindMod("foo")
	if !found {
		t.Error("expected to find mod 'foo'")
	}
	if path != idx.ResolveIndexPath("mods/foo.pw.toml") {
		t.Errorf("FindMod('foo') path = %q, want %q", path, idx.ResolveIndexPath("mods/foo.pw.toml"))
	}

	path, found = idx.FindMod("bar")
	if !found {
		t.Error("expected to find mod 'bar' (old .toml extension)")
	}
	if path != idx.ResolveIndexPath("mods/bar.toml") {
		t.Errorf("FindMod('bar') path = %q, want %q", path, idx.ResolveIndexPath("mods/bar.toml"))
	}

	// baz is not a meta file, so it should never be found by name
	_, found = idx.FindMod("baz")
	if found {
		t.Error("did not expect to find 'baz' since it is not a meta file")
	}

	_, found = idx.FindMod("nonexistent")
	if found {
		t.Error("did not expect to find 'nonexistent'")
	}
}

func TestIndexRefresh(t *testing.T) {
	dir := t.TempDir()

	packFile := filepath.Join(dir, "pack.toml")
	indexFilePath := filepath.Join(dir, "index.toml")

	mustWriteFile(t, packFile, "name = \"Test\"\n")
	mustWriteFile(t, indexFilePath, "")
	mustWriteFile(t, filepath.Join(dir, "mods", "test.jar"), "fake jar content")
	mustWriteFile(t, filepath.Join(dir, "config", "config.txt"), "fake config")
	mustWriteFile(t, filepath.Join(dir, ".packwizignore"), "ignored/**\n")
	mustWriteFile(t, filepath.Join(dir, "ignored", "file.txt"), "should be ignored")

	oldPackFile := viper.GetString("pack-file")
	viper.Set("pack-file", packFile)
	oldNoHashes := viper.GetBool("no-internal-hashes")
	viper.Set("no-internal-hashes", false)
	t.Cleanup(func() {
		viper.Set("pack-file", oldPackFile)
		viper.Set("no-internal-hashes", oldNoHashes)
	})

	idx := Index{
		HashFormat: "sha256",
		indexFile:  indexFilePath,
		packRoot:   dir,
		Files: IndexFiles{
			// Entry for a file that no longer exists on disk - should be dropped
			"old/removed.txt": &indexFile{File: "old/removed.txt"},
		},
	}

	if err := idx.Refresh(); err != nil {
		t.Fatalf("Refresh() returned error: %v", err)
	}

	if _, ok := idx.Files["mods/test.jar"]; !ok {
		t.Error("expected mods/test.jar to be added to the index")
	}
	if _, ok := idx.Files["config/config.txt"]; !ok {
		t.Error("expected config/config.txt to be added to the index")
	}
	if _, ok := idx.Files["ignored/file.txt"]; ok {
		t.Error("expected ignored/file.txt to be excluded per .packwizignore")
	}
	if _, ok := idx.Files["pack.toml"]; ok {
		t.Error("expected pack.toml to be excluded from the index")
	}
	if _, ok := idx.Files["index.toml"]; ok {
		t.Error("expected index.toml to be excluded from the index")
	}
	if _, ok := idx.Files[".packwizignore"]; ok {
		t.Error("expected .packwizignore itself to be excluded from the index")
	}
	if _, ok := idx.Files["old/removed.txt"]; ok {
		t.Error("expected old/removed.txt to be dropped since it no longer exists on disk")
	}

	// Verify the hash was actually computed correctly for a real file
	testJarEntry, ok := idx.Files["mods/test.jar"].(*indexFile)
	if !ok {
		t.Fatalf("expected mods/test.jar to be *indexFile, got %T", idx.Files["mods/test.jar"])
	}
	h, err := GetHashImpl("sha256")
	if err != nil {
		t.Fatalf("GetHashImpl() returned error: %v", err)
	}
	if _, err := h.Write([]byte("fake jar content")); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	wantHash := h.HashToString(h.Sum(nil))
	if testJarEntry.Hash != wantHash {
		t.Errorf("mods/test.jar hash = %q, want %q", testJarEntry.Hash, wantHash)
	}
}

func TestIndexRefreshSkipsSymlinkedDirectory(t *testing.T) {
	dir := t.TempDir()

	packFile := filepath.Join(dir, "pack.toml")
	indexFilePath := filepath.Join(dir, "index.toml")

	mustWriteFile(t, packFile, "name = \"Test\"\n")
	mustWriteFile(t, indexFilePath, "")
	mustWriteFile(t, filepath.Join(dir, "mods", "test.jar"), "fake jar content")

	// A symlink pointing at a directory, not covered by any ignore
	// pattern. WalkDir reports this as a non-directory entry, so
	// without special handling it gets opened as a regular file and
	// fails with "is a directory".
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(dir, "linked-dir")); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	oldPackFile := viper.GetString("pack-file")
	viper.Set("pack-file", packFile)
	oldNoHashes := viper.GetBool("no-internal-hashes")
	viper.Set("no-internal-hashes", false)
	t.Cleanup(func() {
		viper.Set("pack-file", oldPackFile)
		viper.Set("no-internal-hashes", oldNoHashes)
	})

	idx := Index{
		HashFormat: "sha256",
		indexFile:  indexFilePath,
		packRoot:   dir,
		Files:      IndexFiles{},
	}

	if err := idx.Refresh(); err != nil {
		t.Fatalf("Refresh() returned error: %v", err)
	}

	if _, ok := idx.Files["mods/test.jar"]; !ok {
		t.Error("expected mods/test.jar to be added to the index")
	}
	if _, ok := idx.Files["linked-dir"]; ok {
		t.Error("expected linked-dir to be excluded since it resolves to a directory")
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func TestIndexFilesToMemoryRepAndBackSingle(t *testing.T) {
	rep := indexFilesTomlRepresentation{
		{File: "mods/b.jar", Hash: "hb"},
		{File: "mods/a.jar", Hash: "ha"},
	}

	mem := rep.toMemoryRep()
	if len(mem) != 2 {
		t.Fatalf("toMemoryRep() len = %d, want 2", len(mem))
	}

	aEntry, ok := mem["mods/a.jar"].(*indexFile)
	if !ok {
		t.Fatalf("expected mods/a.jar entry to be *indexFile, got %T", mem["mods/a.jar"])
	}
	if aEntry.Hash != "ha" {
		t.Errorf("mods/a.jar hash = %q, want %q", aEntry.Hash, "ha")
	}

	back := mem.toTomlRep()
	if len(back) != 2 {
		t.Fatalf("toTomlRep() len = %d, want 2", len(back))
	}
	// Sorted by File
	if back[0].File != "mods/a.jar" || back[1].File != "mods/b.jar" {
		t.Errorf("toTomlRep() order = [%q, %q], want [mods/a.jar, mods/b.jar]", back[0].File, back[1].File)
	}
}

func TestIndexFilesMultipleAliasRoundTrip(t *testing.T) {
	rep := indexFilesTomlRepresentation{
		{File: "mods/a.jar", Alias: "alias2", Hash: "h2"},
		{File: "mods/a.jar", Alias: "alias1", Hash: "h1"},
		{File: "mods/b.jar", Hash: "h3"},
	}

	mem := rep.toMemoryRep()
	if len(mem) != 2 {
		t.Fatalf("toMemoryRep() len = %d, want 2", len(mem))
	}

	aliasMap, ok := mem["mods/a.jar"].(*indexFileMultipleAlias)
	if !ok {
		t.Fatalf("expected mods/a.jar entry to be *indexFileMultipleAlias, got %T", mem["mods/a.jar"])
	}
	if len(*aliasMap) != 2 {
		t.Fatalf("expected 2 aliases for mods/a.jar, got %d", len(*aliasMap))
	}
	if (*aliasMap)["alias1"].Hash != "h1" {
		t.Errorf("alias1 hash = %q, want %q", (*aliasMap)["alias1"].Hash, "h1")
	}
	if (*aliasMap)["alias2"].Hash != "h2" {
		t.Errorf("alias2 hash = %q, want %q", (*aliasMap)["alias2"].Hash, "h2")
	}

	bEntry, ok := mem["mods/b.jar"].(*indexFile)
	if !ok {
		t.Fatalf("expected mods/b.jar entry to be *indexFile, got %T", mem["mods/b.jar"])
	}
	if bEntry.Hash != "h3" {
		t.Errorf("mods/b.jar hash = %q, want %q", bEntry.Hash, "h3")
	}

	// Round trip back to TOML representation: sorted by File then Alias
	back := mem.toTomlRep()
	if len(back) != 3 {
		t.Fatalf("toTomlRep() len = %d, want 3", len(back))
	}
	if back[0].File != "mods/a.jar" || back[0].Alias != "alias1" {
		t.Errorf("back[0] = %+v, want File=mods/a.jar Alias=alias1", back[0])
	}
	if back[1].File != "mods/a.jar" || back[1].Alias != "alias2" {
		t.Errorf("back[1] = %+v, want File=mods/a.jar Alias=alias2", back[1])
	}
	if back[2].File != "mods/b.jar" {
		t.Errorf("back[2] = %+v, want File=mods/b.jar", back[2])
	}
}

func TestIndexFilesAliasEmptyStringNormalisedToClean(t *testing.T) {
	rep := indexFilesTomlRepresentation{
		{File: "mods/a.jar", Alias: ""},
	}
	mem := rep.toMemoryRep()
	entry, ok := mem["mods/a.jar"].(*indexFile)
	if !ok {
		t.Fatalf("expected mods/a.jar entry to be *indexFile, got %T", mem["mods/a.jar"])
	}
	if entry.Alias != "" {
		t.Errorf("Alias = %q, want empty string (not '.')", entry.Alias)
	}
}
