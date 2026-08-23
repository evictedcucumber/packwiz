package core

import "testing"

func TestReencodeURLNormal(t *testing.T) {
	u := "https://example.com/path/to/file.jar?query=1&other=2"
	got, err := ReencodeURL(u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != u {
		t.Errorf("ReencodeURL(%q) = %q, want unchanged", u, got)
	}
}

func TestReencodeURLBrackets(t *testing.T) {
	u := "https://example.com/mods/[1.20.1]-mod.jar"
	got, err := ReencodeURL(u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://example.com/mods/%5B1.20.1%5D-mod.jar"
	if got != want {
		t.Errorf("ReencodeURL(%q) = %q, want %q", u, got, want)
	}
}

func TestReencodeURLParseError(t *testing.T) {
	// A control character makes url.Parse fail
	u := "http://example.com/\x7f"
	_, err := ReencodeURL(u)
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}
