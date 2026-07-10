package chatd

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"github.com/dustin/go-humanize"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shopspring/decimal"
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
	"github.com/coder/coder/v2/coderd/notifications"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/xjson"
	"github.com/coder/coder/v2/coderd/webpush"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	skillspkg "github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
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
	manualTitlePersistTimeout    = 5 * time.Second
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

	db       database.Store
	workerID uuid.UUID
	logger   slog.Logger

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

	usageTracker      *workspacestats.UsageTracker
	clock             quartz.Clock
	metrics           *chatloop.Metrics
	chatWorker        *chatWorker
	messagePartBuffer *messagepartbuffer.Buffer
	streamSyncPoller  *streamSyncPoller
	recordingSem      chan struct{}

	aibridgeTransportFactory *atomic.Pointer[aibridge.TransportFactory]
	experiments              codersdk.Experiments

	// Configuration
	pendingChatAcquireInterval time.Duration
	maxChatsPerAcquire         int32
	inFlightChatStaleAfter     time.Duration
	chatHeartbeatInterval      time.Duration
}

// chatTemplateAllowlist returns the deployment-wide template
// allowlist as a set of permitted template IDs. The callback
// signature matches what the chat tools expect. When the
// allowlist is empty or cannot be loaded the function returns
// nil, which the tools interpret as "all templates allowed".
func (p *Server) chatTemplateAllowlist() map[uuid.UUID]bool {
	//nolint:gocritic // AsChatd provides narrowly-scoped daemon
	// access for reading deployment config.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	//nolint:gocritic // AsChatd provides narrowly-scoped read
	// access to deployment config (the template allowlist).
	ctx = dbauthz.AsChatd(ctx)
	raw, err := p.db.GetChatTemplateAllowlist(ctx)
	if err != nil {
		p.logger.Warn(ctx, "failed to load chat template allowlist", slog.Error(err))
		return nil
	}
	ids, err := xjson.ParseUUIDList(raw)
	if err != nil {
		p.logger.Warn(ctx, "failed to parse chat template allowlist", slog.Error(err))
		return nil
	}
	m := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func (p *Server) loadAdvisorConfig(ctx context.Context, logger slog.Logger) codersdk.AdvisorConfig {
	cfg, err := p.configCache.AdvisorConfig(ctx)
	if err != nil {
		logger.Warn(ctx, "failed to load advisor config", slog.Error(err))
		return codersdk.AdvisorConfig{}
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

func (p *Server) resolveAdvisorModelOverride(
	ctx context.Context,
	chat database.Chat,
	advisorCfg codersdk.AdvisorConfig,
	fallbackModel fantasy.LanguageModel,
	fallbackCallConfig codersdk.ChatModelCallConfig,
	modelOpts modelBuildOptions,
	logger slog.Logger,
) (fantasy.LanguageModel, codersdk.ChatModelCallConfig, error) {
	if advisorCfg.ModelConfigID == uuid.Nil {
		return fallbackModel, fallbackCallConfig, nil
	}

	// Re-read the override instead of using the cache so disabled models
	// or providers stop routing advisor prompts immediately.
	overrideConfig, err := p.db.GetEnabledChatModelConfigByID(
		ctx,
		advisorCfg.ModelConfigID,
	)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			logger.Warn(
				ctx,
				"advisor model config is disabled or unavailable, continuing with chat model",
				slog.F("model_config_id", advisorCfg.ModelConfigID),
			)
			return fallbackModel, fallbackCallConfig, nil
		}
		logger.Warn(
			ctx,
			"failed to resolve advisor model config, continuing with chat model",
			slog.F("model_config_id", advisorCfg.ModelConfigID),
			slog.Error(err),
		)
		return fallbackModel, fallbackCallConfig, nil
	}

	overrideCallConfig := codersdk.ChatModelCallConfig{}
	if len(overrideConfig.Options) > 0 {
		if err := json.Unmarshal(overrideConfig.Options, &overrideCallConfig); err != nil {
			logger.Warn(
				ctx,
				"failed to parse advisor model config, continuing with chat model",
				slog.F("model_config_id", advisorCfg.ModelConfigID),
				slog.Error(err),
			)
			return fallbackModel, fallbackCallConfig, nil
		}
	}

	route, err := p.resolveModelRouteForConfig(
		ctx,
		chat.OwnerID,
		overrideConfig,
	)
	if err != nil {
		if overrideConfig.AIProviderID.Valid {
			return nil, codersdk.ChatModelCallConfig{}, xerrors.Errorf("resolve advisor override route: %w", err)
		}
		logger.Warn(
			ctx,
			"failed to resolve advisor override route, continuing with chat model",
			slog.F("model_config_id", advisorCfg.ModelConfigID),
			slog.Error(err),
		)
		return fallbackModel, fallbackCallConfig, nil
	}
	overrideModel, err := p.newModel(ctx, modelClientRequest{
		Chat:          chat,
		ModelName:     overrideConfig.Model,
		UserAgent:     chatprovider.UserAgent(),
		ExtraHeaders:  chatprovider.CoderHeaders(chat),
		ConfigOptions: overrideConfig.Options,
	}, route, modelOpts)
	if err != nil {
		if overrideConfig.AIProviderID.Valid {
			return nil, codersdk.ChatModelCallConfig{}, xerrors.Errorf("create advisor override model: %w", err)
		}
		logger.Warn(
			ctx,
			"failed to create advisor override model, continuing with chat model",
			slog.F("model_config_id", advisorCfg.ModelConfigID),
			slog.Error(err),
		)
		return fallbackModel, fallbackCallConfig, nil
	}

	if advisorCfg.ReasoningEffort != nil {
		resolvedEffort := chatprovider.ResolveReasoningEffort(
			advisorCfg.ReasoningEffort,
			overrideCallConfig.ReasoningEffort,
		)
		if resolvedEffort != nil {
			overrideCallConfig.ReasoningEffort = &codersdk.ChatModelReasoningEffortConfig{
				Default: resolvedEffort,
				Max:     resolvedEffort,
			}
		}
	}

	return overrideModel, overrideCallConfig, nil
}

func (p *Server) newAdvisorRuntime(
	ctx context.Context,
	chat database.Chat,
	advisorCfg codersdk.AdvisorConfig,
	fallbackModel fantasy.LanguageModel,
	fallbackCallConfig codersdk.ChatModelCallConfig,
	modelOpts modelBuildOptions,
	logger slog.Logger,
) (*chatadvisor.Runtime, error) {
	advisorModel, advisorCallConfig, err := p.resolveAdvisorModelOverride(
		ctx,
		chat,
		advisorCfg,
		fallbackModel,
		fallbackCallConfig,
		modelOpts,
		logger,
	)
	if err != nil {
		return nil, err
	}

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

	advisorCallConfig.MaxOutputTokens = ptr.Ref(maxOutputTokens)
	// The override resolver pins an explicit advisor effort into the model
	// config. Fallback models keep their configured default effort.
	advisorReasoningEffort := chatprovider.ResolveReasoningEffort(
		nil,
		advisorCallConfig.ReasoningEffort,
	)
	providerOptions := chatprovider.ProviderOptionsFromChatModelConfig(
		advisorModel,
		advisorCallConfig.ProviderOptions,
	)
	providerOptions = chatprovider.ApplyReasoningEffort(
		advisorModel,
		providerOptions,
		advisorReasoningEffort,
	)

	rt, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model:           advisorModel,
		ModelConfig:     advisorCallConfig,
		ProviderOptions: providerOptions,
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

type turnWorkspaceContext struct {
	server           *Server
	chatStateMu      *sync.Mutex
	currentChat      *database.Chat
	loadChatSnapshot func(context.Context, uuid.UUID) (database.Chat, error)

	mu                sync.Mutex
	agent             database.WorkspaceAgent
	agentLoaded       bool
	conn              workspacesdk.AgentConn
	releaseConn       func()
	cachedWorkspaceID uuid.NullUUID
}

func (c *turnWorkspaceContext) close() {
	c.clearCachedWorkspaceState()
}

func (c *turnWorkspaceContext) clearCachedWorkspaceState() {
	c.mu.Lock()
	releaseConn := c.releaseConn
	c.agent = database.WorkspaceAgent{}
	c.agentLoaded = false
	c.conn = nil
	c.releaseConn = nil
	c.cachedWorkspaceID = uuid.NullUUID{}
	c.mu.Unlock()

	if releaseConn != nil {
		releaseConn()
	}
}

func (c *turnWorkspaceContext) setCurrentChat(chat database.Chat) {
	c.chatStateMu.Lock()
	*c.currentChat = chat
	c.chatStateMu.Unlock()
}

func (c *turnWorkspaceContext) currentChatSnapshot() database.Chat {
	c.chatStateMu.Lock()
	chatSnapshot := *c.currentChat
	c.chatStateMu.Unlock()
	return chatSnapshot
}

func (c *turnWorkspaceContext) selectWorkspace(chat database.Chat) {
	c.setCurrentChat(chat)
	c.clearCachedWorkspaceState()
}

func (c *turnWorkspaceContext) currentWorkspaceMatches(expected uuid.NullUUID) (database.Chat, bool) {
	chatSnapshot := c.currentChatSnapshot()
	return chatSnapshot, nullUUIDEqual(chatSnapshot.WorkspaceID, expected)
}

func (c *turnWorkspaceContext) trackWorkspaceUsage(ctx context.Context, chatSnapshot database.Chat) {
	if c.server == nil || !chatSnapshot.WorkspaceID.Valid {
		return
	}
	logger := c.server.logger.With(
		slog.F("chat_id", chatSnapshot.ID),
		slog.F("owner_id", chatSnapshot.OwnerID),
	)
	c.server.trackWorkspaceUsage(ctx, chatSnapshot.ID, chatSnapshot.WorkspaceID, logger)
}

func nullUUIDEqual(left, right uuid.NullUUID) bool {
	if left.Valid != right.Valid {
		return false
	}
	if !left.Valid {
		return true
	}
	return left.UUID == right.UUID
}

func (c *turnWorkspaceContext) persistBuildAgentBinding(
	ctx context.Context,
	chatSnapshot database.Chat,
	buildID uuid.UUID,
	agentID uuid.UUID,
) (database.Chat, error) {
	updatedChat, err := c.server.db.UpdateChatBuildAgentBinding(
		ctx,
		database.UpdateChatBuildAgentBindingParams{
			ID: chatSnapshot.ID,
			BuildID: uuid.NullUUID{
				UUID:  buildID,
				Valid: true,
			},
			AgentID: uuid.NullUUID{
				UUID:  agentID,
				Valid: true,
			},
		},
	)
	if err != nil {
		return chatSnapshot, xerrors.Errorf(
			"update chat build/agent binding: %w", err,
		)
	}

	// If the chat was rebound to a different agent (e.g. a workspace rebuild
	// produced a new agent), re-pin its context to the new agent so it stops
	// injecting the previous agent's resources. Best-effort: a context error
	// must never fail the binding. The pinned context fields on updatedChat
	// are background state, reloaded on the next snapshot fetch.
	if chatSnapshot.AgentID.Valid && chatSnapshot.AgentID.UUID != agentID {
		//nolint:gocritic // Chatd re-pins chats it does not own as the daemon subject.
		repinCtx := dbauthz.AsChatd(ctx)
		if repinErr := database.ReadModifyUpdate(c.server.db, func(tx database.Store) error {
			return repinChatContext(repinCtx, tx, chatSnapshot.ID, uuid.NullUUID{UUID: agentID, Valid: true})
		}); repinErr != nil {
			c.server.logger.Warn(ctx, "re-pin chat context after agent rebind",
				slog.F("chat_id", chatSnapshot.ID),
				slog.F("agent_id", agentID),
				slog.Error(repinErr))
		}
	}

	c.setCurrentChat(updatedChat)
	return updatedChat, nil
}

func (c *turnWorkspaceContext) getWorkspaceAgent(ctx context.Context) (database.WorkspaceAgent, error) {
	_, agent, err := c.ensureWorkspaceAgent(ctx)
	return agent, err
}

func (c *turnWorkspaceContext) ensureWorkspaceAgent(
	ctx context.Context,
) (database.Chat, database.WorkspaceAgent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.agentLoaded {
		chatSnapshot := c.currentChatSnapshot()
		if nullUUIDEqual(c.cachedWorkspaceID, chatSnapshot.WorkspaceID) {
			return chatSnapshot, c.agent, nil
		}
		c.agent = database.WorkspaceAgent{}
		c.agentLoaded = false
	}

	return c.loadWorkspaceAgentLocked(ctx)
}

func (c *turnWorkspaceContext) loadWorkspaceAgentLocked(
	ctx context.Context,
) (database.Chat, database.WorkspaceAgent, error) {
	chatSnapshot := c.currentChatSnapshot()

	for attempt := 0; attempt < 2; attempt++ {
		if !chatSnapshot.WorkspaceID.Valid {
			refreshedChat, refreshErr := refreshChatWorkspaceSnapshot(
				ctx,
				chatSnapshot,
				c.loadChatSnapshot,
			)
			if refreshErr != nil {
				return chatSnapshot, database.WorkspaceAgent{}, refreshErr
			}
			if refreshedChat.WorkspaceID.Valid {
				c.setCurrentChat(refreshedChat)
				chatSnapshot = refreshedChat
			}
		}

		if !chatSnapshot.WorkspaceID.Valid {
			return chatSnapshot, database.WorkspaceAgent{}, xerrors.New("no workspace is associated with this chat. Use the create_workspace tool to create one")
		}

		if chatSnapshot.AgentID.Valid {
			agent, err := c.server.db.GetWorkspaceAgentByID(ctx, chatSnapshot.AgentID.UUID)
			if err == nil {
				latestChat, workspaceMatches := c.currentWorkspaceMatches(chatSnapshot.WorkspaceID)
				if !workspaceMatches {
					chatSnapshot = latestChat
					continue
				}
				c.agent = agent
				c.agentLoaded = true
				c.cachedWorkspaceID = chatSnapshot.WorkspaceID
				return chatSnapshot, c.agent, nil
			}
			if !xerrors.Is(err, sql.ErrNoRows) {
				c.server.logger.Warn(ctx, "agent binding lookup failed, re-resolving",
					slog.F("agent_id", chatSnapshot.AgentID.UUID),
					slog.Error(err),
				)
			}
		}

		agents, err := c.server.db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(
			ctx,
			chatSnapshot.WorkspaceID.UUID,
		)
		if err != nil {
			return chatSnapshot, database.WorkspaceAgent{}, xerrors.Errorf(
				"get workspace agents in latest build: %w",
				err,
			)
		}
		if len(agents) == 0 {
			return chatSnapshot, database.WorkspaceAgent{}, errChatHasNoWorkspaceAgent
		}
		selected, err := agentselect.FindChatAgent(agents)
		if err != nil {
			return chatSnapshot, database.WorkspaceAgent{}, xerrors.Errorf(
				"find chat agent: %w",
				err,
			)
		}

		build, err := c.server.db.GetLatestWorkspaceBuildByWorkspaceID(ctx, chatSnapshot.WorkspaceID.UUID)
		if err != nil {
			return chatSnapshot, database.WorkspaceAgent{}, xerrors.Errorf("get latest workspace build: %w", err)
		}

		updatedChat, err := c.persistBuildAgentBinding(
			ctx,
			chatSnapshot,
			build.ID,
			selected.ID,
		)
		if err != nil {
			return chatSnapshot, database.WorkspaceAgent{}, err
		}

		chatSnapshot = updatedChat
		latestChat, workspaceMatches := c.currentWorkspaceMatches(chatSnapshot.WorkspaceID)
		if !workspaceMatches {
			chatSnapshot = latestChat
			continue
		}
		c.agent = selected
		c.agentLoaded = true
		c.cachedWorkspaceID = chatSnapshot.WorkspaceID
		return chatSnapshot, c.agent, nil
	}

	return chatSnapshot, database.WorkspaceAgent{}, xerrors.New(
		"chat workspace changed while resolving agent",
	)
}

func (c *turnWorkspaceContext) latestWorkspaceAgentID(
	ctx context.Context,
	workspaceID uuid.UUID,
) (uuid.UUID, error) {
	agents, err := c.server.db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(
		ctx,
		workspaceID,
	)
	if err != nil {
		return uuid.Nil, xerrors.Errorf(
			"get workspace agents in latest build: %w",
			err,
		)
	}
	if len(agents) == 0 {
		return uuid.Nil, errChatHasNoWorkspaceAgent
	}
	selected, err := agentselect.FindChatAgent(agents)
	if err != nil {
		return uuid.Nil, xerrors.Errorf(
			"find chat agent: %w",
			err,
		)
	}
	return selected.ID, nil
}

func (c *turnWorkspaceContext) workspaceAgentIDForConn(
	ctx context.Context,
) (database.Chat, uuid.UUID, error) {
	for attempt := 0; attempt < 2; attempt++ {
		chatSnapshot := c.currentChatSnapshot()
		if !chatSnapshot.WorkspaceID.Valid || !chatSnapshot.AgentID.Valid {
			updatedChat, agent, err := c.ensureWorkspaceAgent(ctx)
			if err != nil {
				return updatedChat, uuid.Nil, err
			}
			return updatedChat, agent.ID, nil
		}

		currentAgentID, err := c.latestWorkspaceAgentID(
			ctx,
			chatSnapshot.WorkspaceID.UUID,
		)
		if err != nil {
			if xerrors.Is(err, errChatHasNoWorkspaceAgent) {
				c.clearCachedWorkspaceState()
			}
			return chatSnapshot, uuid.Nil, err
		}

		latestChat, workspaceMatches := c.currentWorkspaceMatches(
			chatSnapshot.WorkspaceID,
		)
		if !workspaceMatches {
			continue
		}
		return latestChat, currentAgentID, nil
	}

	chatSnapshot := c.currentChatSnapshot()
	return chatSnapshot, uuid.Nil, xerrors.New(
		"chat workspace changed while resolving agent",
	)
}

// getWorkspaceConnLocked returns the cached connection when it still matches
// the current workspace. When the workspace changed, it clears the stale
// cached state and returns the release func for the caller to run after
// unlocking.
func (c *turnWorkspaceContext) getWorkspaceConnLocked() (workspacesdk.AgentConn, func()) {
	if c.conn == nil {
		return nil, nil
	}

	chatSnapshot := c.currentChatSnapshot()
	if nullUUIDEqual(c.cachedWorkspaceID, chatSnapshot.WorkspaceID) {
		return c.conn, nil
	}

	agentRelease := c.releaseConn
	c.agent = database.WorkspaceAgent{}
	c.agentLoaded = false
	c.conn = nil
	c.releaseConn = nil
	c.cachedWorkspaceID = uuid.NullUUID{}
	return nil, agentRelease
}

// isAgentUnreachable reports whether the given agent row's
// status is disconnected or timed out. It uses timestamp
// arithmetic on the row. The "connecting" state is allowed
// through because it is normal after a fresh workspace build.
func isAgentUnreachable(now time.Time, agent database.WorkspaceAgent, inactiveTimeout time.Duration) bool {
	status := agent.Status(now, inactiveTimeout)
	return status.Status == database.WorkspaceAgentStatusDisconnected ||
		status.Status == database.WorkspaceAgentStatusTimeout
}

func agentDisconnectedFor(now time.Time, agent database.WorkspaceAgent, inactiveTimeout time.Duration) (time.Duration, bool) {
	status := agent.Status(now, inactiveTimeout)
	if status.Status != database.WorkspaceAgentStatusDisconnected || status.DisconnectedAt == nil {
		return 0, false
	}

	disconnectedFor := now.Sub(*status.DisconnectedAt)
	if disconnectedFor < 0 {
		disconnectedFor = 0
	}
	return disconnectedFor, true
}

func (c *turnWorkspaceContext) latestWorkspaceAgentRecoveryError(
	ctx context.Context,
	workspaceID uuid.UUID,
) error {
	agentID, err := c.latestWorkspaceAgentID(ctx, workspaceID)
	if err != nil {
		if xerrors.Is(err, errChatHasNoWorkspaceAgent) {
			return err
		}
		c.server.logger.Warn(ctx, "failed to resolve latest agent for timeout classification", slog.Error(err))
		return errChatDialTimeout
	}

	agent, err := c.server.db.GetWorkspaceAgentByID(ctx, agentID)
	if err != nil {
		c.server.logger.Warn(ctx, "failed to load latest agent for timeout classification",
			slog.F("agent_id", agentID),
			slog.Error(err),
		)
		return errChatDialTimeout
	}

	now := c.server.clock.Now()
	status := agent.Status(now, c.server.agentInactiveDisconnectTimeout)
	recoveryErr := errChatDialTimeout
	if status.Status == database.WorkspaceAgentStatusTimeout {
		recoveryErr = errChatAgentNeverConnected
	} else if status.Status == database.WorkspaceAgentStatusDisconnected && status.DisconnectedAt != nil {
		disconnectedFor := now.Sub(*status.DisconnectedAt)
		if disconnectedFor < 0 {
			disconnectedFor = 0
		}
		if disconnectedFor >= agentDisconnectedRecoveryThreshold {
			recoveryErr = errChatAgentDisconnected
		}
	}
	return c.externalAgentError(ctx, agent, recoveryErr)
}

func (c *turnWorkspaceContext) externalAgentError(
	ctx context.Context,
	agent database.WorkspaceAgent,
	fallback error,
) error {
	isExternal, err := chattool.IsExternalWorkspaceAgent(ctx, c.server.db, agent)
	if err != nil || !isExternal {
		return fallback
	}
	return newChatExternalAgentUnavailableError(agent)
}

func (c *turnWorkspaceContext) externalAgentPreflightError(
	ctx context.Context,
	chatSnapshot database.Chat,
	agent database.WorkspaceAgent,
) error {
	// Mirror the cache-hit gate: only short-circuit on clearly offline
	// states (Disconnected/Timeout). Connecting is allowed through so
	// an external agent the user just started can still connect inside
	// the normal dial window.
	if !isAgentUnreachable(c.server.clock.Now(), agent, c.server.agentInactiveDisconnectTimeout) {
		return nil
	}

	isExternal, err := chattool.IsExternalWorkspaceAgent(ctx, c.server.db, agent)
	if err != nil || !isExternal || !chatSnapshot.WorkspaceID.Valid {
		return nil
	}

	// Stale agent bindings rely on dialWithLazyValidation to discover
	// replacement agents, so only skip the dial when this agent is still
	// the latest selected chat agent for the workspace.
	latestAgentID, err := c.latestWorkspaceAgentID(ctx, chatSnapshot.WorkspaceID.UUID)
	if err != nil || latestAgentID != agent.ID {
		return nil
	}
	return newChatExternalAgentUnavailableError(agent)
}

func (c *turnWorkspaceContext) getWorkspaceConn(ctx context.Context) (workspacesdk.AgentConn, error) {
	if c.server.agentConnFn == nil {
		return nil, xerrors.New("workspace agent connector is not configured")
	}

	for attempt := 0; attempt < 2; attempt++ {
		c.mu.Lock()
		currentConn, staleRelease := c.getWorkspaceConnLocked()
		// Capture agentID in the same lock section as
		// currentConn to prevent a TOCTOU race with
		// concurrent clearCachedWorkspaceState calls.
		agentID := c.agent.ID
		c.mu.Unlock()

		// Status check on cache hit: re-fetch the agent
		// row so we see the latest heartbeat rather than
		// a potentially stale cached copy.
		if currentConn != nil {
			chatSnapshot := c.currentChatSnapshot()
			if agentID != uuid.Nil {
				freshAgent, err := c.server.db.GetWorkspaceAgentByID(ctx, agentID)
				if err != nil {
					c.server.logger.Warn(ctx, "failed to re-fetch agent for status check",
						slog.F("agent_id", agentID),
						slog.Error(err),
					)
					// On DB error the check re-runs on the
					// next tool call.
				} else if _, disconnected := agentDisconnectedFor(
					c.server.clock.Now(),
					freshAgent,
					c.server.agentInactiveDisconnectTimeout,
				); disconnected {
					c.clearCachedWorkspaceState()
					continue
				}
			}
			c.trackWorkspaceUsage(ctx, chatSnapshot)
			return currentConn, nil
		}
		if staleRelease != nil {
			staleRelease()
		}

		chatSnapshot, agent, err := c.ensureWorkspaceAgent(ctx)
		if err != nil {
			return nil, err
		}
		if err := c.externalAgentPreflightError(ctx, chatSnapshot, agent); err != nil {
			return nil, err
		}

		// Wrap the dial in a timeout to bound the time spent
		// waiting for an unreachable agent. The timeout scopes
		// only dialWithLazyValidation, not ensureWorkspaceAgent
		// or the post-dial binding steps.
		dialCtx, dialCancelCause := context.WithCancelCause(ctx)
		dialTimer := c.server.clock.AfterFunc(
			c.server.dialTimeout,
			func() { dialCancelCause(errChatDialTimeout) },
			"chatd",
			dialTimeoutTimerTag,
		)
		dialCancel := func() {
			dialTimer.Stop()
			dialCancelCause(nil)
		}
		dialResult, err := dialWithLazyValidation(
			dialCtx,
			c.server.clock,
			agent.ID,
			chatSnapshot.WorkspaceID.UUID,
			DialFunc(c.server.agentConnFn),
			func(ctx context.Context, workspaceID uuid.UUID) (uuid.UUID, error) {
				return c.latestWorkspaceAgentID(ctx, workspaceID)
			},
			workspaceDialValidationDelay,
		)
		dialCancel()
		if err != nil {
			if xerrors.Is(err, errChatHasNoWorkspaceAgent) {
				c.clearCachedWorkspaceState()
				return nil, err
			}
			// Surface the dial timeout sentinel only when the
			// parent context is still alive. If the parent was
			// canceled (e.g. ErrInterrupted), its error must
			// propagate unchanged so the chatloop can detect it.
			if ctx.Err() == nil && errors.Is(context.Cause(dialCtx), errChatDialTimeout) {
				c.clearCachedWorkspaceState()
				return nil, c.latestWorkspaceAgentRecoveryError(ctx, chatSnapshot.WorkspaceID.UUID)
			}
			return nil, err
		}
		agentConn := dialResult.Conn
		agentRelease := dialResult.Release
		if dialResult.WasSwitched {
			build, err := c.server.db.GetLatestWorkspaceBuildByWorkspaceID(ctx, chatSnapshot.WorkspaceID.UUID)
			if err != nil {
				if agentRelease != nil {
					agentRelease()
				}
				return nil, xerrors.Errorf("get latest workspace build: %w", err)
			}

			switchedAgent, err := c.server.db.GetWorkspaceAgentByID(ctx, dialResult.AgentID)
			if err != nil {
				if agentRelease != nil {
					agentRelease()
				}
				return nil, xerrors.Errorf("get workspace agent by id: %w", err)
			}

			updatedChat, err := c.persistBuildAgentBinding(
				ctx,
				chatSnapshot,
				build.ID,
				switchedAgent.ID,
			)
			if err != nil {
				if agentRelease != nil {
					agentRelease()
				}
				return nil, err
			}
			chatSnapshot = updatedChat

			c.mu.Lock()
			c.agent = switchedAgent
			c.agentLoaded = true
			c.cachedWorkspaceID = chatSnapshot.WorkspaceID
			c.mu.Unlock()
		}

		if _, workspaceMatches := c.currentWorkspaceMatches(chatSnapshot.WorkspaceID); !workspaceMatches {
			if agentRelease != nil {
				agentRelease()
			}
			c.clearCachedWorkspaceState()
			continue
		}

		c.mu.Lock()
		if c.conn == nil {
			c.conn = agentConn
			c.releaseConn = agentRelease
			c.cachedWorkspaceID = chatSnapshot.WorkspaceID

			var ancestorIDs []string
			if chatSnapshot.ParentChatID.Valid {
				ancestorIDs = append(ancestorIDs, chatSnapshot.ParentChatID.UUID.String())
			}
			ancestorJSON, marshalErr := json.Marshal(ancestorIDs)
			if marshalErr != nil {
				ancestorJSON = []byte("[]")
			}
			agentConn.SetExtraHeaders(http.Header{
				workspacesdk.CoderChatIDHeader:          {chatSnapshot.ID.String()},
				workspacesdk.CoderAncestorChatIDsHeader: {string(ancestorJSON)},
			})

			c.mu.Unlock()
			c.server.logger.Debug(ctx, "set chat headers on agent conn",
				slog.F("chat_id", chatSnapshot.ID),
				slog.F("ancestor_chat_ids", ancestorIDs),
				slog.F("workspace_id", chatSnapshot.WorkspaceID.UUID),
				slog.F("agent_id", dialResult.AgentID),
			)
			c.trackWorkspaceUsage(ctx, chatSnapshot)
			return agentConn, nil
		}
		currentConn = c.conn
		c.mu.Unlock()

		if agentRelease != nil {
			agentRelease()
		}
		c.trackWorkspaceUsage(ctx, chatSnapshot)
		return currentConn, nil
	}

	return nil, xerrors.New("chat workspace changed while connecting")
}

// AgentConnFunc provides access to workspace agent connections.
type AgentConnFunc func(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error)

var (
	// ErrInvalidModelConfigID indicates the requested model config does not exist.
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
	ErrNoDefaultChatModelConfig = xerrors.New("no default chat model config is configured")
)

// UsageLimitExceededError indicates the user has exceeded their chat spend
// limit.
type UsageLimitExceededError struct {
	LimitMicros    int64
	ConsumedMicros int64
	PeriodEnd      time.Time
}

func formatMicrosAsDollars(micros int64) string {
	return "$" + decimal.NewFromInt(micros).Shift(-6).StringFixed(2)
}

func (e *UsageLimitExceededError) Error() string {
	return fmt.Sprintf(
		"usage limit exceeded: spent %s of %s limit, resets at %s",
		formatMicrosAsDollars(e.ConsumedMicros),
		formatMicrosAsDollars(e.LimitMicros),
		e.PeriodEnd.Format(time.RFC3339),
	)
}

// CreateOptions controls chat creation in the shared chat mutation path.
type CreateOptions struct {
	OrganizationID     uuid.UUID
	OwnerID            uuid.UUID
	WorkspaceID        uuid.NullUUID
	BuildID            uuid.NullUUID
	AgentID            uuid.NullUUID
	ParentChatID       uuid.NullUUID
	RootChatID         uuid.NullUUID
	Title              string
	ModelConfigID      uuid.UUID
	ReasoningEffort    *string
	ChatMode           database.NullChatMode
	PlanMode           database.NullChatPlanMode
	ClientType         database.ChatClientType
	SystemPrompt       string
	InitialUserContent []codersdk.ChatMessagePart
	MCPServerIDs       []uuid.UUID
	Labels             database.StringMap
	DynamicTools       json.RawMessage
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
	Chat          database.Chat
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
}

// EditMessageResult contains the replacement user message and chat status.
type EditMessageResult struct {
	Message database.ChatMessage
	Chat    database.Chat
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

	// Usage limits gate the create before we touch the state machine.
	if limitErr := p.checkUsageLimit(ctx, p.db, opts.OwnerID, uuid.NullUUID{UUID: opts.OrganizationID, Valid: true}); limitErr != nil {
		return database.Chat{}, limitErr
	}

	apiKeyID, err := p.ensureSyntheticAPIKeyID(ctx, opts.OwnerID)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("ensure synthetic API key: %w", err)
	}

	labelsJSON, err := json.Marshal(opts.Labels)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("marshal labels: %w", err)
	}

	userPrompt := SanitizePromptText(opts.SystemPrompt)
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
	userContent, err := chatprompt.MarshalParts(opts.InitialUserContent)
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
	initialMessages = append(initialMessages, userMessageWithAPIKeyID(userContent, opts.ModelConfigID, opts.OwnerID, apiKeyID, opts.ReasoningEffort))

	result, err := chatstate.CreateChat(ctx, p.db, p.pubsub, chatstate.CreateChatInput{
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

	content, err := chatprompt.MarshalParts(opts.Content)
	if err != nil {
		return SendMessageResult{}, xerrors.Errorf("marshal message content: %w", err)
	}

	requestedPlanMode := opts.PlanMode
	requestedMCPServerIDs := opts.MCPServerIDs

	chat, err := p.db.GetChatByID(ctx, opts.ChatID)
	if err != nil {
		return SendMessageResult{}, xerrors.Errorf("load chat: %w", err)
	}
	apiKeyID, err := p.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		return SendMessageResult{}, xerrors.Errorf("ensure synthetic API key: %w", err)
	}

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

		// Enforce usage limits before any state-machine work.
		if limitErr := p.checkUsageLimit(ctx, store, lockedChat.OwnerID, uuid.NullUUID{UUID: lockedChat.OrganizationID, Valid: true}); limitErr != nil {
			return limitErr
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

		// Update MCP server IDs on the chat when explicitly provided.
		// Explore child chats keep the spawn-time snapshot immutable.
		if requestedMCPServerIDs != nil {
			if isExploreSubagentMode(lockedChat.Mode) {
				p.logger.Warn(ctx,
					"ignoring explore subagent mcp server ids update, snapshot is immutable after spawn",
					slog.F("chat_id", opts.ChatID),
				)
			} else {
				lockedChat, err = store.UpdateChatMCPServerIDs(ctx, database.UpdateChatMCPServerIDsParams{
					ID:           opts.ChatID,
					MCPServerIDs: *requestedMCPServerIDs,
				})
				if err != nil {
					return xerrors.Errorf("update chat mcp server ids: %w", err)
				}
			}
		}

		messageCreatedBy := opts.CreatedBy
		if messageCreatedBy == uuid.Nil {
			messageCreatedBy = lockedChat.OwnerID
		}

		// Queue capacity is enforced inside tx.SendMessage; this
		// wrapper only propagates the typed error.
		sendResult, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userMessageWithAPIKeyID(content, modelConfigID, messageCreatedBy, apiKeyID, opts.ReasoningEffort),
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

func (p *Server) checkUsageLimit(ctx context.Context, store database.Store, ownerID uuid.UUID, organizationID uuid.NullUUID) error {
	status, err := ResolveUsageLimitStatus(ctx, store, ownerID, organizationID, time.Now())
	if err != nil {
		// Fail open: never block chat due to a limit-resolution failure.
		p.logger.Warn(ctx, "usage limit check failed, allowing message",
			slog.F("owner_id", ownerID),
			slog.Error(err),
		)
		return nil
	}
	if status == nil {
		return nil
	}
	// Block when current spend reaches or exceeds limit (>= ensures
	// the user cannot start new conversations once the limit is hit).
	if status.SpendLimitMicros != nil && status.CurrentSpend >= *status.SpendLimitMicros {
		return &UsageLimitExceededError{
			LimitMicros:    *status.SpendLimitMicros,
			ConsumedMicros: status.CurrentSpend,
			PeriodEnd:      status.PeriodEnd,
		}
	}
	return nil
}

func chatdModelConfigLookupContext(ctx context.Context) context.Context {
	//nolint:gocritic // Chat message admission needs daemon-scoped
	// deployment-config reads for model config validation.
	return dbauthz.AsChatd(ctx)
}

func resolveSendMessageModelConfigID(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	requested uuid.UUID,
) (uuid.UUID, error) {
	if requested == uuid.Nil {
		return resolveFallbackModelConfigID(ctx, store, chat.LastModelConfigID)
	}

	chatdCtx := chatdModelConfigLookupContext(ctx)
	if _, err := store.GetChatModelConfigByID(chatdCtx, requested); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, xerrors.Errorf(
				"%w: %s",
				ErrInvalidModelConfigID,
				requested,
			)
		}
		return uuid.Nil, xerrors.Errorf(
			"get requested model config %s: %w",
			requested,
			err,
		)
	}
	return requested, nil
}

func resolveFallbackModelConfigID(
	ctx context.Context,
	store database.Store,
	modelConfigID uuid.UUID,
) (uuid.UUID, error) {
	chatdCtx := chatdModelConfigLookupContext(ctx)
	if modelConfigID != uuid.Nil {
		if _, err := store.GetChatModelConfigByID(chatdCtx, modelConfigID); err == nil {
			return modelConfigID, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, xerrors.Errorf(
				"get chat model config %s: %w",
				modelConfigID,
				err,
			)
		}
	}

	defaultConfig, err := store.GetDefaultChatModelConfig(chatdCtx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, ErrNoDefaultChatModelConfig
		}
		return uuid.Nil, xerrors.Errorf("get default chat model config: %w", err)
	}
	return defaultConfig.ID, nil
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

	content, err := chatprompt.MarshalParts(opts.Content)
	if err != nil {
		return EditMessageResult{}, xerrors.Errorf("marshal message content: %w", err)
	}
	chat, err := p.db.GetChatByID(ctx, opts.ChatID)
	if err != nil {
		return EditMessageResult{}, xerrors.Errorf("load chat: %w", err)
	}
	apiKeyID, err := p.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		return EditMessageResult{}, xerrors.Errorf("ensure synthetic API key: %w", err)
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
		if limitErr := p.checkUsageLimit(ctx, store, lockedChat.OwnerID, uuid.NullUUID{UUID: lockedChat.OrganizationID, Valid: true}); limitErr != nil {
			return limitErr
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
		editedMsg = target

		// Validate the optional model-config override up front so
		// the user sees ErrInvalidModelConfigID instead of a
		// foreign-key error from the message-insert path.
		var modelOverride uuid.NullUUID
		if opts.ModelConfigID != uuid.Nil {
			if _, err := store.GetChatModelConfigByID(
				chatdModelConfigLookupContext(ctx),
				opts.ModelConfigID,
			); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return xerrors.Errorf(
						"%w: %s",
						ErrInvalidModelConfigID,
						opts.ModelConfigID,
					)
				}
				return xerrors.Errorf(
					"get requested model config %s: %w",
					opts.ModelConfigID,
					err,
				)
			}
			modelOverride = uuid.NullUUID{UUID: opts.ModelConfigID, Valid: true}
		}

		var reasoningEffortOverride database.NullChatReasoningEffort
		if opts.ReasoningEffort != nil && *opts.ReasoningEffort != "" {
			reasoningEffortOverride = database.NullChatReasoningEffort{ChatReasoningEffort: database.ChatReasoningEffort(*opts.ReasoningEffort), Valid: true}
		}

		editResult, err := tx.EditMessage(chatstate.EditMessageInput{
			MessageID:               opts.EditedMessageID,
			CreatedBy:               opts.CreatedBy,
			Content:                 content,
			ModelConfigIDOverride:   modelOverride,
			ReasoningEffortOverride: reasoningEffortOverride,
			APIKeyID:                sql.NullString{String: apiKeyID, Valid: true},
		})
		if err != nil {
			if errors.Is(err, chatstate.ErrEditedMessageNotUser) {
				return ErrEditedMessageNotUser
			}
			return err
		}
		result.Message = editResult.ReplacementMessage
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

// SubmitToolResults validates and persists client-provided tool
// results, returning the chat to running through the chatstate state
// machine. Validation runs inside the same transaction as the
// transition so the assistant message and pending tool calls cannot
// drift between reads.
func (p *Server) SubmitToolResults(
	ctx context.Context,
	opts SubmitToolResultsOptions,
) error {
	var (
		statusConflict *ToolResultStatusConflictError
		refreshChat    database.Chat
		refreshedOK    bool
	)
	machine := p.newChatMachine(opts.ChatID)
	updateErr := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		locked, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if locked.Archived {
			return ErrChatArchived
		}

		toolResults := make([]chatstate.ToolResultInput, 0, len(opts.Results))
		for _, r := range opts.Results {
			toolResults = append(toolResults, chatstate.ToolResultInput{
				ToolCallID: r.ToolCallID,
				Output:     r.Output,
				IsError:    r.IsError,
			})
		}
		modelConfigID := opts.ModelConfigID
		if modelConfigID == uuid.Nil {
			modelConfigID = locked.LastModelConfigID
		}
		if _, err := tx.CompleteRequiresAction(chatstate.CompleteRequiresActionInput{
			CreatedBy:     opts.UserID,
			ModelConfigID: modelConfigID,
			Results:       toolResults,
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
		// Capture the chat inside the transaction so the watch event
		// uses the snapshot bump and status change produced by the
		// transition itself.
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

// RegenerateChatTitle regenerates a chat title from the chat's visible
// messages, persists it when it changes, and broadcasts the update.
func (p *Server) RegenerateChatTitle(
	ctx context.Context,
	chat database.Chat,
) (database.Chat, error) {
	// Reuse chatd's scoped auth context for deployment-config lookups while
	// keeping chat ownership authorization at the HTTP layer.
	//nolint:gocritic // Non-admin users need chatd-scoped config reads here.
	chatdCtx := dbauthz.AsChatd(ctx)
	return p.regenerateChatTitleWithStore(
		chatdCtx,
		p.db,
		chat,
	)
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
	if limitErr := p.checkUsageLimit(ctx, store, chat.OwnerID, uuid.NullUUID{UUID: chat.OrganizationID, Valid: true}); limitErr != nil {
		return "", limitErr
	}

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

	model, modelConfig, err := p.resolveManualTitleModel(ctx, store, chat, modelOpts)
	if err != nil {
		return "", err
	}

	titleCtx := ctx
	titleModel := model
	finishDebugRun := func(error) {}
	if debugSvc := p.debugService(); debugSvc != nil && debugSvc.IsEnabled(ctx, chat.ID, chat.OwnerID) {
		titleCtx, titleModel, finishDebugRun = p.prepareManualTitleDebugRun(
			ctx,
			debugSvc,
			chat,
			modelConfig,
			modelOpts,
			messages,
			model,
		)
	}

	title, err := generateManualTitle(
		titleCtx,
		messages,
		pasteText,
		titleModel,
		p.titleGenerationProviderOptions(ctx, titleModel, modelConfig),
	)
	finishDebugRun(err)
	if err != nil {
		return "", xerrors.Errorf("generate manual title: %w", err)
	}

	return title, nil
}

func (p *Server) regenerateChatTitleWithStore(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
) (database.Chat, error) {
	title, err := p.generateManualTitleCandidate(ctx, store, chat)
	if err != nil {
		return database.Chat{}, err
	}
	if title == "" {
		return chat, nil
	}

	// Generation already happened; don't let a client disconnect drop the
	// title write.
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), manualTitlePersistTimeout)
	defer persistCancel()

	updatedChat, wroteTitle, err := persistManualTitle(persistCtx, store, chat, title)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("update chat title: %w", err)
	}
	// Publish only when this regeneration wrote the title. When a
	// concurrent rename won the race, the rename path already published
	// the fresher title; re-publishing the re-read row here could
	// deliver a stale title_change after an even newer rename.
	if !wroteTitle {
		return updatedChat, nil
	}

	p.publishChatPubsubEvent(updatedChat, codersdk.ChatWatchEventKindTitleChange, nil)
	return updatedChat, nil
}

func (p *Server) prepareManualTitleDebugRun(
	ctx context.Context,
	debugSvc *chatdebug.Service,
	chat database.Chat,
	modelConfig database.ChatModelConfig,
	modelOpts modelBuildOptions,
	messages []database.ChatMessage,
	fallbackModel fantasy.LanguageModel,
) (context.Context, fantasy.LanguageModel, func(error)) {
	titleCtx := ctx
	titleModel := fallbackModel
	finishDebugRun := func(error) {}

	route, routeErr := p.resolveModelRouteForConfig(ctx, chat.OwnerID, modelConfig)
	var routeProvider string
	if routeErr == nil {
		routeProvider = string(route.Provider.Type)
	} else if modelConfig.AIProviderID.Valid {
		// Route resolution failed, but the linked provider still identifies the
		// type for the debug run record. Best-effort: leave empty if disabled.
		if provider, err := p.enabledAIProviderByID(ctx, modelConfig.AIProviderID.UUID); err == nil {
			routeProvider = string(provider.Type)
		}
	}
	debugOpts := modelOpts
	debugOpts.RecordHTTP = true
	var debugModelErr error
	var debugModel fantasy.LanguageModel
	if routeErr != nil {
		debugModelErr = routeErr
	} else {
		debugModel, debugModelErr = p.newModel(ctx, modelClientRequest{
			Chat:          chat,
			ModelName:     modelConfig.Model,
			UserAgent:     chatprovider.UserAgent(),
			ExtraHeaders:  chatprovider.CoderHeaders(chat),
			ConfigOptions: modelConfig.Options,
		}, route, debugOpts)
	}
	switch {
	case debugModelErr != nil:
		p.logger.Warn(ctx, "failed to create debug-aware manual title model",
			slog.F("chat_id", chat.ID),
			slog.F("model", modelConfig.Model),
			slog.Error(debugModelErr),
		)
	case debugModel == nil:
		p.logger.Warn(ctx, "manual title debug model creation returned nil",
			slog.F("chat_id", chat.ID),
			slog.F("model", modelConfig.Model),
		)
	default:
		titleModel = chatdebug.WrapModel(debugModel, debugSvc, chatdebug.RecorderOptions{
			ChatID:   chat.ID,
			OwnerID:  chat.OwnerID,
			Provider: routeProvider,
			Model:    modelConfig.Model,
		})
	}

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
		Provider:            routeProvider,
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
		return titleCtx, titleModel, finishDebugRun
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

	return titleCtx, titleModel, finishDebugRun
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
) (fantasy.LanguageModel, database.ChatModelConfig, error) {
	overrideConfig, overrideModel, _, overrideSet, overrideErr := p.resolveTitleGenerationModelOverride(
		ctx,
		chat,
		modelOpts,
	)
	if overrideErr != nil {
		if overrideSet {
			return nil, database.ChatModelConfig{}, xerrors.Errorf(
				"resolve manual title generation model override: %w",
				overrideErr,
			)
		}
		p.logger.Debug(ctx, "failed to resolve title generation model override for manual title",
			slog.F("chat_id", chat.ID),
			slog.Error(overrideErr),
		)
	} else if overrideSet {
		return overrideModel, overrideConfig, nil
	}

	configs, err := store.GetEnabledChatModelConfigs(ctx)
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

	route, err := p.resolveModelRouteForConfig(ctx, chat.OwnerID, config)
	if err != nil {
		p.logger.Debug(ctx, "manual title preferred model unavailable",
			slog.F("chat_id", chat.ID),
			slog.F("model", config.Model),
			slog.Error(err),
		)
		return p.resolveFallbackManualTitleModel(ctx, chat, modelOpts)
	}
	model, err := p.newModel(ctx, modelClientRequest{
		Chat:          chat,
		ModelName:     config.Model,
		UserAgent:     chatprovider.UserAgent(),
		ExtraHeaders:  chatprovider.CoderHeaders(chat),
		ConfigOptions: config.Options,
	}, route, modelOpts)
	if err != nil {
		p.logger.Debug(ctx, "manual title preferred model unavailable",
			slog.F("chat_id", chat.ID),
			slog.F("model", config.Model),
			slog.Error(err),
		)
		return p.resolveFallbackManualTitleModel(ctx, chat, modelOpts)
	}

	return model, config, nil
}

func (p *Server) resolveFallbackManualTitleModel(
	ctx context.Context,
	chat database.Chat,
	modelOpts modelBuildOptions,
) (fantasy.LanguageModel, database.ChatModelConfig, error) {
	config, err := p.resolveModelConfig(ctx, chat)
	if err != nil {
		return nil, database.ChatModelConfig{}, xerrors.Errorf(
			"resolve fallback manual title model config: %w",
			err,
		)
	}
	route, err := p.resolveModelRouteForConfig(ctx, chat.OwnerID, config)
	if err != nil {
		return nil, database.ChatModelConfig{}, err
	}
	model, err := p.newModel(ctx, modelClientRequest{
		Chat:          chat,
		ModelName:     config.Model,
		UserAgent:     chatprovider.UserAgent(),
		ExtraHeaders:  chatprovider.CoderHeaders(chat),
		ConfigOptions: config.Options,
	}, route, modelOpts)
	if err != nil {
		return nil, database.ChatModelConfig{}, xerrors.Errorf(
			"create fallback manual title model: %w",
			err,
		)
	}
	return model, config, nil
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

// persistManualTitle writes newTitle only if the chat title still
// matches the caller's snapshot. The returned bool reports whether the
// title was actually written; it is false when a concurrent writer
// changed the title first or when newTitle matches the current title.
// Token usage for manual title generation is not recorded here; AI
// Gateway tracks it independently, and writing to chat_messages outside
// the chatstate state machine would break in-flight task fences.
func persistManualTitle(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	newTitle string,
) (database.Chat, bool, error) {
	updatedChat := chat
	wroteTitle := false
	err := store.InTx(func(tx database.Store) error {
		lockedChat, err := tx.GetChatByIDForUpdate(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("lock chat for manual title persist: %w", err)
		}
		updatedChat = lockedChat
		wroteTitle = false
		if lockedChat.Title == chat.Title && newTitle != lockedChat.Title {
			updatedChat, err = tx.UpdateChatByID(ctx, database.UpdateChatByIDParams{
				ID:    chat.ID,
				Title: newTitle,
			})
			if err != nil {
				return xerrors.Errorf("update chat title: %w", err)
			}
			wroteTitle = true
		}
		return nil
	}, nil)
	if err != nil {
		return database.Chat{}, false, err
	}
	return updatedChat, wroteTitle, nil
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
	totalCostMicros     int64
	runtimeMs           int64
}

type userChatMessage struct {
	chatMessage
	apiKeyID string
}

func (m userChatMessage) withCreatedBy(id uuid.UUID) userChatMessage {
	m.chatMessage = m.chatMessage.withCreatedBy(id)
	return m
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

func newUserChatMessage(
	apiKeyID string,
	content pqtype.NullRawMessage,
	visibility database.ChatMessageVisibility,
	modelConfigID uuid.UUID,
	contentVersion int16,
) userChatMessage {
	return userChatMessage{
		chatMessage: newChatMessage(
			database.ChatMessageRoleUser,
			content,
			visibility,
			modelConfigID,
			contentVersion,
		),
		apiKeyID: apiKeyID,
	}
}

func (m chatMessage) withCreatedBy(id uuid.UUID) chatMessage {
	m.createdBy = id
	return m
}

func appendMessageFields(
	params *database.InsertChatMessagesParams,
	msg chatMessage,
	apiKeyID string,
) {
	params.CreatedBy = append(params.CreatedBy, msg.createdBy)
	params.APIKeyID = append(params.APIKeyID, apiKeyID)
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
	params.TotalCostMicros = append(params.TotalCostMicros, msg.totalCostMicros)
	params.RuntimeMs = append(params.RuntimeMs, msg.runtimeMs)
}

func appendChatMessage(params *database.InsertChatMessagesParams, msg chatMessage) {
	if msg.role == database.ChatMessageRoleUser {
		panic("developer error: use appendUserChatMessage for user-role messages")
	}
	appendMessageFields(params, msg, "")
}

func appendUserChatMessage(params *database.InsertChatMessagesParams, msg userChatMessage) {
	appendMessageFields(params, msg.chatMessage, msg.apiKeyID)
}

// BuildSingleUserChatMessageInsertParams creates batch insert params for
// one user message, requiring an apiKeyID for AI Gateway attribution.
// BuildSingleChatMessageInsertParams creates batch insert params for one
// non-user message using the shared chat message builder.
func BuildSingleChatMessageInsertParams(
	chatID uuid.UUID,
	role database.ChatMessageRole,
	content pqtype.NullRawMessage,
	visibility database.ChatMessageVisibility,
	modelConfigID uuid.UUID,
	contentVersion int16,
	createdBy uuid.UUID,
) database.InsertChatMessagesParams {
	params := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
		ChatID: chatID,
	}
	msg := newChatMessage(role, content, visibility, modelConfigID, contentVersion)
	if createdBy != uuid.Nil {
		msg = msg.withCreatedBy(createdBy)
	}
	if role == database.ChatMessageRoleUser {
		appendMessageFields(&params, msg, "")
	} else {
		appendChatMessage(&params, msg)
	}
	return params
}

func BuildSingleUserChatMessageInsertParams(
	chatID uuid.UUID,
	apiKeyID string,
	content pqtype.NullRawMessage,
	visibility database.ChatMessageVisibility,
	modelConfigID uuid.UUID,
	contentVersion int16,
	createdBy uuid.UUID,
) database.InsertChatMessagesParams {
	params := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendUserChatMessage.
		ChatID: chatID,
	}
	msg := newUserChatMessage(apiKeyID, content, visibility, modelConfigID, contentVersion)
	if createdBy != uuid.Nil {
		msg = msg.withCreatedBy(createdBy)
	}
	appendUserChatMessage(&params, msg)
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
	UsageTracker                   *workspacestats.UsageTracker
	Clock                          quartz.Clock
	AIBridgeTransportFactory       *atomic.Pointer[aibridge.TransportFactory]
	Experiments                    codersdk.Experiments

	PrometheusRegistry prometheus.Registerer

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
	p := &Server{
		cancel:                         cancel,
		db:                             cfg.Database,
		workerID:                       workerID,
		logger:                         cfg.Logger.Named("processor"),
		agentConnFn:                    cfg.AgentConn,
		agentInactiveDisconnectTimeout: cfg.AgentInactiveDisconnectTimeout,
		dialTimeout:                    defaultDialTimeout,
		instructionLookupTimeout:       instructionLookupTimeout,
		createWorkspaceFn:              cfg.CreateWorkspace,
		startWorkspaceFn:               cfg.StartWorkspace,
		stopWorkspaceFn:                cfg.StopWorkspace,
		pubsub:                         ps,
		webpushDispatcher:              cfg.WebpushDispatcher,
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
	chatWorker, err := newChatWorker(p, chatWorkerOptions{
		WorkerID:              workerID,
		Store:                 cfg.Database,
		Pubsub:                ps,
		Logger:                cfg.Logger.Named("chatworker"),
		Clock:                 clk,
		MessagePartBuffer:     p.messagePartBuffer,
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
			case coderdpubsub.ChatConfigEventModelConfig:
				p.configCache.InvalidateModelConfig(ev.EntityID)
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
	if p.pubsub == nil {
		return
	}
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
	FinalAssistantText  string
	StatusLabelModel    fantasy.LanguageModel
	FallbackProvider    string
	FallbackRoute       aiGatewayModelRoute
	FallbackModel       string
	ModelBuildOptions   modelBuildOptions
	TriggerMessageID    int64
	HistoryTipMessageID int64
}

func allToolNames(allTools []fantasy.AgentTool) []string {
	toolNames := make([]string, 0, len(allTools))
	for _, tool := range allTools {
		toolNames = append(toolNames, tool.Info().Name)
	}
	return toolNames
}

func isExploreSubagentMode(mode database.NullChatMode) bool {
	return mode.Valid && mode.ChatMode == database.ChatModeExplore
}

// filterExternalMCPConfigsForTurn returns the external MCP server configs
// visible on the current turn. Explore children snapshot this filtered set at
// spawn time so later model overrides cannot widen the external-tool boundary.
func filterExternalMCPConfigsForTurn(
	configs []database.MCPServerConfig,
	mode database.NullChatPlanMode,
	parentChatID uuid.NullUUID,
) ([]database.MCPServerConfig, map[uuid.UUID]struct{}) {
	if !mode.Valid || mode.ChatPlanMode != database.ChatPlanModePlan {
		return configs, nil
	}
	if parentChatID.Valid {
		// Plan-mode subagents do not receive external MCP tools because
		// their trust boundary is narrower than the root chat's.
		return nil, map[uuid.UUID]struct{}{}
	}

	filtered := make([]database.MCPServerConfig, 0, len(configs))
	approvedIDs := make(map[uuid.UUID]struct{})
	for _, cfg := range configs {
		if !cfg.AllowInPlanMode {
			continue
		}
		filtered = append(filtered, cfg)
		approvedIDs[cfg.ID] = struct{}{}
	}
	return filtered, approvedIDs
}

func builtinPlanToolAllowed(name string, isRootChat bool) bool {
	switch name {
	case "read_file", "execute", "process_output", "read_skill", "read_skill_file":
		return true
	case "write_file", "edit_files", "list_templates", "read_template",
		"create_workspace", "start_workspace", "stop_workspace", "propose_plan", "spawn_agent",
		"spawn_explore_agent", "wait_agent", "list_agents", "ask_user_question", "attach_file":
		return isRootChat
	case "process_list", "process_signal", "message_agent", "interrupt_agent", "close_agent",
		"spawn_computer_use_agent":
		return false
	default:
		return false
	}
}

func toolAllowedForTurn(
	tool fantasy.AgentTool,
	mode database.NullChatPlanMode,
	parentChatID uuid.NullUUID,
	approvedMCPConfigIDs map[uuid.UUID]struct{},
) bool {
	if !mode.Valid || mode.ChatPlanMode != database.ChatPlanModePlan {
		return true
	}
	if builtinPlanToolAllowed(tool.Info().Name, !parentChatID.Valid) {
		return true
	}
	mcpTool, ok := tool.(mcpclient.MCPToolIdentifier)
	if !ok {
		return false
	}
	_, approved := approvedMCPConfigIDs[mcpTool.MCPServerConfigID()]
	return approved
}

func filterToolsForTurn(
	allTools []fantasy.AgentTool,
	mode database.NullChatPlanMode,
	parentChatID uuid.NullUUID,
	approvedMCPConfigIDs map[uuid.UUID]struct{},
) []fantasy.AgentTool {
	if !mode.Valid || mode.ChatPlanMode != database.ChatPlanModePlan {
		return allTools
	}

	filtered := make([]fantasy.AgentTool, 0, len(allTools))
	for _, tool := range allTools {
		if toolAllowedForTurn(tool, mode, parentChatID, approvedMCPConfigIDs) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

// activeToolNamesForTurn extends the built-in plan allowlist with approved
// external MCP tools for root plan-mode chats.
func activeToolNamesForTurn(
	allTools []fantasy.AgentTool,
	mode database.NullChatPlanMode,
	parentChatID uuid.NullUUID,
	approvedMCPConfigIDs map[uuid.UUID]struct{},
) []string {
	toolNames := make([]string, 0, len(allTools))
	for _, tool := range allTools {
		if toolAllowedForTurn(tool, mode, parentChatID, approvedMCPConfigIDs) {
			toolNames = append(toolNames, tool.Info().Name)
		}
	}
	return toolNames
}

func allowedExploreToolNames(allTools []fantasy.AgentTool) []string {
	builtinExplorePolicy := map[string]bool{
		"read_file":         true,
		"write_file":        false,
		"edit_files":        false,
		"execute":           true,
		"process_output":    true,
		"process_list":      false,
		"process_signal":    false,
		"list_templates":    false,
		"read_template":     false,
		"create_workspace":  false,
		"start_workspace":   false,
		"stop_workspace":    false,
		"propose_plan":      false,
		"spawn_agent":       false,
		"wait_agent":        false,
		"message_agent":     false,
		"interrupt_agent":   false,
		"close_agent":       false,
		"list_agents":       false,
		"read_skill":        true,
		"read_skill_file":   true,
		"ask_user_question": false,
	}

	toolNames := make([]string, 0, len(allTools))
	for _, tool := range allTools {
		name := tool.Info().Name
		if builtinExplorePolicy[name] {
			toolNames = append(toolNames, name)
			continue
		}
		// External MCP tools pass through here. They were snapshot-filtered
		// at spawn time on chat.MCPServerIDs. WorkspaceMCPTool does not
		// implement MCPToolIdentifier, so workspace tools are excluded
		// here too, in addition to the structural exclusion in runChat
		// tool assembly.
		if _, ok := tool.(mcpclient.MCPToolIdentifier); ok {
			toolNames = append(toolNames, name)
		}
	}
	return toolNames
}

// allowedBehaviorToolNames runs only on non-plan turns because
// appendDynamicTools returns early for plan mode. Within that boundary,
// Explore mode wins over the default behavior that allows all tools.
func allowedBehaviorToolNames(
	allTools []fantasy.AgentTool,
	chatMode database.NullChatMode,
) []string {
	if isExploreSubagentMode(chatMode) {
		return allowedExploreToolNames(allTools)
	}
	return allToolNames(allTools)
}

func stopAfterPlanTools(
	planMode database.NullChatPlanMode,
	parentChatID uuid.NullUUID,
) map[string]struct{} {
	if !planMode.Valid || planMode.ChatPlanMode != database.ChatPlanModePlan {
		return nil
	}
	stopTools := map[string]struct{}{
		"propose_plan": {},
	}
	if !parentChatID.Valid {
		stopTools["ask_user_question"] = struct{}{}
	}
	return stopTools
}

func stopAfterBehaviorTools(
	planMode database.NullChatPlanMode,
	chatMode database.NullChatMode,
	parentChatID uuid.NullUUID,
) map[string]struct{} {
	if isExploreSubagentMode(chatMode) {
		return nil
	}
	return stopAfterPlanTools(planMode, parentChatID)
}

type systemPromptBehaviorContext struct {
	planMode             database.NullChatPlanMode
	chatMode             database.NullChatMode
	planModeInstructions string
	isRootChat           bool
}

func workspaceSkillsForResolution(workspaceSkills []chattool.SkillMeta) []skillspkg.Skill {
	if len(workspaceSkills) == 0 {
		return nil
	}
	resolved := make([]skillspkg.Skill, 0, len(workspaceSkills))
	for _, skill := range workspaceSkills {
		resolved = append(resolved, skillspkg.Skill{
			Name:        skill.Name,
			Description: skill.Description,
			Source:      skillspkg.SourceWorkspace,
		})
	}
	return resolved
}

func mergeTurnSkills(
	personalSkills []skillspkg.Skill,
	workspaceSkills []chattool.SkillMeta,
) []skillspkg.ResolvedSkill {
	return skillspkg.MergeSkills(
		personalSkills,
		workspaceSkillsForResolution(workspaceSkills),
	)
}

// buildSystemPrompt applies system-level prompt injections in a fixed
// order: subagent instruction, chat instruction, skill index, user prompt,
// then mode overlay prompts.
func buildSystemPrompt(
	prompt []fantasy.Message,
	subagentInstruction string,
	instruction string,
	resolvedSkills []skillspkg.ResolvedSkill,
	userPrompt string,
	behaviorContext systemPromptBehaviorContext,
) []fantasy.Message {
	if subagentInstruction != "" {
		prompt = chatprompt.InsertSystem(prompt, subagentInstruction)
	}
	if instruction != "" {
		prompt = chatprompt.InsertSystem(prompt, instruction)
	}
	if skillIndex := chattool.FormatResolvedSkillIndex(resolvedSkills); skillIndex != "" {
		prompt = chatprompt.InsertSystem(prompt, skillIndex)
	}
	if userPrompt != "" {
		prompt = chatprompt.InsertSystem(prompt, userPrompt)
	}
	if isExploreSubagentMode(behaviorContext.chatMode) {
		prompt = chatprompt.InsertSystem(prompt, ExploreSubagentOverlayPrompt)
		return prompt
	}
	isPlanModeTurn := behaviorContext.planMode.Valid && behaviorContext.planMode.ChatPlanMode == database.ChatPlanModePlan
	if isPlanModeTurn {
		if behaviorContext.isRootChat {
			prompt = chatprompt.InsertSystem(prompt, PlanningOverlayPrompt())
			if behaviorContext.planModeInstructions != "" {
				prompt = chatprompt.InsertSystem(prompt, behaviorContext.planModeInstructions)
			}
		} else {
			prompt = chatprompt.InsertSystem(prompt, PlanningSubagentOverlayPrompt)
		}
	}
	return prompt
}

func removeSkillIndexMessages(prompt []fantasy.Message) []fantasy.Message {
	out := make([]fantasy.Message, 0, len(prompt))
	removed := false
	for _, message := range prompt {
		if isSkillIndexMessage(message) {
			removed = true
			continue
		}
		out = append(out, message)
	}
	if !removed {
		return prompt
	}
	return out
}

func isSkillIndexMessage(message fantasy.Message) bool {
	if message.Role != fantasy.MessageRoleSystem || len(message.Content) != 1 {
		return false
	}
	textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](message.Content[0])
	if !ok {
		return false
	}
	text := strings.TrimSpace(textPart.Text)
	return strings.HasPrefix(text, chattool.AvailableSkillsOpenTag+"\n") && strings.HasSuffix(text, chattool.AvailableSkillsCloseTag)
}

type rootChatToolsOptions struct {
	chat            database.Chat
	modelConfigID   uuid.UUID
	workspaceCtx    *turnWorkspaceContext
	workspaceMu     *sync.Mutex
	resolvePlanPath func(context.Context) (string, string, error)
	storeFile       chattool.StoreFileFunc
	isPlanModeTurn  bool
}

func (p *Server) loadPlanModeInstructions(
	ctx context.Context,
	mode database.NullChatPlanMode,
	logger slog.Logger,
) string {
	if !mode.Valid || mode.ChatPlanMode != database.ChatPlanModePlan {
		return ""
	}

	// Plan-mode instructions live in deployment config, but chat workers do
	// not carry a deployment-config actor during background execution.
	//nolint:gocritic // Required to read deployment config during background chat processing.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	fetched, err := p.db.GetChatPlanModeInstructions(systemCtx)
	if err != nil {
		logger.Warn(ctx,
			"failed to fetch plan mode instructions",
			slog.Error(err),
		)
		return ""
	}

	return fetched
}

func userSkillContext(ctx context.Context, userID uuid.UUID) context.Context {
	actor := rbac.Subject{
		Type:  rbac.SubjectTypeUser,
		ID:    userID.String(),
		Roles: rbac.RoleIdentifiers{rbac.RoleMember()},
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()
	// Chat turns run asynchronously after admission, so the original request
	// actor may no longer be available when a worker loads personal skills.
	// We synthesize the chat owner as a member instead of reusing that actor.
	// Hardcoding RoleMember is safe because dbauthz enforces
	// ResourceUserSkill.WithOwner(userID), so this actor cannot read any other
	// user's skills regardless of role. Org scoping is not needed because
	// personal skills are user-scoped, not org-scoped.
	//nolint:gocritic // The synthetic actor is intentional for the reasons above.
	return dbauthz.As(ctx, actor)
}

func (p *Server) fetchPersonalSkillMetadata(
	ctx context.Context,
	userID uuid.UUID,
	logger slog.Logger,
) []skillspkg.Skill {
	rows, err := p.db.ListUserSkillMetadataByUserID(userSkillContext(ctx, userID), userID)
	// See package coderd/x/skills (doc.go) for why metadata fetch failures
	// intentionally degrade to an empty personal-skill list instead of
	// failing the chat turn.
	if err != nil {
		logger.Warn(ctx, "failed to load personal skill metadata",
			slog.F("owner_id", userID),
			slog.Error(err),
		)
		return nil
	}

	personalSkills := make([]skillspkg.Skill, 0, len(rows))
	for _, row := range rows {
		personalSkills = append(personalSkills, skillspkg.Skill{
			Name:        row.Name,
			Description: row.Description,
			Source:      skillspkg.SourcePersonal,
		})
	}
	return personalSkills
}

func (p *Server) loadPersonalSkillBody(
	ctx context.Context,
	userID uuid.UUID,
	name string,
) (skillspkg.ParsedSkill, error) {
	row, err := p.db.GetUserSkillByUserIDAndName(
		userSkillContext(ctx, userID),
		database.GetUserSkillByUserIDAndNameParams{
			UserID: userID,
			Name:   name,
		},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return skillspkg.ParsedSkill{}, skillspkg.ErrSkillNotFound
		}
		p.logger.Error(ctx, "load personal skill body failed",
			slog.F("user_id", userID),
			slog.F("name", name),
			slog.Error(err),
		)
		return skillspkg.ParsedSkill{}, xerrors.Errorf("load personal skill body: %w", err)
	}

	parsed, err := skillspkg.ParsePersonalSkillMarkdown([]byte(row.Content))
	if err != nil {
		p.logger.Error(ctx, "parse personal skill body failed",
			slog.F("user_id", userID),
			slog.F("name", name),
			slog.Error(err),
		)
		return skillspkg.ParsedSkill{}, xerrors.Errorf("parse personal skill body: %w", err)
	}
	return parsed, nil
}

func (p *Server) appendRootChatTools(
	ctx context.Context,
	tools []fantasy.AgentTool,
	opts rootChatToolsOptions,
) []fantasy.AgentTool {
	onChatUpdated := func(updatedChat database.Chat) {
		opts.workspaceCtx.selectWorkspace(updatedChat)
		// Notify the frontend immediately so it can start streaming
		// build logs before the tool completes.
		p.publishChatPubsubEvent(updatedChat, codersdk.ChatWatchEventKindStatusChange, nil)
	}

	tools = append(tools,
		chattool.ListTemplates(p.db, opts.chat.OrganizationID, chattool.ListTemplatesOptions{
			OwnerID:            opts.chat.OwnerID,
			Logger:             p.logger,
			Clock:              p.clock,
			AllowedTemplateIDs: p.chatTemplateAllowlist,
		}),
		chattool.ReadTemplate(p.db, opts.chat.OrganizationID, chattool.ReadTemplateOptions{
			OwnerID:            opts.chat.OwnerID,
			AllowedTemplateIDs: p.chatTemplateAllowlist,
		}),
		chattool.CreateWorkspace(p.db, opts.chat.OrganizationID, opts.chat.ID, chattool.CreateWorkspaceOptions{
			OwnerID:                        opts.chat.OwnerID,
			CreateFn:                       p.createWorkspaceFn,
			AgentConnFn:                    chattool.AgentConnFunc(p.agentConnFn),
			AgentInactiveDisconnectTimeout: p.agentInactiveDisconnectTimeout,
			WorkspaceMu:                    opts.workspaceMu,
			OnChatUpdated:                  onChatUpdated,
			Logger:                         p.logger,
			AllowedTemplateIDs:             p.chatTemplateAllowlist,
		}),
		chattool.StartWorkspace(p.db, opts.chat.ID, chattool.StartWorkspaceOptions{
			OwnerID:       opts.chat.OwnerID,
			StartFn:       p.startWorkspaceFn,
			AgentConnFn:   chattool.AgentConnFunc(p.agentConnFn),
			WorkspaceMu:   opts.workspaceMu,
			OnChatUpdated: onChatUpdated,
			Logger:        p.logger,
		}),
		chattool.StopWorkspace(p.db, opts.chat.ID, chattool.StopWorkspaceOptions{
			OwnerID:       opts.chat.OwnerID,
			StopFn:        p.stopWorkspaceFn,
			WorkspaceMu:   opts.workspaceMu,
			OnChatUpdated: onChatUpdated,
			Logger:        p.logger,
		}),
	)
	if opts.isPlanModeTurn {
		tools = append(tools, chattool.ProposePlan(chattool.ProposePlanOptions{
			GetWorkspaceConn: opts.workspaceCtx.getWorkspaceConn,
			ResolvePlanPath:  opts.resolvePlanPath,
			IsPlanTurn:       opts.isPlanModeTurn,
			StoreFile:        opts.storeFile,
		}))
	}

	return append(tools, p.subagentTools(ctx, func() database.Chat {
		return opts.chat
	}, opts.modelConfigID)...)
}

func appendDynamicTools(
	ctx context.Context,
	logger slog.Logger,
	tools []fantasy.AgentTool,
	raw pqtype.NullRawMessage,
	planMode database.NullChatPlanMode,
	chatMode database.NullChatMode,
) ([]fantasy.AgentTool, map[string]bool, error) {
	if isExploreSubagentMode(chatMode) || (planMode.Valid && planMode.ChatPlanMode == database.ChatPlanModePlan) {
		return tools, nil, nil
	}

	dynamicToolNames, err := parseDynamicToolNames(raw)
	if err != nil {
		return nil, nil, xerrors.Errorf("parse dynamic tool names: %w", err)
	}
	if len(dynamicToolNames) == 0 {
		return tools, dynamicToolNames, nil
	}

	var dynamicToolDefs []codersdk.DynamicTool
	if raw.Valid {
		if err := json.Unmarshal(raw.RawMessage, &dynamicToolDefs); err != nil {
			return nil, nil, xerrors.Errorf("unmarshal dynamic tools: %w", err)
		}
	}

	activeToolNames := make(map[string]struct{}, len(tools))
	for _, name := range allowedBehaviorToolNames(tools, chatMode) {
		activeToolNames[name] = struct{}{}
	}
	for _, t := range tools {
		info := t.Info()
		if _, active := activeToolNames[info.Name]; !active {
			continue
		}
		if dynamicToolNames[info.Name] {
			logger.Warn(ctx, "dynamic tool name collides with built-in tool, built-in takes precedence",
				slog.F("tool_name", info.Name))
			delete(dynamicToolNames, info.Name)
		}
	}

	var filteredDefs []codersdk.DynamicTool
	for _, dt := range dynamicToolDefs {
		if dynamicToolNames[dt.Name] {
			filteredDefs = append(filteredDefs, dt)
		}
	}

	return append(tools, dynamicToolsFromSDK(logger, filteredDefs)...), dynamicToolNames, nil
}

// buildProviderTools creates provider-native tool definitions
// (like web search) based on the model configuration. These
// tools are executed server-side by the LLM provider.
func buildProviderTools(options *codersdk.ChatModelProviderOptions) []chatloop.ProviderTool {
	var tools []chatloop.ProviderTool

	if options == nil {
		return nil
	}

	if options.Anthropic != nil && options.Anthropic.WebSearchEnabled != nil && *options.Anthropic.WebSearchEnabled {
		tools = append(tools, chatloop.ProviderTool{
			Definition: anthropic.WebSearchTool(&anthropic.WebSearchToolOptions{
				AllowedDomains: options.Anthropic.AllowedDomains,
				BlockedDomains: options.Anthropic.BlockedDomains,
			}),
		})
	}

	if tool, ok := chatopenai.WebSearchTool(options.OpenAI); ok {
		tools = append(tools, chatloop.ProviderTool{
			Definition: tool,
		})
	}

	if options.Google != nil && options.Google.WebSearchEnabled != nil && *options.Google.WebSearchEnabled {
		tools = append(tools, chatloop.ProviderTool{
			Definition: fantasy.ProviderDefinedTool{
				ID:   "web_search",
				Name: "web_search",
			},
		})
	}

	return tools
}

func (p *Server) resolveChatModel(
	ctx context.Context,
	chat database.Chat,
	modelOpts modelBuildOptions,
) (
	model fantasy.LanguageModel,
	dbConfig database.ChatModelConfig,
	route aiGatewayModelRoute,
	debugEnabled bool,
	resolvedProvider string,
	resolvedModel string,
	err error,
) {
	dbConfig, err = p.resolveModelConfig(ctx, chat)
	if err != nil {
		return nil, database.ChatModelConfig{}, aiGatewayModelRoute{}, false, "", "", xerrors.Errorf("resolve model config: %w", err)
	}

	if !dbConfig.Enabled {
		return nil, database.ChatModelConfig{}, aiGatewayModelRoute{}, false, "", "", xerrors.Errorf("chat model config %s is disabled", dbConfig.ID)
	}

	route, err = p.resolveModelRouteForConfig(ctx, chat.OwnerID, dbConfig)
	if err != nil {
		return nil, database.ChatModelConfig{}, aiGatewayModelRoute{}, false, "", "", err
	}

	providerHint := route.ModelProviderHint
	resolvedProvider, resolvedModel, err = chatprovider.ResolveModelWithProviderHint(
		dbConfig.Model,
		providerHint,
	)
	if err != nil {
		return nil, database.ChatModelConfig{}, aiGatewayModelRoute{}, false, "", "", xerrors.Errorf(
			"resolve model metadata: %w", err,
		)
	}

	model, debugEnabled, err = p.newDebugAwareModel(ctx, modelClientRequest{
		Chat:          chat,
		ModelName:     dbConfig.Model,
		UserAgent:     chatprovider.UserAgent(),
		ExtraHeaders:  chatprovider.CoderHeaders(chat),
		ConfigOptions: dbConfig.Options,
	}, route, modelOpts)
	if err != nil {
		return nil, database.ChatModelConfig{}, aiGatewayModelRoute{}, false, "", "", xerrors.Errorf(
			"create model: %w", err,
		)
	}
	return model, dbConfig, route, debugEnabled, resolvedProvider, resolvedModel, nil
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

// resolveModelConfig looks up the chat's model config by its
// LastModelConfigID. If the referenced config no longer exists
// (e.g. it was deleted), it falls back to the default model
// config. Returns an error when no usable config is available.
func (p *Server) resolveModelConfig(
	ctx context.Context,
	chat database.Chat,
) (database.ChatModelConfig, error) {
	if chat.LastModelConfigID != uuid.Nil {
		modelConfig, err := p.configCache.ModelConfigByID(
			ctx, chat.LastModelConfigID,
		)
		if err == nil {
			return modelConfig, nil
		}
		if !xerrors.Is(err, sql.ErrNoRows) {
			return database.ChatModelConfig{}, xerrors.Errorf(
				"get chat model config %s: %w",
				chat.LastModelConfigID, err,
			)
		}
		// Model config was deleted, fall through to default.
	}

	defaultConfig, err := p.configCache.DefaultModelConfig(ctx)
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

	sanitizedCustom := SanitizePromptText(config.ChatSystemPrompt)
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
		return
	}

	switch status {
	case database.ChatStatusWaiting:
		p.finalizeSuccessfulTurnStatusLabelAndPush(ctx, chat, status, runResult, logger)

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
	if assistantText == "" || runResult.StatusLabelModel == nil {
		return fallbackTurnStatusLabel(status)
	}

	statusLabel := p.generateTurnStatusLabel(
		ctx,
		chat,
		status,
		assistantText,
		runResult.FallbackProvider,
		runResult.FallbackModel,
		runResult.StatusLabelModel,
		runResult.FallbackRoute,
		runResult.ModelBuildOptions,
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

func (p *Server) maybeClearLastTurnSummaryAsync(
	ctx context.Context,
	chat database.Chat,
	logger slog.Logger,
) {
	if chat.ParentChatID.Valid {
		return
	}
	p.clearLastTurnSummaryAsync(ctx, chat, logger)
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

	//nolint:gocritic // Chatd needs system-level write access to
	// persist the refreshed OAuth2 token for the user.
	updated, err := p.db.UpsertMCPServerUserToken(
		dbauthz.AsSystemRestricted(ctx),
		database.UpsertMCPServerUserTokenParams{
			MCPServerConfigID: tok.MCPServerConfigID,
			UserID:            tok.UserID,
			AccessToken:       result.AccessToken,
			AccessTokenKeyID:  sql.NullString{},
			RefreshToken:      result.RefreshToken,
			RefreshTokenKeyID: sql.NullString{},
			TokenType:         result.TokenType,
			Expiry:            expiry,
		},
	)
	if err != nil {
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

	//nolint:gocritic // Chatd needs system-level write access to
	// persist the refresh failure for the user.
	marked, err := p.db.MarkMCPServerUserTokenRefreshFailure(
		dbauthz.AsSystemRestricted(ctx),
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
		//nolint:gocritic // Chatd needs system-level read access to
		// load the concurrently updated token.
		current, readErr := p.db.GetMCPServerUserToken(
			dbauthz.AsSystemRestricted(ctx),
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
