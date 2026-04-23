package apidump

// sensitiveRequestHeaders are headers that should be redacted from request dumps.
var sensitiveRequestHeaders = map[string]struct{}{
	"Authorization":        {},
	"X-Api-Key":            {},
	"Api-Key":              {},
	"X-Auth-Token":         {},
	"Cookie":               {},
	"Proxy-Authorization":  {},
	"X-Amz-Security-Token": {},
}

// sensitiveResponseHeaders are headers that should be redacted from response dumps.
// Note: header names use Go's canonical form (http.CanonicalHeaderKey).
var sensitiveResponseHeaders = map[string]struct{}{
	"Set-Cookie":         {},
	"Www-Authenticate":   {},
	"Proxy-Authenticate": {},
}
