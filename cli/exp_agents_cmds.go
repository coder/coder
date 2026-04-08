package cli

import (
	"context"
	"io"

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
		chat codersdk.Chat
		err  error
	}
	chatHistoryMsg struct {
		messages []codersdk.ChatMessage
		err      error
	}
	chatCreatedMsg struct {
		chat codersdk.Chat
		err  error
	}
	messageSentMsg struct {
		resp codersdk.CreateChatMessageResponse
		err  error
	}
	chatInterruptedMsg struct {
		chat codersdk.Chat
		err  error
	}
	modelsListedMsg struct {
		catalog codersdk.ChatModelsResponse
		err     error
	}
	gitChangesMsg struct {
		changes []codersdk.ChatGitChange
		err     error
	}
	diffContentsMsg struct {
		diff codersdk.ChatDiffContents
		err  error
	}
	chatStreamEventMsg struct {
		event codersdk.ChatStreamEvent
		err   error
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

func streamCmd[T any](ch <-chan T, wrap func(T, error) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		value, ok := <-ch
		if !ok {
			var zero T
			return wrap(zero, io.EOF)
		}
		return wrap(value, nil)
	}
}

func listChatsCmd(ctx context.Context, client *codersdk.ExperimentalClient) tea.Cmd {
	return apiCmd(func() ([]codersdk.Chat, error) { return client.ListChats(ctx, nil) }, func(chats []codersdk.Chat, err error) tea.Msg { return chatsListedMsg{chats: chats, err: err} })
}

func openChatCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID) tea.Cmd {
	return apiCmd(func() (codersdk.Chat, error) { return client.GetChat(ctx, chatID) }, func(chat codersdk.Chat, err error) tea.Msg { return chatOpenedMsg{chat: chat, err: err} })
}

func loadChatHistoryCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID) tea.Cmd {
	return apiCmd(func() ([]codersdk.ChatMessage, error) { return fetchAllChatMessages(ctx, client, chatID) }, func(messages []codersdk.ChatMessage, err error) tea.Msg {
		return chatHistoryMsg{messages: messages, err: err}
	})
}

func createChatCmd(ctx context.Context, client *codersdk.ExperimentalClient, req codersdk.CreateChatRequest) tea.Cmd {
	return apiCmd(func() (codersdk.Chat, error) { return client.CreateChat(ctx, req) }, func(chat codersdk.Chat, err error) tea.Msg { return chatCreatedMsg{chat: chat, err: err} })
}

func sendMessageCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID, req codersdk.CreateChatMessageRequest) tea.Cmd {
	return apiCmd(func() (codersdk.CreateChatMessageResponse, error) { return client.CreateChatMessage(ctx, chatID, req) }, func(resp codersdk.CreateChatMessageResponse, err error) tea.Msg {
		return messageSentMsg{resp: resp, err: err}
	})
}

func interruptChatCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID) tea.Cmd {
	return apiCmd(func() (codersdk.Chat, error) { return client.InterruptChat(ctx, chatID) }, func(chat codersdk.Chat, err error) tea.Msg { return chatInterruptedMsg{chat: chat, err: err} })
}

func listModelsCmd(ctx context.Context, client *codersdk.ExperimentalClient) tea.Cmd {
	return apiCmd(func() (codersdk.ChatModelsResponse, error) { return client.ListChatModels(ctx) }, func(catalog codersdk.ChatModelsResponse, err error) tea.Msg {
		return modelsListedMsg{catalog: catalog, err: err}
	})
}

func loadGitChangesCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID) tea.Cmd {
	return apiCmd(func() ([]codersdk.ChatGitChange, error) { return client.GetChatGitChanges(ctx, chatID) }, func(changes []codersdk.ChatGitChange, err error) tea.Msg {
		return gitChangesMsg{changes: changes, err: err}
	})
}

func loadDiffContentsCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID) tea.Cmd {
	return apiCmd(func() (codersdk.ChatDiffContents, error) { return client.GetChatDiffContents(ctx, chatID) }, func(diff codersdk.ChatDiffContents, err error) tea.Msg { return diffContentsMsg{diff: diff, err: err} })
}

func listenToStream(eventCh <-chan codersdk.ChatStreamEvent) tea.Cmd {
	return streamCmd(eventCh, func(event codersdk.ChatStreamEvent, err error) tea.Msg {
		return chatStreamEventMsg{event: event, err: err}
	})
}
