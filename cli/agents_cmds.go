package cli

import (
	"context"
	"io"
	"slices"
	"time"

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
	chatPlanModeUpdatedMsg struct {
		generation uint64
		chatID     uuid.UUID
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
	// showAskUserQuestionMsg tells the parent model to open the
	// ask-user-question overlay.
	showAskUserQuestionMsg struct {
		state *askUserQuestionState
	}
	// hideAskUserQuestionMsg tells the parent model to close the
	// ask-user-question overlay.
	hideAskUserQuestionMsg struct{}
	// toolResultsSubmittedMsg is sent after the async SubmitToolResults
	// call completes.
	toolResultsSubmittedMsg struct {
		generation uint64
		chatID     uuid.UUID
		err        error
	}
	streamRetryMsg struct {
		generation uint64
	}
	toggleModelPickerMsg struct{}
	toggleDiffDrawerMsg  struct{}
)

func scheduleStreamRetry(generation uint64, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return streamRetryMsg{generation: generation}
	})
}

func apiCmd[T any](fn func() (T, error), wrap func(T, error) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		value, err := fn()
		return wrap(value, err)
	}
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

func submitAskUserQuestionCmd(client *codersdk.Client, chatID uuid.UUID, generation uint64, state *askUserQuestionState) tea.Cmd {
	output, err := buildAskUserQuestionToolResult(state)
	if err != nil {
		return func() tea.Msg {
			return toolResultsSubmittedMsg{generation: generation, chatID: chatID, err: err}
		}
	}

	req := codersdk.SubmitToolResultsRequest{
		Results: []codersdk.ToolResult{{
			ToolCallID: state.ToolCallID,
			Output:     output,
			IsError:    false,
		}},
	}
	return apiCmd(func() (struct{}, error) {
		return struct{}{}, codersdk.NewExperimentalClient(client).SubmitToolResults(context.Background(), chatID, req)
	}, func(_ struct{}, err error) tea.Msg {
		return toolResultsSubmittedMsg{generation: generation, chatID: chatID, err: err}
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
