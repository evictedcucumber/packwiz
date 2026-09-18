package cmd

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"os"
	"testing"

	"github.com/evictedcucumber/packwiz/core"
	"github.com/evictedcucumber/packwiz/internal/cmdtest"
	"github.com/jarcoal/httpmock"
)

func TestRehashCmdMigratesHashFormat(t *testing.T) {
	httpmock.Activate(t)

	cmdtest.Chdir(t)
	cmdtest.WritePackFile(t, core.Pack{
		Name: "Test Pack", PackFormat: core.CurrentPackFormat,
		Versions: map[string]string{"minecraft": "1.20.1"},
	})
	cmdtest.SetViper(t, "cache.directory", t.TempDir())

	content := []byte("mod jar contents")
	sha256Sum := sha256.Sum256(content)
	sha256Hex := hex.EncodeToString(sha256Sum[:])
	sha512Sum := sha512.Sum512(content)
	sha512Hex := hex.EncodeToString(sha512Sum[:])

	httpmock.RegisterResponder("GET", "https://example.com/one.jar", httpmock.NewBytesResponder(200, content))

	if err := os.MkdirAll("mods", 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}
	modContent := `name = "One Mod"
filename = "one.jar"

[download]
url = "https://example.com/one.jar"
hash-format = "sha256"
hash = "` + sha256Hex + `"
`
	if err := os.WriteFile("mods/one.pw.toml", []byte(modContent), 0644); err != nil {
		t.Fatalf("failed to write mod fixture: %v", err)
	}
	if err := os.WriteFile("index.toml", []byte(`hash-format = "sha256"

[[files]]
file = "mods/one.pw.toml"
hash = "irrelevant"
metafile = true
`), 0644); err != nil {
		t.Fatalf("failed to write index.toml fixture: %v", err)
	}

	cmdtest.CaptureStdout(t, func() {
		rehashCmd.Run(rehashCmd, []string{"sha512"})
	})

	mod, err := core.LoadMod("mods/one.pw.toml")
	if err != nil {
		t.Fatalf("LoadMod() returned error: %v", err)
	}
	if mod.Download.HashFormat != "sha512" {
		t.Errorf("Download.HashFormat = %q, want %q", mod.Download.HashFormat, "sha512")
	}
	if mod.Download.Hash != sha512Hex {
		t.Errorf("Download.Hash = %q, want %q", mod.Download.Hash, sha512Hex)
	}

	pack, err := core.LoadPack()
	if err != nil {
		t.Fatalf("LoadPack() returned error: %v", err)
	}
	if pack.Index.Hash == "" {
		t.Error("Index.Hash is empty; expected pack.UpdateIndexHash() to have run")
	}
}
