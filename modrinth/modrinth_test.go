package modrinth

import (
	"strings"
	"testing"
	"time"

	modrinthApi "codeberg.org/jmansfield/go-modrinth/modrinth"
	"github.com/evictedcucumber/packwiz/core"
	"github.com/spf13/viper"
)

func strPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}

// --- parseUrl ---

func TestParseUrlModPage(t *testing.T) {
	var slug, version, versionID, filename string
	err := parseUrl("https://modrinth.com/mod/example-mod", &slug, &version, &versionID, &filename)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slug != "example-mod" {
		t.Errorf("expected slug %q, got %q", "example-mod", slug)
	}
	if version != "" {
		t.Errorf("expected empty version, got %q", version)
	}
}

func TestParseUrlModPageWithWWW(t *testing.T) {
	var slug, version, versionID, filename string
	err := parseUrl("https://www.modrinth.com/mod/example-mod", &slug, &version, &versionID, &filename)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slug != "example-mod" {
		t.Errorf("expected slug %q, got %q", "example-mod", slug)
	}
}

func TestParseUrlModPageWithVersion(t *testing.T) {
	var slug, version, versionID, filename string
	err := parseUrl("https://modrinth.com/mod/example-mod/version/1.2.3", &slug, &version, &versionID, &filename)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slug != "example-mod" {
		t.Errorf("expected slug %q, got %q", "example-mod", slug)
	}
	if version != "1.2.3" {
		t.Errorf("expected version %q, got %q", "1.2.3", version)
	}
}

func TestParseUrlOtherCategories(t *testing.T) {
	for _, category := range []string{"plugin", "resourcepack", "shader", "datapack"} {
		var slug, version, versionID, filename string
		url := "https://modrinth.com/" + category + "/example-project"
		err := parseUrl(url, &slug, &version, &versionID, &filename)
		if err != nil {
			t.Errorf("category %s: unexpected error: %v", category, err)
			continue
		}
		if slug != "example-project" {
			t.Errorf("category %s: expected slug %q, got %q", category, "example-project", slug)
		}
	}
}

func TestParseUrlUnknownCategory(t *testing.T) {
	var slug, version, versionID, filename string
	err := parseUrl("https://modrinth.com/foobar/example-project", &slug, &version, &versionID, &filename)
	if err == nil {
		t.Fatal("expected an error for unknown project type, got nil")
	}
	if !strings.Contains(err.Error(), "unknown project type") {
		t.Errorf("expected error to mention 'unknown project type', got: %v", err)
	}
}

func TestParseUrlCdnFile(t *testing.T) {
	var slug, version, versionID, filename string
	err := parseUrl("https://cdn.modrinth.com/data/AANobbMI/versions/abcXYZ123/My%20Mod+File.jar", &slug, &version, &versionID, &filename)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slug != "AANobbMI" {
		t.Errorf("expected slug %q, got %q", "AANobbMI", slug)
	}
	if versionID != "abcXYZ123" {
		t.Errorf("expected versionID %q, got %q", "abcXYZ123", versionID)
	}
	// PathUnescape decodes %20 to a space, but does NOT treat '+' as a space
	// (unlike QueryUnescape) - verify that distinction holds.
	if filename != "My Mod+File.jar" {
		t.Errorf("expected filename %q, got %q", "My Mod+File.jar", filename)
	}
}

func TestParseUrlInvalid(t *testing.T) {
	var slug, version, versionID, filename string
	err := parseUrl("https://example.com/not-a-modrinth-url", &slug, &version, &versionID, &filename)
	if err == nil {
		t.Fatal("expected an error for invalid URL, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid Modrinth URL") {
		t.Errorf("expected error to mention 'not a valid Modrinth URL', got: %v", err)
	}
}

// --- getProjectTypeFolder ---

func TestGetProjectTypeFolderModpack(t *testing.T) {
	_, err := getProjectTypeFolder("modpack", nil, nil)
	if err == nil {
		t.Fatal("expected an error for modpack project type, got nil")
	}
}

func TestGetProjectTypeFolderResourcepack(t *testing.T) {
	folder, err := getProjectTypeFolder("resourcepack", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder != "resourcepacks" {
		t.Errorf("expected %q, got %q", "resourcepacks", folder)
	}
}

func TestGetProjectTypeFolderShaderPrefersEarlierPreference(t *testing.T) {
	// iris comes before optifine in loaderPreferenceList, so iris' folder should be used
	folder, err := getProjectTypeFolder("shader", []string{"iris", "optifine"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder != loaderFolders["iris"] {
		t.Errorf("expected %q (iris' folder), got %q", loaderFolders["iris"], folder)
	}
}

func TestGetProjectTypeFolderShaderPicksCanvasOverIris(t *testing.T) {
	// canvas maps to a different folder than iris/optifine, so this differentiates the choice
	folder, err := getProjectTypeFolder("shader", []string{"iris", "canvas"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder != "resourcepacks" {
		t.Errorf("expected %q (canvas' folder), got %q", "resourcepacks", folder)
	}
}

func TestGetProjectTypeFolderShaderFallback(t *testing.T) {
	folder, err := getProjectTypeFolder("shader", []string{"nonexistent-loader"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder != "shaderpacks" {
		t.Errorf("expected fallback %q, got %q", "shaderpacks", folder)
	}
}

func TestGetProjectTypeFolderModPicksMostPreferred(t *testing.T) {
	// Only loaders present in both fileLoaders and packLoaders are considered;
	// paper is preferred over bukkit in loaderPreferenceList
	folder, err := getProjectTypeFolder("mod", []string{"bukkit", "paper"}, []string{"bukkit", "paper"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder != "plugins" {
		t.Errorf("expected %q, got %q", "plugins", folder)
	}
}

func TestGetProjectTypeFolderModIgnoresLoadersNotInPack(t *testing.T) {
	// fileLoaders has quilt, but packLoaders (installed loaders) is just fabric,
	// so quilt should be ignored and fabric should be used
	folder, err := getProjectTypeFolder("mod", []string{"quilt", "fabric"}, []string{"fabric"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder != "mods" {
		t.Errorf("expected %q, got %q", "mods", folder)
	}
}

func TestGetProjectTypeFolderModDatapackWithFolderSet(t *testing.T) {
	old := viper.GetString("datapack-folder")
	viper.Set("datapack-folder", "datapacks")
	t.Cleanup(func() { viper.Set("datapack-folder", old) })

	folder, err := getProjectTypeFolder("mod", []string{"datapack"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder != "datapacks" {
		t.Errorf("expected %q, got %q", "datapacks", folder)
	}
}

func TestGetProjectTypeFolderModDatapackWithoutFolderSet(t *testing.T) {
	old := viper.GetString("datapack-folder")
	viper.Set("datapack-folder", "")
	t.Cleanup(func() { viper.Set("datapack-folder", old) })

	_, err := getProjectTypeFolder("mod", []string{"datapack"}, nil)
	if err == nil {
		t.Fatal("expected an error when datapack-folder is not set, got nil")
	}
}

func TestGetProjectTypeFolderModDefault(t *testing.T) {
	folder, err := getProjectTypeFolder("mod", []string{"nonexistent-loader"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder != "mods" {
		t.Errorf("expected default %q, got %q", "mods", folder)
	}
}

func TestGetProjectTypeFolderUnknown(t *testing.T) {
	_, err := getProjectTypeFolder("something-else", nil, nil)
	if err == nil {
		t.Fatal("expected an error for unknown project type, got nil")
	}
}

// --- compareLoaderLists ---

func TestCompareLoaderListsBasicPreference(t *testing.T) {
	// quilt is preferred over fabric
	result := compareLoaderLists([]string{"fabric"}, []string{"quilt"})
	if result <= 0 {
		t.Errorf("expected a positive result (b/quilt more preferred), got %d", result)
	}
}

func TestCompareLoaderListsCompatGroupDiscountsWhenBothSharePrerequisite(t *testing.T) {
	// Both lists contain "fabric" (the prerequisite for the quilt compat group), so the
	// extra "quilt" entry in a should be discounted, making the lists equally preferred.
	result := compareLoaderLists([]string{"fabric", "quilt"}, []string{"fabric"})
	if result != 0 {
		t.Errorf("expected 0 (quilt discounted since both share fabric), got %d", result)
	}
}

func TestCompareLoaderListsCompatGroupNotDiscountedWithoutSharedPrerequisite(t *testing.T) {
	// b does not contain "fabric", so the compat group is not triggered and quilt in a
	// should count fully, making a more preferred.
	result := compareLoaderLists([]string{"fabric", "quilt"}, []string{"neoforge"})
	if result >= 0 {
		t.Errorf("expected a negative result (a/quilt more preferred), got %d", result)
	}
}

// --- findLatestVersion ---

func TestFindLatestVersionFlexVerPicksHigherVersion(t *testing.T) {
	date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	v1 := &modrinthApi.Version{
		VersionNumber: strPtr("1.0.0"),
		GameVersions:  []string{"1.20"},
		Loaders:       []string{"fabric"},
		DatePublished: timePtr(date),
	}
	v2 := &modrinthApi.Version{
		VersionNumber: strPtr("2.0.0"),
		GameVersions:  []string{"1.20"},
		Loaders:       []string{"fabric"},
		DatePublished: timePtr(date),
	}

	result := findLatestVersion([]*modrinthApi.Version{v1, v2}, []string{"1.20"}, true)
	if result != v2 {
		t.Errorf("expected v2 (higher FlexVer version) to be picked")
	}
}

func TestFindLatestVersionByDateWhenNotUsingFlexVer(t *testing.T) {
	earlier := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	v1 := &modrinthApi.Version{
		VersionNumber: strPtr("1.0.0"),
		GameVersions:  []string{},
		Loaders:       []string{},
		DatePublished: timePtr(earlier),
	}
	v2 := &modrinthApi.Version{
		VersionNumber: strPtr("1.0.0"),
		GameVersions:  []string{},
		Loaders:       []string{},
		DatePublished: timePtr(later),
	}

	result := findLatestVersion([]*modrinthApi.Version{v1, v2}, []string{}, false)
	if result != v2 {
		t.Errorf("expected v2 (later DatePublished) to be picked")
	}
}

func TestFindLatestVersionRespectsGameVersionPreference(t *testing.T) {
	date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// gameVersions is ordered with the most preferred (main) version specified last
	gameVersions := []string{"1.19", "1.20"}
	v1 := &modrinthApi.Version{
		VersionNumber: strPtr("1.0.0"),
		GameVersions:  []string{"1.19"},
		Loaders:       []string{"fabric"},
		DatePublished: timePtr(date),
	}
	v2 := &modrinthApi.Version{
		VersionNumber: strPtr("1.0.0"),
		GameVersions:  []string{"1.20"},
		Loaders:       []string{"fabric"},
		DatePublished: timePtr(date),
	}

	result := findLatestVersion([]*modrinthApi.Version{v1, v2}, gameVersions, true)
	if result != v2 {
		t.Errorf("expected v2 (more preferred/later game version) to be picked")
	}
}

func TestFindLatestVersionUsesLoaderListAsTiebreaker(t *testing.T) {
	date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	v1 := &modrinthApi.Version{
		VersionNumber: strPtr("1.0.0"),
		GameVersions:  []string{},
		Loaders:       []string{"fabric"},
		DatePublished: timePtr(date),
	}
	v2 := &modrinthApi.Version{
		VersionNumber: strPtr("1.0.0"),
		GameVersions:  []string{},
		Loaders:       []string{"quilt"},
		DatePublished: timePtr(date),
	}

	result := findLatestVersion([]*modrinthApi.Version{v1, v2}, []string{}, true)
	if result != v2 {
		t.Errorf("expected v2 (quilt preferred over fabric) to be picked")
	}
}

// --- getSide ---

func TestGetSideUniversal(t *testing.T) {
	project := &modrinthApi.Project{ServerSide: strPtr("required"), ClientSide: strPtr("required")}
	if got := getSide(project); got != core.UniversalSide {
		t.Errorf("expected %q, got %q", core.UniversalSide, got)
	}
}

func TestGetSideServerOnly(t *testing.T) {
	project := &modrinthApi.Project{ServerSide: strPtr("required"), ClientSide: strPtr("unsupported")}
	if got := getSide(project); got != core.ServerSide {
		t.Errorf("expected %q, got %q", core.ServerSide, got)
	}
}

func TestGetSideClientOnly(t *testing.T) {
	project := &modrinthApi.Project{ServerSide: strPtr("unsupported"), ClientSide: strPtr("optional")}
	if got := getSide(project); got != core.ClientSide {
		t.Errorf("expected %q, got %q", core.ClientSide, got)
	}
}

func TestGetSideNeither(t *testing.T) {
	project := &modrinthApi.Project{ServerSide: nil, ClientSide: nil}
	if got := getSide(project); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestGetSideUnsupportedBoth(t *testing.T) {
	project := &modrinthApi.Project{ServerSide: strPtr("unsupported"), ClientSide: strPtr("unsupported")}
	if got := getSide(project); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// --- shouldDownloadOnSide ---

func TestShouldDownloadOnSide(t *testing.T) {
	cases := map[string]bool{
		"required":    true,
		"optional":    true,
		"unsupported": false,
		"":            false,
	}
	for side, expected := range cases {
		if got := shouldDownloadOnSide(side); got != expected {
			t.Errorf("side %q: expected %v, got %v", side, expected, got)
		}
	}
}

// --- getBestHash ---

func TestGetBestHashPrefersSha512(t *testing.T) {
	f := &modrinthApi.File{Hashes: map[string]string{
		"sha512":  "v512",
		"sha256":  "v256",
		"sha1":    "v1",
		"murmur2": "vm2",
	}}
	algo, val := getBestHash(f)
	if algo != "sha512" || val != "v512" {
		t.Errorf("expected sha512/v512, got %s/%s", algo, val)
	}
}

func TestGetBestHashPrefersSha256(t *testing.T) {
	f := &modrinthApi.File{Hashes: map[string]string{
		"sha256":  "v256",
		"sha1":    "v1",
		"murmur2": "vm2",
	}}
	algo, val := getBestHash(f)
	if algo != "sha256" || val != "v256" {
		t.Errorf("expected sha256/v256, got %s/%s", algo, val)
	}
}

func TestGetBestHashPrefersSha1(t *testing.T) {
	f := &modrinthApi.File{Hashes: map[string]string{
		"sha1":    "v1",
		"murmur2": "vm2",
	}}
	algo, val := getBestHash(f)
	if algo != "sha1" || val != "v1" {
		t.Errorf("expected sha1/v1, got %s/%s", algo, val)
	}
}

func TestGetBestHashPrefersMurmur2(t *testing.T) {
	f := &modrinthApi.File{Hashes: map[string]string{
		"murmur2": "vm2",
	}}
	algo, val := getBestHash(f)
	if algo != "murmur2" || val != "vm2" {
		t.Errorf("expected murmur2/vm2, got %s/%s", algo, val)
	}
}

func TestGetBestHashFallsBackToAnyRemaining(t *testing.T) {
	f := &modrinthApi.File{Hashes: map[string]string{
		"weird-hash": "weird-val",
	}}
	algo, val := getBestHash(f)
	if algo != "weird-hash" || val != "weird-val" {
		t.Errorf("expected weird-hash/weird-val, got %s/%s", algo, val)
	}
}

func TestGetBestHashEmpty(t *testing.T) {
	f := &modrinthApi.File{Hashes: map[string]string{}}
	algo, val := getBestHash(f)
	if algo != "" || val != "" {
		t.Errorf("expected empty strings, got %s/%s", algo, val)
	}
}

// --- mapDepOverride ---

func TestMapDepOverrideFabricApiById(t *testing.T) {
	result := mapDepOverride("P7dR8mSH", true, "1.20.0")
	if result != "qvIfYCYJ" {
		t.Errorf("expected %q, got %q", "qvIfYCYJ", result)
	}
}

func TestMapDepOverrideFabricApiBySlug(t *testing.T) {
	result := mapDepOverride("fabric-api", true, "1.20.0")
	if result != "qvIfYCYJ" {
		t.Errorf("expected %q, got %q", "qvIfYCYJ", result)
	}
}

func TestMapDepOverrideFlkInRange(t *testing.T) {
	result := mapDepOverride("Ha28R6CL", true, "1.19.2")
	if result != "lwVhp9o5" {
		t.Errorf("expected %q, got %q", "lwVhp9o5", result)
	}
}

func TestMapDepOverrideFlkOutOfRange(t *testing.T) {
	result := mapDepOverride("Ha28R6CL", true, "1.19.0")
	if result != "Ha28R6CL" {
		t.Errorf("expected unchanged %q, got %q", "Ha28R6CL", result)
	}
}

func TestMapDepOverrideFlkAtUpperBound(t *testing.T) {
	result := mapDepOverride("Ha28R6CL", true, "2.0.0")
	if result != "Ha28R6CL" {
		t.Errorf("expected unchanged %q, got %q", "Ha28R6CL", result)
	}
}

func TestMapDepOverrideNonQuilt(t *testing.T) {
	result := mapDepOverride("P7dR8mSH", false, "1.20.0")
	if result != "P7dR8mSH" {
		t.Errorf("expected unchanged %q, got %q", "P7dR8mSH", result)
	}
}

func TestMapDepOverrideUnrelatedDepID(t *testing.T) {
	result := mapDepOverride("some-other-mod", true, "1.20.0")
	if result != "some-other-mod" {
		t.Errorf("expected unchanged %q, got %q", "some-other-mod", result)
	}
}
