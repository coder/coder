package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyazure "charm.land/fantasy/providers/azure"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	fantasygoogle "charm.land/fantasy/providers/google"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	// DefaultPollInterval is the default time between polling for pending chats.
	DefaultPollInterval = time.Second
	// DefaultStaleThreshold is the default time after which a running chat is
	// considered stale and should be recovered.
	DefaultStaleThreshold = 5 * time.Minute

	toolCreateWorkspace = "create_workspace"
	toolReadFile        = "read_file"
	toolWriteFile       = "write_file"
	toolEditFiles       = "edit_files"
	toolExecute         = "execute"
	toolTask            = "task"
	toolTaskAwait       = "task_await"
	toolTaskTerminate   = "task_terminate"
	toolAgentReport     = "agent_report"

	defaultExecuteTimeout = 60 * time.Second
	maxChatSteps          = 1200

	defaultChatModel = "claude-opus-4-6"

	maxCreateWorkspaceBuildLogLines     = 120
	maxCreateWorkspaceBuildLogChars     = 16 * 1024
	maxCreateWorkspaceBuildLogLineChars = 240

	defaultTitleGenerationPrompt = "Generate a concise title (max 8 words) for " +
		"the user's first message. Return plain text only, with no surrounding " +
		"quotes."

	defaultNoWorkspaceInstruction = "No workspace is selected yet. Call the create_workspace tool first before using read_file, write_file, or execute. If create_workspace fails, ask the user to clarify the template or workspace request."
	reportOnlyInstruction         = "Report-only follow-up pass. Call agent_report exactly once with a concise summary and nothing else."
)

var (
	ErrChatInterrupted = xerrors.New("chat interrupted")

	toolCallIDSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
)

// Processor handles background processing of pending chats.
type Processor struct {
	cancel   context.CancelFunc
	closed   chan struct{}
	inflight sync.WaitGroup

	db       database.Store
	workerID uuid.UUID
	logger   slog.Logger

	agentConnector   AgentConnector
	workspaceCreator WorkspaceCreator
	modelResolver    ModelResolver
	streamManager    *StreamManager
	taskService      *TaskService
	providerKeys     ProviderAPIKeys
	providerKeysFn   ProviderAPIKeysResolver
	titleGeneration  TitleGenerationConfig
	titleModelLookup func(ProviderAPIKeys) (fantasy.LanguageModel, error)

	activeMu      sync.Mutex
	activeCancels map[uuid.UUID]context.CancelCauseFunc

	// Configuration
	pollInterval   time.Duration
	staleThreshold time.Duration
}

// AgentConnector provides access to workspace agent connections.
type AgentConnector interface {
	AgentConn(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error)
}

// WorkspaceCreator creates workspaces for chats when none are selected.
type WorkspaceCreator interface {
	CreateWorkspace(ctx context.Context, req CreateWorkspaceToolRequest) (CreateWorkspaceToolResult, error)
}

// CreateWorkspaceToolRequest is the request payload for the create_workspace tool.
type CreateWorkspaceToolRequest struct {
	Chat            database.Chat
	Model           fantasy.LanguageModel
	Prompt          string
	Spec            json.RawMessage
	BuildLogHandler CreateWorkspaceBuildLogHandler
}

type CreateWorkspaceBuildLogHandler func(CreateWorkspaceBuildLog)

type CreateWorkspaceBuildLog struct {
	Source string
	Level  string
	Stage  string
	Output string
}

// CreateWorkspaceToolResult is the normalized result payload for the create_workspace tool.
type CreateWorkspaceToolResult struct {
	Created          bool
	WorkspaceID      uuid.UUID
	WorkspaceAgentID uuid.UUID
	WorkspaceName    string
	WorkspaceURL     string
	Reason           string
}

// ToolResultBlock is the persisted chat tool result shape.
type ToolResultBlock struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Result     any    `json:"result"`
	IsError    bool   `json:"is_error,omitempty"`
}

// ModelResolver resolves a model for a chat.
type ModelResolver func(chat database.Chat) (fantasy.LanguageModel, error)

// ProviderAPIKeysResolver resolves provider API keys for chat model calls.
type ProviderAPIKeysResolver func(context.Context) (ProviderAPIKeys, error)

// TitleGenerationConfig controls AI-generated chat title behavior.
type TitleGenerationConfig struct {
	Prompt string
}

func (c TitleGenerationConfig) withDefaults() TitleGenerationConfig {
	cfg := TitleGenerationConfig{
		Prompt: strings.TrimSpace(c.Prompt),
	}
	if cfg.Prompt == "" {
		cfg.Prompt = defaultTitleGenerationPrompt
	}
	return cfg
}

// StreamManager broadcasts in-flight chat stream events.
type StreamManager struct {
	logger slog.Logger
	mu     sync.Mutex
	chats  map[uuid.UUID]*chatStreamState
}

type chatStreamState struct {
	buffer      []codersdk.ChatStreamEvent
	buffering   bool
	subscribers map[uuid.UUID]chan codersdk.ChatStreamEvent
}

func NewStreamManager(logger slog.Logger) *StreamManager {
	return &StreamManager{
		logger: logger.Named("chat-stream"),
		chats:  make(map[uuid.UUID]*chatStreamState),
	}
}

func (m *StreamManager) StartStream(chatID uuid.UUID) {
	m.mu.Lock()
	state := m.stateLocked(chatID)
	state.buffer = nil
	state.buffering = true
	m.mu.Unlock()
}

func (m *StreamManager) StopStream(chatID uuid.UUID) {
	m.mu.Lock()
	state, ok := m.chats[chatID]
	if ok {
		state.buffer = nil
		state.buffering = false
	}
	m.mu.Unlock()
}

func (m *StreamManager) Publish(chatID uuid.UUID, event codersdk.ChatStreamEvent) {
	m.mu.Lock()
	state := m.stateLocked(chatID)
	if state.buffering && event.Type == codersdk.ChatStreamEventTypeMessagePart {
		state.buffer = append(state.buffer, event)
	}
	subscribers := make([]chan codersdk.ChatStreamEvent, 0, len(state.subscribers))
	for _, ch := range state.subscribers {
		subscribers = append(subscribers, ch)
	}
	m.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
			m.logger.Warn(context.Background(), "dropping chat stream event",
				slog.F("chat_id", chatID), slog.F("type", event.Type))
		}
	}
}

func (m *StreamManager) Subscribe(chatID uuid.UUID) (
	[]codersdk.ChatStreamEvent,
	<-chan codersdk.ChatStreamEvent,
	func(),
) {
	m.mu.Lock()
	state := m.stateLocked(chatID)
	snapshot := append([]codersdk.ChatStreamEvent(nil), state.buffer...)
	id := uuid.New()
	ch := make(chan codersdk.ChatStreamEvent, 128)
	state.subscribers[id] = ch
	m.mu.Unlock()

	cancel := func() {
		m.mu.Lock()
		state, ok := m.chats[chatID]
		if ok {
			if subscriber, exists := state.subscribers[id]; exists {
				delete(state.subscribers, id)
				close(subscriber)
			}
		}
		m.mu.Unlock()
	}

	return snapshot, ch, cancel
}

func (m *StreamManager) stateLocked(chatID uuid.UUID) *chatStreamState {
	state, ok := m.chats[chatID]
	if !ok {
		state = &chatStreamState{subscribers: make(map[uuid.UUID]chan codersdk.ChatStreamEvent)}
		m.chats[chatID] = state
	}
	return state
}

type Option func(*Processor)

// WithPollInterval sets the interval between polling for pending chats.
func WithPollInterval(interval time.Duration) Option {
	return func(p *Processor) {
		p.pollInterval = interval
	}
}

// WithStaleThreshold sets the time after which a running chat is considered stale.
func WithStaleThreshold(threshold time.Duration) Option {
	return func(p *Processor) {
		p.staleThreshold = threshold
	}
}

// WithAgentConnector sets the workspace agent connector used for tools.
func WithAgentConnector(connector AgentConnector) Option {
	return func(p *Processor) {
		p.agentConnector = connector
	}
}

// WithWorkspaceCreator sets the workspace creator used for create_workspace.
func WithWorkspaceCreator(creator WorkspaceCreator) Option {
	return func(p *Processor) {
		p.workspaceCreator = creator
	}
}

// WithModelResolver sets a model resolver override for the processor.
func WithModelResolver(resolver ModelResolver) Option {
	return func(p *Processor) {
		p.modelResolver = resolver
	}
}

// WithStreamManager sets the stream manager used to broadcast chat events.
func WithStreamManager(manager *StreamManager) Option {
	return func(p *Processor) {
		p.streamManager = manager
	}
}

// WithProviderAPIKeys sets fallback provider API keys used for model calls.
func WithProviderAPIKeys(keys ProviderAPIKeys) Option {
	return func(p *Processor) {
		p.providerKeys = keys
	}
}

// WithProviderAPIKeysResolver sets a dynamic provider key resolver.
func WithProviderAPIKeysResolver(resolver ProviderAPIKeysResolver) Option {
	return func(p *Processor) {
		p.providerKeysFn = resolver
	}
}

// WithTitleGenerationConfig sets chat title generation defaults.
func WithTitleGenerationConfig(config TitleGenerationConfig) Option {
	return func(p *Processor) {
		p.titleGeneration = config.withDefaults()
	}
}

// NewProcessor creates a new chat processor. The processor polls for pending
// chats and processes them. It is the caller's responsibility to call Close
// on the returned instance.
func NewProcessor(logger slog.Logger, db database.Store, opts ...Option) *Processor {
	ctx, cancel := context.WithCancel(context.Background())

	p := &Processor{
		cancel:         cancel,
		closed:         make(chan struct{}),
		db:             db,
		workerID:       uuid.New(),
		logger:         logger.Named("chat-processor"),
		activeCancels:  make(map[uuid.UUID]context.CancelCauseFunc),
		pollInterval:   DefaultPollInterval,
		staleThreshold: DefaultStaleThreshold,
		titleGeneration: TitleGenerationConfig{
			Prompt: defaultTitleGenerationPrompt,
		}.withDefaults(),
		titleModelLookup: anyAvailableModel,
	}

	for _, opt := range opts {
		opt(p)
	}

	p.taskService = newTaskService(p.db, p)

	//nolint:gocritic // The chat processor is a system-level service.
	ctx = dbauthz.AsSystemRestricted(ctx)
	go p.start(ctx)

	return p
}

func (p *Processor) start(ctx context.Context) {
	defer close(p.closed)

	// First, recover any stale chats from crashed workers.
	p.recoverStaleChats(ctx)

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.processOnce(ctx)
		}
	}
}

func (p *Processor) processOnce(ctx context.Context) {
	// Try to acquire a pending chat.
	chat, err := p.db.AcquireChat(ctx, database.AcquireChatParams{
		StartedAt: time.Now(),
		WorkerID:  p.workerID,
	})
	if err != nil {
		if !xerrors.Is(err, sql.ErrNoRows) {
			p.logger.Error(ctx, "failed to acquire chat", slog.Error(err))
		}
		// No pending chats or error.
		return
	}

	// Process the chat (don't block the main loop).
	p.inflight.Add(1)
	go func() {
		defer p.inflight.Done()
		p.processChat(ctx, chat)
	}()
}

func (p *Processor) registerChat(chatID uuid.UUID, cancel context.CancelCauseFunc) {
	p.activeMu.Lock()
	p.activeCancels[chatID] = cancel
	p.activeMu.Unlock()
}

func (p *Processor) unregisterChat(chatID uuid.UUID) {
	p.activeMu.Lock()
	delete(p.activeCancels, chatID)
	p.activeMu.Unlock()
}

func (p *Processor) InterruptChat(chatID uuid.UUID) bool {
	p.activeMu.Lock()
	cancel, ok := p.activeCancels[chatID]
	p.activeMu.Unlock()
	if !ok {
		return false
	}
	cancel(ErrChatInterrupted)
	if p.streamManager != nil {
		p.streamManager.StopStream(chatID)
	}
	return true
}

func (p *Processor) publishEvent(chatID uuid.UUID, event codersdk.ChatStreamEvent) {
	if p.streamManager == nil {
		return
	}
	if event.ChatID == uuid.Nil {
		event.ChatID = chatID
	}
	p.streamManager.Publish(chatID, event)
}

func (p *Processor) publishStatus(chatID uuid.UUID, status database.ChatStatus) {
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeStatus,
		Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatus(status)},
	})
}

func (p *Processor) publishError(chatID uuid.UUID, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type:  codersdk.ChatStreamEventTypeError,
		Error: &codersdk.ChatStreamError{Message: message},
	})
}

func processingFailureReason(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	reason := strings.TrimSpace(err.Error())
	if reason == "" {
		return "", false
	}
	return reason, true
}

func panicFailureReason(recovered any) string {
	var reason string
	switch typed := recovered.(type) {
	case string:
		reason = strings.TrimSpace(typed)
	case error:
		reason = strings.TrimSpace(typed.Error())
	default:
		reason = strings.TrimSpace(fmt.Sprint(typed))
	}

	if reason == "" || reason == "<nil>" {
		return "chat processing panicked"
	}
	return "chat processing panicked: " + reason
}

func (p *Processor) publishMessage(chatID uuid.UUID, message database.ChatMessage) {
	sdkMessage := SDKChatMessage(message)
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type:    codersdk.ChatStreamEventTypeMessage,
		Message: &sdkMessage,
	})
}

func (p *Processor) publishMessagePart(chatID uuid.UUID, role string, part codersdk.ChatMessagePart) {
	if part.Type == "" {
		return
	}
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role: role,
			Part: part,
		},
	})
}

func (p *Processor) processChat(ctx context.Context, chat database.Chat) {
	logger := p.logger.With(slog.F("chat_id", chat.ID))
	logger.Info(ctx, "processing chat")
	reportOnly := chat.ParentChatID.Valid &&
		chat.TaskStatus == database.ChatTaskStatusAwaitingReport

	chatCtx, cancel := context.WithCancelCause(ctx)
	p.registerChat(chat.ID, cancel)
	defer p.unregisterChat(chat.ID)
	defer cancel(nil)

	if chat.ParentChatID.Valid && p.taskService != nil {
		if err := p.taskService.SetTaskRunning(chatCtx, chat.ID); err != nil {
			logger.Warn(ctx, "failed to set delegated chat task running", slog.Error(err))
		}
	}

	p.publishStatus(chat.ID, database.ChatStatusRunning)

	// Determine the final status to set when we're done.
	status := database.ChatStatusWaiting

	defer func() {
		// Handle panics gracefully.
		if r := recover(); r != nil {
			logger.Error(ctx, "panic during chat processing", slog.F("panic", r))
			p.publishError(chat.ID, panicFailureReason(r))
			status = database.ChatStatusError
		}

		if chat.ParentChatID.Valid && p.taskService != nil {
			hasActiveDescendants, err := p.taskService.HasActiveDescendants(ctx, chat.ID)
			if err != nil {
				logger.Warn(ctx, "failed to check delegated chat descendants", slog.Error(err))
			} else if !hasActiveDescendants {
				chatState, stateErr := p.db.GetChatByID(ctx, chat.ID)
				if stateErr != nil {
					logger.Warn(ctx, "failed to read delegated chat state", slog.Error(stateErr))
				} else if chatState.TaskStatus != database.ChatTaskStatusReported {
					if reportOnly {
						report := p.taskService.SynthesizeFallbackTaskReport(ctx, chat.ID)
						if err := p.taskService.MarkTaskReported(ctx, chat.ID, report); err != nil {
							logger.Warn(ctx, "failed to mark delegated chat reported with fallback", slog.Error(err))
						}
					} else {
						if err := p.taskService.SetTaskAwaitingReport(ctx, chat.ID); err != nil {
							logger.Warn(ctx, "failed to set delegated chat awaiting report", slog.Error(err))
						} else {
							status = database.ChatStatusPending
						}
					}
				}
			}
		}

		// Release the chat when done.
		_, err := p.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
			ID:        chat.ID,
			Status:    status,
			WorkerID:  uuid.NullUUID{}, // Clear worker.
			StartedAt: sql.NullTime{},  // Clear started_at.
		})
		if err != nil {
			logger.Error(ctx, "failed to release chat", slog.Error(err))
		}

		p.publishStatus(chat.ID, status)
	}()

	if err := p.runChat(chatCtx, chat, logger, reportOnly); err != nil {
		if errors.Is(err, ErrChatInterrupted) || errors.Is(context.Cause(chatCtx), ErrChatInterrupted) {
			logger.Info(ctx, "chat interrupted")
			status = database.ChatStatusWaiting
			return
		}
		logger.Error(ctx, "failed to process chat", slog.Error(err))
		if reason, ok := processingFailureReason(err); ok {
			p.publishError(chat.ID, reason)
		}
		status = database.ChatStatusError
		return
	}
}

func (p *Processor) runChat(
	ctx context.Context,
	chat database.Chat,
	logger slog.Logger,
	reportOnly bool,
) error {
	messages, err := p.db.GetChatMessagesByChatID(ctx, chat.ID)
	if err != nil {
		return xerrors.Errorf("get chat messages: %w", err)
	}
	p.maybeGenerateChatTitle(ctx, chat, messages, logger)

	chat, err = p.recoverMissingWorkspace(ctx, chat, logger)
	if err != nil {
		return err
	}

	prompt, err := chatMessagesToPrompt(messages)
	if err != nil {
		return xerrors.Errorf("build chat prompt: %w", err)
	}
	if reportOnly {
		prompt = appendSystemInstruction(prompt, reportOnlyInstruction)
	} else if !chat.WorkspaceID.Valid {
		prompt = prependSystemInstruction(prompt, defaultNoWorkspaceInstruction)
	}

	if p.streamManager != nil {
		p.streamManager.StartStream(chat.ID)
		defer p.streamManager.StopStream(chat.ID)
	}

	model, err := p.resolveChatModel(ctx, chat)
	if err != nil {
		return err
	}

	result, err := p.runChatWithAgent(ctx, chat, model, prompt, reportOnly)
	if err != nil {
		return err
	}
	if result != nil && len(result.Steps) >= maxChatSteps {
		lastStep := result.Steps[len(result.Steps)-1]
		if lastStep.FinishReason == fantasy.FinishReasonToolCalls &&
			len(lastStep.Content.ToolCalls()) > 0 {
			return xerrors.Errorf("chat exceeded %d tool steps", maxChatSteps)
		}
	}
	return nil
}

func (p *Processor) resolveChatModel(ctx context.Context, chat database.Chat) (fantasy.LanguageModel, error) {
	if p.modelResolver != nil {
		model, err := p.modelResolver(chat)
		if err != nil {
			return nil, xerrors.Errorf("resolve model: %w", err)
		}
		return model, nil
	}

	keys, err := p.resolveProviderAPIKeys(ctx)
	if err != nil {
		return nil, xerrors.Errorf("resolve provider API keys: %w", err)
	}
	model, err := modelFromChat(chat, keys)
	if err != nil {
		return nil, xerrors.Errorf("resolve model: %w", err)
	}
	return model, nil
}

func (p *Processor) runChatWithAgent(
	ctx context.Context,
	chat database.Chat,
	model fantasy.LanguageModel,
	prompt []fantasy.Message,
	reportOnly bool,
) (*fantasy.AgentResult, error) {
	currentChat := chat
	var (
		chatStateMu sync.Mutex
		conn        workspacesdk.AgentConn
		releaseConn func()
	)
	closeConn := func() {
		if releaseConn != nil {
			releaseConn()
			releaseConn = nil
		}
	}
	defer closeConn()

	getWorkspaceConn := func(ctx context.Context) (workspacesdk.AgentConn, error) {
		chatStateMu.Lock()
		if conn != nil {
			currentConn := conn
			chatStateMu.Unlock()
			return currentConn, nil
		}
		chatSnapshot := currentChat
		chatStateMu.Unlock()

		if p.agentConnector == nil {
			return nil, xerrors.New("workspace agent connector is not configured")
		}

		agentID, err := p.resolveAgentID(ctx, chatSnapshot)
		if err != nil {
			return nil, err
		}

		agentConn, agentRelease, err := p.agentConnector.AgentConn(ctx, agentID)
		if err != nil {
			return nil, xerrors.Errorf("connect to workspace agent: %w", err)
		}

		chatStateMu.Lock()
		if conn == nil {
			conn = agentConn
			releaseConn = agentRelease
			chatStateMu.Unlock()
			return agentConn, nil
		}
		currentConn := conn
		chatStateMu.Unlock()

		agentRelease()
		return currentConn, nil
	}

	persistAssistant := func(content []fantasy.Content) error {
		if len(content) == 0 {
			return nil
		}

		assistantContent, err := marshalContentBlocks(content)
		if err != nil {
			return err
		}

		assistantMessage, err := p.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID:  chat.ID,
			Role:    string(fantasy.MessageRoleAssistant),
			Content: assistantContent,
			ToolCallID: sql.NullString{
				Valid: false,
			},
			Thinking: sql.NullString{
				Valid: false,
			},
			Hidden: false,
		})
		if err != nil {
			return xerrors.Errorf("insert assistant message: %w", err)
		}
		p.publishMessage(chat.ID, assistantMessage)
		return nil
	}

	persistToolResult := func(result ToolResultBlock) error {
		resultContent, err := marshalToolResults([]ToolResultBlock{result})
		if err != nil {
			return err
		}

		toolMessage, err := p.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID:  chat.ID,
			Role:    string(fantasy.MessageRoleTool),
			Content: resultContent,
			ToolCallID: sql.NullString{
				String: result.ToolCallID,
				Valid:  result.ToolCallID != "",
			},
			Hidden: false,
		})
		if err != nil {
			return xerrors.Errorf("insert tool result: %w", err)
		}

		p.publishMessage(chat.ID, toolMessage)
		return nil
	}

	var (
		stepStateMu     sync.Mutex
		streamToolNames map[string]string
		stepToolResults []ToolResultBlock
	)
	resetStepState := func() {
		stepStateMu.Lock()
		streamToolNames = make(map[string]string)
		stepToolResults = nil
		stepStateMu.Unlock()
	}
	resetStepState()

	agent := fantasy.NewAgent(
		model,
		fantasy.WithTools(p.agentTools(model, &currentChat, &chatStateMu, getWorkspaceConn)...),
		fantasy.WithStopConditions(fantasy.StepCountIs(maxChatSteps)),
	)
	sentinelPrompt := "__chatd_agent_prompt_sentinel_" + uuid.NewString()

	result, err := agent.Stream(ctx, fantasy.AgentStreamCall{
		Prompt:   sentinelPrompt,
		Messages: prompt,
		PrepareStep: func(
			stepCtx context.Context,
			options fantasy.PrepareStepFunctionOptions,
		) (context.Context, fantasy.PrepareStepResult, error) {
			return stepCtx, prepareAgentStepResult(options.Messages, sentinelPrompt, reportOnly), nil
		},
		OnStepStart: func(_ int) error {
			resetStepState()
			return nil
		},
		OnTextDelta: func(_ string, text string) error {
			p.publishMessagePart(chat.ID, string(fantasy.MessageRoleAssistant), codersdk.ChatMessagePart{
				Type: codersdk.ChatMessagePartTypeText,
				Text: text,
			})
			return nil
		},
		OnReasoningDelta: func(_ string, text string) error {
			p.publishMessagePart(chat.ID, string(fantasy.MessageRoleAssistant), codersdk.ChatMessagePart{
				Type: codersdk.ChatMessagePartTypeReasoning,
				Text: text,
			})
			return nil
		},
		OnToolInputStart: func(id, toolName string) error {
			stepStateMu.Lock()
			streamToolNames[id] = toolName
			stepStateMu.Unlock()
			return nil
		},
		OnToolInputDelta: func(id, delta string) error {
			stepStateMu.Lock()
			toolName := streamToolNames[id]
			stepStateMu.Unlock()
			p.publishMessagePart(chat.ID, string(fantasy.MessageRoleAssistant), codersdk.ChatMessagePart{
				Type:       codersdk.ChatMessagePartTypeToolCall,
				ToolCallID: id,
				ToolName:   toolName,
				ArgsDelta:  delta,
			})
			return nil
		},
		OnToolCall: func(toolCall fantasy.ToolCallContent) error {
			stepStateMu.Lock()
			streamToolNames[toolCall.ToolCallID] = toolCall.ToolName
			stepStateMu.Unlock()
			p.publishMessagePart(
				chat.ID,
				string(fantasy.MessageRoleAssistant),
				contentBlockToPart(toolCall),
			)
			return nil
		},
		OnSource: func(source fantasy.SourceContent) error {
			p.publishMessagePart(
				chat.ID,
				string(fantasy.MessageRoleAssistant),
				contentBlockToPart(source),
			)
			return nil
		},
		OnToolResult: func(result fantasy.ToolResultContent) error {
			toolResult := toolResultBlockFromAgentToolResult(result)
			p.publishMessagePart(chat.ID, string(fantasy.MessageRoleTool), toolResultToPart(toolResult))

			stepStateMu.Lock()
			stepToolResults = append(stepToolResults, toolResult)
			stepStateMu.Unlock()

			return nil
		},
		OnStepFinish: func(stepResult fantasy.StepResult) error {
			stepStateMu.Lock()
			toolResults := append([]ToolResultBlock(nil), stepToolResults...)
			stepStateMu.Unlock()

			if err := persistAssistant(stepAssistantContent(stepResult.Content, toolResults)); err != nil {
				return err
			}
			for _, toolResult := range toolResults {
				if err := persistToolResult(toolResult); err != nil {
					return err
				}
			}
			return nil
		},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) && errors.Is(context.Cause(ctx), ErrChatInterrupted) {
			return nil, ErrChatInterrupted
		}
		return nil, xerrors.Errorf("stream response: %w", err)
	}

	return result, nil
}

func stripAgentPromptSentinel(messages []fantasy.Message, sentinel string) []fantasy.Message {
	filtered := make([]fantasy.Message, 0, len(messages))
	removed := false
	for _, message := range messages {
		if !removed && isAgentPromptSentinelMessage(message, sentinel) {
			removed = true
			continue
		}
		filtered = append(filtered, message)
	}
	return filtered
}

func prepareAgentStepResult(
	messages []fantasy.Message,
	sentinel string,
	reportOnly bool,
) fantasy.PrepareStepResult {
	result := fantasy.PrepareStepResult{
		Messages: stripAgentPromptSentinel(messages, sentinel),
	}
	if reportOnly {
		result.ActiveTools = []string{toolAgentReport}
	}
	return result
}

func isAgentPromptSentinelMessage(message fantasy.Message, sentinel string) bool {
	if message.Role != fantasy.MessageRoleUser {
		return false
	}
	if len(message.Content) != 1 {
		return false
	}
	textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](message.Content[0])
	if !ok {
		return false
	}
	return textPart.Text == sentinel
}

func stepAssistantContent(content []fantasy.Content, toolResults []ToolResultBlock) []fantasy.Content {
	toolResultIDs := make(map[string]struct{}, len(toolResults))
	for _, toolResult := range toolResults {
		if toolResult.ToolCallID == "" {
			continue
		}
		toolResultIDs[toolResult.ToolCallID] = struct{}{}
	}

	filtered := make([]fantasy.Content, 0, len(content))
	for _, block := range content {
		toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
		if !ok {
			filtered = append(filtered, block)
			continue
		}
		if _, tracked := toolResultIDs[toolResult.ToolCallID]; tracked {
			continue
		}
		filtered = append(filtered, block)
	}
	return filtered
}

func (p *Processor) maybeGenerateChatTitle(
	ctx context.Context,
	chat database.Chat,
	messages []database.ChatMessage,
	logger slog.Logger,
) {
	titleInput, shouldGenerate, err := chatTitleInput(chat, messages)
	if err != nil {
		logger.Debug(ctx, "skipping AI title generation due to invalid user content",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return
	}
	if !shouldGenerate {
		return
	}

	titleCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	title, err := p.generateChatTitle(titleCtx, titleInput)
	if err != nil {
		logger.Debug(ctx, "failed to generate chat title",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return
	}
	if title == "" || title == chat.Title {
		return
	}

	_, err = p.db.UpdateChatByID(ctx, database.UpdateChatByIDParams{
		ID:    chat.ID,
		Title: title,
	})
	if err != nil {
		logger.Warn(ctx, "failed to update generated chat title",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
	}
}

func (p *Processor) generateChatTitle(ctx context.Context, input string) (string, error) {
	config := p.titleGeneration.withDefaults()
	keys, err := p.resolveProviderAPIKeys(ctx)
	if err != nil {
		return "", xerrors.Errorf("resolve provider API keys: %w", err)
	}
	modelLookup := p.titleModelLookup
	if modelLookup == nil {
		modelLookup = anyAvailableModel
	}
	model, err := modelLookup(keys)
	if err != nil {
		return "", xerrors.Errorf("resolve title generation model: %w", err)
	}

	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: config.Prompt},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: input},
			},
		},
	}
	toolChoice := fantasy.ToolChoiceNone
	response, err := model.Generate(ctx, fantasy.Call{
		Prompt:     prompt,
		ToolChoice: &toolChoice,
	})
	if err != nil {
		return "", xerrors.Errorf("generate title text: %w", err)
	}

	title := normalizeGeneratedChatTitle(contentBlocksToText(response.Content))
	if title == "" {
		return "", xerrors.New("generated title was empty")
	}
	return title, nil
}

func chatTitleInput(chat database.Chat, messages []database.ChatMessage) (string, bool, error) {
	userCount := 0
	firstUserText := ""

	for _, message := range messages {
		if message.Hidden {
			continue
		}

		switch message.Role {
		case string(fantasy.MessageRoleAssistant), string(fantasy.MessageRoleTool):
			return "", false, nil
		case string(fantasy.MessageRoleUser):
			userCount++
			if firstUserText == "" {
				text, err := userMessageText(message.Content)
				if err != nil {
					return "", false, err
				}
				firstUserText = text
			}
		}
	}

	if userCount != 1 || firstUserText == "" {
		return "", false, nil
	}

	currentTitle := strings.TrimSpace(chat.Title)
	if currentTitle == "" {
		return firstUserText, true, nil
	}

	if currentTitle != fallbackChatTitle(firstUserText) {
		return "", false, nil
	}

	return firstUserText, true, nil
}

func (p *Processor) resolveProviderAPIKeys(ctx context.Context) (ProviderAPIKeys, error) {
	if p.providerKeysFn == nil {
		return p.providerKeys, nil
	}
	return p.providerKeysFn(ctx)
}

func userMessageText(raw pqtype.NullRawMessage) (string, error) {
	content, err := parseContentBlocks(string(fantasy.MessageRoleUser), raw)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(contentBlocksToText(content)), nil
}

func contentBlocksToText(content []fantasy.Content) string {
	parts := make([]string, 0, len(content))
	for _, block := range content {
		textBlock, ok := fantasy.AsContentType[fantasy.TextContent](block)
		if !ok {
			continue
		}
		text := strings.TrimSpace(textBlock.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " ")
}

func normalizeGeneratedChatTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}

	title = strings.Trim(title, "\"'`")
	title = strings.Join(strings.Fields(title), " ")
	return truncateRunes(title, 80)
}

func fallbackChatTitle(message string) string {
	const maxWords = 6
	const maxRunes = 80

	words := strings.Fields(message)
	if len(words) == 0 {
		return "New Chat"
	}

	truncated := false
	if len(words) > maxWords {
		words = words[:maxWords]
		truncated = true
	}

	title := strings.Join(words, " ")
	if truncated {
		title += "…"
	}

	return truncateRunes(title, maxRunes)
}

func (p *Processor) recoverMissingWorkspace(
	ctx context.Context,
	chat database.Chat,
	logger slog.Logger,
) (database.Chat, error) {
	if !chat.WorkspaceID.Valid {
		return chat, nil
	}

	workspace, err := p.db.GetWorkspaceByID(ctx, chat.WorkspaceID.UUID)
	switch {
	case err == nil && !workspace.Deleted:
		return chat, nil
	case err == nil && workspace.Deleted:
		// Continue and clear workspace linkage for deleted workspaces.
	case errors.Is(err, sql.ErrNoRows):
		// Continue and clear workspace linkage for missing workspaces.
	default:
		return database.Chat{}, xerrors.Errorf("get chat workspace: %w", err)
	}

	updatedChat, err := p.persistChatWorkspace(ctx, chat, CreateWorkspaceToolResult{})
	if err != nil {
		return database.Chat{}, err
	}

	logger.Info(ctx, "chat workspace reference no longer exists; cleared workspace linkage",
		slog.F("workspace_id", chat.WorkspaceID.UUID),
	)

	return updatedChat, nil
}

func (p *Processor) resolveAgentID(ctx context.Context, chat database.Chat) (uuid.UUID, error) {
	if chat.WorkspaceAgentID.Valid {
		return chat.WorkspaceAgentID.UUID, nil
	}
	if !chat.WorkspaceID.Valid {
		return uuid.Nil, xerrors.New("chat has no workspace agent")
	}

	agents, err := p.db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, chat.WorkspaceID.UUID)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("get workspace agents: %w", err)
	}
	if len(agents) == 0 {
		return uuid.Nil, xerrors.New("no workspace agents available")
	}
	return agents[0].ID, nil
}

func (p *Processor) persistChatWorkspace(
	ctx context.Context,
	chat database.Chat,
	result CreateWorkspaceToolResult,
) (database.Chat, error) {
	updater, ok := p.db.(interface {
		UpdateChatWorkspace(context.Context, database.UpdateChatWorkspaceParams) (database.Chat, error)
	})
	if !ok {
		return database.Chat{}, xerrors.New("update chat workspace is not implemented by store")
	}

	updatedChat, err := updater.UpdateChatWorkspace(ctx, database.UpdateChatWorkspaceParams{
		ID: chat.ID,
		WorkspaceID: uuid.NullUUID{
			UUID:  result.WorkspaceID,
			Valid: result.WorkspaceID != uuid.Nil,
		},
		WorkspaceAgentID: uuid.NullUUID{
			UUID:  result.WorkspaceAgentID,
			Valid: result.WorkspaceAgentID != uuid.Nil,
		},
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("update chat workspace: %w", err)
	}
	return updatedChat, nil
}

func (p *Processor) recoverStaleChats(ctx context.Context) {
	staleThreshold := time.Now().Add(-p.staleThreshold)
	staleChats, err := p.db.GetStaleChats(ctx, staleThreshold)
	if err != nil {
		p.logger.Error(ctx, "failed to get stale chats", slog.Error(err))
		return
	}

	for _, chat := range staleChats {
		p.logger.Info(ctx, "recovering stale chat", slog.F("chat_id", chat.ID))

		// Reset to pending so any replica can pick it up.
		_, err := p.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
			ID:        chat.ID,
			Status:    database.ChatStatusPending,
			WorkerID:  uuid.NullUUID{},
			StartedAt: sql.NullTime{},
		})
		if err != nil {
			p.logger.Error(ctx, "failed to recover stale chat",
				slog.F("chat_id", chat.ID), slog.Error(err))
		}
	}

	if len(staleChats) > 0 {
		p.logger.Info(ctx, "recovered stale chats", slog.F("count", len(staleChats)))
	}
}

// Close stops the processor and waits for it to finish.
func (p *Processor) Close() error {
	p.cancel()
	<-p.closed
	p.inflight.Wait()
	return nil
}

func chatMessagesToPrompt(messages []database.ChatMessage) ([]fantasy.Message, error) {
	prompt := make([]fantasy.Message, 0, len(messages))
	for _, message := range messages {
		// System messages are always included in the prompt even when
		// hidden, because the system prompt must reach the LLM. Other
		// hidden messages (e.g. internal bookkeeping) are skipped.
		if message.Hidden && message.Role != string(fantasy.MessageRoleSystem) {
			continue
		}

		switch message.Role {
		case string(fantasy.MessageRoleSystem):
			content, err := parseSystemContent(message.Content)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(content) == "" {
				continue
			}
			prompt = append(prompt, fantasy.Message{
				Role: fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: content},
				},
			})
		case string(fantasy.MessageRoleUser):
			content, err := parseContentBlocks(string(fantasy.MessageRoleUser), message.Content)
			if err != nil {
				return nil, err
			}
			prompt = append(prompt, fantasy.Message{
				Role:    fantasy.MessageRoleUser,
				Content: contentToMessageParts(content),
			})
		case string(fantasy.MessageRoleAssistant):
			content, err := parseContentBlocks(string(fantasy.MessageRoleAssistant), message.Content)
			if err != nil {
				return nil, err
			}
			prompt = append(prompt, fantasy.Message{
				Role:    fantasy.MessageRoleAssistant,
				Content: contentToMessageParts(content),
			})
		case string(fantasy.MessageRoleTool):
			results, err := parseToolResults(message.Content)
			if err != nil {
				return nil, err
			}
			prompt = append(prompt, toolMessageFromResults(results))
		default:
			return nil, xerrors.Errorf("unsupported chat message role %q", message.Role)
		}
	}
	return injectMissingToolResults(prompt), nil
}

// injectMissingToolResults scans the prompt for assistant messages
// that contain tool calls without corresponding tool result messages
// and injects synthetic "interrupted" tool results. This can happen
// when a chat is interrupted mid-tool-call: the assistant message
// with tool_use blocks is persisted but the tool results are not.
// The Anthropic API requires every tool_use to have a matching
// tool_result in the immediately following message.
func injectMissingToolResults(prompt []fantasy.Message) []fantasy.Message {
	result := make([]fantasy.Message, 0, len(prompt))
	for i := 0; i < len(prompt); i++ {
		msg := prompt[i]
		result = append(result, msg)

		if msg.Role != fantasy.MessageRoleAssistant {
			continue
		}
		toolCalls := extractToolCallsFromMessageParts(msg.Content)
		if len(toolCalls) == 0 {
			continue
		}

		// Collect the tool call IDs that have results in the
		// following tool message(s).
		answered := make(map[string]struct{})
		j := i + 1
		for ; j < len(prompt); j++ {
			if prompt[j].Role != fantasy.MessageRoleTool {
				break
			}
			for _, part := range prompt[j].Content {
				tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
				if !ok {
					continue
				}
				answered[tr.ToolCallID] = struct{}{}
			}
		}
		if i+1 < j {
			// Preserve persisted tool result ordering and inject any
			// synthetic results after the existing contiguous tool messages.
			result = append(result, prompt[i+1:j]...)
			i = j - 1
		}

		// Build synthetic results for any unanswered tool calls.
		var missing []ToolResultBlock
		for _, tc := range toolCalls {
			if _, ok := answered[tc.ToolCallID]; !ok {
				missing = append(missing, ToolResultBlock{
					ToolCallID: tc.ToolCallID,
					ToolName:   tc.ToolName,
					Result:     map[string]any{"error": "tool call was interrupted and did not receive a result"},
					IsError:    true,
				})
			}
		}
		if len(missing) > 0 {
			result = append(result, toolMessageFromResults(missing))
		}
	}
	return result
}

func prependSystemInstruction(prompt []fantasy.Message, instruction string) []fantasy.Message {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return prompt
	}
	for _, message := range prompt {
		if message.Role != fantasy.MessageRoleSystem {
			continue
		}
		for _, part := range message.Content {
			textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](part)
			if !ok {
				continue
			}
			if strings.Contains(strings.ToLower(textPart.Text), "create_workspace") {
				return prompt
			}
		}
	}

	out := make([]fantasy.Message, 0, len(prompt)+1)
	out = append(out, fantasy.Message{
		Role: fantasy.MessageRoleSystem,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: instruction},
		},
	})
	out = append(out, prompt...)
	return out
}

func appendSystemInstruction(prompt []fantasy.Message, instruction string) []fantasy.Message {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return prompt
	}
	out := make([]fantasy.Message, 0, len(prompt)+1)
	out = append(out, prompt...)
	out = append(out, fantasy.Message{
		Role: fantasy.MessageRoleSystem,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: instruction},
		},
	})
	return out
}

func parseSystemContent(raw pqtype.NullRawMessage) (string, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return "", nil
	}

	var content string
	if err := json.Unmarshal(raw.RawMessage, &content); err != nil {
		return "", xerrors.Errorf("parse system message content: %w", err)
	}
	return content, nil
}

func parseContentBlocks(role string, raw pqtype.NullRawMessage) ([]fantasy.Content, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return nil, nil
	}

	var text string
	if err := json.Unmarshal(raw.RawMessage, &text); err == nil {
		return []fantasy.Content{fantasy.TextContent{Text: text}}, nil
	}

	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(raw.RawMessage, &rawBlocks); err != nil {
		return nil, xerrors.Errorf("parse %s content: %w", role, err)
	}

	content := make([]fantasy.Content, 0, len(rawBlocks))
	for i, rawBlock := range rawBlocks {
		block, err := fantasy.UnmarshalContent(rawBlock)
		if err != nil {
			return nil, xerrors.Errorf("parse %s content block %d: %w", role, i, err)
		}
		content = append(content, block)
	}
	return content, nil
}

func parseToolResults(raw pqtype.NullRawMessage) ([]ToolResultBlock, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return nil, nil
	}

	var results []ToolResultBlock
	if err := json.Unmarshal(raw.RawMessage, &results); err != nil {
		return nil, xerrors.Errorf("parse tool content: %w", err)
	}
	return results, nil
}

func contentToMessageParts(content []fantasy.Content) []fantasy.MessagePart {
	parts := make([]fantasy.MessagePart, 0, len(content))
	for _, block := range content {
		switch value := block.(type) {
		case fantasy.TextContent:
			parts = append(parts, fantasy.TextPart{Text: value.Text})
		case *fantasy.TextContent:
			parts = append(parts, fantasy.TextPart{Text: value.Text})
		case fantasy.ReasoningContent:
			parts = append(parts, fantasy.ReasoningPart{Text: value.Text})
		case *fantasy.ReasoningContent:
			parts = append(parts, fantasy.ReasoningPart{Text: value.Text})
		case fantasy.ToolCallContent:
			parts = append(parts, fantasy.ToolCallPart{
				ToolCallID:       sanitizeToolCallID(value.ToolCallID),
				ToolName:         value.ToolName,
				Input:            value.Input,
				ProviderExecuted: value.ProviderExecuted,
			})
		case *fantasy.ToolCallContent:
			parts = append(parts, fantasy.ToolCallPart{
				ToolCallID:       sanitizeToolCallID(value.ToolCallID),
				ToolName:         value.ToolName,
				Input:            value.Input,
				ProviderExecuted: value.ProviderExecuted,
			})
		case fantasy.FileContent:
			parts = append(parts, fantasy.FilePart{
				Data:      value.Data,
				MediaType: value.MediaType,
			})
		case *fantasy.FileContent:
			parts = append(parts, fantasy.FilePart{
				Data:      value.Data,
				MediaType: value.MediaType,
			})
		}
	}
	return parts
}

func toolMessageFromResults(results []ToolResultBlock) fantasy.Message {
	parts := make([]fantasy.MessagePart, 0, len(results))
	for _, result := range results {
		parts = append(parts, toolResultToMessagePart(result))
	}
	return fantasy.Message{
		Role:    fantasy.MessageRoleTool,
		Content: parts,
	}
}

func toolResultToMessagePart(result ToolResultBlock) fantasy.ToolResultPart {
	toolCallID := sanitizeToolCallID(result.ToolCallID)

	payload := result.Result
	if payload == nil {
		payload = map[string]any{}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte(`{}`)
	}

	if result.IsError {
		message := strings.TrimSpace(string(raw))
		if fields, ok := payload.(map[string]any); ok {
			if extracted, ok := fields["error"].(string); ok && strings.TrimSpace(extracted) != "" {
				message = extracted
			}
		}
		return fantasy.ToolResultPart{
			ToolCallID: toolCallID,
			Output: fantasy.ToolResultOutputContentError{
				Error: xerrors.New(message),
			},
		}
	}

	return fantasy.ToolResultPart{
		ToolCallID: toolCallID,
		Output: fantasy.ToolResultOutputContentText{
			Text: string(raw),
		},
	}
}

func sanitizeToolCallID(id string) string {
	if id == "" {
		return ""
	}
	return toolCallIDSanitizer.ReplaceAllString(id, "_")
}

func extractToolCallsFromMessageParts(parts []fantasy.MessagePart) []fantasy.ToolCallContent {
	toolCalls := make([]fantasy.ToolCallContent, 0, len(parts))
	for _, part := range parts {
		toolCall, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part)
		if !ok {
			continue
		}
		toolCalls = append(toolCalls, fantasy.ToolCallContent{
			ToolCallID:       toolCall.ToolCallID,
			ToolName:         toolCall.ToolName,
			Input:            toolCall.Input,
			ProviderExecuted: toolCall.ProviderExecuted,
		})
	}
	return toolCalls
}

func marshalContentBlocks(blocks []fantasy.Content) (pqtype.NullRawMessage, error) {
	if len(blocks) == 0 {
		return pqtype.NullRawMessage{}, nil
	}
	data, err := json.Marshal(blocks)
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("encode content blocks: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}, nil
}

func marshalToolResults(results []ToolResultBlock) (pqtype.NullRawMessage, error) {
	if len(results) == 0 {
		return pqtype.NullRawMessage{}, nil
	}
	data, err := json.Marshal(results)
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("encode tool results: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}, nil
}

func chatMessageParts(role string, raw pqtype.NullRawMessage) ([]codersdk.ChatMessagePart, error) {
	switch role {
	case string(fantasy.MessageRoleSystem):
		content, err := parseSystemContent(raw)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(content) == "" {
			return nil, nil
		}
		return []codersdk.ChatMessagePart{{
			Type: codersdk.ChatMessagePartTypeText,
			Text: content,
		}}, nil
	case string(fantasy.MessageRoleUser), string(fantasy.MessageRoleAssistant):
		content, err := parseContentBlocks(role, raw)
		if err != nil {
			return nil, err
		}
		parts := make([]codersdk.ChatMessagePart, 0, len(content))
		for _, block := range content {
			part := contentBlockToPart(block)
			if part.Type == "" {
				continue
			}
			parts = append(parts, part)
		}
		return parts, nil
	case string(fantasy.MessageRoleTool):
		results, err := parseToolResults(raw)
		if err != nil {
			return nil, err
		}
		parts := make([]codersdk.ChatMessagePart, 0, len(results))
		for _, result := range results {
			part := toolResultToPart(result)
			if part.Type == "" {
				continue
			}
			parts = append(parts, part)
		}
		return parts, nil
	default:
		return nil, nil
	}
}

func contentBlockToPart(block fantasy.Content) codersdk.ChatMessagePart {
	switch value := block.(type) {
	case fantasy.TextContent:
		return codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeText,
			Text: value.Text,
		}
	case *fantasy.TextContent:
		return codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeText,
			Text: value.Text,
		}
	case fantasy.ReasoningContent:
		return codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeReasoning,
			Text: value.Text,
		}
	case *fantasy.ReasoningContent:
		return codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeReasoning,
			Text: value.Text,
		}
	case fantasy.ToolCallContent:
		return codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: value.ToolCallID,
			ToolName:   value.ToolName,
			Args:       []byte(value.Input),
		}
	case *fantasy.ToolCallContent:
		return codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: value.ToolCallID,
			ToolName:   value.ToolName,
			Args:       []byte(value.Input),
		}
	case fantasy.SourceContent:
		return codersdk.ChatMessagePart{
			Type:     codersdk.ChatMessagePartTypeSource,
			SourceID: value.ID,
			URL:      value.URL,
			Title:    value.Title,
		}
	case *fantasy.SourceContent:
		return codersdk.ChatMessagePart{
			Type:     codersdk.ChatMessagePartTypeSource,
			SourceID: value.ID,
			URL:      value.URL,
			Title:    value.Title,
		}
	case fantasy.FileContent:
		return codersdk.ChatMessagePart{
			Type:      codersdk.ChatMessagePartTypeFile,
			MediaType: value.MediaType,
			Data:      value.Data,
		}
	case *fantasy.FileContent:
		return codersdk.ChatMessagePart{
			Type:      codersdk.ChatMessagePartTypeFile,
			MediaType: value.MediaType,
			Data:      value.Data,
		}
	case fantasy.ToolResultContent:
		return toolResultToPart(toolResultBlockFromContent(value))
	case *fantasy.ToolResultContent:
		return toolResultToPart(toolResultBlockFromContent(*value))
	default:
		return codersdk.ChatMessagePart{}
	}
}

func toolResultBlockFromContent(content fantasy.ToolResultContent) ToolResultBlock {
	result := ToolResultBlock{
		ToolCallID: content.ToolCallID,
		ToolName:   content.ToolName,
	}
	switch output := content.Result.(type) {
	case fantasy.ToolResultOutputContentError:
		result.IsError = true
		if output.Error != nil {
			result.Result = map[string]any{"error": output.Error.Error()}
		} else {
			result.Result = map[string]any{"error": ""}
		}
	case fantasy.ToolResultOutputContentText:
		decoded := map[string]any{}
		if err := json.Unmarshal([]byte(output.Text), &decoded); err == nil {
			result.Result = decoded
		} else {
			result.Result = map[string]any{"output": output.Text}
		}
	case fantasy.ToolResultOutputContentMedia:
		result.Result = map[string]any{
			"data":      output.Data,
			"mime_type": output.MediaType,
			"text":      output.Text,
		}
	default:
		result.Result = map[string]any{}
	}
	return result
}

func toolResultToPart(result ToolResultBlock) codersdk.ChatMessagePart {
	return codersdk.ChatMessagePart{
		Type:       codersdk.ChatMessagePartTypeToolResult,
		ToolCallID: result.ToolCallID,
		ToolName:   result.ToolName,
		Result:     toRawJSON(result.Result),
		IsError:    result.IsError,
		ResultMeta: toolResultMetadata(result.Result),
	}
}

func toRawJSON(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

func toolResultMetadata(value any) *codersdk.ChatToolResultMetadata {
	fields, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	meta := codersdk.ChatToolResultMetadata{}
	if s, ok := stringValue(fields["error"]); ok {
		meta.Error = s
	}
	if s, ok := stringValue(fields["output"]); ok {
		meta.Output = s
	}
	if n, ok := intValue(fields["exit_code"]); ok {
		meta.ExitCode = &n
	}
	if s, ok := stringValue(fields["content"]); ok {
		meta.Content = s
	}
	if s, ok := stringValue(fields["mime_type"]); ok {
		meta.MimeType = s
	}
	if b, ok := boolValue(fields["created"]); ok {
		meta.Created = &b
	}
	if s, ok := stringValue(fields["workspace_id"]); ok {
		meta.WorkspaceID = s
	}
	if s, ok := stringValue(fields["workspace_agent_id"]); ok {
		meta.WorkspaceAgentID = s
	}
	if s, ok := stringValue(fields["workspace_name"]); ok {
		meta.WorkspaceName = s
	}
	if s, ok := stringValue(fields["workspace_url"]); ok {
		meta.WorkspaceURL = s
	}
	if s, ok := stringValue(fields["reason"]); ok {
		meta.Reason = s
	}

	if meta.Error == "" &&
		meta.Output == "" &&
		meta.ExitCode == nil &&
		meta.Content == "" &&
		meta.MimeType == "" &&
		meta.Created == nil &&
		meta.WorkspaceID == "" &&
		meta.WorkspaceAgentID == "" &&
		meta.WorkspaceName == "" &&
		meta.WorkspaceURL == "" &&
		meta.Reason == "" {
		return nil
	}

	return &meta
}

func stringValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	default:
		return "", false
	}
}

func boolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	default:
		return false, false
	}
}

func intValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		n, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

type chatModelConfig struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model"`
}

func modelFromChat(chat database.Chat, providerKeys ProviderAPIKeys) (fantasy.LanguageModel, error) {
	config := chatModelConfig{}
	if len(chat.ModelConfig) > 0 {
		if err := json.Unmarshal(chat.ModelConfig, &config); err != nil {
			return nil, xerrors.Errorf("parse model config: %w", err)
		}
	}
	if strings.TrimSpace(config.Model) == "" {
		config.Model = defaultChatModel
	}
	return modelFromConfig(config, providerKeys)
}

func modelFromName(modelName string, providerKeys ProviderAPIKeys) (fantasy.LanguageModel, error) {
	return modelFromConfig(chatModelConfig{Model: modelName}, providerKeys)
}

// anyAvailableModel returns a language model from whichever provider
// has an API key configured. This is used for lightweight tasks like
// title generation where we don't need a specific model.
func anyAvailableModel(keys ProviderAPIKeys) (fantasy.LanguageModel, error) {
	candidates := []chatModelConfig{
		{Provider: fantasyopenai.Name, Model: "gpt-4o-mini"},
		{Provider: fantasyanthropic.Name, Model: "claude-haiku-4-5"},
		{Provider: fantasyazure.Name, Model: "gpt-4o-mini"},
		{Provider: fantasyopenrouter.Name, Model: "gpt-4o-mini"},
		{Provider: fantasyvercel.Name, Model: "gpt-4o-mini"},
		{Provider: fantasygoogle.Name, Model: "gemini-2.5-flash"},
		{Provider: fantasybedrock.Name, Model: "anthropic.claude-haiku-4-5-20251001-v1:0"},
		{Provider: fantasyopenaicompat.Name, Model: "gpt-4o-mini"},
	}

	var firstErr error
	for _, candidate := range candidates {
		if keys.apiKey(candidate.Provider) == "" {
			continue
		}

		model, err := modelFromConfig(candidate, keys)
		if err != nil {
			if firstErr == nil {
				firstErr = xerrors.Errorf(
					"initialize title model for provider %q: %w",
					candidate.Provider,
					err,
				)
			}
			continue
		}
		return model, nil
	}

	if firstErr != nil {
		return nil, firstErr
	}

	return nil, xerrors.New("no AI provider API keys are configured")
}

func modelFromConfig(config chatModelConfig, providerKeys ProviderAPIKeys) (fantasy.LanguageModel, error) {
	provider, modelID, err := resolveModelWithProviderHint(config.Model, config.Provider)
	if err != nil {
		return nil, err
	}

	apiKey := providerKeys.apiKey(provider)
	if apiKey == "" {
		return nil, missingProviderAPIKeyError(provider)
	}

	var providerClient fantasy.Provider
	switch provider {
	case fantasyanthropic.Name:
		providerClient, err = fantasyanthropic.New(fantasyanthropic.WithAPIKey(apiKey))
	case fantasyazure.Name:
		return nil, xerrors.New(
			"azure provider requires a base URL, but chat provider configs do not support base URLs yet",
		)
	case fantasybedrock.Name:
		providerClient, err = fantasybedrock.New(fantasybedrock.WithAPIKey(apiKey))
	case fantasygoogle.Name:
		providerClient, err = fantasygoogle.New(fantasygoogle.WithGeminiAPIKey(apiKey))
	case fantasyopenai.Name:
		providerClient, err = fantasyopenai.New(fantasyopenai.WithAPIKey(apiKey))
	case fantasyopenaicompat.Name:
		providerClient, err = fantasyopenaicompat.New(fantasyopenaicompat.WithAPIKey(apiKey))
	case fantasyopenrouter.Name:
		providerClient, err = fantasyopenrouter.New(fantasyopenrouter.WithAPIKey(apiKey))
	case fantasyvercel.Name:
		providerClient, err = fantasyvercel.New(fantasyvercel.WithAPIKey(apiKey))
	default:
		return nil, xerrors.Errorf("unsupported model provider %q", provider)
	}
	if err != nil {
		return nil, xerrors.Errorf("create %s provider: %w", provider, err)
	}

	model, err := providerClient.LanguageModel(context.Background(), modelID)
	if err != nil {
		return nil, xerrors.Errorf("load %s model: %w", provider, err)
	}
	return model, nil
}

func missingProviderAPIKeyError(provider string) error {
	switch provider {
	case fantasyanthropic.Name:
		return xerrors.New("ANTHROPIC_API_KEY is not set")
	case fantasyazure.Name:
		return xerrors.New("AZURE_OPENAI_API_KEY is not set")
	case fantasybedrock.Name:
		return xerrors.New("BEDROCK_API_KEY is not set")
	case fantasygoogle.Name:
		return xerrors.New("GOOGLE_API_KEY is not set")
	case fantasyopenai.Name:
		return xerrors.New("OPENAI_API_KEY is not set")
	case fantasyopenaicompat.Name:
		return xerrors.New("OPENAI_COMPAT_API_KEY is not set")
	case fantasyopenrouter.Name:
		return xerrors.New("OPENROUTER_API_KEY is not set")
	case fantasyvercel.Name:
		return xerrors.New("VERCEL_API_KEY is not set")
	default:
		return xerrors.Errorf("API key for provider %q is not set", provider)
	}
}

type readFileArgs struct {
	Path   string `json:"path"`
	Offset *int64 `json:"offset,omitempty"`
	Limit  *int64 `json:"limit,omitempty"`
}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type editFilesArgs struct {
	Files []workspacesdk.FileEdits `json:"files"`
}

type executeArgs struct {
	Command        string `json:"command"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
}

type createWorkspaceArgs struct {
	Prompt    string          `json:"prompt,omitempty"`
	Workspace json.RawMessage `json:"workspace,omitempty"`
	Request   json.RawMessage `json:"request,omitempty"`
}

type taskArgs struct {
	Prompt     string `json:"prompt"`
	Title      string `json:"title,omitempty"`
	Background bool   `json:"background,omitempty"`
}

type taskAwaitArgs struct {
	ChatID         string `json:"chat_id"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
}

type taskTerminateArgs struct {
	ChatID string `json:"chat_id"`
}

type agentReportArgs struct {
	Report string `json:"report"`
}

func (p *Processor) agentTools(
	model fantasy.LanguageModel,
	chatState *database.Chat,
	chatStateMu *sync.Mutex,
	getWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error),
) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		fantasy.NewAgentTool(
			toolCreateWorkspace,
			"Create a workspace when no workspace is selected, or when you need "+
				"a different template. Accepts a natural-language prompt and/or a "+
				"workspace request object.",
			func(ctx context.Context, _ createWorkspaceArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				toolCall := fantasy.ToolCallContent{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Input:      call.Input,
				}

				chatStateMu.Lock()
				chatSnapshot := *chatState
				chatStateMu.Unlock()

				toolResult, wsResult := p.executeCreateWorkspaceTool(ctx, chatSnapshot, model, toolCall)
				if wsResult.Created {
					if wsResult.WorkspaceID == uuid.Nil {
						toolResult = toolError(ToolResultBlock{
							ToolCallID: toolCall.ToolCallID,
							ToolName:   toolCall.ToolName,
						}, xerrors.New("workspace creator returned a created workspace without an ID"))
					} else {
						updatedChat, err := p.persistChatWorkspace(ctx, chatSnapshot, wsResult)
						if err != nil {
							toolResult = toolError(ToolResultBlock{
								ToolCallID: toolCall.ToolCallID,
								ToolName:   toolCall.ToolName,
							}, err)
						} else {
							chatStateMu.Lock()
							*chatState = updatedChat
							chatStateMu.Unlock()
						}
					}
				}
				return toolResultBlockToAgentResponse(toolResult), nil
			},
		),
		fantasy.NewAgentTool(
			toolReadFile,
			"Read a file from the workspace.",
			func(ctx context.Context, args readFileArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := toolResultBlockBaseFromAgentToolCall(call)
				conn, err := getWorkspaceConn(ctx)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				return toolResultBlockToAgentResponse(executeReadFileTool(ctx, conn, call.ID, args)), nil
			},
		),
		fantasy.NewAgentTool(
			toolWriteFile,
			"Write a file to the workspace.",
			func(ctx context.Context, args writeFileArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := toolResultBlockBaseFromAgentToolCall(call)
				conn, err := getWorkspaceConn(ctx)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				return toolResultBlockToAgentResponse(executeWriteFileTool(ctx, conn, call.ID, args)), nil
			},
		),
		fantasy.NewAgentTool(
			toolEditFiles,
			"Perform search-and-replace edits on one or more files in the workspace."+
				" Each file can have multiple edits applied atomically.",
			func(ctx context.Context, args editFilesArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := toolResultBlockBaseFromAgentToolCall(call)
				conn, err := getWorkspaceConn(ctx)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				return toolResultBlockToAgentResponse(executeEditFilesTool(ctx, conn, call.ID, args)), nil
			},
		),
		fantasy.NewAgentTool(
			toolExecute,
			"Execute a shell command in the workspace.",
			func(ctx context.Context, args executeArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := toolResultBlockBaseFromAgentToolCall(call)
				conn, err := getWorkspaceConn(ctx)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				return toolResultBlockToAgentResponse(executeExecuteTool(ctx, conn, call.ID, args)), nil
			},
		),
		fantasy.NewAgentTool(
			toolTask,
			"Create a delegated child task chat. If background=false, this call waits for the child report.",
			func(ctx context.Context, args taskArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := toolResultBlockBaseFromAgentToolCall(call)
				if p.taskService == nil {
					return toolResultBlockToAgentResponse(toolError(base, xerrors.New("task service is not configured"))), nil
				}

				chatStateMu.Lock()
				chatSnapshot := *chatState
				chatStateMu.Unlock()

				if chatSnapshot.ParentChatID.Valid {
					return toolResultBlockToAgentResponse(toolError(
						base,
						xerrors.New("delegated chats cannot create child tasks in phase-1"),
					)), nil
				}

				childChat, err := p.taskService.CreateChildTaskChat(
					ctx,
					chatSnapshot,
					args.Prompt,
					args.Title,
					args.Background,
				)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				payload := map[string]any{
					"chat_id":    childChat.ID.String(),
					"background": args.Background,
					"status":     string(childChat.TaskStatus),
				}
				if args.Background {
					return toolResultBlockToAgentResponse(ToolResultBlock{
						ToolCallID: call.ID,
						ToolName:   call.Name,
						Result:     payload,
					}), nil
				}

				report, err := p.taskService.AwaitTaskReport(
					ctx,
					chatSnapshot.ID,
					childChat.ID,
					defaultTaskAwaitTimeout,
				)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				payload["report"] = report
				payload["status"] = string(database.ChatTaskStatusReported)

				return toolResultBlockToAgentResponse(ToolResultBlock{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Result:     payload,
				}), nil
			},
		),
		fantasy.NewAgentTool(
			toolTaskAwait,
			"Wait for a delegated descendant task chat to report completion.",
			func(ctx context.Context, args taskAwaitArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := toolResultBlockBaseFromAgentToolCall(call)
				if p.taskService == nil {
					return toolResultBlockToAgentResponse(toolError(base, xerrors.New("task service is not configured"))), nil
				}

				chatID, err := parseTaskToolChatID(args.ChatID)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				timeout := defaultTaskAwaitTimeout
				if args.TimeoutSeconds != nil {
					timeout = time.Duration(*args.TimeoutSeconds) * time.Second
				}

				chatStateMu.Lock()
				chatSnapshot := *chatState
				chatStateMu.Unlock()

				report, err := p.taskService.AwaitTaskReport(
					ctx,
					chatSnapshot.ID,
					chatID,
					timeout,
				)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				return toolResultBlockToAgentResponse(ToolResultBlock{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Result: map[string]any{
						"chat_id": chatID.String(),
						"report":  report,
						"status":  string(database.ChatTaskStatusReported),
					},
				}), nil
			},
		),
		fantasy.NewAgentTool(
			toolTaskTerminate,
			"Terminate a delegated descendant task subtree.",
			func(ctx context.Context, args taskTerminateArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := toolResultBlockBaseFromAgentToolCall(call)
				if p.taskService == nil {
					return toolResultBlockToAgentResponse(toolError(base, xerrors.New("task service is not configured"))), nil
				}

				chatID, err := parseTaskToolChatID(args.ChatID)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				chatStateMu.Lock()
				chatSnapshot := *chatState
				chatStateMu.Unlock()

				if err := p.taskService.TerminateTaskSubtree(ctx, chatSnapshot.ID, chatID); err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				return toolResultBlockToAgentResponse(ToolResultBlock{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Result: map[string]any{
						"chat_id":    chatID.String(),
						"terminated": true,
						"status":     string(database.ChatTaskStatusReported),
					},
				}), nil
			},
		),
		fantasy.NewAgentTool(
			toolAgentReport,
			"Mark the current delegated task chat as reported when all descendants are complete.",
			func(ctx context.Context, args agentReportArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := toolResultBlockBaseFromAgentToolCall(call)
				if p.taskService == nil {
					return toolResultBlockToAgentResponse(toolError(base, xerrors.New("task service is not configured"))), nil
				}

				chatStateMu.Lock()
				chatSnapshot := *chatState
				chatStateMu.Unlock()

				hasActiveDescendants, err := p.taskService.HasActiveDescendants(ctx, chatSnapshot.ID)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				if hasActiveDescendants {
					return toolResultBlockToAgentResponse(toolError(
						base,
						xerrors.New("cannot report while active delegated descendants remain"),
					)), nil
				}

				if err := p.taskService.MarkTaskReported(ctx, chatSnapshot.ID, args.Report); err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				return toolResultBlockToAgentResponse(ToolResultBlock{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Result: map[string]any{
						"chat_id":  chatSnapshot.ID.String(),
						"title":    chatSnapshot.Title,
						"report":   args.Report,
						"reported": true,
						"status":   string(database.ChatTaskStatusReported),
					},
				}), nil
			},
		),
	}
}

func parseTaskToolChatID(raw string) (uuid.UUID, error) {
	chatID, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, xerrors.New("chat_id must be a valid UUID")
	}
	return chatID, nil
}

func (p *Processor) executeCreateWorkspaceTool(
	ctx context.Context,
	chat database.Chat,
	model fantasy.LanguageModel,
	toolCall fantasy.ToolCallContent,
) (ToolResultBlock, CreateWorkspaceToolResult) {
	base := ToolResultBlock{
		ToolCallID: toolCall.ToolCallID,
		ToolName:   toolCall.ToolName,
	}

	if p.workspaceCreator == nil {
		return toolError(base, xerrors.New("workspace creator is not configured")), CreateWorkspaceToolResult{}
	}

	args := createWorkspaceArgs{}
	if err := json.Unmarshal([]byte(toolCall.Input), &args); err != nil {
		return toolError(base, err), CreateWorkspaceToolResult{}
	}

	toolReq := CreateWorkspaceToolRequest{
		Chat:   chat,
		Model:  model,
		Prompt: strings.TrimSpace(args.Prompt),
		Spec:   json.RawMessage(toolCall.Input),
	}
	logEmitter := newCreateWorkspaceBuildLogEmitter(p, chat.ID, toolCall.ToolCallID, toolCall.ToolName)
	if logEmitter != nil {
		toolReq.BuildLogHandler = logEmitter.Emit
	}
	if len(args.Workspace) > 0 && string(args.Workspace) != "null" {
		toolReq.Spec = args.Workspace
	} else if len(args.Request) > 0 && string(args.Request) != "null" {
		toolReq.Spec = args.Request
	}

	wsResult, err := p.workspaceCreator.CreateWorkspace(ctx, toolReq)
	if err != nil {
		return toolError(base, err), CreateWorkspaceToolResult{}
	}

	if strings.TrimSpace(wsResult.Reason) == "" && !wsResult.Created {
		wsResult.Reason = "workspace was not created"
	}

	payload := map[string]any{
		"success": wsResult.Created,
		"created": wsResult.Created,
	}
	if wsResult.WorkspaceID != uuid.Nil {
		payload["workspace_id"] = wsResult.WorkspaceID.String()
	}
	if wsResult.WorkspaceAgentID != uuid.Nil {
		payload["workspace_agent_id"] = wsResult.WorkspaceAgentID.String()
	}
	if wsResult.WorkspaceName != "" {
		payload["workspace_name"] = wsResult.WorkspaceName
	}
	if wsResult.WorkspaceURL != "" {
		payload["workspace_url"] = wsResult.WorkspaceURL
	}
	if wsResult.Reason != "" {
		payload["reason"] = wsResult.Reason
	}

	return ToolResultBlock{
		ToolCallID: toolCall.ToolCallID,
		ToolName:   toolCall.ToolName,
		Result:     payload,
		IsError:    !wsResult.Created,
	}, wsResult
}

type createWorkspaceBuildLogEmitter struct {
	processor  *Processor
	chatID     uuid.UUID
	toolCallID string
	toolName   string
	lineCount  int
	charCount  int
	started    bool
	truncated  bool
}

func newCreateWorkspaceBuildLogEmitter(
	processor *Processor,
	chatID uuid.UUID,
	toolCallID string,
	toolName string,
) *createWorkspaceBuildLogEmitter {
	if processor == nil || toolCallID == "" {
		return nil
	}

	return &createWorkspaceBuildLogEmitter{
		processor:  processor,
		chatID:     chatID,
		toolCallID: toolCallID,
		toolName:   toolName,
	}
}

func (e *createWorkspaceBuildLogEmitter) Emit(entry CreateWorkspaceBuildLog) {
	if e == nil || e.truncated {
		return
	}

	output := strings.ReplaceAll(entry.Output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return
	}

	prefix := createWorkspaceBuildLogPrefix(entry)
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		e.emitLine(prefix + line)
		if e.truncated {
			return
		}
	}
}

func createWorkspaceBuildLogPrefix(entry CreateWorkspaceBuildLog) string {
	parts := []string{"build"}
	if stage := strings.TrimSpace(entry.Stage); stage != "" {
		parts = append(parts, stage)
	}
	if level := strings.TrimSpace(entry.Level); level != "" {
		parts = append(parts, level)
	}
	return "[" + strings.Join(parts, "/") + "] "
}

func (e *createWorkspaceBuildLogEmitter) emitLine(line string) {
	if e == nil || e.truncated {
		return
	}

	if !e.started {
		e.publishDelta("\n[workspace build logs]\n")
		e.started = true
	}

	line = truncateRunes(line, maxCreateWorkspaceBuildLogLineChars)
	if line == "" {
		return
	}

	delta := line + "\n"
	if e.lineCount >= maxCreateWorkspaceBuildLogLines || e.charCount+len(delta) > maxCreateWorkspaceBuildLogChars {
		e.publishDelta("[workspace build logs truncated]\n")
		e.truncated = true
		return
	}

	e.publishDelta(delta)
	e.lineCount++
	e.charCount += len(delta)
}

func (e *createWorkspaceBuildLogEmitter) publishDelta(delta string) {
	if e == nil || strings.TrimSpace(delta) == "" {
		return
	}

	e.processor.publishMessagePart(e.chatID, string(fantasy.MessageRoleTool), codersdk.ChatMessagePart{
		Type:        codersdk.ChatMessagePartTypeToolResult,
		ToolCallID:  e.toolCallID,
		ToolName:    e.toolName,
		ResultDelta: delta,
	})
}

func executeReadFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	toolCallID string,
	args readFileArgs,
) ToolResultBlock {
	result := toolResultBlockBase(toolCallID, toolReadFile)
	if args.Path == "" {
		return toolError(result, xerrors.New("path is required"))
	}

	offset := int64(0)
	limit := int64(0)
	if args.Offset != nil {
		offset = *args.Offset
	}
	if args.Limit != nil {
		limit = *args.Limit
	}

	reader, mimeType, err := conn.ReadFile(ctx, args.Path, offset, limit)
	if err != nil {
		return toolError(result, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return toolError(result, err)
	}

	result.Result = map[string]any{
		"content":   string(data),
		"mime_type": mimeType,
	}
	return result
}

func executeWriteFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	toolCallID string,
	args writeFileArgs,
) ToolResultBlock {
	result := toolResultBlockBase(toolCallID, toolWriteFile)
	if args.Path == "" {
		return toolError(result, xerrors.New("path is required"))
	}

	if err := conn.WriteFile(ctx, args.Path, strings.NewReader(args.Content)); err != nil {
		return toolError(result, err)
	}
	result.Result = map[string]any{"ok": true}
	return result
}

func executeEditFilesTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	toolCallID string,
	args editFilesArgs,
) ToolResultBlock {
	result := toolResultBlockBase(toolCallID, toolEditFiles)
	if len(args.Files) == 0 {
		return toolError(result, xerrors.New("files is required"))
	}

	if err := conn.EditFiles(ctx, workspacesdk.FileEditRequest{Files: args.Files}); err != nil {
		return toolError(result, err)
	}
	result.Result = map[string]any{"ok": true}
	return result
}

func executeExecuteTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	toolCallID string,
	args executeArgs,
) ToolResultBlock {
	result := toolResultBlockBase(toolCallID, toolExecute)
	if args.Command == "" {
		return toolError(result, xerrors.New("command is required"))
	}

	timeout := defaultExecuteTimeout
	if args.TimeoutSeconds != nil {
		timeout = time.Duration(*args.TimeoutSeconds) * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, exitCode, err := runCommand(cmdCtx, conn, args.Command)
	resultPayload := map[string]any{
		"output":    output,
		"exit_code": exitCode,
	}
	if err != nil {
		resultPayload["error"] = err.Error()
		result.IsError = true
	}
	result.Result = resultPayload
	return result
}

func toolResultBlockBase(toolCallID string, toolName string) ToolResultBlock {
	return ToolResultBlock{
		ToolCallID: toolCallID,
		ToolName:   toolName,
	}
}

func runCommand(ctx context.Context, conn workspacesdk.AgentConn, command string) (string, int, error) {
	sshClient, err := conn.SSHClient(ctx)
	if err != nil {
		return "", 0, err
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		return "", 0, err
	}
	defer session.Close()

	resultCh := make(chan struct {
		output   string
		exitCode int
		err      error
	}, 1)

	go func() {
		output, err := session.CombinedOutput(command)
		exitCode := 0
		if err != nil {
			var exitErr *ssh.ExitError
			if xerrors.As(err, &exitErr) {
				exitCode = exitErr.ExitStatus()
			} else {
				exitCode = 1
			}
		}
		resultCh <- struct {
			output   string
			exitCode int
			err      error
		}{
			output:   string(output),
			exitCode: exitCode,
			err:      err,
		}
	}()

	select {
	case <-ctx.Done():
		_ = session.Close()
		return "", 0, ctx.Err()
	case result := <-resultCh:
		return result.output, result.exitCode, result.err
	}
}

func toolError(result ToolResultBlock, err error) ToolResultBlock {
	result.IsError = true
	result.Result = map[string]any{"error": err.Error()}
	return result
}

func toolResultBlockBaseFromAgentToolCall(call fantasy.ToolCall) ToolResultBlock {
	return toolResultBlockBase(call.ID, call.Name)
}

func toolResultBlockToAgentResponse(result ToolResultBlock) fantasy.ToolResponse {
	content := ""
	if result.IsError {
		if fields, ok := result.Result.(map[string]any); ok {
			if extracted, ok := fields["error"].(string); ok && strings.TrimSpace(extracted) != "" {
				content = extracted
			}
		}
		if content == "" {
			if raw, err := json.Marshal(result.Result); err == nil {
				content = strings.TrimSpace(string(raw))
			}
		}
	} else if payload, err := json.Marshal(result.Result); err == nil {
		content = string(payload)
	}

	metadata := ""
	if encoded, err := json.Marshal(result); err == nil {
		metadata = string(encoded)
	}

	return fantasy.ToolResponse{
		Type:     "text",
		Content:  content,
		Metadata: metadata,
		IsError:  result.IsError,
	}
}

func toolResultBlockFromAgentToolResult(content fantasy.ToolResultContent) ToolResultBlock {
	if strings.TrimSpace(content.ClientMetadata) != "" {
		var block ToolResultBlock
		if err := json.Unmarshal([]byte(content.ClientMetadata), &block); err == nil {
			if block.ToolCallID == "" {
				block.ToolCallID = content.ToolCallID
			}
			if block.ToolName == "" {
				block.ToolName = content.ToolName
			}
			return block
		}
	}

	return toolResultBlockFromContent(content)
}

func SDKChatMessage(m database.ChatMessage) codersdk.ChatMessage {
	msg := codersdk.ChatMessage{
		ID:        m.ID,
		ChatID:    m.ChatID,
		CreatedAt: m.CreatedAt,
		Role:      m.Role,
		Hidden:    m.Hidden,
	}
	if m.Content.Valid {
		msg.Content = m.Content.RawMessage
		parts, err := chatMessageParts(m.Role, m.Content)
		if err == nil {
			msg.Parts = parts
		}
	}
	if m.ToolCallID.Valid {
		msg.ToolCallID = &m.ToolCallID.String
	}
	if m.Thinking.Valid {
		msg.Thinking = &m.Thinking.String
	}
	return msg
}
