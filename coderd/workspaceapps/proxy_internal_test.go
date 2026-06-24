package workspaceapps

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_originLocalPath checks that originLocalPath keeps a redirect target on
// the current origin. Call sites build the Location as
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

	t.Run("EscapesControlCharacters", func(t *testing.T) {
		t.Parallel()

		// A redirect built from a path containing a raw control character is an
		// open redirect: http.Redirect emits it verbatim (url.Parse rejects the
		// control byte and skips cleaning) and browsers strip tab/newline/CR
		// before resolving, re-forming "//evil.com". Building the Location via
		// url.URL percent-encodes each one. Assert every class is escaped so a
		// future change that breaks encoding for only one class is caught.
		cases := []struct {
			name string
			in   string
			want string
		}{
			{name: "Tab", in: "/\t/evil.com", want: "/%09/evil.com"},
			{name: "Newline", in: "/\n/evil.com", want: "/%0A/evil.com"},
			{name: "CarriageReturn", in: "/\r/evil.com", want: "/%0D/evil.com"},
		}

		for _, tc := range cases {
			loc := (&url.URL{Path: originLocalPath(tc.in)}).String()
			require.Equalf(t, tc.want, loc, "%s: originLocalPath(%q) must percent-encode the control character", tc.name, tc.in)
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
