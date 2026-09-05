package chatd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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

func (m *chatMutator) update(
	ctx context.Context,
	chatID uuid.UUID,
	operation string,
	transition func(*chatstate.Tx, database.Store, *database.Chat) error,
) (database.Chat, error) {
	var updated database.Chat
	err := m.server.newChatMachine(chatID).Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
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
	m.server.publishChatPubsubEvent(updated, codersdk.ChatWatchEventKindStatusChange, nil)
	return updated, nil
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
		// Hooks run before the transaction, so admission is rechecked.
		if chat.Archived {
			return SendMessageResult{}, ErrChatArchived
		}
		if _, err := resolveSendMessageModelConfigID(ctx, m.server.db, chat, opts.ModelConfigID); err != nil {
			return SendMessageResult{}, err
		}
		// Avoid dispatching hooks for a known-full queue; the transaction
		// rechecks it.
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
	refreshed, updateErr := m.update(ctx, opts.ChatID, "send", func(tx *chatstate.Tx, store database.Store, _ *database.Chat) error {
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
			// When the message is not queued, cancellation rows precede it.
			result.Message = sendResult.InsertedMessages[len(sendResult.InsertedMessages)-1]
		}
		// Queued sends may also insert history rows.
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
		// Hooks run before the transaction, so admission is rechecked.
		if chat.Archived {
			return EditMessageResult{}, ErrChatArchived
		}
		if _, err := validateEditTarget(ctx, m.server.db, opts.ChatID, opts.EditedMessageID); err != nil {
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
		result    EditMessageResult
		editedMsg database.ChatMessage
	)
	refreshed, err := m.update(ctx, opts.ChatID, "edit", func(tx *chatstate.Tx, store database.Store, _ *database.Chat) error {
		lockedChat, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if lockedChat.Archived {
			return ErrChatArchived
		}
		target, err := validateEditTarget(ctx, store, opts.ChatID, opts.EditedMessageID)
		if err != nil {
			return err
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
			// The original model may be disabled, so use the normal fallback path.
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
		// The replacement already contains the prompt response. Append only the
		// session-start response so later edits discard it with the suffix.
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

	// An interrupted worker may write stale debug rows after the edit. Use the
	// database timestamp with a skew allowance so retries do not delete rows
	// from the replacement turn.
	editCutoff := refreshed.UpdatedAt.Add(-debugCleanupClockSkew)
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

func (m *chatMutator) ArchiveChat(ctx context.Context, chat database.Chat) error {
	return m.setChatFamilyArchived(ctx, chat, chatstate.SetFamilyArchivedInput{Archived: true})
}

func (m *chatMutator) UnarchiveChat(ctx context.Context, chat database.Chat) error {
	return m.setChatFamilyArchived(ctx, chat, chatstate.SetFamilyArchivedInput{})
}

func (m *chatMutator) setChatFamilyArchived(ctx context.Context, chat database.Chat, input chatstate.SetFamilyArchivedInput) error {
	if chat.ID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}
	if chat.ParentChatID.Valid {
		return ErrArchiveRequiresRootChat
	}

	input.RootID = chat.ID
	familyChats, err := chatstate.SetFamilyArchived(ctx, m.server.db, m.server.pubsub, input)
	if err != nil {
		return err
	}
	watchKind := codersdk.ChatWatchEventKindCreated
	if input.Archived {
		m.server.scheduleArchiveDebugCleanup(ctx, familyChats)
		watchKind = codersdk.ChatWatchEventKindDeleted
	}
	m.server.publishChatPubsubEvents(familyChats, watchKind)
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
	_, updateErr := m.update(ctx, opts.ChatID, "promote", func(tx *chatstate.Tx, store database.Store, _ *database.Chat) error {
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

	_, updateErr := m.update(ctx, opts.ChatID, "tool results", func(tx *chatstate.Tx, store database.Store, _ *database.Chat) error {
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
	if updateErr != nil {
		return translateToolResultValidationError(updateErr)
	}

	return nil
}

func translateToolResultValidationError(err error) error {
	var v *chatstate.ToolResultValidationError
	if !errors.As(err, &v) {
		return err
	}
	switch {
	case xerrors.Is(v, chatstate.ErrToolResultDuplicate):
		return &ToolResultValidationError{Message: "Duplicate tool_call_id in results.", Detail: fmt.Sprintf("Duplicate tool call ID %q.", v.ToolCallID)}
	case xerrors.Is(v, chatstate.ErrToolResultMissing):
		return &ToolResultValidationError{Message: "Missing tool result.", Detail: fmt.Sprintf("Missing result for tool call %q.", v.ToolCallID)}
	case xerrors.Is(v, chatstate.ErrToolResultUnexpected):
		return &ToolResultValidationError{Message: "Unexpected tool result.", Detail: fmt.Sprintf("No pending tool call with ID %q.", v.ToolCallID)}
	case xerrors.Is(v, chatstate.ErrToolResultInvalidJSON):
		return &ToolResultValidationError{Message: "Tool result output must be valid JSON.", Detail: fmt.Sprintf("Output for tool call %q is not valid JSON.", v.ToolCallID)}
	default:
		return err
	}
}

func (m *chatMutator) InterruptChat(
	ctx context.Context,
	chat database.Chat,
) (database.Chat, error) {
	if chat.ID == uuid.Nil {
		return chat, xerrors.New("chat_id is required")
	}

	refreshed, err := m.update(ctx, chat.ID, "interrupt", func(tx *chatstate.Tx, _ database.Store, _ *database.Chat) error {
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

	refreshed, err := m.update(ctx, chat.ID, "compact", func(tx *chatstate.Tx, store database.Store, updated *database.Chat) error {
		lockedChat, err := store.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if lockedChat.Archived {
			return ErrChatArchived
		}
		// Run the transition first so busy chats report a state conflict before
		// no-op validation.
		result, err := tx.RequestCompaction(chatstate.RequestCompactionInput{})
		if err != nil {
			return err
		}
		// Validate compactable history in the transaction so no-op requests roll
		// back before starting an LLM call.
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

func (m *chatMutator) ClearChat(
	ctx context.Context,
	chat database.Chat,
) (database.Chat, error) {
	if chat.ID == uuid.Nil {
		return chat, xerrors.New("chat_id is required")
	}

	refreshed, err := m.update(ctx, chat.ID, "clear", func(tx *chatstate.Tx, store database.Store, updated *database.Chat) error {
		lockedChat, err := store.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if lockedChat.Archived {
			return ErrChatArchived
		}
		// Read history before adding the boundary, but transition first so busy
		// chats report a state conflict before no-op validation.
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
		// Validate clearable history in the transaction so no-op requests cannot
		// add a duplicate boundary.
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

func (m *chatMutator) ReconcileInvalidStateChat(
	ctx context.Context,
	chat database.Chat,
) (database.Chat, error) {
	if chat.ID == uuid.Nil {
		return chat, xerrors.New("chat_id is required")
	}

	refreshed, err := m.update(ctx, chat.ID, "reconcile", func(tx *chatstate.Tx, _ database.Store, _ *database.Chat) error {
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
