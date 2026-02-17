package chatd_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"

	"go.jetify.com/ai/api"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

const readFileToolName = "read_file"
const writeFileToolName = "write_file"
const createWorkspaceToolName = "create_workspace"
const executeToolName = "execute"

type fakeModel struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeModel) ProviderName() string {
	return "fake"
}

func (f *fakeModel) ModelID() string {
	return "fake"
}

func (f *fakeModel) SupportedUrls() []api.SupportedURL {
	return nil
}

func (f *fakeModel) Generate(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.Response, error) {
	return &api.Response{Content: []api.ContentBlock{&api.TextBlock{Text: "fallback"}}}, nil
}

func (f *fakeModel) Stream(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.StreamResponse, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.mu.Unlock()

	var events []api.StreamEvent
	if call == 1 {
		args, err := json.Marshal(map[string]string{"path": "/hello.txt"})
		if err != nil {
			return nil, err
		}
		events = []api.StreamEvent{
			&api.ToolCallEvent{
				ToolCallID: "call-1",
				ToolName:   readFileToolName,
				Args:       args,
			},
			&api.FinishEvent{Usage: api.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}},
		}
	} else {
		events = []api.StreamEvent{
			&api.TextDeltaEvent{TextDelta: "done"},
			&api.FinishEvent{Usage: api.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}},
		}
	}

	return &api.StreamResponse{
		Stream: iter.Seq[api.StreamEvent](func(yield func(api.StreamEvent) bool) {
			for _, event := range events {
				if !yield(event) {
					return
				}
			}
		}),
	}, nil
}

type testAgentConnector struct {
	client *workspacesdk.Client
	logger slog.Logger
}

func (c testAgentConnector) AgentConn(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
	conn, err := c.client.DialAgent(ctx, agentID, &workspacesdk.DialAgentOptions{Logger: c.logger})
	if err != nil {
		return nil, nil, err
	}
	return conn, func() {
		_ = conn.Close()
	}, nil
}

func parseAssistantContent(raw pqtype.NullRawMessage) ([]api.ContentBlock, error) {
	payload := struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content,omitempty"`
	}{
		Role:    string(api.MessageRoleAssistant),
		Content: raw.RawMessage,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var message api.AssistantMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return nil, err
	}
	return message.Content, nil
}

type createWorkspaceToolModel struct {
	mu          sync.Mutex
	calls       int
	firstPrompt []api.Message
	args        json.RawMessage
}

func (m *createWorkspaceToolModel) ProviderName() string {
	return "fake"
}

func (m *createWorkspaceToolModel) ModelID() string {
	return "fake"
}

func (m *createWorkspaceToolModel) SupportedUrls() []api.SupportedURL {
	return nil
}

func (m *createWorkspaceToolModel) Generate(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.Response, error) {
	return &api.Response{Content: []api.ContentBlock{&api.TextBlock{Text: "fallback"}}}, nil
}

func (m *createWorkspaceToolModel) Stream(_ context.Context, prompt []api.Message, _ api.CallOptions) (*api.StreamResponse, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	if call == 1 {
		m.firstPrompt = append([]api.Message(nil), prompt...)
	}
	m.mu.Unlock()

	var events []api.StreamEvent
	if call == 1 {
		args := m.args
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		events = []api.StreamEvent{
			&api.ToolCallEvent{
				ToolCallID: "ws-call-1",
				ToolName:   createWorkspaceToolName,
				Args:       args,
			},
			&api.FinishEvent{Usage: api.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}},
		}
	} else {
		events = []api.StreamEvent{
			&api.TextDeltaEvent{TextDelta: "done"},
			&api.FinishEvent{Usage: api.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}},
		}
	}

	return &api.StreamResponse{
		Stream: iter.Seq[api.StreamEvent](func(yield func(api.StreamEvent) bool) {
			for _, event := range events {
				if !yield(event) {
					return
				}
			}
		}),
	}, nil
}

func (m *createWorkspaceToolModel) FirstPrompt() []api.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]api.Message(nil), m.firstPrompt...)
}

type executeTimeoutToolModel struct {
	mu    sync.Mutex
	calls int
}

type createWorkspaceExecuteModel struct {
	mu    sync.Mutex
	calls int
}

func (*createWorkspaceExecuteModel) ProviderName() string {
	return "fake"
}

func (*createWorkspaceExecuteModel) ModelID() string {
	return "fake"
}

func (*createWorkspaceExecuteModel) SupportedUrls() []api.SupportedURL {
	return nil
}

func (*createWorkspaceExecuteModel) Generate(
	_ context.Context,
	_ []api.Message,
	_ api.CallOptions,
) (*api.Response, error) {
	return &api.Response{Content: []api.ContentBlock{&api.TextBlock{Text: "fallback"}}}, nil
}

func (m *createWorkspaceExecuteModel) Stream(
	_ context.Context,
	_ []api.Message,
	_ api.CallOptions,
) (*api.StreamResponse, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()

	var events []api.StreamEvent
	switch call {
	case 1:
		args, err := json.Marshal(map[string]any{"prompt": "workspace for execute test"})
		if err != nil {
			return nil, err
		}
		events = []api.StreamEvent{
			&api.ToolCallEvent{
				ToolCallID: "create-workspace-call-1",
				ToolName:   createWorkspaceToolName,
				Args:       args,
			},
			&api.FinishEvent{Usage: api.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}},
		}
	case 2:
		args, err := json.Marshal(map[string]any{"command": "echo test"})
		if err != nil {
			return nil, err
		}
		events = []api.StreamEvent{
			&api.ToolCallEvent{
				ToolCallID: "execute-call-1",
				ToolName:   executeToolName,
				Args:       args,
			},
			&api.FinishEvent{Usage: api.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}},
		}
	default:
		events = []api.StreamEvent{
			&api.TextDeltaEvent{TextDelta: "done"},
			&api.FinishEvent{Usage: api.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}},
		}
	}

	return &api.StreamResponse{
		Stream: iter.Seq[api.StreamEvent](func(yield func(api.StreamEvent) bool) {
			for _, event := range events {
				if !yield(event) {
					return
				}
			}
		}),
	}, nil
}

func (m *createWorkspaceExecuteModel) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (*executeTimeoutToolModel) ProviderName() string {
	return "fake"
}

func (*executeTimeoutToolModel) ModelID() string {
	return "fake"
}

func (*executeTimeoutToolModel) SupportedUrls() []api.SupportedURL {
	return nil
}

func (*executeTimeoutToolModel) Generate(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.Response, error) {
	return &api.Response{Content: []api.ContentBlock{&api.TextBlock{Text: "fallback"}}}, nil
}

func (m *executeTimeoutToolModel) Stream(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.StreamResponse, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()

	var events []api.StreamEvent
	if call == 1 {
		args, err := json.Marshal(map[string]any{
			"command":         "sh -c 'while :; do :; done'",
			"timeout_seconds": 1,
		})
		if err != nil {
			return nil, err
		}
		events = []api.StreamEvent{
			&api.ToolCallEvent{
				ToolCallID: fmt.Sprintf("exec-timeout-%d", call),
				ToolName:   executeToolName,
				Args:       args,
			},
			&api.FinishEvent{Usage: api.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}},
		}
	} else {
		events = []api.StreamEvent{
			&api.TextDeltaEvent{TextDelta: "continued after tool error"},
			&api.FinishEvent{Usage: api.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}},
		}
	}

	return &api.StreamResponse{
		Stream: iter.Seq[api.StreamEvent](func(yield func(api.StreamEvent) bool) {
			for _, event := range events {
				if !yield(event) {
					return
				}
			}
		}),
	}, nil
}

func (m *executeTimeoutToolModel) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type singleToolThenTextModel struct {
	mu       sync.Mutex
	calls    int
	toolName string
	args     json.RawMessage
	final    string
}

func (*singleToolThenTextModel) ProviderName() string {
	return "fake"
}

func (*singleToolThenTextModel) ModelID() string {
	return "fake"
}

func (*singleToolThenTextModel) SupportedUrls() []api.SupportedURL {
	return nil
}

func (*singleToolThenTextModel) Generate(
	_ context.Context,
	_ []api.Message,
	_ api.CallOptions,
) (*api.Response, error) {
	return &api.Response{Content: []api.ContentBlock{&api.TextBlock{Text: "fallback"}}}, nil
}

func (m *singleToolThenTextModel) Stream(
	_ context.Context,
	_ []api.Message,
	_ api.CallOptions,
) (*api.StreamResponse, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()

	var events []api.StreamEvent
	if call == 1 {
		args := m.args
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		events = []api.StreamEvent{
			&api.ToolCallEvent{
				ToolCallID: fmt.Sprintf("%s-call-1", m.toolName),
				ToolName:   m.toolName,
				Args:       args,
			},
			&api.FinishEvent{Usage: api.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}},
		}
	} else {
		final := m.final
		if final == "" {
			final = "done"
		}
		events = []api.StreamEvent{
			&api.TextDeltaEvent{TextDelta: final},
			&api.FinishEvent{Usage: api.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}},
		}
	}

	return &api.StreamResponse{
		Stream: iter.Seq[api.StreamEvent](func(yield func(api.StreamEvent) bool) {
			for _, event := range events {
				if !yield(event) {
					return
				}
			}
		}),
	}, nil
}

func (m *singleToolThenTextModel) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type panicStreamModel struct {
	panicValue any
}

func (*panicStreamModel) ProviderName() string {
	return "fake"
}

func (*panicStreamModel) ModelID() string {
	return "fake"
}

func (*panicStreamModel) SupportedUrls() []api.SupportedURL {
	return nil
}

func (*panicStreamModel) Generate(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.Response, error) {
	return &api.Response{Content: []api.ContentBlock{&api.TextBlock{Text: "fallback"}}}, nil
}

func (m *panicStreamModel) Stream(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.StreamResponse, error) {
	panic(m.panicValue)
}

type workspaceCreatorFunc func(context.Context, chatd.CreateWorkspaceToolRequest) (chatd.CreateWorkspaceToolResult, error)

func (f workspaceCreatorFunc) CreateWorkspace(
	ctx context.Context,
	req chatd.CreateWorkspaceToolRequest,
) (chatd.CreateWorkspaceToolResult, error) {
	return f(ctx, req)
}

func insertChatWithUserMessage(
	t *testing.T,
	db database.Store,
	dbCtx context.Context,
	ownerID uuid.UUID,
	title string,
	message string,
) database.Chat {
	t.Helper()

	chat, err := db.InsertChat(dbCtx, database.InsertChatParams{
		OwnerID:     ownerID,
		Title:       title,
		ModelConfig: json.RawMessage(`{"model":"fake"}`),
	})
	require.NoError(t, err)

	content, err := json.Marshal(api.ContentFromText(message))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(api.MessageRoleUser),
		Content: pqtype.NullRawMessage{RawMessage: content, Valid: true},
		Hidden:  false,
	})
	require.NoError(t, err)

	return chat
}

func TestRunChatLoop(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	zipBytes := make([]byte, 22)
	zipBytes[0] = 80
	zipBytes[1] = 75
	zipBytes[2] = 0o5
	zipBytes[3] = 0o6
	uploadRes, err := client.Upload(ctx, codersdk.ContentTypeZip, bytes.NewReader(zipBytes))
	require.NoError(t, err)

	tv := dbfake.TemplateVersion(t, db).
		FileID(uploadRes.ID).
		Seed(database.TemplateVersion{
			OrganizationID: user.OrganizationID,
			CreatedBy:      user.UserID,
		}).
		Do()
	wbr := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
		TemplateID:     tv.Template.ID,
	}).Resource().WithAgent().Do()
	ws, err := client.Workspace(ctx, wbr.Workspace.ID)
	require.NoError(t, err)

	agentID := ws.LatestBuild.Resources[0].Agents[0].ID

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "hello.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("hello"), 0o600))
	_ = agenttest.New(t, client.URL, wbr.AgentToken, func(o *agent.Options) {
		o.Filesystem = afero.NewBasePathFs(afero.NewOsFs(), tempDir)
	})
	coderdtest.NewWorkspaceAgentWaiter(t, client, wbr.Workspace.ID).Wait()

	chat, err := db.InsertChat(dbCtx, database.InsertChatParams{
		OwnerID: user.UserID,
		WorkspaceID: uuid.NullUUID{
			UUID:  ws.ID,
			Valid: true,
		},
		WorkspaceAgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
		Title:       "Test",
		ModelConfig: json.RawMessage(`{"model":"fake"}`),
	})
	require.NoError(t, err)

	content, err := json.Marshal(api.ContentFromText("read file"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(api.MessageRoleUser),
		Content: pqtype.NullRawMessage{RawMessage: content, Valid: true},
		Hidden:  false,
	})
	require.NoError(t, err)

	logger := testutil.Logger(t).Named("chatd")
	model := &fakeModel{}
	processor := chatd.NewProcessor(
		logger,
		db,
		chatd.WithAgentConnector(testAgentConnector{client: workspacesdk.New(client), logger: logger}),
		chatd.WithModelResolver(func(_ database.Chat) (api.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithPollInterval(50*time.Millisecond),
	)
	defer processor.Close()

	_, err = db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		stored, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
		if err != nil {
			return false
		}
		return len(stored) == 4
	}, testutil.WaitLong, 50*time.Millisecond)

	stored, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.Len(t, stored, 4)

	toolMessage := stored[2]
	require.Equal(t, string(api.MessageRoleTool), toolMessage.Role)
	var toolResults []api.ToolResultBlock
	require.NoError(t, json.Unmarshal(toolMessage.Content.RawMessage, &toolResults))
	require.Len(t, toolResults, 1)
	resultMap, ok := toolResults[0].Result.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "hello", resultMap["content"])

	finalMessage := stored[3]
	finalContent, err := parseAssistantContent(finalMessage.Content)
	require.NoError(t, err)
	require.Len(t, finalContent, 1)
	textBlock, ok := finalContent[0].(*api.TextBlock)
	require.True(t, ok)
	require.Equal(t, "done", textBlock.Text)
}

func TestRunChatLoop_ExecuteTimeoutContinues(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	zipBytes := make([]byte, 22)
	zipBytes[0] = 80
	zipBytes[1] = 75
	zipBytes[2] = 0o5
	zipBytes[3] = 0o6
	uploadRes, err := client.Upload(ctx, codersdk.ContentTypeZip, bytes.NewReader(zipBytes))
	require.NoError(t, err)

	tv := dbfake.TemplateVersion(t, db).
		FileID(uploadRes.ID).
		Seed(database.TemplateVersion{
			OrganizationID: user.OrganizationID,
			CreatedBy:      user.UserID,
		}).
		Do()
	wbr := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
		TemplateID:     tv.Template.ID,
	}).Resource().WithAgent().Do()
	ws, err := client.Workspace(ctx, wbr.Workspace.ID)
	require.NoError(t, err)

	agentID := ws.LatestBuild.Resources[0].Agents[0].ID
	_ = agenttest.New(t, client.URL, wbr.AgentToken)
	coderdtest.NewWorkspaceAgentWaiter(t, client, wbr.Workspace.ID).Wait()

	chat, err := db.InsertChat(dbCtx, database.InsertChatParams{
		OwnerID: user.UserID,
		WorkspaceID: uuid.NullUUID{
			UUID:  ws.ID,
			Valid: true,
		},
		WorkspaceAgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
		Title:       "Execute timeout",
		ModelConfig: json.RawMessage(`{"model":"fake"}`),
	})
	require.NoError(t, err)

	content, err := json.Marshal(api.ContentFromText("run command"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(api.MessageRoleUser),
		Content: pqtype.NullRawMessage{RawMessage: content, Valid: true},
		Hidden:  false,
	})
	require.NoError(t, err)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named("chatd")
	manager := chatd.NewStreamManager(logger)
	model := &executeTimeoutToolModel{}
	processor := chatd.NewProcessor(
		logger.Named("processor"),
		db,
		chatd.WithAgentConnector(testAgentConnector{client: workspacesdk.New(client), logger: logger}),
		chatd.WithModelResolver(func(_ database.Chat) (api.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithStreamManager(manager),
		chatd.WithPollInterval(50*time.Millisecond),
	)
	defer processor.Close()

	_, events, cancel := manager.Subscribe(chat.ID)
	defer cancel()

	_, err = db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	var (
		streamErr string
	)
	require.Eventually(t, func() bool {
		select {
		case event, ok := <-events:
			if !ok {
				return false
			}
			if event.Type == codersdk.ChatStreamEventTypeError && event.Error != nil {
				streamErr = strings.TrimSpace(event.Error.Message)
			}
		default:
		}
		storedChat, err := db.GetChatByID(dbCtx, chat.ID)
		if err != nil {
			return false
		}
		return streamErr != "" || storedChat.Status == database.ChatStatusWaiting || storedChat.Status == database.ChatStatusCompleted
	}, testutil.WaitLong, 25*time.Millisecond)

	require.Empty(t, streamErr)

	storedChat, err := db.GetChatByID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.True(t, storedChat.Status == database.ChatStatusWaiting || storedChat.Status == database.ChatStatusCompleted)

	require.Eventually(t, func() bool {
		messages, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
		return err == nil && len(messages) == 4
	}, testutil.WaitLong, 50*time.Millisecond)

	messages, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.Len(t, messages, 4)

	var toolResults []api.ToolResultBlock
	require.NoError(t, json.Unmarshal(messages[2].Content.RawMessage, &toolResults))
	require.Len(t, toolResults, 1)
	require.Equal(t, executeToolName, toolResults[0].ToolName)
	require.True(t, toolResults[0].IsError)

	resultMap, ok := toolResults[0].Result.(map[string]any)
	require.True(t, ok)
	errorText, ok := resultMap["error"].(string)
	require.True(t, ok)
	require.Contains(t, errorText, "context deadline exceeded")

	finalContent, err := parseAssistantContent(messages[3].Content)
	require.NoError(t, err)
	require.Len(t, finalContent, 1)
	textBlock, ok := finalContent[0].(*api.TextBlock)
	require.True(t, ok)
	require.Equal(t, "continued after tool error", textBlock.Text)

	require.Equal(t, 2, model.CallCount())
}

func TestRunChatLoop_ReadWriteToolErrorsContinue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		toolName string
	}{
		{
			name:     "ReadFile",
			toolName: readFileToolName,
		},
		{
			name:     "WriteFile",
			toolName: writeFileToolName,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitLong)
			client, db := coderdtest.NewWithDatabase(t, nil)
			user := coderdtest.CreateFirstUser(t, client)
			dbCtx := dbauthz.AsSystemRestricted(ctx)

			chat := insertChatWithUserMessage(t, db, dbCtx, user.UserID, tc.name, "run tool")
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named("chatd")
			manager := chatd.NewStreamManager(logger)
			model := &singleToolThenTextModel{
				toolName: tc.toolName,
				final:    "done after tool error",
			}

			processor := chatd.NewProcessor(
				logger.Named("processor"),
				db,
				chatd.WithStreamManager(manager),
				chatd.WithModelResolver(func(_ database.Chat) (api.LanguageModel, error) {
					return model, nil
				}),
				chatd.WithPollInterval(50*time.Millisecond),
			)
			defer processor.Close()

			_, events, cancel := manager.Subscribe(chat.ID)
			defer cancel()

			_, err := db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
				ID:        chat.ID,
				Status:    database.ChatStatusPending,
				WorkerID:  uuid.NullUUID{},
				StartedAt: sql.NullTime{},
			})
			require.NoError(t, err)

			var streamErr string
			require.Eventually(t, func() bool {
				select {
				case event, ok := <-events:
					if !ok {
						return false
					}
					if event.Type == codersdk.ChatStreamEventTypeError && event.Error != nil {
						streamErr = strings.TrimSpace(event.Error.Message)
					}
				default:
				}

				storedChat, err := db.GetChatByID(dbCtx, chat.ID)
				if err != nil {
					return false
				}
				return streamErr != "" || storedChat.Status == database.ChatStatusWaiting || storedChat.Status == database.ChatStatusCompleted
			}, testutil.WaitLong, 25*time.Millisecond)
			require.Empty(t, streamErr)

			storedChat, err := db.GetChatByID(dbCtx, chat.ID)
			require.NoError(t, err)
			require.True(t, storedChat.Status == database.ChatStatusWaiting || storedChat.Status == database.ChatStatusCompleted)

			require.Eventually(t, func() bool {
				messages, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
				return err == nil && len(messages) == 4
			}, testutil.WaitLong, 50*time.Millisecond)

			messages, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
			require.NoError(t, err)
			require.Len(t, messages, 4)

			var toolResults []api.ToolResultBlock
			require.NoError(t, json.Unmarshal(messages[2].Content.RawMessage, &toolResults))
			require.Len(t, toolResults, 1)
			require.Equal(t, tc.toolName, toolResults[0].ToolName)
			require.True(t, toolResults[0].IsError)
			resultMap, ok := toolResults[0].Result.(map[string]any)
			require.True(t, ok)
			errorText, ok := resultMap["error"].(string)
			require.True(t, ok)
			require.NotEmpty(t, strings.TrimSpace(errorText))

			finalContent, err := parseAssistantContent(messages[3].Content)
			require.NoError(t, err)
			require.Len(t, finalContent, 1)
			textBlock, ok := finalContent[0].(*api.TextBlock)
			require.True(t, ok)
			require.Equal(t, "done after tool error", textBlock.Text)

			require.Equal(t, 2, model.CallCount())
		})
	}
}

func TestCreateWorkspaceTool_NilCreator(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	chat, err := db.InsertChat(dbCtx, database.InsertChatParams{
		OwnerID:     user.UserID,
		Title:       "Nil creator",
		ModelConfig: json.RawMessage(`{"model":"fake"}`),
	})
	require.NoError(t, err)

	content, err := json.Marshal(api.ContentFromText("create a workspace"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(api.MessageRoleUser),
		Content: pqtype.NullRawMessage{RawMessage: content, Valid: true},
		Hidden:  false,
	})
	require.NoError(t, err)

	model := &createWorkspaceToolModel{
		args: json.RawMessage(`{"prompt":"python api"}`),
	}
	processor := chatd.NewProcessor(
		testutil.Logger(t).Named("chatd"),
		db,
		chatd.WithModelResolver(func(_ database.Chat) (api.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithPollInterval(50*time.Millisecond),
	)
	defer processor.Close()

	_, err = db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		messages, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
		return err == nil && len(messages) == 4
	}, testutil.WaitLong, 50*time.Millisecond)

	messages, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.Len(t, messages, 4)

	var toolResults []api.ToolResultBlock
	require.NoError(t, json.Unmarshal(messages[2].Content.RawMessage, &toolResults))
	require.Len(t, toolResults, 1)
	require.True(t, toolResults[0].IsError)
	resultMap, ok := toolResults[0].Result.(map[string]any)
	require.True(t, ok)
	require.Contains(t, resultMap["error"], "workspace creator is not configured")

	storedChat, err := db.GetChatByID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.False(t, storedChat.WorkspaceID.Valid)
	require.False(t, storedChat.WorkspaceAgentID.Valid)

	prompt := model.FirstPrompt()
	require.NotEmpty(t, prompt)
	systemMessage, ok := prompt[0].(*api.SystemMessage)
	require.True(t, ok)
	require.True(t, strings.Contains(strings.ToLower(systemMessage.Content), "create_workspace"))
}

func TestCreateWorkspaceTool_StreamSnapshotIncludesBuildLogDeltas(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	chat, err := db.InsertChat(dbCtx, database.InsertChatParams{
		OwnerID:     user.UserID,
		Title:       "Stream build logs",
		ModelConfig: json.RawMessage(`{"model":"fake"}`),
	})
	require.NoError(t, err)

	content, err := json.Marshal(api.ContentFromText("create a workspace"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(api.MessageRoleUser),
		Content: pqtype.NullRawMessage{RawMessage: content, Valid: true},
		Hidden:  false,
	})
	require.NoError(t, err)

	creatorStarted := make(chan struct{})
	releaseCreator := make(chan struct{})
	creator := workspaceCreatorFunc(func(_ context.Context, req chatd.CreateWorkspaceToolRequest) (chatd.CreateWorkspaceToolResult, error) {
		if req.BuildLogHandler != nil {
			req.BuildLogHandler(chatd.CreateWorkspaceBuildLog{
				Stage:  "build",
				Level:  "info",
				Output: "cloning repository",
			})
		}
		close(creatorStarted)
		<-releaseCreator
		return chatd.CreateWorkspaceToolResult{
			Created: false,
			Reason:  "stopped for test",
		}, nil
	})

	model := &createWorkspaceToolModel{
		args: json.RawMessage(`{"prompt":"workspace for tests"}`),
	}
	manager := chatd.NewStreamManager(testutil.Logger(t))
	processor := chatd.NewProcessor(
		testutil.Logger(t).Named("chatd"),
		db,
		chatd.WithWorkspaceCreator(creator),
		chatd.WithModelResolver(func(_ database.Chat) (api.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithStreamManager(manager),
		chatd.WithPollInterval(50*time.Millisecond),
	)
	defer processor.Close()

	_, err = db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	select {
	case <-creatorStarted:
	case <-ctx.Done():
		t.Fatal("timed out waiting for create_workspace tool execution")
	}

	snapshot, _, cancel := manager.Subscribe(chat.ID)
	defer cancel()

	require.NotEmpty(t, snapshot)
	hasToolDelta := false
	for _, event := range snapshot {
		if event.Type != codersdk.ChatStreamEventTypeMessagePart || event.MessagePart == nil {
			continue
		}
		if event.MessagePart.Role != string(api.MessageRoleTool) {
			continue
		}
		if event.MessagePart.Part.Type != codersdk.ChatMessagePartTypeToolResult {
			continue
		}
		if event.MessagePart.Part.ResultDelta == "" {
			continue
		}
		hasToolDelta = true
		break
	}
	require.True(t, hasToolDelta)

	close(releaseCreator)

	require.Eventually(t, func() bool {
		updatedChat, err := db.GetChatByID(dbCtx, chat.ID)
		if err != nil {
			return false
		}
		return updatedChat.Status == database.ChatStatusWaiting
	}, testutil.WaitLong, 50*time.Millisecond)
}

func TestRunChatLoop_MissingWorkspaceRecovery(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	wbr := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
		Deleted:        true,
	}).Resource().WithAgent().Do()
	require.NotEmpty(t, wbr.Agents)

	chat, err := db.InsertChat(dbCtx, database.InsertChatParams{
		OwnerID: user.UserID,
		WorkspaceID: uuid.NullUUID{
			UUID:  wbr.Workspace.ID,
			Valid: true,
		},
		WorkspaceAgentID: uuid.NullUUID{
			UUID:  wbr.Agents[0].ID,
			Valid: true,
		},
		Title:       "Missing workspace recovery",
		ModelConfig: json.RawMessage(`{"model":"fake"}`),
	})
	require.NoError(t, err)

	content, err := json.Marshal(api.ContentFromText("create workspace"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(api.MessageRoleUser),
		Content: pqtype.NullRawMessage{RawMessage: content, Valid: true},
		Hidden:  false,
	})
	require.NoError(t, err)

	model := &createWorkspaceToolModel{
		args: json.RawMessage(`{"prompt":"go backend"}`),
	}
	processor := chatd.NewProcessor(
		testutil.Logger(t).Named("chatd"),
		db,
		chatd.WithModelResolver(func(_ database.Chat) (api.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithPollInterval(50*time.Millisecond),
	)
	defer processor.Close()

	_, err = db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		messages, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
		return err == nil && len(messages) == 4
	}, testutil.WaitLong, 50*time.Millisecond)

	storedChat, err := db.GetChatByID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.False(t, storedChat.WorkspaceID.Valid)
	require.False(t, storedChat.WorkspaceAgentID.Valid)

	prompt := model.FirstPrompt()
	require.NotEmpty(t, prompt)
	systemMessage, ok := prompt[0].(*api.SystemMessage)
	require.True(t, ok)
	require.True(t, strings.Contains(strings.ToLower(systemMessage.Content), "create_workspace"))
}

func TestCreateWorkspaceTool_PersistsWorkspaceIDs(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	chat, err := db.InsertChat(dbCtx, database.InsertChatParams{
		OwnerID:     user.UserID,
		Title:       "Persist workspace IDs",
		ModelConfig: json.RawMessage(`{"model":"fake"}`),
	})
	require.NoError(t, err)

	content, err := json.Marshal(api.ContentFromText("create workspace"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(api.MessageRoleUser),
		Content: pqtype.NullRawMessage{RawMessage: content, Valid: true},
		Hidden:  false,
	})
	require.NoError(t, err)

	wbr := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).Resource().WithAgent().Do()
	require.NotEmpty(t, wbr.Agents)
	workspaceID := wbr.Workspace.ID
	workspaceAgentID := wbr.Agents[0].ID
	creator := workspaceCreatorFunc(func(context.Context, chatd.CreateWorkspaceToolRequest) (chatd.CreateWorkspaceToolResult, error) {
		return chatd.CreateWorkspaceToolResult{
			Created:          true,
			WorkspaceID:      workspaceID,
			WorkspaceAgentID: workspaceAgentID,
			WorkspaceName:    "me/ws",
			WorkspaceURL:     "https://example.test/@me/ws",
		}, nil
	})

	model := &createWorkspaceToolModel{
		args: json.RawMessage(`{"prompt":"go backend"}`),
	}
	processor := chatd.NewProcessor(
		testutil.Logger(t).Named("chatd"),
		db,
		chatd.WithWorkspaceCreator(creator),
		chatd.WithModelResolver(func(_ database.Chat) (api.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithPollInterval(50*time.Millisecond),
	)
	defer processor.Close()

	_, err = db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		storedChat, err := db.GetChatByID(dbCtx, chat.ID)
		if err != nil {
			return false
		}
		return storedChat.WorkspaceID.Valid && storedChat.WorkspaceID.UUID == workspaceID &&
			storedChat.WorkspaceAgentID.Valid && storedChat.WorkspaceAgentID.UUID == workspaceAgentID
	}, testutil.WaitLong, 50*time.Millisecond)

	storedChat, err := db.GetChatByID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.True(t, storedChat.WorkspaceID.Valid)
	require.Equal(t, workspaceID, storedChat.WorkspaceID.UUID)
	require.True(t, storedChat.WorkspaceAgentID.Valid)
	require.Equal(t, workspaceAgentID, storedChat.WorkspaceAgentID.UUID)
}

func TestRunChatLoop_CreateWorkspaceThenExecute_NoHang(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	chat, err := db.InsertChat(dbCtx, database.InsertChatParams{
		OwnerID:     user.UserID,
		Title:       "Create workspace then execute",
		ModelConfig: json.RawMessage(`{"model":"fake"}`),
	})
	require.NoError(t, err)

	content, err := json.Marshal(api.ContentFromText("create workspace then run echo test"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(api.MessageRoleUser),
		Content: pqtype.NullRawMessage{RawMessage: content, Valid: true},
		Hidden:  false,
	})
	require.NoError(t, err)

	var (
		creatorMu               sync.Mutex
		creatorCalls            int
		createdWorkspaceID      uuid.UUID
		createdWorkspaceAgentID uuid.UUID
	)

	creator := workspaceCreatorFunc(func(context.Context, chatd.CreateWorkspaceToolRequest) (chatd.CreateWorkspaceToolResult, error) {
		wbr := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).Resource().WithAgent().Do()
		if len(wbr.Agents) == 0 {
			return chatd.CreateWorkspaceToolResult{}, xerrors.New("workspace build did not create agents")
		}

		_ = agenttest.New(t, client.URL, wbr.AgentToken)
		coderdtest.NewWorkspaceAgentWaiter(t, client, wbr.Workspace.ID).Wait()

		creatorMu.Lock()
		creatorCalls++
		createdWorkspaceID = wbr.Workspace.ID
		createdWorkspaceAgentID = wbr.Agents[0].ID
		creatorMu.Unlock()

		return chatd.CreateWorkspaceToolResult{
			Created:          true,
			WorkspaceID:      wbr.Workspace.ID,
			WorkspaceAgentID: wbr.Agents[0].ID,
			WorkspaceName:    "me/ws",
			WorkspaceURL:     "https://example.test/@me/ws",
		}, nil
	})

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named("chatd")
	manager := chatd.NewStreamManager(logger)
	model := &createWorkspaceExecuteModel{}
	processor := chatd.NewProcessor(
		logger.Named("processor"),
		db,
		chatd.WithWorkspaceCreator(creator),
		chatd.WithAgentConnector(testAgentConnector{client: workspacesdk.New(client), logger: logger}),
		chatd.WithModelResolver(func(_ database.Chat) (api.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithStreamManager(manager),
		chatd.WithPollInterval(50*time.Millisecond),
	)
	defer processor.Close()

	_, events, cancel := manager.Subscribe(chat.ID)
	defer cancel()

	_, err = db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	var finalStatus database.ChatStatus
	require.Eventually(t, func() bool {
		storedChat, err := db.GetChatByID(dbCtx, chat.ID)
		if err != nil {
			return false
		}

		finalStatus = storedChat.Status
		if finalStatus != database.ChatStatusWaiting && finalStatus != database.ChatStatusCompleted {
			return false
		}

		messages, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
		if err != nil {
			return false
		}

		for _, message := range messages {
			if message.Role != string(api.MessageRoleTool) {
				continue
			}
			var toolResults []api.ToolResultBlock
			if err := json.Unmarshal(message.Content.RawMessage, &toolResults); err != nil {
				return false
			}
			for _, toolResult := range toolResults {
				if toolResult.ToolName != executeToolName {
					continue
				}
				resultMap, ok := toolResult.Result.(map[string]any)
				if !ok {
					return false
				}
				exitCode, ok := resultMap["exit_code"].(float64)
				if !ok {
					return false
				}
				output, ok := resultMap["output"].(string)
				if !ok {
					return false
				}
				return exitCode == 0 && strings.Contains(output, "test")
			}
		}
		return false
	}, testutil.WaitLong, 50*time.Millisecond)

	require.True(t, finalStatus == database.ChatStatusWaiting || finalStatus == database.ChatStatusCompleted)
	creatorMu.Lock()
	creatorCallsValue := creatorCalls
	createdWorkspaceIDValue := createdWorkspaceID
	createdWorkspaceAgentIDValue := createdWorkspaceAgentID
	creatorMu.Unlock()

	require.Equal(t, 1, creatorCallsValue)
	require.NotEqual(t, uuid.Nil, createdWorkspaceIDValue)
	require.NotEqual(t, uuid.Nil, createdWorkspaceAgentIDValue)

	storedChat, err := db.GetChatByID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.True(t, storedChat.WorkspaceID.Valid)
	require.Equal(t, createdWorkspaceIDValue, storedChat.WorkspaceID.UUID)
	require.True(t, storedChat.WorkspaceAgentID.Valid)
	require.Equal(t, createdWorkspaceAgentIDValue, storedChat.WorkspaceAgentID.UUID)

	var (
		lastStreamStatus codersdk.ChatStatus
		sawRunning       bool
		sawTerminal      bool
	)
	require.Eventually(t, func() bool {
		select {
		case event, ok := <-events:
			if !ok || event.Type != codersdk.ChatStreamEventTypeStatus || event.Status == nil {
				return false
			}
			lastStreamStatus = event.Status.Status
			if lastStreamStatus == codersdk.ChatStatusRunning {
				sawRunning = true
			}
			if lastStreamStatus == codersdk.ChatStatusWaiting || lastStreamStatus == codersdk.ChatStatusCompleted {
				sawTerminal = true
			}
			return sawRunning && sawTerminal
		default:
			return false
		}
	}, testutil.WaitLong, 25*time.Millisecond)

	require.NotEqual(t, codersdk.ChatStatusRunning, lastStreamStatus)
	require.GreaterOrEqual(t, model.CallCount(), 3)
}

func TestRunChatLoop_PublishesStreamErrorOnProcessingFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	chat := insertChatWithUserMessage(t, db, dbCtx, user.UserID, "processing failure", "hello")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named("chatd")
	manager := chatd.NewStreamManager(logger)

	processor := chatd.NewProcessor(
		logger.Named("processor"),
		db,
		chatd.WithStreamManager(manager),
		chatd.WithModelResolver(func(_ database.Chat) (api.LanguageModel, error) {
			return nil, xerrors.New("model resolver failed")
		}),
		chatd.WithPollInterval(50*time.Millisecond),
	)
	defer processor.Close()

	_, events, cancel := manager.Subscribe(chat.ID)
	defer cancel()

	_, err := db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	var streamErr string
	require.Eventually(t, func() bool {
		select {
		case event, ok := <-events:
			if !ok || event.Type != codersdk.ChatStreamEventTypeError || event.Error == nil {
				return false
			}
			streamErr = strings.TrimSpace(event.Error.Message)
			return streamErr != ""
		default:
			return false
		}
	}, testutil.WaitLong, 25*time.Millisecond)
	require.Contains(t, streamErr, "resolve model: model resolver failed")

	require.Eventually(t, func() bool {
		storedChat, err := db.GetChatByID(dbCtx, chat.ID)
		return err == nil && storedChat.Status == database.ChatStatusError
	}, testutil.WaitLong, 50*time.Millisecond)
}

func TestRunChatLoop_PublishesStreamErrorOnPanic(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	chat := insertChatWithUserMessage(t, db, dbCtx, user.UserID, "panic failure", "hello")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named("chatd")
	manager := chatd.NewStreamManager(logger)
	model := &panicStreamModel{panicValue: "stream panic for test"}

	processor := chatd.NewProcessor(
		logger.Named("processor"),
		db,
		chatd.WithStreamManager(manager),
		chatd.WithModelResolver(func(_ database.Chat) (api.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithPollInterval(50*time.Millisecond),
	)
	defer processor.Close()

	_, events, cancel := manager.Subscribe(chat.ID)
	defer cancel()

	_, err := db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	var streamErr string
	require.Eventually(t, func() bool {
		select {
		case event, ok := <-events:
			if !ok || event.Type != codersdk.ChatStreamEventTypeError || event.Error == nil {
				return false
			}
			streamErr = strings.TrimSpace(event.Error.Message)
			return streamErr != ""
		default:
			return false
		}
	}, testutil.WaitLong, 25*time.Millisecond)
	require.Contains(t, streamErr, "chat processing panicked")
	require.Contains(t, streamErr, "stream panic for test")

	require.Eventually(t, func() bool {
		storedChat, err := db.GetChatByID(dbCtx, chat.ID)
		return err == nil && storedChat.Status == database.ChatStatusError
	}, testutil.WaitLong, 50*time.Millisecond)
}
