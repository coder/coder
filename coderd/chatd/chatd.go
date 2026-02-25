package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	"github.com/coder/coder/v2/coderd/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	// DefaultPendingChatAcquireInterval is the default time between attempts to
	// acquire pending chats.
	DefaultPendingChatAcquireInterval = time.Second
	// DefaultInFlightChatStaleAfter is the default age after which a running
	// chat is considered stale and should be recovered.
	DefaultInFlightChatStaleAfter = 5 * time.Minute

	defaultExecuteTimeout        = 60 * time.Second
	defaultExternalAuthWait      = 5 * time.Minute
	homeInstructionLookupTimeout = 5 * time.Second
	maxChatSteps                 = 1200

	defaultContextCompressionThresholdPercent = int32(70)

	maxCreateWorkspaceBuildLogLines     = 120
	maxCreateWorkspaceBuildLogChars     = 16 * 1024
	maxCreateWorkspaceBuildLogLineChars = 240

	defaultTitleGenerationPrompt = "Generate a concise title (max 8 words, under 128 characters) for " +
		"the user's first message. Return plain text only — no quotes, no emoji, " +
		"no markdown, no special characters."

	defaultNoWorkspaceInstruction = "No workspace is selected yet. Call the create_workspace tool first before using read_file, write_file, or execute. If create_workspace fails, ask the user to clarify the template or workspace request."
	reportOnlyInstruction         = "Report-only follow-up pass. Call subagent_report exactly once with a concise summary and nothing else."

	chatAgentEnvVar                 = "CODER_CHAT_AGENT"
	gitAuthRequiredMarkerPrefix     = "CODER_GITAUTH_REQUIRED:"
	authRequiredResultReason        = "authentication_required"
	externalAuthWaitPollInterval    = time.Second
	externalAuthWaitTimedOutStatus  = "timed_out"
	externalAuthWaitCompletedStatus = "completed"
)

// Processor handles background processing of pending chats.
type Processor struct {
	cancel   context.CancelFunc
	closed   chan struct{}
	inflight sync.WaitGroup

	db       database.Store
	workerID uuid.UUID
	logger   slog.Logger

	agentConnFn              AgentConnFunc
	createWorkspaceFn        CreateWorkspaceFunc
	testing                  *TestingConfig
	streamManager            *StreamManager
	pubsub                   pubsub.Pubsub
	localMode                *localMode
	subagentService          *SubagentService
	resolveProviderAPIKeysFn ProviderAPIKeysResolver
	titleGeneration          TitleGenerationConfig
	titleModelLookup         func(chatprovider.ProviderAPIKeys) (fantasy.LanguageModel, error)

	activeMu      sync.Mutex
	activeCancels map[uuid.UUID]context.CancelCauseFunc

	// Configuration
	pendingChatAcquireInterval time.Duration
	inFlightChatStaleAfter     time.Duration
}

// AgentConnFunc provides access to workspace agent connections.
type AgentConnFunc func(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error)

// CreateWorkspaceFunc creates workspaces for chats when none are selected.
type CreateWorkspaceFunc func(
	ctx context.Context,
	req CreateWorkspaceToolRequest,
) (CreateWorkspaceToolResult, error)

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

// ProviderAPIKeysResolver resolves provider API keys for chat model calls.
type ProviderAPIKeysResolver func(context.Context) (chatprovider.ProviderAPIKeys, error)

// TestingConfig contains hooks intended only for tests.
type TestingConfig struct {
	ResolveChatModel func(chat database.Chat) (fantasy.LanguageModel, error)
}

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
	if event.Type == codersdk.ChatStreamEventTypeMessagePart {
		if !state.buffering {
			m.mu.Unlock()
			return
		}
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

// Config configures a chat processor.
type Config struct {
	Logger                     slog.Logger
	Database                   database.Store
	PendingChatAcquireInterval time.Duration
	InFlightChatStaleAfter     time.Duration
	AgentConn                  AgentConnFunc
	CreateWorkspace            CreateWorkspaceFunc
	StreamManager              *StreamManager
	Pubsub                     pubsub.Pubsub
	ResolveProviderAPIKeys     ProviderAPIKeysResolver
	TitleGeneration            TitleGenerationConfig
	Local                      *LocalConfig
	Testing                    *TestingConfig
}

// LocalConfig enables local workspace mode integration in chatd.
type LocalConfig struct {
	AccessURL    *url.URL
	HTTPClient   *http.Client
	DeploymentID string
}

// New creates a new chat processor. The processor polls for pending
// chats and processes them. It is the caller's responsibility to call Close
// on the returned instance.
func New(cfg Config) *Processor {
	ctx, cancel := context.WithCancel(context.Background())

	pendingChatAcquireInterval := cfg.PendingChatAcquireInterval
	if pendingChatAcquireInterval == 0 {
		pendingChatAcquireInterval = DefaultPendingChatAcquireInterval
	}

	inFlightChatStaleAfter := cfg.InFlightChatStaleAfter
	if inFlightChatStaleAfter == 0 {
		inFlightChatStaleAfter = DefaultInFlightChatStaleAfter
	}

	resolveProviderAPIKeys := cfg.ResolveProviderAPIKeys
	if resolveProviderAPIKeys == nil {
		resolveProviderAPIKeys = func(context.Context) (chatprovider.ProviderAPIKeys, error) {
			return chatprovider.ProviderAPIKeys{}, nil
		}
	}
	streamManager := cfg.StreamManager
	if streamManager == nil {
		streamManager = NewStreamManager(cfg.Logger.Named("chat-streams"))
	}

	var localMode *localMode
	if cfg.Local != nil {
		localMode = newLocalMode(localModeOptions{
			logger:       cfg.Logger.Named("chat-local"),
			database:     cfg.Database,
			accessURL:    cfg.Local.AccessURL,
			httpClient:   cfg.Local.HTTPClient,
			deploymentID: strings.TrimSpace(cfg.Local.DeploymentID),
		})
	}

	p := &Processor{
		cancel:                     cancel,
		closed:                     make(chan struct{}),
		db:                         cfg.Database,
		workerID:                   uuid.New(),
		logger:                     cfg.Logger.Named("chat-processor"),
		agentConnFn:                cfg.AgentConn,
		createWorkspaceFn:          cfg.CreateWorkspace,
		testing:                    cfg.Testing,
		streamManager:              streamManager,
		pubsub:                     cfg.Pubsub,
		localMode:                  localMode,
		resolveProviderAPIKeysFn:   resolveProviderAPIKeys,
		titleGeneration:            cfg.TitleGeneration.withDefaults(),
		titleModelLookup:           anyAvailableModel,
		activeCancels:              make(map[uuid.UUID]context.CancelCauseFunc),
		pendingChatAcquireInterval: pendingChatAcquireInterval,
		inFlightChatStaleAfter:     inFlightChatStaleAfter,
	}

	p.subagentService = newSubagentService(p.db, p.InterruptChat)
	p.subagentService.setStreamManager(p.streamManager)

	//nolint:gocritic // The chat processor is a system-level service.
	ctx = dbauthz.AsSystemRestricted(ctx)
	go p.start(ctx)

	return p
}

func (p *Processor) start(ctx context.Context) {
	defer close(p.closed)

	// First, recover any stale chats from crashed workers.
	p.recoverStaleChats(ctx)

	ticker := time.NewTicker(p.pendingChatAcquireInterval)
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
	cancel(chatloop.ErrInterrupted)
	if p.streamManager != nil {
		p.streamManager.StopStream(chatID)
	}
	return true
}

// IsChatActive reports whether the processor currently has an in-flight
// worker for the chat.
func (p *Processor) IsChatActive(chatID uuid.UUID) bool {
	if p == nil {
		return false
	}
	p.activeMu.Lock()
	_, ok := p.activeCancels[chatID]
	p.activeMu.Unlock()
	return ok
}

func (p *Processor) EnsureLocalWorkspaceBinding(
	ctx context.Context,
	ownerID uuid.UUID,
	sessionToken string,
) (LocalWorkspaceBinding, error) {
	if p == nil || p.localMode == nil {
		return LocalWorkspaceBinding{}, xerrors.New("local chat mode is not configured")
	}
	return p.localMode.EnsureWorkspaceBinding(ctx, ownerID, sessionToken)
}

func (p *Processor) EnsureLocalAgentRuntimeForChat(
	ctx context.Context,
	chat database.Chat,
) error {
	if workspaceModeFromChat(chat) != codersdk.ChatWorkspaceModeLocal {
		return nil
	}
	if p == nil || p.localMode == nil {
		return xerrors.New("local chat mode is not configured")
	}
	return p.localMode.MaybeLaunchAgentForChat(ctx, chat)
}

// ListModels returns the currently available chat model catalog.
func (p *Processor) ListModels(ctx context.Context) (codersdk.ChatModelsResponse, error) {
	if p == nil {
		return codersdk.ChatModelsResponse{}, xerrors.New("chat processor is not configured")
	}
	//nolint:gocritic // Background chat processor has no user context.
	ctx = dbauthz.AsSystemRestricted(ctx)

	enabledProviders, err := p.db.GetEnabledChatProviders(ctx)
	if err != nil {
		return codersdk.ChatModelsResponse{}, err
	}
	enabledModels, err := p.db.GetEnabledChatModelConfigs(ctx)
	if err != nil {
		return codersdk.ChatModelsResponse{}, err
	}

	configuredProviders := make([]chatprovider.ConfiguredProvider, 0, len(enabledProviders))
	for _, provider := range enabledProviders {
		configuredProviders = append(configuredProviders, chatprovider.ConfiguredProvider{
			Provider: provider.Provider,
			APIKey:   provider.APIKey,
			BaseURL:  provider.BaseUrl,
		})
	}
	configuredModels := make([]chatprovider.ConfiguredModel, 0, len(enabledModels))
	for _, model := range enabledModels {
		configuredModels = append(configuredModels, chatprovider.ConfiguredModel{
			Provider:    model.Provider,
			Model:       model.Model,
			DisplayName: model.DisplayName,
		})
	}

	keys, err := p.resolveProviderAPIKeys(ctx)
	if err != nil {
		return codersdk.ChatModelsResponse{}, err
	}
	catalog := chatprovider.NewModelCatalog(keys)
	if response, ok := catalog.ListConfiguredModels(configuredProviders, configuredModels); ok {
		return response, nil
	}
	return catalog.ListConfiguredProviderAvailability(configuredProviders), nil
}

// EffectiveProviderKeys merges configured provider keys over resolver keys.
func (p *Processor) EffectiveProviderKeys(
	ctx context.Context,
	configuredProviders []chatprovider.ConfiguredProvider,
) (chatprovider.ProviderAPIKeys, error) {
	if p == nil {
		return chatprovider.ProviderAPIKeys{}, xerrors.New("chat processor is not configured")
	}
	//nolint:gocritic // All authenticated users need to list available models.
	ctx = dbauthz.AsSystemRestricted(ctx)
	keys, err := p.resolveProviderAPIKeys(ctx)
	if err != nil {
		return chatprovider.ProviderAPIKeys{}, err
	}
	return chatprovider.MergeProviderAPIKeys(keys, configuredProviders), nil
}

func (p *Processor) PublishStream(chatID uuid.UUID, event codersdk.ChatStreamEvent) {
	if p == nil || p.streamManager == nil {
		return
	}
	p.streamManager.Publish(chatID, event)
}

func (p *Processor) SubscribeStream(chatID uuid.UUID) (
	[]codersdk.ChatStreamEvent,
	<-chan codersdk.ChatStreamEvent,
	func(),
	bool,
) {
	if p == nil || p.streamManager == nil {
		return nil, nil, nil, false
	}
	snapshot, events, cancel := p.streamManager.Subscribe(chatID)
	return snapshot, events, cancel, true
}

func (p *Processor) StopStream(chatID uuid.UUID) {
	if p == nil || p.streamManager == nil {
		return
	}
	p.streamManager.StopStream(chatID)
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

// publishChatPubsubEvent broadcasts a chat lifecycle event via PostgreSQL
// pubsub so that all replicas can push updates to watching clients.
func (p *Processor) publishChatPubsubEvent(chat database.Chat, kind coderdpubsub.ChatEventKind) {
	if p.pubsub == nil {
		return
	}
	event := coderdpubsub.ChatEvent{
		Kind: kind,
		Chat: convertChatForPubsub(chat),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		p.logger.Error(context.Background(), "failed to marshal chat pubsub event",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return
	}
	if err := p.pubsub.Publish(coderdpubsub.ChatEventChannel(chat.OwnerID), payload); err != nil {
		p.logger.Error(context.Background(), "failed to publish chat pubsub event",
			slog.F("chat_id", chat.ID),
			slog.F("kind", kind),
			slog.Error(err),
		)
	}
}

// convertChatForPubsub converts a database.Chat to codersdk.Chat for pubsub events.
// This is a lightweight conversion — we don't include diff status since
// the watch subscriber can fetch that separately if needed.
func convertChatForPubsub(c database.Chat) codersdk.Chat {
	chat := codersdk.Chat{
		ID:        c.ID,
		OwnerID:   c.OwnerID,
		Title:     c.Title,
		Status:    codersdk.ChatStatus(c.Status),
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
	if c.ParentChatID.Valid {
		parentChatID := c.ParentChatID.UUID
		chat.ParentChatID = &parentChatID
	}
	if c.RootChatID.Valid {
		rootChatID := c.RootChatID.UUID
		chat.RootChatID = &rootChatID
	} else if !c.ParentChatID.Valid {
		rootChatID := c.ID
		chat.RootChatID = &rootChatID
	}
	if c.WorkspaceID.Valid {
		chat.WorkspaceID = &c.WorkspaceID.UUID
	}
	if c.WorkspaceAgentID.Valid {
		chat.WorkspaceAgentID = &c.WorkspaceAgentID.UUID
	}
	return chat
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
	sdkMessage := db2sdk.ChatMessage(message)
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
	reportOnly := false
	activeSubagentRequestID := uuid.Nil
	if chat.ParentChatID.Valid && p.subagentService != nil {
		pendingRequestID, hasPending, err := p.subagentService.LatestPendingRequestID(ctx, chat.ID)
		if err != nil {
			logger.Warn(ctx, "failed to get pending delegated request", slog.Error(err))
		} else if hasPending {
			activeSubagentRequestID = pendingRequestID
			reportOnly, err = p.subagentService.ShouldRunReportOnlyPass(
				ctx,
				chat.ID,
				pendingRequestID,
			)
			if err != nil {
				logger.Warn(ctx, "failed to determine delegated report-only mode", slog.Error(err))
				reportOnly = false
			}
		}
	}

	chatCtx, cancel := context.WithCancelCause(ctx)
	p.registerChat(chat.ID, cancel)
	defer p.unregisterChat(chat.ID)
	defer cancel(nil)

	p.publishStatus(chat.ID, database.ChatStatusRunning)
	if p.subagentService != nil {
		p.subagentService.publishChildStatus(chat, database.ChatStatusRunning)
	}

	// Determine the final status to set when we're done.
	status := database.ChatStatusWaiting

	defer func() {
		// Handle panics gracefully.
		if r := recover(); r != nil {
			logger.Error(ctx, "panic during chat processing", slog.F("panic", r))
			p.publishError(chat.ID, panicFailureReason(r))
			status = database.ChatStatusError
		}

		if chat.ParentChatID.Valid && p.subagentService != nil {
			hasActiveDescendants, err := p.subagentService.HasActiveDescendants(ctx, chat.ID)
			if err != nil {
				logger.Warn(ctx, "failed to check delegated chat descendants", slog.Error(err))
			} else if !hasActiveDescendants {
				pendingRequestID, hasPending, pendingErr := p.subagentService.LatestPendingRequestID(ctx, chat.ID)
				if pendingErr != nil {
					logger.Warn(ctx, "failed to get pending delegated request", slog.Error(pendingErr))
				} else if hasPending {
					if reportOnly {
						report := p.subagentService.SynthesizeFallbackSubagentReport(
							ctx,
							chat.ID,
							pendingRequestID,
						)
						_, markErr := p.subagentService.MarkSubagentReported(
							ctx,
							chat.ID,
							report,
							uuid.NullUUID{UUID: pendingRequestID, Valid: true},
						)
						if markErr != nil {
							logger.Warn(ctx, "failed to mark delegated chat reported with fallback", slog.Error(markErr))
						}
					} else {
						if err := p.subagentService.MarkReportOnlyPassRequested(ctx, chat.ID, pendingRequestID); err != nil {
							logger.Warn(ctx, "failed to request delegated report-only pass", slog.Error(err))
						} else {
							status = database.ChatStatusPending
						}
					}
				}
			}

			if status == database.ChatStatusWaiting {
				hasPendingRequest, pendingErr := p.subagentService.HasPendingRequest(ctx, chat.ID)
				if pendingErr != nil {
					logger.Warn(ctx, "failed to check delegated pending request", slog.Error(pendingErr))
				} else if hasPendingRequest {
					status = database.ChatStatusPending
				}
			}
		}

		// Check for queued messages and auto-promote the next one.
		// This must be done atomically with the status update to avoid
		// races with the promote endpoint (which also sets status to
		// pending). We use a transaction with FOR UPDATE to ensure we
		// don't overwrite a status change made by another caller.
		err := p.db.InTx(func(tx database.Store) error {
			// Re-read the chat status under lock — another caller
			// (e.g. promote) may have already set it to pending.
			latestChat, lockErr := tx.GetChatByIDForUpdate(ctx, chat.ID)
			if lockErr != nil {
				return xerrors.Errorf("lock chat for release: %w", lockErr)
			}

			// If someone else already set the chat to pending (e.g.
			// the promote endpoint), don't overwrite it — just clear
			// the worker and let the processor pick it back up.
			if latestChat.Status == database.ChatStatusPending && status == database.ChatStatusWaiting {
				status = database.ChatStatusPending
			} else if status == database.ChatStatusWaiting {
				// Try to auto-promote the next queued message.
				nextQueued, popErr := tx.PopNextQueuedMessage(ctx, chat.ID)
				if popErr == nil {
					msg, insertErr := tx.InsertChatMessage(ctx, database.InsertChatMessageParams{
						ChatID: chat.ID,
						Role:   "user",
						Content: pqtype.NullRawMessage{
							RawMessage: nextQueued.Content,
							Valid:      len(nextQueued.Content) > 0,
						},
						Hidden: false,
						ToolCallID: sql.NullString{},
						Thinking: sql.NullString{},
						SubagentRequestID: uuid.NullUUID{},
						SubagentEvent: sql.NullString{},
						InputTokens: sql.NullInt64{},
						OutputTokens: sql.NullInt64{},
						TotalTokens: sql.NullInt64{},
						ReasoningTokens: sql.NullInt64{},
						CacheCreationTokens: sql.NullInt64{},
						CacheReadTokens: sql.NullInt64{},
						ContextLimit: sql.NullInt64{},
						Compressed: sql.NullBool{},
					})
					if insertErr != nil {
						logger.Error(ctx, "failed to promote queued message",
							slog.F("queued_message_id", nextQueued.ID), slog.Error(insertErr))
					} else {
						status = database.ChatStatusPending

						sdkMsg := db2sdk.ChatMessage(msg)
						p.publishEvent(chat.ID, codersdk.ChatStreamEvent{
							Type:    codersdk.ChatStreamEventTypeMessage,
							Message: &sdkMsg,
						})

						remaining, qErr := tx.GetChatQueuedMessages(ctx, chat.ID)
						if qErr == nil {
							sdkQueued := make([]codersdk.ChatQueuedMessage, 0, len(remaining))
							for _, q := range remaining {
								sdkQueued = append(sdkQueued, codersdk.ChatQueuedMessage{
									ID:        q.ID,
									ChatID:    q.ChatID,
									Content:   q.Content,
									CreatedAt: q.CreatedAt,
								})
							}
							p.publishEvent(chat.ID, codersdk.ChatStreamEvent{
								Type:           codersdk.ChatStreamEventTypeQueueUpdate,
								QueuedMessages: sdkQueued,
							})
						}
					}
				}
			}

			_, updateErr := tx.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
				ID:        chat.ID,
				Status:    status,
				WorkerID:  uuid.NullUUID{},
				StartedAt: sql.NullTime{},
			})
			return updateErr
		}, nil)
		if err != nil {
			logger.Error(ctx, "failed to release chat", slog.Error(err))
		}

		p.publishStatus(chat.ID, status)
		chat.Status = status
		p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindStatusChange)
		if p.subagentService != nil {
			p.subagentService.publishChildStatus(chat, status)
		}
	}()

	if err := p.runChat(chatCtx, chat, logger, reportOnly, activeSubagentRequestID); err != nil {
		if errors.Is(err, chatloop.ErrInterrupted) || errors.Is(context.Cause(chatCtx), chatloop.ErrInterrupted) {
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
	subagentRequestID uuid.UUID,
) error {
	messages, err := p.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	if err != nil {
		return xerrors.Errorf("get chat messages: %w", err)
	}
	p.maybeGenerateChatTitle(ctx, chat, messages, logger)

	chat, err = p.recoverMissingWorkspace(ctx, chat, logger)
	if err != nil {
		return err
	}
	if err := p.EnsureLocalAgentRuntimeForChat(ctx, chat); err != nil {
		return xerrors.Errorf("ensure local chat runtime: %w", err)
	}

	prompt, err := chatprompt.ConvertMessages(
		messages,
		subagentReportToolCallIDPrefix,
	)
	if err != nil {
		return xerrors.Errorf("build chat prompt: %w", err)
	}
	if reportOnly {
		prompt = chatprompt.AppendUser(prompt, reportOnlyInstruction)
	} else if workspaceModeFromChat(chat) != codersdk.ChatWorkspaceModeLocal &&
		!chat.WorkspaceID.Valid {
		prompt = chatprompt.PrependSystem(prompt, defaultNoWorkspaceInstruction)
	}

	if p.streamManager != nil {
		p.streamManager.StartStream(chat.ID)
		defer p.streamManager.StopStream(chat.ID)
	}

	model, modelConfig, err := p.resolveChatModel(ctx, chat)
	if err != nil {
		return err
	}

	result, err := p.runChatWithAgent(
		ctx,
		chat,
		logger,
		model,
		modelConfig,
		prompt,
		reportOnly,
		subagentRequestID,
	)
	if err != nil {
		return err
	}
	if hasExceededChatToolSteps(result) {
		return xerrors.Errorf("chat exceeded %d tool steps", maxChatSteps)
	}
	return nil
}

type chatContextCompressionConfig struct {
	ContextLimit     int64
	ThresholdPercent int32
}

func (p *Processor) resolveChatContextCompressionConfig(
	ctx context.Context,
	chat database.Chat,
) (chatContextCompressionConfig, error) {
	config := chatContextCompressionConfig{
		ThresholdPercent: defaultContextCompressionThresholdPercent,
	}

	chatConfig, err := parseChatModelConfig(chat.ModelConfig)
	if err != nil {
		return config, nil
	}

	if chatConfig.ContextLimit > 0 {
		config.ContextLimit = chatConfig.ContextLimit
	}
	modelName := strings.TrimSpace(chatConfig.Model)
	providerHint := strings.TrimSpace(chatConfig.Provider)
	if modelName == "" {
		keys, resolveKeysErr := p.resolveProviderAPIKeys(ctx)
		if resolveKeysErr != nil {
			return config, nil
		}

		fallbackConfig, fallbackErr := p.resolveFallbackChatModelConfig(
			ctx,
			keys,
			providerHint,
		)
		if fallbackErr != nil {
			return config, nil
		}

		if config.ContextLimit <= 0 {
			config.ContextLimit = fallbackConfig.ContextLimit
		}
		config.ThresholdPercent = fallbackConfig.CompressionThreshold
		return config, nil
	}

	provider, modelID, err := chatprovider.ResolveModelWithProviderHint(
		modelName,
		providerHint,
	)
	if err != nil {
		return config, nil
	}

	modelConfig, err := p.db.GetChatModelConfigByProviderAndModel(
		ctx,
		database.GetChatModelConfigByProviderAndModelParams{
			Provider: provider,
			Model:    modelID,
		},
	)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return config, nil
		}
		return config, xerrors.Errorf("load model compression config: %w", err)
	}

	if config.ContextLimit <= 0 {
		config.ContextLimit = modelConfig.ContextLimit
	}
	config.ThresholdPercent = modelConfig.CompressionThreshold
	return config, nil
}

func (p *Processor) persistChatContextSummary(
	ctx context.Context,
	chatID uuid.UUID,
	result chatloop.CompactionResult,
) error {
	if strings.TrimSpace(result.SystemSummary) == "" ||
		strings.TrimSpace(result.SummaryReport) == "" {
		return nil
	}

	systemContent, err := json.Marshal(result.SystemSummary)
	if err != nil {
		return xerrors.Errorf("encode system summary: %w", err)
	}

	_, err = p.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID: chatID,
		Role:   string(fantasy.MessageRoleSystem),
		Content: pqtype.NullRawMessage{
			RawMessage: systemContent,
			Valid:      len(systemContent) > 0,
		},
		Hidden:     true,
		Compressed: sql.NullBool{Bool: true, Valid: true},
		ToolCallID: sql.NullString{},
		Thinking: sql.NullString{},
		SubagentRequestID: uuid.NullUUID{},
		SubagentEvent: sql.NullString{},
		InputTokens: sql.NullInt64{},
		OutputTokens: sql.NullInt64{},
		TotalTokens: sql.NullInt64{},
		ReasoningTokens: sql.NullInt64{},
		CacheCreationTokens: sql.NullInt64{},
		CacheReadTokens: sql.NullInt64{},
		ContextLimit: sql.NullInt64{},
	})
	if err != nil {
		return xerrors.Errorf("insert hidden summary message: %w", err)
	}

	toolCallID := "chat_summarized_" + uuid.NewString()
	args, err := json.Marshal(map[string]any{
		"source":            "automatic",
		"threshold_percent": result.ThresholdPercent,
	})
	if err != nil {
		return xerrors.Errorf("encode summary tool args: %w", err)
	}

	assistantContent, err := chatprompt.MarshalContent([]fantasy.Content{
		fantasy.ToolCallContent{
			ToolCallID: toolCallID,
			ToolName:   "chat_summarized",
			Input:      string(args),
		},
	})
	if err != nil {
		return xerrors.Errorf("encode summary tool call: %w", err)
	}

	assistantMessage, err := p.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:  chatID,
		Role:    string(fantasy.MessageRoleAssistant),
		Content: assistantContent,
		Hidden:  false,
		Compressed: sql.NullBool{
			Bool:  true,
			Valid: true,
		},
		ToolCallID: sql.NullString{},
		Thinking: sql.NullString{},
		SubagentRequestID: uuid.NullUUID{},
		SubagentEvent: sql.NullString{},
		InputTokens: sql.NullInt64{},
		OutputTokens: sql.NullInt64{},
		TotalTokens: sql.NullInt64{},
		ReasoningTokens: sql.NullInt64{},
		CacheCreationTokens: sql.NullInt64{},
		CacheReadTokens: sql.NullInt64{},
		ContextLimit: sql.NullInt64{},
	})
	if err != nil {
		return xerrors.Errorf("insert summary tool call message: %w", err)
	}

	toolResult, err := chatprompt.MarshalResults([]chatprompt.ToolResultBlock{{
		ToolCallID: toolCallID,
		ToolName:   "chat_summarized",
		Result: map[string]any{
			"summary":              result.SummaryReport,
			"source":               "automatic",
			"threshold_percent":    result.ThresholdPercent,
			"usage_percent":        result.UsagePercent,
			"context_tokens":       result.ContextTokens,
			"context_limit_tokens": result.ContextLimit,
		},
	}})
	if err != nil {
		return xerrors.Errorf("encode summary tool result: %w", err)
	}

	toolMessage, err := p.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:  chatID,
		Role:    string(fantasy.MessageRoleTool),
		Content: toolResult,
		ToolCallID: sql.NullString{
			String: toolCallID,
			Valid:  true,
		},
		Hidden: false,
		Compressed: sql.NullBool{
			Bool:  true,
			Valid: true,
		},
		Thinking: sql.NullString{},
		SubagentRequestID: uuid.NullUUID{},
		SubagentEvent: sql.NullString{},
		InputTokens: sql.NullInt64{},
		OutputTokens: sql.NullInt64{},
		TotalTokens: sql.NullInt64{},
		ReasoningTokens: sql.NullInt64{},
		CacheCreationTokens: sql.NullInt64{},
		CacheReadTokens: sql.NullInt64{},
		ContextLimit: sql.NullInt64{},
	})
	if err != nil {
		return xerrors.Errorf("insert summary tool result message: %w", err)
	}

	p.publishMessage(chatID, assistantMessage)
	p.publishMessage(chatID, toolMessage)
	return nil
}

func (p *Processor) resolveChatModel(
	ctx context.Context,
	chat database.Chat,
) (fantasy.LanguageModel, chatModelConfig, error) {
	config, parseErr := parseChatModelConfig(chat.ModelConfig)
	if parseErr != nil {
		return nil, chatModelConfig{}, xerrors.Errorf(
			"parse model config: %w",
			parseErr,
		)
	}

	if p.testing != nil && p.testing.ResolveChatModel != nil {
		model, err := p.testing.ResolveChatModel(chat)
		if err != nil {
			return nil, chatModelConfig{}, xerrors.Errorf("resolve model: %w", err)
		}
		return model, config, nil
	}

	keys, err := p.resolveProviderAPIKeys(ctx)
	if err != nil {
		return nil, chatModelConfig{}, xerrors.Errorf(
			"resolve provider API keys: %w",
			err,
		)
	}
	model, config, err := p.modelFromChat(ctx, config, keys)
	if err != nil {
		return nil, chatModelConfig{}, xerrors.Errorf("resolve model: %w", err)
	}
	return model, config, nil
}

func (p *Processor) modelFromChat(
	ctx context.Context,
	config chatModelConfig,
	providerKeys chatprovider.ProviderAPIKeys,
) (fantasy.LanguageModel, chatModelConfig, error) {
	if strings.TrimSpace(config.Model) == "" {
		fallbackConfig, fallbackErr := p.resolveFallbackChatModelConfig(
			ctx,
			providerKeys,
			config.Provider,
		)
		if fallbackErr != nil {
			return nil, chatModelConfig{}, fallbackErr
		}
		config = applyFallbackChatModelConfig(config, fallbackConfig)
	}

	model, err := modelFromConfig(config, providerKeys)
	if err != nil {
		return nil, chatModelConfig{}, err
	}
	return model, config, nil
}

func applyFallbackChatModelConfig(
	config chatModelConfig,
	fallbackConfig database.ChatModelConfig,
) chatModelConfig {
	config.Provider = fallbackConfig.Provider
	config.Model = fallbackConfig.Model

	defaults, err := parseChatModelConfig(fallbackConfig.ModelConfig)
	if err != nil {
		return config
	}

	chatprovider.MergeMissingCallConfig(
		&config.ChatModelCallConfig,
		defaults.ChatModelCallConfig,
	)
	return config
}

func (p *Processor) resolveFallbackChatModelConfig(
	ctx context.Context,
	providerKeys chatprovider.ProviderAPIKeys,
	providerHint string,
) (database.ChatModelConfig, error) {
	modelConfigs, err := p.db.GetEnabledChatModelConfigs(ctx)
	if err != nil {
		return database.ChatModelConfig{}, xerrors.Errorf(
			"load enabled chat model configs: %w",
			err,
		)
	}

	normalizedProviderHint := chatprovider.NormalizeProvider(providerHint)
	if strings.TrimSpace(providerHint) != "" && normalizedProviderHint == "" {
		return database.ChatModelConfig{}, xerrors.Errorf(
			"unknown provider %q in chat model config",
			providerHint,
		)
	}

	for _, modelConfig := range modelConfigs {
		provider := chatprovider.NormalizeProvider(modelConfig.Provider)
		if provider == "" {
			continue
		}
		if normalizedProviderHint != "" && provider != normalizedProviderHint {
			continue
		}
		if providerKeys.APIKey(provider) == "" {
			continue
		}
		return modelConfig, nil
	}

	if normalizedProviderHint != "" {
		return database.ChatModelConfig{}, xerrors.Errorf(
			"chat model is not configured and no enabled models with API keys are available for provider %q",
			normalizedProviderHint,
		)
	}

	return database.ChatModelConfig{}, xerrors.New(
		"chat model is not configured and no enabled models with API keys are available",
	)
}

func (p *Processor) runChatWithAgent(
	ctx context.Context,
	chat database.Chat,
	logger slog.Logger,
	model fantasy.LanguageModel,
	modelConfig chatModelConfig,
	prompt []fantasy.Message,
	reportOnly bool,
	subagentRequestID uuid.UUID,
) (*fantasy.AgentResult, error) {
	subagentRequest := uuid.NullUUID{}
	if chat.ParentChatID.Valid && subagentRequestID != uuid.Nil {
		subagentRequest = uuid.NullUUID{UUID: subagentRequestID, Valid: true}
	}

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

		if p.agentConnFn == nil {
			return nil, xerrors.New("workspace agent connector is not configured")
		}

		agentID, err := p.resolveAgentID(ctx, chatSnapshot)
		if err != nil {
			return nil, err
		}

		agentConn, agentRelease, err := p.agentConnFn(ctx, agentID)
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

	prompt = p.appendHomeInstructionToPrompt(
		ctx,
		chat,
		prompt,
		getWorkspaceConn,
	)

	// Resolve the model config's context_limit so we can use it as a
	// fallback when the LLM provider doesn't include context_limit in
	// its response metadata (which is the common case).
	compressionConfig := chatContextCompressionConfig{
		ThresholdPercent: defaultContextCompressionThresholdPercent,
	}
	if resolvedCompressionConfig, err := p.resolveChatContextCompressionConfig(ctx, chat); err != nil {
		logger.Warn(ctx, "failed to resolve chat compaction config", slog.Error(err))
	} else {
		compressionConfig = resolvedCompressionConfig
	}
	modelConfigContextLimit := compressionConfig.ContextLimit

	persistStep := func(persistCtx context.Context, step chatloop.PersistedStep) error {
		if len(step.AssistantContent) > 0 {
			assistantContent, err := chatprompt.MarshalContent(step.AssistantContent)
			if err != nil {
				return err
			}

			hasUsage := step.Usage != (fantasy.Usage{})
			assistantMessage, err := p.db.InsertChatMessage(persistCtx, database.InsertChatMessageParams{
				ChatID:  chat.ID,
				Role:    string(fantasy.MessageRoleAssistant),
				Content: assistantContent,
				ToolCallID: sql.NullString{
					Valid: false,
				},
				Thinking: sql.NullString{
					Valid: false,
				},
				Hidden:            false,
				SubagentRequestID: subagentRequest,
				InputTokens:       usageNullInt64(step.Usage.InputTokens, hasUsage),
				OutputTokens:      usageNullInt64(step.Usage.OutputTokens, hasUsage),
				TotalTokens:       usageNullInt64(step.Usage.TotalTokens, hasUsage),
				ReasoningTokens:   usageNullInt64(step.Usage.ReasoningTokens, hasUsage),
				CacheCreationTokens: usageNullInt64(
					step.Usage.CacheCreationTokens,
					hasUsage,
				),
				CacheReadTokens: usageNullInt64(step.Usage.CacheReadTokens, hasUsage),
				ContextLimit:    step.ContextLimit,
				SubagentEvent: sql.NullString{},
				Compressed: sql.NullBool{},
			})
			if err != nil {
				return xerrors.Errorf("insert assistant message: %w", err)
			}
			p.publishMessage(chat.ID, assistantMessage)
		}

		for _, result := range step.ToolResults {
			resultContent, err := chatprompt.MarshalResults([]chatprompt.ToolResultBlock{result})
			if err != nil {
				return err
			}

			toolMessage, err := p.db.InsertChatMessage(persistCtx, database.InsertChatMessageParams{
				ChatID:  chat.ID,
				Role:    string(fantasy.MessageRoleTool),
				Content: resultContent,
				ToolCallID: sql.NullString{
					String: result.ToolCallID,
					Valid:  result.ToolCallID != "",
				},
				Hidden:            false,
				SubagentRequestID: subagentRequest,
				Thinking: sql.NullString{},
				SubagentEvent: sql.NullString{},
				InputTokens: sql.NullInt64{},
				OutputTokens: sql.NullInt64{},
				TotalTokens: sql.NullInt64{},
				ReasoningTokens: sql.NullInt64{},
				CacheCreationTokens: sql.NullInt64{},
				CacheReadTokens: sql.NullInt64{},
				ContextLimit: sql.NullInt64{},
				Compressed: sql.NullBool{},
			})
			if err != nil {
				return xerrors.Errorf("insert tool result: %w", err)
			}

			p.publishMessage(chat.ID, toolMessage)
		}
		return nil
	}
	streamCall := streamCallOptionsFromChatModelConfig(model, modelConfig)
	var activeTools []string
	if reportOnly {
		activeTools = append(activeTools, "subagent_report")
	}
	var compactionOptions *chatloop.CompactionOptions
	if !reportOnly {
		compactionOptions = &chatloop.CompactionOptions{
			ThresholdPercent: compressionConfig.ThresholdPercent,
			ContextLimit:     compressionConfig.ContextLimit,
			Persist: func(
				persistCtx context.Context,
				result chatloop.CompactionResult,
			) error {
				if err := p.persistChatContextSummary(persistCtx, chat.ID, result); err != nil {
					return xerrors.Errorf("persist context summary: %w", err)
				}
				logger.Info(persistCtx, "chat context summarized",
					slog.F("chat_id", chat.ID),
					slog.F("threshold_percent", result.ThresholdPercent),
					slog.F("usage_percent", result.UsagePercent),
					slog.F("context_tokens", result.ContextTokens),
					slog.F("context_limit", result.ContextLimit),
				)
				return nil
			},
			OnError: func(err error) {
				logger.Warn(ctx, "failed to compact chat context", slog.Error(err))
			},
		}
	}

	return chatloop.Run(ctx, chatloop.RunOptions{
		Model:      model,
		Messages:   prompt,
		Tools:      p.agentTools(model, &currentChat, &chatStateMu, getWorkspaceConn),
		StreamCall: streamCall,
		MaxSteps:   maxChatSteps,

		ActiveTools:          activeTools,
		ContextLimitFallback: modelConfigContextLimit,

		PersistStep: persistStep,
		PublishMessagePart: func(
			role fantasy.MessageRole,
			part codersdk.ChatMessagePart,
		) {
			p.publishMessagePart(chat.ID, string(role), part)
		},
		Compaction: compactionOptions,

		OnInterruptedPersistError: func(err error) {
			p.logger.Warn(ctx, "failed to persist interrupted chat step", slog.Error(err))
		},
	})
}

func hasExceededChatToolSteps(result *fantasy.AgentResult) bool {
	if result == nil || len(result.Steps) < maxChatSteps {
		return false
	}
	lastStep := result.Steps[len(result.Steps)-1]
	return lastStep.FinishReason == fantasy.FinishReasonToolCalls &&
		len(lastStep.Content.ToolCalls()) > 0
}

func usageNullInt64(value int64, valid bool) sql.NullInt64 {
	if !valid {
		return sql.NullInt64{}
	}
	return sql.NullInt64{
		Int64: value,
		Valid: valid,
	}
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
		return
	}
	chat.Title = title
	p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindTitleChange)
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

func (p *Processor) appendHomeInstructionToPrompt(
	ctx context.Context,
	chat database.Chat,
	prompt []fantasy.Message,
	getWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error),
) []fantasy.Message {
	if !chat.WorkspaceID.Valid || getWorkspaceConn == nil {
		return prompt
	}

	instructionCtx, cancel := context.WithTimeout(ctx, homeInstructionLookupTimeout)
	defer cancel()

	conn, err := getWorkspaceConn(instructionCtx)
	if err != nil {
		p.logger.Debug(ctx, "failed to resolve workspace connection for home instruction file",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return prompt
	}

	content, sourcePath, truncated, err := readHomeInstructionFile(instructionCtx, conn)
	if err != nil {
		p.logger.Debug(ctx, "failed to load home instruction file",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return prompt
	}

	instruction := formatHomeInstruction(content, sourcePath, truncated)
	if instruction == "" {
		return prompt
	}

	return chatprompt.InsertSystem(prompt, instruction)
}

func (p *Processor) resolveProviderAPIKeys(
	ctx context.Context,
) (chatprovider.ProviderAPIKeys, error) {
	if p.resolveProviderAPIKeysFn == nil {
		return chatprovider.ProviderAPIKeys{}, nil
	}
	return p.resolveProviderAPIKeysFn(ctx)
}

func userMessageText(raw pqtype.NullRawMessage) (string, error) {
	content, err := chatprompt.ParseContent(string(fantasy.MessageRoleUser), raw)
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

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= max {
		return value
	}

	return string(runes[:max])
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
	staleAfter := time.Now().Add(-p.inFlightChatStaleAfter)
	staleChats, err := p.db.GetStaleChats(ctx, staleAfter)
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
	if p.localMode != nil {
		return p.localMode.Close()
	}
	return nil
}

type chatModelConfig struct {
	codersdk.ChatModelCallConfig
	Provider      string                     `json:"provider,omitempty"`
	Model         string                     `json:"model"`
	WorkspaceMode codersdk.ChatWorkspaceMode `json:"workspace_mode,omitempty"`
	ContextLimit  int64                      `json:"context_limit,omitempty"`
}

func parseChatModelConfig(raw json.RawMessage) (chatModelConfig, error) {
	config := chatModelConfig{}
	if len(raw) == 0 {
		return config, nil
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return chatModelConfig{}, err
	}
	return config, nil
}

func streamCallOptionsFromChatModelConfig(
	model fantasy.LanguageModel,
	config chatModelConfig,
) fantasy.AgentStreamCall {
	streamCall := fantasy.AgentStreamCall{
		MaxOutputTokens:  config.MaxOutputTokens,
		Temperature:      config.Temperature,
		TopP:             config.TopP,
		TopK:             config.TopK,
		PresencePenalty:  config.PresencePenalty,
		FrequencyPenalty: config.FrequencyPenalty,
		ProviderOptions: chatprovider.ProviderOptionsFromChatModelConfig(
			model,
			config.ProviderOptions,
		),
	}

	if streamCall.MaxOutputTokens == nil {
		maxOutputTokens := int64(32_000)
		streamCall.MaxOutputTokens = &maxOutputTokens
	}

	return streamCall
}

func workspaceModeFromChat(chat database.Chat) codersdk.ChatWorkspaceMode {
	config, err := parseChatModelConfig(chat.ModelConfig)
	if err != nil {
		return codersdk.ChatWorkspaceModeWorkspace
	}

	switch codersdk.ChatWorkspaceMode(
		strings.ToLower(strings.TrimSpace(string(config.WorkspaceMode))),
	) {
	case codersdk.ChatWorkspaceModeLocal:
		return codersdk.ChatWorkspaceModeLocal
	default:
		return codersdk.ChatWorkspaceModeWorkspace
	}
}

func modelFromName(
	modelName string,
	providerKeys chatprovider.ProviderAPIKeys,
) (fantasy.LanguageModel, error) {
	return modelFromConfig(chatModelConfig{Model: modelName}, providerKeys)
}

// anyAvailableModel returns a language model from whichever provider
// has an API key configured. This is used for lightweight tasks like
// title generation where we don't need a specific model.
func anyAvailableModel(
	keys chatprovider.ProviderAPIKeys,
) (fantasy.LanguageModel, error) {
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
		if keys.APIKey(candidate.Provider) == "" {
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

func modelFromConfig(
	config chatModelConfig,
	providerKeys chatprovider.ProviderAPIKeys,
) (fantasy.LanguageModel, error) {
	return chatprovider.ModelFromConfig(config.Provider, config.Model, providerKeys)
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

type waitForExternalAuthArgs struct {
	ProviderID     string `json:"provider_id"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
}

type gitAuthRequiredMarker struct {
	ProviderID          string `json:"provider_id"`
	ProviderType        string `json:"provider_type,omitempty"`
	ProviderDisplayName string `json:"provider_display_name,omitempty"`
	AuthenticateURL     string `json:"authenticate_url"`
	Host                string `json:"host,omitempty"`
}

type createWorkspaceArgs struct {
	Prompt    string          `json:"prompt,omitempty"`
	Workspace json.RawMessage `json:"workspace,omitempty"`
	Request   json.RawMessage `json:"request,omitempty"`
}

type subagentArgs struct {
	Prompt     string `json:"prompt"`
	Title      string `json:"title,omitempty"`
	Background bool   `json:"background,omitempty"`
}

type subagentAwaitArgs struct {
	ChatID         string `json:"chat_id"`
	RequestID      string `json:"request_id"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
}

type subagentMessageArgs struct {
	ChatID         string `json:"chat_id"`
	Message        string `json:"message"`
	Await          bool   `json:"await,omitempty"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
}

type subagentTerminateArgs struct {
	ChatID string `json:"chat_id"`
}

type subagentReportArgs struct {
	Report    string `json:"report"`
	RequestID string `json:"request_id,omitempty"`
}

func (p *Processor) agentTools(
	model fantasy.LanguageModel,
	chatState *database.Chat,
	chatStateMu *sync.Mutex,
	getWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error),
) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		fantasy.NewAgentTool(
			"create_workspace",
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
						toolResult = toolError(chatprompt.ToolResultBlock{
							ToolCallID: toolCall.ToolCallID,
							ToolName:   toolCall.ToolName,
						}, xerrors.New("workspace creator returned a created workspace without an ID"))
					} else {
						updatedChat, err := p.persistChatWorkspace(ctx, chatSnapshot, wsResult)
						if err != nil {
							toolResult = toolError(chatprompt.ToolResultBlock{
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
			"read_file",
			"Read a file from the workspace.",
			func(ctx context.Context, args readFileArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := chatprompt.ToolResultBlock{ToolCallID: call.ID, ToolName: call.Name}
				conn, err := getWorkspaceConn(ctx)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				return toolResultBlockToAgentResponse(executeReadFileTool(ctx, conn, call.ID, args)), nil
			},
		),
		fantasy.NewAgentTool(
			"write_file",
			"Write a file to the workspace.",
			func(ctx context.Context, args writeFileArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := chatprompt.ToolResultBlock{ToolCallID: call.ID, ToolName: call.Name}
				conn, err := getWorkspaceConn(ctx)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				return toolResultBlockToAgentResponse(executeWriteFileTool(ctx, conn, call.ID, args)), nil
			},
		),
		fantasy.NewAgentTool(
			"edit_files",
			"Perform search-and-replace edits on one or more files in the workspace."+
				" Each file can have multiple edits applied atomically.",
			func(ctx context.Context, args editFilesArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := chatprompt.ToolResultBlock{ToolCallID: call.ID, ToolName: call.Name}
				conn, err := getWorkspaceConn(ctx)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				return toolResultBlockToAgentResponse(executeEditFilesTool(ctx, conn, call.ID, args)), nil
			},
		),
		fantasy.NewAgentTool(
			"execute",
			"Execute a shell command in the workspace.",
			func(ctx context.Context, args executeArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := chatprompt.ToolResultBlock{ToolCallID: call.ID, ToolName: call.Name}
				conn, err := getWorkspaceConn(ctx)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				return toolResultBlockToAgentResponse(executeExecuteTool(ctx, conn, call.ID, args)), nil
			},
		),
		fantasy.NewAgentTool(
			"wait_for_external_auth",
			"Wait for external authentication to complete after execute reports auth_required=true. "+
				"Use this before retrying git commands, and do not rerun commands automatically.",
			func(ctx context.Context, args waitForExternalAuthArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := chatprompt.ToolResultBlock{ToolCallID: call.ID, ToolName: call.Name}
				providerID := strings.TrimSpace(args.ProviderID)
				if providerID == "" {
					return toolResultBlockToAgentResponse(toolError(base, xerrors.New("provider_id is required"))), nil
				}

				timeout := defaultExternalAuthWait
				if args.TimeoutSeconds != nil {
					timeout = time.Duration(*args.TimeoutSeconds) * time.Second
				}

				chatStateMu.Lock()
				chatSnapshot := *chatState
				chatStateMu.Unlock()

				authenticated, timedOut, err := p.waitForExternalAuth(
					ctx,
					chatSnapshot.OwnerID,
					providerID,
					timeout,
				)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				status := externalAuthWaitCompletedStatus
				if timedOut {
					status = externalAuthWaitTimedOutStatus
				}

				return toolResultBlockToAgentResponse(chatprompt.ToolResultBlock{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Result: map[string]any{
						"provider_id":   providerID,
						"authenticated": authenticated,
						"timed_out":     timedOut,
						"status":        status,
					},
				}), nil
			},
		),
		fantasy.NewAgentTool(
			"subagent",
			"Create a delegated child subagent chat. If background=false, this call waits for the child report.",
			func(ctx context.Context, args subagentArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := chatprompt.ToolResultBlock{ToolCallID: call.ID, ToolName: call.Name}
				if p.subagentService == nil {
					return toolResultBlockToAgentResponse(toolError(base, xerrors.New("subagent service is not configured"))), nil
				}

				chatStateMu.Lock()
				chatSnapshot := *chatState
				chatStateMu.Unlock()

				if chatSnapshot.ParentChatID.Valid {
					return toolResultBlockToAgentResponse(toolError(
						base,
						xerrors.New("delegated chats cannot create child subagents in phase-1"),
					)), nil
				}

				childChat, requestID, err := p.subagentService.CreateChildSubagentChat(
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
					"title":      childChat.Title,
					"request_id": requestID.String(),
					"background": args.Background,
					"status":     "pending",
				}
				if args.Background {
					return toolResultBlockToAgentResponse(chatprompt.ToolResultBlock{
						ToolCallID: call.ID,
						ToolName:   call.Name,
						Result:     payload,
					}), nil
				}

				awaitResult, err := p.subagentService.AwaitSubagentReport(
					ctx,
					chatSnapshot.ID,
					childChat.ID,
					requestID,
					defaultSubagentAwaitTimeout,
				)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				payload["report"] = awaitResult.Report
				payload["duration_ms"] = awaitResult.DurationMS
				payload["status"] = "completed"

				return toolResultBlockToAgentResponse(chatprompt.ToolResultBlock{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Result:     payload,
				}), nil
			},
		),
		fantasy.NewAgentTool(
			"subagent_await",
			"Wait for a delegated descendant subagent chat to report completion.",
			func(ctx context.Context, args subagentAwaitArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := chatprompt.ToolResultBlock{ToolCallID: call.ID, ToolName: call.Name}
				if p.subagentService == nil {
					return toolResultBlockToAgentResponse(toolError(base, xerrors.New("subagent service is not configured"))), nil
				}

				chatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				requestID, err := parseSubagentToolRequestID(args.RequestID)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				timeout := defaultSubagentAwaitTimeout
				if args.TimeoutSeconds != nil {
					timeout = time.Duration(*args.TimeoutSeconds) * time.Second
				}

				chatStateMu.Lock()
				chatSnapshot := *chatState
				chatStateMu.Unlock()

				awaitResult, err := p.subagentService.AwaitSubagentReport(
					ctx,
					chatSnapshot.ID,
					chatID,
					requestID,
					timeout,
				)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				targetChat, err := p.subagentService.db.GetChatByID(ctx, chatID)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				return toolResultBlockToAgentResponse(chatprompt.ToolResultBlock{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Result: map[string]any{
						"chat_id":     chatID.String(),
						"title":       targetChat.Title,
						"request_id":  awaitResult.RequestID.String(),
						"report":      awaitResult.Report,
						"duration_ms": awaitResult.DurationMS,
						"status":      "completed",
					},
				}), nil
			},
		),
		fantasy.NewAgentTool(
			"subagent_message",
			"Send a follow-up user message to a delegated descendant subagent chat and optionally wait for a new report.",
			func(ctx context.Context, args subagentMessageArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := chatprompt.ToolResultBlock{ToolCallID: call.ID, ToolName: call.Name}
				if p.subagentService == nil {
					return toolResultBlockToAgentResponse(toolError(base, xerrors.New("subagent service is not configured"))), nil
				}

				chatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				timeout := defaultSubagentAwaitTimeout
				if args.TimeoutSeconds != nil {
					timeout = time.Duration(*args.TimeoutSeconds) * time.Second
				}

				chatStateMu.Lock()
				chatSnapshot := *chatState
				chatStateMu.Unlock()

				targetChat, requestID, err := p.subagentService.SendSubagentMessage(
					ctx,
					chatSnapshot.ID,
					chatID,
					args.Message,
				)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				payload := map[string]any{
					"chat_id":    chatID.String(),
					"title":      targetChat.Title,
					"request_id": requestID.String(),
					"status":     string(targetChat.Status),
				}
				if !args.Await {
					return toolResultBlockToAgentResponse(chatprompt.ToolResultBlock{
						ToolCallID: call.ID,
						ToolName:   call.Name,
						Result:     payload,
					}), nil
				}

				awaitResult, err := p.subagentService.AwaitSubagentReport(
					ctx,
					chatSnapshot.ID,
					chatID,
					requestID,
					timeout,
				)
				if err != nil {
					// Include the chat metadata and request_id
					// in the error response so the LLM can retry
					// with subagent_await using the correct
					// request_id.
					payload["error"] = err.Error()
					payload["status"] = "error"
					return toolResultBlockToAgentResponse(chatprompt.ToolResultBlock{
						ToolCallID: call.ID,
						ToolName:   call.Name,
						IsError:    true,
						Result:     payload,
					}), nil
				}

				payload["report"] = awaitResult.Report
				payload["duration_ms"] = awaitResult.DurationMS
				payload["status"] = "completed"

				return toolResultBlockToAgentResponse(chatprompt.ToolResultBlock{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Result:     payload,
				}), nil
			},
		),
		fantasy.NewAgentTool(
			"subagent_terminate",
			"Terminate a delegated descendant subagent subtree.",
			func(ctx context.Context, args subagentTerminateArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := chatprompt.ToolResultBlock{ToolCallID: call.ID, ToolName: call.Name}
				if p.subagentService == nil {
					return toolResultBlockToAgentResponse(toolError(base, xerrors.New("subagent service is not configured"))), nil
				}

				chatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				chatStateMu.Lock()
				chatSnapshot := *chatState
				chatStateMu.Unlock()

				if err := p.subagentService.TerminateSubagentSubtree(ctx, chatSnapshot.ID, chatID); err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				return toolResultBlockToAgentResponse(chatprompt.ToolResultBlock{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Result: map[string]any{
						"chat_id":    chatID.String(),
						"terminated": true,
						"status":     "terminated",
					},
				}), nil
			},
		),
		fantasy.NewAgentTool(
			"subagent_report",
			"Mark the current delegated subagent chat as reported when all descendants are complete.",
			func(ctx context.Context, args subagentReportArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				base := chatprompt.ToolResultBlock{ToolCallID: call.ID, ToolName: call.Name}
				if p.subagentService == nil {
					return toolResultBlockToAgentResponse(toolError(base, xerrors.New("subagent service is not configured"))), nil
				}

				chatStateMu.Lock()
				chatSnapshot := *chatState
				chatStateMu.Unlock()

				hasActiveDescendants, err := p.subagentService.HasActiveDescendants(ctx, chatSnapshot.ID)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}
				if hasActiveDescendants {
					return toolResultBlockToAgentResponse(toolError(
						base,
						xerrors.New("cannot report while active delegated descendants remain"),
					)), nil
				}

				requestID := uuid.NullUUID{}
				if strings.TrimSpace(args.RequestID) != "" {
					parsedRequestID, parseErr := parseSubagentToolRequestID(args.RequestID)
					if parseErr != nil {
						return toolResultBlockToAgentResponse(toolError(base, parseErr)), nil
					}
					requestID = uuid.NullUUID{UUID: parsedRequestID, Valid: true}
				}

				awaitResult, err := p.subagentService.MarkSubagentReported(
					ctx,
					chatSnapshot.ID,
					args.Report,
					requestID,
				)
				if err != nil {
					return toolResultBlockToAgentResponse(toolError(base, err)), nil
				}

				return toolResultBlockToAgentResponse(chatprompt.ToolResultBlock{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Result: map[string]any{
						"chat_id":     chatSnapshot.ID.String(),
						"title":       chatSnapshot.Title,
						"request_id":  awaitResult.RequestID.String(),
						"report":      awaitResult.Report,
						"duration_ms": awaitResult.DurationMS,
						"reported":    true,
						"status":      "reported",
					},
				}), nil
			},
		),
	}
}

func parseSubagentToolChatID(raw string) (uuid.UUID, error) {
	chatID, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, xerrors.New("chat_id must be a valid UUID")
	}
	return chatID, nil
}

func parseSubagentToolRequestID(raw string) (uuid.UUID, error) {
	requestID, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, xerrors.New("request_id must be a valid UUID")
	}
	return requestID, nil
}

func (p *Processor) waitForExternalAuth(
	ctx context.Context,
	ownerID uuid.UUID,
	providerID string,
	timeout time.Duration,
) (authenticated bool, timedOut bool, err error) {
	if timeout <= 0 {
		timeout = defaultExternalAuthWait
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(externalAuthWaitPollInterval)
	defer ticker.Stop()

	for {
		link, linkErr := p.db.GetExternalAuthLink(
			//nolint:gocritic // Background wait for external auth has no user context.
			dbauthz.AsSystemRestricted(waitCtx),
			database.GetExternalAuthLinkParams{
				ProviderID: providerID,
				UserID:     ownerID,
			},
		)
		if linkErr == nil {
			unexpired := link.OAuthExpiry.IsZero() || link.OAuthExpiry.After(time.Now())
			if strings.TrimSpace(link.OAuthAccessToken) != "" && unexpired {
				return true, false, nil
			}
		} else if !errors.Is(linkErr, sql.ErrNoRows) {
			return false, false, xerrors.Errorf("get external auth link: %w", linkErr)
		}

		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return false, true, nil
			}
			return false, false, waitCtx.Err()
		case <-ticker.C:
		}
	}
}

func (p *Processor) executeCreateWorkspaceTool(
	ctx context.Context,
	chat database.Chat,
	model fantasy.LanguageModel,
	toolCall fantasy.ToolCallContent,
) (chatprompt.ToolResultBlock, CreateWorkspaceToolResult) {
	base := chatprompt.ToolResultBlock{
		ToolCallID: toolCall.ToolCallID,
		ToolName:   toolCall.ToolName,
	}

	if p.createWorkspaceFn == nil {
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
	if p != nil && toolCall.ToolCallID != "" {
		logEmitter := &createWorkspaceBuildLogEmitter{
			processor:  p,
			chatID:     chat.ID,
			toolCallID: toolCall.ToolCallID,
			toolName:   toolCall.ToolName,
		}
		toolReq.BuildLogHandler = logEmitter.Emit
	}
	if len(args.Workspace) > 0 && string(args.Workspace) != "null" {
		toolReq.Spec = args.Workspace
	} else if len(args.Request) > 0 && string(args.Request) != "null" {
		toolReq.Spec = args.Request
	}

	wsResult, err := p.createWorkspaceFn(ctx, toolReq)
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

	return chatprompt.ToolResultBlock{
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

	parts := []string{"build"}
	if stage := strings.TrimSpace(entry.Stage); stage != "" {
		parts = append(parts, stage)
	}
	if level := strings.TrimSpace(entry.Level); level != "" {
		parts = append(parts, level)
	}
	prefix := "[" + strings.Join(parts, "/") + "] "

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
) chatprompt.ToolResultBlock {
	result := chatprompt.ToolResultBlock{
		ToolCallID: toolCallID,
		ToolName:   "read_file",
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
}

func executeWriteFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	toolCallID string,
	args writeFileArgs,
) chatprompt.ToolResultBlock {
	result := chatprompt.ToolResultBlock{
		ToolCallID: toolCallID,
		ToolName:   "write_file",
	}
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
) chatprompt.ToolResultBlock {
	result := chatprompt.ToolResultBlock{
		ToolCallID: toolCallID,
		ToolName:   "edit_files",
	}
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
) chatprompt.ToolResultBlock {
	result := chatprompt.ToolResultBlock{
		ToolCallID: toolCallID,
		ToolName:   "execute",
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
	authRequired, cleanedOutput := extractGitAuthRequiredMarker(output)
	resultPayload := map[string]any{
		"output":    cleanedOutput,
		"exit_code": exitCode,
	}
	if authRequired != nil {
		resultPayload["auth_required"] = true
		resultPayload["authenticate_url"] = authRequired.AuthenticateURL
		resultPayload["reason"] = authRequiredResultReason
		if strings.TrimSpace(authRequired.ProviderID) != "" {
			resultPayload["provider_id"] = authRequired.ProviderID
		}
		if strings.TrimSpace(authRequired.ProviderType) != "" {
			resultPayload["provider_type"] = authRequired.ProviderType
		}
		if strings.TrimSpace(authRequired.ProviderDisplayName) != "" {
			resultPayload["provider_display_name"] = authRequired.ProviderDisplayName
		}
		if err != nil {
			resultPayload["error"] = err.Error()
		}
		result.Result = resultPayload
		return result
	}
	if err != nil {
		resultPayload["error"] = err.Error()
		result.IsError = true
	}
	result.Result = resultPayload
	return result
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
	if err := session.Setenv(chatAgentEnvVar, "true"); err != nil {
		return "", 0, xerrors.Errorf("set %s: %w", chatAgentEnvVar, err)
	}

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

func extractGitAuthRequiredMarker(output string) (*gitAuthRequiredMarker, string) {
	if output == "" {
		return nil, output
	}

	var marker *gitAuthRequiredMarker
	lines := strings.Split(output, "\n")
	filteredLines := make([]string, 0, len(lines))
	for _, line := range lines {
		idx := strings.Index(line, gitAuthRequiredMarkerPrefix)
		if idx == -1 {
			filteredLines = append(filteredLines, line)
			continue
		}

		rawPayload := strings.TrimSpace(line[idx+len(gitAuthRequiredMarkerPrefix):])
		candidate := gitAuthRequiredMarker{}
		if rawPayload == "" || json.Unmarshal([]byte(rawPayload), &candidate) != nil || strings.TrimSpace(candidate.AuthenticateURL) == "" {
			filteredLines = append(filteredLines, line)
			continue
		}
		if marker == nil {
			marker = &candidate
		}

		prefix := strings.TrimSpace(line[:idx])
		if prefix != "" {
			filteredLines = append(filteredLines, prefix)
		}
	}
	return marker, strings.Join(filteredLines, "\n")
}

func toolError(result chatprompt.ToolResultBlock, err error) chatprompt.ToolResultBlock {
	result.IsError = true
	result.Result = map[string]any{"error": err.Error()}
	return result
}

func toolResultBlockToAgentResponse(result chatprompt.ToolResultBlock) fantasy.ToolResponse {
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
