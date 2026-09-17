package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/viper"
)

// writeCacheFile writes content to the correct cache-layout location
// (cachePath/hash[:2]/hash[2:]) under the sha256 cache hash format, and
// returns the computed sha256 hash string.
func writeCacheFile(t *testing.T, cachePath string, content []byte) string {
	t.Helper()
	h, err := GetHashImpl(cacheHashFormat)
	if err != nil {
		t.Fatalf("GetHashImpl() returned error: %v", err)
	}
	if _, err := h.Write(content); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	hashStr := h.HashToString(h.Sum(nil))

	dir := filepath.Join(cachePath, hashStr[:2])
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, hashStr[2:]), content, 0644); err != nil {
		t.Fatalf("failed to write cache file: %v", err)
	}
	return hashStr
}

func hashOf(t *testing.T, hashType string, content []byte) string {
	t.Helper()
	h, err := GetHashImpl(hashType)
	if err != nil {
		t.Fatalf("GetHashImpl(%q) returned error: %v", hashType, err)
	}
	if _, err := h.Write(content); err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	return h.HashToString(h.Sum(nil))
}

func TestCacheIndexGetHashesMap(t *testing.T) {
	c := CacheIndex{Hashes: map[string][]string{
		"sha256": {"a", "b", ""},
		"md5":    {"x", "y"},
	}}

	got := c.getHashesMap(0)
	if got["sha256"] != "a" || got["md5"] != "x" {
		t.Errorf("getHashesMap(0) = %v, want sha256=a md5=x", got)
	}

	got = c.getHashesMap(2)
	if len(got) != 0 {
		t.Errorf("getHashesMap(2) = %v, want empty (sha256[2] is empty, md5 has no index 2)", got)
	}
}

func TestCacheIndexGetHandleFromHash(t *testing.T) {
	c := CacheIndex{Hashes: map[string][]string{
		"sha256": {"aaa", "bbb"},
		"md5":    {"111", "222"},
	}}

	handle := c.GetHandleFromHash("sha256", "AAA")
	if handle == nil {
		t.Fatal("expected handle for case-insensitive match, got nil")
	}
	if handle.hashIdx != 0 {
		t.Errorf("hashIdx = %d, want 0", handle.hashIdx)
	}
	if handle.Hashes["md5"] != "111" {
		t.Errorf("Hashes[md5] = %q, want %q", handle.Hashes["md5"], "111")
	}

	if h := c.GetHandleFromHash("sha256", "zzz"); h != nil {
		t.Error("expected nil handle for unknown hash")
	}
	if h := c.GetHandleFromHash("unknownformat", "aaa"); h != nil {
		t.Error("expected nil handle for unknown hash format")
	}
}

func TestCacheIndexGetHandleFromHashForceRehash(t *testing.T) {
	cachePath := t.TempDir()
	content := []byte("cache content for rehash test")
	sha256Hash := writeCacheFile(t, cachePath, content)
	md5Hash := hashOf(t, "md5", content)

	c := CacheIndex{cachePath: cachePath, Hashes: map[string][]string{"sha256": {sha256Hash}}}

	handle, err := c.GetHandleFromHashForce("md5", md5Hash)
	if err != nil {
		t.Fatalf("GetHandleFromHashForce() returned error: %v", err)
	}
	if handle == nil {
		t.Fatal("expected a handle, got nil")
	}
	if handle.Hashes["md5"] != md5Hash {
		t.Errorf("Hashes[md5] = %q, want %q", handle.Hashes["md5"], md5Hash)
	}
	if handle.Hashes["sha256"] != sha256Hash {
		t.Errorf("Hashes[sha256] = %q, want %q", handle.Hashes["sha256"], sha256Hash)
	}
	// The rehashed value should be persisted back into the index
	if c.Hashes["md5"][0] != md5Hash {
		t.Errorf("c.Hashes[md5][0] = %q, want %q", c.Hashes["md5"][0], md5Hash)
	}
}

func TestCacheIndexGetHandleFromHashForceNoMatch(t *testing.T) {
	cachePath := t.TempDir()
	content := []byte("some other content")
	sha256Hash := writeCacheFile(t, cachePath, content)

	c := CacheIndex{cachePath: cachePath, Hashes: map[string][]string{"sha256": {sha256Hash}}}

	handle, err := c.GetHandleFromHashForce("md5", strings.Repeat("f", 32))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handle != nil {
		t.Errorf("expected nil handle for non-matching hash, got %+v", handle)
	}
}

func TestCacheIndexGetHandleFromHashForceAlreadyPresent(t *testing.T) {
	cachePath := t.TempDir()
	content := []byte("already indexed content")
	sha256Hash := writeCacheFile(t, cachePath, content)
	md5Hash := hashOf(t, "md5", content)

	c := CacheIndex{cachePath: cachePath, Hashes: map[string][]string{
		"sha256": {sha256Hash},
		"md5":    {md5Hash},
	}}

	handle, err := c.GetHandleFromHashForce("md5", md5Hash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handle == nil {
		t.Fatal("expected a handle, got nil")
	}
	if handle.hashIdx != 0 {
		t.Errorf("hashIdx = %d, want 0", handle.hashIdx)
	}
}

func TestCacheIndexNewHandleFromHashesNew(t *testing.T) {
	c := CacheIndex{Hashes: map[string][]string{"sha256": {}}}

	handle, alreadyExists := c.NewHandleFromHashes(map[string]string{"sha256": "newhash"})
	if alreadyExists {
		t.Error("expected alreadyExists = false for a brand new hash")
	}
	if handle.hashIdx != 0 {
		t.Errorf("hashIdx = %d, want 0", handle.hashIdx)
	}
	if c.nextHashIdx != 1 {
		t.Errorf("nextHashIdx = %d, want 1", c.nextHashIdx)
	}
}

func TestCacheIndexNewHandleFromHashesExisting(t *testing.T) {
	c := CacheIndex{Hashes: map[string][]string{"sha256": {"existinghash"}}}

	handle, alreadyExists := c.NewHandleFromHashes(map[string]string{
		"sha256": "existinghash",
		"md5":    "extramd5",
	})
	if !alreadyExists {
		t.Error("expected alreadyExists = true for a known hash")
	}
	if handle.hashIdx != 0 {
		t.Errorf("hashIdx = %d, want 0", handle.hashIdx)
	}
	if handle.Hashes["md5"] != "extramd5" {
		t.Errorf("Hashes[md5] = %q, want %q", handle.Hashes["md5"], "extramd5")
	}
}

func TestCacheIndexNewHandleFromHashesPanicsWithoutCacheFormat(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when hashes map lacks the cache hash format")
		}
	}()
	c := CacheIndex{Hashes: map[string][]string{}}
	c.NewHandleFromHashes(map[string]string{"md5": "abc"})
}

func TestCacheIndexHandleUpdateIndex(t *testing.T) {
	c := CacheIndex{Hashes: map[string][]string{}}
	handle := &CacheIndexHandle{index: &c, hashIdx: 0, Hashes: map[string]string{"sha256": "h1", "md5": "m1"}}

	warnings := handle.UpdateIndex()
	if len(warnings) != 0 {
		t.Errorf("expected no warnings on first update, got %v", warnings)
	}
	if c.Hashes["sha256"][0] != "h1" {
		t.Errorf("c.Hashes[sha256][0] = %q, want %q", c.Hashes["sha256"][0], "h1")
	}
	if c.Hashes["md5"][0] != "m1" {
		t.Errorf("c.Hashes[md5][0] = %q, want %q", c.Hashes["md5"][0], "m1")
	}

	// Simulate an inconsistent write at the same index
	handle2 := &CacheIndexHandle{index: &c, hashIdx: 0, Hashes: map[string]string{"sha256": "different"}}
	warnings = handle2.UpdateIndex()
	if len(warnings) == 0 {
		t.Error("expected a warning for inconsistent hash overwrite")
	}
	if c.Hashes["sha256"][0] != "different" {
		t.Errorf("c.Hashes[sha256][0] = %q, want %q (should be overwritten)", c.Hashes["sha256"][0], "different")
	}
}

func TestCacheIndexHandleRemove(t *testing.T) {
	c := CacheIndex{Hashes: map[string][]string{
		"sha256": {"h0", "h1", "h2"},
		"md5":    {"m0", "m1", "m2"},
	}}
	handle := &CacheIndexHandle{index: &c, hashIdx: 1, Hashes: map[string]string{"sha256": "h1", "md5": "m1"}}

	handle.Remove()

	if len(c.Hashes["sha256"]) != 2 || c.Hashes["sha256"][0] != "h0" || c.Hashes["sha256"][1] != "h2" {
		t.Errorf("c.Hashes[sha256] = %v, want [h0 h2]", c.Hashes["sha256"])
	}
	if len(c.Hashes["md5"]) != 2 || c.Hashes["md5"][0] != "m0" || c.Hashes["md5"][1] != "m2" {
		t.Errorf("c.Hashes[md5] = %v, want [m0 m2]", c.Hashes["md5"])
	}
}

func TestCacheIndexHandlePath(t *testing.T) {
	cachePath := t.TempDir()
	c := CacheIndex{cachePath: cachePath}
	fullHash := "abcdef1234567890"
	handle := &CacheIndexHandle{index: &c, Hashes: map[string]string{"sha256": fullHash}}

	got := handle.Path()
	want := filepath.Join(cachePath, fullHash[:2], fullHash[2:])
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestCacheIndexHandleCreateFromTemp(t *testing.T) {
	cachePath := t.TempDir()
	c := CacheIndex{cachePath: cachePath}

	content := []byte("temp file content")
	sha256Hash := hashOf(t, "sha256", content)
	handle := &CacheIndexHandle{index: &c, Hashes: map[string]string{"sha256": sha256Hash}}

	tempFile, err := os.CreateTemp(cachePath, "tmp-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := tempFile.Write(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tempPath := tempFile.Name()

	f, err := handle.CreateFromTemp(tempFile)
	if err != nil {
		t.Fatalf("CreateFromTemp() returned error: %v", err)
	}
	defer f.Close()

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("failed to read moved file: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Errorf("moved file content = %q, want %q", data, content)
	}
	if f.Name() != handle.Path() {
		t.Errorf("moved file path = %q, want %q", f.Name(), handle.Path())
	}
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Error("expected the original temp file to no longer exist after rename")
	}
}

func TestCacheIndexUpdateVersionRemovesEmptyFile(t *testing.T) {
	cachePath := t.TempDir()

	validContent := []byte("valid cached content")
	validHash := writeCacheFile(t, cachePath, validContent)

	// Zero-size file (broken download from version 1)
	emptyHash := hashOf(t, "sha256", []byte{})
	emptyDir := filepath.Join(cachePath, emptyHash[:2])
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(emptyDir, emptyHash[2:]), []byte{}, 0644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}

	c := CacheIndex{
		Version:   1,
		cachePath: cachePath,
		Hashes: map[string][]string{
			cacheHashFormat: {emptyHash, validHash},
			"md5":           {"m-empty", "m-valid"},
		},
	}

	c.updateVersion()

	if c.Version != 2 {
		t.Errorf("Version = %d, want 2", c.Version)
	}
	if got := c.Hashes[cacheHashFormat]; len(got) != 1 || got[0] != validHash {
		t.Errorf("Hashes[%s] = %v, want [%s]", cacheHashFormat, got, validHash)
	}
	if got := c.Hashes["md5"]; len(got) != 1 || got[0] != "m-valid" {
		t.Errorf("Hashes[md5] = %v, want [m-valid]", got)
	}
}

func TestCacheIndexUpdateVersionRemovesMissingFile(t *testing.T) {
	cachePath := t.TempDir()

	validContent := []byte("valid cached content 2")
	validHash := writeCacheFile(t, cachePath, validContent)

	// A hash entry with no backing file at all
	missingHash := strings.Repeat("de", 32)

	c := CacheIndex{
		Version:   1,
		cachePath: cachePath,
		Hashes: map[string][]string{
			cacheHashFormat: {validHash, missingHash},
			"md5":           {"m-valid", "m-missing"},
		},
	}

	c.updateVersion()

	if c.Version != 2 {
		t.Errorf("Version = %d, want 2", c.Version)
	}
	if got := c.Hashes[cacheHashFormat]; len(got) != 1 || got[0] != validHash {
		t.Errorf("Hashes[%s] = %v, want [%s]", cacheHashFormat, got, validHash)
	}
	if got := c.Hashes["md5"]; len(got) != 1 || got[0] != "m-valid" {
		t.Errorf("Hashes[md5] = %v, want [m-valid]", got)
	}
}

func TestCacheIndexUpdateVersionRemovesMultipleFiles(t *testing.T) {
	// Regression test for the removeIndices misalignment bug (see
	// TestRemoveIndicesMultiple): removing more than one broken v1 entry in a
	// single updateVersion() pass used to leave the per-format hash lists
	// misaligned with each other.
	cachePath := t.TempDir()

	validContent := []byte("valid cached content 3")
	validHash := writeCacheFile(t, cachePath, validContent)

	emptyHash := hashOf(t, "sha256", []byte{})
	emptyDir := filepath.Join(cachePath, emptyHash[:2])
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(emptyDir, emptyHash[2:]), []byte{}, 0644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}

	missingHash := strings.Repeat("de", 32)

	c := CacheIndex{
		Version:   1,
		cachePath: cachePath,
		Hashes: map[string][]string{
			// Two broken entries (empty file, missing file) interleaved with the one valid entry
			cacheHashFormat: {emptyHash, validHash, missingHash},
			"md5":           {"m-empty", "m-valid", "m-missing"},
		},
	}

	c.updateVersion()

	if c.Version != 2 {
		t.Errorf("Version = %d, want 2", c.Version)
	}
	if got := c.Hashes[cacheHashFormat]; len(got) != 1 || got[0] != validHash {
		t.Errorf("Hashes[%s] = %v, want [%s]", cacheHashFormat, got, validHash)
	}
	if got := c.Hashes["md5"]; len(got) != 1 || got[0] != "m-valid" {
		t.Errorf("Hashes[md5] = %v, want [m-valid]", got)
	}
}

func TestCacheIndexHandleRemoveKeepsSparseFormatsAligned(t *testing.T) {
	// Regression test: Remove() previously only deleted the index entry from
	// hash-format lists present in the handle's own Hashes map, silently
	// skipping any format whose value happened to be empty/absent at that
	// index - even if that format's list had real data at later indices,
	// which then ended up misaligned with the other (shortened) lists.
	c := CacheIndex{Hashes: map[string][]string{
		"sha256":  {"h0", "h1", "h2"},
		"murmur2": {"", "", "m2"}, // no murmur2 data recorded for index 0 or 1
	}}
	// Handle for index 0: its own Hashes snapshot has no murmur2 entry, since
	// murmur2[0] is empty - this is what a real GetHandleFromHash call would produce.
	handle := &CacheIndexHandle{index: &c, hashIdx: 0, Hashes: map[string]string{"sha256": "h0"}}

	handle.Remove()

	if got := c.Hashes["sha256"]; len(got) != 2 || got[0] != "h1" || got[1] != "h2" {
		t.Errorf("c.Hashes[sha256] = %v, want [h1 h2]", got)
	}
	// murmur2 must be shifted down too, so index 1 (was index 2) still holds "m2"
	if got := c.Hashes["murmur2"]; len(got) != 2 || got[1] != "m2" {
		t.Errorf("c.Hashes[murmur2] = %v, want [_ m2] (m2 shifted down to stay aligned with sha256)", got)
	}
}

func TestRemoveIndices(t *testing.T) {
	got := removeIndices([]string{"a", "b", "c", "d"}, []int{1})
	want := []string{"a", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("removeIndices() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("removeIndices()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRemoveIndicesMultiple(t *testing.T) {
	// Regression test: removeIndices previously compared the write-position
	// counter against the target indices instead of the read position, so it
	// misaligned and failed to remove every intended entry once more than one
	// index needed removing (removing [1,3] from a 4-element list used to
	// yield ["a","c","d"] instead of ["a","c"]).
	got := removeIndices([]string{"a", "b", "c", "d"}, []int{1, 3})
	want := []string{"a", "c"}
	if len(got) != len(want) {
		t.Fatalf("removeIndices() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("removeIndices()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRemoveIndicesConsecutive(t *testing.T) {
	got := removeIndices([]string{"a", "b", "c", "d", "e"}, []int{0, 1, 4})
	want := []string{"c", "d"}
	if len(got) != len(want) {
		t.Fatalf("removeIndices() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("removeIndices()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRemoveIndicesEmpty(t *testing.T) {
	input := []string{"a", "b", "c"}
	got := removeIndices(input, nil)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("removeIndices() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("removeIndices()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRemoveEmpty(t *testing.T) {
	got, indices := removeEmpty([]string{"a", "", "b", "", ""})
	wantList := []string{"a", "b"}
	wantIndices := []int{1, 3, 4}

	if len(got) != len(wantList) {
		t.Fatalf("removeEmpty() list = %v, want %v", got, wantList)
	}
	for i := range wantList {
		if got[i] != wantList[i] {
			t.Errorf("removeEmpty() list[%d] = %q, want %q", i, got[i], wantList[i])
		}
	}
	if len(indices) != len(wantIndices) {
		t.Fatalf("removeEmpty() indices = %v, want %v", indices, wantIndices)
	}
	for i := range wantIndices {
		if indices[i] != wantIndices[i] {
			t.Errorf("removeEmpty() indices[%d] = %d, want %d", i, indices[i], wantIndices[i])
		}
	}
}

func TestSelectPreferredHash(t *testing.T) {
	// Only one candidate present
	format, hash := selectPreferredHash(map[string]string{"murmur2": "m2"})
	if format != "murmur2" || hash != "m2" {
		t.Errorf("selectPreferredHash() = (%q, %q), want (murmur2, m2)", format, hash)
	}

	// Multiple candidates: preferredHashList is ordered weakest to strongest,
	// and the loop does not break early, so the strongest present format wins.
	format, hash = selectPreferredHash(map[string]string{"murmur2": "m2", "sha256": "s2"})
	if format != "sha256" || hash != "s2" {
		t.Errorf("selectPreferredHash() = (%q, %q), want (sha256, s2)", format, hash)
	}

	// No candidates
	format, hash = selectPreferredHash(map[string]string{})
	if format != "" || hash != "" {
		t.Errorf("selectPreferredHash() = (%q, %q), want empty", format, hash)
	}
}

func TestGetHashListsForDownload(t *testing.T) {
	cl, hashes := getHashListsForDownload([]string{"md5", "sha256", "murmur2"}, "md5", "abc")
	if hashes["md5"] != "abc" {
		t.Errorf("hashes[md5] = %q, want %q", hashes["md5"], "abc")
	}
	wantCl := []string{"sha256", "murmur2"}
	if len(cl) != len(wantCl) {
		t.Fatalf("cl = %v, want %v", cl, wantCl)
	}
	for i := range wantCl {
		if cl[i] != wantCl[i] {
			t.Errorf("cl[%d] = %q, want %q", i, cl[i], wantCl[i])
		}
	}
}

func TestGetHashListsForDownloadValidateIsCacheFormat(t *testing.T) {
	cl, hashes := getHashListsForDownload([]string{"sha256", "md5"}, cacheHashFormat, "xyz")
	if hashes[cacheHashFormat] != "xyz" {
		t.Errorf("hashes[%s] = %q, want %q", cacheHashFormat, hashes[cacheHashFormat], "xyz")
	}
	wantCl := []string{"md5"}
	if len(cl) != len(wantCl) || cl[0] != wantCl[0] {
		t.Errorf("cl = %v, want %v", cl, wantCl)
	}
}

func TestTeeHashesSuccess(t *testing.T) {
	content := []byte("hello teeHashes world")
	sha256Hash := hashOf(t, "sha256", content)

	hashes := map[string]string{"sha256": sha256Hash}
	dst := &bytes.Buffer{}
	src := bytes.NewReader(content)

	err := teeHashes([]string{"md5"}, hashes, dst, src)
	if err != nil {
		t.Fatalf("teeHashes() returned error: %v", err)
	}
	if !bytes.Equal(dst.Bytes(), content) {
		t.Errorf("dst content = %q, want %q", dst.Bytes(), content)
	}
	wantMD5 := hashOf(t, "md5", content)
	if hashes["md5"] != wantMD5 {
		t.Errorf("hashes[md5] = %q, want %q", hashes["md5"], wantMD5)
	}
}

func TestTeeHashesMismatch(t *testing.T) {
	content := []byte("hello teeHashes world")
	hashes := map[string]string{"sha256": strings.Repeat("0", 64)}
	dst := &bytes.Buffer{}
	src := bytes.NewReader(content)

	err := teeHashes(nil, hashes, dst, src)
	if err == nil {
		t.Error("expected error for mismatched hash, got nil")
	}
}

func TestTeeHashesNoPreferredHash(t *testing.T) {
	hashes := map[string]string{}
	dst := &bytes.Buffer{}
	src := bytes.NewReader([]byte("data"))

	err := teeHashes(nil, hashes, dst, src)
	if err == nil {
		t.Error("expected error when no preferred hash format is available, got nil")
	}
}

// TestCreateDownloadSessionConcurrentDownloads exercises StartDownloads()'s
// worker-pool concurrency: many independent downloads in parallel, plus two
// mods that happen to share identical content/hash, downloaded at the same
// time. The shared-hash pair specifically exercises the lock ordering in
// downloadNewFile (NewHandleFromHashes + UpdateIndex must happen atomically)
// that prevents two goroutines from both deciding they're new and racing to
// os.Rename into the same cache destination. Run with -race to be meaningful.
func TestCreateDownloadSessionConcurrentDownloads(t *testing.T) {
	httpmock.Activate(t)

	cacheDir := t.TempDir()
	oldCacheDir := viper.GetString("cache.directory")
	viper.Set("cache.directory", cacheDir)
	t.Cleanup(func() { viper.Set("cache.directory", oldCacheDir) })

	const numMods = 16
	var mods []*Mod
	for i := 0; i < numMods; i++ {
		content := []byte(fmt.Sprintf("distinct content for mod %d", i))
		hash := hashOf(t, "sha256", content)
		url := fmt.Sprintf("https://example.com/mod%d.jar", i)
		httpmock.RegisterResponder("GET", url, httpmock.NewBytesResponder(200, content))
		mods = append(mods, &Mod{
			Name:     fmt.Sprintf("mod%d", i),
			FileName: fmt.Sprintf("mod%d.jar", i),
			Download: ModDownload{URL: url, HashFormat: "sha256", Hash: hash},
		})
	}

	// Two mods with byte-identical content: both resolve to the same cache
	// hash and, downloaded concurrently, are the case that must not race.
	dupContent := []byte("duplicate content shared by two mods")
	dupHash := hashOf(t, "sha256", dupContent)
	httpmock.RegisterResponder("GET", "https://example.com/dupA.jar", httpmock.NewBytesResponder(200, dupContent))
	httpmock.RegisterResponder("GET", "https://example.com/dupB.jar", httpmock.NewBytesResponder(200, dupContent))
	mods = append(mods,
		&Mod{Name: "dupA", FileName: "dupA.jar", Download: ModDownload{URL: "https://example.com/dupA.jar", HashFormat: "sha256", Hash: dupHash}},
		&Mod{Name: "dupB", FileName: "dupB.jar", Download: ModDownload{URL: "https://example.com/dupB.jar", HashFormat: "sha256", Hash: dupHash}},
	)

	session, err := CreateDownloadSession(mods, []string{"sha256"})
	if err != nil {
		t.Fatalf("CreateDownloadSession() returned error: %v", err)
	}

	results := make(map[string]CompletedDownload)
	for dl := range session.StartDownloads() {
		if dl.Error != nil {
			t.Errorf("download for %s failed: %v", dl.Mod.Name, dl.Error)
			continue
		}
		results[dl.Mod.Name] = dl
		_ = dl.File.Close()
	}

	if len(results) != len(mods) {
		t.Fatalf("got %d results, want %d", len(results), len(mods))
	}
	for i := 0; i < numMods; i++ {
		name := fmt.Sprintf("mod%d", i)
		wantHash := hashOf(t, "sha256", []byte(fmt.Sprintf("distinct content for mod %d", i)))
		if results[name].Hashes["sha256"] != wantHash {
			t.Errorf("%s: Hashes[sha256] = %q, want %q", name, results[name].Hashes["sha256"], wantHash)
		}
	}
	if results["dupA"].Hashes["sha256"] != dupHash {
		t.Errorf("dupA: Hashes[sha256] = %q, want %q", results["dupA"].Hashes["sha256"], dupHash)
	}
	if results["dupB"].Hashes["sha256"] != dupHash {
		t.Errorf("dupB: Hashes[sha256] = %q, want %q", results["dupB"].Hashes["sha256"], dupHash)
	}

	if err := session.SaveIndex(); err != nil {
		t.Errorf("SaveIndex() returned error: %v", err)
	}
}
