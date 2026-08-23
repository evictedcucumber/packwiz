package core

import "testing"

func TestIsValidReleaseType(t *testing.T) {
	for _, v := range []string{"release", "beta", "alpha"} {
		if !IsValidReleaseType(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	for _, v := range []string{"", "snapshot", "Release"} {
		if IsValidReleaseType(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestReleaseTypeAccepts(t *testing.T) {
	cases := []struct {
		accepted string
		file     string
		want     bool
	}{
		{ReleaseTypeRelease, ReleaseTypeRelease, true},
		{ReleaseTypeRelease, ReleaseTypeBeta, false},
		{ReleaseTypeRelease, ReleaseTypeAlpha, false},
		{ReleaseTypeBeta, ReleaseTypeRelease, true},
		{ReleaseTypeBeta, ReleaseTypeBeta, true},
		{ReleaseTypeBeta, ReleaseTypeAlpha, false},
		{ReleaseTypeAlpha, ReleaseTypeRelease, true},
		{ReleaseTypeAlpha, ReleaseTypeBeta, true},
		{ReleaseTypeAlpha, ReleaseTypeAlpha, true},
	}
	for _, c := range cases {
		got := ReleaseTypeAccepts(c.accepted, c.file)
		if got != c.want {
			t.Errorf("ReleaseTypeAccepts(%q, %q) = %v, want %v", c.accepted, c.file, got, c.want)
		}
	}
}

func TestReleaseTypeAcceptsUnknownAcceptedFallsBackToRelease(t *testing.T) {
	if !ReleaseTypeAccepts("bogus", ReleaseTypeRelease) {
		t.Error("expected unknown accepted release type to fall back to requiring release")
	}
	if ReleaseTypeAccepts("bogus", ReleaseTypeBeta) {
		t.Error("expected unknown accepted release type to fall back to requiring release")
	}
}
