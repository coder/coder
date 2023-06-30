package exp

import (
	"net/http"

	"github.com/coder/coder/codersdk"
)

// bypassRateLimitHeaderTransport is a http.RoundTripper that adds the HTTP header
// X-Coder-Bypass-Ratelimit: true to all requests.
type bypassRateLimitHeaderTransport struct {
	transport http.RoundTripper
}

func (t *bypassRateLimitHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add(codersdk.BypassRatelimitHeader, "true")
	return t.transport.RoundTrip(req)
}
