package appurl

import (
	"fmt"
	"hash/fnv"
	"math/big"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const (
	// maxLabelLength is the DNS limit for one hostname label (RFC 1035).
	maxLabelLength = 63
	// longestPort is the longest value a port takes in a hostname.
	longestPort = "65535s"
	// workspaceIDLength is the length of a base36 workspace UUID.
	workspaceIDLength = 25
	// agentNameHashLength is the length of a base36 32-bit agent name hash.
	agentNameHashLength = 7
	// portPlaceholder is a port template that callers replace with a port.
	portPlaceholder = "{{port}}"
)

var (
	// nameRegex is the same as our UsernameRegex without the ^ and $.
	nameRegex = "[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*"
	// Supports apps with and without agent name
	// Format: {PORT/APP_SLUG}[--{AGENT_NAME}]--{WORKSPACE_NAME}--{USERNAME}
	appURL = regexp.MustCompile(fmt.Sprintf(
		`^(?P<AppSlug>%[1]s)(?:--(?P<AgentName>%[1]s))?--(?P<WorkspaceName>%[1]s)--(?P<Username>%[1]s)$`,
		nameRegex))
	// PortRegex should not be able to be greater than 65535. In usage though, if a
	// user tries to use a greater port, the proxy will just block it and not cause
	// any issues. This is a good enough regex check.
	PortRegex = regexp.MustCompile(`^\d{4,5}s?$`)

	validHostnameLabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

	// anyPortRegex matches every port, also ports below 1000. The short form
	// and New use it, so they agree with the frontend. PortRegex keeps the
	// long form parser behavior.
	anyPortRegex = regexp.MustCompile(`^\d{1,5}s?$`)

	// shortAppURL matches the short form, which identifies the workspace by
	// ID instead of by owner and name. It never matches a long form URL: a
	// long app URL has at least three segments, and a long port URL has four.
	// Format: {APP_SLUG}--{WORKSPACE_ID} or {PORT}--{WORKSPACE_ID}--{AGENT_HASH}
	shortAppURL = regexp.MustCompile(fmt.Sprintf(
		`^(?P<AppSlugOrPort>%[1]s)--(?P<WorkspaceID>[0-9a-zA-Z]{%[2]d})(?:--(?P<AgentNameHash>[0-9a-zA-Z]{%[3]d}))?$`,
		nameRegex, workspaceIDLength, agentNameHashLength))
)

// SubdomainAppHost returns the URL of the apphost for subdomain based apps.
// It will omit the scheme.
//
// Arguments:
// apphost: Expected to contain a wildcard, example: "*.coder.com"
// accessURL: The access url for the deployment.
//
// Returns:
// 'apphost:port'
//
// For backwards compatibility and for "accessurl=localhost:0" purposes, we need
// to use the port from the accessurl if the apphost doesn't have a port.
// If the user specifies a port in the apphost, we will use that port instead.
func SubdomainAppHost(apphost string, accessURL *url.URL) string {
	if apphost == "" {
		return ""
	}

	if apphost != "" && accessURL.Port() != "" {
		// This should always parse if we prepend a scheme. We should add
		// the access url port if the apphost doesn't have a port specified.
		appHostU, err := url.Parse(fmt.Sprintf("https://%s", apphost))
		if err != nil || (err == nil && appHostU.Port() == "") {
			apphost += fmt.Sprintf(":%s", accessURL.Port())
		}
	}

	return apphost
}

// ApplicationURL is a parsed application URL hostname. A long form URL sets
// AgentName, WorkspaceName and Username. A short form URL sets WorkspaceID,
// and AgentNameHash for ports.
type ApplicationURL struct {
	Prefix        string
	AppSlugOrPort string
	AgentName     string
	WorkspaceName string
	Username      string
	WorkspaceID   uuid.UUID
	AgentNameHash string
}

// New returns the application URL for an app slug or port. It returns the
// long form when it fits in one DNS label next to the fixed part of appHost,
// and the short form otherwise. For ports, the decision uses the longest
// possible port, so all ports of an agent get the same form. appHost is the
// wildcard app hostname, for example "*.apps.coder.com".
func New(appSlugOrPort, agentName, workspaceName, username string, workspaceID uuid.UUID, appHost string) ApplicationURL {
	isPort := appSlugOrPort == portPlaceholder || anyPortRegex.MatchString(appSlugOrPort)
	if !isPort {
		// App slugs are unique in a workspace, so the agent is not needed.
		agentName = ""
	}

	long := ApplicationURL{
		AppSlugOrPort: appSlugOrPort,
		AgentName:     agentName,
		WorkspaceName: workspaceName,
		Username:      username,
	}
	longest := long
	if isPort {
		longest.AppSlugOrPort = longestPort
	}
	if len(longest.String()) <= MaxSubdomainLength(appHost) {
		return long
	}

	short := ApplicationURL{
		AppSlugOrPort: appSlugOrPort,
		WorkspaceID:   workspaceID,
	}
	if isPort {
		short.AgentNameHash = AgentNameHash(agentName)
	}
	return short
}

// MaxSubdomainLength returns how many characters the wildcard in appHost can
// take before the first label passes the DNS label limit.
func MaxSubdomainLength(appHost string) int {
	label, _, _ := strings.Cut(appHost, ".")
	return maxLabelLength - len(strings.TrimPrefix(label, "*"))
}

// MaxShortSubdomainLength is the length of the longest short form port URL.
// If MaxSubdomainLength of the app host is lower, some URLs do not fit.
const MaxShortSubdomainLength = len(longestPort) + 2 + workspaceIDLength + 2 + agentNameHashLength

// AgentNameHash returns the short form identifier of an agent name: the
// 32-bit FNV-1a hash in base36. The frontend uses the same algorithm, so the
// two must stay the same.
func AgentNameHash(agentName string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(agentName))
	return fmt.Sprintf("%0*s", agentNameHashLength, strconv.FormatUint(uint64(h.Sum32()), 36))
}

// encodeWorkspaceID returns the UUID in base36, padded to a fixed length.
func encodeWorkspaceID(id uuid.UUID) string {
	return fmt.Sprintf("%0*s", workspaceIDLength, new(big.Int).SetBytes(id[:]).Text(36))
}

// decodeWorkspaceID reverses encodeWorkspaceID.
func decodeWorkspaceID(s string) (uuid.UUID, error) {
	n, ok := new(big.Int).SetString(strings.ToLower(s), 36)
	if !ok || n.BitLen() > 128 {
		return uuid.Nil, xerrors.Errorf("invalid workspace ID %q", s)
	}
	var id uuid.UUID
	n.FillBytes(id[:])
	return id, nil
}

// String returns the application URL hostname without scheme. You will likely
// want to append a period and the base hostname.
func (a ApplicationURL) String() string {
	var appURL strings.Builder
	_, _ = appURL.WriteString(a.Prefix)
	_, _ = appURL.WriteString(a.AppSlugOrPort)
	if a.WorkspaceID != uuid.Nil {
		_, _ = appURL.WriteString("--")
		_, _ = appURL.WriteString(encodeWorkspaceID(a.WorkspaceID))
		if a.AgentNameHash != "" {
			_, _ = appURL.WriteString("--")
			_, _ = appURL.WriteString(a.AgentNameHash)
		}
		return appURL.String()
	}
	if a.AgentName != "" {
		_, _ = appURL.WriteString("--")
		_, _ = appURL.WriteString(a.AgentName)
	}
	_, _ = appURL.WriteString("--")
	_, _ = appURL.WriteString(a.WorkspaceName)
	_, _ = appURL.WriteString("--")
	_, _ = appURL.WriteString(a.Username)
	return appURL.String()
}

// Path is a helper function to get the url path of the app if it is not served
// on a subdomain. In practice this is not really used because we use the chi
// `{variable}` syntax to extract these parts. For testing purposes and for
// completeness of this package, we include it.
func (a ApplicationURL) Path() string {
	if a.AgentName != "" {
		return fmt.Sprintf("/@%s/%s.%s/apps/%s", a.Username, a.WorkspaceName, a.AgentName, a.AppSlugOrPort)
	}
	return fmt.Sprintf("/@%s/%s/apps/%s", a.Username, a.WorkspaceName, a.AppSlugOrPort)
}

// PortInfo returns the port, protocol, and whether the AppSlugOrPort is a port or not.
func (a ApplicationURL) PortInfo() (uint, string, bool) {
	var (
		port     uint64
		protocol string
		isPort   bool
		err      error
	)

	if strings.HasSuffix(a.AppSlugOrPort, "s") {
		trimmed := strings.TrimSuffix(a.AppSlugOrPort, "s")
		port, err = strconv.ParseUint(trimmed, 10, 16)
		if err == nil {
			protocol = "https"
			isPort = true
		}
	} else {
		port, err = strconv.ParseUint(a.AppSlugOrPort, 10, 16)
		if err == nil {
			protocol = "http"
			isPort = true
		}
	}

	return uint(port), protocol, isPort
}

func (a *ApplicationURL) ChangePortProtocol(target string) ApplicationURL {
	newAppURL := *a
	port, protocol, isPort := a.PortInfo()
	if !isPort {
		return newAppURL
	}

	if target == protocol {
		return newAppURL
	}

	if target == "https" {
		newAppURL.AppSlugOrPort = fmt.Sprintf("%ds", port)
	}

	if target == "http" {
		newAppURL.AppSlugOrPort = fmt.Sprintf("%d", port)
	}

	return newAppURL
}

// ParseSubdomainAppURL parses an ApplicationURL from the given subdomain. If
// the subdomain is not a valid application URL hostname, returns a non-nil
// error. If the hostname is not a subdomain of the given base hostname, returns
// a non-nil error.
//
// Subdomains should be in the form:
//
//	({PREFIX}---)?{PORT{s?}/APP_SLUG}[--{AGENT_NAME}]--{WORKSPACE_NAME}--{USERNAME}
//
// Where agent name is:
//   - REQUIRED for ports: 8080--agent--workspace--user, 8080s--agent--workspace--user
//   - OPTIONAL for app slugs: myapp--workspace--user (agent name omitted)
//
// Examples:
//   - https://8080--main--dev--dean.hi.c8s.io (port with required agent)
//   - https://8080s--main--dev--dean.hi.c8s.io (port with required agent)
//   - https://app--dev--dean.hi.c8s.io (app slug, no agent name required)
//   - https://prefix---8080--main--dev--dean.hi.c8s.io (port with prefix)
//   - https://prefix---app--dev--dean.hi.c8s.io (app slug with prefix)
//
// The short form identifies the workspace by its base36 ID. Ports also carry
// a hash of the agent name:
//   - https://8080s--0h4x2b5mvd6uwd2y6wqmdnwk1--1q2w3e4.hi.c8s.io (port)
//   - https://app--0h4x2b5mvd6uwd2y6wqmdnwk1.hi.c8s.io (app slug)
//
// The optional prefix is permitted to allow customers to put additional URL at
// the beginning of their application URL (i.e. if they want to simulate
// different subdomains on the same app/port).
//
// Prefix requires three hyphens at the end to separate it from the rest of the
// URL so we can add/remove segments in the future from the parsing logic.
func ParseSubdomainAppURL(subdomain string) (ApplicationURL, error) {
	var (
		prefixSegments = strings.Split(subdomain, "---")
		prefix         = ""
	)
	if len(prefixSegments) > 1 {
		prefix = strings.Join(prefixSegments[:len(prefixSegments)-1], "---") + "---"
		subdomain = prefixSegments[len(prefixSegments)-1]
	}

	if app, ok, err := parseShortSubdomain(subdomain); ok {
		app.Prefix = prefix
		return app, err
	}

	matches := appURL.FindStringSubmatch(subdomain)
	if matches == nil {
		return ApplicationURL{}, xerrors.Errorf("invalid application url format: %q", subdomain)
	}

	appSlug := matches[appURL.SubexpIndex("AppSlug")]
	agentName := matches[appURL.SubexpIndex("AgentName")]

	// Agent name is optional for app slugs but required for ports
	if PortRegex.MatchString(appSlug) {
		if agentName == "" {
			return ApplicationURL{}, xerrors.Errorf("agent name is required for port-based URLs: %q", subdomain)
		}
	} else {
		agentName = ""
	}

	return ApplicationURL{
		Prefix:        prefix,
		AppSlugOrPort: appSlug,
		AgentName:     agentName,
		WorkspaceName: matches[appURL.SubexpIndex("WorkspaceName")],
		Username:      matches[appURL.SubexpIndex("Username")],
	}, nil
}

// parseShortSubdomain parses a short form subdomain without a prefix. It
// returns false if the subdomain does not have the short form shape.
func parseShortSubdomain(subdomain string) (ApplicationURL, bool, error) {
	matches := shortAppURL.FindStringSubmatch(subdomain)
	if len(matches) == 0 {
		return ApplicationURL{}, false, nil
	}
	appSlugOrPort := matches[shortAppURL.SubexpIndex("AppSlugOrPort")]
	agentNameHash := strings.ToLower(matches[shortAppURL.SubexpIndex("AgentNameHash")])
	// Only port URLs have an agent hash in the short form. An app slug with
	// three segments is a long form URL.
	if anyPortRegex.MatchString(appSlugOrPort) != (agentNameHash != "") {
		return ApplicationURL{}, false, nil
	}
	workspaceID, err := decodeWorkspaceID(matches[shortAppURL.SubexpIndex("WorkspaceID")])
	if err != nil {
		return ApplicationURL{}, true, err
	}
	return ApplicationURL{
		AppSlugOrPort: appSlugOrPort,
		WorkspaceID:   workspaceID,
		AgentNameHash: agentNameHash,
	}, true, nil
}

// HostnamesMatch returns true if the hostnames are equal, disregarding
// capitalization, extra leading or trailing periods, and ports.
func HostnamesMatch(a, b string) bool {
	a = strings.Trim(a, ".")
	b = strings.Trim(b, ".")

	aHost, _, err := net.SplitHostPort(a)
	if err != nil {
		aHost = a
	}
	bHost, _, err := net.SplitHostPort(b)
	if err != nil {
		bHost = b
	}

	return strings.EqualFold(aHost, bHost)
}

// CompileHostnamePattern compiles a hostname pattern into a regular expression.
// A hostname pattern is a string that may contain a single wildcard character
// at the beginning. The wildcard character matches any number of hostname-safe
// characters excluding periods. The pattern is case-insensitive.
//
// The supplied pattern:
//   - must not start or end with a period
//   - must contain exactly one asterisk at the beginning
//   - must not contain any other wildcard characters
//   - must not contain any other characters that are not hostname-safe (including
//     whitespace)
//   - must contain at least two hostname labels/segments (i.e. "foo" or "*" are
//     not valid patterns, but "foo.bar" and "*.bar" are).
//
// The returned regular expression will match an entire hostname with optional
// trailing periods and whitespace. The first submatch will be the wildcard
// match.
func CompileHostnamePattern(pattern string) (*regexp.Regexp, error) {
	pattern = strings.ToLower(pattern)
	if strings.Contains(pattern, "http:") || strings.Contains(pattern, "https:") {
		return nil, xerrors.Errorf("hostname pattern must not contain a scheme: %q", pattern)
	}

	if strings.HasPrefix(pattern, ".") || strings.HasSuffix(pattern, ".") {
		return nil, xerrors.Errorf("hostname pattern must not start or end with a period: %q", pattern)
	}
	if strings.Count(pattern, ".") < 1 {
		return nil, xerrors.Errorf("hostname pattern must contain at least two labels/segments: %q", pattern)
	}
	if strings.Count(pattern, "*") != 1 {
		return nil, xerrors.Errorf("hostname pattern must contain exactly one asterisk: %q", pattern)
	}
	if !strings.HasPrefix(pattern, "*") {
		return nil, xerrors.Errorf("hostname pattern must only contain an asterisk at the beginning: %q", pattern)
	}

	// If there is a hostname:port, we only care about the hostname. For hostname
	// pattern reasons, we do not actually care what port the client is requesting.
	// Any port provided here is used for generating urls for the ui, not for
	// validation.
	hostname, _, err := net.SplitHostPort(pattern)
	if err == nil {
		pattern = hostname
	}

	for i, label := range strings.Split(pattern, ".") {
		if i == 0 {
			// We have to allow the asterisk to be a valid hostname label, so
			// we strip the asterisk (which is only on the first one).
			label = strings.TrimPrefix(label, "*")
			// Put an "a" at the start to stand in for the asterisk in the regex
			// test below. This makes `*.coder.com` become `a.coder.com` and
			// `*--prod.coder.com` become `a--prod.coder.com`.
			label = "a" + label
		}
		if !validHostnameLabelRegex.MatchString(label) {
			return nil, xerrors.Errorf("hostname pattern contains invalid label %q: %q", label, pattern)
		}
	}

	// Replace periods with escaped periods.
	regexPattern := strings.ReplaceAll(pattern, ".", "\\.")

	// Capture wildcard match.
	regexPattern = strings.Replace(regexPattern, "*", "([^.]+)", 1)

	// Allow trailing period.
	regexPattern += "\\.?"

	// Allow optional port number.
	regexPattern += "(:\\d+)?"

	// Allow leading and trailing whitespace.
	regexPattern = `^\s*` + regexPattern + `\s*$`

	return regexp.Compile(regexPattern)
}

// ExecuteHostnamePattern executes a pattern generated by CompileHostnamePattern
// and returns the wildcard match. If the pattern does not match the hostname,
// returns false.
func ExecuteHostnamePattern(pattern *regexp.Regexp, hostname string) (string, bool) {
	matches := pattern.FindStringSubmatch(hostname)
	if len(matches) < 2 {
		return "", false
	}

	return matches[1], true
}

// ConvertAppHostForCSP converts the wildcard host to a format accepted by CSP.
// For example *--apps.coder.com must become *.coder.com.  If there is no
// wildcard host, or it cannot be converted, return the base host.
func ConvertAppHostForCSP(host, wildcard string) string {
	if wildcard == "" {
		return host
	}
	parts := strings.Split(wildcard, ".")
	for i, part := range parts {
		if strings.Contains(part, "*") {
			// The wildcard can only be in the first section.
			if i != 0 {
				return host
			}
			parts[i] = "*"
		}
	}
	return strings.Join(parts, ".")
}
