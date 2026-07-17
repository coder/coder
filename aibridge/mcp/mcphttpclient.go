package mcp

import (
	"flag"
	"net/http"
)

// mcpHTTPClient returns an isolated *http.Client when running
// inside tests, or nil for production. During tests,
// httptest.Server.Close() calls
// http.DefaultTransport.CloseIdleConnections(), which disrupts
// any MCP client sharing that transport. When DefaultTransport
// is a *http.Transport it is cloned; otherwise a minimal
// transport with ProxyFromEnvironment is created as a fallback.
func mcpHTTPClient() *http.Client {
	if flag.Lookup("test.v") == nil {
		return nil
	}
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		return &http.Client{Transport: dt.Clone()}
	}
	return &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}}
}
