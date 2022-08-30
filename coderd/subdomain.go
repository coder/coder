package coderd

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/coder/coder/coderd/httpmw"

	"github.com/go-chi/chi/v5"

	"golang.org/x/xerrors"
)

const (
	// XForwardedHostHeader is a header used by proxies to indicate the
	// original host of the request.
	XForwardedHostHeader = "X-Forwarded-Host"
)

// ApplicationURL is a parsed application url into it's components
type ApplicationURL struct {
	AppName       string
	WorkspaceName string
	Agent         string
	Username      string
	Path          string

	// Domain is used to output the url to reach the app.
	Domain string
}

func (api *API) handleSubdomain(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			app, err := ParseSubdomainAppURL(r)

			if err != nil {
				// Subdomain is not a valid application url. Pass through.
				// TODO: @emyrk we should probably catch invalid subdomains. Meaning
				// 	an invalid application should not route to the coderd.
				//	To do this we would need to know the list of valid access urls
				//	though?
				next.ServeHTTP(rw, r)
				return
			}

			workspaceAgentKey := app.WorkspaceName
			if app.Agent != "" {
				workspaceAgentKey += "." + app.Agent
			}
			chiCtx := chi.RouteContext(ctx)
			chiCtx.URLParams.Add("workspace_and_agent", workspaceAgentKey)
			chiCtx.URLParams.Add("user", app.Username)

			// Use the passed in app middlewares before passing to the proxy app.
			mws := chi.Middlewares(middlewares)
			mws.Handler(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				workspace := httpmw.WorkspaceParam(r)
				agent := httpmw.WorkspaceAgentParam(r)

				api.proxyWorkspaceApplication(proxyApplication{
					Workspace: workspace,
					Agent:     agent,
					AppName:   app.AppName,
				}, rw, r)
			})).ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

var (
	nameRegex = `[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*`
	appURL    = regexp.MustCompile(fmt.Sprintf(
		// {USERNAME}--{WORKSPACE_NAME}}--{{AGENT_NAME}}--{{PORT}}
		`^(?P<UserName>%[1]s)--(?P<WorkspaceName>%[1]s)(--(?P<AgentName>%[1]s))?--(?P<AppName>%[1]s)$`,
		nameRegex))
)

// ParseSubdomainAppURL parses an application from the subdomain of r's Host header.
// If the application string is not valid, returns a non-nil error.
//   1) {USERNAME}--{WORKSPACE_NAME}}--{{AGENT_NAME}}--{{PORT/AppName}}
//  	(eg. http://admin--myenv--main--8080.cdrdeploy.c8s.io)
func ParseSubdomainAppURL(r *http.Request) (ApplicationURL, error) {
	host := RequestHost(r)
	if host == "" {
		return ApplicationURL{}, xerrors.Errorf("no host header")
	}

	subdomain, domain, err := SplitSubdomain(host)
	if err != nil {
		return ApplicationURL{}, xerrors.Errorf("split host domain: %w", err)
	}

	matches := appURL.FindAllStringSubmatch(subdomain, -1)
	if len(matches) == 0 {
		return ApplicationURL{}, xerrors.Errorf("invalid application url format: %q", subdomain)
	}

	if len(matches) > 1 {
		return ApplicationURL{}, xerrors.Errorf("multiple matches (%d) for application url: %q", len(matches), subdomain)
	}
	matchGroup := matches[0]

	return ApplicationURL{
		AppName:       matchGroup[appURL.SubexpIndex("AppName")],
		WorkspaceName: matchGroup[appURL.SubexpIndex("WorkspaceName")],
		Agent:         matchGroup[appURL.SubexpIndex("AgentName")],
		Username:      matchGroup[appURL.SubexpIndex("UserName")],
		Path:          r.URL.Path,
		Domain:        domain,
	}, nil
}

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
