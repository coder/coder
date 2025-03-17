package workspaceapps

import (
	"context"
	"database/sql"
	"encoding/json"
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
	"github.com/coder/coder/v2/coderd/audit"
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
)

// DBTokenProvider provides authentication and authorization for workspace apps
// by querying the database if the request is missing a valid token.
type DBTokenProvider struct {
	Logger slog.Logger

	// DashboardURL is the main dashboard access URL for error pages.
	DashboardURL                    *url.URL
	Authorizer                      rbac.Authorizer
	Auditor                         *atomic.Pointer[audit.Auditor]
	Database                        database.Store
	DeploymentValues                *codersdk.DeploymentValues
	OAuth2Configs                   *httpmw.OAuth2Configs
	WorkspaceAgentInactiveTimeout   time.Duration
	WorkspaceAppAuditSessionTimeout time.Duration
	Keycache                        cryptokeys.SigningKeycache
}

var _ SignedTokenProvider = &DBTokenProvider{}

func NewDBTokenProvider(log slog.Logger,
	accessURL *url.URL,
	authz rbac.Authorizer,
	auditor *atomic.Pointer[audit.Auditor],
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
		Auditor:                         auditor,
		Database:                        db,
		DeploymentValues:                cfg,
		OAuth2Configs:                   oauth2Cfgs,
		WorkspaceAgentInactiveTimeout:   workspaceAgentInactiveTimeout,
		WorkspaceAppAuditSessionTimeout: workspaceAppAuditSessionTimeout,
		Keycache:                        signer,
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

	aReq, commitAudit := p.auditInitRequest(ctx, rw, r)
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
		SessionTokenFunc: func(r *http.Request) string {
			return issueReq.SessionToken
		},
	})
	if !ok {
		return nil, "", false
	}

	aReq.apiKey = apiKey // Update audit request.

	// Lookup workspace app details from DB.
	dbReq, err := appReq.getDatabase(dangerousSystemCtx, p.Database)
	if xerrors.Is(err, sql.ErrNoRows) {
		WriteWorkspaceApp404(p.Logger, p.DashboardURL, rw, r, &appReq, nil, err.Error())
		return nil, "", false
	} else if xerrors.Is(err, errWorkspaceStopped) {
		WriteWorkspaceOffline(p.Logger, p.DashboardURL, rw, r, &appReq)
		return nil, "", false
	} else if err != nil {
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

// authorizeRequest returns true/false if the request is authorized. The returned []string
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
		if roles != nil && slices.Contains(roles.Roles.Names(), rbac.RoleOwner()) {
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
		rbacResourceOwned rbac.Object = rbac.ResourceWorkspace.WithOwner(roles.ID)
	)
	if dbReq.AccessMethod == AccessMethodTerminal {
		rbacAction = policy.ActionSSH
		rbacResourceOwned = rbac.ResourceWorkspace.WithOwner(roles.ID)
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
	case database.AppSharingLevelPublic:
		// We don't really care about scopes and stuff if it's public anyways.
		// Someone with a restricted-scope API key could just not submit the API
		// key cookie in the request and access the page.
		return true, []string{}, nil
	}

	// No checks were successful.
	return false, warnings, nil
}

type auditRequest struct {
	time   time.Time
	apiKey *database.APIKey
	dbReq  *databaseRequest
}

// auditInitRequest creates a new audit session and audit log for the given
// request, if one does not already exist. If an audit session already exists,
// it will be updated with the current timestamp. A session is used to reduce
// the number of audit logs created.
//
// A session is unique to the agent, app, user and users IP. If any of these
// values change, a new session and audit log is created.
func (p *DBTokenProvider) auditInitRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) (aReq *auditRequest, commit func()) {
	// Get the status writer from the request context so we can figure
	// out the HTTP status and autocommit the audit log.
	sw, ok := w.(*tracing.StatusWriter)
	if !ok {
		panic("dev error: http.ResponseWriter is not *tracing.StatusWriter")
	}

	aReq = &auditRequest{
		time: dbtime.Now(),
	}

	// Set the commit function on the status writer to create an audit
	// log, this ensures that the status and response body are available.
	var committed bool
	return aReq, func() {
		if committed {
			return
		}
		committed = true

		if aReq.dbReq == nil {
			// App doesn't exist, there's information in the Request
			// struct but we need UUIDs for audit logging.
			return
		}

		userID := uuid.Nil
		if aReq.apiKey != nil {
			userID = aReq.apiKey.UserID
		}
		userAgent := r.UserAgent()
		ip := r.RemoteAddr

		// Approximation of the status code.
		statusCode := sw.Status
		if statusCode == 0 {
			statusCode = http.StatusOK
		}

		type additionalFields struct {
			audit.AdditionalFields
			SlugOrPort string `json:"slug_or_port,omitempty"`
		}
		appInfo := additionalFields{
			AdditionalFields: audit.AdditionalFields{
				WorkspaceOwner: aReq.dbReq.Workspace.OwnerUsername,
				WorkspaceName:  aReq.dbReq.Workspace.Name,
				WorkspaceID:    aReq.dbReq.Workspace.ID,
			},
		}
		switch {
		case aReq.dbReq.AccessMethod == AccessMethodTerminal:
			appInfo.SlugOrPort = "terminal"
		case aReq.dbReq.App.ID == uuid.Nil:
			// If this isn't an app or a terminal, it's a port.
			appInfo.SlugOrPort = aReq.dbReq.AppSlugOrPort
		}

		// If we end up logging, ensure relevant fields are set.
		logger := p.Logger.With(
			slog.F("workspace_id", aReq.dbReq.Workspace.ID),
			slog.F("agent_id", aReq.dbReq.Agent.ID),
			slog.F("app_id", aReq.dbReq.App.ID),
			slog.F("user_id", userID),
			slog.F("user_agent", userAgent),
			slog.F("app_slug_or_port", appInfo.SlugOrPort),
			slog.F("status_code", statusCode),
		)

		var startedAt time.Time
		err := p.Database.InTx(func(tx database.Store) (err error) {
			// nolint:gocritic // System context is needed to write audit sessions.
			dangerousSystemCtx := dbauthz.AsSystemRestricted(ctx)

			startedAt, err = tx.UpsertWorkspaceAppAuditSession(dangerousSystemCtx, database.UpsertWorkspaceAppAuditSessionParams{
				// Config.
				StaleIntervalMS: p.WorkspaceAppAuditSessionTimeout.Milliseconds(),

				// Data.
				AgentID:    aReq.dbReq.Agent.ID,
				AppID:      aReq.dbReq.App.ID, // Can be unset, in which case uuid.Nil is fine.
				UserID:     userID,            // Can be unset, in which case uuid.Nil is fine.
				Ip:         ip,
				UserAgent:  userAgent,
				SlugOrPort: appInfo.SlugOrPort,
				StatusCode: int32(statusCode),
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

			// Avoid spamming the audit log if deduplication failed, this should
			// only happen if there are problems communicating with the database.
			return
		}

		if !startedAt.Equal(aReq.time) {
			// If the unique session wasn't renewed, we don't want to log a new
			// audit event for it.
			return
		}

		// Marshal additional fields only if we're writing an audit log entry.
		appInfoBytes, err := json.Marshal(appInfo)
		if err != nil {
			logger.Error(ctx, "marshal additional fields failed", slog.Error(err))
		}

		// We use the background audit function instead of init request
		// here because we don't know the resource type ahead of time.
		// This also allows us to log unauthenticated access.
		auditor := *p.Auditor.Load()
		requestID := httpmw.RequestID(r)
		switch {
		case aReq.dbReq.App.ID != uuid.Nil:
			audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.WorkspaceApp]{
				Audit: auditor,
				Log:   logger,

				Action:           database.AuditActionOpen,
				OrganizationID:   aReq.dbReq.Workspace.OrganizationID,
				UserID:           userID,
				RequestID:        requestID,
				Time:             aReq.time,
				Status:           statusCode,
				IP:               ip,
				UserAgent:        userAgent,
				New:              aReq.dbReq.App,
				AdditionalFields: appInfoBytes,
			})
		default:
			// Web terminal, port app, etc.
			audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.WorkspaceAgent]{
				Audit: auditor,
				Log:   logger,

				Action:           database.AuditActionOpen,
				OrganizationID:   aReq.dbReq.Workspace.OrganizationID,
				UserID:           userID,
				RequestID:        requestID,
				Time:             aReq.time,
				Status:           statusCode,
				IP:               ip,
				UserAgent:        userAgent,
				New:              aReq.dbReq.Agent,
				AdditionalFields: appInfoBytes,
			})
		}
	}
}
