package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

type chatMutator struct {
	server *Server
}

type chatMutation struct {
	chat database.Chat
}

func (m *chatMutator) update(
	ctx context.Context,
	chatID uuid.UUID,
	operation string,
	transition func(*chatstate.Tx, database.Store, *chatMutation) error,
) (database.Chat, error) {
	mutation := chatMutation{}
	err := m.server.newChatMachine(chatID).Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if err := transition(tx, store, &mutation); err != nil {
			return err
		}
		if mutation.chat.ID != uuid.Nil {
			return nil
		}
		chat, err := store.GetChatByID(ctx, chatID)
		if err != nil {
			return xerrors.Errorf("reload chat after %s: %w", operation, err)
		}
		mutation.chat = chat
		return nil
	})
	if err != nil {
		return database.Chat{}, err
	}
	m.server.publishChatPubsubEvent(mutation.chat, codersdk.ChatWatchEventKindStatusChange, nil)
	return mutation.chat, nil
}

func (m *chatMutator) SendMessage(
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
	if m.server.hooks.Enabled() {
		turnID := uuid.New()
		chat, err := m.server.db.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return SendMessageResult{}, xerrors.Errorf("load chat for user_prompt_submit: %w", err)
		}
		// Repeat these admission checks under the transaction lock.
		if chat.Archived {
			return SendMessageResult{}, ErrChatArchived
		}
		if _, err := resolveSendMessageModelConfigID(ctx, m.server.db, chat, opts.ModelConfigID); err != nil {
			return SendMessageResult{}, err
		}
		// Check queue capacity before dispatch; the transaction
		// rechecks it under lock.
		queuedCount, err := m.server.db.CountChatQueuedMessages(ctx, opts.ChatID)
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
		promptResult, err := m.server.hooks.Trigger(ctx, chathooks.ChatFor(chat, &turnID), promptMessage, agenthooks.EventUserPromptSubmit, dispatch.CapacityClassAdmission)
		if err != nil {
			return SendMessageResult{}, m.server.handleUserPromptDispatchError(ctx, opts.ChatID, chathooks.UserPromptDenial(err))
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
	refreshed, updateErr := m.update(ctx, opts.ChatID, "send", func(tx *chatstate.Tx, store database.Store, _ *chatMutation) error {
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

		lockedChat, err = m.server.applyRequestedMCPServerIDs(ctx, store, lockedChat, requestedMCPServerIDs)
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
		return chatstate.LinkFiles(ctx, store, opts.ChatID, chatprompt.FileIDs(contentParts))
	})
	if updateErr != nil {
		return SendMessageResult{}, updateErr
	}

	result.Chat = refreshed
	return result, nil
}

func (m *chatMutator) EditMessage(
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
	if m.server.hooks.Enabled() {
		turnID := uuid.New()
		chat, err := m.server.db.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return EditMessageResult{}, xerrors.Errorf("load chat for edit hooks: %w", err)
		}
		// Repeat these admission checks under the transaction lock.
		if chat.Archived {
			return EditMessageResult{}, ErrChatArchived
		}
		if err := validateEditTarget(ctx, m.server.db, opts.ChatID, opts.EditedMessageID); err != nil {
			return EditMessageResult{}, err
		}
		if _, err := validateModelConfigOverride(ctx, m.server.db, chat.OrganizationID, opts.ModelConfigID); err != nil {
			return EditMessageResult{}, err
		}
		sessionStartHookResult, err = m.server.hooks.Trigger(ctx, chathooks.ChatFor(chat, &turnID), chathooks.Message{Source: chathooks.SessionStartSourceClear}, agenthooks.EventSessionStart, dispatch.CapacityClassAdmission)
		if err != nil {
			return EditMessageResult{}, m.server.handleAPIDispatchError(ctx, opts.ChatID, agenthooks.EventSessionStart, err)
		}
		promptMessage, err := chathooks.UserPromptMessage(contentParts)
		if err != nil {
			return EditMessageResult{}, err
		}
		promptResult, err := m.server.hooks.Trigger(ctx, chathooks.ChatFor(chat, &turnID), promptMessage, agenthooks.EventUserPromptSubmit, dispatch.CapacityClassAdmission)
		if err != nil {
			return EditMessageResult{}, m.server.handleUserPromptDispatchError(ctx, opts.ChatID, chathooks.UserPromptDenial(err))
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
	refreshed, err := m.update(ctx, opts.ChatID, "edit", func(tx *chatstate.Tx, store database.Store, _ *chatMutation) error {
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

		lockedChat, err = m.server.applyRequestedMCPServerIDs(ctx, store, lockedChat, opts.MCPServerIDs)
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
		return chatstate.LinkFiles(ctx, store, opts.ChatID, chatprompt.FileIDs(contentParts))
	})
	if err != nil {
		return EditMessageResult{}, err
	}

	result.Chat = refreshed
	editedCutoffT = refreshed.UpdatedAt

	// Editing can race with an interrupted worker still flushing its
	// final debug writes. Run a short bounded retry loop so we converge
	// quickly without relying on the much longer stale-finalization
	// sweep. Source editCutoff from the DB-stamped updated_at returned
	// by the post-edit chat row so the filter uses the same clock that
	// stamps replacement-turn debug rows; subtract
	// debugCleanupClockSkew so replica clock drift cannot let the retry
	// delete a replacement turn's debug rows.
	editCutoff := editedCutoffT.Add(-debugCleanupClockSkew)
	m.server.scheduleDebugCleanup(
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

type archiveMutation struct {
	archived  bool
	watchKind codersdk.ChatWatchEventKind
}

func (m *chatMutator) ArchiveChat(ctx context.Context, chat database.Chat) error {
	return m.setChatFamilyArchived(ctx, chat, archiveMutation{
		archived:  true,
		watchKind: codersdk.ChatWatchEventKindDeleted,
	})
}

func (m *chatMutator) UnarchiveChat(ctx context.Context, chat database.Chat) error {
	return m.setChatFamilyArchived(ctx, chat, archiveMutation{
		watchKind: codersdk.ChatWatchEventKindCreated,
	})
}

func (m *chatMutator) setChatFamilyArchived(ctx context.Context, chat database.Chat, mutation archiveMutation) error {
	if chat.ID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}
	if chat.ParentChatID.Valid {
		return ErrArchiveRequiresRootChat
	}

	familyChats, err := chatstate.SetFamilyArchived(ctx, m.server.db, m.server.pubsub, chatstate.SetFamilyArchivedInput{
		RootID:   chat.ID,
		Archived: mutation.archived,
	})
	if err != nil {
		return err
	}
	if mutation.archived {
		m.server.scheduleArchiveDebugCleanup(ctx, familyChats)
	}
	m.server.publishChatPubsubEvents(familyChats, mutation.watchKind)
	return nil
}

func (m *chatMutator) DeleteQueued(ctx context.Context, chatID uuid.UUID, queuedMessageID int64) error {
	if chatID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}

	machine := m.server.newChatMachine(chatID)
	return machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.DeleteQueuedMessage(chatstate.DeleteQueuedMessageInput{
			QueuedMessageID: queuedMessageID,
		})
		return err
	})
}

func (m *chatMutator) PromoteQueued(
	ctx context.Context,
	opts PromoteQueuedOptions,
) (PromoteQueuedResult, error) {
	if opts.ChatID == uuid.Nil {
		return PromoteQueuedResult{}, xerrors.New("chat_id is required")
	}

	var result PromoteQueuedResult
	_, updateErr := m.update(ctx, opts.ChatID, "promote", func(tx *chatstate.Tx, store database.Store, _ *chatMutation) error {
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
		return nil
	})
	if updateErr != nil {
		return PromoteQueuedResult{}, updateErr
	}

	return result, nil
}

func (m *chatMutator) SubmitToolResults(
	ctx context.Context,
	opts SubmitToolResultsOptions,
) error {
	machine := m.server.newChatMachine(opts.ChatID)
	var hookSuffix []chatstate.Message
	if m.server.hooks.Enabled() {
		state, err := loadDynamicPostToolUseState(ctx, machine, opts)
		if err != nil {
			return err
		}
		for _, result := range opts.Results {
			response, err := m.server.hooks.Trigger(ctx, chathooks.ChatFor(state.chat, nil), chathooks.DynamicPostToolUseMessage(result, state.toolNames[result.ToolCallID]), agenthooks.EventPostToolUse, dispatch.CapacityClassGeneration)
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

	var statusConflict *ToolResultStatusConflictError
	_, updateErr := m.update(ctx, opts.ChatID, "tool results", func(tx *chatstate.Tx, store database.Store, _ *chatMutation) error {
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
		return nil
	})
	if updateErr != nil {
		if statusConflict != nil {
			return statusConflict
		}
		return translateToolResultValidationError(updateErr)
	}

	return nil
}

func (m *chatMutator) InterruptChat(
	ctx context.Context,
	chat database.Chat,
) (database.Chat, error) {
	if chat.ID == uuid.Nil {
		return chat, xerrors.New("chat_id is required")
	}

	refreshed, err := m.update(ctx, chat.ID, "interrupt", func(tx *chatstate.Tx, _ database.Store, _ *chatMutation) error {
		if _, err := tx.Interrupt(chatstate.InterruptInput{
			Reason: "Tool execution interrupted by user",
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return chat, err
	}

	return refreshed, nil
}

func (m *chatMutator) CompactChat(
	ctx context.Context,
	chat database.Chat,
) (database.Chat, error) {
	if chat.ID == uuid.Nil {
		return chat, xerrors.New("chat_id is required")
	}

	refreshed, err := m.update(ctx, chat.ID, "compact", func(tx *chatstate.Tx, store database.Store, mutation *chatMutation) error {
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
		mutation.chat = result.Chat
		return nil
	})
	if err != nil {
		return chat, err
	}

	return refreshed, nil
}

func (m *chatMutator) ClearChat(
	ctx context.Context,
	chat database.Chat,
) (database.Chat, error) {
	if chat.ID == uuid.Nil {
		return chat, xerrors.New("chat_id is required")
	}

	refreshed, err := m.update(ctx, chat.ID, "clear", func(tx *chatstate.Tx, store database.Store, mutation *chatMutation) error {
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
		mutation.chat = result.Chat
		return nil
	})
	if err != nil {
		return chat, err
	}

	return refreshed, nil
}

func (m *chatMutator) ReconcileInvalidStateChat(
	ctx context.Context,
	chat database.Chat,
) (database.Chat, error) {
	if chat.ID == uuid.Nil {
		return chat, xerrors.New("chat_id is required")
	}

	refreshed, err := m.update(ctx, chat.ID, "reconcile", func(tx *chatstate.Tx, _ database.Store, _ *chatMutation) error {
		if _, err := tx.ReconcileInvalidState(chatstate.ReconcileInvalidStateInput{}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return chat, err
	}

	return refreshed, nil
}

func (p *Server) publishChatPubsubEvents(chats []database.Chat, kind codersdk.ChatWatchEventKind) {
	for _, chat := range chats {
		p.publishChatPubsubEvent(chat, kind, nil)
	}
}

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
