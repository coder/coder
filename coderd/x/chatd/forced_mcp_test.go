package chatd_test

// Regression tests for Cure53 CDM-02-010: the Force On MCP server
// availability policy must be enforced on the backend. A client that
// omits force_on entries from mcp_server_ids when creating a chat or
// sending a message must not be able to exclude those servers from
// the conversation.

import (
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func testAccessControlStorePointer() *atomic.Pointer[dbauthz.AccessControlStore] {
	acs := &atomic.Pointer[dbauthz.AccessControlStore]{}
	var store dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
	acs.Store(&store)
	return acs
}

// newEchoMCPTestServer starts an MCP test server exposing an "echo"
// tool and returns its base URL.
func newEchoMCPTestServer(t *testing.T, name string) string {
	t.Helper()
	srv := newTestMCPServer(name)
	addTestMCPTextTool(srv, "echo", "Echoes the input", "echo: ")
	ts := httptest.NewServer(testMCPHTTPHandler(srv))
	t.Cleanup(ts.Close)
	return ts.URL
}

// newToolRecordingOpenAI returns a mock OpenAI URL that answers every
// streamed call with plain text and records the tool names offered on
// each streamed call, plus an accessor for the recorded calls.
func newToolRecordingOpenAI(t *testing.T) (string, func() [][]string) {
	t.Helper()
	var (
		mu    sync.Mutex
		calls [][]string
	)
	url := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		names := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			names = append(names, tool.Function.Name)
		}
		mu.Lock()
		calls = append(calls, names)
		mu.Unlock()
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("ok")...)
	})
	recorded := func() [][]string {
		mu.Lock()
		defer mu.Unlock()
		out := make([][]string, len(calls))
		copy(out, calls)
		return out
	}
	return url, recorded
}

// TestCreateChat_ForceOnMCPServerEnforced reproduces CDM-02-010 for
// chat creation: stripping mcp_server_ids from the create request must
// not exclude force_on MCP servers.
func TestCreateChat_ForceOnMCPServerEnforced(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	forcedURL := newEchoMCPTestServer(t, "forced-mcp")
	openAIURL, recordedCalls := newToolRecordingOpenAI(t)

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	forcedConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: org.ID,
		DisplayName:    "Forced MCP",
		Slug:           "forced-mcp",
		Url:            forcedURL,
		Availability:   "force_on",
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
	})
	// A force_on server whose ACL denies the owner must not attach:
	// Force On cannot widen access beyond the server's ACL. The grant
	// goes to an unrelated group instead of the Everyone group.
	dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: org.ID,
		DisplayName:    "ACL Denied Forced MCP",
		Slug:           "acl-denied-forced-mcp",
		Url:            newEchoMCPTestServer(t, "acl-denied-forced-mcp"),
		Availability:   "force_on",
		GroupACL: database.ChatACL{
			uuid.NewString(): {Permissions: []policy.Action{policy.ActionRead}},
		},
		UserACL:   database.ChatACL{},
		CreatedBy: uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy: uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	// The ACL check only exists at the dbauthz layer, so this server
	// runs on a dbauthz-wrapped store like production instead of the
	// raw store other chatd tests use.
	authzDB := dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		testAccessControlStorePointer(),
	)
	server := newActiveTestServer(t, authzDB, ps, func(cfg *chatd.Config) {
		withoutMCPToolSearch(cfg)
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	// Chat creation requires the agents-access org role, which API
	// users receive through org settings; grant it to the seeded
	// member directly.
	_, err := db.UpdateMemberRoles(dbauthz.AsSystemRestricted(ctx), database.UpdateMemberRolesParams{
		GrantedRoles: []string{"agents-access"},
		UserID:       user.ID,
		OrgID:        org.ID,
	})
	require.NoError(t, err)
	ownerSubject, _, err := httpmw.UserRBACSubject(dbauthz.AsSystemRestricted(ctx), db, user.ID, rbac.ScopeAll)
	require.NoError(t, err)
	ownerCtx := dbauthz.As(ctx, ownerSubject)

	// The attacker strips every MCP server ID from the request.
	chat, err := server.CreateChat(ownerCtx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "forced-mcp-create",
		ModelConfigID:  model.ID,
		MCPServerIDs:   []uuid.UUID{},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("hello"),
		},
	})
	require.NoError(t, err)

	// The force_on server must be persisted despite the empty list.
	dbChat, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{forcedConfig.ID}, dbChat.MCPServerIDs,
		"force_on MCP server must be enforced on chat creation")

	waitForChatProcessed(ctx, t, db, chat.ID, server)

	chatResult, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	// The forced server's tool must be offered to the LLM.
	calls := recordedCalls()
	require.NotEmpty(t, calls)
	require.Contains(t, calls[0], "forced-mcp__echo",
		"force_on MCP tools must be offered to the LLM despite a stripped mcp_server_ids list")
	require.NotContains(t, calls[0], "acl-denied-forced-mcp__echo",
		"force_on MCP tools must not be offered when the server's ACL denies the chat owner")
}

// TestSendMessage_ForceOnMCPServerEnforced reproduces CDM-02-010 for
// message sends: a tampered mcp_server_ids update must not remove
// force_on MCP servers from the chat.
func TestSendMessage_ForceOnMCPServerEnforced(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	forcedURL := newEchoMCPTestServer(t, "forced-mcp")
	optionalURL := newEchoMCPTestServer(t, "optional-mcp")
	openAIURL, _ := newToolRecordingOpenAI(t)

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	forcedConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: org.ID,
		DisplayName:    "Forced MCP",
		Slug:           "forced-mcp",
		Url:            forcedURL,
		Availability:   "force_on",
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
	})
	optionalConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: org.ID,
		DisplayName:    "Optional MCP",
		Slug:           "optional-mcp",
		Url:            optionalURL,
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		withoutMCPToolSearch(cfg)
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	// Creation with a tampered list that omits the forced server.
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "forced-mcp-send",
		ModelConfigID:  model.ID,
		MCPServerIDs:   []uuid.UUID{optionalConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("hello"),
		},
	})
	require.NoError(t, err)

	dbChat, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{optionalConfig.ID, forcedConfig.ID}, dbChat.MCPServerIDs,
		"force_on MCP server must be appended to a tampered create list")

	waitForChatProcessed(ctx, t, db, chat.ID, server)

	// The attacker clears the MCP server list on a message send.
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		CreatedBy:    user.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("clear the list")},
		MCPServerIDs: &[]uuid.UUID{},
	})
	require.NoError(t, err)

	dbChat, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{forcedConfig.ID}, dbChat.MCPServerIDs,
		"force_on MCP server must survive an emptied mcp_server_ids update")

	waitForChatProcessed(ctx, t, db, chat.ID, server)

	// A legitimate update keeps both the selection and the forced server.
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		CreatedBy:    user.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("select optional")},
		MCPServerIDs: &[]uuid.UUID{optionalConfig.ID},
	})
	require.NoError(t, err)

	dbChat, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{optionalConfig.ID, forcedConfig.ID}, dbChat.MCPServerIDs,
		"force_on MCP server must be appended to a tampered update list")

	waitForChatProcessed(ctx, t, db, chat.ID, server)
}

// TestGeneration_ForceOnMCPServerEnforcedForExistingChats covers chats
// whose stored mcp_server_ids predates the force_on policy (or was
// tampered before enforcement existed): generation must still include
// force_on servers without relying on the stored list.
func TestGeneration_ForceOnMCPServerEnforcedForExistingChats(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	forcedURL := newEchoMCPTestServer(t, "forced-mcp")
	openAIURL, recordedCalls := newToolRecordingOpenAI(t)

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		withoutMCPToolSearch(cfg)
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	// The chat is created before any force_on MCP server exists, so
	// its stored mcp_server_ids is empty.
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "forced-mcp-existing",
		ModelConfigID:  model.ID,
		MCPServerIDs:   []uuid.UUID{},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("hello"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)

	// An admin marks a server force_on after the chat already exists.
	dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: org.ID,
		DisplayName:    "Forced MCP",
		Slug:           "forced-mcp",
		Url:            forcedURL,
		Availability:   "force_on",
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	// A force_on server in another organization must not attach: the
	// forced set is scoped to the chat's organization.
	dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: dbgen.Organization(t, db, database.Organization{}).ID,
		DisplayName:    "Foreign Forced MCP",
		Slug:           "foreign-forced-mcp",
		Url:            forcedURL,
		Availability:   "force_on",
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	// A send that does not touch mcp_server_ids must still pick up
	// the force_on server at generation time.
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("again")},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)

	chatResult, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	// nil MCPServerIDs must keep the stored list untouched.
	require.Empty(t, chatResult.MCPServerIDs)

	calls := recordedCalls()
	require.GreaterOrEqual(t, len(calls), 2)
	require.NotContains(t, calls[0], "forced-mcp__echo",
		"no force_on server existed during the first turn")
	require.Contains(t, calls[len(calls)-1], "forced-mcp__echo",
		"force_on MCP tools must reach generation for chats created before the policy")
	require.NotContains(t, calls[len(calls)-1], "foreign-forced-mcp__echo",
		"another organization's force_on server must not attach")
}
