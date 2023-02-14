package coderd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"gopkg.in/square/go-jose.v2"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/site"
)

// workspaceApplicationAuth is an endpoint on the main router that handles
// redirects from the subdomain handler.
//
// This endpoint is under /api so we don't return the friendly error page here.
// Any errors on this endpoint should be errors that are unlikely to happen
// in production unless the user messes with the URL.
//
// @Summary Redirect to URI with encrypted API key
// @ID redirect-to-uri-with-encrypted-api-key
// @Security CoderSessionToken
// @Tags Applications
// @Param redirect_uri query string false "Redirect destination"
// @Success 307
// @Router /applications/auth-redirect [get]
func (api *API) workspaceApplicationAuth(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if api.AppHostname == "" {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "The server does not accept subdomain-based application requests.",
		})
		return
	}

	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, rbac.ActionCreate, apiKey) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// Get the redirect URI from the query parameters and parse it.
	redirectURI := r.URL.Query().Get(redirectURIQueryParam)
	if redirectURI == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing redirect_uri query parameter.",
		})
		return
	}
	u, err := url.Parse(redirectURI)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid redirect_uri query parameter.",
			Detail:  err.Error(),
		})
		return
	}
	// Force the redirect URI to use the same scheme as the access URL for
	// security purposes.
	u.Scheme = api.AccessURL.Scheme

	// Ensure that the redirect URI is a subdomain of api.AppHostname and is a
	// valid app subdomain.
	subdomain, ok := httpapi.ExecuteHostnamePattern(api.AppHostnameRegex, u.Host)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The redirect_uri query parameter must be a valid app subdomain.",
		})
		return
	}
	_, err = httpapi.ParseSubdomainAppURL(subdomain)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The redirect_uri query parameter must be a valid app subdomain.",
			Detail:  err.Error(),
		})
		return
	}

	// Create the application_connect-scoped API key with the same lifetime as
	// the current session.
	exp := apiKey.ExpiresAt
	lifetimeSeconds := apiKey.LifetimeSeconds
	if exp.IsZero() || time.Until(exp) > api.DeploymentConfig.SessionDuration.Value {
		exp = database.Now().Add(api.DeploymentConfig.SessionDuration.Value)
		lifetimeSeconds = int64(api.DeploymentConfig.SessionDuration.Value.Seconds())
	}
	cookie, _, err := api.createAPIKey(ctx, createAPIKeyParams{
		UserID:          apiKey.UserID,
		LoginType:       database.LoginTypePassword,
		ExpiresAt:       exp,
		LifetimeSeconds: lifetimeSeconds,
		Scope:           database.APIKeyScopeApplicationConnect,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create API key.",
			Detail:  err.Error(),
		})
		return
	}

	// Encrypt the API key.
	encryptedAPIKey, err := encryptAPIKey(encryptedAPIKeyPayload{
		APIKey: cookie.Value,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to encrypt API key.",
			Detail:  err.Error(),
		})
		return
	}

	// Redirect to the redirect URI with the encrypted API key in the query
	// parameters.
	q := u.Query()
	q.Set(subdomainProxyAPIKeyParam, encryptedAPIKey)
	u.RawQuery = q.Encode()
	http.Redirect(rw, r, u.String(), http.StatusTemporaryRedirect)
}

type workspaceAppRequest struct {
	AccessMethod workspaceAppAccessMethod
	// BasePath of the app. For path apps, this is the path prefix in the router
	// for this particular app. For subdomain apps, this should be "/". This is
	// used for setting the cookie path.
	BasePath string

	UsernameOrID string
	// WorkspaceAndAgent xor WorkspaceNameOrID are required.
	WorkspaceAndAgent string // "workspace" or "workspace.agent"
	WorkspaceNameOrID string
	// AgentNameOrID is not required if the workspace has only one agent.
	AgentNameOrID string
	AppSlugOrPort string
}

func (r workspaceAppRequest) Validate() error {
	if r.AccessMethod != workspaceAppAccessMethodPath && r.AccessMethod != workspaceAppAccessMethodSubdomain {
		return xerrors.Errorf("invalid access method: %q", r.AccessMethod)
	}
	if r.BasePath == "" {
		return xerrors.New("base path is required")
	}
	if r.UsernameOrID == "" {
		return xerrors.New("username or ID is required")
	}
	if r.UsernameOrID == codersdk.Me {
		// We block "me" for workspace app auth to avoid any security issues
		// caused by having an identical workspace name on yourself and a
		// different user and potentially reusing a ticket.
		//
		// This is also mitigated by storing the workspace/agent ID in the
		// ticket, but we block it here to be double safe.
		//
		// Subdomain apps have never been used with "me" from our code, and path
		// apps now have a redirect to remove the "me" from the URL.
		return xerrors.New(`username cannot be "me" in app requests`)
	}
	if r.WorkspaceAndAgent != "" {
		split := strings.Split(r.WorkspaceAndAgent, ".")
		if split[0] == "" || (len(split) == 2 && split[1] == "") || len(split) > 2 {
			return xerrors.Errorf("invalid workspace and agent: %q", r.WorkspaceAndAgent)
		}
		if r.WorkspaceNameOrID != "" || r.AgentNameOrID != "" {
			return xerrors.New("dev error: cannot specify both WorkspaceAndAgent and (WorkspaceNameOrID and AgentNameOrID)")
		}
	}
	if r.WorkspaceAndAgent == "" && r.WorkspaceNameOrID == "" {
		return xerrors.New("workspace name or ID is required")
	}
	if r.AppSlugOrPort == "" {
		return xerrors.New("app slug or port is required")
	}

	return nil
}

func (api *API) resolveWorkspaceApp(rw http.ResponseWriter, r *http.Request, appReq workspaceAppRequest) (*workspaceAppTicket, bool) {
	err := appReq.Validate()
	if err != nil {
		api.writeWorkspaceApp500(rw, r, &appReq, err, "invalid app request")
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
			api.writeWorkspaceApp500(rw, r, &appReq, err, "invalid app request")
			return nil, false
		}
	}

	// Get the existing ticket from the request.
	ticketCookie, err := r.Cookie(codersdk.DevURLSessionTicketCookie)
	if err == nil {
		ticket, err := api.parseWorkspaceAppTicket(ticketCookie.Value)
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
	//
	// We use the regular API key extraction middleware here to avoid any
	// differences in behavior between the two.
	var (
		ticket = workspaceAppTicket{
			AccessMethod:      appReq.AccessMethod,
			UsernameOrID:      appReq.UsernameOrID,
			WorkspaceNameOrID: appReq.WorkspaceNameOrID,
			AgentNameOrID:     appReq.AgentNameOrID,
			AppSlugOrPort:     appReq.AppSlugOrPort,
		}
		ticketOK = false
	)
	httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
		DB: api.Database,
		OAuth2Configs: &httpmw.OAuth2Configs{
			Github: api.GithubOAuth2Config,
			OIDC:   api.OIDCConfig,
		},
		// Optional is true to allow for public apps. If an authorization check
		// fails and the user is not authenticated, they will be redirected to
		// the login page below.
		RedirectToLogin:             false,
		DisableSessionExpiryRefresh: api.DeploymentConfig.DisableSessionExpiryRefresh.Value,
		Optional:                    true,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get user.
		var (
			user    database.User
			userErr error
		)
		if userID, uuidErr := uuid.Parse(appReq.UsernameOrID); uuidErr == nil {
			user, userErr = api.Database.GetUserByID(r.Context(), userID)
		} else {
			user, userErr = api.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
				Username: appReq.UsernameOrID,
			})
		}
		if xerrors.Is(userErr, sql.ErrNoRows) {
			api.writeWorkspaceApp404(rw, r, &appReq, fmt.Sprintf("user %q not found", appReq.UsernameOrID))
			return
		} else if userErr != nil {
			api.writeWorkspaceApp500(rw, r, &appReq, userErr, "get user")
			return
		}
		ticket.UserID = user.ID

		// Get workspace.
		var (
			workspace    database.Workspace
			workspaceErr error
		)
		if workspaceID, uuidErr := uuid.Parse(appReq.WorkspaceNameOrID); uuidErr == nil {
			workspace, workspaceErr = api.Database.GetWorkspaceByID(r.Context(), workspaceID)
		} else {
			workspace, workspaceErr = api.Database.GetWorkspaceByOwnerIDAndName(r.Context(), database.GetWorkspaceByOwnerIDAndNameParams{
				OwnerID: user.ID,
				Name:    appReq.WorkspaceNameOrID,
				Deleted: false,
			})
		}
		if xerrors.Is(workspaceErr, sql.ErrNoRows) {
			api.writeWorkspaceApp404(rw, r, &appReq, fmt.Sprintf("workspace %q not found", appReq.WorkspaceNameOrID))
			return
		} else if workspaceErr != nil {
			api.writeWorkspaceApp500(rw, r, &appReq, workspaceErr, "get workspace")
			return
		}
		ticket.WorkspaceID = workspace.ID

		// Get agent.
		var (
			agent      database.WorkspaceAgent
			agentErr   error
			trustAgent = false
		)
		if agentID, uuidErr := uuid.Parse(appReq.AgentNameOrID); uuidErr == nil {
			agent, agentErr = api.Database.GetWorkspaceAgentByID(r.Context(), agentID)
		} else {
			build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), workspace.ID)
			if err != nil {
				api.writeWorkspaceApp500(rw, r, &appReq, err, "get latest workspace build")
				return
			}

			resources, err := api.Database.GetWorkspaceResourcesByJobID(r.Context(), build.JobID)
			if err != nil {
				api.writeWorkspaceApp500(rw, r, &appReq, err, "get workspace resources")
				return
			}
			resourcesIDs := []uuid.UUID{}
			for _, resource := range resources {
				resourcesIDs = append(resourcesIDs, resource.ID)
			}

			agents, err := api.Database.GetWorkspaceAgentsByResourceIDs(r.Context(), resourcesIDs)
			if err != nil {
				api.writeWorkspaceApp500(rw, r, &appReq, err, "get workspace agents")
				return
			}

			if appReq.AgentNameOrID == "" {
				if len(agents) != 1 {
					api.writeWorkspaceApp404(rw, r, &appReq, "no agent specified, but multiple exist in workspace")
					return
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
			api.writeWorkspaceApp404(rw, r, &appReq, fmt.Sprintf("agent %q not found", appReq.AgentNameOrID))
			return
		} else if agentErr != nil {
			api.writeWorkspaceApp500(rw, r, &appReq, agentErr, "get agent")
			return
		}

		// Verify the agent belongs to the workspace.
		if !trustAgent {
			agentResource, err := api.Database.GetWorkspaceResourceByID(r.Context(), agent.ResourceID)
			if err != nil {
				api.writeWorkspaceApp500(rw, r, &appReq, err, "get agent resource")
				return
			}
			build, err := api.Database.GetWorkspaceBuildByJobID(r.Context(), agentResource.JobID)
			if err != nil {
				api.writeWorkspaceApp500(rw, r, &appReq, err, "get agent workspace build")
				return
			}
			if build.WorkspaceID != workspace.ID {
				api.writeWorkspaceApp404(rw, r, &appReq, "agent does not belong to workspace")
				return
			}
		}
		ticket.AgentID = agent.ID

		// Get app.
		appSharingLevel := database.AppSharingLevelOwner
		portUint, portUintErr := strconv.ParseUint(appReq.AppSlugOrPort, 10, 16)
		if appReq.AccessMethod == workspaceAppAccessMethodSubdomain && portUintErr == nil {
			// If the app does not exist, but the app slug is a port number, then route
			// to the port as an "anonymous app". We only support HTTP for port-based
			// URLs.
			//
			// This is only supported for subdomain-based applications.
			ticket.AppURL = fmt.Sprintf("http://%s:%d", appReq.AppSlugOrPort, portUint)
		} else {
			app, ok := api.lookupWorkspaceApp(rw, r, agent.ID, appReq.AppSlugOrPort)
			if !ok {
				return
			}

			if !app.Url.Valid {
				site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
					Status:       http.StatusBadRequest,
					Title:        "Bad Request",
					Description:  fmt.Sprintf("Application %q does not have a URL set.", app.Slug),
					RetryEnabled: true,
					DashboardURL: api.AccessURL.String(),
				})
				return
			}

			if app.SharingLevel != "" {
				appSharingLevel = app.SharingLevel
			}
			ticket.AppURL = app.Url.String
		}

		// Verify the user has access to the app.
		authed, ok := api.fetchWorkspaceApplicationAuth(rw, r, appReq.AccessMethod, workspace, appSharingLevel)
		if !ok {
			return
		}
		if !authed {
			_, hasAPIKey := httpmw.APIKeyOptional(r)
			if hasAPIKey {
				// The request has a valid API key but insufficient permissions.
				api.writeWorkspaceApp404(rw, r, &appReq, "insufficient permissions")
				return
			}

			// Redirect to login as they don't have permission to access the app and
			// they aren't signed in.
			if appReq.AccessMethod == workspaceAppAccessMethodSubdomain {
				redirectURI := *r.URL
				redirectURI.Scheme = api.AccessURL.Scheme
				redirectURI.Host = httpapi.RequestHost(r)

				u := *api.AccessURL
				u.Path = "/api/v2/applications/auth-redirect"
				q := u.Query()
				q.Add(redirectURIQueryParam, redirectURI.String())
				u.RawQuery = q.Encode()

				http.Redirect(rw, r, u.String(), http.StatusTemporaryRedirect)
			} else {
				httpmw.RedirectToLogin(rw, r, httpmw.SignedOutErrorMessage)
			}
			return
		}

		ticketOK = true
	})).ServeHTTP(rw, r)
	if !ticketOK {
		return nil, false
	}

	// As a sanity check, ensure the ticket we just made is valid for this
	// request.
	if !ticket.MatchesRequest(appReq) {
		api.writeWorkspaceApp500(rw, r, &appReq, nil, "fresh ticket does not match request")
		return nil, false
	}

	// Sign the ticket.
	ticketStr, err := api.generateWorkspaceAppTicket(ticket)
	if err != nil {
		api.writeWorkspaceApp500(rw, r, &appReq, err, "generate ticket")
		return nil, false
	}

	// Write the ticket cookie. We always want this to apply to the current
	// hostname (even for subdomain apps, without any wildcard shenanigans,
	// because the ticket is only valid for a single app).
	http.SetCookie(rw, &http.Cookie{
		Name:  codersdk.DevURLSessionTicketCookie,
		Value: ticketStr,
		Path:  appReq.BasePath,
		// TODO: constant/configurable expiry
		Expires: time.Now().Add(time.Minute),
	})

	return &ticket, true
}

// lookupWorkspaceApp looks up the workspace application by slug in the given
// agent and returns it. If the application is not found or there was a server
// error while looking it up, an HTML error page is returned and false is
// returned so the caller can return early.
func (api *API) lookupWorkspaceApp(rw http.ResponseWriter, r *http.Request, agentID uuid.UUID, appSlug string) (database.WorkspaceApp, bool) {
	app, err := api.Database.GetWorkspaceAppByAgentIDAndSlug(r.Context(), database.GetWorkspaceAppByAgentIDAndSlugParams{
		AgentID: agentID,
		Slug:    appSlug,
	})
	if xerrors.Is(err, sql.ErrNoRows) {
		api.writeWorkspaceApp404(rw, r, nil, "application not found in agent by slug")
		return database.WorkspaceApp{}, false
	}
	if err != nil {
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusInternalServerError,
			Title:        "Internal Server Error",
			Description:  "Could not fetch workspace application: " + err.Error(),
			RetryEnabled: true,
			DashboardURL: api.AccessURL.String(),
		})
		return database.WorkspaceApp{}, false
	}

	return app, true
}

//nolint:revive
func (api *API) authorizeWorkspaceApp(r *http.Request, accessMethod workspaceAppAccessMethod, sharingLevel database.AppSharingLevel, workspace database.Workspace) (bool, error) {
	ctx := r.Context()

	if accessMethod == "" {
		accessMethod = workspaceAppAccessMethodPath
	}
	isPathApp := accessMethod == workspaceAppAccessMethodPath

	// If path-based app sharing is disabled (which is the default), we can
	// force the sharing level to be "owner" so that the user can only access
	// their own apps.
	//
	// Site owners are blocked from accessing path-based apps unless the
	// Dangerous.AllowPathAppSiteOwnerAccess flag is enabled in the check below.
	if isPathApp && !api.DeploymentConfig.Dangerous.AllowPathAppSharing.Value {
		sharingLevel = database.AppSharingLevelOwner
	}

	// Short circuit if not authenticated.
	roles, ok := httpmw.UserAuthorizationOptional(r)
	if !ok {
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
		!api.DeploymentConfig.Dangerous.AllowPathAppSiteOwnerAccess.Value {

		return false, nil
	}

	// Do a standard RBAC check. This accounts for share level "owner" and any
	// other RBAC rules that may be in place.
	//
	// Regardless of share level or whether it's enabled or not, the owner of
	// the workspace can always access applications (as long as their API key's
	// scope allows it).
	err := api.Authorizer.Authorize(ctx, roles.Actor, rbac.ActionCreate, workspace.ApplicationConnectRBAC())
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
		err := api.Authorizer.Authorize(ctx, roles.Actor, rbac.ActionCreate, object)
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

// fetchWorkspaceApplicationAuth authorizes the user using api.AppAuthorizer
// for a given app share level in the given workspace. The user's authorization
// status is returned. If a server error occurs, a HTML error page is rendered
// and false is returned so the caller can return early.
func (api *API) fetchWorkspaceApplicationAuth(rw http.ResponseWriter, r *http.Request, accessMethod workspaceAppAccessMethod, workspace database.Workspace, appSharingLevel database.AppSharingLevel) (authed bool, ok bool) {
	ok, err := api.authorizeWorkspaceApp(r, accessMethod, appSharingLevel, workspace)
	if err != nil {
		api.Logger.Error(r.Context(), "authorize workspace app", slog.Error(err))
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       http.StatusInternalServerError,
			Title:        "Internal Server Error",
			Description:  "Could not verify authorization. Please try again or contact an administrator.",
			RetryEnabled: true,
			DashboardURL: api.AccessURL.String(),
		})
		return false, false
	}

	return ok, true
}

// writeWorkspaceApp404 writes a HTML 404 error page for a workspace app. If
// appReq is not nil, it will be used to log the request details at debug level.
func (api *API) writeWorkspaceApp404(rw http.ResponseWriter, r *http.Request, appReq *workspaceAppRequest, msg string) {
	if appReq != nil {
		slog.Helper()
		api.Logger.Debug(r.Context(),
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
		DashboardURL: api.AccessURL.String(),
	})
}

// writeWorkspaceApp500 writes a HTML 500 error page for a workspace app. If
// appReq is not nil, it's fields will be added to the logged error message.
func (api *API) writeWorkspaceApp500(rw http.ResponseWriter, r *http.Request, appReq *workspaceAppRequest, err error, msg string) {
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
	api.Logger.Warn(r.Context(),
		"workspace app auth server error: "+msg,
		slog.Error(err),
	)

	site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
		Status:       http.StatusInternalServerError,
		Title:        "Internal Server Error",
		Description:  "An internal server error occurred.",
		RetryEnabled: false,
		DashboardURL: api.AccessURL.String(),
	})
}

// workspaceAppTicket is the struct data contained inside a workspace app ticket
// JWE. It contains the details of the workspace app that the ticket is valid
// for to avoid database queries.
//
// The JSON field names are short to reduce the size of the ticket.
type workspaceAppTicket struct {
	// Request details.
	AccessMethod      workspaceAppAccessMethod `json:"access_method"`
	UsernameOrID      string                   `json:"username_or_id"`
	WorkspaceNameOrID string                   `json:"workspace_name_or_id"`
	AgentNameOrID     string                   `json:"agent_name_or_id"`
	AppSlugOrPort     string                   `json:"app_slug_or_port"`

	// Trusted resolved details.
	Expiry      int64     `json:"expiry"` // set by generateWorkspaceAppTicket
	UserID      uuid.UUID `json:"user_id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	AgentID     uuid.UUID `json:"agent_id"`
	AppURL      string    `json:"app_url"`
}

func (t workspaceAppTicket) MatchesRequest(req workspaceAppRequest) bool {
	return t.AccessMethod == req.AccessMethod &&
		t.UsernameOrID == req.UsernameOrID &&
		t.WorkspaceNameOrID == req.WorkspaceNameOrID &&
		t.AgentNameOrID == req.AgentNameOrID &&
		t.AppSlugOrPort == req.AppSlugOrPort
}

func (api *API) generateWorkspaceAppTicket(payload workspaceAppTicket) (string, error) {
	payload.Expiry = time.Now().Add(1 * time.Minute).Unix()
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", xerrors.Errorf("marshal payload to JSON: %w", err)
	}

	// We use symmetric signing with an RSA key to support satellites in the
	// future.
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.PS512, Key: api.AppSigningKey}, nil)
	if err != nil {
		return "", xerrors.Errorf("create signer: %w", err)
	}

	signedObject, err := signer.Sign(payloadBytes)
	if err != nil {
		return "", xerrors.Errorf("sign payload: %w", err)
	}

	serialized, err := signedObject.CompactSerialize()
	if err != nil {
		return "", xerrors.Errorf("serialize JWS: %w", err)
	}

	return serialized, nil
}

func (api *API) parseWorkspaceAppTicket(ticketStr string) (workspaceAppTicket, error) {
	object, err := jose.ParseSigned(ticketStr)
	if err != nil {
		return workspaceAppTicket{}, xerrors.Errorf("parse JWS: %w", err)
	}

	output, err := object.Verify(&api.AppSigningKey.PublicKey)
	if err != nil {
		return workspaceAppTicket{}, xerrors.Errorf("verify JWS: %w", err)
	}

	var ticket workspaceAppTicket
	err = json.Unmarshal(output, &ticket)
	if err != nil {
		return workspaceAppTicket{}, xerrors.Errorf("unmarshal payload: %w", err)
	}
	if ticket.Expiry < time.Now().Unix() {
		return workspaceAppTicket{}, xerrors.New("ticket expired")
	}

	return ticket, nil
}
