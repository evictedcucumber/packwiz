package modrinth

import (
	"testing"

	"github.com/evictedcucumber/packwiz/core"
)

func TestCanBeIncludedDirectlyURLModeNoRestrict(t *testing.T) {
	mod := &core.Mod{Download: core.ModDownload{Mode: core.ModeURL, URL: "https://example.com/foo.jar"}}
	if !canBeIncludedDirectly(mod, false) {
		t.Error("expected true when restrictDomains is false")
	}
}

func TestCanBeIncludedDirectlyEmptyModeNoRestrict(t *testing.T) {
	mod := &core.Mod{Download: core.ModDownload{Mode: "", URL: "https://example.com/foo.jar"}}
	if !canBeIncludedDirectly(mod, false) {
		t.Error("expected true when restrictDomains is false and mode is empty")
	}
}

func TestCanBeIncludedDirectlyNonURLMode(t *testing.T) {
	mod := &core.Mod{Download: core.ModDownload{Mode: "metadata:curseforge", URL: "https://cdn.modrinth.com/foo.jar"}}
	if canBeIncludedDirectly(mod, false) {
		t.Error("expected false for a non-URL download mode, regardless of restrictDomains")
	}
	if canBeIncludedDirectly(mod, true) {
		t.Error("expected false for a non-URL download mode, regardless of restrictDomains")
	}
}

func TestCanBeIncludedDirectlyWhitelistedHosts(t *testing.T) {
	whitelisted := []string{
		"https://cdn.modrinth.com/data/foo/versions/bar/baz.jar",
		"https://github.com/foo/bar/releases/download/v1/baz.jar",
		"https://raw.githubusercontent.com/foo/bar/main/baz.jar",
		"https://gitlab.com/foo/bar/-/raw/main/baz.jar",
	}
	for _, u := range whitelisted {
		mod := &core.Mod{Download: core.ModDownload{Mode: core.ModeURL, URL: u}}
		if !canBeIncludedDirectly(mod, true) {
			t.Errorf("expected true for whitelisted host URL %q with restrictDomains=true", u)
		}
	}
}

func TestCanBeIncludedDirectlyNonWhitelistedHostRestricted(t *testing.T) {
	mod := &core.Mod{Download: core.ModDownload{Mode: core.ModeURL, URL: "https://example.com/foo.jar"}}
	if canBeIncludedDirectly(mod, true) {
		t.Error("expected false for a non-whitelisted host with restrictDomains=true")
	}
}

func TestCanBeIncludedDirectlyEmptyModeRestrictedWhitelisted(t *testing.T) {
	mod := &core.Mod{Download: core.ModDownload{Mode: "", URL: "https://github.com/foo/bar"}}
	if !canBeIncludedDirectly(mod, true) {
		t.Error("expected true for empty mode with a whitelisted host and restrictDomains=true")
	}
}
