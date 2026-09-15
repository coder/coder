package workspaceapps

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// MCPAppSandboxHostPrefix is the prefix of the wildcard app subdomain
	// reserved for serving the MCP app sandbox proxy document. The full
	// subdomain is this prefix followed by 32 lowercase hex characters.
	MCPAppSandboxHostPrefix = "mcpapp-"

	// mcpAppSandboxMaxDomainsPerList caps the number of entries accepted in
	// each list of the csp query parameter.
	mcpAppSandboxMaxDomainsPerList = 32
	// mcpAppSandboxMaxCSPHeaderBytes caps the serialized length of the
	// Content-Security-Policy header sent with the sandbox document.
	mcpAppSandboxMaxCSPHeaderBytes = 8 * 1024
)

var (
	// MCPAppSandboxHostRegex matches a wildcard app subdomain reserved for the
	// MCP app sandbox proxy document.
	MCPAppSandboxHostRegex = regexp.MustCompile("^" + MCPAppSandboxHostPrefix + "[0-9a-f]{32}$")

	// mcpAppSandboxDomainCharsRegex is the character set permitted in a raw
	// CSP domain entry. Excluding whitespace, semicolons and quotes keeps a
	// single entry from spanning multiple CSP sources or directives.
	mcpAppSandboxDomainCharsRegex = regexp.MustCompile(`^[A-Za-z0-9.:/*-]+$`)
	// mcpAppSandboxHostLabelRegex is the character set permitted in a
	// lowercased hostname label of a CSP domain entry.
	mcpAppSandboxHostLabelRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

	// mcpAppSandboxParentDomainWarnOnce limits the misconfiguration warning
	// for a wildcard suffix that is a parent domain of the dashboard host to a
	// single log line per process.
	mcpAppSandboxParentDomainWarnOnce sync.Once
)

//go:embed mcpappsandbox.html
var mcpAppSandboxHTML string

// mcpAppSandboxTemplate renders the sandbox proxy document with the dashboard
// origin injected into a meta tag.
var mcpAppSandboxTemplate = template.Must(template.New("mcpappsandbox.html").Parse(mcpAppSandboxHTML))

// mcpAppSandboxTemplateData is the data passed to mcpAppSandboxTemplate.
type mcpAppSandboxTemplateData struct {
	// HostOrigin is the origin (scheme://host[:port]) of the page that embeds
	// the sandbox proxy iframe.
	HostOrigin string
}

// mcpAppSandboxCSP is the CSP metadata declared by an MCP app resource, passed
// to the sandbox document as URL-encoded JSON in the csp query parameter.
type mcpAppSandboxCSP struct {
	ConnectDomains  []string `json:"connectDomains"`
	ResourceDomains []string `json:"resourceDomains"`
	FrameDomains    []string `json:"frameDomains"`
	BaseURIDomains  []string `json:"baseUriDomains"`
}

// mcpAppSandboxWildcardSuffix returns the lowercased part of the wildcard app
// hostname pattern that follows the wildcard label, e.g. "apps.example.com"
// for "*.apps.example.com". Any port in the pattern is dropped.
func mcpAppSandboxWildcardSuffix(hostnamePattern string) string {
	suffix := strings.ToLower(hostnamePattern)
	if host, _, err := net.SplitHostPort(suffix); err == nil {
		suffix = host
	}
	suffix = strings.TrimPrefix(suffix, "*")
	suffix = strings.TrimPrefix(suffix, ".")
	return suffix
}

// urlOrigin returns the scheme://host[:port] form of u without any path,
// query or fragment.
func urlOrigin(u *url.URL) string {
	return u.Scheme + "://" + u.Host
}

// serveMCPAppSandbox serves the MCP app sandbox proxy document on a reserved
// wildcard app subdomain. Only GET and HEAD requests for the root path are
// served so that nothing else on the sandbox origin is fetchable by the
// sandboxed content.
//
// The document is refused when the wildcard suffix is a strict parent domain
// of the dashboard host, i.e. the dashboard host ends with "." followed by
// the suffix ("*.example.com" with "coder.example.com"). A dashboard host
// equal to the suffix ("*.coder.example.com" with "coder.example.com") is
// served.
func (s *Server) serveMCPAppSandbox(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Not found.",
		})
		return
	}
	if r.URL.Path != "/" {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "Not found.",
		})
		return
	}

	dashboardHost := strings.ToLower(s.DashboardURL.Hostname())
	wildcardSuffix := mcpAppSandboxWildcardSuffix(s.Hostname)
	if strings.HasSuffix(dashboardHost, "."+wildcardSuffix) {
		mcpAppSandboxParentDomainWarnOnce.Do(func() {
			s.Logger.Warn(ctx, "refusing to serve the MCP app sandbox: the wildcard access URL is a strict parent domain of the access URL",
				slog.F("wildcard_access_url", s.Hostname),
				slog.F("access_url_host", dashboardHost),
			)
		})
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "MCP apps cannot be rendered on this deployment.",
			Detail: fmt.Sprintf("The wildcard access URL %q must not be a parent domain of the access URL %q for MCP apps to be rendered.",
				s.Hostname, dashboardHost),
		})
		return
	}

	var csp mcpAppSandboxCSP
	if raw := r.URL.Query().Get("csp"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &csp); err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid csp query parameter.",
				Detail:  err.Error(),
			})
			return
		}
	}
	if err := s.validateMCPAppSandboxCSP(csp, dashboardHost, wildcardSuffix); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid csp query parameter.",
			Detail:  err.Error(),
		})
		return
	}

	hostOrigin := urlOrigin(s.DashboardURL)
	cspHeader := buildMCPAppSandboxCSP(csp, hostOrigin)
	if len(cspHeader) > mcpAppSandboxMaxCSPHeaderBytes {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid csp query parameter.",
			Detail:  fmt.Sprintf("The resulting Content-Security-Policy header exceeds %d bytes.", mcpAppSandboxMaxCSPHeaderBytes),
		})
		return
	}

	var body bytes.Buffer
	err := mcpAppSandboxTemplate.Execute(&body, mcpAppSandboxTemplateData{
		HostOrigin: hostOrigin,
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("render MCP app sandbox document: %w", err))
		return
	}

	header := rw.Header()
	header.Set("Content-Security-Policy", cspHeader)
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Content-Length", strconv.Itoa(body.Len()))
	rw.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = rw.Write(body.Bytes())
}

// validateMCPAppSandboxCSP validates every domain list in csp. Each list is
// capped at mcpAppSandboxMaxDomainsPerList entries and every entry must pass
// validateCSPDomain and validateCSPDomainWildcardPattern.
func (s *Server) validateMCPAppSandboxCSP(csp mcpAppSandboxCSP, dashboardHost, wildcardSuffix string) error {
	lists := []struct {
		name    string
		entries []string
	}{
		{name: "connectDomains", entries: csp.ConnectDomains},
		{name: "resourceDomains", entries: csp.ResourceDomains},
		{name: "frameDomains", entries: csp.FrameDomains},
		{name: "baseUriDomains", entries: csp.BaseURIDomains},
	}
	for _, list := range lists {
		if len(list.entries) > mcpAppSandboxMaxDomainsPerList {
			return xerrors.Errorf("%s has %d entries, the maximum is %d", list.name, len(list.entries), mcpAppSandboxMaxDomainsPerList)
		}
		for _, entry := range list.entries {
			if err := validateCSPDomain(entry, dashboardHost, wildcardSuffix); err != nil {
				return xerrors.Errorf("%s entry %q: %w", list.name, entry, err)
			}
			if err := validateCSPDomainWildcardPattern(entry, s.HostnameRegex, s.Hostname); err != nil {
				return xerrors.Errorf("%s entry %q: %w", list.name, entry, err)
			}
		}
	}
	return nil
}

// validateCSPDomainWildcardPattern rejects an entry whose host matches the
// wildcard app hostname pattern, or whose leading "*." wildcard covers hosts
// matching it. Patterns of the form "*-suffix.example.com" place app hosts
// directly under "example.com", so this check also covers the labels after
// the wildcard label. entry must already have passed validateCSPDomain.
func validateCSPDomainWildcardPattern(entry string, hostnameRegex *regexp.Regexp, hostnamePattern string) error {
	if hostnameRegex == nil || hostnamePattern == "" {
		return nil
	}
	u, err := url.Parse(entry)
	if err != nil {
		return xerrors.Errorf("entry is not a valid URL: %w", err)
	}
	hostname := strings.ToLower(u.Hostname())
	wildcard := strings.HasPrefix(hostname, "*.")
	bareHost := strings.TrimPrefix(hostname, "*.")

	if _, ok := appurl.ExecuteHostnamePattern(hostnameRegex, bareHost); ok {
		return xerrors.New("host matches the wildcard access URL")
	}
	if !wildcard {
		return nil
	}
	// The labels after the wildcard label, e.g. "example.com" for
	// "*-apps.example.com" or "apps.example.com" for "*.apps.example.com".
	pattern := strings.ToLower(hostnamePattern)
	if host, _, err := net.SplitHostPort(pattern); err == nil {
		pattern = host
	}
	_, parentDomain, _ := strings.Cut(pattern, ".")
	if bareHost == parentDomain || strings.HasSuffix(parentDomain, "."+bareHost) {
		return xerrors.New("wildcard covers the wildcard access URL")
	}
	return nil
}

// validateCSPDomain checks that entry is an https or wss origin suitable for
// inclusion as a single CSP source expression. The entry must not point at an
// IP address, localhost, the dashboard host, or any host under the wildcard
// app suffix. dashboardHost and wildcardSuffix must be lowercase.
func validateCSPDomain(entry, dashboardHost, wildcardSuffix string) error {
	if entry == "" {
		return xerrors.New("entry is empty")
	}
	if !mcpAppSandboxDomainCharsRegex.MatchString(entry) {
		return xerrors.New("entry contains characters outside of [A-Za-z0-9.:/*-]")
	}
	if !strings.Contains(entry, "://") {
		return xerrors.New("entry must include an https:// or wss:// scheme")
	}

	u, err := url.Parse(entry)
	if err != nil {
		return xerrors.Errorf("entry is not a valid URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "wss":
	default:
		return xerrors.Errorf("scheme %q is not https or wss", u.Scheme)
	}
	if u.Opaque != "" {
		return xerrors.New("entry must be an origin")
	}
	if u.User != nil {
		return xerrors.New("entry must not contain user info")
	}
	if u.Path != "" && u.Path != "/" {
		return xerrors.New("entry must not contain a path")
	}
	if u.RawQuery != "" || u.ForceQuery {
		return xerrors.New("entry must not contain a query")
	}
	if u.Fragment != "" || u.RawFragment != "" {
		return xerrors.New("entry must not contain a fragment")
	}
	if u.Host == "" {
		return xerrors.New("entry must contain a host")
	}
	if port := u.Port(); port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return xerrors.Errorf("port %q is invalid", port)
		}
	}

	hostname := strings.ToLower(u.Hostname())
	if hostname == "" {
		return xerrors.New("entry must contain a host")
	}
	if net.ParseIP(hostname) != nil {
		return xerrors.New("IP address literals are not allowed")
	}

	// A single leading wildcard label is the only permitted use of "*".
	wildcard := strings.HasPrefix(hostname, "*.")
	bareHost := strings.TrimPrefix(hostname, "*.")
	if bareHost == "" {
		return xerrors.New("wildcard entries must include a domain after \"*.\"")
	}
	if strings.Contains(bareHost, "*") {
		return xerrors.New("only a single leading \"*.\" wildcard label is allowed")
	}
	for _, label := range strings.Split(bareHost, ".") {
		if !mcpAppSandboxHostLabelRegex.MatchString(label) {
			return xerrors.Errorf("hostname label %q is invalid", label)
		}
	}

	if bareHost == "localhost" || strings.HasSuffix(bareHost, ".localhost") {
		return xerrors.New("localhost is not allowed")
	}

	if bareHost == dashboardHost {
		return xerrors.New("the access URL host is not allowed")
	}
	if wildcard && strings.HasSuffix(dashboardHost, "."+bareHost) {
		return xerrors.New("wildcard covers the access URL host")
	}
	if bareHost == wildcardSuffix || strings.HasSuffix(bareHost, "."+wildcardSuffix) {
		return xerrors.New("hosts under the wildcard access URL are not allowed")
	}
	if wildcard && strings.HasSuffix(wildcardSuffix, "."+bareHost) {
		return xerrors.New("wildcard covers the wildcard access URL")
	}

	return nil
}

// buildMCPAppSandboxCSP builds the Content-Security-Policy header value for
// the sandbox proxy document. Every domain in csp must already be validated.
// frameAncestor is the only origin permitted to embed the document.
func buildMCPAppSandboxCSP(csp mcpAppSandboxCSP, frameAncestor string) string {
	withDomains := func(directive string, domains []string) string {
		if len(domains) == 0 {
			return directive
		}
		return directive + " " + strings.Join(domains, " ")
	}
	orKeyword := func(directive string, domains []string, keyword string) string {
		if len(domains) == 0 {
			return directive + " " + keyword
		}
		return directive + " " + strings.Join(domains, " ")
	}

	directives := []string{
		"default-src 'none'",
		withDomains("script-src 'self' 'unsafe-inline'", csp.ResourceDomains),
		withDomains("style-src 'self' 'unsafe-inline'", csp.ResourceDomains),
		withDomains("connect-src 'self'", csp.ConnectDomains),
		withDomains("img-src 'self' data:", csp.ResourceDomains),
		withDomains("font-src 'self'", csp.ResourceDomains),
		withDomains("media-src 'self' data:", csp.ResourceDomains),
		orKeyword("frame-src", csp.FrameDomains, "'none'"),
		"object-src 'none'",
		orKeyword("base-uri", csp.BaseURIDomains, "'self'"),
		"form-action 'none'",
		"frame-ancestors " + frameAncestor,
	}
	return strings.Join(directives, "; ")
}
