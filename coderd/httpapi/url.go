package httpapi

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

var (
	// Remove the "starts with" and "ends with" regex components.
	nameRegex = strings.Trim(UsernameValidRegex.String(), "^$")
	appURL    = regexp.MustCompile(fmt.Sprintf(
		// {PORT/APP_NAME}--{AGENT_NAME}--{WORKSPACE_NAME}--{USERNAME}
		`^(?P<AppName>%[1]s)--(?P<AgentName>%[1]s)--(?P<WorkspaceName>%[1]s)--(?P<Username>%[1]s)$`,
		nameRegex))

	validHostnameLabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
)

// ApplicationURL is a parsed application URL hostname.
type ApplicationURL struct {
	// Only one of AppName or Port will be set.
	AppName       string
	Port          uint16
	AgentName     string
	WorkspaceName string
	Username      string
}

// String returns the application URL hostname without scheme. You will likely
// want to append a period and the base hostname.
func (a ApplicationURL) String() string {
	appNameOrPort := a.AppName
	if a.Port != 0 {
		appNameOrPort = strconv.Itoa(int(a.Port))
	}

	return fmt.Sprintf("%s--%s--%s--%s", appNameOrPort, a.AgentName, a.WorkspaceName, a.Username)
}

// ParseSubdomainAppURL parses an ApplicationURL from the given subdomain. If
// the subdomain is not a valid application URL hostname, returns a non-nil
// error. If the hostname is not a subdomain of the given base hostname, returns
// a non-nil error.
//
// The base hostname should not include a scheme, leading asterisk or dot.
//
// Subdomains should be in the form:
//
//	{PORT/APP_NAME}--{AGENT_NAME}--{WORKSPACE_NAME}--{USERNAME}
//	(eg. https://8080--main--dev--dean.hi.c8s.io)
func ParseSubdomainAppURL(subdomain string) (ApplicationURL, error) {
	matches := appURL.FindAllStringSubmatch(subdomain, -1)
	if len(matches) == 0 {
		return ApplicationURL{}, xerrors.Errorf("invalid application url format: %q", subdomain)
	}
	matchGroup := matches[0]

	appName, port := AppNameOrPort(matchGroup[appURL.SubexpIndex("AppName")])
	return ApplicationURL{
		AppName:       appName,
		Port:          port,
		AgentName:     matchGroup[appURL.SubexpIndex("AgentName")],
		WorkspaceName: matchGroup[appURL.SubexpIndex("WorkspaceName")],
		Username:      matchGroup[appURL.SubexpIndex("Username")],
	}, nil
}

// AppNameOrPort takes a string and returns either the input string or a port
// number.
func AppNameOrPort(val string) (string, uint16) {
	port, err := strconv.ParseUint(val, 10, 16)
	if err != nil || port == 0 {
		port = 0
	} else {
		val = ""
	}

	return val, uint16(port)
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
			// We have to allow the asterisk to be a valid hostname label.
			label = strings.TrimPrefix(label, "*")
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
