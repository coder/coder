package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  version
		ok    bool
	}{
		{"v2.32.0", version{Major: 2, Minor: 32, Patch: 0}, true},
		{"v2.32.1", version{Major: 2, Minor: 32, Patch: 1}, true},
		{"v2.32.0-rc.0", version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0"}, true},
		{"v2.32.0-rc.12", version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.12"}, true},
		{"v1.0.0-beta.1", version{Major: 1, Minor: 0, Patch: 0, Pre: "beta.1"}, true},
		{"2.32.0", version{}, false},          // missing v prefix
		{"v2.32", version{}, false},            // missing patch
		{"v2.32.0-", version{}, false},         // trailing dash
		{"vx.y.z", version{}, false},           // non-numeric
		{"v2.32.0-rc 1", version{}, false},     // space in pre-release
		{"v2.32.0-rc.0.1", version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0.1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, ok := parseVersion(tt.input)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "v2.32.0", version{Major: 2, Minor: 32, Patch: 0}.String())
	assert.Equal(t, "v2.32.0-rc.0", version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0"}.String())
	assert.Equal(t, "v1.0.0-beta.1", version{Major: 1, Minor: 0, Patch: 0, Pre: "beta.1"}.String())
}

func TestVersionIsPrerelease(t *testing.T) {
	t.Parallel()
	assert.False(t, version{Major: 2, Minor: 32, Patch: 0}.IsPrerelease())
	assert.True(t, version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0"}.IsPrerelease())
}

func TestVersionGreaterThan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b version
		want bool
	}{
		{"major", version{Major: 3}, version{Major: 2}, true},
		{"minor", version{Major: 2, Minor: 32}, version{Major: 2, Minor: 31}, true},
		{"patch", version{Major: 2, Minor: 32, Patch: 1}, version{Major: 2, Minor: 32, Patch: 0}, true},
		{"stable > rc", version{Major: 2, Minor: 32, Patch: 0}, version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.1"}, true},
		{"rc < stable", version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.1"}, version{Major: 2, Minor: 32, Patch: 0}, false},
		{"rc.1 > rc.0", version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.1"}, version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0"}, true},
		{"rc.0 < rc.1", version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0"}, version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.1"}, false},
		{"equal not greater", version{Major: 2, Minor: 32, Patch: 0}, version{Major: 2, Minor: 32, Patch: 0}, false},
		{"equal rc not greater", version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0"}, version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.a.GreaterThan(tt.b))
		})
	}
}

func TestVersionEqual(t *testing.T) {
	t.Parallel()
	assert.True(t, version{Major: 2, Minor: 32, Patch: 0}.Equal(version{Major: 2, Minor: 32, Patch: 0}))
	assert.False(t, version{Major: 2, Minor: 32, Patch: 0}.Equal(version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0"}))
	assert.True(t, version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0"}.Equal(version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0"}))
}

func TestVersionBaseEqual(t *testing.T) {
	t.Parallel()
	assert.True(t, version{Major: 2, Minor: 32, Patch: 0}.baseEqual(version{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0"}))
	assert.False(t, version{Major: 2, Minor: 32, Patch: 0}.baseEqual(version{Major: 2, Minor: 32, Patch: 1}))
}

func TestFilterStable(t *testing.T) {
	t.Parallel()
	tags := []version{
		{Major: 2, Minor: 32, Patch: 0},
		{Major: 2, Minor: 32, Patch: 0, Pre: "rc.1"},
		{Major: 2, Minor: 32, Patch: 0, Pre: "rc.0"},
		{Major: 2, Minor: 31, Patch: 1},
	}
	stable := filterStable(tags)
	require.Len(t, stable, 2)
	assert.Equal(t, "v2.32.0", stable[0].String())
	assert.Equal(t, "v2.31.1", stable[1].String())
}

func TestRCNumber(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, version{Pre: "rc.0"}.rcNumber())
	assert.Equal(t, 12, version{Pre: "rc.12"}.rcNumber())
	assert.Equal(t, -1, version{Pre: "beta.1"}.rcNumber())
	assert.Equal(t, -1, version{}.rcNumber())
}
