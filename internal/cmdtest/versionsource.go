package cmdtest

import (
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

// VersionSource stands in for a source of mods, such as Modrinth, that can look up the readable version of the file
// a mod has installed. A mod uses it if it has an [update.<name>] table with a version ID, which is what the source
// looks up.
type VersionSource struct {
	name string
	// Versions maps the version ID a mod has installed to its readable version. IDs it doesn't have are unknown.
	Versions map[string]string
	// Err, if set, makes every lookup fail, as if the source couldn't be reached.
	Err error
	// Calls is how many lookups have been made.
	Calls int
}

// RegisterVersionSource registers a VersionSource under name for the duration of the test.
func RegisterVersionSource(t *testing.T, name string, versions map[string]string) *VersionSource {
	t.Helper()
	s := &VersionSource{name: name, Versions: versions}
	core.Updaters[name] = s
	t.Cleanup(func() { delete(core.Updaters, name) })
	return s
}

// UpdateData is the [update.<name>] table for a mod that has the given version ID installed.
func (s *VersionSource) UpdateData(versionID string) map[string]map[string]interface{} {
	return map[string]map[string]interface{}{s.name: {"version": versionID}}
}

func (s *VersionSource) ParseUpdate(raw map[string]interface{}) (interface{}, error) {
	return raw["version"], nil
}

func (s *VersionSource) CheckUpdate([]*core.Mod, core.Pack) ([]core.UpdateCheck, error) {
	return nil, nil
}

func (s *VersionSource) DoUpdate([]*core.Mod, []interface{}) error { return nil }

func (s *VersionSource) ResolveVersions(mods []*core.Mod) ([]string, error) {
	s.Calls++
	if s.Err != nil {
		return nil, s.Err
	}
	out := make([]string, len(mods))
	for i, m := range mods {
		data, _ := m.GetParsedUpdateData(s.name)
		id, _ := data.(string)
		out[i] = s.Versions[id]
	}
	return out, nil
}
