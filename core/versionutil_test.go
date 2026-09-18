package core

import (
	"embed"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jarcoal/httpmock"
)

// For reproducability, we store a list of sample xml files for various endpoints
// these have been edited slightly to cut down on the number of entries, but are
// otherwise taken from the endpoints themselves

//go:embed version_test_files/*
var versionTestFiles embed.FS

func registerMock(url string, filename string) {
	bytes, err := versionTestFiles.ReadFile("version_test_files/" + filename)
	if err != nil {
		println("Error " + filename + " not in version_test_files/")
		os.Exit(1)
	}
	httpmock.RegisterResponder("GET", url, httpmock.NewBytesResponder(200, bytes))
}

func queryWithMock(t *testing.T, q VersionListQuery) *ModLoaderVersions {
	httpmock.Activate(t)

	registerMock("https://maven.neoforged.net/releases/net/neoforged/forge/maven-metadata.xml", "neoforge_old.xml")
	registerMock("https://maven.neoforged.net/releases/net/neoforged/neoforge/maven-metadata.xml", "neoforge.xml")

	versionData, err := DoQuery(q)

	if err != nil {
		t.Logf("Error fetching versions for %s: %s", q.Loader.FriendlyName, err)
		if strings.Contains(err.Error(), "no responder found") {
			t.Log("You likely need to register a mock for this url")
		}
		t.FailNow()
	}

	return versionData
}

func expectLatest(t *testing.T, loader string, version string, expectedLatest string) string {
	loaderData, ok := ModLoaders[loader]
	if !ok {
		t.Fatal("Could not find loader")
	}
	versionData := queryWithMock(t, MakeQuery(loaderData, version))

	if len(versionData.Versions) == 0 {
		t.Error("There should be at least one version")
	}
	if versionData.Latest != expectedLatest {
		t.Errorf("Expected latest version to be %s, found %s", expectedLatest, versionData.Latest)
	}

	return versionData.Latest
}

func expectValid(t *testing.T, loader string, version string, expectedValid string) {
	loaderData, ok := ModLoaders[loader]
	if !ok {
		t.Fatal("Could not find loader")
	}
	versionData := queryWithMock(t, MakeQuery(loaderData, version))

	if !slices.Contains(versionData.Versions, expectedValid) {
		t.Errorf("Expected %s to be a valid version for %s. Valid versions:\n%s", expectedValid, loaderData.FriendlyName, versionData.Versions)
	}
}

func expectInvalid(t *testing.T, loader string, version string, expectedValid string) {
	loaderData, ok := ModLoaders[loader]
	if !ok {
		t.Fatal("Could not find loader")
	}
	versionData := queryWithMock(t, MakeQuery(loaderData, version))

	if slices.Contains(versionData.Versions, expectedValid) {
		t.Errorf("Expected %s not to be a valid version for %s. Valid versions:\n%s", expectedValid, loaderData.FriendlyName, versionData.Versions)
	}
}

func TestNeoForge1201(t *testing.T) {
	expectLatest(t, "neoforge", "1.20.1", "47.1.106")
}

func TestNeoForge121(t *testing.T) {
	expectLatest(t, "neoforge", "1.21", "21.0.167")
}

func TestNeoForge1211(t *testing.T) {
	expectLatest(t, "neoforge", "1.21.1", "21.1.213")
	expectValid(t, "neoforge", "1.21.1", "21.1.201")
	expectInvalid(t, "neoforge", "1.21.1", "21.10.43-beta")
}

func TestNeoForge1210(t *testing.T) {
	expectLatest(t, "neoforge", "1.21.10", "21.10.43-beta")
}

func TestNeoForge261snapshot6(t *testing.T) {
	expectLatest(t, "neoforge", "26.1-snapshot-6", "26.1.0.0-alpha.10+snapshot-6")
	expectValid(t, "neoforge", "26.1-snapshot-6", "26.1.0.0-alpha.9+snapshot-6")
	expectInvalid(t, "neoforge", "26.1-snapshot-6", "26.1.0.0-alpha.11+snapshot-7")
}

func TestWithQueryType(t *testing.T) {
	base := MakeQuery(ModLoaders["neoforge"], "1.20.1")
	if base.QueryType != Latest {
		t.Fatalf("MakeQuery() QueryType = %v, want Latest", base.QueryType)
	}

	updated := base.WithQueryType(Recommended)
	if updated.QueryType != Recommended {
		t.Errorf("WithQueryType(Recommended) QueryType = %v, want Recommended", updated.QueryType)
	}
	if updated.Loader.Name != base.Loader.Name {
		t.Errorf("WithQueryType() Loader = %v, want %v", updated.Loader, base.Loader)
	}
	if updated.McVersion != base.McVersion {
		t.Errorf("WithQueryType() McVersion = %q, want %q", updated.McVersion, base.McVersion)
	}
	// The original query must be untouched.
	if base.QueryType != Latest {
		t.Errorf("original query QueryType = %v, want Latest (WithQueryType must not mutate the receiver)", base.QueryType)
	}
}

func TestComponentToFriendlyName(t *testing.T) {
	cases := []struct {
		component string
		want      string
	}{
		{"minecraft", "Minecraft"},
		{"neoforge", "NeoForge"},
		{"unknown-loader", "unknown-loader"},
	}
	for _, c := range cases {
		t.Run(c.component, func(t *testing.T) {
			if got := ComponentToFriendlyName(c.component); got != c.want {
				t.Errorf("ComponentToFriendlyName(%q) = %q, want %q", c.component, got, c.want)
			}
		})
	}
}

func TestHighestSliceIndex(t *testing.T) {
	cases := []struct {
		name   string
		slice  []string
		values []string
		want   int
	}{
		{"no match", []string{"a", "b", "c"}, []string{"z"}, -1},
		{"single match", []string{"a", "b", "c"}, []string{"b"}, 1},
		{"picks highest of several matches", []string{"a", "b", "c"}, []string{"a", "c"}, 2},
		{"empty values", []string{"a", "b", "c"}, nil, -1},
		{"empty slice", nil, []string{"a"}, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HighestSliceIndex(c.slice, c.values); got != c.want {
				t.Errorf("HighestSliceIndex(%v, %v) = %d, want %d", c.slice, c.values, got, c.want)
			}
		})
	}
}
