package cmdshared

import (
	"archive/zip"
	"fmt"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
)

func ListManualDownloads(session core.DownloadSession) {
	manualDownloads := session.GetManualDownloads()
	if len(manualDownloads) > 0 {
		ui.Warning.Printf("Found %v manual downloads; these mods are unable to be downloaded by packwiz (due to API limitations) and must be manually downloaded:\n",
			len(manualDownloads))
		for _, dl := range manualDownloads {
			fmt.Printf("%s %s from %s\n", ui.Bold.Sprint(dl.Name), ui.Muted.Sprintf("(%s)", dl.FileName), ui.Info.Sprint(dl.URL))
		}
		cacheDir, err := core.GetPackwizCache()
		if err != nil {
			ui.Error.Printf("Error locating cache folder: %v", err)
			os.Exit(1)
		}

		ui.Warning.Printf("Once you have done so, place these files in %s and re-run this command.\n",
			filepath.Join(cacheDir, core.DownloadCacheImportFolder))
		os.Exit(1)
	}
}

func AddToZip(dl core.CompletedDownload, exp *zip.Writer, dir string, index *core.Index) bool {
	if dl.Error != nil {
		notice.Errorf("Download of %s (%s) failed: %v", dl.Mod.Name, dl.Mod.FileName, dl.Error)
		return false
	}
	for _, warning := range dl.Warnings {
		notice.Warnf("Warning for %s (%s): %v", dl.Mod.Name, dl.Mod.FileName, warning)
	}

	p, err := index.RelIndexPath(dl.Mod.GetDestFilePath())
	if err != nil {
		notice.Errorf("Error resolving external file: %v", err)
		return false
	}
	modFile, err := exp.Create(path.Join(dir, p))
	if err != nil {
		notice.Errorf("Error creating metadata file %s: %v", p, err)
		return false
	}
	_, err = io.Copy(modFile, dl.File)
	if err != nil {
		notice.Errorf("Error copying file %s: %v", p, err)
		return false
	}
	err = dl.File.Close()
	if err != nil {
		notice.Errorf("Error closing file %s: %v", p, err)
		return false
	}

	return true
}

// AddNonMetafileOverrides saves all non-metadata files into an overrides folder in the zip
func AddNonMetafileOverrides(index *core.Index, exp *zip.Writer) {
	AddNonMetafiles(index, exp, "overrides", nil)
}

// AddNonMetafiles saves the files the index lists that aren't metadata files (config files and the like) into dir in the
// zip ("" for its top), in the order of their paths, but for those that skip, if it isn't nil, says to leave out.
func AddNonMetafiles(index *core.Index, exp *zip.Writer, dir string, skip func(p string) bool) {
	for _, p := range slices.Sorted(maps.Keys(index.Files)) {
		if index.Files[p].IsMetaFile() || (skip != nil && skip(p)) {
			continue
		}
		// Read from disk, without checking hashes (assumed to have no errors)
		if err := AddFileToZip(exp, index.ResolveIndexPath(p), path.Join(dir, p)); err != nil {
			notice.Errorf("%s", err.Error())
			// TODO: exit(1)?
		}
	}
}

// AddFileToZip copies the file at src on disk into the zip as name.
func AddFileToZip(exp *zip.Writer, src, name string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("Error reading file %s: %w", name, err)
	}
	defer func() { _ = in.Close() }()
	file, err := exp.Create(name)
	if err != nil {
		return fmt.Errorf("Error creating file %s: %w", name, err)
	}
	if _, err = io.Copy(file, in); err != nil {
		return fmt.Errorf("Error copying file %s: %w", name, err)
	}
	return nil
}

// AddDocOverrides saves the pack's README, licence and changelog (core.DocFiles), whichever of them are in the pack's
// folder, into the overrides folder of the zip. The index doesn't list them, so AddNonMetafileOverrides doesn't. A pack
// with none of them, or with a folder where one should be, is not an error: they are for the pack's author to write.
func AddDocOverrides(index *core.Index, exp *zip.Writer) {
	AddDocs(index, exp, "overrides", nil)
}

// AddDocs is AddDocOverrides into dir in the zip ("" for its top), but for those that skip, if it isn't nil, says to
// leave out.
func AddDocs(index *core.Index, exp *zip.Writer, dir string, skip func(name string) bool) {
	for _, name := range core.DocFiles {
		if skip != nil && skip(name) {
			continue
		}
		src := index.ResolveIndexPath(name)
		if info, err := os.Stat(src); err != nil || !info.Mode().IsRegular() {
			if err != nil && !os.IsNotExist(err) {
				notice.Errorf("Error reading file %s: %s", name, err.Error())
			}
			continue
		}
		if err := AddFileToZip(exp, src, path.Join(dir, name)); err != nil {
			notice.Errorf("%s", err.Error())
		}
	}
}

func PrintDisclaimer() {
	ui.Warning.Println("Disclaimer: you are responsible for ensuring you comply with ALL the licenses, or obtain appropriate permissions, for the files stored in the pack, which are listed below with paths in overrides/, client-overrides/ or server-overrides/")
	ui.Warning.Println("packwiz is currently unable to match metadata between mod sites - if any of these are available from Modrinth you should change them to use Modrinth metadata (e.g. by re-adding them using the mr commands)")
	fmt.Println()
}
