package coderd_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestChatProjectMCPServerGitHubApp(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	key := newGitHubAppKey(t)
	github := newFakeGitHubApp(t, &key.PublicKey)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID

	server := createGitHubAppMCPServer(t, client, orgID, github.srv.URL)
	project := createChatProject(t, client, orgID, "Reviewer")
	servers := []uuid.UUID{server.ID}
	_, err := client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{MCPServerIDs: &servers})
	require.NoError(t, err)
	set := func(req codersdk.UpdateMCPServerUserTokenRequest) (codersdk.MCPServerUserTokenStatus, error) {
		return client.SetChatProjectMCPServerToken(ctx, orgID, project.ID, server.ID, req)
	}
	app := func(appID, installationID int64, privateKey string, repos ...string) codersdk.UpdateMCPServerUserTokenRequest {
		return codersdk.UpdateMCPServerUserTokenRequest{GitHubApp: &codersdk.MCPServerGitHubAppCredentialRequest{
			AppID: appID, InstallationID: installationID, PrivateKey: privateKey, Repositories: repos,
		}}
	}
	//nolint:gocritic // Reads the stored credentials directly.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	storedApps := func(userID uuid.UUID) []database.MCPServerGitHubAppCredential {
		creds, err := db.GetMCPServerGitHubAppCredentialsByUserID(sysCtx, userID)
		require.NoError(t, err)
		return creds
	}
	storedTokens := func(userID uuid.UUID) []database.MCPServerUserToken {
		tokens, err := db.GetMCPServerUserTokensByUserID(sysCtx, userID)
		require.NoError(t, err)
		return tokens
	}

	t.Log("Coder checks the credential with GitHub before saving it")
	_, err = set(app(42, 7, "not a key"))
	sdkErr := coderdtest.SDKError(t, err)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	require.Equal(t, "github_app.private_key", sdkErr.Validations[0].Field)

	_, err = set(app(42, 7, gitHubAppKeyPEM(newGitHubAppKey(t))))
	sdkErr = coderdtest.SDKError(t, err)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	require.Equal(t, "GitHub rejected the GitHub App credential.", sdkErr.Message)
	require.Contains(t, sdkErr.Detail, "Check the app ID and the private key.")
	require.Contains(t, sdkErr.Detail, "A JSON web token could not be decoded")

	_, err = set(app(42, 404, gitHubAppKeyPEM(key)))
	sdkErr = coderdtest.SDKError(t, err)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	require.Contains(t, sdkErr.Detail, "GitHub said: Not Found")
	require.Empty(t, storedApps(project.AgentUser.ID))

	t.Log("A valid credential is stored for the agent's user")
	status, err := set(app(42, 7, gitHubAppKeyPEM(key), "coder/coder"))
	require.NoError(t, err)
	require.True(t, status.HasToken)
	require.Equal(t, codersdk.MCPServerCredentialKindGitHubApp, status.CredentialKind)
	require.Equal(t, &codersdk.MCPServerGitHubApp{AppID: 42, InstallationID: 7, Repositories: []string{"coder"}}, status.GitHubApp)
	require.False(t, status.GitHubApp.UseForGit, "git access is off unless asked for")
	require.Equal(t, []string{"coder"}, github.lastRepositories())
	require.Equal(t, "42", github.lastIssuer())

	apps := storedApps(project.AgentUser.ID)
	require.Len(t, apps, 1)
	require.Equal(t, gitHubAppKeyPEM(key), apps[0].PrivateKey+"\n")
	require.Empty(t, storedApps(firstUser.UserID), "only the agent's user gets the app")

	list, err := client.ChatProjectMCPServerTokens(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.True(t, list[0].HasToken)
	require.Equal(t, codersdk.MCPServerCredentialKindGitHubApp, list[0].CredentialKind)
	require.Equal(t, status.GitHubApp, list[0].GitHubApp)
	raw, err := json.Marshal(list)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "PRIVATE KEY")

	t.Log("Only the owner sees the app's setup details")
	editorSDK, editor := coderdtest.CreateAnotherUser(t, client.Client, orgID)
	editorClient := codersdk.NewExperimentalClient(editorSDK)
	require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACLRequest{
		UserRoles: map[string]codersdk.ChatProjectRole{editor.ID.String(): codersdk.ChatProjectRoleEdit},
	}))
	editorList, err := editorClient.ChatProjectMCPServerTokens(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Len(t, editorList, 1)
	require.True(t, editorList[0].HasToken)
	require.Equal(t, codersdk.MCPServerCredentialKindGitHubApp, editorList[0].CredentialKind)
	require.Nil(t, editorList[0].GitHubApp)

	t.Log("People cannot use GitHub Apps for themselves")
	_, err = client.SetMCPServerUserToken(ctx, server.ID, app(42, 7, gitHubAppKeyPEM(key)))
	sdkErr = coderdtest.SDKError(t, err)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	require.Equal(t, "credential_kind", sdkErr.Validations[0].Field)

	t.Log("Saving the same app again keeps its write-only key")
	status, err = set(app(42, 7, "", "docs"))
	require.NoError(t, err)
	require.Equal(t, []string{"docs"}, status.GitHubApp.Repositories)
	require.Equal(t, apps[0].PrivateKey, storedApps(project.AgentUser.ID)[0].PrivateKey)

	t.Log("Saving an unchanged app again gets a fresh token, picking up new app permissions")
	before := github.calls.Load()
	_, err = set(app(42, 7, "", "docs"))
	require.NoError(t, err)
	require.Equal(t, before+1, github.calls.Load())

	_, err = set(app(43, 7, ""))
	sdkErr = coderdtest.SDKError(t, err)
	require.Equal(t, "github_app.private_key", sdkErr.Validations[0].Field)

	t.Log("A pasted token and a GitHub App replace each other")
	status, err = set(codersdk.UpdateMCPServerUserTokenRequest{Token: "ghp_pasted"})
	require.NoError(t, err)
	require.Equal(t, codersdk.MCPServerCredentialKindToken, status.CredentialKind)
	require.Empty(t, storedApps(project.AgentUser.ID))
	require.Len(t, storedTokens(project.AgentUser.ID), 1)
	_, err = set(app(42, 7, gitHubAppKeyPEM(key)))
	require.NoError(t, err)
	require.Empty(t, storedTokens(project.AgentUser.ID))

	t.Log("Removing the credential removes the app")
	require.NoError(t, client.DeleteChatProjectMCPServerToken(ctx, orgID, project.ID, server.ID))
	require.Empty(t, storedApps(project.AgentUser.ID))
	list, err = client.ChatProjectMCPServerTokens(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.False(t, list[0].HasToken)
	require.Empty(t, list[0].CredentialKind)

	t.Log("Pointing the server at another GitHub drops the app")
	_, err = set(app(42, 7, gitHubAppKeyPEM(key)))
	require.NoError(t, err)
	otherURL := server.URL + "&toolsets=repos"
	_, err = client.UpdateMCPServerConfig(ctx, orgID, server.ID, codersdk.UpdateMCPServerConfigRequest{URL: &otherURL})
	require.NoError(t, err)
	require.Empty(t, storedApps(project.AgentUser.ID))
}

func TestWorkspaceAgentExternalAuthGitHubApp(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	key := newGitHubAppKey(t)
	github := newFakeGitHubApp(t, &key.PublicKey)
	client, db := newChatClientWithDatabase(t, func(options *coderdtest.Options) {
		options.DeploymentValues.Experiments = serpent.StringArray{string(codersdk.ExperimentChatProjects)}
		options.ExternalAuthConfigs = []*externalauth.Config{{
			ID:                       "github",
			Type:                     codersdk.EnhancedExternalAuthProviderGitHub.String(),
			Regex:                    regexp.MustCompile(`github\.com`),
			ValidateURL:              github.srv.URL + "/user",
			InstrumentedOAuth2Config: &testutil.OAuth2Config{},
			RefreshGroup:             new(singleflight.Group),
		}}
	}, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID

	server := createGitHubAppMCPServer(t, client, orgID, github.srv.URL)
	project := createChatProject(t, client, orgID, "Reviewer")
	servers := []uuid.UUID{server.ID}
	_, err := client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{MCPServerIDs: &servers})
	require.NoError(t, err)
	_, err = client.SetChatProjectMCPServerToken(ctx, orgID, project.ID, server.ID, codersdk.UpdateMCPServerUserTokenRequest{
		GitHubApp: &codersdk.MCPServerGitHubAppCredentialRequest{AppID: 42, InstallationID: 7, PrivateKey: gitHubAppKeyPEM(key)},
	})
	require.NoError(t, err)

	agentWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: orgID,
		OwnerID:        project.AgentUser.ID,
	}).WithAgent().Do()
	agentClient := agentsdk.New(client.URL, agentsdk.WithFixedToken(agentWorkspace.AgentToken))

	t.Log("Git does not use the app until the owner turns on git access")
	resp, err := agentClient.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{ID: "github"})
	require.NoError(t, err)
	require.NotEmpty(t, resp.URL)
	require.Empty(t, resp.AccessToken)

	t.Log("Git access without a repository list covers the whole installation")
	status, err := client.SetChatProjectMCPServerToken(ctx, orgID, project.ID, server.ID, codersdk.UpdateMCPServerUserTokenRequest{
		GitHubApp: &codersdk.MCPServerGitHubAppCredentialRequest{AppID: 42, InstallationID: 7, UseForGit: true},
	})
	require.NoError(t, err)
	require.True(t, status.GitHubApp.UseForGit)
	require.Empty(t, status.GitHubApp.Repositories)
	wholeInstall, err := agentClient.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{ID: "github"})
	require.NoError(t, err)
	require.Empty(t, wholeInstall.URL)
	require.Equal(t, "x-access-token", wholeInstall.Username)
	require.NotEmpty(t, wholeInstall.Password)
	require.Empty(t, github.lastRepositories(), "tokens without a list are not limited to repositories")

	status, err = client.SetChatProjectMCPServerToken(ctx, orgID, project.ID, server.ID, codersdk.UpdateMCPServerUserTokenRequest{
		GitHubApp: &codersdk.MCPServerGitHubAppCredentialRequest{AppID: 42, InstallationID: 7, Repositories: []string{"coder/coder"}, UseForGit: true},
	})
	require.NoError(t, err)
	require.True(t, status.GitHubApp.UseForGit)
	minted := github.calls.Load()

	t.Log("The agent's workspace gets the app's installation token for git")
	byID, err := agentClient.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{ID: "github"})
	require.NoError(t, err)
	require.Empty(t, byID.URL)
	require.Equal(t, "x-access-token", byID.Username)
	require.Equal(t, fmt.Sprintf("ghs_%d", minted), byID.Password)
	require.Equal(t, byID.Password, byID.AccessToken)
	require.False(t, byID.ExpiresAt.IsZero())

	byMatch, err := agentClient.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{Match: "https://github.com/coder/coder"})
	require.NoError(t, err)
	require.Equal(t, byID.Password, byMatch.Password)
	require.Equal(t, minted, github.calls.Load(), "the token saved with the credential is reused")
	require.Equal(t, []string{"coder"}, github.lastRepositories(), "git tokens only cover the listed repositories")

	t.Log("A person's workspace still uses their own OAuth link")
	personWorkspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: orgID,
		OwnerID:        firstUser.UserID,
	}).WithAgent().Do()
	personAgent := agentsdk.New(client.URL, agentsdk.WithFixedToken(personWorkspace.AgentToken))
	resp, err = personAgent.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{ID: "github"})
	require.NoError(t, err)
	require.NotEmpty(t, resp.URL)
	require.Empty(t, resp.AccessToken)

	t.Log("Detaching the server from the agent stops lending its identity")
	servers = []uuid.UUID{}
	_, err = client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{MCPServerIDs: &servers})
	require.NoError(t, err)
	resp, err = agentClient.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{ID: "github"})
	require.NoError(t, err)
	require.NotEmpty(t, resp.URL)
	require.Empty(t, resp.AccessToken)
}

func createGitHubAppMCPServer(t *testing.T, client *codersdk.ExperimentalClient, orgID uuid.UUID, apiBaseURL string) codersdk.MCPServerConfig {
	t.Helper()
	server, err := client.CreateMCPServerConfig(testutil.Context(t, testutil.WaitLong), orgID, codersdk.CreateMCPServerConfigRequest{
		DisplayName:  "GitHub",
		Slug:         "github",
		Transport:    "streamable_http",
		URL:          "https://mcp.example.com/github?api_base_url=" + url.QueryEscape(apiBaseURL),
		AuthType:     codersdk.MCPServerAuthTypeUserAPIKey,
		Availability: "default_off",
		Enabled:      true,
	})
	require.NoError(t, err)
	return server
}

// fakeGitHubApp serves GitHub's installation token endpoint. It verifies
// the app JWT against the app's public key and knows no installation 404.
type fakeGitHubApp struct {
	srv   *httptest.Server
	calls atomic.Int64

	mu     sync.Mutex
	repos  []string
	issuer string
}

func (f *fakeGitHubApp) lastRepositories() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.repos
}

func (f *fakeGitHubApp) lastIssuer() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.issuer
}

func newFakeGitHubApp(t *testing.T, pub *rsa.PublicKey) *fakeGitHubApp {
	t.Helper()
	f := &fakeGitHubApp{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /app/installations/{id}/access_tokens", func(rw http.ResponseWriter, r *http.Request) {
		writeErr := func(status int, msg string) {
			rw.WriteHeader(status)
			_ = json.NewEncoder(rw).Encode(map[string]string{"message": msg})
		}
		const prefix = "Bearer "
		auth := r.Header.Get("Authorization")
		if len(auth) <= len(prefix) {
			writeErr(http.StatusUnauthorized, "A JSON web token could not be decoded")
			return
		}
		parsed, err := jwt.ParseSigned(auth[len(prefix):], []jose.SignatureAlgorithm{jose.RS256})
		var claims jwt.Claims
		if err == nil {
			err = parsed.Claims(pub, &claims)
		}
		if err == nil {
			err = claims.ValidateWithLeeway(jwt.Expected{Time: time.Now()}, time.Minute)
		}
		if err != nil {
			writeErr(http.StatusUnauthorized, "A JSON web token could not be decoded")
			return
		}
		if r.PathValue("id") == "404" {
			writeErr(http.StatusNotFound, "Not Found")
			return
		}
		var body struct {
			Repositories []string `json:"repositories"`
		}
		if r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		n := f.calls.Add(1)
		f.mu.Lock()
		f.repos, f.issuer = body.Repositories, claims.Issuer
		f.mu.Unlock()
		rw.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"token":      fmt.Sprintf("ghs_%d", n),
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func newGitHubAppKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func gitHubAppKeyPEM(key *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}
