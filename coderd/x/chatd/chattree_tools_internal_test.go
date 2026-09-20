package chatd //nolint:testpackage // Exercises unexported chat tree tool internals.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

// chatTreeFixture holds one owner's chat tree for tool tests.
type chatTreeFixture struct {
	ctx         context.Context
	db          database.Store
	ps          pubsub.Pubsub
	server      *Server
	user        database.User
	org         database.Organization
	modelConfig database.ChatModelConfig
	root        database.Chat
}

func newChatTreeFixture(t *testing.T, opts ...internalTestServerOpt) *chatTreeFixture {
	t.Helper()
	return newChatTreeFixtureWithServerStore(t, func(db database.Store) database.Store { return db }, opts...)
}

// newAuthzChatTreeFixture seeds through the raw store but gives the server a
// dbauthz-wrapped store, so tool calls run against authorization exactly as
// on a deployment.
func newAuthzChatTreeFixture(t *testing.T, opts ...internalTestServerOpt) *chatTreeFixture {
	t.Helper()
	return newChatTreeFixtureWithServerStore(t, func(db database.Store) database.Store {
		var acs dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
		acsPtr := &atomic.Pointer[dbauthz.AccessControlStore]{}
		acsPtr.Store(&acs)
		return dbauthz.New(
			db,
			rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
			slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			acsPtr,
		)
	}, opts...)
}

func newChatTreeFixtureWithServerStore(t *testing.T, serverStore func(database.Store) database.Store, opts ...internalTestServerOpt) *chatTreeFixture {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:          "gpt-4o-mini",
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		OrganizationID: org.ID,
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})
	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Kind:              database.ChatKindRoot,
		Title:             "root " + uuid.NewString(),
		LastModelConfigID: modelConfig.ID,
	})
	server := newInternalTestServer(t, serverStore(db), ps, chatprovider.ProviderAPIKeys{}, opts...)
	return &chatTreeFixture{
		ctx:         ctx,
		db:          db,
		ps:          ps,
		server:      server,
		user:        user,
		org:         org,
		modelConfig: modelConfig,
		root:        root,
	}
}

// namedChild seeds a kind=chat child under parent in the given status.
func (f *chatTreeFixture) namedChild(t *testing.T, parent database.Chat, status database.ChatStatus) database.Chat {
	t.Helper()
	return dbgen.Chat(t, f.db, database.Chat{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		Kind:              database.ChatKindChat,
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		Title:             "child " + uuid.NewString(),
		LastModelConfigID: f.modelConfig.ID,
		Status:            status,
	})
}

// subagentChild seeds a kind=subagent child under parent.
func (f *chatTreeFixture) subagentChild(t *testing.T, parent database.Chat) database.Chat {
	t.Helper()
	return dbgen.Chat(t, f.db, database.Chat{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		Kind:              database.ChatKindSubagent,
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
		Title:             "subagent " + uuid.NewString(),
		LastModelConfigID: f.modelConfig.ID,
		Status:            database.ChatStatusRunning,
	})
}

func (f *chatTreeFixture) reload(t *testing.T, chatID uuid.UUID) database.Chat {
	t.Helper()
	chat, err := f.db.GetChatByID(f.ctx, chatID)
	require.NoError(t, err)
	return chat
}

// sendCallID is the tool call id used for a sender's current call.
const sendCallID = "call-send-1"

func humanRow(t *testing.T, text string) database.ChatMessage {
	t.Helper()
	return database.ChatMessage{
		Role:           database.ChatMessageRoleUser,
		Content:        mustMarshalText(t, text),
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func relayedRow(t *testing.T, hop int) database.ChatMessage {
	t.Helper()
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageSenderChat(uuid.New(), "sender", codersdk.ChatSenderChatRelationParent, hop),
		codersdk.ChatMessageText("relayed"),
	})
	require.NoError(t, err)
	return database.ChatMessage{
		Role:           database.ChatMessageRoleUser,
		Content:        content,
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func assistantSendRow(t *testing.T, callIDs ...string) database.ChatMessage {
	t.Helper()
	parts := make([]codersdk.ChatMessagePart, 0, len(callIDs))
	for _, id := range callIDs {
		parts = append(parts, codersdk.ChatMessageToolCall(id, sendChatMessageToolName, json.RawMessage(`{}`)))
	}
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	return database.ChatMessage{
		Role:           database.ChatMessageRoleAssistant,
		Content:        content,
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func toolResultRow(t *testing.T, callIDs ...string) database.ChatMessage {
	t.Helper()
	parts := make([]codersdk.ChatMessagePart, 0, len(callIDs))
	for _, id := range callIDs {
		parts = append(parts, codersdk.ChatMessageToolResult(id, sendChatMessageToolName, json.RawMessage(`{}`), false, false))
	}
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	return database.ChatMessage{
		Role:           database.ChatMessageRoleTool,
		Content:        content,
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

// humanTurnHistory is the simplest sender history: one human prompt and
// the assistant row carrying the current send call.
func humanTurnHistory(t *testing.T) []database.ChatMessage {
	t.Helper()
	return []database.ChatMessage{humanRow(t, "hello"), assistantSendRow(t, sendCallID)}
}

func (f *chatTreeFixture) runTool(
	t *testing.T,
	sender database.Chat,
	history []database.ChatMessage,
	name string,
	args any,
) fantasy.ToolResponse {
	t.Helper()
	tools := f.server.chatTreeTools(
		func() database.Chat { return sender },
		func() []database.ChatMessage { return history },
	)
	tool := findToolByName(tools, name)
	require.NotNil(t, tool, "%s tool must be present", name)
	input, err := json.Marshal(args)
	require.NoError(t, err)
	resp, err := tool.Run(f.ctx, fantasy.ToolCall{ID: sendCallID, Name: name, Input: string(input)})
	require.NoError(t, err)
	return resp
}

func (f *chatTreeFixture) send(t *testing.T, sender database.Chat, history []database.ChatMessage, args sendChatMessageArgs) fantasy.ToolResponse {
	t.Helper()
	return f.runTool(t, sender, history, sendChatMessageToolName, args)
}

func requireToolError(t *testing.T, resp fantasy.ToolResponse, want error) map[string]any {
	t.Helper()
	result := requireToolResponseMap(t, resp, true)
	require.Equal(t, want.Error(), result["error"])
	return result
}

// deliveredParts returns the parts of the single user row in chatID.
func (f *chatTreeFixture) deliveredParts(t *testing.T, chatID uuid.UUID) []codersdk.ChatMessagePart {
	t.Helper()
	messages, err := f.db.GetChatMessagesByChatID(f.ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
	require.NoError(t, err)
	var userRows []database.ChatMessage
	for _, msg := range messages {
		if msg.Role == database.ChatMessageRoleUser {
			userRows = append(userRows, msg)
		}
	}
	require.Len(t, userRows, 1)
	parts, err := chatprompt.ParseContent(userRows[0])
	require.NoError(t, err)
	return parts
}

func TestChatTreeToolsPresence(t *testing.T) {
	t.Parallel()

	t.Run("ExperimentOnAppendsForRootAndNamedChat", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		child := f.namedChild(t, f.root, database.ChatStatusRunning)
		for _, chat := range []database.Chat{f.root, child} {
			tools := f.server.appendRootChatTools(f.ctx, nil, rootChatToolsOptions{chat: chat, modelConfigID: f.modelConfig.ID})
			require.NotNil(t, findToolByName(tools, sendChatMessageToolName), "kind %s", chat.Kind)
			require.NotNil(t, findToolByName(tools, listChatTreeToolName), "kind %s", chat.Kind)
		}
	})

	t.Run("ExperimentOffOmitsTools", func(t *testing.T) {
		t.Parallel()
		experiments := slices.DeleteFunc(slices.Clone(codersdk.ExperimentsKnown), func(e codersdk.Experiment) bool {
			return e == codersdk.ExperimentChatTree
		})
		f := newChatTreeFixture(t, withInternalTestServerExperiments(experiments))
		tools := f.server.appendRootChatTools(f.ctx, nil, rootChatToolsOptions{chat: f.root, modelConfigID: f.modelConfig.ID})
		require.Nil(t, findToolByName(tools, sendChatMessageToolName))
		require.Nil(t, findToolByName(tools, listChatTreeToolName))
	})

	t.Run("SubagentPreparationOmitsTools", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t, withInternalTestServerTransportFactory(&aibridgeTestFactory{}))
		parent := f.namedChild(t, f.root, database.ChatStatusRunning)
		created, err := chatstate.CreateChat(f.ctx, f.db, f.ps, chatstate.CreateChatInput{
			OrganizationID:    f.org.ID,
			OwnerID:           f.user.ID,
			ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
			LastModelConfigID: f.modelConfig.ID,
			Title:             "subagent",
			ClientType:        database.ChatClientTypeApi,
			InitialMessages: []chatstate.Message{{
				Role:           database.ChatMessageRoleUser,
				Content:        mustMarshalText(t, "inspect"),
				Visibility:     database.ChatMessageVisibilityBoth,
				ModelConfigID:  uuid.NullUUID{UUID: f.modelConfig.ID, Valid: true},
				CreatedBy:      uuid.NullUUID{UUID: f.user.ID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
			}},
		})
		require.NoError(t, err)
		require.Equal(t, database.ChatKindSubagent, created.Chat.Kind)
		prepared, err := f.server.prepareGeneration(f.ctx, generationPrepareInput{Chat: created.Chat, Messages: created.InitialMessages})
		require.NoError(t, err)
		t.Cleanup(prepared.Cleanup)
		require.NotContains(t, prepared.ActiveTools, sendChatMessageToolName)
		require.NotContains(t, prepared.ActiveTools, listChatTreeToolName)
	})

	t.Run("PlanModeKeepsToolsForEveryKind", func(t *testing.T) {
		t.Parallel()
		for _, isRoot := range []bool{true, false} {
			require.True(t, builtinPlanToolAllowed(sendChatMessageToolName, isRoot))
			require.True(t, builtinPlanToolAllowed(listChatTreeToolName, isRoot))
		}
		require.NotContains(t, allowedExploreToolNames([]fantasy.AgentTool{
			newTestAgentTool(sendChatMessageToolName),
			newTestAgentTool(listChatTreeToolName),
		}), sendChatMessageToolName)
	})
}

func TestSendChatMessage_ChildToParent(t *testing.T) {
	t.Parallel()

	t.Run("ParentLiteralStartsWaitingRoot", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		child := f.namedChild(t, f.root, database.ChatStatusRunning)

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "  status: done  "})
		result := requireToolResponseMap(t, resp, false)
		require.Equal(t, f.root.ID.String(), result["chat_id"])
		require.Equal(t, f.root.Title, result["title"])
		require.Equal(t, "parent", result["relation"])
		require.Equal(t, "waiting", result["previous_status"])
		require.Equal(t, "running", result["status"])
		require.Equal(t, "started", result["delivery"])
		require.EqualValues(t, 1, result["relay_hop"])
		require.Contains(t, result, "message_id")
		require.NotContains(t, result, "queued_message_id")
		require.NotContains(t, result, "downgraded_from")

		parts := f.deliveredParts(t, f.root.ID)
		require.Len(t, parts, 2)
		require.Equal(t, codersdk.ChatMessagePartTypeSenderChat, parts[0].Type)
		require.NotNil(t, parts[0].SenderChatID)
		require.Equal(t, child.ID, *parts[0].SenderChatID)
		require.Equal(t, child.Title, parts[0].SenderChatTitle)
		require.Equal(t, codersdk.ChatSenderChatRelationChild, parts[0].SenderChatRelation)
		require.Equal(t, 1, parts[0].RelayHop)
		require.Equal(t, codersdk.ChatMessageText("status: done"), parts[1])
		require.Equal(t, database.ChatStatusRunning, f.reload(t, f.root.ID).Status)

		// The client conversion keeps the provenance part.
		messages, err := f.db.GetChatMessagesByChatID(f.ctx, database.GetChatMessagesByChatIDParams{ChatID: f.root.ID})
		require.NoError(t, err)
		sdkMessage := db2sdk.ChatMessage(messages[len(messages)-1])
		require.Equal(t, codersdk.ChatMessagePartTypeSenderChat, sdkMessage.Content[0].Type)

		require.Equal(t, float64(1), promtestutil.ToFloat64(
			f.server.metrics.ChatTreeMessagesTotal.WithLabelValues("parent", "queue", "started"),
		))
	})

	t.Run("ParentUUIDAndNamedParent", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		parent := f.namedChild(t, f.root, database.ChatStatusWaiting)
		child := f.namedChild(t, parent, database.ChatStatusRunning)

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: parent.ID.String(), Message: "report"})
		result := requireToolResponseMap(t, resp, false)
		require.Equal(t, parent.ID.String(), result["chat_id"])
		require.Equal(t, "parent", result["relation"])
		require.Equal(t, "started", result["delivery"])
	})

	t.Run("RunningTargetQueues", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		parent := f.namedChild(t, f.root, database.ChatStatusRunning)
		child := f.namedChild(t, parent, database.ChatStatusRunning)

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "later", Delivery: "queue"})
		result := requireToolResponseMap(t, resp, false)
		require.Equal(t, "queued", result["delivery"])
		require.Equal(t, "running", result["status"])
		require.Contains(t, result, "queued_message_id")
		require.NotContains(t, result, "message_id")

		queued, err := f.db.GetChatQueuedMessages(f.ctx, parent.ID)
		require.NoError(t, err)
		require.Len(t, queued, 1)
		require.Contains(t, string(queued[0].Content), `"sender-chat"`)
	})

	t.Run("RunningTargetInterrupts", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		parent := f.namedChild(t, f.root, database.ChatStatusRunning)
		child := f.namedChild(t, parent, database.ChatStatusRunning)

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "stop", Delivery: "interrupt"})
		result := requireToolResponseMap(t, resp, false)
		require.Equal(t, "interrupting", result["delivery"])
		require.Equal(t, "interrupting", result["status"])
		require.Equal(t, database.ChatStatusInterrupting, f.reload(t, parent.ID).Status)
		require.Equal(t, float64(1), promtestutil.ToFloat64(
			f.server.metrics.ChatTreeMessagesTotal.WithLabelValues("parent", "interrupt", "interrupting"),
		))
	})

	t.Run("ErrorTargetStarts", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		parent := f.namedChild(t, f.root, database.ChatStatusError)
		child := f.namedChild(t, parent, database.ChatStatusRunning)

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "retry"})
		result := requireToolResponseMap(t, resp, false)
		require.Equal(t, "error", result["previous_status"])
		require.Equal(t, "started", result["delivery"])
		require.Equal(t, database.ChatStatusRunning, f.reload(t, parent.ID).Status)
	})

	t.Run("RequiresActionDowngradesInterrupt", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		parent := f.namedChild(t, f.root, database.ChatStatusRequiresAction)
		child := f.namedChild(t, parent, database.ChatStatusRunning)

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "fyi", Delivery: "interrupt"})
		result := requireToolResponseMap(t, resp, false)
		require.Equal(t, "queued", result["delivery"])
		require.Equal(t, "interrupt", result["downgraded_from"])
		require.Equal(t, database.ChatStatusRequiresAction, f.reload(t, parent.ID).Status)
		queued, err := f.db.GetChatQueuedMessages(f.ctx, parent.ID)
		require.NoError(t, err)
		require.Len(t, queued, 1)
		require.Equal(t, float64(1), promtestutil.ToFloat64(
			f.server.metrics.ChatTreeMessagesTotal.WithLabelValues("parent", "queue", "queued"),
		))
	})

	t.Run("ErrorTargetWithQueueQueuesAndPromotesHead", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		parent := f.namedChild(t, f.root, database.ChatStatusError)
		child := f.namedChild(t, parent, database.ChatStatusRunning)
		head, err := f.db.InsertChatQueuedMessage(f.ctx, database.InsertChatQueuedMessageParams{
			ChatID:        parent.ID,
			Content:       mustMarshalText(t, "head").RawMessage,
			ModelConfigID: uuid.NullUUID{UUID: f.modelConfig.ID, Valid: true},
		})
		require.NoError(t, err)

		// E1 + queue promotes the previous head and queues the new row.
		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "after head"})
		result := requireToolResponseMap(t, resp, false)
		require.Equal(t, "error", result["previous_status"])
		require.Equal(t, "queued", result["delivery"])
		require.Equal(t, "running", result["status"])
		require.Contains(t, result, "queued_message_id")
		require.NotEqual(t, float64(head.ID), result["queued_message_id"])

		queued, err := f.db.GetChatQueuedMessages(f.ctx, parent.ID)
		require.NoError(t, err)
		require.Len(t, queued, 1)
		require.Contains(t, string(queued[0].Content), `"sender-chat"`)
		parts := f.deliveredParts(t, parent.ID)
		require.Equal(t, codersdk.ChatMessageText("head"), parts[0])
	})

	t.Run("InterruptingTargetReportsQueued", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		parent := f.namedChild(t, f.root, database.ChatStatusInterrupting)
		child := f.namedChild(t, parent, database.ChatStatusRunning)

		// I0 + interrupt only queues; the target was already interrupting.
		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "again", Delivery: "interrupt"})
		result := requireToolResponseMap(t, resp, false)
		require.Equal(t, "interrupting", result["previous_status"])
		require.Equal(t, "queued", result["delivery"])
		require.Equal(t, "interrupting", result["status"])
		require.NotContains(t, result, "downgraded_from")
		require.Equal(t, float64(1), promtestutil.ToFloat64(
			f.server.metrics.ChatTreeMessagesTotal.WithLabelValues("parent", "interrupt", "queued"),
		))
	})

	t.Run("QueueFullRejects", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		parent := f.namedChild(t, f.root, database.ChatStatusRunning)
		child := f.namedChild(t, parent, database.ChatStatusRunning)
		for i := 0; i < chatstate.MaxQueueSize; i++ {
			_, err := f.db.InsertChatQueuedMessage(f.ctx, database.InsertChatQueuedMessageParams{
				ChatID:        parent.ID,
				Content:       mustMarshalText(t, "q").RawMessage,
				ModelConfigID: uuid.NullUUID{UUID: f.modelConfig.ID, Valid: true},
			})
			require.NoError(t, err)
		}

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "one more"})
		result := requireToolError(t, resp, &chatstate.MessageQueueFullError{Max: chatstate.MaxQueueSize})
		require.Equal(t, parent.ID.String(), result["chat_id"])
		queued, err := f.db.GetChatQueuedMessages(f.ctx, parent.ID)
		require.NoError(t, err)
		require.Len(t, queued, chatstate.MaxQueueSize)
	})
}

func TestSendChatMessage_ParentToChild(t *testing.T) {
	t.Parallel()
	f := newChatTreeFixture(t)
	child := f.namedChild(t, f.root, database.ChatStatusWaiting)

	resp := f.send(t, f.root, humanTurnHistory(t), sendChatMessageArgs{ChatID: child.ID.String(), Message: "proceed"})
	result := requireToolResponseMap(t, resp, false)
	require.Equal(t, child.ID.String(), result["chat_id"])
	require.Equal(t, "child", result["relation"])
	require.Equal(t, "started", result["delivery"])

	parts := f.deliveredParts(t, child.ID)
	require.Equal(t, codersdk.ChatSenderChatRelationParent, parts[0].SenderChatRelation)
	require.Equal(t, f.root.ID, *parts[0].SenderChatID)
	require.Equal(t, float64(1), promtestutil.ToFloat64(
		f.server.metrics.ChatTreeMessagesTotal.WithLabelValues("child", "queue", "started"),
	))
}

func TestChatTreeTools_ThroughDBAuthz(t *testing.T) {
	t.Parallel()

	t.Run("SendChildToParent", func(t *testing.T) {
		t.Parallel()
		f := newAuthzChatTreeFixture(t)
		child := f.namedChild(t, f.root, database.ChatStatusRunning)

		// f.ctx is the chatd worker context the generation loop hands to
		// tool callbacks.
		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "authz"})
		result := requireToolResponseMap(t, resp, false)
		require.Equal(t, "started", result["delivery"])
		parts := f.deliveredParts(t, f.root.ID)
		require.Equal(t, child.ID, *parts[0].SenderChatID)
		require.Equal(t, database.ChatStatusRunning, f.reload(t, f.root.ID).Status)
	})

	t.Run("SendParentToChildWithInterrupt", func(t *testing.T) {
		t.Parallel()
		f := newAuthzChatTreeFixture(t)
		child := f.namedChild(t, f.root, database.ChatStatusRunning)

		resp := f.send(t, f.root, humanTurnHistory(t), sendChatMessageArgs{ChatID: child.ID.String(), Message: "stop", Delivery: "interrupt"})
		result := requireToolResponseMap(t, resp, false)
		require.Equal(t, "interrupting", result["delivery"])
		require.Equal(t, database.ChatStatusInterrupting, f.reload(t, child.ID).Status)
	})

	t.Run("OtherOwnerIsNotANeighbour", func(t *testing.T) {
		t.Parallel()
		f := newAuthzChatTreeFixture(t)
		otherUser := dbgen.User(t, f.db, database.User{})
		dbgen.OrganizationMember(t, f.db, database.OrganizationMember{UserID: otherUser.ID, OrganizationID: f.org.ID})
		foreign := dbgen.Chat(t, f.db, database.Chat{
			OrganizationID:    f.org.ID,
			OwnerID:           otherUser.ID,
			Kind:              database.ChatKindChat,
			ParentChatID:      uuid.NullUUID{UUID: f.root.ID, Valid: true},
			LastModelConfigID: f.modelConfig.ID,
		})

		resp := f.send(t, f.root, humanTurnHistory(t), sendChatMessageArgs{ChatID: foreign.ID.String(), Message: "x"})
		requireToolError(t, resp, ErrChatTreeNotNeighbour)
	})

	t.Run("SuspendedOwner", func(t *testing.T) {
		t.Parallel()
		f := newAuthzChatTreeFixture(t)
		child := f.namedChild(t, f.root, database.ChatStatusRunning)
		_, err := f.db.UpdateUserStatus(f.ctx, database.UpdateUserStatusParams{
			ID:        f.user.ID,
			Status:    database.UserStatusSuspended,
			UpdatedAt: time.Now(),
		})
		require.NoError(t, err)

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "x"})
		requireToolError(t, resp, ErrChatOwnerInactive)
	})

	t.Run("ListChatTree", func(t *testing.T) {
		t.Parallel()
		f := newAuthzChatTreeFixture(t)
		sender := f.namedChild(t, f.root, database.ChatStatusRunning)
		grandchild := f.namedChild(t, sender, database.ChatStatusWaiting)
		f.subagentChild(t, sender)

		resp := f.runTool(t, sender, nil, listChatTreeToolName, listChatTreeArgs{})
		result := requireToolResponseMap(t, resp, false)
		require.Equal(t, f.root.ID.String(), result["parent"].(map[string]any)["chat_id"])
		children := result["children"].([]any)
		require.Len(t, children, 1)
		require.Equal(t, grandchild.ID.String(), children[0].(map[string]any)["chat_id"])
	})
}

func TestSendChatMessage_Rejections(t *testing.T) {
	t.Parallel()
	f := newChatTreeFixture(t)
	sender := f.namedChild(t, f.root, database.ChatStatusRunning)
	sibling := f.namedChild(t, f.root, database.ChatStatusWaiting)
	child := f.namedChild(t, sender, database.ChatStatusWaiting)
	grandchild := f.namedChild(t, child, database.ChatStatusWaiting)
	subagent := f.subagentChild(t, sender)
	archivedChild := f.namedChild(t, sender, database.ChatStatusWaiting)
	_, err := f.db.ArchiveChatByID(f.ctx, archivedChild.ID)
	require.NoError(t, err)

	otherUser := dbgen.User(t, f.db, database.User{})
	dbgen.OrganizationMember(t, f.db, database.OrganizationMember{UserID: otherUser.ID, OrganizationID: f.org.ID})
	foreign := dbgen.Chat(t, f.db, database.Chat{
		OrganizationID:    f.org.ID,
		OwnerID:           otherUser.ID,
		Kind:              database.ChatKindChat,
		ParentChatID:      uuid.NullUUID{UUID: sender.ID, Valid: true},
		LastModelConfigID: f.modelConfig.ID,
	})
	legacy := dbgen.Chat(t, f.db, database.Chat{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		Kind:              database.ChatKindChat,
		LastModelConfigID: f.modelConfig.ID,
		Status:            database.ChatStatusRunning,
	})

	history := humanTurnHistory(t)
	cases := []struct {
		name   string
		sender database.Chat
		args   sendChatMessageArgs
		want   error
	}{
		{name: "Self", sender: sender, args: sendChatMessageArgs{ChatID: sender.ID.String(), Message: "x"}, want: ErrChatTreeNotNeighbour},
		{name: "Sibling", sender: sender, args: sendChatMessageArgs{ChatID: sibling.ID.String(), Message: "x"}, want: ErrChatTreeNotNeighbour},
		{name: "Grandchild", sender: sender, args: sendChatMessageArgs{ChatID: grandchild.ID.String(), Message: "x"}, want: ErrChatTreeNotNeighbour},
		{name: "SubagentChild", sender: sender, args: sendChatMessageArgs{ChatID: subagent.ID.String(), Message: "x"}, want: ErrChatTreeNotNeighbour},
		{name: "OtherOwner", sender: sender, args: sendChatMessageArgs{ChatID: foreign.ID.String(), Message: "x"}, want: ErrChatTreeNotNeighbour},
		{name: "Missing", sender: sender, args: sendChatMessageArgs{ChatID: uuid.NewString(), Message: "x"}, want: ErrChatTreeNotNeighbour},
		{name: "ArchivedChild", sender: sender, args: sendChatMessageArgs{ChatID: archivedChild.ID.String(), Message: "x"}, want: ErrChatArchived},
		{name: "Empty", sender: sender, args: sendChatMessageArgs{ChatID: child.ID.String(), Message: " \n\t"}, want: ErrChatTreeMessageEmpty},
		{name: "TooLong", sender: sender, args: sendChatMessageArgs{ChatID: child.ID.String(), Message: strings.Repeat("ü", chatTreeMessageMaxRunes+1)}, want: ErrChatTreeMessageTooLong},
		{name: "UnparsableID", sender: sender, args: sendChatMessageArgs{ChatID: "not-a-uuid", Message: "x"}, want: ErrChatTreeInvalidChatID},
		{name: "InvalidDelivery", sender: sender, args: sendChatMessageArgs{ChatID: child.ID.String(), Message: "x", Delivery: "later"}, want: ErrChatTreeInvalidDelivery},
		{name: "LegacyParentlessSender", sender: legacy, args: sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "x"}, want: ErrChatTreeNoParent},
		{name: "SubagentSender", sender: subagent, args: sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "x"}, want: ErrChatTreeSenderNotEligible},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := f.send(t, tc.sender, history, tc.args)
			requireToolError(t, resp, tc.want)
		})
	}

	t.Run("ExactLimitAcceptedAndRejectedTargetsUntouched", func(t *testing.T) {
		t.Parallel()
		// Rejection subtests above run in parallel with this one; none of
		// them delivers anything, so the target checks below hold
		// regardless of ordering.
		resp := f.send(t, sender, history, sendChatMessageArgs{ChatID: child.ID.String(), Message: strings.Repeat("ü", chatTreeMessageMaxRunes)})
		requireToolResponseMap(t, resp, false)

		for _, chatID := range []uuid.UUID{sibling.ID, grandchild.ID, subagent.ID, foreign.ID, archivedChild.ID, f.root.ID} {
			messages, err := f.db.GetChatMessagesByChatID(f.ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
			require.NoError(t, err)
			require.Empty(t, messages, "chat %s", chatID)
		}
	})
}

func TestSendChatMessage_InactiveOwner(t *testing.T) {
	t.Parallel()

	t.Run("Suspended", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		child := f.namedChild(t, f.root, database.ChatStatusRunning)
		_, err := f.db.UpdateUserStatus(f.ctx, database.UpdateUserStatusParams{
			ID:        f.user.ID,
			Status:    database.UserStatusSuspended,
			UpdatedAt: time.Now(),
		})
		require.NoError(t, err)

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "x"})
		requireToolError(t, resp, ErrChatOwnerInactive)
		messages, err := f.db.GetChatMessagesByChatID(f.ctx, database.GetChatMessagesByChatIDParams{ChatID: f.root.ID})
		require.NoError(t, err)
		require.Empty(t, messages)
	})

	t.Run("Deleted", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		child := f.namedChild(t, f.root, database.ChatStatusRunning)
		// Deletion keeps status active; the owner row is filtered by the
		// deleted flag.
		require.NoError(t, f.db.UpdateUserDeletedByID(f.ctx, f.user.ID))

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "x"})
		requireToolError(t, resp, ErrChatOwnerInactive)
		messages, err := f.db.GetChatMessagesByChatID(f.ctx, database.GetChatMessagesByChatIDParams{ChatID: f.root.ID})
		require.NoError(t, err)
		require.Empty(t, messages)
	})
}

func TestSendChatMessage_RelayHop(t *testing.T) {
	t.Parallel()

	t.Run("RelayedTurnIncrementsMax", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		child := f.namedChild(t, f.root, database.ChatStatusRunning)
		history := []database.ChatMessage{
			humanRow(t, "older"),
			assistantSendRow(t, "call-old"),
			toolResultRow(t, "call-old"),
			relayedRow(t, 2),
			humanRow(t, "human in same segment"),
			relayedRow(t, 5),
			assistantSendRow(t, sendCallID),
		}
		resp := f.send(t, child, history, sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "x"})
		result := requireToolResponseMap(t, resp, false)
		require.EqualValues(t, 6, result["relay_hop"])
		require.Equal(t, 6, f.deliveredParts(t, f.root.ID)[0].RelayHop)
	})

	t.Run("HumanOnlySegmentResets", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		child := f.namedChild(t, f.root, database.ChatStatusRunning)
		history := []database.ChatMessage{
			relayedRow(t, chatTreeMessageMaxHops),
			assistantSendRow(t, "call-old"),
			toolResultRow(t, "call-old"),
			humanRow(t, "fresh"),
			assistantSendRow(t, sendCallID),
		}
		resp := f.send(t, child, history, sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "x"})
		result := requireToolResponseMap(t, resp, false)
		require.EqualValues(t, 1, result["relay_hop"])
	})

	t.Run("HopLimitRejects", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		child := f.namedChild(t, f.root, database.ChatStatusRunning)
		history := []database.ChatMessage{relayedRow(t, chatTreeMessageMaxHops), assistantSendRow(t, sendCallID)}
		resp := f.send(t, child, history, sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "x"})
		result := requireToolError(t, resp, ErrChatTreeMessageRelayLimit)
		require.Equal(t, f.root.ID.String(), result["chat_id"])
		messages, err := f.db.GetChatMessagesByChatID(f.ctx, database.GetChatMessagesByChatIDParams{ChatID: f.root.ID})
		require.NoError(t, err)
		require.Empty(t, messages)
		require.Equal(t, float64(1), promtestutil.ToFloat64(
			f.server.metrics.ChatTreeMessagesTotal.WithLabelValues("parent", "queue", "rejected"),
		))
	})

	t.Run("EmptyHistoryFailsClosed", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		child := f.namedChild(t, f.root, database.ChatStatusRunning)
		resp := f.send(t, child, nil, sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "x"})
		requireToolError(t, resp, errChatTreeHistoryUnavailable)
	})

	t.Run("ModelOnlyRowsAreNotInHistory", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		child := f.namedChild(t, f.root, database.ChatStatusRunning)
		insert := func(seed database.ChatMessage, visibility database.ChatMessageVisibility) {
			seed.ChatID = child.ID
			seed.Visibility = visibility
			dbgen.ChatMessage(t, f.db, seed)
		}
		insert(relayedRow(t, 3), database.ChatMessageVisibilityBoth)
		insert(assistantSendRow(t, "call-old"), database.ChatMessageVisibilityBoth)
		insert(toolResultRow(t, "call-old"), database.ChatMessageVisibilityBoth)
		// A model-only user row after the relayed prompt would start a
		// fresh human segment if it were visible.
		insert(humanRow(t, "model-only sentinel"), database.ChatMessageVisibilityModel)
		insert(assistantSendRow(t, sendCallID), database.ChatMessageVisibilityBoth)

		history, err := f.db.GetChatMessagesByChatID(f.ctx, database.GetChatMessagesByChatIDParams{ChatID: child.ID})
		require.NoError(t, err)
		require.Len(t, history, 4)
		state, err := chatTreeTurnStateFromHistory(history)
		require.NoError(t, err)
		require.True(t, state.relayed)
		require.Equal(t, 1, state.resolvedSends)

		resp := f.send(t, child, history, sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "x"})
		result := requireToolResponseMap(t, resp, false)
		require.EqualValues(t, 4, result["relay_hop"])
	})
}

func TestChatTreeTurnStateFromHistory(t *testing.T) {
	t.Parallel()

	t.Run("BudgetCountsResolvedAndBatchPosition", func(t *testing.T) {
		t.Parallel()
		history := []database.ChatMessage{
			relayedRow(t, 1),
			assistantSendRow(t, "a"),
			toolResultRow(t, "a"),
			assistantSendRow(t, "b", "c", "d"),
		}
		state, err := chatTreeTurnStateFromHistory(history)
		require.NoError(t, err)
		require.True(t, state.relayed)
		require.Equal(t, 1, state.maxHop)
		require.Equal(t, 1, state.resolvedSends)
		require.Equal(t, []string{"b", "c", "d"}, state.unresolvedSendCallIDs)
		require.Equal(t, chatTreeRelayedTurnSendLimit, state.sendLimit())
		// resolved 1 + position: b=2, c=3 allowed, d=4 rejected.
		require.True(t, state.allowSend("b"))
		require.True(t, state.allowSend("c"))
		require.False(t, state.allowSend("d"))
		// Re-executing an allowed call yields the same decision.
		require.True(t, state.allowSend("b"))
		// An unknown call id sorts after every unresolved call.
		require.False(t, state.allowSend("unknown"))
	})

	t.Run("HumanTurnBudget", func(t *testing.T) {
		t.Parallel()
		history := []database.ChatMessage{humanRow(t, "go")}
		for i := 0; i < chatTreeHumanTurnSendLimit-1; i++ {
			id := uuid.NewString()
			history = append(history, assistantSendRow(t, id), toolResultRow(t, id))
		}
		history = append(history, assistantSendRow(t, "last", "over"))
		state, err := chatTreeTurnStateFromHistory(history)
		require.NoError(t, err)
		require.False(t, state.relayed)
		require.Equal(t, chatTreeHumanTurnSendLimit, state.sendLimit())
		require.True(t, state.allowSend("last"))
		require.False(t, state.allowSend("over"))
	})

	t.Run("PreviousTurnDoesNotCount", func(t *testing.T) {
		t.Parallel()
		history := []database.ChatMessage{
			relayedRow(t, 7),
			assistantSendRow(t, "old"),
			toolResultRow(t, "old"),
			humanRow(t, "new turn"),
			{Role: database.ChatMessageRoleSystem, Content: mustMarshalText(t, "notice"), ContentVersion: chatprompt.CurrentContentVersion},
			assistantSendRow(t, "now"),
		}
		state, err := chatTreeTurnStateFromHistory(history)
		require.NoError(t, err)
		require.False(t, state.relayed)
		require.Equal(t, 0, state.maxHop)
		require.Equal(t, 0, state.resolvedSends)
		require.Equal(t, []string{"now"}, state.unresolvedSendCallIDs)
	})

	t.Run("DeletedRowsIgnored", func(t *testing.T) {
		t.Parallel()
		deleted := relayedRow(t, 9)
		deleted.Deleted = true
		history := []database.ChatMessage{humanRow(t, "x"), deleted, assistantSendRow(t, "n")}
		state, err := chatTreeTurnStateFromHistory(history)
		require.NoError(t, err)
		require.False(t, state.relayed)
	})

	t.Run("NoUserRowFails", func(t *testing.T) {
		t.Parallel()
		_, err := chatTreeTurnStateFromHistory([]database.ChatMessage{assistantSendRow(t, "n")})
		require.ErrorIs(t, err, errChatTreeHistoryUnavailable)
	})

	t.Run("DuplicateCallIDsOccupySeparateSlots", func(t *testing.T) {
		t.Parallel()
		history := []database.ChatMessage{
			relayedRow(t, 1),
			assistantSendRow(t, "dup", "dup", "dup", "dup"),
		}
		state, err := chatTreeTurnStateFromHistory(history)
		require.NoError(t, err)
		require.Equal(t, []string{"dup", "dup", "dup", "dup"}, state.unresolvedSendCallIDs)
		// The last occurrence sits at position 3, past the relayed limit.
		require.False(t, state.allowSend("dup"))
	})

	t.Run("CompressedRowsStillCount", func(t *testing.T) {
		t.Parallel()
		compress := func(msg database.ChatMessage) database.ChatMessage {
			msg.Compressed = true
			return msg
		}
		history := []database.ChatMessage{
			compress(relayedRow(t, 4)),
			compress(assistantSendRow(t, "a")),
			compress(toolResultRow(t, "a")),
			assistantSendRow(t, "b"),
		}
		state, err := chatTreeTurnStateFromHistory(history)
		require.NoError(t, err)
		require.True(t, state.relayed)
		require.Equal(t, 4, state.maxHop)
		require.Equal(t, 1, state.resolvedSends)
		require.Equal(t, []string{"b"}, state.unresolvedSendCallIDs)
	})

	t.Run("CanceledTurnBeforeHumanIsNotInSegment", func(t *testing.T) {
		t.Parallel()
		// An interrupted turn leaves its assistant row and synthetic
		// cancellation result before the next human prompt.
		history := []database.ChatMessage{
			relayedRow(t, 6),
			assistantSendRow(t, "pending"),
			toolResultRow(t, "pending"),
			humanRow(t, "fresh"),
			assistantSendRow(t, "now"),
		}
		state, err := chatTreeTurnStateFromHistory(history)
		require.NoError(t, err)
		require.False(t, state.relayed)
		require.Equal(t, 0, state.maxHop)
		require.Equal(t, 0, state.resolvedSends)
		require.Equal(t, []string{"now"}, state.unresolvedSendCallIDs)
	})
}

func TestSendChatMessage_TurnBudget(t *testing.T) {
	t.Parallel()
	f := newChatTreeFixture(t)
	child := f.namedChild(t, f.root, database.ChatStatusRunning)
	history := []database.ChatMessage{relayedRow(t, 1)}
	for i := 0; i < chatTreeRelayedTurnSendLimit; i++ {
		id := uuid.NewString()
		history = append(history, assistantSendRow(t, id), toolResultRow(t, id))
	}
	history = append(history, assistantSendRow(t, sendCallID))

	resp := f.send(t, child, history, sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "x"})
	requireToolError(t, resp, ErrChatTreeMessageTurnLimit)
	messages, err := f.db.GetChatMessagesByChatID(f.ctx, database.GetChatMessagesByChatIDParams{ChatID: f.root.ID})
	require.NoError(t, err)
	require.Empty(t, messages)
}

// TestSendChatMessage_TurnBudgetThroughGenerationLoop drives a real worker
// turn with a fake model that emits more send_chat_message calls in one
// step than the relayed budget allows. The tool must derive the budget from
// the history the generation loop loaded for the step.
func TestSendChatMessage_TurnBudgetThroughGenerationLoop(t *testing.T) {
	t.Parallel()

	const fanOutPrompt = "fan out to the parent"
	const attempts = chatTreeRelayedTurnSendLimit + 1

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if len(req.Messages) == 0 {
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		}
		last := req.Messages[len(req.Messages)-1]
		if last.Role != "user" || !strings.Contains(last.Content, fanOutPrompt) {
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		}
		chunks := make([]chattest.OpenAIChunk, 0, attempts)
		for i := 0; i < attempts; i++ {
			args, err := json.Marshal(sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: fmt.Sprintf("ping %d", i)})
			require.NoError(t, err)
			chunks = append(chunks, chattest.OpenAIChunk{
				ID:      "chatcmpl-fanout",
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   "gpt-4",
				Choices: []chattest.OpenAIChunkChoice{{
					ToolCalls: []chattest.OpenAIToolCall{{
						Index:    i,
						ID:       fmt.Sprintf("call_fanout_%d", i),
						Type:     "function",
						Function: chattest.OpenAIToolCallFunction{Name: sendChatMessageToolName, Arguments: string(args)},
					}},
				}},
			})
		}
		return chattest.OpenAIStreamingResponse(chunks...)
	})
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, _ := seedInternalChatDeps(t, db)
	provider := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai-compat",
		DisplayName: "OpenAI compatible",
		BaseUrl:     openAIURL,
	})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:          "gpt-4o-mini",
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		OrganizationID: org.ID,
	})
	server := newInternalTestServer(
		t, db, ps, chatprovider.ProviderAPIKeys{},
		withInternalTestServerWorker(),
		withInternalTestServerTransportFactory(factory),
	)
	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Kind:              database.ChatKindRoot,
		Title:             "root " + uuid.NewString(),
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusWaiting,
	})

	// The sender's starting prompt is itself a relayed message, so the
	// relayed per-turn budget applies.
	sender, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "sender",
		ModelConfigID:  model.ID,
		ParentChatID:   uuid.NullUUID{UUID: root.ID, Valid: true},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageSenderChat(root.ID, root.Title, codersdk.ChatSenderChatRelationParent, 1),
			codersdk.ChatMessageText(fanOutPrompt),
		},
	})
	require.NoError(t, err)

	var sendResults []codersdk.ChatMessagePart
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		chat, err := db.GetChatByID(ctx, sender.ID)
		if err != nil || chat.Status != database.ChatStatusWaiting {
			return false
		}
		messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: sender.ID})
		if err != nil {
			return false
		}
		sendResults = sendResults[:0]
		for _, msg := range messages {
			if msg.Role != database.ChatMessageRoleTool {
				continue
			}
			parts, err := chatprompt.ParseContent(msg)
			if err != nil {
				return false
			}
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == sendChatMessageToolName {
					sendResults = append(sendResults, part)
				}
			}
		}
		return len(sendResults) == attempts
	}, testutil.IntervalFast, "sender chat did not finish its turn")

	senderChat, err := db.GetChatByID(ctx, sender.ID)
	require.NoError(t, err)
	require.False(t, senderChat.LastError.Valid, "sender turn failed: %s", string(senderChat.LastError.RawMessage))

	delivered := 0
	for _, part := range sendResults {
		// Error results keep the tool's JSON object rather than being
		// wrapped as {"error": <text>}.
		var result map[string]any
		require.NoError(t, json.Unmarshal(part.Result, &result), "result %s", part.Result)
		require.Equal(t, root.ID.String(), result["chat_id"])
		if part.IsError {
			require.Equal(t, ErrChatTreeMessageTurnLimit.Error(), result["error"])
			continue
		}
		delivered++
	}
	require.Equal(t, chatTreeRelayedTurnSendLimit, delivered)

	// Every accepted call reached the root as a relayed prompt or a
	// queued message; the rejected one left nothing behind.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		queued, err := db.GetChatQueuedMessages(ctx, root.ID)
		if err != nil {
			return false
		}
		messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: root.ID})
		if err != nil {
			return false
		}
		relayed := 0
		for _, msg := range messages {
			if msg.Role != database.ChatMessageRoleUser {
				continue
			}
			parts, err := chatprompt.ParseContent(msg)
			if err != nil {
				return false
			}
			if slices.ContainsFunc(parts, func(p codersdk.ChatMessagePart) bool { return p.Type == codersdk.ChatMessagePartTypeSenderChat }) {
				relayed++
			}
		}
		return relayed+len(queued) == chatTreeRelayedTurnSendLimit
	}, testutil.IntervalFast, "root did not receive exactly the budgeted messages")
}

func newChatTreeHookDispatcher(t *testing.T, consumer *httptest.Server) *dispatch.Dispatcher {
	t.Helper()
	return dispatch.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		consumer.Client(),
		consumer.URL,
		false,
		"test-hook-secret-32-bytes-minimum!!",
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	)
}

func TestSendChatMessage_Hooks(t *testing.T) {
	t.Parallel()

	t.Run("PromptSubmitSeesTargetAndText", func(t *testing.T) {
		t.Parallel()
		var received agenthooks.Request
		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
			_, err := w.Write([]byte(`{}`))
			require.NoError(t, err)
		}))
		t.Cleanup(consumer.Close)
		f := newChatTreeFixture(t, withInternalTestServerHookDispatcher(newChatTreeHookDispatcher(t, consumer)))
		child := f.namedChild(t, f.root, database.ChatStatusRunning)

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "admit me"})
		requireToolResponseMap(t, resp, false)
		require.Equal(t, agenthooks.EventUserPromptSubmit, received.Type)
		require.Equal(t, f.root.ID, received.Meta.ChatID)
		var data agenthooks.UserPromptSubmitData
		require.NoError(t, json.Unmarshal(received.Data, &data))
		require.Equal(t, "admit me", data.Prompt)
		require.Contains(t, string(data.Parts), `"sender-chat"`)
	})

	t.Run("DenialIsToolError", func(t *testing.T) {
		t.Parallel()
		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write([]byte(`{"permission":{"decision":"deny"},"user_message":"not now"}`))
			require.NoError(t, err)
		}))
		t.Cleanup(consumer.Close)
		f := newChatTreeFixture(t, withInternalTestServerHookDispatcher(newChatTreeHookDispatcher(t, consumer)))
		child := f.namedChild(t, f.root, database.ChatStatusRunning)

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "blocked"})
		result := requireToolResponseMap(t, resp, true)
		require.Contains(t, result["error"], "not now")
		messages, err := f.db.GetChatMessagesByChatID(f.ctx, database.GetChatMessagesByChatIDParams{ChatID: f.root.ID})
		require.NoError(t, err)
		require.Empty(t, messages)
	})

	t.Run("DispatchFailureIsRawErrorAndParksTarget", func(t *testing.T) {
		t.Parallel()
		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(consumer.Close)
		f := newChatTreeFixture(t, withInternalTestServerHookDispatcher(newChatTreeHookDispatcher(t, consumer)))
		child := f.namedChild(t, f.root, database.ChatStatusRunning)

		tools := f.server.chatTreeTools(
			func() database.Chat { return child },
			func() []database.ChatMessage { return humanTurnHistory(t) },
		)
		tool := findToolByName(tools, sendChatMessageToolName)
		require.NotNil(t, tool)
		input, err := json.Marshal(sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "fails"})
		require.NoError(t, err)
		_, err = tool.Run(f.ctx, fantasy.ToolCall{ID: sendCallID, Name: sendChatMessageToolName, Input: string(input)})
		var dispatchErr *dispatch.Error
		require.ErrorAs(t, err, &dispatchErr)
		require.Equal(t, database.ChatStatusError, f.reload(t, f.root.ID).Status)
	})

	t.Run("RequiresActionReachedDuringDispatchIsNotInterrupted", func(t *testing.T) {
		t.Parallel()
		// The hook consumer runs between the pre-send snapshot and the
		// locked transition; it moves the target from running to
		// requires_action so the downgrade decision must come from the
		// locked status.
		var (
			f      *chatTreeFixture
			parent database.Chat
		)
		consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := f.db.UpdateChatStatus(f.ctx, database.UpdateChatStatusParams{
				ID:     parent.ID,
				Status: database.ChatStatusRequiresAction,
			})
			require.NoError(t, err)
			_, err = w.Write([]byte(`{}`))
			require.NoError(t, err)
		}))
		t.Cleanup(consumer.Close)
		f = newChatTreeFixture(t, withInternalTestServerHookDispatcher(newChatTreeHookDispatcher(t, consumer)))
		parent = f.namedChild(t, f.root, database.ChatStatusRunning)
		child := f.namedChild(t, parent, database.ChatStatusRunning)

		resp := f.send(t, child, humanTurnHistory(t), sendChatMessageArgs{ChatID: chatTreeParentTarget, Message: "urgent", Delivery: "interrupt"})
		result := requireToolResponseMap(t, resp, false)
		require.Equal(t, "requires_action", result["previous_status"])
		require.Equal(t, "queued", result["delivery"])
		require.Equal(t, "interrupt", result["downgraded_from"])
		require.Equal(t, "requires_action", result["status"])

		require.Equal(t, database.ChatStatusRequiresAction, f.reload(t, parent.ID).Status)
		queued, err := f.db.GetChatQueuedMessages(f.ctx, parent.ID)
		require.NoError(t, err)
		require.Len(t, queued, 1)
		// No cancellation rows were written for the pending approval.
		messages, err := f.db.GetChatMessagesByChatID(f.ctx, database.GetChatMessagesByChatIDParams{ChatID: parent.ID})
		require.NoError(t, err)
		require.Empty(t, messages)
		require.Equal(t, float64(1), promtestutil.ToFloat64(
			f.server.metrics.ChatTreeMessagesTotal.WithLabelValues("parent", "queue", "queued"),
		))
	})
}

func TestListChatTree(t *testing.T) {
	t.Parallel()

	t.Run("NamedChatListsParentAndUnarchivedChildren", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		sender := f.namedChild(t, f.root, database.ChatStatusRunning)
		older := f.namedChild(t, sender, database.ChatStatusWaiting)
		newer := f.namedChild(t, sender, database.ChatStatusRunning)
		archived := f.namedChild(t, sender, database.ChatStatusWaiting)
		_, err := f.db.ArchiveChatByID(f.ctx, archived.ID)
		require.NoError(t, err)
		f.subagentChild(t, sender)
		// Bump newer so it sorts first regardless of insertion timing.
		_, err = f.db.UpdateChatStatus(f.ctx, database.UpdateChatStatusParams{ID: newer.ID, Status: database.ChatStatusWaiting})
		require.NoError(t, err)

		resp := f.runTool(t, sender, nil, listChatTreeToolName, listChatTreeArgs{})
		result := requireToolResponseMap(t, resp, false)
		self := result["self"].(map[string]any)
		require.Equal(t, sender.ID.String(), self["chat_id"])
		require.Equal(t, "chat", self["kind"])
		parent := result["parent"].(map[string]any)
		require.Equal(t, f.root.ID.String(), parent["chat_id"])
		require.Equal(t, "root", parent["kind"])
		children := result["children"].([]any)
		require.Len(t, children, 2)
		require.Equal(t, newer.ID.String(), children[0].(map[string]any)["chat_id"])
		require.Equal(t, older.ID.String(), children[1].(map[string]any)["chat_id"])
	})

	t.Run("UpdatedAtTiesOrderByID", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		rows := make([]database.GetChildChatsByParentIDsRow, 0, 3)
		for _, id := range []string{
			"cccccccc-0000-0000-0000-000000000000",
			"aaaaaaaa-0000-0000-0000-000000000000",
			"bbbbbbbb-0000-0000-0000-000000000000",
		} {
			rows = append(rows, database.GetChildChatsByParentIDsRow{Chat: database.Chat{ID: uuid.MustParse(id), UpdatedAt: now}})
		}
		sortChatTreeChildren(rows)
		require.Equal(t, "aaaaaaaa-0000-0000-0000-000000000000", rows[0].Chat.ID.String())
		require.Equal(t, "bbbbbbbb-0000-0000-0000-000000000000", rows[1].Chat.ID.String())
		require.Equal(t, "cccccccc-0000-0000-0000-000000000000", rows[2].Chat.ID.String())
	})

	t.Run("RootHasNoParent", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		resp := f.runTool(t, f.root, nil, listChatTreeToolName, listChatTreeArgs{})
		result := requireToolResponseMap(t, resp, false)
		require.Nil(t, result["parent"])
		require.Empty(t, result["children"])
	})

	t.Run("SuspendedOwner", func(t *testing.T) {
		t.Parallel()
		f := newChatTreeFixture(t)
		_, err := f.db.UpdateUserStatus(f.ctx, database.UpdateUserStatusParams{
			ID:        f.user.ID,
			Status:    database.UserStatusSuspended,
			UpdatedAt: time.Now(),
		})
		require.NoError(t, err)
		resp := f.runTool(t, f.root, nil, listChatTreeToolName, listChatTreeArgs{})
		requireToolError(t, resp, ErrChatOwnerInactive)
	})
}

func TestChatOwnerContextActsAsOwner(t *testing.T) {
	t.Parallel()
	f := newChatTreeFixture(t)
	ownerCtx, err := chatOwnerContext(f.ctx, f.db, f.user.ID)
	require.NoError(t, err)
	actor, ok := dbauthz.ActorFromContext(ownerCtx)
	require.True(t, ok)
	require.Equal(t, f.user.ID.String(), actor.ID)

	// The owner cannot read another user's chat, which the target
	// resolution reports as a missing neighbor.
	otherUser := dbgen.User(t, f.db, database.User{})
	foreign := dbgen.Chat(t, f.db, database.Chat{
		OrganizationID:    f.org.ID,
		OwnerID:           otherUser.ID,
		LastModelConfigID: f.modelConfig.ID,
	})
	_, err = resolveChatTreeMessageTarget(ownerCtx, f.db, f.root, foreign.ID.String())
	require.ErrorIs(t, err, ErrChatTreeNotNeighbour)
}
