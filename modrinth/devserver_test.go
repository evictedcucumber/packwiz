package modrinth

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
)

// TestHelperFakeServer isn't a test: it is what the dev server's tests run as their server, the test binary run again as
// a process, in a mode of "serve" or "hang". It echoes what it is sent, stops on "stop" (unless it hangs), and fails on
// "crash".
func TestHelperFakeServer(t *testing.T) {
	mode := os.Getenv("PACKWIZ_FAKE_SERVER")
	if mode == "" {
		return
	}
	fmt.Println("fake server started")
	lines := bufio.NewScanner(os.Stdin)
	for lines.Scan() {
		switch line := lines.Text(); {
		case line == "stop" && mode != "hang":
			fmt.Println("fake server stopping")
			os.Exit(0)
		case line == "crash":
			os.Exit(3)
		default:
			fmt.Println("server got: " + line)
		}
	}
	os.Exit(0)
}

// fakeServerCommand starts TestHelperFakeServer, in mode
func fakeServerCommand(mode string) func() (*exec.Cmd, error) {
	return func() (*exec.Cmd, error) {
		c := exec.Command(os.Args[0], "-test.run=^TestHelperFakeServer$")
		c.Env = append(os.Environ(), "PACKWIZ_FAKE_SERVER="+mode)
		return c, nil
	}
}

// devFixture is a dev server in a folder of its own, installed from a folder that has user_jvm_args.txt, whose server
// pack is files, which a test can change between resyncs
type devFixture struct {
	*devServer
	files map[string]string
}

func newDevFixture(t *testing.T, mode string) *devFixture {
	t.Helper()
	install := t.TempDir()
	writeTestFile(t, filepath.Join(install, "user_jvm_args.txt"), "# the installer's")
	writeTestFile(t, filepath.Join(install, installedMarker), "")
	f := &devFixture{files: map[string]string{}}
	f.devServer = &devServer{
		dir: t.TempDir(), install: install, command: fakeServerCommand(mode), stopTimeout: 10 * time.Second,
		write: func(a cmdshared.Archive) error {
			for name, content := range f.files {
				if err := a.Add(name, strings.NewReader(content)); err != nil {
					return err
				}
			}
			return nil
		},
	}
	t.Cleanup(func() { _ = f.stopServer() })
	if err := f.copyInstall(); err != nil {
		t.Fatalf("copyInstall() returned error: %v", err)
	}
	return f
}

func (f *devFixture) read(t *testing.T, name string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(name)))
	return string(data), err == nil
}

func TestDevServerIsSetUpFromTheInstallWithoutPackwizsOwnFiles(t *testing.T) {
	f := newDevFixture(t, "serve")
	if got, ok := f.read(t, "user_jvm_args.txt"); !ok || got != "# the installer's" {
		t.Errorf("user_jvm_args.txt = %q, %v, want the installer's", got, ok)
	}
	if _, ok := f.read(t, installedMarker); ok {
		t.Errorf("the folder has %s, which is packwiz's", installedMarker)
	}
}

func TestResyncPutsThePackInAndTakesOutOnlyWhatItPutThereBefore(t *testing.T) {
	f := newDevFixture(t, "serve")
	f.files["server.properties"] = "motd=one"
	f.files["config/a/b.toml"] = "b"
	f.files["user_jvm_args.txt"] = "-Xmx4G"
	cmdtest.CaptureStdout(t, func() {
		if err := f.resync(); err != nil {
			t.Fatalf("resync() returned error: %v", err)
		}
	})
	if got, _ := f.read(t, "user_jvm_args.txt"); got != "-Xmx4G" {
		t.Errorf("user_jvm_args.txt = %q, want the pack's, in place of the installer's", got)
	}
	// What the server made is the server's
	writeTestFile(t, filepath.Join(f.dir, "world", "level.dat"), "the world")

	f.files["server.properties"] = "motd=two"
	delete(f.files, "config/a/b.toml")
	delete(f.files, "user_jvm_args.txt")
	out := cmdtest.CaptureStdout(t, func() {
		if err := f.resync(); err != nil {
			t.Fatalf("resync() returned error: %v", err)
		}
	})
	if !strings.Contains(out, "Resynced 1 files, and took out 2 the pack no longer has") {
		t.Errorf("resync() said %q, want what it did", out)
	}
	if got, _ := f.read(t, "server.properties"); got != "motd=two" {
		t.Errorf("server.properties = %q, want it written again", got)
	}
	if _, ok := f.read(t, "config/a/b.toml"); ok {
		t.Error("config/a/b.toml is still there, though the pack no longer has it")
	}
	if _, err := os.Stat(filepath.Join(f.dir, "config")); !os.IsNotExist(err) {
		t.Errorf("the folders left empty are still there (%v)", err)
	}
	if got, _ := f.read(t, "user_jvm_args.txt"); got != "# the installer's" {
		t.Errorf("user_jvm_args.txt = %q, want the installer's put back", got)
	}
	if got, ok := f.read(t, "world/level.dat"); !ok || got != "the world" {
		t.Errorf("the world is %q, %v, want it left alone", got, ok)
	}
}

func TestTheConsoleStartsTheServerSendsItWhatIsTypedAndStopsIt(t *testing.T) {
	f := newDevFixture(t, "serve")
	lines := make(chan string, 10)
	for _, line := range []string{"status", "start", ":start", "say hello", ":status", ":stop", "stop", "say nothing", "exit", "never read"} {
		lines <- line
	}
	out := cmdtest.CaptureStdout(t, func() { f.run(lines, nil) })

	for _, want := range []string{
		"The server isn't running; its folder is " + f.dir,
		"Starting the server", "fake server started", "The server is already running",
		"server got: say hello", "The server is running, in " + f.dir,
		"fake server stopping", "The server stopped",
		"The server isn't running\n",
		`Unknown command "say nothing"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the console doesn't say %q:\n%s", want, out)
		}
	}
	// What starts with a colon is always the console's
	if strings.Contains(out, "server got: :start") || strings.Contains(out, "server got: :status") {
		t.Errorf("a command of the console was sent to the server:\n%s", out)
	}
	if len(lines) != 1 {
		t.Errorf("%d lines are left, want the session to have ended at exit", len(lines))
	}
}

func TestTheConsoleEndsWhenThereIsNothingMoreToRead(t *testing.T) {
	f := newDevFixture(t, "serve")
	lines := make(chan string)
	close(lines)
	cmdtest.CaptureStdout(t, func() { f.run(lines, nil) })
}

func TestTheConsoleSaysWhenTheServerStopsOfItsOwnAccord(t *testing.T) {
	f := newDevFixture(t, "serve")
	out := cmdtest.CaptureStdout(t, func() {
		f.handle("start")
		f.handle("crash")
		<-f.server.done
		f.reportExit()
	})
	if !strings.Contains(out, "The server stopped: exit status 3") {
		t.Errorf("the console doesn't say the server stopped and why:\n%s", out)
	}
	if f.running() {
		t.Error("the server is still taken to be running")
	}
}

func TestRestartStopsTheServerAndStartsItAgain(t *testing.T) {
	f := newDevFixture(t, "serve")
	out := cmdtest.CaptureStdout(t, func() {
		f.handle("restart")
		f.handle(":restart")
		f.handle(":stop")
	})
	if strings.Count(out, "fake server started") != 2 || strings.Count(out, "fake server stopping") != 2 {
		t.Errorf("the server wasn't started twice and stopped twice:\n%s", out)
	}
}

func TestAServerThatDoesNotStopIsKilled(t *testing.T) {
	f := newDevFixture(t, "hang")
	f.stopTimeout = 200 * time.Millisecond
	out := cmdtest.CaptureStdout(t, func() {
		f.handle("start")
		f.handle(":stop")
	})
	if !strings.Contains(out, "didn't stop within 200ms, so it was killed") || f.running() {
		t.Errorf("the server wasn't killed:\n%s", out)
	}
}

func TestResyncWhileTheServerRunsSaysToRestart(t *testing.T) {
	f := newDevFixture(t, "serve")
	f.files["server.properties"] = "motd=one"
	out := cmdtest.CaptureStdout(t, func() {
		f.handle("start")
		f.handle(":resync")
		f.handle(":stop")
	})
	if !strings.Contains(out, "restart it to use what was resynced") {
		t.Errorf("resync doesn't say to restart:\n%s", out)
	}
	if got, _ := f.read(t, "server.properties"); got != "motd=one" {
		t.Errorf("server.properties = %q, want it resynced", got)
	}
}

func TestTheConsoleRunsShellCommandsInTheFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the command is written for sh")
	}
	f := newDevFixture(t, "serve")
	out := cmdtest.CaptureStdout(t, func() {
		f.handle("!echo made > made.txt && echo done")
		f.handle("!")
	})
	if got, _ := f.read(t, "made.txt"); got != "made\n" {
		t.Errorf("made.txt = %q, want it made in the folder", got)
	}
	if !strings.Contains(out, "done") || !strings.Contains(out, "Give a command to run after the !") {
		t.Errorf("the console said %q", out)
	}
}

func TestTheEULAIsAgreedToOnlyWhenSaidTo(t *testing.T) {
	for _, tc := range []struct {
		name, answer, eula string
		accept, agreed     bool
	}{
		{name: "yes", answer: "y\n", agreed: true},
		{name: "no", answer: "n\n"},
		{name: "no answer is no", answer: "\n"},
		{name: "anything but yes is no", answer: "maybe\n"},
		{name: "--accept-eula", accept: true, agreed: true},
		{name: "the server pack's eula.txt", eula: "eula=true\n", agreed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDevFixture(t, "serve")
			if tc.eula != "" {
				writeTestFile(t, filepath.Join(f.dir, "eula.txt"), tc.eula)
			}
			cmdtest.SetStdin(t, tc.answer)
			var err error
			cmdtest.CaptureStdout(t, func() { err = f.agreeToEULA(tc.accept) })
			got, _ := f.read(t, "eula.txt")
			if agreed := err == nil && strings.Contains(got, "eula=true"); agreed != tc.agreed {
				t.Errorf("agreeToEULA() = %v and eula.txt is %q, want agreed %v", err, got, tc.agreed)
			}
		})
	}
}

func TestTheServerOfThePacksModLoader(t *testing.T) {
	for _, tc := range []struct {
		mc, neoforge, name, url string
	}{
		{"1.21.1", "21.1.77", "neoforge-21.1.77", "https://maven.neoforged.net/releases/net/neoforged/neoforge/21.1.77/neoforge-21.1.77-installer.jar"},
		{"1.20.1", "47.1.106", "forge-1.20.1-47.1.106", "https://maven.neoforged.net/releases/net/neoforged/forge/1.20.1-47.1.106/forge-1.20.1-47.1.106-installer.jar"},
		{"1.20.1", "1.20.1-47.1.106", "forge-1.20.1-47.1.106", "https://maven.neoforged.net/releases/net/neoforged/forge/1.20.1-47.1.106/forge-1.20.1-47.1.106-installer.jar"},
	} {
		server, err := serverOf(core.Pack{Versions: map[string]string{"minecraft": tc.mc, "neoforge": tc.neoforge}})
		if err != nil || server.name != tc.name || server.installer != tc.url {
			t.Errorf("serverOf(%s, %s) = %+v, %v, want %s from %s", tc.mc, tc.neoforge, server, err, tc.name, tc.url)
		}
	}
	if _, err := serverOf(core.Pack{Versions: map[string]string{"minecraft": "1.21.1"}}); err == nil || !strings.Contains(err.Error(), "NeoForge") {
		t.Errorf("serverOf() of a pack without a mod loader = %v, want an error that says so", err)
	}
}

func TestTheServerIsStartedWithTheArgumentsTheInstallerWrote(t *testing.T) {
	dir := t.TempDir()
	if _, err := serverCommand("java", dir); err == nil {
		t.Error("serverCommand() of a folder with nothing installed returned no error")
	}
	name := "unix_args.txt"
	if runtime.GOOS == "windows" {
		name = "win_args.txt"
	}
	writeTestFile(t, filepath.Join(dir, "libraries", "net", "neoforged", "neoforge", "21.1.77", name), "-cp x")
	c, err := serverCommand("/path/to/java", dir)
	if err != nil {
		t.Fatalf("serverCommand() returned error: %v", err)
	}
	if want := []string{"/path/to/java", "@libraries/net/neoforged/neoforge/21.1.77/" + name, "nogui"}; strings.Join(c.Args, " ") != strings.Join(want, " ") || c.Dir != dir {
		t.Errorf("serverCommand() = %v in %s, want %v in %s", c.Args, c.Dir, want, dir)
	}
	writeTestFile(t, filepath.Join(dir, "user_jvm_args.txt"), "-Xmx2G")
	if c, _ = serverCommand("java", dir); c.Args[1] != "@user_jvm_args.txt" {
		t.Errorf("serverCommand() = %v, want the JVM's arguments first", c.Args)
	}
}

func TestAnInstalledServerIsTakenFromTheCache(t *testing.T) {
	cache := t.TempDir()
	cmdtest.SetViper(t, "cache.directory", cache)
	server := loaderServer{name: "neoforge-21.1.77", installer: "https://example.invalid/installer.jar"}
	installed := filepath.Join(cache, "dev-server", server.name)
	writeTestFile(t, filepath.Join(installed, installedMarker), "")
	// Nothing is downloaded or run: java isn't even there
	dir, err := cachedServer(server, "no-such-java")
	if err != nil || dir != installed {
		t.Errorf("cachedServer() = %q, %v, want %q", dir, err, installed)
	}
}

func TestTheDevServersFilesAreTheServerPack(t *testing.T) {
	exportablePack(t, validMod("alpha"))
	writeTestFile(t, "serverconfig/server.properties", "motd=hi")
	writeTestFile(t, "config/a.toml", "a")
	archive := cmdshared.NewDirArchive(t.TempDir())
	cmdtest.CaptureStdout(t, func() {
		if err := writeDevServerPack(archive); err != nil {
			t.Fatalf("writeDevServerPack() returned error: %v", err)
		}
	})
	for _, name := range []string{"mods/alpha.jar", "server.properties", "config/a.toml"} {
		if !archive.Added[name] {
			t.Errorf("the dev server doesn't have %s: %v", name, archive.Added)
		}
	}
	data, err := os.ReadFile(filepath.Join(archive.Root, "mods", "alpha.jar"))
	if err != nil || string(data) != "bytes of alpha" {
		t.Errorf("mods/alpha.jar is %q, %v, want the mod downloaded", data, err)
	}
}
