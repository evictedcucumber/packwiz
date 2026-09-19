package modrinth

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// Modrinth lists some mods, mostly libraries and world generation, as unsupported on the client, since only the server
// needs them to work. A mod that is installed on the client and requires one of them still crashes without it, and so
// does singleplayer, which runs a server of its own. So a mod that only has a server side (see getSide) goes on both
// sides when a mod that runs on the client requires it.

// sidePromotion is a mod that was put on both sides, and the mod that needs it on the client
type sidePromotion struct {
	mod      *core.Mod
	neededBy *core.Mod
}

// runsOnClient reports whether a mod with the given side is loaded by clients. Anything that isn't explicitly
// server-only is treated as running on the client, including an empty side (which packwiz treats as "both").
func runsOnClient(side string) bool {
	return side != core.ServerSide
}

// widenSides puts every server-only mod on both sides when a mod that runs on the client requires it. What a mod that
// is put on the client requires is then needed there too, so this goes on until nothing more changes, and a chain of
// dependencies is followed all the way. Only required dependencies count, and only ones that are in mods.
//
// It changes the mods in place, and returns what it changed, once for each mod. When several mods need the same one,
// it is the first of them (by the path of its metadata file) that is named as needing it.
func widenSides(mods []*core.Mod) []sidePromotion {
	byProject := make(map[string]*core.Mod, len(mods))
	for _, mod := range mods {
		if data, ok := modrinthUpdateData(mod); ok && data.ProjectID != "" {
			byProject[data.ProjectID] = mod
		}
	}

	// Map iteration order (which the index lists mods in) is random; go in path order so a pack always gives the
	// same answer
	ordered := slices.SortedFunc(slices.Values(mods), func(a, b *core.Mod) int {
		return cmp.Compare(a.GetFilePath(), b.GetFilePath())
	})

	var promotions []sidePromotion
	// A mod only ever goes from server to both, so this ends
	for changed := true; changed; {
		changed = false
		for _, mod := range ordered {
			if !runsOnClient(mod.Side) {
				continue
			}
			for _, dep := range mod.Dependencies {
				needed := byProject[dep.ID]
				if dep.Type != "required" || needed == nil || needed.Side != core.ServerSide {
					continue
				}
				needed.Side = core.UniversalSide
				promotions = append(promotions, sidePromotion{mod: needed, neededBy: mod})
				changed = true
			}
		}
	}
	return promotions
}

// promoteSides puts the mods in the pack that others need on the client on both sides (see widenSides), saving their
// metadata files and saying what it changed. The index is updated but not written, which is for the caller to do.
func promoteSides(index *core.Index) error {
	mods, err := index.LoadAllMods()
	if err != nil {
		return fmt.Errorf("failed to check which sides the mods need to be on: %w", err)
	}

	for _, p := range widenSides(mods) {
		format, hash, err := p.mod.Write()
		if err != nil {
			return err
		}
		if err := index.RefreshFileWithHash(p.mod.GetFilePath(), format, hash, true); err != nil {
			return err
		}
		ui.Info.Printf("Notice: %s is now on both sides, as %s needs it on the client\n", ui.Bold.Sprint(p.mod.Name), ui.Bold.Sprint(p.neededBy.Name))
	}
	return nil
}
