package cmdshared

import (
	"archive/zip"
	"fmt"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"io"
	"os"
	"path"
	"path/filepath"
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
	for p, v := range index.Files {
		if !v.IsMetaFile() {
			file, err := exp.Create(path.Join("overrides", p))
			if err != nil {
				notice.Errorf("Error creating file: %s", err.Error())
				// TODO: exit(1)?
				continue
			}
			// Attempt to read the file from disk, without checking hashes (assumed to have no errors)
			src, err := os.Open(index.ResolveIndexPath(p))
			if err != nil {
				_ = src.Close()
				notice.Errorf("Error reading file: %s", err.Error())
				// TODO: exit(1)?
				continue
			}
			_, err = io.Copy(file, src)
			if err != nil {
				_ = src.Close()
				notice.Errorf("Error copying file: %s", err.Error())
				// TODO: exit(1)?
				continue
			}

			_ = src.Close()
		}
	}
}

// AddDocOverrides saves the pack's README, licence and changelog (core.DocFiles), whichever of them are in the pack's
// folder, into the overrides folder of the zip. The index doesn't list them, so AddNonMetafileOverrides doesn't. A pack
// with none of them, or with a folder where one should be, is not an error: they are for the pack's author to write.
func AddDocOverrides(index *core.Index, exp *zip.Writer) {
	for _, name := range core.DocFiles {
		src := index.ResolveIndexPath(name)
		if info, err := os.Stat(src); err != nil || !info.Mode().IsRegular() {
			if err != nil && !os.IsNotExist(err) {
				notice.Errorf("Error reading file %s: %s", name, err.Error())
			}
			continue
		}
		in, err := os.Open(src)
		if err != nil {
			notice.Errorf("Error reading file %s: %s", name, err.Error())
			continue
		}
		file, err := exp.Create(path.Join("overrides", name))
		if err != nil {
			_ = in.Close()
			notice.Errorf("Error creating file %s: %s", name, err.Error())
			continue
		}
		if _, err = io.Copy(file, in); err != nil {
			notice.Errorf("Error copying file %s: %s", name, err.Error())
		}
		_ = in.Close()
	}
}

func PrintDisclaimer() {
	ui.Warning.Println("Disclaimer: you are responsible for ensuring you comply with ALL the licenses, or obtain appropriate permissions, for the files stored in the pack, which are listed below with paths in overrides/, client-overrides/ or server-overrides/")
	ui.Warning.Println("packwiz is currently unable to match metadata between mod sites - if any of these are available from Modrinth you should change them to use Modrinth metadata (e.g. by re-adding them using the mr commands)")
	fmt.Println()
}
