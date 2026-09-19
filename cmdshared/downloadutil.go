package cmdshared

import (
	"archive/zip"
	"fmt"
	"github.com/evictedcucumber/packwiz/core"
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
		ui.Error.Printf("Download of %s (%s) failed: %v\n", dl.Mod.Name, dl.Mod.FileName, dl.Error)
		return false
	}
	for _, warning := range dl.Warnings {
		ui.Warning.Printf("Warning for %s (%s): %v\n", dl.Mod.Name, dl.Mod.FileName, warning)
	}

	p, err := index.RelIndexPath(dl.Mod.GetDestFilePath())
	if err != nil {
		ui.Error.Printf("Error resolving external file: %v\n", err)
		return false
	}
	modFile, err := exp.Create(path.Join(dir, p))
	if err != nil {
		ui.Error.Printf("Error creating metadata file %s: %v\n", p, err)
		return false
	}
	_, err = io.Copy(modFile, dl.File)
	if err != nil {
		ui.Error.Printf("Error copying file %s: %v\n", p, err)
		return false
	}
	err = dl.File.Close()
	if err != nil {
		ui.Error.Printf("Error closing file %s: %v\n", p, err)
		return false
	}

	fmt.Printf("%s %s added to zip\n", ui.Bold.Sprint(dl.Mod.Name), ui.Muted.Sprintf("(%s)", dl.Mod.FileName))
	return true
}

// AddNonMetafileOverrides saves all non-metadata files into an overrides folder in the zip
func AddNonMetafileOverrides(index *core.Index, exp *zip.Writer) {
	for p, v := range index.Files {
		if !v.IsMetaFile() {
			file, err := exp.Create(path.Join("overrides", p))
			if err != nil {
				ui.Error.Printf("Error creating file: %s\n", err.Error())
				// TODO: exit(1)?
				continue
			}
			// Attempt to read the file from disk, without checking hashes (assumed to have no errors)
			src, err := os.Open(index.ResolveIndexPath(p))
			if err != nil {
				_ = src.Close()
				ui.Error.Printf("Error reading file: %s\n", err.Error())
				// TODO: exit(1)?
				continue
			}
			_, err = io.Copy(file, src)
			if err != nil {
				_ = src.Close()
				ui.Error.Printf("Error copying file: %s\n", err.Error())
				// TODO: exit(1)?
				continue
			}

			_ = src.Close()
		}
	}
}

func PrintDisclaimer() {
	ui.Warning.Println("Disclaimer: you are responsible for ensuring you comply with ALL the licenses, or obtain appropriate permissions, for the files \"added to zip\" below")
	ui.Warning.Println("packwiz is currently unable to match metadata between mod sites - if any of these are available from Modrinth you should change them to use Modrinth metadata (e.g. by re-adding them using the mr commands)")
	fmt.Println()
}
