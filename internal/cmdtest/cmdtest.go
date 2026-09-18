// Package cmdtest provides small helpers shared by the CLI command packages'
// tests (cmd, settings, utils, migrate). Those commands hold their logic
// directly in cobra Run closures that print via fmt.Println and exit via
// os.Exit(1) on error, so tests invoke Run() directly in an isolated temp
// directory and capture real stdout, rather than driving cobra's Execute().
package cmdtest

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/viper"
)

// Chdir creates a fresh temp directory and changes the test's working
// directory into it, restoring the original working directory on cleanup.
// (go.mod targets go1.23, which predates testing.T.Chdir, so this is done
// manually.)
func Chdir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir into %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("failed to restore working directory: %v", err)
		}
	})
	return dir
}

// CaptureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it. Output is drained concurrently so fn can't
// deadlock by writing more than the pipe's buffer size.
func CaptureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	outC := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = old
	return <-outC
}

// WritePackFile writes pack.toml (in the current directory - use Chdir
// first) and points viper's "pack-file" at it for the duration of the test.
//
// Note: core.LoadPack merges the pack's [options] table into viper's global
// config via viper.MergeConfigMap, which has no way to be cleanly undone
// afterward (an override masking it back to "" would out-rank, and thus
// permanently block, any later legitimate merge of that same key for the
// rest of the process). Tests that round-trip a value through pack.toml's
// [options] table (e.g. "settings release-type") should call viper.Reset()
// themselves before writing their fixture - safe there since they always
// re-set "pack-file" via this function immediately afterward, but not done
// unconditionally here since it would also discard the viper.BindPFlag
// bindings other commands (e.g. "list") rely on to read their own flags.
func WritePackFile(t *testing.T, pack core.Pack) {
	t.Helper()
	old := viper.GetString("pack-file")
	viper.Set("pack-file", "pack.toml")
	t.Cleanup(func() { viper.Set("pack-file", old) })

	if err := pack.Write(); err != nil {
		t.Fatalf("failed to write pack.toml fixture: %v", err)
	}
}

// SetViper sets a viper key for the duration of the test, restoring its
// previous string value on cleanup. Use SetViperBool for boolean flags.
func SetViper(t *testing.T, key string, value string) {
	t.Helper()
	old := viper.GetString(key)
	viper.Set(key, value)
	t.Cleanup(func() { viper.Set(key, old) })
}

// SetViperBool sets a boolean viper key for the duration of the test,
// restoring its previous value on cleanup.
func SetViperBool(t *testing.T, key string, value bool) {
	t.Helper()
	old := viper.GetBool(key)
	viper.Set(key, value)
	t.Cleanup(func() { viper.Set(key, old) })
}
