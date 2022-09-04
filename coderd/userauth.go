package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

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
	AllowOrganizations []string
	AllowTeams         []GithubOAuth2Team
}

func (api *API) userAuthMethods(rw http.ResponseWriter, _ *http.Request) {
	httpapi.Write(rw, http.StatusOK, codersdk.AuthMethods{
		Password: true,
		Github:   api.GithubOAuth2Config != nil,
		OIDC:     api.OIDCConfig != nil,
	})
}

func (api *API) userOAuth2Github(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx   = r.Context()
		state = httpmw.OAuth2(r)
	)

	oauthClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(state.Token))
	memberships, err := api.GithubOAuth2Config.ListOrganizationMemberships(ctx, oauthClient)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching authenticated Github user organizations.",
			Detail:  err.Error(),
		})
		return
	}
	var selectedMembership *github.Membership
	for _, membership := range memberships {
		for _, allowed := range api.GithubOAuth2Config.AllowOrganizations {
			if *membership.Organization.Login != allowed {
				continue
			}
			selectedMembership = membership
			break
		}
	}
	if selectedMembership == nil {
		httpapi.Write(rw, http.StatusUnauthorized, codersdk.Response{
			Message: "You aren't a member of the authorized Github organizations!",
		})
		return
	}

	ghUser, err := api.GithubOAuth2Config.AuthenticatedUser(ctx, oauthClient)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching authenticated Github user.",
			Detail:  err.Error(),
		})
		return
	}

	// The default if no teams are specified is to allow all.
	if len(api.GithubOAuth2Config.AllowTeams) > 0 {
		var allowedTeam *github.Membership
		for _, allowTeam := range api.GithubOAuth2Config.AllowTeams {
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
		if allowedTeam == nil {
			httpapi.Write(rw, http.StatusUnauthorized, codersdk.Response{
				Message: fmt.Sprintf("You aren't a member of an authorized team in the %s Github organization!", *selectedMembership.Organization.Login),
			})
			return
		}
	}

	emails, err := api.GithubOAuth2Config.ListEmails(ctx, oauthClient)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
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
		httpapi.Write(rw, http.StatusPreconditionRequired, codersdk.Response{
			Message: "Your primary email must be verified on GitHub!",
		})
		return
	}

	cookie, err := api.oauthLogin(r, oauthLoginParams{
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
		httpapi.Write(rw, httpErr.code, codersdk.Response{
			Message: httpErr.msg,
			Detail:  httpErr.detail,
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to process OAuth login.",
			Detail:  err.Error(),
		})
		return
	}

	http.SetCookie(rw, cookie)

	redirect := state.Redirect
	if redirect == "" {
		redirect = "/"
	}
	http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
}

type OIDCConfig struct {
	httpmw.OAuth2Config

	Verifier *oidc.IDTokenVerifier
	// EmailDomain is the domain to enforce when a user authenticates.
	EmailDomain  string
	AllowSignups bool
}

func (api *API) userOIDC(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx   = r.Context()
		state = httpmw.OAuth2(r)
	)

	// See the example here: https://github.com/coreos/go-oidc
	rawIDToken, ok := state.Token.Extra("id_token").(string)
	if !ok {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "id_token not found in response payload. Ensure your OIDC callback is configured correctly!",
		})
		return
	}

	idToken, err := api.OIDCConfig.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to verify OIDC token.",
			Detail:  err.Error(),
		})
		return
	}

	var claims struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
		Username string `json:"preferred_username"`
		Picture  string `json:"picture"`
	}
	err = idToken.Claims(&claims)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to extract OIDC claims.",
			Detail:  err.Error(),
		})
		return
	}
	if claims.Email == "" {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "No email found in OIDC payload!",
		})
		return
	}
	if !claims.Verified {
		httpapi.Write(rw, http.StatusForbidden, codersdk.Response{
			Message: fmt.Sprintf("Verify the %q email address on your OIDC provider to authenticate!", claims.Email),
		})
		return
	}
	// The username is a required property in Coder. We make a best-effort
	// attempt at using what the claims provide, but if that fails we will
	// generate a random username.
	if !httpapi.UsernameValid(claims.Username) {
		// If no username is provided, we can default to use the email address.
		// This will be converted in the from function below, so it's safe
		// to keep the domain.
		if claims.Username == "" {
			claims.Username = claims.Email
		}
		claims.Username = httpapi.UsernameFrom(claims.Username)
	}
	if api.OIDCConfig.EmailDomain != "" {
		if !strings.HasSuffix(claims.Email, api.OIDCConfig.EmailDomain) {
			httpapi.Write(rw, http.StatusForbidden, codersdk.Response{
				Message: fmt.Sprintf("Your email %q is not a part of the %q domain!", claims.Email, api.OIDCConfig.EmailDomain),
			})
			return
		}
	}

	cookie, err := api.oauthLogin(r, oauthLoginParams{
		State:        state,
		LinkedID:     oidcLinkedID(idToken),
		LoginType:    database.LoginTypeOIDC,
		AllowSignups: api.OIDCConfig.AllowSignups,
		Email:        claims.Email,
		Username:     claims.Username,
		AvatarURL:    claims.Picture,
	})
	var httpErr httpError
	if xerrors.As(err, &httpErr) {
		httpapi.Write(rw, httpErr.code, codersdk.Response{
			Message: httpErr.msg,
			Detail:  httpErr.detail,
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to process OAuth login.",
			Detail:  err.Error(),
		})
		return
	}

	http.SetCookie(rw, cookie)

	redirect := state.Redirect
	if redirect == "" {
		redirect = "/"
	}
	http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
}

type oauthLoginParams struct {
	State     httpmw.OAuth2State
	LinkedID  string
	LoginType database.LoginType

	// The following are necessary in order to
	// create new users.
	AllowSignups bool
	Email        string
	Username     string
	AvatarURL    string
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

func (api *API) oauthLogin(r *http.Request, params oauthLoginParams) (*http.Cookie, error) {
	var (
		ctx  = r.Context()
		user database.User
	)

	err := api.Database.InTx(func(tx database.Store) error {
		var (
			link database.UserLink
			err  error
		)

		user, link, err = findLinkedUser(ctx, tx, params.LinkedID, params.Email)
		if err != nil {
			return xerrors.Errorf("find linked user: %w", err)
		}

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
			organizations, _ := tx.GetOrganizations(ctx)
			if len(organizations) > 0 {
				// Add the user to the first organization. Once multi-organization
				// support is added, we should enable a configuration map of user
				// email to organization.
				organizationID = organizations[0].ID
			}

			user, _, err = api.createUser(ctx, tx, createUserRequest{
				CreateUserRequest: codersdk.CreateUserRequest{
					Email:          params.Email,
					Username:       params.Username,
					OrganizationID: organizationID,
				},
				LoginType: database.LoginTypeOIDC,
			})
			if err != nil {
				return xerrors.Errorf("create user: %w", err)
			}
		}

		if link.UserID == uuid.Nil {
			link, err = tx.InsertUserLink(ctx, database.InsertUserLinkParams{
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

		// LEGACY: Remove 10/2022.
		// We started tracking linked IDs later so it's possible for a user to be a
		// pre-existing OAuth user and not have a linked ID.
		// The migration that added the user_links table could not populate
		// the 'linked_id' field since it requires fields off the access token.
		if link.LinkedID == "" {
			link, err = tx.UpdateUserLinkedID(ctx, database.UpdateUserLinkedIDParams{
				UserID:    user.ID,
				LoginType: params.LoginType,
				LinkedID:  params.LinkedID,
			})
			if err != nil {
				return xerrors.Errorf("update user linked ID: %w", err)
			}
		}

		if link.UserID != uuid.Nil {
			link, err = tx.UpdateUserLink(ctx, database.UpdateUserLinkParams{
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
			user, err = tx.UpdateUserProfile(ctx, database.UpdateUserProfileParams{
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
	})
	if err != nil {
		return nil, xerrors.Errorf("in tx: %w", err)
	}

	cookie, err := api.createAPIKey(r, createAPIKeyParams{
		UserID:    user.ID,
		LoginType: params.LoginType,
	})
	if err != nil {
		return nil, xerrors.Errorf("create API key: %w", err)
	}

	return cookie, nil
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
		return user, link, nil
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
