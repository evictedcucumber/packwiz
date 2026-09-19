package cmdtest

import (
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

// ConfigDirSource stands in for a source of mods, such as Modrinth, that has one which reads a pack's files from a
// folder of its own, as Configured Defaults does. A mod with an [update.<name>] table is that mod.
type ConfigDirSource struct {
	name string
	dir  string
}

// RegisterConfigDirSource registers a ConfigDirSource under name for the duration of the test. A pack with a mod that
// uses it keeps its files in dir.
func RegisterConfigDirSource(t *testing.T, name, dir string) *ConfigDirSource {
	t.Helper()
	s := &ConfigDirSource{name: name, dir: dir}
	core.Updaters[name] = s
	t.Cleanup(func() { delete(core.Updaters, name) })
	return s
}

// UpdateData is the [update.<name>] table for a mod that has the pack keep its files in the folder.
func (s *ConfigDirSource) UpdateData() map[string]map[string]interface{} {
	return map[string]map[string]interface{}{s.name: {"version": "any"}}
}

func (s *ConfigDirSource) ParseUpdate(raw map[string]interface{}) (interface{}, error) {
	return raw, nil
}

func (s *ConfigDirSource) CheckUpdate([]*core.Mod, core.Pack) ([]core.UpdateCheck, error) {
	return nil, nil
}

func (s *ConfigDirSource) DoUpdate([]*core.Mod, []interface{}) error { return nil }

func (s *ConfigDirSource) ConfigDir(mod *core.Mod) string {
	if _, ok := mod.GetParsedUpdateData(s.name); ok {
		return s.dir
	}
	return ""
}
