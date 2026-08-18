package mcpclient_test

import (
	"net/http"
	"net/netip"

	"github.com/coder/coder/v2/coderd/mcpssrf"
)

var testLoopbackPrefixes = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("::1/128"),
}

func testMCPHTTPClient(base *http.Client) *http.Client {
	return mcpssrf.NewHTTPClient(base, testLoopbackPrefixes)
}
