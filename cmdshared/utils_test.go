package cmdshared

import "testing"

func TestGetRawForgeVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"mc-loader format", "1.20.1-47.1.106", "47.1.106"},
		{"loader only", "47.1.106", "47.1.106"},
		{"multiple dashes", "1.20.1-47.1.106-beta", "47.1.106"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRawForgeVersion(tt.input)
			if got != tt.expected {
				t.Errorf("GetRawForgeVersion(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
