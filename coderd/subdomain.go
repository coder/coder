package coderd

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/xerrors"
)

const (
	// XForwardedHostHeader is a header used by proxies to indicate the
	// original host of the request.
	XForwardedHostHeader = "X-Forwarded-Host"
	xForwardedProto      = "X-Forwarded-Proto"
)

type Application struct {
	AppURL    string
	AppName   string
	Workspace string
	Agent     string
	User      string
	Path      string

	// Domain is used to output the url to reach the app.
	Domain string
}

func (api *API) handleSubdomain(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

	})
}

var (
	nameRegex = `[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*`
	appURL    = regexp.MustCompile(fmt.Sprintf(
		// {USERNAME}--{WORKSPACE_NAME}}--{{AGENT_NAME}}--{{PORT}}
		`^(?P<UserName>%[1]s)--(?P<WorkspaceName>%[1]s)(--(?P<AgentName>%[1]s))?--(?P<AppName>%[1]s)$`,
		nameRegex))
)

func ParseSubdomainAppURL(r *http.Request) (Application, error) {
	host := RequestHost(r)
	if host == "" {
		return Application{}, xerrors.Errorf("no host header")
	}

	subdomain, domain, err := SplitSubdomain(host)
	if err != nil {
		return Application{}, xerrors.Errorf("split host domain: %w", err)
	}

	matches := appURL.FindAllStringSubmatch(subdomain, -1)
	if len(matches) == 0 {
		return Application{}, xerrors.Errorf("invalid application url format: %q", subdomain)
	}

	if len(matches) > 1 {
		return Application{}, xerrors.Errorf("multiple matches (%d) for application url: %q", len(matches), subdomain)
	}
	matchGroup := matches[0]

	return Application{
		AppURL:    "",
		AppName:   matchGroup[appURL.SubexpIndex("AppName")],
		Workspace: matchGroup[appURL.SubexpIndex("WorkspaceName")],
		Agent:     matchGroup[appURL.SubexpIndex("AgentName")],
		User:      matchGroup[appURL.SubexpIndex("UserName")],
		Path:      r.URL.Path,
		Domain:    domain,
	}, nil
}

// Parse parses a DevURL from the subdomain of r's Host header.
// If DevURL is not valid, returns a non-nil error.
//
// devurls can be in two forms, each field separate by 2 hypthens:
//   1) port-envname-user (eg. http://8080--myenv--johndoe.cdrdeploy.c8s.io)
//   2) name-user         (eg. http://demosvc--johndoe.cdrdeploy.c8s.io)
//
// Note that envname itself can contain hyphens.
// If subdomain begins with a sequence of numbers, form 1 is assumed.
// Otherwise, form 2 is assumed.
//func Parse(r *http.Request, devurlSuffix string) (Application, error) {
//
//	return d, nil
//}

// RequestHost returns the name of the host from the request.  It prioritizes
// 'X-Forwarded-Host' over r.Host since most requests are being proxied.
func RequestHost(r *http.Request) string {
	host := r.Header.Get(XForwardedHostHeader)
	if host != "" {
		return host
	}

	return r.Host
}

// SplitSubdomain splits a subdomain from a domain. E.g.:
//   - "foo.bar.com" becomes "foo", "bar.com"
//   - "foo.bar.baz.com" becomes "foo", "bar.baz.com"
//
// An error is returned if the string doesn't contain a period.
func SplitSubdomain(hostname string) (string, string, error) {
	toks := strings.SplitN(hostname, ".", 2)
	if len(toks) < 2 {
		return "", "", xerrors.Errorf("no domain")
	}

	return toks[0], toks[1], nil
}
