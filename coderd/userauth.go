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
type GithubOAuth2Provider interface {
	httpmw.OAuth2Provider
	PersonalUser(ctx context.Context, client *github.Client) (*github.User, error)
	ListEmails(ctx context.Context, client *github.Client) ([]*github.UserEmail, error)
}

func (api *api) userAuthGithub(rw http.ResponseWriter, r *http.Request) {
	state := httpmw.OAuth2(r)

	ghClient := github.NewClient(oauth2.NewClient(r.Context(), oauth2.StaticTokenSource(state.Token)))
	ghUser, err := api.GithubOAuth2Provider.PersonalUser(r.Context(), ghClient)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get personal github user: %s", err),
		})
		return
	}
	emails, err := api.GithubOAuth2Provider.ListEmails(r.Context(), ghClient)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get personal github user: %s", err),
		})
		return
	}
	var user database.User
	// Search for existing users with matching and verified emails.
	// If a verified GitHub email matches a Coder user, we will
	// return.
	for _, email := range emails {
		if email.Verified == nil {
			continue
		}
		if !*email.Verified {
			continue
		}
		user, err = api.Database.GetUserByEmailOrUsername(r.Context(), database.GetUserByEmailOrUsernameParams{
			Username: *ghUser.Name,
			Email:    *email.Email,
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
		break
	}
	// If the user doesn't exist, create a new one!
	if user.ID == uuid.Nil {
		userCount, err := api.Database.GetUserCount(r.Context())
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
				Message: fmt.Sprintf("get user count: %s", err.Error()),
			})
			return
		}
		var organization database.Organization
		// If there aren't any users yet, create one!
		if userCount == 0 {
			organization, err = api.Database.InsertOrganization(r.Context(), database.InsertOrganizationParams{
				ID:        uuid.New(),
				Name:      *ghUser.Name,
				CreatedAt: database.Now(),
				UpdatedAt: database.Now(),
			})
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("create organization: %s", err),
				})
				return
			}
		} else {
			organizations, err := api.Database.GetOrganizations(r.Context())
			if err != nil {
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get organizations: %s", err),
				})
				return
			}
			// Add the user to the first organization. Once multi-organization
			// support is added, we should enable a configuration map of user
			// email to organization.
			organization = organizations[0]
		}

		user, err = api.createUser(r.Context(), api.Database, codersdk.CreateUserRequest{
			Email:          *ghUser.Email,
			Username:       *ghUser.Name,
			OrganizationID: organization.ID,
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
