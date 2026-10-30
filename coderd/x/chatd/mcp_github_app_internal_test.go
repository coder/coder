package chatd

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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/coderd/x/githubapp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestGitHubAppMCPTokens(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	authzDB := dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), logger, nil)
	ctx := dbauthz.AsChatd(testutil.Context(t, testutil.WaitLong))

	var calls atomic.Int64
	github := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/app/installations/404/access_tokens" {
			rw.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(rw).Encode(map[string]string{"message": "Not Found"})
			return
		}
		n := calls.Add(1)
		rw.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"token":      fmt.Sprintf("ghs_%d", n),
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	t.Cleanup(github.Close)
	server := &Server{db: authzDB, githubAppTokens: githubapp.NewMinter(github.Client(), quartz.NewReal())}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))

	agentUser := dbgen.User(t, db, database.User{IsAgent: true, LoginType: database.LoginTypeNone, Email: ""})
	person := dbgen.User(t, db, database.User{})
	serverURL := "https://coder.example.com/api/experimental/mcp/builtin/github?api_base_url=" + url.QueryEscape(github.URL)
	newConfig := func(authType, slug string) database.MCPServerConfig {
		return dbgen.MCPServerConfig(t, db, database.MCPServerConfig{AuthType: authType, Slug: slug, Url: serverURL, Enabled: true})
	}
	viaApp := newConfig(codersdk.MCPServerAuthTypeUserAPIKey, "github")
	failing := newConfig(codersdk.MCPServerAuthTypeUserAPIKey, "github-broken")
	pasted := newConfig(codersdk.MCPServerAuthTypeUserAPIKey, "slack")
	notAttached := newConfig(codersdk.MCPServerAuthTypeUserAPIKey, "other")
	for _, c := range []struct {
		config         database.MCPServerConfig
		installationID int64
	}{{viaApp, 7}, {failing, 404}, {notAttached, 7}} {
		_, err := db.UpsertMCPServerGitHubAppCredential(ctx, database.UpsertMCPServerGitHubAppCredentialParams{
			MCPServerConfigID: c.config.ID,
			UserID:            agentUser.ID,
			AppID:             42,
			InstallationID:    c.installationID,
			PrivateKey:        keyPEM,
			Repositories:      []string{},
		})
		require.NoError(t, err)
	}
	stale := database.MCPServerUserToken{MCPServerConfigID: viaApp.ID, UserID: agentUser.ID, AccessToken: "ghp_stale"}
	slack := database.MCPServerUserToken{MCPServerConfigID: pasted.ID, UserID: agentUser.ID, AccessToken: "xoxb-pasted"}
	configs := []database.MCPServerConfig{viaApp, failing, pasted}
	attached := map[uuid.UUID]struct{}{viaApp.ID: {}, failing.ID: {}, pasted.ID: {}}

	tokens, failures := server.githubAppMCPTokens(ctx, logger, agentUser.ID, attached, configs, []database.MCPServerUserToken{stale, slack})
	byConfig := map[uuid.UUID]string{}
	for _, tok := range tokens {
		require.Equal(t, agentUser.ID, tok.UserID)
		byConfig[tok.MCPServerConfigID] = tok.AccessToken
	}
	require.Equal(t, map[uuid.UUID]string{viaApp.ID: "ghs_1", pasted.ID: "xoxb-pasted"}, byConfig,
		"the app's token replaces a pasted one, other servers keep theirs, and a failed mint sends nothing")
	require.Len(t, failures, 1)
	require.Equal(t, mcpclient.ConnectOutcomeError, failures[failing.ID].Outcome)
	require.Contains(t, failures[failing.ID].Error, "Not Found")
	require.EqualValues(t, 1, calls.Load(), "servers the chat does not use are not minted for")

	// The next turn reuses the cached token.
	tokens, _ = server.githubAppMCPTokens(ctx, logger, agentUser.ID, attached, configs[:1], nil)
	require.Len(t, tokens, 1)
	require.Equal(t, "ghs_1", tokens[0].AccessToken)
	require.EqualValues(t, 1, calls.Load())

	// A chat that lists a server the agent no longer has gets no app token.
	tokens, failures = server.githubAppMCPTokens(ctx, logger, agentUser.ID, map[uuid.UUID]struct{}{}, []database.MCPServerConfig{viaApp, notAttached}, nil)
	require.Empty(t, tokens)
	require.Empty(t, failures)

	// Other chat owners never get the agent's app identity.
	tokens, failures = server.githubAppMCPTokens(ctx, logger, person.ID, nil, configs, nil)
	require.Empty(t, tokens)
	require.Empty(t, failures)
}

func TestAgentAttachedMCPServers(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	authzDB := dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), logger, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	server := &Server{db: authzDB}

	owner := dbgen.User(t, db, database.User{})
	agentUser := dbgen.User(t, db, database.User{IsAgent: true, LoginType: database.LoginTypeNone, Email: ""})
	attachedConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{})
	detachedConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{OrganizationID: attachedConfig.OrganizationID})
	dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: attachedConfig.OrganizationID,
		OwnerID:        owner.ID,
		AgentUserID:    uuid.NullUUID{UUID: agentUser.ID, Valid: true},
		MCPServerIDs:   []uuid.UUID{attachedConfig.ID},
	})

	attached, err := server.agentAttachedMCPServers(ctx, agentUser.ID)
	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]struct{}{attachedConfig.ID: {}}, attached)
	tokens := onlyAttachedMCPTokens(attached, []database.MCPServerUserToken{
		{MCPServerConfigID: attachedConfig.ID, AccessToken: "kept"},
		{MCPServerConfigID: detachedConfig.ID, AccessToken: "dropped"},
	})
	require.Len(t, tokens, 1)
	require.Equal(t, "kept", tokens[0].AccessToken, "the agent's credentials only reach its attached servers")

	personServers, err := server.agentAttachedMCPServers(ctx, owner.ID)
	require.NoError(t, err)
	require.Nil(t, personServers)
	personTokens := []database.MCPServerUserToken{{MCPServerConfigID: detachedConfig.ID}}
	require.Equal(t, personTokens, onlyAttachedMCPTokens(personServers, personTokens), "a person's tokens are untouched")
}
