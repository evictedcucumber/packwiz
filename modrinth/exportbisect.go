package modrinth

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/notice"
)

// serverStarterVersion is the release of NeoForge's Server Starter Jar that a Bisect pack has: the jar that Bisect Hosting
// starts as server.jar, which reads the java command in the server's run script and starts the server with it
// (https://github.com/neoforged/ServerStarterJar)
const serverStarterVersion = "0.1.35"

// serverStarterURL is where the Server Starter Jar is downloaded from
var serverStarterURL = "https://github.com/neoforged/ServerStarterJar/releases/download/" + serverStarterVersion + "/server.jar"

// serverStarterSHA256 is the SHA-256 of that release's server.jar, which what is downloaded has to have to be kept
var serverStarterSHA256 = "d019d815868d451e57bdb965174b8443ec361336e84e9c41abc41a6c627bacd1"

// bisectJar is the jar in the Bisect pack that Bisect Hosting runs, as "java -jar server.jar"
const bisectJar = "server.jar"

// BisectPackName is the file the Bisect Hosting pack of a pack is written to unless told another, in the current
// directory.
func BisectPackName(pack core.Pack) string {
	return pack.GetPackName() + core.BisectPackSuffix
}

// BisectJava is the version of Java that Bisect Hosting's panel is to be set to for a version of Minecraft, and whether
// it is known: 17 for 1.18 to 1.20.4, 21 for 1.20.5 to 1.21.x and 25 from 26.1 on.
func BisectJava(minecraft string) (version int, ok bool) {
	var parts []int
	for _, part := range strings.Split(minecraft, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return 0, false
		}
		parts = append(parts, n)
	}
	for len(parts) < 3 {
		parts = append(parts, 0)
	}
	major, minor, patch := parts[0], parts[1], parts[2]
	switch {
	case major >= 26:
		return 25, true
	case major != 1:
		return 0, false
	case minor > 20, minor == 20 && patch >= 5:
		return 21, true
	case minor >= 18:
		return 17, true
	}
	return 0, false
}

// bisectInstructions is what to do with the Bisect pack once it is written, as a line of plain text
func bisectInstructions(pack core.Pack, fileName string) string {
	java := "the version of Java that the pack's version of Minecraft needs"
	if mc, err := pack.GetMCVersion(); err == nil {
		if v, ok := BisectJava(mc); ok {
			java = "Java " + strconv.Itoa(v)
		}
	}
	return fmt.Sprintf("On Bisect Hosting: install \"Custom JAR\", upload %s and Unarchive it in the server's root folder, and set the Java version to %s on the Home tab", filepath.Base(fileName), java)
}

// cachedStarterJar is the Server Starter Jar in packwiz's cache, which is downloaded the first time it is needed. It is
// only put there once it has been downloaded in full.
func cachedStarterJar() (string, error) {
	cache, err := core.GetPackwizCache()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, cacheFolder)
	jar := filepath.Join(dir, "server-starter-"+serverStarterVersion+".jar")
	if info, err := os.Stat(jar); err == nil && info.Size() > 0 {
		return jar, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	notice.Mutedf("Downloading the Server Starter Jar %s (only the first time)...", serverStarterVersion)
	part, err := os.CreateTemp(dir, "server-starter-*.part")
	if err != nil {
		return "", err
	}
	partName := part.Name()
	_ = part.Close()
	defer func() { _ = os.Remove(partName) }()
	if err := download(serverStarterURL, partName); err != nil {
		return "", fmt.Errorf("failed to download the Server Starter Jar: %w", err)
	}
	if info, err := os.Stat(partName); err != nil || info.Size() == 0 {
		return "", fmt.Errorf("the Server Starter Jar downloaded from %s is empty", serverStarterURL)
	}
	if sum, err := fileSHA256(partName); err != nil {
		return "", err
	} else if sum != serverStarterSHA256 {
		return "", fmt.Errorf("the Server Starter Jar downloaded from %s isn't the release it should be: its SHA-256 is %s, not %s", serverStarterURL, sum, serverStarterSHA256)
	}
	if err := os.Rename(partName, jar); err != nil {
		return "", err
	}
	return jar, nil
}

// fileSHA256 is the SHA-256 of a file, in hex
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// bisectServer is the folder the server of the pack's mod loader is installed in, in packwiz's cache. If it isn't
// installed yet that needs java, which is said if it can't be found.
func bisectServer(server loaderServer, java string) (string, error) {
	_, installed, err := serverCache(server)
	if err != nil {
		return "", err
	}
	if !installed {
		path, err := exec.LookPath(java)
		if err != nil {
			return "", fmt.Errorf("java is needed to install the %s server, which a Bisect Hosting pack has in it (only the first time): install Java, or say where it is with --java: %w", server.name, err)
		}
		// The installer is run in the cache, so a relative path would be looked for there
		if java, err = filepath.Abs(path); err != nil {
			return "", err
		}
	}
	return cachedServer(server, java)
}

// bisectArchive is a zip that a Bisect pack is put in. Each path is in it once, whichever is added first having it, and a
// script is added as one that is to be run.
type bisectArchive struct {
	zip  cmdshared.ZipArchive
	have map[string]bool
}

func newBisectArchive(w *zip.Writer) *bisectArchive {
	return &bisectArchive{zip: cmdshared.ZipArchive{Writer: w}, have: map[string]bool{}}
}

// Add puts the file in the archive, unless there is one of that path already, which is left as it is.
func (b *bisectArchive) Add(name string, content io.Reader) error {
	if b.have[name] {
		return nil
	}
	b.have[name] = true
	if strings.HasSuffix(name, ".sh") {
		return b.zip.AddMode(name, content, 0o755)
	}
	return b.zip.Add(name, content)
}

// addInstalled puts the files of the server installed in dir, and the Server Starter Jar, in the archive, in the order of
// their paths. What is in the archive already, which is the pack's, is left as it is: it replaces an installed file, as
// the pack's files do any other.
func (b *bisectArchive) addInstalled(dir, starter string) error {
	if err := cmdshared.AddFile(b, starter, bisectJar); err != nil {
		return err
	}
	installed, err := installedFiles(dir)
	if err != nil {
		return fmt.Errorf("Error reading the installed server: %v", err)
	}
	for _, p := range slices.Sorted(maps.Keys(installed)) {
		if err := cmdshared.AddFile(b, filepath.Join(dir, filepath.FromSlash(p)), p); err != nil {
			return err
		}
	}
	return nil
}

// exportBisectPack writes the Bisect Hosting pack of a pack as a zip: its server pack (see writeServerPack) with the
// server of its mod loader installed in it, as the loader's installer leaves it, and the Server Starter Jar as server.jar,
// all at the top of the zip, so that it can be unarchived in the root folder of a server there. The server is installed
// the first time it is needed, which needs java, and kept in packwiz's cache. What it would print it says through
// notice, and what it makes it returns.
func exportBisectPack(pack core.Pack, index *core.Index, mods []*core.Mod, options ExportOptions, hooks exportHooks) (result *ExportResult, err error) {
	// Everything that can fail before there is a file is found first, so that a pack that can't be made leaves nothing
	server, err := serverOf(pack)
	if err != nil {
		return nil, err
	}
	java := options.Java
	if java == "" {
		java = "java"
	}
	installDir, err := bisectServer(server, java)
	if err != nil {
		return nil, err
	}
	starter, err := cachedStarterJar()
	if err != nil {
		return nil, err
	}

	fileName := options.Output
	if fileName == "" {
		fileName = BisectPackName(pack)
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

	archive := newBisectArchive(exp)
	files, err := writeServerPack(pack, index, mods, archive, hooks)
	if err != nil {
		return nil, err
	}
	if err = archive.addInstalled(installDir, starter); err != nil {
		return nil, err
	}
	if err = exp.Close(); err != nil {
		return nil, errors.New("Error writing export file: " + err.Error())
	}
	if err = expFile.Close(); err != nil {
		return nil, errors.New("Error writing export file: " + err.Error())
	}
	return &ExportResult{Path: fileName, Server: true, Bisect: true, Instructions: bisectInstructions(pack, fileName), Files: files}, nil
}
