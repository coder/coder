package coderd_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/coderd/x/mcpbuiltin"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/safedial"
)

// TestBuiltinMCPSlackConversation sends signed requests the way chatd
// does and checks that Slack replies default to the thread the chat's
// trigger conversation came from.
func TestBuiltinMCPSlackConversation(t *testing.T) {
	t.Parallel()

	const signingSecret = "0123456789abcdef0123456789abcdef"

	type post struct{ channel, threadTS, auth string }
	var (
		mu    sync.Mutex
		posts []post
	)
	slack := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, r.ParseForm()) {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/chat.postMessage") {
			mu.Lock()
			posts = append(posts, post{
				channel:  r.PostForm.Get("channel"),
				threadTS: r.PostForm.Get("thread_ts"),
				auth:     r.Header.Get("Authorization"),
			})
			mu.Unlock()
		}
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"ok":true,"channel":"C1","ts":"1.2"}`))
	}))
	t.Cleanup(slack.Close)

	client, db, _ := newChatClientWithoutAIBridge(t, withChatWorkerDisabled, func(o *coderdtest.Options) {
		// Trust the fake Slack's TLS certificate for upstream calls.
		o.HTTPClient = slack.Client()
	})
	user := coderdtest.CreateFirstUser(t, client.Client)
	model := createChatModel(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)
	owner, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)
	ownerCtx := dbauthz.As(ctx, coderdtest.AuthzUserSubjectWithDB(ctx, t, db, owner))

	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: user.OrganizationID, OwnerID: user.UserID})
	trigger, err := db.InsertChatProjectTrigger(ownerCtx, database.InsertChatProjectTriggerParams{
		Name:         "slack",
		HashedSecret: []byte("hashed"),
		CreatedBy:    user.UserID,
		Source:       string(codersdk.ChatProjectTriggerSourceSlack),
		Events:       []string{codersdk.ChatProjectTriggerSlackEventAppMention},
		Senders:      []string{},
		BusyBehavior: "queue",
		ProjectID:    project.ID,
	})
	require.NoError(t, err)
	newChat := func(parent *database.Chat) database.Chat {
		seed := database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: model.ID,
		}
		if parent != nil {
			seed.ParentChatID = uuid.NullUUID{UUID: parent.ID, Valid: true}
			seed.RootChatID = uuid.NullUUID{UUID: parent.ID, Valid: true}
		}
		return dbgen.Chat(t, db, seed)
	}
	setConversation := func(chat database.Chat, key string) {
		_, err := db.UpsertChatProjectTriggerConversation(ownerCtx, database.UpsertChatProjectTriggerConversationParams{
			TriggerID:       trigger.ID,
			ConversationKey: key,
			ChatID:          chat.ID,
		})
		require.NoError(t, err)
	}

	slackURL := client.URL.JoinPath(mcpbuiltin.EndpointPath(mcpbuiltin.ServerSlack))
	slackURL.RawQuery = url.Values{mcpbuiltin.SlackOptionAPIBaseURL: {slack.URL + "/api"}}.Encode()
	_ = dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID:      user.OrganizationID,
		Slug:                "slack",
		Url:                 slackURL.String(),
		ForwardCoderHeaders: true,
		SigningSecret:       signingSecret,
	})

	// A config with the same path on another host must not be able to
	// sign requests for this deployment.
	const otherSecret = "fedcba9876543210fedcba9876543210"
	_ = dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID:      user.OrganizationID,
		Slug:                "other-slack",
		Url:                 "https://other.example.com" + mcpbuiltin.EndpointPath(mcpbuiltin.ServerSlack),
		ForwardCoderHeaders: true,
		SigningSecret:       otherSecret,
	})

	// connectAs connects chatd's MCP client to the Slack server, signed
	// with secret and the identity chatd forwards for chat.
	connectAs := func(t *testing.T, chat database.Chat, token, secret string) []fantasy.AgentTool {
		t.Helper()
		cfg := mcpclient.Server{
			ID:                  uuid.New(),
			Slug:                "slack",
			URL:                 slackURL.String(),
			Transport:           mcpclient.TransportStreamableHTTP,
			Headers:             map[string]string{"Authorization": "Bearer " + token},
			ForwardCoderHeaders: true,
			SigningSecret:       secret,
		}
		tools, _, cleanup := mcpclient.ConnectAll(
			ctx, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			[]mcpclient.Server{cfg}, nil, uuid.Nil, nil, chatprovider.CoderHeaders(chat),
			mcpclient.NewHTTPClient(nil, safedial.WithAllowedPrefixes(netip.MustParsePrefix("127.0.0.0/8"))),
		)
		t.Cleanup(cleanup)
		return tools
	}

	// sendAs posts a Slack message through chatd's MCP client. Each
	// caller uses its own token so parallel subtests can find their post.
	sendAs := func(t *testing.T, chat database.Chat, token string) post {
		t.Helper()
		tools := connectAs(t, chat, token, signingSecret)
		var send fantasy.AgentTool
		for _, tool := range tools {
			if tool.Info().Name == "slack__slack_send_message" {
				send = tool
			}
		}
		require.NotNil(t, send)
		resp, err := send.Run(ctx, fantasy.ToolCall{ID: "1", Name: "slack__slack_send_message", Input: `{"message":"hi"}`})
		require.NoError(t, err)
		require.False(t, resp.IsError, resp.Content)

		mu.Lock()
		defer mu.Unlock()
		for _, p := range posts {
			if p.auth == "Bearer "+token {
				return p
			}
		}
		require.FailNow(t, "no Slack post for token", token)
		return post{}
	}

	root := newChat(nil)
	setConversation(root, "T1/C100/1700000000.000100")

	t.Run("UnsignedRejected", func(t *testing.T) {
		t.Parallel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, slackURL.String(), strings.NewReader(`{}`))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer xoxb-anonymous")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("OtherHostSecretRejected", func(t *testing.T) {
		t.Parallel()
		tools := connectAs(t, root, "xoxb-other", otherSecret)
		assert.Empty(t, tools)
		mu.Lock()
		defer mu.Unlock()
		for _, p := range posts {
			assert.NotEqual(t, "Bearer xoxb-other", p.auth)
		}
	})

	t.Run("RootChat", func(t *testing.T) {
		t.Parallel()
		got := sendAs(t, root, "xoxb-root")
		assert.Equal(t, post{channel: "C100", threadTS: "1700000000.000100", auth: "Bearer xoxb-root"}, got)
	})

	t.Run("SubchatUsesRootConversation", func(t *testing.T) {
		t.Parallel()
		got := sendAs(t, newChat(&root), "xoxb-sub")
		assert.Equal(t, "C100", got.channel)
		assert.Equal(t, "1700000000.000100", got.threadTS)
	})

	t.Run("SubchatOwnConversationWins", func(t *testing.T) {
		t.Parallel()
		sub := newChat(&root)
		setConversation(sub, "T1/C200/1700000000.000200")
		got := sendAs(t, sub, "xoxb-own")
		assert.Equal(t, "C200", got.channel)
		assert.Equal(t, "1700000000.000200", got.threadTS)
	})
}
