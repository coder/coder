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
)

// SplitSubdomain splits a subdomain from the rest of the hostname. E.g.:
//   - "foo.bar.com" becomes "foo", "bar.com"
//   - "foo.bar.baz.com" becomes "foo", "bar.baz.com"
//   - "foo" becomes "foo", ""
func SplitSubdomain(hostname string) (subdomain string, rest string) {
	toks := strings.SplitN(hostname, ".", 2)
	if len(toks) < 2 {
		return toks[0], ""
	}

	return toks[0], toks[1]
}

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
