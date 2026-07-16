package chattool_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/slack-go/slack"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
)

// mcpFakeSlackAPI implements chattool.SlackAPI for the MCP tool tests.
type mcpFakeSlackAPI struct {
	postChannel string
	postOptions []slack.MsgOption
	postTS      string
	postErr     error

	updateChannel string
	updateTS      string
}

func (f *mcpFakeSlackAPI) PostMessageContext(_ context.Context, channelID string, options ...slack.MsgOption) (respChannel, respTS string, err error) {
	f.postChannel = channelID
	f.postOptions = options
	if f.postErr != nil {
		return "", "", f.postErr
	}
	return channelID, f.postTS, nil
}

func (f *mcpFakeSlackAPI) UpdateMessageContext(_ context.Context, channelID, timestamp string, _ ...slack.MsgOption) (respChannel, respTS, respText string, err error) {
	f.updateChannel = channelID
	f.updateTS = timestamp
	return channelID, timestamp, "", nil
}

func (*mcpFakeSlackAPI) AddReactionContext(context.Context, string, slack.ItemRef) error {
	return nil
}

func (*mcpFakeSlackAPI) RemoveReactionContext(context.Context, string, slack.ItemRef) error {
	return nil
}

func (*mcpFakeSlackAPI) GetConversationRepliesContext(context.Context, *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
	return nil, false, "", nil
}

func (*mcpFakeSlackAPI) GetUserInfoContext(context.Context, string) (*slack.User, error) {
	return nil, xerrors.New("not implemented")
}

func (*mcpFakeSlackAPI) UploadFileContext(context.Context, slack.UploadFileParameters) (*slack.FileSummary, error) {
	return &slack.FileSummary{}, nil
}

func (*mcpFakeSlackAPI) SetAssistantThreadsStatusContext(context.Context, slack.AssistantThreadsSetStatusParameters) error {
	return nil
}

func runMCPTool(t *testing.T, tools []fantasy.AgentTool, name string, args any) map[string]any {
	t.Helper()
	var tool fantasy.AgentTool
	for _, candidate := range tools {
		if candidate.Info().Name == name {
			tool = candidate
			break
		}
	}
	require.NotNil(t, tool, "tool %q not found", name)
	input, err := json.Marshal(args)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  name,
		Input: string(input),
	})
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	return result
}

// mcpTestSetup seeds a chat owner and chat, and returns tool options
// wired to the database plus the mutable enabled-ID set that backs the
// ChatMCPServerIDs/EnableMCPServer callbacks.
func mcpTestSetup(t *testing.T, db database.Store, api chattool.SlackAPI) (chattool.MCPServerToolsOptions, *[]uuid.UUID) {
	t.Helper()
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OwnerID:           owner.ID,
		OrganizationID:    org.ID,
		LastModelConfigID: modelConfig.ID,
	})

	accessURL := testutil.MustURL(t, "https://coder.example.com")

	enabled := &[]uuid.UUID{}
	opts := chattool.MCPServerToolsOptions{
		DB:            db,
		ChatID:        chat.ID,
		ChatOwnerID:   owner.ID,
		AccessURL:     accessURL,
		SlackAPI:      api,
		Channel:       "C123",
		ThreadTS:      "1700000000.000100",
		SlackSenderID: "USENDER",
		ResolveSlackUser: func(_ context.Context, slackUserID string) (uuid.UUID, error) {
			if slackUserID != "USENDER" {
				return uuid.Nil, xerrors.New("unknown slack user")
			}
			return owner.ID, nil
		},
		ChatMCPServerIDs: func(context.Context) ([]uuid.UUID, error) {
			return *enabled, nil
		},
		EnableMCPServer: func(_ context.Context, serverID uuid.UUID) error {
			for _, id := range *enabled {
				if id == serverID {
					return nil
				}
			}
			*enabled = append(*enabled, serverID)
			return nil
		},
	}
	return opts, enabled
}

func TestMCPServerToolSets(t *testing.T) {
	t.Parallel()

	opts := chattool.MCPServerToolsOptions{}

	var allNames []string
	for _, tool := range chattool.MCPServerTools(opts) {
		allNames = append(allNames, tool.Info().Name)
	}
	require.ElementsMatch(t, []string{
		"list_mcp_servers",
		"enable_mcp_server",
		"propose_mcp_server",
	}, allNames)

	var readOnlyNames []string
	for _, tool := range chattool.MCPServerReadOnlyTools(opts) {
		readOnlyNames = append(readOnlyNames, tool.Info().Name)
	}
	require.ElementsMatch(t, []string{"list_mcp_servers"}, readOnlyNames)
}

func TestListMCPServers(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	opts, enabled := mcpTestSetup(t, db, &mcpFakeSlackAPI{})

	// Visible: an enabled global config, a force_on global config, an
	// oauth2 config without a token, and the owner's personal config.
	// Another user's personal config must stay hidden.
	global := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		Slug: "global", Enabled: true, AuthType: "none",
	})
	dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		Slug: "forced", Enabled: true, AuthType: "api_key", Availability: "force_on",
	})
	oauth := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		Slug: "oauth", Enabled: true, AuthType: "oauth2",
	})
	personal := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		Slug: "personal", Enabled: true, AuthType: "none",
		OwnerID: uuid.NullUUID{UUID: opts.ChatOwnerID, Valid: true},
	})
	otherUser := dbgen.User(t, db, database.User{})
	dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		Slug: "other-personal", Enabled: true, AuthType: "none",
		OwnerID: uuid.NullUUID{UUID: otherUser.ID, Valid: true},
	})

	*enabled = append(*enabled, global.ID)

	tools := chattool.MCPServerReadOnlyTools(opts)
	result := runMCPTool(t, tools, "list_mcp_servers", map[string]any{})
	views := mcpServerViewsBySlug(t, result)
	require.Len(t, views, 4)
	require.NotContains(t, views, "other-personal")

	require.Equal(t, true, views["global"]["enabled_for_this_chat"])
	require.Equal(t, true, views["global"]["authenticated"])

	// force_on servers count as enabled for the chat without being in
	// chat.MCPServerIDs; api_key auth is always authenticated.
	require.Equal(t, true, views["forced"]["enabled_for_this_chat"])
	require.Equal(t, true, views["forced"]["authenticated"])

	require.Equal(t, false, views["oauth"]["enabled_for_this_chat"])
	require.Equal(t, false, views["oauth"]["authenticated"])

	require.Equal(t, false, views["personal"]["enabled_for_this_chat"])
	require.Equal(t, personal.ID.String(), views["personal"]["id"])

	// Secret and admin metadata never appears in the view.
	for _, view := range views {
		require.NotContains(t, view, "api_key_value")
		require.NotContains(t, view, "oauth2_client_secret")
		require.NotContains(t, view, "custom_headers")
	}

	// A stored token makes the oauth2 config authenticated.
	_, err := db.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
		MCPServerConfigID: oauth.ID,
		UserID:            opts.ChatOwnerID,
		AccessToken:       "token",
		RefreshToken:      "refresh",
		TokenType:         "Bearer",
		Expiry:            sql.NullTime{},
	})
	require.NoError(t, err)
	result = runMCPTool(t, tools, "list_mcp_servers", map[string]any{})
	views = mcpServerViewsBySlug(t, result)
	require.Equal(t, true, views["oauth"]["authenticated"])
}

func mcpServerViewsBySlug(t *testing.T, result map[string]any) map[string]map[string]any {
	t.Helper()
	rawViews, ok := result["mcp_servers"].([]any)
	require.True(t, ok, "unexpected result: %v", result)
	views := make(map[string]map[string]any, len(rawViews))
	for _, raw := range rawViews {
		view, ok := raw.(map[string]any)
		require.True(t, ok)
		slug, ok := view["slug"].(string)
		require.True(t, ok)
		views[slug] = view
	}
	return views
}

func TestEnableMCPServer(t *testing.T) {
	t.Parallel()

	t.Run("BySlugCaseInsensitive", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		opts, enabled := mcpTestSetup(t, db, &mcpFakeSlackAPI{})
		config := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			Slug: "linear", Enabled: true, AuthType: "none",
		})

		result := runMCPTool(t, chattool.MCPServerTools(opts), "enable_mcp_server", map[string]any{"server": "LiNeAr"})
		require.NotContains(t, result, "error")
		require.Equal(t, []uuid.UUID{config.ID}, *enabled)
		view, ok := result["mcp_server"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, true, view["enabled_for_this_chat"])
		require.NotContains(t, result, "connect_url")
	})

	t.Run("ByID", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		opts, enabled := mcpTestSetup(t, db, &mcpFakeSlackAPI{})
		config := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			Slug: "by-id", Enabled: true, AuthType: "none",
		})

		result := runMCPTool(t, chattool.MCPServerTools(opts), "enable_mcp_server", map[string]any{"server": config.ID.String()})
		require.NotContains(t, result, "error")
		require.Equal(t, []uuid.UUID{config.ID}, *enabled)
	})

	t.Run("UnknownServer", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		opts, enabled := mcpTestSetup(t, db, &mcpFakeSlackAPI{})

		result := runMCPTool(t, chattool.MCPServerTools(opts), "enable_mcp_server", map[string]any{"server": "nope"})
		require.Contains(t, result["error"], "list_mcp_servers")
		require.Empty(t, *enabled)
	})

	t.Run("DisabledServer", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		opts, enabled := mcpTestSetup(t, db, &mcpFakeSlackAPI{})
		// Disabled personal servers stay visible to their owner but
		// cannot be enabled per chat. dbgen defaults Enabled to true,
		// so insert directly.
		_, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
			DisplayName:   "Disabled",
			Slug:          "disabled",
			Transport:     "streamable_http",
			Url:           "https://mcp.example.com",
			AuthType:      "none",
			CustomHeaders: "{}",
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
			Availability:  "default_off",
			Enabled:       false,
			CreatedBy:     opts.ChatOwnerID,
			UpdatedBy:     opts.ChatOwnerID,
			OwnerID:       uuid.NullUUID{UUID: opts.ChatOwnerID, Valid: true},
		})
		require.NoError(t, err)

		result := runMCPTool(t, chattool.MCPServerTools(opts), "enable_mcp_server", map[string]any{"server": "disabled"})
		require.Contains(t, result["error"], "disabled")
		require.Empty(t, *enabled)
	})

	t.Run("OAuth2NotAuthenticatedIncludesConnectURL", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		opts, _ := mcpTestSetup(t, db, &mcpFakeSlackAPI{})
		config := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			Slug: "oauth", Enabled: true, AuthType: "oauth2",
		})

		result := runMCPTool(t, chattool.MCPServerTools(opts), "enable_mcp_server", map[string]any{"server": "oauth"})
		require.NotContains(t, result, "error")
		view, ok := result["mcp_server"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, false, view["authenticated"])
		require.Equal(t,
			"https://coder.example.com/api/experimental/mcp/servers/"+config.ID.String()+"/oauth2/connect",
			result["connect_url"])
	})

	t.Run("EnableCallbackError", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		opts, _ := mcpTestSetup(t, db, &mcpFakeSlackAPI{})
		opts.EnableMCPServer = func(context.Context, uuid.UUID) error {
			return xerrors.New("boom failure")
		}
		dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			Slug: "boom", Enabled: true, AuthType: "none",
		})

		result := runMCPTool(t, chattool.MCPServerTools(opts), "enable_mcp_server", map[string]any{"server": "boom"})
		require.Contains(t, result["error"], "boom failure")
	})
}

func TestProposeMCPServer(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		// A real slack.Client against a stub HTTP server records the
		// full posted payload, including attachments, which the fake
		// cannot render.
		var (
			formMu sync.Mutex
			form   url.Values
		)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			formMu.Lock()
			form = r.PostForm
			formMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"channel":"C123","ts":"1700000002.000300"}`))
		}))
		t.Cleanup(srv.Close)
		api := slack.New("token", slack.OptionAPIURL(srv.URL+"/"))
		opts, _ := mcpTestSetup(t, db, api)

		result := runMCPTool(t, chattool.MCPServerTools(opts), "propose_mcp_server", map[string]any{
			"display_name": "Linear",
			"slug":         "linear",
			"url":          "https://mcp.linear.app/mcp",
			"auth_type":    "oauth2",
		})
		require.Equal(t, true, result["ok"])
		require.Equal(t, "1700000002.000300", result["dialog_ts"])
		require.Contains(t, result["note"], "NOT created yet")

		rawID, ok := result["proposal_id"].(string)
		require.True(t, ok)
		proposalID, err := uuid.Parse(rawID)
		require.NoError(t, err)

		// The proposal row was persisted with the resolved thread and
		// the requesting Coder user (the chat owner), and the request
		// round-trips.
		proposal, err := db.GetMCPServerProposalByID(ctx, proposalID)
		require.NoError(t, err)
		require.Equal(t, opts.ChatID, proposal.ChatID)
		require.Equal(t, opts.ChatOwnerID, proposal.RequesterID)
		require.Equal(t, "C123", proposal.Channel)
		require.Equal(t, "1700000000.000100", proposal.ThreadTs)
		require.Equal(t, "1700000002.000300", proposal.MessageTs)
		require.Equal(t, "pending", proposal.Status)
		var req chattool.MCPServerProposalRequest
		require.NoError(t, json.Unmarshal(proposal.Request, &req))
		require.Equal(t, "Linear", req.DisplayName)
		require.Equal(t, "oauth2", req.AuthType)
		require.Equal(t, "streamable_http", req.Transport)

		// The card was posted to the bound thread with the
		// personal-availability line, the Review URL, and the Cancel
		// button carrying the proposal id.
		formMu.Lock()
		defer formMu.Unlock()
		require.Equal(t, "C123", form.Get("channel"))
		require.Equal(t, "1700000000.000100", form.Get("thread_ts"))
		attachments := form.Get("attachments")
		require.Contains(t, attachments, "This MCP server will only be available to you.")
		require.Contains(t, attachments, "https://coder.example.com/mcp-proposals/"+proposalID.String())
		require.Contains(t, attachments, chattool.MCPProposalCancelActionID)
		require.Contains(t, attachments, chattool.MCPProposalPendingColor)
		require.Contains(t, attachments, "Connect Linear?")
	})

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name    string
			args    map[string]any
			wantErr string
		}{
			{"MissingDisplayName", map[string]any{"slug": "s", "url": "https://x"}, "display_name is required"},
			{"MissingSlug", map[string]any{"display_name": "X", "url": "https://x"}, "slug is required"},
			{"MissingURL", map[string]any{"display_name": "X", "slug": "s"}, "url is required"},
			{"BadTransport", map[string]any{"display_name": "X", "slug": "s", "url": "https://x", "transport": "grpc"}, "invalid transport"},
			{"BadAuthType", map[string]any{"display_name": "X", "slug": "s", "url": "https://x", "auth_type": "user_oidc"}, "invalid auth_type"},
			{"APIKeyMissingValue", map[string]any{"display_name": "X", "slug": "s", "url": "https://x", "auth_type": "api_key"}, "api_key_header and api_key_value"},
			{"CustomHeadersEmpty", map[string]any{"display_name": "X", "slug": "s", "url": "https://x", "auth_type": "custom_headers"}, "at least one custom header"},
			{"PartialOAuth2", map[string]any{"display_name": "X", "slug": "s", "url": "https://x", "auth_type": "oauth2", "oauth2_client_id": "id"}, "all of oauth2_client_id"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				api := &mcpFakeSlackAPI{}
				tools := chattool.MCPServerTools(chattool.MCPServerToolsOptions{SlackAPI: api})
				result := runMCPTool(t, tools, "propose_mcp_server", tc.args)
				require.Contains(t, result["error"], tc.wantErr)
				// Nothing was posted for invalid input.
				require.Empty(t, api.postChannel)
			})
		}
	})

	t.Run("SharedModeAsksUserToConnect", func(t *testing.T) {
		t.Parallel()
		api := &mcpFakeSlackAPI{}
		opts := chattool.MCPServerToolsOptions{
			SlackAPI:   api,
			SharedMode: true,
			AccessURL:  testutil.MustURL(t, "https://coder.example.com"),
		}

		result := runMCPTool(t, chattool.MCPServerTools(opts), "propose_mcp_server", map[string]any{
			"display_name": "X", "slug": "x", "url": "https://x",
		})
		require.Contains(t, result["error"], "shared mode")
		require.Contains(t, result["error"], "connect their Coder account to Slack")
		require.Contains(t, result["error"], "https://coder.example.com/settings/external-auth")
		require.Empty(t, api.postChannel)
	})

	t.Run("ResolverErrorDoesNotPost", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		api := &mcpFakeSlackAPI{}
		opts, _ := mcpTestSetup(t, db, api)
		opts.ResolveSlackUser = func(context.Context, string) (uuid.UUID, error) {
			return uuid.Nil, xerrors.New("ambiguous account link")
		}

		result := runMCPTool(t, chattool.MCPServerTools(opts), "propose_mcp_server", map[string]any{
			"display_name": "X", "slug": "x", "url": "https://x",
		})
		require.Contains(t, result["error"], "ambiguous account link")
		require.Empty(t, api.postChannel)
	})

	t.Run("ResolvedUserMustOwnChat", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		api := &mcpFakeSlackAPI{}
		opts, _ := mcpTestSetup(t, db, api)
		opts.ResolveSlackUser = func(context.Context, string) (uuid.UUID, error) {
			return uuid.New(), nil
		}

		result := runMCPTool(t, chattool.MCPServerTools(opts), "propose_mcp_server", map[string]any{
			"display_name": "X", "slug": "x", "url": "https://x",
		})
		require.Contains(t, result["error"], "does not own this chat")
		require.Empty(t, api.postChannel)
	})

	t.Run("SlackPostError", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		api := &mcpFakeSlackAPI{postErr: xerrors.New("channel_not_found")}
		opts, _ := mcpTestSetup(t, db, api)

		result := runMCPTool(t, chattool.MCPServerTools(opts), "propose_mcp_server", map[string]any{
			"display_name": "X", "slug": "x", "url": "https://x",
		})
		require.Contains(t, result["error"], "channel_not_found")
	})
}

func TestMCPAuthConnected(t *testing.T) {
	t.Parallel()

	now := time.Now()
	oauth := database.MCPServerConfig{AuthType: "oauth2"}
	require.True(t, chattool.MCPAuthConnected(database.MCPServerConfig{AuthType: "none"}, nil, now))
	require.True(t, chattool.MCPAuthConnected(database.MCPServerConfig{AuthType: "api_key"}, nil, now))
	require.False(t, chattool.MCPAuthConnected(oauth, nil, now))
	require.True(t, chattool.MCPAuthConnected(oauth, &database.MCPServerUserToken{RefreshToken: "r"}, now))
	require.True(t, chattool.MCPAuthConnected(oauth, &database.MCPServerUserToken{AccessToken: "a"}, now))
	require.True(t, chattool.MCPAuthConnected(oauth, &database.MCPServerUserToken{
		AccessToken: "a",
		Expiry:      sql.NullTime{Time: now.Add(time.Hour), Valid: true},
	}, now))
	require.False(t, chattool.MCPAuthConnected(oauth, &database.MCPServerUserToken{
		AccessToken: "a",
		Expiry:      sql.NullTime{Time: now.Add(-time.Hour), Valid: true},
	}, now))
}
