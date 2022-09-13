package httpapi

import (
	"fmt"
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
//
// An error is returned if the string doesn't contain a period.
func SplitSubdomain(hostname string) (subdomain string, rest string, err error) {
	toks := strings.SplitN(hostname, ".", 2)
	if len(toks) < 2 {
		return "", "", xerrors.New("no subdomain")
	}

	return toks[0], toks[1], nil
}

// ApplicationURL is a parsed application URL hostname.
type ApplicationURL struct {
	// Only one of AppName or Port will be set.
	AppName       string
	Port          uint16
	AgentName     string
	WorkspaceName string
	Username      string
	// BaseHostname is the rest of the hostname minus the application URL part
	// and the first dot.
	BaseHostname string
}

// String returns the application URL hostname without scheme.
func (a ApplicationURL) String() string {
	appNameOrPort := a.AppName
	if a.Port != 0 {
		appNameOrPort = strconv.Itoa(int(a.Port))
	}

	return fmt.Sprintf("%s--%s--%s--%s.%s", appNameOrPort, a.AgentName, a.WorkspaceName, a.Username, a.BaseHostname)
}

// ParseSubdomainAppURL parses an ApplicationURL from the given hostname. If
// the subdomain is not a valid application URL hostname, returns a non-nil
// error.
//
// Subdomains should be in the form:
//
//	{PORT/APP_NAME}--{AGENT_NAME}--{WORKSPACE_NAME}--{USERNAME}
//	(eg. http://8080--main--dev--dean.hi.c8s.io)
func ParseSubdomainAppURL(hostname string) (ApplicationURL, error) {
	subdomain, rest, err := SplitSubdomain(hostname)
	if err != nil {
		return ApplicationURL{}, xerrors.Errorf("split host domain %q: %w", hostname, err)
	}

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
		BaseHostname:  rest,
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
