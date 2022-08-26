package coderd

import (
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"

	"github.com/coder/coder/coderd/database"

	"golang.org/x/xerrors"
)

const (
	// XForwardedHostHeader is a header used by proxies to indicate the
	// original host of the request.
	XForwardedHostHeader = "X-Forwarded-Host"
	xForwardedProto      = "X-Forwarded-Proto"
)

type Application struct {
	AppURL        string
	AppName       string
	WorkspaceName string
	Agent         string
	Username      string
	Path          string

	// Domain is used to output the url to reach the app.
	Domain string
}

func (api *API) handleSubdomain(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		app, err := ParseSubdomainAppURL(r)
		if err != nil {
			// Not a Dev URL, proceed as usual.
			// TODO: @emyrk we should probably catch invalid subdomains. Meaning
			// 	an invalid devurl should not route to the coderd.
			next.ServeHTTP(rw, r)
			return
		}

		user, err := api.Database.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
			Username: app.Username,
		})
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				httpapi.ResourceNotFound(rw)
				return
			}
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching user.",
				Detail:  err.Error(),
			})
			return
		}

		workspace, err := api.Database.GetWorkspaceByOwnerIDAndName(ctx, database.GetWorkspaceByOwnerIDAndNameParams{
			OwnerID: user.ID,
			Name:    app.WorkspaceName,
		})
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				httpapi.ResourceNotFound(rw)
				return
			}
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching workspace.",
				Detail:  err.Error(),
			})
			return
		}

		api.proxyWorkspaceApplication(proxyApplication{
			Workspace: workspace,
			// TODO: Fetch workspace agent
			Agent:   database.WorkspaceAgent{},
			AppName: app.AppName,
		}, rw, r)
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
		AppURL:        "",
		AppName:       matchGroup[appURL.SubexpIndex("AppName")],
		WorkspaceName: matchGroup[appURL.SubexpIndex("WorkspaceName")],
		Agent:         matchGroup[appURL.SubexpIndex("AgentName")],
		Username:      matchGroup[appURL.SubexpIndex("UserName")],
		Path:          r.URL.Path,
		Domain:        domain,
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
