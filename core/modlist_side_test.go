package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateModListMarkdownFiltersBySide(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create client-only mod
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "client.pw.toml"), []byte("name = \"Client Mod\"\nfilename = \"client.jar\"\nversion = \"1.0\"\npage-url = \"https://example.com/client\"\nside = \"client\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create server-only mod
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "server.pw.toml"), []byte("name = \"Server Mod\"\nfilename = \"server.jar\"\nversion = \"1.0\"\npage-url = \"https://example.com/server\"\nside = \"server\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create both-side mod
	if err := os.WriteFile(filepath.Join(tempDir, "mods", "both.pw.toml"), []byte("name = \"Both Mod\"\nfilename = \"both.jar\"\nversion = \"1.0\"\npage-url = \"https://example.com/both\"\nside = \"both\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := Index{
		packRoot: tempDir,
		Files: IndexFiles{
			"mods/client.pw.toml": &indexFile{File: "mods/client.pw.toml", MetaFile: true},
			"mods/server.pw.toml": &indexFile{File: "mods/server.pw.toml", MetaFile: true},
			"mods/both.pw.toml":   &indexFile{File: "mods/both.pw.toml", MetaFile: true},
		},
	}

	// Client side: should include client and both, but not server
	mdClient, err := idx.GenerateModListMarkdownWithSide(ClientSide)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mdClient, "Client Mod") || !strings.Contains(mdClient, "Both Mod") || strings.Contains(mdClient, "Server Mod") {
		t.Fatalf("client side filtering failed, got:\n%s", mdClient)
	}

	// Server side: should include server and both, but not client
	mdServer, err := idx.GenerateModListMarkdownWithSide(ServerSide)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mdServer, "Server Mod") || !strings.Contains(mdServer, "Both Mod") || strings.Contains(mdServer, "Client Mod") {
		t.Fatalf("server side filtering failed, got:\n%s", mdServer)
	}

	// Both side: should include all three
	mdBoth, err := idx.GenerateModListMarkdownWithSide(UniversalSide)
	if err != nil {
		t.Fatal(err)
	}
	if !(strings.Contains(mdBoth, "Client Mod") && strings.Contains(mdBoth, "Server Mod") && strings.Contains(mdBoth, "Both Mod")) {
		t.Fatalf("both side filtering failed, got:\n%s", mdBoth)
	}
}
