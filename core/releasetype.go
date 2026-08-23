package core

// The three possible values of a release type, from least to most stable
const (
	ReleaseTypeAlpha   = "alpha"
	ReleaseTypeBeta    = "beta"
	ReleaseTypeRelease = "release"
)

// releaseTypeStability ranks release types from least (0) to most stable; a pack/mod configured to
// accept a given release type also accepts any more stable type
var releaseTypeStability = map[string]int{
	ReleaseTypeAlpha:   0,
	ReleaseTypeBeta:    1,
	ReleaseTypeRelease: 2,
}

// IsValidReleaseType returns whether the given string is a recognised release type
func IsValidReleaseType(releaseType string) bool {
	_, ok := releaseTypeStability[releaseType]
	return ok
}

// ReleaseTypeAccepts returns whether a file/version with fileReleaseType should be accepted when
// acceptedReleaseType is the minimum acceptable release type (e.g. "beta" accepts "beta" and "release")
func ReleaseTypeAccepts(acceptedReleaseType string, fileReleaseType string) bool {
	minStability, ok := releaseTypeStability[acceptedReleaseType]
	if !ok {
		minStability = releaseTypeStability[ReleaseTypeRelease]
	}
	fileStability, ok := releaseTypeStability[fileReleaseType]
	if !ok {
		// Unknown/missing release types are treated as the most stable, so they aren't filtered out
		fileStability = releaseTypeStability[ReleaseTypeRelease]
	}
	return fileStability >= minStability
}
