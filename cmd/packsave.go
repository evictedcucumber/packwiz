package cmd

import (
	"github.com/evictedcucumber/packwiz/core"
)

func writePackAndIndex(pack *core.Pack, index *core.Index) error {
	if err := index.Write(); err != nil {
		return err
	}
	if err := pack.UpdateIndexHash(); err != nil {
		return err
	}
	return pack.Write()
}

func finalizePackWithDependencies(pack *core.Pack, index *core.Index, opts core.SyncDepsOpts) error {
	if err := index.SyncDependencyMetadata(*pack, opts); err != nil {
		return err
	}
	return writePackAndIndex(pack, index)
}
