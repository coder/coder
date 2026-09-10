package chatd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

// updateChat runs transition in one chatstate transaction, reloads the chat
// inside it so the result and the sidebar watch event carry the transition's
// snapshot bump, then publishes only that watch event (chat:update consumers
// handle stream side effects). A transition that already holds the
// post-transition row sets *updated to skip the reload.
func (p *Server) updateChat(
	ctx context.Context,
	chatID uuid.UUID,
	operation string,
	transition func(tx *chatstate.Tx, store database.Store, updated *database.Chat) error,
) (database.Chat, error) {
	var updated database.Chat
	err := p.newChatMachine(chatID).Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if err := transition(tx, store, &updated); err != nil {
			return err
		}
		if updated.ID != uuid.Nil {
			return nil
		}
		chat, err := store.GetChatByID(ctx, chatID)
		if err != nil {
			return xerrors.Errorf("reload chat after %s: %w", operation, err)
		}
		updated = chat
		return nil
	})
	if err != nil {
		return database.Chat{}, err
	}
	p.publishChatPubsubEvent(updated, codersdk.ChatWatchEventKindStatusChange, nil)
	return updated, nil
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

	contentParts, err := p.resolveChatSkillInvocations(ctx, opts.ChatID, opts.Content)
	if err != nil {
		return SendMessageResult{}, err
	}
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
		queue, err := chatstate.LoadQueueState(ctx, p.db, opts.ChatID)
		if err != nil {
			return SendMessageResult{}, xerrors.Errorf("load queue state: %w", err)
		}
		if err := chatstate.CheckSendMessageQueueCapacity(chat, queue, queuedCount, busyBehaviorToChatState(busyBehavior), p.chatLimits.MaxQueuedMessagesPerChat); err != nil {
			return SendMessageResult{}, err
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

	var (
		result               SendMessageResult
		promotedQueuedAt     time.Time
		finishedInterruption bool
	)
	refreshed, err := p.updateChat(ctx, opts.ChatID, "send", func(tx *chatstate.Tx, store database.Store, _ *database.Chat) error {
		lockedChat, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}

		if lockedChat.Archived {
			return ErrChatArchived
		}

		if opts.PlanMode != nil {
			lockedChat, err = store.UpdateChatPlanModeByID(ctx, database.UpdateChatPlanModeByIDParams{
				PlanMode: *opts.PlanMode,
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

		lockedChat, err = p.applyRequestedMCPServerIDs(ctx, store, lockedChat, opts.MCPServerIDs)
		if err != nil {
			return err
		}

		if opts.InlineMCPServers != nil {
			if err := chatstate.ReplaceInlineMCPServers(ctx, store, lockedChat.ID, *opts.InlineMCPServers); err != nil {
				return xerrors.Errorf("replace inline MCP servers: %w", err)
			}
		}

		messageCreatedBy := opts.CreatedBy
		if messageCreatedBy == uuid.Nil {
			messageCreatedBy = lockedChat.OwnerID
		}

		firstUserTurn := false
		if lockedChat.Title == chatprompt.DefaultChatTitle {
			visible, err := store.GetChatMessagesByChatIDAscPaginated(ctx, database.GetChatMessagesByChatIDAscPaginatedParams{
				ChatID:   opts.ChatID,
				LimitVal: 1,
			})
			if err != nil {
				return xerrors.Errorf("probe visible chat messages: %w", err)
			}
			firstUserTurn = len(visible) == 0
		}

		// Queue capacity is enforced inside tx.SendMessage; this
		// wrapper only propagates the typed error.
		message := userMessage(content, modelConfigID, messageCreatedBy, opts.ReasoningEffort)
		sendResult, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      message,
			BusyBehavior: busyBehaviorToChatState(busyBehavior),
			MaxQueueSize: p.chatLimits.MaxQueuedMessagesPerChat,
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
		promotedQueuedAt = sendResult.PromotedQueuedAt
		finishedInterruption = sendResult.FinishedInterruption

		// File-link errors must roll back the message.
		if err := chatstate.LinkFiles(ctx, store, opts.ChatID, chatprompt.FileIDs(contentParts), p.chatLimits.MaxAttachmentsPerChat); err != nil {
			return err
		}
		if firstUserTurn && !result.Queued {
			result.FirstUserTurn = true
			// Persist the fallback title with the message so a failed
			// async title call cannot leave the placeholder forever.
			firstMessage := []database.ChatMessage{result.Message}
			pasteText, err := titlePasteText(ctx, store, firstMessage)
			if err != nil {
				return err
			}
			if titleText, ok := titleInput(lockedChat, firstMessage, pasteText); ok {
				if _, err := store.UpdateChatTitleByID(ctx, database.UpdateChatTitleByIDParams{
					ID:          opts.ChatID,
					Title:       chatprompt.FallbackTitle(titleText),
					TitleSource: database.ChatTitleSourceFallback,
				}); err != nil {
					return xerrors.Errorf("update fallback chat title: %w", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return SendMessageResult{}, err
	}

	result.Chat = refreshed
	if result.FirstUserTurn && result.Chat.Title != chatprompt.DefaultChatTitle {
		p.publishChatPubsubEvent(result.Chat, codersdk.ChatWatchEventKindTitleChange, nil)
	}
	p.recordQueueWait(ctx, result.Chat, promotedQueuedAt)
	if finishedInterruption {
		p.afterInlineInterruption(ctx, result.Chat)
	}
	return result, nil
}

func validateEditTarget(ctx context.Context, store database.Store, chatID uuid.UUID, messageID int64) (database.ChatMessage, error) {
	target, err := store.GetChatMessageByID(ctx, messageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return database.ChatMessage{}, ErrEditedMessageNotFound
		}
		return database.ChatMessage{}, xerrors.Errorf("get edited message: %w", err)
	}
	if target.ChatID != chatID || target.Deleted {
		return database.ChatMessage{}, ErrEditedMessageNotFound
	}
	if target.Role != database.ChatMessageRoleUser {
		return database.ChatMessage{}, ErrEditedMessageNotUser
	}
	return target, nil
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

	contentParts, err := p.resolveChatSkillInvocations(ctx, opts.ChatID, opts.Content)
	if err != nil {
		return EditMessageResult{}, err
	}
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
		if _, err := validateEditTarget(ctx, p.db, opts.ChatID, opts.EditedMessageID); err != nil {
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
		result    EditMessageResult
		editedMsg database.ChatMessage
	)
	refreshed, err := p.updateChat(ctx, opts.ChatID, "edit", func(tx *chatstate.Tx, store database.Store, _ *database.Chat) error {
		lockedChat, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if lockedChat.Archived {
			return ErrChatArchived
		}
		// The target feeds the post-commit debug cleanup below; the
		// transition itself revalidates ownership and role.
		target, err := validateEditTarget(ctx, store, opts.ChatID, opts.EditedMessageID)
		if err != nil {
			return err
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
		return chatstate.LinkFiles(ctx, store, opts.ChatID, chatprompt.FileIDs(contentParts), p.chatLimits.MaxAttachmentsPerChat)
	})
	if err != nil {
		return EditMessageResult{}, err
	}
	result.Chat = refreshed

	// An interrupted worker may still flush debug rows after the edit;
	// a short bounded retry converges without waiting for dbpurge's
	// much longer debug retention window. Cut off at the DB-stamped
	// updated_at, the clock that also stamps replacement-turn rows,
	// minus debugCleanupClockSkew so replica drift cannot delete the
	// replacement turn's rows.
	editCutoff := refreshed.UpdatedAt.Add(-debugCleanupClockSkew)
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
	return p.setChatFamilyArchived(ctx, chat, true)
}

// UnarchiveChat unarchives a root chat and every child in its family
// through the chatstate state machine. Like ArchiveChat the cascade
// is atomic; ChildChat unarchive attempts are rejected with
// [ErrArchiveRequiresRootChat].
func (p *Server) UnarchiveChat(ctx context.Context, chat database.Chat) error {
	return p.setChatFamilyArchived(ctx, chat, false)
}

// setChatFamilyArchived applies SetArchived(archived) to every chat
// in chat's family through chatstate. The transaction-captured
// family rows feed the post-commit debug cleanup and sidebar watch
// events.
//
//nolint:revive // Existing API takes the target archive state as a boolean.
func (p *Server) setChatFamilyArchived(ctx context.Context, chat database.Chat, archived bool) error {
	if chat.ID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}
	if chat.ParentChatID.Valid {
		return ErrArchiveRequiresRootChat
	}

	familyChats, err := chatstate.SetFamilyArchived(ctx, p.db, p.pubsub, chatstate.SetFamilyArchivedInput{
		RootID:   chat.ID,
		Archived: archived,
	})
	if err != nil {
		return err
	}
	watchKind := codersdk.ChatWatchEventKindCreated
	if archived {
		p.scheduleArchiveDebugCleanup(ctx, familyChats)
		watchKind = codersdk.ChatWatchEventKindDeleted
	}
	p.publishChatPubsubEvents(familyChats, watchKind)
	return nil
}

// MoveChatToProject moves a root chat into a project, or out of its
// project when projectID is NULL. Its subagent chats follow because they
// use the root's project. While the chat or a subagent chat is not idle,
// it returns [chatstate.ErrTransitionNotAllowed], unless the chat is
// already in that project, which changes nothing.
func (p *Server) MoveChatToProject(ctx context.Context, chat database.Chat, projectID uuid.NullUUID) error {
	if chat.ID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}
	if chat.ParentChatID.Valid {
		return chatstate.ErrChatNotRoot
	}

	familyChats, err := chatstate.SetFamilyProject(ctx, p.db, p.pubsub, chatstate.SetFamilyProjectInput{
		RootID:    chat.ID,
		ProjectID: projectID,
	})
	if err != nil {
		return err
	}
	p.publishChatPubsubEvents(familyChats, codersdk.ChatWatchEventKindProjectChange)
	return nil
}

// DeleteQueued removes a queued user message through the chatstate
// state machine. Stream side effects are handled by chat:update
// consumers. Deleting a held head from a waiting chat can promote the
// next row; the sidebar watch event covers that status change.
func (p *Server) DeleteQueued(
	ctx context.Context,
	chatID uuid.UUID,
	queuedMessageID int64,
) error {
	if chatID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}

	var (
		refreshChat database.Chat
		promoted    bool
	)
	machine := p.newChatMachine(chatID)
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		res, err := tx.DeleteQueuedMessage(chatstate.DeleteQueuedMessageInput{
			QueuedMessageID: queuedMessageID,
		})
		if err != nil {
			return err
		}
		if res.PromotedMessage == nil {
			return nil
		}
		promoted = true
		refreshChat, err = store.GetChatByID(ctx, chatID)
		if err != nil {
			return xerrors.Errorf("reload chat after delete: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if promoted {
		p.publishChatPubsubEvent(refreshChat, codersdk.ChatWatchEventKindStatusChange, nil)
	}
	return nil
}

// EditQueuedMessageOptions controls [Server.EditQueuedMessage]. Zero
// values leave the corresponding attribute untouched.
type EditQueuedMessageOptions struct {
	ChatID          uuid.UUID
	QueuedMessageID int64
	// Content, when non-empty, replaces the queued content. It goes
	// through the same user_prompt_submit hook as a new send so the
	// stored row matches what a fresh send would have queued.
	Content []codersdk.ChatMessagePart
	// ModelConfigID, when non-zero, overrides the row's model. It is
	// only applied together with Content.
	ModelConfigID uuid.UUID
	// ReasoningEffort, when non-nil and non-empty, overrides the row's
	// reasoning effort. It is only applied together with Content.
	ReasoningEffort *string
	// Held, when non-nil, sets or clears the row's hold.
	Held *bool
}

// EditQueuedMessageResult is returned by [Server.EditQueuedMessage].
type EditQueuedMessageResult struct {
	// QueuedMessage is the row after the edit. When Promoted is true the
	// row was popped into history and no longer exists in the queue.
	QueuedMessage database.ChatQueuedMessage
	Promoted      bool
	// InsertedMessages holds every message a release-from-waiting
	// promotion inserted, in insertion order: synthetic tool
	// cancellations, then the promoted user message.
	InsertedMessages []database.ChatMessage
	Chat             database.Chat
}

// EditQueuedMessage rewrites a queued row's content and/or hold through
// the chatstate.EditQueuedMessage transition. Holding a row parks it
// and everything queued behind it until the owner saves or cancels the
// edit; see the transition for the queue semantics.
func (p *Server) EditQueuedMessage(
	ctx context.Context,
	opts EditQueuedMessageOptions,
) (EditQueuedMessageResult, error) {
	if opts.ChatID == uuid.Nil {
		return EditQueuedMessageResult{}, xerrors.New("chat_id is required")
	}
	if opts.QueuedMessageID <= 0 {
		return EditQueuedMessageResult{}, xerrors.New("queued_message_id is required")
	}
	if len(opts.Content) == 0 && opts.Held == nil {
		return EditQueuedMessageResult{}, xerrors.New("content or held is required")
	}

	contentParts := opts.Content
	if len(contentParts) > 0 && p.hooks.Enabled() {
		turnID := uuid.New()
		chat, err := p.db.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return EditQueuedMessageResult{}, xerrors.Errorf("load chat for user_prompt_submit: %w", err)
		}
		// Repeat these admission checks under the transaction lock.
		if chat.Archived {
			return EditQueuedMessageResult{}, ErrChatArchived
		}
		if _, err := validateModelConfigOverride(ctx, p.db, chat.OrganizationID, opts.ModelConfigID); err != nil {
			return EditQueuedMessageResult{}, err
		}
		promptMessage, err := chathooks.UserPromptMessage(contentParts)
		if err != nil {
			return EditQueuedMessageResult{}, err
		}
		promptResult, err := p.hooks.Trigger(ctx, chathooks.ChatFor(chat, &turnID), promptMessage, agenthooks.EventUserPromptSubmit, dispatch.CapacityClassAdmission)
		if err != nil {
			return EditQueuedMessageResult{}, p.handleUserPromptDispatchError(ctx, opts.ChatID, chathooks.UserPromptDenial(err))
		}
		contentParts, _, err = chathooks.ComposeUserPromptContent(contentParts, promptResult)
		if err != nil {
			return EditQueuedMessageResult{}, err
		}
	}

	input := chatstate.EditQueuedMessageInput{
		QueuedMessageID: opts.QueuedMessageID,
		Held:            opts.Held,
	}
	if len(contentParts) > 0 {
		content, err := chatprompt.MarshalParts(contentParts)
		if err != nil {
			return EditQueuedMessageResult{}, xerrors.Errorf("marshal message content: %w", err)
		}
		input.Content = content.RawMessage
		if opts.ReasoningEffort != nil && *opts.ReasoningEffort != "" {
			input.ReasoningEffortOverride = database.NullChatReasoningEffort{
				ChatReasoningEffort: database.ChatReasoningEffort(*opts.ReasoningEffort),
				Valid:               true,
			}
		}
	}

	var result EditQueuedMessageResult
	machine := p.newChatMachine(opts.ChatID)
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		lockedChat, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if lockedChat.Archived {
			return ErrChatArchived
		}
		if len(contentParts) > 0 {
			input.ModelConfigIDOverride, err = validateModelConfigOverride(ctx, store, lockedChat.OrganizationID, opts.ModelConfigID)
			if err != nil {
				return err
			}
		}
		editResult, err := tx.EditQueuedMessage(input)
		if err != nil {
			return err
		}
		result.QueuedMessage = editResult.QueuedMessage
		if editResult.PromotedMessage != nil {
			result.Promoted = true
			result.InsertedMessages = append(result.InsertedMessages, editResult.CancellationMessages...)
			result.InsertedMessages = append(result.InsertedMessages, *editResult.PromotedMessage)
		}
		if len(contentParts) > 0 {
			// File-link errors must roll back the edit.
			if err := chatstate.LinkFiles(ctx, store, opts.ChatID, chatprompt.FileIDs(contentParts)); err != nil {
				return err
			}
		}
		// Capture the chat inside the transaction so the watch event
		// below uses the snapshot bump and status change produced by
		// the transition itself.
		refreshed, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("reload chat after queued edit: %w", err)
		}
		result.Chat = refreshed
		return nil
	})
	if err != nil {
		return EditQueuedMessageResult{}, err
	}
	if result.Promoted {
		p.publishChatPubsubEvent(result.Chat, codersdk.ChatWatchEventKindStatusChange, nil)
	}
	return result, nil
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
		result           PromoteQueuedResult
		promotedQueuedAt time.Time
	)
	refreshed, err := p.updateChat(ctx, opts.ChatID, "promote", func(tx *chatstate.Tx, store database.Store, _ *database.Chat) error {
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
		promotedQueuedAt = promoteResult.PromotedQueuedAt
		return nil
	})
	if err != nil {
		return PromoteQueuedResult{}, err
	}

	p.recordQueueWait(ctx, refreshed, promotedQueuedAt)
	return result, nil
}

// SubmitToolResults dispatches hooks before completing the
// requires_action transition.
func (p *Server) SubmitToolResults(
	ctx context.Context,
	opts SubmitToolResultsOptions,
) error {
	var hookSuffix []chatstate.Message
	if p.hooks.Enabled() {
		state, err := loadDynamicPostToolUseState(ctx, p.newChatMachine(opts.ChatID), opts)
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

	_, err := p.updateChat(ctx, opts.ChatID, "tool results", func(tx *chatstate.Tx, store database.Store, _ *database.Chat) error {
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
				return &ToolResultStatusConflictError{ActualStatus: locked.Status}
			}
			return xerrors.Errorf("complete requires action: %w", err)
		}
		return nil
	})
	if err != nil {
		var conflict *ToolResultStatusConflictError
		if errors.As(err, &conflict) {
			return conflict
		}
		return translateToolResultValidationError(err)
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

	var result chatstate.InterruptResult
	refreshed, err := p.updateChat(ctx, chat.ID, "interrupt", func(tx *chatstate.Tx, _ database.Store, _ *database.Chat) error {
		var err error
		result, err = tx.Interrupt(chatstate.InterruptInput{
			Reason: "Tool execution interrupted by user",
		})
		return err
	})
	if err != nil {
		return chat, err
	}

	p.recordQueueWait(ctx, refreshed, result.PromotedQueuedAt)
	if result.FinishedInterruption {
		p.afterInlineInterruption(ctx, refreshed)
	}
	return refreshed, nil
}

// afterInlineInterruption applies the side effects a worker applies
// after it finishes an interruption, for an interruption that a
// transition finished inline because no worker owned the chat. The
// transition already committed, so a failure is logged.
func (p *Server) afterInlineInterruption(ctx context.Context, chat database.Chat) {
	if err := p.afterInterruptionOutcome(ctx, interruptionOutcome{
		Chat: chat,
		Kind: runnerActionKindFinishInterruption,
	}); err != nil {
		p.logger.Warn(ctx, "interruption post-outcome side effects failed",
			slog.F("chat_id", chat.ID), slog.Error(err))
	}
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

	refreshed, err := p.updateChat(ctx, chat.ID, "compact", func(tx *chatstate.Tx, store database.Store, updated *database.Chat) error {
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
		*updated = result.Chat
		return nil
	})
	if err != nil {
		return chat, err
	}
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

	refreshed, err := p.updateChat(ctx, chat.ID, "clear", func(tx *chatstate.Tx, store database.Store, updated *database.Chat) error {
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
		*updated = result.Chat
		return nil
	})
	if err != nil {
		return chat, err
	}
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

	refreshed, err := p.updateChat(ctx, chat.ID, "reconcile", func(tx *chatstate.Tx, _ database.Store, _ *database.Chat) error {
		_, err := tx.ReconcileInvalidState(chatstate.ReconcileInvalidStateInput{})
		return err
	})
	if err != nil {
		return chat, err
	}
	return refreshed, nil
}
