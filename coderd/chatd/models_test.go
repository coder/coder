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

	"charm.land/fantasy"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
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

const (
	readFileToolName        = "read_file"
	writeFileToolName       = "write_file"
	createWorkspaceToolName = "create_workspace"
	executeToolName         = "execute"
)

type fakeLanguageModelBase struct{}

func (fakeLanguageModelBase) Provider() string {
	return "fake"
}

func (fakeLanguageModelBase) Model() string {
	return "fake"
}

func (fakeLanguageModelBase) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, xerrors.New("not implemented")
}

func (fakeLanguageModelBase) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, xerrors.New("not implemented")
}

func fallbackResponse() *fantasy.Response {
	return &fantasy.Response{
		Content: []fantasy.Content{
			fantasy.TextContent{Text: "fallback"},
		},
	}
}

func streamFromParts(parts []fantasy.StreamPart) fantasy.StreamResponse {
	return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
		for _, part := range parts {
			if !yield(part) {
				return
			}
		}
	})
}

type fakeModel struct {
	fakeLanguageModelBase
	mu    sync.Mutex
	calls int
}

func (*fakeModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	return fallbackResponse(), nil
}

func (f *fakeModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.mu.Unlock()

	var parts []fantasy.StreamPart
	if call == 1 {
		args, err := json.Marshal(map[string]string{"path": "/hello.txt"})
		if err != nil {
			return nil, err
		}
		parts = []fantasy.StreamPart{
			{
				Type:          fantasy.StreamPartTypeToolCall,
				ID:            "call-1",
				ToolCallName:  readFileToolName,
				ToolCallInput: string(args),
			},
			{
				Type:         fantasy.StreamPartTypeFinish,
				Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				FinishReason: fantasy.FinishReasonToolCalls,
			},
		}
	} else {
		parts = []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
			{
				Type:         fantasy.StreamPartTypeFinish,
				Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				FinishReason: fantasy.FinishReasonToolCalls,
			},
		}
	}

	return streamFromParts(parts), nil
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

func parseAssistantContent(raw pqtype.NullRawMessage) ([]fantasy.Content, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return nil, nil
	}

	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(raw.RawMessage, &rawBlocks); err != nil {
		return nil, err
	}

	content := make([]fantasy.Content, 0, len(rawBlocks))
	for _, rawBlock := range rawBlocks {
		block, err := fantasy.UnmarshalContent(rawBlock)
		if err != nil {
			return nil, err
		}
		content = append(content, block)
	}

	return content, nil
}

func contentFromText(text string) []fantasy.Content {
	return []fantasy.Content{
		fantasy.TextContent{Text: text},
	}
}

type createWorkspaceToolModel struct {
	fakeLanguageModelBase
	mu          sync.Mutex
	calls       int
	firstPrompt []fantasy.Message
	args        json.RawMessage
}

func (*createWorkspaceToolModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	return fallbackResponse(), nil
}

func (m *createWorkspaceToolModel) Stream(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	m.mu.Lock()
	m.calls++
	callNum := m.calls
	if callNum == 1 {
		m.firstPrompt = append([]fantasy.Message(nil), call.Prompt...)
	}
	m.mu.Unlock()

	var parts []fantasy.StreamPart
	if callNum == 1 {
		args := m.args
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		parts = []fantasy.StreamPart{
			{
				Type:          fantasy.StreamPartTypeToolCall,
				ID:            "ws-call-1",
				ToolCallName:  createWorkspaceToolName,
				ToolCallInput: string(args),
			},
			{
				Type:         fantasy.StreamPartTypeFinish,
				Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				FinishReason: fantasy.FinishReasonToolCalls,
			},
		}
	} else {
		parts = []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
			{
				Type:         fantasy.StreamPartTypeFinish,
				Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				FinishReason: fantasy.FinishReasonToolCalls,
			},
		}
	}

	return streamFromParts(parts), nil
}

func (m *createWorkspaceToolModel) FirstPrompt() []fantasy.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]fantasy.Message(nil), m.firstPrompt...)
}

type executeTimeoutToolModel struct {
	fakeLanguageModelBase
	mu    sync.Mutex
	calls int
}

type createWorkspaceExecuteModel struct {
	fakeLanguageModelBase
	mu    sync.Mutex
	calls int
}

func (*createWorkspaceExecuteModel) Generate(
	_ context.Context,
	_ fantasy.Call,
) (*fantasy.Response, error) {
	return fallbackResponse(), nil
}

func (m *createWorkspaceExecuteModel) Stream(
	_ context.Context,
	_ fantasy.Call,
) (fantasy.StreamResponse, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()

	var parts []fantasy.StreamPart
	switch call {
	case 1:
		args, err := json.Marshal(map[string]any{"prompt": "workspace for execute test"})
		if err != nil {
			return nil, err
		}
		parts = []fantasy.StreamPart{
			{
				Type:          fantasy.StreamPartTypeToolCall,
				ID:            "create-workspace-call-1",
				ToolCallName:  createWorkspaceToolName,
				ToolCallInput: string(args),
			},
			{
				Type:         fantasy.StreamPartTypeFinish,
				Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				FinishReason: fantasy.FinishReasonToolCalls,
			},
		}
	case 2:
		args, err := json.Marshal(map[string]any{"command": "echo test"})
		if err != nil {
			return nil, err
		}
		parts = []fantasy.StreamPart{
			{
				Type:          fantasy.StreamPartTypeToolCall,
				ID:            "execute-call-1",
				ToolCallName:  executeToolName,
				ToolCallInput: string(args),
			},
			{
				Type:         fantasy.StreamPartTypeFinish,
				Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				FinishReason: fantasy.FinishReasonToolCalls,
			},
		}
	default:
		parts = []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
			{
				Type:         fantasy.StreamPartTypeFinish,
				Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				FinishReason: fantasy.FinishReasonToolCalls,
			},
		}
	}

	return streamFromParts(parts), nil
}

func (m *createWorkspaceExecuteModel) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (*executeTimeoutToolModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return fallbackResponse(), nil
}

func (m *executeTimeoutToolModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()

	var parts []fantasy.StreamPart
	if call == 1 {
		args, err := json.Marshal(map[string]any{
			"command":         "sh -c 'while :; do :; done'",
			"timeout_seconds": 1,
		})
		if err != nil {
			return nil, err
		}
		parts = []fantasy.StreamPart{
			{
				Type:          fantasy.StreamPartTypeToolCall,
				ID:            fmt.Sprintf("exec-timeout-%d", call),
				ToolCallName:  executeToolName,
				ToolCallInput: string(args),
			},
			{
				Type:         fantasy.StreamPartTypeFinish,
				Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				FinishReason: fantasy.FinishReasonToolCalls,
			},
		}
	} else {
		parts = []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
			{
				Type:  fantasy.StreamPartTypeTextDelta,
				ID:    "text-1",
				Delta: "continued after tool error",
			},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
			{
				Type:         fantasy.StreamPartTypeFinish,
				Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				FinishReason: fantasy.FinishReasonToolCalls,
			},
		}
	}

	return streamFromParts(parts), nil
}

func (m *executeTimeoutToolModel) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type singleToolThenTextModel struct {
	fakeLanguageModelBase
	mu       sync.Mutex
	calls    int
	toolName string
	args     json.RawMessage
	final    string
}

func (*singleToolThenTextModel) Generate(
	_ context.Context,
	_ fantasy.Call,
) (*fantasy.Response, error) {
	return fallbackResponse(), nil
}

func (m *singleToolThenTextModel) Stream(
	_ context.Context,
	_ fantasy.Call,
) (fantasy.StreamResponse, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()

	var parts []fantasy.StreamPart
	if call == 1 {
		args := m.args
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		parts = []fantasy.StreamPart{
			{
				Type:          fantasy.StreamPartTypeToolCall,
				ID:            fmt.Sprintf("%s-call-1", m.toolName),
				ToolCallName:  m.toolName,
				ToolCallInput: string(args),
			},
			{
				Type:         fantasy.StreamPartTypeFinish,
				Usage:        fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				FinishReason: fantasy.FinishReasonToolCalls,
			},
		}
	} else {
		final := m.final
		if final == "" {
			final = "done"
		}
		parts = []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: final},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
			{
				Type:  fantasy.StreamPartTypeFinish,
				Usage: fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
			},
		}
	}

	return streamFromParts(parts), nil
}

func (m *singleToolThenTextModel) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type streamErrorToolInputModel struct {
	fakeLanguageModelBase
	mu    sync.Mutex
	calls int
}

func (*streamErrorToolInputModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return fallbackResponse(), nil
}

func (m *streamErrorToolInputModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()

	if call == 1 {
		return streamFromParts([]fantasy.StreamPart{
			{
				Type:         fantasy.StreamPartTypeToolInputStart,
				ID:           "stream-error-tool-1",
				ToolCallName: createWorkspaceToolName,
			},
			{
				Type:         fantasy.StreamPartTypeToolInputDelta,
				ID:           "stream-error-tool-1",
				ToolCallName: createWorkspaceToolName,
				Delta:        `{"prompt":"workspace from stream error"}`,
			},
			{
				Type:  fantasy.StreamPartTypeError,
				Error: xerrors.New("stream failed after tool input"),
			},
		}), nil
	}

	return streamFromParts([]fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
		{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "continued after stream error"},
		{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
		{
			Type:  fantasy.StreamPartTypeFinish,
			Usage: fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
		},
	}), nil
}

func (m *streamErrorToolInputModel) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type streamErrorNoToolModel struct {
	fakeLanguageModelBase
}

func (*streamErrorNoToolModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return fallbackResponse(), nil
}

func (*streamErrorNoToolModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	return streamFromParts([]fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
		{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "partial"},
		{
			Type:  fantasy.StreamPartTypeError,
			Error: xerrors.New("stream failed without tool call"),
		},
	}), nil
}

type unsupportedStreamModel struct {
	fakeLanguageModelBase
	mu            sync.Mutex
	streamCalls   int
	generateCalls int
}

func (m *unsupportedStreamModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	m.mu.Lock()
	m.generateCalls++
	m.mu.Unlock()
	return fallbackResponse(), nil
}

func (m *unsupportedStreamModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	m.mu.Lock()
	m.streamCalls++
	m.mu.Unlock()
	return nil, xerrors.New("stream is not supported")
}

func (m *unsupportedStreamModel) CallCounts() (streamCalls int, generateCalls int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.streamCalls, m.generateCalls
}

type panicStreamModel struct {
	fakeLanguageModelBase
	panicValue any
}

func (*panicStreamModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return fallbackResponse(), nil
}

func (m *panicStreamModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	panic(m.panicValue)
}

type blockingCloseModel struct {
	fakeLanguageModelBase
	started chan struct{}
	release chan struct{}
}

func (*blockingCloseModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return fallbackResponse(), nil
}

func (m *blockingCloseModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	select {
	case <-m.started:
	default:
		close(m.started)
	}

	<-m.release

	parts := []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
		{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
		{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
		{
			Type:  fantasy.StreamPartTypeFinish,
			Usage: fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
		},
	}
	return streamFromParts(parts), nil
}

type fixedTextModel struct {
	fakeLanguageModelBase
	mu    sync.Mutex
	calls int
	text  string
}

func (*fixedTextModel) Generate(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
	return fallbackResponse(), nil
}

func (m *fixedTextModel) Stream(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
	m.mu.Lock()
	m.calls++
	text := m.text
	m.mu.Unlock()

	if strings.TrimSpace(text) == "" {
		text = "done"
	}
	return streamFromParts([]fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
		{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: text},
		{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
		{
			Type:  fantasy.StreamPartTypeFinish,
			Usage: fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
		},
	}), nil
}

func (m *fixedTextModel) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
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

	content, err := json.Marshal(contentFromText(message))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(fantasy.MessageRoleUser),
		Content: pqtype.NullRawMessage{RawMessage: content, Valid: true},
		Hidden:  false,
	})
	require.NoError(t, err)

	return chat
}

func insertDelegatedChildChatWithUserMessage(
	t *testing.T,
	db database.Store,
	dbCtx context.Context,
	ownerID uuid.UUID,
	parentID uuid.UUID,
	title string,
	message string,
) (database.Chat, uuid.UUID) {
	t.Helper()

	child, err := db.InsertChat(dbCtx, database.InsertChatParams{
		OwnerID: ownerID,
		ParentChatID: uuid.NullUUID{
			UUID:  parentID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  parentID,
			Valid: true,
		},
		Title:       title,
		ModelConfig: json.RawMessage(`{"model":"fake"}`),
	})
	require.NoError(t, err)

	requestID := uuid.New()
	content, err := json.Marshal(contentFromText(message))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  child.ID,
		Role:    string(fantasy.MessageRoleUser),
		Content: pqtype.NullRawMessage{RawMessage: content, Valid: true},
		Hidden:  false,
		SubagentRequestID: uuid.NullUUID{
			UUID:  requestID,
			Valid: true,
		},
		SubagentEvent: sql.NullString{
			String: "request",
			Valid:  true,
		},
	})
	require.NoError(t, err)

	return child, requestID
}

func insertSubagentReportOnlyMarker(
	t *testing.T,
	db database.Store,
	dbCtx context.Context,
	chatID uuid.UUID,
	requestID uuid.UUID,
) {
	t.Helper()

	content, err := json.Marshal(contentFromText("report-only pass requested"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chatID,
		Role:    "__subagent_report_only_marker",
		Content: pqtype.NullRawMessage{RawMessage: content, Valid: true},
		Hidden:  true,
		SubagentRequestID: uuid.NullUUID{
			UUID:  requestID,
			Valid: true,
		},
	})
	require.NoError(t, err)
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

	content, err := json.Marshal(contentFromText("read file"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(fantasy.MessageRoleUser),
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
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
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
	require.Equal(t, string(fantasy.MessageRoleTool), toolMessage.Role)
	var toolResults []chatd.ToolResultBlock
	require.NoError(t, json.Unmarshal(toolMessage.Content.RawMessage, &toolResults))
	require.Len(t, toolResults, 1)
	resultMap, ok := toolResults[0].Result.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "hello", resultMap["content"])

	finalMessage := stored[3]
	finalContent, err := parseAssistantContent(finalMessage.Content)
	require.NoError(t, err)
	require.Len(t, finalContent, 1)
	textBlock, ok := fantasy.AsContentType[fantasy.TextContent](finalContent[0])
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

	content, err := json.Marshal(contentFromText("run command"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(fantasy.MessageRoleUser),
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
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
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

	var toolResults []chatd.ToolResultBlock
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
	textBlock, ok := fantasy.AsContentType[fantasy.TextContent](finalContent[0])
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
				chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
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

			var toolResults []chatd.ToolResultBlock
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
			textBlock, ok := fantasy.AsContentType[fantasy.TextContent](finalContent[0])
			require.True(t, ok)
			require.Equal(t, "done after tool error", textBlock.Text)

			require.Equal(t, 2, model.CallCount())
		})
	}
}

func TestRunChatLoop_StreamErrorWithToolInputDeltaErrors(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named("chatd")

	chat := insertChatWithUserMessage(t, db, dbCtx, user.UserID, "stream error tool delta", "create a workspace")
	model := &streamErrorToolInputModel{}

	var (
		creatorMu     sync.Mutex
		creatorCalls  int
		creatorPrompt string
	)
	creator := workspaceCreatorFunc(func(_ context.Context, req chatd.CreateWorkspaceToolRequest) (chatd.CreateWorkspaceToolResult, error) {
		creatorMu.Lock()
		creatorCalls++
		creatorPrompt = req.Prompt
		creatorMu.Unlock()

		return chatd.CreateWorkspaceToolResult{
			Created: false,
			Reason:  "declined for test",
		}, nil
	})

	processor := chatd.NewProcessor(
		logger.Named("processor"),
		db,
		chatd.WithWorkspaceCreator(creator),
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithPollInterval(50*time.Millisecond),
	)
	defer processor.Close()

	_, err := db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		storedChat, err := db.GetChatByID(dbCtx, chat.ID)
		return err == nil && storedChat.Status == database.ChatStatusError
	}, testutil.WaitLong, 50*time.Millisecond)

	messages, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)

	creatorMu.Lock()
	creatorCallsValue := creatorCalls
	creatorPromptValue := strings.TrimSpace(creatorPrompt)
	creatorMu.Unlock()
	require.Equal(t, 0, creatorCallsValue)
	require.Equal(t, "", creatorPromptValue)
	require.Equal(t, 1, model.CallCount())

	storedChat, err := db.GetChatByID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, storedChat.Status)
}

func TestRunChatLoop_UnsupportedStreamingDoesNotFallbackToGenerate(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named("chatd")

	chat := insertChatWithUserMessage(t, db, dbCtx, user.UserID, "unsupported stream", "hello")
	model := &unsupportedStreamModel{}

	processor := chatd.NewProcessor(
		logger.Named("processor"),
		db,
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithPollInterval(50*time.Millisecond),
	)
	defer processor.Close()

	_, err := db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		storedChat, err := db.GetChatByID(dbCtx, chat.ID)
		return err == nil && storedChat.Status == database.ChatStatusError
	}, testutil.WaitLong, 50*time.Millisecond)

	streamCalls, generateCalls := model.CallCounts()
	require.Equal(t, 1, streamCalls)
	require.Equal(t, 0, generateCalls)

	messages, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
}

func TestRunChatLoop_StreamErrorWithoutToolCallsErrors(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	chat := insertChatWithUserMessage(t, db, dbCtx, user.UserID, "stream error no tool", "hello")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Named("chatd")
	manager := chatd.NewStreamManager(logger)
	model := &streamErrorNoToolModel{}

	processor := chatd.NewProcessor(
		logger.Named("processor"),
		db,
		chatd.WithStreamManager(manager),
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
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
				if streamErr != "" {
					return true
				}
			}
		default:
		}

		storedChat, err := db.GetChatByID(dbCtx, chat.ID)
		if err != nil {
			return false
		}
		return storedChat.Status == database.ChatStatusError
	}, testutil.WaitLong, 25*time.Millisecond)

	if streamErr != "" {
		require.Contains(t, streamErr, "stream failed without tool call")
	}

	storedChat, err := db.GetChatByID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, storedChat.Status)

	messages, err := db.GetChatMessagesByChatID(dbCtx, chat.ID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
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

	content, err := json.Marshal(contentFromText("create a workspace"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(fantasy.MessageRoleUser),
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
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
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

	var toolResults []chatd.ToolResultBlock
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
	require.Equal(t, fantasy.MessageRoleSystem, prompt[0].Role)
	require.NotEmpty(t, prompt[0].Content)
	systemPart, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
	require.True(t, ok)
	require.True(t, strings.Contains(strings.ToLower(systemPart.Text), "create_workspace"))
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

	content, err := json.Marshal(contentFromText("create a workspace"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(fantasy.MessageRoleUser),
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
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
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
		if event.MessagePart.Role != string(fantasy.MessageRoleTool) {
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

	content, err := json.Marshal(contentFromText("create workspace"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(fantasy.MessageRoleUser),
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
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
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
	require.Equal(t, fantasy.MessageRoleSystem, prompt[0].Role)
	require.NotEmpty(t, prompt[0].Content)
	systemPart, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
	require.True(t, ok)
	require.True(t, strings.Contains(strings.ToLower(systemPart.Text), "create_workspace"))
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

	content, err := json.Marshal(contentFromText("create workspace"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(fantasy.MessageRoleUser),
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
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
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

	content, err := json.Marshal(contentFromText("create workspace then run echo test"))
	require.NoError(t, err)
	_, err = db.InsertChatMessage(dbCtx, database.InsertChatMessageParams{
		ChatID:  chat.ID,
		Role:    string(fantasy.MessageRoleUser),
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
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
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
			if message.Role != string(fantasy.MessageRoleTool) {
				continue
			}
			var toolResults []chatd.ToolResultBlock
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

func TestProcessorCloseWaitsForInFlightChatProcessing(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	chat := insertChatWithUserMessage(t, db, dbCtx, user.UserID, "close waits", "hello")
	model := &blockingCloseModel{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	processor := chatd.NewProcessor(
		testutil.Logger(t).Named("chatd"),
		db,
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithPollInterval(25*time.Millisecond),
	)

	_, err := db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	select {
	case <-model.started:
	case <-time.After(testutil.WaitLong):
		t.Fatal("timed out waiting for chat processing to start")
	}

	closeDone := make(chan struct{})
	go func() {
		_ = processor.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
		t.Fatal("processor.Close returned before in-flight chat processing finished")
	case <-time.After(200 * time.Millisecond):
	}

	close(model.release)

	select {
	case <-closeDone:
	case <-time.After(testutil.WaitLong):
		t.Fatal("processor.Close did not return after in-flight chat processing finished")
	}
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
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
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
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
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

func TestRunChatLoop_DelegatedChildWithoutReportRequeuesForReportPass(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	parent := insertChatWithUserMessage(t, db, dbCtx, user.UserID, "parent", "parent prompt")
	child, requestID := insertDelegatedChildChatWithUserMessage(
		t,
		db,
		dbCtx,
		user.UserID,
		parent.ID,
		"child",
		"run delegated subagent",
	)

	model := &fixedTextModel{text: "child completed without explicit report"}
	processor := chatd.NewProcessor(
		testutil.Logger(t).Named("chatd"),
		db,
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithPollInterval(10*time.Second),
	)
	defer processor.Close()

	_, err := db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        child.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		storedChild, err := db.GetChatByID(dbCtx, child.ID)
		if err != nil {
			return false
		}
		if storedChild.Status != database.ChatStatusPending {
			return false
		}

		messages, msgErr := db.GetChatMessagesByChatID(dbCtx, child.ID)
		if msgErr != nil {
			return false
		}
		for _, message := range messages {
			if message.Role != "__subagent_report_only_marker" || !message.Hidden {
				continue
			}
			return message.SubagentRequestID.Valid &&
				message.SubagentRequestID.UUID == requestID
		}
		return false
	}, testutil.WaitLong, 25*time.Millisecond)
}

func TestRunChatLoop_ReportOnlyPassWithoutSubagentReportFallsBack(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := coderdtest.NewWithDatabase(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	dbCtx := dbauthz.AsSystemRestricted(ctx)

	parent := insertChatWithUserMessage(t, db, dbCtx, user.UserID, "parent", "parent prompt")
	_, err := db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        parent.ID,
		Status:    database.ChatStatusRunning,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	child, requestID := insertDelegatedChildChatWithUserMessage(
		t,
		db,
		dbCtx,
		user.UserID,
		parent.ID,
		"child report-only subagent",
		"finish delegated subagent",
	)
	insertSubagentReportOnlyMarker(t, db, dbCtx, child.ID, requestID)

	const fallbackSummary = "summarized completion from report-only pass"
	model := &fixedTextModel{text: fallbackSummary}
	processor := chatd.NewProcessor(
		testutil.Logger(t).Named("chatd"),
		db,
		chatd.WithModelResolver(func(_ database.Chat) (fantasy.LanguageModel, error) {
			return model, nil
		}),
		chatd.WithPollInterval(750*time.Millisecond),
	)
	defer processor.Close()

	_, err = db.UpdateChatStatus(dbCtx, database.UpdateChatStatusParams{
		ID:        child.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		responseMarker, markerErr := db.GetSubagentResponseMessageByChatIDAndRequestID(
			dbCtx,
			database.GetSubagentResponseMessageByChatIDAndRequestIDParams{
				ChatID:            child.ID,
				SubagentRequestID: requestID,
			},
		)
		if markerErr != nil {
			return false
		}

		if !strings.Contains(string(responseMarker.Content.RawMessage), fallbackSummary) {
			return false
		}

		durationMS, durationErr := db.GetSubagentRequestDurationByChatIDAndRequestID(
			dbCtx,
			database.GetSubagentRequestDurationByChatIDAndRequestIDParams{
				ChatID:            child.ID,
				SubagentRequestID: requestID,
			},
		)
		return durationErr == nil && durationMS > 0
	}, testutil.WaitLong, 25*time.Millisecond)

	require.Eventually(t, func() bool {
		messages, msgErr := db.GetChatMessagesByChatID(dbCtx, parent.ID)
		if msgErr != nil {
			return false
		}
		for _, message := range messages {
			if message.Role != string(fantasy.MessageRoleTool) {
				continue
			}
			var blocks []chatd.ToolResultBlock
			if err := json.Unmarshal(message.Content.RawMessage, &blocks); err != nil {
				continue
			}
			if len(blocks) != 1 || blocks[0].ToolName != "subagent_report" {
				continue
			}
			payload, ok := blocks[0].Result.(map[string]any)
			if !ok {
				continue
			}
			report, reportOK := payload["report"].(string)
			request, requestOK := payload["request_id"].(string)
			return reportOK && requestOK && report == fallbackSummary && request == requestID.String()
		}
		return false
	}, testutil.WaitLong, 25*time.Millisecond)
}
