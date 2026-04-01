package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/coder/coder/v2/coderd/webpush"
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

	homeInstructionLookupTimeout = 5 * time.Second
	instructionCacheTTL          = 5 * time.Minute
	chatHeartbeatInterval        = 30 * time.Second
	maxChatSteps                 = 1200
	// maxStreamBufferSize caps the number of events buffered
	// per chat during a single LLM step. When exceeded the
	// oldest event is evicted so memory stays bounded.
	maxStreamBufferSize = 10000

	// staleRecoveryIntervalDivisor determines how often the stale
	// recovery loop runs relative to the stale threshold. A value
	// of 5 means recovery runs at 1/5 of the stale-after duration.
	staleRecoveryIntervalDivisor = 5

	defaultSubagentInstruction = "You are running as a delegated sub-agent chat. Complete the delegated task and provide clear, concise assistant responses for the parent agent."
)

// Server handles background processing of pending chats.
type Server struct {
	cancel   context.CancelFunc
	closed   chan struct{}
	inflight sync.WaitGroup

	db       database.Store
	workerID uuid.UUID
	logger   slog.Logger

	subscribeFn SubscribeFn

	agentConnFn       AgentConnFunc
	createWorkspaceFn chattool.CreateWorkspaceFn
	startWorkspaceFn  chattool.StartWorkspaceFn
	pubsub            pubsub.Pubsub
	webpushDispatcher webpush.Dispatcher
	providerAPIKeys   chatprovider.ProviderAPIKeys

	// streamMu guards chatStreams which tracks in-flight chat
	// stream state for broadcasting ephemeral events.
	streamMu    sync.Mutex
	chatStreams map[uuid.UUID]*chatStreamState

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
	buffer      []codersdk.ChatStreamEvent
	buffering   bool
	subscribers map[uuid.UUID]chan codersdk.ChatStreamEvent
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

// CreateOptions controls chat creation in the shared chat mutation path.
type CreateOptions struct {
	OwnerID            uuid.UUID
	WorkspaceID        uuid.NullUUID
	ParentChatID       uuid.NullUUID
	RootChatID         uuid.NullUUID
	Title              string
	ModelConfigID      uuid.UUID
	SystemPrompt       string
	InitialUserContent []fantasy.Content
	// ContentFileIDs maps content block indices to their chat_files IDs
	// so the file_id can be preserved in the stored message JSON.
	ContentFileIDs map[int]uuid.UUID
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
	ChatID         uuid.UUID
	Content        []fantasy.Content
	ContentFileIDs map[int]uuid.UUID
	ModelConfigID  *uuid.UUID
	BusyBehavior   SendMessageBusyBehavior
}

// SendMessageResult contains the outcome of user message processing.
type SendMessageResult struct {
	Queued        bool
	QueuedMessage *database.ChatQueuedMessage
	Message       database.ChatMessage
	Chat          database.Chat
}

// EditMessageOptions controls in-place user message edits.
type EditMessageOptions struct {
	ChatID          uuid.UUID
	EditedMessageID int64
	Content         []fantasy.Content
	ContentFileIDs  map[int]uuid.UUID
}

// EditMessageResult contains the updated user message and chat status.
type EditMessageResult struct {
	Message database.ChatMessage
	Chat    database.Chat
}

// PromoteQueuedOptions controls queued-message promotion.
type PromoteQueuedOptions struct {
	ChatID          uuid.UUID
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

	var chat database.Chat
	txErr := p.db.InTx(func(tx database.Store) error {
		insertedChat, err := tx.InsertChat(ctx, database.InsertChatParams{
			OwnerID:           opts.OwnerID,
			WorkspaceID:       opts.WorkspaceID,
			ParentChatID:      opts.ParentChatID,
			RootChatID:        opts.RootChatID,
			LastModelConfigID: opts.ModelConfigID,
			Title:             opts.Title,
		})
		if err != nil {
			return xerrors.Errorf("insert chat: %w", err)
		}

		systemPrompt := strings.TrimSpace(opts.SystemPrompt)
		if systemPrompt != "" {
			systemContent, err := json.Marshal(systemPrompt)
			if err != nil {
				return xerrors.Errorf("marshal system prompt: %w", err)
			}
			_, err = tx.InsertChatMessage(ctx, database.InsertChatMessageParams{
				ChatID: insertedChat.ID,
				ModelConfigID: uuid.NullUUID{
					UUID:  opts.ModelConfigID,
					Valid: true,
				},
				Role: "system",
				Content: pqtype.NullRawMessage{
					RawMessage: systemContent,
					Valid:      len(systemContent) > 0,
				},
				Visibility:          database.ChatMessageVisibilityModel,
				InputTokens:         sql.NullInt64{},
				OutputTokens:        sql.NullInt64{},
				TotalTokens:         sql.NullInt64{},
				ReasoningTokens:     sql.NullInt64{},
				CacheCreationTokens: sql.NullInt64{},
				CacheReadTokens:     sql.NullInt64{},
				ContextLimit:        sql.NullInt64{},
				Compressed:          sql.NullBool{},
			})
			if err != nil {
				return xerrors.Errorf("insert system message: %w", err)
			}
		}

		userContent, err := chatprompt.MarshalContent(opts.InitialUserContent, opts.ContentFileIDs)
		if err != nil {
			return xerrors.Errorf("marshal initial user content: %w", err)
		}
		_, err = insertChatMessageWithStore(ctx, tx, database.InsertChatMessageParams{
			ChatID: insertedChat.ID,
			ModelConfigID: uuid.NullUUID{
				UUID:  opts.ModelConfigID,
				Valid: true,
			},
			Role:                "user",
			Content:             userContent,
			Visibility:          database.ChatMessageVisibilityBoth,
			InputTokens:         sql.NullInt64{},
			OutputTokens:        sql.NullInt64{},
			TotalTokens:         sql.NullInt64{},
			ReasoningTokens:     sql.NullInt64{},
			CacheCreationTokens: sql.NullInt64{},
			CacheReadTokens:     sql.NullInt64{},
			ContextLimit:        sql.NullInt64{},
			Compressed:          sql.NullBool{},
		})
		if err != nil {
			return xerrors.Errorf("insert initial user message: %w", err)
		}

		chat, err = setChatPendingWithStore(ctx, tx, insertedChat.ID)
		if err != nil {
			return xerrors.Errorf("set chat pending: %w", err)
		}

		if !chat.RootChatID.Valid && !chat.ParentChatID.Valid {
			chat.RootChatID = uuid.NullUUID{UUID: chat.ID, Valid: true}
		}
		return nil
	}, nil)
	if txErr != nil {
		return database.Chat{}, txErr
	}

	p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindCreated)
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

	content, err := chatprompt.MarshalContent(opts.Content, opts.ContentFileIDs)
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
		modelConfigID := lockedChat.LastModelConfigID
		if opts.ModelConfigID != nil {
			modelConfigID = *opts.ModelConfigID
		}

		// Both queue and interrupt behaviors queue messages
		// when the chat is busy. Interrupt additionally
		// signals the running loop to stop so the queued
		// message is promoted sooner. Crucially, this
		// guarantees the interrupted assistant response is
		// persisted (with a lower id/created_at) before the
		// user message is promoted into chat_messages,
		// preserving correct conversation order.
		if shouldQueueUserMessage(lockedChat.Status) {
			existingQueued, err := tx.GetChatQueuedMessages(ctx, opts.ChatID)
			if err != nil {
				return xerrors.Errorf("get queued messages: %w", err)
			}
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
	p.publishChatPubsubEvent(result.Chat, coderdpubsub.ChatEventKindStatusChange)
	return result, nil
}

// EditMessage updates a user message in-place, truncates all following messages,
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

	content, err := chatprompt.MarshalContent(opts.Content, opts.ContentFileIDs)
	if err != nil {
		return EditMessageResult{}, xerrors.Errorf("marshal message content: %w", err)
	}

	var result EditMessageResult
	txErr := p.db.InTx(func(tx database.Store) error {
		_, err := tx.GetChatByIDForUpdate(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("lock chat: %w", err)
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
		if existing.Role != "user" {
			return ErrEditedMessageNotUser
		}

		updatedMessage, err := tx.UpdateChatMessageByID(ctx, database.UpdateChatMessageByIDParams{
			ModelConfigID: uuid.NullUUID{},
			Content:       content,
			ID:            opts.EditedMessageID,
		})
		if err != nil {
			return xerrors.Errorf("update chat message: %w", err)
		}

		err = tx.DeleteChatMessagesAfterID(ctx, database.DeleteChatMessagesAfterIDParams{
			ChatID:  opts.ChatID,
			AfterID: opts.EditedMessageID,
		})
		if err != nil {
			return xerrors.Errorf("delete later chat messages: %w", err)
		}

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

		result.Message = updatedMessage
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
	p.publishChatPubsubEvent(result.Chat, coderdpubsub.ChatEventKindStatusChange)

	return result, nil
}

// ArchiveChat archives a chat and all descendants, then broadcasts a deleted event.
func (p *Server) ArchiveChat(ctx context.Context, chatID uuid.UUID) error {
	if chatID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}

	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("get chat: %w", err)
	}

	if err := p.db.ArchiveChatByID(ctx, chatID); err != nil {
		return xerrors.Errorf("archive chat: %w", err)
	}

	p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindDeleted)
	return nil
}

// UnarchiveChat unarchives a chat and publishes a created event so sidebar
// clients are notified that the chat has reappeared.
func (p *Server) UnarchiveChat(ctx context.Context, chatID uuid.UUID) error {
	if chatID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}

	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("get chat: %w", err)
	}

	if err := p.db.UnarchiveChatByID(ctx, chatID); err != nil {
		return xerrors.Errorf("unarchive chat: %w", err)
	}

	p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindCreated)
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
	p.publishChatPubsubEvent(updatedChat, coderdpubsub.ChatEventKindStatusChange)

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

func setChatPendingWithStore(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
) (database.Chat, error) {
	chat, err := store.GetChatByID(ctx, chatID)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("get chat: %w", err)
	}
	if chat.Status == database.ChatStatusPending {
		return chat, nil
	}

	updatedChat, err := store.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusPending,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   sql.NullString{},
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("set chat pending: %w", err)
	}
	return updatedChat, nil
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
	p.publishChatPubsubEvent(updatedChat, coderdpubsub.ChatEventKindStatusChange)
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

func insertUserMessageAndSetPending(
	ctx context.Context,
	store database.Store,
	lockedChat database.Chat,
	modelConfigID uuid.UUID,
	content pqtype.NullRawMessage,
) (database.ChatMessage, database.Chat, error) {
	message, err := insertChatMessageWithStore(ctx, store, database.InsertChatMessageParams{
		ChatID:              lockedChat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: modelConfigID, Valid: true},
		Role:                "user",
		Content:             content,
		Visibility:          database.ChatMessageVisibilityBoth,
		InputTokens:         sql.NullInt64{},
		OutputTokens:        sql.NullInt64{},
		TotalTokens:         sql.NullInt64{},
		ReasoningTokens:     sql.NullInt64{},
		CacheCreationTokens: sql.NullInt64{},
		CacheReadTokens:     sql.NullInt64{},
		ContextLimit:        sql.NullInt64{},
		Compressed:          sql.NullBool{},
	})
	if err != nil {
		return database.ChatMessage{}, database.Chat{}, err
	}

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
	Logger                     slog.Logger
	Database                   database.Store
	ReplicaID                  uuid.UUID
	SubscribeFn                SubscribeFn
	PendingChatAcquireInterval time.Duration
	InFlightChatStaleAfter     time.Duration
	AgentConn                  AgentConnFunc
	CreateWorkspace            chattool.CreateWorkspaceFn
	StartWorkspace             chattool.StartWorkspaceFn
	Pubsub                     pubsub.Pubsub
	ProviderAPIKeys            chatprovider.ProviderAPIKeys
	WebpushDispatcher          webpush.Dispatcher
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
		subscribeFn:                cfg.SubscribeFn,
		agentConnFn:                cfg.AgentConn,
		createWorkspaceFn:          cfg.CreateWorkspace,
		startWorkspaceFn:           cfg.StartWorkspace,
		pubsub:                     cfg.Pubsub,
		webpushDispatcher:          cfg.WebpushDispatcher,
		providerAPIKeys:            cfg.ProviderAPIKeys,
		chatStreams:                make(map[uuid.UUID]*chatStreamState),
		instructionCache:           make(map[uuid.UUID]cachedInstruction),
		pendingChatAcquireInterval: pendingChatAcquireInterval,
		inFlightChatStaleAfter:     inFlightChatStaleAfter,
	}

	//nolint:gocritic // The chat processor uses a scoped chatd context.
	ctx = dbauthz.AsChatd(ctx)
	go p.start(ctx)

	return p
}

func (p *Server) start(ctx context.Context) {
	defer close(p.closed)

	// Recover stale chats on startup and periodically thereafter
	// to handle chats orphaned by crashed or redeployed workers.
	p.recoverStaleChats(ctx)

	acquireTicker := time.NewTicker(p.pendingChatAcquireInterval)
	defer acquireTicker.Stop()

	staleRecoveryInterval := p.inFlightChatStaleAfter / staleRecoveryIntervalDivisor
	staleTicker := time.NewTicker(staleRecoveryInterval)
	defer staleTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-acquireTicker.C:
			p.processOnce(ctx)
		case <-staleTicker.C:
			p.recoverStaleChats(ctx)
		}
	}
}

func (p *Server) processOnce(ctx context.Context) {
	// Bail out early if the server is shutting down. The main
	// loop's select can randomly pick the ticker over ctx.Done(),
	// so we must guard against acquiring a chat we cannot process.
	if ctx.Err() != nil {
		return
	}

	// Try to acquire a pending chat. We detach from the server
	// lifetime to prevent a phantom-acquire race: when the server
	// context is canceled, the pq driver's watchCancel goroutine
	// races with the actual query on the wire. The UPDATE can
	// commit in Postgres (setting the chat to "running") before
	// the cancel request arrives via a second TCP connection, yet
	// the Go driver still returns context.Canceled to the caller
	// because the awaitDone goroutine in database/sql closes the
	// Rows before Scan reads them. This leaves the chat stuck as
	// "running" with no goroutine to process it. Using a context
	// that cannot be canceled ensures the driver sees the query
	// result if Postgres executed it.
	acquireCtx, acquireCancel := context.WithTimeout(
		context.WithoutCancel(ctx), 10*time.Second,
	)
	defer acquireCancel()
	chat, err := p.db.AcquireChat(acquireCtx, database.AcquireChatParams{
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

	// If the server context was canceled while we were acquiring,
	// release the chat back to pending immediately so another
	// replica can pick it up.
	if ctx.Err() != nil {
		releaseCtx, releaseCancel := context.WithTimeout(
			context.WithoutCancel(ctx), 10*time.Second,
		)
		defer releaseCancel()
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
		return
	}

	// Process the chat (don't block the main loop).
	p.inflight.Add(1)
	go func() {
		defer p.inflight.Done()
		p.processChat(ctx, chat)
	}()
}

func (p *Server) publishToStream(chatID uuid.UUID, event codersdk.ChatStreamEvent) {
	p.streamMu.Lock()
	state := p.streamStateLocked(chatID)
	if event.Type == codersdk.ChatStreamEventTypeMessagePart {
		if !state.buffering {
			p.cleanupStreamIfIdleLocked(chatID, state)
			p.streamMu.Unlock()
			return
		}
		if len(state.buffer) >= maxStreamBufferSize {
			p.logger.Warn(context.Background(), "chat stream buffer full, dropping oldest event",
				slog.F("chat_id", chatID), slog.F("buffer_size", len(state.buffer)))
			state.buffer = state.buffer[1:]
		}
		state.buffer = append(state.buffer, event)
	}
	subscribers := make([]chan codersdk.ChatStreamEvent, 0, len(state.subscribers))
	for _, ch := range state.subscribers {
		subscribers = append(subscribers, ch)
	}
	p.streamMu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
			p.logger.Warn(context.Background(), "dropping chat stream event",
				slog.F("chat_id", chatID), slog.F("type", event.Type))
		}
	}

	// Clean up the stream entry if it was created by
	// streamStateLocked but has no subscribers and is not
	// actively buffering (e.g. publish with no watchers).
	p.streamMu.Lock()
	if cur, ok := p.chatStreams[chatID]; ok {
		p.cleanupStreamIfIdleLocked(chatID, cur)
	}
	p.streamMu.Unlock()
}

func (p *Server) subscribeToStream(chatID uuid.UUID) (
	[]codersdk.ChatStreamEvent,
	<-chan codersdk.ChatStreamEvent,
	func(),
) {
	p.streamMu.Lock()
	state := p.streamStateLocked(chatID)
	snapshot := append([]codersdk.ChatStreamEvent(nil), state.buffer...)
	id := uuid.New()
	ch := make(chan codersdk.ChatStreamEvent, 128)
	state.subscribers[id] = ch
	p.streamMu.Unlock()

	cancel := func() {
		p.streamMu.Lock()
		state, ok := p.chatStreams[chatID]
		if ok {
			// Remove the subscriber but do not close the channel.
			// publishToStream copies subscriber references under
			// streamMu then sends outside the lock; closing here
			// races with that send and can panic. The channel
			// becomes unreachable once removed and will be GC'd.
			delete(state.subscribers, id)
			p.cleanupStreamIfIdleLocked(chatID, state)
		}
		p.streamMu.Unlock()
	}

	return snapshot, ch, cancel
}

// cleanupStreamIfIdleLocked removes the chat entry when there
// are no subscribers and the stream is not buffering. The
// caller must hold p.streamMu.
func (p *Server) cleanupStreamIfIdleLocked(chatID uuid.UUID, state *chatStreamState) {
	if !state.buffering && len(state.subscribers) == 0 {
		delete(p.chatStreams, chatID)
	}
}

func (p *Server) streamStateLocked(chatID uuid.UUID) *chatStreamState {
	state, ok := p.chatStreams[chatID]
	if !ok {
		state = &chatStreamState{subscribers: make(map[uuid.UUID]chan codersdk.ChatStreamEvent)}
		p.chatStreams[chatID] = state
	}
	return state
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

	// Subscribe to local stream for message_parts (ephemeral).
	localSnapshot, localParts, localCancel := p.subscribeToStream(chatID)

	// Merge all event sources.
	mergedCtx, mergedCancel := context.WithCancel(ctx)
	mergedEvents := make(chan codersdk.ChatStreamEvent, 128)

	var allCancels []func()
	allCancels = append(allCancels, localCancel)

	// Subscribe to pubsub for durable events (status, messages,
	// queue updates, errors). When pubsub is nil (e.g. in-memory
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

	// Track the last message ID we've seen for DB queries.
	// Initialize from afterMessageID so that when the caller passes
	// afterMessageID > 0 but no new messages exist yet, the first
	// pubsub catch-up doesn't re-fetch already-seen messages.
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
					afterID := lastMessageID
					if notify.FullRefresh {
						afterID = 0
					}
					newMessages, msgErr := p.db.GetChatMessagesByChatID(mergedCtx, database.GetChatMessagesByChatIDParams{
						ChatID:  chatID,
						AfterID: afterID,
					})
					if msgErr != nil {
						p.logger.Warn(mergedCtx, "failed to get chat messages after pubsub notification",
							slog.F("chat_id", chatID),
							slog.Error(msgErr),
						)
					} else {
						for _, msg := range newMessages {
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
					// Only forward message_part events from local
					// (durable events come via pubsub).
					if event.Type == codersdk.ChatStreamEventTypeMessagePart {
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
	sdkChat := codersdk.Chat{
		ID:        chat.ID,
		OwnerID:   chat.OwnerID,
		Title:     chat.Title,
		Status:    codersdk.ChatStatus(chat.Status),
		CreatedAt: chat.CreatedAt,
		UpdatedAt: chat.UpdatedAt,
	}
	if chat.ParentChatID.Valid {
		parentChatID := chat.ParentChatID.UUID
		sdkChat.ParentChatID = &parentChatID
	}
	if chat.RootChatID.Valid {
		rootChatID := chat.RootChatID.UUID
		sdkChat.RootChatID = &rootChatID
	} else if !chat.ParentChatID.Valid {
		rootChatID := chat.ID
		sdkChat.RootChatID = &rootChatID
	}
	if chat.WorkspaceID.Valid {
		sdkChat.WorkspaceID = &chat.WorkspaceID.UUID
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

	p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindDiffStatusChange)
	return nil
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

// publishEditedMessage is like publishMessage but uses
// AfterMessageID=0 so remote subscribers re-fetch from the
// beginning, ensuring the edit is never silently dropped.
func (p *Server) publishEditedMessage(chatID uuid.UUID, message database.ChatMessage) {
	sdkMessage := db2sdk.ChatMessage(message)
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type:    codersdk.ChatStreamEventTypeMessage,
		Message: &sdkMessage,
	})
	p.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{
		FullRefresh: true,
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
				Data:      f.Data,
				MediaType: f.Mimetype,
			}
		}
		return result, nil
	}
}

func (p *Server) processChat(ctx context.Context, chat database.Chat) {
	logger := p.logger.With(slog.F("chat_id", chat.ID))
	logger.Info(ctx, "processing chat request")

	chatCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	controlCancel := p.subscribeChatControl(chatCtx, chat.ID, cancel, logger)
	defer func() {
		if controlCancel != nil {
			controlCancel()
		}
	}()

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
			}
		}
	}()

	// Start buffering stream events BEFORE publishing the running
	// status. This closes a race where a subscriber sees
	// status=running but misses message_part events because
	// buffering hasn't started yet — the subscriber gets an empty
	// snapshot and publishToStream drops message_parts while
	// buffering is false.
	p.streamMu.Lock()
	startState := p.streamStateLocked(chat.ID)
	startState.buffer = nil
	startState.buffering = true
	p.streamMu.Unlock()
	defer func() {
		p.streamMu.Lock()
		if stopState, ok := p.chatStreams[chat.ID]; ok {
			stopState.buffer = nil
			stopState.buffering = false
			p.cleanupStreamIfIdleLocked(chat.ID, stopState)
		}
		p.streamMu.Unlock()
	}()

	p.publishStatus(chat.ID, database.ChatStatusRunning, uuid.NullUUID{
		UUID:  p.workerID,
		Valid: true,
	})

	// Determine the final status and last error to set when we're done.
	status := database.ChatStatusWaiting
	wasInterrupted := false
	lastError := ""
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
			p.publishError(chat.ID, lastError)
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
			} else if status == database.ChatStatusWaiting {
				// Try to auto-promote the next queued message.
				nextQueued, popErr := tx.PopNextQueuedMessage(cleanupCtx, chat.ID)
				if popErr == nil {
					msg, insertErr := tx.InsertChatMessage(cleanupCtx, database.InsertChatMessageParams{
						ChatID:        chat.ID,
						ModelConfigID: uuid.NullUUID{UUID: latestChat.LastModelConfigID, Valid: true},
						Role:          "user",
						Content: pqtype.NullRawMessage{
							RawMessage: nextQueued.Content,
							Valid:      len(nextQueued.Content) > 0,
						},
						Visibility:          database.ChatMessageVisibilityBoth,
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
						logger.Error(cleanupCtx, "failed to promote queued message",
							slog.F("queued_message_id", nextQueued.ID), slog.Error(insertErr))
					} else {
						status = database.ChatStatusPending
						promotedMessage = &msg

						remaining, qErr := tx.GetChatQueuedMessages(cleanupCtx, chat.ID)
						if qErr == nil {
							remainingQueuedMessages = remaining
							shouldPublishQueueUpdate = true
						}
					}
				}
			}

			_, updateErr := tx.UpdateChatStatus(cleanupCtx, database.UpdateChatStatusParams{
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
		// Re-read the chat from the database to pick up any title
		// changes made during processing (e.g. AI-generated titles
		// from maybeGenerateChatTitle). The local `chat` variable
		// is a value copy and won't reflect updates made in runChat.
		if freshChat, readErr := p.db.GetChatByID(cleanupCtx, chat.ID); readErr == nil {
			chat = freshChat
		} else {
			logger.Warn(cleanupCtx, "failed to re-read chat for status event",
				slog.F("chat_id", chat.ID), slog.Error(readErr))
		}
		chat.Status = status
		p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindStatusChange)

		if !wasInterrupted {
			p.maybeSendPushNotification(cleanupCtx, chat, status, lastError, logger)
		}
	}()

	if err := p.runChat(chatCtx, chat, logger); err != nil {
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
		if reason, ok := processingFailureReason(err); ok {
			lastError = reason
			p.publishError(chat.ID, lastError)
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

func (p *Server) runChat(
	ctx context.Context,
	chat database.Chat,
	logger slog.Logger,
) error {
	model, modelConfig, providerKeys, err := p.resolveChatModel(ctx, chat)
	if err != nil {
		return err
	}

	var callConfig codersdk.ChatModelCallConfig
	if len(modelConfig.Options) > 0 {
		if err := json.Unmarshal(modelConfig.Options, &callConfig); err != nil {
			return xerrors.Errorf("parse model call config: %w", err)
		}
	}

	messages, err := p.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	if err != nil {
		return xerrors.Errorf("get chat messages: %w", err)
	}
	// Fire title generation asynchronously so it doesn't block the
	// chat response. It uses a detached context so it can finish
	// even after the chat processing context is canceled.
	p.inflight.Add(1)
	go func() {
		defer p.inflight.Done()
		p.maybeGenerateChatTitle(context.WithoutCancel(ctx), chat, messages, model, providerKeys, logger)
	}()

	prompt, err := chatprompt.ConvertMessagesWithFiles(ctx, messages, p.chatFileResolver())
	if err != nil {
		return xerrors.Errorf("build chat prompt: %w", err)
	}
	if chat.ParentChatID.Valid {
		prompt = chatprompt.InsertSystem(prompt, defaultSubagentInstruction)
	}

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

		if !chatSnapshot.WorkspaceID.Valid {
			refreshedChat, refreshErr := refreshChatWorkspaceSnapshot(
				ctx,
				chatSnapshot,
				loadChatSnapshot,
			)
			if refreshErr != nil {
				return nil, refreshErr
			}
			if refreshedChat.WorkspaceID.Valid {
				chatStateMu.Lock()
				currentChat = refreshedChat
				chatSnapshot = refreshedChat
				chatStateMu.Unlock()
			}
		}

		if !chatSnapshot.WorkspaceID.Valid {
			return nil, xerrors.New("chat has no workspace")
		}

		agents, err := p.db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(
			ctx,
			chatSnapshot.WorkspaceID.UUID,
		)
		if err != nil || len(agents) == 0 {
			return nil, xerrors.New("chat has no workspace agent")
		}

		agentConn, agentRelease, err := p.agentConnFn(ctx, agents[0].ID)
		if err != nil {
			return nil, xerrors.Errorf("connect to workspace agent: %w", err)
		}

		chatStateMu.Lock()
		if conn == nil {
			conn = agentConn
			releaseConn = agentRelease

			var ancestorIDs []string
			if chatSnapshot.ParentChatID.Valid {
				ancestorIDs = append(ancestorIDs, chatSnapshot.ParentChatID.UUID.String())
			}
			ancestorJSON, err := json.Marshal(ancestorIDs)
			if err != nil {
				logger.Warn(ctx, "failed to marshal ancestor chat IDs", slog.Error(err))
				ancestorJSON = []byte("[]")
			}
			agentConn.SetExtraHeaders(http.Header{
				workspacesdk.CoderChatIDHeader:          {chatSnapshot.ID.String()},
				workspacesdk.CoderAncestorChatIDsHeader: {string(ancestorJSON)},
			})

			chatStateMu.Unlock()
			return agentConn, nil
		}
		currentConn := conn
		chatStateMu.Unlock()

		agentRelease()
		return currentConn, nil
	}

	if instruction := p.resolveInstructions(ctx, chat, getWorkspaceConn); instruction != "" {
		prompt = chatprompt.InsertSystem(prompt, instruction)
	}
	if userPrompt := p.resolveUserPrompt(ctx, chat.OwnerID); userPrompt != "" {
		prompt = chatprompt.InsertSystem(prompt, userPrompt)
	}

	// Use the model config's context_limit as a fallback when the LLM	// provider doesn't include context_limit in its response metadata
	// (which is the common case).
	modelConfigContextLimit := modelConfig.ContextLimit

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
		// with the appropriate roles.
		var assistantBlocks []fantasy.Content
		var toolResults []fantasy.ToolResultContent
		for _, block := range step.Content {
			if tr, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok {
				toolResults = append(toolResults, tr)
				continue
			}
			if trPtr, ok := fantasy.AsContentType[*fantasy.ToolResultContent](block); ok && trPtr != nil {
				toolResults = append(toolResults, *trPtr)
				continue
			}
			assistantBlocks = append(assistantBlocks, block)
		}

		var insertedMessages []database.ChatMessage
		err := p.db.InTx(func(tx database.Store) error {
			// Verify this worker still owns the chat before
			// inserting messages. This closes the race where
			// EditMessage truncates history and clears worker_id
			// while persistInterruptedStep (which uses an
			// uncancelable context) is still running.
			lockedChat, lockErr := tx.GetChatByIDForUpdate(persistCtx, chat.ID)
			if lockErr != nil {
				return xerrors.Errorf("lock chat for persist: %w", lockErr)
			}
			if !lockedChat.WorkerID.Valid || lockedChat.WorkerID.UUID != p.workerID {
				return chatloop.ErrInterrupted
			}

			if len(assistantBlocks) > 0 {
				assistantContent, marshalErr := chatprompt.MarshalContent(assistantBlocks, nil)
				if marshalErr != nil {
					return marshalErr
				}

				hasUsage := step.Usage != (fantasy.Usage{})
				assistantMessage, insertErr := tx.InsertChatMessage(persistCtx, database.InsertChatMessageParams{
					ChatID:        chat.ID,
					ModelConfigID: uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
					Role:          string(fantasy.MessageRoleAssistant),
					Content:       assistantContent,
					Visibility:    database.ChatMessageVisibilityBoth,
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
				if insertErr != nil {
					return xerrors.Errorf("insert assistant message: %w", insertErr)
				}
				insertedMessages = append(insertedMessages, assistantMessage)
			}

			for _, tr := range toolResults {
				resultContent, marshalErr := chatprompt.MarshalToolResultContent(tr)
				if marshalErr != nil {
					return marshalErr
				}

				toolMessage, insertErr := tx.InsertChatMessage(persistCtx, database.InsertChatMessageParams{
					ChatID:              chat.ID,
					ModelConfigID:       uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
					Role:                string(fantasy.MessageRoleTool),
					Content:             resultContent,
					Visibility:          database.ChatMessageVisibilityBoth,
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
					return xerrors.Errorf("insert tool result: %w", insertErr)
				}
				insertedMessages = append(insertedMessages, toolMessage)
			}

			return nil
		}, nil)
		if err != nil {
			return xerrors.Errorf("persist step transaction: %w", err)
		}

		for _, msg := range insertedMessages {
			p.publishMessage(chat.ID, msg)
		}

		// Clear the stream buffer now that the step is
		// persisted. Late-joining subscribers will load
		// these messages from the database instead.
		p.streamMu.Lock()
		if state, ok := p.chatStreams[chat.ID]; ok {
			state.buffer = nil
		}
		p.streamMu.Unlock()

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
	compactionOptions := &chatloop.CompactionOptions{
		ThresholdPercent: modelConfig.CompressionThreshold,
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
				slog.F("threshold_percent", result.ThresholdPercent),
				slog.F("usage_percent", result.UsagePercent),
				slog.F("context_tokens", result.ContextTokens),
				slog.F("context_limit", result.ContextLimit),
			)
			return nil
		},
		ToolCallID: compactionToolCallID,
		ToolName:   "chat_summarized",
		PublishMessagePart: func(role fantasy.MessageRole, part codersdk.ChatMessagePart) {
			p.publishMessagePart(chat.ID, string(role), part)
		},
		OnError: func(err error) {
			logger.Warn(ctx, "failed to compact chat context", slog.Error(err))
		},
	}

	// Here are all the tools we have for the chat.
	tools := []fantasy.AgentTool{
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
			ChatID:           chat.ID.String(),
		}),
		chattool.ProcessOutput(chattool.ProcessToolOptions{
			GetWorkspaceConn: getWorkspaceConn,
		}),
		chattool.ProcessList(chattool.ProcessToolOptions{
			GetWorkspaceConn: getWorkspaceConn,
		}),
		chattool.ProcessSignal(chattool.ProcessToolOptions{
			GetWorkspaceConn: getWorkspaceConn,
		}),
	}
	// Only root chats (not delegated subagents) get workspace
	// provisioning and subagent tools. Child agents must not
	// create workspaces or spawn further subagents — they should
	// focus on completing their delegated task.
	if !chat.ParentChatID.Valid {
		tools = append(tools,
			chattool.ListTemplates(chattool.ListTemplatesOptions{
				DB:      p.db,
				OwnerID: chat.OwnerID,
			}),
			chattool.ReadTemplate(chattool.ReadTemplateOptions{
				DB:      p.db,
				OwnerID: chat.OwnerID,
			}),
			chattool.CreateWorkspace(chattool.CreateWorkspaceOptions{
				DB:          p.db,
				OwnerID:     chat.OwnerID,
				ChatID:      chat.ID,
				CreateFn:    p.createWorkspaceFn,
				AgentConnFn: chattool.AgentConnFunc(p.agentConnFn),
				WorkspaceMu: &workspaceMu,
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
		tools = append(tools, p.subagentTools(func() database.Chat {
			return chat
		})...)
	}

	err = chatloop.Run(ctx, chatloop.RunOptions{
		Model:    model,
		Messages: prompt,
		Tools:    tools,
		MaxSteps: maxChatSteps,

		ModelConfig:     callConfig,
		ProviderOptions: chatprovider.ProviderOptionsFromChatModelConfig(model, callConfig.ProviderOptions),

		ContextLimitFallback: modelConfigContextLimit,

		PersistStep: persistStep,
		PublishMessagePart: func(
			role fantasy.MessageRole,
			part codersdk.ChatMessagePart,
		) {
			p.publishMessagePart(chat.ID, string(role), part)
		},
		Compaction: compactionOptions,
		ReloadMessages: func(reloadCtx context.Context) ([]fantasy.Message, error) {
			reloadedMsgs, err := p.db.GetChatMessagesForPromptByChatID(reloadCtx, chat.ID)
			if err != nil {
				return nil, xerrors.Errorf("reload chat messages: %w", err)
			}
			reloadedPrompt, err := chatprompt.ConvertMessagesWithFiles(reloadCtx, reloadedMsgs, p.chatFileResolver())
			if err != nil {
				return nil, xerrors.Errorf("convert reloaded messages: %w", err)
			}
			if chat.ParentChatID.Valid {
				reloadedPrompt = chatprompt.InsertSystem(reloadedPrompt, defaultSubagentInstruction)
			}
			if instruction := p.resolveInstructions(reloadCtx, chat, getWorkspaceConn); instruction != "" {
				reloadedPrompt = chatprompt.InsertSystem(reloadedPrompt, instruction)
			}
			if userPrompt := p.resolveUserPrompt(reloadCtx, chat.OwnerID); userPrompt != "" {
				reloadedPrompt = chatprompt.InsertSystem(reloadedPrompt, userPrompt)
			}
			return reloadedPrompt, nil
		},

		OnRetry: func(attempt int, retryErr error, delay time.Duration) {
			p.streamMu.Lock()
			if state, ok := p.chatStreams[chat.ID]; ok {
				state.buffer = nil
			}
			p.streamMu.Unlock()

			logger.Warn(ctx, "retrying LLM stream",
				slog.F("attempt", attempt),
				slog.F("delay", delay.String()),
				slog.Error(retryErr),
			)
			p.publishEvent(chat.ID, codersdk.ChatStreamEvent{
				Type:   codersdk.ChatStreamEventTypeRetry,
				ChatID: chat.ID,
				Retry: &codersdk.ChatStreamRetry{
					Attempt:    attempt,
					DelayMs:    delay.Milliseconds(),
					Error:      retryErr.Error(),
					RetryingAt: time.Now().Add(delay),
				},
			})
		},

		OnInterruptedPersistError: func(err error) {
			p.logger.Warn(ctx, "failed to persist interrupted chat step", slog.Error(err))
		},
	})
	return err
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

	systemContent, err := json.Marshal(result.SystemSummary)
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

	assistantContent, err := chatprompt.MarshalContent([]fantasy.Content{
		fantasy.ToolCallContent{
			ToolCallID: toolCallID,
			ToolName:   "chat_summarized",
			Input:      string(args),
		},
	}, nil)
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
	toolResult, err := chatprompt.MarshalToolResult(
		toolCallID,
		"chat_summarized",
		summaryResult,
		false,
	)
	if err != nil {
		return xerrors.Errorf("encode summary tool result: %w", err)
	}

	var insertedMessages []database.ChatMessage

	txErr := p.db.InTx(func(tx database.Store) error {
		_, txErr := tx.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID:        chatID,
			ModelConfigID: uuid.NullUUID{UUID: modelConfigID, Valid: true},
			Role:          string(fantasy.MessageRoleUser),
			Content: pqtype.NullRawMessage{
				RawMessage: systemContent,
				Valid:      len(systemContent) > 0,
			},
			Visibility:          database.ChatMessageVisibilityModel,
			Compressed:          sql.NullBool{Bool: true, Valid: true},
			InputTokens:         sql.NullInt64{},
			OutputTokens:        sql.NullInt64{},
			TotalTokens:         sql.NullInt64{},
			ReasoningTokens:     sql.NullInt64{},
			CacheCreationTokens: sql.NullInt64{},
			CacheReadTokens:     sql.NullInt64{},
			ContextLimit:        sql.NullInt64{},
		})
		if txErr != nil {
			return xerrors.Errorf("insert hidden summary message: %w", txErr)
		}

		assistantMessage, txErr := tx.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID:        chatID,
			ModelConfigID: uuid.NullUUID{UUID: modelConfigID, Valid: true},
			Role:          string(fantasy.MessageRoleAssistant),
			Content:       assistantContent,
			Visibility:    database.ChatMessageVisibilityUser,
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
		if txErr != nil {
			return xerrors.Errorf("insert summary tool call message: %w", txErr)
		}
		insertedMessages = append(insertedMessages, assistantMessage)

		toolMessage, txErr := tx.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID:        chatID,
			ModelConfigID: uuid.NullUUID{UUID: modelConfigID, Valid: true},
			Role:          string(fantasy.MessageRoleTool),
			Content:       toolResult,
			Visibility:    database.ChatMessageVisibilityBoth,
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
		if txErr != nil {
			return xerrors.Errorf("insert summary tool result message: %w", txErr)
		}
		insertedMessages = append(insertedMessages, toolMessage)

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
	dbConfig, err := p.resolveModelConfig(ctx, chat)
	if err != nil {
		return nil, database.ChatModelConfig{}, chatprovider.ProviderAPIKeys{}, xerrors.Errorf(
			"resolve model config: %w", err,
		)
	}

	providers, err := p.db.GetEnabledChatProviders(ctx)
	if err != nil {
		return nil, database.ChatModelConfig{}, chatprovider.ProviderAPIKeys{}, xerrors.Errorf(
			"get enabled chat providers: %w", err,
		)
	}
	dbProviders := make(
		[]chatprovider.ConfiguredProvider, 0, len(providers),
	)
	for _, provider := range providers {
		dbProviders = append(dbProviders, chatprovider.ConfiguredProvider{
			Provider: provider.Provider,
			APIKey:   provider.APIKey,
			BaseURL:  provider.BaseUrl,
		})
	}
	keys := chatprovider.MergeProviderAPIKeys(
		p.providerAPIKeys, dbProviders,
	)

	model, err := chatprovider.ModelFromConfig(
		dbConfig.Provider, dbConfig.Model, keys,
	)
	if err != nil {
		return nil, database.ChatModelConfig{}, chatprovider.ProviderAPIKeys{}, xerrors.Errorf(
			"create model: %w", err,
		)
	}
	return model, dbConfig, keys, nil
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
		modelConfig, err := p.db.GetChatModelConfigByID(
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

	defaultConfig, err := p.db.GetDefaultChatModelConfig(ctx)
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

// resolveInstructions returns the combined system instructions for the
// workspace agent. It reads the home-level (~/.coder/AGENTS.md) and
// working-directory-level (<pwd>/AGENTS.md) instruction files, combines
// them with agent metadata (OS, directory), and caches the result.
func (p *Server) resolveInstructions(
	ctx context.Context,
	chat database.Chat,
	getWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error),
) string {
	if !chat.WorkspaceID.Valid {
		return ""
	}

	agents, agentsErr := p.db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(
		ctx,
		chat.WorkspaceID.UUID,
	)
	if agentsErr != nil || len(agents) == 0 {
		return ""
	}
	agentID := agents[0].ID

	p.instructionCacheMu.Lock()
	cached, ok := p.instructionCache[agentID]
	p.instructionCacheMu.Unlock()

	if ok && time.Since(cached.fetchedAt) < instructionCacheTTL {
		return cached.instruction
	}

	// Look up the agent's OS and working directory.
	agent, err := p.db.GetWorkspaceAgentByID(ctx, agentID)
	if err != nil {
		p.logger.Debug(ctx, "failed to look up workspace agent for instruction context",
			slog.F("agent_id", agentID),
			slog.Error(err),
		)
	}
	directory := agent.ExpandedDirectory
	if directory == "" {
		directory = agent.Directory
	}

	// Read instruction files from the workspace agent.
	var sections []instructionFileSection
	if getWorkspaceConn != nil {
		instructionCtx, cancel := context.WithTimeout(ctx, homeInstructionLookupTimeout)
		defer cancel()

		conn, connErr := getWorkspaceConn(instructionCtx)
		if connErr != nil {
			p.logger.Debug(ctx, "failed to resolve workspace connection for instruction files",
				slog.F("chat_id", chat.ID),
				slog.Error(connErr),
			)
		} else {
			// ~/.coder/AGENTS.md
			if content, source, truncated, err := readHomeInstructionFile(instructionCtx, conn); err != nil {
				p.logger.Debug(ctx, "failed to load home instruction file",
					slog.F("chat_id", chat.ID), slog.Error(err))
			} else if content != "" {
				sections = append(sections, instructionFileSection{content, source, truncated})
			}

			// <pwd>/AGENTS.md
			if pwdPath := pwdInstructionFilePath(directory); pwdPath != "" {
				if content, source, truncated, err := readInstructionFile(instructionCtx, conn, pwdPath); err != nil {
					p.logger.Debug(ctx, "failed to load working directory instruction file",
						slog.F("chat_id", chat.ID), slog.F("directory", directory), slog.Error(err))
				} else if content != "" {
					sections = append(sections, instructionFileSection{content, source, truncated})
				}
			}
		}
	}

	instruction := formatSystemInstructions(agent.OperatingSystem, directory, sections)

	p.instructionCacheMu.Lock()
	p.instructionCache[agentID] = cachedInstruction{
		instruction: instruction,
		fetchedAt:   time.Now(),
	}
	p.instructionCacheMu.Unlock()

	return instruction
}

// resolveUserPrompt fetches the user's custom chat prompt from the
// database and wraps it in <user-instructions> tags. Returns empty
// string if no prompt is set.
func (p *Server) resolveUserPrompt(ctx context.Context, userID uuid.UUID) string {
	raw, err := p.db.GetUserChatCustomPrompt(ctx, userID)
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

			msg, err := p.db.GetLastChatMessageByRole(pushCtx, database.GetLastChatMessageByRoleParams{
				ChatID: chat.ID,
				Role:   "assistant",
			})
			if err == nil {
				content, parseErr := chatprompt.ParseContent(msg.Role, msg.Content)
				if parseErr == nil {
					assistantText := strings.TrimSpace(contentBlocksToText(content))
					if assistantText != "" {
						model, _, keys, resolveErr := p.resolveChatModel(pushCtx, chat)
						if resolveErr == nil {
							if summary := generatePushSummary(pushCtx, chat.Title, assistantText, model, keys, logger); summary != "" {
								pushBody = summary
							}
						}
					}
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
	p.cancel()
	<-p.closed
	p.inflight.Wait()
	return nil
}
