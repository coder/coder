//go:build !slim

package chatexec

import (
	"context"
	"errors"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// ChatRunnerClient is the narrow agent SDK surface needed by the chat executor.
type ChatRunnerClient interface {
	ChatRunnerRuntimeContext(ctx context.Context, req agentsdk.ChatRunnerRuntimeContextRequest) (agentsdk.ChatRunnerRuntimeContextResponse, error)
	ChatRunnerPersistStep(ctx context.Context, req agentsdk.ChatRunnerPersistStepRequest) (agentsdk.ChatRunnerPersistStepResponse, error)
	ChatRunnerPublishStreamPart(ctx context.Context, req agentsdk.ChatRunnerPublishStreamPartRequest) (agentsdk.ChatRunnerPublishStreamPartResponse, error)
	ChatRunnerPublishStreamParts(ctx context.Context, req agentsdk.ChatRunnerPublishStreamPartsRequest) (agentsdk.ChatRunnerPublishStreamPartsResponse, error)
	ChatRunnerReloadMessages(ctx context.Context, req agentsdk.ChatRunnerReloadMessagesRequest) (agentsdk.ChatRunnerReloadMessagesResponse, error)
	ChatRunnerListTemplates(ctx context.Context, req agentsdk.ChatRunnerListTemplatesRequest) (agentsdk.ChatRunnerListTemplatesResponse, error)
	ChatRunnerReadTemplate(ctx context.Context, req agentsdk.ChatRunnerReadTemplateRequest) (agentsdk.ChatRunnerReadTemplateResponse, error)
	ChatRunnerMCPToolCall(ctx context.Context, req agentsdk.ChatRunnerMCPToolCallRequest) (agentsdk.ChatRunnerMCPToolCallResponse, error)
}

// Executor runs chat runner work against the production chatloop implementation.
type Executor struct {
	client ChatRunnerClient
	logger slog.Logger
	// getLocalConn resolves a connection back to the current agent so the
	// executor can run workspace-local tools after handoff.
	getLocalConn func(context.Context) (workspacesdk.AgentConn, error)

	// Test seams. When nil, production defaults are used.
	buildModel func(providerHint, modelName string, keys chatprovider.ProviderAPIKeys, userAgent string, extraHeaders map[string]string) (fantasy.LanguageModel, error)
	runLoop    func(ctx context.Context, opts chatloop.RunOptions) error
	clock      quartz.Clock
}

// New constructs a production chat executor.
func New(
	client ChatRunnerClient,
	logger slog.Logger,
	getLocalConn func(context.Context) (workspacesdk.AgentConn, error),
) *Executor {
	if client == nil {
		panic("chatexec.New: client must not be nil")
	}

	return &Executor{
		client:       client,
		logger:       logger,
		getLocalConn: getLocalConn,
	}
}

// Execute loads runtime context for chatID and runs the production chatloop.
func (e *Executor) Execute(ctx context.Context, chatID uuid.UUID) error {
	resp, err := e.client.ChatRunnerRuntimeContext(ctx, agentsdk.ChatRunnerRuntimeContextRequest{
		ChatID: chatID,
	})
	if err != nil {
		return xerrors.Errorf("fetch runtime context: %w", err)
	}

	leaseEpoch := resp.LeaseEpoch
	modelConfigID := resp.ModelConfigID

	keys := chatprovider.ProviderAPIKeys{
		ByProvider:        resp.ProviderAPIKeys,
		BaseURLByProvider: resp.ProviderBaseURLs,
	}

	buildModel := e.buildModel
	if buildModel == nil {
		buildModel = func(providerHint, modelName string, keys chatprovider.ProviderAPIKeys, userAgent string, extraHeaders map[string]string) (fantasy.LanguageModel, error) {
			return chatprovider.ModelFromConfig(providerHint, modelName, keys, userAgent, extraHeaders, nil)
		}
	}
	model, err := buildModel(resp.Provider, resp.Model, keys, chatprovider.UserAgent(), nil)
	if err != nil {
		return xerrors.Errorf("build model: %w", err)
	}

	batcher := newPublishBatcher(e.client, e.logger, chatID, leaseEpoch, e.clock)
	defer batcher.Close()

	providerOptions := chatprovider.ProviderOptionsFromChatModelConfig(model, resp.CallConfig.ProviderOptions)

	messages, err := MessagesFromSDK(e.logger, resp.Messages)
	if err != nil {
		return xerrors.Errorf("convert messages: %w", err)
	}
	localTools, err := buildLocalTools(resp.BuiltinTools, e.getLocalConn)
	if err != nil {
		return xerrors.Errorf("build local tools: %w", err)
	}
	controlPlaneTools, err := buildControlPlaneTools(resp.BuiltinTools, e.client, chatID, leaseEpoch)
	if err != nil {
		return xerrors.Errorf("build control plane tools: %w", err)
	}
	providerTools, err := buildProviderTools(resp.ProviderTools, e.getLocalConn, e.logger)
	if err != nil {
		return xerrors.Errorf("build provider tools: %w", err)
	}
	dynamicTools := buildDynamicTools(e.logger, resp.DynamicTools)
	mcpTools := buildMCPTools(resp.MCPTools, e.client, chatID, leaseEpoch)

	tools := make([]fantasy.AgentTool, 0, len(localTools)+len(controlPlaneTools)+len(dynamicTools)+len(mcpTools))
	tools = append(tools, localTools...)
	tools = append(tools, controlPlaneTools...)
	occupiedToolNames := make(map[string]struct{}, len(tools)+len(providerTools))
	for _, tool := range tools {
		occupiedToolNames[tool.Info().Name] = struct{}{}
	}
	for _, tool := range providerTools {
		occupiedToolNames[tool.Definition.GetName()] = struct{}{}
	}

	var dynamicToolNames map[string]bool
	if len(dynamicTools) > 0 {
		filteredDynamicTools := make([]fantasy.AgentTool, 0, len(dynamicTools))
		for _, tool := range dynamicTools {
			name := tool.Info().Name
			if _, exists := occupiedToolNames[name]; exists {
				e.logger.Warn(ctx, "dynamic tool name collides with existing tool, existing tool takes precedence",
					slog.F("tool_name", name),
				)
				continue
			}
			filteredDynamicTools = append(filteredDynamicTools, tool)
		}
		if len(filteredDynamicTools) > 0 {
			dynamicToolNames = make(map[string]bool, len(filteredDynamicTools))
			for _, tool := range filteredDynamicTools {
				name := tool.Info().Name
				dynamicToolNames[name] = true
				occupiedToolNames[name] = struct{}{}
			}
			tools = append(tools, filteredDynamicTools...)
		}
	}
	if len(mcpTools) > 0 {
		filteredMCPTools := make([]fantasy.AgentTool, 0, len(mcpTools))
		for _, tool := range mcpTools {
			name := tool.Info().Name
			if _, exists := occupiedToolNames[name]; exists {
				e.logger.Warn(ctx, "mcp tool name collides with existing tool, existing tool takes precedence",
					slog.F("tool_name", name),
				)
				continue
			}
			occupiedToolNames[name] = struct{}{}
			filteredMCPTools = append(filteredMCPTools, tool)
		}
		tools = append(tools, filteredMCPTools...)
	}

	opts := chatloop.RunOptions{
		Model:                     model,
		Messages:                  messages,
		ModelConfig:               resp.CallConfig,
		ProviderOptions:           providerOptions,
		ContextLimitFallback:      resp.ContextLimit,
		MaxSteps:                  1200,
		Tools:                     tools,
		ProviderTools:             providerTools,
		DynamicToolNames:          dynamicToolNames,
		ActiveTools:               nil,
		PersistStep:               e.makePersistStep(chatID, leaseEpoch, modelConfigID),
		PublishMessagePart:        batcher.Enqueue,
		ReloadMessages:            e.makeReloadMessages(chatID, leaseEpoch),
		OnRetry:                   e.makeOnRetry(chatID),
		OnInterruptedPersistError: e.makeOnInterruptedPersistError(chatID),
	}

	if resp.CompactionThresholdPercent > 0 {
		opts.Compaction = &chatloop.CompactionOptions{
			ThresholdPercent:   resp.CompactionThresholdPercent,
			ContextLimit:       resp.ContextLimit,
			ToolCallID:         "compaction-tool-call",
			ToolName:           "coder_chat_compaction",
			PublishMessagePart: opts.PublishMessagePart,
			OnError: func(err error) {
				e.logger.Warn(ctx, "compaction error",
					slog.F("chat_id", chatID),
					slog.Error(err),
				)
			},
		}
	}

	runLoop := e.runLoop
	if runLoop == nil {
		runLoop = chatloop.Run
	}
	err = runLoop(ctx, opts)
	if errors.Is(err, chatloop.ErrDynamicToolCall) {
		return ErrRequiresAction
	}
	return err
}

const (
	publishBatcherDebounce = 50 * time.Millisecond
	publishBatcherTimeout  = 5 * time.Second
)

type publishBatcher struct {
	client     ChatRunnerClient
	logger     slog.Logger
	chatID     uuid.UUID
	leaseEpoch int64
	clock      quartz.Clock

	mu       sync.Mutex
	pending  []agentsdk.ChatRunnerPublishStreamPart
	deadline time.Time
	timer    *quartz.Timer
	closed   bool

	flushMu sync.Mutex
	wg      sync.WaitGroup
}

func newPublishBatcher(
	client ChatRunnerClient,
	logger slog.Logger,
	chatID uuid.UUID,
	leaseEpoch int64,
	clock quartz.Clock,
) *publishBatcher {
	if client == nil {
		panic("newPublishBatcher: client must not be nil")
	}
	if clock == nil {
		clock = quartz.NewReal()
	}
	return &publishBatcher{
		client:     client,
		logger:     logger,
		chatID:     chatID,
		leaseEpoch: leaseEpoch,
		clock:      clock,
	}
}

func (b *publishBatcher) Enqueue(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.pending = append(b.pending, agentsdk.ChatRunnerPublishStreamPart{
		Role: role,
		Part: cloneChatMessagePart(part),
	})
	b.deadline = b.clock.Now().Add(publishBatcherDebounce)
	if b.timer != nil {
		return
	}

	b.armTimerLocked(publishBatcherDebounce)
}

func (b *publishBatcher) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		b.wg.Wait()
		return
	}

	b.closed = true
	stopped := false
	if b.timer != nil {
		stopped = b.timer.Stop()
		b.timer = nil
	}
	batch := append([]agentsdk.ChatRunnerPublishStreamPart(nil), b.pending...)
	b.pending = nil
	b.mu.Unlock()

	if stopped {
		b.wg.Done()
	}

	b.publish(batch)
	b.wg.Wait()
}

func (b *publishBatcher) armTimerLocked(delay time.Duration) {
	if delay <= 0 {
		panic("publishBatcher.armTimerLocked: delay must be positive")
	}
	b.wg.Add(1)
	b.timer = b.clock.AfterFunc(delay, b.onTimer)
}

func (b *publishBatcher) onTimer() {
	defer b.wg.Done()

	b.mu.Lock()
	if b.closed {
		b.timer = nil
		b.mu.Unlock()
		return
	}

	remaining := b.clock.Until(b.deadline)
	b.timer = nil
	if remaining > 0 {
		// Enqueue extends the deadline without resetting a timer that's already
		// firing. Re-arm once for the remaining debounce window instead.
		b.armTimerLocked(remaining)
		b.mu.Unlock()
		return
	}

	batch := append([]agentsdk.ChatRunnerPublishStreamPart(nil), b.pending...)
	b.pending = nil
	b.mu.Unlock()

	b.publish(batch)
}

func (b *publishBatcher) publish(batch []agentsdk.ChatRunnerPublishStreamPart) {
	if len(batch) == 0 {
		return
	}

	b.flushMu.Lock()
	defer b.flushMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), publishBatcherTimeout)
	defer cancel()

	_, err := b.client.ChatRunnerPublishStreamParts(ctx, agentsdk.ChatRunnerPublishStreamPartsRequest{
		ChatID:     b.chatID,
		LeaseEpoch: b.leaseEpoch,
		Parts:      batch,
	})
	if err != nil {
		b.logger.Warn(ctx, "publish stream parts failed (best-effort)",
			slog.F("chat_id", b.chatID),
			slog.F("part_count", len(batch)),
			slog.Error(err),
		)
	}
}

func cloneChatMessagePart(part codersdk.ChatMessagePart) codersdk.ChatMessagePart {
	clone := part
	clone.Args = append([]byte(nil), part.Args...)
	clone.Data = append([]byte(nil), part.Data...)
	clone.Result = append([]byte(nil), part.Result...)
	clone.ProviderMetadata = append([]byte(nil), part.ProviderMetadata...)
	if part.CreatedAt != nil {
		createdAt := *part.CreatedAt
		clone.CreatedAt = &createdAt
	}
	return clone
}

func (e *Executor) makePersistStep(
	chatID uuid.UUID,
	leaseEpoch int64,
	modelConfigID uuid.UUID,
) func(context.Context, chatloop.PersistedStep) error {
	return func(ctx context.Context, step chatloop.PersistedStep) error {
		assistantParts, toolResults := SplitPersistedContent(step.Content)
		usage := UsageToSDK(step.Usage)

		var contextLimit *int64
		if step.ContextLimit.Valid {
			contextLimit = &step.ContextLimit.Int64
		}

		_, err := e.client.ChatRunnerPersistStep(ctx, agentsdk.ChatRunnerPersistStepRequest{
			ChatID:             chatID,
			LeaseEpoch:         leaseEpoch,
			AssistantParts:     assistantParts,
			ToolResults:        toolResults,
			Usage:              usage,
			ContextLimit:       contextLimit,
			ProviderResponseID: step.ProviderResponseID,
			RuntimeMs:          step.Runtime.Milliseconds(),
			ModelConfigID:      modelConfigID,
		})
		if err != nil {
			return xerrors.Errorf("persist step: %w", err)
		}
		return nil
	}
}

func (e *Executor) makeReloadMessages(
	chatID uuid.UUID,
	leaseEpoch int64,
) func(context.Context) ([]fantasy.Message, error) {
	return func(ctx context.Context) ([]fantasy.Message, error) {
		resp, err := e.client.ChatRunnerReloadMessages(ctx, agentsdk.ChatRunnerReloadMessagesRequest{
			ChatID:     chatID,
			LeaseEpoch: leaseEpoch,
		})
		if err != nil {
			return nil, xerrors.Errorf("reload messages: %w", err)
		}
		msgs, err := MessagesFromSDK(e.logger, resp.Messages)
		if err != nil {
			return nil, xerrors.Errorf("convert reloaded messages: %w", err)
		}
		return msgs, nil
	}
}

func (e *Executor) makeOnRetry(chatID uuid.UUID) chatretry.OnRetryFn {
	return func(
		attempt int,
		rawErr error,
		classified chatretry.ClassifiedError,
		delay time.Duration,
	) {
		e.logger.Warn(context.Background(), "retrying model call",
			slog.F("chat_id", chatID),
			slog.F("attempt", attempt),
			slog.F("error_class", classified),
			slog.F("delay", delay),
			slog.Error(rawErr),
		)
	}
}

func (e *Executor) makeOnInterruptedPersistError(chatID uuid.UUID) func(error) {
	return func(err error) {
		e.logger.Error(context.Background(), "interrupted persist failed",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
	}
}
