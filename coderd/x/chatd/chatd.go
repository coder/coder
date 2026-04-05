package chatd

import (
	"bytes"
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
	"time"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/xjson"
	"github.com/coder/coder/v2/coderd/webpush"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/coderd/x/chatd/chatcost"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/internal/agentselect"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
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
	instructionCacheTTL          = 5 * time.Minute
	workspaceDialValidationDelay = 5 * time.Second
	workspaceMCPDiscoveryTimeout = 5 * time.Second
	// DefaultChatHeartbeatInterval is the default time between chat
	// heartbeat updates while a chat is being processed.
	DefaultChatHeartbeatInterval = 30 * time.Second
	maxChatSteps                 = 1200
	// maxStreamBufferSize caps the number of message_part events buffered
	// per chat during a single LLM step. When exceeded the oldest event is
	// evicted so memory stays bounded.
	maxStreamBufferSize = 10000
	// maxDurableMessageCacheSize caps the number of recent durable message
	// events cached per chat for same-replica stream catch-up.
	maxDurableMessageCacheSize = 256

	// maxConcurrentRecordingUploads caps the number of recording
	// stop-and-store operations that can run concurrently. Each
	// slot buffers up to MaxRecordingSize (100 MB) in memory, so
	// this value implicitly bounds memory to roughly
	// maxConcurrentRecordingUploads * 100 MB.
	maxConcurrentRecordingUploads = 25

	// staleRecoveryIntervalDivisor determines how often the stale
	// recovery loop runs relative to the stale threshold. A value
	// of 5 means recovery runs at 1/5 of the stale-after duration.
	staleRecoveryIntervalDivisor = 5

	// streamDropWarnInterval controls how often WARN-level logs are
	// emitted when stream events are dropped. Between intervals the
	// drop is logged at DEBUG to avoid log spam. This uses a
	// timestamp comparison rather than a quartz.Ticker because the
	// state is per-chat — a ticker per chat would require extra
	// goroutines and lifecycle management.
	streamDropWarnInterval = 10 * time.Second

	// bufferRetainGracePeriod is how long the message_part
	// buffer is kept after processing completes. This gives
	// cross-replica relay subscribers time to connect and
	// snapshot the buffer before it is garbage-collected.
	bufferRetainGracePeriod = 5 * time.Second

	// DefaultMaxChatsPerAcquire is the maximum number of chats to
	// acquire in a single processOnce call. Batching avoids
	// waiting a full polling interval between acquisitions
	// when many chats are pending.
	DefaultMaxChatsPerAcquire int32 = 10

	defaultSubagentInstruction = "You are running as a delegated sub-agent chat. Complete the delegated task and provide clear, concise assistant responses for the parent agent."
)

var errChatHasNoWorkspaceAgent = xerrors.New("workspace has no running agent: the workspace is likely stopped. Use the start_workspace tool to start it")

// Server handles background processing of pending chats.
type Server struct {
	cancel     context.CancelFunc
	closed     chan struct{}
	inflight   sync.WaitGroup
	inflightMu sync.Mutex

	db       database.Store
	workerID uuid.UUID
	logger   slog.Logger

	subscribeFn SubscribeFn

	agentConnFn                    AgentConnFunc
	agentInactiveDisconnectTimeout time.Duration
	instructionLookupTimeout       time.Duration
	createWorkspaceFn              chattool.CreateWorkspaceFn
	startWorkspaceFn               chattool.StartWorkspaceFn
	pubsub                         pubsub.Pubsub
	webpushDispatcher              webpush.Dispatcher
	providerAPIKeys                chatprovider.ProviderAPIKeys
	configCache                    *chatConfigCache
	configCacheUnsubscribe         func()

	// chatStreams stores per-chat stream state. Using sync.Map
	// gives each chat independent locking — concurrent chats
	// never contend with each other.
	chatStreams sync.Map // uuid.UUID -> *chatStreamState

	// workspaceMCPToolsCache caches workspace MCP tool definitions
	// per chat to avoid re-fetching on every turn. The cache is
	// keyed by chat ID and invalidated when the agent changes.
	workspaceMCPToolsCache sync.Map // uuid.UUID -> *cachedWorkspaceMCPTools

	usageTracker *workspacestats.UsageTracker
	clock        quartz.Clock
	recordingSem chan struct{}

	// Configuration
	pendingChatAcquireInterval time.Duration
	maxChatsPerAcquire         int32
	inFlightChatStaleAfter     time.Duration
	chatHeartbeatInterval      time.Duration

	// wakeCh is signaled by SendMessage, EditMessage, CreateChat,
	// and PromoteQueued so the run loop calls processOnce
	// immediately instead of waiting for the next ticker.
	wakeCh chan struct{}
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

// cachedWorkspaceMCPTools stores workspace MCP tools discovered
// from a workspace agent, keyed by the agent ID that provided them.
type cachedWorkspaceMCPTools struct {
	agentID uuid.UUID
	tools   []workspacesdk.MCPToolInfo
}

// loadCachedWorkspaceContext checks the MCP tools cache for the
// given chat and agent. Returns non-nil tools when the cache hits,
// which signals the caller to skip the slow MCP discovery path.
func (p *Server) loadCachedWorkspaceContext(
	chatID uuid.UUID,
	agent database.WorkspaceAgent,
	getConn func(context.Context) (workspacesdk.AgentConn, error),
) []fantasy.AgentTool {
	cached, ok := p.workspaceMCPToolsCache.Load(chatID)
	if !ok {
		return nil
	}
	entry, ok := cached.(*cachedWorkspaceMCPTools)
	if !ok || entry.agentID != agent.ID {
		return nil
	}

	var tools []fantasy.AgentTool
	for _, t := range entry.tools {
		tools = append(tools, chattool.NewWorkspaceMCPTool(t, getConn))
	}

	return tools
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

func (c *turnWorkspaceContext) getWorkspaceConn(ctx context.Context) (workspacesdk.AgentConn, error) {
	if c.server.agentConnFn == nil {
		return nil, xerrors.New("workspace agent connector is not configured")
	}

	for attempt := 0; attempt < 2; attempt++ {
		c.mu.Lock()
		currentConn, staleRelease := c.getWorkspaceConnLocked()
		c.mu.Unlock()
		if currentConn != nil {
			return currentConn, nil
		}
		if staleRelease != nil {
			staleRelease()
		}

		chatSnapshot, agent, err := c.ensureWorkspaceAgent(ctx)
		if err != nil {
			return nil, err
		}

		dialResult, err := dialWithLazyValidation(
			ctx,
			agent.ID,
			chatSnapshot.WorkspaceID.UUID,
			DialFunc(c.server.agentConnFn),
			func(ctx context.Context, workspaceID uuid.UUID) (uuid.UUID, error) {
				return c.latestWorkspaceAgentID(ctx, workspaceID)
			},
			workspaceDialValidationDelay,
		)
		if err != nil {
			if xerrors.Is(err, errChatHasNoWorkspaceAgent) {
				c.clearCachedWorkspaceState()
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
			return agentConn, nil
		}
		currentConn = c.conn
		c.mu.Unlock()

		if agentRelease != nil {
			agentRelease()
		}
		return currentConn, nil
	}

	return nil, xerrors.New("chat workspace changed while connecting")
}

// AgentConnFunc provides access to workspace agent connections.
type AgentConnFunc func(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error)

// SubscribeFn replaces the default local-only subscription with a
// multi-replica-aware implementation that merges pubsub notifications,
// remote relay streams, and local parts into a single event channel.
// When set, Subscribe delegates the event-merge goroutine to this
// function instead of using simple local forwarding.
//
// Parameters:
//   - ctx: subscription lifetime context (canceled on unsubscribe).
//   - params: all state needed to build the merged stream.
//
// Returns the merged event channel. Cleanup is driven by ctx
// cancellation — the merge goroutine tears down all relay state
// in its defer when ctx is done.
// Set by enterprise for HA deployments. Nil in AGPL single-replica.
type SubscribeFn func(
	ctx context.Context,
	params SubscribeFnParams,
) <-chan codersdk.ChatStreamEvent

// StatusNotification informs the enterprise relay manager of chat
// status changes so it can open or close relay connections.
type StatusNotification struct {
	Status   database.ChatStatus
	WorkerID uuid.UUID
}

// SubscribeFnParams carries the state that the enterprise
// SubscribeFn implementation needs from the OSS Subscribe preamble.
type SubscribeFnParams struct {
	ChatID              uuid.UUID
	Chat                database.Chat
	WorkerID            uuid.UUID
	StatusNotifications <-chan StatusNotification
	RequestHeader       http.Header
	DB                  database.Store
	Logger              slog.Logger
}

type chatStreamState struct {
	mu                   sync.Mutex
	buffer               []codersdk.ChatStreamEvent
	buffering            bool
	durableMessages      []codersdk.ChatStreamEvent
	durableEvictedBefore int64 // highest message ID evicted from durable cache
	subscribers          map[uuid.UUID]chan codersdk.ChatStreamEvent
	bufferDropCount      int64
	bufferLastWarnAt     time.Time
	subscriberDropCount  int64
	subscriberLastWarnAt time.Time
	// bufferRetainedAt records when processing completed and
	// the buffer was retained for late-connecting relay
	// subscribers. Zero while buffering is active. When
	// non-zero, cleanupStreamIfIdle skips GC until the grace
	// period expires so cross-replica relays can still
	// snapshot the buffer.
	bufferRetainedAt time.Time
}

// resetDropCounters zeroes the rate-limiting state for both buffer
// and subscriber drop warnings. The caller must hold s.mu.
func (s *chatStreamState) resetDropCounters() {
	s.bufferDropCount = 0
	s.bufferLastWarnAt = time.Time{}
	s.subscriberDropCount = 0
	s.subscriberLastWarnAt = time.Time{}
}

// MaxQueueSize is the maximum number of queued user messages per chat.
const MaxQueueSize = 20

var (
	// ErrMessageQueueFull indicates the per-chat queue limit was reached.
	ErrMessageQueueFull = xerrors.New("chat message queue is full")
	// ErrEditedMessageNotFound indicates the edited message does not exist
	// in the target chat.
	ErrEditedMessageNotFound = xerrors.New("edited message not found")
	// ErrEditedMessageNotUser indicates a non-user message edit attempt.
	ErrEditedMessageNotUser = xerrors.New("only user messages can be edited")

	// errChatTakenByOtherWorker is a sentinel used inside the
	// processChat cleanup transaction to signal that another
	// worker acquired the chat, so all post-TX side effects
	// (status publish, pubsub, web push) must be skipped.
	errChatTakenByOtherWorker = xerrors.New("chat acquired by another worker")
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
	OwnerID            uuid.UUID
	WorkspaceID        uuid.NullUUID
	BuildID            uuid.NullUUID
	AgentID            uuid.NullUUID
	ParentChatID       uuid.NullUUID
	RootChatID         uuid.NullUUID
	Title              string
	ModelConfigID      uuid.UUID
	ChatMode           database.NullChatMode
	SystemPrompt       string
	InitialUserContent []codersdk.ChatMessagePart
	MCPServerIDs       []uuid.UUID
	Labels             database.StringMap
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
	ChatID        uuid.UUID
	CreatedBy     uuid.UUID
	Content       []codersdk.ChatMessagePart
	ModelConfigID *uuid.UUID
	BusyBehavior  SendMessageBusyBehavior
	MCPServerIDs  *[]uuid.UUID
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
	ModelConfigID   *uuid.UUID
}

// PromoteQueuedResult contains post-promotion message metadata.
type PromoteQueuedResult struct {
	PromotedMessage database.ChatMessage
}

// CreateChat creates a chat, inserts optional system prompt and initial user
// message, and moves the chat into pending status.
func (p *Server) CreateChat(ctx context.Context, opts CreateOptions) (database.Chat, error) {
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

	var chat database.Chat
	txErr := p.db.InTx(func(tx database.Store) error {
		if limitErr := p.checkUsageLimit(ctx, tx, opts.OwnerID); limitErr != nil {
			return limitErr
		}

		labelsJSON, err := json.Marshal(opts.Labels)
		if err != nil {
			return xerrors.Errorf("marshal labels: %w", err)
		}

		insertedChat, err := tx.InsertChat(ctx, database.InsertChatParams{
			OwnerID:           opts.OwnerID,
			WorkspaceID:       opts.WorkspaceID,
			BuildID:           opts.BuildID,
			AgentID:           opts.AgentID,
			ParentChatID:      opts.ParentChatID,
			RootChatID:        opts.RootChatID,
			LastModelConfigID: opts.ModelConfigID,
			Title:             opts.Title,
			Mode:              opts.ChatMode,
			// Chats created with an initial user message start pending.
			// Waiting is reserved for idle chats with no pending work.
			Status:       database.ChatStatusPending,
			MCPServerIDs: opts.MCPServerIDs,
			Labels: pqtype.NullRawMessage{
				RawMessage: labelsJSON,
				Valid:      true,
			},
		})
		if err != nil {
			return xerrors.Errorf("insert chat: %w", err)
		}

		systemPrompt := strings.TrimSpace(opts.SystemPrompt)
		var workspaceAwareness string
		if opts.WorkspaceID.Valid {
			workspaceAwareness = "This chat is attached to a workspace. You can use workspace tools like execute, read_file, write_file, etc."
		} else {
			workspaceAwareness = "There is no workspace associated with this chat yet. Create one using the create_workspace tool before using workspace tools like execute, read_file, write_file, etc."
		}
		workspaceAwarenessContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(workspaceAwareness),
		})
		if err != nil {
			return xerrors.Errorf("marshal workspace awareness: %w", err)
		}
		userContent, err := chatprompt.MarshalParts(opts.InitialUserContent)
		if err != nil {
			return xerrors.Errorf("marshal initial user content: %w", err)
		}

		msgParams := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
			ChatID: insertedChat.ID,
		}

		if systemPrompt != "" {
			systemContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
				codersdk.ChatMessageText(systemPrompt),
			})
			if err != nil {
				return xerrors.Errorf("marshal system prompt: %w", err)
			}
			appendChatMessage(&msgParams, newChatMessage(
				database.ChatMessageRoleSystem,
				systemContent,
				database.ChatMessageVisibilityModel,
				opts.ModelConfigID,
				chatprompt.CurrentContentVersion,
			))
		}

		appendChatMessage(&msgParams, newChatMessage(
			database.ChatMessageRoleSystem,
			workspaceAwarenessContent,
			database.ChatMessageVisibilityModel,
			opts.ModelConfigID,
			chatprompt.CurrentContentVersion,
		))

		appendChatMessage(&msgParams, newChatMessage(
			database.ChatMessageRoleUser,
			userContent,
			database.ChatMessageVisibilityBoth,
			opts.ModelConfigID,
			chatprompt.CurrentContentVersion,
		).withCreatedBy(opts.OwnerID))

		_, err = tx.InsertChatMessages(ctx, msgParams)
		if err != nil {
			return xerrors.Errorf("insert initial chat messages: %w", err)
		}

		chat = insertedChat

		if !chat.RootChatID.Valid && !chat.ParentChatID.Valid {
			chat.RootChatID = uuid.NullUUID{UUID: chat.ID, Valid: true}
		}
		return nil
	}, nil)
	if txErr != nil {
		return database.Chat{}, txErr
	}

	p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindCreated, nil)
	p.signalWake()
	return chat, nil
}

// SendMessage inserts a user message and optionally queues it while the chat
// is busy, then publishes stream + pubsub updates.
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

	var (
		result            SendMessageResult
		queuedMessagesSDK []codersdk.ChatQueuedMessage
	)

	txErr := p.db.InTx(func(tx database.Store) error {
		lockedChat, err := tx.GetChatByIDForUpdate(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("lock chat: %w", err)
		}

		// Enforce usage limits before queueing or inserting.
		if limitErr := p.checkUsageLimit(ctx, tx, lockedChat.OwnerID); limitErr != nil {
			return limitErr
		}

		modelConfigID := lockedChat.LastModelConfigID
		if opts.ModelConfigID != nil {
			modelConfigID = *opts.ModelConfigID
		}

		// Update MCP server IDs on the chat when explicitly provided.
		if opts.MCPServerIDs != nil {
			lockedChat, err = tx.UpdateChatMCPServerIDs(ctx, database.UpdateChatMCPServerIDsParams{
				ID:           opts.ChatID,
				MCPServerIDs: *opts.MCPServerIDs,
			})
			if err != nil {
				return xerrors.Errorf("update chat mcp server ids: %w", err)
			}
		}

		existingQueued, err := tx.GetChatQueuedMessages(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("get queued messages: %w", err)
		}

		// Both queue and interrupt behaviors queue messages
		// when the chat is busy. We also keep queueing while a
		// backlog exists so waiting chats blocked by spend limits
		// preserve FIFO user-message order. Interrupt additionally
		// signals the running loop to stop so the queued message
		// is promoted sooner. Crucially, this guarantees the
		// interrupted assistant response is persisted (with a
		// lower id/created_at) before the user message is
		// promoted into chat_messages, preserving correct
		// conversation order.
		if shouldQueueUserMessage(lockedChat.Status) || len(existingQueued) > 0 {
			if len(existingQueued) >= MaxQueueSize {
				return ErrMessageQueueFull
			}

			queued, err := tx.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{
				ChatID:  opts.ChatID,
				Content: content.RawMessage,
			})
			if err != nil {
				return xerrors.Errorf("insert queued message: %w", err)
			}

			queuedMessages, err := tx.GetChatQueuedMessages(ctx, opts.ChatID)
			if err != nil {
				return xerrors.Errorf("get queued messages: %w", err)
			}

			result.Queued = true
			result.QueuedMessage = &queued
			result.Chat = lockedChat
			queuedMessagesSDK = db2sdk.ChatQueuedMessages(queuedMessages)
			return nil
		}

		message, updatedChat, err := insertUserMessageAndSetPending(
			ctx,
			tx,
			lockedChat,
			modelConfigID,
			content,
			opts.CreatedBy,
		)
		if err != nil {
			return err
		}
		result.Message = message
		result.Chat = updatedChat

		return nil
	}, nil)
	if txErr != nil {
		return SendMessageResult{}, txErr
	}

	if result.Queued {
		p.publishEvent(opts.ChatID, codersdk.ChatStreamEvent{
			Type:           codersdk.ChatStreamEventTypeQueueUpdate,
			ChatID:         opts.ChatID,
			QueuedMessages: queuedMessagesSDK,
		})
		p.publishChatStreamNotify(opts.ChatID, coderdpubsub.ChatStreamNotifyMessage{
			QueueUpdate: true,
		})

		// For interrupt behavior, signal the running loop to
		// stop. setChatWaiting publishes a status notification
		// that the worker's control subscriber detects, causing
		// it to cancel with ErrInterrupted. The deferred cleanup
		// in processChat then auto-promotes the queued message
		// after persisting the partial assistant response.
		if busyBehavior == SendMessageBusyBehaviorInterrupt {
			updatedChat, err := p.setChatWaiting(ctx, opts.ChatID)
			if err != nil {
				// The message is already queued so the chat is
				// not in a broken state — the user can still
				// wait for the current run to finish. Log the
				// error but don't fail the request.
				p.logger.Error(ctx, "failed to interrupt chat for queued message",
					slog.F("chat_id", opts.ChatID),
					slog.Error(err),
				)
			} else {
				result.Chat = updatedChat
			}
		}

		return result, nil
	}

	p.publishMessage(opts.ChatID, result.Message)
	p.publishStatus(opts.ChatID, result.Chat.Status, result.Chat.WorkerID)
	p.publishChatPubsubEvent(result.Chat, coderdpubsub.ChatEventKindStatusChange, nil)
	p.signalWake()
	return result, nil
}

func (p *Server) checkUsageLimit(ctx context.Context, store database.Store, ownerID uuid.UUID) error {
	status, err := ResolveUsageLimitStatus(ctx, store, ownerID, time.Now())
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

// EditMessage marks the old user message as deleted, soft-deletes all
// following messages, inserts a new message with the updated content,
// clears queued messages, and moves the chat into pending status.
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

	var result EditMessageResult
	txErr := p.db.InTx(func(tx database.Store) error {
		lockedChat, err := tx.GetChatByIDForUpdate(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("lock chat: %w", err)
		}

		if limitErr := p.checkUsageLimit(ctx, tx, lockedChat.OwnerID); limitErr != nil {
			return limitErr
		}

		existing, err := tx.GetChatMessageByID(ctx, opts.EditedMessageID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrEditedMessageNotFound
			}
			return xerrors.Errorf("get edited message: %w", err)
		}
		if existing.ChatID != opts.ChatID {
			return ErrEditedMessageNotFound
		}
		if existing.Role != database.ChatMessageRoleUser {
			return ErrEditedMessageNotUser
		}

		// Soft-delete the original message instead of updating in place
		// so that usage/cost data is preserved.
		err = tx.SoftDeleteChatMessageByID(ctx, opts.EditedMessageID)
		if err != nil {
			return xerrors.Errorf("soft-delete edited message: %w", err)
		}

		// Soft-delete all messages that came after the edited one.
		err = tx.SoftDeleteChatMessagesAfterID(ctx, database.SoftDeleteChatMessagesAfterIDParams{
			ChatID:  opts.ChatID,
			AfterID: opts.EditedMessageID,
		})
		if err != nil {
			return xerrors.Errorf("soft-delete later chat messages: %w", err)
		}

		// Insert a new message with the updated content.
		msgParams := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
			ChatID: opts.ChatID,
		}
		appendChatMessage(&msgParams, newChatMessage(
			database.ChatMessageRoleUser,
			content,
			existing.Visibility,
			existing.ModelConfigID.UUID,
			chatprompt.CurrentContentVersion,
		).withCreatedBy(opts.CreatedBy))
		newMessages, err := insertChatMessageWithStore(ctx, tx, msgParams)
		if err != nil {
			return xerrors.Errorf("insert replacement message: %w", err)
		}
		newMessage := newMessages[0]

		err = tx.DeleteAllChatQueuedMessages(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("delete queued messages: %w", err)
		}
		updatedChat, err := tx.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
			ID:          opts.ChatID,
			Status:      database.ChatStatusPending,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
		})
		if err != nil {
			return xerrors.Errorf("set chat pending: %w", err)
		}

		result.Message = newMessage
		result.Chat = updatedChat
		return nil
	}, nil)
	if txErr != nil {
		return EditMessageResult{}, txErr
	}

	p.publishEditedMessage(opts.ChatID, result.Message)
	p.publishEvent(opts.ChatID, codersdk.ChatStreamEvent{
		Type:           codersdk.ChatStreamEventTypeQueueUpdate,
		QueuedMessages: []codersdk.ChatQueuedMessage{},
	})
	p.publishChatStreamNotify(opts.ChatID, coderdpubsub.ChatStreamNotifyMessage{
		QueueUpdate: true,
	})
	p.publishStatus(opts.ChatID, result.Chat.Status, result.Chat.WorkerID)
	p.publishChatPubsubEvent(result.Chat, coderdpubsub.ChatEventKindStatusChange, nil)
	p.signalWake()

	return result, nil
}

// ArchiveChat archives a chat family and broadcasts deleted events for each
// affected chat so watching clients converge without a full refetch. If the
// target chat is pending or running, it first transitions the chat back to
// waiting so active processing stops before the archive is broadcast.
func (p *Server) ArchiveChat(ctx context.Context, chat database.Chat) error {
	if chat.ID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}

	statusChat := chat
	interrupted := false
	var archivedChats []database.Chat
	if err := p.db.InTx(func(tx database.Store) error {
		lockedChat, err := tx.GetChatByIDForUpdate(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("lock chat for archive: %w", err)
		}
		statusChat = lockedChat

		// We do not call setChatWaiting here because it intentionally preserves
		// pending chats so queued-message promotion can win. Archiving is a
		// harder stop: both pending and running chats must transition to waiting.
		if lockedChat.Status == database.ChatStatusPending || lockedChat.Status == database.ChatStatusRunning {
			statusChat, err = tx.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
				ID:          chat.ID,
				Status:      database.ChatStatusWaiting,
				WorkerID:    uuid.NullUUID{},
				StartedAt:   sql.NullTime{},
				HeartbeatAt: sql.NullTime{},
				LastError:   sql.NullString{},
			})
			if err != nil {
				return xerrors.Errorf("set chat waiting before archive: %w", err)
			}
			interrupted = true
		}

		archivedChats, err = tx.ArchiveChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("archive chat: %w", err)
		}
		return nil
	}, nil); err != nil {
		return err
	}

	if interrupted {
		p.publishStatus(chat.ID, statusChat.Status, statusChat.WorkerID)
		p.publishChatPubsubEvent(statusChat, coderdpubsub.ChatEventKindStatusChange, nil)
	}

	p.publishChatPubsubEvents(archivedChats, coderdpubsub.ChatEventKindDeleted)
	return nil
}

// UnarchiveChat unarchives a chat family and publishes created events for
// each affected chat so watching clients see every chat that reappeared.
func (p *Server) UnarchiveChat(ctx context.Context, chat database.Chat) error {
	if chat.ID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}

	return p.applyChatLifecycleTransition(
		ctx,
		chat.ID,
		"unarchive",
		coderdpubsub.ChatEventKindCreated,
		p.db.UnarchiveChatByID,
	)
}

func (p *Server) applyChatLifecycleTransition(
	ctx context.Context,
	chatID uuid.UUID,
	action string,
	kind coderdpubsub.ChatEventKind,
	transition func(context.Context, uuid.UUID) ([]database.Chat, error),
) error {
	updatedChats, err := transition(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("%s chat: %w", action, err)
	}

	p.publishChatPubsubEvents(updatedChats, kind)
	return nil
}

// DeleteQueued removes a queued user message and publishes the queue update.
func (p *Server) DeleteQueued(
	ctx context.Context,
	chatID uuid.UUID,
	queuedMessageID int64,
) error {
	if chatID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}

	var queuedMessages []database.ChatQueuedMessage
	var queueLoadedOK bool

	txErr := p.db.InTx(func(tx database.Store) error {
		// Lock the chat row to prevent processChat from
		// auto-promoting a message the user intended to delete.
		if _, err := tx.GetChatByIDForUpdate(ctx, chatID); err != nil {
			return xerrors.Errorf("lock chat: %w", err)
		}

		err := tx.DeleteChatQueuedMessage(ctx, database.DeleteChatQueuedMessageParams{
			ID:     queuedMessageID,
			ChatID: chatID,
		})
		if err != nil {
			return xerrors.Errorf("delete queued message: %w", err)
		}

		var err2 error
		queuedMessages, err2 = tx.GetChatQueuedMessages(ctx, chatID)
		if err2 != nil {
			p.logger.Warn(ctx, "failed to load queued messages after delete",
				slog.F("chat_id", chatID),
				slog.F("queued_message_id", queuedMessageID),
				slog.Error(err2),
			)
			// Non-fatal: the delete succeeded, so we still commit.
			return nil
		}
		queueLoadedOK = true

		return nil
	}, nil)
	if txErr != nil {
		return txErr
	}

	if queueLoadedOK {
		p.publishEvent(chatID, codersdk.ChatStreamEvent{
			Type:           codersdk.ChatStreamEventTypeQueueUpdate,
			QueuedMessages: db2sdk.ChatQueuedMessages(queuedMessages),
		})
	}
	// Always notify subscribers so they can re-fetch, even if we
	// failed to load the updated queue payload above.
	p.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		QueueUpdate: true,
	})
	return nil
}

// PromoteQueued promotes a queued message into chat history and marks the chat pending.
func (p *Server) PromoteQueued(
	ctx context.Context,
	opts PromoteQueuedOptions,
) (PromoteQueuedResult, error) {
	if opts.ChatID == uuid.Nil {
		return PromoteQueuedResult{}, xerrors.New("chat_id is required")
	}

	var (
		result         PromoteQueuedResult
		promoted       database.ChatMessage
		updatedChat    database.Chat
		remainingQueue []database.ChatQueuedMessage
	)

	txErr := p.db.InTx(func(tx database.Store) error {
		lockedChat, err := tx.GetChatByIDForUpdate(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("lock chat: %w", err)
		}
		modelConfigID := lockedChat.LastModelConfigID
		if opts.ModelConfigID != nil {
			modelConfigID = *opts.ModelConfigID
		}

		queuedMessages, err := tx.GetChatQueuedMessages(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("get queued messages: %w", err)
		}

		var (
			targetContent json.RawMessage
			found         bool
		)
		for _, qm := range queuedMessages {
			if qm.ID == opts.QueuedMessageID {
				targetContent = qm.Content
				found = true
				break
			}
		}
		if !found {
			return xerrors.New("queued message not found")
		}

		err = tx.DeleteChatQueuedMessage(ctx, database.DeleteChatQueuedMessageParams{
			ID:     opts.QueuedMessageID,
			ChatID: opts.ChatID,
		})
		if err != nil {
			return xerrors.Errorf("delete queued message: %w", err)
		}

		promoted, updatedChat, err = insertUserMessageAndSetPending(
			ctx,
			tx,
			lockedChat,
			modelConfigID,
			pqtype.NullRawMessage{
				RawMessage: targetContent,
				Valid:      len(targetContent) > 0,
			},
			opts.CreatedBy,
		)
		if err != nil {
			return err
		}

		remainingQueue, err = tx.GetChatQueuedMessages(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("get remaining queue: %w", err)
		}
		result.PromotedMessage = promoted

		return nil
	}, nil)
	if txErr != nil {
		return PromoteQueuedResult{}, txErr
	}

	p.publishEvent(opts.ChatID, codersdk.ChatStreamEvent{
		Type:           codersdk.ChatStreamEventTypeQueueUpdate,
		QueuedMessages: db2sdk.ChatQueuedMessages(remainingQueue),
	})
	p.publishChatStreamNotify(opts.ChatID, coderdpubsub.ChatStreamNotifyMessage{
		QueueUpdate: true,
	})
	p.publishMessage(opts.ChatID, promoted)
	p.publishStatus(opts.ChatID, updatedChat.Status, updatedChat.WorkerID)
	p.publishChatPubsubEvent(updatedChat, coderdpubsub.ChatEventKindStatusChange, nil)
	p.signalWake()

	return result, nil
}

// InterruptChat interrupts execution, sets waiting status, and broadcasts status updates.
func (p *Server) InterruptChat(
	ctx context.Context,
	chat database.Chat,
) database.Chat {
	if chat.ID == uuid.Nil {
		return chat
	}

	updatedChat, err := p.setChatWaiting(ctx, chat.ID)
	if err != nil {
		p.logger.Error(ctx, "failed to mark chat as waiting",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return chat
	}
	return updatedChat
}

const manualTitleMessageWindowLimit = 50

var ErrManualTitleRegenerationInProgress = xerrors.New(
	"manual title regeneration already in progress",
)

type manualTitleGenerationError struct {
	cause       error
	modelConfig database.ChatModelConfig
	usage       fantasy.Usage
}

func (e *manualTitleGenerationError) Error() string {
	return e.cause.Error()
}

func (e *manualTitleGenerationError) Unwrap() error {
	return e.cause
}

var manualTitleLockWorkerID = uuid.MustParse(
	"00000000-0000-0000-0000-000000000001",
)

const manualTitleLockStaleAfter = time.Minute

func isFreshManualTitleLock(chat database.Chat, now time.Time) bool {
	if !chat.WorkerID.Valid || chat.WorkerID.UUID != manualTitleLockWorkerID {
		return false
	}
	leaseAt := chat.HeartbeatAt
	if !leaseAt.Valid {
		leaseAt = chat.StartedAt
	}
	return leaseAt.Valid && leaseAt.Time.After(now.Add(-manualTitleLockStaleAfter))
}

// updateChatStatusPreserveUpdatedAt applies internal lock transitions without
// changing chat recency, because chat list ordering uses updated_at.
func updateChatStatusPreserveUpdatedAt(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	workerID uuid.NullUUID,
	startedAt sql.NullTime,
	heartbeatAt sql.NullTime,
) (database.Chat, error) {
	return store.UpdateChatStatusPreserveUpdatedAt(
		ctx,
		database.UpdateChatStatusPreserveUpdatedAtParams{
			ID:          chat.ID,
			Status:      chat.Status,
			WorkerID:    workerID,
			StartedAt:   startedAt,
			HeartbeatAt: heartbeatAt,
			LastError:   chat.LastError,
			UpdatedAt:   chat.UpdatedAt,
		},
	)
}

func (p *Server) acquireManualTitleLock(ctx context.Context, chatID uuid.UUID) error {
	now := time.Now()
	return p.db.InTx(func(tx database.Store) error {
		lockedChat, err := tx.GetChatByIDForUpdate(ctx, chatID)
		if err != nil {
			return xerrors.Errorf("lock chat for manual title regeneration: %w", err)
		}
		// Only a fresh manual lock or a chat without a real worker should
		// block title regeneration. Running chats with a real worker may
		// regenerate their title concurrently, and last write wins.
		hasRealWorker := lockedChat.Status == database.ChatStatusRunning &&
			lockedChat.WorkerID.Valid &&
			lockedChat.WorkerID.UUID != manualTitleLockWorkerID
		if lockedChat.Status == database.ChatStatusPending ||
			(lockedChat.Status == database.ChatStatusRunning && !hasRealWorker) ||
			isFreshManualTitleLock(lockedChat, now) {
			return ErrManualTitleRegenerationInProgress
		}
		if hasRealWorker {
			return nil
		}

		_, err = updateChatStatusPreserveUpdatedAt(
			ctx,
			tx,
			lockedChat,
			uuid.NullUUID{UUID: manualTitleLockWorkerID, Valid: true},
			sql.NullTime{Time: now, Valid: true},
			sql.NullTime{},
		)
		if err != nil {
			return xerrors.Errorf("mark chat for manual title regeneration: %w", err)
		}
		return nil
	}, database.DefaultTXOptions().WithID("chat_title_regenerate_lock"))
}

func (p *Server) releaseManualTitleLock(ctx context.Context, chatID uuid.UUID) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	err := p.db.InTx(func(tx database.Store) error {
		lockedChat, err := tx.GetChatByIDForUpdate(cleanupCtx, chatID)
		if err != nil {
			return xerrors.Errorf("lock chat to release manual title regeneration: %w", err)
		}
		if !lockedChat.WorkerID.Valid || lockedChat.WorkerID.UUID != manualTitleLockWorkerID {
			return nil
		}
		_, err = updateChatStatusPreserveUpdatedAt(
			cleanupCtx,
			tx,
			lockedChat,
			uuid.NullUUID{},
			sql.NullTime{},
			sql.NullTime{},
		)
		if err != nil {
			return xerrors.Errorf("clear manual title regeneration marker: %w", err)
		}
		return nil
	}, database.DefaultTXOptions().WithID("chat_title_regenerate_unlock"))
	if err != nil {
		p.logger.Warn(cleanupCtx, "failed to release manual title regeneration marker",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
	}
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
	keys, err := p.resolveUserProviderAPIKeys(chatdCtx, chat.OwnerID)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("resolve chat providers: %w", err)
	}
	if err := p.acquireManualTitleLock(ctx, chat.ID); err != nil {
		return database.Chat{}, err
	}
	defer p.releaseManualTitleLock(chatdCtx, chat.ID)

	updatedChat, err := p.regenerateChatTitleWithStore(
		chatdCtx,
		p.db,
		chat,
		keys,
	)
	if err != nil {
		var generationErr *manualTitleGenerationError
		if errors.As(err, &generationErr) {
			// Reuse chatd's scoped auth context for failure accounting while
			// detaching from request cancellation so usage is still recorded.
			//nolint:gocritic // Failure accounting still needs chatd-scoped config reads.
			recordCtx, recordCancel := context.WithTimeout(
				dbauthz.AsChatd(context.WithoutCancel(ctx)),
				5*time.Second,
			)
			defer recordCancel()
			if _, recordErr := recordManualTitleUsage(
				recordCtx,
				p.db,
				chat,
				generationErr.modelConfig,
				generationErr.usage,
				"",
			); recordErr != nil {
				return database.Chat{}, errors.Join(
					generationErr,
					xerrors.Errorf("record manual title usage: %w", recordErr),
				)
			}
			return database.Chat{}, generationErr
		}
		return database.Chat{}, err
	}
	return updatedChat, nil
}

func (p *Server) regenerateChatTitleWithStore(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	keys chatprovider.ProviderAPIKeys,
) (database.Chat, error) {
	if limitErr := p.checkUsageLimit(ctx, store, chat.OwnerID); limitErr != nil {
		return database.Chat{}, limitErr
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
		return database.Chat{}, xerrors.Errorf("get head chat messages: %w", err)
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
		return database.Chat{}, xerrors.Errorf("get tail chat messages: %w", err)
	}
	messages := mergeManualTitleMessages(headMessages, tailMessages)
	if len(messages) == 0 {
		return chat, nil
	}

	model, modelConfig, err := p.resolveManualTitleModel(ctx, store, chat, keys)
	if err != nil {
		return database.Chat{}, err
	}

	title, usage, err := generateManualTitle(ctx, messages, model)
	if err != nil {
		wrappedErr := xerrors.Errorf("generate manual title: %w", err)
		if usage == (fantasy.Usage{}) {
			return database.Chat{}, wrappedErr
		}
		return database.Chat{}, &manualTitleGenerationError{
			cause:       wrappedErr,
			modelConfig: modelConfig,
			usage:       usage,
		}
	}

	recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer recordCancel()

	updatedChat, recordErr := recordManualTitleUsage(
		recordCtx,
		store,
		chat,
		modelConfig,
		usage,
		title,
	)
	if recordErr != nil {
		if title != "" {
			return database.Chat{}, xerrors.Errorf("record manual title usage and update chat title: %w", recordErr)
		}
		return database.Chat{}, xerrors.Errorf("record manual title usage: %w", recordErr)
	}
	if updatedChat.Title == chat.Title {
		return updatedChat, nil
	}

	p.publishChatPubsubEvent(updatedChat, coderdpubsub.ChatEventKindTitleChange, nil)
	return updatedChat, nil
}

func (p *Server) resolveManualTitleModel(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	keys chatprovider.ProviderAPIKeys,
) (fantasy.LanguageModel, database.ChatModelConfig, error) {
	configs, err := store.GetEnabledChatModelConfigs(ctx)
	if err != nil {
		p.logger.Debug(ctx, "failed to list manual title model configs",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return p.resolveFallbackManualTitleModel(ctx, chat, keys)
	}

	config, ok := selectPreferredConfiguredShortTextModelConfig(configs)
	if !ok {
		return p.resolveFallbackManualTitleModel(ctx, chat, keys)
	}

	model, err := chatprovider.ModelFromConfig(
		config.Provider,
		config.Model,
		keys,
		chatprovider.UserAgent(),
		chatprovider.CoderHeaders(chat),
	)
	if err != nil {
		p.logger.Debug(ctx, "manual title preferred model unavailable",
			slog.F("chat_id", chat.ID),
			slog.F("provider", config.Provider),
			slog.F("model", config.Model),
			slog.Error(err),
		)
		return p.resolveFallbackManualTitleModel(ctx, chat, keys)
	}

	return model, config, nil
}

func (p *Server) resolveFallbackManualTitleModel(
	ctx context.Context,
	chat database.Chat,
	keys chatprovider.ProviderAPIKeys,
) (fantasy.LanguageModel, database.ChatModelConfig, error) {
	config, err := p.resolveModelConfig(ctx, chat)
	if err != nil {
		return nil, database.ChatModelConfig{}, xerrors.Errorf(
			"resolve fallback manual title model config: %w",
			err,
		)
	}
	model, err := chatprovider.ModelFromConfig(
		config.Provider,
		config.Model,
		keys,
		chatprovider.UserAgent(),
		chatprovider.CoderHeaders(chat),
	)
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

func fantasyUsageToChatMessageUsage(usage fantasy.Usage) codersdk.ChatMessageUsage {
	var chatUsage codersdk.ChatMessageUsage
	if usage.InputTokens != 0 {
		chatUsage.InputTokens = ptr.Ref(usage.InputTokens)
	}
	if usage.OutputTokens != 0 {
		chatUsage.OutputTokens = ptr.Ref(usage.OutputTokens)
	}
	if usage.ReasoningTokens != 0 {
		chatUsage.ReasoningTokens = ptr.Ref(usage.ReasoningTokens)
	}
	if usage.CacheCreationTokens != 0 {
		chatUsage.CacheCreationTokens = ptr.Ref(usage.CacheCreationTokens)
	}
	if usage.CacheReadTokens != 0 {
		chatUsage.CacheReadTokens = ptr.Ref(usage.CacheReadTokens)
	}
	return chatUsage
}

func recordManualTitleUsage(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	modelConfig database.ChatModelConfig,
	usage fantasy.Usage,
	newTitle string,
) (database.Chat, error) {
	hasUsage := usage != (fantasy.Usage{})
	if !hasUsage && newTitle == "" {
		return chat, nil
	}

	var totalCostMicros *int64
	if hasUsage {
		callConfig := codersdk.ChatModelCallConfig{}
		if len(modelConfig.Options) > 0 {
			if err := json.Unmarshal(modelConfig.Options, &callConfig); err != nil {
				return database.Chat{}, xerrors.Errorf("parse model call config: %w", err)
			}
		}
		totalCostMicros = chatcost.CalculateTotalCostMicros(
			fantasyUsageToChatMessageUsage(usage),
			callConfig.Cost,
		)
	}

	// Use a valid empty JSON array for the content column.
	// MarshalParts returns a null NullRawMessage for empty
	// slices, which becomes an empty string that PostgreSQL
	// rejects as invalid JSON.
	content := "[]"

	updatedChat := chat
	err := store.InTx(func(tx database.Store) error {
		lockedChat, err := tx.GetChatByIDForUpdate(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("lock chat for manual title usage: %w", err)
		}
		updatedChat = lockedChat
		if hasUsage {
			messages, err := tx.InsertChatMessages(ctx, database.InsertChatMessagesParams{
				ChatID:              chat.ID,
				CreatedBy:           []uuid.UUID{chat.OwnerID},
				ModelConfigID:       []uuid.UUID{modelConfig.ID},
				Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
				Content:             []string{content},
				ContentVersion:      []int16{chatprompt.CurrentContentVersion},
				Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityModel},
				InputTokens:         []int64{usage.InputTokens},
				OutputTokens:        []int64{usage.OutputTokens},
				TotalTokens:         []int64{usage.TotalTokens},
				ReasoningTokens:     []int64{usage.ReasoningTokens},
				CacheCreationTokens: []int64{usage.CacheCreationTokens},
				CacheReadTokens:     []int64{usage.CacheReadTokens},
				ContextLimit:        []int64{modelConfig.ContextLimit},
				Compressed:          []bool{false},
				TotalCostMicros:     []int64{ptr.NilToDefault(totalCostMicros, 0)},
				RuntimeMs:           []int64{0},
				ProviderResponseID:  []string{""},
			})
			if err != nil {
				return xerrors.Errorf("insert manual title usage message: %w", err)
			}
			if len(messages) != 1 {
				return xerrors.Errorf("expected 1 manual title usage message, got %d", len(messages))
			}
			if err := tx.SoftDeleteChatMessageByID(ctx, messages[0].ID); err != nil {
				return xerrors.Errorf("soft delete manual title usage message: %w", err)
			}
			if lockedChat.LastModelConfigID != modelConfig.ID {
				if _, err := tx.UpdateChatLastModelConfigByID(ctx, database.UpdateChatLastModelConfigByIDParams{
					ID:                chat.ID,
					LastModelConfigID: lockedChat.LastModelConfigID,
				}); err != nil {
					return xerrors.Errorf("restore chat model config after manual title usage: %w", err)
				}
			}
		}
		if newTitle != "" && lockedChat.Title == chat.Title && newTitle != lockedChat.Title {
			updatedChat, err = tx.UpdateChatByID(ctx, database.UpdateChatByIDParams{
				ID:    chat.ID,
				Title: newTitle,
			})
			if err != nil {
				return xerrors.Errorf("update chat title: %w", err)
			}
		}
		return nil
	}, nil)
	if err != nil {
		return database.Chat{}, err
	}
	return updatedChat, nil
}

// RefreshStatus loads the latest chat status and publishes it to stream subscribers.
func (p *Server) RefreshStatus(ctx context.Context, chatID uuid.UUID) error {
	if chatID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}

	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("get chat: %w", err)
	}

	p.publishStatus(chat.ID, chat.Status, chat.WorkerID)
	return nil
}

func (p *Server) setChatWaiting(ctx context.Context, chatID uuid.UUID) (database.Chat, error) {
	var updatedChat database.Chat
	err := p.db.InTx(func(tx database.Store) error {
		locked, lockErr := tx.GetChatByIDForUpdate(ctx, chatID)
		if lockErr != nil {
			return xerrors.Errorf("lock chat for waiting: %w", lockErr)
		}
		// If the chat has already transitioned to pending (e.g.
		// SendMessage with interrupt behavior), don't overwrite
		// it — the pending status takes priority so the new
		// message gets processed.
		if locked.Status == database.ChatStatusPending {
			updatedChat = locked
			return nil
		}
		var updateErr error
		updatedChat, updateErr = tx.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
			ID:          chatID,
			Status:      database.ChatStatusWaiting,
			WorkerID:    uuid.NullUUID{},
			StartedAt:   sql.NullTime{},
			HeartbeatAt: sql.NullTime{},
			LastError:   sql.NullString{},
		})
		return updateErr
	}, nil)
	if err != nil {
		return database.Chat{}, err
	}
	p.publishStatus(chatID, updatedChat.Status, updatedChat.WorkerID)
	p.publishChatPubsubEvent(updatedChat, coderdpubsub.ChatEventKindStatusChange, nil)
	return updatedChat, nil
}

func insertChatMessageWithStore(
	ctx context.Context,
	store database.Store,
	params database.InsertChatMessagesParams,
) ([]database.ChatMessage, error) {
	messages, err := store.InsertChatMessages(ctx, params)
	if err != nil {
		return nil, xerrors.Errorf("insert chat message: %w", err)
	}
	return messages, nil
}

// chatMessage describes a single message to insert as part of a batch.
// Use newChatMessage to create one, then chain builder methods for
// optional fields. For nullable UUID fields (ModelConfigID, CreatedBy),
// use uuid.Nil to represent NULL — the SQL uses NULLIF to convert zero
// UUIDs to NULL. For nullable int64 fields, use 0 to represent NULL —
// the SQL uses NULLIF to convert zeros to NULL.
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
	providerResponseID  string
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

func (m chatMessage) withCompressed() chatMessage {
	m.compressed = true
	return m
}

func (m chatMessage) withUsage(
	inputTokens, outputTokens, totalTokens, reasoningTokens,
	cacheCreationTokens, cacheReadTokens int64,
) chatMessage {
	m.inputTokens = inputTokens
	m.outputTokens = outputTokens
	m.totalTokens = totalTokens
	m.reasoningTokens = reasoningTokens
	m.cacheCreationTokens = cacheCreationTokens
	m.cacheReadTokens = cacheReadTokens
	return m
}

func (m chatMessage) withContextLimit(limit int64) chatMessage {
	m.contextLimit = limit
	return m
}

func (m chatMessage) withTotalCostMicros(cost int64) chatMessage {
	m.totalCostMicros = cost
	return m
}

func (m chatMessage) withRuntimeMs(ms int64) chatMessage {
	m.runtimeMs = ms
	return m
}

func (m chatMessage) withProviderResponseID(id string) chatMessage {
	m.providerResponseID = id
	return m
}

// chainModeInfo holds the information needed to determine whether
// a follow-up turn can use OpenAI's previous_response_id chaining
// instead of replaying full conversation history.
type chainModeInfo struct {
	// previousResponseID is the provider response ID from the last
	// assistant message, if any.
	previousResponseID string
	// modelConfigID is the model configuration used to produce the
	// assistant message referenced by previousResponseID.
	modelConfigID uuid.UUID
	// trailingUserCount is the number of contiguous user messages
	// at the end of the conversation that form the current turn.
	trailingUserCount int
}

// resolveChainMode scans DB messages from the end to count trailing user
// messages for the current turn and detect whether the immediately
// preceding assistant/tool block can chain from a provider response ID.
func resolveChainMode(messages []database.ChatMessage) chainModeInfo {
	var info chainModeInfo
	i := len(messages) - 1
	for ; i >= 0; i-- {
		if messages[i].Role == database.ChatMessageRoleUser {
			info.trailingUserCount++
			continue
		}
		break
	}
	for ; i >= 0; i-- {
		switch messages[i].Role {
		case database.ChatMessageRoleAssistant:
			if messages[i].ProviderResponseID.Valid &&
				messages[i].ProviderResponseID.String != "" {
				info.previousResponseID = messages[i].ProviderResponseID.String
				if messages[i].ModelConfigID.Valid {
					info.modelConfigID = messages[i].ModelConfigID.UUID
				}
				return info
			}
			return info
		case database.ChatMessageRoleTool:
			continue
		default:
			return info
		}
	}
	return info
}

// filterPromptForChainMode keeps only system messages and the last
// trailingUserCount user messages from the prompt. Assistant and tool
// messages are dropped because the provider already has them via the
// previous_response_id chain.
func filterPromptForChainMode(
	prompt []fantasy.Message,
	trailingUserCount int,
) []fantasy.Message {
	if trailingUserCount <= 0 {
		return prompt
	}

	totalUsers := 0
	for _, msg := range prompt {
		if msg.Role == "user" {
			totalUsers++
		}
	}

	usersToSkip := totalUsers - trailingUserCount
	if usersToSkip < 0 {
		usersToSkip = 0
	}

	filtered := make([]fantasy.Message, 0, len(prompt))
	usersSeen := 0
	for _, msg := range prompt {
		switch msg.Role {
		case "system":
			filtered = append(filtered, msg)
		case "user":
			usersSeen++
			if usersSeen > usersToSkip {
				filtered = append(filtered, msg)
			}
		}
	}

	return filtered
}

// appendChatMessage appends a single message to the batch insert params.
func appendChatMessage(
	params *database.InsertChatMessagesParams,
	msg chatMessage,
) {
	params.CreatedBy = append(params.CreatedBy, msg.createdBy)
	params.ModelConfigID = append(params.ModelConfigID, msg.modelConfigID)
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
	params.ProviderResponseID = append(params.ProviderResponseID, msg.providerResponseID)
}

func insertUserMessageAndSetPending(
	ctx context.Context,
	store database.Store,
	lockedChat database.Chat,
	modelConfigID uuid.UUID,
	content pqtype.NullRawMessage,
	createdBy uuid.UUID,
) (database.ChatMessage, database.Chat, error) {
	msgParams := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
		ChatID: lockedChat.ID,
	}
	appendChatMessage(&msgParams, newChatMessage(
		database.ChatMessageRoleUser,
		content,
		database.ChatMessageVisibilityBoth,
		modelConfigID,
		chatprompt.CurrentContentVersion,
	).withCreatedBy(createdBy))
	messages, err := insertChatMessageWithStore(ctx, store, msgParams)
	if err != nil {
		return database.ChatMessage{}, database.Chat{}, err
	}
	message := messages[0]

	if lockedChat.Status == database.ChatStatusPending {
		return message, lockedChat, nil
	}

	updatedChat, err := store.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          lockedChat.ID,
		Status:      database.ChatStatusPending,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   sql.NullString{},
	})
	if err != nil {
		return database.ChatMessage{}, database.Chat{}, xerrors.Errorf("set chat pending: %w", err)
	}
	return message, updatedChat, nil
}

// shouldQueueUserMessage reports whether a user message should be
// queued while a chat is active.
func shouldQueueUserMessage(status database.ChatStatus) bool {
	switch status {
	case database.ChatStatusRunning, database.ChatStatusPending:
		return true
	default:
		return false
	}
}

// Config configures a chat processor.
type Config struct {
	Logger                         slog.Logger
	Database                       database.Store
	ReplicaID                      uuid.UUID
	SubscribeFn                    SubscribeFn
	PendingChatAcquireInterval     time.Duration
	MaxChatsPerAcquire             int32
	InFlightChatStaleAfter         time.Duration
	ChatHeartbeatInterval          time.Duration
	AgentConn                      AgentConnFunc
	AgentInactiveDisconnectTimeout time.Duration
	InstructionLookupTimeout       time.Duration
	CreateWorkspace                chattool.CreateWorkspaceFn
	StartWorkspace                 chattool.StartWorkspaceFn
	Pubsub                         pubsub.Pubsub
	ProviderAPIKeys                chatprovider.ProviderAPIKeys
	WebpushDispatcher              webpush.Dispatcher
	UsageTracker                   *workspacestats.UsageTracker
	Clock                          quartz.Clock
}

// New creates a new chat processor. The processor polls for pending
// chats and processes them. It is the caller's responsibility to call Close
// on the returned instance.
func New(cfg Config) *Server {
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

	instructionLookupTimeout := cfg.InstructionLookupTimeout
	if instructionLookupTimeout == 0 {
		instructionLookupTimeout = homeInstructionLookupTimeout
	}

	workerID := cfg.ReplicaID
	if workerID == uuid.Nil {
		workerID = uuid.New()
	}

	p := &Server{
		cancel:                         cancel,
		closed:                         make(chan struct{}),
		db:                             cfg.Database,
		workerID:                       workerID,
		logger:                         cfg.Logger.Named("processor"),
		subscribeFn:                    cfg.SubscribeFn,
		agentConnFn:                    cfg.AgentConn,
		agentInactiveDisconnectTimeout: cfg.AgentInactiveDisconnectTimeout,
		instructionLookupTimeout:       instructionLookupTimeout,
		createWorkspaceFn:              cfg.CreateWorkspace,
		startWorkspaceFn:               cfg.StartWorkspace,
		pubsub:                         cfg.Pubsub,
		webpushDispatcher:              cfg.WebpushDispatcher,
		providerAPIKeys:                cfg.ProviderAPIKeys,
		pendingChatAcquireInterval:     pendingChatAcquireInterval,
		maxChatsPerAcquire:             maxChatsPerAcquire,
		inFlightChatStaleAfter:         inFlightChatStaleAfter,
		chatHeartbeatInterval:          chatHeartbeatInterval,
		usageTracker:                   cfg.UsageTracker,
		clock:                          clk,
		recordingSem:                   make(chan struct{}, maxConcurrentRecordingUploads),
		wakeCh:                         make(chan struct{}, 1),
	}

	//nolint:gocritic // The chat processor uses a scoped chatd context.
	ctx = dbauthz.AsChatd(ctx)

	p.configCache = newChatConfigCache(ctx, cfg.Database, clk)
	if p.pubsub != nil {
		cancelConfigSub, err := p.pubsub.SubscribeWithErr(
			coderdpubsub.ChatConfigEventChannel,
			coderdpubsub.HandleChatConfigEvent(func(ctx context.Context, ev coderdpubsub.ChatConfigEvent, err error) {
				if err != nil {
					p.logger.Warn(ctx, "chat config event error", slog.Error(err))
					return
				}
				switch ev.Kind {
				case coderdpubsub.ChatConfigEventProviders:
					p.configCache.InvalidateProviders()
				case coderdpubsub.ChatConfigEventModelConfig:
					p.configCache.InvalidateModelConfig(ev.EntityID)
				case coderdpubsub.ChatConfigEventUserPrompt:
					p.configCache.InvalidateUserPrompt(ev.EntityID)
				}
			}),
		)
		if err != nil {
			p.logger.Error(ctx, "subscribe to chat config events", slog.Error(err))
		}
		p.configCacheUnsubscribe = cancelConfigSub
	}
	go p.start(ctx)

	return p
}

func (p *Server) start(ctx context.Context) {
	defer close(p.closed)

	// Recover stale chats on startup and periodically thereafter
	// to handle chats orphaned by crashed or redeployed workers.
	p.recoverStaleChats(ctx)

	acquireTicker := p.clock.NewTicker(
		p.pendingChatAcquireInterval,
		"chatd",
		"acquire",
	)
	defer acquireTicker.Stop()

	staleRecoveryInterval := p.inFlightChatStaleAfter / staleRecoveryIntervalDivisor
	staleTicker := p.clock.NewTicker(
		staleRecoveryInterval,
		"chatd",
		"stale-recovery",
	)
	defer staleTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-acquireTicker.C:
			p.processOnce(ctx)
		case <-p.wakeCh:
			p.processOnce(ctx)
		case <-staleTicker.C:
			p.recoverStaleChats(ctx)
		}
	}
}

// signalWake wakes the run loop so it calls processOnce immediately.
// Non-blocking: if a signal is already pending it is a no-op.
func (p *Server) signalWake() {
	select {
	case p.wakeCh <- struct{}{}:
	default:
	}
}

func (p *Server) processOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	// We detach from the server lifetime to prevent a
	// phantom-acquire race: when the server context is
	// canceled, the pq driver's watchCancel goroutine
	// races with the actual query on the wire. Using a
	// context that cannot be canceled ensures the driver
	// sees the query result if Postgres executed it.
	acquireCtx, acquireCancel := context.WithTimeout(
		context.WithoutCancel(ctx), 10*time.Second,
	)
	chats, err := p.db.AcquireChats(acquireCtx, database.AcquireChatsParams{
		StartedAt: time.Now(),
		WorkerID:  p.workerID,
		NumChats:  p.maxChatsPerAcquire,
	})
	acquireCancel()
	if err != nil {
		p.logger.Error(ctx, "failed to acquire chats", slog.Error(err))
		return
	}
	if len(chats) == 0 {
		return
	}

	// If the server context was canceled while we were
	// acquiring, release the chats back to pending.
	if ctx.Err() != nil {
		releaseCtx, releaseCancel := context.WithTimeout(
			context.WithoutCancel(ctx), 10*time.Second,
		)
		for _, chat := range chats {
			_, updateErr := p.db.UpdateChatStatus(releaseCtx, database.UpdateChatStatusParams{
				ID:          chat.ID,
				Status:      database.ChatStatusPending,
				WorkerID:    uuid.NullUUID{},
				StartedAt:   sql.NullTime{},
				HeartbeatAt: sql.NullTime{},
				LastError:   sql.NullString{},
			})
			if updateErr != nil {
				p.logger.Error(ctx, "failed to release chat acquired during shutdown",
					slog.F("chat_id", chat.ID), slog.Error(updateErr))
			}
		}
		releaseCancel()
		return
	}

	p.inflightMu.Lock()
	for _, chat := range chats {
		p.inflight.Add(1)
		go func() {
			defer p.inflight.Done()
			p.processChat(ctx, chat)
		}()
	}
	p.inflightMu.Unlock()
}

func (p *Server) publishToStream(chatID uuid.UUID, event codersdk.ChatStreamEvent) {
	state := p.getOrCreateStreamState(chatID)
	state.mu.Lock()
	if event.Type == codersdk.ChatStreamEventTypeMessagePart {
		if !state.buffering {
			p.cleanupStreamIfIdle(chatID, state)
			state.mu.Unlock()
			return
		}
		if len(state.buffer) >= maxStreamBufferSize {
			state.bufferDropCount++
			now := p.clock.Now()
			if now.Sub(state.bufferLastWarnAt) >= streamDropWarnInterval {
				p.logger.Warn(context.Background(), "chat stream buffer full, dropping oldest event",
					slog.F("chat_id", chatID),
					slog.F("buffer_size", len(state.buffer)),
					slog.F("dropped_count", state.bufferDropCount),
				)
				state.bufferDropCount = 0
				state.bufferLastWarnAt = now
			}
			state.buffer = state.buffer[1:]
		}
		state.buffer = append(state.buffer, event)
	}
	subscribers := make([]chan codersdk.ChatStreamEvent, 0, len(state.subscribers))
	for _, ch := range state.subscribers {
		subscribers = append(subscribers, ch)
	}
	state.mu.Unlock()

	var subDropped int64
	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
			subDropped++
		}
	}

	// Re-acquire the lock once for both subscriber-drop logging and
	// idle cleanup. Merging these avoids an unnecessary unlock/re-lock
	// gap between the two sections.
	state.mu.Lock()
	if subDropped > 0 {
		state.subscriberDropCount += subDropped
		now := p.clock.Now()
		if now.Sub(state.subscriberLastWarnAt) >= streamDropWarnInterval {
			p.logger.Warn(context.Background(), "dropping chat stream event",
				slog.F("chat_id", chatID),
				slog.F("type", event.Type),
				slog.F("dropped_count", state.subscriberDropCount),
			)
			state.subscriberDropCount = 0
			state.subscriberLastWarnAt = now
		}
	}
	p.cleanupStreamIfIdle(chatID, state)
	state.mu.Unlock()
}

// cacheDurableMessage stores a recently persisted message event in the
// per-chat stream state so that same-replica subscribers can catch up
// from memory instead of the database. The afterMessageID is the
// message ID that precedes this message (i.e. message.ID - 1).
func (p *Server) cacheDurableMessage(chatID uuid.UUID, event codersdk.ChatStreamEvent) {
	state := p.getOrCreateStreamState(chatID)
	state.mu.Lock()
	defer state.mu.Unlock()

	if len(state.durableMessages) >= maxDurableMessageCacheSize {
		if evicted := state.durableMessages[0]; evicted.Message != nil {
			state.durableEvictedBefore = evicted.Message.ID
		}
		state.durableMessages = state.durableMessages[1:]
	}
	state.durableMessages = append(state.durableMessages, event)
}

// getCachedDurableMessages returns cached durable messages with IDs
// greater than afterID. Returns nil when the cache has no relevant
// entries.
func (p *Server) getCachedDurableMessages(
	chatID uuid.UUID,
	afterID int64,
) []codersdk.ChatStreamEvent {
	state := p.getOrCreateStreamState(chatID)
	state.mu.Lock()
	defer state.mu.Unlock()

	if afterID < state.durableEvictedBefore {
		return nil
	}

	var result []codersdk.ChatStreamEvent
	for _, event := range state.durableMessages {
		if event.Message != nil && event.Message.ID > afterID {
			result = append(result, event)
		}
	}
	return result
}

func (p *Server) subscribeToStream(chatID uuid.UUID) (
	[]codersdk.ChatStreamEvent,
	<-chan codersdk.ChatStreamEvent,
	func(),
) {
	state := p.getOrCreateStreamState(chatID)
	state.mu.Lock()
	snapshot := append([]codersdk.ChatStreamEvent(nil), state.buffer...)
	id := uuid.New()
	ch := make(chan codersdk.ChatStreamEvent, 128)
	state.subscribers[id] = ch
	state.mu.Unlock()

	cancel := func() {
		state.mu.Lock()
		// Remove the subscriber but do not close the channel.
		// publishToStream copies subscriber references under
		// the per-chat lock then sends outside; closing here
		// races with that send and can panic. The channel
		// becomes unreachable once removed and will be GC'd.
		delete(state.subscribers, id)
		p.cleanupStreamIfIdle(chatID, state)
		state.mu.Unlock()
	}

	return snapshot, ch, cancel
}

// getOrCreateStreamState returns the per-chat stream state,
// creating one atomically if it doesn't exist. The returned
// state has its own mutex — callers must lock state.mu for
// access.
func (p *Server) getOrCreateStreamState(chatID uuid.UUID) *chatStreamState {
	if val, ok := p.chatStreams.Load(chatID); ok {
		state, _ := val.(*chatStreamState)
		return state
	}
	val, _ := p.chatStreams.LoadOrStore(chatID, &chatStreamState{
		subscribers: make(map[uuid.UUID]chan codersdk.ChatStreamEvent),
	})
	state, _ := val.(*chatStreamState)
	return state
}

// cleanupStreamIfIdle removes the chat entry from the sync.Map
// when there are no subscribers and the stream is not buffering.
// When bufferRetainedAt is set, cleanup is deferred until the
// grace period expires so cross-replica relay subscribers can
// still snapshot the buffer.
// The caller must hold state.mu.
func (p *Server) cleanupStreamIfIdle(chatID uuid.UUID, state *chatStreamState) {
	if state.buffering || len(state.subscribers) > 0 {
		return
	}
	// Keep stream state alive during the grace period so
	// late-connecting relay subscribers can snapshot the
	// buffer after the worker finishes processing.
	if !state.bufferRetainedAt.IsZero() &&
		p.clock.Now().Before(state.bufferRetainedAt.Add(bufferRetainGracePeriod)) {
		return
	}
	p.chatStreams.Delete(chatID)
	p.workspaceMCPToolsCache.Delete(chatID)
}

func (p *Server) Subscribe(
	ctx context.Context,
	chatID uuid.UUID,
	requestHeader http.Header,
	afterMessageID int64,
) (
	[]codersdk.ChatStreamEvent,
	<-chan codersdk.ChatStreamEvent,
	func(),
	bool,
) {
	if p == nil {
		return nil, nil, nil, false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Subscribe to the local stream for message_parts and same-replica
	// persisted messages.
	localSnapshot, localParts, localCancel := p.subscribeToStream(chatID)

	// Merge all event sources.
	mergedCtx, mergedCancel := context.WithCancel(ctx)
	mergedEvents := make(chan codersdk.ChatStreamEvent, 128)

	var allCancels []func()
	allCancels = append(allCancels, localCancel)

	// Subscribe to pubsub for durable and structured control
	// events (status, messages, queue updates, retry, errors).
	// When pubsub is nil (e.g. in-memory
	// single-instance) we skip this and deliver all local events.
	//
	// This MUST happen before the DB queries below so that any
	// notification published between the query and the subscription
	// is not lost (subscribe-first-then-query pattern).
	var notifications <-chan coderdpubsub.ChatStreamNotifyMessage
	var errCh <-chan error
	if p.pubsub != nil {
		notifyCh := make(chan coderdpubsub.ChatStreamNotifyMessage, 10)
		errNotifyCh := make(chan error, 1)
		notifications = notifyCh
		errCh = errNotifyCh

		listener := func(_ context.Context, message []byte, listenErr error) {
			if listenErr != nil {
				select {
				case <-mergedCtx.Done():
				case errNotifyCh <- listenErr:
				}
				return
			}
			var notify coderdpubsub.ChatStreamNotifyMessage
			if unmarshalErr := json.Unmarshal(message, &notify); unmarshalErr != nil {
				select {
				case <-mergedCtx.Done():
				case errNotifyCh <- xerrors.Errorf("unmarshal chat stream notify: %w", unmarshalErr):
				}
				return
			}
			select {
			case <-mergedCtx.Done():
			case notifyCh <- notify:
			}
		}

		if pubsubCancel, pubsubErr := p.pubsub.SubscribeWithErr(
			coderdpubsub.ChatStreamNotifyChannel(chatID),
			listener,
		); pubsubErr == nil {
			allCancels = append(allCancels, pubsubCancel)
		} else {
			p.logger.Warn(ctx, "failed to subscribe to chat stream notifications",
				slog.F("chat_id", chatID),
				slog.Error(pubsubErr),
			)
		}
	}

	// Build initial snapshot synchronously. The pubsub subscription
	// is already active so no notifications can be lost during this
	// window.
	initialSnapshot := make([]codersdk.ChatStreamEvent, 0)
	// Add local message_parts to snapshot
	for _, event := range localSnapshot {
		if event.Type == codersdk.ChatStreamEventTypeMessagePart {
			initialSnapshot = append(initialSnapshot, event)
		}
	}

	// Load initial messages from DB. When afterMessageID > 0 the
	// caller already has messages up to that ID (e.g. from the REST
	// endpoint), so we only fetch newer ones to avoid sending
	// duplicate data.
	messages, err := p.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chatID,
		AfterID: afterMessageID,
	})
	if err != nil {
		p.logger.Error(ctx, "failed to load initial chat messages",
			slog.Error(err),
			slog.F("chat_id", chatID),
		)
		initialSnapshot = append(initialSnapshot, codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypeError,
			ChatID: chatID,
			Error:  &codersdk.ChatStreamError{Message: "failed to load initial snapshot"},
		})
	} else {
		for _, msg := range messages {
			sdkMsg := db2sdk.ChatMessage(msg)
			initialSnapshot = append(initialSnapshot, codersdk.ChatStreamEvent{
				Type:    codersdk.ChatStreamEventTypeMessage,
				ChatID:  chatID,
				Message: &sdkMsg,
			})
		}
	}

	// Load initial queue.
	queued, err := p.db.GetChatQueuedMessages(ctx, chatID)
	if err != nil {
		p.logger.Error(ctx, "failed to load initial queued messages",
			slog.Error(err),
			slog.F("chat_id", chatID),
		)
		initialSnapshot = append(initialSnapshot, codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypeError,
			ChatID: chatID,
			Error:  &codersdk.ChatStreamError{Message: "failed to load initial snapshot"},
		})
	} else if len(queued) > 0 {
		initialSnapshot = append(initialSnapshot, codersdk.ChatStreamEvent{
			Type:           codersdk.ChatStreamEventTypeQueueUpdate,
			ChatID:         chatID,
			QueuedMessages: db2sdk.ChatQueuedMessages(queued),
		})
	}

	// Get initial chat state to determine if we need a relay.
	chat, chatErr := p.db.GetChatByID(ctx, chatID)

	// Include the current chat status in the snapshot so the
	// frontend can gate message_part processing correctly from
	// the very first batch, without waiting for a separate REST
	// query.
	if chatErr != nil {
		p.logger.Error(ctx, "failed to load initial chat state",
			slog.Error(chatErr),
			slog.F("chat_id", chatID),
		)
		initialSnapshot = append(initialSnapshot, codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypeError,
			ChatID: chatID,
			Error:  &codersdk.ChatStreamError{Message: "failed to load initial snapshot"},
		})
	} else {
		statusEvent := codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypeStatus,
			ChatID: chatID,
			Status: &codersdk.ChatStreamStatus{
				Status: codersdk.ChatStatus(chat.Status),
			},
		}
		// Prepend so the frontend sees the status before any
		// message_part events.
		initialSnapshot = append([]codersdk.ChatStreamEvent{statusEvent}, initialSnapshot...)
	}

	// Track the highest durable message ID delivered to this subscriber,
	// whether it came from the initial DB snapshot, the same-replica local
	// stream, or a later DB/cache catch-up.
	lastMessageID := afterMessageID
	if len(messages) > 0 {
		lastMessageID = messages[len(messages)-1].ID
	}

	// When an enterprise SubscribeFn is provided and the chat
	// lookup succeeded, call it to get relay events (message_parts
	// from remote replicas). OSS now owns pubsub subscription,
	// message catch-up, queue updates, and status forwarding;
	// enterprise only manages relay dialing.
	var relayEvents <-chan codersdk.ChatStreamEvent
	var statusNotifications chan StatusNotification
	if p.subscribeFn != nil && chatErr == nil {
		statusNotifications = make(chan StatusNotification, 10)
		relayEvents = p.subscribeFn(mergedCtx, SubscribeFnParams{
			ChatID:              chatID,
			Chat:                chat,
			WorkerID:            p.workerID,
			StatusNotifications: statusNotifications,
			RequestHeader:       requestHeader,
			DB:                  p.db,
			Logger:              p.logger,
		})
	}
	hasPubsub := false
	if p.pubsub != nil {
		// hasPubsub is only true when we actually subscribed
		// successfully above (allCancels will contain the pubsub
		// cancel func in that case).
		hasPubsub = len(allCancels) > 1
	}

	//nolint:nestif
	go func() {
		defer close(mergedEvents)
		if statusNotifications != nil {
			defer close(statusNotifications)
		}
		for {
			select {
			case <-mergedCtx.Done():
				return
			case psErr := <-errCh:
				p.logger.Error(mergedCtx, "chat stream pubsub error",
					slog.F("chat_id", chatID),
					slog.Error(psErr),
				)
				select {
				case mergedEvents <- codersdk.ChatStreamEvent{
					Type:   codersdk.ChatStreamEventTypeError,
					ChatID: chatID,
					Error: &codersdk.ChatStreamError{
						Message: psErr.Error(),
					},
				}:
				case <-mergedCtx.Done():
				}
				return
			case notify := <-notifications:
				if notify.AfterMessageID > 0 || notify.FullRefresh {
					if notify.FullRefresh {
						lastMessageID = 0
					}
					cached := p.getCachedDurableMessages(chatID, lastMessageID)
					if !notify.FullRefresh && len(cached) > 0 {
						for _, event := range cached {
							select {
							case <-mergedCtx.Done():
								return
							case mergedEvents <- event:
							}
							lastMessageID = event.Message.ID
						}
					} else if newMessages, msgErr := p.db.GetChatMessagesByChatID(mergedCtx, database.GetChatMessagesByChatIDParams{
						ChatID:  chatID,
						AfterID: lastMessageID,
					}); msgErr != nil {
						p.logger.Warn(mergedCtx, "failed to get chat messages after pubsub notification",
							slog.F("chat_id", chatID),
							slog.Error(msgErr),
						)
					} else {
						for _, msg := range newMessages {
							if msg.ID <= lastMessageID {
								continue
							}
							sdkMsg := db2sdk.ChatMessage(msg)
							select {
							case <-mergedCtx.Done():
								return
							case mergedEvents <- codersdk.ChatStreamEvent{
								Type:    codersdk.ChatStreamEventTypeMessage,
								ChatID:  chatID,
								Message: &sdkMsg,
							}:
							}
							lastMessageID = msg.ID
						}
					}
				}
				if notify.Status != "" {
					status := database.ChatStatus(notify.Status)
					select {
					case <-mergedCtx.Done():
						return
					case mergedEvents <- codersdk.ChatStreamEvent{
						Type:   codersdk.ChatStreamEventTypeStatus,
						ChatID: chatID,
						Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatus(status)},
					}:
					}
					// Notify enterprise relay manager if present.
					if statusNotifications != nil {
						workerID := uuid.Nil
						if notify.WorkerID != "" {
							if parsed, parseErr := uuid.Parse(notify.WorkerID); parseErr == nil {
								workerID = parsed
							}
						}
						select {
						case statusNotifications <- StatusNotification{Status: status, WorkerID: workerID}:
						case <-mergedCtx.Done():
							return
						}
					}
				}
				if notify.Retry != nil {
					select {
					case <-mergedCtx.Done():
						return
					case mergedEvents <- codersdk.ChatStreamEvent{
						Type:   codersdk.ChatStreamEventTypeRetry,
						ChatID: chatID,
						Retry:  notify.Retry,
					}:
					}
				}
				if notify.ErrorPayload != nil {
					select {
					case <-mergedCtx.Done():
						return
					case mergedEvents <- codersdk.ChatStreamEvent{
						Type:   codersdk.ChatStreamEventTypeError,
						ChatID: chatID,
						Error:  notify.ErrorPayload,
					}:
					}
				} else if notify.Error != "" {
					select {
					case <-mergedCtx.Done():
						return
					case mergedEvents <- codersdk.ChatStreamEvent{
						Type:   codersdk.ChatStreamEventTypeError,
						ChatID: chatID,
						Error: &codersdk.ChatStreamError{
							Message: notify.Error,
						},
					}:
					}
				}
				if notify.QueueUpdate {
					queuedMsgs, queueErr := p.db.GetChatQueuedMessages(mergedCtx, chatID)
					if queueErr != nil {
						p.logger.Warn(mergedCtx, "failed to get queued messages after pubsub notification",
							slog.F("chat_id", chatID),
							slog.Error(queueErr),
						)
					} else {
						select {
						case <-mergedCtx.Done():
							return
						case mergedEvents <- codersdk.ChatStreamEvent{
							Type:           codersdk.ChatStreamEventTypeQueueUpdate,
							ChatID:         chatID,
							QueuedMessages: db2sdk.ChatQueuedMessages(queuedMsgs),
						}:
						}
					}
				}
			case event, ok := <-localParts:
				if !ok {
					localParts = nil
					// Local parts channel closed. If pubsub is
					// active we continue with pubsub-driven events.
					// Otherwise terminate.
					if !hasPubsub {
						return
					}
					continue
				}
				if hasPubsub {
					// Forward transient events from local.
					// Durable events (messages, queue updates)
					// come via pubsub + cache.  Status is
					// included alongside message_part because
					// both travel through the same ordered
					// channel: publishStatus is called before
					// the first message_part, so FIFO delivery
					// guarantees the frontend sees
					// status=running before any content.
					// Pubsub will deliver a duplicate status
					// later; the frontend deduplicates it
					// (setChatStatus is idempotent).
					if event.Type == codersdk.ChatStreamEventTypeMessagePart ||
						event.Type == codersdk.ChatStreamEventTypeStatus {
						select {
						case <-mergedCtx.Done():
							return
						case mergedEvents <- event:
						}
					}
				} else {
					// No pubsub: forward all event types.
					select {
					case <-mergedCtx.Done():
						return
					case mergedEvents <- event:
					}
				}
			case event, ok := <-relayEvents:
				if !ok {
					relayEvents = nil
					continue
				}
				select {
				case <-mergedCtx.Done():
					return
				case mergedEvents <- event:
				}
			}
		}
	}()

	cancel := func() {
		mergedCancel()
		for _, cancelFn := range allCancels {
			if cancelFn != nil {
				cancelFn()
			}
		}
	}
	return initialSnapshot, mergedEvents, cancel, true
}

func (p *Server) publishEvent(chatID uuid.UUID, event codersdk.ChatStreamEvent) {
	if event.ChatID == uuid.Nil {
		event.ChatID = chatID
	}
	p.publishToStream(chatID, event)
}

func (p *Server) publishStatus(chatID uuid.UUID, status database.ChatStatus, workerID uuid.NullUUID) {
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeStatus,
		Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatus(status)},
	})
	notify := coderdpubsub.ChatStreamNotifyMessage{
		Status: string(status),
	}
	if workerID.Valid {
		notify.WorkerID = workerID.UUID.String()
	}
	p.publishChatStreamNotify(chatID, notify)
}

// publishChatStreamNotify broadcasts a per-chat stream notification via
// PostgreSQL pubsub so that all replicas can merge durable database updates
// with transient control events.
func (p *Server) publishChatStreamNotify(chatID uuid.UUID, notify coderdpubsub.ChatStreamNotifyMessage) {
	if p.pubsub == nil {
		return
	}
	payload, err := json.Marshal(notify)
	if err != nil {
		p.logger.Error(context.Background(), "failed to marshal chat stream notify",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
		return
	}
	if err := p.pubsub.Publish(coderdpubsub.ChatStreamNotifyChannel(chatID), payload); err != nil {
		p.logger.Error(context.Background(), "failed to publish chat stream notify",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
	}
}

// publishChatPubsubEvents broadcasts a lifecycle event for each affected chat.
func (p *Server) publishChatPubsubEvents(chats []database.Chat, kind coderdpubsub.ChatEventKind) {
	for _, chat := range chats {
		p.publishChatPubsubEvent(chat, kind, nil)
	}
}

// publishChatPubsubEvent broadcasts a chat lifecycle event via PostgreSQL
// pubsub so that all replicas can push updates to watching clients.
func (p *Server) publishChatPubsubEvent(chat database.Chat, kind coderdpubsub.ChatEventKind, diffStatus *codersdk.ChatDiffStatus) {
	if p.pubsub == nil {
		return
	}
	sdkChat := db2sdk.Chat(chat, nil) // we have diffStatus already converted
	if diffStatus != nil {
		sdkChat.DiffStatus = diffStatus
	}
	event := coderdpubsub.ChatEvent{
		Kind: kind,
		Chat: sdkChat,
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

// PublishDiffStatusChange broadcasts a diff_status_change event for
// the given chat so that watching clients know to re-fetch the diff
// status. This is called from the HTTP layer after the diff status
// is updated in the database.
func (p *Server) PublishDiffStatusChange(ctx context.Context, chatID uuid.UUID) error {
	if p.pubsub == nil {
		return nil
	}

	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("get chat: %w", err)
	}

	dbStatus, err := p.db.GetChatDiffStatusByChatID(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("get chat diff status: %w", err)
	}

	sdkStatus := db2sdk.ChatDiffStatus(chatID, &dbStatus)
	p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindDiffStatusChange, &sdkStatus)
	return nil
}

func (p *Server) publishRetry(chatID uuid.UUID, payload *codersdk.ChatStreamRetry) {
	if payload == nil {
		return
	}
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type:  codersdk.ChatStreamEventTypeRetry,
		Retry: payload,
	})
	p.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		Retry: payload,
	})
}

func (p *Server) publishError(chatID uuid.UUID, classified chaterror.ClassifiedError) {
	payload := chaterror.StreamErrorPayload(classified)
	if payload == nil {
		return
	}
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type:  codersdk.ChatStreamEventTypeError,
		Error: payload,
	})
	p.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		ErrorPayload: payload,
		Error:        payload.Message,
	})
}

func processingFailure(err error) (chaterror.ClassifiedError, bool) {
	if err == nil {
		return chaterror.ClassifiedError{}, false
	}

	classified := chaterror.Classify(err)
	if classified.Message == "" {
		return chaterror.ClassifiedError{}, false
	}
	return classified, true
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

func (p *Server) publishMessage(chatID uuid.UUID, message database.ChatMessage) {
	sdkMessage := db2sdk.ChatMessage(message)
	event := codersdk.ChatStreamEvent{
		Type:    codersdk.ChatStreamEventTypeMessage,
		ChatID:  chatID,
		Message: &sdkMessage,
	}
	p.cacheDurableMessage(chatID, event)
	p.publishEvent(chatID, event)
	p.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		AfterMessageID: message.ID - 1,
	})
}

// publishEditedMessage is like publishMessage but uses FullRefresh
// so remote subscribers re-fetch from the beginning, ensuring the
// edit is never silently dropped. The durable cache is replaced
// with only the edited message.
func (p *Server) publishEditedMessage(chatID uuid.UUID, message database.ChatMessage) {
	sdkMessage := db2sdk.ChatMessage(message)
	event := codersdk.ChatStreamEvent{
		Type:    codersdk.ChatStreamEventTypeMessage,
		ChatID:  chatID,
		Message: &sdkMessage,
	}
	state := p.getOrCreateStreamState(chatID)
	state.mu.Lock()
	state.durableMessages = []codersdk.ChatStreamEvent{event}
	state.durableEvictedBefore = 0
	state.mu.Unlock()
	p.publishEvent(chatID, event)
	p.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		FullRefresh: true,
	})
}

func (p *Server) publishMessagePart(chatID uuid.UUID, role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
	if part.Type == "" {
		return
	}
	// Strip internal-only fields before client delivery.
	// Mirrors db2sdk.chatMessageParts stripping for REST.
	part.StripInternal()
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role: role,
			Part: part,
		},
	})
}

func shouldCancelChatFromControlNotification(
	notify coderdpubsub.ChatStreamNotifyMessage,
	workerID uuid.UUID,
) bool {
	status := database.ChatStatus(strings.TrimSpace(notify.Status))
	switch status {
	case database.ChatStatusWaiting, database.ChatStatusPending, database.ChatStatusError:
		return true
	case database.ChatStatusRunning:
		worker := strings.TrimSpace(notify.WorkerID)
		if worker == "" {
			return false
		}
		notifyWorkerID, err := uuid.Parse(worker)
		if err != nil {
			return false
		}
		return notifyWorkerID != workerID
	default:
		return false
	}
}

func (p *Server) subscribeChatControl(
	ctx context.Context,
	chatID uuid.UUID,
	cancel context.CancelCauseFunc,
	logger slog.Logger,
) func() {
	if p.pubsub == nil {
		return nil
	}

	listener := func(_ context.Context, message []byte, err error) {
		if err != nil {
			logger.Warn(ctx, "chat control pubsub error", slog.Error(err))
			return
		}

		var notify coderdpubsub.ChatStreamNotifyMessage
		if unmarshalErr := json.Unmarshal(message, &notify); unmarshalErr != nil {
			logger.Warn(ctx, "failed to unmarshal chat control notify", slog.Error(unmarshalErr))
			return
		}

		if shouldCancelChatFromControlNotification(notify, p.workerID) {
			cancel(chatloop.ErrInterrupted)
		}
	}

	controlCancel, err := p.pubsub.SubscribeWithErr(
		coderdpubsub.ChatStreamNotifyChannel(chatID),
		listener,
	)
	if err != nil {
		logger.Warn(ctx, "failed to subscribe to chat control notifications", slog.Error(err))
		return nil
	}
	return controlCancel
}

// chatFileResolver returns a FileResolver that fetches chat file
// content from the database by ID.
func (p *Server) chatFileResolver() chatprompt.FileResolver {
	return func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]chatprompt.FileData, error) {
		files, err := p.db.GetChatFilesByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		result := make(map[uuid.UUID]chatprompt.FileData, len(files))
		for _, f := range files {
			result[f.ID] = chatprompt.FileData{
				Name:      f.Name,
				Data:      f.Data,
				MediaType: f.Mimetype,
			}
		}
		return result, nil
	}
}

// tryAutoPromoteQueuedMessage pops the next queued message and converts it
// into a pending user message inside the caller's transaction. Queued
// messages were already admitted through SendMessage, so this preserves FIFO
// order without re-checking usage limits.
func (p *Server) tryAutoPromoteQueuedMessage(
	ctx context.Context,
	tx database.Store,
	chat database.Chat,
) (*database.ChatMessage, []database.ChatQueuedMessage, bool, error) {
	logger := p.logger.With(slog.F("chat_id", chat.ID))

	nextQueued, err := tx.PopNextQueuedMessage(ctx, chat.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, xerrors.Errorf("pop next queued message: %w", err)
	}

	msgParams := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
		ChatID: chat.ID,
	}
	appendChatMessage(&msgParams, newChatMessage(
		database.ChatMessageRoleUser,
		pqtype.NullRawMessage{
			RawMessage: nextQueued.Content,
			Valid:      len(nextQueued.Content) > 0,
		},
		database.ChatMessageVisibilityBoth,
		chat.LastModelConfigID,
		chatprompt.CurrentContentVersion,
	).withCreatedBy(chat.OwnerID))
	msgs, err := insertChatMessageWithStore(ctx, tx, msgParams)
	if err != nil {
		logger.Error(ctx, "failed to promote queued message",
			slog.F("queued_message_id", nextQueued.ID), slog.Error(err))
		return nil, nil, false, nil
	}
	msg := msgs[0]

	remainingQueuedMessages, err := tx.GetChatQueuedMessages(ctx, chat.ID)
	if err != nil {
		logger.Error(ctx, "failed to load remaining queued messages after auto-promotion",
			slog.F("queued_message_id", nextQueued.ID), slog.Error(err))
		return &msg, nil, false, nil
	}

	return &msg, remainingQueuedMessages, true, nil
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
		// writes when 5% of the deadline has elapsed — most calls
		// perform a read-only CTE lookup with no UPDATE.
		//
		// Scaling note: for 10,000 active chats, this could lead to
		// approx. 333 CTE queries/second. A cheap fix for this could
		// be to heartbeat every Nth query. Leaving as potential future
		// low-hanging fruit if needed.
		workspacestats.ActivityBumpWorkspace(ctx, logger.Named("activity_bump"), p.db, wsID.UUID, time.Time{})
	}
	return wsID
}

func (p *Server) processChat(ctx context.Context, chat database.Chat) {
	logger := p.logger.With(slog.F("chat_id", chat.ID))
	logger.Info(ctx, "processing chat request")

	chatCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	// Gate the control subscriber behind a channel that is closed
	// after we publish "running" status. This prevents stale
	// pubsub notifications (e.g. the "pending" notification from
	// SendMessage that triggered this processing) from
	// interrupting us before we start work. Due to async
	// PostgreSQL NOTIFY delivery, a notification published before
	// subscribeChatControl registers its queue can still arrive
	// after registration.
	controlArmed := make(chan struct{})
	gatedCancel := func(cause error) {
		select {
		case <-controlArmed:
			cancel(cause)
		default:
			logger.Debug(ctx, "ignoring control notification before armed")
		}
	}

	controlCancel := p.subscribeChatControl(chatCtx, chat.ID, gatedCancel, logger)
	defer func() {
		if controlCancel != nil {
			controlCancel()
		}
	}()

	// Periodically update the heartbeat so other replicas know this
	// worker is still alive. The goroutine stops when chatCtx is
	// canceled (either by completion or interruption).
	go func() {
		ticker := p.clock.NewTicker(p.chatHeartbeatInterval, "chatd", "heartbeat")
		defer ticker.Stop()
		for {
			select {
			case <-chatCtx.Done():
				return
			case <-ticker.C:
				rows, err := p.db.UpdateChatHeartbeat(chatCtx, database.UpdateChatHeartbeatParams{
					ID:       chat.ID,
					WorkerID: p.workerID,
				})
				if err != nil {
					logger.Warn(chatCtx, "failed to update chat heartbeat", slog.Error(err))
					continue
				}
				if rows == 0 {
					cancel(chatloop.ErrInterrupted)
					return
				}
				chat.WorkspaceID = p.trackWorkspaceUsage(chatCtx, chat.ID, chat.WorkspaceID, logger)
			}
		}
	}()

	// Start buffering stream events BEFORE publishing the running
	// status. This closes a race where a subscriber sees
	// status=running but misses message_part events because
	// buffering hasn't started yet — the subscriber gets an empty
	// snapshot and publishToStream drops message_parts while
	// buffering is false.
	streamState := p.getOrCreateStreamState(chat.ID)
	streamState.mu.Lock()
	streamState.buffer = nil
	streamState.bufferRetainedAt = time.Time{}
	streamState.resetDropCounters()
	streamState.buffering = true
	streamState.mu.Unlock()
	defer func() {
		streamState.mu.Lock()
		streamState.resetDropCounters()
		streamState.buffering = false
		// Retain the buffer for a grace period so
		// cross-replica relay subscribers can still snapshot
		// it after processing completes. The buffer is
		// cleared when the next processChat starts or when
		// cleanupStreamIfIdle runs after the grace period.
		streamState.bufferRetainedAt = p.clock.Now()
		streamState.mu.Unlock()
	}()

	p.publishStatus(chat.ID, database.ChatStatusRunning, uuid.NullUUID{
		UUID:  p.workerID,
		Valid: true,
	})

	// Arm the control subscriber. Closing the channel is a
	// happens-before guarantee in the Go memory model — any
	// notification dispatched after this point will correctly
	// interrupt processing.
	close(controlArmed)

	// Determine the final status and last error to set when we're done.
	status := database.ChatStatusWaiting
	wasInterrupted := false
	lastError := ""
	generatedTitle := &generatedChatTitle{}
	runResult := runChatResult{}
	remainingQueuedMessages := []database.ChatQueuedMessage{}
	shouldPublishQueueUpdate := false
	var promotedMessage *database.ChatMessage

	defer func() {
		// Use a context that is not canceled by Close() so we can
		// reliably update the chat status in the database during
		// graceful shutdown.
		cleanupCtx := context.WithoutCancel(ctx)

		// Handle panics gracefully.
		if r := recover(); r != nil {
			logger.Error(cleanupCtx, "panic during chat processing", slog.F("panic", r))
			lastError = panicFailureReason(r)
			p.publishError(chat.ID, chaterror.ClassifiedError{
				Message: lastError,
				Kind:    chaterror.KindGeneric,
			})
			status = database.ChatStatusError
		}

		// Check for queued messages and auto-promote the next one.
		// This must be done atomically with the status update to avoid
		// races with the promote endpoint (which also sets status to
		// pending). We use a transaction with FOR UPDATE to ensure we
		// don't overwrite a status change made by another caller.
		var updatedChat database.Chat
		err := p.db.InTx(func(tx database.Store) error {
			// Re-read the chat status under lock — another caller
			// (e.g. promote) may have already set it to pending.
			latestChat, lockErr := tx.GetChatByIDForUpdate(cleanupCtx, chat.ID)
			if lockErr != nil {
				return xerrors.Errorf("lock chat for release: %w", lockErr)
			}

			// If another worker has already acquired this chat,
			// bail out — we must not overwrite their running
			// status or publish spurious events.
			if latestChat.Status == database.ChatStatusRunning &&
				latestChat.WorkerID.Valid &&
				latestChat.WorkerID.UUID != p.workerID {
				return errChatTakenByOtherWorker
			}

			// If someone else already set the chat to pending (e.g.
			// the promote endpoint), don't overwrite it — just clear
			// the worker and let the processor pick it back up.
			if latestChat.Status == database.ChatStatusPending {
				status = database.ChatStatusPending
			} else if status == database.ChatStatusWaiting && !latestChat.Archived {
				// Queued messages were already admitted through SendMessage,
				// so auto-promotion only preserves FIFO order here. Archived
				// chats skip promotion so archiving behaves like a hard stop.
				var promoteErr error
				promotedMessage, remainingQueuedMessages, shouldPublishQueueUpdate, promoteErr = p.tryAutoPromoteQueuedMessage(cleanupCtx, tx, latestChat)
				if promoteErr != nil {
					logger.Error(cleanupCtx, "failed to auto-promote queued message", slog.Error(promoteErr))
				} else if promotedMessage != nil {
					status = database.ChatStatusPending
				}
			}

			var updateErr error
			updatedChat, updateErr = tx.UpdateChatStatus(cleanupCtx, database.UpdateChatStatusParams{
				ID:          chat.ID,
				Status:      status,
				WorkerID:    uuid.NullUUID{},
				StartedAt:   sql.NullTime{},
				HeartbeatAt: sql.NullTime{},
				LastError:   sql.NullString{String: lastError, Valid: lastError != ""},
			})
			return updateErr
		}, nil)
		if errors.Is(err, errChatTakenByOtherWorker) {
			// Another worker owns this chat now — skip all
			// post-TX side effects (status publish, pubsub,
			// web push) to avoid overwriting their state.
			return
		}
		if err != nil {
			logger.Error(cleanupCtx, "failed to release chat", slog.Error(err))
			return
		}

		if promotedMessage != nil {
			p.publishMessage(chat.ID, *promotedMessage)
		}
		if shouldPublishQueueUpdate {
			p.publishEvent(chat.ID, codersdk.ChatStreamEvent{
				Type:           codersdk.ChatStreamEventTypeQueueUpdate,
				QueuedMessages: db2sdk.ChatQueuedMessages(remainingQueuedMessages),
			})
			p.publishChatStreamNotify(chat.ID, coderdpubsub.ChatStreamNotifyMessage{
				QueueUpdate: true,
			})
		}

		p.publishStatus(chat.ID, status, uuid.NullUUID{})
		// Best-effort: use any generated title captured during
		// processing so push notifications and the status snapshot
		// can reflect it without another DB read. The dedicated
		// title_change event remains the source of truth.
		if title, ok := generatedTitle.Load(); ok {
			updatedChat.Title = title
		}
		p.publishChatPubsubEvent(updatedChat, coderdpubsub.ChatEventKindStatusChange, nil)

		if !wasInterrupted {
			p.maybeSendPushNotification(cleanupCtx, updatedChat, status, lastError, runResult, logger)
		}
	}()

	runResult, err := p.runChat(chatCtx, chat, generatedTitle, logger)
	if err != nil {
		if errors.Is(err, chatloop.ErrInterrupted) || errors.Is(context.Cause(chatCtx), chatloop.ErrInterrupted) {
			logger.Info(ctx, "chat interrupted")
			status = database.ChatStatusWaiting
			wasInterrupted = true
			return
		}
		if isShutdownCancellation(ctx, chatCtx, err) {
			logger.Info(ctx, "chat canceled during shutdown; returning to pending")
			status = database.ChatStatusPending
			lastError = ""
			return
		}
		logger.Error(ctx, "failed to process chat", slog.Error(err))
		if classified, ok := processingFailure(err); ok {
			lastError = classified.Message
			p.publishError(chat.ID, classified)
		}
		status = database.ChatStatusError
		return
	}

	// If runChat completed successfully but the server context was
	// canceled (e.g. during Close()), the chat should be returned
	// to pending so another replica can pick it up. There is a
	// race where the LLM stream finishes just as the server is
	// shutting down — the HTTP response completes before context
	// cancellation propagates, so runChat returns nil instead of
	// a context.Canceled error. Without this check the chat would
	// be marked "waiting" and never retried.
	if ctx.Err() != nil {
		logger.Info(ctx, "chat completed during shutdown; returning to pending")
		status = database.ChatStatusPending
		lastError = ""
		return
	}
}

func isShutdownCancellation(
	serverCtx context.Context,
	chatCtx context.Context,
	err error,
) bool {
	if err == nil {
		return false
	}
	// During Close(), the server context is canceled. In-flight chats should
	// be returned to pending so another replica can retry them.
	if serverCtx.Err() == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	return errors.Is(context.Cause(chatCtx), context.Canceled)
}

// generatedChatTitle shares an asynchronously generated title between the
// detached title-generation goroutine and the deferred cleanup path.
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

type runChatResult struct {
	FinalAssistantText string
	PushSummaryModel   fantasy.LanguageModel
	ProviderKeys       chatprovider.ProviderAPIKeys
}

func (p *Server) runChat(
	ctx context.Context,
	chat database.Chat,
	generatedTitle *generatedChatTitle,
	logger slog.Logger,
) (runChatResult, error) {
	result := runChatResult{}
	var (
		model        fantasy.LanguageModel
		modelConfig  database.ChatModelConfig
		providerKeys chatprovider.ProviderAPIKeys
		callConfig   codersdk.ChatModelCallConfig
		messages     []database.ChatMessage
	)

	// Load MCP server configs and user tokens in parallel with
	// model resolution and message loading. These queries have
	// no dependencies on each other and all hit different tables.
	var (
		mcpConfigs []database.MCPServerConfig
		mcpTokens  []database.MCPServerUserToken
	)
	var g errgroup.Group
	g.Go(func() error {
		var err error
		model, modelConfig, providerKeys, err = p.resolveChatModel(ctx, chat)
		if err != nil {
			return err
		}
		if len(modelConfig.Options) > 0 {
			if err := json.Unmarshal(modelConfig.Options, &callConfig); err != nil {
				return xerrors.Errorf("parse model call config: %w", err)
			}
		}
		return nil
	})
	g.Go(func() error {
		var err error
		messages, err = p.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("get chat messages: %w", err)
		}
		return nil
	})
	if len(chat.MCPServerIDs) > 0 {
		g.Go(func() error {
			var err error
			mcpConfigs, err = p.db.GetMCPServerConfigsByIDs(
				ctx, chat.MCPServerIDs,
			)
			if err != nil {
				logger.Warn(ctx,
					"failed to load MCP server configs",
					slog.Error(err),
				)
			}
			return nil
		})
		g.Go(func() error {
			var err error
			// If token loading fails, ConnectAll will still
			// proceed but oauth2-authenticated servers will
			// attempt to connect without credentials. Those
			// connections may succeed or fail depending on
			// the remote server's auth requirements.
			mcpTokens, err = p.db.GetMCPServerUserTokensByUserID(
				ctx, chat.OwnerID,
			)
			if err != nil {
				logger.Warn(ctx,
					"failed to load MCP user tokens",
					slog.Error(err),
				)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return result, err
	}
	chainInfo := resolveChainMode(messages)
	result.PushSummaryModel = model
	result.ProviderKeys = providerKeys
	// Fire title generation asynchronously so it doesn't block the
	// chat response. It uses a detached context so it can finish
	// even after the chat processing context is canceled.
	// Snapshot the original chat model so the goroutine doesn't
	// race with the model = cuModel reassignment below.
	titleModel := result.PushSummaryModel
	p.inflight.Add(1)
	go func() {
		defer p.inflight.Done()
		p.maybeGenerateChatTitle(
			context.WithoutCancel(ctx),
			chat,
			messages,
			titleModel,
			providerKeys,
			generatedTitle,
			logger,
		)
	}()

	prompt, err := chatprompt.ConvertMessagesWithFiles(ctx, messages, p.chatFileResolver(), logger)
	if err != nil {
		return result, xerrors.Errorf("build chat prompt: %w", err)
	}
	if chat.ParentChatID.Valid {
		prompt = chatprompt.InsertSystem(prompt, defaultSubagentInstruction)
	}

	// Detect computer-use subagent via the mode column.
	isComputerUse := chat.Mode.Valid && chat.Mode.ChatMode == database.ChatModeComputerUse

	// NOTE: Buffering was already started in processChat before
	// the running status was published, so message_part events
	// are captured from the moment subscribers can see
	// status=running. The deferred cleanup also lives in
	// processChat.

	currentChat := chat
	loadChatSnapshot := func(
		loadCtx context.Context,
		chatID uuid.UUID,
	) (database.Chat, error) {
		return p.db.GetChatByID(loadCtx, chatID)
	}
	var (
		chatStateMu sync.Mutex
		workspaceMu sync.Mutex
	)
	workspaceCtx := turnWorkspaceContext{
		server:           p,
		chatStateMu:      &chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: loadChatSnapshot,
	}
	defer workspaceCtx.close()

	// Connect to MCP servers in parallel with instruction
	// resolution. ConnectAll only depends on mcpConfigs and
	// mcpTokens which are available after g.Wait() above.
	var (
		instruction        string
		resolvedUserPrompt string
		mcpTools           []fantasy.AgentTool
		mcpCleanup         func()
		workspaceMCPTools  []fantasy.AgentTool
		skills             []chattool.SkillMeta
	)
	// Check if instruction files need to be (re-)persisted.
	// This happens when no context-file parts exist yet, or when
	// the workspace agent has changed (e.g. workspace rebuilt).
	needsInstructionPersist := false
	hasContextFiles := false
	if chat.WorkspaceID.Valid {
		persistedAgentID, found := contextFileAgentID(messages)
		hasContextFiles = found
		if !hasContextFiles {
			needsInstructionPersist = true
		} else if agent, agentErr := workspaceCtx.getWorkspaceAgent(ctx); agentErr == nil && agent.ID != persistedAgentID {
			// Agent changed — persist fresh instruction files.
			// Old context-file messages remain in the conversation
			// to preserve the prompt cache prefix.
			needsInstructionPersist = true
		}
	}
	var g2 errgroup.Group
	if needsInstructionPersist {
		g2.Go(func() error {
			var persistErr error
			instruction, skills, persistErr = p.persistInstructionFiles(
				ctx,
				chat,
				modelConfig.ID,
				workspaceCtx.getWorkspaceAgent,
				func(instructionCtx context.Context) (workspacesdk.AgentConn, error) {
					if _, _, err := workspaceCtx.workspaceAgentIDForConn(instructionCtx); err != nil {
						return nil, err
					}
					return workspaceCtx.getWorkspaceConn(instructionCtx)
				},
			)
			if persistErr != nil {
				p.logger.Warn(ctx, "failed to persist instruction files",
					slog.F("chat_id", chat.ID),
					slog.Error(persistErr),
				)
			}
			return nil
		})
	} else if hasContextFiles {
		// On subsequent turns, extract the instruction text and
		// skill index from persisted parts so they can be
		// re-injected via InsertSystem after compaction drops
		// those messages. No workspace dial needed.
		instruction = instructionFromContextFiles(messages)
		skills = skillsFromParts(messages)
	}
	g2.Go(func() error {
		resolvedUserPrompt = p.resolveUserPrompt(ctx, chat.OwnerID)
		return nil
	})
	if len(mcpConfigs) > 0 {
		g2.Go(func() error {
			// Refresh expired OAuth2 tokens before connecting.
			mcpTokens = p.refreshExpiredMCPTokens(ctx, logger, mcpConfigs, mcpTokens)
			mcpTools, mcpCleanup = mcpclient.ConnectAll(
				ctx, logger, mcpConfigs, mcpTokens,
			)
			return nil
		})
	}
	if chat.WorkspaceID.Valid {
		g2.Go(func() error {
			// Fast path: check cache using the in-memory cached
			// agent (ensureWorkspaceAgent is free when already
			// loaded). This avoids a per-turn latest-build DB
			// query on the common subsequent-turn path.
			agent, agentErr := workspaceCtx.getWorkspaceAgent(ctx)
			if agentErr == nil {
				if workspaceMCPTools = p.loadCachedWorkspaceContext(
					chat.ID, agent, workspaceCtx.getWorkspaceConn,
				); workspaceMCPTools != nil {
					return nil
				}
			} // Cache miss, agent changed, or no cache: validate
			// that the workspace still has a live agent before
			// attempting a dial.
			workspaceMCPCtx, cancel := context.WithTimeout(
				ctx,
				workspaceMCPDiscoveryTimeout,
			)
			defer cancel()

			_, _, agentErr = workspaceCtx.workspaceAgentIDForConn(workspaceMCPCtx)
			if agentErr != nil {
				if xerrors.Is(agentErr, errChatHasNoWorkspaceAgent) {
					p.workspaceMCPToolsCache.Delete(chat.ID)
					return nil
				}
				logger.Warn(ctx, "failed to resolve workspace agent for MCP tools",
					slog.Error(agentErr))
				return nil
			}

			// List workspace MCP tools via the agent conn.
			conn, connErr := workspaceCtx.getWorkspaceConn(workspaceMCPCtx)
			if connErr != nil {
				logger.Warn(ctx, "failed to get workspace conn for MCP tools",
					slog.Error(connErr))
				return nil
			}
			toolsResp, listErr := conn.ListMCPTools(workspaceMCPCtx)
			if listErr != nil {
				logger.Warn(ctx, "failed to list workspace MCP tools",
					slog.Error(listErr))
				return nil
			}
			// Cache the result for subsequent turns. Skip
			// caching when the list is empty because the
			// agent's MCP Connect may not have finished yet;
			// caching an empty list would hide tools
			// permanently.
			if len(toolsResp.Tools) > 0 {
				if agent, agentErr := workspaceCtx.getWorkspaceAgent(workspaceMCPCtx); agentErr == nil {
					p.workspaceMCPToolsCache.Store(chat.ID, &cachedWorkspaceMCPTools{
						agentID: agent.ID,
						tools:   toolsResp.Tools,
					})
				}
			}

			for _, t := range toolsResp.Tools {
				workspaceMCPTools = append(workspaceMCPTools,
					chattool.NewWorkspaceMCPTool(t, workspaceCtx.getWorkspaceConn),
				)
			}
			return nil
		})
	}
	// All g2 goroutines return nil; error is discarded.
	_ = g2.Wait()
	if mcpCleanup != nil {
		defer mcpCleanup()
	}

	// Build a lookup from tool name to MCP server config ID
	// so we can annotate persisted parts with the originating
	// server.
	toolNameToConfigID := make(map[string]uuid.UUID)
	for _, t := range mcpTools {
		if mcp, ok := t.(mcpclient.MCPToolIdentifier); ok {
			toolNameToConfigID[t.Info().Name] = mcp.MCPServerConfigID()
		}
	}

	if instruction != "" {
		prompt = chatprompt.InsertSystem(prompt, instruction)
	}
	if skillIndex := chattool.FormatSkillIndex(skills); skillIndex != "" {
		prompt = chatprompt.InsertSystem(prompt, skillIndex)
	}
	if resolvedUserPrompt != "" {
		prompt = chatprompt.InsertSystem(prompt, resolvedUserPrompt)
	}

	// Use the model config's context_limit as a fallback when the LLM
	// provider doesn't include context_limit in its response metadata
	// (which is the common case).
	modelConfigContextLimit := modelConfig.ContextLimit
	var finalAssistantText string

	persistStep := func(persistCtx context.Context, step chatloop.PersistedStep) error {
		// If the chat context has been canceled, bail out before
		// inserting any messages. We distinguish the cause so that
		// the caller can tell an intentional interruption (e.g.
		// EditMessage, user stop) from a server shutdown:
		//   - ErrInterrupted cause → return ErrInterrupted
		//     (processChat sets status = waiting).
		//   - Any other cause (e.g. context.Canceled during
		//     Close()) → return the original context error so
		//     isShutdownCancellation can match and set status =
		//     pending, allowing another replica to retry.
		if persistCtx.Err() != nil {
			if errors.Is(context.Cause(persistCtx), chatloop.ErrInterrupted) {
				return chatloop.ErrInterrupted
			}
			return persistCtx.Err()
		}

		// Split the step content into assistant blocks and tool
		// result blocks so they can be stored as separate messages
		// with the appropriate roles. Provider-executed tool results
		// (e.g. web_search) stay in the assistant content because
		// the LLM provider expects them inline in the assistant
		// turn, not as separate tool messages.
		var assistantBlocks []fantasy.Content
		var toolResults []fantasy.ToolResultContent
		for _, block := range step.Content {
			if tr, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok {
				if !tr.ProviderExecuted {
					toolResults = append(toolResults, tr)
					continue
				}
			}
			if trPtr, ok := fantasy.AsContentType[*fantasy.ToolResultContent](block); ok && trPtr != nil {
				if !trPtr.ProviderExecuted {
					toolResults = append(toolResults, *trPtr)
					continue
				}
			}
			assistantBlocks = append(assistantBlocks, block)
		}

		// Pre-marshal all content outside the transaction so the
		// FOR UPDATE lock is held only for the INSERT statements.
		// Marshaling is pure CPU work with no database dependency.
		var assistantContent pqtype.NullRawMessage
		if len(assistantBlocks) > 0 {
			sdkParts := make([]codersdk.ChatMessagePart, 0, len(assistantBlocks))
			for _, block := range assistantBlocks {
				part := chatprompt.PartFromContent(block)
				if part.ToolName != "" {
					if configID, ok := toolNameToConfigID[part.ToolName]; ok {
						part.MCPServerConfigID = uuid.NullUUID{UUID: configID, Valid: true}
					}
				}
				sdkParts = append(sdkParts, part)
			}
			finalAssistantText = strings.TrimSpace(contentBlocksToText(sdkParts))
			var marshalErr error
			assistantContent, marshalErr = chatprompt.MarshalParts(sdkParts)
			if marshalErr != nil {
				return xerrors.Errorf("marshal assistant content: %w", marshalErr)
			}
		}

		toolResultContents := make([]pqtype.NullRawMessage, len(toolResults))
		for i, tr := range toolResults {
			trPart := chatprompt.PartFromContent(tr)
			if trPart.ToolName != "" {
				if configID, ok := toolNameToConfigID[trPart.ToolName]; ok {
					trPart.MCPServerConfigID = uuid.NullUUID{UUID: configID, Valid: true}
				}
			}
			var marshalErr error
			toolResultContents[i], marshalErr = chatprompt.MarshalParts([]codersdk.ChatMessagePart{trPart})
			if marshalErr != nil {
				return xerrors.Errorf("marshal tool result %d: %w", i, marshalErr)
			}
		}

		hasUsage := step.Usage != (fantasy.Usage{})
		usageForCost := fantasyUsageToChatMessageUsage(step.Usage)
		totalCostMicros := chatcost.CalculateTotalCostMicros(usageForCost, callConfig.Cost)

		var insertedMessages []database.ChatMessage
		err := p.db.InTx(func(tx database.Store) error {
			// Verify this worker still owns the chat before
			// inserting messages. This closes the race where
			// EditMessage soft-deletes history and clears worker_id
			// while persistInterruptedStep (which uses an
			// uncancelable context) is still running.
			//
			// When the chat is in "waiting" status (set by
			// InterruptChat / setChatWaiting), the worker_id has
			// already been cleared but we still want to persist
			// the partial assistant response. We allow the write
			// because the history has NOT been truncated — the
			// user simply asked to stop. In contrast, EditMessage
			// sets the chat to "pending" after truncating, so the
			// pending check still correctly blocks stale writes.
			lockedChat, lockErr := tx.GetChatByIDForUpdate(persistCtx, chat.ID)
			if lockErr != nil {
				return xerrors.Errorf("lock chat for persist: %w", lockErr)
			}
			if !lockedChat.WorkerID.Valid || lockedChat.WorkerID.UUID != p.workerID {
				// The worker_id was cleared. Only allow the persist
				// if the chat transitioned to "waiting" (interrupt),
				// not "pending" (edit) or any other status.
				if lockedChat.Status != database.ChatStatusWaiting {
					return chatloop.ErrInterrupted
				}
			}

			stepParams := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
				ChatID: chat.ID,
			}

			var contextLimit int64
			if step.ContextLimit.Valid {
				contextLimit = step.ContextLimit.Int64
			}

			var runtimeMs int64
			if step.Runtime > 0 {
				runtimeMs = step.Runtime.Milliseconds()
			}

			var totalCostVal int64
			if totalCostMicros != nil {
				totalCostVal = *totalCostMicros
			}

			var inputTokens, outputTokens, totalTokens int64
			var reasoningTokens, cacheCreationTokens, cacheReadTokens int64
			if hasUsage {
				inputTokens = step.Usage.InputTokens
				outputTokens = step.Usage.OutputTokens
				totalTokens = step.Usage.TotalTokens
				reasoningTokens = step.Usage.ReasoningTokens
				cacheCreationTokens = step.Usage.CacheCreationTokens
				cacheReadTokens = step.Usage.CacheReadTokens
			}

			if assistantContent.Valid {
				appendChatMessage(&stepParams, newChatMessage(
					database.ChatMessageRoleAssistant,
					assistantContent,
					database.ChatMessageVisibilityBoth,
					modelConfig.ID,
					chatprompt.CurrentContentVersion,
				).withUsage(
					inputTokens, outputTokens, totalTokens,
					reasoningTokens, cacheCreationTokens, cacheReadTokens,
				).withContextLimit(contextLimit).
					withTotalCostMicros(totalCostVal).
					withRuntimeMs(runtimeMs).
					withProviderResponseID(step.ProviderResponseID))
			}

			for _, resultContent := range toolResultContents {
				appendChatMessage(&stepParams, newChatMessage(
					database.ChatMessageRoleTool,
					resultContent,
					database.ChatMessageVisibilityBoth,
					modelConfig.ID,
					chatprompt.CurrentContentVersion,
				))
			}

			if len(stepParams.Role) > 0 {
				inserted, insertErr := tx.InsertChatMessages(persistCtx, stepParams)
				if insertErr != nil {
					return xerrors.Errorf("insert step messages: %w", insertErr)
				}
				insertedMessages = append(insertedMessages, inserted...)
			}

			return nil
		}, nil)
		if err != nil {
			return xerrors.Errorf("persist step transaction: %w", err)
		}

		for _, msg := range insertedMessages {
			p.publishMessage(chat.ID, msg)
		}

		// Do NOT clear the stream buffer here. Cross-replica
		// relay subscribers may still need to snapshot buffered
		// message_parts after processing completes. The buffer
		// is bounded by maxStreamBufferSize and is cleared when
		// the next processChat starts or when the stream state
		// is garbage-collected after the retention grace period.

		return nil
	}
	// Apply the default MaxOutputTokens if the model config
	// does not specify one.
	if callConfig.MaxOutputTokens == nil {
		maxOutputTokens := int64(32_000)
		callConfig.MaxOutputTokens = &maxOutputTokens
	}

	// Generate the tool call ID up front so that the streaming
	// parts and durable messages share the same identifier.
	// Without this the client cannot correlate the
	// "Summarizing..." tool call with the "Summarized" tool
	// result.
	compactionToolCallID := "chat_summarized_" + uuid.NewString()
	effectiveThreshold := modelConfig.CompressionThreshold
	thresholdSource := "model_default"
	if override, ok := p.resolveUserCompactionThreshold(ctx, chat.OwnerID, modelConfig.ID); ok {
		effectiveThreshold = override
		thresholdSource = "user_override"
	}
	compactionOptions := &chatloop.CompactionOptions{
		ThresholdPercent: effectiveThreshold,
		ContextLimit:     modelConfig.ContextLimit,
		Persist: func(
			persistCtx context.Context,
			result chatloop.CompactionResult,
		) error {
			if err := p.persistChatContextSummary(
				persistCtx,
				chat.ID,
				modelConfig.ID,
				compactionToolCallID,
				result,
			); err != nil {
				return xerrors.Errorf("persist context summary: %w", err)
			}
			logger.Info(persistCtx, "chat context summarized",
				slog.F("chat_id", chat.ID),
				slog.F("threshold_source", thresholdSource),
				slog.F("threshold_percent", result.ThresholdPercent),
				slog.F("usage_percent", result.UsagePercent),
				slog.F("context_tokens", result.ContextTokens),
				slog.F("context_limit", result.ContextLimit),
			)
			return nil
		},
		ToolCallID: compactionToolCallID,
		ToolName:   "chat_summarized",
		PublishMessagePart: func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
			p.publishMessagePart(chat.ID, role, part)
		},
		OnError: func(err error) {
			logger.Warn(ctx, "failed to compact chat context", slog.Error(err))
		},
	}

	if isComputerUse {
		// Override model for computer use subagent.
		cuModel, cuErr := chatprovider.ModelFromConfig(
			chattool.ComputerUseModelProvider,
			chattool.ComputerUseModelName,
			providerKeys,
			chatprovider.UserAgent(),
			chatprovider.CoderHeaders(chat),
		)
		if cuErr != nil {
			return result, xerrors.Errorf("resolve computer use model: %w", cuErr)
		}
		model = cuModel
	}

	tools := []fantasy.AgentTool{
		chattool.ReadFile(chattool.ReadFileOptions{
			GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
		}),
		chattool.WriteFile(chattool.WriteFileOptions{
			GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
		}),
		chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
		}),
		chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
		}),
		chattool.ProcessOutput(chattool.ProcessToolOptions{
			GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
		}),
		chattool.ProcessList(chattool.ProcessToolOptions{
			GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
		}),
		chattool.ProcessSignal(chattool.ProcessToolOptions{
			GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
		}),
	}
	// Only root chats (not delegated subagents) get workspace
	// provisioning and subagent tools. Child agents must not
	// create workspaces or spawn further subagents — they should
	// focus on completing their delegated task.
	if !chat.ParentChatID.Valid {
		// Workspace provisioning tools.
		tools = append(tools,
			chattool.ListTemplates(chattool.ListTemplatesOptions{
				DB:                 p.db,
				OwnerID:            chat.OwnerID,
				AllowedTemplateIDs: p.chatTemplateAllowlist,
			}),
			chattool.ReadTemplate(chattool.ReadTemplateOptions{
				DB:                 p.db,
				OwnerID:            chat.OwnerID,
				AllowedTemplateIDs: p.chatTemplateAllowlist,
			}),
			chattool.CreateWorkspace(chattool.CreateWorkspaceOptions{
				DB:                             p.db,
				OwnerID:                        chat.OwnerID,
				ChatID:                         chat.ID,
				CreateFn:                       p.createWorkspaceFn,
				AgentConnFn:                    chattool.AgentConnFunc(p.agentConnFn),
				AgentInactiveDisconnectTimeout: p.agentInactiveDisconnectTimeout,
				WorkspaceMu:                    &workspaceMu,
				OnChatUpdated:                  workspaceCtx.selectWorkspace,
				Logger:                         p.logger,
				AllowedTemplateIDs:             p.chatTemplateAllowlist,
			}),
			chattool.StartWorkspace(chattool.StartWorkspaceOptions{
				DB:          p.db,
				OwnerID:     chat.OwnerID,
				ChatID:      chat.ID,
				StartFn:     p.startWorkspaceFn,
				AgentConnFn: chattool.AgentConnFunc(p.agentConnFn),
				WorkspaceMu: &workspaceMu,
			}),
		)
		// Plan presentation tool.
		tools = append(tools, chattool.ProposePlan(chattool.ProposePlanOptions{
			GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
			StoreFile: func(ctx context.Context, name string, mediaType string, data []byte) (uuid.UUID, error) {
				workspaceCtx.chatStateMu.Lock()
				chatSnapshot := *workspaceCtx.currentChat
				workspaceCtx.chatStateMu.Unlock()

				if !chatSnapshot.WorkspaceID.Valid {
					return uuid.Nil, xerrors.New("no workspace is associated with this chat. Use the create_workspace tool to create one")
				}

				ws, err := p.db.GetWorkspaceByID(ctx, chatSnapshot.WorkspaceID.UUID)
				if err != nil {
					return uuid.Nil, xerrors.Errorf("resolve workspace: %w", err)
				}

				row, err := p.db.InsertChatFile(ctx, database.InsertChatFileParams{
					OwnerID:        chatSnapshot.OwnerID,
					OrganizationID: ws.OrganizationID,
					Name:           name,
					Mimetype:       mediaType,
					Data:           data,
				})
				if err != nil {
					return uuid.Nil, xerrors.Errorf("insert chat file: %w", err)
				}

				return row.ID, nil
			},
		}))
		tools = append(tools, p.subagentTools(ctx, func() database.Chat {
			return chat
		})...)
	}

	// Append skill tools when the workspace has skills.
	if len(skills) > 0 {
		skillOpts := chattool.ReadSkillOptions{
			GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
			GetSkills: func() []chattool.SkillMeta {
				return skills
			},
		}
		tools = append(tools,
			chattool.ReadSkill(skillOpts),
			chattool.ReadSkillFile(skillOpts),
		)
	}

	// Append tools from external MCP servers. These appear
	// after the built-in tools so the LLM sees them as
	// additional capabilities.
	tools = append(tools, mcpTools...)
	tools = append(tools, workspaceMCPTools...)

	// Build provider-native tools (e.g., web search) based on
	// the model configuration.
	var providerTools []chatloop.ProviderTool
	if callConfig.ProviderOptions != nil {
		providerTools = buildProviderTools(model.Provider(), callConfig.ProviderOptions)
	}

	if isComputerUse {
		desktopGeometry := workspacesdk.DefaultDesktopGeometry()
		providerTools = append(providerTools, chatloop.ProviderTool{
			Definition: chattool.ComputerUseProviderTool(
				desktopGeometry.DeclaredWidth,
				desktopGeometry.DeclaredHeight,
			),
			Runner: chattool.NewComputerUseTool(
				desktopGeometry.DeclaredWidth,
				desktopGeometry.DeclaredHeight,
				workspaceCtx.getWorkspaceConn,
				quartz.NewReal(),
			),
		})
	}

	providerOptions := chatprovider.ProviderOptionsFromChatModelConfig(
		model,
		callConfig.ProviderOptions,
	)
	// When the OpenAI Responses API has store=true, the provider
	// retains conversation history server-side. For follow-up turns,
	// we set previous_response_id and send only system instructions
	// plus the new user input, avoiding redundant replay of prior
	// assistant and tool messages that the provider already has.
	chainModeActive := chatprovider.IsResponsesStoreEnabled(providerOptions) &&
		chainInfo.previousResponseID != "" &&
		chainInfo.trailingUserCount > 0 &&
		chainInfo.modelConfigID == modelConfig.ID
	if chainModeActive {
		providerOptions = chatprovider.CloneWithPreviousResponseID(
			providerOptions,
			chainInfo.previousResponseID,
		)
		prompt = filterPromptForChainMode(prompt, chainInfo.trailingUserCount)
	}

	err = chatloop.Run(ctx, chatloop.RunOptions{
		Model:    model,
		Messages: prompt,
		Tools:    tools, MaxSteps: maxChatSteps,

		ModelConfig:     callConfig,
		ProviderOptions: providerOptions,
		ProviderTools:   providerTools,

		ContextLimitFallback: modelConfigContextLimit,

		PersistStep: persistStep,
		PublishMessagePart: func(
			role codersdk.ChatMessageRole,
			part codersdk.ChatMessagePart,
		) {
			if part.ToolName != "" {
				if configID, ok := toolNameToConfigID[part.ToolName]; ok {
					part.MCPServerConfigID = uuid.NullUUID{UUID: configID, Valid: true}
				}
			}
			p.publishMessagePart(chat.ID, role, part)
		},
		Compaction: compactionOptions,
		ReloadMessages: func(reloadCtx context.Context) ([]fantasy.Message, error) {
			reloadedMsgs, err := p.db.GetChatMessagesForPromptByChatID(reloadCtx, chat.ID)
			if err != nil {
				return nil, xerrors.Errorf("reload chat messages: %w", err)
			}
			reloadedPrompt, err := chatprompt.ConvertMessagesWithFiles(reloadCtx, reloadedMsgs, p.chatFileResolver(), logger)
			if err != nil {
				return nil, xerrors.Errorf("convert reloaded messages: %w", err)
			}
			if chat.ParentChatID.Valid {
				reloadedPrompt = chatprompt.InsertSystem(reloadedPrompt, defaultSubagentInstruction)
			}
			if instruction != "" {
				reloadedPrompt = chatprompt.InsertSystem(reloadedPrompt, instruction)
			}
			if skillIndex := chattool.FormatSkillIndex(skills); skillIndex != "" {
				reloadedPrompt = chatprompt.InsertSystem(reloadedPrompt, skillIndex)
			}
			reloadUserPrompt := p.resolveUserPrompt(reloadCtx, chat.OwnerID)
			if reloadUserPrompt != "" {
				reloadedPrompt = chatprompt.InsertSystem(reloadedPrompt, reloadUserPrompt)
			}
			if chainModeActive {
				reloadedPrompt = filterPromptForChainMode(
					reloadedPrompt,
					chainInfo.trailingUserCount,
				)
			}
			return reloadedPrompt, nil
		},
		DisableChainMode: func() {
			chainModeActive = false
		},

		OnRetry: func(
			attempt int,
			retryErr error,
			classified chatretry.ClassifiedError,
			delay time.Duration,
		) {
			if val, ok := p.chatStreams.Load(chat.ID); ok {
				if rs, ok := val.(*chatStreamState); ok {
					rs.mu.Lock()
					rs.buffer = nil
					rs.resetDropCounters()
					rs.mu.Unlock()
				}
			}
			logger.Warn(ctx, "retrying LLM stream",
				slog.F("attempt", attempt),
				slog.F("delay", delay.String()),
				slog.Error(retryErr),
			)
			payload := chaterror.StreamRetryPayload(attempt, delay, classified)
			p.publishRetry(chat.ID, payload)
		},

		OnInterruptedPersistError: func(err error) {
			p.logger.Warn(ctx, "failed to persist interrupted chat step", slog.Error(err))
		},
	})
	if err != nil {
		classified := chaterror.Classify(err).WithProvider(model.Provider())
		return result, chaterror.WithClassification(err, classified)
	}
	result.FinalAssistantText = finalAssistantText
	return result, nil
}

// buildProviderTools creates provider-native tool definitions
// (like web search) based on the model configuration. These
// tools are executed server-side by the LLM provider.
func buildProviderTools(_ string, options *codersdk.ChatModelProviderOptions) []chatloop.ProviderTool {
	var tools []chatloop.ProviderTool

	if options.Anthropic != nil && options.Anthropic.WebSearchEnabled != nil && *options.Anthropic.WebSearchEnabled {
		tools = append(tools, chatloop.ProviderTool{
			Definition: anthropic.WebSearchTool(&anthropic.WebSearchToolOptions{
				AllowedDomains: options.Anthropic.AllowedDomains,
				BlockedDomains: options.Anthropic.BlockedDomains,
			}),
		})
	}

	if options.OpenAI != nil && options.OpenAI.WebSearchEnabled != nil && *options.OpenAI.WebSearchEnabled {
		args := map[string]any{}
		if options.OpenAI.SearchContextSize != nil && *options.OpenAI.SearchContextSize != "" {
			args["search_context_size"] = *options.OpenAI.SearchContextSize
		}
		if len(options.OpenAI.AllowedDomains) > 0 {
			args["allowed_domains"] = options.OpenAI.AllowedDomains
		}
		tools = append(tools, chatloop.ProviderTool{
			Definition: fantasy.ProviderDefinedTool{
				ID:   "web_search",
				Name: "web_search",
				Args: args,
			},
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

// persistChatContextSummary persists a chat context summary to the database.
// This is invoked via the chat loop's compaction callback.
func (p *Server) persistChatContextSummary(
	ctx context.Context,
	chatID uuid.UUID,
	modelConfigID uuid.UUID,
	toolCallID string,
	result chatloop.CompactionResult,
) error {
	if strings.TrimSpace(result.SystemSummary) == "" ||
		strings.TrimSpace(result.SummaryReport) == "" {
		return nil
	}

	systemContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText(result.SystemSummary),
	})
	if err != nil {
		return xerrors.Errorf("encode system summary: %w", err)
	}

	args, err := json.Marshal(map[string]any{
		"source":            "automatic",
		"threshold_percent": result.ThresholdPercent,
	})
	if err != nil {
		return xerrors.Errorf("encode summary tool args: %w", err)
	}

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolCall(toolCallID, "chat_summarized", args),
	})
	if err != nil {
		return xerrors.Errorf("encode summary tool call: %w", err)
	}

	summaryResult, err := json.Marshal(map[string]any{
		"summary":              result.SummaryReport,
		"source":               "automatic",
		"threshold_percent":    result.ThresholdPercent,
		"usage_percent":        result.UsagePercent,
		"context_tokens":       result.ContextTokens,
		"context_limit_tokens": result.ContextLimit,
	})
	if err != nil {
		return xerrors.Errorf("encode summary result payload: %w", err)
	}
	toolResult, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult(toolCallID, "chat_summarized", summaryResult, false, false),
	})
	if err != nil {
		return xerrors.Errorf("encode summary tool result: %w", err)
	}

	var insertedMessages []database.ChatMessage

	txErr := p.db.InTx(func(tx database.Store) error {
		summaryParams := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
			ChatID: chatID,
		}

		// Hidden summary user message (not published to subscribers).
		appendChatMessage(&summaryParams, newChatMessage(
			database.ChatMessageRoleUser,
			systemContent,
			database.ChatMessageVisibilityModel,
			modelConfigID,
			chatprompt.CurrentContentVersion,
		).withCompressed())

		// Assistant tool-call message.
		appendChatMessage(&summaryParams, newChatMessage(
			database.ChatMessageRoleAssistant,
			assistantContent,
			database.ChatMessageVisibilityUser,
			modelConfigID,
			chatprompt.CurrentContentVersion,
		).withCompressed())

		// Tool result message.
		appendChatMessage(&summaryParams, newChatMessage(
			database.ChatMessageRoleTool,
			toolResult,
			database.ChatMessageVisibilityBoth,
			modelConfigID,
			chatprompt.CurrentContentVersion,
		).withCompressed())

		allInserted, txErr := tx.InsertChatMessages(ctx, summaryParams)
		if txErr != nil {
			return xerrors.Errorf("insert summary messages: %w", txErr)
		}
		// Skip the first message (hidden summary user msg) when
		// publishing — only the assistant and tool messages are
		// visible to subscribers.
		insertedMessages = allInserted[1:]

		return nil
	}, nil)
	if txErr != nil {
		return txErr
	}

	// Publish after transaction commits to avoid notifying
	// subscribers about messages that could be rolled back.
	for _, msg := range insertedMessages {
		p.publishMessage(chatID, msg)
	}
	return nil
}

func (p *Server) resolveChatModel(
	ctx context.Context,
	chat database.Chat,
) (fantasy.LanguageModel, database.ChatModelConfig, chatprovider.ProviderAPIKeys, error) {
	var dbConfig database.ChatModelConfig
	var keys chatprovider.ProviderAPIKeys

	var g errgroup.Group
	g.Go(func() error {
		var err error
		dbConfig, err = p.resolveModelConfig(ctx, chat)
		if err != nil {
			return xerrors.Errorf("resolve model config: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		keys, err = p.resolveUserProviderAPIKeys(ctx, chat.OwnerID)
		if err != nil {
			return xerrors.Errorf("resolve provider API keys: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, database.ChatModelConfig{}, chatprovider.ProviderAPIKeys{}, err
	}

	model, err := chatprovider.ModelFromConfig(
		dbConfig.Provider, dbConfig.Model, keys, chatprovider.UserAgent(),
		chatprovider.CoderHeaders(chat),
	)
	if err != nil {
		return nil, database.ChatModelConfig{}, chatprovider.ProviderAPIKeys{}, xerrors.Errorf(
			"create model: %w", err,
		)
	}
	return model, dbConfig, keys, nil
}

func (p *Server) resolveUserProviderAPIKeys(
	ctx context.Context,
	ownerID uuid.UUID,
) (chatprovider.ProviderAPIKeys, error) {
	providers, err := p.configCache.EnabledProviders(ctx)
	if err != nil {
		return chatprovider.ProviderAPIKeys{}, xerrors.Errorf(
			"get enabled chat providers: %w",
			err,
		)
	}
	configuredProviders := make(
		[]chatprovider.ConfiguredProvider, 0, len(providers),
	)
	for _, provider := range providers {
		configuredProviders = append(
			configuredProviders, chatprovider.ConfiguredProvider{
				ProviderID:                 provider.ID,
				Provider:                   provider.Provider,
				APIKey:                     provider.APIKey,
				BaseURL:                    provider.BaseUrl,
				CentralAPIKeyEnabled:       provider.CentralApiKeyEnabled,
				AllowUserAPIKey:            provider.AllowUserApiKey,
				AllowCentralAPIKeyFallback: provider.AllowCentralApiKeyFallback,
			},
		)
	}
	allowAnyUserAPIKey := false
	for _, provider := range configuredProviders {
		if provider.AllowUserAPIKey {
			allowAnyUserAPIKey = true
			break
		}
	}

	userKeys := []chatprovider.UserProviderKey{}
	if allowAnyUserAPIKey {
		userKeyRows, err := p.db.GetUserChatProviderKeys(ctx, ownerID)
		if err != nil {
			return chatprovider.ProviderAPIKeys{}, xerrors.Errorf(
				"get user chat provider keys: %w",
				err,
			)
		}
		userKeys = make([]chatprovider.UserProviderKey, 0, len(userKeyRows))
		for _, userKey := range userKeyRows {
			userKeys = append(userKeys, chatprovider.UserProviderKey{
				ChatProviderID: userKey.ChatProviderID,
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
			return database.ChatModelConfig{}, xerrors.New(
				"no default chat model config is available",
			)
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

// contextFileAgentID extracts the workspace agent ID from the most
// recent persisted context-file parts. Returns uuid.Nil, false if no
// context-file parts exist.
func contextFileAgentID(messages []database.ChatMessage) (uuid.UUID, bool) {
	var lastID uuid.UUID
	found := false
	for _, msg := range messages {
		if !msg.Content.Valid || !bytes.Contains(msg.Content.RawMessage, []byte(`"context-file"`)) {
			continue
		}
		var parts []codersdk.ChatMessagePart
		if err := json.Unmarshal(msg.Content.RawMessage, &parts); err != nil {
			continue
		}
		for _, p := range parts {
			if p.Type == codersdk.ChatMessagePartTypeContextFile && p.ContextFileAgentID.Valid {
				lastID = p.ContextFileAgentID.UUID
				found = true
				break
			}
		}
	}
	return lastID, found
}

// persistInstructionFiles reads instruction files and discovers
// skills from the workspace agent, persisting both as message
// parts. This is called once when a workspace is first attached
// to a chat (or when the agent changes). Returns the formatted
// instruction string and skill index for injection into the
// current turn's prompt.
func (p *Server) persistInstructionFiles(
	ctx context.Context,
	chat database.Chat,
	modelConfigID uuid.UUID,
	getWorkspaceAgent func(context.Context) (database.WorkspaceAgent, error),
	getWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error),
) (instruction string, skills []chattool.SkillMeta, err error) {
	if !chat.WorkspaceID.Valid || getWorkspaceAgent == nil {
		return "", nil, nil
	}

	agent, err := getWorkspaceAgent(ctx)
	if err != nil {
		return "", nil, nil
	}

	directory := agent.ExpandedDirectory
	if directory == "" {
		directory = agent.Directory
	}

	// Fetch context configuration from the agent. Parts
	// arrive pre-populated with context-file and skill entries
	// so we don't need additional round-trips.
	var workspaceConnOK bool
	var agentParts []codersdk.ChatMessagePart

	if getWorkspaceConn != nil {
		instructionCtx, cancel := context.WithTimeout(ctx, p.instructionLookupTimeout)
		defer cancel()

		conn, connErr := getWorkspaceConn(instructionCtx)
		if connErr != nil {
			p.logger.Debug(ctx, "failed to resolve workspace connection for instruction files",
				slog.F("chat_id", chat.ID),
				slog.Error(connErr),
			)
		} else {
			workspaceConnOK = true

			agentCfg, cfgErr := conn.ContextConfig(instructionCtx)
			if cfgErr != nil {
				p.logger.Debug(ctx, "failed to fetch context config from agent",
					slog.F("chat_id", chat.ID), slog.Error(cfgErr))
				// Treat a transient ContextConfig failure the
				// same as a failed connection so no sentinel is
				// persisted. The next turn will retry.
				workspaceConnOK = false
			} else {
				agentParts = agentCfg.Parts
			}
		}
	}

	// Stamp server-side fields and sanitize content. The
	// agent cannot know its own UUID, OS metadata, or
	// directory — those are added here at the trust boundary.
	var discoveredSkills []chattool.SkillMeta
	var hasContent bool
	agentID := uuid.NullUUID{UUID: agent.ID, Valid: true}

	for i := range agentParts {
		agentParts[i].ContextFileAgentID = agentID
		switch agentParts[i].Type {
		case codersdk.ChatMessagePartTypeContextFile:
			agentParts[i].ContextFileContent = SanitizePromptText(agentParts[i].ContextFileContent)
			agentParts[i].ContextFileOS = agent.OperatingSystem
			agentParts[i].ContextFileDirectory = directory
			if agentParts[i].ContextFileContent != "" {
				hasContent = true
			}
		case codersdk.ChatMessagePartTypeSkill:
			discoveredSkills = append(discoveredSkills, chattool.SkillMeta{
				Name:        agentParts[i].SkillName,
				Description: agentParts[i].SkillDescription,
				Dir:         agentParts[i].SkillDir,
				MetaFile:    agentParts[i].ContextFileSkillMetaFile,
			})
		}
	}

	if !hasContent {
		if !workspaceConnOK {
			return "", nil, nil
		}
		// Persist a sentinel (plus any skill-only parts) so
		// subsequent turns skip the workspace agent dial.
		if len(agentParts) == 0 {
			agentParts = []codersdk.ChatMessagePart{{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFileAgentID: agentID,
			}}
		}
		content, err := chatprompt.MarshalParts(agentParts)
		if err != nil {
			return "", nil, nil
		}
		msgParams := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
			ChatID: chat.ID,
		}
		appendChatMessage(&msgParams, newChatMessage(
			database.ChatMessageRoleUser,
			content,
			database.ChatMessageVisibilityBoth,
			modelConfigID,
			chatprompt.CurrentContentVersion,
		))
		_, _ = p.db.InsertChatMessages(ctx, msgParams)
		// Update the cache column: persist skills if any
		// exist, or clear to NULL so stale data from a
		// previous agent doesn't linger.
		skillParts := filterSkillParts(agentParts)
		p.updateLastInjectedContext(ctx, chat.ID, skillParts)
		return "", discoveredSkills, nil
	}
	content, err := chatprompt.MarshalParts(agentParts)
	if err != nil {
		return "", nil, xerrors.Errorf("marshal context-file parts: %w", err)
	}

	msgParams := database.InsertChatMessagesParams{ //nolint:exhaustruct // Fields populated by appendChatMessage.
		ChatID: chat.ID,
	}
	appendChatMessage(&msgParams, newChatMessage(
		database.ChatMessageRoleUser,
		content,
		database.ChatMessageVisibilityBoth,
		modelConfigID,
		chatprompt.CurrentContentVersion,
	))
	if _, err := p.db.InsertChatMessages(ctx, msgParams); err != nil {
		return "", nil, xerrors.Errorf("persist instruction files: %w", err)
	}
	// Build stripped copies for the cache column so internal
	// fields (full file content, OS, directory, skill paths)
	// are never persisted or returned to API clients.
	stripped := make([]codersdk.ChatMessagePart, len(agentParts))
	copy(stripped, agentParts)
	for i := range stripped {
		stripped[i].StripInternal()
	}
	p.updateLastInjectedContext(ctx, chat.ID, stripped)

	// Return the formatted instruction text and discovered skills
	// so the caller can inject them into this turn's prompt (since
	// the prompt was built before we persisted).
	return formatSystemInstructions(agent.OperatingSystem, directory, agentParts), discoveredSkills, nil
}

// updateLastInjectedContext persists the injected context
// parts (AGENTS.md files and skills) on the chat row so they
// are directly queryable without scanning messages. This is
// best-effort — a failure here is logged but does not block
// the turn.
func (p *Server) updateLastInjectedContext(ctx context.Context, chatID uuid.UUID, parts []codersdk.ChatMessagePart) {
	param := pqtype.NullRawMessage{Valid: false}
	if parts != nil {
		raw, err := json.Marshal(parts)
		if err != nil {
			p.logger.Warn(ctx, "failed to marshal injected context",
				slog.F("chat_id", chatID),
				slog.Error(err),
			)
			return
		}
		param = pqtype.NullRawMessage{RawMessage: raw, Valid: true}
	}
	if _, err := p.db.UpdateChatLastInjectedContext(ctx, database.UpdateChatLastInjectedContextParams{
		ID:                  chatID,
		LastInjectedContext: param,
	}); err != nil {
		p.logger.Warn(ctx, "failed to update injected context",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
	}
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

func (p *Server) recoverStaleChats(ctx context.Context) {
	staleAfter := time.Now().Add(-p.inFlightChatStaleAfter)
	staleChats, err := p.db.GetStaleChats(ctx, staleAfter)
	if err != nil {
		p.logger.Error(ctx, "failed to get stale chats", slog.Error(err))
		return
	}

	recovered := 0
	for _, chat := range staleChats {
		p.logger.Info(ctx, "recovering stale chat", slog.F("chat_id", chat.ID))

		// Use a transaction with FOR UPDATE to avoid a TOCTOU race:
		// between GetStaleChats (a bare SELECT) and here, the chat's
		// heartbeat may have been refreshed. We re-check freshness
		// under the row lock before resetting.
		err := p.db.InTx(func(tx database.Store) error {
			locked, lockErr := tx.GetChatByIDForUpdate(ctx, chat.ID)
			if lockErr != nil {
				return xerrors.Errorf("lock chat for recovery: %w", lockErr)
			}

			// Only recover chats that are still running.
			// Between GetStaleChats and this lock, the chat
			// may have completed normally.
			if locked.Status != database.ChatStatusRunning {
				p.logger.Debug(ctx, "chat status changed since snapshot, skipping recovery",
					slog.F("chat_id", chat.ID),
					slog.F("status", locked.Status))
				return nil
			}

			// Re-check: only recover if the chat is still stale.
			// A valid heartbeat that is at or after the stale
			// threshold means the chat was refreshed after our
			// initial snapshot — skip it.
			if locked.HeartbeatAt.Valid && !locked.HeartbeatAt.Time.Before(staleAfter) {
				p.logger.Debug(ctx, "chat heartbeat refreshed since snapshot, skipping recovery",
					slog.F("chat_id", chat.ID))
				return nil
			}

			// Reset to pending so any replica can pick it up.
			_, updateErr := tx.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
				ID:          chat.ID,
				Status:      database.ChatStatusPending,
				WorkerID:    uuid.NullUUID{},
				StartedAt:   sql.NullTime{},
				HeartbeatAt: sql.NullTime{},
				LastError:   sql.NullString{},
			})
			if updateErr != nil {
				return updateErr
			}
			recovered++
			return nil
		}, nil)
		if err != nil {
			p.logger.Error(ctx, "failed to recover stale chat",
				slog.F("chat_id", chat.ID), slog.Error(err))
		}
	}

	if recovered > 0 {
		p.logger.Info(ctx, "recovered stale chats", slog.F("count", recovered))
	}
}

// maybeSendPushNotification sends a web push notification when an
// agent chat reaches a terminal state. For errors it dispatches
// synchronously; for successful completions it spawns a goroutine
// that generates a short LLM summary before dispatching. The caller
// is responsible for skipping interrupted chats.
func (p *Server) maybeSendPushNotification(
	ctx context.Context,
	chat database.Chat,
	status database.ChatStatus,
	lastError string,
	runResult runChatResult,
	logger slog.Logger,
) {
	if p.webpushDispatcher == nil || p.webpushDispatcher.PublicKey() == "" {
		return
	}
	if chat.ParentChatID.Valid {
		return
	}

	switch status {
	case database.ChatStatusError:
		pushBody := "Agent encountered an error."
		if lastError != "" {
			pushBody = lastError
		}
		p.dispatchPush(ctx, chat, pushBody, status, logger)

	case database.ChatStatusWaiting:
		// Generate a push notification summary asynchronously
		// using a cheap LLM model. This avoids blocking the
		// deferred cleanup path while still providing a
		// meaningful notification body.
		p.inflight.Add(1)
		go func() {
			defer p.inflight.Done()
			pushCtx := context.WithoutCancel(ctx)
			pushBody := "Agent has finished running."
			assistantText := strings.TrimSpace(runResult.FinalAssistantText)
			if assistantText != "" && runResult.PushSummaryModel != nil {
				if summary := generatePushSummary(
					pushCtx,
					chat,
					assistantText,
					runResult.PushSummaryModel,
					runResult.ProviderKeys,
					logger,
				); summary != "" {
					pushBody = summary
				}
			}

			p.dispatchPush(pushCtx, chat, pushBody, status, logger)
		}()
	}
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
	if unsub := p.configCacheUnsubscribe; unsub != nil {
		p.configCacheUnsubscribe = nil
		unsub()
	}
	p.cancel()
	<-p.closed
	p.drainInflight()
	return nil
}

// drainInflight waits for all in-flight operations to complete.
// It acquires inflightMu to prevent processOnce from spawning
// new goroutines (via inflight.Add) concurrently with Wait,
// which would violate sync.WaitGroup's contract.
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
