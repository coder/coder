package httpapi

import "net/http"

const (
	// XForwardedHostHeader is a header used by proxies to indicate the
	// original host of the request.
	XForwardedHostHeader = "X-Forwarded-Host"
)

func IsWebsocketUpgrade(r *http.Request) bool {
	vs := r.Header.Values("Upgrade")
	for _, v := range vs {
		if v == "websocket" {
			return true
		}
	}
	return false
}
