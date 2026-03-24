package cli

import (
	"context"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

type chatsListedMsg struct {
	chats []codersdk.Chat
	err   error
}

type chatOpenedMsg struct {
	chat codersdk.Chat
	err  error
}

type chatHistoryMsg struct {
	messages []codersdk.ChatMessage
	err      error
}

type chatCreatedMsg struct {
	chat codersdk.Chat
	err  error
}

type messageSentMsg struct {
	resp codersdk.CreateChatMessageResponse
	err  error
}

type chatInterruptedMsg struct {
	chat codersdk.Chat
	err  error
}

type modelsListedMsg struct {
	catalog codersdk.ChatModelsResponse
	err     error
}

type gitChangesMsg struct {
	changes []codersdk.ChatGitChange
	err     error
}

type diffContentsMsg struct {
	diff codersdk.ChatDiffContents
	err  error
}

type chatStreamEventMsg struct {
	event codersdk.ChatStreamEvent
	err   error
}

type toggleModelPickerMsg struct{}

type toggleDiffDrawerMsg struct{}

func listChatsCmd(ctx context.Context, client *codersdk.ExperimentalClient) tea.Cmd {
	return func() tea.Msg {
		chats, err := client.ListChats(ctx, nil)
		return chatsListedMsg{chats: chats, err: err}
	}
}

func openChatCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID) tea.Cmd {
	return func() tea.Msg {
		chat, err := client.GetChat(ctx, chatID)
		return chatOpenedMsg{chat: chat, err: err}
	}
}

func loadChatHistoryCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID) tea.Cmd {
	return func() tea.Msg {
		messages, err := fetchAllChatMessages(ctx, client, chatID)
		return chatHistoryMsg{messages: messages, err: err}
	}
}

func createChatCmd(ctx context.Context, client *codersdk.ExperimentalClient, req codersdk.CreateChatRequest) tea.Cmd {
	return func() tea.Msg {
		chat, err := client.CreateChat(ctx, req)
		return chatCreatedMsg{chat: chat, err: err}
	}
}

func sendMessageCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID, req codersdk.CreateChatMessageRequest) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.CreateChatMessage(ctx, chatID, req)
		return messageSentMsg{resp: resp, err: err}
	}
}

func interruptChatCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID) tea.Cmd {
	return func() tea.Msg {
		chat, err := client.InterruptChat(ctx, chatID)
		return chatInterruptedMsg{chat: chat, err: err}
	}
}

func listModelsCmd(ctx context.Context, client *codersdk.ExperimentalClient) tea.Cmd {
	return func() tea.Msg {
		catalog, err := client.ListChatModels(ctx)
		return modelsListedMsg{catalog: catalog, err: err}
	}
}

func loadGitChangesCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID) tea.Cmd {
	return func() tea.Msg {
		changes, err := client.GetChatGitChanges(ctx, chatID)
		return gitChangesMsg{changes: changes, err: err}
	}
}

func loadDiffContentsCmd(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID) tea.Cmd {
	return func() tea.Msg {
		diff, err := client.GetChatDiffContents(ctx, chatID)
		return diffContentsMsg{diff: diff, err: err}
	}
}

func listenToStream(eventCh <-chan codersdk.ChatStreamEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-eventCh
		if !ok {
			return chatStreamEventMsg{err: io.EOF}
		}

		return chatStreamEventMsg{event: event}
	}
}
