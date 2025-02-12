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
	CSPDirectiveDefaultSrc  CSPFetchDirective = "default-src"
	CSPDirectiveConnectSrc  CSPFetchDirective = "connect-src"
	CSPDirectiveChildSrc    CSPFetchDirective = "child-src"
	CSPDirectiveScriptSrc   CSPFetchDirective = "script-src"
	CSPDirectiveFontSrc     CSPFetchDirective = "font-src"
	CSPDirectiveStyleSrc    CSPFetchDirective = "style-src"
	CSPDirectiveObjectSrc   CSPFetchDirective = "object-src"
	CSPDirectiveManifestSrc CSPFetchDirective = "manifest-src"
	CSPDirectiveFrameSrc    CSPFetchDirective = "frame-src"
	CSPDirectiveImgSrc      CSPFetchDirective = "img-src"
	CSPDirectiveReportURI   CSPFetchDirective = "report-uri"
	CSPDirectiveFormAction  CSPFetchDirective = "form-action"
	CSPDirectiveMediaSrc    CSPFetchDirective = "media-src"
	CSPFrameAncestors       CSPFetchDirective = "frame-ancestors"
	CSPDirectiveWorkerSrc   CSPFetchDirective = "worker-src"
)

// CSPHeaders returns a middleware that sets the Content-Security-Policy header
// for coderd.
//
// Arguments:
//   - websocketHosts: a function that returns a list of supported external websocket hosts.
//     This is to support the terminal connecting to a workspace proxy.
//     The origin of the terminal request does not match the url of the proxy,
//     so the CSP list of allowed hosts must be dynamic and match the current
//     available proxy urls.
//   - staticAdditions: a map of CSP directives to append to the default CSP headers.
//     Used to allow specific static additions to the CSP headers. Allows some niche
//     use cases, such as embedding Coder in an iframe.
//     Example: https://github.com/coder/coder/issues/15118
//
//nolint:revive
func CSPHeaders(telemetry bool, websocketHosts func() []string, staticAdditions map[CSPFetchDirective][]string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Content-Security-Policy disables loading certain content types and can prevent XSS injections.
			// This site helps eval your policy for syntax and other common issues: https://csp-evaluator.withgoogle.com/
			// If we ever want to render something like a PDF, we need to adjust "object-src"
			//
			//	The list of CSP options: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Security-Policy/default-src
			cspSrcs := cspDirectives{
				// All omitted fetch csp srcs default to this.
				CSPDirectiveDefaultSrc: {"'self'"},
				CSPDirectiveConnectSrc: {"'self'"},
				CSPDirectiveChildSrc:   {"'self'"},
				// https://github.com/suren-atoyan/monaco-react/issues/168
				CSPDirectiveScriptSrc: {"'self'"},
				CSPDirectiveStyleSrc:  {"'self' 'unsafe-inline'"},
				// data: is used by monaco editor on FE for Syntax Highlight
				CSPDirectiveFontSrc:   {"'self' data:"},
				CSPDirectiveWorkerSrc: {"'self' blob:"},
				// object-src is needed to support code-server
				CSPDirectiveObjectSrc: {"'self'"},
				// blob: for loading the pwa manifest for code-server
				CSPDirectiveManifestSrc: {"'self' blob:"},
				CSPDirectiveFrameSrc:    {"'self'"},
				// data: for loading base64 encoded icons for generic applications.
				// https: allows loading images from external sources. This is not ideal
				// 	but is required for the templates page that renders readmes.
				//	We should find a better solution in the future.
				CSPDirectiveImgSrc:     {"'self' https: data:"},
				CSPDirectiveFormAction: {"'self'"},
				CSPDirectiveMediaSrc:   {"'self'"},
				// Report all violations back to the server to log
				CSPDirectiveReportURI: {"/api/v2/csp/reports"},
				CSPFrameAncestors:     {"'none'"},

				// Only scripts can manipulate the dom. This prevents someone from
				// naming themselves something like '<svg onload="alert(/cross-site-scripting/)" />'.
				// "require-trusted-types-for" : []string{"'script'"},
			}

			if telemetry {
				// If telemetry is enabled, we report to coder.com.
				cspSrcs.Append(CSPDirectiveConnectSrc, "https://coder.com")
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
				cspSrcs.Append(CSPDirectiveConnectSrc, fmt.Sprintf("wss://%[1]s ws://%[1]s", host))
			}

			// The terminal requires a websocket connection to the workspace proxy.
			// Make sure we allow this connection to healthy proxies.
			extraConnect := websocketHosts()
			if len(extraConnect) > 0 {
				for _, extraHost := range extraConnect {
					if extraHost == "*" {
						// '*' means all
						cspSrcs.Append(CSPDirectiveConnectSrc, "*")
						continue
					}
					cspSrcs.Append(CSPDirectiveConnectSrc, fmt.Sprintf("wss://%[1]s ws://%[1]s", extraHost))
					// We also require this to make http/https requests to the workspace proxy for latency checking.
					cspSrcs.Append(CSPDirectiveConnectSrc, fmt.Sprintf("https://%[1]s http://%[1]s", extraHost))
				}
			}

			for directive, values := range staticAdditions {
				cspSrcs.Append(directive, values...)
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
