package core

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestGetPackwizLocalStoreXDGDataHome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG_DATA_HOME only takes precedence on linux")
	}
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	got, err := GetPackwizLocalStore()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, "packwiz")
	if got != want {
		t.Errorf("GetPackwizLocalStore() = %q, want %q", got, want)
	}
}

func TestGetPackwizLocalStoreSuffix(t *testing.T) {
	got, err := GetPackwizLocalStore()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, "packwiz") {
		t.Errorf("GetPackwizLocalStore() = %q, want suffix %q", got, "packwiz")
	}
}

func TestGetPackwizLocalCacheSuffix(t *testing.T) {
	got, err := GetPackwizLocalCache()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, "packwiz") {
		t.Errorf("GetPackwizLocalCache() = %q, want suffix %q", got, "packwiz")
	}
}

func TestGetPackwizInstallBinPath(t *testing.T) {
	got, err := GetPackwizInstallBinPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("packwiz", "bin")
	if !strings.HasSuffix(got, want) {
		t.Errorf("GetPackwizInstallBinPath() = %q, want suffix %q", got, want)
	}
}

func TestGetPackwizInstallBinFile(t *testing.T) {
	got, err := GetPackwizInstallBinFile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var exeName string
	if runtime.GOOS == "windows" {
		exeName = "packwiz.exe"
	} else {
		exeName = "packwiz"
	}
	want := filepath.Join("packwiz", "bin", exeName)
	if !strings.HasSuffix(got, want) {
		t.Errorf("GetPackwizInstallBinFile() = %q, want suffix %q", got, want)
	}
}

func TestGetPackwizCacheDefault(t *testing.T) {
	old := viper.GetString("cache.directory")
	viper.Set("cache.directory", "")
	t.Cleanup(func() { viper.Set("cache.directory", old) })

	got, err := GetPackwizCache()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("packwiz", "cache")
	if !strings.HasSuffix(got, want) {
		t.Errorf("GetPackwizCache() = %q, want suffix %q", got, want)
	}
}

func TestGetPackwizCacheOverride(t *testing.T) {
	old := viper.GetString("cache.directory")
	customDir := filepath.Join(t.TempDir(), "my-cache")
	viper.Set("cache.directory", customDir)
	t.Cleanup(func() { viper.Set("cache.directory", old) })

	got, err := GetPackwizCache()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != customDir {
		t.Errorf("GetPackwizCache() = %q, want %q (viper override should take precedence)", got, customDir)
	}
}
