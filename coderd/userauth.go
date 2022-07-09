package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

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
	ListTeams                   func(ctx context.Context, client *http.Client, org string) ([]*github.Team, error)

	AllowSignups       bool
	AllowOrganizations []string
	AllowTeams         []GithubOAuth2Team
}

func (api *API) userAuthMethods(rw http.ResponseWriter, _ *http.Request) {
	httpapi.Write(rw, http.StatusOK, codersdk.AuthMethods{
		Password: true,
		Github:   api.GithubOAuth2Config != nil,
	})
}

func (api *API) userOAuth2Github(rw http.ResponseWriter, r *http.Request) {
	state := httpmw.OAuth2(r)

	oauthClient := oauth2.NewClient(r.Context(), oauth2.StaticTokenSource(state.Token))
	memberships, err := api.GithubOAuth2Config.ListOrganizationMemberships(r.Context(), oauthClient)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
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
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "You aren't a member of the authorized Github organizations!",
		})
		return
	}

	// The default if no teams are specified is to allow all.
	if len(api.GithubOAuth2Config.AllowTeams) > 0 {
		teams, err := api.GithubOAuth2Config.ListTeams(r.Context(), oauthClient, *selectedMembership.Organization.Login)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: "Failed to fetch teams from GitHub.",
				Detail:  err.Error(),
			})
			return
		}

		var allowedTeam *github.Team
		for _, team := range teams {
			for _, allowTeam := range api.GithubOAuth2Config.AllowTeams {
				if allowTeam.Organization != *selectedMembership.Organization.Login {
					// This needs to continue because multiple organizations
					// could exist in the allow/team listings.
					continue
				}
				if allowTeam.Slug != *team.Slug {
					continue
				}
				allowedTeam = team
				break
			}
		}

		if allowedTeam == nil {
			httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
				Message: fmt.Sprintf("You aren't a member of an authorized team in the %s Github organization!", *selectedMembership.Organization.Login),
			})
			return
		}
	}

	emails, err := api.GithubOAuth2Config.ListEmails(r.Context(), oauthClient)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
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
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("Internal error fetching user by email %q.", *email.Email),
				Detail:  err.Error(),
			})
			return
		}
		if !*email.Verified {
			httpapi.Write(rw, http.StatusForbidden, httpapi.Response{
				Message: fmt.Sprintf("Verify the %q email address on Github to authenticate!", *email.Email),
			})
			return
		}
		break
	}

	// If the user doesn't exist, create a new one!
	if user.ID == uuid.Nil {
		if !api.GithubOAuth2Config.AllowSignups {
			httpapi.Write(rw, http.StatusForbidden, httpapi.Response{
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
		ghUser, err := api.GithubOAuth2Config.AuthenticatedUser(r.Context(), oauthClient)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: "Internal error fetching authenticated Github user.",
				Detail:  err.Error(),
			})
			return
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
			httpapi.Write(rw, http.StatusPreconditionRequired, httpapi.Response{
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
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
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
