package workspaceapps

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// DBTokenProvider provides authentication and authorization for workspace apps
// by querying the database if the request is missing a valid token.
type DBTokenProvider struct {
	Logger slog.Logger

	// DashboardURL is the main dashboard access URL for error pages.
	DashboardURL                    *url.URL
	Authorizer                      rbac.Authorizer
	ConnectionLogger                *atomic.Pointer[connectionlog.ConnectionLogger]
	Database                        database.Store
	DeploymentValues                *codersdk.DeploymentValues
	OAuth2Configs                   *httpmw.OAuth2Configs
	WorkspaceAgentInactiveTimeout   time.Duration
	WorkspaceAppAuditSessionTimeout time.Duration
	Keycache                        cryptokeys.SigningKeycache
	Clock                           quartz.Clock
}

var _ SignedTokenProvider = &DBTokenProvider{}

func NewDBTokenProvider(log slog.Logger,
	accessURL *url.URL,
	authz rbac.Authorizer,
	connectionLogger *atomic.Pointer[connectionlog.ConnectionLogger],
	db database.Store,
	cfg *codersdk.DeploymentValues,
	oauth2Cfgs *httpmw.OAuth2Configs,
	workspaceAgentInactiveTimeout time.Duration,
	workspaceAppAuditSessionTimeout time.Duration,
	signer cryptokeys.SigningKeycache,
) SignedTokenProvider {
	if workspaceAgentInactiveTimeout == 0 {
		workspaceAgentInactiveTimeout = 1 * time.Minute
	}
	if workspaceAppAuditSessionTimeout == 0 {
		workspaceAppAuditSessionTimeout = time.Hour
	}

	return &DBTokenProvider{
		Logger:                          log,
		DashboardURL:                    accessURL,
		Authorizer:                      authz,
		ConnectionLogger:                connectionLogger,
		Database:                        db,
		DeploymentValues:                cfg,
		OAuth2Configs:                   oauth2Cfgs,
		WorkspaceAgentInactiveTimeout:   workspaceAgentInactiveTimeout,
		WorkspaceAppAuditSessionTimeout: workspaceAppAuditSessionTimeout,
		Keycache:                        signer,
		Clock:                           quartz.NewReal(),
	}
}

func (p *DBTokenProvider) FromRequest(r *http.Request) (*SignedToken, bool) {
	return FromRequest(r, p.Keycache)
}

func (p *DBTokenProvider) Issue(ctx context.Context, rw http.ResponseWriter, r *http.Request, issueReq IssueTokenRequest) (*SignedToken, string, bool) {
	// nolint:gocritic // We need to make a number of database calls. Setting a system context here
	//                 // is simpler than calling dbauthz.AsSystemRestricted on every call.
	//                 // dangerousSystemCtx is only used for database calls. The actual authentication
	//                 // logic is handled in Provider.authorizeWorkspaceApp which directly checks the actor's
	//                 // permissions.
	dangerousSystemCtx := dbauthz.AsSystemRestricted(ctx)

	aReq, commitAudit := p.connLogInitRequest(ctx, rw, r)
	defer commitAudit()

	appReq := issueReq.AppRequest.Normalize()
	err := appReq.Check()
	if err != nil {
		WriteWorkspaceApp500(p.Logger, p.DashboardURL, rw, r, &appReq, err, "invalid app request")
		return nil, "", false
	}

	token := SignedToken{
		Request: appReq,
	}

	// We use the regular API apiKey extraction middleware fn here to avoid any
	// differences in behavior between the two.
	apiKey, authz, ok := httpmw.ExtractAPIKey(rw, r, httpmw.ExtractAPIKeyConfig{
		DB:                          p.Database,
		OAuth2Configs:               p.OAuth2Configs,
		RedirectToLogin:             false,
		DisableSessionExpiryRefresh: p.DeploymentValues.Sessions.DisableExpiryRefresh.Value(),
		// Optional is true to allow for public apps. If the authorization check
		// (later on) fails and the user is not authenticated, they will be
		// redirected to the login page or app auth endpoint using code below.
		Optional: true,
		SessionTokenFunc: func(_ *http.Request) string {
			return issueReq.SessionToken
		},
	})
	if !ok {
		return nil, "", false
	}

	aReq.apiKey = apiKey // Update audit request.

	// Lookup workspace app details from DB.
	dbReq, err := appReq.getDatabase(dangerousSystemCtx, p.Database)
	switch {
	case xerrors.Is(err, sql.ErrNoRows):
		WriteWorkspaceApp404(p.Logger, p.DashboardURL, rw, r, &appReq, nil, err.Error())
		return nil, "", false
	case xerrors.Is(err, errWorkspaceStopped):
		WriteWorkspaceOffline(p.Logger, p.DashboardURL, rw, r, &appReq)
		return nil, "", false
	case err != nil:
		WriteWorkspaceApp500(p.Logger, p.DashboardURL, rw, r, &appReq, err, "get app details from database")
		return nil, "", false
	}

	aReq.dbReq = dbReq // Update audit request.

	token.UserID = dbReq.User.ID
	token.WorkspaceID = dbReq.Workspace.ID
	token.AgentID = dbReq.Agent.ID
	if dbReq.AppURL != nil {
		token.AppURL = dbReq.AppURL.String()
	}
	token.CORSBehavior = codersdk.CORSBehavior(dbReq.CorsBehavior)

	// Verify the user has access to the app.
	authed, warnings, err := p.authorizeRequest(r.Context(), authz, dbReq)
	if err != nil {
		WriteWorkspaceApp500(p.Logger, p.DashboardURL, rw, r, &appReq, err, "verify authz")
		return nil, "", false
	}
	if !authed {
		if apiKey != nil {
			// The request has a valid API key but insufficient permissions.
			WriteWorkspaceApp404(p.Logger, p.DashboardURL, rw, r, &appReq, warnings, "insufficient permissions")
			return nil, "", false
		}

		// Redirect to login as they don't have permission to access the app
		// and they aren't signed in.

		// We don't support login redirects for the terminal since it's a
		// WebSocket endpoint and redirects won't work. The token must be
		// specified as a query parameter.
		if appReq.AccessMethod == AccessMethodTerminal {
			httpapi.ResourceNotFound(rw)
			return nil, "", false
		}

		appBaseURL, err := issueReq.AppBaseURL()
		if err != nil {
			WriteWorkspaceApp500(p.Logger, p.DashboardURL, rw, r, &appReq, err, "get app base URL")
			return nil, "", false
		}

		// If the app is a path app and it's on the same host as the dashboard
		// access URL, then we need to redirect to login using the standard
		// login redirect function.
		if appReq.AccessMethod == AccessMethodPath && appBaseURL.Host == p.DashboardURL.Host {
			httpmw.RedirectToLogin(rw, r, p.DashboardURL, httpmw.SignedOutErrorMessage)
			return nil, "", false
		}

		// Otherwise, we need to redirect to the app auth endpoint, which will
		// redirect back to the app (with an encrypted API key) after the user
		// has logged in.
		//
		// TODO: We should just make this a "BrowserURL" field on the issue struct. Then
		// we can remove this logic and just defer to that. It can be set closer to the
		// actual initial request that makes the IssueTokenRequest. Eg the external moon.
		// This would replace RawQuery and AppPath fields.
		redirectURI := *appBaseURL
		if dbReq.AppURL != nil {
			// Just use the user's current path and query if set.
			if issueReq.AppPath != "" {
				redirectURI.Path = path.Join(redirectURI.Path, issueReq.AppPath)
			} else if !strings.HasSuffix(redirectURI.Path, "/") {
				redirectURI.Path += "/"
			}
			q := issueReq.AppQuery
			if q != "" && dbReq.AppURL.RawQuery != "" {
				q = dbReq.AppURL.RawQuery
			}
			redirectURI.RawQuery = q
		}

		// This endpoint accepts redirect URIs from the primary app wildcard
		// host, proxy access URLs and proxy wildcard app hosts. It does not
		// accept redirect URIs from the primary access URL or any other host.
		u := *p.DashboardURL
		u.Path = "/api/v2/applications/auth-redirect"
		q := u.Query()
		q.Add(RedirectURIQueryParam, redirectURI.String())
		u.RawQuery = q.Encode()

		http.Redirect(rw, r, u.String(), http.StatusSeeOther)
		return nil, "", false
	}

	// Check that the agent is online.
	agentStatus := dbReq.Agent.Status(p.WorkspaceAgentInactiveTimeout)
	if agentStatus.Status != database.WorkspaceAgentStatusConnected {
		WriteWorkspaceAppOffline(p.Logger, p.DashboardURL, rw, r, &appReq, fmt.Sprintf("Agent state is %q, not %q", agentStatus.Status, database.WorkspaceAgentStatusConnected))
		return nil, "", false
	}

	// This is where we used to check app health, but we don't do that anymore
	// in case there are bugs with the healthcheck code that lock users out of
	// their apps completely.

	// As a sanity check, ensure the token we just made is valid for this
	// request.
	if !token.MatchesRequest(appReq) {
		WriteWorkspaceApp500(p.Logger, p.DashboardURL, rw, r, &appReq, nil, "fresh token does not match request")
		return nil, "", false
	}

	token.RegisteredClaims = jwtutils.RegisteredClaims{
		Expiry: jwt.NewNumericDate(time.Now().Add(DefaultTokenExpiry)),
	}
	// Sign the token.
	tokenStr, err := jwtutils.Sign(ctx, p.Keycache, token)
	if err != nil {
		WriteWorkspaceApp500(p.Logger, p.DashboardURL, rw, r, &appReq, err, "generate token")
		return nil, "", false
	}

	return &token, tokenStr, true
}

// authorizeRequest returns true if the request is authorized. The returned []string
// are warnings that aid in debugging. These messages do not prevent authorization,
// but may indicate that the request is not configured correctly.
// If an error is returned, the request should be aborted with a 500 error.
func (p *DBTokenProvider) authorizeRequest(ctx context.Context, roles *rbac.Subject, dbReq *databaseRequest) (bool, []string, error) {
	var warnings []string
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
		if dbReq.AppSharingLevel != database.AppSharingLevelOwner {
			// This is helpful for debugging, and ok to leak to the user.
			// This is because the app has the sharing level set to something that
			// should be shared, but we are disabling it from a deployment wide
			// flag. So the template should be fixed to set the sharing level to
			// "owner" instead and this will not appear.
			warnings = append(warnings, fmt.Sprintf("unable to use configured sharing level %q because path-based app sharing is disabled (see --dangerous-allow-path-app-sharing), using sharing level \"owner\" instead", sharingLevel))
		}
		sharingLevel = database.AppSharingLevelOwner
	}

	// Short circuit if not authenticated.
	if roles == nil {
		// The user is not authenticated, so they can only access the app if it
		// is public.
		return sharingLevel == database.AppSharingLevelPublic, warnings, nil
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
		dbReq.Workspace.OwnerID.String() != roles.ID &&
		!p.DeploymentValues.Dangerous.AllowPathAppSiteOwnerAccess.Value() {
		// This is not ideal to check for the 'owner' role, but we are only checking
		// to determine whether to show a warning for debugging reasons. This does
		// not do any authz checks, so it is ok.
		if slices.Contains(roles.Roles.Names(), rbac.RoleOwner()) {
			warnings = append(warnings, "path-based apps with \"owner\" share level are only accessible by the workspace owner (see --dangerous-allow-path-app-site-owner-access)")
		}
		return false, warnings, nil
	}

	// Figure out which RBAC resource to check. For terminals we use execution
	// instead of application connect.
	var (
		rbacAction   policy.Action = policy.ActionApplicationConnect
		rbacResource rbac.Object   = dbReq.Workspace.RBACObject()
		// rbacResourceOwned is for the level "authenticated". We still need to
		// make sure the API key has permissions to connect to the actor's own
		// workspace. Scopes would prevent this.
		// TODO: This is an odd repercussion of the org_member permission level.
		// This Object used to not specify an org restriction, and `InOrg` would
		// actually have a significantly different meaning (only sharing with
		// other authenticated users in the same org, whereas the existing behavior
		// is to share with any authenticated user). Because workspaces are always
		// jointly owned by an organization, there _must_ be an org restriction on
		// the object to check the proper permissions. AnyOrg is almost the same,
		// but technically excludes users who are not in any organization. This is
		// the closest we can get though without more significant refactoring.
		rbacResourceOwned rbac.Object = rbac.ResourceWorkspace.WithOwner(roles.ID).AnyOrganization()
	)
	if dbReq.AccessMethod == AccessMethodTerminal {
		rbacAction = policy.ActionSSH
	}

	// Do a standard RBAC check. This accounts for share level "owner" and any
	// other RBAC rules that may be in place.
	//
	// Regardless of share level or whether it's enabled or not, the owner of
	// the workspace can always access applications (as long as their API key's
	// scope allows it).
	err := p.Authorizer.Authorize(ctx, *roles, rbacAction, rbacResource)
	if err == nil {
		return true, []string{}, nil
	}

	switch sharingLevel {
	case database.AppSharingLevelOwner:
		// We essentially already did this above with the regular RBAC check.
		// Owners can always access their own apps according to RBAC rules, so
		// they have already been returned from this function.
	case database.AppSharingLevelAuthenticated:
		// Check with the owned resource to ensure the API key has permissions
		// to connect to the actor's own workspace. This enforces scopes.
		err := p.Authorizer.Authorize(ctx, *roles, rbacAction, rbacResourceOwned)
		if err == nil {
			return true, []string{}, nil
		}
	case database.AppSharingLevelOrganization:
		// First check if they have permission to connect to their own workspace (enforces scopes)
		err := p.Authorizer.Authorize(ctx, *roles, rbacAction, rbacResourceOwned)
		if err != nil {
			return false, warnings, nil
		}

		// Check if the user is a member of the same organization as the workspace
		workspaceOrgID := dbReq.Workspace.OrganizationID
		expandedRoles, err := roles.Roles.Expand()
		if err != nil {
			return false, warnings, xerrors.Errorf("expand roles: %w", err)
		}
		for _, role := range expandedRoles {
			if _, ok := role.ByOrgID[workspaceOrgID.String()]; ok {
				return true, []string{}, nil
			}
		}
		// User is not a member of the workspace's organization
		return false, warnings, nil
	case database.AppSharingLevelPublic:
		// We don't really care about scopes and stuff if it's public anyways.
		// Someone with a restricted-scope API key could just not submit the API
		// key cookie in the request and access the page.
		return true, []string{}, nil
	}

	// No checks were successful.
	return false, warnings, nil
}

type connLogRequest struct {
	time   time.Time
	apiKey *database.APIKey
	dbReq  *databaseRequest
}

// connLogInitRequest creates a new connection log session and connect log for the
// given request, if one does not already exist. If a connection log session
// already exists, it will be updated with the current timestamp. A session is used to
// reduce the number of connection logs created.
//
// A session is unique to the agent, app, user and users IP. If any of these
// values change, a new session and connect log is created.
func (p *DBTokenProvider) connLogInitRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) (aReq *connLogRequest, commit func()) {
	// Get the status writer from the request context so we can figure
	// out the HTTP status and autocommit the audit log.
	sw, ok := w.(*tracing.StatusWriter)
	if !ok {
		panic("dev error: http.ResponseWriter is not *tracing.StatusWriter")
	}

	aReq = &connLogRequest{
		time: dbtime.Time(p.Clock.Now()),
	}

	// Set the commit function on the status writer to create a connection log
	// this ensures that the status and response body are available.
	var committed bool
	return aReq, func() {
		if committed {
			return
		}
		committed = true

		if aReq.dbReq == nil {
			// App doesn't exist, there's information in the Request
			// struct but we need UUIDs for connection logging.
			return
		}

		userID := uuid.Nil
		if aReq.apiKey != nil {
			userID = aReq.apiKey.UserID
		}
		userAgent := r.UserAgent()
		ip := r.RemoteAddr

		// Approximation of the status code.
		// #nosec G115 - Safe conversion as HTTP status code is expected to be within int32 range (typically 100-599)
		var statusCode int32 = int32(sw.Status)
		if statusCode == 0 {
			statusCode = http.StatusOK
		}

		var (
			connType   database.ConnectionType
			slugOrPort = aReq.dbReq.AppSlugOrPort
		)

		switch {
		case aReq.dbReq.AccessMethod == AccessMethodTerminal:
			connType = database.ConnectionTypeWorkspaceApp
			slugOrPort = "terminal"
		case aReq.dbReq.App.ID == uuid.Nil:
			connType = database.ConnectionTypePortForwarding
		default:
			connType = database.ConnectionTypeWorkspaceApp
		}

		// If we end up logging, ensure relevant fields are set.
		logger := p.Logger.With(
			slog.F("workspace_id", aReq.dbReq.Workspace.ID),
			slog.F("agent_id", aReq.dbReq.Agent.ID),
			slog.F("app_id", aReq.dbReq.App.ID),
			slog.F("user_id", userID),
			slog.F("user_agent", userAgent),
			slog.F("app_slug_or_port", slugOrPort),
			slog.F("status_code", statusCode),
		)

		var newOrStale bool
		err := p.Database.InTx(func(tx database.Store) (err error) {
			// nolint:gocritic // System context is needed to write audit sessions.
			dangerousSystemCtx := dbauthz.AsSystemRestricted(ctx)

			newOrStale, err = tx.UpsertWorkspaceAppAuditSession(dangerousSystemCtx, database.UpsertWorkspaceAppAuditSessionParams{
				// Config.
				StaleIntervalMS: p.WorkspaceAppAuditSessionTimeout.Milliseconds(),
				Now:             aReq.time,

				// Data.
				ID:         uuid.New(),
				AgentID:    aReq.dbReq.Agent.ID,
				AppID:      aReq.dbReq.App.ID, // Can be unset, in which case uuid.Nil is fine.
				UserID:     userID,            // Can be unset, in which case uuid.Nil is fine.
				Ip:         ip,
				UserAgent:  userAgent,
				SlugOrPort: slugOrPort,
				StatusCode: statusCode,
				StartedAt:  aReq.time,
				UpdatedAt:  aReq.time,
			})
			if err != nil {
				return xerrors.Errorf("insert workspace app audit session: %w", err)
			}

			return nil
		}, nil)
		if err != nil {
			logger.Error(ctx, "update workspace app audit session failed", slog.Error(err))

			// Avoid spamming the connection log if deduplication failed, this should
			// only happen if there are problems communicating with the database.
			return
		}

		if !newOrStale {
			// We either didn't insert a new session, or the session
			// didn't timeout due to inactivity.
			return
		}

		connLogger := *p.ConnectionLogger.Load()

		err = connLogger.Upsert(ctx, database.UpsertConnectionLogParams{
			ID:               uuid.New(),
			Time:             aReq.time,
			OrganizationID:   aReq.dbReq.Workspace.OrganizationID,
			WorkspaceOwnerID: aReq.dbReq.Workspace.OwnerID,
			WorkspaceID:      aReq.dbReq.Workspace.ID,
			WorkspaceName:    aReq.dbReq.Workspace.Name,
			AgentName:        aReq.dbReq.Agent.Name,
			Type:             connType,
			Code: sql.NullInt32{
				Int32: statusCode,
				Valid: true,
			},
			Ip:        database.ParseIP(ip),
			UserAgent: sql.NullString{Valid: userAgent != "", String: userAgent},
			UserID: uuid.NullUUID{
				UUID:  userID,
				Valid: userID != uuid.Nil,
			},
			SlugOrPort:       sql.NullString{Valid: slugOrPort != "", String: slugOrPort},
			ConnectionStatus: database.ConnectionStatusConnected,

			// N/A
			ConnectionID:     uuid.NullUUID{},
			DisconnectReason: sql.NullString{},
		})
		if err != nil {
			logger.Error(ctx, "upsert connection log failed", slog.Error(err))
			return
		}
	}
}
