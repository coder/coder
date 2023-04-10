package workspaceapps

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// DBTokenProvider provides authentication and authorization for workspace apps
// by querying the database if the request is missing a valid token.
type DBTokenProvider struct {
	Logger slog.Logger

	// AccessURL is the main dashboard access URL for error pages.
	AccessURL                     *url.URL
	Authorizer                    rbac.Authorizer
	Database                      database.Store
	DeploymentValues              *codersdk.DeploymentValues
	OAuth2Configs                 *httpmw.OAuth2Configs
	WorkspaceAgentInactiveTimeout time.Duration
	SigningKey                    SecurityKey
}

var _ SignedTokenProvider = &DBTokenProvider{}

func NewDBTokenProvider(log slog.Logger, accessURL *url.URL, authz rbac.Authorizer, db database.Store, cfg *codersdk.DeploymentValues, oauth2Cfgs *httpmw.OAuth2Configs, workspaceAgentInactiveTimeout time.Duration, signingKey SecurityKey) SignedTokenProvider {
	if workspaceAgentInactiveTimeout == 0 {
		workspaceAgentInactiveTimeout = 1 * time.Minute
	}

	return &DBTokenProvider{
		Logger:                        log,
		AccessURL:                     accessURL,
		Authorizer:                    authz,
		Database:                      db,
		DeploymentValues:              cfg,
		OAuth2Configs:                 oauth2Cfgs,
		WorkspaceAgentInactiveTimeout: workspaceAgentInactiveTimeout,
		SigningKey:                    signingKey,
	}
}

func (p *DBTokenProvider) TokenFromRequest(r *http.Request) (*SignedToken, bool) {
	// Get the existing token from the request.
	tokenCookie, err := r.Cookie(codersdk.DevURLSignedAppTokenCookie)
	if err == nil {
		token, err := p.SigningKey.VerifySignedToken(tokenCookie.Value)
		if err == nil {
			req := token.Request.Normalize()
			err := req.Validate()
			if err == nil {
				// The request has a valid signed app token, which is a valid
				// token signed by us. The caller must check that it matches
				// the request.
				return &token, true
			}
		}
	}

	return nil, false
}

// ResolveRequest takes an app request, checks if it's valid and authenticated,
// and returns a token with details about the app.
func (p *DBTokenProvider) CreateToken(ctx context.Context, rw http.ResponseWriter, r *http.Request, appReq Request) (*SignedToken, string, bool) {
	// nolint:gocritic // We need to make a number of database calls. Setting a system context here
	//                 // is simpler than calling dbauthz.AsSystemRestricted on every call.
	//                 // dangerousSystemCtx is only used for database calls. The actual authentication
	//                 // logic is handled in Provider.authorizeWorkspaceApp which directly checks the actor's
	//                 // permissions.
	dangerousSystemCtx := dbauthz.AsSystemRestricted(ctx)

	appReq = appReq.Normalize()
	err := appReq.Validate()
	if err != nil {
		WriteWorkspaceApp500(p.Logger, p.AccessURL, rw, r, &appReq, err, "invalid app request")
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
		WriteWorkspaceApp404(p.Logger, p.AccessURL, rw, r, &appReq, err.Error())
		return nil, "", false
	} else if err != nil {
		WriteWorkspaceApp500(p.Logger, p.AccessURL, rw, r, &appReq, err, "get app details from database")
		return nil, "", false
	}
	token.UserID = dbReq.User.ID
	token.WorkspaceID = dbReq.Workspace.ID
	token.AgentID = dbReq.Agent.ID
	token.AppURL = dbReq.AppURL

	// Verify the user has access to the app.
	authed, err := p.authorizeRequest(r.Context(), authz, dbReq)
	if err != nil {
		WriteWorkspaceApp500(p.Logger, p.AccessURL, rw, r, &appReq, err, "verify authz")
		return nil, "", false
	}
	if !authed {
		if apiKey != nil {
			// The request has a valid API key but insufficient permissions.
			WriteWorkspaceApp404(p.Logger, p.AccessURL, rw, r, &appReq, "insufficient permissions")
			return nil, "", false
		}

		// Redirect to login as they don't have permission to access the app
		// and they aren't signed in.
		switch appReq.AccessMethod {
		case AccessMethodPath:
			// TODO(@deansheather): this doesn't work on moons so will need to
			// be updated to include the access URL as a param
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
		WriteWorkspaceAppOffline(p.Logger, p.AccessURL, rw, r, &appReq, fmt.Sprintf("Agent state is %q, not %q", agentStatus.Status, database.WorkspaceAgentStatusConnected))
		return nil, "", false
	}

	// Check that the app is healthy.
	if dbReq.AppHealth != "" && dbReq.AppHealth != database.WorkspaceAppHealthDisabled && dbReq.AppHealth != database.WorkspaceAppHealthHealthy {
		WriteWorkspaceAppOffline(p.Logger, p.AccessURL, rw, r, &appReq, fmt.Sprintf("App health is %q, not %q", dbReq.AppHealth, database.WorkspaceAppHealthHealthy))
		return nil, "", false
	}

	// As a sanity check, ensure the token we just made is valid for this
	// request.
	if !token.MatchesRequest(appReq) {
		WriteWorkspaceApp500(p.Logger, p.AccessURL, rw, r, &appReq, nil, "fresh token does not match request")
		return nil, "", false
	}

	// Sign the token.
	token.Expiry = time.Now().Add(DefaultTokenExpiry)
	tokenStr, err := p.SigningKey.SignToken(token)
	if err != nil {
		WriteWorkspaceApp500(p.Logger, p.AccessURL, rw, r, &appReq, err, "generate token")
		return nil, "", false
	}

	return &token, tokenStr, true
}

func (p *DBTokenProvider) authorizeRequest(ctx context.Context, roles *httpmw.Authorization, dbReq *databaseRequest) (bool, error) {
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
