// Command botemu emulates a first-party Slack bot driving the Coder
// Agents (chats) API with several users in one chat.
//
// Subcommands:
//
//	setup   create users, provider, model, workspace, MCP config, OAuth2 apps
//	tokens  obtain OAuth2 access tokens for alice, bob, carol (and alice no-share)
//	run     run scenarios S0..S8 (S9 with -s9; S4A, S9B, S9C only via -only) and write results
//
// Run it from the repository root; it reads the develop.sh session and
// Postgres credentials from .coderv2. Secrets (session tokens, OAuth2
// access tokens, MCP bearer tokens) are stored only in state.json (mode
// 0600) under the harness directory and are never printed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/codersdk"
)

const (
	defaultAccessURL = "http://127.0.0.1:3000"
	defaultPassword  = "SomeSecurePassword!"
	harnessDir       = "scripts/chatactor-dogfood"
	stateFile        = harnessDir + "/state.json"
	resultsFile      = harnessDir + "/results.md"
	developLog       = harnessDir + "/logs/develop.log"
	mcpServerURL     = "http://127.0.0.1:3999/mcp"
	mcpSlug          = "whoami"
	workspaceName    = "alice-ws"
	callbackURL      = "http://localhost:9876/callback"
	// botScopes is the scope set known to pass S0..S8. Without user:read
	// and workspace:ssh, GET /api/v2/users/me returned 404 (S1). Without
	// chat_model_config:read, POST /api/v2/chats returned 400 "No chat
	// model is available" (S2). Posting, interrupting, and submitting tool
	// results need chat:use. A bot does not need chat:update; it stays
	// here so the S7 archive denial exercises the chat ACL, not the scope.
	// Without mcp_server_config:read, GET /organizations/{org}/mcp-servers
	// returns an empty list for a scoped token (S4).
	botScopes = "chat:create chat:read chat:use chat:update chat:share workspace:read workspace:share user:read_personal user:read workspace:ssh chat_model_config:read mcp_server_config:read"
	// noShareScopes is a bot scope set without chat:share, used only to
	// prove that PATCH /chats/{id}/acl is denied without that scope.
	noShareScopes = "chat:create chat:read chat:use chat:update workspace:read workspace:share user:read_personal"
)

// userState holds one Coder user's identity and credentials.
type userState struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	SessionToken string    `json:"session_token"`
	// AccessToken is the OAuth2 access token from the full-scope app.
	AccessToken string `json:"access_token"`
	// Scopes is the scope string the token was issued with.
	Scopes string `json:"scopes"`
	// MCPToken is the per-user bearer token seeded for the MCP server.
	MCPToken string `json:"mcp_token,omitempty"`
}

// state is persisted between subcommands.
type state struct {
	AccessURL      string               `json:"access_url"`
	OrgID          uuid.UUID            `json:"org_id"`
	AdminID        uuid.UUID            `json:"admin_id"`
	Users          map[string]userState `json:"users"`
	ProviderID     uuid.UUID            `json:"provider_id"`
	ModelConfigID  uuid.UUID            `json:"model_config_id"`
	ModelName      string               `json:"model_name"`
	TemplateID     uuid.UUID            `json:"template_id"`
	WorkspaceID    uuid.UUID            `json:"workspace_id"`
	WorkspaceName  string               `json:"workspace_name"`
	AgentID        uuid.UUID            `json:"agent_id"`
	MCPConfigID    uuid.UUID            `json:"mcp_config_id"`
	AppID          uuid.UUID            `json:"app_id"`
	AppSecret      string               `json:"app_secret"`
	NoShareAppID   uuid.UUID            `json:"noshare_app_id"`
	NoShareSecret  string               `json:"noshare_app_secret"`
	AliceNoShareTk string               `json:"alice_noshare_token"`
	// ChatID is the chat created by S2; later scenarios reuse it.
	ChatID uuid.UUID `json:"chat_id"`
	// S9BChatID and S9BWorkspaceID are the unbound chat and bob's workspace
	// created by S9B.
	S9BChatID      uuid.UUID `json:"s9b_chat_id,omitempty"`
	S9BWorkspaceID uuid.UUID `json:"s9b_workspace_id,omitempty"`
	// S9CChatID and S9CWorkspaceID are the bound chat and bob's replacement
	// workspace created by S9C.
	S9CChatID      uuid.UUID `json:"s9c_chat_id,omitempty"`
	S9CWorkspaceID uuid.UUID `json:"s9c_workspace_id,omitempty"`
}

func loadState() (*state, error) {
	b, err := os.ReadFile(stateFile)
	if os.IsNotExist(err) {
		return &state{AccessURL: defaultAccessURL, Users: map[string]userState{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var s state
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.Users == nil {
		s.Users = map[string]userState{}
	}
	return &s, nil
}

func (s *state) save() error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(stateFile), 0o700); err != nil {
		return err
	}
	return os.WriteFile(stateFile, b, 0o600)
}

func (s *state) client(token string) *codersdk.Client {
	u, _ := url.Parse(s.AccessURL)
	c := codersdk.New(u)
	c.SetSessionToken(token)
	return c
}

// adminClient reads the develop.sh admin session from .coderv2.
func (s *state) adminClient() (*codersdk.Client, error) {
	root := config.Root(".coderv2")
	tok, err := root.Session().Read()
	if err != nil {
		return nil, xerrors.Errorf("read .coderv2 session: %w", err)
	}
	return s.client(tok), nil
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "botemu:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return xerrors.New("usage: botemu <setup|tokens|run> [flags]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	st, err := loadState()
	if err != nil {
		return err
	}
	var runErr error
	switch os.Args[1] {
	case "setup":
		fs := flag.NewFlagSet("setup", flag.ExitOnError)
		model := fs.String("model", "gpt-4.1-mini", "OpenAI model name")
		skipWorkspace := fs.Bool("skip-workspace", false, "do not create the workspace")
		_ = fs.Parse(os.Args[2:])
		runErr = runSetup(ctx, st, *model, *skipWorkspace)
	case "tokens":
		fs := flag.NewFlagSet("tokens", flag.ExitOnError)
		scopes := fs.String("scopes", botScopes, "scope set for the full app tokens")
		_ = fs.Parse(os.Args[2:])
		runErr = runTokens(ctx, st, *scopes)
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		only := fs.String("only", "", "comma-separated scenario IDs to run (default all S0..S8; S4A, S9B, S9C run only when listed)")
		s9 := fs.Bool("s9", false, "also run the optional S9 scenario")
		newChat := fs.Bool("new-chat", true, "create a fresh chat in S2 (false reuses state chat_id)")
		_ = fs.Parse(os.Args[2:])
		runErr = runScenarios(ctx, st, *only, *s9, *newChat)
	default:
		runErr = xerrors.Errorf("unknown subcommand %q", os.Args[1])
	}
	if saveErr := st.save(); saveErr != nil {
		return saveErr
	}
	return runErr
}

// httpStatus extracts the HTTP status code from a codersdk error, or 0.
func httpStatus(err error) int {
	var sdkErr *codersdk.Error
	if ok := asSDKError(err, &sdkErr); ok {
		return sdkErr.StatusCode()
	}
	return 0
}

func asSDKError(err error, target **codersdk.Error) bool {
	e, ok := codersdk.AsError(err)
	if ok {
		*target = e
	}
	return ok
}

// errBody returns the API error message and detail without secrets.
func errBody(err error) string {
	if err == nil {
		return "<nil>"
	}
	if e, ok := codersdk.AsError(err); ok {
		return fmt.Sprintf("status=%d message=%q detail=%q", e.StatusCode(), e.Message, e.Detail)
	}
	return err.Error()
}
