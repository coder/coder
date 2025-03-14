package httpmw

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

)
const (
	hstsHeader = "Strict-Transport-Security"

)
type HSTSConfig struct {
	// HeaderValue is an empty string if hsts header is disabled.
	HeaderValue string

}
func HSTSConfigOptions(maxAge int, options []string) (HSTSConfig, error) {
	if maxAge <= 0 {
		// No header, so no need to build the header string.
		return HSTSConfig{HeaderValue: ""}, nil

	}
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Strict-Transport-Security
	var str strings.Builder
	_, err := str.WriteString(fmt.Sprintf("max-age=%d", maxAge))
	if err != nil {
		return HSTSConfig{}, fmt.Errorf("hsts: write max-age: %w", err)

	}
	for _, option := range options {
		switch {
		// Only allow valid options and fix any casing mistakes
		case strings.EqualFold(option, "includeSubDomains"):
			option = "includeSubDomains"
		case strings.EqualFold(option, "preload"):

			option = "preload"
		default:
			return HSTSConfig{}, fmt.Errorf("hsts: invalid option: %q. Must be 'preload' and/or 'includeSubDomains'", option)
		}
		_, err = str.WriteString("; " + option)
		if err != nil {
			return HSTSConfig{}, fmt.Errorf("hsts: write option: %w", err)
		}
	}
	return HSTSConfig{
		HeaderValue: str.String(),
	}, nil
}
// HSTS will add the strict-transport-security header if enabled. This header
// forces a browser to always use https for the domain after it loads https once.
// Meaning: On first load of product.coder.com, they are redirected to https. On
// all subsequent loads, the client's local browser forces https. This prevents
// man in the middle.
//
// This header only makes sense if the app is using tls.

//
// Full header example:
// Strict-Transport-Security: max-age=63072000; includeSubDomains; preload
func HSTS(next http.Handler, cfg HSTSConfig) http.Handler {
	if cfg.HeaderValue == "" {
		// No header, so no need to wrap the handler.
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(hstsHeader, cfg.HeaderValue)
		next.ServeHTTP(w, r)
	})
}
