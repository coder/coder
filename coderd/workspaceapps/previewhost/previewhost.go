// Package previewhost decides how a Coder server hosts agent-authored
// previews: which hostnames serve them and what every response on those
// hostnames carries.
package previewhost

import (
	"bufio"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// SignInParam carries the encrypted sign-in payload that coderd mints
	// for one preview host.
	SignInParam = "coder_preview_sign_in"

	maxHostnameLabelLength = 63
	maxURLPathLength       = 2048
	strictSandbox          = "allow-scripts allow-same-origin allow-forms allow-modals allow-downloads"
)

var (
	defaultPorts = map[string]string{"http": "80", "https": "443"}
	// reservedQueryParams are read by the preview host itself, which refuses
	// or strips them before the preview sees the query.
	reservedQueryParams = []string{SignInParam, appurl.SubdomainProxyAPIKeyParam}
)

// CheckURLPath returns an error unless urlPath is empty or a path and query
// that a preview opens at: one leading slash, no backslash, fragment or
// control character, at most 2048 bytes, a path with valid percent-encoding,
// and none of the query parameters the preview host reads itself.
func CheckURLPath(urlPath string) error {
	if urlPath == "" {
		return nil
	}
	if len(urlPath) > maxURLPathLength ||
		!strings.HasPrefix(urlPath, "/") ||
		strings.HasPrefix(urlPath, "//") ||
		strings.ContainsAny(urlPath, `\#`) ||
		strings.ContainsFunc(urlPath, unicode.IsControl) {
		return xerrors.Errorf("url_path must start with a single /, contain only a path and query, have no backslashes, fragments or control characters, and be at most %d bytes", maxURLPathLength)
	}
	ref, err := url.Parse(urlPath)
	if err != nil {
		return xerrors.Errorf("url_path must use valid percent-encoding: %w", err)
	}
	for _, key := range reservedQueryParams {
		if ref.Query().Has(key) {
			return xerrors.Errorf("url_path must not contain the %s query parameter, which Coder reads on the preview host", key)
		}
	}
	return nil
}

// ErrHostPrefixCookieOff is why a deployment without --host-prefix-cookie
// serves no previews in any region. A preview page can set cookies for the
// dashboard's whole site, and only the __Host- prefix keeps such a cookie
// from replacing the Coder session cookie.
var ErrHostPrefixCookieOff = xerrors.New("previews require --host-prefix-cookie")

// CheckCookieConfig returns ErrHostPrefixCookieOff unless cfg gives the
// session cookie the __Host- prefix.
func CheckCookieConfig(cfg codersdk.HTTPCookieConfig) error {
	if !cfg.EnableHostPrefix {
		return ErrHostPrefixCookieOff
	}
	return nil
}

// Hosting is where one server, coderd or a workspace proxy, serves previews.
type Hosting struct {
	pattern *regexp.Regexp
	// host is the wildcard host as browsers send it: with the serving port
	// unless it is the scheme's default, such as "*.apps.example.com:8443".
	host   string
	scheme string
	// csp is added to every response on a preview host.
	csp string
	// sandbox is the iframe sandbox the dashboard frames a preview with.
	sandbox string
}

// Strict returns the hosting of a server that serves strict previews under
// its app wildcard hostname, such as "*.apps.example.com", at the scheme and
// port of accessURL. dashboardURL is the only other origin that may frame a
// preview.
func Strict(wildcardHostname string, accessURL, dashboardURL *url.URL) (*Hosting, error) {
	pattern, err := appurl.CompileHostnamePattern(wildcardHostname)
	if err != nil {
		return nil, xerrors.Errorf("compile preview hostname pattern: %w", err)
	}
	dashboardOrigin := (&url.URL{Scheme: dashboardURL.Scheme, Host: dashboardURL.Host}).String()
	// A sign-in token holds the host exactly as the browser sends it, and
	// browsers leave the scheme's default port out of the Host header.
	host := appurl.SubdomainAppHost(wildcardHostname, accessURL)
	if hostname, port, err := net.SplitHostPort(host); err == nil && port == defaultPorts[accessURL.Scheme] {
		host = hostname
	}
	// Every request a page makes must stay on its own host, because requests
	// to the user's other apps carry the user's app cookie. CSP cannot stop a
	// page from navigating its own frame, so the dashboard wraps the frame in
	// a page whose frame-src allows only the preview.
	return &Hosting{
		pattern: pattern,
		host:    host,
		scheme:  accessURL.Scheme,
		csp: "sandbox " + strictSandbox +
			"; default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; frame-src 'self'" +
			"; object-src 'none'; form-action 'self'; frame-ancestors 'self' " + dashboardOrigin,
		sandbox: strictSandbox,
	}, nil
}

// Sandbox returns the iframe sandbox the dashboard frames h's previews with.
func (h *Hosting) Sandbox() string {
	return h.sandbox
}

// SessionCookieName returns the name of the cookie that signs the owner in to
// one preview host of h. Over HTTPS it carries the __Host- prefix, so browsers
// keep it Secure and host-only and refuse a cookie of that name set from a
// parent domain, such as by another preview.
func (h *Hosting) SessionCookieName() string {
	if h.scheme == "https" {
		return "__Host-" + codersdk.PreviewSessionTokenCookie
	}
	return codersdk.PreviewSessionTokenCookie
}

// SessionCookie returns the cookie that signs the owner in to one preview host
// of h with apiKey. It has no Domain, so it stays on that host. Over plain
// HTTP, where browsers reject Secure from most hosts, Secure follows cfg.
func (h *Hosting) SessionCookie(apiKey string, cfg codersdk.HTTPCookieConfig) *http.Cookie {
	cookie := cfg.Apply(&http.Cookie{
		Name:     h.SessionCookieName(),
		Value:    apiKey,
		Path:     "/",
		HttpOnly: true,
	})
	cookie.Secure = cookie.Secure || h.scheme == "https"
	return cookie
}

// Covers reports whether host is a preview host of h. That includes a
// prefixed preview host, which Parse refuses, so its error page carries the
// policy too.
func (h *Hosting) Covers(host string) bool {
	_, _, ok := appurl.ParsePreviewHostname(h.pattern, host)
	return ok
}

// Parse returns the app and target that a preview host of h names.
func (h *Hosting) Parse(host string) (appurl.ApplicationURL, appurl.PreviewTarget, error) {
	app, target, ok := appurl.ParsePreviewHostname(h.pattern, host)
	if !ok || app.Prefix != "" {
		return appurl.ApplicationURL{}, appurl.PreviewTarget{}, xerrors.Errorf("%q is not a preview hostname", host)
	}
	return app, target, nil
}

// URL returns the URL of app's preview at urlPath, a path and query such as
// "/docs/?page=2". It errors when urlPath fails CheckURLPath, when app's
// hostname would not parse back to app, such as when a name holds "--", "."
// or ":", or when the hostname label is longer than the DNS limit of 63
// characters.
func (h *Hosting) URL(app appurl.ApplicationURL, urlPath string) (*url.URL, error) {
	subdomain := app.String()
	parsed, err := appurl.ParseSubdomainAppURL(subdomain)
	if _, isPreview := appurl.ParsePreviewSlug(app.AppSlugOrPort); err != nil || parsed != app || !isPreview || app.Prefix != "" {
		return nil, xerrors.Errorf("%q is not a valid preview hostname", subdomain)
	}
	pattern, _, _ := strings.Cut(h.host, ".")
	label := strings.Replace(pattern, "*", subdomain, 1)
	if len(label) > maxHostnameLabelLength {
		return nil, xerrors.Errorf("the hostname label %q is %d characters, over the DNS limit of %d", label, len(label), maxHostnameLabelLength)
	}
	if err := CheckURLPath(urlPath); err != nil {
		return nil, err
	}
	ref, err := url.Parse(urlPath)
	if err != nil {
		return nil, xerrors.Errorf("parse URL path: %w", err)
	}
	u := &url.URL{
		Scheme:   h.scheme,
		Host:     strings.Replace(h.host, "*", subdomain, 1),
		Path:     ref.Path,
		RawPath:  ref.RawPath,
		RawQuery: ref.RawQuery,
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u, nil
}

// ApplyPolicy applies h's policy to the header of a response on a preview
// host. Applying it again changes nothing, so the CSP appears once.
func (h *Hosting) ApplyPolicy(header http.Header) {
	if !slices.Contains(header.Values("Content-Security-Policy"), h.csp) {
		header.Add("Content-Security-Policy", h.csp)
	}
	header.Del("Link")
	// Clear-Site-Data clears cookies for the whole registrable domain.
	header.Del("Clear-Site-Data")
	// A cookie with a Domain attribute is sent to the user's other apps. This
	// strips the ones the preview's server sets, including HttpOnly cookies
	// that page script cannot create. Page script can still set Domain
	// cookies, which the experiment accepts.
	cookies := header.Values("Set-Cookie")
	header.Del("Set-Cookie")
	for _, line := range cookies {
		if !hasDomainAttribute(line) {
			header.Add("Set-Cookie", line)
		}
	}
}

// Wrap returns rw with h's policy applied to every response written through
// it. Go's ReverseProxy clears the header map after forwarding each 1xx
// response, so a header set before proxying can be lost; the policy is
// applied when the final status is written. A proxied WebSocket upgrade is
// written to the hijacked connection instead, so the proxy applies the
// policy to it with ApplyPolicy.
func (h *Hosting) Wrap(rw http.ResponseWriter) http.ResponseWriter {
	return &responseWriter{ResponseWriter: rw, hosting: h}
}

type responseWriter struct {
	http.ResponseWriter
	hosting     *Hosting
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(code int) {
	// A 103 Early Hints response can make the browser preload from other
	// hosts before the final response's policy exists.
	if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
		return
	}
	if !w.wroteHeader {
		w.wroteHeader = true
		w.hosting.ApplyPolicy(w.Header())
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Flush applies the policy before a flush can send the header.
func (w *responseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// Hijack hands over the connection for WebSocket upgrades. A response
// written to it bypasses WriteHeader, so its writer applies the policy.
func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// hasDomainAttribute reports whether a browser could read a Domain attribute
// in the Set-Cookie line.
func hasDomainAttribute(line string) bool {
	_, attrs, _ := strings.Cut(line, ";")
	for attr := range strings.SplitSeq(attrs, ";") {
		name, _, _ := strings.Cut(attr, "=")
		if strings.EqualFold(strings.TrimSpace(name), "domain") {
			return true
		}
	}
	return false
}
