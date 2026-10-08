package modrinth

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/jarcoal/httpmock"
)

const starterJar = "bytes of the starter jar"

// bisectFixture is a pack with one mod, whose NeoForge server is installed in the cache already, with the files an
// installer leaves, and whose Server Starter Jar is served at serverStarterURL (counted: starterDownloads).
func bisectFixture(t *testing.T) (installed string) {
	t.Helper()
	pack, _ := exportablePack(t, validMod("alpha"))
	// So the pack has nothing to warn of
	serverModList(t)
	server, err := serverOf(pack)
	if err != nil {
		t.Fatalf("serverOf() returned error: %v", err)
	}
	dir, _, err := serverCache(server)
	if err != nil {
		t.Fatalf("serverCache() returned error: %v", err)
	}
	writeTestFile(t, filepath.Join(dir, installedMarker), "")
	writeTestFile(t, filepath.Join(dir, installLog), "the installer's log")
	writeTestFile(t, filepath.Join(dir, "installer.jar"), "the installer")
	writeTestFile(t, filepath.Join(dir, "libraries", "net", "neoforged", "forge", "1.0", "unix_args.txt"), "-cp libs")
	writeTestFile(t, filepath.Join(dir, "libraries", "com", "example", "lib.jar"), "a library")
	writeTestFile(t, filepath.Join(dir, "run.sh"), "java @user_jvm_args.txt @libraries/net/neoforged/forge/1.0/unix_args.txt nogui")
	writeTestFile(t, filepath.Join(dir, "run.bat"), "java ...")
	writeTestFile(t, filepath.Join(dir, "user_jvm_args.txt"), "# the installer's")

	old := serverStarterURL
	serverStarterURL = "https://example.invalid/server-starter/server.jar"
	oldSum := serverStarterSHA256
	serverStarterSHA256 = starterJarSHA256()
	t.Cleanup(func() { serverStarterURL, serverStarterSHA256 = old, oldSum })
	httpmock.RegisterResponder("GET", serverStarterURL, httpmock.NewStringResponder(200, starterJar))
	return dir
}

func starterJarSHA256() string {
	sum := sha256.Sum256([]byte(starterJar))
	return hex.EncodeToString(sum[:])
}

func starterDownloads() int {
	return httpmock.GetCallCountInfo()["GET "+serverStarterURL]
}

func TestBisectExportHasTheInstalledServerTheStarterJarAndThePacksFilesAtTheRoot(t *testing.T) {
	bisectFixture(t)
	writeTestFile(t, "config/beta.toml", "shared = true")
	writeTestFile(t, "README.md", "about the pack")
	writeTestFile(t, "serverconfig/server.properties", "motd=hi")
	writeTestFile(t, "serverconfig/user_jvm_args.txt", "-Xmx1G")
	output := t.TempDir() + "/bisect.zip"

	var result *ExportResult
	var err error
	out := cmdtest.CaptureStdout(t, func() {
		result, err = Export(ExportOptions{Output: output, Bisect: true}, nil)
	})
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("Export() wrote %q to the terminal", out)
	}
	if !result.Server || !result.Bisect || result.Path != output {
		t.Errorf("the result is %+v, want the Bisect pack at %s, which is a server pack", result, output)
	}
	if len(result.Notices) != 0 {
		t.Errorf("the notices are %v, want none", result.Notices)
	}
	// The pack is on a version of Minecraft that Bisect runs with Java 17
	for _, want := range []string{"Custom JAR", "Unarchive", "Java 17", "bisect.zip"} {
		if !strings.Contains(result.Instructions, want) {
			t.Errorf("Instructions = %q, want it to say %q", result.Instructions, want)
		}
	}
	// The table is of the pack's files, not of the server's
	if len(result.Files) != 1 || result.Files[0].Path != "mods/alpha.jar" {
		t.Errorf("Files = %+v, want only the mod", result.Files)
	}

	files := readZip(t, output)
	want := map[string]string{
		"server.jar": starterJar,
		"libraries/net/neoforged/forge/1.0/unix_args.txt": "-cp libs",
		"libraries/com/example/lib.jar":                   "a library",
		"run.sh":                                          "java @user_jvm_args.txt @libraries/net/neoforged/forge/1.0/unix_args.txt nogui",
		"run.bat":                                         "java ...",
		"mods/alpha.jar":                                  "bytes of alpha",
		"config/beta.toml":                                "shared = true",
		"README.md":                                       "about the pack",
		"server.properties":                               "motd=hi",
		// The pack's own file replaces the installer's
		"user_jvm_args.txt": "-Xmx1G",
	}
	for name, content := range want {
		if got, ok := files[name]; !ok || got != content {
			t.Errorf("%s holds %q (there: %v), want %q", name, got, ok, content)
		}
	}
	// The server's list of mods is the pack's MODS.md
	if _, ok := files["MODS.md"]; !ok {
		t.Error("the zip has no MODS.md")
	}
	delete(files, "MODS.md")
	if len(files) != len(want) {
		t.Errorf("the zip has %d files, want %d: %v", len(files), len(want), files)
	}
	for _, name := range []string{installedMarker, installLog, "installer.jar", "serverconfig/server.properties"} {
		if _, ok := files[name]; ok {
			t.Errorf("the zip has %s", name)
		}
	}
}

func TestBisectExportTakesThePacksServerJarOverTheStarterJarAndThatOverTheInstallers(t *testing.T) {
	dir := bisectFixture(t)
	writeTestFile(t, filepath.Join(dir, "server.jar"), "the installer's")
	output := t.TempDir() + "/bisect.zip"
	if _, err := Export(ExportOptions{Output: output, Bisect: true}, nil); err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if got := readZip(t, output)["server.jar"]; got != starterJar {
		t.Errorf("server.jar holds %q, want the starter jar over the installer's", got)
	}

	writeTestFile(t, "serverconfig/server.jar", "the pack's")
	if _, err := Export(ExportOptions{Output: output, Bisect: true}, nil); err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if got := readZip(t, output)["server.jar"]; got != "the pack's" {
		t.Errorf("server.jar holds %q, want the pack's own over the starter jar", got)
	}
}

func TestBisectExportKeepsScriptsExecutable(t *testing.T) {
	bisectFixture(t)
	writeTestFile(t, "serverconfig/start.sh", "echo hi")
	output := t.TempDir() + "/bisect.zip"
	if _, err := Export(ExportOptions{Output: output, Bisect: true}, nil); err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	r, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("failed to open %s: %v", output, err)
	}
	defer func() { _ = r.Close() }()
	for _, f := range r.File {
		executable := f.Mode()&0o111 != 0
		if want := strings.HasSuffix(f.Name, ".sh"); executable != want {
			t.Errorf("%s has mode %v, want executable: %v", f.Name, f.Mode(), want)
		}
	}
}

func TestBisectExportOfAPackWithoutNeoForgeFailsWritingNothing(t *testing.T) {
	pack, index := exportablePack(t, validMod("alpha"))
	delete(pack.Versions, "neoforge")
	saveFixture(t, pack, index)
	output := t.TempDir() + "/bisect.zip"
	_, err := Export(ExportOptions{Output: output, Bisect: true}, nil)
	if err == nil || !strings.Contains(err.Error(), "NeoForge") {
		t.Errorf("Export() returned %v, want an error that the pack has no NeoForge", err)
	}
	if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("the pack was written anyway: %v", statErr)
	}
}

func TestBisectExportDownloadsTheStarterJarOnceAndThenTakesItFromTheCache(t *testing.T) {
	bisectFixture(t)
	output := t.TempDir() + "/bisect.zip"
	for i := 0; i < 2; i++ {
		if _, err := Export(ExportOptions{Output: output, Bisect: true}, nil); err != nil {
			t.Fatalf("Export() returned error: %v", err)
		}
	}
	if n := starterDownloads(); n != 1 {
		t.Errorf("the starter jar was downloaded %d times, want once", n)
	}
	if got := readZip(t, output)["server.jar"]; got != starterJar {
		t.Errorf("server.jar holds %q, want the starter jar", got)
	}
}

func TestBisectExportDoesNotKeepAStarterJarThatFailedToDownload(t *testing.T) {
	bisectFixture(t)
	output := t.TempDir() + "/bisect.zip"
	for name, responder := range map[string]httpmock.Responder{
		"an error":    httpmock.NewStringResponder(http.StatusNotFound, "not found"),
		"nothing":     httpmock.NewStringResponder(http.StatusOK, ""),
		"a failure":   httpmock.NewErrorResponder(errors.New("connection reset")),
		"another jar": httpmock.NewStringResponder(http.StatusOK, "not the release"),
	} {
		httpmock.RegisterResponder("GET", serverStarterURL, responder)
		if _, err := Export(ExportOptions{Output: output, Bisect: true}, nil); err == nil || !strings.Contains(err.Error(), "Server Starter Jar") {
			t.Errorf("Export() with %s returned %v, want an error about the starter jar", name, err)
		}
		if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the pack was written anyway, with %s: %v", name, err)
		}
		cache, _ := core.GetPackwizCache()
		if entries, _ := filepath.Glob(filepath.Join(cache, cacheFolder, "server-starter-*")); len(entries) != 0 {
			t.Errorf("the cache has %v after %s", entries, name)
		}
	}

	httpmock.RegisterResponder("GET", serverStarterURL, httpmock.NewStringResponder(200, starterJar))
	if _, err := Export(ExportOptions{Output: output, Bisect: true}, nil); err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if got := readZip(t, output)["server.jar"]; got != starterJar {
		t.Errorf("server.jar holds %q, want the starter jar", got)
	}
}

func TestBisectCannotBeExportedWithAServerPackOrADevPack(t *testing.T) {
	bisectFixture(t)
	output := t.TempDir() + "/bisect.zip"
	for _, options := range []ExportOptions{{Bisect: true, Server: true}, {Bisect: true, Dev: true}} {
		options.Output = output
		if _, err := Export(options, nil); err == nil || !strings.Contains(err.Error(), "can't be exported together") {
			t.Errorf("Export(%+v) returned %v, want it refused", options, err)
		}
		if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a pack was written anyway: %v", err)
		}
	}
}

func TestBisectExportIsNamedAfterThePackByDefault(t *testing.T) {
	bisectFixture(t)
	result, err := Export(ExportOptions{Bisect: true}, nil)
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if result.Path != "Test Pack-1.0.0-bisect.zip" {
		t.Errorf("Path = %q, want the pack's name and version, as a Bisect pack", result.Path)
	}
	// And exporting again doesn't put the last one in the pack
	if _, err := Export(ExportOptions{Bisect: true}, nil); err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if _, ok := readZip(t, result.Path)[result.Path]; ok {
		t.Errorf("the Bisect pack has the Bisect pack exported before it")
	}
}

func TestBisectExportFailsWhereThePackCannotBeWritten(t *testing.T) {
	bisectFixture(t)
	if _, err := Export(ExportOptions{Output: t.TempDir() + "/no-such-folder/bisect.zip", Bisect: true}, nil); err == nil || !strings.Contains(err.Error(), "Failed to create zip") {
		t.Errorf("Export() returned %v, want an error that the pack couldn't be created", err)
	}
}

// fakeJava is a java that does what the NeoForge installer does, as a script, which the first export installs the
// server with. It fails unless it is run as the installer is.
func fakeJava(t *testing.T) string {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if runtime.GOOS == "windows" || err != nil {
		t.Skip("a fake java is a shell script, and there is no sh")
	}
	script := filepath.Join(t.TempDir(), "java")
	writeTestFile(t, script, "#!"+sh+`
[ "$1 $2 $3 $4" = "-jar installer.jar --installServer ." ] || exit 2
[ "$(cat installer.jar)" = "the installer" ] || exit 3
mkdir -p libraries/net/neoforged/forge/1.0
echo "-cp libs" > libraries/net/neoforged/forge/1.0/unix_args.txt
echo "java nogui" > run.sh
echo "# the installer's" > user_jvm_args.txt
echo "log" > installer.jar.log
`)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatalf("Chmod() returned error: %v", err)
	}
	return script
}

func TestBisectExportInstallsTheServerTheFirstTimeAndThenTakesItFromTheCache(t *testing.T) {
	java := fakeJava(t)
	pack, _ := exportablePack(t, validMod("alpha"))
	serverModList(t)
	server, err := serverOf(pack)
	if err != nil {
		t.Fatalf("serverOf() returned error: %v", err)
	}
	httpmock.RegisterResponder("GET", server.installer, httpmock.NewStringResponder(200, "the installer"))
	old := serverStarterURL
	serverStarterURL = "https://example.invalid/server-starter/server.jar"
	oldSum := serverStarterSHA256
	serverStarterSHA256 = starterJarSHA256()
	t.Cleanup(func() { serverStarterURL, serverStarterSHA256 = old, oldSum })
	httpmock.RegisterResponder("GET", serverStarterURL, httpmock.NewStringResponder(200, starterJar))
	output := t.TempDir() + "/bisect.zip"

	var result *ExportResult
	out := cmdtest.CaptureStdout(t, func() {
		result, err = Export(ExportOptions{Output: output, Bisect: true, Java: java}, nil)
	})
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
	if out != "" {
		t.Errorf("Export() wrote %q to the terminal", out)
	}
	if len(result.Notices) != 0 {
		t.Errorf("the notices are %v, want none (what is being done is not one)", result.Notices)
	}
	files := readZip(t, output)
	for name, content := range map[string]string{
		"server.jar": starterJar,
		"libraries/net/neoforged/forge/1.0/unix_args.txt": "-cp libs\n",
		"run.sh":            "java nogui\n",
		"user_jvm_args.txt": "# the installer's\n",
		"mods/alpha.jar":    "bytes of alpha",
	} {
		if got, ok := files[name]; !ok || got != content {
			t.Errorf("%s holds %q (there: %v), want %q", name, got, ok, content)
		}
	}
	for _, name := range []string{installedMarker, installLog, "installer.jar", "installer.jar.log"} {
		if _, ok := files[name]; ok {
			t.Errorf("the zip has %s, which is packwiz's or the installer's", name)
		}
	}

	// It is installed now, so neither the installer nor java is needed again
	if _, err := Export(ExportOptions{Output: output, Bisect: true, Java: "no-such-java"}, nil); err != nil {
		t.Fatalf("the second Export() returned error: %v", err)
	}
	if n := httpmock.GetCallCountInfo()["GET "+server.installer]; n != 1 {
		t.Errorf("the installer was downloaded %d times, want once", n)
	}
}

func TestBisectExportSaysWhenJavaIsNeededAndNotThere(t *testing.T) {
	pack, _ := exportablePack(t, validMod("alpha"))
	server, _ := serverOf(pack)
	httpmock.RegisterResponder("GET", server.installer, httpmock.NewStringResponder(200, "the installer"))
	output := t.TempDir() + "/bisect.zip"
	_, err := Export(ExportOptions{Output: output, Bisect: true, Java: "no-such-java"}, nil)
	if err == nil || !strings.Contains(err.Error(), "--java") || !strings.Contains(err.Error(), "install Java") {
		t.Errorf("Export() returned %v, want an error that says how to give it java", err)
	}
	if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("the pack was written anyway: %v", statErr)
	}
	if n := httpmock.GetCallCountInfo()["GET "+server.installer]; n != 0 {
		t.Errorf("the installer was downloaded, though java wasn't there to run it")
	}
}

func TestBisectExportDoesNotKeepAServerThatFailedToInstall(t *testing.T) {
	java := fakeJava(t)
	pack, _ := exportablePack(t, validMod("alpha"))
	server, _ := serverOf(pack)
	// The script fails unless the installer is the one it expects
	httpmock.RegisterResponder("GET", server.installer, httpmock.NewStringResponder(200, "not the installer"))
	output := t.TempDir() + "/bisect.zip"
	if _, err := Export(ExportOptions{Output: output, Bisect: true, Java: java}, nil); err == nil || !strings.Contains(err.Error(), "installer") {
		t.Errorf("Export() returned %v, want an error that the installer failed", err)
	}
	if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("the pack was written anyway: %v", statErr)
	}
	if _, installed, _ := serverCache(server); installed {
		t.Error("the server that failed to install is taken for installed")
	}
}

func TestBisectJavaIsTheOneTheVersionOfMinecraftNeeds(t *testing.T) {
	for mc, want := range map[string]int{
		"1.20.1": 17, "1.20.2": 17, "1.20.4": 17, "1.20.5": 21, "1.20.6": 21, "1.21": 21, "1.21.1": 21, "1.21.11": 21, "26.1": 25, "26.2.1": 25,
	} {
		if got, ok := BisectJava(mc); !ok || got != want {
			t.Errorf("BisectJava(%q) = %d, %v, want %d", mc, got, ok, want)
		}
	}
	for _, mc := range []string{"", "24w14a", "1.16.5", "x.y"} {
		if got, ok := BisectJava(mc); ok {
			t.Errorf("BisectJava(%q) = %d, want it unknown", mc, got)
		}
	}
	instructions := bisectInstructions(core.Pack{Versions: map[string]string{"minecraft": "1.21.1"}}, "dir/pack-bisect.zip")
	if !strings.Contains(instructions, "Java 21") || !strings.Contains(instructions, "upload pack-bisect.zip") {
		t.Errorf("bisectInstructions() = %q, want the Java version and the file's name", instructions)
	}
}
