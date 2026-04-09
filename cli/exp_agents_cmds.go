package cli

import (
	"context"
	"io"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

type (
	chatsListedMsg struct {
		chats []codersdk.Chat
		err   error
	}
	chatOpenedMsg struct {
		generation uint64
		chatID     uuid.UUID
		chat       codersdk.Chat
		err        error
	}
	chatHistoryMsg struct {
		generation uint64
		chatID     uuid.UUID
		messages   []codersdk.ChatMessage
		err        error
	}
	chatCreatedMsg struct {
		generation uint64
		chatID     uuid.UUID
		chat       codersdk.Chat
		err        error
	}
	messageSentMsg struct {
		generation uint64
		chatID     uuid.UUID
		resp       codersdk.CreateChatMessageResponse
		err        error
	}
	chatInterruptedMsg struct {
		generation uint64
		chatID     uuid.UUID
		chat       codersdk.Chat
		err        error
	}
	modelsListedMsg struct {
		catalog codersdk.ChatModelsResponse
		err     error
	}
	gitChangesMsg struct {
		generation uint64
		chatID     uuid.UUID
		changes    []codersdk.ChatGitChange
		err        error
	}
	diffContentsMsg struct {
		generation uint64
		chatID     uuid.UUID
		diff       codersdk.ChatDiffContents
		err        error
	}
	chatStreamEventMsg struct {
		generation uint64
		chatID     uuid.UUID
		event      codersdk.ChatStreamEvent
		err        error
	}
	toggleModelPickerMsg struct{}
	toggleDiffDrawerMsg  struct{}
)

func apiCmd[T any](fn func() (T, error), wrap func(T, error) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		value, err := fn()
		return wrap(value, err)
	}
}

func listChatsCmd(ctx context.Context, client *codersdk.ExperimentalClient) tea.Cmd {
	return apiCmd(func() ([]codersdk.Chat, error) { return client.ListChats(ctx, nil) }, func(chats []codersdk.Chat, err error) tea.Msg { return chatsListedMsg{chats: chats, err: err} })
}

func openChatCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID, generation uint64) tea.Cmd {
	return apiCmd(func() (codersdk.Chat, error) { return client.GetChat(ctx, chatID) }, func(chat codersdk.Chat, err error) tea.Msg {
		return chatOpenedMsg{generation: generation, chatID: chatID, chat: chat, err: err}
	})
}

func loadChatHistoryCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID, generation uint64) tea.Cmd {
	return apiCmd(func() ([]codersdk.ChatMessage, error) {
		var (
			allMessages []codersdk.ChatMessage
			opts        *codersdk.ChatMessagesPaginationOptions
		)

		for {
			resp, err := client.GetChatMessages(ctx, chatID, opts)
			if err != nil {
				return nil, err
			}

			allMessages = append(allMessages, resp.Messages...)
			if !resp.HasMore || len(resp.Messages) == 0 {
				break
			}

			opts = &codersdk.ChatMessagesPaginationOptions{
				BeforeID: resp.Messages[len(resp.Messages)-1].ID,
			}
		}

		slices.SortStableFunc(allMessages, func(a, b codersdk.ChatMessage) int {
			switch {
			case a.CreatedAt.Before(b.CreatedAt):
				return -1
			case a.CreatedAt.After(b.CreatedAt):
				return 1
			case a.ID < b.ID:
				return -1
			case a.ID > b.ID:
				return 1
			default:
				return 0
			}
		})

		return allMessages, nil
	}, func(messages []codersdk.ChatMessage, err error) tea.Msg {
		return chatHistoryMsg{generation: generation, chatID: chatID, messages: messages, err: err}
	})
}

func createChatCmd(ctx context.Context, client *codersdk.ExperimentalClient, req codersdk.CreateChatRequest, modelOverride *string, generation uint64) tea.Cmd {
	return apiCmd(func() (codersdk.Chat, error) {
		if req.ModelConfigID == nil && modelOverride != nil {
			modelConfigID, err := resolveModelConfigID(ctx, client, modelOverride)
			if err != nil {
				return codersdk.Chat{}, err
			}
			req.ModelConfigID = modelConfigID
		}
		return client.CreateChat(ctx, req)
	}, func(chat codersdk.Chat, err error) tea.Msg {
		return chatCreatedMsg{generation: generation, chatID: chat.ID, chat: chat, err: err}
	})
}

func sendMessageCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID, req codersdk.CreateChatMessageRequest, modelOverride *string, generation uint64) tea.Cmd {
	return apiCmd(func() (codersdk.CreateChatMessageResponse, error) {
		if req.ModelConfigID == nil && modelOverride != nil {
			modelConfigID, err := resolveModelConfigID(ctx, client, modelOverride)
			if err != nil {
				return codersdk.CreateChatMessageResponse{}, err
			}
			req.ModelConfigID = modelConfigID
		}
		return client.CreateChatMessage(ctx, chatID, req)
	}, func(resp codersdk.CreateChatMessageResponse, err error) tea.Msg {
		return messageSentMsg{generation: generation, chatID: chatID, resp: resp, err: err}
	})
}

func listenToStream(chatID uuid.UUID, generation uint64, eventCh <-chan codersdk.ChatStreamEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-eventCh
		if !ok {
			return chatStreamEventMsg{generation: generation, chatID: chatID, err: io.EOF}
		}
		return chatStreamEventMsg{generation: generation, chatID: chatID, event: event}
	}
}
