package chatd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/openai/openai-go/v2"
	openaioption "github.com/openai/openai-go/v2/option"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"go.jetify.com/ai"
	"go.jetify.com/ai/api"
	aianthropic "go.jetify.com/ai/provider/anthropic"
	aiopenai "go.jetify.com/ai/provider/openai"

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
)

var ErrChatInterrupted = xerrors.New("chat interrupted")

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
	providerKeys     ProviderAPIKeys
	providerKeysFn   ProviderAPIKeysResolver
	titleGeneration  TitleGenerationConfig
	titleModelLookup func(ProviderAPIKeys) (api.LanguageModel, error)

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
	Model           api.LanguageModel
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

// ModelResolver resolves a model for a chat.
type ModelResolver func(chat database.Chat) (api.LanguageModel, error)

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

	var streamErr *api.ErrorEvent
	// streamChatResponse already publishes stream error events for api.ErrorEvent.
	if errors.As(err, &streamErr) {
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

	chatCtx, cancel := context.WithCancelCause(ctx)
	p.registerChat(chat.ID, cancel)
	defer p.unregisterChat(chat.ID)
	defer cancel(nil)

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

	if err := p.runChat(chatCtx, chat, logger); err != nil {
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

func (p *Processor) runChat(ctx context.Context, chat database.Chat, logger slog.Logger) error {
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
	if !chat.WorkspaceID.Valid {
		prompt = prependSystemInstruction(prompt, defaultNoWorkspaceInstruction)
	}

	tools := toolDefinitions()
	var streamHandler StreamEventHandler
	if p.streamManager != nil {
		p.streamManager.StartStream(chat.ID)
		defer p.streamManager.StopStream(chat.ID)
		streamHandler = func(event codersdk.ChatStreamEvent) {
			p.publishEvent(chat.ID, event)
		}
	}

	for step := 0; step < maxChatSteps; step++ {
		if err := ctx.Err(); err != nil {
			if errors.Is(context.Cause(ctx), ErrChatInterrupted) {
				return ErrChatInterrupted
			}
			return err
		}

		var (
			model api.LanguageModel
			err   error
		)
		if p.modelResolver != nil {
			model, err = p.modelResolver(chat)
		} else {
			keys, keyErr := p.resolveProviderAPIKeys(ctx)
			if keyErr != nil {
				return xerrors.Errorf("resolve provider API keys: %w", keyErr)
			}
			model, err = modelFromChat(chat, keys)
		}
		if err != nil {
			return xerrors.Errorf("resolve model: %w", err)
		}

		response, toolCalls, assistantMessage, done, err := func() (streamResult, []*api.ToolCallBlock, database.ChatMessage, bool, error) {
			response, err := streamChatResponse(ctx, model, prompt, tools, streamHandler)
			if err != nil {
				if errors.Is(err, context.Canceled) && errors.Is(context.Cause(ctx), ErrChatInterrupted) {
					return streamResult{}, nil, database.ChatMessage{}, false, ErrChatInterrupted
				}
				return streamResult{}, nil, database.ChatMessage{}, false, xerrors.Errorf("stream response: %w", err)
			}

			if len(response.Content) == 0 {
				return streamResult{}, nil, database.ChatMessage{}, true, nil
			}

			assistantContent, err := marshalContentBlocks(response.Content)
			if err != nil {
				return streamResult{}, nil, database.ChatMessage{}, false, err
			}

			assistantMessage, err := p.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
				ChatID:  chat.ID,
				Role:    string(api.MessageRoleAssistant),
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
				return streamResult{}, nil, database.ChatMessage{}, false, xerrors.Errorf("insert assistant message: %w", err)
			}

			return response, extractToolCalls(response.Content), assistantMessage, false, nil
		}()
		if err != nil {
			return err
		}
		if done {
			return nil
		}

		p.publishMessage(chat.ID, assistantMessage)

		prompt = append(prompt, &api.AssistantMessage{Content: response.Content})

		if len(toolCalls) == 0 {
			return nil
		}

		var (
			conn    workspacesdk.AgentConn
			release func()
		)
		releaseConn := func() {
			if release != nil {
				release()
				release = nil
			}
		}

		for _, toolCall := range toolCalls {
			var toolResult api.ToolResultBlock

			switch toolCall.ToolName {
			case toolCreateWorkspace:
				wsResult := CreateWorkspaceToolResult{}
				toolResult, wsResult = p.executeCreateWorkspaceTool(ctx, chat, model, toolCall)
				if wsResult.Created {
					if wsResult.WorkspaceID == uuid.Nil {
						toolResult = toolError(api.ToolResultBlock{
							ToolCallID: toolCall.ToolCallID,
							ToolName:   toolCall.ToolName,
						}, xerrors.New("workspace creator returned a created workspace without an ID"))
						break
					}

					updatedChat, err := p.persistChatWorkspace(ctx, chat, wsResult)
					if err != nil {
						toolResult = toolError(api.ToolResultBlock{
							ToolCallID: toolCall.ToolCallID,
							ToolName:   toolCall.ToolName,
						}, err)
						break
					}
					chat = updatedChat
				}
			default:
				base := api.ToolResultBlock{
					ToolCallID: toolCall.ToolCallID,
					ToolName:   toolCall.ToolName,
				}

				if p.agentConnector == nil {
					toolResult = toolError(base, xerrors.New("workspace agent connector is not configured"))
					break
				}

				if conn == nil {
					agentID, err := p.resolveAgentID(ctx, chat)
					if err != nil {
						toolResult = toolError(base, err)
						break
					}
					agentConn, agentRelease, err := p.agentConnector.AgentConn(ctx, agentID)
					if err != nil {
						toolResult = toolError(base, xerrors.Errorf("connect to workspace agent: %w", err))
						break
					}
					conn = agentConn
					release = agentRelease
				}

				toolResult = executeTool(ctx, conn, toolCall)
			}

			p.publishMessagePart(chat.ID, string(api.MessageRoleTool), toolResultToPart(toolResult))

			resultContent, err := marshalToolResults([]api.ToolResultBlock{toolResult})
			if err != nil {
				releaseConn()
				return err
			}

			toolMessage, err := p.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
				ChatID:  chat.ID,
				Role:    string(api.MessageRoleTool),
				Content: resultContent,
				ToolCallID: sql.NullString{
					String: toolResult.ToolCallID,
					Valid:  toolResult.ToolCallID != "",
				},
				Hidden: false,
			})
			if err != nil {
				releaseConn()
				return xerrors.Errorf("insert tool result: %w", err)
			}

			p.publishMessage(chat.ID, toolMessage)

			prompt = append(prompt, &api.ToolMessage{Content: []api.ToolResultBlock{toolResult}})
		}

		releaseConn()
	}

	return xerrors.Errorf("chat exceeded %d tool steps", maxChatSteps)
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

	prompt := []api.Message{
		&api.SystemMessage{Content: config.Prompt},
		&api.UserMessage{Content: api.ContentFromText(input)},
	}
	response, err := ai.GenerateText(ctx, prompt, ai.WithModel(model))
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
		case string(api.MessageRoleAssistant), string(api.MessageRoleTool):
			return "", false, nil
		case string(api.MessageRoleUser):
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
	content, err := parseContentBlocks(string(api.MessageRoleUser), raw)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(contentBlocksToText(content)), nil
}

func contentBlocksToText(content []api.ContentBlock) string {
	parts := make([]string, 0, len(content))
	for _, block := range content {
		textBlock, ok := block.(*api.TextBlock)
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

func chatMessagesToPrompt(messages []database.ChatMessage) ([]api.Message, error) {
	prompt := make([]api.Message, 0, len(messages))
	for _, message := range messages {
		// System messages are always included in the prompt even when
		// hidden, because the system prompt must reach the LLM. Other
		// hidden messages (e.g. internal bookkeeping) are skipped.
		if message.Hidden && message.Role != string(api.MessageRoleSystem) {
			continue
		}

		switch message.Role {
		case string(api.MessageRoleSystem):
			content, err := parseSystemContent(message.Content)
			if err != nil {
				return nil, err
			}
			prompt = append(prompt, &api.SystemMessage{Content: content})
		case string(api.MessageRoleUser):
			content, err := parseContentBlocks(string(api.MessageRoleUser), message.Content)
			if err != nil {
				return nil, err
			}
			prompt = append(prompt, &api.UserMessage{Content: content})
		case string(api.MessageRoleAssistant):
			content, err := parseContentBlocks(string(api.MessageRoleAssistant), message.Content)
			if err != nil {
				return nil, err
			}
			prompt = append(prompt, &api.AssistantMessage{Content: content})
		case string(api.MessageRoleTool):
			results, err := parseToolResults(message.Content)
			if err != nil {
				return nil, err
			}
			prompt = append(prompt, &api.ToolMessage{Content: results})
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
func injectMissingToolResults(prompt []api.Message) []api.Message {
	result := make([]api.Message, 0, len(prompt))
	for i := 0; i < len(prompt); i++ {
		msg := prompt[i]
		result = append(result, msg)

		assistantMsg, ok := msg.(*api.AssistantMessage)
		if !ok {
			continue
		}
		toolCalls := extractToolCalls(assistantMsg.Content)
		if len(toolCalls) == 0 {
			continue
		}

		// Collect the tool call IDs that have results in the
		// following tool message(s).
		answered := make(map[string]struct{})
		j := i + 1
		for ; j < len(prompt); j++ {
			toolMsg, ok := prompt[j].(*api.ToolMessage)
			if !ok {
				break
			}
			for _, tr := range toolMsg.Content {
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
		var missing []api.ToolResultBlock
		for _, tc := range toolCalls {
			if _, ok := answered[tc.ToolCallID]; !ok {
				missing = append(missing, api.ToolResultBlock{
					ToolCallID: tc.ToolCallID,
					ToolName:   tc.ToolName,
					Result:     map[string]any{"error": "tool call was interrupted and did not receive a result"},
					IsError:    true,
				})
			}
		}
		if len(missing) > 0 {
			result = append(result, &api.ToolMessage{Content: missing})
		}
	}
	return result
}

func prependSystemInstruction(prompt []api.Message, instruction string) []api.Message {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return prompt
	}
	for _, message := range prompt {
		systemMessage, ok := message.(*api.SystemMessage)
		if !ok {
			continue
		}
		if strings.Contains(strings.ToLower(systemMessage.Content), "create_workspace") {
			return prompt
		}
	}

	out := make([]api.Message, 0, len(prompt)+1)
	out = append(out, &api.SystemMessage{Content: instruction})
	out = append(out, prompt...)
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

func parseContentBlocks(role string, raw pqtype.NullRawMessage) ([]api.ContentBlock, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return nil, nil
	}

	var text string
	if err := json.Unmarshal(raw.RawMessage, &text); err == nil {
		return api.ContentFromText(text), nil
	}

	payload := struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content,omitempty"`
	}{
		Role:    role,
		Content: raw.RawMessage,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, xerrors.Errorf("encode content payload: %w", err)
	}

	switch role {
	case string(api.MessageRoleUser):
		var message api.UserMessage
		if err := json.Unmarshal(data, &message); err != nil {
			return nil, xerrors.Errorf("parse user content: %w", err)
		}
		return message.Content, nil
	case string(api.MessageRoleAssistant):
		var message api.AssistantMessage
		if err := json.Unmarshal(data, &message); err != nil {
			return nil, xerrors.Errorf("parse assistant content: %w", err)
		}
		return message.Content, nil
	default:
		return nil, xerrors.Errorf("unsupported role for content blocks %q", role)
	}
}

func parseToolResults(raw pqtype.NullRawMessage) ([]api.ToolResultBlock, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return nil, nil
	}

	payload := struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content,omitempty"`
	}{
		Role:    string(api.MessageRoleTool),
		Content: raw.RawMessage,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, xerrors.Errorf("encode tool content payload: %w", err)
	}

	var message api.ToolMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return nil, xerrors.Errorf("parse tool content: %w", err)
	}
	return message.Content, nil
}

func marshalContentBlocks(blocks []api.ContentBlock) (pqtype.NullRawMessage, error) {
	if len(blocks) == 0 {
		return pqtype.NullRawMessage{}, nil
	}
	data, err := json.Marshal(blocks)
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("encode content blocks: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}, nil
}

func marshalToolResults(results []api.ToolResultBlock) (pqtype.NullRawMessage, error) {
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
	case string(api.MessageRoleSystem):
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
	case string(api.MessageRoleUser), string(api.MessageRoleAssistant):
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
	case string(api.MessageRoleTool):
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

func contentBlockToPart(block api.ContentBlock) codersdk.ChatMessagePart {
	switch value := block.(type) {
	case *api.TextBlock:
		return codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeText,
			Text: value.Text,
		}
	case *api.ReasoningBlock:
		return codersdk.ChatMessagePart{
			Type:      codersdk.ChatMessagePartTypeReasoning,
			Text:      value.Text,
			Signature: value.Signature,
		}
	case *api.ToolCallBlock:
		return codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: value.ToolCallID,
			ToolName:   value.ToolName,
			Args:       value.Args,
		}
	case *api.ToolResultBlock:
		return toolResultToPart(*value)
	case *api.SourceBlock:
		return codersdk.ChatMessagePart{
			Type:     codersdk.ChatMessagePartTypeSource,
			SourceID: value.ID,
			URL:      value.URL,
			Title:    value.Title,
		}
	case *api.FileBlock:
		return codersdk.ChatMessagePart{
			Type:      codersdk.ChatMessagePartTypeFile,
			MediaType: value.MediaType,
			Data:      value.Data,
		}
	default:
		return codersdk.ChatMessagePart{}
	}
}

func toolResultToPart(result api.ToolResultBlock) codersdk.ChatMessagePart {
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

func modelFromChat(chat database.Chat, providerKeys ProviderAPIKeys) (api.LanguageModel, error) {
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

func modelFromName(modelName string, providerKeys ProviderAPIKeys) (api.LanguageModel, error) {
	return modelFromConfig(chatModelConfig{Model: modelName}, providerKeys)
}

// anyAvailableModel returns a language model from whichever provider
// has an API key configured. This is used for lightweight tasks like
// title generation where we don't need a specific model.
func anyAvailableModel(keys ProviderAPIKeys) (api.LanguageModel, error) {
	if key := keys.apiKey(aiopenai.ProviderName); key != "" {
		client := openai.NewClient(openaioption.WithAPIKey(key))
		return aiopenai.NewLanguageModel("gpt-4o-mini", aiopenai.WithClient(client)), nil
	}
	if key := keys.apiKey(aianthropic.ProviderName); key != "" {
		client := anthropic.NewClient(anthropicoption.WithAPIKey(key))
		return aianthropic.NewLanguageModel("claude-haiku-4-5", aianthropic.WithClient(client)), nil
	}
	return nil, xerrors.New("no AI provider API keys are configured")
}

func modelFromConfig(config chatModelConfig, providerKeys ProviderAPIKeys) (api.LanguageModel, error) {
	provider, modelID, err := resolveModelWithProviderHint(config.Model, config.Provider)
	if err != nil {
		return nil, err
	}

	switch provider {
	case aianthropic.ProviderName:
		apiKey := providerKeys.apiKey(provider)
		if apiKey == "" {
			return nil, xerrors.New("ANTHROPIC_API_KEY is not set")
		}
		client := anthropic.NewClient(anthropicoption.WithAPIKey(apiKey))
		return aianthropic.NewLanguageModel(modelID, aianthropic.WithClient(client)), nil
	case aiopenai.ProviderName:
		apiKey := providerKeys.apiKey(provider)
		if apiKey == "" {
			return nil, xerrors.New("OPENAI_API_KEY is not set")
		}
		client := openai.NewClient(openaioption.WithAPIKey(apiKey))
		return aiopenai.NewLanguageModel(modelID, aiopenai.WithClient(client)), nil
	default:
		return nil, xerrors.Errorf("unsupported model provider %q", provider)
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

func toolDefinitions() []api.ToolDefinition {
	return []api.ToolDefinition{
		&api.FunctionTool{
			Name: toolCreateWorkspace,
			Description: "Create a workspace when no workspace is selected, or when you need a different template. " +
				"Accepts a natural-language prompt and/or a workspace request object.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"prompt":    {Type: "string"},
					"workspace": {Type: "object"},
					"request":   {Type: "object"},
				},
			},
		},
		&api.FunctionTool{
			Name:        toolReadFile,
			Description: "Read a file from the workspace.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"path":   {Type: "string"},
					"offset": {Type: "integer"},
					"limit":  {Type: "integer"},
				},
				Required: []string{"path"},
			},
		},
		&api.FunctionTool{
			Name:        toolWriteFile,
			Description: "Write a file to the workspace.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"path":    {Type: "string"},
					"content": {Type: "string"},
				},
				Required: []string{"path", "content"},
			},
		},
		&api.FunctionTool{
			Name:        toolEditFiles,
			Description: "Perform search-and-replace edits on one or more files in the workspace. Each file can have multiple edits applied atomically.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"files": {
						Type:        "array",
						Description: "An array of file edit operations.",
						Items: &jsonschema.Schema{
							Type: "object",
							Properties: map[string]*jsonschema.Schema{
								"path": {Type: "string", Description: "The absolute path of the file to edit."},
								"edits": {
									Type:        "array",
									Description: "An array of search/replace pairs to apply to the file.",
									Items: &jsonschema.Schema{
										Type: "object",
										Properties: map[string]*jsonschema.Schema{
											"search":  {Type: "string", Description: "The exact string to search for."},
											"replace": {Type: "string", Description: "The string to replace it with."},
										},
										Required: []string{"search", "replace"},
									},
								},
							},
							Required: []string{"path", "edits"},
						},
					},
				},
				Required: []string{"files"},
			},
		},
		&api.FunctionTool{
			Name:        toolExecute,
			Description: "Execute a shell command in the workspace.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"command":         {Type: "string"},
					"timeout_seconds": {Type: "integer"},
				},
				Required: []string{"command"},
			},
		},
	}
}

func (p *Processor) executeCreateWorkspaceTool(
	ctx context.Context,
	chat database.Chat,
	model api.LanguageModel,
	toolCall *api.ToolCallBlock,
) (api.ToolResultBlock, CreateWorkspaceToolResult) {
	base := api.ToolResultBlock{
		ToolCallID: toolCall.ToolCallID,
		ToolName:   toolCall.ToolName,
	}

	if p.workspaceCreator == nil {
		return toolError(base, xerrors.New("workspace creator is not configured")), CreateWorkspaceToolResult{}
	}

	args := createWorkspaceArgs{}
	if err := json.Unmarshal(toolCall.Args, &args); err != nil {
		return toolError(base, err), CreateWorkspaceToolResult{}
	}

	toolReq := CreateWorkspaceToolRequest{
		Chat:   chat,
		Model:  model,
		Prompt: strings.TrimSpace(args.Prompt),
		Spec:   toolCall.Args,
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

	return api.ToolResultBlock{
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

	e.processor.publishMessagePart(e.chatID, string(api.MessageRoleTool), codersdk.ChatMessagePart{
		Type:        codersdk.ChatMessagePartTypeToolResult,
		ToolCallID:  e.toolCallID,
		ToolName:    e.toolName,
		ResultDelta: delta,
	})
}

func executeTool(ctx context.Context, conn workspacesdk.AgentConn, toolCall *api.ToolCallBlock) api.ToolResultBlock {
	result := api.ToolResultBlock{
		ToolCallID: toolCall.ToolCallID,
		ToolName:   toolCall.ToolName,
	}

	switch toolCall.ToolName {
	case toolReadFile:
		args := readFileArgs{}
		if err := json.Unmarshal(toolCall.Args, &args); err != nil {
			return toolError(result, err)
		}
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
	case toolWriteFile:
		args := writeFileArgs{}
		if err := json.Unmarshal(toolCall.Args, &args); err != nil {
			return toolError(result, err)
		}
		if args.Path == "" {
			return toolError(result, xerrors.New("path is required"))
		}

		if err := conn.WriteFile(ctx, args.Path, strings.NewReader(args.Content)); err != nil {
			return toolError(result, err)
		}
		result.Result = map[string]any{"ok": true}
		return result
	case toolEditFiles:
		args := editFilesArgs{}
		if err := json.Unmarshal(toolCall.Args, &args); err != nil {
			return toolError(result, err)
		}
		if len(args.Files) == 0 {
			return toolError(result, xerrors.New("files is required"))
		}

		if err := conn.EditFiles(ctx, workspacesdk.FileEditRequest{Files: args.Files}); err != nil {
			return toolError(result, err)
		}
		result.Result = map[string]any{"ok": true}
		return result
	case toolExecute:
		args := executeArgs{}
		if err := json.Unmarshal(toolCall.Args, &args); err != nil {
			return toolError(result, err)
		}
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
	default:
		return toolError(result, xerrors.Errorf("unsupported tool %q", toolCall.ToolName))
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

func toolError(result api.ToolResultBlock, err error) api.ToolResultBlock {
	result.IsError = true
	result.Result = map[string]any{"error": err.Error()}
	return result
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

type streamResult struct {
	Content []api.ContentBlock
	Usage   api.Usage
}

type StreamEventHandler func(event codersdk.ChatStreamEvent)

type toolCallDelta struct {
	name string
	args bytes.Buffer
}

func streamChatResponse(
	ctx context.Context,
	model api.LanguageModel,
	prompt []api.Message,
	tools []api.ToolDefinition,
	handler StreamEventHandler,
) (streamResult, error) {
	response, err := ai.StreamText(ctx, prompt, ai.WithModel(model), ai.WithTools(tools...))
	if err != nil {
		var unsupported *api.UnsupportedFunctionalityError
		if errors.As(err, &unsupported) {
			generated, genErr := generateChatResponse(ctx, model, prompt, tools)
			if genErr != nil {
				return streamResult{}, genErr
			}
			if handler != nil {
				for _, block := range generated.Content {
					part := contentBlockToPart(block)
					if part.Type == "" {
						continue
					}
					handler(codersdk.ChatStreamEvent{
						Type: codersdk.ChatStreamEventTypeMessagePart,
						MessagePart: &codersdk.ChatStreamMessagePart{
							Role: string(api.MessageRoleAssistant),
							Part: part,
						},
					})
				}
			}
			return generated, nil
		}
		return streamResult{}, err
	}

	var result streamResult
	var currentText *api.TextBlock
	var currentReasoning *api.ReasoningBlock
	toolCallDeltas := map[string]*toolCallDelta{}
	toolCallIDs := map[string]struct{}{}

	emit := func(event codersdk.ChatStreamEvent) {
		if handler != nil {
			handler(event)
		}
	}
	emitPart := func(role string, part codersdk.ChatMessagePart) {
		if part.Type == "" {
			return
		}
		emit(codersdk.ChatStreamEvent{
			Type: codersdk.ChatStreamEventTypeMessagePart,
			MessagePart: &codersdk.ChatStreamMessagePart{
				Role: role,
				Part: part,
			},
		})
	}

	for event := range response.Stream {
		switch payload := event.(type) {
		case *api.TextDeltaEvent:
			if currentText == nil {
				currentText = &api.TextBlock{}
				result.Content = append(result.Content, currentText)
			}
			currentText.Text += payload.TextDelta
			emitPart(string(api.MessageRoleAssistant), codersdk.ChatMessagePart{
				Type: codersdk.ChatMessagePartTypeText,
				Text: payload.TextDelta,
			})
		case *api.ReasoningEvent:
			if currentReasoning == nil {
				currentReasoning = &api.ReasoningBlock{}
				result.Content = append(result.Content, currentReasoning)
			}
			currentReasoning.Text += payload.TextDelta
			emitPart(string(api.MessageRoleAssistant), codersdk.ChatMessagePart{
				Type: codersdk.ChatMessagePartTypeReasoning,
				Text: payload.TextDelta,
			})
		case *api.ReasoningSignatureEvent:
			if currentReasoning != nil {
				currentReasoning.Signature = payload.Signature
				emitPart(string(api.MessageRoleAssistant), codersdk.ChatMessagePart{
					Type:      codersdk.ChatMessagePartTypeReasoning,
					Signature: payload.Signature,
				})
			}
		case *api.SourceEvent:
			source := &api.SourceBlock{
				ID:    payload.Source.ID,
				URL:   payload.Source.URL,
				Title: payload.Source.Title,
			}
			result.Content = append(result.Content, source)
			emitPart(string(api.MessageRoleAssistant), contentBlockToPart(source))
		case *api.FileEvent:
			file := &api.FileBlock{
				Data:      payload.Data,
				MediaType: payload.MediaType,
			}
			result.Content = append(result.Content, file)
			emitPart(string(api.MessageRoleAssistant), contentBlockToPart(file))
		case *api.ToolCallDeltaEvent:
			delta := toolCallDeltas[payload.ToolCallID]
			if delta == nil {
				delta = &toolCallDelta{name: payload.ToolName}
				toolCallDeltas[payload.ToolCallID] = delta
			}
			delta.args.Write(payload.ArgsDelta)
			emitPart(string(api.MessageRoleAssistant), codersdk.ChatMessagePart{
				Type:       codersdk.ChatMessagePartTypeToolCall,
				ToolCallID: payload.ToolCallID,
				ToolName:   payload.ToolName,
				ArgsDelta:  string(payload.ArgsDelta),
			})
		case *api.ToolCallEvent:
			call := &api.ToolCallBlock{
				ToolCallID: payload.ToolCallID,
				ToolName:   payload.ToolName,
				Args:       payload.Args,
			}
			result.Content = append(result.Content, call)
			toolCallIDs[payload.ToolCallID] = struct{}{}
			emitPart(string(api.MessageRoleAssistant), contentBlockToPart(call))
		case *api.ResponseMetadataEvent:
			continue
		case *api.FinishEvent:
			result.Usage = payload.Usage
		case *api.ErrorEvent:
			emit(codersdk.ChatStreamEvent{
				Type:  codersdk.ChatStreamEventTypeError,
				Error: &codersdk.ChatStreamError{Message: payload.Error()},
			})
			return streamResult{}, payload
		}
	}

	for toolCallID, delta := range toolCallDeltas {
		if _, ok := toolCallIDs[toolCallID]; ok {
			continue
		}
		call := &api.ToolCallBlock{
			ToolCallID: toolCallID,
			ToolName:   delta.name,
			Args:       delta.args.Bytes(),
		}
		result.Content = append(result.Content, call)
		emitPart(string(api.MessageRoleAssistant), contentBlockToPart(call))
	}

	return result, nil
}

func generateChatResponse(
	ctx context.Context,
	model api.LanguageModel,
	prompt []api.Message,
	tools []api.ToolDefinition,
) (streamResult, error) {
	response, err := ai.GenerateText(ctx, prompt, ai.WithModel(model), ai.WithTools(tools...))
	if err != nil {
		return streamResult{}, err
	}

	result := streamResult{
		Content: response.Content,
		Usage:   response.Usage,
	}
	return result, nil
}

func extractToolCalls(content []api.ContentBlock) []*api.ToolCallBlock {
	var toolCalls []*api.ToolCallBlock
	for _, block := range content {
		if call, ok := block.(*api.ToolCallBlock); ok {
			toolCalls = append(toolCalls, call)
		}
	}
	return toolCalls
}
