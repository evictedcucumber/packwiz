package modrinth

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/evictedcucumber/packwiz/cmd"
	"github.com/evictedcucumber/packwiz/cmdshared"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// eulaURL is the Minecraft EULA, which a server won't start without being told has been agreed to
const eulaURL = "https://aka.ms/MinecraftEULA"

// devCmd is the group of commands for trying the pack out while it is made
var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Try the pack out while you make it",
}

var devServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the pack's server in a folder of its own, with a console to start, stop and resync it",
	Long: `Makes a temporary folder, installs the pack's NeoForge server in it (installed once into packwiz's cache, and
copied from there after), and puts the server pack in it, as "packwiz modrinth export --server" would make it: the
server's mods, downloaded, the pack's config files, and the files in serverconfig/. Then it gives you a console:

    start          start the server
    stop           stop it
    restart        stop it if it is running, and start it
    resync         put the pack's server files in the folder again, as they are now (then restart to use them)
    status         say whether the server is running, and where the folder is
    !<command>     run a shell command in the folder, such as !ls or !cat logs/latest.log
    help           list these
    exit           stop the server, delete the folder, and end the session

While the server is running, anything else you type goes to the server's console (say hello, op name, stop), and the
commands above are written with a colon in front (:restart, :resync, :exit) so as not to be mistaken for the server's
own. Ctrl+C and Ctrl+D end the session as exit does.

resync writes the files again from the pack (refreshing its index first, as export does), so it picks up changes to
serverconfig/, to config files and to the mods, and it removes the files that it put there before that the pack no
longer has, putting back the installer's own where there was one. Anything else in the folder, such as the world and the
logs, is left alone.

The folder is deleted when the session ends, world and all, unless --keep is given. The server needs you to agree to
the Minecraft EULA (` + eulaURL + `), which you are asked to do unless serverconfig/eula.txt says you have, or
--accept-eula is given.`,
	Args: cobra.NoArgs,
	Run: func(c *cobra.Command, args []string) {
		if err := runDevServer(viper.GetString("dev.server.java"), viper.GetBool("dev.server.keep"), viper.GetBool("dev.server.accept-eula")); err != nil {
			ui.Error.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	devCmd.AddCommand(devServerCmd)
	devServerCmd.Flags().String("java", "java", "The java to run the server and its installer with")
	devServerCmd.Flags().Bool("keep", false, "Keep the server's folder when the session ends, rather than deleting it")
	devServerCmd.Flags().Bool("accept-eula", false, "Agree to the Minecraft EULA ("+eulaURL+") without being asked")
	_ = viper.BindPFlag("dev.server.java", devServerCmd.Flags().Lookup("java"))
	_ = viper.BindPFlag("dev.server.keep", devServerCmd.Flags().Lookup("keep"))
	_ = viper.BindPFlag("dev.server.accept-eula", devServerCmd.Flags().Lookup("accept-eula"))
	cmd.Add(devCmd)
}

// runDevServer is "packwiz dev server": it sets the server up, runs the console until the session ends, and deletes the
// folder unless keep is set.
func runDevServer(java string, keep, acceptEULA bool) (err error) {
	pack, err := core.LoadPack()
	if err != nil {
		return err
	}
	server, err := serverOf(pack)
	if err != nil {
		return err
	}
	if java, err = exec.LookPath(java); err != nil {
		return fmt.Errorf("java is needed to run the server: %w", err)
	}
	install, err := cachedServer(server, java)
	if err != nil {
		return err
	}

	dir, err := os.MkdirTemp("", "packwiz-dev-server-*")
	if err != nil {
		return err
	}
	d := &devServer{dir: dir, install: install, write: writeDevServerPack, stopTimeout: time.Minute}
	d.command = func() (*exec.Cmd, error) { return serverCommand(java, dir) }
	defer func() {
		if stopErr := d.stopServer(); stopErr != nil && err == nil {
			err = stopErr
		}
		if keep {
			ui.Info.Printf("The server's folder is kept at %s\n", ui.Bold.Sprint(dir))
			return
		}
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			ui.Error.Printf("Failed to delete the server's folder %s: %v\n", dir, rmErr)
			return
		}
		ui.Muted.Printf("Deleted %s\n", dir)
	}()

	ui.Muted.Printf("Setting up %s in %s...\n", server.name, dir)
	if err := d.copyInstall(); err != nil {
		return err
	}
	if err := d.resync(); err != nil {
		return err
	}
	if err := d.agreeToEULA(acceptEULA); err != nil {
		return err
	}

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)

	d.help()
	d.run(readLines(), interrupts)
	return nil
}

// readLines reads what is typed, a line at a time, until there is no more
func readLines() <-chan string {
	lines := make(chan string)
	go func() {
		defer close(lines)
		for {
			line, err := cmdshared.ReadLine()
			if line != "" || err == nil {
				lines <- line
			}
			if err != nil {
				return
			}
		}
	}()
	return lines
}

// writeDevServerPack writes the server pack into an archive, as "packwiz modrinth export --server" does, after refreshing
// the index as it does. A file that has to be downloaded by hand is an error, as asking for it would end the session.
func writeDevServerPack(archive cmdshared.Archive) error {
	pack, err := core.LoadPack()
	if err != nil {
		return err
	}
	index, err := pack.LoadIndex()
	if err != nil {
		return err
	}
	if err := index.Refresh(); err != nil {
		return err
	}
	if err := pack.SaveIndex(index); err != nil {
		return err
	}
	mods, err := index.LoadAllMods()
	if err != nil {
		return fmt.Errorf("Error reading file: %v", err)
	}
	_, err = writeServerPack(pack, &index, mods, archive, exportHooks{})
	return err
}

// devServer is a server run in a folder of its own, from a console.
type devServer struct {
	// dir is the server's folder, and install the folder of the installed server it was copied from
	dir, install string
	// write puts the server pack in an archive
	write func(cmdshared.Archive) error
	// command is what starts the server
	command func() (*exec.Cmd, error)
	// stopTimeout is how long the server is given to stop before it is killed
	stopTimeout time.Duration

	// placed are the files that the last resync put in the folder, by their paths with forward slashes
	placed map[string]bool
	// server is the server while it runs
	server *serverProcess
}

// copyInstall copies the installed server into the folder
func (d *devServer) copyInstall() error {
	files, err := installedFiles(d.install)
	if err != nil {
		return err
	}
	for f := range files {
		if err := copyFile(filepath.Join(d.install, filepath.FromSlash(f)), filepath.Join(d.dir, filepath.FromSlash(f))); err != nil {
			return fmt.Errorf("failed to copy the installed server: %w", err)
		}
	}
	return nil
}

// resync puts the server pack in the folder, and takes out what the last resync put there that the pack no longer has,
// putting back the installed server's own file where it has one. It says what it did.
func (d *devServer) resync() error {
	archive := cmdshared.NewDirArchive(d.dir)
	if err := d.write(archive); err != nil {
		return err
	}
	removed := 0
	for _, p := range slices.Sorted(maps.Keys(d.placed)) {
		if archive.Added[p] {
			continue
		}
		dest := filepath.Join(d.dir, filepath.FromSlash(p))
		installed := filepath.Join(d.install, filepath.FromSlash(p))
		if _, err := os.Stat(installed); err == nil && !notInstalled[p] {
			if err := copyFile(installed, dest); err != nil {
				return err
			}
		} else if err := os.Remove(dest); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		} else {
			removeEmptyFolders(d.dir, filepath.Dir(dest))
		}
		removed++
	}
	first := d.placed == nil
	d.placed = archive.Added
	switch {
	case first:
		ui.Success.Printf("Put %d files of the server pack in the folder\n", len(archive.Added))
	case removed > 0:
		ui.Success.Printf("Resynced %d files, and took out %d the pack no longer has\n", len(archive.Added), removed)
	default:
		ui.Success.Printf("Resynced %d files\n", len(archive.Added))
	}
	return nil
}

// removeEmptyFolders removes dir if it is empty, and each folder above it that that leaves empty, up to root
func removeEmptyFolders(root, dir string) {
	for dir != root && strings.HasPrefix(dir, root) {
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// agreeToEULA has the server told that the Minecraft EULA has been agreed to, asking first unless accept is set or the
// server pack's eula.txt says so
func (d *devServer) agreeToEULA(accept bool) error {
	path := filepath.Join(d.dir, "eula.txt")
	if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), "eula=true") {
		return nil
	}
	if !accept && !cmdshared.PromptYesNoDefault("Do you agree to the Minecraft EULA ("+eulaURL+")? [y/N]", false) {
		return errors.New("the server can't be run without agreeing to the Minecraft EULA")
	}
	return os.WriteFile(path, []byte("# Agreed to through packwiz dev server\neula=true\n"), 0o644)
}

// running is whether the server is running
func (d *devServer) running() bool {
	return d.server != nil && !d.server.exited()
}

// run is the console: it does what each line says until one ends the session, there are no more, or there is an
// interrupt
func (d *devServer) run(lines <-chan string, interrupts <-chan os.Signal) {
	d.prompt()
	for {
		var exited <-chan struct{}
		if d.server != nil {
			exited = d.server.done
		}
		select {
		case line, ok := <-lines:
			if !ok {
				fmt.Println()
				return
			}
			if !d.handle(line) {
				return
			}
			d.prompt()
		case <-exited:
			d.reportExit()
			d.prompt()
		case <-interrupts:
			fmt.Println()
			return
		}
	}
}

// prompt asks for a command, when the server isn't running: while it is, what it prints would run into the prompt
func (d *devServer) prompt() {
	if !d.running() {
		fmt.Print(ui.Prompt("dev> "))
	}
}

// reportExit says that the server stopped, if it stopped of its own accord, and forgets it
func (d *devServer) reportExit() {
	if d.server == nil {
		return
	}
	if err := d.server.err; err != nil {
		ui.Warning.Printf("The server stopped: %v\n", err)
	} else {
		ui.Info.Println("The server stopped")
	}
	d.server = nil
}

// handle does what a line of the console says, and reports whether the session goes on
func (d *devServer) handle(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	if shell, ok := strings.CutPrefix(line, "!"); ok {
		d.shell(shell)
		return true
	}
	command, prefixed := strings.CutPrefix(line, ":")
	if d.running() && !prefixed {
		if err := d.server.send(line); err != nil {
			ui.Error.Printf("Failed to send that to the server: %v\n", err)
		}
		return true
	}

	switch strings.ToLower(strings.TrimSpace(command)) {
	case "start":
		d.start()
	case "stop":
		if !d.running() {
			ui.Info.Println("The server isn't running")
			return true
		}
		if err := d.stopServer(); err != nil {
			ui.Error.Println(err)
		}
	case "restart":
		if err := d.stopServer(); err != nil {
			ui.Error.Println(err)
			return true
		}
		d.start()
	case "resync":
		if err := d.resync(); err != nil {
			ui.Error.Println(err)
		} else if d.running() {
			ui.Info.Println("The server is still running what it had: restart it to use what was resynced (:restart)")
		}
	case "status":
		if d.running() {
			ui.Info.Printf("The server is running, in %s\n", d.dir)
		} else {
			ui.Info.Printf("The server isn't running; its folder is %s\n", d.dir)
		}
	case "help", "?":
		d.help()
	case "exit", "quit":
		return false
	default:
		ui.Error.Printf("Unknown command %q: type help for the ones there are\n", command)
	}
	return true
}

// help lists the commands of the console
func (d *devServer) help() {
	fmt.Println(ui.Bold.Sprint("The server's folder is ") + d.dir)
	for _, c := range [][2]string{
		{"start", "start the server"},
		{"stop", "stop it"},
		{"restart", "stop it if it is running, and start it"},
		{"resync", "put the pack's server files in the folder again, as they are now"},
		{"status", "say whether the server is running"},
		{"!<command>", "run a shell command in the folder"},
		{"exit", "stop the server, delete the folder, and end the session"},
	} {
		fmt.Printf("  %s %s\n", ui.Bold.Sprint(padRight(c[0], 11)), ui.Muted.Sprint(c[1]))
	}
	ui.Muted.Println("While the server runs, what you type goes to its console, and these are written :restart, :resync and so on.")
}

// start starts the server, unless it is running
func (d *devServer) start() {
	if d.running() {
		ui.Info.Println("The server is already running")
		return
	}
	command, err := d.command()
	if err != nil {
		ui.Error.Printf("Failed to start the server: %v\n", err)
		return
	}
	server, err := startServerProcess(command, os.Stdout)
	if err != nil {
		ui.Error.Printf("Failed to start the server: %v\n", err)
		return
	}
	d.server = server
	ui.Info.Println("Starting the server; type :stop to stop it")
}

// stopServer stops the server if it is running, killing it if it doesn't stop in time, and waits for it
func (d *devServer) stopServer() error {
	if d.server == nil {
		return nil
	}
	server := d.server
	d.server = nil
	if server.exited() {
		return nil
	}
	ui.Info.Println("Stopping the server...")
	if server.stop(d.stopTimeout) {
		ui.Warning.Printf("The server didn't stop within %s, so it was killed\n", d.stopTimeout)
		return nil
	}
	ui.Info.Println("The server stopped")
	return nil
}

// shell runs a shell command in the server's folder
func (d *devServer) shell(command string) {
	if strings.TrimSpace(command) == "" {
		ui.Error.Println("Give a command to run after the !, such as !ls")
		return
	}
	c := shellCommand(command)
	c.Dir = d.dir
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		ui.Error.Println(err)
	}
}

// serverProcess is a running server, whose console is its stdin.
type serverProcess struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	// done is closed when it has exited, and err is then why, if it failed
	done chan struct{}
	err  error
}

// startServerProcess starts the server, with what it prints going to out. It is in a process group of its own, so that
// an interrupt for the console isn't one for the server, which is stopped as it should be.
func startServerProcess(command *exec.Cmd, out io.Writer) (*serverProcess, error) {
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	command.Stdout, command.Stderr = out, out
	ownProcessGroup(command)
	if err := command.Start(); err != nil {
		return nil, err
	}
	p := &serverProcess{cmd: command, stdin: stdin, done: make(chan struct{})}
	go func() {
		p.err = command.Wait()
		close(p.done)
	}()
	return p, nil
}

func (p *serverProcess) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// send types a line into the server's console
func (p *serverProcess) send(line string) error {
	_, err := io.WriteString(p.stdin, line+"\n")
	return err
}

// stop tells the server to stop, and waits for it to, killing it if it hasn't within timeout, which it reports
func (p *serverProcess) stop(timeout time.Duration) (killed bool) {
	_ = p.send("stop")
	select {
	case <-p.done:
		return false
	case <-time.After(timeout):
		_ = p.cmd.Process.Kill()
		<-p.done
		return true
	}
}
