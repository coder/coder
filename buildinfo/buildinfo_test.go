package buildinfo_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"

	"github.com/coder/coder/v2/buildinfo"
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

	t.Run("VersionsMatch", func(t *testing.T) {
		t.Parallel()

		type testcase struct {
			name        string
			v1          string
			v2          string
			expectMatch bool
		}

		cases := []testcase{
			{
				name:        "OK",
				v1:          "v1.2.3",
				v2:          "v1.2.3",
				expectMatch: true,
			},
			// Test that we return true if a developer version is detected.
			// Developers do not need to be warned of mismatched versions.
			{
				name:        "DevelIgnored",
				v1:          "v0.0.0-devel+123abac",
				v2:          "v1.2.3",
				expectMatch: true,
			},
			// Our CI instance uses a "-devel" prerelease
			// flag.
			{
				name:        "DevelPreleaseMajor",
				v1:          "v1.1.1-devel+123abac",
				v2:          "v1.2.3",
				expectMatch: false,
			},
			{
				name:        "DevelPreleaseSame",
				v1:          "v1.1.1-devel+123abac",
				v2:          "v1.1.9",
				expectMatch: true,
			},
			{
				name:        "MajorMismatch",
				v1:          "v1.2.3",
				v2:          "v0.1.2",
				expectMatch: false,
			},
			{
				name:        "MinorMismatch",
				v1:          "v1.2.3",
				v2:          "v1.3.2",
				expectMatch: false,
			},
			// Different patches are ok, breaking changes are not allowed
			// in patches.
			{
				name:        "PatchMismatch",
				v1:          "v1.2.3+hash.whocares",
				v2:          "v1.2.4+somestuff.hm.ok",
				expectMatch: true,
			},
		}

		for _, c := range cases {
			c := c
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				require.Equal(t, c.expectMatch, buildinfo.VersionsMatch(c.v1, c.v2),
					fmt.Sprintf("expected match=%v for version %s and %s", c.expectMatch, c.v1, c.v2),
				)
			})
		}
	})
}
