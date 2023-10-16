package httpapi

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	"golang.org/x/xerrors"
)

var (
	// Remove the "starts with" and "ends with" regex components.
	nameRegex = strings.Trim(UsernameValidRegex.String(), "^$")
	appURL    = regexp.MustCompile(fmt.Sprintf(
		// {PORT/APP_SLUG}--{AGENT_NAME}--{WORKSPACE_NAME}--{USERNAME}
		`^(?P<AppSlug>%[1]s)--(?P<AgentName>%[1]s)--(?P<WorkspaceName>%[1]s)--(?P<Username>%[1]s)$`,
		nameRegex))

	validHostnameLabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
)

// ApplicationURL is a parsed application URL hostname.
type ApplicationURL struct {
	Prefix        string
	AppSlugOrPort string
	AgentName     string
	WorkspaceName string
	Username      string
}

// String returns the application URL hostname without scheme. You will likely
// want to append a period and the base hostname.
func (a ApplicationURL) String() string {
	var appURL strings.Builder
	_, _ = appURL.WriteString(a.Prefix)
	_, _ = appURL.WriteString(a.AppSlugOrPort)
	_, _ = appURL.WriteString("--")
	_, _ = appURL.WriteString(a.AgentName)
	_, _ = appURL.WriteString("--")
	_, _ = appURL.WriteString(a.WorkspaceName)
	_, _ = appURL.WriteString("--")
	_, _ = appURL.WriteString(a.Username)
	return appURL.String()
}

// ParseSubdomainAppURL parses an ApplicationURL from the given subdomain. If
// the subdomain is not a valid application URL hostname, returns a non-nil
// error. If the hostname is not a subdomain of the given base hostname, returns
// a non-nil error.
//
// Subdomains should be in the form:
//
//		({PREFIX}---)?{PORT/APP_SLUG}--{AGENT_NAME}--{WORKSPACE_NAME}--{USERNAME}
//		e.g.
//	     https://8080--main--dev--dean.hi.c8s.io
//	     https://app--main--dev--dean.hi.c8s.io
//	     https://prefix---8080--main--dev--dean.hi.c8s.io
//	     https://prefix---app--main--dev--dean.hi.c8s.io
//
// The optional prefix is permitted to allow customers to put additional URL at
// the beginning of their application URL (i.e. if they want to simulate
// different subdomains on the same app/port).
//
// Prefix requires three hyphens at the end to separate it from the rest of the
// URL so we can add/remove segments in the future from the parsing logic.
//
// TODO(dean): make the agent name optional when using the app slug. This will
// reduce the character count for app URLs.
func ParseSubdomainAppURL(subdomain string) (ApplicationURL, error) {
	var (
		prefixSegments = strings.Split(subdomain, "---")
		prefix         = ""
	)
	if len(prefixSegments) > 1 {
		prefix = strings.Join(prefixSegments[:len(prefixSegments)-1], "---") + "---"
		subdomain = prefixSegments[len(prefixSegments)-1]
	}

	matches := appURL.FindAllStringSubmatch(subdomain, -1)
	if len(matches) == 0 {
		return ApplicationURL{}, xerrors.Errorf("invalid application url format: %q", subdomain)
	}
	matchGroup := matches[0]

	return ApplicationURL{
		Prefix:        prefix,
		AppSlugOrPort: matchGroup[appURL.SubexpIndex("AppSlug")],
		AgentName:     matchGroup[appURL.SubexpIndex("AgentName")],
		WorkspaceName: matchGroup[appURL.SubexpIndex("WorkspaceName")],
		Username:      matchGroup[appURL.SubexpIndex("Username")],
	}, nil
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
	if strings.Contains(pattern, ":") {
		return nil, xerrors.Errorf("hostname pattern must not contain a port: %q", pattern)
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
	regexPattern = regexPattern + "\\.?"

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
