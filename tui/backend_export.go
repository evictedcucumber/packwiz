package tui

import (
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/modrinth"
)

// exportBackend is what the export screen needs of the pack: what it would call the file it exports, and exporting it.
type exportBackend interface {
	// defaultExportName is the file the pack is exported to if it isn't told another, as "packwiz modrinth export" names it,
	// for each kind of export.
	defaultExportName() (exportNames, error)
	// exportPack exports the pack as a .mrpack, its server pack or its Bisect Hosting pack, as "packwiz modrinth export" does. progress says how
	// many of the files are done.
	exportPack(options modrinth.ExportOptions, progress func(done, total int)) (*modrinth.ExportResult, error)
}

// exportNames are the files that each kind of export goes to unless told another.
type exportNames struct {
	mrpack, server, bisect string
}

func (packBackend) defaultExportName() (exportNames, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return exportNames{}, err
	}
	return exportNames{pack.GetPackName() + ".mrpack", modrinth.ServerPackName(pack), modrinth.BisectPackName(pack)}, nil
}

func (packBackend) exportPack(options modrinth.ExportOptions, progress func(done, total int)) (*modrinth.ExportResult, error) {
	return modrinth.Export(options, progress)
}
