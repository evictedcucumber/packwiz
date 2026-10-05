package modrinth

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
)

// neoForgeMaven is where NeoForge's installers are downloaded from
var neoForgeMaven = "https://maven.neoforged.net/releases/net/neoforged/"

// loaderServer is the server of a mod loader, for a version of Minecraft.
type loaderServer struct {
	// name is what it is called, with its version, as a folder name: "neoforge-21.1.77"
	name string
	// installer is the URL of its installer
	installer string
}

// serverOf is the server of the pack's mod loader. Only NeoForge has one, as it is the only mod loader there is: for
// Minecraft 1.20.1 it is released as "forge", with versions written "1.20.1-47.1.106", where the pack records "47.1.106".
func serverOf(pack core.Pack) (loaderServer, error) {
	version, ok := pack.Versions["neoforge"]
	if !ok || version == "" {
		return loaderServer{}, errors.New("the pack has no mod loader to run a server of: pack.toml has no NeoForge version")
	}
	mc, err := pack.GetMCVersion()
	if err != nil {
		return loaderServer{}, err
	}
	artifact := "neoforge"
	if mc == "1.20.1" {
		artifact = "forge"
		if !strings.HasPrefix(version, mc+"-") {
			version = mc + "-" + version
		}
	}
	return loaderServer{
		name:      artifact + "-" + version,
		installer: neoForgeMaven + artifact + "/" + version + "/" + artifact + "-" + version + "-installer.jar",
	}, nil
}

// installedMarker is the file in a cached server's folder that says it was installed in full
const installedMarker = ".packwiz-installed"

// installLog is where the installer's output goes, in the cached server's folder
const installLog = "installer.log"

// cachedServer is the folder in packwiz's cache that the server of a mod loader is installed in, and so the folder each
// dev server copies it from. It is installed the first time it is needed, which runs its installer with java, and it is
// only ever used once its installer has finished.
func cachedServer(server loaderServer, java string) (string, error) {
	cache, err := core.GetPackwizCache()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "dev-server", server.name)
	if _, err := os.Stat(filepath.Join(dir, installedMarker)); err == nil {
		return dir, nil
	}

	// Anything there is what an installer that didn't finish left
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ui.Muted.Printf("Installing the %s server (only the first time)...\n", server.name)
	installer := filepath.Join(dir, "installer.jar")
	if err := download(server.installer, installer); err != nil {
		return "", fmt.Errorf("failed to download the installer of %s: %w", server.name, err)
	}
	log, err := os.Create(filepath.Join(dir, installLog))
	if err != nil {
		return "", err
	}
	install := exec.Command(java, "-jar", "installer.jar", "--installServer", ".")
	install.Dir = dir
	install.Stdout, install.Stderr = log, log
	err = install.Run()
	_ = log.Close()
	if err != nil {
		return "", fmt.Errorf("the installer of %s failed (%v): %s says why", server.name, err, filepath.Join(dir, installLog))
	}
	if err := os.WriteFile(filepath.Join(dir, installedMarker), nil, 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

// download saves what is at url to dest
func download(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// notInstalled are the files of a cached server's folder that are packwiz's or the installer's, not the server's
var notInstalled = map[string]bool{installedMarker: true, installLog: true, "installer.jar": true, "installer.jar.log": true}

// installedFiles are the files of the server installed in dir, by their paths relative to it with forward slashes
func installedFiles(dir string) (map[string]bool, error) {
	files := map[string]bool{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel = filepath.ToSlash(rel); !notInstalled[rel] {
			files[rel] = true
		}
		return nil
	})
	return files, err
}

// copyFile copies the file at src to dest, making its folder
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// serverArgsFile is the file of arguments that the installer wrote for starting the server installed in dir, for java to
// read (as @file), relative to dir: the one for this operating system, which is the same for every version of NeoForge
// but for where it is
func serverArgsFile(dir string) (string, error) {
	name := "unix_args.txt"
	if runtime.GOOS == "windows" {
		name = "win_args.txt"
	}
	matches, err := filepath.Glob(filepath.Join(dir, "libraries", "net", "neoforged", "*", "*", name))
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("the server's %s isn't where the installer puts it (found %d)", name, len(matches))
	}
	return filepath.Rel(dir, matches[0])
}

// serverCommand is the command that starts the server installed in dir, as the installer's run script does, but with java
// rather than whatever is on the PATH, and without its graphical console
func serverCommand(java, dir string) (*exec.Cmd, error) {
	args, err := serverArgsFile(dir)
	if err != nil {
		return nil, err
	}
	var command []string
	if _, err := os.Stat(filepath.Join(dir, "user_jvm_args.txt")); err == nil {
		command = append(command, "@user_jvm_args.txt")
	}
	command = append(command, "@"+filepath.ToSlash(args), "nogui")
	cmd := exec.Command(java, command...)
	cmd.Dir = dir
	return cmd, nil
}
