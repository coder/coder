package chatd

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// fakeUploadAgent stands in for a workspace agent's write-once chat file
// upload: the first upload of a file ID stores its content, later ones
// report the stored file.
type fakeUploadAgent struct {
	workspacesdk.AgentConn

	mu       sync.Mutex
	stream   func(ctx context.Context, body io.Reader) ([]byte, error)
	fail     func(req workspacesdk.UploadChatFileRequest) error
	afterPut func()
	requests []workspacesdk.UploadChatFileRequest
	timeouts []time.Duration
	files    map[uuid.UUID][]byte
}

func newFakeUploadAgent() *fakeUploadAgent {
	return &fakeUploadAgent{files: map[uuid.UUID][]byte{}}
}

func (*fakeUploadAgent) SetExtraHeaders(http.Header) {}

func (a *fakeUploadAgent) UploadChatFile(ctx context.Context, req workspacesdk.UploadChatFileRequest) (workspacesdk.AgentUploadChatFileResponse, error) {
	a.mu.Lock()
	stream := a.stream
	a.mu.Unlock()
	if stream == nil {
		stream = func(_ context.Context, body io.Reader) ([]byte, error) { return io.ReadAll(body) }
	}
	data, err := stream(ctx, req.Body)
	if err != nil {
		return workspacesdk.AgentUploadChatFileResponse{}, err
	}
	a.mu.Lock()
	req.Body = nil
	a.requests = append(a.requests, req)
	if deadline, ok := ctx.Deadline(); ok {
		a.timeouts = append(a.timeouts, time.Until(deadline))
	}
	fail, afterPut := a.fail, a.afterPut
	a.mu.Unlock()
	if fail != nil {
		if err := fail(req); err != nil {
			return workspacesdk.AgentUploadChatFileResponse{}, err
		}
	}

	a.mu.Lock()
	if _, ok := a.files[req.FileID]; !ok {
		a.files[req.FileID] = data
	}
	stored := a.files[req.FileID]
	a.mu.Unlock()
	if afterPut != nil {
		afterPut()
	}
	return workspacesdk.AgentUploadChatFileResponse{
		Path: path.Join("/home/coder/.coder/chats", req.ChatID, "files", req.FileID.String(), req.Name),
		Name: req.Name,
		Size: int64(len(stored)),
	}, nil
}

func (a *fakeUploadAgent) storedFiles() map[uuid.UUID][]byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	files := make(map[uuid.UUID][]byte, len(a.files))
	for id, data := range a.files {
		files[id] = data
	}
	return files
}

func (a *fakeUploadAgent) uploadRequests() []workspacesdk.UploadChatFileRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]workspacesdk.UploadChatFileRequest(nil), a.requests...)
}

func (a *fakeUploadAgent) uploadTimeouts() []time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]time.Duration(nil), a.timeouts...)
}

func agentStatusError(status int) error {
	return codersdk.ReadBodyAsError(&http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"message":"agent says no"}`)),
		Request: &http.Request{Method: http.MethodPost, URL: &url.URL{
			Scheme: "http",
			Host:   "[fd7a:115c:a1e0::1]:4",
			Path:   "/api/v0/upload-chat-file",
		}},
	})
}

type workspaceFileDeliveryFixture struct {
	*workerTestFixture
	clock *quartz.Mock
	agent *fakeUploadAgent
}

func newWorkspaceFileDeliveryFixture(t *testing.T) workspaceFileDeliveryFixture {
	t.Helper()
	clock := quartz.NewMock(t)
	clock.Set(dbtime.Now())
	return workspaceFileDeliveryFixture{
		workerTestFixture: newWorkerTestFixture(t),
		clock:             clock,
		agent:             newFakeUploadAgent(),
	}
}

// worker returns a worker that delivers through the fake agent. Workers
// of one fixture share its database, like replicas.
func (f workspaceFileDeliveryFixture) worker(t *testing.T) *chatWorker {
	t.Helper()
	// Delivery reads only the worker clock. The server's own tickers
	// stay on a real clock so they cannot block Advance.
	server := newUnstartedServer(t, f.pubsub, f.db)
	server.agentInactiveDisconnectTimeout = time.Hour
	server.agentConnFn = func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		return f.agent, func() {}, nil
	}
	opts := testOptions(t, f.workerTestFixture, nil)
	opts.Clock = f.clock
	worker, err := newChatWorker(server, opts)
	require.NoError(t, err)
	return worker
}

// loggingWorker is worker with a logger whose entries the test reads.
func (f workspaceFileDeliveryFixture) loggingWorker(t *testing.T) (*chatWorker, *testutil.FakeSink) {
	t.Helper()
	sink := testutil.NewFakeSink(t)
	worker := f.worker(t)
	worker.opts.Logger = sink.Logger()
	return worker, sink
}

func warnings(sink *testutil.FakeSink) []slog.SinkEntry {
	return sink.Entries(func(entry slog.SinkEntry) bool { return entry.Level >= slog.LevelWarn })
}

// workspaceChat creates a running chat bound to a started workspace with
// one connected agent. The seed sets the agent's lifecycle state and may
// mark it disconnected.
func (f workspaceFileDeliveryFixture) workspaceChat(ctx context.Context, t *testing.T, seed database.WorkspaceAgent) (database.Chat, database.WorkspaceAgent) {
	t.Helper()
	build := dbfake.WorkspaceBuild(t, f.db, database.WorkspaceTable{
		OwnerID:        f.user.ID,
		OrganizationID: f.org.ID,
	}).Seed(database.WorkspaceBuild{Transition: database.WorkspaceTransitionStart}).Do()
	resource := dbgen.WorkspaceResource(t, f.db, database.WorkspaceResource{JobID: build.Build.JobID})
	connectedAt := sql.NullTime{Time: dbtime.Now(), Valid: true}
	agent := dbgen.WorkspaceAgent(t, f.db, database.WorkspaceAgent{
		ResourceID:       resource.ID,
		Name:             "dev",
		FirstConnectedAt: connectedAt,
		LastConnectedAt:  connectedAt,
		DisconnectedAt:   seed.DisconnectedAt,
	})
	f.setLifecycle(ctx, t, agent.ID, seed.LifecycleState)

	res, err := chatstate.CreateChat(ctx, f.db, f.pubsub, chatstate.CreateChatInput{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		WorkspaceID:       uuid.NullUUID{UUID: build.Workspace.ID, Valid: true},
		LastModelConfigID: f.model.ID,
		Title:             "test",
		ClientType:        database.ChatClientTypeApi,
		InitialStatus:     database.ChatStatusRunning,
		InitialMessages: []chatstate.Message{
			userTextMessage(t, "hello", f.user.ID, f.model.ID, f.apiKey.ID),
		},
	})
	require.NoError(t, err)
	return res.Chat, agent
}

func (f workspaceFileDeliveryFixture) setLifecycle(ctx context.Context, t *testing.T, agentID uuid.UUID, state database.WorkspaceAgentLifecycleState) {
	t.Helper()
	require.NoError(t, f.db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
		ID:             agentID,
		LifecycleState: state,
	}))
}

// restartAgent records a finished agent startup, which sets a new
// ready_at like the agent's lifecycle report does.
func (f workspaceFileDeliveryFixture) restartAgent(ctx context.Context, t *testing.T, agentID uuid.UUID) {
	t.Helper()
	now := sql.NullTime{Time: dbtime.Now(), Valid: true}
	require.NoError(t, f.db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
		ID:             agentID,
		LifecycleState: database.WorkspaceAgentLifecycleStateReady,
		StartedAt:      now,
		ReadyAt:        now,
	}))
}

func (f workspaceFileDeliveryFixture) workspaceFile(ctx context.Context, t *testing.T, chatID uuid.UUID, data string) database.InsertChatFileRow {
	t.Helper()
	file, err := f.db.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID:        f.user.ID,
		OrganizationID: f.org.ID,
		Name:           "report.zip",
		Mimetype:       "application/zip",
		Data:           []byte(data),
		Purpose:        database.NullChatFilePurpose{ChatFilePurpose: database.ChatFilePurposeWorkspace, Valid: true},
	})
	require.NoError(t, err)
	_, err = f.db.LinkChatFiles(ctx, database.LinkChatFilesParams{
		ChatID:       chatID,
		FileIds:      []uuid.UUID{file.ID},
		MaxFileLinks: 10,
	})
	require.NoError(t, err)
	return file
}

func (f workspaceFileDeliveryFixture) fileState(ctx context.Context, t *testing.T, chatID, fileID uuid.UUID) database.GetChatFileMetadataByChatIDRow {
	t.Helper()
	files, err := f.db.GetChatFileMetadataByChatID(ctx, chatID)
	require.NoError(t, err)
	for _, file := range files {
		if file.ID == fileID {
			return file
		}
	}
	t.Fatalf("file %s is not linked to chat %s", fileID, chatID)
	return database.GetChatFileMetadataByChatIDRow{}
}

func (f workspaceFileDeliveryFixture) requireDelivered(ctx context.Context, t *testing.T, chat database.Chat, fileID uuid.UUID) database.GetChatFileMetadataByChatIDRow {
	t.Helper()
	var state database.GetChatFileMetadataByChatIDRow
	testutil.Eventually(ctx, t, func(context.Context) bool {
		state = f.fileState(ctx, t, chat.ID, fileID)
		return state.WorkspaceDeliveredAt.Valid
	}, testutil.IntervalFast)
	require.Equal(t, int64(len("zip bytes")), state.SizeBytes)
	require.Equal(t, path.Join("~/.coder/chats", chat.ID.String(), "files", fileID.String(), "report.zip"), state.WorkspaceDeliveredPath.String)
	require.Equal(t, chat.WorkspaceID, state.WorkspaceDeliveredWorkspaceID)
	return state
}

func TestWorkspaceFileDeliveryLoop(t *testing.T) {
	t.Parallel()

	// The loop sweeps every chat in its database, so each case gets its
	// own fixture.
	start := func(ctx context.Context, t *testing.T, f workspaceFileDeliveryFixture) {
		t.Helper()
		trap := f.clock.Trap().NewTicker("chatworker", "workspace-file-delivery")
		defer trap.Close()
		loopCtx, cancel := context.WithCancel(ctx)
		worker := f.worker(t)
		done := make(chan struct{})
		go func() {
			defer close(done)
			worker.workspaceFileDeliveryLoop(loopCtx)
		}()
		t.Cleanup(func() {
			cancel()
			<-done
		})
		// The ticker starts after the catch-up sweep.
		trap.MustWait(ctx).MustRelease(ctx)
	}

	t.Run("StartupEventDelivers", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, agent := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateStarting})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		before, err := f.db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		messagesBefore, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
		require.NoError(t, err)

		start(ctx, t, f)
		require.Empty(t, f.agent.uploadRequests())

		f.setLifecycle(ctx, t, agent.ID, database.WorkspaceAgentLifecycleStateReady)
		require.NoError(t, wspubsub.PublishAgentStartupFinished(ctx, f.pubsub, wspubsub.AgentStartupFinishedEvent{
			WorkspaceID: chat.WorkspaceID.UUID,
			AgentID:     agent.ID,
		}))
		f.requireDelivered(ctx, t, chat, file.ID)
		require.Equal(t, map[uuid.UUID][]byte{file.ID: []byte("zip bytes")}, f.agent.storedFiles())

		// Delivery during a running turn must not touch the transcript:
		// a history change cancels the turn.
		after, err := f.db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusRunning, after.Status)
		require.Equal(t, before.HistoryVersion, after.HistoryVersion)
		messagesAfter, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
		require.NoError(t, err)
		require.Equal(t, messagesBefore, messagesAfter)
	})

	t.Run("SweepCatchesMissedEvent", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, agent := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateStarting})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")

		start(ctx, t, f)
		f.setLifecycle(ctx, t, agent.ID, database.WorkspaceAgentLifecycleStateReady)
		f.clock.Advance(defaultWorkspaceFileDeliveryInterval).MustWait(ctx)
		f.requireDelivered(ctx, t, chat, file.ID)
	})

	t.Run("AgentStartupRetriesParkedFile", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		// An agent without write-once uploads rejects the file until the
		// workspace restarts with an updated agent.
		f.agent.fail = func(workspacesdk.UploadChatFileRequest) error {
			return agentStatusError(http.StatusBadRequest)
		}
		chat, agent := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")

		start(ctx, t, f)
		state := f.fileState(ctx, t, chat.ID, file.ID)
		require.Equal(t, database.ChatWorkspaceFileMaxDeliveryAttempts, state.WorkspaceDeliveryAttempts)
		require.Contains(t, state.WorkspaceDeliveryError.String, "update the workspace agent")

		f.agent.mu.Lock()
		f.agent.fail = nil
		f.agent.mu.Unlock()
		f.restartAgent(ctx, t, agent.ID)
		require.NoError(t, wspubsub.PublishAgentStartupFinished(ctx, f.pubsub, wspubsub.AgentStartupFinishedEvent{
			WorkspaceID: chat.WorkspaceID.UUID,
			AgentID:     agent.ID,
		}))
		f.requireDelivered(ctx, t, chat, file.ID)
	})

	t.Run("SweepRetriesParkedFileAfterMissedStartupEvent", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		f.agent.fail = func(workspacesdk.UploadChatFileRequest) error {
			return agentStatusError(http.StatusBadRequest)
		}
		chat, agent := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")

		start(ctx, t, f)
		state := f.fileState(ctx, t, chat.ID, file.ID)
		require.Equal(t, database.ChatWorkspaceFileMaxDeliveryAttempts, state.WorkspaceDeliveryAttempts)

		f.agent.mu.Lock()
		f.agent.fail = nil
		f.agent.mu.Unlock()
		// The updated agent starts, but its startup event never arrives.
		f.restartAgent(ctx, t, agent.ID)
		f.clock.Advance(defaultWorkspaceFileDeliveryInterval).MustWait(ctx)
		f.requireDelivered(ctx, t, chat, file.ID)
	})

	// startServer runs the delivery loop of a server's own worker, the
	// one the server's wake-ups poke, on the fixture clock. The clock
	// never reaches the sweep, so only a wake-up delivers a file that
	// becomes due afterwards.
	startServer := func(ctx context.Context, t *testing.T, f workspaceFileDeliveryFixture) *Server {
		t.Helper()
		server := newUnstartedServer(t, f.pubsub, f.db, withInternalTestServerClock(f.clock))
		server.agentInactiveDisconnectTimeout = time.Hour
		server.agentConnFn = func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return f.agent, func() {}, nil
		}
		trap := f.clock.Trap().NewTicker("chatworker", "workspace-file-delivery")
		defer trap.Close()
		loopCtx, cancel := context.WithCancel(ctx)
		loopDone := make(chan struct{})
		go func() {
			defer close(loopDone)
			server.chatWorker.workspaceFileDeliveryLoop(loopCtx)
		}()
		t.Cleanup(func() {
			cancel()
			<-loopDone
		})
		trap.MustWait(ctx).MustRelease(ctx)
		return server
	}

	t.Run("SendToRunningWorkspaceDelivers", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		file, err := f.db.InsertChatFile(ctx, database.InsertChatFileParams{
			OwnerID:        f.user.ID,
			OrganizationID: f.org.ID,
			Name:           "report.zip",
			Mimetype:       "application/zip",
			Data:           []byte("zip bytes"),
			Purpose:        database.NullChatFilePurpose{ChatFilePurpose: database.ChatFilePurposeWorkspace, Valid: true},
		})
		require.NoError(t, err)
		server := startServer(ctx, t, f)

		// The workspace already finished starting, so no startup event
		// follows: only the send can wake the loop.
		_, err = server.SendMessage(ctx, SendMessageOptions{
			ChatID:    chat.ID,
			CreatedBy: f.user.ID,
			Content: []codersdk.ChatMessagePart{
				codersdk.ChatMessageWorkspaceAttachment(file.ID, "application/zip", "report.zip", int64(len("zip bytes"))),
			},
		})
		require.NoError(t, err)
		f.requireDelivered(ctx, t, chat, file.ID)
	})

	t.Run("BindingDelivers", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		workspaceID := chat.WorkspaceID
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		_, err := f.db.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{ID: chat.ID})
		require.NoError(t, err)
		server := startServer(ctx, t, f)

		// PATCH /chats/{chat} binds a running workspace like this.
		chat, err = f.db.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{ID: chat.ID, WorkspaceID: workspaceID})
		require.NoError(t, err)
		server.RequestWorkspaceFileDelivery()
		f.requireDelivered(ctx, t, chat, file.ID)
	})

	t.Run("UnarchiveDelivers", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		// Only an idle chat can be archived.
		_, err := f.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{ID: chat.ID, Status: database.ChatStatusWaiting})
		require.NoError(t, err)
		_, err = f.db.ArchiveChatByID(ctx, chat.ID)
		require.NoError(t, err)
		server := startServer(ctx, t, f)
		require.Empty(t, f.agent.uploadRequests())

		chat, err = f.db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.NoError(t, server.UnarchiveChat(ctx, chat))
		f.requireDelivered(ctx, t, chat, file.ID)
	})
}

func TestWorkspaceFileDeliveryOutcomes(t *testing.T) {
	t.Parallel()

	t.Run("ServerErrorsRetryThenPark", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		f.agent.fail = func(workspacesdk.UploadChatFileRequest) error {
			return agentStatusError(http.StatusInternalServerError)
		}
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		worker := f.worker(t)

		for attempt := int32(1); attempt <= database.ChatWorkspaceFileMaxDeliveryAttempts; attempt++ {
			worker.deliverWorkspaceFiles(ctx)
			state := f.fileState(ctx, t, chat.ID, file.ID)
			require.Equal(t, attempt, state.WorkspaceDeliveryAttempts)
			require.Contains(t, state.WorkspaceDeliveryError.String, "agent says no")
			require.WithinDuration(t, f.clock.Now().Add(workspaceFileDeliveryRetryDelay), state.WorkspaceDeliveryNextAttemptAt.Time, time.Second)

			// Not due again until the retry delay passes.
			worker.deliverWorkspaceFiles(ctx)
			require.Len(t, f.agent.uploadRequests(), int(attempt))
			f.clock.Advance(workspaceFileDeliveryRetryDelay)
		}

		f.clock.Advance(time.Hour)
		worker.deliverWorkspaceFiles(ctx)
		require.Len(t, f.agent.uploadRequests(), int(database.ChatWorkspaceFileMaxDeliveryAttempts))
		require.False(t, f.fileState(ctx, t, chat.ID, file.ID).WorkspaceDeliveredAt.Valid)
	})

	t.Run("AgentRejectionParks", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name      string
			status    int
			wantError string
		}{
			// An agent without write-once uploads answers 400 to file_id.
			{name: "OutdatedAgent", status: http.StatusBadRequest, wantError: "update the workspace agent"},
			// An unwritable chat files directory answers 403.
			{name: "Forbidden", status: http.StatusForbidden, wantError: "status 403"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newWorkspaceFileDeliveryFixture(t)
				ctx := testutil.Context(t, testutil.WaitLong)
				f.agent.fail = func(workspacesdk.UploadChatFileRequest) error {
					return agentStatusError(tc.status)
				}
				chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
				file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
				worker := f.worker(t)

				worker.deliverWorkspaceFiles(ctx)
				state := f.fileState(ctx, t, chat.ID, file.ID)
				require.Equal(t, database.ChatWorkspaceFileMaxDeliveryAttempts, state.WorkspaceDeliveryAttempts)
				require.Contains(t, state.WorkspaceDeliveryError.String, tc.wantError)

				f.clock.Advance(time.Hour)
				worker.deliverWorkspaceFiles(ctx)
				require.Len(t, f.agent.uploadRequests(), 1)
			})
		}
	})

	t.Run("DifferentExistingFileParks", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		// The write-once target already holds an unrelated file, so the
		// agent reports that file instead of storing this one.
		f.agent.files[file.ID] = []byte("junk")
		worker := f.worker(t)

		worker.deliverWorkspaceFiles(ctx)
		state := f.fileState(ctx, t, chat.ID, file.ID)
		require.False(t, state.WorkspaceDeliveredAt.Valid)
		require.Equal(t, database.ChatWorkspaceFileMaxDeliveryAttempts, state.WorkspaceDeliveryAttempts)
		require.Contains(t, state.WorkspaceDeliveryError.String, path.Join("/home/coder/.coder/chats", chat.ID.String(), "files", file.ID.String(), "report.zip"))
		require.Contains(t, state.WorkspaceDeliveryError.String, "(4 bytes, expected 9 bytes)")
		stored, err := f.db.GetChatFileByID(ctx, file.ID)
		require.NoError(t, err)
		require.Equal(t, []byte("zip bytes"), stored.Data)

		f.clock.Advance(time.Hour)
		worker.deliverWorkspaceFiles(ctx)
		require.Len(t, f.agent.uploadRequests(), 1)
	})

	t.Run("FencedOutcomeLogsNoFailure", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		// Another replica records an outcome under the claim while the
		// upload runs, so this worker no longer holds it.
		f.agent.afterPut = func() {
			f.agent.mu.Lock()
			f.agent.afterPut = nil
			f.agent.mu.Unlock()
			lease := f.fileState(ctx, t, chat.ID, file.ID).WorkspaceDeliveryNextAttemptAt.Time
			failed, err := f.db.FailChatWorkspaceFileDelivery(ctx, database.FailChatWorkspaceFileDeliveryParams{
				ID:             file.ID,
				Error:          "recorded by another replica",
				NextAttemptAt:  lease.Add(time.Hour),
				Mode:           database.ChatWorkspaceFileDeliveryFailureRetry,
				MaxAttempts:    database.ChatWorkspaceFileMaxDeliveryAttempts,
				LeaseExpiresAt: lease,
			})
			require.NoError(t, err)
			require.EqualValues(t, 1, failed)
		}

		worker, logs := f.loggingWorker(t)
		worker.deliverWorkspaceFiles(ctx)
		require.Equal(t, "recorded by another replica", f.fileState(ctx, t, chat.ID, file.ID).WorkspaceDeliveryError.String)
		require.Empty(t, warnings(logs))
	})

	t.Run("RebindDuringUploadDeliversToTheNewWorkspace", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		other, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		var rebound database.Chat
		f.agent.afterPut = func() {
			f.agent.mu.Lock()
			f.agent.afterPut = nil
			f.agent.mu.Unlock()
			var err error
			rebound, err = f.db.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{
				ID:          chat.ID,
				WorkspaceID: other.WorkspaceID,
			})
			require.NoError(t, err)
		}

		f.worker(t).deliverWorkspaceFiles(ctx)
		f.requireDelivered(ctx, t, rebound, file.ID)
		require.Len(t, f.agent.uploadRequests(), 2)
	})

	t.Run("AttemptDeadlineCoversTheUpload", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		// The file takes four seconds at the slowest upload rate a lease
		// leaves time for.
		f.workspaceFile(ctx, t, chat.ID, strings.Repeat("x", 4*workspaceFileDeliveryMinUploadBytesPerSecond))

		// The test context's own deadline is shorter than an attempt's.
		deliverCtx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		f.worker(t).deliverWorkspaceFiles(deliverCtx)
		timeouts := f.agent.uploadTimeouts()
		require.Len(t, timeouts, 1)
		want := workspaceFileDeliveryLease - time.Minute + 4*time.Second
		require.Greater(t, timeouts[0], want-time.Second)
		require.LessOrEqual(t, timeouts[0], want)
	})

	t.Run("UploadKeepsTheWorkspaceAlive", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		build, err := f.db.GetLatestWorkspaceBuildByWorkspaceID(ctx, chat.WorkspaceID.UUID)
		require.NoError(t, err)
		// No chat turn heartbeats while the workspace nears its autostop
		// deadline.
		nearAutostop := func() {
			t.Helper()
			require.NoError(t, f.db.UpdateWorkspaceBuildDeadlineByID(ctx, database.UpdateWorkspaceBuildDeadlineByIDParams{
				ID:        build.ID,
				Deadline:  dbtime.Now().Add(time.Minute),
				UpdatedAt: dbtime.Now(),
			}))
		}
		requireBumped := func() {
			t.Helper()
			latest, err := f.db.GetLatestWorkspaceBuildByWorkspaceID(ctx, build.WorkspaceID)
			require.NoError(t, err)
			require.True(t, latest.Deadline.After(dbtime.Now().Add(30*time.Minute)), "deadline %s was not bumped", latest.Deadline)
		}
		nearAutostop()
		f.agent.stream = func(_ context.Context, body io.Reader) ([]byte, error) {
			requireBumped()
			nearAutostop()
			var head [1]byte
			if _, err := io.ReadFull(body, head[:]); err != nil {
				return nil, err
			}
			f.clock.Advance(workspaceFileDeliveryUsageInterval)
			rest, err := io.ReadAll(body)
			if err != nil {
				return nil, err
			}
			requireBumped()
			return append(head[:], rest...), nil
		}

		f.worker(t).deliverWorkspaceFiles(ctx)
		f.requireDelivered(ctx, t, chat, file.ID)
	})

	t.Run("StalledUploadRetries", func(t *testing.T) {
		t.Parallel()
		// waitForCancel stands in for an agent that stops taking the body
		// or never answers: only the attempt's cancellation ends it.
		waitForCancel := func(ctx, testCtx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-testCtx.Done():
				return xerrors.New("the upload was not canceled")
			}
		}
		for _, tc := range []struct {
			name   string
			stream func(ctx, testCtx context.Context, clock *quartz.Mock, body io.Reader) ([]byte, error)
		}{
			{name: "BodyStalls", stream: func(ctx, testCtx context.Context, clock *quartz.Mock, body io.Reader) ([]byte, error) {
				if _, err := io.ReadFull(body, make([]byte, 1)); err != nil {
					return nil, err
				}
				clock.Advance(workspaceFileDeliveryStallTimeout).MustWait(testCtx)
				return nil, waitForCancel(ctx, testCtx)
			}},
			{name: "ResponseHangs", stream: func(ctx, testCtx context.Context, clock *quartz.Mock, body io.Reader) ([]byte, error) {
				if _, err := io.ReadAll(body); err != nil {
					return nil, err
				}
				clock.Advance(workspaceFileDeliveryStallTimeout).MustWait(testCtx)
				return nil, waitForCancel(ctx, testCtx)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newWorkspaceFileDeliveryFixture(t)
				ctx := testutil.Context(t, testutil.WaitLong)
				chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
				file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
				f.agent.stream = func(uploadCtx context.Context, body io.Reader) ([]byte, error) {
					return tc.stream(uploadCtx, ctx, f.clock, body)
				}

				f.worker(t).deliverWorkspaceFiles(ctx)
				state := f.fileState(ctx, t, chat.ID, file.ID)
				require.False(t, state.WorkspaceDeliveredAt.Valid)
				require.Equal(t, int32(1), state.WorkspaceDeliveryAttempts)
				require.Equal(t, "the workspace agent accepted no data for 1m0s", state.WorkspaceDeliveryError.String)
				require.WithinDuration(t, f.clock.Now().Add(workspaceFileDeliveryRetryDelay), state.WorkspaceDeliveryNextAttemptAt.Time, time.Millisecond)
			})
		}
	})

	t.Run("SlowUploadKeepsGoing", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
		// Each byte arrives just inside the stall timeout.
		f.agent.stream = func(_ context.Context, body io.Reader) ([]byte, error) {
			var data []byte
			for {
				f.clock.Advance(workspaceFileDeliveryStallTimeout - time.Second).MustWait(ctx)
				var b [1]byte
				n, err := body.Read(b[:])
				data = append(data, b[:n]...)
				if errors.Is(err, io.EOF) {
					return data, nil
				}
				if err != nil {
					return nil, err
				}
			}
		}

		f.worker(t).deliverWorkspaceFiles(ctx)
		f.requireDelivered(ctx, t, chat, file.ID)
	})

	t.Run("DisconnectedAgentRefunds", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			// away is how long before the first attempt the agent last
			// connected; zero means it never connected.
			away   time.Duration
			delays []time.Duration
		}{
			// The delay grows with the time the agent has been away, up
			// to a cap.
			{name: "Disconnected", away: 90 * time.Second, delays: []time.Duration{90 * time.Second, 3 * time.Minute, 6 * time.Minute, 10 * time.Minute, 10 * time.Minute}},
			{name: "RecentlyDisconnected", away: time.Second, delays: []time.Duration{time.Minute, time.Minute + time.Second}},
			{name: "NeverConnected", delays: []time.Duration{time.Minute, time.Minute, time.Minute}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newWorkspaceFileDeliveryFixture(t)
				ctx := testutil.Context(t, testutil.WaitLong)
				chat, agent := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
				connection := database.UpdateWorkspaceAgentConnectionByIDParams{ID: agent.ID, UpdatedAt: dbtime.Now()}
				if tc.away > 0 {
					lastConnected := f.clock.Now().Add(-tc.away)
					connection.FirstConnectedAt = sql.NullTime{Time: lastConnected, Valid: true}
					connection.LastConnectedAt = sql.NullTime{Time: lastConnected, Valid: true}
					connection.DisconnectedAt = sql.NullTime{Time: lastConnected.Add(time.Millisecond), Valid: true}
				}
				require.NoError(t, f.db.UpdateWorkspaceAgentConnectionByID(ctx, connection))
				file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")
				worker, logs := f.loggingWorker(t)

				for _, delay := range tc.delays {
					worker.deliverWorkspaceFiles(ctx)
					state := f.fileState(ctx, t, chat.ID, file.ID)
					require.Zero(t, state.WorkspaceDeliveryAttempts)
					require.Contains(t, state.WorkspaceDeliveryError.String, "not connected")
					require.WithinDuration(t, f.clock.Now().Add(delay), state.WorkspaceDeliveryNextAttemptAt.Time, time.Millisecond)
					f.clock.Advance(delay)
				}
				require.Empty(t, f.agent.uploadRequests())
				require.Empty(t, warnings(logs))
			})
		}
	})

	t.Run("CrashAfterAgentWriteRedeliversToSamePath", func(t *testing.T) {
		t.Parallel()
		f := newWorkspaceFileDeliveryFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
		file := f.workspaceFile(ctx, t, chat.ID, "zip bytes")

		// The replica dies after the agent stored the file and before it
		// records the delivery.
		crashCtx, crash := context.WithCancel(ctx)
		f.agent.afterPut = crash
		f.worker(t).deliverWorkspaceFiles(crashCtx)
		state := f.fileState(ctx, t, chat.ID, file.ID)
		require.False(t, state.WorkspaceDeliveredAt.Valid)
		require.False(t, state.WorkspaceDeliveryError.Valid)

		f.agent.mu.Lock()
		f.agent.afterPut = nil
		f.agent.mu.Unlock()
		// The lease also covers the upload time of the file's nine bytes.
		f.clock.Advance(workspaceFileDeliveryLease + time.Second)
		f.worker(t).deliverWorkspaceFiles(ctx)
		f.requireDelivered(ctx, t, chat, file.ID)

		requests := f.agent.uploadRequests()
		require.Len(t, requests, 2)
		for _, req := range requests {
			require.Equal(t, file.ID, req.FileID)
			require.Equal(t, "report.zip", req.Name)
		}
		require.Equal(t, map[uuid.UUID][]byte{file.ID: []byte("zip bytes")}, f.agent.storedFiles())
	})
}

func TestWorkspaceFileDeliveryTwoReplicas(t *testing.T) {
	t.Parallel()
	f := newWorkspaceFileDeliveryFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	chat, _ := f.workspaceChat(ctx, t, database.WorkspaceAgent{LifecycleState: database.WorkspaceAgentLifecycleStateReady})
	var files []database.InsertChatFileRow
	for range 3 {
		files = append(files, f.workspaceFile(ctx, t, chat.ID, "zip bytes"))
	}

	replicas := []*chatWorker{f.worker(t), f.worker(t)}
	var wg sync.WaitGroup
	for _, replica := range replicas {
		wg.Go(func() { replica.deliverWorkspaceFiles(ctx) })
	}
	wg.Wait()

	for _, file := range files {
		f.requireDelivered(ctx, t, chat, file.ID)
	}
	uploads := map[uuid.UUID]int{}
	for _, req := range f.agent.uploadRequests() {
		uploads[req.FileID]++
	}
	require.Len(t, uploads, len(files))
	for fileID, count := range uploads {
		require.Equal(t, 1, count, "file %s uploaded more than once", fileID)
	}
}

func TestClassifyWorkspaceFileUploadError(t *testing.T) {
	t.Parallel()
	const agentURL = "http://[fd7a:115c:a1e0::1]:4/api/v0/upload-chat-file?chat_id=c&file_id=f&name=report.zip"
	for _, tc := range []struct {
		name     string
		err      error
		mode     string
		stored   string
		inLogged string
	}{
		{
			name:     "ServerError",
			err:      agentStatusError(http.StatusBadGateway),
			mode:     database.ChatWorkspaceFileDeliveryFailureRetry,
			stored:   "the workspace agent failed the upload with status 502 (agent says no)",
			inLogged: "/api/v0/upload-chat-file",
		},
		{
			name:     "Transport",
			err:      xerrors.Errorf("upload chat file: %w", &url.Error{Op: "Post", URL: agentURL, Err: xerrors.New("connection reset by peer")}),
			mode:     database.ChatWorkspaceFileDeliveryFailureRetry,
			stored:   "upload to the workspace agent: connection reset by peer",
			inLogged: agentURL,
		},
		{
			name:     "Rejected",
			err:      agentStatusError(http.StatusForbidden),
			mode:     database.ChatWorkspaceFileDeliveryFailurePark,
			stored:   "the workspace agent rejected the upload with status 403 (agent says no)",
			inLogged: "/api/v0/upload-chat-file",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			failure := classifyWorkspaceFileUploadError(tc.err)
			require.Equal(t, tc.mode, failure.mode)
			require.Equal(t, tc.stored, failure.err.Error())
			require.Contains(t, failure.logError().Error(), tc.inLogged)
		})
	}
}
