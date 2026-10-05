package modrinth

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
)

// ExportOptions say how to export the pack.
type ExportOptions struct {
	// Output is the file to write, or "" for one named after the pack, in the current directory
	Output string
	// Server is whether to export the server pack (see exportServerPack), a zip of what a server needs with the mods in
	// it, rather than a .mrpack. RestrictDomains means nothing for it, as every file is in it.
	Server bool
	// Dev is whether to export a pack for development: every mod is in it for both the client and the server, whatever
	// side it is on, so one instance can run everything. It is a .mrpack, with -dev in its default name. Server and Dev
	// can't be asked for together.
	Dev bool
	// RestrictDomains is whether only files that are on the domains Modrinth allows are left for the launcher to download:
	// any other is stored in the pack itself, which is bigger and is yours to have the right to distribute
	RestrictDomains bool
}

// ExportFile is a file that went into an exported pack.
type ExportFile struct {
	Name string
	// Path is where the file is in the pack: what the launcher downloads it as or, for a file stored in the pack itself, where
	// it is in the archive
	Path string
	// Client and Server say whether it is needed there: "required", "optional" or "unsupported"
	Client, Server string
	Size           uint64
	// Bundled is whether it is stored in the pack itself, rather than left for the launcher to download
	Bundled bool
}

// ExportPromotion is a mod that is only exported for the server, though another needs it on the client.
type ExportPromotion struct {
	Mod, NeededBy string
}

// ExportResult is what exporting the pack made.
type ExportResult struct {
	// Path is the file that was written
	Path string
	// Server is whether it is the server pack, whose files are all in it, at their paths, rather than a .mrpack
	Server bool
	// Files are what went into it, in the order of their paths
	Files []ExportFile
	// Promotions are the mods that are only for the server, that a mod on the client needs: 'packwiz validate' says more
	Promotions []ExportPromotion
	// Notices are what was said along the way, such as that a file failed to download, in plain text
	Notices []string
}

// Breakdown is a table of what went into the pack, with what it adds up to, styled as "packwiz modrinth export" prints it.
func (r *ExportResult) Breakdown() string {
	files := make([]exportedFile, len(r.Files))
	for i, f := range r.Files {
		files[i] = exportedFile{name: f.Name, path: f.Path, client: f.Client, server: f.Server, size: f.Size, bundled: f.Bundled}
	}
	return breakdown(files)
}

// exportHooks are what the command line does at points of exporting, which an interface of another kind has no use for.
type exportHooks struct {
	// disclaimer is called before anything is downloaded if a file is to be stored in the pack itself
	disclaimer func()
	// manual is given the download session before it starts, to deal with files that have to be downloaded by hand. If it
	// is nil, exporting fails if there are any.
	manual func(core.DownloadSession)
	// progress is told how many of the files are done
	progress func(done, total int)
}

// Export exports the pack as a .mrpack, or its server pack if options say so, as "packwiz modrinth export" does, without saying anything on the terminal. The
// index is refreshed first, so that the files it lists are the ones that are there. progress, if it isn't nil, is told how
// many of the files are done, as downloading them is what takes the time.
func Export(options ExportOptions, progress func(done, total int)) (*ExportResult, error) {
	pack, err := core.LoadPack()
	if err != nil {
		return nil, err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return nil, err
	}
	var result *ExportResult
	var notices []notice.Notice
	notices = notice.Collect(func() {
		// Do a refresh to ensure files are up to date
		if _, err = index.RefreshQuietly(); err != nil {
			return
		}
		if err = pack.SaveIndex(index); err != nil {
			return
		}
		var mods []*core.Mod
		if mods, err = index.LoadAllMods(); err != nil {
			err = fmt.Errorf("Error reading file: %v", err)
			return
		}
		result, err = exportWith(pack, &index, mods, options, exportHooks{progress: progress})
	})
	if err != nil {
		return nil, err
	}
	for _, n := range notices {
		if n.Level != notice.Muted {
			result.Notices = append(result.Notices, n.Text)
		}
	}
	return result, nil
}

// exportWith writes the server pack of a pack if options say so, and its .mrpack if not.
func exportWith(pack core.Pack, index *core.Index, mods []*core.Mod, options ExportOptions, hooks exportHooks) (*ExportResult, error) {
	if options.Server && options.Dev {
		return nil, errors.New("a server pack and a dev pack can't be exported together")
	}
	if options.Server {
		return exportServerPack(pack, index, mods, options, hooks)
	}
	return exportPack(pack, index, mods, options, hooks)
}

// exportPack writes the .mrpack of a pack, given its index and mods. What it would print it says through notice, and what
// it makes it returns.
func exportPack(pack core.Pack, index *core.Index, mods []*core.Mod, options ExportOptions, hooks exportHooks) (result *ExportResult, err error) {
	fileName := options.Output
	if fileName == "" {
		fileName = pack.GetPackName() + ".mrpack"
		if options.Dev {
			fileName = pack.GetPackName() + "-dev.mrpack"
		}
	}
	if options.Dev {
		mods = onBothSides(mods)
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

	// Add an overrides folder even if there are no files to go in it
	if _, err = exp.Create("overrides/"); err != nil {
		return nil, fmt.Errorf("Failed to add overrides folder: %s", err.Error())
	}

	// Found now, from the mods as they are in the pack, and said once the files are listed
	promotions := sidePromotions(mods)

	restrictDomains := options.RestrictDomains
	if hooks.disclaimer != nil {
		for _, mod := range mods {
			if !canBeIncludedDirectly(mod, restrictDomains) {
				hooks.disclaimer()
				break
			}
		}
	}

	session, err := core.CreateDownloadSession(mods, []string{"sha1", "sha512", "length-bytes"})
	if err != nil {
		return nil, fmt.Errorf("Error retrieving external files: %v", err)
	}
	if hooks.manual != nil {
		hooks.manual(session)
	} else if manual := session.GetManualDownloads(); len(manual) > 0 {
		return nil, fmt.Errorf("%d of the files have to be downloaded by hand, which can't be done here: run 'packwiz modrinth export' to see which", len(manual))
	}

	manifestFiles := make([]PackFile, 0)
	var exported []exportedFile
	done := 0
	for dl := range session.StartDownloads() {
		done++
		if hooks.progress != nil {
			hooks.progress(done, len(mods))
		}
		if canBeIncludedDirectly(dl.Mod, restrictDomains) {
			if dl.Error != nil {
				notice.Errorf("Download of %s (%s) failed: %v", dl.Mod.Name, dl.Mod.FileName, dl.Error)
				continue
			}
			for _, warning := range dl.Warnings {
				notice.Warnf("Warning for %s (%s): %v", dl.Mod.Name, dl.Mod.FileName, warning)
			}

			path, err := index.RelIndexPath(dl.Mod.GetDestFilePath())
			if err != nil {
				notice.Errorf("Error resolving external file: %s", err.Error())
				// TODO: exit(1)?
				continue
			}

			hashes := make(map[string]string)
			hashes["sha1"] = dl.Hashes["sha1"]
			hashes["sha512"] = dl.Hashes["sha512"]
			fileSize, err := strconv.ParseUint(dl.Hashes["length-bytes"], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("Error reading the size of %s: %v", dl.Mod.FileName, err)
			}

			// Create env options based on configured optional/side
			clientEnv, serverEnv := exportEnv(dl.Mod.Side, dl.Mod.Option != nil && dl.Mod.Option.Optional)

			// Modrinth URLs must be RFC3986
			u, err := core.ReencodeURL(dl.Mod.Download.URL)
			if err != nil {
				notice.Errorf("Error re-encoding download URL: %s", err.Error())
				u = dl.Mod.Download.URL
			}

			manifestFiles = append(manifestFiles, PackFile{
				Path:   path,
				Hashes: hashes,
				Env: &struct {
					Client string `json:"client"`
					Server string `json:"server"`
				}{Client: clientEnv, Server: serverEnv},
				Downloads: []string{u},
				FileSize:  fileSize,
			})

			exported = append(exported, exportedFile{name: dl.Mod.Name, path: path, client: clientEnv, server: serverEnv, size: fileSize})
		} else {
			folder := "overrides"
			if dl.Mod.Side == core.ClientSide {
				folder = "client-overrides"
			} else if dl.Mod.Side == core.ServerSide {
				folder = "server-overrides"
			}
			if cmdshared.AddToZip(dl, cmdshared.ZipArchive{Writer: exp}, folder, index) {
				exported = append(exported, bundledFile(dl, folder, index))
			}
		}
	}
	// sort by `path` property before serialising to ensure reproducibility
	sort.Slice(manifestFiles, func(i, j int) bool {
		return manifestFiles[i].Path < manifestFiles[j].Path
	})

	if err = session.SaveIndex(); err != nil {
		return nil, fmt.Errorf("Error saving cache index: %v", err)
	}

	dependencies := make(map[string]string)
	dependencies["minecraft"], err = pack.GetMCVersion()
	if err != nil {
		return nil, errors.New("Error creating manifest: " + err.Error())
	}
	if neoforgeVersion, ok := pack.Versions["neoforge"]; ok {
		dependencies["neoforge"] = neoforgeVersion
	}

	manifest := Pack{
		FormatVersion: 1,
		Game:          "minecraft",
		VersionID:     pack.Version,
		Name:          pack.Name,
		Summary:       pack.Description,
		Files:         manifestFiles,
		Dependencies:  dependencies,
	}

	if len(pack.Version) == 0 {
		notice.Warnf("Warning: pack.toml version field must not be empty to create a valid Modrinth pack")
	}

	manifestFile, err := exp.Create("modrinth.index.json")
	if err != nil {
		return nil, errors.New("Error creating manifest: " + err.Error())
	}

	w := json.NewEncoder(manifestFile)
	w.SetIndent("", "    ") // Documentation uses 4 spaces
	if err = w.Encode(manifest); err != nil {
		return nil, errors.New("Error writing manifest: " + err.Error())
	}

	cmdshared.AddNonMetafileOverrides(index, exp)
	// The README, licence and changelog aren't in the index, so they go in on their own
	cmdshared.AddDocOverrides(index, exp)

	if err = exp.Close(); err != nil {
		return nil, errors.New("Error writing export file: " + err.Error())
	}
	if err = expFile.Close(); err != nil {
		return nil, errors.New("Error writing export file: " + err.Error())
	}

	result = &ExportResult{Path: fileName}
	for _, f := range exported {
		result.Files = append(result.Files, ExportFile{Name: f.name, Path: f.path, Client: f.client, Server: f.server, Size: f.size, Bundled: f.bundled})
	}
	for _, p := range promotions {
		result.Promotions = append(result.Promotions, ExportPromotion{Mod: p.mod.Name, NeededBy: p.neededBy.Name})
	}
	return result, nil
}

// onBothSides is mods as copies that are all on both sides, as a dev export has them. Everything else about a mod,
// including that it is optional, is as it was.
func onBothSides(mods []*core.Mod) []*core.Mod {
	all := make([]*core.Mod, len(mods))
	for i, mod := range mods {
		c := *mod
		c.Side = core.UniversalSide
		all[i] = &c
	}
	return all
}
