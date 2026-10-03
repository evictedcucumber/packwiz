package modrinth

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/evictedcucumber/packwiz/changelog"
	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
)

// ServerPackName is the file the server pack of a pack is written to unless told another, in the current directory.
func ServerPackName(pack core.Pack) string {
	return pack.GetPackName() + core.ServerPackSuffix
}

// onServer reports whether a mod goes in the server pack: one that runs on the server, unless it is optional and not
// on by default, as it is then left out of what a client gets until it is chosen
func onServer(mod *core.Mod) bool {
	if mod.Side == core.ClientSide {
		return false
	}
	return mod.Option == nil || !mod.Option.Optional || mod.Option.Default
}

// serverConfigFiles are the files in the pack's serverconfig folder (core.ServerConfigDir), by their paths relative to
// it with forward slashes, each with its path on disk. A pack with no such folder (or a file of that name) has none.
func serverConfigFiles(index *core.Index) (map[string]string, error) {
	root := index.ResolveIndexPath(core.ServerConfigDir)
	files := map[string]string{}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("Error reading %s: %v", core.ServerConfigDir, err)
		}
		return files, nil
	}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = p
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("Error reading %s: %v", core.ServerConfigDir, err)
	}
	return files, nil
}

// checkServerModList says if the server's list of mods (core.ServerModListFile) is missing, or isn't what "packwiz list
// --save --side server" would write now, as the server pack has it in place of the pack's own
func checkServerModList(pack core.Pack, index *core.Index, mods []*core.Mod, serverFiles map[string]string) {
	const write = "run 'packwiz list --save --side server' to write it"
	src, ok := serverFiles[core.ModListFile]
	if !ok {
		notice.Warnf("Warning: the server pack has no %s, as there is no %s: %s", core.ModListFile, core.ServerModListFile, write)
		return
	}
	want, err := cmd.RenderModList(pack, *index, cmd.ServerModList(mods))
	if err != nil {
		return
	}
	if have, err := os.ReadFile(src); err == nil && !bytes.Equal(have, []byte(want)) {
		notice.Warnf("Warning: %s doesn't list the server's mods as they are now: %s", core.ServerModListFile, write)
	}
}

// checkServerChangelog says if the server's changelog (changelog.ServerMarkdownFile) is missing although the pack has
// releases and a server pack, so the server pack has the pack's own changelog, or if it doesn't have the releases as they
// are now
func checkServerChangelog(index *core.Index, serverFiles map[string]string) {
	if !changelog.HasServerPack(index.ResolveIndexPath(".")) {
		return
	}
	history, err := changelog.LoadHistory(index.ResolveIndexPath(changelog.HistoryFile))
	if err != nil || len(history.Releases) == 0 {
		return
	}
	const write = "run 'packwiz changelog --save' to write it"
	src, ok := serverFiles[changelog.MarkdownFile]
	if !ok {
		notice.Warnf("Warning: the server pack has the pack's own %s, as there is no %s: %s", changelog.MarkdownFile, changelog.ServerMarkdownFile, write)
		return
	}
	if have, err := os.ReadFile(src); err == nil && !changelog.ServerMarkdownIsCurrent(string(have), history.Releases) {
		notice.Warnf("Warning: %s doesn't have the pack's releases as they are now: %s", changelog.ServerMarkdownFile, write)
	}
}

// exportServerPack writes the server pack of a pack, given its index and mods: a zip of the files a server needs, laid
// out as they are on the server, with the mods that run there downloaded into it. Then the files the index lists that
// aren't mods, the pack's README, licence and changelog, and last the files in its serverconfig folder, each of which
// replaces whatever else would be at its path. What it would print it says through notice, and what it makes it returns.
func exportServerPack(pack core.Pack, index *core.Index, mods []*core.Mod, options ExportOptions, hooks exportHooks) (result *ExportResult, err error) {
	serverFiles, err := serverConfigFiles(index)
	if err != nil {
		return nil, err
	}
	replaced := func(p string) bool {
		_, ok := serverFiles[p]
		return ok
	}

	var serverMods []*core.Mod
	for _, mod := range mods {
		if onServer(mod) {
			serverMods = append(serverMods, mod)
		}
	}

	fileName := options.Output
	if fileName == "" {
		fileName = ServerPackName(pack)
	}
	expFile, err := os.Create(fileName)
	if err != nil {
		return nil, fmt.Errorf("Failed to create zip: %s", err.Error())
	}
	exp := zip.NewWriter(expFile)
	// What is half written isn't a pack, so it isn't left behind to be mistaken for one
	defer func() {
		if err != nil {
			_ = exp.Close()
			_ = expFile.Close()
			_ = os.Remove(fileName)
		}
	}()

	session, err := core.CreateDownloadSession(serverMods, []string{"length-bytes"})
	if err != nil {
		return nil, fmt.Errorf("Error retrieving external files: %v", err)
	}
	if hooks.manual != nil {
		hooks.manual(session)
	} else if manual := session.GetManualDownloads(); len(manual) > 0 {
		return nil, fmt.Errorf("%d of the files have to be downloaded by hand, which can't be done here: run 'packwiz modrinth export --server' to see which", len(manual))
	}

	var exported []exportedFile
	done := 0
	for dl := range session.StartDownloads() {
		done++
		if hooks.progress != nil {
			hooks.progress(done, len(serverMods))
		}
		p, err := index.RelIndexPath(dl.Mod.GetDestFilePath())
		if err == nil && replaced(p) {
			notice.Infof("%s is replaced by %s/%s", dl.Mod.Name, core.ServerConfigDir, p)
			if dl.File != nil {
				_ = dl.File.Close()
			}
			continue
		}
		if cmdshared.AddToZip(dl, exp, "", index) {
			client, server := exportEnv(dl.Mod.Side, dl.Mod.Option != nil && dl.Mod.Option.Optional)
			size, _ := strconv.ParseUint(dl.Hashes["length-bytes"], 10, 64)
			exported = append(exported, exportedFile{name: dl.Mod.Name, path: p, client: client, server: server, size: size})
		}
	}

	if err = session.SaveIndex(); err != nil {
		return nil, fmt.Errorf("Error saving cache index: %v", err)
	}

	cmdshared.AddNonMetafiles(index, exp, "", replaced)
	cmdshared.AddDocs(index, exp, "", replaced)
	checkServerModList(pack, index, mods, serverFiles)
	checkServerChangelog(index, serverFiles)
	for _, p := range slices.Sorted(maps.Keys(serverFiles)) {
		if err := cmdshared.AddFileToZip(exp, serverFiles[p], p); err != nil {
			notice.Errorf("%s", err.Error())
		}
	}

	if err = exp.Close(); err != nil {
		return nil, errors.New("Error writing export file: " + err.Error())
	}
	if err = expFile.Close(); err != nil {
		return nil, errors.New("Error writing export file: " + err.Error())
	}

	result = &ExportResult{Path: fileName, Server: true}
	for _, f := range exported {
		result.Files = append(result.Files, ExportFile{Name: f.name, Path: f.path, Client: f.client, Server: f.server, Size: f.size})
	}
	return result, nil
}
