package httpapi

import "net/http"

const (
	// XForwardedHostHeader is a header used by proxies to indicate the
	// original host of the request.
	XForwardedHostHeader = "X-Forwarded-Host"
)

// RequestHost returns the name of the host from the request.  It prioritizes
// 'X-Forwarded-Host' over r.Host since most requests are being proxied.
func RequestHost(r *http.Request) string {
	host := r.Header.Get(XForwardedHostHeader)
	if host != "" {
		return host
	}

	return r.Host
}

func IsWebsocketUpgrade(r *http.Request) bool {
	vs := r.Header.Values("Upgrade")
	for _, v := range vs {
		if v == "websocket" {
			return true
		}
	}
	return false
}
