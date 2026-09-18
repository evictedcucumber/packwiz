package changelog

import (
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in   string
		want Version
	}{
		{"1.2.3", Version{1, 2, 3}},
		{"v1.2.3", Version{1, 2, 3}},
		{"0.0.0", Version{0, 0, 0}},
		{"10.20.30", Version{10, 20, 30}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseVersion(tt.in)
			if err != nil {
				t.Fatalf("ParseVersion(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseVersion(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseVersionRejectsInvalid(t *testing.T) {
	for _, in := range []string{
		"", "1", "1.2", "1.2.3.4", "a.b.c", "1.2.x", "-1.2.3", "1.-2.3", "+1.2.3",
		"01.2.3", "1.02.3", "1.2.03", // leading zeros aren't valid semver
		"1.2.3-beta", "1.2.3+build", " 1.2.3", "1.2.3 ",
		"99999999999999999999.0.0", // overflows int
	} {
		t.Run(in, func(t *testing.T) {
			if got, err := ParseVersion(in); err == nil {
				t.Errorf("ParseVersion(%q) = %v, want an error", in, got)
			}
		})
	}
}

func TestVersionStringDropsPrefix(t *testing.T) {
	v, err := ParseVersion("v4.5.6")
	if err != nil {
		t.Fatalf("ParseVersion() returned error: %v", err)
	}
	if got := v.String(); got != "4.5.6" {
		t.Errorf("String() = %q, want %q", got, "4.5.6")
	}
}

func TestVersionBump(t *testing.T) {
	base := Version{1, 2, 3}
	tests := []struct {
		bump Bump
		want Version
	}{
		{BumpMajor, Version{2, 0, 0}}, // minor and patch reset
		{BumpMinor, Version{1, 3, 0}}, // patch resets
		{BumpPatch, Version{1, 2, 4}},
		{BumpNone, Version{1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.bump.String(), func(t *testing.T) {
			if got := base.Bump(tt.bump); got != tt.want {
				t.Errorf("Bump(%v) = %v, want %v", tt.bump, got, tt.want)
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		a, b Version
		want int
	}{
		{Version{1, 2, 3}, Version{1, 2, 3}, 0},
		{Version{1, 2, 3}, Version{1, 2, 4}, -1},
		{Version{1, 2, 4}, Version{1, 2, 3}, 1},
		{Version{1, 3, 0}, Version{1, 2, 9}, 1},
		{Version{2, 0, 0}, Version{1, 9, 9}, 1},
		{Version{1, 9, 9}, Version{2, 0, 0}, -1},
		// Components compare numerically, not as text
		{Version{1, 10, 0}, Version{1, 9, 0}, 1},
	}
	for _, tt := range tests {
		if got := tt.a.Compare(tt.b); got != tt.want {
			t.Errorf("%v.Compare(%v) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestBumpOrdering(t *testing.T) {
	// HighestBump relies on more significant bumps comparing greater
	if !(BumpNone < BumpPatch && BumpPatch < BumpMinor && BumpMinor < BumpMajor) {
		t.Error("expected BumpNone < BumpPatch < BumpMinor < BumpMajor")
	}
}

func TestBumpTextRoundTrip(t *testing.T) {
	for b := range bumpNames {
		text, err := b.MarshalText()
		if err != nil {
			t.Fatalf("%v.MarshalText() returned error: %v", b, err)
		}
		var got Bump
		if err := got.UnmarshalText(text); err != nil {
			t.Fatalf("UnmarshalText(%q) returned error: %v", text, err)
		}
		if got != b {
			t.Errorf("round trip of %v = %v", b, got)
		}
	}
}

func TestBumpRejectsUnknown(t *testing.T) {
	var b Bump
	err := b.UnmarshalText([]byte("huge"))
	if err == nil || !strings.Contains(err.Error(), "huge") {
		t.Errorf("UnmarshalText(%q) error = %v, want one naming the bad value", "huge", err)
	}
	if _, err := Bump(99).MarshalText(); err == nil {
		t.Error("MarshalText() on an out-of-range Bump returned no error")
	}
}
