package codersdk_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestValidateOAuth2RedirectURIShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		url   string
		valid bool
	}{
		{url: "https://app.example.com/callback", valid: true},
		{url: "http://127.0.0.1:3000/callback", valid: true},
		{url: "http://example.com/callback", valid: true},
		{url: "cursor://anysphere.cursor-mcp/oauth/callback", valid: true},
		{url: "vscode://coder.coder-remote/oauth/callback", valid: true},
		{url: "com.example.app:/oauth2redirect", valid: true},
		{url: "urn:ietf:wg:oauth:2.0:oob", valid: true},
		{url: "", valid: false},
		{url: "javascript:alert(1)", valid: false},
		{url: "data:text/plain,hello", valid: false},
		{url: "file:///tmp/callback", valid: false},
		{url: "ftp://example.com/callback", valid: false},
		{url: "urn:example:callback", valid: false},
		{url: "https:/example.com", valid: false},
		{url: "http:///example.com", valid: false},
		{url: "localhost:3000", valid: false},
		{url: "vscode:", valid: false},
		{url: "vscode://", valid: false},
		{url: "mailto:a@b", valid: false},
		{url: "https://app.example.com/callback#fragment", valid: false},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			err := codersdk.ValidateOAuth2RedirectURIShape(tc.url)
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			// Dynamic registration must not store a URI the shape check
			// rejects, whatever the client type.
			require.Error(t, codersdk.ValidateRedirectURI(tc.url, codersdk.OAuth2ClientTypeConfidential))
			require.Error(t, codersdk.ValidateRedirectURI(tc.url, codersdk.OAuth2ClientTypePublic))
		})
	}
}

func TestValidateRedirectURIsSize(t *testing.T) {
	t.Parallel()

	list := func(n int) []string {
		out := make([]string, 0, n)
		for i := range n {
			out = append(out, fmt.Sprintf("https://example.com/callback/%d", i))
		}
		return out
	}

	t.Run("AtCountLimit", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, codersdk.ValidateRedirectURIs(list(codersdk.OAuth2RedirectURIsMaxCount), codersdk.OAuth2ClientTypeConfidential))
	})

	t.Run("OverCountLimit", func(t *testing.T) {
		t.Parallel()
		err := codersdk.ValidateRedirectURIs(list(codersdk.OAuth2RedirectURIsMaxCount+1), codersdk.OAuth2ClientTypeConfidential)
		require.ErrorContains(t, err, "at most 32 redirect URIs")
	})

	t.Run("AtByteLimit", func(t *testing.T) {
		t.Parallel()
		prefix := "https://example.com/"
		long := prefix + strings.Repeat("a", codersdk.OAuth2RedirectURIMaxBytes-len(prefix))
		require.Len(t, long, codersdk.OAuth2RedirectURIMaxBytes)
		require.NoError(t, codersdk.ValidateRedirectURI(long, codersdk.OAuth2ClientTypeConfidential))
	})

	t.Run("OverByteLimit", func(t *testing.T) {
		t.Parallel()
		prefix := "https://example.com/"
		long := prefix + strings.Repeat("a", codersdk.OAuth2RedirectURIMaxBytes-len(prefix)+1)
		err := codersdk.ValidateRedirectURI(long, codersdk.OAuth2ClientTypeConfidential)
		require.ErrorContains(t, err, "at most 2048 bytes")
	})

	// The length check runs before parsing, so an oversized value reports
	// its size and is never inspected further.
	t.Run("SizeBeforeParse", func(t *testing.T) {
		t.Parallel()
		huge := "javascript:" + strings.Repeat("a", 1<<20)
		err := codersdk.ValidateRedirectURIs([]string{huge}, codersdk.OAuth2ClientTypePublic)
		require.ErrorContains(t, err, "at most 2048 bytes")
		require.NotContains(t, err.Error(), "javascript")
	})

	t.Run("IndexIsReported", func(t *testing.T) {
		t.Parallel()
		err := codersdk.ValidateRedirectURIs([]string{"https://ok.example/cb", "mailto:a@b"}, codersdk.OAuth2ClientTypePublic)
		require.ErrorContains(t, err, "redirect URI at index 1: public clients may not use the mailto scheme")
	})
}
