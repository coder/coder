package chatd

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"charm.land/fantasy"
	"github.com/dustin/go-humanize"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/notifications"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/webpush"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/quartz"
)

const (
	// DefaultPendingChatAcquireInterval is the default time between attempts to
	// acquire pending chats.
	DefaultPendingChatAcquireInterval = time.Second
	// DefaultInFlightChatStaleAfter is the default age after which a running
	// chat is considered stale and should be recovered.
	DefaultInFlightChatStaleAfter = 5 * time.Minute

	homeInstructionLookupTimeout = 5 * time.Second
	workspaceDialValidationDelay = 5 * time.Second
	turnStatusLabelWriteTimeout  = 5 * time.Second
	// defaultDialTimeout matches the timeout used by ~8 other
	// server-side AgentConn callers.
	defaultDialTimeout = 30 * time.Second
	// planPathLookupTimeout bounds resolving the per-chat plan path, which
	// dials the workspace agent to read its home directory. It must exceed
	// defaultDialTimeout so a cold dial, bounded internally by that timeout,
	// can finish before this outer budget fires, with a small margin for the
	// follow-up LS call.
	planPathLookupTimeout = defaultDialTimeout + 5*time.Second
	// DefaultChatHeartbeatInterval is the default time between chat
	// heartbeat updates while a chat is being processed.
	DefaultChatHeartbeatInterval = 30 * time.Second
	maxChatSteps                 = 1200

	// slowPrepareThreshold is the generation-preparation duration
	// above which a warning is logged. Preparation runs before
	// every generation step, so sustained slowness (workspace
	// dials, MCP connects) taxes the whole turn.
	slowPrepareThreshold = 30 * time.Second

	// maxConcurrentRecordingUploads caps the number of recording
	// stop-and-store operations that can run concurrently. Each
	// slot buffers up to MaxRecordingSize + MaxThumbnailSize
	// (110 MB) in memory, so this value implicitly bounds memory
	// to roughly maxConcurrentRecordingUploads * 110 MB.
	maxConcurrentRecordingUploads = 25

	// agentDisconnectedRecoveryThreshold is how long the latest
	// workspace agent must be disconnected before chatd suggests
	// destructive stop/start recovery. This is intentionally longer
	// than the inactive-disconnect timeout so short heartbeat gaps do
	// not prompt a workspace restart.
	agentDisconnectedRecoveryThreshold = 90 * time.Second

	// DefaultMaxChatsPerAcquire is the maximum number of chats to
	// acquire in a single processOnce call. Batching avoids
	// waiting a full polling interval between acquisitions
	// when many chats are pending.
	DefaultMaxChatsPerAcquire int32 = 10

	defaultSubagentInstruction = "You are running as a delegated sub-agent chat. Complete the delegated task and provide clear, concise assistant responses for the parent agent."

	// defaultAdvisorMaxOutputTokens caps the nested advisor response
	// when the admin config omits the field (or sets it to <= 0).
	// It is intentionally generous relative to the advisor's concise
	// guidance remit so short plans are not truncated mid-reasoning.
	defaultAdvisorMaxOutputTokens = 16384
)

var (
	errChatHasNoWorkspaceAgent = xerrors.New("workspace has no running agent: the workspace is likely stopped. Use the start_workspace tool to start it")
	errChatAgentDisconnected   = xerrors.New(
		"workspace agent has been disconnected for at least 90 seconds " +
			"and cannot execute tools. To recover, call stop_workspace " +
			"to stop the workspace, then start_workspace to start it " +
			"again",
	)
	errChatAgentNeverConnected = xerrors.New(
		"workspace agent never connected and its connection timeout has " +
			"elapsed, so it cannot execute tools. To recover, call " +
			"stop_workspace to stop the workspace, then start_workspace " +
			"to start it again",
	)
	errChatDialTimeout = xerrors.New(
		"connection to the workspace agent timed out. " +
			"The agent may still be reachable on the next attempt.",
	)
	errChatExternalAgentUnavailable = xerrors.New("external workspace agent unavailable")
	errInflightClosed               = xerrors.New("chatd server inflight closed")
)

type chatExternalAgentUnavailableError struct {
	message string
}

func (e chatExternalAgentUnavailableError) Error() string {
	return e.message
}

func (chatExternalAgentUnavailableError) Is(target error) bool {
	return target == errChatExternalAgentUnavailable
}

func newChatExternalAgentUnavailableError(agent database.WorkspaceAgent) error {
	return chatExternalAgentUnavailableError{
		message: chattool.ExternalAgentUnavailableMessage(agent),
	}
}

// Server handles background processing of pending chats.
type Server struct {
	cancel         context.CancelFunc
	ctx            context.Context
	wg             sync.WaitGroup
	inflight       sync.WaitGroup
	inflightMu     sync.Mutex
	inflightClosed atomic.Bool

	db                 database.Store
	workerID           uuid.UUID
	logger             slog.Logger
	modelConfigContext func(context.Context, uuid.UUID) (context.Context, error)

	streamPartsDialer StreamPartsDialer

	agentConnFn                    AgentConnFunc
	agentInactiveDisconnectTimeout time.Duration
	dialTimeout                    time.Duration
	instructionLookupTimeout       time.Duration
	createWorkspaceFn              chattool.CreateWorkspaceFn
	startWorkspaceFn               chattool.StartWorkspaceFn
	stopWorkspaceFn                chattool.StopWorkspaceFn
	pubsub                         pubsub.Pubsub
	webpushDispatcher              webpush.Dispatcher
	hooks                          *chathooks.Trigger
	providerAPIKeys                chatprovider.ProviderAPIKeys
	allowBYOK                      bool
	oidcTokenSource                mcpclient.UserOIDCTokenSource
	debugSvc                       *chatdebug.Service
	debugSvcFactory                func() *chatdebug.Service
	debugSvcReady                  atomic.Bool
	debugSvcInit                   sync.Once
	configCache                    *chatConfigCache
	configCacheUnsubscribe         func()
	providerCacheUnsubscribe       func()

	usageTracker         *workspacestats.UsageTracker
	clock                quartz.Clock
	metrics              *chatloop.Metrics
	chatWorker           *chatWorker
	messagePartBuffer    *messagepartbuffer.Buffer
	streamSyncPoller     *streamSyncPoller
	recordingSem         chan struct{}
	agentCapacityLimiter AgentCapacityLimiter

	aibridgeTransportFactory *atomic.Pointer[aibridge.TransportFactory]
	experiments              codersdk.Experiments

	// Configuration
	pendingChatAcquireInterval time.Duration
	maxChatsPerAcquire         int32
	inFlightChatStaleAfter     time.Duration
	chatHeartbeatInterval      time.Duration
}

func (p *Server) loadAdvisorConfig(ctx context.Context, logger slog.Logger) advisorRuntimeConfig {
	cfg, err := p.configCache.AdvisorConfig(ctx)
	if err != nil {
		logger.Warn(ctx, "failed to load advisor config", slog.Error(err))
		return advisorRuntimeConfig{}
	}
	return cfg
}

// stripAdvisorGuidanceBlock removes any system message whose text content
// matches chatadvisor.ParentGuidanceBlock after whitespace normalization.
// The block is meant for the parent agent (it advertises the advisor tool)
// and would waste context tokens if forwarded to the advisor's nested run.
func stripAdvisorGuidanceBlock(msgs []fantasy.Message) []fantasy.Message {
	filtered := msgs[:0]
	for _, msg := range msgs {
		if msg.Role == fantasy.MessageRoleSystem && isAdvisorGuidanceMessage(msg) {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

func isAdvisorGuidanceMessage(msg fantasy.Message) bool {
	if len(msg.Content) != 1 {
		return false
	}
	text, ok := msg.Content[0].(fantasy.TextPart)
	if !ok {
		return false
	}
	return strings.TrimSpace(text.Text) == strings.TrimSpace(chatadvisor.ParentGuidanceBlock)
}

const advisorOverrideContext = "advisor"

// resolveAdvisorModelOverride resolves the advisor model override for the
// chat's organization. Missing or unusable overrides fall back to the chat
// model. Linked-provider route and client failures remain hard failures.
func (p *Server) resolveAdvisorModelOverride(
	ctx context.Context,
	chat database.Chat,
	maxOutputTokens int64,
	modelOpts modelBuildOptions,
	logger slog.Logger,
) (resolvedModelCall, bool, error) {
	//nolint:gocritic // Chatd reads organization-scoped runtime configuration.
	override, err := p.db.GetChatOrganizationModelOverride(
		dbauthz.AsChatd(ctx),
		database.GetChatOrganizationModelOverrideParams{
			OrganizationID: chat.OrganizationID,
			Context:        advisorOverrideContext,
		},
	)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return resolvedModelCall{}, false, nil
		}
		logger.Warn(
			ctx,
			"failed to load advisor model override, continuing with chat model",
			slog.F("organization_id", chat.OrganizationID),
			slog.Error(err),
		)
		return resolvedModelCall{}, false, nil
	}

	modelCtx, modelCtxErr := p.callerModelConfigContext(ctx, chat.OwnerID)
	if modelCtxErr != nil {
		logger.Warn(
			ctx,
			"failed to load advisor model authorization, continuing with chat model",
			slog.F("model_config_id", override.ModelConfigID),
			slog.Error(modelCtxErr),
		)
		return resolvedModelCall{}, false, nil
	}
	// Re-read the model row for every runtime so disabled models or providers
	// stop routing advisor prompts immediately.
	overrideConfig, err := p.db.GetEnabledChatModelConfigByID(modelCtx, override.ModelConfigID)
	if err == nil && overrideConfig.OrganizationID != chat.OrganizationID {
		err = sql.ErrNoRows
	}
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			logger.Warn(
				ctx,
				"advisor model config is disabled or unavailable, continuing with chat model",
				slog.F("model_config_id", override.ModelConfigID),
			)
			return resolvedModelCall{}, false, nil
		}
		logger.Warn(
			ctx,
			"failed to resolve advisor model config, continuing with chat model",
			slog.F("model_config_id", override.ModelConfigID),
			slog.Error(err),
		)
		return resolvedModelCall{}, false, nil
	}

	resolved, err := p.resolveModelCall(ctx, modelCallSpec{
		purpose:         "advisor",
		chat:            chat,
		explicitConfig:  &overrideConfig,
		requestedEffort: ptr.FromNullString(override.ReasoningEffort),
		maxOutputTokens: ptr.Ref(maxOutputTokens),
		buildOptions:    modelOpts,
	})
	if err != nil {
		var parseErr modelCallConfigParseError
		if overrideConfig.AIProviderID.Valid && !xerrors.As(err, &parseErr) {
			return resolvedModelCall{}, false, xerrors.Errorf("resolve advisor override model: %w", err)
		}
		logger.Warn(
			ctx,
			"failed to resolve advisor override model, continuing with chat model",
			slog.F("model_config_id", override.ModelConfigID),
			slog.Error(err),
		)
		return resolvedModelCall{}, false, nil
	}
	return resolved, true, nil
}

func (p *Server) newAdvisorRuntime(
	ctx context.Context,
	chat database.Chat,
	advisorCfg advisorRuntimeConfig,
	modelOpts modelBuildOptions,
	logger slog.Logger,
) (*chatadvisor.Runtime, error) {
	maxUsesPerRun := advisorCfg.MaxUsesPerRun
	switch {
	case maxUsesPerRun == 0:
		// Advisor config treats 0 as unlimited, but the runtime
		// requires a positive bound. maxChatSteps is the
		// effective upper bound because advisor can run at most
		// once per loop step.
		maxUsesPerRun = maxChatSteps
	case maxUsesPerRun < 0:
		logger.Warn(
			ctx,
			"invalid advisor max uses per run, continuing without advisor",
			slog.F("max_uses_per_run", maxUsesPerRun),
		)
		return nil, nil //nolint:nilnil // Nil runtime with nil error means advisor is skipped for this turn.
	}

	maxOutputTokens := advisorCfg.MaxOutputTokens
	if maxOutputTokens <= 0 {
		maxOutputTokens = defaultAdvisorMaxOutputTokens
	}

	advisor, ok, err := p.resolveAdvisorModelOverride(
		ctx,
		chat,
		maxOutputTokens,
		modelOpts,
		logger,
	)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Without a usable override the advisor runs on the chat model,
		// resolved with the advisor's output cap and the config's default
		// reasoning effort. The configured advisor effort applies only to
		// the override model it was tuned for.
		advisor, err = p.resolveModelCall(ctx, modelCallSpec{
			purpose:         "advisor",
			chat:            chat,
			maxOutputTokens: &maxOutputTokens,
			buildOptions:    modelOpts,
		})
		if err != nil {
			logger.Warn(
				ctx,
				"failed to resolve advisor chat model, continuing without advisor",
				slog.Error(err),
			)
			return nil, nil //nolint:nilnil // Nil runtime with nil error means advisor is skipped for this turn.
		}
	}

	rt, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model:           advisor.model.LanguageModel(),
		CallTemplate:    advisor.newCall(),
		MaxUsesPerRun:   maxUsesPerRun,
		MaxOutputTokens: maxOutputTokens,
	})
	if err != nil {
		logger.Warn(
			ctx,
			"failed to create advisor runtime, continuing without advisor",
			slog.Error(err),
		)
		return nil, nil //nolint:nilnil // Nil runtime with nil error means advisor is skipped for this turn.
	}
	return rt, nil
}

// resolveWorkspaceMCPTools builds the workspace MCP tool set for a turn from
// the chat's pinned context snapshot (chat_context_resources). The agent
// reports its MCP servers in the snapshot it pushes, so a chat with no pinned
// rows, or one whose workspace advertises no MCP servers, contributes no
// workspace MCP tools. A read failure is logged and yields no tools rather
// than aborting the turn.
func (p *Server) resolveWorkspaceMCPTools(
	ctx context.Context,
	logger slog.Logger,
	chat database.Chat,
	workspaceCtx *turnWorkspaceContext,
) []fantasy.AgentTool {
	tools, err := p.pinnedWorkspaceMCPTools(ctx, chat, workspaceCtx.getWorkspaceConn)
	if err != nil {
		logger.Warn(ctx, "failed to read pinned workspace MCP tools",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return nil
	}
	return tools
}

// pinnedWorkspaceMCPTools builds workspace MCP tools from the chat's pinned
// context snapshot (chat_context_resources). Each tool still proxies its calls
// back through the workspace agent connection; the snapshot carries tool
// definitions, not a way to execute them, so execution requires a reachable
// agent. There is no per-chat cache to invalidate: a server removed or renamed
// in the workspace surfaces as a dirty chat on the agent's next push, and the
// user refreshes to re-pin, so a nil invalidate callback (a 404 no-op) is
// correct here.
func (p *Server) pinnedWorkspaceMCPTools(
	ctx context.Context,
	chat database.Chat,
	getConn func(context.Context) (workspacesdk.AgentConn, error),
) ([]fantasy.AgentTool, error) {
	resources, err := p.db.ListChatContextResourcesByChatID(ctx, chat.ID)
	if err != nil {
		return nil, xerrors.Errorf("list chat context resources: %w", err)
	}
	infos := workspaceMCPToolInfosFromResources(resources)
	return chattool.NewWorkspaceMCPTools(infos, getConn, nil), nil
}

type AgentConnFunc func(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error)

var (
	// ErrInvalidModelConfigID indicates the requested model config does not
	// exist, is disabled, or its provider is disabled.
	ErrInvalidModelConfigID = xerrors.New("invalid model config ID")
	// ErrEditedMessageNotFound indicates the edited message does not exist
	// in the target chat.
	ErrEditedMessageNotFound = xerrors.New("edited message not found")
	// ErrEditedMessageNotUser indicates a non-user message edit attempt.
	ErrEditedMessageNotUser = xerrors.New("only user messages can be edited")
	// ErrChatArchived indicates the chat is archived and cannot
	// accept modifications (messages, edits, promotions, or
	// tool-result submissions).
	ErrChatArchived = xerrors.New("chat is archived")
	// ErrNoDefaultChatModelConfig indicates no default chat model config
	// is configured, so chatd cannot resolve a model for the request.
	ErrNoDefaultChatModelConfig = chatstate.ErrNoDefaultChatModelConfig
	// ErrNothingToCompact indicates a manual compaction request found
	// no uncompressed conversation after the latest compaction
	// boundary, so running a compaction would produce nothing.
	ErrNothingToCompact = xerrors.New("nothing to compact")
	// ErrNothingToClear indicates a manual context clear found no
	// model-visible conversation after the latest context boundary,
	// so a clear would be a no-op.
	ErrNothingToClear = xerrors.New("nothing to clear")
)

// CreateOptions controls chat creation in the shared chat mutation path.
type CreateOptions struct {
	OrganizationID          uuid.UUID
	OwnerID                 uuid.UUID
	WorkspaceID             uuid.NullUUID
	BuildID                 uuid.NullUUID
	AgentID                 uuid.NullUUID
	ParentChatID            uuid.NullUUID
	RootChatID              uuid.NullUUID
	Title                   string
	TitleDerivedFromContent bool
	ModelConfigID           uuid.UUID
	ReasoningEffort         *string
	ChatMode                database.NullChatMode
	PlanMode                database.NullChatPlanMode
	ClientType              database.ChatClientType
	SystemPrompt            string
	InitialUserContent      []codersdk.ChatMessagePart
	MCPServerIDs            []uuid.UUID
	Labels                  database.StringMap
	DynamicTools            json.RawMessage
}

// SendMessageBusyBehavior controls what happens when a chat is already active.
type SendMessageBusyBehavior string

const (
	// SendMessageBusyBehaviorQueue queues user messages while the chat is busy.
	SendMessageBusyBehaviorQueue SendMessageBusyBehavior = "queue"
	// SendMessageBusyBehaviorInterrupt queues the message and
	// interrupts the active run. The queued message is
	// auto-promoted after the interrupted assistant response is
	// persisted, ensuring correct message ordering.
	SendMessageBusyBehaviorInterrupt SendMessageBusyBehavior = "interrupt"
)

// SendMessageOptions controls user message insertion with busy-state behavior.
type SendMessageOptions struct {
	ChatID          uuid.UUID
	CreatedBy       uuid.UUID
	Content         []codersdk.ChatMessagePart
	ModelConfigID   uuid.UUID
	ReasoningEffort *string
	BusyBehavior    SendMessageBusyBehavior
	PlanMode        *database.NullChatPlanMode
	MCPServerIDs    *[]uuid.UUID
}

// SendMessageResult contains the outcome of user message processing.
type SendMessageResult struct {
	Queued        bool
	QueuedMessage *database.ChatQueuedMessage
	Message       database.ChatMessage
	// InsertedMessages holds every message the send inserted, in
	// insertion order. A queued send on an errored chat can still
	// insert messages by promoting the previous queue head.
	InsertedMessages []database.ChatMessage
	Chat             database.Chat
}

// EditMessageOptions controls user message edits via soft-delete and re-insert.
type EditMessageOptions struct {
	ChatID          uuid.UUID
	CreatedBy       uuid.UUID
	EditedMessageID int64
	Content         []codersdk.ChatMessagePart
	// ModelConfigID, when non-zero, overrides the model used for
	// the replacement user message. When set to uuid.Nil the
	// original message's model is preserved.
	ModelConfigID   uuid.UUID
	ReasoningEffort *string
	// MCPServerIDs, when non-nil, replaces the chat's MCP server
	// selection before the replacement turn runs. When nil the
	// current selection is preserved.
	MCPServerIDs *[]uuid.UUID
}

// EditMessageResult contains the replacement user message and chat status.
type EditMessageResult struct {
	Message database.ChatMessage
	// InsertedMessages holds every message the edit inserted, in
	// insertion order: synthetic tool cancellations, the replacement
	// user message, then hook suffix messages.
	InsertedMessages []database.ChatMessage
	// DeletedMessageIDs holds every previously visible message the
	// edit soft-deleted.
	DeletedMessageIDs []int64
	Chat              database.Chat
}

// PromoteQueuedOptions controls queued-message promotion.
type PromoteQueuedOptions struct {
	ChatID          uuid.UUID
	CreatedBy       uuid.UUID
	QueuedMessageID int64
}

// PromoteQueuedResult contains post-promotion message metadata.
type PromoteQueuedResult struct {
	// PromotedMessage is the inserted user message. For a chat that
	// was running at promote time, the insertion is deferred to the
	// worker's auto-promote and PromotedMessage is the zero value.
	PromotedMessage database.ChatMessage
}

// forcedMCPServerConfigsForOwner filters enabled Force On configs
// through the chat owner's ACL so availability cannot widen access.
func forcedMCPServerConfigsForOwner(ctx context.Context, store database.Store, organizationID, ownerID uuid.UUID) ([]database.MCPServerConfig, error) {
	owner, _, err := httpmw.UserRBACSubject(ctx, store, ownerID, rbac.ScopeAll)
	if err != nil {
		return nil, xerrors.Errorf("load chat owner authorization: %w", err)
	}
	forced, err := store.GetForcedMCPServerConfigsByOrganization(dbauthz.As(ctx, owner), organizationID)
	if err != nil {
		return nil, xerrors.Errorf("get forced MCP server configs: %w", err)
	}
	return forced, nil
}

// enforceForcedMCPServerIDs appends owner-readable Force On config IDs
// missing from ids so callers cannot exclude such servers by stripping
// IDs from a request (Cure53 CDM-02-010).
func enforceForcedMCPServerIDs(ctx context.Context, store database.Store, organizationID, ownerID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	forced, err := forcedMCPServerConfigsForOwner(ctx, store, organizationID, ownerID)
	if err != nil {
		// Fail closed: proceeding without the forced set would
		// silently bypass a security policy.
		return nil, err
	}
	merged := slices.Clone(ids)
	if merged == nil {
		merged = []uuid.UUID{}
	}
	seen := make(map[uuid.UUID]struct{}, len(merged))
	for _, id := range merged {
		seen[id] = struct{}{}
	}
	for _, cfg := range forced {
		if _, ok := seen[cfg.ID]; !ok {
			merged = append(merged, cfg.ID)
		}
	}
	return merged, nil
}

// applyRequestedMCPServerIDs replaces the chat's MCP server selection
// inside the state-machine transaction when a request provides one.
// Explore child chats keep the spawn-time snapshot immutable. Force On
// MCP servers are enforced server-side so a caller cannot remove them
// by tampering with the update (Cure53 CDM-02-010).
func (p *Server) applyRequestedMCPServerIDs(ctx context.Context, store database.Store, lockedChat database.Chat, requested *[]uuid.UUID) (database.Chat, error) {
	if requested == nil {
		return lockedChat, nil
	}
	if isExploreSubagentMode(lockedChat.Mode) {
		p.logger.Warn(ctx,
			"ignoring explore subagent mcp server ids update, snapshot is immutable after spawn",
			slog.F("chat_id", lockedChat.ID),
		)
		return lockedChat, nil
	}
	enforcedIDs, err := enforceForcedMCPServerIDs(ctx, store, lockedChat.OrganizationID, lockedChat.OwnerID, *requested)
	if err != nil {
		return database.Chat{}, err
	}
	updated, err := store.UpdateChatMCPServerIDs(ctx, database.UpdateChatMCPServerIDsParams{
		ID:           lockedChat.ID,
		MCPServerIDs: enforcedIDs,
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("update chat mcp server ids: %w", err)
	}
	return updated, nil
}

// CreateChat creates a chat with its initial history through
// chatstate.CreateChat. The new chat starts in `running` status per
// the chat execution state model. Ownership hints wake chat workers.
func (p *Server) CreateChat(ctx context.Context, opts CreateOptions) (database.Chat, error) {
	if opts.OrganizationID == uuid.Nil {
		return database.Chat{}, xerrors.New("organization_id is required")
	}
	if opts.OwnerID == uuid.Nil {
		return database.Chat{}, xerrors.New("owner_id is required")
	}
	if strings.TrimSpace(opts.Title) == "" {
		return database.Chat{}, xerrors.New("title is required")
	}
	if len(opts.InitialUserContent) == 0 {
		return database.Chat{}, xerrors.New("initial user content is required")
	}
	// Ensure MCPServerIDs is non-nil so pq.Array produces '{}'
	// instead of SQL NULL, which violates the NOT NULL column
	// constraint.
	if opts.MCPServerIDs == nil {
		opts.MCPServerIDs = []uuid.UUID{}
	}
	// Force On MCP servers are enforced server-side so a caller
	// cannot exclude them by stripping IDs from the request
	// (Cure53 CDM-02-010).
	enforcedMCPServerIDs, err := enforceForcedMCPServerIDs(ctx, p.db, opts.OrganizationID, opts.OwnerID, opts.MCPServerIDs)
	if err != nil {
		return database.Chat{}, err
	}
	opts.MCPServerIDs = enforcedMCPServerIDs
	if opts.Labels == nil {
		opts.Labels = database.StringMap{}
	}
	opts.ClientType = cmp.Or(opts.ClientType, database.ChatClientTypeApi)
	if !opts.ClientType.Valid() {
		return database.Chat{}, xerrors.Errorf("invalid client_type: %q", opts.ClientType)
	}
	// Resolve the deployment prompt before opening the transaction so
	// chat creation does not hold one DB connection while waiting for
	// another pool checkout.
	deploymentPrompt := p.resolveDeploymentSystemPrompt(ctx)

	if opts.ModelConfigID != uuid.Nil {
		if err := requireEnabledChatModelConfig(ctx, p.db, opts.OrganizationID, opts.ModelConfigID); err != nil {
			return database.Chat{}, err
		}
	}

	labelsJSON, err := json.Marshal(opts.Labels)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("marshal labels: %w", err)
	}

	chatID := uuid.New()
	contentParts := opts.InitialUserContent
	if p.hooks.Enabled() {
		// Validate model admission before dispatch, matching the insert path.
		if err := validateCreateModelConfigID(ctx, p.db, opts.OrganizationID, opts.ModelConfigID); err != nil {
			return database.Chat{}, err
		}
		turnID := uuid.New()
		promptMessage, err := chathooks.UserPromptMessage(contentParts)
		if err != nil {
			return database.Chat{}, err
		}
		promptResult, err := p.hooks.Trigger(ctx, chathooks.Chat{
			ID:          chatID,
			OwnerID:     opts.OwnerID,
			WorkspaceID: opts.WorkspaceID,
			TurnID:      &turnID,
		}, promptMessage, agenthooks.EventUserPromptSubmit, dispatch.CapacityClassAdmission)
		if err != nil {
			return database.Chat{}, chathooks.UserPromptDenial(err)
		}
		composed, overridden, err := chathooks.ComposeUserPromptContent(contentParts, promptResult)
		if err != nil {
			return database.Chat{}, err
		}
		contentParts = composed
		// Avoid deriving titles from the prompt that policy replaced.
		if overridden && opts.TitleDerivedFromContent {
			opts.Title = chatprompt.FallbackTitle(chatprompt.TitleText(contentParts, nil))
		}
	}

	userPrompt := codersdk.SanitizePromptText(opts.SystemPrompt)
	workspaceAwareness := workspaceDetachedAwareness
	if opts.WorkspaceID.Valid {
		workspaceAwareness = workspaceAttachedAwareness
	}
	workspaceAwarenessContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText(workspaceAwareness),
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("marshal workspace awareness: %w", err)
	}
	userContent, err := chatprompt.MarshalParts(contentParts)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("marshal initial user content: %w", err)
	}

	var initialMessages []chatstate.Message
	if deploymentPrompt != "" {
		deploymentContent, marshalErr := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(deploymentPrompt),
		})
		if marshalErr != nil {
			return database.Chat{}, xerrors.Errorf("marshal deployment system prompt: %w", marshalErr)
		}
		initialMessages = append(initialMessages, systemMessage(deploymentContent, opts.ModelConfigID))
	}
	if userPrompt != "" {
		userPromptContent, marshalErr := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(userPrompt),
		})
		if marshalErr != nil {
			return database.Chat{}, xerrors.Errorf("marshal user system prompt: %w", marshalErr)
		}
		initialMessages = append(initialMessages, systemMessage(userPromptContent, opts.ModelConfigID))
	}
	initialMessages = append(initialMessages, systemMessage(workspaceAwarenessContent, opts.ModelConfigID))
	initialMessages = append(initialMessages, userMessage(userContent, opts.ModelConfigID, opts.OwnerID, opts.ReasoningEffort))

	result, err := chatstate.CreateChatWithID(ctx, p.db, p.pubsub, chatID, chatstate.CreateChatInput{
		OrganizationID:    opts.OrganizationID,
		OwnerID:           opts.OwnerID,
		WorkspaceID:       opts.WorkspaceID,
		BuildID:           opts.BuildID,
		AgentID:           opts.AgentID,
		ParentChatID:      opts.ParentChatID,
		RootChatID:        opts.RootChatID,
		LastModelConfigID: opts.ModelConfigID,
		Title:             opts.Title,
		Mode:              opts.ChatMode,
		PlanMode:          opts.PlanMode,
		MCPServerIDs:      opts.MCPServerIDs,
		Labels: pqtype.NullRawMessage{
			RawMessage: labelsJSON,
			Valid:      true,
		},
		DynamicTools: pqtype.NullRawMessage{
			RawMessage: opts.DynamicTools,
			Valid:      len(opts.DynamicTools) > 0,
		},
		ClientType:      opts.ClientType,
		InitialMessages: initialMessages,
		FileIDs:         chatprompt.FileIDs(contentParts),
	})
	if err != nil {
		return database.Chat{}, err
	}
	chat := result.Chat
	if !chat.RootChatID.Valid && !chat.ParentChatID.Valid {
		chat.RootChatID = uuid.NullUUID{UUID: chat.ID, Valid: true}
	}

	// Publish the sidebar watch event explicitly after chatstate has
	// committed and emitted its own state-machine notifications. The
	// watch endpoint is maintained separately from chatstate notifications.
	p.publishChatPubsubEvent(chat, codersdk.ChatWatchEventKindCreated, nil)

	// Pin the chat to the agent's latest context snapshot if one exists.
	// Best-effort: a chat created before its agent has pushed is hydrated
	// by that agent's next push.
	p.hydrateChatContextOnCreate(ctx, chat)
	return chat, nil
}

// SendMessage admits a user message through the chatstate.SendMessage
// transition. Pre-transition admission policy (usage limit, plan-mode
// metadata update, MCP server ID update, model-config resolution, queue
// cap) runs inside the same chatstate transaction via the transactional
// store so everything commits or rolls back together.
func (p *Server) SendMessage(
	ctx context.Context,
	opts SendMessageOptions,
) (SendMessageResult, error) {
	if opts.ChatID == uuid.Nil {
		return SendMessageResult{}, xerrors.New("chat_id is required")
	}
	if len(opts.Content) == 0 {
		return SendMessageResult{}, xerrors.New("content is required")
	}

	busyBehavior := opts.BusyBehavior
	if busyBehavior == "" {
		busyBehavior = SendMessageBusyBehaviorQueue
	}
	switch busyBehavior {
	case SendMessageBusyBehaviorQueue, SendMessageBusyBehaviorInterrupt:
	default:
		return SendMessageResult{}, xerrors.Errorf("invalid busy behavior %q", opts.BusyBehavior)
	}

	contentParts := opts.Content
	if p.hooks.Enabled() {
		turnID := uuid.New()
		chat, err := p.db.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return SendMessageResult{}, xerrors.Errorf("load chat for user_prompt_submit: %w", err)
		}
		// Repeat these admission checks under the transaction lock.
		if chat.Archived {
			return SendMessageResult{}, ErrChatArchived
		}
		if _, err := resolveSendMessageModelConfigID(ctx, p.db, chat, opts.ModelConfigID); err != nil {
			return SendMessageResult{}, err
		}
		// Check queue capacity before dispatch; the transaction
		// rechecks it under lock.
		queuedCount, err := p.db.CountChatQueuedMessages(ctx, opts.ChatID)
		if err != nil {
			return SendMessageResult{}, xerrors.Errorf("count queued messages: %w", err)
		}
		if queuedCount >= chatstate.MaxQueueSize {
			return SendMessageResult{}, &chatstate.MessageQueueFullError{Max: chatstate.MaxQueueSize}
		}
		promptMessage, err := chathooks.UserPromptMessage(contentParts)
		if err != nil {
			return SendMessageResult{}, err
		}
		promptResult, err := p.hooks.Trigger(ctx, chathooks.ChatFor(chat, &turnID), promptMessage, agenthooks.EventUserPromptSubmit, dispatch.CapacityClassAdmission)
		if err != nil {
			return SendMessageResult{}, p.handleUserPromptDispatchError(ctx, opts.ChatID, chathooks.UserPromptDenial(err))
		}
		contentParts, _, err = chathooks.ComposeUserPromptContent(contentParts, promptResult)
		if err != nil {
			return SendMessageResult{}, err
		}
	}

	content, err := chatprompt.MarshalParts(contentParts)
	if err != nil {
		return SendMessageResult{}, xerrors.Errorf("marshal message content: %w", err)
	}

	requestedPlanMode := opts.PlanMode
	requestedMCPServerIDs := opts.MCPServerIDs

	var result SendMessageResult
	machine := p.newChatMachine(opts.ChatID)
	updateErr := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		lockedChat, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}

		if lockedChat.Archived {
			return ErrChatArchived
		}

		if requestedPlanMode != nil {
			lockedChat, err = store.UpdateChatPlanModeByID(ctx, database.UpdateChatPlanModeByIDParams{
				PlanMode: *requestedPlanMode,
				ID:       opts.ChatID,
			})
			if err != nil {
				return xerrors.Errorf("update chat plan mode: %w", err)
			}
		}

		modelConfigID, err := resolveSendMessageModelConfigID(
			ctx,
			store,
			lockedChat,
			opts.ModelConfigID,
		)
		if err != nil {
			return err
		}

		lockedChat, err = p.applyRequestedMCPServerIDs(ctx, store, lockedChat, requestedMCPServerIDs)
		if err != nil {
			return err
		}

		messageCreatedBy := opts.CreatedBy
		if messageCreatedBy == uuid.Nil {
			messageCreatedBy = lockedChat.OwnerID
		}

		// Queue capacity is enforced inside tx.SendMessage; this
		// wrapper only propagates the typed error.
		message := userMessage(content, modelConfigID, messageCreatedBy, opts.ReasoningEffort)
		sendResult, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      message,
			BusyBehavior: busyBehaviorToChatState(busyBehavior),
		})
		if err != nil {
			return err
		}

		if sendResult.QueuedMessage != nil {
			result.Queued = true
			result.QueuedMessage = sendResult.QueuedMessage
		} else if len(sendResult.InsertedMessages) > 0 {
			// The state machine prepends synthetic tool-result
			// cancellation messages; the user message is always
			// last in the inserted slice.
			result.Message = sendResult.InsertedMessages[len(sendResult.InsertedMessages)-1]
		}
		// A queued send on an errored chat can also promote the
		// previous queue head into history; report those inserts so
		// clients can update their caches.
		result.InsertedMessages = sendResult.InsertedMessages

		// File-link errors must roll back the message.
		if err := chatstate.LinkFiles(ctx, store, opts.ChatID, chatprompt.FileIDs(contentParts)); err != nil {
			return err
		}
		// Capture the post-transition chat inside the same
		// transaction so the returned chat and the watch event
		// reflect the snapshot bump and status change produced by
		// the transition itself.
		refreshed, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("reload chat after send: %w", err)
		}
		result.Chat = refreshed
		return nil
	})
	if updateErr != nil {
		return SendMessageResult{}, updateErr
	}

	// Sidebar watch event keeps the chat list in sync. Stream side
	// effects are handled by chat:update consumers.
	p.publishChatPubsubEvent(result.Chat, codersdk.ChatWatchEventKindStatusChange, nil)
	return result, nil
}

func (p *Server) callerModelConfigContext(ctx context.Context, ownerID uuid.UUID) (context.Context, error) {
	if p.modelConfigContext == nil {
		return ctx, nil
	}
	return p.modelConfigContext(ctx, ownerID)
}

func callerModelConfigContext(
	ctx context.Context,
	store database.Store,
	ownerID uuid.UUID,
) (context.Context, error) {
	if ownerID == uuid.Nil {
		return ctx, nil
	}
	if actor, ok := dbauthz.ActorFromContext(ctx); ok &&
		actor.Type == rbac.SubjectTypeUser && actor.ID == ownerID.String() {
		return ctx, nil
	}
	actor, _, err := httpmw.UserRBACSubject(ctx, store, ownerID, rbac.ScopeAll)
	if err != nil {
		return nil, xerrors.Errorf("load model config authorization: %w", err)
	}
	//nolint:gocritic // Background Chatd work must use the chat owner's model ACLs.
	return dbauthz.As(ctx, actor), nil
}

func resolveSendMessageModelConfigID(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	requested uuid.UUID,
) (uuid.UUID, error) {
	if requested == uuid.Nil {
		return resolveFallbackModelConfigID(ctx, store, chat, chat.LastModelConfigID)
	}

	if err := requireEnabledChatModelConfig(ctx, store, chat.OrganizationID, requested); err != nil {
		return uuid.Nil, err
	}
	return requested, nil
}

// requireEnabledChatModelConfig rechecks enabled state inside the daemon.
// The coderd preflight can race an admin disabling the model or provider.
func requireEnabledChatModelConfig(
	ctx context.Context,
	store database.Store,
	organizationID uuid.UUID,
	modelConfigID uuid.UUID,
) error {
	config, err := store.GetEnabledChatModelConfigByID(ctx, modelConfigID)
	if err == nil {
		if config.OrganizationID == organizationID {
			return nil
		}
		err = sql.ErrNoRows
	}
	if errors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf(
			"%w: %s",
			ErrInvalidModelConfigID,
			modelConfigID,
		)
	}
	return xerrors.Errorf(
		"get requested model config %s: %w",
		modelConfigID,
		err,
	)
}

func validateCreateModelConfigID(
	ctx context.Context,
	store database.Store,
	organizationID uuid.UUID,
	modelConfigID uuid.UUID,
) error {
	if modelConfigID == uuid.Nil {
		return xerrors.Errorf("%w: %s", ErrInvalidModelConfigID, modelConfigID)
	}
	config, err := store.GetChatModelConfigByID(ctx, modelConfigID)
	if err == nil {
		if config.OrganizationID == organizationID {
			return nil
		}
		err = sql.ErrNoRows
	}
	if errors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("%w: %s", ErrInvalidModelConfigID, modelConfigID)
	}
	return xerrors.Errorf("get requested model config %s: %w", modelConfigID, err)
}

func resolveFallbackModelConfigID(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	modelConfigID uuid.UUID,
) (uuid.UUID, error) {
	if modelConfigID != uuid.Nil {
		config, err := store.GetEnabledChatModelConfigByID(ctx, modelConfigID)
		if err == nil {
			if config.OrganizationID == chat.OrganizationID {
				return modelConfigID, nil
			}
		} else if !errors.Is(err, sql.ErrNoRows) && !dbauthz.IsNotAuthorizedError(err) {
			return uuid.Nil, xerrors.Errorf(
				"get chat model config %s: %w",
				modelConfigID,
				err,
			)
		}
	}

	defaultConfig, err := effectiveDefaultChatModelConfig(ctx, store, chat.OrganizationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, ErrNoDefaultChatModelConfig
		}
		return uuid.Nil, xerrors.Errorf("get default chat model config: %w", err)
	}
	return defaultConfig.ID, nil
}

func validateModelConfigOverride(
	ctx context.Context,
	store database.Store,
	organizationID uuid.UUID,
	requested uuid.UUID,
) (uuid.NullUUID, error) {
	if requested == uuid.Nil {
		return uuid.NullUUID{}, nil
	}
	if err := requireEnabledChatModelConfig(ctx, store, organizationID, requested); err != nil {
		return uuid.NullUUID{}, err
	}
	return uuid.NullUUID{UUID: requested, Valid: true}, nil
}

func validateEditTarget(ctx context.Context, store database.Store, chatID uuid.UUID, messageID int64) error {
	target, err := store.GetChatMessageByID(ctx, messageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEditedMessageNotFound
		}
		return xerrors.Errorf("get edited message: %w", err)
	}
	if target.ChatID != chatID || target.Deleted {
		return ErrEditedMessageNotFound
	}
	if target.Role != database.ChatMessageRoleUser {
		return ErrEditedMessageNotUser
	}
	return nil
}

func loadEffectiveChatModelConfigs(
	ctx context.Context,
	store database.Store,
	organizationID uuid.UUID,
) (database.EffectiveChatModelConfigs, error) {
	rows, err := store.GetEnabledChatModelConfigsByOrganization(ctx, organizationID)
	if err != nil {
		return database.EffectiveChatModelConfigs{}, err
	}
	return database.DeriveEffectiveChatModelConfigs(rows), nil
}

func effectiveDefaultChatModelConfig(
	ctx context.Context,
	store database.Store,
	organizationID uuid.UUID,
) (database.ChatModelConfig, error) {
	effective, err := loadEffectiveChatModelConfigs(ctx, store, organizationID)
	if err != nil {
		return database.ChatModelConfig{}, err
	}
	if effective.DefaultConfig.ID == uuid.Nil {
		return database.ChatModelConfig{}, sql.ErrNoRows
	}
	return effective.DefaultConfig, nil
}

// enabledChatModelConfigsForOrganization returns enabled configs from the chat organization.
func enabledChatModelConfigsForOrganization(
	ctx context.Context,
	store database.Store,
	organizationID uuid.UUID,
) ([]database.GetEnabledChatModelConfigsByOrganizationRow, error) {
	effective, err := loadEffectiveChatModelConfigs(ctx, store, organizationID)
	if err != nil {
		return nil, err
	}
	return effective.Configs, nil
}

// EditMessage replaces an earlier user message and discards the
// active-history suffix through chatstate.EditMessage. Model-config
// override validation and usage-limit admission run in the same
// transaction as the state-machine transition.
func (p *Server) EditMessage(
	ctx context.Context,
	opts EditMessageOptions,
) (EditMessageResult, error) {
	if opts.ChatID == uuid.Nil {
		return EditMessageResult{}, xerrors.New("chat_id is required")
	}
	if opts.EditedMessageID <= 0 {
		return EditMessageResult{}, xerrors.New("edited_message_id is required")
	}
	if len(opts.Content) == 0 {
		return EditMessageResult{}, xerrors.New("content is required")
	}

	contentParts := opts.Content
	var sessionStartHookResult *chathooks.Result
	if p.hooks.Enabled() {
		turnID := uuid.New()
		chat, err := p.db.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return EditMessageResult{}, xerrors.Errorf("load chat for edit hooks: %w", err)
		}
		// Repeat these admission checks under the transaction lock.
		if chat.Archived {
			return EditMessageResult{}, ErrChatArchived
		}
		if err := validateEditTarget(ctx, p.db, opts.ChatID, opts.EditedMessageID); err != nil {
			return EditMessageResult{}, err
		}
		if _, err := validateModelConfigOverride(ctx, p.db, chat.OrganizationID, opts.ModelConfigID); err != nil {
			return EditMessageResult{}, err
		}
		sessionStartHookResult, err = p.hooks.Trigger(ctx, chathooks.ChatFor(chat, &turnID), chathooks.Message{Source: chathooks.SessionStartSourceClear}, agenthooks.EventSessionStart, dispatch.CapacityClassAdmission)
		if err != nil {
			return EditMessageResult{}, p.handleAPIDispatchError(ctx, opts.ChatID, agenthooks.EventSessionStart, err)
		}
		promptMessage, err := chathooks.UserPromptMessage(contentParts)
		if err != nil {
			return EditMessageResult{}, err
		}
		promptResult, err := p.hooks.Trigger(ctx, chathooks.ChatFor(chat, &turnID), promptMessage, agenthooks.EventUserPromptSubmit, dispatch.CapacityClassAdmission)
		if err != nil {
			return EditMessageResult{}, p.handleUserPromptDispatchError(ctx, opts.ChatID, chathooks.UserPromptDenial(err))
		}
		contentParts, _, err = chathooks.ComposeUserPromptContent(contentParts, promptResult)
		if err != nil {
			return EditMessageResult{}, err
		}
	}

	content, err := chatprompt.MarshalParts(contentParts)
	if err != nil {
		return EditMessageResult{}, xerrors.Errorf("marshal message content: %w", err)
	}
	var (
		result        EditMessageResult
		editedMsg     database.ChatMessage
		editedCutoffT time.Time
	)
	machine := p.newChatMachine(opts.ChatID)
	err = machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		lockedChat, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if lockedChat.Archived {
			return ErrChatArchived
		}
		// Capture the target message for the post-commit debug
		// cleanup hook below. The transition itself revalidates
		// chat ownership and user-message constraints.
		target, err := store.GetChatMessageByID(ctx, opts.EditedMessageID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrEditedMessageNotFound
			}
			return xerrors.Errorf("get edited message: %w", err)
		}
		if target.ChatID != opts.ChatID {
			return ErrEditedMessageNotFound
		}
		if target.Deleted {
			return ErrEditedMessageNotFound
		}
		if target.Role != database.ChatMessageRoleUser {
			return ErrEditedMessageNotUser
		}
		editedMsg = target

		lockedChat, err = p.applyRequestedMCPServerIDs(ctx, store, lockedChat, opts.MCPServerIDs)
		if err != nil {
			return err
		}

		modelOverride, err := validateModelConfigOverride(ctx, store, lockedChat.OrganizationID, opts.ModelConfigID)
		if err != nil {
			return err
		}
		if !modelOverride.Valid {
			// Without an explicit override the transition preserves
			// the edited message's original model, which may have been
			// disabled since; resolve it like a normal message send.
			preserved := uuid.Nil
			if target.ModelConfigID.Valid {
				preserved = target.ModelConfigID.UUID
			}
			resolved, err := resolveFallbackModelConfigID(ctx, store, lockedChat, preserved)
			if err != nil {
				return err
			}
			if resolved != preserved {
				modelOverride = uuid.NullUUID{UUID: resolved, Valid: true}
			}
		}

		modelConfigID := target.ModelConfigID.UUID
		if modelOverride.Valid {
			modelConfigID = modelOverride.UUID
		}
		// The prompt response already rides in the replacement content;
		// only the session_start(clear) response needs transcript rows.
		// They insert after the replacement so a later edit's suffix
		// truncation cleans them up.
		suffixMessages, err := chathooks.EventMessages(sessionStartHookResult, modelConfigID)
		if err != nil {
			return err
		}

		var reasoningEffortOverride database.NullChatReasoningEffort
		if opts.ReasoningEffort != nil && *opts.ReasoningEffort != "" {
			reasoningEffortOverride = database.NullChatReasoningEffort{ChatReasoningEffort: database.ChatReasoningEffort(*opts.ReasoningEffort), Valid: true}
		}

		editResult, err := tx.EditMessage(chatstate.EditMessageInput{
			MessageID:               opts.EditedMessageID,
			SuffixMessages:          suffixMessages,
			CreatedBy:               opts.CreatedBy,
			Content:                 content,
			ModelConfigIDOverride:   modelOverride,
			ReasoningEffortOverride: reasoningEffortOverride,
		})
		if err != nil {
			if errors.Is(err, chatstate.ErrEditedMessageNotUser) {
				return ErrEditedMessageNotUser
			}
			return err
		}
		result.Message = editResult.ReplacementMessage
		inserted := make([]database.ChatMessage, 0, len(editResult.CancellationMessages)+len(editResult.SuffixMessages)+1)
		inserted = append(inserted, editResult.CancellationMessages...)
		inserted = append(inserted, editResult.ReplacementMessage)
		inserted = append(inserted, editResult.SuffixMessages...)
		result.InsertedMessages = inserted
		result.DeletedMessageIDs = editResult.DeletedMessageIDs
		if err := chatstate.LinkFiles(ctx, store, opts.ChatID, chatprompt.FileIDs(contentParts)); err != nil {
			return err
		}
		// Capture the post-edit chat inside the same transaction so
		// the returned chat and the debug-cleanup cutoff use the
		// snapshot bump and updated_at stamped by the transition.
		refreshed, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("reload chat after edit: %w", err)
		}
		result.Chat = refreshed
		editedCutoffT = refreshed.UpdatedAt
		return nil
	})
	if err != nil {
		return EditMessageResult{}, err
	}

	// Sidebar watch event keeps the chat list responsive. Stream
	// side effects are handled by chat:update consumers.
	p.publishChatPubsubEvent(result.Chat, codersdk.ChatWatchEventKindStatusChange, nil)

	// Editing can race with an interrupted worker still flushing its
	// final debug writes. Run a short bounded retry loop so we converge
	// quickly without relying on the much longer stale-finalization
	// sweep. Source editCutoff from the DB-stamped updated_at returned
	// by the post-edit chat row so the filter uses the same clock that
	// stamps replacement-turn debug rows; subtract
	// debugCleanupClockSkew so replica clock drift cannot let the retry
	// delete a replacement turn's debug rows.
	editCutoff := editedCutoffT.Add(-debugCleanupClockSkew)
	p.scheduleDebugCleanup(
		ctx,
		"failed to delete chat debug rows after edit",
		[]slog.Field{
			slog.F("chat_id", opts.ChatID),
			slog.F("edited_message_id", editedMsg.ID),
		},
		func(cleanupCtx context.Context, debugSvc *chatdebug.Service) error {
			_, err := debugSvc.DeleteAfterMessageID(cleanupCtx, opts.ChatID, editedMsg.ID-1, editCutoff)
			return err
		},
	)

	return result, nil
}

// ErrArchiveRequiresRootChat is returned by [Server.ArchiveChat] and
// [Server.UnarchiveChat] when the supplied chat is a child chat.
// Archive state changes must always target the root chat so the
// whole family flips together.
var ErrArchiveRequiresRootChat = xerrors.New(
	"chat archive state can only be changed on the root chat",
)

// ArchiveChat archives a root chat and every child in its family
// through the chatstate state machine. The transition is atomic over
// the whole family: either every member is archived or none is. The
// state machine only permits archive from the idle / error execution
// states (W, E0, E1); active members cause a state conflict that the
// HTTP handler maps to a client error.
//
// Child chats must not be archived independently. ArchiveChat
// rejects them with [ErrArchiveRequiresRootChat] so callers cannot
// silently break the parent-implies-child archive invariant.
//
//nolint:staticcheck // Receiver name matches the other Server methods in this file.
func (p *Server) ArchiveChat(ctx context.Context, chat database.Chat) error {
	if chat.ID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}
	if chat.ParentChatID.Valid {
		return ErrArchiveRequiresRootChat
	}
	return p.setChatFamilyArchived(ctx, chat, true, codersdk.ChatWatchEventKindDeleted)
}

// UnarchiveChat unarchives a root chat and every child in its family
// through the chatstate state machine. Like ArchiveChat the cascade
// is atomic; ChildChat unarchive attempts are rejected with
// [ErrArchiveRequiresRootChat].
func (p *Server) UnarchiveChat(ctx context.Context, chat database.Chat) error {
	if chat.ID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}
	if chat.ParentChatID.Valid {
		return ErrArchiveRequiresRootChat
	}
	return p.setChatFamilyArchived(ctx, chat, false, codersdk.ChatWatchEventKindCreated)
}

// setChatFamilyArchived applies SetArchived(archived) to every chat
// in chat's family through chatstate. The transaction-captured
// family rows feed the post-commit debug cleanup and sidebar watch
// events. Callers must only invoke this for root chats.
//
//nolint:revive // Existing API takes the target archive state as a boolean.
func (p *Server) setChatFamilyArchived(
	ctx context.Context,
	chat database.Chat,
	archived bool,
	watchKind codersdk.ChatWatchEventKind,
) error {
	if chat.ID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}
	if chat.ParentChatID.Valid {
		return ErrArchiveRequiresRootChat
	}

	familyChats, err := chatstate.SetFamilyArchived(
		ctx,
		p.db,
		p.pubsub,
		chatstate.SetFamilyArchivedInput{
			RootID:   chat.ID,
			Archived: archived,
		},
	)
	if err != nil {
		return err
	}

	if archived {
		p.scheduleArchiveDebugCleanup(ctx, familyChats)
	}

	p.publishChatPubsubEvents(familyChats, watchKind)
	return nil
}

// DeleteQueued removes a queued user message through the chatstate
// state machine. Stream side effects are handled by chat:update
// consumers.
func (p *Server) DeleteQueued(
	ctx context.Context,
	chatID uuid.UUID,
	queuedMessageID int64,
) error {
	if chatID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}

	machine := p.newChatMachine(chatID)
	err := machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.DeleteQueuedMessage(chatstate.DeleteQueuedMessageInput{
			QueuedMessageID: queuedMessageID,
		})
		return err
	})
	return err
}

// PromoteQueued promotes a queued message through the chatstate state
// machine. From running / interrupting states the state machine
// transitions the chat to `interrupting` so the worker can drain the
// in-flight generation before promoting; from idle / error / requires
// action states it inserts the user message into history
// synchronously.
func (p *Server) PromoteQueued(
	ctx context.Context,
	opts PromoteQueuedOptions,
) (PromoteQueuedResult, error) {
	if opts.ChatID == uuid.Nil {
		return PromoteQueuedResult{}, xerrors.New("chat_id is required")
	}

	var (
		result      PromoteQueuedResult
		refreshChat database.Chat
		refreshedOK bool
	)
	machine := p.newChatMachine(opts.ChatID)
	updateErr := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		lockedChat, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if lockedChat.Archived {
			return ErrChatArchived
		}

		promoteResult, err := tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{
			QueuedMessageID: opts.QueuedMessageID,
		})
		if err != nil {
			return err
		}
		if promoteResult.InsertedMessage != nil {
			result.PromotedMessage = *promoteResult.InsertedMessage
		}
		// Capture the chat inside the transaction so the watch event
		// published below uses the snapshot bump and status change
		// produced by the transition itself.
		refreshed, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("reload chat after promote: %w", err)
		}
		refreshChat = refreshed
		refreshedOK = true
		return nil
	})
	if updateErr != nil {
		return PromoteQueuedResult{}, updateErr
	}

	if refreshedOK {
		p.publishChatPubsubEvent(refreshChat, codersdk.ChatWatchEventKindStatusChange, nil)
	}
	return result, nil
}

// SubmitToolResultsOptions controls tool result submission.
type SubmitToolResultsOptions struct {
	ChatID        uuid.UUID
	UserID        uuid.UUID
	ModelConfigID uuid.UUID
	Results       []codersdk.ToolResult
	DynamicTools  json.RawMessage
}

// ToolResultValidationError indicates the submitted tool results
// failed validation (e.g. missing, duplicate, or unexpected IDs,
// or invalid JSON output).
type ToolResultValidationError struct {
	Message string
	Detail  string
}

func (e *ToolResultValidationError) Error() string {
	if e.Detail != "" {
		return e.Message + ": " + e.Detail
	}
	return e.Message
}

// ToolResultStatusConflictError indicates the chat is not in the
// requires_action state expected for tool result submission.
type ToolResultStatusConflictError struct {
	ActualStatus database.ChatStatus
}

func (e *ToolResultStatusConflictError) Error() string {
	return fmt.Sprintf(
		"chat status is %q, expected %q",
		e.ActualStatus, database.ChatStatusRequiresAction,
	)
}

// SubmitToolResults dispatches hooks before completing the
// requires_action transition.
func (p *Server) SubmitToolResults(
	ctx context.Context,
	opts SubmitToolResultsOptions,
) error {
	machine := p.newChatMachine(opts.ChatID)
	var hookSuffix []chatstate.Message
	if p.hooks.Enabled() {
		state, err := loadDynamicPostToolUseState(ctx, machine, opts)
		if err != nil {
			return err
		}
		for _, result := range opts.Results {
			response, err := p.hooks.Trigger(ctx, chathooks.ChatFor(state.chat, nil), chathooks.DynamicPostToolUseMessage(result, state.toolNames[result.ToolCallID]), agenthooks.EventPostToolUse, dispatch.CapacityClassGeneration)
			if err != nil {
				// Leave pending calls intact so the client can resubmit after recovery.
				return chathooks.GenerationDispatchError(agenthooks.EventPostToolUse, err)
			}
			responseMessages, err := chathooks.EventMessages(response, state.modelConfigID)
			if err != nil {
				return err
			}
			hookSuffix = append(hookSuffix, responseMessages...)
		}
	}

	var (
		statusConflict *ToolResultStatusConflictError
		refreshChat    database.Chat
		refreshedOK    bool
	)
	updateErr := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		locked, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if locked.Archived {
			return ErrChatArchived
		}

		toolResults := make([]chatstate.ToolResultInput, 0, len(opts.Results))
		for _, result := range opts.Results {
			toolResults = append(toolResults, chatstate.ToolResultInput{
				ToolCallID: result.ToolCallID,
				Output:     result.Output,
				IsError:    result.IsError,
			})
		}
		modelConfigID := opts.ModelConfigID
		if modelConfigID == uuid.Nil {
			modelConfigID = locked.LastModelConfigID
		}
		if _, err := tx.CompleteRequiresAction(chatstate.CompleteRequiresActionInput{
			CreatedBy:      opts.UserID,
			ModelConfigID:  modelConfigID,
			Results:        toolResults,
			SuffixMessages: hookSuffix,
		}); err != nil {
			if !errors.Is(err, chatstate.ErrInvalidState) &&
				locked.Status != database.ChatStatusRequiresAction &&
				errors.Is(err, chatstate.ErrTransitionNotAllowed) {
				statusConflict = &ToolResultStatusConflictError{
					ActualStatus: locked.Status,
				}
				return statusConflict
			}
			return xerrors.Errorf("complete requires action: %w", err)
		}
		refreshed, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("reload chat after tool results: %w", err)
		}
		refreshChat = refreshed
		refreshedOK = true
		return nil
	})
	if updateErr != nil {
		if statusConflict != nil {
			return statusConflict
		}
		return translateToolResultValidationError(updateErr)
	}

	if refreshedOK {
		p.publishChatPubsubEvent(refreshChat, codersdk.ChatWatchEventKindStatusChange, nil)
	}
	return nil
}

// translateToolResultValidationError converts a chatstate tool-result
// validation error into the legacy chatd.ToolResultValidationError
// shape so HTTP handlers preserve their existing response detail. If
// err is not a tool-result validation error, it is returned
// unchanged.
func translateToolResultValidationError(err error) error {
	var v *chatstate.ToolResultValidationError
	if !errors.As(err, &v) {
		return err
	}
	switch {
	case xerrors.Is(v, chatstate.ErrToolResultDuplicate):
		return &ToolResultValidationError{
			Message: "Duplicate tool_call_id in results.",
			Detail:  fmt.Sprintf("Duplicate tool call ID %q.", v.ToolCallID),
		}
	case xerrors.Is(v, chatstate.ErrToolResultMissing):
		return &ToolResultValidationError{
			Message: "Missing tool result.",
			Detail:  fmt.Sprintf("Missing result for tool call %q.", v.ToolCallID),
		}
	case xerrors.Is(v, chatstate.ErrToolResultUnexpected):
		return &ToolResultValidationError{
			Message: "Unexpected tool result.",
			Detail:  fmt.Sprintf("No pending tool call with ID %q.", v.ToolCallID),
		}
	case xerrors.Is(v, chatstate.ErrToolResultInvalidJSON):
		return &ToolResultValidationError{
			Message: "Tool result output must be valid JSON.",
			Detail:  fmt.Sprintf("Output for tool call %q is not valid JSON.", v.ToolCallID),
		}
	default:
		return err
	}
}

// InterruptChat interrupts execution through the chatstate.Interrupt
// transition. Active runs land in `interrupting`; requires-action
// chats synthesize cancellation messages and return to running.
//
// Returns the post-transition chat and an error so callers can map
// state conflicts deliberately. Idle chats return a
// chatstate.ErrTransitionNotAllowed wrapper.
func (p *Server) InterruptChat(
	ctx context.Context,
	chat database.Chat,
) (database.Chat, error) {
	if chat.ID == uuid.Nil {
		return chat, xerrors.New("chat_id is required")
	}

	var refreshed database.Chat
	machine := p.newChatMachine(chat.ID)
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := tx.Interrupt(chatstate.InterruptInput{
			Reason: "Tool execution interrupted by user",
		}); err != nil {
			return err
		}
		// Capture the post-interrupt chat inside the transaction so
		// the returned chat and the watch event reflect the snapshot
		// bump and status change produced by the transition itself.
		latest, err := store.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("reload chat after interrupt: %w", err)
		}
		refreshed = latest
		return nil
	})
	if err != nil {
		return chat, err
	}

	p.publishChatPubsubEvent(refreshed, codersdk.ChatWatchEventKindStatusChange, nil)
	return refreshed, nil
}

// CompactChat records a manual compaction request through the
// chatstate.RequestCompaction transition and wakes workers. The chat
// must be idle (waiting) or errored; the request clears any stored
// error. The worker then generates and commits the compaction summary
// through the normal generation loop, bypassing the usage threshold,
// and the chat returns to waiting with no assistant follow-up unless
// queued messages remain or a post_compact hook commits a
// user-visible message.
//
// Returns the post-transition chat and an error so callers can map
// state conflicts deliberately: archived chats return ErrChatArchived,
// generating chats return a chatstate.ErrTransitionNotAllowed wrapper,
// and chats with no compactable conversation return
// ErrNothingToCompact.
func (p *Server) CompactChat(
	ctx context.Context,
	chat database.Chat,
) (database.Chat, error) {
	if chat.ID == uuid.Nil {
		return chat, xerrors.New("chat_id is required")
	}

	var refreshed database.Chat
	machine := p.newChatMachine(chat.ID)
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		lockedChat, err := store.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if lockedChat.Archived {
			return ErrChatArchived
		}
		// Run the transition before content and usage validation so busy
		// chats surface the state conflict first.
		result, err := tx.RequestCompaction(chatstate.RequestCompactionInput{})
		if err != nil {
			return err
		}
		// Reject requests with nothing to compact inside the same
		// transaction (rolling back the transition) so no LLM call
		// is ever started for an empty or already-compacted chat.
		// This also covers a double-/compact.
		messages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if err != nil {
			return xerrors.Errorf("load chat messages: %w", err)
		}
		boundary := latestContextBoundaryIndex(messages)
		if _, ok := firstUncompressedAssistantAfter(messages, boundary); !ok {
			return ErrNothingToCompact
		}
		refreshed = result.Chat
		return nil
	})
	if err != nil {
		return chat, err
	}

	p.publishChatPubsubEvent(refreshed, codersdk.ChatWatchEventKindStatusChange, nil)
	return refreshed, nil
}

// ClearChat commits a manual context reset through the
// chatstate.ClearContext transition. Unlike CompactChat it is fully
// synchronous: the boundary rows commit in this transaction with no
// model call, the transcript is preserved, and any stored error is
// cleared. Archived chats return ErrChatArchived, busy chats return a
// chatstate.ErrTransitionNotAllowed wrapper, and chats with nothing
// after the latest context boundary return ErrNothingToClear.
func (p *Server) ClearChat(
	ctx context.Context,
	chat database.Chat,
) (database.Chat, error) {
	if chat.ID == uuid.Nil {
		return chat, xerrors.New("chat_id is required")
	}

	var refreshed database.Chat
	machine := p.newChatMachine(chat.ID)
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		lockedChat, err := store.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if lockedChat.Archived {
			return ErrChatArchived
		}
		// Read the pre-clear history before the transition inserts the
		// boundary rows; eligibility is evaluated afterwards so busy
		// chats surface the state conflict first.
		messages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if err != nil {
			return xerrors.Errorf("load chat messages: %w", err)
		}
		clearMessages, err := buildClearMessages(buildClearMessagesInput{
			modelConfigID: lockedChat.LastModelConfigID,
			toolCallID:    "chat_cleared_" + uuid.NewString(),
		})
		if err != nil {
			return xerrors.Errorf("build clear messages: %w", err)
		}
		result, err := tx.ClearContext(chatstate.ClearContextInput{Messages: clearMessages})
		if err != nil {
			return err
		}
		// Reject no-op clears inside the same transaction so an empty
		// or already-cleared chat never gains a duplicate boundary.
		boundary := latestContextBoundaryIndex(messages)
		if !hasClearableMessageAfter(messages, boundary) {
			return ErrNothingToClear
		}
		refreshed = result.Chat
		return nil
	})
	if err != nil {
		return chat, err
	}

	p.publishChatPubsubEvent(refreshed, codersdk.ChatWatchEventKindStatusChange, nil)
	return refreshed, nil
}

// ReconcileInvalidStateChat recovers a chat stuck in an invalid
// execution-state combination by running the
// chatstate.ReconcileInvalidState transition. The chat lands in an
// error state (E0/E1); queued messages are preserved and pending
// dynamic-tool calls are closed with synthetic cancellations.
//
// Returns the post-transition chat. When the chat is not actually in an
// invalid state the transition returns a wrapped
// chatstate.ErrTransitionNotAllowed; a missing chat returns
// chatstate.ErrChatNotFound. Callers map these to deliberate HTTP
// responses.
func (p *Server) ReconcileInvalidStateChat(
	ctx context.Context,
	chat database.Chat,
) (database.Chat, error) {
	if chat.ID == uuid.Nil {
		return chat, xerrors.New("chat_id is required")
	}

	var refreshed database.Chat
	machine := p.newChatMachine(chat.ID)
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := tx.ReconcileInvalidState(chatstate.ReconcileInvalidStateInput{}); err != nil {
			return err
		}
		// Capture the post-reconcile chat inside the transaction so
		// the returned chat and the watch event reflect the snapshot
		// bump and status change produced by the transition itself.
		latest, err := store.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("reload chat after reconcile: %w", err)
		}
		refreshed = latest
		return nil
	})
	if err != nil {
		return chat, err
	}

	p.publishChatPubsubEvent(refreshed, codersdk.ChatWatchEventKindStatusChange, nil)
	return refreshed, nil
}

const manualTitleMessageWindowLimit = 50

// generatedChatTitle carries the title produced by the detached
// automatic title-generation goroutine. maybeGenerateChatTitle stores
// the generated title here so tests can observe it without a database
// read; the title_change pubsub event it publishes remains the source of
// truth for clients.
type generatedChatTitle struct {
	mu    sync.RWMutex
	title string
}

func (t *generatedChatTitle) Store(title string) {
	if t == nil || title == "" {
		return
	}

	t.mu.Lock()
	t.title = title
	t.mu.Unlock()
}

func (t *generatedChatTitle) Load() (string, bool) {
	if t == nil {
		return "", false
	}

	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.title == "" {
		return "", false
	}
	return t.title, true
}

// RenameChatTitle persists a user-supplied chat title.
func (p *Server) RenameChatTitle(
	ctx context.Context,
	chat database.Chat,
	newTitle string,
) (updated database.Chat, wrote bool, err error) {
	currentChat, err := p.db.GetChatByID(ctx, chat.ID)
	if err != nil {
		return database.Chat{}, false, xerrors.Errorf("get chat for rename: %w", err)
	}
	if newTitle == currentChat.Title {
		return currentChat, false, nil
	}

	updatedChat, err := p.db.UpdateChatTitleByID(ctx, database.UpdateChatTitleByIDParams{
		ID:    chat.ID,
		Title: newTitle,
	})
	if err != nil {
		return database.Chat{}, false, xerrors.Errorf("update chat title: %w", err)
	}
	return updatedChat, true, nil
}

// PublishTitleChange broadcasts a title_change event for the given chat.
func (p *Server) PublishTitleChange(chat database.Chat) {
	p.publishChatPubsubEvent(chat, codersdk.ChatWatchEventKindTitleChange, nil)
}

// ProposeChatTitle generates a title suggestion from the chat's
// visible messages without persisting it.
func (p *Server) ProposeChatTitle(
	ctx context.Context,
	chat database.Chat,
) (string, error) {
	//nolint:gocritic // Non-admin users need chatd-scoped config reads here.
	chatdCtx := dbauthz.AsChatd(ctx)
	return p.generateManualTitleCandidate(chatdCtx, p.db, chat)
}

// generateManualTitleCandidate generates a title candidate from the chat's
// visible messages. It returns "" when the chat has no messages to summarize.
// Endpoint-specific commit paths decide whether to persist the title.
func (p *Server) generateManualTitleCandidate(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
) (string, error) {
	headMessages, err := store.GetChatMessagesByChatIDAscPaginated(
		ctx,
		database.GetChatMessagesByChatIDAscPaginatedParams{
			ChatID:   chat.ID,
			AfterID:  0,
			LimitVal: manualTitleMessageWindowLimit,
		},
	)
	if err != nil {
		return "", xerrors.Errorf("get head chat messages: %w", err)
	}
	tailMessages, err := store.GetChatMessagesByChatIDDescPaginated(
		ctx,
		database.GetChatMessagesByChatIDDescPaginatedParams{
			ChatID:   chat.ID,
			BeforeID: 0,
			LimitVal: manualTitleMessageWindowLimit,
		},
	)
	if err != nil {
		return "", xerrors.Errorf("get tail chat messages: %w", err)
	}
	messages := mergeManualTitleMessages(headMessages, tailMessages)
	if len(messages) == 0 {
		return "", nil
	}
	pasteText, err := titlePasteText(ctx, store, messages)
	if err != nil {
		return "", xerrors.Errorf("get pasted-text attachments for manual title: %w", err)
	}
	apiKeyID, err := p.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		return "", xerrors.Errorf("ensure synthetic API key: %w", err)
	}
	modelOpts := modelBuildOptions{ActiveAPIKeyID: apiKeyID}

	resolved, err := p.resolveManualTitleModel(ctx, store, chat, modelOpts)
	if err != nil {
		return "", err
	}

	titleCtx := ctx
	finishDebugRun := func(error) {}
	if resolved.debugEnabled {
		titleCtx, finishDebugRun = p.prepareManualTitleDebugRun(
			ctx,
			p.debugService(),
			chat,
			resolved,
			messages,
		)
	}

	title, err := generateManualTitle(
		titleCtx,
		messages,
		pasteText,
		resolved.model.LanguageModel(),
		titleObjectCall(resolved),
	)
	finishDebugRun(err)
	if err != nil {
		return "", xerrors.Errorf("generate manual title: %w", err)
	}

	return title, nil
}

func (p *Server) prepareManualTitleDebugRun(
	ctx context.Context,
	debugSvc *chatdebug.Service,
	chat database.Chat,
	resolved resolvedModelCall,
	messages []database.ChatMessage,
) (context.Context, func(error)) {
	titleCtx := ctx
	finishDebugRun := func(error) {}
	modelConfig := resolved.dbConfig

	var historyTipMessageID int64
	if len(messages) > 0 {
		historyTipMessageID = messages[len(messages)-1].ID
	}

	// Derive a first_message label from the first user message.
	var firstUserLabel string
	for _, msg := range messages {
		if msg.Role == database.ChatMessageRoleUser {
			if parts, parseErr := chatprompt.ParseContent(msg); parseErr == nil {
				firstUserLabel = contentBlocksToText(parts)
			}
			break
		}
	}
	if firstUserLabel == "" {
		firstUserLabel = "Title generation"
	}
	seedSummary := chatdebug.SeedSummary(
		chatdebug.TruncateLabel(firstUserLabel, chatdebug.MaxLabelLength),
	)

	createRunCtx, createRunCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	debugRun, createRunErr := debugSvc.CreateRun(createRunCtx, chatdebug.CreateRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       modelConfig.ID,
		Provider:            string(resolved.route.Provider.Type),
		Model:               modelConfig.Model,
		Kind:                chatdebug.KindTitleGeneration,
		Status:              chatdebug.StatusInProgress,
		HistoryTipMessageID: historyTipMessageID,
		TriggerMessageID:    0,
		Summary:             seedSummary,
	})
	createRunCancel()
	if createRunErr != nil {
		p.logger.Warn(ctx, "failed to create manual title debug run",
			slog.F("chat_id", chat.ID),
			slog.F("model", modelConfig.Model),
			slog.Error(createRunErr),
		)
		return titleCtx, finishDebugRun
	}

	runContext := chatdebugRunContext(debugRun)
	titleCtx = chatdebug.ContextWithRun(titleCtx, &runContext)
	finishDebugRun = func(generateErr error) {
		if finalizeErr := debugSvc.FinalizeRun(ctx, chatdebug.FinalizeRunParams{
			RunID:       debugRun.ID,
			ChatID:      debugRun.ChatID,
			Status:      chatdebug.ClassifyError(generateErr),
			SeedSummary: seedSummary,
		}); finalizeErr != nil {
			p.logger.Warn(ctx, "failed to finalize manual title debug run",
				slog.F("chat_id", chat.ID),
				slog.F("run_id", debugRun.ID),
				slog.Error(finalizeErr),
			)
		}
	}

	return titleCtx, finishDebugRun
}

func chatdebugRunContext(run database.ChatDebugRun) chatdebug.RunContext {
	runContext := chatdebug.RunContext{
		RunID:  run.ID,
		ChatID: run.ChatID,
		Kind:   chatdebug.RunKind(run.Kind),
	}
	if run.RootChatID.Valid {
		runContext.RootChatID = run.RootChatID.UUID
	}
	if run.ParentChatID.Valid {
		runContext.ParentChatID = run.ParentChatID.UUID
	}
	if run.ModelConfigID.Valid {
		runContext.ModelConfigID = run.ModelConfigID.UUID
	}
	if run.TriggerMessageID.Valid {
		runContext.TriggerMessageID = run.TriggerMessageID.Int64
	}
	if run.HistoryTipMessageID.Valid {
		runContext.HistoryTipMessageID = run.HistoryTipMessageID.Int64
	}
	if run.Provider.Valid {
		runContext.Provider = run.Provider.String
	}
	if run.Model.Valid {
		runContext.Model = run.Model.String
	}
	return runContext
}

func deriveChatDebugSeed(messages []database.ChatMessage) (
	triggerMessageID int64,
	historyTipMessageID int64,
	triggerLabel string,
) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != database.ChatMessageRoleUser {
			continue
		}
		triggerMessageID = messages[i].ID
		if parts, parseErr := chatprompt.ParseContent(messages[i]); parseErr == nil {
			triggerLabel = contentBlocksToText(parts)
		}
		break
	}

	if len(messages) > 0 {
		historyTipMessageID = messages[len(messages)-1].ID
	}

	return triggerMessageID, historyTipMessageID, triggerLabel
}

func (p *Server) resolveManualTitleModel(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	modelOpts modelBuildOptions,
) (resolvedModelCall, error) {
	overrideResolved, overrideSet, overrideErr := p.resolveTitleGenerationModelOverride(
		ctx,
		chat,
		modelOpts,
	)
	if overrideErr != nil {
		if overrideSet {
			return resolvedModelCall{}, xerrors.Errorf(
				"resolve manual title generation model override: %w",
				overrideErr,
			)
		}
		p.logger.Debug(ctx, "failed to resolve title generation model override for manual title",
			slog.F("chat_id", chat.ID),
			slog.Error(overrideErr),
		)
	} else if overrideSet {
		return overrideResolved, nil
	}

	modelCtx, err := p.callerModelConfigContext(ctx, chat.OwnerID)
	if err != nil {
		return resolvedModelCall{}, err
	}
	configs, err := enabledChatModelConfigsForOrganization(modelCtx, store, chat.OrganizationID)
	if err != nil {
		p.logger.Debug(ctx, "failed to list manual title model configs",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return p.resolveFallbackManualTitleModel(ctx, chat, modelOpts)
	}

	config, ok := selectPreferredConfiguredShortTextModelConfig(configs)
	if !ok {
		return p.resolveFallbackManualTitleModel(ctx, chat, modelOpts)
	}

	resolved, err := p.resolveModelCall(ctx, modelCallSpec{
		purpose:        "title",
		chat:           chat,
		explicitConfig: &config,
		buildOptions:   modelOpts,
	})
	if err != nil {
		p.logger.Debug(ctx, "manual title preferred model unavailable",
			slog.F("chat_id", chat.ID),
			slog.F("model", config.Model),
			slog.Error(err),
		)
		return p.resolveFallbackManualTitleModel(ctx, chat, modelOpts)
	}
	return resolved, nil
}

func (p *Server) resolveFallbackManualTitleModel(
	ctx context.Context,
	chat database.Chat,
	modelOpts modelBuildOptions,
) (resolvedModelCall, error) {
	config, err := p.resolveModelConfig(ctx, chat)
	if err != nil {
		return resolvedModelCall{}, xerrors.Errorf(
			"resolve fallback manual title model config: %w",
			err,
		)
	}
	resolved, err := p.resolveModelCall(ctx, modelCallSpec{
		purpose:        "title",
		chat:           chat,
		explicitConfig: &config,
		buildOptions:   modelOpts,
	})
	if err != nil {
		return resolvedModelCall{}, xerrors.Errorf(
			"create fallback manual title model: %w",
			err,
		)
	}
	return resolved, nil
}

func mergeManualTitleMessages(
	headMessages []database.ChatMessage,
	tailMessagesDesc []database.ChatMessage,
) []database.ChatMessage {
	merged := make([]database.ChatMessage, 0, len(headMessages)+len(tailMessagesDesc))
	seen := make(map[int64]struct{}, len(headMessages)+len(tailMessagesDesc))
	appendUnique := func(message database.ChatMessage) {
		if _, ok := seen[message.ID]; ok {
			return
		}
		seen[message.ID] = struct{}{}
		merged = append(merged, message)
	}
	for _, message := range headMessages {
		appendUnique(message)
	}
	for i := len(tailMessagesDesc) - 1; i >= 0; i-- {
		appendUnique(tailMessagesDesc[i])
	}
	return merged
}

type chatMessage struct {
	role                database.ChatMessageRole
	content             pqtype.NullRawMessage
	visibility          database.ChatMessageVisibility
	modelConfigID       uuid.UUID
	createdBy           uuid.UUID
	contentVersion      int16
	compressed          bool
	inputTokens         int64
	outputTokens        int64
	totalTokens         int64
	reasoningTokens     int64
	cacheCreationTokens int64
	cacheReadTokens     int64
	contextLimit        int64
	runtimeMs           int64
}

func newChatMessage(
	role database.ChatMessageRole,
	content pqtype.NullRawMessage,
	visibility database.ChatMessageVisibility,
	modelConfigID uuid.UUID,
	contentVersion int16,
) chatMessage {
	return chatMessage{
		role:           role,
		content:        content,
		visibility:     visibility,
		modelConfigID:  modelConfigID,
		contentVersion: contentVersion,
	}
}

func (m chatMessage) withCreatedBy(id uuid.UUID) chatMessage {
	m.createdBy = id
	return m
}

func appendMessageFields(
	params *database.InsertChatMessagesParams,
	msg chatMessage,
) {
	params.CreatedBy = append(params.CreatedBy, msg.createdBy)
	params.ModelConfigID = append(params.ModelConfigID, msg.modelConfigID)
	params.ReasoningEffort = append(params.ReasoningEffort, "")
	params.Role = append(params.Role, msg.role)
	params.Content = append(params.Content, string(msg.content.RawMessage))
	params.ContentVersion = append(params.ContentVersion, msg.contentVersion)
	params.Visibility = append(params.Visibility, msg.visibility)
	params.InputTokens = append(params.InputTokens, msg.inputTokens)
	params.OutputTokens = append(params.OutputTokens, msg.outputTokens)
	params.TotalTokens = append(params.TotalTokens, msg.totalTokens)
	params.ReasoningTokens = append(params.ReasoningTokens, msg.reasoningTokens)
	params.CacheCreationTokens = append(params.CacheCreationTokens, msg.cacheCreationTokens)
	params.CacheReadTokens = append(params.CacheReadTokens, msg.cacheReadTokens)
	params.ContextLimit = append(params.ContextLimit, msg.contextLimit)
	params.Compressed = append(params.Compressed, msg.compressed)
	params.RuntimeMs = append(params.RuntimeMs, msg.runtimeMs)
}

// BuildSingleChatMessageInsertParams builds insert parameters for one chat message.
func BuildSingleChatMessageInsertParams(
	chatID uuid.UUID,
	role database.ChatMessageRole,
	content pqtype.NullRawMessage,
	visibility database.ChatMessageVisibility,
	modelConfigID uuid.UUID,
	contentVersion int16,
	createdBy uuid.UUID,
) database.InsertChatMessagesParams {
	params := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendMessageFields.
		ChatID: chatID,
	}
	msg := newChatMessage(role, content, visibility, modelConfigID, contentVersion)
	if createdBy != uuid.Nil {
		msg = msg.withCreatedBy(createdBy)
	}
	appendMessageFields(&params, msg)
	return params
}

// Config configures a chat processor.
type Config struct {
	Logger    slog.Logger
	Database  database.Store
	ReplicaID uuid.UUID
	// StreamPartsDialer dials remote stream parts. Nil uses the local
	// in-process channel dialer for every stream.
	StreamPartsDialer              StreamPartsDialer
	PendingChatAcquireInterval     time.Duration
	MaxChatsPerAcquire             int32
	InFlightChatStaleAfter         time.Duration
	ChatHeartbeatInterval          time.Duration
	AgentConn                      AgentConnFunc
	AgentInactiveDisconnectTimeout time.Duration
	InstructionLookupTimeout       time.Duration
	CreateWorkspace                chattool.CreateWorkspaceFn
	StartWorkspace                 chattool.StartWorkspaceFn
	StopWorkspace                  chattool.StopWorkspaceFn
	ProviderAPIKeys                chatprovider.ProviderAPIKeys
	AllowBYOK                      bool
	AllowBYOKSet                   bool
	AlwaysEnableDebugLogs          bool
	WebpushDispatcher              webpush.Dispatcher
	HookDispatcher                 *dispatch.Dispatcher
	UsageTracker                   *workspacestats.UsageTracker
	Clock                          quartz.Clock
	AIBridgeTransportFactory       *atomic.Pointer[aibridge.TransportFactory]
	Experiments                    codersdk.Experiments
	PrometheusRegistry             prometheus.Registerer

	AgentCapacityUnlock AgentCapacityUnlock

	// OIDCTokenSource resolves the calling user's OIDC access
	// token for MCP servers configured with auth_type=user_oidc.
	// May be nil if the deployment has no OIDC provider; servers
	// using user_oidc will then send no Authorization header.
	OIDCTokenSource mcpclient.UserOIDCTokenSource

	NotificationsEnqueuer notifications.Enqueuer
	Auditor               *atomic.Pointer[audit.Auditor]
}

// New creates a new chat processor with the required pubsub dependency.
// The processor polls for pending chats and processes them. It is the
// caller's responsibility to call Close on the returned instance.
func New(ps pubsub.Pubsub, cfg Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	pendingChatAcquireInterval := cfg.PendingChatAcquireInterval
	if pendingChatAcquireInterval == 0 {
		pendingChatAcquireInterval = DefaultPendingChatAcquireInterval
	}

	inFlightChatStaleAfter := cfg.InFlightChatStaleAfter
	if inFlightChatStaleAfter == 0 {
		inFlightChatStaleAfter = DefaultInFlightChatStaleAfter
	}

	maxChatsPerAcquire := cfg.MaxChatsPerAcquire
	if maxChatsPerAcquire <= 0 {
		maxChatsPerAcquire = DefaultMaxChatsPerAcquire
	}

	chatHeartbeatInterval := cfg.ChatHeartbeatInterval
	if chatHeartbeatInterval == 0 {
		chatHeartbeatInterval = DefaultChatHeartbeatInterval
	}

	clk := cfg.Clock
	if clk == nil {
		clk = quartz.NewReal()
	}

	notificationsEnqueuer := cfg.NotificationsEnqueuer
	if notificationsEnqueuer == nil {
		notificationsEnqueuer = notifications.NewNoopEnqueuer()
	}

	instructionLookupTimeout := cfg.InstructionLookupTimeout
	if instructionLookupTimeout == 0 {
		instructionLookupTimeout = homeInstructionLookupTimeout
	}

	workerID := cfg.ReplicaID
	if workerID == uuid.Nil {
		workerID = uuid.New()
	}

	allowBYOK := true
	if cfg.AllowBYOKSet {
		allowBYOK = cfg.AllowBYOK
	}

	// Require the experiment even for injected dispatchers to
	// preserve explicit opt-in.
	hookDispatcher := cfg.HookDispatcher
	if hookDispatcher != nil && !cfg.Experiments.Enabled(codersdk.ExperimentAgentLifecycleHooks) {
		cfg.Logger.Warn(ctx, "ignoring chat lifecycle hook dispatcher; the agent-lifecycle-hooks experiment is not enabled")
		hookDispatcher = nil
	}
	p := &Server{
		cancel:   cancel,
		db:       cfg.Database,
		workerID: workerID,
		logger:   cfg.Logger.Named("processor"),
		modelConfigContext: func(ctx context.Context, ownerID uuid.UUID) (context.Context, error) {
			return callerModelConfigContext(ctx, cfg.Database, ownerID)
		},
		agentConnFn:                    cfg.AgentConn,
		agentInactiveDisconnectTimeout: cfg.AgentInactiveDisconnectTimeout,
		dialTimeout:                    defaultDialTimeout,
		instructionLookupTimeout:       instructionLookupTimeout,
		createWorkspaceFn:              cfg.CreateWorkspace,
		startWorkspaceFn:               cfg.StartWorkspace,
		stopWorkspaceFn:                cfg.StopWorkspace,
		pubsub:                         ps,
		webpushDispatcher:              cfg.WebpushDispatcher,
		hooks:                          chathooks.NewTrigger(hookDispatcher),
		providerAPIKeys:                cfg.ProviderAPIKeys,
		allowBYOK:                      allowBYOK,
		oidcTokenSource:                cfg.OIDCTokenSource,
		debugSvcFactory: func() *chatdebug.Service {
			debugSvc := chatdebug.NewService(
				cfg.Database,
				cfg.Logger.Named("chatdebug"),
				ps,
				chatdebug.WithAlwaysEnable(cfg.AlwaysEnableDebugLogs),
			)
			// Debug runs do not heartbeat during model streams; their
			// updated_at is only touched on step/run completion. Use a
			// longer stale window so long-running turns are not falsely
			// finalized as stale while still executing.
			debugSvc.SetStaleAfter(inFlightChatStaleAfter * 3)
			return debugSvc
		},
		aibridgeTransportFactory:   cfg.AIBridgeTransportFactory,
		experiments:                cfg.Experiments,
		pendingChatAcquireInterval: pendingChatAcquireInterval,
		maxChatsPerAcquire:         maxChatsPerAcquire,
		inFlightChatStaleAfter:     inFlightChatStaleAfter,
		chatHeartbeatInterval:      chatHeartbeatInterval,
		usageTracker:               cfg.UsageTracker,
		clock:                      clk,
		recordingSem:               make(chan struct{}, maxConcurrentRecordingUploads),
	}
	var chatAutoArchiveRecords prometheus.Counter
	if cfg.PrometheusRegistry != nil {
		p.metrics = chatloop.NewMetrics(cfg.PrometheusRegistry)
		chatAutoArchiveRecords = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "chat_auto_archive",
			Name:      "records_archived_total",
			Help:      "Total number of chats archived by the auto-archive job (counting both roots and cascaded children).",
		})
		cfg.PrometheusRegistry.MustRegister(chatAutoArchiveRecords)
	} else {
		p.metrics = chatloop.NopMetrics()
	}
	p.messagePartBuffer = messagepartbuffer.New(messagepartbuffer.Options{Clock: clk})
	localStreamPartsDialer := NewLocalStreamPartsDialer(LocalStreamPartsDialerConfig{
		Buffer: p.messagePartBuffer,
		Logger: cfg.Logger,
	})
	p.streamPartsDialer = streamPartsDialerForServer(workerID, localStreamPartsDialer, cfg.StreamPartsDialer)
	p.streamSyncPoller = newStreamSyncPoller(ctx, cfg.Database, clk, cfg.Logger.Named("chatstream"))
	p.streamSyncPoller.Start()
	agentCapacityLimiter := newAgentCapacityLimiter(
		cfg.AgentCapacityUnlock,
		int32(inFlightChatStaleAfter.Seconds()),
	)
	var agentCapacityMetrics *capacityMetrics
	if cfg.PrometheusRegistry != nil {
		agentCapacityMetrics = newCapacityMetrics(cfg.PrometheusRegistry)
	}
	p.agentCapacityLimiter = agentCapacityLimiter
	chatWorker, err := newChatWorker(p, chatWorkerOptions{
		WorkerID:              workerID,
		Store:                 cfg.Database,
		Pubsub:                ps,
		Logger:                cfg.Logger.Named("chatworker"),
		Clock:                 clk,
		MessagePartBuffer:     p.messagePartBuffer,
		AgentCapacityLimiter:  agentCapacityLimiter,
		CapacityMetrics:       agentCapacityMetrics,
		AcquisitionInterval:   pendingChatAcquireInterval,
		AcquisitionBatchSize:  maxChatsPerAcquire,
		HeartbeatInterval:     chatHeartbeatInterval,
		HeartbeatStaleSeconds: int32(inFlightChatStaleAfter.Seconds()),
		NotificationsEnqueuer: notificationsEnqueuer,
		Auditor:               cfg.Auditor,
		AutoArchiveRecords:    chatAutoArchiveRecords,
	})
	if err != nil {
		panic("chatd: create chat worker: " + err.Error())
	}
	p.chatWorker = chatWorker

	//nolint:gocritic // The chat processor uses a scoped chatd context.
	ctx = dbauthz.AsChatd(ctx)

	p.configCache = newChatConfigCache(ctx, cfg.Database, clk)
	cancelConfigSub, err := p.pubsub.SubscribeWithErr(
		coderdpubsub.ChatConfigEventChannel,
		coderdpubsub.HandleChatConfigEvent(func(ctx context.Context, ev coderdpubsub.ChatConfigEvent, err error) {
			if err != nil {
				p.logger.Warn(ctx, "chat config event error", slog.Error(err))
				return
			}
			switch ev.Kind {
			case coderdpubsub.ChatConfigEventUserPrompt:
				p.configCache.InvalidateUserPrompt(ev.EntityID)
			case coderdpubsub.ChatConfigEventAdvisorConfig:
				p.configCache.InvalidateAdvisorConfig()
			}
		}),
	)
	if err != nil {
		p.logger.Error(ctx, "subscribe to chat config events", slog.Error(err))
	} else {
		p.configCacheUnsubscribe = cancelConfigSub
	}

	cancelProviderSub, err := p.pubsub.SubscribeWithErr(
		coderdpubsub.AIProvidersChangedChannel,
		func(cbCtx context.Context, _ []byte, err error) {
			if err != nil {
				p.logger.Warn(cbCtx, "ai providers changed event error", slog.Error(err))
				return
			}
			p.configCache.InvalidateProviders()
		},
	)
	if err != nil {
		p.logger.Error(ctx, "subscribe to ai providers changed events", slog.Error(err))
	} else {
		p.providerCacheUnsubscribe = cancelProviderSub
	}

	p.ctx = ctx

	// Spawn background goroutines that all servers need.

	return p
}

// Start runs the background acquire/wake loop that picks up
// pending chats and processes them. Callers that want a passive
// server (e.g. tests) can skip this call; heartbeat, stream
// janitor, and stale recovery still run.
func (p *Server) Start() *Server {
	if p.chatWorker != nil {
		if err := p.chatWorker.Start(p.ctx); err != nil {
			p.logger.Error(p.ctx, "failed to start chat worker", slog.Error(err))
		}
	}
	return p
}

func subscribeWithInitialError(chatID uuid.UUID, message string) (
	[]codersdk.ChatStreamEvent,
	<-chan codersdk.ChatStreamEvent,
	func(),
	bool,
) {
	events := make(chan codersdk.ChatStreamEvent)
	close(events)
	return []codersdk.ChatStreamEvent{{
		Type:   codersdk.ChatStreamEventTypeError,
		ChatID: chatID,
		Error:  &codersdk.ChatError{Message: message},
	}}, events, func() {}, true
}

// publishChatPubsubEvents broadcasts a lifecycle event for each affected chat.
func (p *Server) publishChatPubsubEvents(chats []database.Chat, kind codersdk.ChatWatchEventKind) {
	for _, chat := range chats {
		p.publishChatPubsubEvent(chat, kind, nil)
	}
}

// chatWatchEventSDKChat builds the chat embedded in ChatWatchEvent
// notifications. These payloads travel through PostgreSQL NOTIFY, so
// omit fields that can grow large and that watch consumers already read
// from the REST chat endpoint.
func chatWatchEventSDKChat(chat database.Chat, diffStatus *codersdk.ChatDiffStatus) codersdk.Chat {
	sdkChat := db2sdk.Chat(chat, nil, nil)
	sdkChat.Files = nil
	if diffStatus != nil {
		sdkChat.DiffStatus = diffStatus
	}
	return sdkChat
}

// publishChatPubsubEvent broadcasts a chat lifecycle event via PostgreSQL
// pubsub so that all replicas can push updates to watching clients.
func (p *Server) publishChatPubsubEvent(chat database.Chat, kind codersdk.ChatWatchEventKind, diffStatus *codersdk.ChatDiffStatus) {
	event := codersdk.ChatWatchEvent{
		Kind: kind,
		Chat: chatWatchEventSDKChat(chat, diffStatus),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		p.logger.Error(context.Background(), "failed to marshal chat pubsub event",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return
	}
	if err := p.pubsub.Publish(coderdpubsub.ChatWatchEventChannel(chat.OwnerID), payload); err != nil {
		p.logger.Error(context.Background(), "failed to publish chat pubsub event",
			slog.F("chat_id", chat.ID),
			slog.F("kind", kind),
			slog.Error(err),
		)
	}
}

// ChatQueuedForCapacity reports whether the chat is waiting for a
// concurrent-agent capacity slot. Uncapped deployments always return false.
func (p *Server) ChatQueuedForCapacity(ctx context.Context, chat database.Chat) (bool, error) {
	limits, capped := p.agentCapacityLimiter.Limits()
	if !capped {
		return false, nil
	}
	if chat.Archived || chat.Status != database.ChatStatusRunning {
		return false, nil
	}
	// The pool count spans other users' chats, which the requester cannot
	// read directly.
	//nolint:gocritic // Capacity accounting is chatd-internal state.
	return p.db.GetChatQueuedForCapacity(dbauthz.AsChatd(ctx), database.GetChatQueuedForCapacityParams{
		ChatID:           chat.ID,
		StaleSeconds:     int32(p.inFlightChatStaleAfter.Seconds()),
		RootCapacity:     limits.Root,
		SubagentCapacity: limits.Subagent,
	})
}

// PublishDiffStatusChange broadcasts a diff_status_change event for
// the given chat so that watching clients know to re-fetch the diff
// status. This is called from the HTTP layer after the diff status
// is updated in the database.
func (p *Server) PublishDiffStatusChange(ctx context.Context, chatID uuid.UUID) error {
	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("get chat: %w", err)
	}

	dbStatus, err := p.db.GetChatDiffStatusByChatID(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("get chat diff status: %w", err)
	}

	sdkStatus := db2sdk.ChatDiffStatus(chatID, &dbStatus)
	p.publishChatPubsubEvent(chat, codersdk.ChatWatchEventKindDiffStatusChange, &sdkStatus)
	return nil
}

// Rejects oversize images on capped providers before any upstream
// request is issued.
//
// Gotcha: a historical oversize image bricks the chat on a capped
// provider until the user switches providers back, starts a new
// chat, or edits a message above the offending one (which truncates
// the prompt forward). A future change should skip the file with a
// user-facing warning, but that requires altering the FileResolver
// contract.
func (p *Server) chatFileResolver(provider string) chatprompt.FileResolver {
	return func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]chatprompt.FileData, error) {
		files, err := p.db.GetChatFilesByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		imageCap, hasImageCap := chatprovider.InlineImageCapBytes(provider)
		normalizedProvider := chatprovider.NormalizeProvider(provider)
		result := make(map[uuid.UUID]chatprompt.FileData, len(files))
		for _, f := range files {
			if hasImageCap &&
				strings.HasPrefix(f.Mimetype, "image/") &&
				len(f.Data) >= imageCap {
				err := xerrors.Errorf(
					"image attachment %q is %d bytes; %s inline image limit is %d bytes",
					f.Name, len(f.Data),
					chatprovider.ProviderDisplayName(normalizedProvider),
					imageCap,
				)
				// User-facing message stays client-agnostic since
				// older web clients and direct API callers don't
				// auto-resize; the wrapped error above keeps the
				// exact byte count for operator logs.
				return nil, chaterror.WithClassification(err, chaterror.ClassifiedError{
					Kind:     codersdk.ChatErrorKindConfig,
					Provider: normalizedProvider,
					Message: fmt.Sprintf(
						"Image attachment exceeds %s's %s inline image limit. Replace it with a smaller image.",
						chatprovider.ProviderDisplayName(normalizedProvider),
						//nolint:gosec // imageCap is a small positive constant defined in chatprovider.
						humanize.IBytes(uint64(imageCap)),
					),
					Retryable: false,
				})
			}
			result[f.ID] = chatprompt.FileData{
				Name:      f.Name,
				Data:      f.Data,
				MediaType: f.Mimetype,
			}
		}
		return result, nil
	}
}

// trackWorkspaceUsage bumps the workspace's last_used_at via the
// usage tracker and extends the workspace's autostop deadline. If
// wsID is not yet valid, it re-reads the chat from the DB to pick
// up late associations (e.g. create_workspace linking a workspace
// mid-conversation). The caller should store the returned value so
// that subsequent calls skip the DB lookup once a workspace has
// been found.
func (p *Server) trackWorkspaceUsage(
	ctx context.Context,
	chatID uuid.UUID,
	wsID uuid.NullUUID,
	logger slog.Logger,
) uuid.NullUUID {
	if p.usageTracker == nil {
		return wsID
	}
	if !wsID.Valid {
		latest, err := p.db.GetChatByID(ctx, chatID)
		if err != nil {
			logger.Warn(ctx, "failed to re-read chat for workspace association", slog.Error(err))
			return wsID
		}
		wsID = latest.WorkspaceID
	}
	if wsID.Valid {
		p.usageTracker.Add(wsID.UUID)
		// Bump the workspace autostop deadline. We pass time.Time{}
		// for nextAutostart since we don't have access to
		// TemplateScheduleStore here. The activity bump logic
		// defaults to the template's activity_bump duration
		// (typically 1 hour). Chat workspaces are never prebuilds,
		// so no prebuild guard is needed (unlike reporter.go).
		//
		// This fires every heartbeat (~30s) but the SQL only
		// writes when 5% of the deadline has elapsed, most calls
		// perform a read-only CTE lookup with no UPDATE.
		//
		// Scaling note: for 10,000 active chats, this could lead to
		// approx. 333 CTE queries/second. A cheap fix for this could
		// be to heartbeat every Nth query. Leaving as potential future
		// low-hanging fruit if needed.
		workspacestats.ActivityBumpWorkspace(ctx, logger.Named("activity_bump"), p.db, wsID.UUID, time.Time{}, workspacestats.ActivityBumpReasonChatHeartbeat)
	}
	return wsID
}

type runChatResult struct {
	FinalAssistantText string
	// StatusLabelCall is nil when status-label model resolution failed.
	StatusLabelCall     *resolvedModelCall
	TriggerMessageID    int64
	HistoryTipMessageID int64
}

func (p *Server) aiProviderConfig(ctx context.Context, provider database.AIProvider) (chatprovider.ConfiguredProvider, error) {
	keys, err := p.db.GetAIProviderKeysByProviderID(ctx, provider.ID)
	if err != nil {
		return chatprovider.ConfiguredProvider{}, xerrors.Errorf("get AI provider keys: %w", err)
	}
	return p.aiProviderConfigFromKeys(provider, keys)
}

func (p *Server) aiProviderConfigFromKeys(provider database.AIProvider, keys []database.AIProviderKey) (chatprovider.ConfiguredProvider, error) {
	if !provider.Enabled {
		return chatprovider.ConfiguredProvider{}, xerrors.Errorf("AI provider %s is disabled", provider.ID)
	}
	settings, err := db2sdk.AIProviderSettings(provider.Settings)
	if err != nil {
		return chatprovider.ConfiguredProvider{}, xerrors.Errorf("decode AI provider settings: %w", err)
	}

	apiKey := ""
	// GetAIProviderKeysByProviderID orders keys oldest first. chatd consumes
	// one provider-scoped key because runtime provider config has one API key slot.
	for _, key := range keys {
		if key.APIKey != "" {
			apiKey = key.APIKey
			break
		}
	}
	region := ""
	if settings.Bedrock != nil {
		region = strings.TrimSpace(settings.Bedrock.Region)
	}
	return chatprovider.ConfiguredProvider{
		ProviderID:                 provider.ID,
		Provider:                   string(provider.Type),
		APIKey:                     apiKey,
		BaseURL:                    provider.BaseUrl,
		Region:                     region,
		CentralAPIKeyEnabled:       true,
		AllowUserAPIKey:            p.allowBYOK,
		AllowCentralAPIKeyFallback: true,
	}, nil
}

func (p *Server) aiProviderConfigs(ctx context.Context, providers []database.AIProvider) ([]chatprovider.ConfiguredProvider, error) {
	if len(providers) == 0 {
		return nil, nil
	}
	providerIDs := make([]uuid.UUID, 0, len(providers))
	for _, provider := range providers {
		providerIDs = append(providerIDs, provider.ID)
	}
	keys, err := p.db.GetAIProviderKeysByProviderIDs(ctx, providerIDs)
	if err != nil {
		return nil, xerrors.Errorf("get AI provider keys: %w", err)
	}
	keysByProviderID := make(map[uuid.UUID][]database.AIProviderKey, len(providers))
	for _, key := range keys {
		keysByProviderID[key.ProviderID] = append(keysByProviderID[key.ProviderID], key)
	}
	configuredProviders := make([]chatprovider.ConfiguredProvider, 0, len(providers))
	for _, provider := range providers {
		configuredProvider, err := p.aiProviderConfigFromKeys(provider, keysByProviderID[provider.ID])
		if err != nil {
			return nil, err
		}
		configuredProviders = append(configuredProviders, configuredProvider)
	}
	return configuredProviders, nil
}

func ensureUniqueConfiguredProviderTypes(providers []chatprovider.ConfiguredProvider) error {
	seen := make(map[string]uuid.UUID, len(providers))
	for _, provider := range providers {
		normalizedProvider := chatprovider.NormalizeProvider(provider.Provider)
		if normalizedProvider == "" {
			continue
		}
		if existingProviderID, ok := seen[normalizedProvider]; ok && existingProviderID != provider.ProviderID {
			return xerrors.Errorf("multiple enabled AI providers use provider type %q; select an AI provider by ID", normalizedProvider)
		}
		seen[normalizedProvider] = provider.ProviderID
	}
	return nil
}

func (p *Server) resolveUserProviderAPIKeysForProvider(
	ctx context.Context,
	ownerID uuid.UUID,
	provider database.AIProvider,
) (chatprovider.ProviderAPIKeys, error) {
	configuredProvider, err := p.aiProviderConfig(ctx, provider)
	if err != nil {
		return chatprovider.ProviderAPIKeys{}, err
	}
	userKeys := []chatprovider.UserProviderKey{}
	if p.allowBYOK {
		userKey, err := p.db.GetUserAIProviderKeyByProviderID(ctx, database.GetUserAIProviderKeyByProviderIDParams{
			UserID:       ownerID,
			AIProviderID: provider.ID,
		})
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return chatprovider.ProviderAPIKeys{}, xerrors.Errorf("get user AI provider key: %w", err)
		}
		if err == nil {
			userKeys = append(userKeys, chatprovider.UserProviderKey{
				ChatProviderID: userKey.AIProviderID,
				APIKey:         userKey.APIKey,
			})
		}
	}
	keys, _ := chatprovider.ResolveUserProviderKeys(
		chatprovider.ProviderAPIKeys{},
		[]chatprovider.ConfiguredProvider{configuredProvider},
		userKeys,
	)
	return keys, nil
}

func (p *Server) resolveUserProviderAPIKeysForProviderType(
	ctx context.Context,
	ownerID uuid.UUID,
	providerType string,
) (chatprovider.ProviderAPIKeys, error) {
	keys, _, err := p.resolveUserProviderAPIKeysAndProviderForProviderType(ctx, ownerID, providerType)
	return keys, err
}

func (p *Server) resolveUserProviderAPIKeysAndProviderForProviderType(
	ctx context.Context,
	ownerID uuid.UUID,
	providerType string,
) (chatprovider.ProviderAPIKeys, *database.AIProvider, error) {
	providers, err := p.db.GetAIProviders(ctx, database.GetAIProvidersParams{})
	if err != nil {
		return chatprovider.ProviderAPIKeys{}, nil, xerrors.Errorf("get enabled AI providers: %w", err)
	}
	normalizedProviderType := chatprovider.NormalizeProvider(providerType)
	for _, provider := range providers {
		if chatprovider.NormalizeProvider(string(provider.Type)) != normalizedProviderType {
			continue
		}
		keys, err := p.resolveUserProviderAPIKeysForProvider(ctx, ownerID, provider)
		if err != nil {
			return chatprovider.ProviderAPIKeys{}, nil, err
		}
		if userCanUseProviderKeys(keys, normalizedProviderType) {
			return keys, &provider, nil
		}
	}
	keys, err := p.resolveUserProviderAPIKeys(ctx, ownerID, uuid.Nil)
	if err != nil {
		return chatprovider.ProviderAPIKeys{}, nil, err
	}
	return keys, nil, nil
}

func (p *Server) resolveUserProviderAPIKeys(
	ctx context.Context,
	ownerID uuid.UUID,
	selectedAIProviderID uuid.UUID,
) (chatprovider.ProviderAPIKeys, error) {
	if selectedAIProviderID != uuid.Nil {
		provider, err := p.db.GetAIProviderByID(ctx, selectedAIProviderID)
		if err != nil {
			return chatprovider.ProviderAPIKeys{}, xerrors.Errorf("get AI provider: %w", err)
		}
		return p.resolveUserProviderAPIKeysForProvider(ctx, ownerID, provider)
	}

	providers, err := p.configCache.EnabledProviders(ctx)
	if err != nil {
		return chatprovider.ProviderAPIKeys{}, xerrors.Errorf(
			"get enabled AI providers: %w",
			err,
		)
	}
	configuredProviders, err := p.aiProviderConfigs(ctx, providers)
	if err != nil {
		return chatprovider.ProviderAPIKeys{}, err
	}
	if err := ensureUniqueConfiguredProviderTypes(configuredProviders); err != nil {
		return chatprovider.ProviderAPIKeys{}, err
	}

	userKeys := []chatprovider.UserProviderKey{}
	if p.allowBYOK {
		userKeyRows, err := p.db.GetUserAIProviderKeysByUserID(ctx, ownerID)
		if err != nil {
			return chatprovider.ProviderAPIKeys{}, xerrors.Errorf(
				"get user AI provider keys: %w",
				err,
			)
		}
		userKeys = make([]chatprovider.UserProviderKey, 0, len(userKeyRows))
		for _, userKey := range userKeyRows {
			userKeys = append(userKeys, chatprovider.UserProviderKey{
				ChatProviderID: userKey.AIProviderID,
				APIKey:         userKey.APIKey,
			})
		}
	}

	keys, _ := chatprovider.ResolveUserProviderKeys(
		p.providerAPIKeys,
		configuredProviders,
		userKeys,
	)
	enabledProviders := make(map[string]struct{}, len(configuredProviders))
	for _, provider := range configuredProviders {
		normalizedProvider := chatprovider.NormalizeProvider(provider.Provider)
		if normalizedProvider == "" {
			continue
		}
		enabledProviders[normalizedProvider] = struct{}{}
	}
	chatprovider.PruneDisabledProviderKeys(&keys, enabledProviders)
	return keys, nil
}

func (p *Server) resolveModelConfigForOrganization(
	ctx context.Context,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	modelConfigID uuid.UUID,
) (database.ChatModelConfig, string, error) {
	modelConfig, providerName, err := p.resolveModelConfigAndNormalizedProvider(ctx, ownerID, modelConfigID)
	if err != nil {
		return database.ChatModelConfig{}, "", err
	}
	if modelConfig.OrganizationID != organizationID {
		return database.ChatModelConfig{}, "", errModelConfigOutsideOrganization
	}
	return modelConfig, providerName, nil
}

// resolveModelConfig looks up the chat's enabled model config by its
// LastModelConfigID. If the referenced config is unavailable or belongs to
// another organization, it falls back to the local default model config.
// Returns an error when no usable local config is available.
func (p *Server) resolveModelConfig(
	ctx context.Context,
	chat database.Chat,
) (database.ChatModelConfig, error) {
	modelCtx, err := p.callerModelConfigContext(ctx, chat.OwnerID)
	if err != nil {
		return database.ChatModelConfig{}, err
	}
	if chat.LastModelConfigID != uuid.Nil {
		modelConfig, err := p.db.GetEnabledChatModelConfigByID(
			modelCtx,
			chat.LastModelConfigID,
		)
		if err == nil {
			if modelConfig.OrganizationID == chat.OrganizationID {
				return modelConfig, nil
			}
			err = sql.ErrNoRows
		}
		if !xerrors.Is(err, sql.ErrNoRows) && !dbauthz.IsNotAuthorizedError(err) {
			return database.ChatModelConfig{}, xerrors.Errorf(
				"get chat model config %s: %w",
				chat.LastModelConfigID, err,
			)
		}
		// The model config is unavailable or belongs to another organization.
		// Fall through to the local default.
	}

	defaultConfig, err := effectiveDefaultChatModelConfig(modelCtx, p.db, chat.OrganizationID)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return database.ChatModelConfig{}, ErrNoDefaultChatModelConfig
		}
		return database.ChatModelConfig{}, xerrors.Errorf(
			"get default chat model config: %w", err,
		)
	}
	return defaultConfig, nil
}

func refreshChatWorkspaceSnapshot(
	ctx context.Context,
	chat database.Chat,
	loadChat func(context.Context, uuid.UUID) (database.Chat, error),
) (database.Chat, error) {
	if chat.WorkspaceID.Valid || loadChat == nil {
		return chat, nil
	}

	refreshedChat, err := loadChat(ctx, chat.ID)
	if err != nil {
		return chat, xerrors.Errorf("reload chat workspace state: %w", err)
	}

	return refreshedChat, nil
}

// resolveUserCompactionThreshold looks up the user's per-model
// compaction threshold override. Returns the override value and
// true if one exists and is valid, or 0 and false otherwise.
func (p *Server) resolveUserCompactionThreshold(ctx context.Context, userID uuid.UUID, modelConfigID uuid.UUID) (int32, bool) {
	raw, err := p.db.GetUserChatCompactionThreshold(ctx, database.GetUserChatCompactionThresholdParams{
		UserID: userID,
		Key:    codersdk.CompactionThresholdKey(modelConfigID),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false
	}
	if err != nil {
		p.logger.Warn(ctx, "failed to fetch compaction threshold override",
			slog.F("user_id", userID),
			slog.F("model_config_id", modelConfigID),
			slog.Error(err),
		)
		return 0, false
	}
	// Range 0..100 must stay in sync with handler validation in
	// coderd/chats.go.
	val, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || val < 0 || val > 100 {
		return 0, false
	}
	return int32(val), true
}

// resolveDeploymentSystemPrompt builds the deployment-level system
// prompt from the built-in default and the admin-configured custom
// prompt stored in site_configs.
func (p *Server) resolveDeploymentSystemPrompt(ctx context.Context) string {
	config, err := p.db.GetChatSystemPromptConfig(ctx)
	if err != nil {
		// Fail open: use the built-in default so chats always have
		// some system guidance.
		p.logger.Error(ctx, "failed to fetch chat system prompt configuration, using default", slog.Error(err))
		return DefaultSystemPrompt
	}

	sanitizedCustom := codersdk.SanitizePromptText(config.ChatSystemPrompt)
	if sanitizedCustom == "" && strings.TrimSpace(config.ChatSystemPrompt) != "" {
		p.logger.Warn(ctx, "custom system prompt became empty after sanitization, omitting custom portion")
	}

	var parts []string
	if config.IncludeDefaultSystemPrompt {
		parts = append(parts, DefaultSystemPrompt)
	}
	if sanitizedCustom != "" {
		parts = append(parts, sanitizedCustom)
	}
	result := strings.Join(parts, "\n\n")
	if result == "" {
		p.logger.Warn(ctx, "resolved system prompt is empty, no system prompt will be injected into chats")
	}
	return result
}

// resolveUserPrompt fetches the user's custom chat prompt from the
// database and wraps it in <user-instructions> tags. Returns empty
// string if no prompt is set.
func (p *Server) resolveUserPrompt(ctx context.Context, userID uuid.UUID) string {
	raw, err := p.configCache.UserPrompt(ctx, userID)
	if err != nil {
		// sql.ErrNoRows is the normal "not set" case.
		return ""
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	return "<user-instructions>\n" + trimmed + "\n</user-instructions>"
}

// renderPlanPathPrompt fills the plan-path placeholder when it is
// present in the prompt.
func renderPlanPathPrompt(prompt []fantasy.Message, planPathBlock string) []fantasy.Message {
	prompt, _ = replacePlanPathPlaceholder(prompt, planPathBlock)
	return prompt
}

func replacePlanPathPlaceholder(
	prompt []fantasy.Message,
	planPathBlock string,
) ([]fantasy.Message, bool) {
	var updatedPrompt []fantasy.Message
	replaced := false
	for i, message := range prompt {
		updatedMessage, ok := replacePlanPathPlaceholderInMessage(message, planPathBlock)
		if !ok {
			continue
		}
		if updatedPrompt == nil {
			updatedPrompt = slices.Clone(prompt)
		}
		updatedPrompt[i] = updatedMessage
		replaced = true
	}
	if !replaced {
		return prompt, false
	}
	return updatedPrompt, true
}

func replacePlanPathPlaceholderInMessage(
	message fantasy.Message,
	planPathBlock string,
) (fantasy.Message, bool) {
	if message.Role != fantasy.MessageRoleSystem {
		return message, false
	}

	content := slices.Clone(message.Content)
	replaced := false
	for i, part := range content {
		textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](part)
		if !ok || !strings.Contains(textPart.Text, defaultSystemPromptPlanPathBlockPlaceholder) {
			continue
		}
		replaced = true
		content[i] = fantasy.TextPart{Text: strings.ReplaceAll(
			textPart.Text,
			defaultSystemPromptPlanPathBlockPlaceholder,
			planPathBlock,
		)}
	}
	if !replaced {
		return message, false
	}
	message.Content = content
	return message, true
}

func formatPlanPathBlock(chatPath, home string) string {
	chatPath = strings.TrimSpace(chatPath)
	if chatPath == "" {
		return ""
	}

	avoidPlanPath := chattool.LegacySharedPlanPath
	home = strings.TrimSpace(home)
	if home != "" {
		avoidPlanPath = strings.TrimRight(home, "/") + "/PLAN.md"
	}

	var b strings.Builder
	_, _ = b.WriteString("<plan-file-path>\n")
	_, _ = b.WriteString("Your plan file path for this chat is: ")
	_, _ = b.WriteString(chatPath)
	_, _ = b.WriteString("\n")
	_, _ = b.WriteString("Always use this exact path when creating or proposing plan files. Do not use ")
	_, _ = b.WriteString(avoidPlanPath)
	_, _ = b.WriteString(".\n")
	_, _ = b.WriteString("</plan-file-path>")
	return b.String()
}

// parseDynamicToolNames unmarshals the dynamic tools JSON column
// and returns a map of tool names. This centralizes the repeated
// pattern of deserializing DynamicTools into a name set.
func parseDynamicToolNames(raw pqtype.NullRawMessage) (map[string]bool, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return make(map[string]bool), nil
	}
	var tools []codersdk.DynamicTool
	if err := json.Unmarshal(raw.RawMessage, &tools); err != nil {
		return nil, xerrors.Errorf("unmarshal dynamic tools: %w", err)
	}
	names := make(map[string]bool, len(tools))
	for _, t := range tools {
		names[t.Name] = true
	}
	return names, nil
}

// maybeFinalizeTurnStatusLabelAndPush updates the cached turn status label
// for parent chats and optionally sends a web push notification.
func (p *Server) maybeFinalizeTurnStatusLabelAndPush(
	ctx context.Context,
	chat database.Chat,
	status database.ChatStatus,
	lastError string,
	runResult runChatResult,
	logger slog.Logger,
) {
	if chat.ParentChatID.Valid {
		// Subagent chats skip turn status labels and generated
		// summaries, but a successful turn's final report doubles as
		// the chat summary so subagents are not summary-less.
		if status == database.ChatStatusWaiting {
			p.storeSubagentReportSummaryAsync(ctx, chat, logger)
		}
		return
	}

	switch status {
	case database.ChatStatusWaiting:
		p.finalizeSuccessfulTurnStatusLabelAndPush(ctx, chat, status, runResult, logger)
		p.maybeGenerateChatSummaryAsync(ctx, logger, chat)

	case database.ChatStatusError:
		p.clearLastTurnSummaryAsync(ctx, chat, logger)
		if p.webpushConfigured() {
			pushBody := fallbackTurnStatusLabel(status)
			if lastError != "" {
				pushBody = lastError
			}
			p.dispatchPush(ctx, chat, pushBody, status, logger)
		}

	case database.ChatStatusRequiresAction:
		p.setLastTurnSummaryAsync(ctx, chat, fallbackTurnStatusLabel(status), logger)

	default:
		// New statuses must be classified before they can safely
		// preserve or finalize a cached turn status label.
		p.clearLastTurnSummaryAsync(ctx, chat, logger)
	}
}

func (p *Server) finalizeSuccessfulTurnStatusLabelAndPush(
	ctx context.Context,
	chat database.Chat,
	status database.ChatStatus,
	runResult runChatResult,
	logger slog.Logger,
) {
	p.finalizeSuccessfulTurnStatusLabelWithAfterFunc(ctx, chat, status, runResult, logger, func(finalizeCtx context.Context, statusLabel string) {
		p.dispatchSuccessfulTurnPush(finalizeCtx, chat, statusLabel, logger)
	})
}

func (p *Server) finalizeSuccessfulTurnStatusLabelWithAfterFunc(
	ctx context.Context,
	chat database.Chat,
	status database.ChatStatus,
	runResult runChatResult,
	logger slog.Logger,
	afterFinalize func(context.Context, string),
) {
	finalizeCtx, stopFinalizeCtx := p.inflightContext(ctx)
	if err := p.goInflight(func() {
		defer stopFinalizeCtx()
		statusLabel := p.generateFinalTurnStatusLabel(finalizeCtx, chat, status, runResult, logger)
		logger.Debug(finalizeCtx, "generated chat turn status label",
			slog.F("chat_id", chat.ID),
			slog.F("status", status),
			slog.F("label_length", len(statusLabel)),
		)

		p.updateLastTurnSummary(finalizeCtx, chat, chat.HistoryVersion, statusLabel, logger)

		afterFinalize(finalizeCtx, statusLabel)
	}); err != nil {
		stopFinalizeCtx()
		logger.Error(context.WithoutCancel(ctx), "failed to schedule chat turn status finalization",
			slog.F("chat_id", chat.ID),
			slog.F("status", status),
			slog.Error(err),
		)
	}
}

func (p *Server) generateFinalTurnStatusLabel(
	ctx context.Context,
	chat database.Chat,
	status database.ChatStatus,
	runResult runChatResult,
	logger slog.Logger,
) string {
	if status != database.ChatStatusWaiting {
		return fallbackTurnStatusLabel(status)
	}

	assistantText := strings.TrimSpace(runResult.FinalAssistantText)
	if assistantText == "" || runResult.StatusLabelCall == nil {
		return fallbackTurnStatusLabel(status)
	}

	statusLabel := generateTurnStatusLabel(
		ctx,
		chat,
		status,
		assistantText,
		*runResult.StatusLabelCall,
		logger,
		p.existingDebugService(),
		runResult.TriggerMessageID,
		runResult.HistoryTipMessageID,
	)
	if statusLabel == "" {
		return fallbackTurnStatusLabel(status)
	}
	return statusLabel
}

func (p *Server) dispatchSuccessfulTurnPush(
	ctx context.Context,
	chat database.Chat,
	statusLabel string,
	logger slog.Logger,
) {
	if !p.webpushConfigured() {
		return
	}
	pushBody := fallbackTurnStatusLabel(database.ChatStatusWaiting)
	if statusLabel != "" {
		pushBody = statusLabel
	}
	p.dispatchPush(ctx, chat, pushBody, database.ChatStatusWaiting, logger)
}

func (p *Server) setLastTurnSummaryAsync(
	ctx context.Context,
	chat database.Chat,
	summary string,
	logger slog.Logger,
) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		p.clearLastTurnSummaryAsync(ctx, chat, logger)
		return
	}
	if chat.LastTurnSummary.Valid && strings.TrimSpace(chat.LastTurnSummary.String) == summary {
		return
	}
	updateCtx, stopUpdateCtx := p.inflightContext(ctx)
	if err := p.goInflight(func() {
		defer stopUpdateCtx()
		p.updateLastTurnSummary(updateCtx, chat, chat.HistoryVersion, summary, logger)
	}); err != nil {
		stopUpdateCtx()
		logger.Error(context.WithoutCancel(ctx), "failed to schedule chat turn summary update",
			slog.F("chat_id", chat.ID),
			slog.F("expected_history_version", chat.HistoryVersion),
			slog.F("summary_length", len(summary)),
			slog.Error(err),
		)
	}
}

func (p *Server) clearLastTurnSummaryAsync(
	ctx context.Context,
	chat database.Chat,
	logger slog.Logger,
) {
	clearCtx, stopClearCtx := p.inflightContext(ctx)
	if err := p.goInflight(func() {
		defer stopClearCtx()
		p.updateLastTurnSummary(clearCtx, chat, chat.HistoryVersion, "", logger)
	}); err != nil {
		stopClearCtx()
		logger.Error(context.WithoutCancel(ctx), "failed to schedule chat turn summary clear",
			slog.F("chat_id", chat.ID),
			slog.F("expected_history_version", chat.HistoryVersion),
			slog.Error(err),
		)
	}
}

// updateLastTurnSummary writes the cached sidebar summary for a chat.
// Callers should pass a detached context because this method is used for
// best-effort background cache writes.
func (p *Server) updateLastTurnSummary(
	ctx context.Context,
	chat database.Chat,
	expectedHistoryVersion int64,
	summary string,
	logger slog.Logger,
) {
	summary = strings.TrimSpace(summary)
	lastTurnSummary := sql.NullString{String: summary, Valid: summary != ""}

	//nolint:gocritic // Narrow daemon access for best-effort summary cache writes.
	updateCtx := dbauthz.AsChatd(ctx)
	updateCtx, cancel := context.WithTimeout(updateCtx, turnStatusLabelWriteTimeout)
	defer cancel()

	affected, err := p.db.UpdateChatLastTurnSummary(updateCtx, database.UpdateChatLastTurnSummaryParams{
		ID:                     chat.ID,
		ExpectedHistoryVersion: expectedHistoryVersion,
		LastTurnSummary:        lastTurnSummary,
	})
	if err != nil {
		logger.Warn(updateCtx, "failed to update chat turn summary",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return
	}
	if affected == 0 {
		if summary != "" {
			logger.Info(updateCtx, "skipped stale chat turn summary update with non-empty summary",
				slog.F("chat_id", chat.ID),
				slog.F("summary_length", len(summary)),
				slog.F("expected_history_version", expectedHistoryVersion),
			)
			return
		}
		logger.Debug(updateCtx, "skipped stale chat turn summary update",
			slog.F("chat_id", chat.ID),
			slog.F("expected_history_version", expectedHistoryVersion),
		)
		return
	}

	updatedChat := chat
	updatedChat.LastTurnSummary = lastTurnSummary
	p.publishChatPubsubEvent(updatedChat, codersdk.ChatWatchEventKindSummaryChange, nil)
}

const (
	// Completed user turns before the first summary is generated.
	summaryInitialTurnThreshold = 1
	// New completed user turns before the summary is regenerated (since the last summary).
	summaryStaleTurnThreshold  = 3
	summaryMinTranscriptRunes  = 200
	chatSummaryWorkTimeout     = 120 * time.Second
	chatSummaryGenerateTimeout = 60 * time.Second
	chatSummaryWriteTimeout    = 5 * time.Second

	// Subagent summaries reuse the final report instead of generating
	// text, so their work timeout only covers two database round trips.
	subagentReportSummaryTimeout = 15 * time.Second
	// Bound the extracted report snippet near the 1-3 sentence
	// generated summaries that root chats get, so subagent and parent
	// summary panels read the same.
	subagentReportSummaryMaxRunes     = 300
	subagentReportSummaryMaxSentences = 3
)

// maybeGenerateChatSummaryAsync launches best-effort whole-chat summary
// generation in the background for a root chat.
func (p *Server) maybeGenerateChatSummaryAsync(
	ctx context.Context,
	logger slog.Logger,
	chat database.Chat,
) {
	if chat.ParentChatID.Valid {
		return
	}
	ctx, cancel := p.inflightContext(ctx)
	if err := p.goInflight(func() {
		defer cancel()
		p.generateAndStoreChatSummary(ctx, logger, chat)
	}); err != nil {
		cancel()
		logger.Debug(ctx, "skipped chat summary generation",
			slog.F("chat_id", chat.ID), slog.Error(err))
	}
}

// generateAndStoreChatSummary regenerates and persists the whole-chat summary
// when due. Best-effort; never clears an existing summary on failure.
func (p *Server) generateAndStoreChatSummary(
	ctx context.Context,
	logger slog.Logger,
	chat database.Chat,
) {
	ctx, cancel := context.WithTimeout(ctx, chatSummaryWorkTimeout)
	defer cancel()

	//nolint:gocritic // Narrow daemon access for best-effort summary generation.
	ctx = dbauthz.AsChatd(ctx)

	// If a turn commits after this read, the stale history_version makes the
	// eventual summary write lose instead of omitting that newer turn.
	chat, err := p.db.GetChatByID(ctx, chat.ID)
	if err != nil {
		logger.Debug(ctx, "failed to re-read chat for summary",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}

	messages, err := p.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	if err != nil {
		logger.Debug(ctx, "failed to load messages for chat summary",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}

	if !shouldGenerateChatSummary(chat, messages) {
		return
	}

	transcript := renderChatSummaryTranscript(messages)
	if len([]rune(transcript)) < summaryMinTranscriptRunes {
		logger.Debug(ctx, "skipping chat summary for short transcript",
			slog.F("chat_id", chat.ID),
			slog.F("transcript_runes", len([]rune(transcript))),
		)
		return
	}

	// Derive the delegated API key from the chat owner so AI Gateway routing
	// attributes summary generation to the correct account. This goroutine may
	// outlive the launching turn, so it cannot rely on that turn's context.
	apiKeyID, err := p.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		logger.Debug(ctx, "failed to ensure synthetic API key for chat summary",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}
	modelOpts := modelBuildOptions{ActiveAPIKeyID: apiKeyID}

	resolved, ok := p.resolveChatSummaryModel(ctx, logger, chat, modelOpts)
	if !ok {
		return
	}

	summaryCtx, cancelGen := context.WithTimeout(ctx, chatSummaryGenerateTimeout)
	defer cancelGen()
	summary, _, genErr := generateChatSummary(summaryCtx, resolved.model.LanguageModel(), summaryObjectCall(resolved), transcript)

	if genErr != nil {
		logger.Debug(ctx, "failed to generate chat summary",
			slog.F("chat_id", chat.ID), slog.Error(genErr))
		return
	}

	p.updateChatSummary(ctx, logger, chat, chat.HistoryVersion, summary)
}

func (p *Server) resolveChatSummaryModel(
	ctx context.Context,
	logger slog.Logger,
	chat database.Chat,
	modelOpts modelBuildOptions,
) (resolvedModelCall, bool) {
	resolved, err := p.resolveModelCall(ctx, modelCallSpec{
		purpose:      "chat_summary",
		chat:         chat,
		buildOptions: modelOpts,
	})
	if err != nil {
		logger.Debug(ctx, "failed to resolve chat model for summary",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return resolvedModelCall{}, false
	}
	return resolved, true
}

func shouldGenerateChatSummary(chat database.Chat, messages []database.ChatMessage) bool {
	if !chat.Summary.Valid {
		return countCompletedTurnsSince(messages, time.Time{}) >= summaryInitialTurnThreshold
	}
	var marker time.Time
	if chat.SummaryGeneratedAt.Valid {
		marker = chat.SummaryGeneratedAt.Time
	}
	return countCompletedTurnsSince(messages, marker) >= summaryStaleTurnThreshold
}

// countCompletedTurnsSince counts visible user messages (one per turn) created
// after the given time. Model-only user messages (injected context, replayed
// compaction summary) are not turns; a zero time counts all.
func countCompletedTurnsSince(messages []database.ChatMessage, after time.Time) int {
	count := 0
	for _, message := range messages {
		if message.Role != database.ChatMessageRoleUser {
			continue
		}
		if message.Visibility != database.ChatMessageVisibilityBoth &&
			message.Visibility != database.ChatMessageVisibilityUser {
			continue
		}
		if !after.IsZero() && !message.CreatedAt.After(after) {
			continue
		}
		count++
	}
	return count
}

// updateChatSummary persists the whole-chat summary. Best-effort background
// write (pass a detached context); a blank summary is a no-op, never clearing
// an existing one.
func (p *Server) updateChatSummary(
	ctx context.Context,
	logger slog.Logger,
	chat database.Chat,
	expectedHistoryVersion int64,
	summary string,
) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return
	}
	sqlSummary := sql.NullString{String: summary, Valid: true}

	ctx, cancel := context.WithTimeout(ctx, chatSummaryWriteTimeout)
	defer cancel()

	affected, err := p.db.UpdateChatSummary(ctx, database.UpdateChatSummaryParams{
		ID:                     chat.ID,
		ExpectedHistoryVersion: expectedHistoryVersion,
		Summary:                sqlSummary,
	})
	if err != nil {
		logger.Warn(ctx, "failed to update chat summary",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}
	if affected == 0 {
		logger.Info(ctx, "skipped stale chat summary update",
			slog.F("chat_id", chat.ID),
			slog.F("summary_length", len(summary)),
			slog.F("expected_history_version", expectedHistoryVersion),
		)
		return
	}

	updatedChat := chat
	updatedChat.Summary = sqlSummary
	p.publishChatPubsubEvent(updatedChat, codersdk.ChatWatchEventKindChatSummaryChange, nil)
}

func (p *Server) storeSubagentReportSummaryAsync(
	ctx context.Context,
	chat database.Chat,
	logger slog.Logger,
) {
	summaryCtx, stopSummaryCtx := p.inflightContext(ctx)
	if err := p.goInflight(func() {
		defer stopSummaryCtx()
		p.storeSubagentReportSummary(summaryCtx, chat, logger)
	}); err != nil {
		stopSummaryCtx()
		logger.Debug(context.WithoutCancel(ctx), "skipped subagent report summary",
			slog.F("chat_id", chat.ID), slog.Error(err))
	}
}

func (p *Server) storeSubagentReportSummary(
	ctx context.Context,
	chat database.Chat,
	logger slog.Logger,
) {
	ctx, cancel := context.WithTimeout(ctx, subagentReportSummaryTimeout)
	defer cancel()

	//nolint:gocritic // Narrow daemon access for best-effort summary writes.
	authCtx := dbauthz.AsChatd(ctx)
	report, err := latestSubagentAssistantMessage(authCtx, p.db, chat.ID)
	if err != nil {
		logger.Debug(ctx, "failed to load subagent report for summary",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}
	summary := subagentReportSummarySnippet(report)
	if summary == "" {
		return
	}
	p.updateChatSummary(ctx, logger, chat, chat.HistoryVersion, summary)
}

func (p *Server) webpushConfigured() bool {
	return p.webpushDispatcher != nil && p.webpushDispatcher.PublicKey() != ""
}

func (p *Server) dispatchPush(
	ctx context.Context,
	chat database.Chat,
	body string,
	status database.ChatStatus,
	logger slog.Logger,
) {
	pushMsg := codersdk.WebpushMessage{
		Title: chat.Title,
		Body:  body,
		Icon:  "/favicon.ico",
		Data:  map[string]string{"url": fmt.Sprintf("/agents/%s", chat.ID)},
	}
	if err := p.webpushDispatcher.Dispatch(ctx, chat.OwnerID, pushMsg); err != nil {
		logger.Warn(ctx, "failed to send chat completion web push",
			slog.F("chat_id", chat.ID),
			slog.F("status", status),
			slog.Error(err),
		)
	}
}

// Close stops the processor and waits for it to finish.
func (p *Server) Close() error {
	p.closeInflightAdmission()
	if unsub := p.configCacheUnsubscribe; unsub != nil {
		p.configCacheUnsubscribe = nil
		unsub()
	}
	if unsub := p.providerCacheUnsubscribe; unsub != nil {
		p.providerCacheUnsubscribe = nil
		unsub()
	}
	if p.chatWorker != nil {
		if err := p.chatWorker.Close(); err != nil {
			p.logger.Warn(context.Background(), "failed to close chat worker", slog.Error(err))
		}
	}
	if p.streamSyncPoller != nil {
		p.streamSyncPoller.Close()
	}
	if p.messagePartBuffer != nil {
		p.messagePartBuffer.Close()
	}
	p.cancel()
	p.wg.Wait()
	p.drainInflight()
	return nil
}

// inflightContext returns a context for an in-flight goroutine launched
// via goInflight. It is detached from reqCtx's cancellation so the work
// can outlive the originating request, while preserving its values for
// auth, routing, and tracing. The context is bound to the server
// lifetime via p.ctx so Close cancels it promptly. The returned stop
// must be called once the work completes to release the shutdown hook.
// The caller is responsible for providing their own timeout.
func (p *Server) inflightContext(reqCtx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(reqCtx))
	stop := context.AfterFunc(p.ctx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

func (p *Server) goInflight(f func()) error {
	if p.inflightClosed.Load() {
		return errInflightClosed
	}

	// Acquire inflightMu around the inflight.Go so Close() cannot
	// call drainInflight concurrently when the counter is at zero.
	// See drainInflight for the WaitGroup contract this preserves.
	p.inflightMu.Lock()
	defer p.inflightMu.Unlock()
	if p.inflightClosed.Load() {
		return errInflightClosed
	}
	p.inflight.Go(f)
	return nil
}

func (p *Server) closeInflightAdmission() {
	p.inflightClosed.Store(true)
}

// drainInflight waits for already-admitted in-flight operations to complete.
// It acquires inflightMu so Wait cannot race with a positive Add from
// goInflight when the WaitGroup counter is zero.
//
// https://pkg.go.dev/sync#WaitGroup.Add
// > Note that calls with a positive delta that occur when the counter is zero must happen before a Wait.
func (p *Server) drainInflight() {
	p.inflightMu.Lock()
	p.inflight.Wait()
	p.inflightMu.Unlock()
}

// refreshExpiredMCPTokens checks each MCP OAuth2 token and refreshes
// any that are expired (or about to expire). Tokens without a
// refresh_token or that fail to refresh are returned unchanged so the
// caller can still attempt the connection (which will likely fail with
// a 401 for the expired ones).
func (p *Server) refreshExpiredMCPTokens(
	ctx context.Context,
	logger slog.Logger,
	configs []database.MCPServerConfig,
	tokens []database.MCPServerUserToken,
) []database.MCPServerUserToken {
	configsByID := make(map[uuid.UUID]database.MCPServerConfig, len(configs))
	for _, cfg := range configs {
		configsByID[cfg.ID] = cfg
	}

	result := slices.Clone(tokens)

	var eg errgroup.Group
	for i, tok := range result {
		cfg, ok := configsByID[tok.MCPServerConfigID]
		if !ok || cfg.AuthType != "oauth2" {
			continue
		}
		if tok.RefreshToken == "" {
			continue
		}
		if tok.OauthRefreshFailureReason != "" {
			// A previous refresh already failed permanently (e.g.
			// revoked grant); the user must reconnect. Skip the
			// provider call entirely.
			continue
		}

		eg.Go(func() error {
			refreshed, err := p.refreshMCPTokenIfNeeded(ctx, logger, cfg, tok)
			if err != nil {
				logger.Warn(ctx, "failed to refresh MCP oauth2 token",
					slog.F("server_slug", cfg.Slug),
					slog.Error(err),
				)
				return nil
			}
			result[i] = refreshed
			return nil
		})
	}
	_ = eg.Wait()

	return result
}

// refreshMCPTokenIfNeeded delegates to mcpclient.RefreshOAuth2Token
// and persists the result to the database when a refresh occurs.
// The logger should carry chat-scoped fields so log lines can be
// correlated with specific chat requests.
func (p *Server) refreshMCPTokenIfNeeded(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
) (database.MCPServerUserToken, error) {
	result, err := mcpclient.RefreshOAuth2Token(ctx, cfg, tok)
	if err != nil {
		if mcpclient.IsPermanentRefreshError(err) {
			return p.markMCPTokenRefreshFailure(ctx, logger, cfg, tok, err), nil
		}
		return tok, err
	}

	if !result.Refreshed {
		return tok, nil
	}

	logger.Info(ctx, "refreshed MCP oauth2 token",
		slog.F("server_slug", cfg.Slug),
		slog.F("user_id", tok.UserID),
	)

	var expiry sql.NullTime
	if !result.Expiry.IsZero() {
		expiry = sql.NullTime{Time: result.Expiry, Valid: true}
	}

	// The chatd subject has no personal-write access; persist the
	// refresh as a subject scoped to this token's owner.
	updated, err := p.db.UpdateMCPServerUserTokenFromRefresh(
		dbauthz.AsChatdTokenOwner(ctx, tok.UserID),
		database.UpdateMCPServerUserTokenFromRefreshParams{
			ID:                tok.ID,
			UpdatedAt:         tok.UpdatedAt,
			AccessToken:       result.AccessToken,
			AccessTokenKeyID:  sql.NullString{},
			RefreshToken:      result.RefreshToken,
			RefreshTokenKeyID: sql.NullString{},
			TokenType:         result.TokenType,
			Expiry:            expiry,
		},
	)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			// A disconnect or re-authentication can win the optimistic update.
			current, readErr := p.db.GetMCPServerUserToken(
				ctx,
				database.GetMCPServerUserTokenParams{
					MCPServerConfigID: tok.MCPServerConfigID,
					UserID:            tok.UserID,
				},
			)
			if readErr == nil {
				return current, nil
			}
			if !xerrors.Is(readErr, sql.ErrNoRows) {
				logger.Warn(ctx, "failed to load MCP oauth2 token after refresh conflict",
					slog.F("server_slug", cfg.Slug),
					slog.Error(readErr),
				)
			}
			tok.AccessToken = ""
			tok.RefreshToken = ""
			tok.Expiry = sql.NullTime{}
			return tok, nil
		}

		// The provider may have rotated the refresh token,
		// invalidating the old one. Use the new token
		// in-memory so at least this connection succeeds.
		logger.Warn(ctx, "failed to persist refreshed MCP oauth2 token, using in-memory",
			slog.F("server_slug", cfg.Slug),
			slog.Error(err),
		)
		tok.AccessToken = result.AccessToken
		tok.RefreshToken = result.RefreshToken
		tok.TokenType = result.TokenType
		tok.Expiry = expiry
		return tok, nil
	}

	return updated, nil
}

// markMCPTokenRefreshFailure persists a permanent refresh failure
// (e.g. the upstream grant was revoked) so the dead token is never
// attached again and the UI can prompt the user to reconnect. It
// always returns a token with cleared auth material, even when
// persistence fails, so the current request does not send a stale
// bearer token.
func (p *Server) markMCPTokenRefreshFailure(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
	refreshErr error,
) database.MCPServerUserToken {
	logger.Warn(ctx, "mcp oauth2 grant permanently unusable, marking token for reconnect",
		slog.F("server_slug", cfg.Slug),
		slog.F("user_id", tok.UserID),
		slog.Error(refreshErr),
	)

	// The chatd subject has no personal-write access; persist the
	// failure as a subject scoped to this token's owner.
	marked, err := p.db.MarkMCPServerUserTokenRefreshFailure(
		dbauthz.AsChatdTokenOwner(ctx, tok.UserID),
		database.MarkMCPServerUserTokenRefreshFailureParams{
			ID:                        tok.ID,
			UpdatedAt:                 tok.UpdatedAt,
			OauthRefreshFailureReason: mcpclient.RefreshFailureReason(refreshErr),
		},
	)
	if err == nil {
		return marked
	}

	if xerrors.Is(err, sql.ErrNoRows) {
		// Optimistic lock miss: a concurrent request refreshed or
		// replaced the token after we read it, so our failure is
		// stale. Use the winner's row instead.
		current, readErr := p.db.GetMCPServerUserToken(
			ctx,
			database.GetMCPServerUserTokenParams{
				MCPServerConfigID: tok.MCPServerConfigID,
				UserID:            tok.UserID,
			},
		)
		if readErr == nil {
			return current
		}
		err = readErr
	}

	logger.Warn(ctx, "failed to persist MCP oauth2 refresh failure",
		slog.F("server_slug", cfg.Slug),
		slog.Error(err),
	)
	tok.AccessToken = ""
	tok.RefreshToken = ""
	tok.Expiry = sql.NullTime{}
	tok.OauthRefreshFailureReason = mcpclient.RefreshFailureReason(refreshErr)
	return tok
}
