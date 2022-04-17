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

// GithubOAuth2Provider exposes required functions for the Github authentication flow.
type GithubOAuth2Config struct {
	httpmw.OAuth2Config
	AuthenticatedUser func(ctx context.Context, client *http.Client) (*github.User, error)
	ListEmails        func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error)
	ListOrganizations func(ctx context.Context, client *http.Client) ([]*github.Organization, error)

	AllowSignups       bool
	AllowOrganizations []string
}

func (api *api) userOAuth2Github(rw http.ResponseWriter, r *http.Request) {
	state := httpmw.OAuth2(r)

	oauthClient := oauth2.NewClient(r.Context(), oauth2.StaticTokenSource(state.Token))
	organizations, err := api.GithubOAuth2Config.ListOrganizations(r.Context(), oauthClient)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get authenticated github user organizations: %s", err),
		})
		return
	}
	var selectedOrganization *github.Organization
	for _, organization := range organizations {
		if organization.Login == nil {
			continue
		}
		for _, allowed := range api.GithubOAuth2Config.AllowOrganizations {
			if *organization.Login != allowed {
				continue
			}
			selectedOrganization = organization
			break
		}
	}
	if selectedOrganization == nil {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: fmt.Sprintf("You aren't a member of the authorized Github organizations!"),
		})
		return
	}

	emails, err := api.GithubOAuth2Config.ListEmails(r.Context(), oauthClient)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get personal github user: %s", err),
		})
		return
	}

	var user database.User
	// Search for existing users with matching and verified emails.
	// If a verified GitHub email matches a Coder user, we will return.
	for _, email := range emails {
		if email.Verified == nil {
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
				Message: fmt.Sprintf("get user by email: %s", err),
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
		organizations, err := api.Database.GetOrganizations(r.Context())
		if err == nil {
			// Add the user to the first organization. Once multi-organization
			// support is added, we should enable a configuration map of user
			// email to organization.
			organizationID = organizations[0].ID
		}
		ghUser, err := api.GithubOAuth2Config.AuthenticatedUser(r.Context(), oauthClient)
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get authenticated github user: %s", err),
			})
			return
		}
		user, _, err = api.createUser(r.Context(), codersdk.CreateUserRequest{
			Email:          *ghUser.Email,
			Username:       *ghUser.Login,
			OrganizationID: organizationID,
		})
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("create user: %s", err),
			})
			return
		}
	}

	_, created := api.createAPIKey(rw, r, user.ID)
	if !created {
		return
	}

	redirect := state.Redirect
	if redirect == "" {
		redirect = "/"
	}
	http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
}
