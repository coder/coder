package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/oauth2"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

// GitAuthConfig is the configuration for an authentication
// provider that is used for git operations.
type GitAuthConfig struct {
	httpmw.OAuth2Config
	// ID is a unique identifier for the authenticator.
	ID string
	// Regex is a regexp that URLs will match against.
	Regex *regexp.Regexp
	// Type is the type of provider.
	Type codersdk.GitProvider
}

// postWorkspaceAgentsGitAuth returns a username and password for use
// with GIT_ASKPASS.
func (api *API) workspaceAgentsGitAuth(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gitURL := r.URL.Query().Get("url")
	if gitURL == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing url query parameter!",
		})
		return
	}
	// listen determines if the request will wait for a
	// new token to be issued!
	listen := r.URL.Query().Has("listen")

	var gitAuthConfig *GitAuthConfig
	for _, gitAuth := range api.GitAuthConfigs {
		matches := gitAuth.Regex.MatchString(gitURL)
		if !matches {
			continue
		}
		gitAuthConfig = gitAuth
	}
	if gitAuthConfig == nil {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("No git provider found for URL %q", gitURL),
		})
		return
	}
	workspaceAgent := httpmw.WorkspaceAgent(r)
	// We must get the workspace to get the owner ID!
	resource, err := api.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace resource.",
			Detail:  err.Error(),
		})
		return
	}
	build, err := api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get build.",
			Detail:  err.Error(),
		})
		return
	}
	workspace, err := api.Database.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}

	if listen {
		// If listening we await a new token...
		authChan := make(chan struct{}, 1)
		cancelFunc, err := api.Pubsub.Subscribe("gitauth", func(ctx context.Context, message []byte) {
			ids := strings.Split(string(message), "|")
			if len(ids) != 2 {
				return
			}
			if ids[0] != gitAuthConfig.ID {
				return
			}
			if ids[1] != workspace.OwnerID.String() {
				return
			}
			select {
			case authChan <- struct{}{}:
			default:
			}
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to listen for git auth token.",
				Detail:  err.Error(),
			})
			return
		}
		defer cancelFunc()
		select {
		case <-r.Context().Done():
			return
		case <-authChan:
		}

		gitAuthLink, err := api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
			ProviderID: gitAuthConfig.ID,
			UserID:     workspace.OwnerID,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to get git auth link.",
				Detail:  err.Error(),
			})
			return
		}

		httpapi.Write(ctx, rw, http.StatusOK, formatGitAuthAccessToken(gitAuthConfig.Type, gitAuthLink.OAuthAccessToken))
		return
	}

	// This is the URL that will redirect the user with a state token.
	url, err := api.AccessURL.Parse(fmt.Sprintf("/git-auth/%s", gitAuthConfig.ID))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to parse access URL.",
			Detail:  err.Error(),
		})
		return
	}

	gitAuthLink, err := api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
		ProviderID: gitAuthConfig.ID,
		UserID:     workspace.OwnerID,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to get git auth link.",
				Detail:  err.Error(),
			})
			return
		}

		httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceAgentGitAuthResponse{
			URL: url.String(),
		})
		return
	}

	token, err := gitAuthConfig.TokenSource(ctx, &oauth2.Token{
		AccessToken:  gitAuthLink.OAuthAccessToken,
		RefreshToken: gitAuthLink.OAuthRefreshToken,
		Expiry:       gitAuthLink.OAuthExpiry,
	}).Token()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceAgentGitAuthResponse{
			URL: url.String(),
		})
		return
	}

	if token.AccessToken != gitAuthLink.OAuthAccessToken {
		// Update it
		err = api.Database.UpdateGitAuthLink(ctx, database.UpdateGitAuthLinkParams{
			ProviderID:        gitAuthConfig.ID,
			UserID:            workspace.OwnerID,
			UpdatedAt:         database.Now(),
			OAuthAccessToken:  token.AccessToken,
			OAuthRefreshToken: token.RefreshToken,
			OAuthExpiry:       token.Expiry,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to update git auth link.",
				Detail:  err.Error(),
			})
			return
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, formatGitAuthAccessToken(gitAuthConfig.Type, token.AccessToken))
}

// Provider types have different username/password formats.
func formatGitAuthAccessToken(_ codersdk.GitProvider, token string) codersdk.WorkspaceAgentGitAuthResponse {
	resp := codersdk.WorkspaceAgentGitAuthResponse{
		Username: token,
	}
	return resp
}

func (api *API) gitAuthCallback(gitAuthConfig *GitAuthConfig) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var (
			ctx    = r.Context()
			state  = httpmw.OAuth2(r)
			apiKey = httpmw.APIKey(r)
		)

		_, err := api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
			ProviderID: gitAuthConfig.ID,
			UserID:     apiKey.UserID,
		})
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to get git auth link.",
					Detail:  err.Error(),
				})
				return
			}

			_, err = api.Database.InsertGitAuthLink(ctx, database.InsertGitAuthLinkParams{
				ProviderID:        gitAuthConfig.ID,
				UserID:            apiKey.UserID,
				CreatedAt:         database.Now(),
				UpdatedAt:         database.Now(),
				OAuthAccessToken:  state.Token.AccessToken,
				OAuthRefreshToken: state.Token.RefreshToken,
				OAuthExpiry:       state.Token.Expiry,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to insert git auth link.",
					Detail:  err.Error(),
				})
				return
			}
		} else {
			err = api.Database.UpdateGitAuthLink(ctx, database.UpdateGitAuthLinkParams{
				ProviderID:        gitAuthConfig.ID,
				UserID:            apiKey.UserID,
				UpdatedAt:         database.Now(),
				OAuthAccessToken:  state.Token.AccessToken,
				OAuthRefreshToken: state.Token.RefreshToken,
				OAuthExpiry:       state.Token.Expiry,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to update git auth link.",
					Detail:  err.Error(),
				})
				return
			}
		}

		err = api.Pubsub.Publish("gitauth", []byte(fmt.Sprintf("%s|%s", gitAuthConfig.ID, apiKey.UserID)))
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to publish auth update.",
				Detail:  err.Error(),
			})
			return
		}

		// This is a nicely rendered screen on the frontend
		http.Redirect(rw, r, "/gitauth", http.StatusTemporaryRedirect)
	}
}
