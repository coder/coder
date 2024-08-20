package httpmw

import (
	"fmt"
	"net/http"
	"strings"
)

// cspDirectives is a map of all csp fetch directives to their values.
// Each directive is a set of values that is joined by a space (' ').
// All directives are semi-colon separated as a single string for the csp header.
type cspDirectives map[CSPFetchDirective][]string

func (s cspDirectives) Append(d CSPFetchDirective, values ...string) {
	if _, ok := s[d]; !ok {
		s[d] = make([]string, 0)
	}
	s[d] = append(s[d], values...)
}

// CSPFetchDirective is the list of all constant fetch directives that
// can be used/appended to.
type CSPFetchDirective string

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
	cspDirectiveWorkerSrc   = "worker-src"
)

// CSPHeaders returns a middleware that sets the Content-Security-Policy header
// for coderd. It takes a function that allows adding supported external websocket
// hosts. This is primarily to support the terminal connecting to a workspace proxy.
//
//nolint:revive
func CSPHeaders(telemetry bool, websocketHosts func() []string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Content-Security-Policy disables loading certain content types and can prevent XSS injections.
			// This site helps eval your policy for syntax and other common issues: https://csp-evaluator.withgoogle.com/
			// If we ever want to render something like a PDF, we need to adjust "object-src"
			//
			//	The list of CSP options: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Security-Policy/default-src
			cspSrcs := cspDirectives{
				// All omitted fetch csp srcs default to this.
				cspDirectiveDefaultSrc: {"'self'"},
				cspDirectiveConnectSrc: {"'self'"},
				cspDirectiveChildSrc:   {"'self'"},
				// https://github.com/suren-atoyan/monaco-react/issues/168
				cspDirectiveScriptSrc: {"'self'"},
				cspDirectiveStyleSrc:  {"'self' 'unsafe-inline'"},
				// data: is used by monaco editor on FE for Syntax Highlight
				cspDirectiveFontSrc:   {"'self' data:"},
				cspDirectiveWorkerSrc: {"'self' blob:"},
				// object-src is needed to support code-server
				cspDirectiveObjectSrc: {"'self'"},
				// blob: for loading the pwa manifest for code-server
				cspDirectiveManifestSrc: {"'self' blob:"},
				cspDirectiveFrameSrc:    {"'self'"},
				// data: for loading base64 encoded icons for generic applications.
				// https: allows loading images from external sources. This is not ideal
				// 	but is required for the templates page that renders readmes.
				//	We should find a better solution in the future.
				cspDirectiveImgSrc:     {"'self' https: data:"},
				cspDirectiveFormAction: {"'self'"},
				cspDirectiveMediaSrc:   {"'self'"},
				// Report all violations back to the server to log
				cspDirectiveReportURI: {"/api/v2/csp/reports"},
				cspFrameAncestors:     {"'none'"},

				// Only scripts can manipulate the dom. This prevents someone from
				// naming themselves something like '<svg onload="alert(/cross-site-scripting/)" />'.
				// "require-trusted-types-for" : []string{"'script'"},
			}

			if telemetry {
				// If telemetry is enabled, we report to coder.com.
				cspSrcs.Append(cspDirectiveConnectSrc, "https://coder.com")
			}

			// This extra connect-src addition is required to support old webkit
			// based browsers (Safari).
			// See issue: https://github.com/w3c/webappsec-csp/issues/7
			// Once webkit browsers support 'self' on connect-src, we can remove this.
			// When we remove this, the csp header can be static, as opposed to being
			// dynamically generated for each request.
			host := r.Host
			// It is important r.Host is not an empty string.
			if host != "" {
				// We can add both ws:// and wss:// as browsers do not let https
				// pages to connect to non-tls websocket connections. So this
				// supports both http & https webpages.
				cspSrcs.Append(cspDirectiveConnectSrc, fmt.Sprintf("wss://%[1]s ws://%[1]s", host))
			}

			// The terminal requires a websocket connection to the workspace proxy.
			// Make sure we allow this connection to healthy proxies.
			extraConnect := websocketHosts()
			if len(extraConnect) > 0 {
				for _, extraHost := range extraConnect {
					if extraHost == "*" {
						// '*' means all
						cspSrcs.Append(cspDirectiveConnectSrc, "*")
						continue
					}
					cspSrcs.Append(cspDirectiveConnectSrc, fmt.Sprintf("wss://%[1]s ws://%[1]s", extraHost))
					// We also require this to make http/https requests to the workspace proxy for latency checking.
					cspSrcs.Append(cspDirectiveConnectSrc, fmt.Sprintf("https://%[1]s http://%[1]s", extraHost))
				}
			}

			var csp strings.Builder
			for src, vals := range cspSrcs {
				_, _ = fmt.Fprintf(&csp, "%s %s; ", src, strings.Join(vals, " "))
			}

			w.Header().Set("Content-Security-Policy", csp.String())
			next.ServeHTTP(w, r)
		})
	}
}
