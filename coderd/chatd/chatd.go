package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/chatd/chattool"
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

	defaultExternalAuthWait      = 5 * time.Minute
	homeInstructionLookupTimeout = 5 * time.Second
	instructionCacheTTL          = 5 * time.Minute
	chatHeartbeatInterval        = 30 * time.Second
	maxChatSteps                 = 1200

	defaultContextCompressionThresholdPercent = int32(70)

	maxReadFileBytes int64 = 1 << 20 // 1 MiB

	defaultNoWorkspaceInstruction = "No workspace is selected yet. Call the create_workspace tool first before using read_file, write_file, or execute. If create_workspace fails, ask the user to clarify the template or workspace request."
	defaultSubagentInstruction    = "You are running as a delegated sub-agent chat. Complete the delegated task and provide clear, concise assistant responses for the parent agent."

	externalAuthWaitPollInterval    = time.Second
	externalAuthWaitTimedOutStatus  = "timed_out"
	externalAuthWaitCompletedStatus = "completed"
)

// Server handles background processing of pending chats.
type Server struct {
	cancel   context.CancelFunc
	closed   chan struct{}
	inflight sync.WaitGroup

	db       database.Store
	workerID uuid.UUID
	logger   slog.Logger

	remotePartsProvider RemotePartsProvider

	agentConnFn              AgentConnFunc
	createWorkspaceFn        chattool.CreateWorkspaceFn
	testing                  *TestingConfig
	streamManager            *StreamManager
	pubsub                   pubsub.Pubsub
	resolveProviderAPIKeysFn ProviderAPIKeysResolver

	activeMu      sync.Mutex
	activeCancels map[uuid.UUID]context.CancelCauseFunc

	// instructionCache caches home instruction file contents by
	// workspace agent ID so we don't re-dial on every chat turn.
	instructionCacheMu sync.Mutex
	instructionCache   map[uuid.UUID]cachedInstruction

	// Configuration
	pendingChatAcquireInterval time.Duration
	inFlightChatStaleAfter     time.Duration
}

type cachedInstruction struct {
	instruction string
	fetchedAt   time.Time
}

// AgentConnFunc provides access to workspace agent connections.
type AgentConnFunc func(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error)

// CreateWorkspaceFn creates a workspace for the given owner.
type CreateWorkspaceFn = chattool.CreateWorkspaceFn

// ProviderAPIKeysResolver resolves provider API keys for chat model calls.
type ProviderAPIKeysResolver func(context.Context) (chatprovider.ProviderAPIKeys, error)

// ReplicaAddressResolver maps a replica ID to its relay address.
type ReplicaAddressResolver func(context.Context, uuid.UUID) (string, bool)

// RemotePartsProvider returns a snapshot and live stream of message_part
// events from the replica that is running the chat. Called when the chat
// is actively running on a different replica. Nil in AGPL single-replica
// deployments.
type RemotePartsProvider func(
	ctx context.Context,
	chatID uuid.UUID,
	workerID uuid.UUID,
	requestHeader http.Header,
) (
	snapshot []codersdk.ChatStreamEvent,
	parts <-chan codersdk.ChatStreamEvent,
	cancel func(),
	err error,
)

// TestingConfig contains hooks intended only for tests.
type TestingConfig struct {
	ResolveChatModel func(chat database.Chat) (fantasy.LanguageModel, error)
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

// StopStream marks the stream as no longer buffering. If
// subscribers remain, the entry stays in the map until the
// last subscriber cancels.
func (m *StreamManager) StopStream(chatID uuid.UUID) {
	m.mu.Lock()
	state, ok := m.chats[chatID]
	if ok {
		state.buffer = nil
		state.buffering = false
		m.cleanupIfIdleLocked(chatID, state)
	}
	m.mu.Unlock()
}

// Len returns the number of tracked chat streams. This is intended
// for use in tests.
func (m *StreamManager) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.chats)
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
			m.cleanupIfIdleLocked(chatID, state)
		}
		m.mu.Unlock()
	}

	return snapshot, ch, cancel
}

// cleanupIfIdleLocked removes the chat entry when there are no
// subscribers and the stream is not buffering. The caller must
// hold m.mu.
func (m *StreamManager) cleanupIfIdleLocked(chatID uuid.UUID, state *chatStreamState) {
	if !state.buffering && len(state.subscribers) == 0 {
		delete(m.chats, chatID)
	}
}

func (m *StreamManager) stateLocked(chatID uuid.UUID) *chatStreamState {
	state, ok := m.chats[chatID]
	if !ok {
		state = &chatStreamState{subscribers: make(map[uuid.UUID]chan codersdk.ChatStreamEvent)}
		m.chats[chatID] = state
	}
	return state
}

// MaxQueueSize is the maximum number of queued user messages per chat.
const MaxQueueSize = 20

// ErrMessageQueueFull indicates the per-chat queue limit was reached.
var ErrMessageQueueFull = xerrors.New("chat message queue is full")

// CreateOptions controls chat creation in the shared chat mutation path.
type CreateOptions struct {
	OwnerID            uuid.UUID
	WorkspaceID        uuid.NullUUID
	WorkspaceAgentID   uuid.NullUUID
	ParentChatID       uuid.NullUUID
	RootChatID         uuid.NullUUID
	Title              string
	ModelConfigID      uuid.UUID
	SystemPrompt       string
	InitialUserContent json.RawMessage
}

// InsertMessageOptions controls direct chat message insertion.
type InsertMessageOptions struct {
	ChatID        uuid.UUID
	ModelConfigID *uuid.UUID
	Role          string
	Content       json.RawMessage
	ToolCallID    *string
	Thinking      *string
	Hidden        bool
	Interrupt     bool
	SetPending    bool
}

// InsertMessageResult contains persisted message metadata and optional chat status.
type InsertMessageResult struct {
	Message database.ChatMessage
	Chat    database.Chat
}

// PostMessagesOptions controls user message insertion with optional queueing.
type PostMessagesOptions struct {
	ChatID          uuid.UUID
	Content         json.RawMessage
	ModelConfigID   *uuid.UUID
	ToolCallID      *string
	Thinking        *string
	Hidden          bool
	Interrupt       bool
	QueueIfBusy     bool
	IncludeMessages bool
}

// PostMessagesResult contains the outcome of user message processing.
type PostMessagesResult struct {
	Queued        bool
	QueuedMessage *database.ChatQueuedMessage
	Message       database.ChatMessage
	Chat          database.Chat
	Messages      []database.ChatMessage
}

// PromoteQueuedOptions controls queued-message promotion.
type PromoteQueuedOptions struct {
	ChatID          uuid.UUID
	QueuedMessageID int64
	ModelConfigID   *uuid.UUID
}

// PromoteQueuedResult contains the post-promotion message list.
type PromoteQueuedResult struct {
	Messages []database.ChatMessage
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

	chat, err := p.db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           opts.OwnerID,
		WorkspaceID:       opts.WorkspaceID,
		WorkspaceAgentID:  opts.WorkspaceAgentID,
		ParentChatID:      opts.ParentChatID,
		RootChatID:        opts.RootChatID,
		LastModelConfigID: opts.ModelConfigID,
		Title:             opts.Title,
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("insert chat: %w", err)
	}

	systemPrompt := strings.TrimSpace(opts.SystemPrompt)
	if systemPrompt != "" {
		systemContent, err := json.Marshal(systemPrompt)
		if err != nil {
			return database.Chat{}, xerrors.Errorf("marshal system prompt: %w", err)
		}
		_, err = p.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID: chat.ID,
			ModelConfigID: uuid.NullUUID{
				UUID:  opts.ModelConfigID,
				Valid: true,
			},
			Role: "system",
			Content: pqtype.NullRawMessage{
				RawMessage: systemContent,
				Valid:      len(systemContent) > 0,
			},
			Visibility: modelChatMessageVisibility(),
		})
		if err != nil {
			return database.Chat{}, xerrors.Errorf("insert system message: %w", err)
		}
	}

	sendResult, err := p.InsertMessage(ctx, InsertMessageOptions{
		ChatID:        chat.ID,
		ModelConfigID: &opts.ModelConfigID,
		Role:          "user",
		Content:       opts.InitialUserContent,
		Hidden:        false,
		SetPending:    true,
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("insert initial user message: %w", err)
	}
	chat = sendResult.Chat

	if !chat.RootChatID.Valid && !chat.ParentChatID.Valid {
		chat.RootChatID = uuid.NullUUID{UUID: chat.ID, Valid: true}
	}

	p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindCreated)
	return chat, nil
}

// PostMessages inserts a user message, optionally queues while a chat is
// busy, and publishes stream + pubsub updates.
func (p *Server) PostMessages(
	ctx context.Context,
	opts PostMessagesOptions,
) (PostMessagesResult, error) {
	if opts.ChatID == uuid.Nil {
		return PostMessagesResult{}, xerrors.New("chat_id is required")
	}

	if opts.Interrupt {
		p.Interrupt(opts.ChatID)
	}

	var (
		result            PostMessagesResult
		queuedMessagesSDK []codersdk.ChatQueuedMessage
	)

	txErr := p.db.InTx(func(tx database.Store) error {
		lockedChat, err := tx.GetChatByIDForUpdate(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("lock chat: %w", err)
		}
		modelConfigID := resolveMessageModelConfigIDFromChat(opts.ModelConfigID, lockedChat)

		isChatActive := p.IsActive(opts.ChatID)
		if opts.QueueIfBusy && ShouldQueueUserMessage(lockedChat.Status, isChatActive) {
			existingQueued, err := tx.GetChatQueuedMessages(ctx, opts.ChatID)
			if err != nil {
				return xerrors.Errorf("get queued messages: %w", err)
			}
			if len(existingQueued) >= MaxQueueSize {
				return ErrMessageQueueFull
			}

			queued, err := tx.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{
				ChatID:  opts.ChatID,
				Content: opts.Content,
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
			queuedMessagesSDK = toSDKQueuedMessages(queuedMessages)
			return nil
		}

		message, err := insertChatMessageWithStore(ctx, tx, database.InsertChatMessageParams{
			ChatID:        opts.ChatID,
			ModelConfigID: modelConfigIDNullUUID(modelConfigID),
			Role:          "user",
			Content: pqtype.NullRawMessage{
				RawMessage: opts.Content,
				Valid:      len(opts.Content) > 0,
			},
			Visibility: visibilityFromLegacyHidden(opts.Hidden),
		})
		if err != nil {
			return err
		}
		result.Message = message

		if lockedChat.Status == database.ChatStatusPending {
			result.Chat = lockedChat
		} else {
			updatedChat, err := tx.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
				ID:        opts.ChatID,
				Status:    database.ChatStatusPending,
				WorkerID:  uuid.NullUUID{},
				StartedAt: sql.NullTime{},
			})
			if err != nil {
				return xerrors.Errorf("set chat pending: %w", err)
			}
			result.Chat = updatedChat
		}

		if opts.IncludeMessages {
			messages, err := tx.GetChatMessagesByChatID(ctx, opts.ChatID)
			if err != nil {
				return xerrors.Errorf("get chat messages: %w", err)
			}
			result.Messages = messages
		}

		return nil
	}, nil)
	if txErr != nil {
		return PostMessagesResult{}, txErr
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
		return result, nil
	}

	p.publishMessage(opts.ChatID, result.Message)
	p.publishStatus(opts.ChatID, result.Chat.Status)
	p.publishChatPubsubEvent(result.Chat, coderdpubsub.ChatEventKindStatusChange)
	return result, nil
}

// Delete removes a chat and all descendants, then broadcasts a deleted event.
func (p *Server) Delete(ctx context.Context, chatID uuid.UUID) error {
	if chatID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}

	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("get chat: %w", err)
	}

	err = p.db.InTx(func(tx database.Store) error {
		// Collect descendants breadth-first, then delete from leaves upward.
		descendantIDs := make([]uuid.UUID, 0)
		queue := []uuid.UUID{chatID}
		for len(queue) > 0 {
			parentID := queue[0]
			queue = queue[1:]

			children, err := tx.ListChildChatsByParentID(ctx, parentID)
			if err != nil {
				return xerrors.Errorf("list children of chat %s: %w", parentID, err)
			}
			for _, child := range children {
				descendantIDs = append(descendantIDs, child.ID)
				queue = append(queue, child.ID)
			}
		}

		for i := len(descendantIDs) - 1; i >= 0; i-- {
			if err := tx.DeleteChatByID(ctx, descendantIDs[i]); err != nil {
				return xerrors.Errorf("delete descendant chat %s: %w", descendantIDs[i], err)
			}
		}

		if err := tx.DeleteChatByID(ctx, chatID); err != nil {
			return xerrors.Errorf("delete chat: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		return err
	}

	p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindDeleted)
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

	err := p.db.DeleteChatQueuedMessage(ctx, database.DeleteChatQueuedMessageParams{
		ID:     queuedMessageID,
		ChatID: chatID,
	})
	if err != nil {
		return xerrors.Errorf("delete queued message: %w", err)
	}

	queuedMessages, err := p.db.GetChatQueuedMessages(ctx, chatID)
	if err != nil {
		p.logger.Warn(ctx, "failed to load queued messages after delete",
			slog.F("chat_id", chatID),
			slog.F("queued_message_id", queuedMessageID),
			slog.Error(err),
		)
		return nil
	}

	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type:           codersdk.ChatStreamEventTypeQueueUpdate,
		QueuedMessages: toSDKQueuedMessages(queuedMessages),
	})
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

	chat, err := p.db.GetChatByID(ctx, opts.ChatID)
	if err != nil {
		return PromoteQueuedResult{}, xerrors.Errorf("get chat: %w", err)
	}
	if chat.Status == database.ChatStatusRunning ||
		chat.Status == database.ChatStatusPending {
		p.Interrupt(opts.ChatID)
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
		modelConfigID := resolveMessageModelConfigIDFromChat(opts.ModelConfigID, lockedChat)

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

		promoted, err = tx.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID:        opts.ChatID,
			ModelConfigID: modelConfigIDNullUUID(modelConfigID),
			Role:          "user",
			Content: pqtype.NullRawMessage{
				RawMessage: targetContent,
				Valid:      len(targetContent) > 0,
			},
			Visibility: bothChatMessageVisibility(),
		})
		if err != nil {
			return xerrors.Errorf("insert message: %w", err)
		}

		updatedChat, err = tx.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
			ID:        opts.ChatID,
			Status:    database.ChatStatusPending,
			WorkerID:  uuid.NullUUID{},
			StartedAt: sql.NullTime{},
		})
		if err != nil {
			return xerrors.Errorf("update status: %w", err)
		}

		remainingQueue, err = tx.GetChatQueuedMessages(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("get remaining queue: %w", err)
		}

		result.Messages, err = tx.GetChatMessagesByChatID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("get messages: %w", err)
		}

		return nil
	}, nil)
	if txErr != nil {
		return PromoteQueuedResult{}, txErr
	}

	p.publishEvent(opts.ChatID, codersdk.ChatStreamEvent{
		Type:           codersdk.ChatStreamEventTypeQueueUpdate,
		QueuedMessages: toSDKQueuedMessages(remainingQueue),
	})
	p.publishChatStreamNotify(opts.ChatID, coderdpubsub.ChatStreamNotifyMessage{
		QueueUpdate: true,
	})
	p.publishMessage(opts.ChatID, promoted)
	p.publishStatus(opts.ChatID, updatedChat.Status)

	return result, nil
}

// InterruptAndSetWaiting interrupts execution, sets waiting status, and broadcasts status updates.
func (p *Server) InterruptAndSetWaiting(
	ctx context.Context,
	chat database.Chat,
) database.Chat {
	if chat.ID == uuid.Nil {
		return chat
	}

	p.Interrupt(chat.ID)
	p.Stop(chat.ID)

	updatedChat, err := p.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusWaiting,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	if err != nil {
		p.logger.Error(ctx, "failed to mark chat as waiting",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
	} else {
		chat = updatedChat
	}

	p.publishStatus(chat.ID, chat.Status)
	p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindStatusChange)
	return chat
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

	p.publishStatus(chat.ID, chat.Status)
	return nil
}

// InsertMessage inserts a message and optionally marks the chat pending.
func (p *Server) InsertMessage(
	ctx context.Context,
	opts InsertMessageOptions,
) (InsertMessageResult, error) {
	if opts.ChatID == uuid.Nil {
		return InsertMessageResult{}, xerrors.New("chat_id is required")
	}
	if opts.Role == "" {
		return InsertMessageResult{}, xerrors.New("role is required")
	}

	switch opts.Role {
	case "user", "assistant", "system", "tool":
	default:
		return InsertMessageResult{}, xerrors.Errorf("invalid role %q", opts.Role)
	}

	message, err := insertChatMessageWithStore(ctx, p.db, database.InsertChatMessageParams{
		ChatID:        opts.ChatID,
		ModelConfigID: modelConfigIDNullUUID(opts.ModelConfigID),
		Role:          opts.Role,
		Content: pqtype.NullRawMessage{
			RawMessage: opts.Content,
			Valid:      len(opts.Content) > 0,
		},
		Visibility: visibilityFromLegacyHidden(opts.Hidden),
	})
	if err != nil {
		return InsertMessageResult{}, err
	}

	if opts.Interrupt {
		p.Interrupt(opts.ChatID)
	}

	result := InsertMessageResult{Message: message}
	if !opts.SetPending {
		return result, nil
	}

	updatedChat, err := p.setChatPending(ctx, opts.ChatID)
	if err != nil {
		return InsertMessageResult{}, err
	}
	result.Chat = updatedChat
	return result, nil
}

func (p *Server) setChatPending(ctx context.Context, chatID uuid.UUID) (database.Chat, error) {
	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("get chat: %w", err)
	}
	if chat.Status == database.ChatStatusPending {
		return chat, nil
	}

	updatedChat, err := p.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("set chat pending: %w", err)
	}
	return updatedChat, nil
}

func insertChatMessageWithStore(
	ctx context.Context,
	store database.Store,
	params database.InsertChatMessageParams,
) (database.ChatMessage, error) {
	message, err := store.InsertChatMessage(ctx, params)
	if err != nil {
		return database.ChatMessage{}, xerrors.Errorf("insert chat message: %w", err)
	}
	return message, nil
}

func chatMessageVisibility(
	visibility database.ChatMessageVisibility,
) database.NullChatMessageVisibility {
	return database.NullChatMessageVisibility{
		ChatMessageVisibility: visibility,
		Valid:                 true,
	}
}

func bothChatMessageVisibility() database.NullChatMessageVisibility {
	return chatMessageVisibility(database.ChatMessageVisibilityBoth)
}

func modelChatMessageVisibility() database.NullChatMessageVisibility {
	return chatMessageVisibility(database.ChatMessageVisibilityModel)
}

func visibilityFromLegacyHidden(hidden bool) database.NullChatMessageVisibility {
	if hidden {
		return modelChatMessageVisibility()
	}
	return bothChatMessageVisibility()
}

// ShouldQueueUserMessage reports whether a user message should be queued while
// a chat is active.
func ShouldQueueUserMessage(status database.ChatStatus, isChatActive bool) bool {
	switch status {
	case database.ChatStatusRunning, database.ChatStatusPending:
		return true
	case database.ChatStatusWaiting:
		return isChatActive
	default:
		return false
	}
}

func toSDKQueuedMessages(messages []database.ChatQueuedMessage) []codersdk.ChatQueuedMessage {
	out := make([]codersdk.ChatQueuedMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, codersdk.ChatQueuedMessage{
			ID:        message.ID,
			ChatID:    message.ChatID,
			Content:   message.Content,
			CreatedAt: message.CreatedAt,
		})
	}
	return out
}

// Config configures a chat processor.
type Config struct {
	Logger                     slog.Logger
	Database                   database.Store
	ReplicaID                  uuid.UUID
	RemotePartsProvider        RemotePartsProvider
	PendingChatAcquireInterval time.Duration
	InFlightChatStaleAfter     time.Duration
	AgentConn                  AgentConnFunc
	CreateWorkspace            CreateWorkspaceFn
	StreamManager              *StreamManager
	Pubsub                     pubsub.Pubsub
	ResolveProviderAPIKeys     ProviderAPIKeysResolver
	Testing                    *TestingConfig
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

	workerID := cfg.ReplicaID
	if workerID == uuid.Nil {
		workerID = uuid.New()
	}

	p := &Server{
		cancel:                     cancel,
		closed:                     make(chan struct{}),
		db:                         cfg.Database,
		workerID:                   workerID,
		logger:                     cfg.Logger.Named("chat-processor"),
		remotePartsProvider:        cfg.RemotePartsProvider,
		agentConnFn:                cfg.AgentConn,
		createWorkspaceFn:          cfg.CreateWorkspace,
		testing:                    cfg.Testing,
		streamManager:              streamManager,
		pubsub:                     cfg.Pubsub,
		resolveProviderAPIKeysFn:   resolveProviderAPIKeys,
		instructionCache:           make(map[uuid.UUID]cachedInstruction),
		activeCancels:              make(map[uuid.UUID]context.CancelCauseFunc),
		pendingChatAcquireInterval: pendingChatAcquireInterval,
		inFlightChatStaleAfter:     inFlightChatStaleAfter,
	}

	//nolint:gocritic // The chat processor is a system-level service.
	ctx = dbauthz.AsSystemRestricted(ctx)
	go p.start(ctx)

	return p
}

func (p *Server) start(ctx context.Context) {
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

func (p *Server) processOnce(ctx context.Context) {
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

func (p *Server) registerChat(chatID uuid.UUID, cancel context.CancelCauseFunc) {
	p.activeMu.Lock()
	p.activeCancels[chatID] = cancel
	p.activeMu.Unlock()
}

func (p *Server) unregisterChat(chatID uuid.UUID) {
	p.activeMu.Lock()
	delete(p.activeCancels, chatID)
	p.activeMu.Unlock()
}

func (p *Server) Interrupt(chatID uuid.UUID) bool {
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

// IsActive reports whether the processor currently has an in-flight
// worker for the chat.
func (p *Server) IsActive(chatID uuid.UUID) bool {
	if p == nil {
		return false
	}
	p.activeMu.Lock()
	_, ok := p.activeCancels[chatID]
	p.activeMu.Unlock()
	return ok
}

// Models returns the currently available chat model catalog.
func (p *Server) Models(ctx context.Context) (codersdk.ChatModelsResponse, error) {
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

// ProviderKeys merges configured provider keys over resolver keys.
func (p *Server) ProviderKeys(
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

func (p *Server) publishStream(chatID uuid.UUID, event codersdk.ChatStreamEvent) {
	if p == nil || p.streamManager == nil {
		return
	}
	p.streamManager.Publish(chatID, event)
}

func (p *Server) Subscribe(
	ctx context.Context,
	chatID uuid.UUID,
	requestHeader http.Header,
) (
	[]codersdk.ChatStreamEvent,
	<-chan codersdk.ChatStreamEvent,
	func(),
	bool,
) {
	if p == nil || p.streamManager == nil {
		return nil, nil, nil, false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Subscribe to local StreamManager for message_parts (ephemeral)
	localSnapshot, localParts, localCancel := p.streamManager.Subscribe(chatID)

	// Build initial snapshot synchronously
	initialSnapshot := make([]codersdk.ChatStreamEvent, 0)
	// Add local message_parts to snapshot
	for _, event := range localSnapshot {
		if event.Type == codersdk.ChatStreamEventTypeMessagePart {
			initialSnapshot = append(initialSnapshot, event)
		}
	}

	// Load initial messages from DB
	//nolint:gocritic // System context needed to read chat messages for stream.
	messages, err := p.db.GetChatMessagesByChatID(dbauthz.AsSystemRestricted(ctx), chatID)
	if err == nil {
		for _, msg := range messages {
			sdkMsg := db2sdk.ChatMessage(msg)
			initialSnapshot = append(initialSnapshot, codersdk.ChatStreamEvent{
				Type:    codersdk.ChatStreamEventTypeMessage,
				ChatID:  chatID,
				Message: &sdkMsg,
			})
		}
	}

	// Load initial queue
	//nolint:gocritic // System context needed to read queued messages for stream.
	queued, err := p.db.GetChatQueuedMessages(dbauthz.AsSystemRestricted(ctx), chatID)
	if err == nil && len(queued) > 0 {
		initialSnapshot = append(initialSnapshot, codersdk.ChatStreamEvent{
			Type:           codersdk.ChatStreamEventTypeQueueUpdate,
			ChatID:         chatID,
			QueuedMessages: toSDKQueuedMessages(queued),
		})
	}

	// Get initial chat state to determine if we need a relay
	//nolint:gocritic // System context needed to read chat state for relay.
	chat, err := p.db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chatID)
	var relayCancel func()
	var relayParts <-chan codersdk.ChatStreamEvent
	if err == nil && chat.Status == database.ChatStatusRunning && chat.WorkerID.Valid && chat.WorkerID.UUID != p.workerID && p.remotePartsProvider != nil {
		// Open relay for initial snapshot
		snapshot, parts, cancel, err := p.remotePartsProvider(ctx, chatID, chat.WorkerID.UUID, requestHeader)
		if err == nil {
			relayCancel = cancel
			relayParts = parts
			// Add relay message_parts to snapshot
			for _, event := range snapshot {
				if event.Type == codersdk.ChatStreamEventTypeMessagePart {
					initialSnapshot = append(initialSnapshot, event)
				}
			}
		}
	}

	// Track the last message ID we've seen for DB queries
	var lastMessageID int64
	if len(messages) > 0 {
		lastMessageID = messages[len(messages)-1].ID
	}

	// Merge all event sources
	mergedCtx, mergedCancel := context.WithCancel(ctx)
	mergedEvents := make(chan codersdk.ChatStreamEvent, 128)
	var allCancels []func()
	allCancels = append(allCancels, localCancel)
	if relayCancel != nil {
		allCancels = append(allCancels, relayCancel)
	}

	// Helper to close relay
	closeRelay := func() {
		if relayCancel != nil {
			relayCancel()
			relayCancel = nil
		}
		relayParts = nil
	}

	// Helper to open relay to a worker
	openRelay := func(workerID uuid.UUID) {
		if p.remotePartsProvider == nil {
			return
		}
		closeRelay()
		snapshot, parts, cancel, err := p.remotePartsProvider(mergedCtx, chatID, workerID, requestHeader)
		if err != nil {
			p.logger.Warn(mergedCtx, "failed to open relay for message parts",
				slog.F("chat_id", chatID),
				slog.F("worker_id", workerID),
				slog.Error(err),
			)
			return
		}
		relayParts = parts
		relayCancel = cancel
		// Send relay snapshot message_parts
		for _, event := range snapshot {
			if event.Type == codersdk.ChatStreamEventTypeMessagePart {
				select {
				case <-mergedCtx.Done():
					return
				case mergedEvents <- event:
				}
			}
		}
	}

	// Subscribe to pubsub for durable events
	var pubsubCancel func()
	if p.pubsub != nil {
		notifications := make(chan coderdpubsub.ChatStreamNotifyMessage, 10)
		errors := make(chan error, 1)

		listener := func(_ context.Context, message []byte, err error) {
			if err != nil {
				select {
				case <-mergedCtx.Done():
				case errors <- err:
				}
				return
			}
			var notify coderdpubsub.ChatStreamNotifyMessage
			if unmarshalErr := json.Unmarshal(message, &notify); unmarshalErr != nil {
				select {
				case <-mergedCtx.Done():
				case errors <- xerrors.Errorf("unmarshal chat stream notify: %w", unmarshalErr):
				}
				return
			}
			select {
			case <-mergedCtx.Done():
			case notifications <- notify:
			}
		}

		var err error
		pubsubCancel, err = p.pubsub.SubscribeWithErr(
			coderdpubsub.ChatStreamNotifyChannel(chatID),
			listener,
		)
		if err != nil {
			p.logger.Warn(mergedCtx, "failed to subscribe to chat stream notifications",
				slog.F("chat_id", chatID),
				slog.Error(err),
			)
		} else {
			allCancels = append(allCancels, pubsubCancel)
		}

		// Handle pubsub notifications in a goroutine
		go func() {
			defer close(mergedEvents)
			defer closeRelay()

			for {
				select {
				case <-mergedCtx.Done():
					return
				case err := <-errors:
					p.logger.Error(mergedCtx, "chat stream pubsub error",
						slog.F("chat_id", chatID),
						slog.Error(err),
					)
					mergedEvents <- codersdk.ChatStreamEvent{
						Type:   codersdk.ChatStreamEventTypeError,
						ChatID: chatID,
						Error: &codersdk.ChatStreamError{
							Message: err.Error(),
						},
					}
					return
				case notify := <-notifications:
					// Handle different notification types
					if notify.AfterMessageID > 0 {
						// Read new messages from DB
						//nolint:gocritic // System context needed to read chat messages for stream.
						messages, err := p.db.GetChatMessagesByChatID(dbauthz.AsSystemRestricted(mergedCtx), chatID)
						if err == nil {
							for _, msg := range messages {
								if msg.ID > lastMessageID {
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
						// Manage relay lifecycle based on status
						if status == database.ChatStatusRunning && notify.WorkerID != "" {
							workerID, err := uuid.Parse(notify.WorkerID)
							if err == nil && workerID != p.workerID {
								openRelay(workerID)
							} else if workerID == p.workerID {
								closeRelay()
							}
						} else {
							closeRelay()
						}
					}
					if notify.Error != "" {
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
						//nolint:gocritic // System context needed to read queued messages for stream.
						queued, err := p.db.GetChatQueuedMessages(dbauthz.AsSystemRestricted(mergedCtx), chatID)
						if err == nil {
							select {
							case <-mergedCtx.Done():
								return
							case mergedEvents <- codersdk.ChatStreamEvent{
								Type:           codersdk.ChatStreamEventTypeQueueUpdate,
								ChatID:         chatID,
								QueuedMessages: toSDKQueuedMessages(queued),
							}:
							}
						}
					}
				case event, ok := <-localParts:
					if !ok {
						// Local parts channel closed, but continue with pubsub
						continue
					}
					// Only forward message_part events from local (durable events come via pubsub)
					if event.Type == codersdk.ChatStreamEventTypeMessagePart {
						select {
						case <-mergedCtx.Done():
							return
						case mergedEvents <- event:
						}
					}
				case event, ok := <-relayParts:
					if !ok {
						relayParts = nil
						continue
					}
					// Only forward message_part events from relay (durable events come via pubsub)
					if event.Type == codersdk.ChatStreamEventTypeMessagePart {
						select {
						case <-mergedCtx.Done():
							return
						case mergedEvents <- event:
						}
					}
				}
			}
		}()
	} else {
		// No pubsub, just merge local parts
		go func() {
			defer close(mergedEvents)
			for _, event := range localSnapshot {
				select {
				case <-mergedCtx.Done():
					return
				case mergedEvents <- event:
				}
			}
			for event := range localParts {
				select {
				case <-mergedCtx.Done():
					return
				case mergedEvents <- event:
				}
			}
		}()
	}

	cancel := func() {
		mergedCancel()
		for _, cancelFn := range allCancels {
			if cancelFn != nil {
				cancelFn()
			}
		}
		closeRelay()
	}

	return initialSnapshot, mergedEvents, cancel, true
}

func (p *Server) Stop(chatID uuid.UUID) {
	if p == nil || p.streamManager == nil {
		return
	}
	p.streamManager.StopStream(chatID)
}

func (p *Server) publishEvent(chatID uuid.UUID, event codersdk.ChatStreamEvent) {
	if p.streamManager == nil {
		return
	}
	if event.ChatID == uuid.Nil {
		event.ChatID = chatID
	}
	p.streamManager.Publish(chatID, event)
}

func (p *Server) publishStatus(chatID uuid.UUID, status database.ChatStatus) {
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeStatus,
		Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatus(status)},
	})
	p.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		Status:   string(status),
		WorkerID: p.workerID.String(),
	})
}

// publishChatStreamNotify broadcasts a per-chat stream notification via
// PostgreSQL pubsub so that all replicas can read updates from the database.
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

// publishChatPubsubEvent broadcasts a chat lifecycle event via PostgreSQL
// pubsub so that all replicas can push updates to watching clients.
func (p *Server) publishChatPubsubEvent(chat database.Chat, kind coderdpubsub.ChatEventKind) {
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

func (p *Server) publishError(chatID uuid.UUID, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type:  codersdk.ChatStreamEventTypeError,
		Error: &codersdk.ChatStreamError{Message: message},
	})
	p.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		Error: message,
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

func (p *Server) publishMessage(chatID uuid.UUID, message database.ChatMessage) {
	sdkMessage := db2sdk.ChatMessage(message)
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type:    codersdk.ChatStreamEventTypeMessage,
		Message: &sdkMessage,
	})
	p.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		AfterMessageID: message.ID - 1,
	})
}

func (p *Server) publishMessagePart(chatID uuid.UUID, role string, part codersdk.ChatMessagePart) {
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

func (p *Server) processChat(ctx context.Context, chat database.Chat) {
	logger := p.logger.With(slog.F("chat_id", chat.ID))
	logger.Info(ctx, "processing chat")

	chatCtx, cancel := context.WithCancelCause(ctx)
	p.registerChat(chat.ID, cancel)
	defer p.unregisterChat(chat.ID)
	defer cancel(nil)

	// Periodically update the heartbeat so other replicas know this
	// worker is still alive. The goroutine stops when chatCtx is
	// canceled (either by completion or interruption).
	go func() {
		ticker := time.NewTicker(chatHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-chatCtx.Done():
				return
			case <-ticker.C:
				if err := p.db.UpdateChatHeartbeat(chatCtx, chat.ID); err != nil {
					logger.Warn(ctx, "failed to update chat heartbeat", slog.Error(err))
				}
			}
		}
	}()

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
					modelConfigID := resolveMessageModelConfigIDFromChat(nil, latestChat)
					msg, insertErr := tx.InsertChatMessage(ctx, database.InsertChatMessageParams{
						ChatID:        chat.ID,
						ModelConfigID: modelConfigIDNullUUID(modelConfigID),
						Role:          "user",
						Content: pqtype.NullRawMessage{
							RawMessage: nextQueued.Content,
							Valid:      len(nextQueued.Content) > 0,
						},
						Visibility:          bothChatMessageVisibility(),
						InputTokens:         sql.NullInt64{},
						OutputTokens:        sql.NullInt64{},
						TotalTokens:         sql.NullInt64{},
						ReasoningTokens:     sql.NullInt64{},
						CacheCreationTokens: sql.NullInt64{},
						CacheReadTokens:     sql.NullInt64{},
						ContextLimit:        sql.NullInt64{},
						Compressed:          sql.NullBool{},
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
							p.publishChatStreamNotify(chat.ID, coderdpubsub.ChatStreamNotifyMessage{
								QueueUpdate: true,
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
	}()

	if err := p.runChat(chatCtx, chat, logger); err != nil {
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

func (p *Server) runChat(
	ctx context.Context,
	chat database.Chat,
	logger slog.Logger,
) error {
	model, modelConfig, err := p.resolveChatModel(ctx, chat)
	if err != nil {
		return err
	}

	messages, err := p.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	if err != nil {
		return xerrors.Errorf("get chat messages: %w", err)
	}
	p.maybeGenerateChatTitle(ctx, chat, messages, model, logger)

	prompt, err := chatprompt.ConvertMessages(messages)
	if err != nil {
		return xerrors.Errorf("build chat prompt: %w", err)
	}
	if chat.ParentChatID.Valid {
		prompt = chatprompt.InsertSystem(prompt, defaultSubagentInstruction)
	}

	if p.streamManager != nil {
		p.streamManager.StartStream(chat.ID)
		defer p.streamManager.StopStream(chat.ID)
	}

	_, err = p.runChatWithAgent(
		ctx,
		chat,
		logger,
		model,
		modelConfig,
		prompt,
	)
	return err
}

type chatContextCompressionConfig struct {
	ContextLimit     int64
	ThresholdPercent int32
}

func (p *Server) resolveChatContextCompressionConfig(
	ctx context.Context,
	chat database.Chat,
) (chatContextCompressionConfig, error) {
	config := chatContextCompressionConfig{
		ThresholdPercent: defaultContextCompressionThresholdPercent,
	}

	chatConfig, err := parseChatModelConfig(nil)
	if err != nil {
		return config, nil
	}
	latestConfig, found, latestErr := p.resolveLatestMessageModelConfig(ctx, chat)
	if latestErr != nil {
		return config, xerrors.Errorf("resolve latest message model config: %w", latestErr)
	}
	if found {
		chatConfig = applyFallbackChatModelConfig(chatConfig, latestConfig)
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

func (p *Server) persistChatContextSummary(
	ctx context.Context,
	chatID uuid.UUID,
	modelConfigID *uuid.UUID,
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
		ChatID:        chatID,
		ModelConfigID: modelConfigIDNullUUID(modelConfigID),
		Role:          string(fantasy.MessageRoleSystem),
		Content: pqtype.NullRawMessage{
			RawMessage: systemContent,
			Valid:      len(systemContent) > 0,
		},
		Visibility:          bothChatMessageVisibility(),
		Compressed:          sql.NullBool{Bool: true, Valid: true},
		InputTokens:         sql.NullInt64{},
		OutputTokens:        sql.NullInt64{},
		TotalTokens:         sql.NullInt64{},
		ReasoningTokens:     sql.NullInt64{},
		CacheCreationTokens: sql.NullInt64{},
		CacheReadTokens:     sql.NullInt64{},
		ContextLimit:        sql.NullInt64{},
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
		ChatID:        chatID,
		ModelConfigID: modelConfigIDNullUUID(modelConfigID),
		Role:          string(fantasy.MessageRoleAssistant),
		Content:       assistantContent,
		Visibility: database.NullChatMessageVisibility{
			ChatMessageVisibility: database.ChatMessageVisibilityUser,
			Valid:                 true,
		},
		Compressed: sql.NullBool{
			Bool:  true,
			Valid: true,
		},
		InputTokens:         sql.NullInt64{},
		OutputTokens:        sql.NullInt64{},
		TotalTokens:         sql.NullInt64{},
		ReasoningTokens:     sql.NullInt64{},
		CacheCreationTokens: sql.NullInt64{},
		CacheReadTokens:     sql.NullInt64{},
		ContextLimit:        sql.NullInt64{},
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
		ChatID:        chatID,
		ModelConfigID: modelConfigIDNullUUID(modelConfigID),
		Role:          string(fantasy.MessageRoleTool),
		Content:       toolResult,
		Visibility:    bothChatMessageVisibility(),
		Compressed: sql.NullBool{
			Bool:  true,
			Valid: true,
		},
		InputTokens:         sql.NullInt64{},
		OutputTokens:        sql.NullInt64{},
		TotalTokens:         sql.NullInt64{},
		ReasoningTokens:     sql.NullInt64{},
		CacheCreationTokens: sql.NullInt64{},
		CacheReadTokens:     sql.NullInt64{},
		ContextLimit:        sql.NullInt64{},
	})
	if err != nil {
		return xerrors.Errorf("insert summary tool result message: %w", err)
	}

	p.publishMessage(chatID, assistantMessage)
	p.publishMessage(chatID, toolMessage)
	return nil
}

func (p *Server) resolveChatModel(
	ctx context.Context,
	chat database.Chat,
) (fantasy.LanguageModel, chatModelConfig, error) {
	config, parseErr := parseChatModelConfig(nil)
	if parseErr != nil {
		config = chatModelConfig{}
	}
	latestConfig, found, latestErr := p.resolveLatestMessageModelConfig(ctx, chat)
	if latestErr != nil {
		return nil, chatModelConfig{}, xerrors.Errorf(
			"resolve latest message model config: %w",
			latestErr,
		)
	}
	if found {
		config = applyFallbackChatModelConfig(config, latestConfig)
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

func (p *Server) modelFromChat(
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
	config = p.resolveModelConfigIDByReference(ctx, config)

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
	if config.ModelConfigID == nil {
		config.ModelConfigID = uuidPointer(fallbackConfig.ID)
	}

	defaults, err := parseChatModelConfig(fallbackConfig.Options)
	if err != nil {
		return config
	}

	chatprovider.MergeMissingCallConfig(
		&config.ChatModelCallConfig,
		defaults.ChatModelCallConfig,
	)
	return config
}

func (p *Server) resolveFallbackChatModelConfig(
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

func (p *Server) resolveLatestMessageModelConfig(
	ctx context.Context,
	chat database.Chat,
) (database.ChatModelConfig, bool, error) {
	if chat.LastModelConfigID == uuid.Nil {
		return database.ChatModelConfig{}, false, xerrors.New("chat model config id is required")
	}

	modelConfig, err := p.db.GetChatModelConfigByID(ctx, chat.LastModelConfigID)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return database.ChatModelConfig{}, false, xerrors.Errorf(
				"chat model config %q was not found",
				chat.LastModelConfigID,
			)
		}
		return database.ChatModelConfig{}, false, xerrors.Errorf("load chat model config by id: %w", err)
	}

	return modelConfig, true, nil
}

func (p *Server) resolveModelConfigIDByReference(
	ctx context.Context,
	config chatModelConfig,
) chatModelConfig {
	if config.ModelConfigID != nil {
		return config
	}

	provider, modelID, err := chatprovider.ResolveModelWithProviderHint(
		strings.TrimSpace(config.Model),
		strings.TrimSpace(config.Provider),
	)
	if err != nil {
		return config
	}

	modelConfig, err := p.db.GetChatModelConfigByProviderAndModel(
		ctx,
		database.GetChatModelConfigByProviderAndModelParams{
			Provider: provider,
			Model:    modelID,
		},
	)
	if err != nil {
		return config
	}

	config.ModelConfigID = uuidPointer(modelConfig.ID)
	return config
}

func (p *Server) runChatWithAgent(
	ctx context.Context,
	chat database.Chat,
	logger slog.Logger,
	model fantasy.LanguageModel,
	modelConfig chatModelConfig,
	prompt []fantasy.Message,
) (*fantasy.AgentResult, error) {
	currentChat := chat
	var (
		chatStateMu sync.Mutex
		workspaceMu sync.Mutex
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

		if !chatSnapshot.WorkspaceAgentID.Valid {
			return nil, xerrors.New("chat has no workspace agent")
		}

		agentConn, agentRelease, err := p.agentConnFn(ctx, chatSnapshot.WorkspaceAgentID.UUID)
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
				ChatID:        chat.ID,
				ModelConfigID: modelConfigIDNullUUID(modelConfig.ModelConfigID),
				Role:          string(fantasy.MessageRoleAssistant),
				Content:       assistantContent,
				Visibility:    bothChatMessageVisibility(),
				InputTokens:   usageNullInt64(step.Usage.InputTokens, hasUsage),
				OutputTokens:  usageNullInt64(step.Usage.OutputTokens, hasUsage),
				TotalTokens:   usageNullInt64(step.Usage.TotalTokens, hasUsage),
				ReasoningTokens: usageNullInt64(
					step.Usage.ReasoningTokens,
					hasUsage,
				),
				CacheCreationTokens: usageNullInt64(
					step.Usage.CacheCreationTokens,
					hasUsage,
				),
				CacheReadTokens: usageNullInt64(step.Usage.CacheReadTokens, hasUsage),
				ContextLimit:    step.ContextLimit,
				Compressed:      sql.NullBool{},
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
				ChatID:        chat.ID,
				ModelConfigID: modelConfigIDNullUUID(modelConfig.ModelConfigID),
				Role:          string(fantasy.MessageRoleTool),
				Content:       resultContent,
				Visibility:    bothChatMessageVisibility(),
			})
			if err != nil {
				return xerrors.Errorf("insert tool result: %w", err)
			}

			p.publishMessage(chat.ID, toolMessage)
		}
		return nil
	}
	streamCall := streamCallOptionsFromChatModelConfig(model, modelConfig)
	compactionOptions := &chatloop.CompactionOptions{
		ThresholdPercent: compressionConfig.ThresholdPercent,
		ContextLimit:     compressionConfig.ContextLimit,
		Persist: func(
			persistCtx context.Context,
			result chatloop.CompactionResult,
		) error {
			if err := p.persistChatContextSummary(
				persistCtx,
				chat.ID,
				modelConfig.ModelConfigID,
				result,
			); err != nil {
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

	return chatloop.Run(ctx, chatloop.RunOptions{
		Model:      model,
		Messages:   prompt,
		Tools:      p.agentTools(model, &currentChat, &chatStateMu, &workspaceMu, getWorkspaceConn),
		StreamCall: streamCall,
		MaxSteps:   maxChatSteps,

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

//nolint:revive // Boolean controls SQL NULL validity.
func usageNullInt64(value int64, valid bool) sql.NullInt64 {
	if !valid {
		return sql.NullInt64{}
	}
	return sql.NullInt64{
		Int64: value,
		Valid: valid,
	}
}

func (p *Server) appendHomeInstructionToPrompt(
	ctx context.Context,
	chat database.Chat,
	prompt []fantasy.Message,
	getWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error),
) []fantasy.Message {
	if !chat.WorkspaceAgentID.Valid || getWorkspaceConn == nil {
		return prompt
	}

	instruction := p.resolveHomeInstruction(ctx, chat, getWorkspaceConn)
	if instruction == "" {
		return prompt
	}

	return chatprompt.InsertSystem(prompt, instruction)
}

// resolveHomeInstruction returns cached home instructions for the
// workspace agent, fetching them on cache miss or expiry.
func (p *Server) resolveHomeInstruction(
	ctx context.Context,
	chat database.Chat,
	getWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error),
) string {
	agentID := chat.WorkspaceAgentID.UUID

	p.instructionCacheMu.Lock()
	cached, ok := p.instructionCache[agentID]
	p.instructionCacheMu.Unlock()

	if ok && time.Since(cached.fetchedAt) < instructionCacheTTL {
		return cached.instruction
	}

	instructionCtx, cancel := context.WithTimeout(ctx, homeInstructionLookupTimeout)
	defer cancel()

	conn, err := getWorkspaceConn(instructionCtx)
	if err != nil {
		p.logger.Debug(ctx, "failed to resolve workspace connection for home instruction file",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return cached.instruction
	}

	content, sourcePath, truncated, err := readHomeInstructionFile(instructionCtx, conn)
	if err != nil {
		p.logger.Debug(ctx, "failed to load home instruction file",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return cached.instruction
	}

	instruction := formatHomeInstruction(content, sourcePath, truncated)

	p.instructionCacheMu.Lock()
	p.instructionCache[agentID] = cachedInstruction{
		instruction: instruction,
		fetchedAt:   time.Now(),
	}
	p.instructionCacheMu.Unlock()

	return instruction
}

func (p *Server) resolveProviderAPIKeys(
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

func truncateRunes(value string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}

	return string(runes[:maxLen])
}

func (p *Server) recoverStaleChats(ctx context.Context) {
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
func (p *Server) Close() error {
	p.cancel()
	<-p.closed
	p.inflight.Wait()
	return nil
}

type chatModelConfig struct {
	codersdk.ChatModelCallConfig
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model"`
	ContextLimit  int64  `json:"context_limit,omitempty"`
	ModelConfigID *uuid.UUID
}

func parseChatModelConfig(raw json.RawMessage) (chatModelConfig, error) {
	config := chatModelConfig{}
	if len(raw) == 0 {
		return config, nil
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return chatModelConfig{}, err
	}
	config.ModelConfigID = parseModelConfigID(raw)
	return config, nil
}

func parseModelConfigID(raw json.RawMessage) *uuid.UUID {
	if len(raw) == 0 {
		return nil
	}

	decoded := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}

	modelConfigIDRaw, ok := decoded["model_config_id"]
	if !ok || len(modelConfigIDRaw) == 0 {
		return nil
	}

	var modelConfigIDText string
	if err := json.Unmarshal(modelConfigIDRaw, &modelConfigIDText); err != nil {
		return nil
	}
	modelConfigIDText = strings.TrimSpace(modelConfigIDText)
	if modelConfigIDText == "" {
		return nil
	}

	modelConfigID, err := uuid.Parse(modelConfigIDText)
	if err != nil || modelConfigID == uuid.Nil {
		return nil
	}

	return uuidPointer(modelConfigID)
}

func resolveMessageModelConfigID(
	explicitModelConfigID uuid.UUID,
	modelConfigRaw json.RawMessage,
) *uuid.UUID {
	if explicitModelConfigID != uuid.Nil {
		return uuidPointer(explicitModelConfigID)
	}
	config, err := parseChatModelConfig(modelConfigRaw)
	if err != nil {
		return nil
	}
	return config.ModelConfigID
}

func resolveMessageModelConfigIDFromChat(
	explicitModelConfigID *uuid.UUID,
	chat database.Chat,
) *uuid.UUID {
	if explicitModelConfigID != nil {
		return explicitModelConfigID
	}
	return uuidPointer(chat.LastModelConfigID)
}

func uuidPointer(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	idCopy := id
	return &idCopy
}

func modelConfigIDNullUUID(modelConfigID *uuid.UUID) uuid.NullUUID {
	if modelConfigID == nil || *modelConfigID == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{
		UUID:  *modelConfigID,
		Valid: true,
	}
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

func modelFromConfig(
	config chatModelConfig,
	providerKeys chatprovider.ProviderAPIKeys,
) (fantasy.LanguageModel, error) {
	return chatprovider.ModelFromConfig(config.Provider, config.Model, providerKeys)
}

type waitForExternalAuthArgs struct {
	ProviderID     string `json:"provider_id"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
}

func (p *Server) agentTools(
	model fantasy.LanguageModel,
	chatState *database.Chat,
	chatStateMu *sync.Mutex,
	workspaceMu *sync.Mutex,
	getWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error),
) []fantasy.AgentTool {
	currentChat := func() database.Chat {
		chatStateMu.Lock()
		snapshot := *chatState
		chatStateMu.Unlock()
		return snapshot
	}

	ownerID := chatState.OwnerID

	tools := []fantasy.AgentTool{
		chattool.ListTemplates(chattool.ListTemplatesOptions{
			DB:      p.db,
			OwnerID: ownerID,
		}),
		chattool.ReadTemplate(chattool.ReadTemplateOptions{
			DB:      p.db,
			OwnerID: ownerID,
		}),
		chattool.CreateWorkspace(chattool.CreateWorkspaceOptions{
			DB:          p.db,
			OwnerID:     ownerID,
			ChatID:      chatState.ID,
			CreateFn:    p.createWorkspaceFn,
			AgentConnFn: chattool.AgentConnFunc(p.agentConnFn),
			WorkspaceMu: workspaceMu,
		}),
		chattool.ReadFile(chattool.ReadFileOptions{
			GetWorkspaceConn: getWorkspaceConn,
		}),
		chattool.WriteFile(chattool.WriteFileOptions{
			GetWorkspaceConn: getWorkspaceConn,
		}),
		chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: getWorkspaceConn,
		}),
		chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: getWorkspaceConn,
		}),
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
				timeout = time.Duration(math.Min(float64(timeout), float64(defaultExternalAuthWait)))

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
	}

	tools = append(tools, p.subagentTools(currentChat)...)

	return tools
}

func (p *Server) waitForExternalAuth(
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
