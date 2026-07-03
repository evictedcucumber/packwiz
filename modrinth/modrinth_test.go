package modrinth

import "testing"

func TestFilterVersionsByAllowedChannel(t *testing.T) {
	stringPtr := func(value string) *string {
		return &value
	}

	versions := []*Version{
		{ID: stringPtr("release"), VersionType: stringPtr("release")},
		{ID: stringPtr("beta"), VersionType: stringPtr("beta")},
		{ID: stringPtr("alpha"), VersionType: stringPtr("alpha")},
	}

	tests := []struct {
		name          string
		allowedChannel string
		wantIDs       []string
	}{
		{name: "release only", allowedChannel: "release", wantIDs: []string{"release"}},
		{name: "release and beta", allowedChannel: "beta", wantIDs: []string{"release", "beta"}},
		{name: "all channels", allowedChannel: "alpha", wantIDs: []string{"release", "beta", "alpha"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filtered := filterVersionsByAllowedChannel(versions, test.allowedChannel)
			if len(filtered) != len(test.wantIDs) {
				t.Fatalf("got %d versions, want %d", len(filtered), len(test.wantIDs))
			}
			for i, version := range filtered {
				if version.ID == nil {
					t.Fatalf("filtered version %d had nil ID", i)
				}
				if *version.ID != test.wantIDs[i] {
					t.Fatalf("filtered version %d = %q, want %q", i, *version.ID, test.wantIDs[i])
				}
			}
		})
	}
}

func TestShouldWarnAboutVersionMismatch(t *testing.T) {
	stringPtr := func(value string) *string {
		return &value
	}

	tests := []struct {
		name     string
		left     *Version
		right    *Version
		wantWarn bool
	}{
		{
			name:     "same channel warns",
			left:     &Version{VersionNumber: stringPtr("1.0.0"), VersionType: stringPtr("release")},
			right:    &Version{VersionNumber: stringPtr("1.0.1"), VersionType: stringPtr("release")},
			wantWarn: true,
		},
		{
			name:     "different channels do not warn",
			left:     &Version{VersionNumber: stringPtr("1.0.0"), VersionType: stringPtr("release")},
			right:    &Version{VersionNumber: stringPtr("1.0.1"), VersionType: stringPtr("beta")},
			wantWarn: false,
		},
		{
			name:     "missing type does not warn",
			left:     &Version{VersionNumber: stringPtr("1.0.0")},
			right:    &Version{VersionNumber: stringPtr("1.0.1"), VersionType: stringPtr("release")},
			wantWarn: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldWarnAboutVersionMismatch(test.left, test.right); got != test.wantWarn {
				t.Fatalf("got %v, want %v", got, test.wantWarn)
			}
		})
	}
}