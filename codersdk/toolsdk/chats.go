package toolsdk

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
)

const chatIDDescription = "UUID of the chat."

func parseChatID(chatID string) (uuid.UUID, error) {
	if chatID == "" {
		return uuid.Nil, xerrors.New("chat_id is required")
	}
	id, err := uuid.Parse(chatID)
	if err != nil {
		return uuid.Nil, xerrors.New("chat_id must be a valid UUID")
	}
	return id, nil
}

type ChatToolFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
}

type ChatToolStatus struct {
	ID              string              `json:"id"`
	Title           string              `json:"title"`
	Status          codersdk.ChatStatus `json:"status"`
	Archived        bool                `json:"archived"`
	LastError       *codersdk.ChatError `json:"last_error,omitempty"`
	LastTurnSummary string              `json:"last_turn_summary,omitempty"`
	WorkspaceID     string              `json:"workspace_id,omitempty"`
	URL             string              `json:"url"`
	Files           []ChatToolFile      `json:"files,omitempty"`
}

func chatToolStatus(deps Deps, chat codersdk.Chat) ChatToolStatus {
	resp := ChatToolStatus{
		ID:        chat.ID.String(),
		Title:     chat.Title,
		Status:    chat.Status,
		Archived:  chat.Archived,
		LastError: chat.LastError,
		URL:       fmt.Sprintf("%s/agents/%s", deps.ServerURL(), chat.ID),
	}
	if chat.LastTurnSummary != nil {
		resp.LastTurnSummary = *chat.LastTurnSummary
	}
	if chat.WorkspaceID != nil {
		resp.WorkspaceID = chat.WorkspaceID.String()
	}
	for _, file := range chat.Files {
		resp.Files = append(resp.Files, ChatToolFile{
			ID:       file.ID.String(),
			Name:     file.Name,
			MimeType: file.MimeType,
		})
	}
	return resp
}

type CreateChatArgs struct {
	Prompt         string            `json:"prompt"`
	OrganizationID string            `json:"organization_id"`
	ModelConfigID  string            `json:"model_config_id"`
	Labels         map[string]string `json:"labels"`
}

var CreateChat = Tool[CreateChatArgs, ChatToolStatus]{
	Tool: aisdk.Tool{
		Name: ToolNameCreateChat,
		Description: `Start a Coder Agents chat: a server-side AI coding agent that works autonomously from a prompt.

The chat runs asynchronously. Poll coder_get_chat for status and read the transcript with coder_get_chat_messages.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Initial prompt for the agent.",
				},
				"organization_id": map[string]any{
					"type":        "string",
					"description": "Optional organization UUID. Defaults to the authenticated user's first organization.",
				},
				"model_config_id": map[string]any{
					"type":        "string",
					"description": "Optional chat model config UUID from coder_list_chat_model_configs. Defaults to the deployment default model.",
				},
				"labels": map[string]any{
					"type":                 "object",
					"description":          "Optional string key/value labels to attach to the chat.",
					"additionalProperties": map[string]any{"type": "string"},
				},
			},
			Required: []string{"prompt"},
		},
	},
	MCPAnnotations: mcpMutationAnnotations,
	Handler: func(ctx context.Context, deps Deps, args CreateChatArgs) (ChatToolStatus, error) {
		if args.Prompt == "" {
			return ChatToolStatus{}, xerrors.New("prompt is required")
		}
		var orgID uuid.UUID
		if args.OrganizationID != "" {
			var err error
			orgID, err = uuid.Parse(args.OrganizationID)
			if err != nil {
				return ChatToolStatus{}, xerrors.New("organization_id must be a valid UUID")
			}
		} else {
			me, err := deps.coderClient.User(ctx, codersdk.Me)
			if err != nil {
				return ChatToolStatus{}, err
			}
			// A user's only membership can be removed by an admin, so an
			// empty slice is reachable and indexing it would panic.
			if len(me.OrganizationIDs) == 0 {
				return ChatToolStatus{}, xerrors.New("authenticated user belongs to no organization; pass organization_id explicitly")
			}
			orgID = me.OrganizationIDs[0]
		}
		var modelConfigID *uuid.UUID
		if args.ModelConfigID != "" {
			id, err := uuid.Parse(args.ModelConfigID)
			if err != nil {
				return ChatToolStatus{}, xerrors.New("model_config_id must be a valid UUID")
			}
			modelConfigID = &id
		}
		chat, err := codersdk.NewExperimentalClient(deps.coderClient).CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: orgID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: args.Prompt,
			}},
			ModelConfigID: modelConfigID,
			Labels:        args.Labels,
		})
		if err != nil {
			return ChatToolStatus{}, xerrors.Errorf("create chat: %w", err)
		}
		return chatToolStatus(deps, chat), nil
	},
}

type GetChatArgs struct {
	ChatID string `json:"chat_id"`
}

var GetChat = Tool[GetChatArgs, ChatToolStatus]{
	Tool: aisdk.Tool{
		Name:        ToolNameGetChat,
		Description: `Get the status of a Coder Agents chat, including its last error, last turn summary, workspace, and attached files.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"chat_id": map[string]any{
					"type":        "string",
					"description": chatIDDescription,
				},
			},
			Required: []string{"chat_id"},
		},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, args GetChatArgs) (ChatToolStatus, error) {
		chatID, err := parseChatID(args.ChatID)
		if err != nil {
			return ChatToolStatus{}, err
		}
		chat, err := codersdk.NewExperimentalClient(deps.coderClient).GetChat(ctx, chatID)
		if err != nil {
			return ChatToolStatus{}, xerrors.Errorf("get chat: %w", err)
		}
		return chatToolStatus(deps, chat), nil
	},
}

type GetChatMessagesArgs struct {
	ChatID   string `json:"chat_id"`
	Limit    int    `json:"limit"`
	BeforeID int64  `json:"before_id"`
}

type ChatToolMessage struct {
	ID        int64                    `json:"id"`
	Role      codersdk.ChatMessageRole `json:"role"`
	CreatedAt time.Time                `json:"created_at"`
	Text      string                   `json:"text"`
}

type GetChatMessagesResponse struct {
	Messages []ChatToolMessage `json:"messages"`
	HasMore  bool              `json:"has_more"`
	// NextBeforeID is the cursor for the next older page when HasMore is
	// true. It is derived from the unfiltered API page, so it stays valid
	// even when every message in this page was filtered out as non-text.
	NextBeforeID int64 `json:"next_before_id,omitempty"`
}

var GetChatMessages = Tool[GetChatMessagesArgs, GetChatMessagesResponse]{
	Tool: aisdk.Tool{
		Name: ToolNameGetChatMessages,
		Description: `Get the newest messages of a Coder Agents chat in chronological order.

Only text content is returned; tool calls and other internal parts are omitted. When has_more is true, pass next_before_id as before_id to page through older messages.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"chat_id": map[string]any{
					"type":        "string",
					"description": chatIDDescription,
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of messages to fetch, from newest to oldest (1-200, default 50).",
				},
				"before_id": map[string]any{
					"type":        "integer",
					"description": "Only fetch messages with an id lower than this cursor. Omit to fetch the newest messages.",
				},
			},
			Required: []string{"chat_id"},
		},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, args GetChatMessagesArgs) (GetChatMessagesResponse, error) {
		chatID, err := parseChatID(args.ChatID)
		if err != nil {
			return GetChatMessagesResponse{}, err
		}
		if args.Limit < 0 || args.Limit > 200 {
			return GetChatMessagesResponse{}, xerrors.New("limit must be between 1 and 200")
		}
		if args.BeforeID < 0 {
			return GetChatMessagesResponse{}, xerrors.New("before_id must be a positive message id")
		}
		var opts *codersdk.ChatMessagesPaginationOptions
		if args.Limit > 0 || args.BeforeID > 0 {
			opts = &codersdk.ChatMessagesPaginationOptions{
				Limit:    args.Limit,
				BeforeID: args.BeforeID,
			}
		}
		resp, err := codersdk.NewExperimentalClient(deps.coderClient).GetChatMessages(ctx, chatID, opts)
		if err != nil {
			return GetChatMessagesResponse{}, xerrors.Errorf("get chat messages: %w", err)
		}
		// The API returns messages newest first; reverse into
		// chronological order so the transcript reads naturally.
		messages := make([]ChatToolMessage, 0, len(resp.Messages))
		for i := len(resp.Messages) - 1; i >= 0; i-- {
			msg := resp.Messages[i]
			var texts []string
			for _, part := range msg.Content {
				if part.Type == codersdk.ChatMessagePartTypeText && part.Text != "" {
					texts = append(texts, part.Text)
				}
			}
			if len(texts) == 0 {
				continue
			}
			messages = append(messages, ChatToolMessage{
				ID:        msg.ID,
				Role:      msg.Role,
				CreatedAt: msg.CreatedAt,
				Text:      strings.Join(texts, "\n"),
			})
		}
		var nextBeforeID int64
		if resp.HasMore && len(resp.Messages) > 0 {
			nextBeforeID = resp.Messages[0].ID
			for _, msg := range resp.Messages {
				if msg.ID < nextBeforeID {
					nextBeforeID = msg.ID
				}
			}
		}
		return GetChatMessagesResponse{
			Messages:     messages,
			HasMore:      resp.HasMore,
			NextBeforeID: nextBeforeID,
		}, nil
	},
}

type SendChatMessageArgs struct {
	ChatID       string                    `json:"chat_id"`
	Text         string                    `json:"text"`
	BusyBehavior codersdk.ChatBusyBehavior `json:"busy_behavior"`
}

type SendChatMessageResponse struct {
	Queued   bool     `json:"queued"`
	Warnings []string `json:"warnings,omitempty"`
}

var SendChatMessage = Tool[SendChatMessageArgs, SendChatMessageResponse]{
	Tool: aisdk.Tool{
		Name:        ToolNameSendChatMessage,
		Description: `Send a message to a Coder Agents chat.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"chat_id": map[string]any{
					"type":        "string",
					"description": chatIDDescription,
				},
				"text": map[string]any{
					"type":        "string",
					"description": "The message to send.",
				},
				"busy_behavior": map[string]any{
					"type":        "string",
					"description": "What to do when the chat is already processing: \"queue\" (default) processes the message after the current run, \"interrupt\" stops the current run first.",
					"enum": []string{
						string(codersdk.ChatBusyBehaviorQueue),
						string(codersdk.ChatBusyBehaviorInterrupt),
					},
				},
			},
			Required: []string{"chat_id", "text"},
		},
	},
	MCPAnnotations: mcpMutationAnnotations,
	Handler: func(ctx context.Context, deps Deps, args SendChatMessageArgs) (SendChatMessageResponse, error) {
		chatID, err := parseChatID(args.ChatID)
		if err != nil {
			return SendChatMessageResponse{}, err
		}
		if args.Text == "" {
			return SendChatMessageResponse{}, xerrors.New("text is required")
		}
		busyBehavior := args.BusyBehavior
		switch busyBehavior {
		case "":
			busyBehavior = codersdk.ChatBusyBehaviorQueue
		case codersdk.ChatBusyBehaviorQueue, codersdk.ChatBusyBehaviorInterrupt:
		default:
			return SendChatMessageResponse{}, xerrors.New(`busy_behavior must be "queue" or "interrupt"`)
		}
		resp, err := codersdk.NewExperimentalClient(deps.coderClient).CreateChatMessage(ctx, chatID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: args.Text,
			}},
			BusyBehavior: busyBehavior,
		})
		if err != nil {
			return SendChatMessageResponse{}, xerrors.Errorf("send chat message: %w", err)
		}
		return SendChatMessageResponse{
			Queued:   resp.Queued,
			Warnings: resp.Warnings,
		}, nil
	},
}

type InterruptChatArgs struct {
	ChatID string `json:"chat_id"`
}

var InterruptChat = Tool[InterruptChatArgs, ChatToolStatus]{
	Tool: aisdk.Tool{
		Name:        ToolNameInterruptChat,
		Description: `Interrupt a running Coder Agents chat. Progress so far is preserved.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"chat_id": map[string]any{
					"type":        "string",
					"description": chatIDDescription,
				},
			},
			Required: []string{"chat_id"},
		},
	},
	MCPAnnotations: mcpMutationAnnotations,
	Handler: func(ctx context.Context, deps Deps, args InterruptChatArgs) (ChatToolStatus, error) {
		chatID, err := parseChatID(args.ChatID)
		if err != nil {
			return ChatToolStatus{}, err
		}
		chat, err := codersdk.NewExperimentalClient(deps.coderClient).InterruptChat(ctx, chatID)
		if err != nil {
			return ChatToolStatus{}, xerrors.Errorf("interrupt chat: %w", err)
		}
		return chatToolStatus(deps, chat), nil
	},
}

type ArchiveChatArgs struct {
	ChatID string `json:"chat_id"`
}

var ArchiveChat = Tool[ArchiveChatArgs, codersdk.Response]{
	Tool: aisdk.Tool{
		Name:        ToolNameArchiveChat,
		Description: `Archive a Coder Agents chat. The chat is hidden from default listings but can be unarchived from the UI.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"chat_id": map[string]any{
					"type":        "string",
					"description": chatIDDescription,
				},
			},
			Required: []string{"chat_id"},
		},
	},
	MCPAnnotations: mcpMutationAnnotations,
	Handler: func(ctx context.Context, deps Deps, args ArchiveChatArgs) (codersdk.Response, error) {
		chatID, err := parseChatID(args.ChatID)
		if err != nil {
			return codersdk.Response{}, err
		}
		archived := true
		err = codersdk.NewExperimentalClient(deps.coderClient).UpdateChat(ctx, chatID, codersdk.UpdateChatRequest{
			Archived: &archived,
		})
		if err != nil {
			return codersdk.Response{}, xerrors.Errorf("archive chat: %w", err)
		}
		return codersdk.Response{
			Message: "Chat archived successfully.",
		}, nil
	},
}

type ChatModelConfigSummary struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	DisplayName string `json:"display_name"`
	IsDefault   bool   `json:"is_default"`
}

type ListChatModelConfigsResponse struct {
	ModelConfigs []ChatModelConfigSummary `json:"model_configs"`
}

var ListChatModelConfigs = Tool[NoArgs, ListChatModelConfigsResponse]{
	Tool: aisdk.Tool{
		Name: ToolNameListChatModelConfigs,
		Description: `List the enabled chat models available for Coder Agents chats. Use a model config ID with coder_create_chat to pick a model.

Per-user provider credentials are validated when creating a chat, so coder_create_chat can still reject a listed model with an explanatory error.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{},
			Required:   []string{},
		},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, _ NoArgs) (ListChatModelConfigsResponse, error) {
		configs, err := codersdk.NewExperimentalClient(deps.coderClient).ListChatModelConfigs(ctx)
		if err != nil {
			return ListChatModelConfigsResponse{}, xerrors.Errorf("list chat model configs: %w", err)
		}
		// Admin model lists include configs backed by disabled providers. Filter
		// them when provider state is available; non-admin lists are already filtered.
		providerEnabled := map[uuid.UUID]bool{}
		if providers, err := deps.coderClient.AIProviders(ctx); err == nil {
			for _, provider := range providers {
				providerEnabled[provider.ID] = provider.Enabled
			}
		}
		summaries := make([]ChatModelConfigSummary, 0, len(configs))
		for _, config := range configs {
			if !config.Enabled {
				continue
			}
			if enabled, ok := providerEnabled[config.AIProviderID]; ok && !enabled {
				continue
			}
			summaries = append(summaries, ChatModelConfigSummary{
				ID:          config.ID.String(),
				Model:       config.Model,
				DisplayName: config.DisplayName,
				IsDefault:   config.IsDefault,
			})
		}
		return ListChatModelConfigsResponse{ModelConfigs: summaries}, nil
	},
}
