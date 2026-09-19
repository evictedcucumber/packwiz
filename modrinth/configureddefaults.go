package modrinth

import "github.com/evictedcucumber/packwiz/core"

// Configured Defaults copies the files in a pack's configureddefaults folder into the game directory when they are
// missing there, so a pack update doesn't overwrite what players changed. Unlike config/, the folder can hold any file
// or folder of the game directory, so a pack that has the mod keeps all of its files there.
const (
	configuredDefaultsProjectID = "SISoSFPP"
	configuredDefaultsDir       = "configureddefaults"
)

// mrUpdater can tell that a pack has Configured Defaults
var _ core.ConfigDirResolver = mrUpdater{}

// ConfigDir implements core.ConfigDirResolver: a pack that has Configured Defaults keeps its files in its folder, so
// only that folder is tracked (see Index.Refresh), and so is what is exported, served and described in commits and the
// changelog.
func (u mrUpdater) ConfigDir(mod *core.Mod) string {
	if data, ok := modrinthUpdateData(mod); ok && data.ProjectID == configuredDefaultsProjectID {
		return configuredDefaultsDir
	}
	return ""
}
