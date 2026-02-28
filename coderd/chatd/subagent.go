package chatd

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/database"
)

var ErrSubagentNotDescendant = xerrors.New("target chat is not a descendant of current chat")

const (
	subagentAwaitPollInterval  = 200 * time.Millisecond
	defaultSubagentWaitTimeout = 5 * time.Minute
)

type spawnAgentArgs struct {
	Prompt string `json:"prompt"`
	Title  string `json:"title,omitempty"`
}

type waitAgentArgs struct {
	ChatID         string `json:"chat_id"`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty"`
}

type messageAgentArgs struct {
	ChatID    string `json:"chat_id"`
	Message   string `json:"message"`
	Interrupt bool   `json:"interrupt,omitempty"`
}

type closeAgentArgs struct {
	ChatID string `json:"chat_id"`
}

func (p *Server) subagentTools(currentChat func() database.Chat) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		fantasy.NewAgentTool(
			"spawn_agent",
			"Spawn a delegated child agent chat from the root chat.",
			func(ctx context.Context, args spawnAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				parent := currentChat()
				if parent.ParentChatID.Valid {
					return fantasy.NewTextErrorResponse("delegated chats cannot create child subagents"), nil
				}

				parent, err := p.db.GetChatByID(ctx, parent.ID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				childChat, err := p.createChildSubagentChat(
					ctx,
					parent,
					args.Prompt,
					args.Title,
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				return toolJSONResponse(map[string]any{
					"chat_id": childChat.ID.String(),
					"title":   childChat.Title,
					"status":  string(childChat.Status),
				}), nil
			},
		),
		fantasy.NewAgentTool(
			"wait_agent",
			"Wait until a delegated descendant agent reaches a non-streaming status.",
			func(ctx context.Context, args waitAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				targetChatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				timeout := defaultSubagentWaitTimeout
				if args.TimeoutSeconds != nil {
					timeout = time.Duration(*args.TimeoutSeconds) * time.Second
				}

				parent := currentChat()
				targetChat, report, err := p.awaitSubagentCompletion(
					ctx,
					parent.ID,
					targetChatID,
					timeout,
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				return toolJSONResponse(map[string]any{
					"chat_id": targetChatID.String(),
					"title":   targetChat.Title,
					"report":  report,
					"status":  string(targetChat.Status),
				}), nil
			},
		),
		fantasy.NewAgentTool(
			"message_agent",
			"Send a message to a delegated descendant agent. Use wait_agent to collect a response.",
			func(ctx context.Context, args messageAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				targetChatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				parent := currentChat()
				busyBehavior := SendMessageBusyBehaviorQueue
				if args.Interrupt {
					busyBehavior = SendMessageBusyBehaviorInterrupt
				}
				targetChat, err := p.sendSubagentMessage(
					ctx,
					parent.ID,
					targetChatID,
					args.Message,
					busyBehavior,
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				return toolJSONResponse(map[string]any{
					"chat_id":     targetChatID.String(),
					"title":       targetChat.Title,
					"status":      string(targetChat.Status),
					"interrupted": args.Interrupt,
				}), nil
			},
		),
		fantasy.NewAgentTool(
			"close_agent",
			"Interrupt a delegated descendant agent immediately.",
			func(ctx context.Context, args closeAgentArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				if currentChat == nil {
					return fantasy.NewTextErrorResponse("subagent callbacks are not configured"), nil
				}

				targetChatID, err := parseSubagentToolChatID(args.ChatID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				parent := currentChat()
				targetChat, err := p.closeSubagent(
					ctx,
					parent.ID,
					targetChatID,
				)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}

				return toolJSONResponse(map[string]any{
					"chat_id":    targetChatID.String(),
					"title":      targetChat.Title,
					"terminated": true,
					"status":     string(targetChat.Status),
				}), nil
			},
		),
	}
}

func parseSubagentToolChatID(raw string) (uuid.UUID, error) {
	chatID, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, xerrors.New("chat_id must be a valid UUID")
	}
	return chatID, nil
}

func (p *Server) createChildSubagentChat(
	ctx context.Context,
	parent database.Chat,
	prompt string,
	title string,
) (database.Chat, error) {
	if parent.ParentChatID.Valid {
		return database.Chat{}, xerrors.New("delegated chats cannot create child subagents")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return database.Chat{}, xerrors.New("prompt is required")
	}

	title = strings.TrimSpace(title)
	if title == "" {
		title = subagentFallbackChatTitle(prompt)
	}

	rootChatID := parent.ID
	if parent.RootChatID.Valid {
		rootChatID = parent.RootChatID.UUID
	}
	if parent.LastModelConfigID == uuid.Nil {
		return database.Chat{}, xerrors.New("parent chat model config id is required")
	}

	child, err := p.CreateChat(ctx, CreateOptions{
		OwnerID:          parent.OwnerID,
		WorkspaceID:      parent.WorkspaceID,
		WorkspaceAgentID: parent.WorkspaceAgentID,
		ParentChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  rootChatID,
			Valid: true,
		},
		ModelConfigID:      parent.LastModelConfigID,
		Title:              title,
		InitialUserContent: []fantasy.Content{fantasy.TextContent{Text: prompt}},
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("create child chat: %w", err)
	}

	return child, nil
}

func (p *Server) sendSubagentMessage(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
	message string,
	busyBehavior SendMessageBusyBehavior,
) (database.Chat, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return database.Chat{}, xerrors.New("message is required")
	}

	isDescendant, err := isSubagentDescendant(ctx, p.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, err
	}
	if !isDescendant {
		return database.Chat{}, ErrSubagentNotDescendant
	}

	sendResult, err := p.SendMessage(ctx, SendMessageOptions{
		ChatID:       targetChatID,
		Content:      []fantasy.Content{fantasy.TextContent{Text: message}},
		BusyBehavior: busyBehavior,
	})
	if err != nil {
		return database.Chat{}, err
	}

	return sendResult.Chat, nil
}

func (p *Server) awaitSubagentCompletion(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
	timeout time.Duration,
) (database.Chat, string, error) {
	isDescendant, err := isSubagentDescendant(ctx, p.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, "", err
	}
	if !isDescendant {
		return database.Chat{}, "", ErrSubagentNotDescendant
	}

	if timeout <= 0 {
		timeout = defaultSubagentWaitTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(subagentAwaitPollInterval)
	defer ticker.Stop()

	for {
		targetChat, report, done, checkErr := p.checkSubagentCompletion(ctx, targetChatID)
		if checkErr != nil {
			return database.Chat{}, "", checkErr
		}
		if done {
			if targetChat.Status == database.ChatStatusError {
				reason := strings.TrimSpace(report)
				if reason == "" {
					reason = "agent reached error status"
				}
				return database.Chat{}, "", xerrors.New(reason)
			}
			return targetChat, report, nil
		}

		select {
		case <-ticker.C:
		case <-timer.C:
			return database.Chat{}, "", xerrors.New("timed out waiting for delegated subagent completion")
		case <-ctx.Done():
			return database.Chat{}, "", ctx.Err()
		}
	}
}

func (p *Server) closeSubagent(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
) (database.Chat, error) {
	isDescendant, err := isSubagentDescendant(ctx, p.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, err
	}
	if !isDescendant {
		return database.Chat{}, ErrSubagentNotDescendant
	}

	targetChat, err := p.db.GetChatByID(ctx, targetChatID)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("get target chat: %w", err)
	}

	if targetChat.Status == database.ChatStatusWaiting {
		return targetChat, nil
	}

	updatedChat := p.InterruptChat(ctx, targetChat)
	if updatedChat.Status != database.ChatStatusWaiting {
		return database.Chat{}, xerrors.New("set target chat waiting")
	}
	return updatedChat, nil
}

func (p *Server) checkSubagentCompletion(
	ctx context.Context,
	chatID uuid.UUID,
) (database.Chat, string, bool, error) {
	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return database.Chat{}, "", false, xerrors.Errorf("get chat: %w", err)
	}

	if chat.Status == database.ChatStatusPending || chat.Status == database.ChatStatusRunning {
		return database.Chat{}, "", false, nil
	}

	report, err := latestSubagentAssistantMessage(ctx, p.db, chatID)
	if err != nil {
		return database.Chat{}, "", false, err
	}

	return chat, report, true, nil
}

func latestSubagentAssistantMessage(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
) (string, error) {
	messages, err := store.GetChatMessagesByChatID(ctx, chatID)
	if err != nil {
		return "", xerrors.Errorf("get chat messages: %w", err)
	}

	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Role != string(fantasy.MessageRoleAssistant) ||
			message.Visibility == database.ChatMessageVisibilityModel {
			continue
		}

		content, parseErr := chatprompt.ParseContent(message.Role, message.Content)
		if parseErr != nil {
			continue
		}
		text := strings.TrimSpace(contentBlocksToText(content))
		if text == "" {
			continue
		}
		return text, nil
	}

	return "", nil
}

func isSubagentDescendant(
	ctx context.Context,
	store database.Store,
	ancestorChatID uuid.UUID,
	targetChatID uuid.UUID,
) (bool, error) {
	if ancestorChatID == targetChatID {
		return false, nil
	}

	descendants, err := listSubagentDescendants(ctx, store, ancestorChatID)
	if err != nil {
		return false, err
	}
	for _, descendant := range descendants {
		if descendant.ID == targetChatID {
			return true, nil
		}
	}
	return false, nil
}

func listSubagentDescendants(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
) ([]database.Chat, error) {
	queue := []uuid.UUID{chatID}
	visited := map[uuid.UUID]struct{}{chatID: {}}

	out := make([]database.Chat, 0)
	for len(queue) > 0 {
		parentChatID := queue[0]
		queue = queue[1:]

		children, err := store.ListChildChatsByParentID(ctx, parentChatID)
		if err != nil {
			return nil, xerrors.Errorf("list child chats for %s: %w", parentChatID, err)
		}

		for _, child := range children {
			if _, ok := visited[child.ID]; ok {
				continue
			}
			visited[child.ID] = struct{}{}
			out = append(out, child)
			queue = append(queue, child.ID)
		}
	}

	return out, nil
}

func subagentFallbackChatTitle(message string) string {
	const maxWords = 6
	const maxRunes = 80

	words := strings.Fields(message)
	if len(words) == 0 {
		return "New Chat"
	}

	truncated := false
	if len(words) > maxWords {
		words = words[:maxWords]
		truncated = true
	}

	title := strings.Join(words, " ")
	if truncated {
		title += "..."
	}

	return subagentTruncateRunes(title, maxRunes)
}

func subagentTruncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}

	return string(runes[:maxRunes])
}

func toolJSONResponse(result map[string]any) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextResponse("{}")
	}
	return fantasy.NewTextResponse(string(data))
}
