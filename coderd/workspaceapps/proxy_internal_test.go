package workspaceapps

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_originLocalURL checks that originLocalURL produces a redirect target that
// stays on the current origin.
func Test_originLocalURL(t *testing.T) {
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
			loc := originLocalURL(p).String()

			// The Location must parse as a relative, same-origin reference.
			require.Falsef(t, strings.HasPrefix(loc, "//"),
				"path %q produced scheme-relative Location %q", p, loc)
			parsed, err := url.Parse(loc)
			require.NoErrorf(t, err, "path %q produced unparseable Location %q", p, loc)
			require.Emptyf(t, parsed.Scheme, "path %q produced Location %q with a scheme", p, loc)
			require.Emptyf(t, parsed.Host, "path %q produced Location %q with a host", p, loc)

			// It must also be free of raw bytes a browser would normalize back
			// into an authority before resolving (a backslash becomes "/", and
			// tab/newline/CR are stripped, either of which could re-form "//host").
			// url.URL.String() guarantees this by percent-encoding them; we assert
			// it here rather than reproducing browser normalization in the code.
			for _, raw := range []string{`\`, "\t", "\n", "\r"} {
				require.NotContainsf(t, loc, raw,
					"path %q produced Location %q containing a raw %q", p, loc, raw)
			}
		}
	})

	t.Run("EscapesControlCharacters", func(t *testing.T) {
		t.Parallel()

		// A redirect built from a path containing a raw control character is an
		// open redirect: http.Redirect emits it verbatim (url.Parse rejects the
		// control byte and skips cleaning) and browsers strip tab/newline/CR
		// before resolving, re-forming "//evil.com". originLocalURL percent-encodes
		// each one. Assert every class is escaped so a future change that breaks
		// encoding for only one class is caught.
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
			loc := originLocalURL(tc.in).String()
			require.Equalf(t, tc.want, loc, "%s: originLocalURL(%q) must percent-encode the control character", tc.name, tc.in)
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
			require.Equalf(t, tc.want, originLocalURL(tc.in).String(), "originLocalURL(%q)", tc.in)
		}
	})
}
