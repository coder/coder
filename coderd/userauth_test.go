package coderd_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/google/go-github/v43/github"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type githubOAuthProvider struct{}

func (g *githubOAuthProvider) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return "/?state=" + url.QueryEscape(state)
}

func (g *githubOAuthProvider) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken: "token",
	}, nil
}

func (g *githubOAuthProvider) PersonalUser(ctx context.Context, client *github.Client) (*github.User, error) {
	return &github.User{
		ID:        github.Int64(1),
		Login:     github.String("testuser"),
		Name:      github.String("some user"),
		Email:     github.String("wow@test.io"),
		AvatarURL: github.String("https://coder.com/avatar.png"),
	}, nil
}

func (g *githubOAuthProvider) ListEmails(ctx context.Context, client *github.Client) ([]*github.UserEmail, error) {
	return []*github.UserEmail{{
		Email:    github.String("someone@io.io"),
		Primary:  github.Bool(true),
		Verified: github.Bool(true),
	}, {
		Email:    github.String("ok@io.io"),
		Primary:  github.Bool(false),
		Verified: github.Bool(false),
	}}, nil
}

func TestUserAuthGithub(t *testing.T) {
	t.Parallel()
	t.Run("FirstUser", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Provider: &githubOAuthProvider{},
		})
		client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}

		state := "somestate"
		oauthURL, err := client.URL.Parse("/api/v2/users/auth/callback/github?code=asd&state=" + state)
		require.NoError(t, err)
		req, err := http.NewRequest("GET", oauthURL.String(), nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{
			Name:  "oauth_state",
			Value: state,
		})
		res, err := client.HTTPClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, res.StatusCode)
	})
	t.Run("NewUser", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Provider: &githubOAuthProvider{},
		})
		client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		_, err := client.CreateFirstUser(context.Background(), codersdk.CreateFirstUserRequest{
			Email:            "someone@io.io",
			Username:         "someone",
			Password:         "testing",
			OrganizationName: "acme-corp",
		})
		require.NoError(t, err)
		token, err := client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
			Email:    "someone@io.io",
			Password: "testing",
		})
		require.NoError(t, err)
		client.SessionToken = token.SessionToken

		state := "somestate"
		oauthURL, err := client.URL.Parse("/api/v2/users/auth/callback/github?code=asd&state=" + state)
		require.NoError(t, err)
		req, err := http.NewRequest("GET", oauthURL.String(), nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{
			Name:  "oauth_state",
			Value: state,
		})
		res, err := client.HTTPClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, res.StatusCode)
	})
	t.Run("ExistingUser", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			GithubOAuth2Provider: &githubOAuthProvider{},
		})
		client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		_, err := client.CreateFirstUser(context.Background(), codersdk.CreateFirstUserRequest{
			Email:            "someone@io.io",
			Username:         "someone",
			Password:         "testing",
			OrganizationName: "acme-corp",
		})
		require.NoError(t, err)
		token, err := client.LoginWithPassword(context.Background(), codersdk.LoginWithPasswordRequest{
			Email:    "someone@io.io",
			Password: "testing",
		})
		require.NoError(t, err)
		client.SessionToken = token.SessionToken

		state := "somestate"
		oauthURL, err := client.URL.Parse("/api/v2/users/auth/callback/github?code=asd&state=" + state)
		require.NoError(t, err)
		req, err := http.NewRequest("GET", oauthURL.String(), nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{
			Name:  "oauth_state",
			Value: state,
		})
		res, err := client.HTTPClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, res.StatusCode)
	})
}
