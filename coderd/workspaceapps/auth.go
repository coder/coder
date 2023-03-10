package workspaceapps

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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

// ResolveRequest takes an app request, checks if it's valid and authenticated,
// and returns a ticket with details about the app.
//
// The ticket is written as a signed JWT into a cookie and will be automatically
// used in the next request to the same app to avoid database calls.
//
// Upstream code should avoid any database calls ever.
func (p *Provider) ResolveRequest(rw http.ResponseWriter, r *http.Request, appReq Request) (*Ticket, bool) {
	// nolint:gocritic // We need to make a number of database calls. Setting a system context here
	//                 // is simpler than calling dbauthz.AsSystemRestricted on every call.
	//                 // dangerousSystemCtx is only used for database calls. The actual authentication
	//                 // logic is handled in Provider.authorizeWorkspaceApp which directly checks the actor's
	//                 // permissions.
	dangerousSystemCtx := dbauthz.AsSystemRestricted(r.Context())
	err := appReq.Validate()
	if err != nil {
		p.writeWorkspaceApp500(rw, r, &appReq, err, "invalid app request")
		return nil, false
	}

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

	// Get the existing ticket from the request.
	ticketCookie, err := r.Cookie(codersdk.DevURLSessionTicketCookie)
	if err == nil {
		ticket, err := p.ParseTicket(ticketCookie.Value)
		if err == nil {
			if ticket.MatchesRequest(appReq) {
				// The request has a ticket, which is a valid ticket signed by
				// us, and matches the app that the user was trying to access.
				return &ticket, true
			}
		}
	}

	// There's no ticket or it's invalid, so we need to check auth using the
	// session token, validate auth and access to the app, then generate a new
	// ticket.
	ticket := Ticket{
		AccessMethod:      appReq.AccessMethod,
		UsernameOrID:      appReq.UsernameOrID,
		WorkspaceNameOrID: appReq.WorkspaceNameOrID,
		AgentNameOrID:     appReq.AgentNameOrID,
		AppSlugOrPort:     appReq.AppSlugOrPort,
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
		return nil, false
	}

	// Get user.
	var (
		user    database.User
		userErr error
	)
	if userID, uuidErr := uuid.Parse(appReq.UsernameOrID); uuidErr == nil {
		user, userErr = p.Database.GetUserByID(dangerousSystemCtx, userID)
	} else {
		user, userErr = p.Database.GetUserByEmailOrUsername(dangerousSystemCtx, database.GetUserByEmailOrUsernameParams{
			Username: appReq.UsernameOrID,
		})
	}
	if xerrors.Is(userErr, sql.ErrNoRows) {
		p.writeWorkspaceApp404(rw, r, &appReq, fmt.Sprintf("user %q not found", appReq.UsernameOrID))
		return nil, false
	} else if userErr != nil {
		p.writeWorkspaceApp500(rw, r, &appReq, userErr, "get user")
		return nil, false
	}
	ticket.UserID = user.ID

	// Get workspace.
	var (
		workspace    database.Workspace
		workspaceErr error
	)
	if workspaceID, uuidErr := uuid.Parse(appReq.WorkspaceNameOrID); uuidErr == nil {
		workspace, workspaceErr = p.Database.GetWorkspaceByID(dangerousSystemCtx, workspaceID)
	} else {
		workspace, workspaceErr = p.Database.GetWorkspaceByOwnerIDAndName(dangerousSystemCtx, database.GetWorkspaceByOwnerIDAndNameParams{
			OwnerID: user.ID,
			Name:    appReq.WorkspaceNameOrID,
			Deleted: false,
		})
	}
	if xerrors.Is(workspaceErr, sql.ErrNoRows) {
		p.writeWorkspaceApp404(rw, r, &appReq, fmt.Sprintf("workspace %q not found", appReq.WorkspaceNameOrID))
		return nil, false
	} else if workspaceErr != nil {
		p.writeWorkspaceApp500(rw, r, &appReq, workspaceErr, "get workspace")
		return nil, false
	}
	ticket.WorkspaceID = workspace.ID

	// Get agent.
	var (
		agent      database.WorkspaceAgent
		agentErr   error
		trustAgent = false
	)
	if agentID, uuidErr := uuid.Parse(appReq.AgentNameOrID); uuidErr == nil {
		agent, agentErr = p.Database.GetWorkspaceAgentByID(dangerousSystemCtx, agentID)
	} else {
		build, err := p.Database.GetLatestWorkspaceBuildByWorkspaceID(dangerousSystemCtx, workspace.ID)
		if err != nil {
			p.writeWorkspaceApp500(rw, r, &appReq, err, "get latest workspace build")
			return nil, false
		}

		// nolint:gocritic // We need to fetch the agent to authenticate the request. This is a system function.
		resources, err := p.Database.GetWorkspaceResourcesByJobID(dangerousSystemCtx, build.JobID)
		if err != nil {
			p.writeWorkspaceApp500(rw, r, &appReq, err, "get workspace resources")
			return nil, false
		}
		resourcesIDs := []uuid.UUID{}
		for _, resource := range resources {
			resourcesIDs = append(resourcesIDs, resource.ID)
		}

		// nolint:gocritic // We need to fetch the agent to authenticate the request. This is a system function.
		agents, err := p.Database.GetWorkspaceAgentsByResourceIDs(dangerousSystemCtx, resourcesIDs)
		if err != nil {
			p.writeWorkspaceApp500(rw, r, &appReq, err, "get workspace agents")
			return nil, false
		}

		if appReq.AgentNameOrID == "" {
			if len(agents) != 1 {
				p.writeWorkspaceApp404(rw, r, &appReq, "no agent specified, but multiple exist in workspace")
				return nil, false
			}

			agent = agents[0]
			trustAgent = true
		} else {
			for _, a := range agents {
				if a.Name == appReq.AgentNameOrID {
					agent = a
					trustAgent = true
					break
				}
			}
		}

		if agent.ID == uuid.Nil {
			agentErr = sql.ErrNoRows
		}
	}
	if xerrors.Is(agentErr, sql.ErrNoRows) {
		p.writeWorkspaceApp404(rw, r, &appReq, fmt.Sprintf("agent %q not found", appReq.AgentNameOrID))
		return nil, false
	} else if agentErr != nil {
		p.writeWorkspaceApp500(rw, r, &appReq, agentErr, "get agent")
		return nil, false
	}

	// Verify the agent belongs to the workspace.
	if !trustAgent {
		//nolint:gocritic // We need to fetch the agent to authenticate the request. This is a system function.
		agentResource, err := p.Database.GetWorkspaceResourceByID(dangerousSystemCtx, agent.ResourceID)
		if err != nil {
			p.writeWorkspaceApp500(rw, r, &appReq, err, "get agent resource")
			return nil, false
		}
		build, err := p.Database.GetWorkspaceBuildByJobID(dangerousSystemCtx, agentResource.JobID)
		if err != nil {
			p.writeWorkspaceApp500(rw, r, &appReq, err, "get agent workspace build")
			return nil, false
		}
		if build.WorkspaceID != workspace.ID {
			p.writeWorkspaceApp404(rw, r, &appReq, "agent does not belong to workspace")
			return nil, false
		}
	}
	ticket.AgentID = agent.ID

	// Get app.
	appSharingLevel := database.AppSharingLevelOwner
	portUint, portUintErr := strconv.ParseUint(appReq.AppSlugOrPort, 10, 16)
	if appReq.AccessMethod == AccessMethodSubdomain && portUintErr == nil {
		// If the app slug is a port number, then route to the port as an
		// "anonymous app". We only support HTTP for port-based URLs.
		//
		// This is only supported for subdomain-based applications.
		ticket.AppURL = fmt.Sprintf("http://127.0.0.1:%d", portUint)
	} else {
		app, ok := p.lookupWorkspaceApp(rw, r, agent.ID, appReq.AppSlugOrPort)
		if !ok {
			return nil, false
		}

		if !app.Url.Valid {
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
				Status:       http.StatusBadRequest,
				Title:        "Bad Request",
				Description:  fmt.Sprintf("Application %q does not have a URL set.", app.Slug),
				RetryEnabled: true,
				DashboardURL: p.AccessURL.String(),
			})
			return nil, false
		}

		if app.SharingLevel != "" {
			appSharingLevel = app.SharingLevel
		}
		ticket.AppURL = app.Url.String
	}

	// Verify the user has access to the app.
	authed, ok := p.fetchWorkspaceApplicationAuth(rw, r, authz, appReq.AccessMethod, workspace, appSharingLevel)
	if !ok {
		return nil, false
	}
	if !authed {
		if apiKey != nil {
			// The request has a valid API key but insufficient permissions.
			p.writeWorkspaceApp404(rw, r, &appReq, "insufficient permissions")
			return nil, false
		}

		// Redirect to login as they don't have permission to access the app
		// and they aren't signed in.
		if appReq.AccessMethod == AccessMethodSubdomain {
			redirectURI := *r.URL
			redirectURI.Scheme = p.AccessURL.Scheme
			redirectURI.Host = httpapi.RequestHost(r)

			u := *p.AccessURL
			u.Path = "/api/v2/applications/auth-redirect"
			q := u.Query()
			q.Add(RedirectURIQueryParam, redirectURI.String())
			u.RawQuery = q.Encode()

			http.Redirect(rw, r, u.String(), http.StatusTemporaryRedirect)
		} else {
			httpmw.RedirectToLogin(rw, r, httpmw.SignedOutErrorMessage)
		}
		return nil, false
	}

	// As a sanity check, ensure the ticket we just made is valid for this
	// request.
	if !ticket.MatchesRequest(appReq) {
		p.writeWorkspaceApp500(rw, r, &appReq, nil, "fresh ticket does not match request")
		return nil, false
	}

	// Sign the ticket.
	ticketExpiry := time.Now().Add(TicketExpiry)
	ticket.Expiry = ticketExpiry.Unix()
	ticketStr, err := p.GenerateTicket(ticket)
	if err != nil {
		p.writeWorkspaceApp500(rw, r, &appReq, err, "generate ticket")
		return nil, false
	}

	// Write the ticket cookie. We always want this to apply to the current
	// hostname (even for subdomain apps, without any wildcard shenanigans,
	// because the ticket is only valid for a single app).
	http.SetCookie(rw, &http.Cookie{
		Name:    codersdk.DevURLSessionTicketCookie,
		Value:   ticketStr,
		Path:    appReq.BasePath,
		Expires: ticketExpiry,
	})

	return &ticket, true
}

// lookupWorkspaceApp looks up the workspace application by slug in the given
// agent and returns it. If the application is not found or there was a server
// error while looking it up, an HTML error page is returned and false is
// returned so the caller can return early.
func (p *Provider) lookupWorkspaceApp(rw http.ResponseWriter, r *http.Request, agentID uuid.UUID, appSlug string) (database.WorkspaceApp, bool) {
	// nolint:gocritic // We need to fetch the workspace app to authorize the request.
	app, err := p.Database.GetWorkspaceAppByAgentIDAndSlug(dbauthz.AsSystemRestricted(r.Context()), database.GetWorkspaceAppByAgentIDAndSlugParams{
		AgentID: agentID,
		Slug:    appSlug,
	})
	if xerrors.Is(err, sql.ErrNoRows) {
		p.writeWorkspaceApp404(rw, r, nil, "application not found in agent by slug")
		return database.WorkspaceApp{}, false
	}
	if err != nil {
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusInternalServerError,
			Title:        "Internal Server Error",
			Description:  "Could not fetch workspace application: " + err.Error(),
			RetryEnabled: true,
			DashboardURL: p.AccessURL.String(),
		})
		return database.WorkspaceApp{}, false
	}

	return app, true
}

func (p *Provider) authorizeWorkspaceApp(ctx context.Context, roles *httpmw.Authorization, accessMethod AccessMethod, sharingLevel database.AppSharingLevel, workspace database.Workspace) (bool, error) {
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
		workspace.OwnerID.String() != roles.Actor.ID &&
		!p.DeploymentValues.Dangerous.AllowPathAppSiteOwnerAccess.Value() {
		return false, nil
	}

	// Do a standard RBAC check. This accounts for share level "owner" and any
	// other RBAC rules that may be in place.
	//
	// Regardless of share level or whether it's enabled or not, the owner of
	// the workspace can always access applications (as long as their API key's
	// scope allows it).
	err := p.Authorizer.Authorize(ctx, roles.Actor, rbac.ActionCreate, workspace.ApplicationConnectRBAC())
	if err == nil {
		return true, nil
	}

	switch sharingLevel {
	case database.AppSharingLevelOwner:
		// We essentially already did this above with the regular RBAC check.
		// Owners can always access their own apps according to RBAC rules, so
		// they have already been returned from this function.
	case database.AppSharingLevelAuthenticated:
		// The user is authenticated at this point, but we need to make sure
		// that they have ApplicationConnect permissions to their own
		// workspaces. This ensures that the key's scope has permission to
		// connect to workspace apps.
		object := rbac.ResourceWorkspaceApplicationConnect.WithOwner(roles.Actor.ID)
		err := p.Authorizer.Authorize(ctx, roles.Actor, rbac.ActionCreate, object)
		if err == nil {
			return true, nil
		}
	case database.AppSharingLevelPublic:
		// We don't really care about scopes and stuff if it's public anyways.
		// Someone with a restricted-scope API key could just not submit the
		// API key cookie in the request and access the page.
		return true, nil
	}

	// No checks were successful.
	return false, nil
}

// fetchWorkspaceApplicationAuth authorizes the user using api.Authorizer for a
// given app share level in the given workspace. The user's authorization status
// is returned. If a server error occurs, a HTML error page is rendered and
// false is returned so the caller can return early.
func (p *Provider) fetchWorkspaceApplicationAuth(rw http.ResponseWriter, r *http.Request, authz *httpmw.Authorization, accessMethod AccessMethod, workspace database.Workspace, appSharingLevel database.AppSharingLevel) (authed bool, ok bool) {
	ok, err := p.authorizeWorkspaceApp(r.Context(), authz, accessMethod, appSharingLevel, workspace)
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
func (p *Provider) writeWorkspaceApp404(rw http.ResponseWriter, r *http.Request, appReq *Request, msg string) {
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
func (p *Provider) writeWorkspaceApp500(rw http.ResponseWriter, r *http.Request, appReq *Request, err error, msg string) {
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
