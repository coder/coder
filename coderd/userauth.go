package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/userpassword"
	"github.com/coder/coder/codersdk"
)

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

	//nolint:gocritic // In order to login, we need to get the user first!
	user, err := api.Database.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
		Email: loginWithPassword.Email,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return
	}

	aReq.UserID = user.ID

	// If the user doesn't exist, it will be a default struct.
	equal, err := userpassword.Compare(string(user.HashedPassword), loginWithPassword.Password)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return
	}
	if !equal {
		// This message is the same as above to remove ease in detecting whether
		// users are registered or not. Attackers still could with a timing attack.
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Incorrect email or password.",
		})
		return
	}

	// If password authentication is disabled and the user does not have the
	// owner role, block the request.
	if api.DeploymentConfig.DisablePasswordAuth.Value {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Password authentication is disabled.",
		})
		return
	}

	if user.LoginType != database.LoginTypePassword {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: fmt.Sprintf("Incorrect login type, attempting to use %q but user is of login type %q", database.LoginTypePassword, user.LoginType),
		})
		return
	}

	//nolint:gocritic // System needs to fetch user roles in order to login user.
	roles, err := api.Database.GetAuthorizationUserRoles(dbauthz.AsSystemRestricted(ctx), user.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error.",
		})
		return
	}

	// If the user logged into a suspended account, reject the login request.
	if roles.Status != database.UserStatusActive {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Your account is suspended. Contact an admin to reactivate your account.",
		})
		return
	}

	userSubj := rbac.Subject{
		ID:     user.ID.String(),
		Roles:  rbac.RoleNames(roles.Roles),
		Groups: roles.Groups,
		Scope:  rbac.ScopeAll,
	}

	//nolint:gocritic // Creating the API key as the user instead of as system.
	cookie, key, err := api.createAPIKey(dbauthz.As(ctx, userSubj), createAPIKeyParams{
		UserID:     user.ID,
		LoginType:  database.LoginTypePassword,
		RemoteAddr: r.RemoteAddr,
	})
	if err != nil {
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

	err := api.Database.DeleteAPIKeyByID(ctx, apiKey.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting API key.",
			Detail:  err.Error(),
		})
		return
	}

	// Deployments should not host app tokens on the same domain as the
	// primary deployment. But in the case they are, we should also delete this
	// token.
	if appCookie, _ := r.Cookie(httpmw.DevURLSessionTokenCookie); appCookie != nil {
		appCookieRemove := &http.Cookie{
			// MaxAge < 0 means to delete the cookie now.
			MaxAge: -1,
			Name:   httpmw.DevURLSessionTokenCookie,
			Path:   "/",
			Domain: "." + api.AccessURL.Hostname(),
		}
		http.SetCookie(rw, appCookieRemove)

		id, _, err := httpmw.SplitAPIToken(appCookie.Value)
		if err == nil {
			err = api.Database.DeleteAPIKeyByID(ctx, id)
			if err != nil {
				// Don't block logout, just log any errors.
				api.Logger.Warn(r.Context(), "failed to delete devurl token on logout",
					slog.Error(err),
					slog.F("id", id),
				)
			}
		}
	}

	// This code should be removed after Jan 1 2023.
	// This code logs out of the old session cookie before we renamed it
	// if it is a valid coder token. Otherwise, this old cookie hangs around
	// and we never log out of the user.
	oldCookie, err := r.Cookie("session_token")
	if err == nil && oldCookie != nil {
		_, _, err := httpmw.SplitAPIToken(oldCookie.Value)
		if err == nil {
			cookie := &http.Cookie{
				// MaxAge < 0 means to delete the cookie now.
				MaxAge: -1,
				Name:   "session_token",
				Path:   "/",
			}
			http.SetCookie(rw, cookie)
		}
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
	httpmw.OAuth2Config
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
			Enabled: !api.DeploymentConfig.DisablePasswordAuth.Value,
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
		ctx               = r.Context()
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

	var selectedMemberships []*github.Membership
	var organizationNames []string
	if !api.GithubOAuth2Config.AllowEveryone {
		memberships, err := api.GithubOAuth2Config.ListOrganizationMemberships(ctx, oauthClient)
		if err != nil {
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
			httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
				Message: "You aren't a member of the authorized Github organizations!",
			})
			return
		}
	}

	ghUser, err := api.GithubOAuth2Config.AuthenticatedUser(ctx, oauthClient)
	if err != nil {
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
			httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
				Message: fmt.Sprintf("You aren't a member of an authorized team in the %v Github organization(s)!", organizationNames),
			})
			return
		}
	}

	emails, err := api.GithubOAuth2Config.ListEmails(ctx, oauthClient)
	if err != nil {
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

	user, link, err := findLinkedUser(ctx, api.Database, githubLinkedID(ghUser), verifiedEmail.GetEmail())
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to find linked user.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.UserID = user.ID

	cookie, key, err := api.oauthLogin(r, oauthLoginParams{
		User:         user,
		Link:         link,
		State:        state,
		LinkedID:     githubLinkedID(ghUser),
		LoginType:    database.LoginTypeGithub,
		AllowSignups: api.GithubOAuth2Config.AllowSignups,
		Email:        verifiedEmail.GetEmail(),
		Username:     ghUser.GetLogin(),
		AvatarURL:    ghUser.GetAvatarURL(),
	})
	var httpErr httpError
	if xerrors.As(err, &httpErr) {
		httpapi.Write(ctx, rw, httpErr.code, codersdk.Response{
			Message: httpErr.msg,
			Detail:  httpErr.detail,
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to process OAuth login.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = key

	http.SetCookie(rw, cookie)

	redirect := state.Redirect
	if redirect == "" {
		redirect = "/"
	}
	http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
}

type OIDCConfig struct {
	httpmw.OAuth2Config

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
	// SignInText is the text to display on the OIDC login button
	SignInText string
	// IconURL points to the URL of an icon to display on the OIDC login button
	IconURL string
}

// @Summary OpenID Connect Callback
// @ID openid-connect-callback
// @Security CoderSessionToken
// @Tags Users
// @Success 307
// @Router /users/oidc/callback [get]
func (api *API) userOIDC(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
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

	// "email_verified" is an optional claim that changes the behavior
	// of our OIDC handler, so each property must be pulled manually out
	// of the claim mapping.
	claims := map[string]interface{}{}
	err = idToken.Claims(&claims)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to extract OIDC claims.",
			Detail:  err.Error(),
		})
		return
	}

	// Not all claims are necessarily embedded in the `id_token`.
	// In GitLab, the username is left empty and must be fetched in UserInfo.
	//
	// The OIDC specification says claims can be in either place, so we fetch
	// user info and merge the two claim sets to be sure we have all of
	// the correct data.
	userInfo, err := api.OIDCConfig.Provider.UserInfo(ctx, oauth2.StaticTokenSource(state.Token))
	if err == nil {
		userInfoClaims := map[string]interface{}{}
		err = userInfo.Claims(&userInfoClaims)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to unmarshal user info claims.",
				Detail:  err.Error(),
			})
			return
		}
		for k, v := range userInfoClaims {
			claims[k] = v
		}
	} else if !strings.Contains(err.Error(), "user info endpoint is not supported by this provider") {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to obtain user information claims.",
			Detail:  "The OIDC provider returned no claims as part of the `id_token`. The attempt to fetch claims via the UserInfo endpoint failed: " + err.Error(),
		})
		return
	}

	usernameRaw, ok := claims[api.OIDCConfig.UsernameField]
	var username string
	if ok {
		username, _ = usernameRaw.(string)
	}

	emailRaw, ok := claims["email"]
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

	verifiedRaw, ok := claims["email_verified"]
	if ok {
		verified, ok := verifiedRaw.(bool)
		if ok && !verified {
			if !api.OIDCConfig.IgnoreEmailVerified {
				httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
					Message: fmt.Sprintf("Verify the %q email address on your OIDC provider to authenticate!", email),
				})
				return
			}
			api.Logger.Warn(ctx, "allowing unverified oidc email %q")
		}
	}

	var groups []string
	groupsRaw, ok := claims["groups"]
	if ok {
		// Convert the []interface{} we get to a []string.
		groupsInterface, ok := groupsRaw.([]interface{})
		if ok {
			for _, groupInterface := range groupsInterface {
				group, ok := groupInterface.(string)
				if !ok {
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: fmt.Sprintf("Invalid group type. Expected string, got: %t", emailRaw),
					})
					return
				}
				groups = append(groups, group)
			}
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
	pictureRaw, ok := claims["picture"]
	if ok {
		picture, _ = pictureRaw.(string)
	}

	user, link, err := findLinkedUser(ctx, api.Database, oidcLinkedID(idToken), email)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to find linked user.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.UserID = user.ID

	cookie, key, err := api.oauthLogin(r, oauthLoginParams{
		User:         user,
		Link:         link,
		State:        state,
		LinkedID:     oidcLinkedID(idToken),
		LoginType:    database.LoginTypeOIDC,
		AllowSignups: api.OIDCConfig.AllowSignups,
		Email:        email,
		Username:     username,
		AvatarURL:    picture,
		Groups:       groups,
	})
	var httpErr httpError
	if xerrors.As(err, &httpErr) {
		httpapi.Write(ctx, rw, httpErr.code, codersdk.Response{
			Message: httpErr.msg,
			Detail:  httpErr.detail,
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to process OAuth login.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = key

	http.SetCookie(rw, cookie)

	redirect := state.Redirect
	if redirect == "" {
		redirect = "/"
	}
	http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
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
	Groups       []string
}

type httpError struct {
	code   int
	msg    string
	detail string
}

func (e httpError) Error() string {
	if e.detail != "" {
		return e.detail
	}

	return e.msg
}

func (api *API) oauthLogin(r *http.Request, params oauthLoginParams) (*http.Cookie, database.APIKey, error) {
	var (
		ctx  = r.Context()
		user database.User
	)

	err := api.Database.InTx(func(tx database.Store) error {
		var (
			link database.UserLink
			err  error
		)

		user = params.User
		link = params.Link

		if user.ID == uuid.Nil && !params.AllowSignups {
			return httpError{
				code: http.StatusForbidden,
				msg:  fmt.Sprintf("Signups are not allowed for login type %q", params.LoginType),
			}
		}

		if user.ID != uuid.Nil && user.LoginType != params.LoginType {
			return httpError{
				code: http.StatusForbidden,
				msg: fmt.Sprintf("Incorrect login type, attempting to use %q but user is of login type %q",
					params.LoginType,
					user.LoginType,
				),
			}
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
				LoginType: params.LoginType,
			})
			if err != nil {
				return xerrors.Errorf("create user: %w", err)
			}
		}

		if link.UserID == uuid.Nil {
			//nolint:gocritic
			link, err = tx.InsertUserLink(dbauthz.AsSystemRestricted(ctx), database.InsertUserLinkParams{
				UserID:            user.ID,
				LoginType:         params.LoginType,
				LinkedID:          params.LinkedID,
				OAuthAccessToken:  params.State.Token.AccessToken,
				OAuthRefreshToken: params.State.Token.RefreshToken,
				OAuthExpiry:       params.State.Token.Expiry,
			})
			if err != nil {
				return xerrors.Errorf("insert user link: %w", err)
			}
		}

		if link.UserID != uuid.Nil {
			//nolint:gocritic
			link, err = tx.UpdateUserLink(dbauthz.AsSystemRestricted(ctx), database.UpdateUserLinkParams{
				UserID:            user.ID,
				LoginType:         params.LoginType,
				OAuthAccessToken:  params.State.Token.AccessToken,
				OAuthRefreshToken: params.State.Token.RefreshToken,
				OAuthExpiry:       params.State.Token.Expiry,
			})
			if err != nil {
				return xerrors.Errorf("update user link: %w", err)
			}
		}

		// Ensure groups are correct.
		if len(params.Groups) > 0 {
			//nolint:gocritic
			err := api.Options.SetUserGroups(dbauthz.AsSystemRestricted(ctx), tx, user.ID, params.Groups)
			if err != nil {
				return xerrors.Errorf("set user groups: %w", err)
			}
		}

		needsUpdate := false
		if user.AvatarURL.String != params.AvatarURL {
			user.AvatarURL = sql.NullString{
				String: params.AvatarURL,
				Valid:  true,
			}
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
				Username:  user.Username,
				UpdatedAt: database.Now(),
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

	//nolint:gocritic
	cookie, key, err := api.createAPIKey(dbauthz.AsSystemRestricted(ctx), createAPIKeyParams{
		UserID:     user.ID,
		LoginType:  params.LoginType,
		RemoteAddr: r.RemoteAddr,
	})
	if err != nil {
		return nil, database.APIKey{}, xerrors.Errorf("create API key: %w", err)
	}

	return cookie, *key, nil
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
