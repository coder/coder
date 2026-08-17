package toolsdk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
)

const chatIDDescription = "UUID of the chat."

func isForbiddenError(err error) bool {
	var sdkErr *codersdk.Error
	return errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusForbidden
}

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
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
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
	Labels          map[string]string   `json:"labels,omitempty"`
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
		Labels:    chat.Labels,
	}
	if chat.LastTurnSummary != nil {
		resp.LastTurnSummary = *chat.LastTurnSummary
	}
	if chat.WorkspaceID != nil {
		resp.WorkspaceID = chat.WorkspaceID.String()
	}
	for _, file := range chat.Files {
		resp.Files = append(resp.Files, ChatToolFile{
			ID:        file.ID.String(),
			Name:      file.Name,
			MimeType:  file.MimeType,
			SizeBytes: file.SizeBytes,
			CreatedAt: file.CreatedAt,
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
			// Admins can remove a user's only organization membership.
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

type DownloadChatFileArgs struct {
	FileID   string `json:"file_id"`
	ChatID   string `json:"chat_id"`
	FileName string `json:"file_name"`
}

type DownloadChatFileResponse struct {
	FileID    string    `json:"file_id"`
	Name      string    `json:"name"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func chatFilesDescription(files []codersdk.ChatFileMetadata) string {
	descriptions := make([]string, len(files))
	for i, file := range files {
		descriptions[i] = fmt.Sprintf("{id: %s, name: %q, mime_type: %q, size_bytes: %d}", file.ID, file.Name, file.MimeType, file.SizeBytes)
	}
	return "[" + strings.Join(descriptions, ", ") + "]"
}

var DownloadChatFile = Tool[DownloadChatFileArgs, DownloadChatFileResponse]{
	Tool: aisdk.Tool{
		Name: ToolNameDownloadChatFile,
		Description: `Create a short-lived download URL for a file attached to a Coder Agents chat.

Address the file with file_id alone, or with chat_id and an exact file_name. The URL expires in about 5 minutes and needs no authentication header. Fetch it with curl -fSs -o <path> "<url>". Do not read binary contents into context.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"file_id": map[string]any{
					"type":        "string",
					"description": "Optional chat file UUID. Use this alone when the file ID is known.",
				},
				"chat_id": map[string]any{
					"type":        "string",
					"description": "Optional chat UUID. Use together with file_name when the file ID is unknown.",
				},
				"file_name": map[string]any{
					"type":        "string",
					"description": "Optional exact file name. Use together with chat_id.",
				},
			},
			Required: []string{},
		},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, args DownloadChatFileArgs) (DownloadChatFileResponse, error) {
		fileIDMode := args.FileID != "" && args.ChatID == "" && args.FileName == ""
		chatFileMode := args.FileID == "" && args.ChatID != "" && args.FileName != ""
		if !fileIDMode && !chatFileMode {
			return DownloadChatFileResponse{}, xerrors.New("provide exactly one addressing mode: file_id alone, or chat_id with file_name")
		}

		var fileID uuid.UUID
		if fileIDMode {
			var err error
			fileID, err = uuid.Parse(args.FileID)
			if err != nil {
				return DownloadChatFileResponse{}, xerrors.New("file_id must be a valid UUID")
			}
		} else {
			chatID, err := parseChatID(args.ChatID)
			if err != nil {
				return DownloadChatFileResponse{}, err
			}
			chat, err := codersdk.NewExperimentalClient(deps.coderClient).GetChat(ctx, chatID)
			if err != nil {
				return DownloadChatFileResponse{}, xerrors.Errorf("get chat: %w", err)
			}
			found := false
			for _, file := range chat.Files {
				if file.Name != args.FileName {
					continue
				}
				if found {
					return DownloadChatFileResponse{}, xerrors.Errorf("multiple chat files named %q; available files: %s", args.FileName, chatFilesDescription(chat.Files))
				}
				fileID = file.ID
				found = true
			}
			if !found {
				return DownloadChatFileResponse{}, xerrors.Errorf("no chat file named %q; available files: %s", args.FileName, chatFilesDescription(chat.Files))
			}
		}

		download, err := codersdk.NewExperimentalClient(deps.coderClient).ChatFileDownloadURL(ctx, fileID)
		if err != nil {
			return DownloadChatFileResponse{}, xerrors.Errorf("create chat file download URL: %w", err)
		}
		return DownloadChatFileResponse{
			FileID:    fileID.String(),
			Name:      download.Name,
			MimeType:  download.MimeType,
			SizeBytes: download.SizeBytes,
			SHA256:    download.SHA256,
			URL:       download.URL,
			ExpiresAt: download.ExpiresAt,
		}, nil
	},
}

type AwaitChatArgs struct {
	ChatID   string `json:"chat_id"`
	WaitSecs int    `json:"wait_secs"`
}

type AwaitChatResponse struct {
	TimedOut bool           `json:"timed_out"`
	Chat     ChatToolStatus `json:"chat"`
}

func chatStatusBusy(status codersdk.ChatStatus) bool {
	return status == codersdk.ChatStatusRunning || status == codersdk.ChatStatusInterrupting
}

var AwaitChat = Tool[AwaitChatArgs, AwaitChatResponse]{
	Tool: aisdk.Tool{
		Name:        ToolNameAwaitChat,
		Description: `Block until a Coder Agents chat stops generating or the wait times out. Waiting, error, and requires_action all end the wait. If timed_out is true, call this tool again to continue waiting.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"chat_id": map[string]any{
					"type":        "string",
					"description": chatIDDescription,
				},
				"wait_secs": map[string]any{
					"type":        "integer",
					"description": "Maximum seconds to wait (1-120, default 60).",
				},
			},
			Required: []string{"chat_id"},
		},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, args AwaitChatArgs) (AwaitChatResponse, error) {
		chatID, err := parseChatID(args.ChatID)
		if err != nil {
			return AwaitChatResponse{}, err
		}
		waitSecs := args.WaitSecs
		if waitSecs == 0 {
			waitSecs = 60
		}
		waitSecs = min(max(waitSecs, 1), 120)

		expClient := codersdk.NewExperimentalClient(deps.coderClient)
		events, closer, err := expClient.WatchChats(ctx)
		if err != nil {
			return AwaitChatResponse{}, xerrors.Errorf("watch chats: %w", err)
		}
		defer func() {
			_ = closer.Close()
		}()

		chat, err := expClient.GetChat(ctx, chatID)
		if err != nil {
			return AwaitChatResponse{}, xerrors.Errorf("get chat: %w", err)
		}
		if !chatStatusBusy(chat.Status) {
			return AwaitChatResponse{Chat: chatToolStatus(deps, chat)}, nil
		}

		timer := time.NewTimer(time.Duration(waitSecs) * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return AwaitChatResponse{}, ctx.Err()
			case <-timer.C:
				chat, err := expClient.GetChat(ctx, chatID)
				if err != nil {
					return AwaitChatResponse{}, xerrors.Errorf("get chat after timeout: %w", err)
				}
				return AwaitChatResponse{TimedOut: chatStatusBusy(chat.Status), Chat: chatToolStatus(deps, chat)}, nil
			case event, ok := <-events:
				if !ok {
					chat, err := expClient.GetChat(ctx, chatID)
					if err != nil {
						return AwaitChatResponse{}, xerrors.Errorf("get chat after watch closed: %w", err)
					}
					return AwaitChatResponse{TimedOut: chatStatusBusy(chat.Status), Chat: chatToolStatus(deps, chat)}, nil
				}
				if event.Chat.ID == chatID && !chatStatusBusy(event.Chat.Status) {
					chat, err := expClient.GetChat(ctx, chatID)
					if err != nil {
						return AwaitChatResponse{}, xerrors.Errorf("get chat after status change: %w", err)
					}
					return AwaitChatResponse{Chat: chatToolStatus(deps, chat)}, nil
				}
			}
		}
	},
}

type ListChatsArgs struct {
	Labels map[string]string `json:"labels"`
	Query  string            `json:"query"`
	Limit  int               `json:"limit"`
}

type ListChatsResponse struct {
	Chats []ChatToolStatus `json:"chats"`
}

var ListChats = Tool[ListChatsArgs, ListChatsResponse]{
	Tool: aisdk.Tool{
		Name:        ToolNameListChats,
		Description: `List Coder Agents chats, optionally filtered by labels or a search query.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"labels": map[string]any{
					"type":                 "object",
					"description":          "Optional exact-match string key/value labels.",
					"additionalProperties": map[string]any{"type": "string"},
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Optional chat search query.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum chats to return (1-100, default 25).",
				},
			},
			Required: []string{},
		},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, args ListChatsArgs) (ListChatsResponse, error) {
		limit := args.Limit
		if limit == 0 {
			limit = 25
		}
		limit = min(max(limit, 1), 100)
		chats, err := codersdk.NewExperimentalClient(deps.coderClient).ListChats(ctx, &codersdk.ListChatsOptions{
			Query:  args.Query,
			Labels: args.Labels,
			Pagination: codersdk.Pagination{
				Limit: limit,
			},
		})
		if err != nil {
			return ListChatsResponse{}, xerrors.Errorf("list chats: %w", err)
		}
		resp := ListChatsResponse{Chats: make([]ChatToolStatus, len(chats))}
		for i, chat := range chats {
			resp.Chats[i] = chatToolStatus(deps, chat)
		}
		return resp, nil
	},
}

type GetChatMessagesArgs struct {
	ChatID   string `json:"chat_id"`
	Limit    int    `json:"limit"`
	BeforeID int64  `json:"before_id"`
	AfterID  int64  `json:"after_id"`
}

type ChatToolMessageFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
}

type ChatToolMessage struct {
	ID        int64                    `json:"id"`
	Role      codersdk.ChatMessageRole `json:"role"`
	CreatedAt time.Time                `json:"created_at"`
	Text      string                   `json:"text"`
	Files     []ChatToolMessageFile    `json:"files,omitempty"`
}

type GetChatMessagesResponse struct {
	Messages []ChatToolMessage `json:"messages"`
	HasMore  bool              `json:"has_more"`
	// Cursors come from the raw page so filtered pages remain traversable.
	NextBeforeID int64 `json:"next_before_id,omitempty"`
	NextAfterID  int64 `json:"next_after_id,omitempty"`
	// QueuedMessages is populated only on the initial page.
	QueuedMessages []string `json:"queued_messages,omitempty"`
}

// Hook notices are user-facing per the SDK contract; hook context is model-only.
func userFacingText(parts []codersdk.ChatMessagePart) string {
	var texts []string
	for _, part := range parts {
		isUserFacingText := part.Type == codersdk.ChatMessagePartTypeText ||
			part.Type == codersdk.ChatMessagePartTypeHookNotice
		if isUserFacingText && part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func chatToolMessage(msg codersdk.ChatMessage) (ChatToolMessage, bool) {
	text := userFacingText(msg.Content)
	if text == "" {
		return ChatToolMessage{}, false
	}
	toolMessage := ChatToolMessage{
		ID:        msg.ID,
		Role:      msg.Role,
		CreatedAt: msg.CreatedAt,
		Text:      text,
	}
	for _, part := range msg.Content {
		if part.Type == codersdk.ChatMessagePartTypeFile && part.FileID.Valid {
			toolMessage.Files = append(toolMessage.Files, ChatToolMessageFile{
				ID:       part.FileID.UUID.String(),
				Name:     part.Name,
				MimeType: part.MediaType,
			})
		}
	}
	return toolMessage, true
}

var GetChatMessages = Tool[GetChatMessagesArgs, GetChatMessagesResponse]{
	Tool: aisdk.Tool{
		Name: ToolNameGetChatMessages,
		Description: `Get messages from a Coder Agents chat in chronological order.

Only user-facing text content is returned (including lifecycle hook notices); tool calls and other internal parts are omitted. Prompts still queued behind a busy chat appear in queued_messages. Use before_id with next_before_id to page backward from the newest messages, or after_id with next_after_id to page forward.`,
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
				"after_id": map[string]any{
					"type":        "integer",
					"description": "Only fetch messages with an id greater than this cursor, in chronological order. Cannot be combined with before_id.",
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
		if args.AfterID < 0 {
			return GetChatMessagesResponse{}, xerrors.New("after_id must be a positive message id")
		}
		if args.BeforeID > 0 && args.AfterID > 0 {
			return GetChatMessagesResponse{}, xerrors.New("before_id and after_id cannot be used together")
		}
		var opts *codersdk.ChatMessagesPaginationOptions
		if args.Limit > 0 || args.BeforeID > 0 || args.AfterID > 0 {
			opts = &codersdk.ChatMessagesPaginationOptions{
				Limit:    args.Limit,
				BeforeID: args.BeforeID,
				AfterID:  args.AfterID,
			}
		}
		resp, err := codersdk.NewExperimentalClient(deps.coderClient).GetChatMessages(ctx, chatID, opts)
		if err != nil {
			return GetChatMessagesResponse{}, xerrors.Errorf("get chat messages: %w", err)
		}
		messages := make([]ChatToolMessage, 0, len(resp.Messages))
		if args.AfterID > 0 {
			for _, msg := range resp.Messages {
				if toolMessage, ok := chatToolMessage(msg); ok {
					messages = append(messages, toolMessage)
				}
			}
		} else {
			for i := len(resp.Messages) - 1; i >= 0; i-- {
				if toolMessage, ok := chatToolMessage(resp.Messages[i]); ok {
					messages = append(messages, toolMessage)
				}
			}
		}
		var queued []string
		for _, msg := range resp.QueuedMessages {
			if text := userFacingText(msg.Content); text != "" {
				queued = append(queued, text)
			}
		}
		var nextBeforeID, nextAfterID int64
		if resp.HasMore && len(resp.Messages) > 0 {
			if args.AfterID > 0 {
				nextAfterID = resp.Messages[0].ID
				for _, msg := range resp.Messages {
					if msg.ID > nextAfterID {
						nextAfterID = msg.ID
					}
				}
			} else {
				nextBeforeID = resp.Messages[0].ID
				for _, msg := range resp.Messages {
					if msg.ID < nextBeforeID {
						nextBeforeID = msg.ID
					}
				}
			}
		}
		return GetChatMessagesResponse{
			Messages:       messages,
			HasMore:        resp.HasMore,
			NextBeforeID:   nextBeforeID,
			NextAfterID:    nextAfterID,
			QueuedMessages: queued,
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
		// Admin model lists include disabled providers; non-admin lists are
		// already filtered server-side.
		var providerEnabled map[uuid.UUID]bool
		providers, err := deps.coderClient.AIProviders(ctx)
		switch {
		case err == nil:
			providerEnabled = make(map[uuid.UUID]bool, len(providers))
			for _, provider := range providers {
				providerEnabled[provider.ID] = provider.Enabled
			}
		case isForbiddenError(err):
			// Deployment-config readers can receive the unfiltered admin list
			// without provider access, so fail closed unless both requests return 403.
			_, dcErr := deps.coderClient.DeploymentConfig(ctx)
			switch {
			case dcErr == nil:
				return ListChatModelConfigsResponse{}, xerrors.New("cannot verify provider availability for the admin model config list: missing AI provider read permission")
			case !isForbiddenError(dcErr):
				return ListChatModelConfigsResponse{}, xerrors.Errorf("verify deployment config access: %w", dcErr)
			}
		default:
			return ListChatModelConfigsResponse{}, xerrors.Errorf("list AI providers: %w", err)
		}
		summaries := make([]ChatModelConfigSummary, 0, len(configs))
		for _, config := range configs {
			if !config.Enabled {
				continue
			}
			// A non-nil map is authoritative because soft-deleted providers are
			// absent while their configs remain in the admin response.
			if providerEnabled != nil && !providerEnabled[config.AIProviderID] {
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
