package chatd

import (
	"context"
	"encoding/json"
	"net/http"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// resolveProjectMemory returns the durable-memory store and project name for
// a chat. Only root chats inside a project have memory; chats outside a
// project, subagents, the disabled experiment, and transient lookup failures
// all report ok=false.
func (p *Server) resolveProjectMemory(ctx context.Context, chat database.Chat) (store chattool.MemoryStore, projectName string, ok bool) {
	if !p.experiments.Enabled(codersdk.ExperimentChatProjects) {
		return nil, "", false
	}
	if chat.ParentChatID.Valid || !chat.ProjectID.Valid {
		return nil, "", false
	}
	project, err := p.db.GetChatProjectByID(ctx, chat.ProjectID.UUID)
	if err != nil {
		p.logger.Debug(ctx, "failed to load chat project for memory", slog.F("chat_id", chat.ID), slog.Error(err))
		return nil, "", false
	}
	return chattool.NewProjectMemoryStore(p.db, chat.ProjectID.UUID, chat.OrganizationID, chat.OwnerID, p.memoryAuditor(chat)), project.Name, true
}

// memoryAuditor records memory changes made by a chat's tools. There is no
// HTTP request, so entries are attributed to the chat owner, whose
// permissions the change ran with, and name the chat that made it.
func (p *Server) memoryAuditor(chat database.Chat) chattool.MemoryAuditFunc {
	return func(ctx context.Context, action database.AuditAction, oldMemory, newMemory database.ChatProjectMemory) {
		if p.chatWorker == nil || p.chatWorker.opts.Auditor == nil {
			return
		}
		auditor := p.chatWorker.opts.Auditor.Load()
		if auditor == nil {
			return
		}
		// Marshaling a map of strings cannot fail.
		raw, _ := json.Marshal(map[string]string{"chat_id": chat.ID.String()})
		status := http.StatusCreated
		if action == database.AuditActionDelete {
			status = http.StatusNoContent
		}
		audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.ChatProjectMemory]{
			Audit:            *auditor,
			Log:              p.logger.With(slog.F("chat_id", chat.ID)),
			UserID:           chat.OwnerID,
			OrganizationID:   chat.OrganizationID,
			Action:           action,
			Old:              oldMemory,
			New:              newMemory,
			Status:           status,
			AdditionalFields: raw,
		})
	}
}

// memoryIndexMessage returns the model-only message that brings the chat's
// view of the project memory index up to date, or ok=false when none is
// due. The index lives in conversation history rather than in a tool
// description or the system prompt, because providers cache the request as
// one prefix of tools, system prompt, then messages: a changing index there
// would rewrite the whole cache for every chat in the project on each memory
// write. Appended messages leave the cached prefix intact.
//
// A full snapshot is sent when the prompt has none, which covers a chat's
// first turn and compaction, which drops the earlier snapshot. Changes since
// the model last saw the index go out only at turn start, so other chats'
// writes reach the model on its next turn and never mid-turn. Nothing is sent
// while the latest assistant step has tool calls without results, which must
// follow it directly.
func (p *Server) memoryIndexMessage(ctx context.Context, chat database.Chat) (chatstate.Message, bool, error) {
	store, _, ok := p.resolveProjectMemory(ctx, chat)
	if !ok {
		return chatstate.Message{}, false, nil
	}
	// Prompt rows include the model-only rows this sends, and exclude rows
	// compaction has replaced.
	promptRows, err := p.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	if err != nil {
		return chatstate.Message{}, false, xerrors.Errorf("load prompt messages: %w", err)
	}
	turnStart := currentTurnStepCount(promptRows) == 0
	if !turnStart {
		if lastActiveMessageRole(promptRows) == database.ChatMessageRoleAssistant {
			return chatstate.Message{}, false, nil
		}
		// A step can commit some tool results while others, such as
		// client-executed dynamic tools, are still pending. Providers reject a
		// message between a tool call and its result, so wait until every call
		// is resolved.
		localCalls, dynamicCalls, _, err := unresolvedToolCallsFromHistory(promptRows, dynamicToolNamesFromChat(chat))
		if err != nil {
			return chatstate.Message{}, false, xerrors.Errorf("find unresolved tool calls: %w", err)
		}
		if len(localCalls) > 0 || len(dynamicCalls) > 0 {
			return chatstate.Message{}, false, nil
		}
	}
	messages := promptRows
	snapshotOnly := !turnStart
	current, err := store.List(ctx)
	if err != nil {
		return chatstate.Message{}, false, xerrors.Errorf("list project memories: %w", err)
	}
	seen, hasSnapshot := chattool.ReplayMemoryIndex(memoryIndexTexts(messages))
	var text string
	switch {
	case !hasSnapshot && len(current) == 0:
		return chatstate.Message{}, false, nil
	case !hasSnapshot:
		text = chattool.FormatMemoryIndexSnapshot(current)
	case snapshotOnly:
		return chatstate.Message{}, false, nil
	default:
		changed, removed := chattool.DiffMemoryIndex(seen, current)
		if len(changed) == 0 && len(removed) == 0 {
			return chatstate.Message{}, false, nil
		}
		text = chattool.FormatMemoryIndexUpdate(changed, removed)
	}
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	if err != nil {
		return chatstate.Message{}, false, xerrors.Errorf("marshal memory index: %w", err)
	}
	return chatstate.Message{
		Role:           database.ChatMessageRoleUser,
		Content:        content,
		Visibility:     database.ChatMessageVisibilityModel,
		ModelConfigID:  uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: chat.LastModelConfigID != uuid.Nil},
		ContentVersion: chatprompt.CurrentContentVersion,
	}, true, nil
}

// memoryIndexAfterCompaction returns the memory index snapshot that
// memoryIndexMessage sends after a compaction drops the earlier one, so
// the post-compaction estimate can count it. ok is false when no snapshot
// would be sent or the memories cannot be listed.
func (p *Server) memoryIndexAfterCompaction(ctx context.Context, chat database.Chat) (msg fantasy.Message, ok bool) {
	store, _, ok := p.resolveProjectMemory(ctx, chat)
	if !ok {
		return fantasy.Message{}, false
	}
	current, err := store.List(ctx)
	if err != nil {
		p.logger.Debug(ctx, "list project memories for compaction estimate", slog.F("chat_id", chat.ID), slog.Error(err))
		return fantasy.Message{}, false
	}
	if len(current) == 0 {
		return fantasy.Message{}, false
	}
	return fantasy.Message{
		Role:    fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{fantasy.TextPart{Text: chattool.FormatMemoryIndexSnapshot(current)}},
	}, true
}

// memoryIndexTexts returns the text of active model-only user rows. Users
// cannot author model-only rows, so index text cannot be forged from chat
// input.
func memoryIndexTexts(messages []database.ChatMessage) []string {
	var texts []string
	for _, msg := range messages {
		if msg.Deleted || msg.Compressed || msg.Role != database.ChatMessageRoleUser || msg.Visibility != database.ChatMessageVisibilityModel {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			continue
		}
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeText {
				texts = append(texts, part.Text)
			}
		}
	}
	return texts
}

func lastActiveMessageRole(messages []database.ChatMessage) database.ChatMessageRole {
	for i := len(messages) - 1; i >= 0; i-- {
		if !messages[i].Deleted && !messages[i].Compressed {
			return messages[i].Role
		}
	}
	return ""
}
