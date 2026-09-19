package modrinth

import (
	"strings"
	"testing"
	"time"

	"github.com/evictedcucumber/packwiz/core"
	modrinthApi "github.com/evictedcucumber/packwiz/modrinth/api"
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
	// fileLoaders has rift, but packLoaders (installed loaders) is just modloader,
	// so rift should be ignored and modloader should be used
	folder, err := getProjectTypeFolder("mod", []string{"rift", "modloader"}, []string{"modloader"})
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
	// neoforge is preferred over modloader
	result := compareLoaderLists([]string{"modloader"}, []string{"neoforge"})
	if result <= 0 {
		t.Errorf("expected a positive result (b/neoforge more preferred), got %d", result)
	}
}

func TestCompareLoaderListsCompatGroupDiscountsWhenBothSharePrerequisite(t *testing.T) {
	// Both lists contain "bukkit" (the prerequisite for the purpur compat group), so the
	// extra "purpur" entry in a should be discounted, making the lists equally preferred.
	result := compareLoaderLists([]string{"purpur", "bukkit"}, []string{"bukkit"})
	if result != 0 {
		t.Errorf("expected 0 (purpur discounted since both share bukkit), got %d", result)
	}
}

func TestCompareLoaderListsCompatGroupNotDiscountedWithoutSharedPrerequisite(t *testing.T) {
	// b does not contain "bukkit", so the compat group is not triggered and purpur in a
	// should count fully, making a more preferred.
	result := compareLoaderLists([]string{"purpur", "bukkit"}, []string{"waterfall"})
	if result >= 0 {
		t.Errorf("expected a negative result (a/purpur more preferred), got %d", result)
	}
}

// --- findLatestVersion ---

func TestFindLatestVersionFlexVerPicksHigherVersion(t *testing.T) {
	date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	v1 := &modrinthApi.Version{
		VersionNumber: strPtr("1.0.0"),
		GameVersions:  []string{"1.20"},
		Loaders:       []string{"neoforge"},
		DatePublished: timePtr(date),
	}
	v2 := &modrinthApi.Version{
		VersionNumber: strPtr("2.0.0"),
		GameVersions:  []string{"1.20"},
		Loaders:       []string{"neoforge"},
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
		Loaders:       []string{"neoforge"},
		DatePublished: timePtr(date),
	}
	v2 := &modrinthApi.Version{
		VersionNumber: strPtr("1.0.0"),
		GameVersions:  []string{"1.20"},
		Loaders:       []string{"neoforge"},
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
		Loaders:       []string{"modloader"},
		DatePublished: timePtr(date),
	}
	v2 := &modrinthApi.Version{
		VersionNumber: strPtr("1.0.0"),
		GameVersions:  []string{},
		Loaders:       []string{"neoforge"},
		DatePublished: timePtr(date),
	}

	result := findLatestVersion([]*modrinthApi.Version{v1, v2}, []string{}, true)
	if result != v2 {
		t.Errorf("expected v2 (neoforge preferred over modloader) to be picked")
	}
}

// --- filterVersionsByReleaseType ---

func TestFilterVersionsByReleaseTypeReleaseExcludesLessStable(t *testing.T) {
	release := &modrinthApi.Version{VersionType: strPtr("release")}
	beta := &modrinthApi.Version{VersionType: strPtr("beta")}
	alpha := &modrinthApi.Version{VersionType: strPtr("alpha")}

	result := filterVersionsByReleaseType([]*modrinthApi.Version{release, beta, alpha}, "release")
	if len(result) != 1 || result[0] != release {
		t.Errorf("expected only the release version, got %v", result)
	}
}

func TestFilterVersionsByReleaseTypeBetaIncludesReleaseAndBeta(t *testing.T) {
	release := &modrinthApi.Version{VersionType: strPtr("release")}
	beta := &modrinthApi.Version{VersionType: strPtr("beta")}
	alpha := &modrinthApi.Version{VersionType: strPtr("alpha")}

	result := filterVersionsByReleaseType([]*modrinthApi.Version{release, beta, alpha}, "beta")
	if len(result) != 2 {
		t.Errorf("expected release and beta versions, got %v", result)
	}
}

func TestFilterVersionsByReleaseTypeAlphaIncludesAll(t *testing.T) {
	release := &modrinthApi.Version{VersionType: strPtr("release")}
	beta := &modrinthApi.Version{VersionType: strPtr("beta")}
	alpha := &modrinthApi.Version{VersionType: strPtr("alpha")}

	result := filterVersionsByReleaseType([]*modrinthApi.Version{release, beta, alpha}, "alpha")
	if len(result) != 3 {
		t.Errorf("expected all versions, got %v", result)
	}
}

func TestFilterVersionsByReleaseTypeMissingTypeTreatedAsRelease(t *testing.T) {
	noType := &modrinthApi.Version{}

	result := filterVersionsByReleaseType([]*modrinthApi.Version{noType}, "release")
	if len(result) != 1 {
		t.Errorf("expected version with no version_type to be treated as a release, got %v", result)
	}
}

// --- comparableVersionNumber ---

func TestComparableVersionNumber(t *testing.T) {
	cases := []struct {
		name    string
		number  string
		loaders []string
		want    string
	}{
		{"loader before", "neoforge_1.21-2.0.8", []string{"neoforge"}, "1.21-2.0.8"},
		{"loader after", "1.21-2.0.15-neoforge", []string{"neoforge"}, "1.21-2.0.15"},
		{"loader in between", "1.21.1-neoforge-2.0.0", []string{"neoforge"}, "1.21.1-2.0.0"},
		{"loader in the appendix", "2.0.15+neoforge", []string{"neoforge"}, "2.0.15"},
		{"case is ignored", "NeoForge-1.0", []string{"neoforge"}, "1.0"},
		{"any of the loaders", "forge_1.0", []string{"forge", "neoforge"}, "1.0"},
		{"no tag", "1.21-2.1.10", []string{"neoforge"}, "1.21-2.1.10"},
		{"loader the version isn't for", "fabric_1.0", []string{"neoforge"}, "fabric_1.0"},
		{"loader that is part of a word", "neoforge_1.0", []string{"forge"}, "neoforge_1.0"},
		{"no loaders", "neoforge_1.0", nil, "neoforge_1.0"},
		{"nothing but a loader", "neoforge", []string{"neoforge"}, "neoforge"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := &modrinthApi.Version{VersionNumber: strPtr(c.number), Loaders: c.loaders}
			if got := comparableVersionNumber(v); got != c.want {
				t.Errorf("comparableVersionNumber(%q, loaders %v) = %q, want %q", c.number, c.loaders, got, c.want)
			}
		})
	}
}

func TestComparableVersionNumberMissingNumber(t *testing.T) {
	v := &modrinthApi.Version{Loaders: []string{"neoforge"}}
	if got := comparableVersionNumber(v); got != "" {
		t.Errorf("comparableVersionNumber() = %q, want an empty number", got)
	}
}

// neoForgeVersion is a NeoForge version of a project, with the given number, release type and date of publication
func neoForgeVersion(number, releaseType string, published time.Time) *modrinthApi.Version {
	return &modrinthApi.Version{
		VersionNumber: strPtr(number),
		VersionType:   strPtr(releaseType),
		GameVersions:  []string{"1.21.1"},
		Loaders:       []string{"neoforge"},
		DatePublished: timePtr(published),
	}
}

func at(month time.Month, day int) time.Time {
	return time.Date(2025, month, day, 0, 0, 0, 0, time.UTC)
}

// Amendments numbered its versions "neoforge_1.21-2.0.8", then "1.21-2.0.15-neoforge", then "1.21-2.1.10"
func TestFindLatestVersionFlexVerIgnoresLoaderTagsInVersionNumbers(t *testing.T) {
	tagged := neoForgeVersion("neoforge_1.21-2.0.8", "release", at(time.October, 1))
	suffixed := neoForgeVersion("1.21-2.0.15-neoforge", "release", at(time.November, 1))
	untagged := neoForgeVersion("1.21-2.1.10", "release", at(time.December, 1))

	result := findLatestVersion([]*modrinthApi.Version{tagged, suffixed, untagged}, []string{"1.21.1"}, true)
	if result != untagged {
		t.Errorf("expected 1.21-2.1.10 (the highest version number) to be picked, got %s", versionNumberOf(result))
	}
}

func TestFindLatestVersionFlexVerToleratesMissingVersionNumber(t *testing.T) {
	numbered := neoForgeVersion("1.0.0", "release", at(time.January, 1))
	unnumbered := neoForgeVersion("", "release", at(time.February, 1))
	unnumbered.VersionNumber = nil

	result := findLatestVersion([]*modrinthApi.Version{unnumbered, numbered}, []string{"1.21.1"}, true)
	if result != numbered {
		t.Errorf("expected the version with a number to be picked over the one without")
	}
}

// --- findHigherNumbered ---

func TestFindHigherNumberedIgnoresChangedLoaderTags(t *testing.T) {
	tagged := neoForgeVersion("neoforge_1.21-2.0.8", "release", at(time.October, 1))
	suffixed := neoForgeVersion("1.21-2.0.15-neoforge", "release", at(time.November, 1))
	latest := neoForgeVersion("1.21-2.1.10", "release", at(time.December, 1))

	if higher := findHigherNumbered(latest, []*modrinthApi.Version{latest, suffixed, tagged}, []string{"1.21.1"}); higher != nil {
		t.Errorf("expected no higher version number, got %s", versionNumberOf(higher))
	}
}

func TestFindHigherNumberedFindsHigherNumberPublishedEarlier(t *testing.T) {
	major := neoForgeVersion("3.0.0", "release", at(time.January, 1))
	backport := neoForgeVersion("2.9.1", "release", at(time.February, 1))

	higher := findHigherNumbered(backport, []*modrinthApi.Version{major, backport}, []string{"1.21.1"})
	if higher != major {
		t.Errorf("expected 3.0.0 to have the higher version number than the newer 2.9.1, got %v", higher)
	}
}

func TestFindHigherNumberedNoneWhenNewestIsHighest(t *testing.T) {
	older := neoForgeVersion("2.0.0", "release", at(time.January, 1))
	latest := neoForgeVersion("3.0.0", "release", at(time.February, 1))

	if higher := findHigherNumbered(latest, []*modrinthApi.Version{older, latest}, []string{"1.21.1"}); higher != nil {
		t.Errorf("expected no higher version number, got %s", versionNumberOf(higher))
	}
}

func TestFindHigherNumberedNoneForEqualVersionNumbers(t *testing.T) {
	older := neoForgeVersion("1.0.0", "release", at(time.January, 1))
	latest := neoForgeVersion("1.0.0", "release", at(time.February, 1))

	if higher := findHigherNumbered(latest, []*modrinthApi.Version{older, latest}, []string{"1.21.1"}); higher != nil {
		t.Errorf("expected no higher version number for equal numbers, got %s", versionNumberOf(higher))
	}
}

// A beta of the next version is ahead of the newest release by nature, so it isn't compared with the release
func TestFindHigherNumberedIgnoresLessStableVersions(t *testing.T) {
	beta := neoForgeVersion("2.1.0-beta.1", "beta", at(time.January, 1))
	latest := neoForgeVersion("2.0.0", "release", at(time.February, 1))

	if higher := findHigherNumbered(latest, []*modrinthApi.Version{beta, latest}, []string{"1.21.1"}); higher != nil {
		t.Errorf("expected the beta not to count against the release, got %s", versionNumberOf(higher))
	}
}

// A release with a higher number than the newest version, which is a beta, is a sign that the beta may be the wrong pick
func TestFindHigherNumberedCountsMoreStableVersions(t *testing.T) {
	release := neoForgeVersion("3.0.0", "release", at(time.January, 1))
	latest := neoForgeVersion("2.1.0-beta.2", "beta", at(time.February, 1))

	higher := findHigherNumbered(latest, []*modrinthApi.Version{release, latest}, []string{"1.21.1"})
	if higher != release {
		t.Errorf("expected the release 3.0.0 to count against the beta 2.1.0-beta.2, got %v", higher)
	}
}

// --- describeVersion ---

func TestDescribeVersion(t *testing.T) {
	v := neoForgeVersion("1.21-2.1.10", "beta", at(time.September, 6))
	if got, want := describeVersion(v), "1.21-2.1.10 (beta, published 2025-09-06)"; got != want {
		t.Errorf("describeVersion() = %q, want %q", got, want)
	}
}

func TestDescribeVersionWithoutTypeOrDate(t *testing.T) {
	v := &modrinthApi.Version{VersionNumber: strPtr("1.0.0")}
	if got, want := describeVersion(v), "1.0.0 (release)"; got != want {
		t.Errorf("describeVersion() = %q, want %q", got, want)
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

func TestGetBestHashFallbackIsDeterministic(t *testing.T) {
	// Regression test: the fallback previously picked an arbitrary entry via
	// Go's randomised map iteration order. It should now deterministically
	// pick the lexicographically smallest format name.
	f := &modrinthApi.File{Hashes: map[string]string{
		"zzz-hash": "zzz-val",
		"aaa-hash": "aaa-val",
		"mmm-hash": "mmm-val",
	}}
	for i := 0; i < 20; i++ {
		algo, val := getBestHash(f)
		if algo != "aaa-hash" || val != "aaa-val" {
			t.Fatalf("run %d: expected aaa-hash/aaa-val, got %s/%s", i, algo, val)
		}
	}
}

// --- isPrimary ---

func TestIsPrimaryTrue(t *testing.T) {
	primary := true
	f := &modrinthApi.File{Primary: &primary}
	if !isPrimary(f) {
		t.Error("expected true when Primary is set to true")
	}
}

func TestIsPrimaryFalse(t *testing.T) {
	primary := false
	f := &modrinthApi.File{Primary: &primary}
	if isPrimary(f) {
		t.Error("expected false when Primary is set to false")
	}
}

func TestIsPrimaryNilField(t *testing.T) {
	// Regression test: Primary is a nullable field in the API response; a nil
	// pointer must not panic and should be treated as "not primary".
	f := &modrinthApi.File{Primary: nil}
	if isPrimary(f) {
		t.Error("expected false when Primary is nil")
	}
}

func TestIsPrimaryNilFile(t *testing.T) {
	if isPrimary(nil) {
		t.Error("expected false for a nil *File")
	}
}
