package httpmw

import (
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/xerrors"
)

const (
	hstsHeader = "Strict-Transport-Security"
)

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
func HSTS(next http.Handler, maxAge int, options []string) (http.Handler, error) {
	if maxAge <= 0 {
		// No header, so no need to wrap the handler
		return next, nil
	}

	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Strict-Transport-Security
	var str strings.Builder
	_, err := str.WriteString(fmt.Sprintf("max-age=%d", maxAge))
	if err != nil {
		return nil, xerrors.Errorf("hsts: write max-age: %w", err)
	}

	for _, option := range options {
		switch {
		// Only allow valid options and fix any casing mistakes
		case strings.EqualFold(option, "includeSubDomains"):
			option = "includeSubDomains"
		case strings.EqualFold(option, "preload"):
			option = "preload"
		default:
			return nil, xerrors.Errorf("hsts: invalid option: %q. Must be 'preload' and/or 'includeSubDomains'", option)
		}
		_, err = str.WriteString("; " + option)
		if err != nil {
			return nil, xerrors.Errorf("hsts: write option: %w", err)
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(hstsHeader, str.String())
		next.ServeHTTP(w, r)
	}), nil
}
