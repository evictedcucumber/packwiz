package tui

import (
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/modrinth"
)

// exportBackend is what the export screen needs of the pack: what it would call the file it exports, and exporting it.
type exportBackend interface {
	// defaultExportName is the file the pack is exported to if it isn't told another, as "packwiz modrinth export" names it,
	// and the file its server pack is exported to.
	defaultExportName() (mrpack, server string, err error)
	// exportPack exports the pack as a .mrpack, or its server pack, as "packwiz modrinth export" does. progress says how
	// many of the files are done.
	exportPack(options modrinth.ExportOptions, progress func(done, total int)) (*modrinth.ExportResult, error)
}

func (packBackend) defaultExportName() (string, string, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return "", "", err
	}
	return pack.GetPackName() + ".mrpack", modrinth.ServerPackName(pack), nil
}

func (packBackend) exportPack(options modrinth.ExportOptions, progress func(done, total int)) (*modrinth.ExportResult, error) {
	return modrinth.Export(options, progress)
}
