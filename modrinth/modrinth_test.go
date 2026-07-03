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