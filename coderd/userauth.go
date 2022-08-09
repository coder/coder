package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
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
	state := httpmw.OAuth2(r)

	oauthClient := oauth2.NewClient(r.Context(), oauth2.StaticTokenSource(state.Token))
	memberships, err := api.GithubOAuth2Config.ListOrganizationMemberships(r.Context(), oauthClient)
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

	ghUser, err := api.GithubOAuth2Config.AuthenticatedUser(r.Context(), oauthClient)
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

			allowedTeam, err = api.GithubOAuth2Config.TeamMembership(r.Context(), oauthClient, allowTeam.Organization, allowTeam.Slug, *ghUser.Login)
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

	emails, err := api.GithubOAuth2Config.ListEmails(r.Context(), oauthClient)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching personal Github user.",
			Detail:  err.Error(),
		})
		return
	}

	var user database.User
	// Search for existing users with matching and verified emails.
	// If a verified GitHub email matches a Coder user, we will return.
	for _, email := range emails {
		if !email.GetVerified() {
			continue
		}
		user, err = api.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
			Email: *email.Email,
		})
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: fmt.Sprintf("Internal error fetching user by email %q.", *email.Email),
				Detail:  err.Error(),
			})
			return
		}
		if !*email.Verified {
			httpapi.Write(rw, http.StatusForbidden, codersdk.Response{
				Message: fmt.Sprintf("Verify the %q email address on Github to authenticate!", *email.Email),
			})
			return
		}
		break
	}

	// If the user doesn't exist, create a new one!
	if user.ID == uuid.Nil {
		if !api.GithubOAuth2Config.AllowSignups {
			httpapi.Write(rw, http.StatusForbidden, codersdk.Response{
				Message: "Signups are disabled for Github authentication!",
			})
			return
		}

		var organizationID uuid.UUID
		organizations, _ := api.Database.GetOrganizations(r.Context())
		if len(organizations) > 0 {
			// Add the user to the first organization. Once multi-organization
			// support is added, we should enable a configuration map of user
			// email to organization.
			organizationID = organizations[0].ID
		}
		var verifiedEmail *github.UserEmail
		for _, email := range emails {
			if !email.GetPrimary() || !email.GetVerified() {
				continue
			}
			verifiedEmail = email
			break
		}
		if verifiedEmail == nil {
			httpapi.Write(rw, http.StatusPreconditionRequired, codersdk.Response{
				Message: "Your primary email must be verified on GitHub!",
			})
			return
		}
		user, _, err = api.createUser(r.Context(), codersdk.CreateUserRequest{
			Email:          *verifiedEmail.Email,
			Username:       *ghUser.Login,
			OrganizationID: organizationID,
		})
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error creating user.",
				Detail:  err.Error(),
			})
			return
		}
	}

	_, created := api.createAPIKey(rw, r, database.InsertAPIKeyParams{
		UserID:            user.ID,
		LoginType:         database.LoginTypeGithub,
		OAuthAccessToken:  state.Token.AccessToken,
		OAuthRefreshToken: state.Token.RefreshToken,
		OAuthExpiry:       state.Token.Expiry,
	})
	if !created {
		return
	}

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

	api.Database.InTx(
		func(store database.Store) error {

		}
	)

	user, found, err := findLinkedUser(ctx, api.Database, database.LoginTypeOIDC, uniqueUserOIDC(idToken), claims.Email)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to find user.",
			Detail:  err.Error(),
		})
		return
	}

	if !found && !api.OIDCConfig.AllowSignups {
		httpapi.Write(rw, http.StatusForbidden, codersdk.Response{
			Message: "Signups are disabled for OIDC authentication!",
		})
		return
	}

	if !found {
		var organizationID uuid.UUID
		organizations, _ := api.Database.GetOrganizations(ctx)
		if len(organizations) > 0 {
			// Add the user to the first organization. Once multi-organization
			// support is added, we should enable a configuration map of user
			// email to organization.
			organizationID = organizations[0].ID
		}
		user, _, err = api.createUser(ctx, codersdk.CreateUserRequest{
			Email:          claims.Email,
			Username:       claims.Username,
			OrganizationID: organizationID,
		})
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error creating user.",
				Detail:  err.Error(),
			})
			return
		}
		_, err = api.Database.InsertUserAuth(ctx, database.InsertUserAuthParams{
			UserID:    user.ID,
			LoginType: database.LoginTypeOIDC,
			LinkedID:  uniqueUserOIDC(idToken),
		})
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to insert user auth metadata.",
				Detail:  err.Error(),
			})
			return
		}
	}
	if user.Email != claims.Email || user.Username != claims.Username {

	}
	_, created := api.createAPIKey(rw, r, database.InsertAPIKeyParams{
		UserID:            user.ID,
		LoginType:         database.LoginTypeOIDC,
		OAuthAccessToken:  state.Token.AccessToken,
		OAuthRefreshToken: state.Token.RefreshToken,
		OAuthExpiry:       state.Token.Expiry,
	})
	if !created {
		return
	}

	redirect := state.Redirect
	if redirect == "" {
		redirect = "/"
	}
	http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
}

func uniqueUserOIDC(tok *oidc.IDToken) string {
	return strings.Join([]string{tok.Issuer, tok.Subject}, "||")
}

func findLinkedUser(ctx context.Context, db database.Store, authType database.LoginType, linkedID string, email string) (database.User, bool, error) {
	var user database.User

	uauth, err := db.GetUserAuthByLinkedID(ctx, linkedID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return user, false, xerrors.Errorf("get user auth by linked ID: %w", err)
	}

	if err == nil {
		user, err := db.GetUserByID(ctx, uauth.UserID)
		if err != nil {
			return user, false, xerrors.Errorf("get user by ID: %w", err)
		}
		return user, true, nil
	}

	user, err = db.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
		Email: email,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return user, false, xerrors.Errorf("get user by email: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return user, false, nil
	}

	// Try getting the UAuth by user ID instead now. Maybe the user
	// logged in using a different login type.
	uauth, err = db.GetUserAuthByUserID(ctx, user.ID)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return user, false, xerrors.Errorf("get user auth by user ID: %w", err)
	}
	if uauth.LoginType != authType {
		return user, false, xerrors.Errorf("cannot login with %q with account is already linked with %q", authType, uauth.LoginType)
	}
	if err == nil {
		return user, false, xerrors.Errorf("user auth already exists with different linked ID? Expecting %q but got %q", linkedID, uauth.LinkedID)
	}

	_, err = db.InsertUserAuth(ctx, database.InsertUserAuthParams{
		UserID:    user.ID,
		LoginType: authType,
		LinkedID:  linkedID,
	})
	if err != nil {
		return user, false, xerrors.Errorf("insert user auth: %w", err)
	}
	return user, true, nil
}
