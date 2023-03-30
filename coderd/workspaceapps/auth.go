package workspaceapps

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/site"
)

const (
	// TODO(@deansheather): configurable expiry
	TicketExpiry = time.Minute

	// RedirectURIQueryParam is the query param for the app URL to be passed
	// back to the API auth endpoint on the main access URL.
	RedirectURIQueryParam = "redirect_uri"
)

// TODO: remove this temporary shim
func (p *DBTicketProvider) ResolveRequest(rw http.ResponseWriter, r *http.Request, appReq Request) (*Ticket, bool) {
	// TODO: this needs to be some sort of normalize function or something
	if appReq.WorkspaceAndAgent != "" {
		// workspace.agent
		workspaceAndAgent := strings.SplitN(appReq.WorkspaceAndAgent, ".", 2)
		appReq.WorkspaceAndAgent = ""
		appReq.WorkspaceNameOrID = workspaceAndAgent[0]
		if len(workspaceAndAgent) > 1 {
			appReq.AgentNameOrID = workspaceAndAgent[1]
		}

		// Sanity check.
		err := appReq.Validate()
		if err != nil {
			p.writeWorkspaceApp500(rw, r, &appReq, err, "invalid app request")
			return nil, false
		}
	}

	ticket, ok := p.TicketFromRequest(r)
	if ok && ticket.MatchesRequest(appReq) {
		// The request has a valid ticket and it matches the request.
		return ticket, true
	}

	ticket, ticketStr, ok := p.CreateTicket(r.Context(), rw, r, appReq)
	if !ok {
		return nil, false
	}

	// Write the ticket cookie. We always want this to apply to the current
	// hostname (even for subdomain apps, without any wildcard shenanigans,
	// because the ticket is only valid for a single app).
	http.SetCookie(rw, &http.Cookie{
		Name:    codersdk.DevURLSessionTicketCookie,
		Value:   ticketStr,
		Path:    appReq.BasePath,
		Expires: ticket.Expiry,
	})

	return ticket, true
}

func (p *DBTicketProvider) TicketFromRequest(r *http.Request) (*Ticket, bool) {
	// Get the existing ticket from the request.
	ticketCookie, err := r.Cookie(codersdk.DevURLSessionTicketCookie)
	if err == nil {
		ticket, err := p.ParseTicket(ticketCookie.Value)
		if err == nil {
			err := ticket.Request.Validate()
			if err == nil {
				// The request has a ticket, which is a valid ticket signed by
				// us. The caller must check that it matches the request.
				return &ticket, true
			}
		}
	}

	return nil, false
}

// ResolveRequest takes an app request, checks if it's valid and authenticated,
// and returns a ticket with details about the app.
//
// The ticket is written as a signed JWT into a cookie and will be automatically
// used in the next request to the same app to avoid database calls.
//
// Upstream code should avoid any database calls ever.
func (p *DBTicketProvider) CreateTicket(ctx context.Context, rw http.ResponseWriter, r *http.Request, appReq Request) (*Ticket, string, bool) {
	// nolint:gocritic // We need to make a number of database calls. Setting a system context here
	//                 // is simpler than calling dbauthz.AsSystemRestricted on every call.
	//                 // dangerousSystemCtx is only used for database calls. The actual authentication
	//                 // logic is handled in Provider.authorizeWorkspaceApp which directly checks the actor's
	//                 // permissions.
	dangerousSystemCtx := dbauthz.AsSystemRestricted(ctx)
	err := appReq.Validate()
	if err != nil {
		p.writeWorkspaceApp500(rw, r, &appReq, err, "invalid app request")
		return nil, "", false
	}

	ticket := Ticket{
		Request: appReq,
	}

	// We use the regular API apiKey extraction middleware fn here to avoid any
	// differences in behavior between the two.
	apiKey, authz, ok := httpmw.ExtractAPIKey(rw, r, httpmw.ExtractAPIKeyConfig{
		DB:                          p.Database,
		OAuth2Configs:               p.OAuth2Configs,
		RedirectToLogin:             false,
		DisableSessionExpiryRefresh: p.DeploymentValues.DisableSessionExpiryRefresh.Value(),
		// Optional is true to allow for public apps. If an authorization check
		// fails and the user is not authenticated, they will be redirected to
		// the login page using code below (not the redirect from the
		// middleware itself).
		Optional: true,
	})
	if !ok {
		return nil, "", false
	}

	// Lookup workspace app details from DB.
	dbReq, err := appReq.getDatabase(dangerousSystemCtx, p.Database)
	if xerrors.Is(err, sql.ErrNoRows) {
		p.writeWorkspaceApp404(rw, r, &appReq, err.Error())
		return nil, "", false
	} else if err != nil {
		p.writeWorkspaceApp500(rw, r, &appReq, err, "get app details from database")
		return nil, "", false
	}
	ticket.UserID = dbReq.User.ID
	ticket.WorkspaceID = dbReq.Workspace.ID
	ticket.AgentID = dbReq.Agent.ID
	ticket.AppURL = dbReq.AppURL

	// TODO(@deansheather): return an error if the agent is offline or the app
	// is not running.

	// Verify the user has access to the app.
	authed, ok := p.verifyAuthz(rw, r, authz, dbReq)
	if !ok {
		return nil, "", false
	}
	if !authed {
		if apiKey != nil {
			// The request has a valid API key but insufficient permissions.
			p.writeWorkspaceApp404(rw, r, &appReq, "insufficient permissions")
			return nil, "", false
		}

		// Redirect to login as they don't have permission to access the app
		// and they aren't signed in.
		switch appReq.AccessMethod {
		case AccessMethodPath:
			// TODO(@deansheather): this doesn't work on moons
			httpmw.RedirectToLogin(rw, r, httpmw.SignedOutErrorMessage)
		case AccessMethodSubdomain:
			// Redirect to the app auth redirect endpoint with a valid redirect
			// URI.
			redirectURI := *r.URL
			redirectURI.Scheme = p.AccessURL.Scheme
			redirectURI.Host = httpapi.RequestHost(r)

			u := *p.AccessURL
			u.Path = "/api/v2/applications/auth-redirect"
			q := u.Query()
			q.Add(RedirectURIQueryParam, redirectURI.String())
			u.RawQuery = q.Encode()

			http.Redirect(rw, r, u.String(), http.StatusTemporaryRedirect)
		case AccessMethodTerminal:
			// Return an error.
			httpapi.ResourceNotFound(rw)
		}
		return nil, "", false
	}

	// Check that the agent is online.
	agentStatus := dbReq.Agent.Status(p.WorkspaceAgentInactiveTimeout)
	if agentStatus.Status != database.WorkspaceAgentStatusConnected {
		p.writeWorkspaceAppOffline(rw, r, &appReq, fmt.Sprintf("Agent state is %q, not %q", agentStatus.Status, database.WorkspaceAgentStatusConnected))
		return nil, "", false
	}

	// Check that the app is healthy.
	if dbReq.AppHealth != "" && dbReq.AppHealth != database.WorkspaceAppHealthDisabled && dbReq.AppHealth != database.WorkspaceAppHealthHealthy {
		p.writeWorkspaceAppOffline(rw, r, &appReq, fmt.Sprintf("App health is %q, not %q", dbReq.AppHealth, database.WorkspaceAppHealthHealthy))
		return nil, "", false
	}

	// As a sanity check, ensure the ticket we just made is valid for this
	// request.
	if !ticket.MatchesRequest(appReq) {
		p.writeWorkspaceApp500(rw, r, &appReq, nil, "fresh ticket does not match request")
		return nil, "", false
	}

	// Sign the ticket.
	ticket.Expiry = time.Now().Add(TicketExpiry)
	ticketStr, err := p.GenerateTicket(ticket)
	if err != nil {
		p.writeWorkspaceApp500(rw, r, &appReq, err, "generate ticket")
		return nil, "", false
	}

	return &ticket, ticketStr, true
}

func (p *DBTicketProvider) authorizeRequest(ctx context.Context, roles *httpmw.Authorization, dbReq *databaseRequest) (bool, error) {
	accessMethod := dbReq.AccessMethod
	if accessMethod == "" {
		accessMethod = AccessMethodPath
	}
	isPathApp := accessMethod == AccessMethodPath

	// If path-based app sharing is disabled (which is the default), we can
	// force the sharing level to be "owner" so that the user can only access
	// their own apps.
	//
	// Site owners are blocked from accessing path-based apps unless the
	// Dangerous.AllowPathAppSiteOwnerAccess flag is enabled in the check below.
	sharingLevel := dbReq.AppSharingLevel
	if isPathApp && !p.DeploymentValues.Dangerous.AllowPathAppSharing.Value() {
		sharingLevel = database.AppSharingLevelOwner
	}

	// Short circuit if not authenticated.
	if roles == nil {
		// The user is not authenticated, so they can only access the app if it
		// is public.
		return sharingLevel == database.AppSharingLevelPublic, nil
	}

	// Block anyone from accessing workspaces they don't own in path-based apps
	// unless the admin disables this security feature. This blocks site-owners
	// from accessing any apps from any user's workspaces.
	//
	// When the Dangerous.AllowPathAppSharing flag is not enabled, the sharing
	// level will be forced to "owner", so this check will always be true for
	// workspaces owned by different users.
	if isPathApp &&
		sharingLevel == database.AppSharingLevelOwner &&
		dbReq.Workspace.OwnerID.String() != roles.Actor.ID &&
		!p.DeploymentValues.Dangerous.AllowPathAppSiteOwnerAccess.Value() {
		return false, nil
	}

	// Figure out which RBAC resource to check. For terminals we use execution
	// instead of application connect.
	var (
		rbacAction   rbac.Action = rbac.ActionCreate
		rbacResource rbac.Object = dbReq.Workspace.ApplicationConnectRBAC()
		// rbacResourceOwned is for the level "authenticated". We still need to
		// make sure the API key has permissions to connect to the actor's own
		// workspace. Scopes would prevent this.
		rbacResourceOwned rbac.Object = rbac.ResourceWorkspaceApplicationConnect.WithOwner(roles.Actor.ID)
	)
	if dbReq.AccessMethod == AccessMethodTerminal {
		rbacResource = dbReq.Workspace.ExecutionRBAC()
		rbacResourceOwned = rbac.ResourceWorkspaceExecution.WithOwner(roles.Actor.ID)
	}

	// Do a standard RBAC check. This accounts for share level "owner" and any
	// other RBAC rules that may be in place.
	//
	// Regardless of share level or whether it's enabled or not, the owner of
	// the workspace can always access applications (as long as their API key's
	// scope allows it).
	err := p.Authorizer.Authorize(ctx, roles.Actor, rbacAction, rbacResource)
	if err == nil {
		return true, nil
	}

	switch sharingLevel {
	case database.AppSharingLevelOwner:
		// We essentially already did this above with the regular RBAC check.
		// Owners can always access their own apps according to RBAC rules, so
		// they have already been returned from this function.
	case database.AppSharingLevelAuthenticated:
		// Check with the owned resource to ensure the API key has permissions
		// to connect to the actor's own workspace. This enforces scopes.
		err := p.Authorizer.Authorize(ctx, roles.Actor, rbacAction, rbacResourceOwned)
		if err == nil {
			return true, nil
		}
	case database.AppSharingLevelPublic:
		// We don't really care about scopes and stuff if it's public anyways.
		// Someone with a restricted-scope API key could just not submit the API
		// key cookie in the request and access the page.
		return true, nil
	}

	// No checks were successful.
	return false, nil
}

// verifyAuthz authorizes the user using api.Authorizer for a
// given app share level in the given workspace. The user's authorization status
// is returned. If a server error occurs, a HTML error page is rendered and
// false is returned so the caller can return early.
func (p *DBTicketProvider) verifyAuthz(rw http.ResponseWriter, r *http.Request, authz *httpmw.Authorization, dbReq *databaseRequest) (authed bool, ok bool) {
	ok, err := p.authorizeRequest(r.Context(), authz, dbReq)
	if err != nil {
		p.Logger.Error(r.Context(), "authorize workspace app", slog.Error(err))
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusInternalServerError,
			Title:        "Internal Server Error",
			Description:  "Could not verify authorization. Please try again or contact an administrator.",
			RetryEnabled: true,
			DashboardURL: p.AccessURL.String(),
		})
		return false, false
	}

	return ok, true
}

// writeWorkspaceApp404 writes a HTML 404 error page for a workspace app. If
// appReq is not nil, it will be used to log the request details at debug level.
func (p *DBTicketProvider) writeWorkspaceApp404(rw http.ResponseWriter, r *http.Request, appReq *Request, msg string) {
	if appReq != nil {
		slog.Helper()
		p.Logger.Debug(r.Context(),
			"workspace app 404: "+msg,
			slog.F("username_or_id", appReq.UsernameOrID),
			slog.F("workspace_and_agent", appReq.WorkspaceAndAgent),
			slog.F("workspace_name_or_id", appReq.WorkspaceNameOrID),
			slog.F("agent_name_or_id", appReq.AgentNameOrID),
			slog.F("app_slug_or_port", appReq.AppSlugOrPort),
		)
	}

	site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
		Status:       http.StatusNotFound,
		Title:        "Application Not Found",
		Description:  "The application or workspace you are trying to access does not exist or you do not have permission to access it.",
		RetryEnabled: false,
		DashboardURL: p.AccessURL.String(),
	})
}

// writeWorkspaceApp500 writes a HTML 500 error page for a workspace app. If
// appReq is not nil, it's fields will be added to the logged error message.
func (p *DBTicketProvider) writeWorkspaceApp500(rw http.ResponseWriter, r *http.Request, appReq *Request, err error, msg string) {
	slog.Helper()
	ctx := r.Context()
	if appReq != nil {
		slog.With(ctx,
			slog.F("username_or_id", appReq.UsernameOrID),
			slog.F("workspace_and_agent", appReq.WorkspaceAndAgent),
			slog.F("workspace_name_or_id", appReq.WorkspaceNameOrID),
			slog.F("agent_name_or_id", appReq.AgentNameOrID),
			slog.F("app_name_or_port", appReq.AppSlugOrPort),
		)
	}
	p.Logger.Warn(ctx,
		"workspace app auth server error: "+msg,
		slog.Error(err),
	)

	site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
		Status:       http.StatusInternalServerError,
		Title:        "Internal Server Error",
		Description:  "An internal server error occurred.",
		RetryEnabled: false,
		DashboardURL: p.AccessURL.String(),
	})
}

// writeWorkspaceAppOffline writes a HTML 502 error page for a workspace app. If
// appReq is not nil, it will be used to log the request details at debug level.
func (p *DBTicketProvider) writeWorkspaceAppOffline(rw http.ResponseWriter, r *http.Request, appReq *Request, msg string) {
	if appReq != nil {
		slog.Helper()
		p.Logger.Debug(r.Context(),
			"workspace app unavailable: "+msg,
			slog.F("username_or_id", appReq.UsernameOrID),
			slog.F("workspace_and_agent", appReq.WorkspaceAndAgent),
			slog.F("workspace_name_or_id", appReq.WorkspaceNameOrID),
			slog.F("agent_name_or_id", appReq.AgentNameOrID),
			slog.F("app_slug_or_port", appReq.AppSlugOrPort),
		)
	}

	site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
		Status:       http.StatusBadGateway,
		Title:        "Application Unavailable",
		Description:  msg,
		RetryEnabled: true,
		DashboardURL: p.AccessURL.String(),
	})
}
