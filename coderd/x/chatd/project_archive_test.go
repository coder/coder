package chatd_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestArchiveChatProjectStopsRunningChat archives a project while one of
// its chats streams a reply. The chat's runner stops, and its partial reply
// is discarded.
func TestArchiveChatProjectStopsRunningChat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	streaming := make(chan struct{})
	stopped := make(chan struct{})
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		chunks := make(chan chattest.OpenAIChunk, 1)
		go func() {
			defer close(chunks)
			for _, c := range chattest.OpenAITextChunks("partial reply") {
				chunks <- c
			}
			close(streaming)
			<-req.Context().Done()
			close(stopped)
		}()
		return chattest.OpenAIResponse{StreamingChunks: chunks}
	})

	server := newChatdServer(t, ps, chatd.Config{
		Logger:                     slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		// A heartbeat would also stop the runner. Delay it past the test,
		// so only the archive's chat:update can.
		ChatHeartbeatInterval:    testutil.WaitSuperLong,
		Experiments:              codersdk.ExperimentsKnown,
		AIBridgeTransportFactory: chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL)),
	})
	server.Start()
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		ProjectID:          uuid.NullUUID{UUID: project.ID, Valid: true},
		Title:              "archive-running-project-chat",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	testutil.TryReceive(ctx, t, streaming)

	archived, err := server.ArchiveChatProject(ctx, project.ID, true)
	require.NoError(t, err)
	require.True(t, archived.ArchivedAt.Valid)
	// The archive's chat:update stops the runner, which cancels the
	// provider request. The bound is shorter than the runner sync
	// interval, which would also stop it.
	testutil.TryReceive(testutil.Context(t, testutil.WaitShort), t, stopped)

	// Close waits for in-flight work, so a partial reply the runner
	// persisted would be visible below.
	require.NoError(t, server.Close())
	stored, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.True(t, stored.Archived)
	require.Equal(t, database.ChatStatusWaiting, stored.Status)
	msgs, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	for _, msg := range msgs {
		require.NotEqual(t, database.ChatMessageRoleAssistant, msg.Role, "the partial reply is discarded")
	}
}
