package mcpclient

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckRevocationRedirect(t *testing.T) {
	t.Parallel()

	req := func(method, rawURL string) *http.Request {
		u, err := url.Parse(rawURL)
		require.NoError(t, err)
		return &http.Request{Method: method, URL: u}
	}

	origin := "https://provider.example/revoke"

	cases := []struct {
		name       string
		req        *http.Request
		origin     string
		wantErr    string
		wantAbsent string
	}{
		{
			name: "SamePathOnOrigin",
			req:  req(http.MethodPost, "https://provider.example/revoke2"),
		},
		{
			name: "ExplicitDefaultPort",
			req:  req(http.MethodPost, "https://provider.example:443/revoke2"),
		},
		{
			name:    "DifferentPort",
			req:     req(http.MethodPost, "https://provider.example:8443/collect"),
			wantErr: "must stay on origin",
		},
		{
			name:       "DifferentHost",
			req:        req(http.MethodPost, "https://attacker.example/collect?token=reflected-token#fragment"),
			wantErr:    "must stay on origin",
			wantAbsent: "reflected-token",
		},
		{
			name:    "BodyDroppingGet",
			req:     req(http.MethodGet, "https://provider.example/other"),
			wantErr: "dropped the POST body",
		},
		{
			name:    "PlaintextTarget",
			req:     req(http.MethodPost, "http://provider.example/revoke"),
			wantErr: "must use https",
		},
		{
			name:   "LoopbackToLoopbackAnyPort",
			req:    req(http.MethodPost, "http://127.0.0.1:9999/revoke"),
			origin: "http://localhost:1234/revoke",
		},
		{
			name:    "OriginToLoopback",
			req:     req(http.MethodPost, "http://localhost:1234/revoke"),
			wantErr: "must stay on origin",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			o := tc.origin
			if o == "" {
				o = origin
			}
			err := checkRevocationRedirect(tc.req, []*http.Request{req(http.MethodPost, o)})
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
			if tc.wantAbsent != "" {
				require.NotContains(t, err.Error(), tc.wantAbsent)
			}
		})
	}
}
