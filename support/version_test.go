package support_test

import (
	"testing"

	"github.com/coder/coder/v2/support"
)

func TestVersionSupportsPprof(t *testing.T) {
	t.Parallel()
	tests := []struct {
		version string
		want    bool
	}{
		{"", false},
		{"v2.27.0", false},
		{"v2.27.9", false},
		{"v2.28.0", true},
		{"v2.28.1", true},
		{"v2.29.0", true},
		{"v3.0.0", true},
		{"2.28.0", true},               // without v prefix
		{"2.27.0", false},              // without v prefix
		{"v2.28.0-devel+abc123", true}, // dev version
		{"v2.27.0-devel+abc123", false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			t.Parallel()
			got := support.VersionSupportsPprof(tt.version)
			if got != tt.want {
				t.Errorf("versionSupportsPprof(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
