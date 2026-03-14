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
	"charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/sync/errgroup"
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
	chatHeartbeatInterval        = 60 * time.Second
	maxChatSteps                 = 1200
	// maxStreamBufferSize caps the number of events buffered
	// per chat during a single LLM step. When exceeded the
	// oldest event is evicted so memory stays bounded.
	maxStreamBufferSize = 10000

	// staleRecoveryIntervalDivisor determines how often the stale
	// recovery loop runs relative to the stale threshold. A value
	// of 5 means recovery runs at 1/5 of the stale-after duration.
	staleRecoveryIntervalDivisor = 5

	// maxAcquirePerCycle is the maximum number of pending
	// steps a single processOnce tick will acquire before
	// yielding back to the polling loop. This prevents a
	// single replica from blocking on acquisition when many
	// steps are queued and lets the timer tick catch up.
	maxAcquirePerCycle = 10

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

	// chatStreams stores per-chat stream state. Using sync.Map
	// gives each chat independent locking — concurrent chats
	// never contend with each other.
	chatStreams sync.Map // uuid.UUID -> *chatStreamState

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
	Status   codersdk.ChatStatus
	WorkerID uuid.UUID
}

// SubscribeFnParams carries the state that the enterprise
// SubscribeFn implementation needs from the OSS Subscribe preamble.
type SubscribeFnParams struct {
	ChatID              uuid.UUID
	Chat                database.Chat
	InitialStatus       codersdk.ChatStatus
	InitialWorkerID     uuid.UUID
	WorkerID            uuid.UUID
	StatusNotifications <-chan StatusNotification
	RequestHeader       http.Header
	DB                  database.Store
	Logger              slog.Logger
}

type chatStreamState struct {
	mu          sync.Mutex
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
)

// CreateOptions controls chat creation in the shared chat mutation path.
type CreateOptions struct {
	OwnerID            uuid.UUID
	WorkspaceID        uuid.NullUUID
	ParentChatID       uuid.NullUUID
	RootChatID         uuid.NullUUID
	Title              string
	ModelConfigID      uuid.UUID
	ChatMode           database.NullChatMode
	SystemPrompt       string
	InitialUserContent []codersdk.ChatMessagePart
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
	CreatedBy       uuid.UUID
	EditedMessageID int64
	Content         []codersdk.ChatMessagePart
}

// EditMessageResult contains the updated user message and chat status.
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

	var chat database.Chat
	txErr := p.db.InTx(func(tx database.Store) error {
		insertedChat, err := tx.InsertChat(ctx, database.InsertChatParams{
			OwnerID:           opts.OwnerID,
			WorkspaceID:       opts.WorkspaceID,
			ParentChatID:      opts.ParentChatID,
			RootChatID:        opts.RootChatID,
			LastModelConfigID: opts.ModelConfigID,
			Title:             opts.Title,
			Mode:              opts.ChatMode,
		})
		if err != nil {
			return xerrors.Errorf("insert chat: %w", err)
		}

		systemPrompt := strings.TrimSpace(opts.SystemPrompt)
		if systemPrompt != "" {
			systemContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
				codersdk.ChatMessageText(systemPrompt),
			})
			if err != nil {
				return xerrors.Errorf("marshal system prompt: %w", err)
			}
			_, err = tx.InsertChatMessage(ctx, database.InsertChatMessageParams{
				ChatID:    insertedChat.ID,
				CreatedBy: uuid.NullUUID{},
				ModelConfigID: uuid.NullUUID{
					UUID:  opts.ModelConfigID,
					Valid: true,
				},
				ChatRunID:           uuid.NullUUID{},
				ChatRunStepID:       uuid.NullUUID{},
				Role:                database.ChatMessageRoleSystem,
				ContentVersion:      chatprompt.CurrentContentVersion,
				Content:             systemContent,
				Visibility:          database.ChatMessageVisibilityModel,
				Compressed:          sql.NullBool{},
			})
			if err != nil {
				return xerrors.Errorf("insert system message: %w", err)
			}
		}

		userContent, err := chatprompt.MarshalParts(opts.InitialUserContent)
		if err != nil {
			return xerrors.Errorf("marshal initial user content: %w", err)
		}
		_, err = insertChatMessageWithStore(ctx, tx, database.InsertChatMessageParams{
			ChatID: insertedChat.ID,
			ModelConfigID: uuid.NullUUID{
				UUID:  opts.ModelConfigID,
				Valid: true,
			},
			ChatRunID:           uuid.NullUUID{},
			ChatRunStepID:       uuid.NullUUID{},
			Role:                database.ChatMessageRoleUser,
			ContentVersion:      chatprompt.CurrentContentVersion,
			Content:             userContent,
			CreatedBy:           uuid.NullUUID{UUID: opts.OwnerID, Valid: opts.OwnerID != uuid.Nil},
			Visibility:          database.ChatMessageVisibilityBoth,
			Compressed:          sql.NullBool{},
		})
		if err != nil {
			return xerrors.Errorf("insert initial user message: %w", err)
		}

		chat, err = tx.GetChatByID(ctx, insertedChat.ID)
		if err != nil {
			return xerrors.Errorf("reload chat: %w", err)
		}

		if !chat.RootChatID.Valid && !chat.ParentChatID.Valid {
			chat.RootChatID = uuid.NullUUID{UUID: chat.ID, Valid: true}
		}

		// Create run+step inside the TX so the chat enters
		// pending status atomically with creation.
		if err := createRunAndStep(ctx, tx, insertedChat.ID); err != nil {
			return xerrors.Errorf("create run and step: %w", err)
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
		busy, busyErr := p.isChatBusy(ctx, opts.ChatID)
		if busyErr != nil {
			return xerrors.Errorf("check chat busy: %w", busyErr)
		}
		if busy {
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

		message, err := insertChatMessageWithStore(ctx, tx, database.InsertChatMessageParams{
			ChatID:              lockedChat.ID,
			ModelConfigID:       uuid.NullUUID{UUID: modelConfigID, Valid: true},
			CreatedBy:           uuid.NullUUID{UUID: opts.CreatedBy, Valid: opts.CreatedBy != uuid.Nil},
			ChatRunID:           uuid.NullUUID{},
			ChatRunStepID:       uuid.NullUUID{},
			Role:                "user",
			Content:             content,
			Visibility:          database.ChatMessageVisibilityBoth,
			Compressed:          sql.NullBool{},
		})
		if err != nil {
			return xerrors.Errorf("insert user message: %w", err)
		}
		result.Message = message
		result.Chat = lockedChat

		// Create run+step inside the TX so the chat becomes
		// pending atomically with the message insert.
		if err := createRunAndStep(ctx, tx, lockedChat.ID); err != nil {
			return xerrors.Errorf("create run and step: %w", err)
		}
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
		// stop by interrupting the active step. The worker's
		// control subscriber detects the status change and
		// cancels with ErrInterrupted. The deferred cleanup
		// in processChat then auto-promotes the queued message
		// after persisting the partial assistant response.
		if busyBehavior == SendMessageBusyBehaviorInterrupt {
			if err := p.interruptActiveStep(ctx, opts.ChatID); err != nil {
				// The message is already queued so the chat is
				// not in a broken state — the user can still
				// wait for the current run to finish. Log the
				// error but don't fail the request.
				p.logger.Error(ctx, "failed to interrupt chat for queued message",
					slog.F("chat_id", opts.ChatID),
					slog.Error(err),
				)
			}
		}

		return result, nil
	}

	p.publishMessage(opts.ChatID, result.Message)
	p.publishStatus(opts.ChatID, codersdk.ChatStatusPending)
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

	content, err := chatprompt.MarshalParts(opts.Content)
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
		if existing.Role != database.ChatMessageRoleUser {
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

		// Interrupt any active step so the edit triggers a
		// fresh run. The createRunAndStep call below creates
		// a new run+step within this transaction.
		if interruptErr := tx.InterruptActiveChatRunStep(ctx, opts.ChatID); interruptErr != nil {
			p.logger.Warn(ctx, "interrupt active step during edit",
				slog.F("chat_id", opts.ChatID),
				slog.Error(interruptErr),
			)
		}

		result.Message = updatedMessage
		result.Chat, err = tx.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("get chat after edit: %w", err)
		}

		// Create run+step inside the TX so the edited message
		// triggers a fresh run atomically.
		if err := createRunAndStep(ctx, tx, opts.ChatID); err != nil {
			return xerrors.Errorf("create run and step: %w", err)
		}
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
	p.publishStatus(opts.ChatID, codersdk.ChatStatusPending)
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

		promoted, err = insertChatMessageWithStore(ctx, tx, database.InsertChatMessageParams{
			ChatID:        lockedChat.ID,
			ModelConfigID: uuid.NullUUID{UUID: modelConfigID, Valid: true},
			ChatRunID:     uuid.NullUUID{},
			ChatRunStepID: uuid.NullUUID{},
			Role:          "user",
			Content: pqtype.NullRawMessage{
				RawMessage: targetContent,
				Valid:      len(targetContent) > 0,
			},
			CreatedBy:           uuid.NullUUID{UUID: opts.CreatedBy, Valid: opts.CreatedBy != uuid.Nil},
			Visibility:          database.ChatMessageVisibilityBoth,
			Compressed:          sql.NullBool{},
		})
		if err != nil {
			return xerrors.Errorf("insert promoted message: %w", err)
		}
		remainingQueue, err = tx.GetChatQueuedMessages(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("get remaining queue: %w", err)
		}
		result.PromotedMessage = promoted

		// Create run+step inside the TX so the promoted
		// message gets processed atomically.
		if err := createRunAndStep(ctx, tx, lockedChat.ID); err != nil {
			return xerrors.Errorf("create run and step: %w", err)
		}
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
	p.publishStatus(opts.ChatID, codersdk.ChatStatusPending)

	// Re-read the chat from the database for the pubsub event.
	if freshChat, readErr := p.db.GetChatByID(ctx, opts.ChatID); readErr == nil {
		p.publishChatPubsubEvent(freshChat, coderdpubsub.ChatEventKindStatusChange)
	}

	return result, nil
}

// InterruptChat interrupts the active run step and broadcasts
// status updates. Returns the refreshed chat row.
func (p *Server) InterruptChat(
	ctx context.Context,
	chat database.Chat,
) database.Chat {
	if chat.ID == uuid.Nil {
		return chat
	}

	if err := p.interruptActiveStep(ctx, chat.ID); err != nil {
		p.logger.Error(ctx, "failed to interrupt active step",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return chat
	}

	updatedChat, err := p.db.GetChatByID(ctx, chat.ID)
	if err != nil {
		p.logger.Error(ctx, "failed to reload chat after interrupt",
			slog.F("chat_id", chat.ID),
			slog.Error(err),
		)
		return chat
	}
	return updatedChat
}

// RefreshStatus derives the current chat status from the
// run/step views and publishes it to stream subscribers.
func (p *Server) RefreshStatus(ctx context.Context, chatID uuid.UUID) error {
	if chatID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}

	status, err := p.deriveChatStatus(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("derive chat status: %w", err)
	}

	p.publishStatus(chatID, status)
	return nil
}

// interruptActiveStep marks the active step for a chat as
// interrupted and publishes a waiting status.
func (p *Server) interruptActiveStep(ctx context.Context, chatID uuid.UUID) error {
	if err := p.db.InterruptActiveChatRunStep(ctx, chatID); err != nil {
		return xerrors.Errorf("interrupt active step: %w", err)
	}
	p.publishStatus(chatID, codersdk.ChatStatusWaiting)
	if chat, err := p.db.GetChatByID(ctx, chatID); err == nil {
		p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindStatusChange)
	}
	return nil
}

// deriveChatStatus computes the current chat status by checking
// whether an active run step exists and its worker state.
func (p *Server) deriveChatStatus(ctx context.Context, chatID uuid.UUID) (codersdk.ChatStatus, error) {
	chatWithStatus, err := p.db.GetChatWithStatusByID(ctx, chatID)
	if err != nil {
		return "", xerrors.Errorf("get chat with status: %w", err)
	}
	return codersdk.ChatStatus(chatWithStatus.ComputedStatus), nil
}

// createRunAndStep inserts a new chat run and its first step
// using the provided store (which may be a transaction handle).
// If the partial unique index on active steps fires, the error
// is returned so the caller can decide how to handle it.
func createRunAndStep(ctx context.Context, store database.Store, chatID uuid.UUID) error {
	run, err := store.InsertChatRun(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("insert chat run: %w", err)
	}
	_, err = store.InsertChatRunStep(ctx, database.InsertChatRunStepParams{
		ChatRunID:     run.ID,
		ChatID:        chatID,
		ModelConfigID: uuid.NullUUID{},
	})
	if err != nil {
		return xerrors.Errorf("insert chat run step: %w", err)
	}
	return nil
}

// reconcileChatRun cleans up stalled steps and then atomically
// creates a new chat run and its first step. If an active step
// already exists (unique constraint violation), the operation is
// silently skipped. This is used by processChat's auto-promote
// path which runs outside of a user-facing transaction.
func (p *Server) reconcileChatRun(ctx context.Context, chatID uuid.UUID) error {
	// Phase 1: clean up stalled steps.
	if err := p.db.ErrorStalledChatRunSteps(ctx, database.ErrorStalledChatRunStepsParams{
		ChatID:         chatID,
		StaleThreshold: time.Now().Add(-p.inFlightChatStaleAfter),
	}); err != nil {
		return xerrors.Errorf("error stalled steps: %w", err)
	}

	// Phase 2: atomically create run + first step.
	err := p.db.InTx(func(tx database.Store) error {
		return createRunAndStep(ctx, tx, chatID)
	}, nil)
	// If the unique constraint fires, there's already an active step.
	if database.IsUniqueViolation(err) {
		return nil
	}
	return err
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

// isChatBusy reports whether the chat has an active (uncompleted,
// non-errored, non-interrupted) run step, meaning a new user message
// should be queued rather than triggering a new run.
func (p *Server) isChatBusy(ctx context.Context, chatID uuid.UUID) (bool, error) {
	_, err := p.db.GetActiveChatRunStep(ctx, chatID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
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

	// Recover stale run steps on startup and periodically thereafter
	// to handle steps orphaned by crashed or redeployed workers.
	p.recoverStaleChatRunSteps(ctx)

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
			p.recoverStaleChatRunSteps(ctx)
		}
	}
}

func (p *Server) processOnce(ctx context.Context) {
	for range maxAcquirePerCycle {
		if !p.acquireAndProcess(ctx) {
			return
		}
	}
}

// acquireAndProcess attempts to claim a single pending step and
// spawn a goroutine to process it. It returns true when work was
// found (the caller should try again), or false when no more work
// is available or the server is shutting down.
func (p *Server) acquireAndProcess(ctx context.Context) bool {
	// Bail out early if the server is shutting down. The main
	// loop's select can randomly pick the ticker over ctx.Done(),
	// so we must guard against acquiring a step we cannot process.
	if ctx.Err() != nil {
		return false
	}

	// Try to acquire an unclaimed active step. We detach from the
	// server lifetime to prevent a phantom-acquire race: when the
	// server context is canceled, the pq driver's watchCancel
	// goroutine races with the actual query on the wire. Using a
	// context that cannot be canceled ensures the driver sees the
	// query result if Postgres executed it.
	acquireCtx, acquireCancel := context.WithTimeout(
		context.WithoutCancel(ctx), 10*time.Second,
	)
	defer acquireCancel()
	run, err := p.db.AcquireChatRunStep(acquireCtx, p.workerID)
	if err != nil {
		if !xerrors.Is(err, sql.ErrNoRows) {
			p.logger.Error(ctx, "failed to acquire chat run step", slog.Error(err))
		}
		// No unclaimed steps or error.
		return false
	}

	// Fetch the chat and the active step.
	chat, err := p.db.GetChatByID(acquireCtx, run.ChatID)
	if err != nil {
		p.logger.Error(ctx, "failed to get chat for acquired run",
			slog.F("chat_id", run.ChatID), slog.Error(err))
		// Release the run so another replica can pick it up.
		if clearErr := p.db.ClearChatRunWorker(acquireCtx, run.ID); clearErr != nil {
			p.logger.Error(ctx, "failed to clear run worker",
				slog.F("run_id", run.ID), slog.Error(clearErr))
		}
		return false
	}
	step, err := p.db.GetActiveChatRunStep(acquireCtx, run.ChatID)
	if err != nil {
		p.logger.Error(ctx, "failed to get active step for acquired run",
			slog.F("chat_id", run.ChatID), slog.Error(err))
		if clearErr := p.db.ClearChatRunWorker(acquireCtx, run.ID); clearErr != nil {
			p.logger.Error(ctx, "failed to clear run worker",
				slog.F("run_id", run.ID), slog.Error(clearErr))
		}
		return false
	}

	// If the server context was canceled while we were acquiring,
	// release the run back so another replica can pick it up.
	if ctx.Err() != nil {
		releaseCtx, releaseCancel := context.WithTimeout(
			context.WithoutCancel(ctx), 10*time.Second,
		)
		defer releaseCancel()
		if clearErr := p.db.ClearChatRunWorker(releaseCtx, run.ID); clearErr != nil {
			p.logger.Error(ctx, "failed to release run acquired during shutdown",
				slog.F("run_id", run.ID), slog.Error(clearErr))
		}
		releaseCancel()
		return false
	}

	// Process the chat (don't block the main loop).
	p.inflight.Add(1)
	go func() {
		defer p.inflight.Done()
		p.processChat(ctx, chat, run, step)
	}()
	return true
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
	state.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
			p.logger.Warn(context.Background(), "dropping chat stream event",
				slog.F("chat_id", chatID), slog.F("type", event.Type))
		}
	}

	// Clean up the stream entry if it was created by
	// getOrCreateStreamState but has no subscribers and is not
	// actively buffering (e.g. publish with no watchers).
	state.mu.Lock()
	p.cleanupStreamIfIdle(chatID, state)
	state.mu.Unlock()
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
// The caller must hold state.mu.
func (p *Server) cleanupStreamIfIdle(chatID uuid.UUID, state *chatStreamState) {
	if !state.buffering && len(state.subscribers) == 0 {
		p.chatStreams.Delete(chatID)
	}
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
	// Derive status and active worker for the enterprise relay
	// and the initial status event.
	var initialStatus codersdk.ChatStatus
	var initialWorkerID uuid.UUID
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
		// Derive status from run/step state.
		status, statusErr := p.deriveChatStatus(ctx, chatID)
		if statusErr != nil {
			status = codersdk.ChatStatusWaiting
		}
		initialStatus = status
		// If the chat is running, find the worker from
		// the active step's run.
		if status == codersdk.ChatStatusRunning {
			if step, stepErr := p.db.GetActiveChatRunStep(ctx, chatID); stepErr == nil {
				if run, runErr := p.db.GetChatRunByID(ctx, step.ChatRunID); runErr == nil && run.WorkerID.Valid {
					initialWorkerID = run.WorkerID.UUID
				}
			}
		}
		statusEvent := codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypeStatus,
			ChatID: chatID,
			Status: &codersdk.ChatStreamStatus{
				Status: status,
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
			InitialStatus:       initialStatus,
			InitialWorkerID:     initialWorkerID,
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
					status := codersdk.ChatStatus(notify.Status)
					select {
					case <-mergedCtx.Done():
						return
					case mergedEvents <- codersdk.ChatStreamEvent{
						Type:   codersdk.ChatStreamEventTypeStatus,
						ChatID: chatID,
						Status: &codersdk.ChatStreamStatus{Status: status},
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

func (p *Server) publishStatus(chatID uuid.UUID, status codersdk.ChatStatus) {
	p.publishEvent(chatID, codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeStatus,
		Status: &codersdk.ChatStreamStatus{Status: status},
	})
	workerID := ""
	if status == codersdk.ChatStatusRunning {
		workerID = p.workerID.String()
	}
	notify := coderdpubsub.ChatStreamNotifyMessage{
		Status:   string(status),
		WorkerID: workerID,
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
		CreatedAt: chat.CreatedAt,
		UpdatedAt: chat.UpdatedAt,
	}
	// Status is derived from run/step state via deriveChatStatus.
	// We use a best-effort approach here — if the derivation
	// fails, we default to "waiting".
	if status, err := p.deriveChatStatus(dbauthz.AsSystemRestricted(context.Background()), chat.ID); err == nil {
		sdkChat.Status = status
	} else {
		sdkChat.Status = codersdk.ChatStatusWaiting
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
	status := codersdk.ChatStatus(strings.TrimSpace(notify.Status))
	switch status {
	case codersdk.ChatStatusWaiting, codersdk.ChatStatusPending, codersdk.ChatStatusError:
		return true
	case codersdk.ChatStatusRunning:
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

func (p *Server) processChat(ctx context.Context, chat database.Chat, run database.ChatRun, step database.ChatRunStep) {
	logger := p.logger.With(slog.F("chat_id", chat.ID), slog.F("run_id", run.ID), slog.F("step_id", step.ID))
	logger.Info(ctx, "processing chat request")

	chatCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	controlCancel := p.subscribeChatControl(chatCtx, chat.ID, cancel, logger)
	defer func() {
		if controlCancel != nil {
			controlCancel()
		}
	}()

	// Periodically update the heartbeat on the active step so other
	// replicas know this worker is still alive.
	go func() {
		ticker := time.NewTicker(chatHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-chatCtx.Done():
				return
			case <-ticker.C:
				rows, err := p.db.UpdateChatRunStepHeartbeat(chatCtx, database.UpdateChatRunStepHeartbeatParams{
					ID:       step.ID,
					WorkerID: p.workerID,
				})
				if err != nil {
					logger.Warn(chatCtx, "failed to update step heartbeat", slog.Error(err))
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
	streamState := p.getOrCreateStreamState(chat.ID)
	streamState.mu.Lock()
	streamState.buffer = nil
	streamState.buffering = true
	streamState.mu.Unlock()
	defer func() {
		streamState.mu.Lock()
		streamState.buffer = nil
		streamState.buffering = false
		p.cleanupStreamIfIdle(chat.ID, streamState)
		streamState.mu.Unlock()
	}()

	p.publishStatus(chat.ID, codersdk.ChatStatusRunning)

	// Determine the final status and last error to set when we're done.
	status := codersdk.ChatStatusWaiting
	wasInterrupted := false
	lastError := ""
	remainingQueuedMessages := []database.ChatQueuedMessage{}
	shouldPublishQueueUpdate := false
	var promotedMessage *database.ChatMessage

	defer func() {
		// Use a context that is not canceled by Close() so we can
		// reliably update step state during graceful shutdown.
		cleanupCtx := context.WithoutCancel(ctx)

		// Handle panics gracefully.
		if r := recover(); r != nil {
			logger.Error(cleanupCtx, "panic during chat processing", slog.F("panic", r))
			lastError = panicFailureReason(r)
			p.publishError(chat.ID, lastError)
			status = codersdk.ChatStatusError
		}

		switch status {
		case codersdk.ChatStatusError:
			// Mark the active step as errored.
			// Terminal-state guard: the query is a no-op if the step
			// was already completed/errored/interrupted.
			if _, errStep := p.db.ErrorChatRunStep(cleanupCtx, database.ErrorChatRunStepParams{
				ID:    step.ID,
				Error: lastError,
			}); errStep != nil && !errors.Is(errStep, sql.ErrNoRows) {
				logger.Error(cleanupCtx, "failed to error chat run step",
					slog.F("step_id", step.ID), slog.Error(errStep))
			}
			if errClear := p.db.ClearChatRunWorker(cleanupCtx, run.ID); errClear != nil {
				logger.Error(cleanupCtx, "failed to clear run worker after error",
					slog.F("run_id", run.ID), slog.Error(errClear))
			}

		case codersdk.ChatStatusPending:
			// Shutdown case: clear the worker so another replica
			// picks up the uncompleted step.
			if errClear := p.db.ClearChatRunWorker(cleanupCtx, run.ID); errClear != nil {
				logger.Error(cleanupCtx, "failed to clear run worker for pending",
					slog.F("run_id", run.ID), slog.Error(errClear))
			}

		case codersdk.ChatStatusWaiting:
			// Completed normally or was interrupted.
			if wasInterrupted {
				// Terminal-state guard: no-op if already terminal.
				if _, errInt := p.db.InterruptChatRunStep(cleanupCtx, step.ID); errInt != nil && !errors.Is(errInt, sql.ErrNoRows) {
					logger.Error(cleanupCtx, "failed to interrupt chat run step",
						slog.F("step_id", step.ID), slog.Error(errInt))
				}
			} else {
				// Steps are completed inside PersistStep with real
				// token data. Nothing to do here for normal completion.
			}
			if errClear := p.db.ClearChatRunWorker(cleanupCtx, run.ID); errClear != nil {
				logger.Error(cleanupCtx, "failed to clear run worker",
					slog.F("run_id", run.ID), slog.Error(errClear))
			}

			// Try to auto-promote the next queued message.
			err := p.db.InTx(func(tx database.Store) error {
				lockedChat, lockErr := tx.GetChatByIDForUpdate(cleanupCtx, chat.ID)
				if lockErr != nil {
					return xerrors.Errorf("lock chat for promote: %w", lockErr)
				}
				nextQueued, popErr := tx.PopNextQueuedMessage(cleanupCtx, chat.ID)
				if popErr != nil {
					// No queued messages, nothing to do.
					return nil
				}
				msg, insertErr := tx.InsertChatMessage(cleanupCtx, database.InsertChatMessageParams{
					ChatID:        chat.ID,
					ModelConfigID: uuid.NullUUID{UUID: lockedChat.LastModelConfigID, Valid: true},
					CreatedBy:     uuid.NullUUID{UUID: chat.OwnerID, Valid: chat.OwnerID != uuid.Nil},
					ChatRunID:     uuid.NullUUID{},
					ChatRunStepID: uuid.NullUUID{},
					Role:          "user",
					Content: pqtype.NullRawMessage{
						RawMessage: nextQueued.Content,
						Valid:      len(nextQueued.Content) > 0,
					},
					Visibility:          database.ChatMessageVisibilityBoth,
					Compressed:          sql.NullBool{},
					})
					if insertErr != nil {
						logger.Error(cleanupCtx, "failed to promote queued message",						slog.F("queued_message_id", nextQueued.ID), slog.Error(insertErr))
					return nil
				}
				promotedMessage = &msg
				status = codersdk.ChatStatusPending

				remaining, qErr := tx.GetChatQueuedMessages(cleanupCtx, chat.ID)
				if qErr == nil {
					remainingQueuedMessages = remaining
					shouldPublishQueueUpdate = true
				}
				return nil
			}, nil)
			if err != nil {
				logger.Error(cleanupCtx, "failed to auto-promote queued message", slog.Error(err))
			}

			// If we promoted a message, create a new run+step for it.
			if promotedMessage != nil {
				if reconcileErr := p.reconcileChatRun(cleanupCtx, chat.ID); reconcileErr != nil {
					logger.Error(cleanupCtx, "failed to reconcile chat run after promote", slog.Error(reconcileErr))
				}
			}
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

		p.publishStatus(chat.ID, status)
		// Re-read the chat from the database to pick up any title
		// changes made during processing.
		if freshChat, readErr := p.db.GetChatByID(cleanupCtx, chat.ID); readErr == nil {
			chat = freshChat
		} else {
			logger.Warn(cleanupCtx, "failed to re-read chat for status event",
				slog.F("chat_id", chat.ID), slog.Error(readErr))
		}
		p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindStatusChange)

		if !wasInterrupted {
			p.maybeSendPushNotification(cleanupCtx, chat, status, lastError, logger)
		}
	}()

	if err := p.runChat(chatCtx, chat, run, step, logger); err != nil {
		if errors.Is(err, chatloop.ErrInterrupted) || errors.Is(context.Cause(chatCtx), chatloop.ErrInterrupted) {
			logger.Info(ctx, "chat interrupted")
			status = codersdk.ChatStatusWaiting
			wasInterrupted = true
			return
		}
		if isShutdownCancellation(ctx, chatCtx, err) {
			logger.Info(ctx, "chat canceled during shutdown; returning to pending")
			status = codersdk.ChatStatusPending
			lastError = ""
			return
		}
		logger.Error(ctx, "failed to process chat", slog.Error(err))
		if reason, ok := processingFailureReason(err); ok {
			lastError = reason
			p.publishError(chat.ID, lastError)
		}
		status = codersdk.ChatStatusError
		return
	}

	// If runChat completed successfully but the server context was
	// canceled, return to pending so another replica can pick it up.
	if ctx.Err() != nil {
		logger.Info(ctx, "chat completed during shutdown; returning to pending")
		status = codersdk.ChatStatusPending
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
	run database.ChatRun,
	initialStep database.ChatRunStep,
	logger slog.Logger,
) error {
	var (
		currentStepMu sync.Mutex
		currentStep   = initialStep
	)

	var (
		model        fantasy.LanguageModel
		modelConfig  database.ChatModelConfig
		providerKeys chatprovider.ProviderAPIKeys
		callConfig   codersdk.ChatModelCallConfig
		messages     []database.ChatMessage
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
	if err := g.Wait(); err != nil {
		return err
	}
	// Fire title generation asynchronously so it doesn't block the
	// chat response. It uses a detached context so it can finish
	// even after the chat processing context is canceled.
	// Snapshot model so the goroutine doesn't race with the
	// model = cuModel reassignment below.
	titleModel := model
	p.inflight.Add(1)
	go func() {
		defer p.inflight.Done()
		p.maybeGenerateChatTitle(context.WithoutCancel(ctx), chat, messages, titleModel, providerKeys, logger)
	}()

	prompt, err := chatprompt.ConvertMessagesWithFiles(ctx, messages, p.chatFileResolver(), logger)
	if err != nil {
		return xerrors.Errorf("build chat prompt: %w", err)
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

	var instruction, resolvedUserPrompt string
	var g2 errgroup.Group
	g2.Go(func() error {
		instruction = p.resolveInstructions(ctx, chat, getWorkspaceConn)
		return nil
	})
	g2.Go(func() error {
		resolvedUserPrompt = p.resolveUserPrompt(ctx, chat.OwnerID)
		return nil
	})
	_ = g2.Wait()

	if instruction != "" {
		prompt = chatprompt.InsertSystem(prompt, instruction)
	}
	if resolvedUserPrompt != "" {
		prompt = chatprompt.InsertSystem(prompt, resolvedUserPrompt)
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

		// Snapshot the current step under the lock so the
		// transaction uses a consistent value.
		currentStepMu.Lock()
		stepSnapshot := currentStep
		currentStepMu.Unlock()

		var insertedMessages []database.ChatMessage
		err := p.db.InTx(func(tx database.Store) error {
			// Verify this worker still owns the chat's run before
			// inserting messages. This closes the race where
			// EditMessage interrupts the active step while
			// persistInterruptedStep (which uses an uncancelable
			// context) is still running.
			_, lockErr := tx.GetChatByIDForUpdate(persistCtx, chat.ID)
			if lockErr != nil {
				return xerrors.Errorf("lock chat for persist: %w", lockErr)
			}
			// Check that the active step for this chat is still
			// the one we are processing. If the step was
			// interrupted/errored or a new step was created, we
			// should not persist new messages.
			activeStep, stepErr := tx.GetActiveChatRunStep(persistCtx, chat.ID)
			if stepErr != nil || activeStep.ID != stepSnapshot.ID {
				return chatloop.ErrInterrupted
			}
			if len(assistantBlocks) > 0 {
				sdkParts := make([]codersdk.ChatMessagePart, 0, len(assistantBlocks))
				for _, block := range assistantBlocks {
					sdkParts = append(sdkParts, chatprompt.PartFromContent(block))
				}
				assistantContent, marshalErr := chatprompt.MarshalParts(sdkParts)
				if marshalErr != nil {
					return marshalErr
				}

				assistantMessage, insertErr := tx.InsertChatMessage(persistCtx, database.InsertChatMessageParams{
					ChatID:         chat.ID,
					CreatedBy:      uuid.NullUUID{},
					ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
					ChatRunID:      uuid.NullUUID{UUID: run.ID, Valid: true},
					ChatRunStepID:  uuid.NullUUID{UUID: stepSnapshot.ID, Valid: true},
					Role:           database.ChatMessageRoleAssistant,
					ContentVersion: chatprompt.CurrentContentVersion,
					Content:        assistantContent,
					Visibility:     database.ChatMessageVisibilityBoth,
					Compressed:     sql.NullBool{},
				})
				if insertErr != nil {
					return xerrors.Errorf("insert assistant message: %w", insertErr)
				}
				insertedMessages = append(insertedMessages, assistantMessage)
			}

			for _, tr := range toolResults {
				trPart := chatprompt.PartFromContent(tr)
				resultContent, marshalErr := chatprompt.MarshalParts([]codersdk.ChatMessagePart{trPart})
				if marshalErr != nil {
					return marshalErr
				}

				toolMessage, insertErr := tx.InsertChatMessage(persistCtx, database.InsertChatMessageParams{
					ChatID:         chat.ID,
					CreatedBy:      uuid.NullUUID{},
					ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
					ChatRunID:      uuid.NullUUID{UUID: run.ID, Valid: true},
					ChatRunStepID:  uuid.NullUUID{UUID: stepSnapshot.ID, Valid: true},
					Role:           database.ChatMessageRoleTool,
					ContentVersion: chatprompt.CurrentContentVersion,
					Content:        resultContent,
					Visibility:     database.ChatMessageVisibilityBoth,
					Compressed:     sql.NullBool{},
				})
				if insertErr != nil {
					return xerrors.Errorf("insert tool result: %w", insertErr)
				}
				insertedMessages = append(insertedMessages, toolMessage)
			}

			// Determine message IDs for the step.
			var firstMsgID, lastMsgID, responseMsgID sql.NullInt64
			if len(insertedMessages) > 0 {
				firstMsgID = sql.NullInt64{Int64: insertedMessages[0].ID, Valid: true}
				lastMsgID = sql.NullInt64{Int64: insertedMessages[len(insertedMessages)-1].ID, Valid: true}
			}
			// The response message is the first assistant message.
			for _, msg := range insertedMessages {
				if msg.Role == database.ChatMessageRoleAssistant {
					responseMsgID = sql.NullInt64{Int64: msg.ID, Valid: true}
					break
				}
			}

			// Count tool calls and tool results from the step
			// content blocks.
			var toolTotal, toolCompleted, toolErrored int32
			for _, block := range step.Content {
				if _, ok := fantasy.AsContentType[fantasy.ToolCallContent](block); ok {
					toolTotal++
				}
				if tr, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok {
					if _, isErr := tr.Result.(fantasy.ToolResultOutputContentError); isErr {
						toolErrored++
					} else {
						toolCompleted++
					}
				}
				if trPtr, ok := fantasy.AsContentType[*fantasy.ToolResultContent](block); ok && trPtr != nil {
					if _, isErr := trPtr.Result.(fantasy.ToolResultOutputContentError); isErr {
						toolErrored++
					} else {
						toolCompleted++
					}
				}
			}

			continuationReason := sql.NullString{}
			if step.ShouldContinue {
				continuationReason = sql.NullString{String: "tool_call", Valid: true}
			}

			_, completeErr := tx.CompleteChatRunStep(persistCtx, database.CompleteChatRunStepParams{
				ID:                  stepSnapshot.ID,
				CompletedAt:         time.Now(),
				ContinuationReason:  continuationReason,
				ResponseMessageID:   responseMsgID,
				FirstMessageID:      firstMsgID,
				LastMessageID:       lastMsgID,
				InputTokens:         sql.NullInt32{Int32: int32(step.Usage.InputTokens), Valid: step.Usage.InputTokens > 0},
				OutputTokens:        sql.NullInt32{Int32: int32(step.Usage.OutputTokens), Valid: step.Usage.OutputTokens > 0},
				TotalTokens:         sql.NullInt32{Int32: int32(step.Usage.TotalTokens), Valid: step.Usage.TotalTokens > 0},
				ReasoningTokens:     sql.NullInt32{Int32: int32(step.Usage.ReasoningTokens), Valid: step.Usage.ReasoningTokens > 0},
				CacheCreationTokens: sql.NullInt32{Int32: int32(step.Usage.CacheCreationTokens), Valid: step.Usage.CacheCreationTokens > 0},
				CacheReadTokens:     sql.NullInt32{Int32: int32(step.Usage.CacheReadTokens), Valid: step.Usage.CacheReadTokens > 0},
				ContextLimit:        sql.NullInt32{Int32: int32(step.ContextLimit.Int64), Valid: step.ContextLimit.Valid},
				TotalCostMicros:     sql.NullInt64{},
				ToolCallsTotal:      toolTotal,
				ToolCallsCompleted:  toolCompleted,
				ToolCallsErrored:    toolErrored,
			})
			if completeErr != nil {
				return xerrors.Errorf("complete chat run step: %w", completeErr)
			}

			// If the loop is continuing, create a new step for the
			// next iteration.
			if step.ShouldContinue {
				newStep, newStepErr := tx.InsertChatRunStep(persistCtx, database.InsertChatRunStepParams{
					ChatRunID:     run.ID,
					ChatID:        chat.ID,
					ModelConfigID: uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
				})
				if newStepErr != nil {
					return xerrors.Errorf("insert next chat run step: %w", newStepErr)
				}
				currentStepMu.Lock()
				currentStep = newStep
				currentStepMu.Unlock()
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
		if val, ok := p.chatStreams.Load(chat.ID); ok {
			if ss, ok := val.(*chatStreamState); ok {
				ss.mu.Lock()
				ss.buffer = nil
				ss.mu.Unlock()
			}
		}

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
		)
		if cuErr != nil {
			return xerrors.Errorf("resolve computer use model: %w", cuErr)
		}
		model = cuModel
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
				Logger:      p.logger,
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
		tools = append(tools, p.subagentTools(ctx, func() database.Chat {
			return chat
		})...)
	}

	// Build provider-native tools (e.g., web search) based on
	// the model configuration.
	var providerTools []chatloop.ProviderTool
	if callConfig.ProviderOptions != nil {
		providerTools = buildProviderTools(model.Provider(), callConfig.ProviderOptions)
	}

	if isComputerUse {
		providerTools = append(providerTools, chatloop.ProviderTool{
			Definition: chattool.ComputerUseProviderTool(
				workspacesdk.DesktopDisplayWidth,
				workspacesdk.DesktopDisplayHeight),
			Runner: chattool.NewComputerUseTool(
				workspacesdk.DesktopDisplayWidth,
				workspacesdk.DesktopDisplayHeight,
				getWorkspaceConn, quartz.NewReal(),
			),
		})
	}
	err = chatloop.Run(ctx, chatloop.RunOptions{
		Model:    model,
		Messages: prompt,
		Tools:    tools, MaxSteps: maxChatSteps,

		ModelConfig:     callConfig,
		ProviderOptions: chatprovider.ProviderOptionsFromChatModelConfig(model, callConfig.ProviderOptions),
		ProviderTools:   providerTools,

		ContextLimitFallback: modelConfigContextLimit,

		PersistStep: persistStep,
		PublishMessagePart: func(
			role codersdk.ChatMessageRole,
			part codersdk.ChatMessagePart,
		) {
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
			var reloadInstruction, reloadUserPrompt string
			var rg errgroup.Group
			rg.Go(func() error {
				reloadInstruction = p.resolveInstructions(reloadCtx, chat, getWorkspaceConn)
				return nil
			})
			rg.Go(func() error {
				reloadUserPrompt = p.resolveUserPrompt(reloadCtx, chat.OwnerID)
				return nil
			})
			_ = rg.Wait()

			if reloadInstruction != "" {
				reloadedPrompt = chatprompt.InsertSystem(reloadedPrompt, reloadInstruction)
			}
			if reloadUserPrompt != "" {
				reloadedPrompt = chatprompt.InsertSystem(reloadedPrompt, reloadUserPrompt)
			}
			return reloadedPrompt, nil
		},

		OnRetry: func(attempt int, retryErr error, delay time.Duration) {
			if val, ok := p.chatStreams.Load(chat.ID); ok {
				if rs, ok := val.(*chatStreamState); ok {
					rs.mu.Lock()
					rs.buffer = nil
					rs.mu.Unlock()
				}
			}
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
		codersdk.ChatMessageToolResult(toolCallID, "chat_summarized", summaryResult, false),
	})
	if err != nil {
		return xerrors.Errorf("encode summary tool result: %w", err)
	}

	var insertedMessages []database.ChatMessage

	txErr := p.db.InTx(func(tx database.Store) error {
		_, txErr := tx.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID:              chatID,
			CreatedBy:           uuid.NullUUID{},
			ModelConfigID:       uuid.NullUUID{UUID: modelConfigID, Valid: true},
			ChatRunID:           uuid.NullUUID{},
			ChatRunStepID:       uuid.NullUUID{},
			Role:                database.ChatMessageRoleUser,
			ContentVersion:      chatprompt.CurrentContentVersion,
			Content:             systemContent,
			Visibility:          database.ChatMessageVisibilityModel,
			Compressed:          sql.NullBool{Bool: true, Valid: true},
		})
		if txErr != nil {
			return xerrors.Errorf("insert hidden summary message: %w", txErr)
		}

		assistantMessage, txErr := tx.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID:         chatID,
			CreatedBy:      uuid.NullUUID{},
			ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: true},
			ChatRunID:      uuid.NullUUID{},
			ChatRunStepID:  uuid.NullUUID{},
			Role:           database.ChatMessageRoleAssistant,
			ContentVersion: chatprompt.CurrentContentVersion,
			Content:        assistantContent,
			Visibility:     database.ChatMessageVisibilityUser,
			Compressed: sql.NullBool{
				Bool:  true,
				Valid: true,
			},
		})
		if txErr != nil {
			return xerrors.Errorf("insert summary tool call message: %w", txErr)
		}
		insertedMessages = append(insertedMessages, assistantMessage)

		toolMessage, txErr := tx.InsertChatMessage(ctx, database.InsertChatMessageParams{
			ChatID:         chatID,
			CreatedBy:      uuid.NullUUID{},
			ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: true},
			ChatRunID:      uuid.NullUUID{},
			ChatRunStepID:  uuid.NullUUID{},
			Role:           database.ChatMessageRoleTool,
			ContentVersion: chatprompt.CurrentContentVersion,
			Content:        toolResult,
			Visibility:     database.ChatMessageVisibilityBoth,
			Compressed: sql.NullBool{
				Bool:  true,
				Valid: true,
			},
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
	var (
		dbConfig  database.ChatModelConfig
		providers []database.ChatProvider
	)

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
		providers, err = p.db.GetEnabledChatProviders(ctx)
		if err != nil {
			return xerrors.Errorf("get enabled chat providers: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, database.ChatModelConfig{}, chatprovider.ProviderAPIKeys{}, err
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
		dbConfig.Provider, dbConfig.Model, keys, chatprovider.UserAgent(),
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

func int64Ptr(value int64) *int64 {
	return &value
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

func usageNullInt64Ptr(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
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

func (p *Server) recoverStaleChatRunSteps(ctx context.Context) {
	staleAfter := time.Now().Add(-p.inFlightChatStaleAfter)
	staleSteps, err := p.db.GetStaleChatRunSteps(ctx, staleAfter)
	if err != nil {
		p.logger.Error(ctx, "failed to get stale chat run steps", slog.Error(err))
		return
	}

	recovered := 0
	for _, staleStep := range staleSteps {
		p.logger.Info(ctx, "recovering stale chat run step",
			slog.F("step_id", staleStep.ID),
			slog.F("chat_id", staleStep.ChatID),
			slog.F("run_id", staleStep.ChatRunID),
		)

		if err := p.db.InTx(func(tx database.Store) error {
			if _, errStep := tx.ErrorChatRunStep(ctx, database.ErrorChatRunStepParams{
				ID:    staleStep.ID,
				Error: "worker heartbeat expired (stale step recovery)",
			}); errStep != nil {
				if errors.Is(errStep, sql.ErrNoRows) {
					return nil // Already terminal, skip.
				}
				return errStep
			}
			return tx.ClearChatRunWorker(ctx, staleStep.ChatRunID)
		}, nil); err != nil {
			p.logger.Error(ctx, "failed to recover stale step",
				slog.F("step_id", staleStep.ID), slog.Error(err))
			continue
		}
		recovered++
	}

	if recovered > 0 {
		p.logger.Info(ctx, "recovered stale chat run steps", slog.F("count", recovered))
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
	status codersdk.ChatStatus,
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
	case codersdk.ChatStatusError:
		pushBody := "Agent encountered an error."
		if lastError != "" {
			pushBody = lastError
		}
		p.dispatchPush(ctx, chat, pushBody, status, logger)

	case codersdk.ChatStatusWaiting:
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
				Role:   database.ChatMessageRoleAssistant,
			})
			if err == nil {
				content, parseErr := chatprompt.ParseContent(msg)
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
	status codersdk.ChatStatus,
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
