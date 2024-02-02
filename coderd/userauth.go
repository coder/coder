package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/parameter"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/site"
)

const (
	userAuthLoggerName      = "userauth"
	OAuthConvertCookieValue = "coder_oauth_convert_jwt"
	mergeStateStringPrefix  = "convert-"
)

type OAuthConvertStateClaims struct {
	jwt.RegisteredClaims

	UserID        uuid.UUID          `json:"user_id"`
	State         string             `json:"state"`
	FromLoginType codersdk.LoginType `json:"from_login_type"`
	ToLoginType   codersdk.LoginType `json:"to_login_type"`
}

// postConvertLoginType replies with an oauth state token capable of converting
// the user to an oauth user.
//
// @Summary Convert user from password to oauth authentication
// @ID convert-user-from-password-to-oauth-authentication
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Authorization
// @Param request body codersdk.ConvertLoginRequest true "Convert request"
// @Param user path string true "User ID, name, or me"
// @Success 201 {object} codersdk.OAuthConversionResponse
// @Router /users/{user}/convert-login [post]
func (api *API) postConvertLoginType(rw http.ResponseWriter, r *http.Request) {
	var (
		user              = httpmw.UserParam(r)
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditOAuthConvertState](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	aReq.Old = database.AuditOAuthConvertState{}
	defer commitAudit()

	var req codersdk.ConvertLoginRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	switch req.ToType {
	case codersdk.LoginTypeGithub, codersdk.LoginTypeOIDC:
		// Allowed!
	case codersdk.LoginTypeNone, codersdk.LoginTypePassword, codersdk.LoginTypeToken:
		// These login types are not allowed to be converted to at this time.
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Cannot convert to login type %q.", req.ToType),
		})
		return
	default:
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Unknown login type %q.", req.ToType),
		})
		return
	}

	// This handles the email/pass checking.
	user, _, ok := api.loginRequest(ctx, rw, codersdk.LoginWithPasswordRequest{
		Email:    user.Email,
		Password: req.Password,
	})
	if !ok {
		return
	}

	// Only support converting from password auth.
	if user.LoginType != database.LoginTypePassword {
		// This is checked in loginRequest, but checked again here in case that shared
		// function changes its checks. Just some defensive programming.
		// This login type is **required** to be password based to prevent
		// users from converting other login types to OIDC.
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "User account must have password based authentication.",
		})
		return
	}

	stateString, err := cryptorand.String(32)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error generating state string.",
			Detail:  err.Error(),
		})
		return
	}
	// The prefix is used to identify this state string as a conversion state
	// without needing to hit the database. The random string is the CSRF protection.
	stateString = fmt.Sprintf("%s%s", mergeStateStringPrefix, stateString)

	// This JWT is the signed payload to authorize the convert to oauth request.
	// When the user does the oauth flow, this jwt will be sent back to coderd.
	// The included information in this payload links it to a state string, so
	// this request is tied 1:1 with an oauth state.
	// This JWT also includes information to tie it 1:1 with a coder deployment
	// and user account. This is mainly to inform the user if they are accidentally
	// switching between coder deployments if the OIDC is misconfigured.
	// Eg: Developers with more than 1 deployment.
	now := time.Now()
	claims := &OAuthConvertStateClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    api.DeploymentID,
			Subject:   stateString,
			Audience:  []string{user.ID.String()},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute * 5)),
			NotBefore: jwt.NewNumericDate(now.Add(time.Second * -1)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
		UserID:        user.ID,
		State:         stateString,
		FromLoginType: codersdk.LoginType(user.LoginType),
		ToLoginType:   req.ToType,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	// Key must be a byte slice, not an array. So make sure to include the [:]
	tokenString, err := token.SignedString(api.OAuthSigningKey[:])
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error signing state jwt.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = database.AuditOAuthConvertState{
		CreatedAt:     claims.IssuedAt.Time,
		ExpiresAt:     claims.ExpiresAt.Time,
		FromLoginType: database.LoginType(claims.FromLoginType),
		ToLoginType:   database.LoginType(claims.ToLoginType),
		UserID:        claims.UserID,
	}

	http.SetCookie(rw, &http.Cookie{
		Name:     OAuthConvertCookieValue,
		Path:     "/",
		Value:    tokenString,
		Expires:  claims.ExpiresAt.Time,
		Secure:   api.SecureAuthCookie,
		HttpOnly: true,
		// Must be SameSite to work on the redirected auth flow from the
		// oauth provider.
		SameSite: http.SameSiteLaxMode,
	})
	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.OAuthConversionResponse{
		StateString: stateString,
		ExpiresAt:   claims.ExpiresAt.Time,
		ToType:      claims.ToLoginType,
		UserID:      claims.UserID,
	})
}

// Authenticates the user with an email and password.
//
// @Summary Log in user
// @ID log-in-user
// @Accept json
// @Produce json
// @Tags Authorization
// @Param request body codersdk.LoginWithPasswordRequest true "Login request"
// @Success 201 {object} codersdk.LoginWithPasswordResponse
// @Router /users/login [post]
func (api *API) postLogin(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		logger            = api.Logger.Named(userAuthLoggerName)
		aReq, commitAudit = audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionLogin,
		})
	)
	aReq.Old = database.APIKey{}
	defer commitAudit()

	var loginWithPassword codersdk.LoginWithPasswordRequest
	if !httpapi.Read(ctx, rw, r, &loginWithPassword) {
		return
	}

	user, roles, ok := api.loginRequest(ctx, rw, loginWithPassword)
	// 'user.ID' will be empty, or will be an actual value. Either is correct
	// here.
	aReq.UserID = user.ID
	if !ok {
		// user failed to login
		return
	}

	userSubj := rbac.Subject{
		ID:     user.ID.String(),
		Roles:  rbac.RoleNames(roles.Roles),
		Groups: roles.Groups,
		Scope:  rbac.ScopeAll,
	}

	//nolint:gocritic // Creating the API key as the user instead of as system.
	cookie, key, err := api.createAPIKey(dbauthz.As(ctx, userSubj), apikey.CreateParams{
		UserID:          user.ID,
		LoginType:       database.LoginTypePassword,
		RemoteAddr:      r.RemoteAddr,
		DefaultLifetime: api.DeploymentValues.SessionDuration.Value(),
	})
	if err != nil {
		logger.Error(ctx, "unable to create API key", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create API key.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = *key

	http.SetCookie(rw, cookie)

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.LoginWithPasswordResponse{
		SessionToken: cookie.Value,
	})
}

// loginRequest will process a LoginWithPasswordRequest and return the user if
// the credentials are correct. If 'false' is returned, the authentication failed
// and the appropriate error will be written to the ResponseWriter.
//
// The user struct is always returned, even if authentication failed. This is
// to support knowing what user attempted to login.
func (api *API) loginRequest(ctx context.Context, rw http.ResponseWriter, req codersdk.LoginWithPasswordRequest) (database.User, database.GetAuthorizationUserRolesRow, bool) {
	logger := api.Logger.Named(userAuthLoggerName)

	//nolint:gocritic // In order to login, we need to get the user first!
	user, err := api.Database.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
		Email: req.Email,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		logger.Error(ctx, "unable to fetch user by email", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return user, database.GetAuthorizationUserRolesRow{}, false
	}

	// If the user doesn't exist, it will be a default struct.
	equal, err := userpassword.Compare(string(user.HashedPassword), req.Password)
	if err != nil {
		logger.Error(ctx, "unable to compare passwords", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return user, database.GetAuthorizationUserRolesRow{}, false
	}

	if !equal {
		// This message is the same as above to remove ease in detecting whether
		// users are registered or not. Attackers still could with a timing attack.
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Incorrect email or password.",
		})
		return user, database.GetAuthorizationUserRolesRow{}, false
	}

	// If password authentication is disabled and the user does not have the
	// owner role, block the request.
	if api.DeploymentValues.DisablePasswordAuth {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Password authentication is disabled.",
		})
		return user, database.GetAuthorizationUserRolesRow{}, false
	}

	if user.LoginType != database.LoginTypePassword {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: fmt.Sprintf("Incorrect login type, attempting to use %q but user is of login type %q", database.LoginTypePassword, user.LoginType),
		})
		return user, database.GetAuthorizationUserRolesRow{}, false
	}

	if user.Status == database.UserStatusDormant {
		//nolint:gocritic // System needs to update status of the user account (dormant -> active).
		user, err = api.Database.UpdateUserStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateUserStatusParams{
			ID:        user.ID,
			Status:    database.UserStatusActive,
			UpdatedAt: dbtime.Now(),
		})
		if err != nil {
			logger.Error(ctx, "unable to update user status to active", slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error occurred. Try again later, or contact an admin for assistance.",
			})
			return user, database.GetAuthorizationUserRolesRow{}, false
		}
	}

	//nolint:gocritic // System needs to fetch user roles in order to login user.
	roles, err := api.Database.GetAuthorizationUserRoles(dbauthz.AsSystemRestricted(ctx), user.ID)
	if err != nil {
		logger.Error(ctx, "unable to fetch authorization user roles", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return user, database.GetAuthorizationUserRolesRow{}, false
	}

	// If the user logged into a suspended account, reject the login request.
	if roles.Status != database.UserStatusActive {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: fmt.Sprintf("Your account is %s. Contact an admin to reactivate your account.", roles.Status),
		})
		return user, database.GetAuthorizationUserRolesRow{}, false
	}

	return user, roles, true
}

// Clear the user's session cookie.
//
// @Summary Log out user
// @ID log-out-user
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Success 200 {object} codersdk.Response
// @Router /users/logout [post]
func (api *API) postLogout(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionLogout,
		})
	)
	defer commitAudit()

	// Get a blank token cookie.
	cookie := &http.Cookie{
		// MaxAge < 0 means to delete the cookie now.
		MaxAge: -1,
		Name:   codersdk.SessionTokenCookie,
		Path:   "/",
	}
	http.SetCookie(rw, cookie)

	// Delete the session token from database.
	apiKey := httpmw.APIKey(r)
	aReq.Old = apiKey

	logger := api.Logger.Named(userAuthLoggerName)

	err := api.Database.DeleteAPIKeyByID(ctx, apiKey.ID)
	if err != nil {
		logger.Error(ctx, "unable to delete API key", slog.F("api_key", apiKey.ID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting API key.",
			Detail:  err.Error(),
		})
		return
	}

	// Invalidate all subdomain app tokens. This saves us from having to
	// track which app tokens are associated which this browser session and
	// doesn't inconvenience the user as they'll just get redirected if they try
	// to access the app again.
	err = api.Database.DeleteApplicationConnectAPIKeysByUserID(ctx, apiKey.UserID)
	if err != nil {
		logger.Error(ctx, "unable to invalidate subdomain app tokens", slog.F("user_id", apiKey.UserID), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting app tokens.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = database.APIKey{}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Logged out!",
	})
}

// GithubOAuth2Team represents a team scoped to an organization.
type GithubOAuth2Team struct {
	Organization string
	Slug         string
}

// GithubOAuth2Provider exposes required functions for the Github authentication flow.
type GithubOAuth2Config struct {
	promoauth.OAuth2Config
	AuthenticatedUser           func(ctx context.Context, client *http.Client) (*github.User, error)
	ListEmails                  func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error)
	ListOrganizationMemberships func(ctx context.Context, client *http.Client) ([]*github.Membership, error)
	TeamMembership              func(ctx context.Context, client *http.Client, org, team, username string) (*github.Membership, error)

	AllowSignups       bool
	AllowEveryone      bool
	AllowOrganizations []string
	AllowTeams         []GithubOAuth2Team
}

// @Summary Get authentication methods
// @ID get-authentication-methods
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Success 200 {object} codersdk.AuthMethods
// @Router /users/authmethods [get]
func (api *API) userAuthMethods(rw http.ResponseWriter, r *http.Request) {
	var signInText string
	var iconURL string

	if api.OIDCConfig != nil {
		signInText = api.OIDCConfig.SignInText
	}
	if api.OIDCConfig != nil {
		iconURL = api.OIDCConfig.IconURL
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.AuthMethods{
		Password: codersdk.AuthMethod{
			Enabled: !api.DeploymentValues.DisablePasswordAuth.Value(),
		},
		Github: codersdk.AuthMethod{Enabled: api.GithubOAuth2Config != nil},
		OIDC: codersdk.OIDCAuthMethod{
			AuthMethod: codersdk.AuthMethod{Enabled: api.OIDCConfig != nil},
			SignInText: signInText,
			IconURL:    iconURL,
		},
	})
}

// @Summary OAuth 2.0 GitHub Callback
// @ID oauth-20-github-callback
// @Security CoderSessionToken
// @Tags Users
// @Success 307
// @Router /users/oauth2/github/callback [get]
func (api *API) userOAuth2Github(rw http.ResponseWriter, r *http.Request) {
	var (
		// userOAuth2Github is a system function.
		//nolint:gocritic
		ctx               = dbauthz.AsSystemRestricted(r.Context())
		state             = httpmw.OAuth2(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionLogin,
		})
	)
	aReq.Old = database.APIKey{}
	defer commitAudit()

	oauthClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(state.Token))

	logger := api.Logger.Named(userAuthLoggerName)

	var selectedMemberships []*github.Membership
	var organizationNames []string
	redirect := state.Redirect
	if !api.GithubOAuth2Config.AllowEveryone {
		memberships, err := api.GithubOAuth2Config.ListOrganizationMemberships(ctx, oauthClient)
		if err != nil {
			logger.Error(ctx, "unable to list organization members", slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching authenticated Github user organizations.",
				Detail:  err.Error(),
			})
			return
		}

		for _, membership := range memberships {
			if membership.GetState() != "active" {
				continue
			}
			for _, allowed := range api.GithubOAuth2Config.AllowOrganizations {
				if *membership.Organization.Login != allowed {
					continue
				}
				selectedMemberships = append(selectedMemberships, membership)
				organizationNames = append(organizationNames, membership.Organization.GetLogin())
				break
			}
		}
		if len(selectedMemberships) == 0 {
			httpmw.CustomRedirectToLogin(rw, r, redirect, "You aren't a member of the authorized Github organizations!", http.StatusUnauthorized)
			return
		}
	}

	ghUser, err := api.GithubOAuth2Config.AuthenticatedUser(ctx, oauthClient)
	if err != nil {
		logger.Error(ctx, "oauth2: unable to fetch authenticated user", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching authenticated Github user.",
			Detail:  err.Error(),
		})
		return
	}

	// The default if no teams are specified is to allow all.
	if !api.GithubOAuth2Config.AllowEveryone && len(api.GithubOAuth2Config.AllowTeams) > 0 {
		var allowedTeam *github.Membership
		for _, allowTeam := range api.GithubOAuth2Config.AllowTeams {
			if allowedTeam != nil {
				break
			}
			for _, selectedMembership := range selectedMemberships {
				if allowTeam.Organization != *selectedMembership.Organization.Login {
					// This needs to continue because multiple organizations
					// could exist in the allow/team listings.
					continue
				}

				allowedTeam, err = api.GithubOAuth2Config.TeamMembership(ctx, oauthClient, allowTeam.Organization, allowTeam.Slug, *ghUser.Login)
				// The calling user may not have permission to the requested team!
				if err != nil {
					continue
				}
			}
		}
		if allowedTeam == nil {
			httpmw.CustomRedirectToLogin(rw, r, redirect, fmt.Sprintf("You aren't a member of an authorized team in the %v Github organization(s)!", organizationNames), http.StatusUnauthorized)
			return
		}
	}

	emails, err := api.GithubOAuth2Config.ListEmails(ctx, oauthClient)
	if err != nil {
		logger.Error(ctx, "oauth2: unable to list emails", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching personal Github user.",
			Detail:  err.Error(),
		})
		return
	}

	var verifiedEmail *github.UserEmail
	for _, email := range emails {
		if email.GetVerified() && email.GetPrimary() {
			verifiedEmail = email
			break
		}
	}

	if verifiedEmail == nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Your primary email must be verified on GitHub!",
		})
		return
	}

	// If we have a nil GitHub ID, that is a big problem. That would mean we link
	// this user and all other users with this bug to the same uuid.
	// We should instead throw an error. This should never occur in production.
	//
	// Verified that the lowest ID on GitHub is "1", so 0 should never occur.
	if ghUser.GetID() == 0 {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "The GitHub user ID is missing, this should never happen. Please report this error.",
			// If this happens, the User could either be:
			//  - Empty, in which case all these fields would also be empty.
			//  - Not a user, in which case the "Type" would be something other than "User"
			Detail: fmt.Sprintf("Other user fields: name=%q, email=%q, type=%q",
				ghUser.GetName(),
				ghUser.GetEmail(),
				ghUser.GetType(),
			),
		})
		return
	}
	user, link, err := findLinkedUser(ctx, api.Database, githubLinkedID(ghUser), verifiedEmail.GetEmail())
	if err != nil {
		logger.Error(ctx, "oauth2: unable to find linked user", slog.F("gh_user", ghUser.Name), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to find linked user.",
			Detail:  err.Error(),
		})
		return
	}

	// If a new user is authenticating for the first time
	// the audit action is 'register', not 'login'
	if user.ID == uuid.Nil {
		aReq.Action = database.AuditActionRegister
	}

	params := (&oauthLoginParams{
		User:         user,
		Link:         link,
		State:        state,
		LinkedID:     githubLinkedID(ghUser),
		LoginType:    database.LoginTypeGithub,
		AllowSignups: api.GithubOAuth2Config.AllowSignups,
		Email:        verifiedEmail.GetEmail(),
		Username:     ghUser.GetLogin(),
		AvatarURL:    ghUser.GetAvatarURL(),
		DebugContext: OauthDebugContext{},
	}).SetInitAuditRequest(func(params *audit.RequestParams) (*audit.Request[database.User], func()) {
		return audit.InitRequest[database.User](rw, params)
	})
	cookies, key, err := api.oauthLogin(r, params)
	defer params.CommitAuditLogs()
	var httpErr httpError
	if xerrors.As(err, &httpErr) {
		httpErr.Write(rw, r)
		return
	}
	if err != nil {
		logger.Error(ctx, "oauth2: login failed", slog.F("user", user.Username), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to process OAuth login.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = key
	aReq.UserID = key.UserID

	for _, cookie := range cookies {
		http.SetCookie(rw, cookie)
	}

	if redirect == "" {
		redirect = "/"
	}
	http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
}

type OIDCConfig struct {
	promoauth.OAuth2Config

	Provider *oidc.Provider
	Verifier *oidc.IDTokenVerifier
	// EmailDomains are the domains to enforce when a user authenticates.
	EmailDomain  []string
	AllowSignups bool
	// IgnoreEmailVerified allows ignoring the email_verified claim
	// from an upstream OIDC provider. See #5065 for context.
	IgnoreEmailVerified bool
	// UsernameField selects the claim field to be used as the created user's
	// username.
	UsernameField string
	// EmailField selects the claim field to be used as the created user's
	// email.
	EmailField string
	// AuthURLParams are additional parameters to be passed to the OIDC provider
	// when requesting an access token.
	AuthURLParams map[string]string
	// IgnoreUserInfo causes Coder to only use claims from the ID token to
	// process OIDC logins. This is useful if the OIDC provider does not
	// support the userinfo endpoint, or if the userinfo endpoint causes
	// undesirable behavior.
	IgnoreUserInfo bool
	// GroupField selects the claim field to be used as the created user's
	// groups. If the group field is the empty string, then no group updates
	// will ever come from the OIDC provider.
	GroupField string
	// CreateMissingGroups controls whether groups returned by the OIDC provider
	// are automatically created in Coder if they are missing.
	CreateMissingGroups bool
	// GroupFilter is a regular expression that filters the groups returned by
	// the OIDC provider. Any group not matched by this regex will be ignored.
	// If the group filter is nil, then no group filtering will occur.
	GroupFilter *regexp.Regexp
	// GroupAllowList is a list of groups that are allowed to log in.
	// If the list length is 0, then the allow list will not be applied and
	// this feature is disabled.
	GroupAllowList map[string]bool
	// GroupMapping controls how groups returned by the OIDC provider get mapped
	// to groups within Coder.
	// map[oidcGroupName]coderGroupName
	GroupMapping map[string]string
	// UserRoleField selects the claim field to be used as the created user's
	// roles. If the field is the empty string, then no role updates
	// will ever come from the OIDC provider.
	UserRoleField string
	// UserRoleMapping controls how groups returned by the OIDC provider get mapped
	// to roles within Coder.
	// map[oidcRoleName][]coderRoleName
	UserRoleMapping map[string][]string
	// UserRolesDefault is the default set of roles to assign to a user if role sync
	// is enabled.
	UserRolesDefault []string
	// SignInText is the text to display on the OIDC login button
	SignInText string
	// IconURL points to the URL of an icon to display on the OIDC login button
	IconURL string
	// SignupsDisabledText is the text do display on the static error page.
	SignupsDisabledText string
}

func (cfg OIDCConfig) RoleSyncEnabled() bool {
	return cfg.UserRoleField != ""
}

// @Summary OpenID Connect Callback
// @ID openid-connect-callback
// @Security CoderSessionToken
// @Tags Users
// @Success 307
// @Router /users/oidc/callback [get]
func (api *API) userOIDC(rw http.ResponseWriter, r *http.Request) {
	var (
		// userOIDC is a system function.
		//nolint:gocritic
		ctx               = dbauthz.AsSystemRestricted(r.Context())
		state             = httpmw.OAuth2(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.APIKey](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionLogin,
		})
	)
	aReq.Old = database.APIKey{}
	defer commitAudit()

	// See the example here: https://github.com/coreos/go-oidc
	rawIDToken, ok := state.Token.Extra("id_token").(string)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "id_token not found in response payload. Ensure your OIDC callback is configured correctly!",
		})
		return
	}

	idToken, err := api.OIDCConfig.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to verify OIDC token.",
			Detail:  err.Error(),
		})
		return
	}

	logger := api.Logger.Named(userAuthLoggerName)

	// "email_verified" is an optional claim that changes the behavior
	// of our OIDC handler, so each property must be pulled manually out
	// of the claim mapping.
	idtokenClaims := map[string]interface{}{}
	err = idToken.Claims(&idtokenClaims)
	if err != nil {
		logger.Error(ctx, "oauth2: unable to extract OIDC claims", slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to extract OIDC claims.",
			Detail:  err.Error(),
		})
		return
	}

	logger.Debug(ctx, "got oidc claims",
		slog.F("source", "id_token"),
		slog.F("claim_fields", claimFields(idtokenClaims)),
		slog.F("blank", blankFields(idtokenClaims)),
	)

	// Not all claims are necessarily embedded in the `id_token`.
	// In GitLab, the username is left empty and must be fetched in UserInfo.
	//
	// The OIDC specification says claims can be in either place, so we fetch
	// user info if required and merge the two claim sets to be sure we have
	// all of the correct data.
	//
	// Some providers (e.g. ADFS) do not support custom OIDC claims in the
	// UserInfo endpoint, so we allow users to disable it and only rely on the
	// ID token.
	userInfoClaims := make(map[string]interface{})
	// If user info is skipped, the idtokenClaims are the claims.
	mergedClaims := idtokenClaims
	if !api.OIDCConfig.IgnoreUserInfo {
		userInfo, err := api.OIDCConfig.Provider.UserInfo(ctx, oauth2.StaticTokenSource(state.Token))
		if err == nil {
			err = userInfo.Claims(&userInfoClaims)
			if err != nil {
				logger.Error(ctx, "oauth2: unable to unmarshal user info claims", slog.Error(err))
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Failed to unmarshal user info claims.",
					Detail:  err.Error(),
				})
				return
			}
			logger.Debug(ctx, "got oidc claims",
				slog.F("source", "userinfo"),
				slog.F("claim_fields", claimFields(userInfoClaims)),
				slog.F("blank", blankFields(userInfoClaims)),
			)

			// Merge the claims from the ID token and the UserInfo endpoint.
			// Information from UserInfo takes precedence.
			mergedClaims = mergeClaims(idtokenClaims, userInfoClaims)

			// Log all of the field names after merging.
			logger.Debug(ctx, "got oidc claims",
				slog.F("source", "merged"),
				slog.F("claim_fields", claimFields(mergedClaims)),
				slog.F("blank", blankFields(mergedClaims)),
			)
		} else if !strings.Contains(err.Error(), "user info endpoint is not supported by this provider") {
			logger.Error(ctx, "oauth2: unable to obtain user information claims", slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to obtain user information claims.",
				Detail:  "The attempt to fetch claims via the UserInfo endpoint failed: " + err.Error(),
			})
			return
		} else {
			// The OIDC provider does not support the UserInfo endpoint.
			// This is not an error, but we should log it as it may mean
			// that some claims are missing.
			logger.Warn(ctx, "OIDC provider does not support the user info endpoint, ensure that all required claims are present in the id_token")
		}
	}

	usernameRaw, ok := mergedClaims[api.OIDCConfig.UsernameField]
	var username string
	if ok {
		username, _ = usernameRaw.(string)
	}

	emailRaw, ok := mergedClaims[api.OIDCConfig.EmailField]
	if !ok {
		// Email is an optional claim in OIDC and
		// instead the email is frequently sent in
		// "preferred_username". See:
		// https://github.com/coder/coder/issues/4472
		_, err = mail.ParseAddress(username)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "No email found in OIDC payload!",
			})
			return
		}
		emailRaw = username
	}

	email, ok := emailRaw.(string)
	if !ok {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Email in OIDC payload isn't a string. Got: %t", emailRaw),
		})
		return
	}

	verifiedRaw, ok := mergedClaims["email_verified"]
	if ok {
		verified, ok := verifiedRaw.(bool)
		if ok && !verified {
			if !api.OIDCConfig.IgnoreEmailVerified {
				httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
					Message: fmt.Sprintf("Verify the %q email address on your OIDC provider to authenticate!", email),
				})
				return
			}
			logger.Warn(ctx, "allowing unverified oidc email %q")
		}
	}

	// The username is a required property in Coder. We make a best-effort
	// attempt at using what the claims provide, but if that fails we will
	// generate a random username.
	usernameValid := httpapi.NameValid(username)
	if usernameValid != nil {
		// If no username is provided, we can default to use the email address.
		// This will be converted in the from function below, so it's safe
		// to keep the domain.
		if username == "" {
			username = email
		}
		username = httpapi.UsernameFrom(username)
	}

	if len(api.OIDCConfig.EmailDomain) > 0 {
		ok = false
		for _, domain := range api.OIDCConfig.EmailDomain {
			if strings.HasSuffix(strings.ToLower(email), strings.ToLower(domain)) {
				ok = true
				break
			}
		}
		if !ok {
			httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
				Message: fmt.Sprintf("Your email %q is not in domains %q !", email, api.OIDCConfig.EmailDomain),
			})
			return
		}
	}

	var picture string
	pictureRaw, ok := mergedClaims["picture"]
	if ok {
		picture, _ = pictureRaw.(string)
	}

	ctx = slog.With(ctx, slog.F("email", email), slog.F("username", username))
	usingGroups, groups, groupErr := api.oidcGroups(ctx, mergedClaims)
	if groupErr != nil {
		groupErr.Write(rw, r)
		return
	}

	roles, roleErr := api.oidcRoles(ctx, mergedClaims)
	if roleErr != nil {
		roleErr.Write(rw, r)
		return
	}

	user, link, err := findLinkedUser(ctx, api.Database, oidcLinkedID(idToken), email)
	if err != nil {
		logger.Error(ctx, "oauth2: unable to find linked user", slog.F("email", email), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to find linked user.",
			Detail:  err.Error(),
		})
		return
	}

	// If a new user is authenticating for the first time
	// the audit action is 'register', not 'login'
	if user.ID == uuid.Nil {
		aReq.Action = database.AuditActionRegister
	}

	params := (&oauthLoginParams{
		User:                user,
		Link:                link,
		State:               state,
		LinkedID:            oidcLinkedID(idToken),
		LoginType:           database.LoginTypeOIDC,
		AllowSignups:        api.OIDCConfig.AllowSignups,
		Email:               email,
		Username:            username,
		AvatarURL:           picture,
		UsingRoles:          api.OIDCConfig.RoleSyncEnabled(),
		Roles:               roles,
		UsingGroups:         usingGroups,
		Groups:              groups,
		CreateMissingGroups: api.OIDCConfig.CreateMissingGroups,
		GroupFilter:         api.OIDCConfig.GroupFilter,
		DebugContext: OauthDebugContext{
			IDTokenClaims:  idtokenClaims,
			UserInfoClaims: userInfoClaims,
		},
	}).SetInitAuditRequest(func(params *audit.RequestParams) (*audit.Request[database.User], func()) {
		return audit.InitRequest[database.User](rw, params)
	})
	cookies, key, err := api.oauthLogin(r, params)
	defer params.CommitAuditLogs()
	var httpErr httpError
	if xerrors.As(err, &httpErr) {
		httpErr.Write(rw, r)
		return
	}
	if err != nil {
		logger.Error(ctx, "oauth2: login failed", slog.F("user", user.Username), slog.Error(err))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to process OAuth login.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = key
	aReq.UserID = key.UserID

	for i := range cookies {
		http.SetCookie(rw, cookies[i])
	}

	redirect := state.Redirect
	if redirect == "" {
		redirect = "/"
	}
	http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
}

// oidcGroups returns the groups for the user from the OIDC claims.
func (api *API) oidcGroups(ctx context.Context, mergedClaims map[string]interface{}) (bool, []string, *httpError) {
	logger := api.Logger.Named(userAuthLoggerName)
	usingGroups := false
	var groups []string

	// If the GroupField is the empty string, then groups from OIDC are not used.
	// This is so we can support manual group assignment.
	if api.OIDCConfig.GroupField != "" {
		// If the allow list is empty, then the user is allowed to log in.
		// Otherwise, they must belong to at least 1 group in the allow list.
		inAllowList := len(api.OIDCConfig.GroupAllowList) == 0

		usingGroups = true
		groupsRaw, ok := mergedClaims[api.OIDCConfig.GroupField]
		if ok {
			parsedGroups, err := parseStringSliceClaim(groupsRaw)
			if err != nil {
				api.Logger.Debug(ctx, "groups field was an unknown type in oidc claims",
					slog.F("type", fmt.Sprintf("%T", groupsRaw)),
					slog.Error(err),
				)
				return false, nil, &httpError{
					code:             http.StatusBadRequest,
					msg:              "Failed to sync groups from OIDC claims",
					detail:           err.Error(),
					renderStaticPage: false,
				}
			}

			api.Logger.Debug(ctx, "groups returned in oidc claims",
				slog.F("len", len(parsedGroups)),
				slog.F("groups", parsedGroups),
			)

			for _, group := range parsedGroups {
				if mappedGroup, ok := api.OIDCConfig.GroupMapping[group]; ok {
					group = mappedGroup
				}
				if _, ok := api.OIDCConfig.GroupAllowList[group]; ok {
					inAllowList = true
				}
				groups = append(groups, group)
			}
		}

		if !inAllowList {
			logger.Debug(ctx, "oidc group claim not in allow list, rejecting login",
				slog.F("allow_list_count", len(api.OIDCConfig.GroupAllowList)),
				slog.F("user_group_count", len(groups)),
			)
			detail := "Ask an administrator to add one of your groups to the whitelist"
			if len(groups) == 0 {
				detail = "You are currently not a member of any groups! Ask an administrator to add you to an authorized group to login."
			}
			return usingGroups, groups, &httpError{
				code:             http.StatusForbidden,
				msg:              "Not a member of an allowed group",
				detail:           detail,
				renderStaticPage: true,
			}
		}
	}

	// This conditional is purely to warn the user they might have misconfigured their OIDC
	// configuration.
	if _, groupClaimExists := mergedClaims["groups"]; !usingGroups && groupClaimExists {
		logger.Debug(ctx, "claim 'groups' was returned, but 'oidc-group-field' is not set, check your coder oidc settings")
	}

	return usingGroups, groups, nil
}

// oidcRoles returns the roles for the user from the OIDC claims.
// If the function returns false, then the caller should return early.
// All writes to the response writer are handled by this function.
// It would be preferred to just return an error, however this function
// decorates returned errors with the appropriate HTTP status codes and details
// that are hard to carry in a standard `error` without more work.
func (api *API) oidcRoles(ctx context.Context, mergedClaims map[string]interface{}) ([]string, *httpError) {
	roles := api.OIDCConfig.UserRolesDefault
	if !api.OIDCConfig.RoleSyncEnabled() {
		return roles, nil
	}

	rolesRow, ok := mergedClaims[api.OIDCConfig.UserRoleField]
	if !ok {
		// If no claim is provided than we can assume the user is just
		// a member. This is because there is no way to tell the difference
		// between []string{} and nil for OIDC claims. IDPs omit claims
		// if they are empty ([]string{}).
		// Use []interface{}{} so the next typecast works.
		rolesRow = []interface{}{}
	}

	parsedRoles, err := parseStringSliceClaim(rolesRow)
	if err != nil {
		api.Logger.Error(ctx, "oidc claims user roles field was an unknown type",
			slog.F("type", fmt.Sprintf("%T", rolesRow)),
			slog.Error(err),
		)
		return nil, &httpError{
			code:             http.StatusInternalServerError,
			msg:              "Login disabled until OIDC config is fixed",
			detail:           fmt.Sprintf("Roles claim must be an array of strings, type found: %T. Disabling role sync will allow login to proceed.", rolesRow),
			renderStaticPage: false,
		}
	}

	api.Logger.Debug(ctx, "roles returned in oidc claims",
		slog.F("len", len(parsedRoles)),
		slog.F("roles", parsedRoles),
	)
	for _, role := range parsedRoles {
		if mappedRoles, ok := api.OIDCConfig.UserRoleMapping[role]; ok {
			if len(mappedRoles) == 0 {
				continue
			}
			// Mapped roles are added to the list of roles
			roles = append(roles, mappedRoles...)
			continue
		}

		roles = append(roles, role)
	}
	return roles, nil
}

// claimFields returns the sorted list of fields in the claims map.
func claimFields(claims map[string]interface{}) []string {
	fields := []string{}
	for field := range claims {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

// blankFields returns the list of fields in the claims map that are
// an empty string.
func blankFields(claims map[string]interface{}) []string {
	fields := make([]string, 0)
	for field, value := range claims {
		if valueStr, ok := value.(string); ok && valueStr == "" {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	return fields
}

// mergeClaims merges the claims from a and b and returns the merged set.
// claims from b take precedence over claims from a.
func mergeClaims(a, b map[string]interface{}) map[string]interface{} {
	c := make(map[string]interface{})
	for k, v := range a {
		c[k] = v
	}
	for k, v := range b {
		c[k] = v
	}
	return c
}

// OauthDebugContext provides helpful information for admins to debug
// OAuth login issues.
type OauthDebugContext struct {
	IDTokenClaims  map[string]interface{} `json:"id_token_claims"`
	UserInfoClaims map[string]interface{} `json:"user_info_claims"`
}

type oauthLoginParams struct {
	User      database.User
	Link      database.UserLink
	State     httpmw.OAuth2State
	LinkedID  string
	LoginType database.LoginType

	// The following are necessary in order to
	// create new users.
	AllowSignups bool
	Email        string
	Username     string
	AvatarURL    string
	// Is UsingGroups is true, then the user will be assigned
	// to the Groups provided.
	UsingGroups         bool
	CreateMissingGroups bool
	Groups              []string
	GroupFilter         *regexp.Regexp
	// Is UsingRoles is true, then the user will be assigned
	// the roles provided.
	UsingRoles bool
	Roles      []string

	DebugContext OauthDebugContext

	commitLock       sync.Mutex
	initAuditRequest func(params *audit.RequestParams) *audit.Request[database.User]
	commits          []func()
}

func (p *oauthLoginParams) SetInitAuditRequest(f func(params *audit.RequestParams) (*audit.Request[database.User], func())) *oauthLoginParams {
	p.initAuditRequest = func(params *audit.RequestParams) *audit.Request[database.User] {
		p.commitLock.Lock()
		defer p.commitLock.Unlock()
		req, commit := f(params)
		p.commits = append(p.commits, commit)
		return req
	}
	return p
}

func (p *oauthLoginParams) CommitAuditLogs() {
	p.commitLock.Lock()
	defer p.commitLock.Unlock()
	for _, f := range p.commits {
		f()
	}
}

type httpError struct {
	code             int
	msg              string
	detail           string
	renderStaticPage bool

	renderDetailMarkdown bool
}

func (e httpError) Write(rw http.ResponseWriter, r *http.Request) {
	if e.renderStaticPage {
		site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
			Status:       e.code,
			HideStatus:   true,
			Title:        e.msg,
			Description:  e.detail,
			RetryEnabled: false,
			DashboardURL: "/login",

			RenderDescriptionMarkdown: e.renderDetailMarkdown,
		})
		return
	}
	httpapi.Write(r.Context(), rw, e.code, codersdk.Response{
		Message: e.msg,
		Detail:  e.detail,
	})
}

func (e httpError) Error() string {
	if e.detail != "" {
		return e.detail
	}

	return e.msg
}

func (api *API) oauthLogin(r *http.Request, params *oauthLoginParams) ([]*http.Cookie, database.APIKey, error) {
	var (
		ctx     = r.Context()
		user    database.User
		cookies []*http.Cookie
		logger  = api.Logger.Named(userAuthLoggerName)
	)

	var isConvertLoginType bool
	err := api.Database.InTx(func(tx database.Store) error {
		var (
			link database.UserLink
			err  error
		)

		user = params.User
		link = params.Link

		// If you do a convert to OIDC and your email does not match, we need to
		// catch this and not make a new account.
		if isMergeStateString(params.State.StateString) {
			// Always clear this cookie. If it succeeds, we no longer need it.
			// If it fails, we no longer care about it.
			cookies = append(cookies, clearOAuthConvertCookie())
			user, err = api.convertUserToOauth(ctx, r, tx, params)
			if err != nil {
				return err
			}
			params.User = user
			isConvertLoginType = true
		}

		if user.ID == uuid.Nil && !params.AllowSignups {
			signupsDisabledText := "Please contact your Coder administrator to request access."
			if api.OIDCConfig != nil && api.OIDCConfig.SignupsDisabledText != "" {
				signupsDisabledText = parameter.HTML(api.OIDCConfig.SignupsDisabledText)
			}
			return httpError{
				code:             http.StatusForbidden,
				msg:              "Signups are disabled",
				detail:           signupsDisabledText,
				renderStaticPage: true,

				renderDetailMarkdown: true,
			}
		}

		if user.ID != uuid.Nil && user.LoginType != params.LoginType {
			return wrongLoginTypeHTTPError(user.LoginType, params.LoginType)
		}

		// This can happen if a user is a built-in user but is signing in
		// with OIDC for the first time.
		if user.ID == uuid.Nil {
			var organizationID uuid.UUID
			//nolint:gocritic
			organizations, _ := tx.GetOrganizations(dbauthz.AsSystemRestricted(ctx))
			if len(organizations) > 0 {
				// Add the user to the first organization. Once multi-organization
				// support is added, we should enable a configuration map of user
				// email to organization.
				organizationID = organizations[0].ID
			}

			//nolint:gocritic
			_, err := tx.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
				Username: params.Username,
			})
			if err == nil {
				var (
					original      = params.Username
					validUsername bool
				)
				for i := 0; i < 10; i++ {
					alternate := fmt.Sprintf("%s-%s", original, namesgenerator.GetRandomName(1))

					params.Username = httpapi.UsernameFrom(alternate)

					//nolint:gocritic
					_, err := tx.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
						Username: params.Username,
					})
					if xerrors.Is(err, sql.ErrNoRows) {
						validUsername = true
						break
					}
					if err != nil {
						return xerrors.Errorf("get user by email/username: %w", err)
					}
				}
				if !validUsername {
					return httpError{
						code: http.StatusConflict,
						msg:  fmt.Sprintf("exhausted alternatives for taken username %q", original),
					}
				}
			}

			//nolint:gocritic
			user, _, err = api.CreateUser(dbauthz.AsSystemRestricted(ctx), tx, CreateUserRequest{
				CreateUserRequest: codersdk.CreateUserRequest{
					Email:          params.Email,
					Username:       params.Username,
					OrganizationID: organizationID,
				},
				// All of the userauth tests depend on this being able to create
				// the first organization. It shouldn't be possible in normal
				// operation.
				CreateOrganization: len(organizations) == 0,
				LoginType:          params.LoginType,
			})
			if err != nil {
				return xerrors.Errorf("create user: %w", err)
			}
		}

		// Activate dormant user on sigin
		if user.Status == database.UserStatusDormant {
			//nolint:gocritic // System needs to update status of the user account (dormant -> active).
			user, err = tx.UpdateUserStatus(dbauthz.AsSystemRestricted(ctx), database.UpdateUserStatusParams{
				ID:        user.ID,
				Status:    database.UserStatusActive,
				UpdatedAt: dbtime.Now(),
			})
			if err != nil {
				logger.Error(ctx, "unable to update user status to active", slog.Error(err))
				return xerrors.Errorf("update user status: %w", err)
			}
		}

		debugContext, err := json.Marshal(params.DebugContext)
		if err != nil {
			return xerrors.Errorf("marshal debug context: %w", err)
		}

		if link.UserID == uuid.Nil {
			//nolint:gocritic // System needs to insert the user link (linked_id, oauth_token, oauth_expiry).
			link, err = tx.InsertUserLink(dbauthz.AsSystemRestricted(ctx), database.InsertUserLinkParams{
				UserID:                 user.ID,
				LoginType:              params.LoginType,
				LinkedID:               params.LinkedID,
				OAuthAccessToken:       params.State.Token.AccessToken,
				OAuthAccessTokenKeyID:  sql.NullString{}, // set by dbcrypt if required
				OAuthRefreshToken:      params.State.Token.RefreshToken,
				OAuthRefreshTokenKeyID: sql.NullString{}, // set by dbcrypt if required
				OAuthExpiry:            params.State.Token.Expiry,
				DebugContext:           debugContext,
			})
			if err != nil {
				return xerrors.Errorf("insert user link: %w", err)
			}
		}

		if link.UserID != uuid.Nil {
			//nolint:gocritic // System needs to update the user link (linked_id, oauth_token, oauth_expiry).
			link, err = tx.UpdateUserLink(dbauthz.AsSystemRestricted(ctx), database.UpdateUserLinkParams{
				UserID:                 user.ID,
				LoginType:              params.LoginType,
				OAuthAccessToken:       params.State.Token.AccessToken,
				OAuthAccessTokenKeyID:  sql.NullString{}, // set by dbcrypt if required
				OAuthRefreshToken:      params.State.Token.RefreshToken,
				OAuthRefreshTokenKeyID: sql.NullString{}, // set by dbcrypt if required
				OAuthExpiry:            params.State.Token.Expiry,
				DebugContext:           debugContext,
			})
			if err != nil {
				return xerrors.Errorf("update user link: %w", err)
			}
		}

		// Ensure groups are correct.
		if params.UsingGroups {
			filtered := params.Groups
			if params.GroupFilter != nil {
				filtered = make([]string, 0, len(params.Groups))
				for _, group := range params.Groups {
					if params.GroupFilter.MatchString(group) {
						filtered = append(filtered, group)
					}
				}
			}

			//nolint:gocritic
			err := api.Options.SetUserGroups(dbauthz.AsSystemRestricted(ctx), logger, tx, user.ID, filtered, params.CreateMissingGroups)
			if err != nil {
				return xerrors.Errorf("set user groups: %w", err)
			}
		}

		// Ensure roles are correct.
		if params.UsingRoles {
			ignored := make([]string, 0)
			filtered := make([]string, 0, len(params.Roles))
			for _, role := range params.Roles {
				if _, err := rbac.RoleByName(role); err == nil {
					filtered = append(filtered, role)
				} else {
					ignored = append(ignored, role)
				}
			}

			//nolint:gocritic
			err := api.Options.SetUserSiteRoles(dbauthz.AsSystemRestricted(ctx), logger, tx, user.ID, filtered)
			if err != nil {
				return httpError{
					code:             http.StatusBadRequest,
					msg:              "Invalid roles through OIDC claims",
					detail:           fmt.Sprintf("Error from role assignment attempt: %s", err.Error()),
					renderStaticPage: true,
				}
			}
			if len(ignored) > 0 {
				logger.Debug(ctx, "OIDC roles ignored in assignment",
					slog.F("ignored", ignored),
					slog.F("assigned", filtered),
					slog.F("user_id", user.ID),
				)
			}
		}

		needsUpdate := false
		if user.AvatarURL != params.AvatarURL {
			user.AvatarURL = params.AvatarURL
			needsUpdate = true
		}

		// If the upstream email or username has changed we should mirror
		// that in Coder. Many enterprises use a user's email/username as
		// security auditing fields so they need to stay synced.
		// NOTE: username updating has been halted since it can have infrastructure
		// provisioning consequences (updates to usernames may delete persistent
		// resources such as user home volumes).
		if user.Email != params.Email {
			user.Email = params.Email
			needsUpdate = true
		}

		if needsUpdate {
			// TODO(JonA): Since we're processing updates to a user's upstream
			// email/username, it's possible for a different built-in user to
			// have already claimed the username.
			// In such cases in the current implementation this user can now no
			// longer sign in until an administrator finds the offending built-in
			// user and changes their username.
			//nolint:gocritic
			user, err = tx.UpdateUserProfile(dbauthz.AsSystemRestricted(ctx), database.UpdateUserProfileParams{
				ID:        user.ID,
				Email:     user.Email,
				Name:      user.Name,
				Username:  user.Username,
				UpdatedAt: dbtime.Now(),
				AvatarURL: user.AvatarURL,
			})
			if err != nil {
				return xerrors.Errorf("update user profile: %w", err)
			}
		}

		return nil
	}, nil)
	if err != nil {
		return nil, database.APIKey{}, xerrors.Errorf("in tx: %w", err)
	}

	var key database.APIKey
	oldKey, _, ok := httpmw.APIKeyFromRequest(ctx, api.Database, nil, r)
	if ok && oldKey != nil && isConvertLoginType {
		// If this is a convert login type, and it succeeds, then delete the old
		// session. Force the user to log back in.
		err := api.Database.DeleteAPIKeyByID(r.Context(), oldKey.ID)
		if err != nil {
			// Do not block this login if we fail to delete the old API key.
			// Just delete the cookie and continue.
			api.Logger.Warn(r.Context(), "failed to delete old API key in convert to oidc",
				slog.Error(err),
				slog.F("old_api_key_id", oldKey.ID),
				slog.F("user_id", user.ID),
			)
		}
		cookies = append(cookies, &http.Cookie{
			Name:     codersdk.SessionTokenCookie,
			Path:     "/",
			MaxAge:   -1,
			Secure:   api.SecureAuthCookie,
			HttpOnly: true,
		})
		// This is intentional setting the key to the deleted old key,
		// as the user needs to be forced to log back in.
		key = *oldKey
	} else {
		//nolint:gocritic
		cookie, newKey, err := api.createAPIKey(dbauthz.AsSystemRestricted(ctx), apikey.CreateParams{
			UserID:          user.ID,
			LoginType:       params.LoginType,
			DefaultLifetime: api.DeploymentValues.SessionDuration.Value(),
			RemoteAddr:      r.RemoteAddr,
		})
		if err != nil {
			return nil, database.APIKey{}, xerrors.Errorf("create API key: %w", err)
		}
		cookies = append(cookies, cookie)
		key = *newKey
	}

	return cookies, key, nil
}

// convertUserToOauth will convert a user from password base loginType to
// an oauth login type. If it fails, it will return a httpError
func (api *API) convertUserToOauth(ctx context.Context, r *http.Request, db database.Store, params *oauthLoginParams) (database.User, error) {
	user := params.User

	// Trying to convert to OIDC, but the email does not match.
	// So do not make a new user, just block the request.
	if user.ID == uuid.Nil {
		return database.User{}, httpError{
			code: http.StatusBadRequest,
			msg:  fmt.Sprintf("The oidc account with the email %q does not match the email of the account you are trying to convert. Contact your administrator to resolve this issue.", params.Email),
		}
	}

	jwtCookie, err := r.Cookie(OAuthConvertCookieValue)
	if err != nil {
		return database.User{}, httpError{
			code: http.StatusBadRequest,
			msg: fmt.Sprintf("Convert to oauth cookie not found. Missing signed jwt to authorize this action. " +
				"Please try again."),
		}
	}
	var claims OAuthConvertStateClaims
	token, err := jwt.ParseWithClaims(jwtCookie.Value, &claims, func(token *jwt.Token) (interface{}, error) {
		return api.OAuthSigningKey[:], nil
	})
	if xerrors.Is(err, jwt.ErrSignatureInvalid) || !token.Valid {
		// These errors are probably because the user is mixing 2 coder deployments.
		return database.User{}, httpError{
			code: http.StatusBadRequest,
			msg:  "Using an invalid jwt to authorize this action. Ensure there is only 1 coder deployment and try again.",
		}
	}
	if err != nil {
		return database.User{}, httpError{
			code: http.StatusInternalServerError,
			msg:  fmt.Sprintf("Error parsing jwt: %v", err),
		}
	}

	// At this point, this request could be an attempt to convert from
	// password auth to oauth auth. Always log these attempts.
	var (
		auditor           = *api.Auditor.Load()
		oauthConvertAudit = params.initAuditRequest(&audit.RequestParams{
			Audit:   auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)

	oauthConvertAudit.UserID = claims.UserID
	oauthConvertAudit.Old = user

	if claims.RegisteredClaims.Issuer != api.DeploymentID {
		return database.User{}, httpError{
			code: http.StatusForbidden,
			msg:  "Request to convert login type failed. Issuer mismatch. Found a cookie from another coder deployment, please try again.",
		}
	}

	if params.State.StateString != claims.State {
		return database.User{}, httpError{
			code: http.StatusForbidden,
			msg:  "Request to convert login type failed. State mismatch.",
		}
	}

	// Make sure the merge state generated matches this OIDC login request.
	// It needs to have the correct login type information for this
	// user.
	if user.ID != claims.UserID ||
		codersdk.LoginType(user.LoginType) != claims.FromLoginType ||
		codersdk.LoginType(params.LoginType) != claims.ToLoginType {
		return database.User{}, httpError{
			code: http.StatusForbidden,
			msg:  fmt.Sprintf("Request to convert login type from %s to %s failed", user.LoginType, params.LoginType),
		}
	}

	// Convert the user and default to the normal login flow.
	// If the login succeeds, this transaction will commit and the user
	// will be converted.
	// nolint:gocritic // system query to update user login type. The user already
	// provided their password to authenticate this request.
	user, err = db.UpdateUserLoginType(dbauthz.AsSystemRestricted(ctx), database.UpdateUserLoginTypeParams{
		NewLoginType: params.LoginType,
		UserID:       user.ID,
	})
	if err != nil {
		return database.User{}, httpError{
			code: http.StatusInternalServerError,
			msg:  "Failed to convert user to new login type",
		}
	}
	oauthConvertAudit.New = user
	return user, nil
}

// githubLinkedID returns the unique ID for a GitHub user.
func githubLinkedID(u *github.User) string {
	return strconv.FormatInt(u.GetID(), 10)
}

// oidcLinkedID returns the uniqued ID for an OIDC user.
// See https://openid.net/specs/openid-connect-core-1_0.html#ClaimStability .
func oidcLinkedID(tok *oidc.IDToken) string {
	return strings.Join([]string{tok.Issuer, tok.Subject}, "||")
}

// findLinkedUser tries to find a user by their unique OAuth-linked ID.
// If it doesn't not find it, it returns the user by their email.
func findLinkedUser(ctx context.Context, db database.Store, linkedID string, emails ...string) (database.User, database.UserLink, error) {
	var (
		user database.User
		link database.UserLink
	)
	link, err := db.GetUserLinkByLinkedID(ctx, linkedID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return user, link, xerrors.Errorf("get user auth by linked ID: %w", err)
	}

	if err == nil {
		user, err = db.GetUserByID(ctx, link.UserID)
		if err != nil {
			return database.User{}, database.UserLink{}, xerrors.Errorf("get user by id: %w", err)
		}
		if !user.Deleted {
			return user, link, nil
		}
		// If the user was deleted, act as if no account link exists.
		user = database.User{}
	}

	for _, email := range emails {
		user, err = db.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
			Email: email,
		})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return user, link, xerrors.Errorf("get user by email: %w", err)
		}
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		break
	}

	if user.ID == uuid.Nil {
		// No user found.
		return database.User{}, database.UserLink{}, nil
	}

	// LEGACY: This is annoying but we have to search for the user_link
	// again except this time we search by user_id and login_type. It's
	// possible that a user_link exists without a populated 'linked_id'.
	link, err = db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
		UserID:    user.ID,
		LoginType: user.LoginType,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return database.User{}, database.UserLink{}, xerrors.Errorf("get user link by user id and login type: %w", err)
	}

	return user, link, nil
}

func isMergeStateString(state string) bool {
	return strings.HasPrefix(state, mergeStateStringPrefix)
}

func clearOAuthConvertCookie() *http.Cookie {
	return &http.Cookie{
		Name:   OAuthConvertCookieValue,
		Path:   "/",
		MaxAge: -1,
	}
}

func wrongLoginTypeHTTPError(user database.LoginType, params database.LoginType) httpError {
	addedMsg := ""
	if user == database.LoginTypePassword {
		addedMsg = " You can convert your account to use this login type by visiting your account settings."
	}
	return httpError{
		code:             http.StatusForbidden,
		renderStaticPage: true,
		msg:              "Incorrect login type",
		detail: fmt.Sprintf("Attempting to use login type %q, but the user has the login type %q.%s",
			params, user, addedMsg),
	}
}

// parseStringSliceClaim parses the claim for groups and roles, expected []string.
//
// Some providers like ADFS return a single string instead of an array if there
// is only 1 element. So this function handles the edge cases.
func parseStringSliceClaim(claim interface{}) ([]string, error) {
	groups := make([]string, 0)
	if claim == nil {
		return groups, nil
	}

	// The simple case is the type is exactly what we expected
	asStringArray, ok := claim.([]string)
	if ok {
		return asStringArray, nil
	}

	asArray, ok := claim.([]interface{})
	if ok {
		for i, item := range asArray {
			asString, ok := item.(string)
			if !ok {
				return nil, xerrors.Errorf("invalid claim type. Element %d expected a string, got: %T", i, item)
			}
			groups = append(groups, asString)
		}
		return groups, nil
	}

	asString, ok := claim.(string)
	if ok {
		if asString == "" {
			// Empty string should be 0 groups.
			return []string{}, nil
		}
		// If it is a single string, first check if it is a csv.
		// If a user hits this, it is likely a misconfiguration and they need
		// to reconfigure their IDP to send an array instead.
		if strings.Contains(asString, ",") {
			return nil, xerrors.Errorf("invalid claim type. Got a csv string (%q), change this claim to return an array of strings instead.", asString)
		}
		return []string{asString}, nil
	}

	// Not sure what the user gave us.
	return nil, xerrors.Errorf("invalid claim type. Expected an array of strings, got: %T", claim)
}
