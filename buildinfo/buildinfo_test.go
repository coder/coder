package buildinfo_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"

	"github.com/coder/coder/buildinfo"
)

func TestBuildInfo(t *testing.T) {
	t.Parallel()
	t.Run("Version", func(t *testing.T) {
		t.Parallel()
		version := buildinfo.Version()
		require.True(t, semver.IsValid(version))
		prerelease := semver.Prerelease(version)
		require.Equal(t, "-devel", prerelease)
		require.Equal(t, "v0", semver.Major(version))
	})
	t.Run("ExternalURL", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "https://github.com/coder/coder", buildinfo.ExternalURL())
	})
	// Tests don't include Go build info.
	t.Run("NoTime", func(t *testing.T) {
		t.Parallel()
		_, valid := buildinfo.Time()
		require.False(t, valid)
	})
}
