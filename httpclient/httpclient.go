package httpclient

import "net/http"

// New creates an HTTP client with an isolated transport cloned from http.DefaultTransport.
// This prevents tests from interfering with each other when CloseIdleConnections is called
// on the shared default transport, among other issues with sharing global state. Each client
// returned by this function has its own connection pool and transport state.
//
// For production use, callers should manage the lifecycle of the returned client and call
// CloseIdleConnections when done. In tests, use t.Cleanup(client.CloseIdleConnections).
func New() *http.Client {
	var transport http.RoundTripper
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = defaultTransport.Clone()
	} else {
		// Fallback if DefaultTransport is not *http.Transport (very unlikely but safe).
		transport = http.DefaultTransport
	}
	return &http.Client{Transport: transport}
}
