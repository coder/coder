package xurl_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/xurl"
)

func TestContainsEncodedPath(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		path string
		want bool
	}{
		{name: "Plain", path: "/v1/models"},
		{name: "ModelID", path: "/v1/models/gpt-4.1"},
		{name: "ResourceID", path: "/v1/responses/resp_abc123"},
		{name: "TrailingSlash", path: "/v1/models/"},
		{name: "EscapedSeparator", path: "/v1/models/org%2Fmodel"},
		{name: "EscapedSpace", path: "/v1/models/a%20b"},
		{name: "DotsInName", path: "/v1/models/..model"},
		{name: "LiteralDotDot", path: "/v1/models/../files", want: true},
		{name: "LiteralDot", path: "/v1/./models", want: true},
		{name: "EncodedDotDot", path: "/v1/models/%2e%2e/files", want: true},
		{name: "EncodedDotDotUpper", path: "/v1/models/%2E%2E/files", want: true},
		{name: "MixedDotDot", path: "/v1/models/.%2e/files", want: true},
		{name: "EncodedDot", path: "/v1/%2e/models", want: true},
		{name: "DotDotEscapedSlash", path: "/v1/models/..%2F..%2Ffiles", want: true},
		{name: "DotDotInsideSegment", path: "/v1/models/a%2F..%2Fb", want: true},
		{name: "DotDotEscapedBackslash", path: "/v1/models/..%5Cfiles", want: true},
		{name: "DoubleEncodedDotDot", path: "/v1/models/%252e%252e/files", want: true},
		{name: "TripleEncodedDotDot", path: "/v1/models/%25252e%25252e/files", want: true},
		{name: "EncodedNUL", path: "/v1/models/a%00b", want: true},
		{name: "EncodedNewline", path: "/v1/models/a%0Ab", want: true},
		{name: "InvalidEscape", path: "/v1/models/%zz", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Build the URL the way the server does for an incoming
			// request line, preserving the client's escaping.
			u, err := url.ParseRequestURI(tc.path)
			if err != nil {
				// Paths the server itself refuses to parse never reach
				// the handler.
				require.True(t, tc.want, "unexpected parse error: %v", err)
				return
			}
			require.Equal(t, tc.want, xurl.ContainsEncodedPath(u))
		})
	}
}
