package modrinth

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// exportEnv is what a .mrpack tells the launcher about a mod on the given side: whether it is needed on the client and
// on the server, each "required" or "optional" where the mod is installed and "unsupported" where it isn't
func exportEnv(side string, optional bool) (client, server string) {
	installed := "required"
	if optional {
		installed = "optional"
	}
	switch side {
	case core.ClientSide:
		return installed, "unsupported"
	case core.ServerSide:
		return "unsupported", installed
	}
	return installed, installed
}

// exportedFile is a mod that went into an exported pack
type exportedFile struct {
	name string
	// path is where the file is in the pack: what the launcher downloads it as, or for a file stored in the pack itself
	// (bundled), where it is in the zip
	path string
	// client and server say whether it is needed there, as exportEnv does
	client, server string
	size           uint64
	bundled        bool
}

// bundledFile describes a mod that was stored in the zip, in folder, rather than left for the launcher to download
func bundledFile(dl core.CompletedDownload, folder string, index *core.Index) exportedFile {
	// Optional is a matter for the launcher, which isn't given a file that is in the zip
	client, server := exportEnv(dl.Mod.Side, false)
	f := exportedFile{name: dl.Mod.Name, path: dl.Mod.FileName, client: client, server: server, bundled: true}
	if p, err := index.RelIndexPath(dl.Mod.GetDestFilePath()); err == nil {
		f.path = folder + "/" + p
	}
	f.size, _ = strconv.ParseUint(dl.Hashes["length-bytes"], 10, 64)
	return f
}

// installedOn reports whether a file is put on a side, given what exportEnv said of it
func installedOn(env string) bool {
	return env != "unsupported"
}

// breakdown describes the files that went into an exported pack, a line for each in the order of their paths, with the
// sides they are needed on, and adds up what there is. It is "" if there are none.
func breakdown(files []exportedFile) string {
	if len(files) == 0 {
		return ""
	}
	files = slices.SortedFunc(slices.Values(files), func(a, b exportedFile) int { return strings.Compare(a.path, b.path) })

	// Text is padded before it is styled, so that the columns line up with or without colour
	const clientHeader, serverHeader, sizeHeader = "Client", "Server", "Size"
	nameWidth, clientWidth, serverWidth, sizeWidth := len("Mod"), len(clientHeader), len(serverHeader), len(sizeHeader)
	sizes := make([]string, len(files))
	for i, f := range files {
		sizes[i] = formatSize(f.size)
		nameWidth = max(nameWidth, utf8.RuneCountInString(f.name))
		clientWidth = max(clientWidth, len(f.client))
		serverWidth = max(serverWidth, len(f.server))
		sizeWidth = max(sizeWidth, len(sizes[i]))
	}

	var b strings.Builder
	b.WriteString(ui.Bold.Sprint("Exported files:") + "\n")
	b.WriteString(ui.Muted.Sprintf("%s  %s  %s  %s  %s", padRight("Mod", nameWidth), padRight(clientHeader, clientWidth), padRight(serverHeader, serverWidth), padLeft(sizeHeader, sizeWidth), "File") + "\n")
	for i, f := range files {
		fmt.Fprintf(&b, "%s  %s  %s  %s  %s\n",
			ui.Bold.Sprint(padRight(f.name, nameWidth)),
			styleEnv(f.client, clientWidth), styleEnv(f.server, serverWidth),
			ui.Muted.Sprint(padLeft(sizes[i], sizeWidth)), f.path)
	}
	b.WriteString(ui.Info.Sprint(summarise(files)) + "\n")
	return b.String()
}

// styleEnv shows what a side needs of a file, so that where it isn't wanted fades back
func styleEnv(env string, width int) string {
	padded := padRight(env, width)
	if !installedOn(env) {
		return ui.Muted.Sprint(padded)
	}
	return padded
}

// summarise adds up a pack's files on a line: how many, how big, and what sides they are on
func summarise(files []exportedFile) string {
	var both, clientOnly, serverOnly, optional, bundled int
	var total uint64
	for _, f := range files {
		client, server := installedOn(f.client), installedOn(f.server)
		switch {
		case client && server:
			both++
		case client:
			clientOnly++
		case server:
			serverOnly++
		}
		if f.client == "optional" || f.server == "optional" {
			optional++
		}
		if f.bundled {
			bundled++
		}
		total += f.size
	}

	summary := fmt.Sprintf("%d %s, %s", len(files), plural(len(files), "file"), formatSize(total))
	var sides []string
	if both > 0 {
		sides = append(sides, fmt.Sprintf("%d on both sides", both))
	}
	if clientOnly > 0 {
		sides = append(sides, fmt.Sprintf("%d client only", clientOnly))
	}
	if serverOnly > 0 {
		sides = append(sides, fmt.Sprintf("%d server only", serverOnly))
	}
	if len(sides) > 0 {
		summary += ": " + strings.Join(sides, ", ")
	}
	if optional > 0 {
		summary += fmt.Sprintf("; %d optional", optional)
	}
	if bundled > 0 {
		summary += fmt.Sprintf("; %d stored in the pack itself, not downloaded by the launcher", bundled)
	}
	return summary
}

// formatSize writes a number of bytes as people read them: 512 B, 1.5 KiB, 2.3 MiB
func formatSize(bytes uint64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	size := float64(bytes) / 1024
	for _, unit := range []string{"KiB", "MiB", "GiB"} {
		if size < 1024 {
			return fmt.Sprintf("%.1f %s", size, unit)
		}
		size /= 1024
	}
	return fmt.Sprintf("%.1f TiB", size)
}

func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s)))
}

func padLeft(s string, width int) string {
	return strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s))) + s
}

// plural is the form of a noun that goes with a count
func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}
