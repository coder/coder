package chatd_test

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestGeneration_MCPNegativeCacheSkipsTimedOutServer proves the
// negative-cache wiring end to end: the first turn pays the full
// connect budget against a black-holed MCP server and records a
// timeout, and the next turn skips the server entirely, visible as
// a "skipped" outcome in the debug run's mcp_connect summary.
func TestGeneration_MCPNegativeCacheSkipsTimedOutServer(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Accepts TCP connections and never responds, so the MCP
	// connect burns its whole budget and classifies as a timeout.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			defer conn.Close()
		}
	}()

	openAIURL, _ := newToolRecordingOpenAI(t)
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		withoutMCPToolSearch(cfg)
		cfg.AlwaysEnableDebugLogs = true
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: org.ID,
		DisplayName:    "Black Hole",
		Slug:           "blackhole",
		Url:            "http://" + ln.Addr().String(),
		Availability:   "force_on",
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "negative-cache",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("hello"),
		},
	})
	require.NoError(t, err)
	// The first turn pays the ~10s connect budget, so wait longer
	// than waitForChatProcessed's WaitShort allows.
	require.Eventually(t, func() bool {
		c, getErr := db.GetChatByID(ctx, chat.ID)
		return getErr == nil && c.Status != database.ChatStatusRunning
	}, testutil.WaitSuperLong, testutil.IntervalMedium)
	chatd.WaitUntilIdleForTest(server)

	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("again")},
	})
	require.NoError(t, err)
	// The second turn skips the black-holed server, so it settles
	// fast.
	require.Eventually(t, func() bool {
		c, getErr := db.GetChatByID(ctx, chat.ID)
		return getErr == nil && c.Status != database.ChatStatusRunning
	}, testutil.WaitLong, testutil.IntervalMedium)
	chatd.WaitUntilIdleForTest(server)

	// Collect per-turn mcp_connect outcomes for the blackhole
	// server from the chat_turn debug runs, oldest turn first.
	runs, err := db.GetChatDebugRunsByChatID(ctx, database.GetChatDebugRunsByChatIDParams{
		ChatID:   chat.ID,
		LimitVal: 50,
	})
	require.NoError(t, err)
	type connectEntry struct {
		Slug    string `json:"slug"`
		Outcome string `json:"outcome"`
	}
	var outcomes []string
	for i := len(runs) - 1; i >= 0; i-- {
		run := runs[i]
		if run.Kind != string(codersdk.ChatDebugRunKindChatTurn) {
			continue
		}
		var summary struct {
			MCPConnect []connectEntry `json:"mcp_connect"`
		}
		require.NoError(t, json.Unmarshal(run.Summary, &summary))
		for _, entry := range summary.MCPConnect {
			if entry.Slug == "blackhole" {
				outcomes = append(outcomes, entry.Outcome)
			}
		}
	}
	require.GreaterOrEqual(t, len(outcomes), 2,
		"expected mcp_connect summaries for both turns, got %v", outcomes)
	require.Equal(t, "timeout", outcomes[0],
		"first turn must record the connect timeout")
	require.Equal(t, "skipped", outcomes[1],
		"second turn must skip the server via the negative cache")
}
