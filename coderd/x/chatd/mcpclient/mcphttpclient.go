package mcpclient

import (
	"net/http"

	"github.com/coder/safedial"
)

func httpClientWithHeaders(base *http.Client, headers map[string]string) *http.Client {
	if base == nil {
		base = safedial.NewHTTPClient(&http.Client{})
	} else if base.Transport == nil {
		base = safedial.NewHTTPClient(base)
	}
	client := *base
	client.CheckRedirect = safedial.CheckSameOriginRedirect
	if len(headers) == 0 {
		return &client
	}
	transport := base.Transport
	client.Transport = &headerRoundTripper{
		base:    transport,
		headers: headers,
	}
	return &client
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for k, v := range h.headers {
		clone.Header.Set(k, v)
	}
	return h.base.RoundTrip(clone)
}
