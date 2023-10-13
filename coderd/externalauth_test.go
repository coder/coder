package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestExternalAuthByID(t *testing.T) {
	t.Parallel()
	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			ExternalAuthConfigs: []*externalauth.Config{{
				ID:           "test",
				OAuth2Config: &testutil.OAuth2Config{},
				Type:         codersdk.EnhancedExternalAuthProviderGitHub.String(),
			}},
		})
		coderdtest.CreateFirstUser(t, client)
		auth, err := client.ExternalAuthByID(context.Background(), "test")
		require.NoError(t, err)
		require.False(t, auth.Authenticated)
	})
	t.Run("AuthenticatedNoUser", func(t *testing.T) {
		// Ensures that a provider that can't obtain a user can
		// still return that the provider is authenticated.
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			ExternalAuthConfigs: []*externalauth.Config{{
				ID:           "test",
				OAuth2Config: &testutil.OAuth2Config{},
				// AzureDevops doesn't have a user endpoint!
				Type: codersdk.EnhancedExternalAuthProviderAzureDevops.String(),
			}},
		})
		coderdtest.CreateFirstUser(t, client)
		resp := coderdtest.RequestExternalAuthCallback(t, "test", client)
		_ = resp.Body.Close()
		auth, err := client.ExternalAuthByID(context.Background(), "test")
		require.NoError(t, err)
		require.True(t, auth.Authenticated)
	})
	t.Run("AuthenticatedWithUser", func(t *testing.T) {
		t.Parallel()
		validateSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), w, http.StatusOK, github.User{
				Login:     github.String("kyle"),
				AvatarURL: github.String("https://avatars.githubusercontent.com/u/12345678?v=4"),
			})
		}))
		defer validateSrv.Close()
		client := coderdtest.New(t, &coderdtest.Options{
			ExternalAuthConfigs: []*externalauth.Config{{
				ID:           "test",
				ValidateURL:  validateSrv.URL,
				OAuth2Config: &testutil.OAuth2Config{},
				Type:         codersdk.EnhancedExternalAuthProviderGitHub.String(),
			}},
		})
		coderdtest.CreateFirstUser(t, client)
		resp := coderdtest.RequestExternalAuthCallback(t, "test", client)
		_ = resp.Body.Close()
		auth, err := client.ExternalAuthByID(context.Background(), "test")
		require.NoError(t, err)
		require.True(t, auth.Authenticated)
		require.NotNil(t, auth.User)
		require.Equal(t, "kyle", auth.User.Login)
	})
	t.Run("AuthenticatedWithInstalls", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/user":
				httpapi.Write(r.Context(), w, http.StatusOK, github.User{
					Login:     github.String("kyle"),
					AvatarURL: github.String("https://avatars.githubusercontent.com/u/12345678?v=4"),
				})
			case "/installs":
				httpapi.Write(r.Context(), w, http.StatusOK, struct {
					Installations []github.Installation `json:"installations"`
				}{
					Installations: []github.Installation{{
						ID: github.Int64(12345678),
						Account: &github.User{
							Login: github.String("coder"),
						},
					}},
				})
			}
		}))
		defer srv.Close()
		client := coderdtest.New(t, &coderdtest.Options{
			ExternalAuthConfigs: []*externalauth.Config{{
				ID:                  "test",
				ValidateURL:         srv.URL + "/user",
				AppInstallationsURL: srv.URL + "/installs",
				OAuth2Config:        &testutil.OAuth2Config{},
				Type:                codersdk.EnhancedExternalAuthProviderGitHub.String(),
			}},
		})
		coderdtest.CreateFirstUser(t, client)
		resp := coderdtest.RequestExternalAuthCallback(t, "test", client)
		_ = resp.Body.Close()
		auth, err := client.ExternalAuthByID(context.Background(), "test")
		require.NoError(t, err)
		require.True(t, auth.Authenticated)
		require.NotNil(t, auth.User)
		require.Equal(t, "kyle", auth.User.Login)
		require.NotNil(t, auth.AppInstallations)
		require.Len(t, auth.AppInstallations, 1)
	})
}

func TestExternalAuthDevice(t *testing.T) {
	t.Parallel()
	t.Run("NotSupported", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			ExternalAuthConfigs: []*externalauth.Config{{
				ID: "test",
			}},
		})
		coderdtest.CreateFirstUser(t, client)
		_, err := client.ExternalAuthDeviceByID(context.Background(), "test")
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})
	t.Run("FetchCode", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), w, http.StatusOK, codersdk.ExternalAuthDevice{
				UserCode: "hey",
			})
		}))
		defer srv.Close()
		client := coderdtest.New(t, &coderdtest.Options{
			ExternalAuthConfigs: []*externalauth.Config{{
				ID: "test",
				DeviceAuth: &externalauth.DeviceAuth{
					ClientID: "test",
					CodeURL:  srv.URL,
					Scopes:   []string{"repo"},
				},
			}},
		})
		coderdtest.CreateFirstUser(t, client)
		device, err := client.ExternalAuthDeviceByID(context.Background(), "test")
		require.NoError(t, err)
		require.Equal(t, "hey", device.UserCode)
	})
	t.Run("ExchangeCode", func(t *testing.T) {
		t.Parallel()
		resp := externalauth.ExchangeDeviceCodeResponse{
			Error: "authorization_pending",
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), w, http.StatusOK, resp)
		}))
		defer srv.Close()
		client := coderdtest.New(t, &coderdtest.Options{
			ExternalAuthConfigs: []*externalauth.Config{{
				ID: "test",
				DeviceAuth: &externalauth.DeviceAuth{
					ClientID: "test",
					TokenURL: srv.URL,
					Scopes:   []string{"repo"},
				},
			}},
		})
		coderdtest.CreateFirstUser(t, client)
		err := client.ExternalAuthDeviceExchange(context.Background(), "test", codersdk.ExternalAuthDeviceExchange{
			DeviceCode: "hey",
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Equal(t, "authorization_pending", sdkErr.Detail)

		resp = externalauth.ExchangeDeviceCodeResponse{
			AccessToken: "hey",
		}

		err = client.ExternalAuthDeviceExchange(context.Background(), "test", codersdk.ExternalAuthDeviceExchange{
			DeviceCode: "hey",
		})
		require.NoError(t, err)

		auth, err := client.ExternalAuthByID(context.Background(), "test")
		require.NoError(t, err)
		require.True(t, auth.Authenticated)
	})
}

// nolint:bodyclose
func TestExternalAuthCallback(t *testing.T) {
	t.Parallel()
	t.Run("NoMatchingConfig", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			ExternalAuthConfigs:      []*externalauth.Config{},
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)
		_, err := agentClient.ExternalAuth(context.Background(), agentsdk.ExternalAuthRequest{
			Match: "github.com",
		})
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusNotFound, apiError.StatusCode())
	})
	t.Run("ReturnsURL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			ExternalAuthConfigs: []*externalauth.Config{{
				OAuth2Config: &testutil.OAuth2Config{},
				ID:           "github",
				Regex:        regexp.MustCompile(`github\.com`),
				Type:         codersdk.EnhancedExternalAuthProviderGitHub.String(),
			}},
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)
		token, err := agentClient.ExternalAuth(context.Background(), agentsdk.ExternalAuthRequest{
			Match: "github.com/asd/asd",
		})
		require.NoError(t, err)
		require.True(t, strings.HasSuffix(token.URL, fmt.Sprintf("/external-auth/%s", "github")), token.URL)
	})
	t.Run("UnauthorizedCallback", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			ExternalAuthConfigs: []*externalauth.Config{{
				OAuth2Config: &testutil.OAuth2Config{},
				ID:           "github",
				Regex:        regexp.MustCompile(`github\.com`),
				Type:         codersdk.EnhancedExternalAuthProviderGitHub.String(),
			}},
		})
		resp := coderdtest.RequestExternalAuthCallback(t, "github", client)
		require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	})
	t.Run("AuthorizedCallback", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			ExternalAuthConfigs: []*externalauth.Config{{
				OAuth2Config: &testutil.OAuth2Config{},
				ID:           "github",
				Regex:        regexp.MustCompile(`github\.com`),
				Type:         codersdk.EnhancedExternalAuthProviderGitHub.String(),
			}},
		})
		_ = coderdtest.CreateFirstUser(t, client)
		resp := coderdtest.RequestExternalAuthCallback(t, "github", client)
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		location, err := resp.Location()
		require.NoError(t, err)
		require.Equal(t, "/external-auth/github", location.Path)

		// Callback again to simulate updating the token.
		resp = coderdtest.RequestExternalAuthCallback(t, "github", client)
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	})
	t.Run("ValidateURL", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		srv := httptest.NewServer(nil)
		defer srv.Close()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			ExternalAuthConfigs: []*externalauth.Config{{
				ValidateURL:  srv.URL,
				OAuth2Config: &testutil.OAuth2Config{},
				ID:           "github",
				Regex:        regexp.MustCompile(`github\.com`),
				Type:         codersdk.EnhancedExternalAuthProviderGitHub.String(),
			}},
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)

		resp := coderdtest.RequestExternalAuthCallback(t, "github", client)
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

		// If the validation URL says unauthorized, the callback
		// URL to re-authenticate should be returned.
		srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		res, err := agentClient.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{
			Match: "github.com/asd/asd",
		})
		require.NoError(t, err)
		require.NotEmpty(t, res.URL)

		// If the validation URL gives a non-OK status code, this
		// should be treated as an internal server error.
		srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Something went wrong!"))
		})
		_, err = agentClient.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{
			Match: "github.com/asd/asd",
		})
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusInternalServerError, apiError.StatusCode())
		require.Equal(t, "validate external auth token: status 403: body: Something went wrong!", apiError.Detail)
	})

	t.Run("ExpiredNoRefresh", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			ExternalAuthConfigs: []*externalauth.Config{{
				OAuth2Config: &testutil.OAuth2Config{
					Token: &oauth2.Token{
						AccessToken:  "token",
						RefreshToken: "something",
						Expiry:       dbtime.Now().Add(-time.Hour),
					},
				},
				ID:        "github",
				Regex:     regexp.MustCompile(`github\.com`),
				Type:      codersdk.EnhancedExternalAuthProviderGitHub.String(),
				NoRefresh: true,
			}},
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)

		token, err := agentClient.ExternalAuth(context.Background(), agentsdk.ExternalAuthRequest{
			Match: "github.com/asd/asd",
		})
		require.NoError(t, err)
		require.NotEmpty(t, token.URL)

		// In the configuration, we set our OAuth provider
		// to return an expired token. Coder consumes this
		// and stores it.
		resp := coderdtest.RequestExternalAuthCallback(t, "github", client)
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)

		// Because the token is expired and `NoRefresh` is specified,
		// a redirect URL should be returned again.
		token, err = agentClient.ExternalAuth(context.Background(), agentsdk.ExternalAuthRequest{
			Match: "github.com/asd/asd",
		})
		require.NoError(t, err)
		require.NotEmpty(t, token.URL)
	})

	t.Run("FullFlow", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			ExternalAuthConfigs: []*externalauth.Config{{
				OAuth2Config: &testutil.OAuth2Config{},
				ID:           "github",
				Regex:        regexp.MustCompile(`github\.com`),
				Type:         codersdk.EnhancedExternalAuthProviderGitHub.String(),
			}},
		})
		user := coderdtest.CreateFirstUser(t, client)
		authToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)

		token, err := agentClient.ExternalAuth(context.Background(), agentsdk.ExternalAuthRequest{
			Match: "github.com/asd/asd",
		})
		require.NoError(t, err)
		require.NotEmpty(t, token.URL)

		// Start waiting for the token callback...
		tokenChan := make(chan agentsdk.ExternalAuthResponse, 1)
		go func() {
			token, err := agentClient.ExternalAuth(context.Background(), agentsdk.ExternalAuthRequest{
				Match:  "github.com/asd/asd",
				Listen: true,
			})
			assert.NoError(t, err)
			tokenChan <- token
		}()

		time.Sleep(250 * time.Millisecond)

		resp := coderdtest.RequestExternalAuthCallback(t, "github", client)
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		token = <-tokenChan
		require.Equal(t, "access_token", token.Username)

		token, err = agentClient.ExternalAuth(context.Background(), agentsdk.ExternalAuthRequest{
			Match: "github.com/asd/asd",
		})
		require.NoError(t, err)
	})
}
