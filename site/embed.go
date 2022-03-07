//go:build !slim
// +build !slim

package site

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/justinas/nosurf"
	"github.com/unrolled/secure"

	"cdr.dev/slog"

	"github.com/coder/coder/site/nextrouter"
)

// The `embed` package ignores recursively including directories
// that prefix with `_`. Wildcarding nested is janky, but seems to
// work quite well for edge-cases.
//go:embed out/_next/*/*/*/*
//go:embed out/_next/*/*/*
//go:embed out/bin/*
//go:embed out
var site embed.FS

// Handler returns an HTTP handler for serving the static site.
func Handler(logger slog.Logger) http.Handler {
	filesystem, err := fs.Sub(site, "out")
	if err != nil {
		// This can't happen... Go would throw a compilation error.
		panic(err)
	}

	// Render CSP and CSRF in the served pages
	templateFunc := func(r *http.Request) interface{} {
		return htmlState{
			// Nonce is the CSP nonce for the given request (if there is one present)
			CSP: cspState{Nonce: secure.CSPNonce(r.Context())},
			// Token is the CSRF token for the given request
			CSRF: csrfState{Token: nosurf.Token(r)},
		}
	}

	nextRouterHandler, err := nextrouter.Handler(filesystem, &nextrouter.Options{
		Logger:           logger,
		TemplateDataFunc: templateFunc,
	})
	if err != nil {
		// There was an error setting up our file system handler.
		// This likely means a problem with our embedded file system.
		panic(err)
	}
	return secureHeaders(nextRouterHandler)
}

type htmlState struct {
	CSP  cspState
	CSRF csrfState
}

type cspState struct {
	Nonce string
}

type csrfState struct {
	Token string
}

// cspDirectives is a map of all csp fetch directives to their values.
// Each directive is a set of values that is joined by a space (' ').
// All directives are semi-colon separated as a single string for the csp header.
type cspDirectives map[cspFetchDirective][]string

// cspFetchDirective is the list of all constant fetch directives that
// can be used/appended to.
type cspFetchDirective string

const (
	cspDirectiveDefaultSrc  = "default-src"
	cspDirectiveConnectSrc  = "connect-src"
	cspDirectiveChildSrc    = "child-src"
	cspDirectiveScriptSrc   = "script-src"
	cspDirectiveFontSrc     = "font-src"
	cspDirectiveStyleSrc    = "style-src"
	cspDirectiveObjectSrc   = "object-src"
	cspDirectiveManifestSrc = "manifest-src"
	cspDirectiveFrameSrc    = "frame-src"
	cspDirectiveImgSrc      = "img-src"
	cspDirectiveReportURI   = "report-uri"
	cspDirectiveFormAction  = "form-action"
	cspDirectiveMediaSrc    = "media-src"
	cspFrameAncestors       = "frame-ancestors"
)

// secureHeaders is only needed for statically served files. We do not need this for api endpoints.
// It adds various headers to enforce browser security features.
func secureHeaders(next http.Handler) http.Handler {
	// Content-Security-Policy disables loading certain content types and can prevent XSS injections.
	// This site helps eval your policy for syntax and other common issues: https://csp-evaluator.withgoogle.com/
	// If we ever want to render something like a PDF, we need to adjust "object-src"
	//
	//	The list of CSP options: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Security-Policy/default-src
	cspSrcs := cspDirectives{
		// All omitted fetch csp srcs default to this.
		cspDirectiveDefaultSrc: {"'self'"},
		cspDirectiveConnectSrc: {"'self' ws: wss:"},
		cspDirectiveChildSrc:   {"'self'"},
		cspDirectiveScriptSrc:  {"'self'"},
		cspDirectiveFontSrc:    {"'self'"},
		cspDirectiveStyleSrc:   {"'self' 'unsafe-inline'"},
		// object-src is needed to support code-server
		cspDirectiveObjectSrc: {"'self'"},
		// blob: for loading the pwa manifest for code-server
		cspDirectiveManifestSrc: {"'self' blob:"},
		cspDirectiveFrameSrc:    {"'self'"},
		// data: for loading base64 encoded icons for generic applications.
		cspDirectiveImgSrc:     {"'self' https://cdn.coder.com data:"},
		cspDirectiveFormAction: {"'self'"},
		cspDirectiveMediaSrc:   {"'self'"},
		// Report all violations back to the server to log
		cspDirectiveReportURI: {"/api/private/csp/reports"},
		cspFrameAncestors:     {"'none'"},

		// Only scripts can manipulate the dom. This prevents someone from
		// naming themselves something like '<svg onload="alert(/cross-site-scripting/)" />'.
		// TODO: @emyrk we need to make FE changes to enable this. We get 'TrustedHTML' and 'TrustedURL' errors
		//		that require FE changes to work.
		// "require-trusted-types-for" : []string{"'script'"},
	}

	var csp strings.Builder
	for src, vals := range cspSrcs {
		_, _ = fmt.Fprintf(&csp, "%s %s; ", src, strings.Join(vals, " "))
	}

	// Permissions-Policy can be used to disabled various browser features that we do not use.
	// This can prevent an embedded iframe from accessing these features.
	// If we support arbitrary iframes such as generic applications, we might need to add permissions
	// based on the app here.
	permissions := strings.Join([]string{
		// =() means it is disabled
		"accelerometer=()",
		"autoplay=()",
		"battery=()",
		"camera=()",
		"document-domain=()",
		"geolocation=()",
		"gyroscope=()",
		"magnetometer=()",
		"microphone=()",
		"midi=()",
		"payment=()",
		"usb=()",
		"vr=()",
		"screen-wake-lock=()",
		"xr-spatial-tracking=()",
	}, ", ")

	return secure.New(secure.Options{
		// Set to ContentSecurityPolicyReportOnly for testing, as all errors are printed to the console log
		// but are not enforced.
		ContentSecurityPolicy: csp.String(),

		PermissionsPolicy: permissions,

		// Prevent the browser from sending Referer header with requests
		ReferrerPolicy: "no-referrer",
	}).Handler(next)
}
