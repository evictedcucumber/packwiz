package changelog

import (
	"fmt"
	"regexp"
	"strconv"
)

// Bump is how far a set of changes raises a pack's version. Higher values are more significant, so the bump for a set
// of changes is simply the largest bump of any one change.
type Bump int

const (
	BumpNone Bump = iota
	BumpPatch
	BumpMinor
	BumpMajor
)

var bumpNames = map[Bump]string{
	BumpNone:  "none",
	BumpPatch: "patch",
	BumpMinor: "minor",
	BumpMajor: "major",
}

func (b Bump) String() string {
	if name, ok := bumpNames[b]; ok {
		return name
	}
	return fmt.Sprintf("Bump(%d)", int(b))
}

// MarshalText implements encoding.TextMarshaler, so a Bump is stored in TOML by name rather than by number.
func (b Bump) MarshalText() ([]byte, error) {
	name, ok := bumpNames[b]
	if !ok {
		return nil, fmt.Errorf("invalid bump %d", int(b))
	}
	return []byte(name), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (b *Bump) UnmarshalText(text []byte) error {
	for candidate, name := range bumpNames {
		if name == string(text) {
			*b = candidate
			return nil
		}
	}
	return fmt.Errorf("invalid bump %q; must be one of: none, patch, minor, major", text)
}

// Version is a semantic version (MAJOR.MINOR.PATCH). Pre-release and build metadata aren't supported, since a
// pack's version is only ever raised by a Bump.
type Version struct {
	Major, Minor, Patch int
}

var versionRegex = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)

// ParseVersion parses a MAJOR.MINOR.PATCH version, tolerating a leading "v" (as used in Git tags).
func ParseVersion(s string) (Version, error) {
	m := versionRegex.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("%q is not a valid version; expected MAJOR.MINOR.PATCH, e.g. 1.2.3", s)
	}
	var parts [3]int
	for i := range parts {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return Version{}, fmt.Errorf("%q is not a valid version: %w", s, err)
		}
		parts[i] = n
	}
	return Version{parts[0], parts[1], parts[2]}, nil
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Bump returns the version raised by b, resetting the lower components as semver requires.
func (v Version) Bump(b Bump) Version {
	switch b {
	case BumpMajor:
		return Version{v.Major + 1, 0, 0}
	case BumpMinor:
		return Version{v.Major, v.Minor + 1, 0}
	case BumpPatch:
		return Version{v.Major, v.Minor, v.Patch + 1}
	default:
		return v
	}
}

// Compare returns -1, 0 or 1 if v is less than, equal to, or greater than other.
func (v Version) Compare(other Version) int {
	for _, pair := range [][2]int{{v.Major, other.Major}, {v.Minor, other.Minor}, {v.Patch, other.Patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}
