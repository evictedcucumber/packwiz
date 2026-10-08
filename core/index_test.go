package core

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

	path, found = idx.FindMod("foo.pw.toml")
	if !found {
		t.Error("expected to find mod 'foo.pw.toml' (name with extension)")
	}
	if path != idx.ResolveIndexPath("mods/foo.pw.toml") {
		t.Errorf("FindMod('foo.pw.toml') path = %q, want %q", path, idx.ResolveIndexPath("mods/foo.pw.toml"))
	}

	path, found = idx.FindMod("bar.toml")
	if !found {
		t.Error("expected to find mod 'bar.toml' (old extension, given explicitly)")
	}
	if path != idx.ResolveIndexPath("mods/bar.toml") {
		t.Errorf("FindMod('bar.toml') path = %q, want %q", path, idx.ResolveIndexPath("mods/bar.toml"))
	}
}

func TestIndexFindModByPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "mods"), 0755); err != nil {
		t.Fatal(err)
	}
	idx := Index{
		packRoot: dir,
		Files: IndexFiles{
			"mods/foo.pw.toml": &indexFile{File: "mods/foo.pw.toml", MetaFile: true},
		},
	}
	wantPath := idx.ResolveIndexPath("mods/foo.pw.toml")

	path, found := idx.FindMod(wantPath)
	if !found {
		t.Error("expected to find mod by its absolute path")
	}
	if path != wantPath {
		t.Errorf("FindMod(absolute path) = %q, want %q", path, wantPath)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(origWd); err != nil {
			t.Fatal(err)
		}
	}()

	path, found = idx.FindMod(filepath.Join("mods", "foo.pw.toml"))
	if !found {
		t.Error("expected to find mod by a path relative to the working directory")
	}
	if path != wantPath {
		t.Errorf("FindMod(relative path) = %q, want %q", path, wantPath)
	}

	if err := os.Chdir(filepath.Join(dir, "mods")); err != nil {
		t.Fatal(err)
	}
	path, found = idx.FindMod("foo.pw.toml")
	if !found {
		t.Error("expected to find mod by its bare file name when the working directory is its containing folder")
	}
	if path != wantPath {
		t.Errorf("FindMod(bare file name from containing folder) = %q, want %q", path, wantPath)
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

// The list of mods that `packwiz list --save` writes is made from the pack, so it isn't part of what is distributed
func TestIndexRefreshDoesNotTrackTheModList(t *testing.T) {
	dir := t.TempDir()

	indexFilePath := filepath.Join(dir, "index.toml")
	mustWriteFile(t, filepath.Join(dir, "pack.toml"), "name = \"Test\"\n")
	mustWriteFile(t, indexFilePath, "")
	mustWriteFile(t, filepath.Join(dir, "mods", "test.jar"), "fake jar content")
	mustWriteFile(t, filepath.Join(dir, ModListFile), "# Test\n")
	mustWriteFile(t, filepath.Join(dir, "docs", ModListFile), "# Test\n")

	oldPackFile := viper.GetString("pack-file")
	viper.Set("pack-file", filepath.Join(dir, "pack.toml"))
	oldNoHashes := viper.GetBool("no-internal-hashes")
	viper.Set("no-internal-hashes", false)
	t.Cleanup(func() {
		viper.Set("pack-file", oldPackFile)
		viper.Set("no-internal-hashes", oldNoHashes)
	})

	idx := Index{HashFormat: "sha256", indexFile: indexFilePath, packRoot: dir, Files: IndexFiles{}}
	if err := idx.Refresh(); err != nil {
		t.Fatalf("Refresh() returned error: %v", err)
	}

	if _, ok := idx.Files["mods/test.jar"]; !ok {
		t.Error("expected mods/test.jar to be added to the index")
	}
	for _, file := range []string{ModListFile, "docs/" + ModListFile} {
		if _, ok := idx.Files[file]; ok {
			t.Errorf("expected %s to be excluded from the index", file)
		}
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

func TestIndexLoadAllMods(t *testing.T) {
	dir := t.TempDir()

	modPath := filepath.Join(dir, "mods", "foo.pw.toml")
	if err := os.MkdirAll(filepath.Dir(modPath), 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}
	if err := os.WriteFile(modPath, []byte(`name = "Foo"
filename = "foo.jar"

[download]
hash-format = "sha256"
hash = "abc123"
`), 0644); err != nil {
		t.Fatalf("failed to write mod fixture: %v", err)
	}

	idx := Index{
		HashFormat: "sha256",
		Files: IndexFiles{
			"mods/foo.pw.toml": &indexFile{File: "mods/foo.pw.toml", MetaFile: true},
			"config/bar.txt":   &indexFile{File: "config/bar.txt"}, // not a metafile, must be skipped
		},
		packRoot: dir,
	}

	mods, err := idx.LoadAllMods()
	if err != nil {
		t.Fatalf("LoadAllMods() returned error: %v", err)
	}
	if len(mods) != 1 {
		t.Fatalf("got %d mods, want 1 (non-metafile entries must be excluded)", len(mods))
	}
	if mods[0].Name != "Foo" {
		t.Errorf("mods[0].Name = %q, want %q", mods[0].Name, "Foo")
	}
}

func TestIndexLoadAllModsMissingFile(t *testing.T) {
	dir := t.TempDir()
	idx := Index{
		Files: IndexFiles{
			"mods/missing.pw.toml": &indexFile{File: "mods/missing.pw.toml", MetaFile: true},
		},
		packRoot: dir,
	}

	if _, err := idx.LoadAllMods(); err == nil {
		t.Error("expected an error when a metafile referenced by the index is missing, got nil")
	}
}

func TestParseIndex(t *testing.T) {
	idx, err := ParseIndex([]byte(`hash-format = "sha512"

[[files]]
file = "mods/foo.pw.toml"
hash = "abc123"
metafile = true

[[files]]
file = "config/bar.txt"
hash = "def456"
`))
	if err != nil {
		t.Fatalf("ParseIndex() returned error: %v", err)
	}
	if idx.HashFormat != "sha512" {
		t.Errorf("HashFormat = %q, want %q", idx.HashFormat, "sha512")
	}
	if len(idx.Files) != 2 {
		t.Fatalf("Files has %d entries, want 2: %v", len(idx.Files), idx.Files)
	}
	if !idx.Files["mods/foo.pw.toml"].IsMetaFile() {
		t.Error("mods/foo.pw.toml should be a metafile")
	}
	if idx.Files["config/bar.txt"].IsMetaFile() {
		t.Error("config/bar.txt should not be a metafile")
	}
}

func TestParseIndexDefaultsHashFormat(t *testing.T) {
	idx, err := ParseIndex([]byte(""))
	if err != nil {
		t.Fatalf("ParseIndex() returned error: %v", err)
	}
	if idx.HashFormat != "sha256" {
		t.Errorf("HashFormat = %q, want the sha256 default", idx.HashFormat)
	}
}

func TestParseIndexRejectsInvalidTOML(t *testing.T) {
	if _, err := ParseIndex([]byte("this is [not toml")); err == nil {
		t.Error("ParseIndex() accepted invalid TOML")
	}
}

func TestParseIndexAgreesWithLoadIndex(t *testing.T) {
	content := []byte(`hash-format = "sha256"

[[files]]
file = "mods/foo.pw.toml"
metafile = true

[[files]]
file = "config/bar.txt"
`)
	path := filepath.Join(t.TempDir(), "index.toml")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	loaded, err := LoadIndex(path)
	if err != nil {
		t.Fatalf("LoadIndex() returned error: %v", err)
	}
	parsed, err := ParseIndex(content)
	if err != nil {
		t.Fatalf("ParseIndex() returned error: %v", err)
	}
	if len(loaded.Files) != len(parsed.Files) {
		t.Fatalf("LoadIndex() found %d files, ParseIndex() %d", len(loaded.Files), len(parsed.Files))
	}
	for p, f := range loaded.Files {
		if other, ok := parsed.Files[p]; !ok || other.IsMetaFile() != f.IsMetaFile() {
			t.Errorf("%s differs between LoadIndex() and ParseIndex()", p)
		}
	}
}

// RefreshQuietly is for a caller that owns the screen, so it must not write a progress bar to stdout as Refresh does
func TestIndexRefreshQuietlyWritesNothingToStdout(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "pack.toml"), "name = \"Test\"\n")
	mustWriteFile(t, filepath.Join(dir, "index.toml"), "")
	mustWriteFile(t, filepath.Join(dir, "config", "a.json"), "{}")
	mustWriteFile(t, filepath.Join(dir, "config", "b.json"), "{}")

	oldPackFile := viper.GetString("pack-file")
	viper.Set("pack-file", filepath.Join(dir, "pack.toml"))
	t.Cleanup(func() { viper.Set("pack-file", oldPackFile) })

	idx := Index{HashFormat: "sha256", indexFile: filepath.Join(dir, "index.toml"), packRoot: dir, Files: IndexFiles{}}

	// A pipe stands in for stdout; mpb looks stdout up when the bar is made, so it is swapped before Refresh
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() returned error: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w
	_, refreshErr := idx.RefreshQuietly()
	os.Stdout = oldStdout
	_ = w.Close()

	if refreshErr != nil {
		t.Fatalf("RefreshQuietly() returned error: %v", refreshErr)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading stdout returned error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("RefreshQuietly() wrote %q to stdout, want nothing", out)
	}
	for _, p := range []string{"config/a.json", "config/b.json"} {
		if _, ok := idx.Files[p]; !ok {
			t.Errorf("expected %s to be added to the index", p)
		}
	}
}

func TestIndexSaveModWritesTheFileAndRecordsItsHash(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "index.toml"), "")

	idx := Index{HashFormat: "sha256", indexFile: filepath.Join(dir, "index.toml"), packRoot: dir, Files: IndexFiles{}}
	mod := &Mod{Name: "Alpha", FileName: "alpha.jar"}
	mod.SetMetaPath(filepath.Join(dir, "mods", "alpha.pw.toml"))
	mod.ClaimConfigFile("config/alpha.json")

	if err := idx.SaveMod(mod); err != nil {
		t.Fatalf("SaveMod() returned error: %v", err)
	}

	saved, err := LoadMod(mod.GetFilePath())
	if err != nil {
		t.Fatalf("the mod file wasn't written: %v", err)
	}
	if saved.ConfigFiles == nil || !slices.Equal(*saved.ConfigFiles, []string{"config/alpha.json"}) {
		t.Errorf("saved config-files = %v, want [config/alpha.json]", saved.ConfigFiles)
	}

	entry, ok := idx.Files["mods/alpha.pw.toml"]
	if !ok {
		t.Fatalf("mods/alpha.pw.toml isn't in the index: %v", idx.Files)
	}
	if !entry.IsMetaFile() {
		t.Error("the mod's file should be marked as a metadata file")
	}
	data, err := os.ReadFile(mod.GetFilePath())
	if err != nil {
		t.Fatalf("reading the mod file returned error: %v", err)
	}
	h, err := GetHashImpl("sha256")
	if err != nil {
		t.Fatalf("GetHashImpl() returned error: %v", err)
	}
	_, _ = h.Write(data)
	if got := entry.(*indexFile).Hash; got != h.HashToString(h.Sum(nil)) {
		t.Errorf("the index records hash %q, but the file's is %q", got, h.HashToString(h.Sum(nil)))
	}
}

// searchFixture makes an index of mods that each have a metadata file with a name, at mods/<slug>.pw.toml
func searchFixture(t *testing.T, mods map[string]string) *Index {
	t.Helper()
	dir := t.TempDir()
	idx := &Index{HashFormat: "sha256", packRoot: dir, Files: IndexFiles{}}
	for slug, name := range mods {
		rel := "mods/" + slug + ".pw.toml"
		mustWriteFile(t, filepath.Join(dir, filepath.FromSlash(rel)), modTOML(name, "", "", slug))
		idx.Files[rel] = &indexFile{File: rel, MetaFile: true}
	}
	// A file that isn't a metadata file is never a mod, whatever it is called
	idx.Files["mods/sodium-decoy.jar"] = &indexFile{File: "mods/sodium-decoy.jar"}
	return idx
}

func slugsOf(matches []ModMatch) []string {
	slugs := make([]string, len(matches))
	for i, m := range matches {
		slugs[i] = m.Slug
	}
	return slugs
}

func TestIndexSearchModsMatchesFuzzilyByNameOrSlug(t *testing.T) {
	idx := searchFixture(t, map[string]string{
		"sodium": "Sodium", "sodium-extra": "Sodium Extra", "iris": "Iris Shaders", "zzz-long-slug": "Plain",
	})

	for _, tt := range []struct {
		text string
		want []string
	}{
		{"sdm", []string{"sodium", "sodium-extra"}}, // characters in order, not together
		{"sod ex", []string{"sodium-extra"}},        // every word, in any order
		{"EXTRA sod", []string{"sodium-extra"}},
		{"iris shaders", []string{"iris"}},      // by name, which isn't the slug
		{"longslug", []string{"zzz-long-slug"}}, // by slug, when the name hasn't it
		{"nothing", nil},
	} {
		got, err := idx.SearchMods(tt.text)
		if err != nil {
			t.Fatalf("SearchMods(%q) returned error: %v", tt.text, err)
		}
		if !slices.Equal(slugsOf(got), tt.want) {
			t.Errorf("SearchMods(%q) = %v, want %v", tt.text, slugsOf(got), tt.want)
		}
	}
}

func TestIndexSearchModsGivesWhereTheModIsAndWhatItIsCalled(t *testing.T) {
	idx := searchFixture(t, map[string]string{"sodium-extra": "Sodium Extra"})
	got, err := idx.SearchMods("sodex")
	if err != nil || len(got) != 1 {
		t.Fatalf("SearchMods() = %v, %v, want the one mod", got, err)
	}
	m := got[0]
	if m.Name != "Sodium Extra" || m.Slug != "sodium-extra" || m.Path != idx.ResolveIndexPath("mods/sodium-extra.pw.toml") || m.Score <= 0 {
		t.Errorf("match = %+v, want the mod's name, slug and file, and a score", m)
	}
}

// The best match is first, then the shorter name, then the file, which is what makes the order the same every time although
// the index is a map
func TestIndexSearchModsIsOrderedBestFirstAndAlwaysTheSame(t *testing.T) {
	idx := searchFixture(t, map[string]string{
		"amod": "Amod", "zed": "Zed Mod", // the start of a word is a better match than the middle of one
		"b": "B Sodium", "a": "A Sodium Plus", // as good, so the shorter name
		"c2": "Sodium C", "c1": "Sodium C", // as good and as long, so the file
	})
	if got, _ := idx.SearchMods("mod"); !slices.Equal(slugsOf(got), []string{"zed", "amod"}) {
		t.Errorf("mod finds %v, want the better match first", slugsOf(got))
	}
	for range 20 { // a map is in a different order each time
		got, _ := idx.SearchMods("sodium")
		if !slices.Equal(slugsOf(got), []string{"b", "c1", "c2", "a"}) {
			t.Fatalf("sodium finds %v, want the shorter names first, and the files in order when they are the same", slugsOf(got))
		}
	}
}

func TestIndexSearchModsFindsNothingByNothing(t *testing.T) {
	idx := searchFixture(t, map[string]string{"sodium": "Sodium"})
	for _, text := range []string{"", "   "} {
		if got, err := idx.SearchMods(text); err != nil || len(got) != 0 {
			t.Errorf("SearchMods(%q) = %v, %v, want nothing: there is nothing to search for", text, got, err)
		}
	}
}

func TestIndexSearchModsFailsForAMetadataFileThatCantBeRead(t *testing.T) {
	idx := searchFixture(t, map[string]string{"sodium": "Sodium"})
	idx.Files["mods/gone.pw.toml"] = &indexFile{File: "mods/gone.pw.toml", MetaFile: true}
	if _, err := idx.SearchMods("sod"); err == nil || !strings.Contains(err.Error(), "gone.pw.toml") {
		t.Errorf("SearchMods() returned %v, want an error that says which file couldn't be read", err)
	}
}

func TestTheFilesThatDescribeThePackAreNotInItsIndex(t *testing.T) {
	// DocFiles are exported without the index, so refreshing mustn't pick them up as well: they would be distributed by
	// the launcher, claimed as config files and counted as changes to the pack
	dir := t.TempDir()
	packFile := filepath.Join(dir, "pack.toml")
	indexFilePath := filepath.Join(dir, "index.toml")
	mustWriteFile(t, packFile, "name = \"Test\"\n")
	mustWriteFile(t, indexFilePath, "")
	mustWriteFile(t, filepath.Join(dir, "config", "config.txt"), "fake config")
	for _, name := range DocFiles {
		mustWriteFile(t, filepath.Join(dir, name), "contents of "+name)
	}

	oldPackFile := viper.GetString("pack-file")
	viper.Set("pack-file", packFile)
	t.Cleanup(func() { viper.Set("pack-file", oldPackFile) })

	idx := Index{HashFormat: "sha256", indexFile: indexFilePath, packRoot: dir, Files: IndexFiles{}}
	if err := idx.Refresh(); err != nil {
		t.Fatalf("Refresh() returned error: %v", err)
	}

	if _, ok := idx.Files["config/config.txt"]; !ok {
		t.Error("expected config/config.txt to be added to the index")
	}
	for _, name := range DocFiles {
		if _, ok := idx.Files[name]; ok {
			t.Errorf("expected %s to be left out of the index", name)
		}
	}
}

func TestTheServerPacksFilesAreNotInTheIndex(t *testing.T) {
	// They are only for the server pack, so a launcher mustn't install them on a client, and an exported server pack
	// mustn't be put in the next one
	dir := t.TempDir()
	packFile := filepath.Join(dir, "pack.toml")
	indexFilePath := filepath.Join(dir, "index.toml")
	mustWriteFile(t, packFile, "name = \"Test\"\n")
	mustWriteFile(t, indexFilePath, "")
	mustWriteFile(t, filepath.Join(dir, "config", "config.txt"), "fake config")
	mustWriteFile(t, filepath.Join(dir, ServerConfigDir, "server.properties"), "motd=hi")
	mustWriteFile(t, filepath.Join(dir, ServerConfigDir, "config", "config.txt"), "server config")
	mustWriteFile(t, filepath.Join(dir, "Test-1.0.0"+ServerPackSuffix), "a zip")
	mustWriteFile(t, filepath.Join(dir, "Test-1.0.0"+BisectPackSuffix), "a zip")

	oldPackFile := viper.GetString("pack-file")
	viper.Set("pack-file", packFile)
	t.Cleanup(func() { viper.Set("pack-file", oldPackFile) })

	idx := Index{HashFormat: "sha256", indexFile: indexFilePath, packRoot: dir, Files: IndexFiles{}}
	if err := idx.Refresh(); err != nil {
		t.Fatalf("Refresh() returned error: %v", err)
	}
	if _, ok := idx.Files["config/config.txt"]; !ok {
		t.Error("expected config/config.txt to be added to the index")
	}
	for p := range idx.Files {
		if p != "config/config.txt" {
			t.Errorf("expected %s to be left out of the index", p)
		}
	}
}
