package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"golang.org/x/xerrors"

	aibridgeutils "github.com/coder/coder/v2/aibridge/utils"
	"github.com/coder/coder/v2/codersdk"
)

func logf(format string, args ...any) {
	_, _ = fmt.Printf(time.Now().Format("15:04:05")+" "+format+"\n", args...)
}

func runSetup(ctx context.Context, st *state, model string, skipWorkspace bool) error { //nolint:revive // skipWorkspace mirrors the -skip-workspace flag.
	admin, err := st.adminClient()
	if err != nil {
		return err
	}
	me, err := admin.User(ctx, codersdk.Me)
	if err != nil {
		return xerrors.Errorf("admin whoami: %w", err)
	}
	st.AdminID = me.ID
	org, err := admin.OrganizationByName(ctx, codersdk.DefaultOrganization)
	if err != nil {
		return err
	}
	st.OrgID = org.ID
	logf("admin=%s org=%s", me.Username, org.ID)

	// Users.
	for _, name := range []string{"alice", "bob", "carol"} {
		u, err := ensureUser(ctx, admin, org.ID, name)
		if err != nil {
			return xerrors.Errorf("ensure user %s: %w", name, err)
		}
		login, err := st.client("").LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    name + "@coder.com",
			Password: defaultPassword,
		})
		if err != nil {
			return xerrors.Errorf("login %s: %w", name, err)
		}
		us := st.Users[name]
		us.ID = u.ID
		us.Username = u.Username
		us.SessionToken = login.SessionToken
		st.Users[name] = us
		logf("user %s id=%s (session obtained)", name, u.ID)
	}
	if err := st.save(); err != nil {
		return err
	}

	// Provider + model.
	if err := ensureProviderAndModel(ctx, st, admin, model); err != nil {
		return err
	}
	if err := st.save(); err != nil {
		return err
	}

	// MCP config + seeded per-user tokens.
	if err := ensureMCPConfig(ctx, st, admin); err != nil {
		return err
	}
	if err := st.save(); err != nil {
		return err
	}

	// OAuth2 apps.
	if err := ensureOAuth2Apps(ctx, st, admin); err != nil {
		return err
	}
	if err := st.save(); err != nil {
		return err
	}

	// Template + workspace.
	if !skipWorkspace {
		if err := ensureWorkspace(ctx, st, admin); err != nil {
			return err
		}
	}
	return st.save()
}

func ensureUser(ctx context.Context, admin *codersdk.Client, orgID uuid.UUID, name string) (codersdk.User, error) {
	if u, err := admin.User(ctx, name); err == nil {
		return u, nil
	}
	active := codersdk.UserStatusActive
	return admin.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
		Email:           name + "@coder.com",
		Username:        name,
		Name:            strings.ToUpper(name[:1]) + name[1:],
		Password:        defaultPassword,
		UserLoginType:   codersdk.LoginTypePassword,
		UserStatus:      &active,
		OrganizationIDs: []uuid.UUID{orgID},
	})
}

func ensureProviderAndModel(ctx context.Context, st *state, admin *codersdk.Client, model string) error {
	providers, err := admin.AIProviders(ctx)
	if err != nil {
		return xerrors.Errorf("list providers: %w", err)
	}
	var provider *codersdk.AIProvider
	for i := range providers {
		p := providers[i]
		if p.Type == codersdk.AIProviderTypeOpenAI && p.Name == "openai" {
			provider = &p
			break
		}
	}
	// The environment key may belong to an OpenAI-compatible gateway; honor
	// OPENAI_BASE_URL when set.
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	key := os.Getenv("OPENAI_API_KEY")
	if provider == nil {
		if key == "" {
			return xerrors.New("OPENAI_API_KEY is not set")
		}
		p, err := admin.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:        codersdk.AIProviderTypeOpenAI,
			Name:        "openai",
			DisplayName: "OpenAI (dogfood)",
			Enabled:     true,
			BaseURL:     baseURL,
			APIKeys:     []string{key},
		})
		if err != nil {
			return xerrors.Errorf("create provider: %w", err)
		}
		provider = &p
	} else if provider.BaseURL != baseURL {
		p, err := admin.UpdateAIProvider(ctx, provider.ID.String(), codersdk.UpdateAIProviderRequest{BaseURL: &baseURL})
		if err != nil {
			return xerrors.Errorf("update provider base url: %w", err)
		}
		provider = &p
		logf("updated provider base_url to %s", baseURL)
	}
	// The AI Gateway key in the environment rotates between sessions. Replace
	// the stored key when none of the stored keys matches the current one.
	if key != "" && !providerHasKey(*provider, key) {
		p, err := admin.UpdateAIProvider(ctx, provider.ID.String(), codersdk.UpdateAIProviderRequest{
			APIKeys: &[]codersdk.AIProviderKeyMutation{{APIKey: &key}},
		})
		if err != nil {
			return xerrors.Errorf("replace provider api key: %w", err)
		}
		provider = &p
		logf("replaced provider api key with the OPENAI_API_KEY from the environment")
	}
	st.ProviderID = provider.ID
	logf("ai provider type=%s name=%s id=%s api_keys=%d enabled=%v base_url=%s", provider.Type, provider.Name, provider.ID, len(provider.APIKeys), provider.Enabled, provider.BaseURL)

	models, err := admin.ChatModels(ctx, st.OrgID)
	if err != nil {
		return xerrors.Errorf("list models: %w", err)
	}
	for _, m := range models.Models {
		if m.Model == model && m.AIProviderID == provider.ID {
			st.ModelConfigID = m.ID
			st.ModelName = m.Model
			logf("model %s already exists id=%s default=%v", m.Model, m.ID, m.IsDefault)
			return nil
		}
	}
	enabled, isDefault := true, true
	contextLimit := int64(128000)
	m, err := admin.CreateChatModel(ctx, st.OrgID, codersdk.CreateChatModelRequest{
		AIProviderID: &provider.ID,
		Model:        model,
		DisplayName:  model,
		Enabled:      &enabled,
		IsDefault:    &isDefault,
		ContextLimit: &contextLimit,
	})
	if err != nil {
		return xerrors.Errorf("create model %s: %w", model, err)
	}
	st.ModelConfigID = m.ID
	st.ModelName = m.Model
	logf("model created %s id=%s default=%v", m.Model, m.ID, m.IsDefault)
	return nil
}

func ensureMCPConfig(ctx context.Context, st *state, admin *codersdk.Client) error {
	configs, err := admin.MCPServerConfigs(ctx, st.OrgID)
	if err != nil {
		return xerrors.Errorf("list mcp configs: %w", err)
	}
	var cfg *codersdk.MCPServerConfig
	for i := range configs {
		if configs[i].Slug == mcpSlug {
			cfg = &configs[i]
			break
		}
	}
	if cfg == nil {
		c, err := admin.CreateMCPServerConfig(ctx, st.OrgID, codersdk.CreateMCPServerConfigRequest{
			DisplayName: "Dogfood whoami",
			Slug:        mcpSlug,
			Description: "Echoes Coder identity headers.",
			Transport:   "streamable_http",
			URL:         mcpServerURL,
			// oauth2 auth type so chatd attaches the per-user token stored in
			// mcp_server_user_tokens. The endpoints are never called because
			// tokens are seeded directly.
			AuthType:            "oauth2",
			OAuth2ClientID:      "dogfood-client",
			OAuth2AuthURL:       "http://127.0.0.1:3999/oauth/authorize",
			OAuth2TokenURL:      "http://127.0.0.1:3999/oauth/token",
			ToolDenyList:        []string{"noop"},
			Availability:        "force_on",
			Enabled:             true,
			ForwardCoderHeaders: true,
		})
		if err != nil {
			return xerrors.Errorf("create mcp config: %w", err)
		}
		cfg = &c
	}
	st.MCPConfigID = cfg.ID
	logf("mcp config slug=%s id=%s url=%s auth_type=%s availability=%s forward_coder_headers=%v deny=%v",
		cfg.Slug, cfg.ID, cfg.URL, cfg.AuthType, cfg.Availability, cfg.ForwardCoderHeaders, cfg.ToolDenyList)

	// Seed per-user tokens for alice and bob via SQL (no API exists; dev DB
	// has no dbcrypt so plaintext is what chatd reads).
	db, err := openDevDB()
	if err != nil {
		return err
	}
	defer db.Close()
	for _, name := range []string{"alice", "bob"} {
		us := st.Users[name]
		if us.MCPToken == "" {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				return err
			}
			us.MCPToken = "mcp-" + name + "-" + hex.EncodeToString(b)
		}
		_, err := db.ExecContext(ctx, `
INSERT INTO mcp_server_user_tokens (id, mcp_server_config_id, user_id, access_token, token_type, created_at, updated_at)
VALUES ($1, $2, $3, $4, 'Bearer', now(), now())
ON CONFLICT (mcp_server_config_id, user_id) DO UPDATE SET access_token = EXCLUDED.access_token, updated_at = now(), oauth_refresh_failure_reason = ''`,
			uuid.New(), cfg.ID, us.ID, us.MCPToken)
		if err != nil {
			return xerrors.Errorf("seed mcp token for %s: %w", name, err)
		}
		st.Users[name] = us
		logf("seeded mcp_server_user_tokens row for %s (config %s)", name, cfg.ID)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM mcp_server_user_tokens WHERE mcp_server_config_id = $1`, cfg.ID).Scan(&n); err != nil {
		return err
	}
	logf("mcp_server_user_tokens rows for config: %d", n)
	return nil
}

// devDBURL builds the built-in Postgres URL used by scripts/develop.sh.
func devDBURL() (string, error) {
	port, err := os.ReadFile(".coderv2/postgres/port")
	if err != nil {
		return "", xerrors.Errorf("read postgres port: %w", err)
	}
	pw, err := os.ReadFile(".coderv2/postgres/password")
	if err != nil {
		return "", xerrors.Errorf("read postgres password: %w", err)
	}
	return fmt.Sprintf("postgres://coder@localhost:%s/coder?sslmode=disable&password=%s",
		strings.TrimSpace(string(port)), url.QueryEscape(strings.TrimSpace(string(pw)))), nil
}

func openDevDB() (*sql.DB, error) {
	u, err := devDBURL()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("postgres", u)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, xerrors.Errorf("ping dev db: %w", err)
	}
	return db, nil
}

func ensureOAuth2Apps(ctx context.Context, st *state, admin *codersdk.Client) error {
	if st.AppID == uuid.Nil {
		app, err := admin.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "slack-bot",
			CallbackURL: callbackURL,
		})
		if err != nil {
			return xerrors.Errorf("create oauth2 app: %w", err)
		}
		secret, err := admin.PostOAuth2ProviderAppSecret(ctx, app.ID)
		if err != nil {
			return xerrors.Errorf("create oauth2 app secret: %w", err)
		}
		st.AppID = app.ID
		st.AppSecret = secret.ClientSecretFull
		logf("oauth2 app slack-bot id=%s callback=%s authorize=%s token=%s", app.ID, app.CallbackURL, app.Endpoints.Authorization, app.Endpoints.Token)
	}
	if st.NoShareAppID == uuid.Nil {
		app, err := admin.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "slack-bot-noshare",
			CallbackURL: callbackURL,
		})
		if err != nil {
			return xerrors.Errorf("create oauth2 noshare app: %w", err)
		}
		secret, err := admin.PostOAuth2ProviderAppSecret(ctx, app.ID)
		if err != nil {
			return xerrors.Errorf("create oauth2 noshare app secret: %w", err)
		}
		st.NoShareAppID = app.ID
		st.NoShareSecret = secret.ClientSecretFull
		logf("oauth2 app slack-bot-noshare id=%s", app.ID)
	}
	return nil
}

// providerHasKey reports whether one of the provider's stored keys masks to
// the same value as key. The API never returns plaintext keys.
func providerHasKey(provider codersdk.AIProvider, key string) bool {
	masked := aibridgeutils.MaskSecret(key)
	for _, k := range provider.APIKeys {
		if k.Masked == masked {
			return true
		}
	}
	return false
}

func ensureWorkspace(ctx context.Context, st *state, admin *codersdk.Client) error {
	tpl, err := admin.TemplateByName(ctx, st.OrgID, "docker")
	if err != nil {
		return xerrors.Errorf("template docker: %w", err)
	}
	st.TemplateID = tpl.ID
	alice := st.client(st.Users["alice"].SessionToken)
	ws, err := alice.WorkspaceByOwnerAndName(ctx, codersdk.Me, workspaceName, codersdk.WorkspaceOptions{})
	if err != nil {
		ws, err = alice.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateID: tpl.ID,
			Name:       workspaceName,
		})
		if err != nil {
			return xerrors.Errorf("create workspace: %w", err)
		}
		logf("workspace %s/%s created id=%s build=%s", ws.OwnerName, ws.Name, ws.ID, ws.LatestBuild.ID)
	}
	st.WorkspaceID = ws.ID
	st.WorkspaceName = ws.Name
	if err := st.save(); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Minute)
	rebuilt := false
	for time.Now().Before(deadline) {
		ws, err = alice.Workspace(ctx, ws.ID)
		if err != nil {
			return err
		}
		if ws.LatestBuild.Job.Status == codersdk.ProvisionerJobFailed {
			return xerrors.Errorf("workspace build failed: %s", ws.LatestBuild.Job.Error)
		}
		for _, r := range ws.LatestBuild.Resources {
			for _, a := range r.Agents {
				if a.Status == codersdk.WorkspaceAgentConnected && a.LifecycleState == codersdk.WorkspaceAgentLifecycleReady {
					st.AgentID = a.ID
					logf("workspace agent %s status=%s lifecycle=%s", a.ID, a.Status, a.LifecycleState)
					return nil
				}
			}
		}
		// A workspace left over from an earlier run reports a running build
		// after its docker container is gone. Start it again once so the
		// agent comes back instead of waiting out the deadline.
		if !rebuilt && ws.LatestBuild.Status == codersdk.WorkspaceStatusRunning && agentsDisconnected(ws) {
			build, err := alice.CreateWorkspaceBuild(ctx, ws.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
			})
			if err != nil {
				return xerrors.Errorf("restart stale workspace: %w", err)
			}
			rebuilt = true
			logf("workspace agent disconnected; started build %s", build.ID)
			continue
		}
		logf("waiting for workspace: job=%s build_status=%s", ws.LatestBuild.Job.Status, ws.LatestBuild.Status)
		time.Sleep(5 * time.Second)
	}
	return xerrors.New("workspace agent did not become ready in time")
}

// agentsDisconnected reports whether the workspace has agents and none of
// them is connected or still connecting.
func agentsDisconnected(ws codersdk.Workspace) bool {
	found := false
	for _, r := range ws.LatestBuild.Resources {
		for _, a := range r.Agents {
			found = true
			if a.Status != codersdk.WorkspaceAgentDisconnected && a.Status != codersdk.WorkspaceAgentTimeout {
				return false
			}
		}
	}
	return found
}
