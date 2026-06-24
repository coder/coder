package workspaceapps

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_originLocalPath verifies that originLocalPath, the helper used to build
// every redirect Location derived from r.URL.Path, keeps the target on the
// current origin (ANT-2026-22456). Call sites build the Location as
// url.URL{Path: originLocalPath(...)}.String(), so the assertions mirror that.
func Test_originLocalPath(t *testing.T) {
	t.Parallel()

	t.Run("RejectsOffOrigin", func(t *testing.T) {
		t.Parallel()

		// Each input models an already-percent-decoded r.URL.Path that tries to
		// smuggle a separate host into the redirect.
		maliciousPaths := []string{
			"//evil.com/phish",
			"///evil.com/phish",
			"/\\evil.com/phish",
			"/\\/evil.com/phish",
			"\\\\evil.com/phish",
			"/\t/evil.com/phish",
			"/\t\\evil.com/phish",
			"/\n/evil.com/phish",
			"/\r/evil.com/phish",
			"/\t//evil.com/phish",
		}

		for _, p := range maliciousPaths {
			loc := (&url.URL{Path: originLocalPath(p)}).String()

			// Model browser URL normalization: tab/newline/CR are stripped and a
			// backslash is treated like a forward slash before resolving.
			browserLoc := strings.NewReplacer("\t", "", "\n", "", "\r", "").Replace(loc)
			browserLoc = strings.ReplaceAll(browserLoc, "\\", "/")
			require.Falsef(t, strings.HasPrefix(browserLoc, "//"),
				"path %q produced off-origin Location %q (browser-normalized %q)", p, loc, browserLoc)

			parsed, err := url.Parse(loc)
			require.NoErrorf(t, err, "path %q produced unparseable Location %q", p, loc)
			require.Emptyf(t, parsed.Scheme, "path %q produced Location %q with a scheme", p, loc)
			require.Emptyf(t, parsed.Host, "path %q produced Location %q with a host", p, loc)
		}
	})

	t.Run("PreservesLegitPaths", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			in   string
			want string
		}{
			{in: "", want: "/"},
			{in: "/", want: "/"},
			{in: "/test", want: "/test"},
			{in: "/app/sub/page", want: "/app/sub/page"},
			{in: "/@user/ws/apps/app", want: "/@user/ws/apps/app"},
		}

		for _, tc := range cases {
			require.Equalf(t, tc.want, originLocalPath(tc.in), "originLocalPath(%q)", tc.in)
		}
	})
}
