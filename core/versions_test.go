package core

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// versionSource is an updater that can look up readable versions, standing in for a real source.
type versionSource struct {
	name string
	// versions maps an installed version ID to its readable version; IDs it doesn't have are unknown
	versions map[string]string
	err      error
	// short makes it return fewer versions than it was asked for
	short bool
	// calls records the installed version IDs it was asked about, one entry per call
	calls [][]string
}

func (s *versionSource) ParseUpdate(raw map[string]interface{}) (interface{}, error) {
	return raw["version"], nil
}
func (s *versionSource) CheckUpdate([]*Mod, Pack) ([]UpdateCheck, error) { return nil, nil }
func (s *versionSource) DoUpdate([]*Mod, []interface{}) error            { return nil }

func (s *versionSource) ResolveVersions(mods []*Mod) ([]string, error) {
	var asked []string
	out := make([]string, len(mods))
	for i, m := range mods {
		data, _ := m.GetParsedUpdateData(s.name)
		id, _ := data.(string)
		asked = append(asked, id)
		out[i] = s.versions[id]
	}
	s.calls = append(s.calls, asked)
	if s.err != nil {
		return nil, s.err
	}
	if s.short {
		return out[:len(out)-1], nil
	}
	return out, nil
}

func registerVersionSource(t *testing.T, name string, versions map[string]string) *versionSource {
	t.Helper()
	s := &versionSource{name: name, versions: versions}
	Updaters[name] = s
	t.Cleanup(func() { delete(Updaters, name) })
	return s
}

// modTOML is the contents of a mod's metadata file. A mod with no version records none, as ones from before the
// version field do, and one with no source has no update table.
func modTOML(name, version, source, id string) string {
	s := "name = \"" + name + "\"\nfilename = \"" + name + "-" + id + ".jar\"\n"
	if version != "" {
		s += "version = \"" + version + "\"\n"
	}
	s += "\n[download]\nhash-format = \"sha256\"\nhash = \"abc\"\n"
	if source != "" {
		s += "\n[update." + source + "]\nversion = \"" + id + "\"\n"
	}
	return s
}

// versionFixture writes metadata files into a temp pack, keyed by their path relative to it, and returns an index
// listing them.
func versionFixture(t *testing.T, files map[string]string) Index {
	t.Helper()
	dir := t.TempDir()
	index := Index{HashFormat: "sha256", Files: make(IndexFiles), packRoot: dir}
	for path, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
		index.Files[path] = &indexFile{File: path, MetaFile: true}
	}
	return index
}

func TestResolveMissingVersions(t *testing.T) {
	src := registerVersionSource(t, "src", map[string]string{"ida": "1.0", "idb": "should not be asked", "idd": "should not be asked"})
	Updaters["plain"] = stubUpdater{} // an updater that can't look versions up
	t.Cleanup(func() { delete(Updaters, "plain") })

	index := versionFixture(t, map[string]string{
		"mods/a.pw.toml": modTOML("a", "", "src", "ida"),      // needs its version, and the source has it
		"mods/b.pw.toml": modTOML("b", "9.9", "src", "idb"),   // already has one
		"mods/c.pw.toml": modTOML("c", "", "", "idc"),         // no source to ask
		"mods/d.pw.toml": modTOML("d", "", "plain", "idd"),    // a source that can't look versions up
		"mods/e.pw.toml": modTOML("e", "", "src", "unknown!"), // the source has never heard of it
	})

	got, err := index.ResolveMissingVersions()
	if err != nil {
		t.Fatalf("ResolveMissingVersions() returned error: %v", err)
	}

	if want := map[string]string{"mods/a.pw.toml": "1.0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ResolveMissingVersions() = %v, want %v", got, want)
	}
	// Only the mods that lack a version and can be asked about were, in one call, in path order
	if want := [][]string{{"ida", "unknown!"}}; !reflect.DeepEqual(src.calls, want) {
		t.Errorf("source was asked %v, want %v", src.calls, want)
	}
}

func TestResolveMissingVersionsAsksEachSourceOnce(t *testing.T) {
	one := registerVersionSource(t, "one", map[string]string{"id1": "1", "id3": "3"})
	two := registerVersionSource(t, "two", map[string]string{"id2": "2"})
	index := versionFixture(t, map[string]string{
		"mods/a.pw.toml": modTOML("a", "", "one", "id1"),
		"mods/b.pw.toml": modTOML("b", "", "two", "id2"),
		"mods/c.pw.toml": modTOML("c", "", "one", "id3"),
	})

	got, err := index.ResolveMissingVersions()
	if err != nil {
		t.Fatalf("ResolveMissingVersions() returned error: %v", err)
	}

	want := map[string]string{"mods/a.pw.toml": "1", "mods/b.pw.toml": "2", "mods/c.pw.toml": "3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ResolveMissingVersions() = %v, want %v", got, want)
	}
	if len(one.calls) != 1 || len(two.calls) != 1 {
		t.Errorf("sources were called %d and %d times, want once each so mods are looked up in a batch", len(one.calls), len(two.calls))
	}
}

func TestResolveMissingVersionsKeepsWhatOtherSourcesFoundOnError(t *testing.T) {
	registerVersionSource(t, "good", map[string]string{"id1": "1"})
	bad := registerVersionSource(t, "bad", nil)
	bad.err = errors.New("network is down")
	index := versionFixture(t, map[string]string{
		"mods/a.pw.toml": modTOML("a", "", "good", "id1"),
		"mods/b.pw.toml": modTOML("b", "", "bad", "id2"),
	})

	got, err := index.ResolveMissingVersions()

	if err == nil || !strings.Contains(err.Error(), "bad") || !strings.Contains(err.Error(), "network is down") {
		t.Errorf("error = %v, want one naming the failing source and why", err)
	}
	if want := map[string]string{"mods/a.pw.toml": "1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ResolveMissingVersions() = %v, want the healthy source's %v", got, want)
	}
}

func TestResolveMissingVersionsRejectsAMiscountedAnswer(t *testing.T) {
	src := registerVersionSource(t, "src", map[string]string{"id1": "1", "id2": "2"})
	src.short = true
	index := versionFixture(t, map[string]string{
		"mods/a.pw.toml": modTOML("a", "", "src", "id1"),
		"mods/b.pw.toml": modTOML("b", "", "src", "id2"),
	})

	got, err := index.ResolveMissingVersions()

	// With fewer answers than mods there is no telling which mod each belongs to, so none are used
	if err == nil {
		t.Error("ResolveMissingVersions() returned no error for a source that answered for too few mods")
	}
	if len(got) != 0 {
		t.Errorf("ResolveMissingVersions() = %v, want nothing from a source whose answer can't be matched to its mods", got)
	}
}

func TestResolveMissingVersionsUnreadableMetadata(t *testing.T) {
	index := versionFixture(t, map[string]string{"mods/a.pw.toml": "this is [not toml"})

	if _, err := index.ResolveMissingVersions(); err == nil || !strings.Contains(err.Error(), "mods/a.pw.toml") {
		t.Errorf("error = %v, want one naming mods/a.pw.toml", err)
	}
}

func TestResolveMissingVersionsIgnoresOtherFiles(t *testing.T) {
	src := registerVersionSource(t, "src", map[string]string{"id1": "1"})
	index := versionFixture(t, map[string]string{"mods/a.pw.toml": modTOML("a", "", "src", "id1")})
	index.Files["config/a.json"] = &indexFile{File: "config/a.json"} // not a metadata file, and not on disk

	got, err := index.ResolveMissingVersions()
	if err != nil {
		t.Fatalf("ResolveMissingVersions() returned error: %v", err)
	}
	if len(got) != 1 || len(src.calls) != 1 {
		t.Errorf("got %v after %d calls, want only the metadata file looked up", got, len(src.calls))
	}
}

func TestRecordVersions(t *testing.T) {
	registerVersionSource(t, "src", nil)
	index := versionFixture(t, map[string]string{"mods/a.pw.toml": modTOML("a", "", "src", "ida")})
	path := index.ResolveIndexPath("mods/a.pw.toml")
	before := index.Files["mods/a.pw.toml"].(*indexFile).Hash

	if err := index.RecordVersions(map[string]string{"mods/a.pw.toml": "1.2.3"}); err != nil {
		t.Fatalf("RecordVersions() returned error: %v", err)
	}

	mod, err := LoadMod(path)
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", mod.Version)
	}
	// Everything else in the file is as it was
	if mod.Name != "a" || mod.FileName != "a-ida.jar" || mod.Download.Hash != "abc" {
		t.Errorf("mod = %+v, want its other fields kept", mod)
	}
	if data, ok := mod.GetParsedUpdateData("src"); !ok || data != "ida" {
		t.Errorf("update data = %v, %v, want the source's version ID kept", data, ok)
	}

	// The index entry describes the file as it is now, so it still verifies against it
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read mod file: %v", err)
	}
	h, _ := GetHashImpl("sha256")
	_, _ = h.Write(content)
	want := h.HashToString(h.Sum(nil))
	entry := index.Files["mods/a.pw.toml"].(*indexFile)
	if entry.Hash != want || entry.Hash == before || !entry.MetaFile {
		t.Errorf("index entry = %+v, want the metafile's hash %s (was %q)", entry, want, before)
	}
}

func TestRecordVersionsKeepsAVersionTheModGainedMeanwhile(t *testing.T) {
	index := versionFixture(t, map[string]string{"mods/a.pw.toml": modTOML("a", "5.0", "", "ida")})

	if err := index.RecordVersions(map[string]string{"mods/a.pw.toml": "1.2.3"}); err != nil {
		t.Fatalf("RecordVersions() returned error: %v", err)
	}

	mod, err := LoadMod(index.ResolveIndexPath("mods/a.pw.toml"))
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.Version != "5.0" {
		t.Errorf("Version = %q, want the existing 5.0 kept", mod.Version)
	}
}

func TestRecordedVersionsAreNotMissingAnyMore(t *testing.T) {
	src := registerVersionSource(t, "src", map[string]string{"ida": "1.0"})
	index := versionFixture(t, map[string]string{"mods/a.pw.toml": modTOML("a", "", "src", "ida")})

	found, err := index.ResolveMissingVersions()
	if err != nil || len(found) != 1 {
		t.Fatalf("ResolveMissingVersions() = %v, %v, want one version", found, err)
	}
	if err := index.RecordVersions(found); err != nil {
		t.Fatalf("RecordVersions() returned error: %v", err)
	}

	again, err := index.ResolveMissingVersions()
	if err != nil || len(again) != 0 {
		t.Errorf("ResolveMissingVersions() after recording = %v, %v, want nothing left", again, err)
	}
	if len(src.calls) != 1 {
		t.Errorf("source was called %d times, want it not asked again once the versions are recorded", len(src.calls))
	}
}

func TestRecordVersionsUnreadableMetadata(t *testing.T) {
	index := versionFixture(t, map[string]string{"mods/a.pw.toml": "this is [not toml"})

	if err := index.RecordVersions(map[string]string{"mods/a.pw.toml": "1"}); err == nil {
		t.Error("RecordVersions() returned no error for unreadable metadata")
	}
}
