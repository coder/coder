package cli //nolint:testpackage // Tests unexported chat TUI reducers.

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

func TestExpAgents(t *testing.T) {
	t.Parallel()

	t.Run("TopLevelModelRouting", func(t *testing.T) {
		t.Parallel()

		t.Run("InitWithoutChatIDReturnsListBatch", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)

			batch := mustBatchMsg(t, model.Init())
			require.Len(t, batch, 2)
			require.Equal(t, viewList, model.currentView)
		})

		t.Run("InitWithChatIDReturnsOpenAndHistoryBatch", func(t *testing.T) {
			t.Parallel()

			chatID := uuid.New()
			model := newExpChatsTUIModel(context.Background(), nil, &chatID, nil, nil)

			batch := mustBatchMsg(t, model.Init())
			require.Len(t, batch, 2)
			require.Equal(t, viewChat, model.currentView)
		})

		t.Run("EscFromModelPickerClosesOverlay", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat
			model.overlay = overlayModelPicker

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated := mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, viewChat, updated.currentView)
			require.Equal(t, overlayNone, updated.overlay)
		})

		t.Run("EscFromDiffDrawerClosesOverlay", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat
			model.overlay = overlayDiffDrawer

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated := mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, viewChat, updated.currentView)
			require.Equal(t, overlayNone, updated.overlay)
		})

		t.Run("EscFromChatViewReturnsToListAndRefreshes", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat
			model.overlay = overlayNone

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewList, updated.currentView)
			require.True(t, updated.list.loading)
			require.NotNil(t, cmd)
		})

		t.Run("EscFromSearchClearsFilterBeforeQuit", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewList
			model.list.loading = false
			model.list.searching = true
			model.list.search.SetValue("test")

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated := mustTUIModel(t, updatedModel, cmd)
			require.False(t, updated.quitting)
			require.True(t, updated.list.searching)
			require.Empty(t, updated.list.search.Value())
		})

		t.Run("EscFromEmptySearchClosesSearchBeforeQuit", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewList
			model.list.loading = false
			model.list.searching = true

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated := mustTUIModel(t, updatedModel, cmd)
			require.False(t, updated.quitting)
			require.False(t, updated.list.searching)
		})

		t.Run("EscFromListQuits", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewList

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.True(t, updated.quitting)
			_, ok := mustMsg(t, cmd).(tea.QuitMsg)
			require.True(t, ok)
		})

		t.Run("CtrlCQuitsFromAnyState", func(t *testing.T) {
			t.Parallel()

			for name, view := range map[string]tuiView{
				"List": viewList,
				"Chat": viewChat,
			} {
				name := name
				view := view
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
					model.currentView = view

					updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
					updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
					require.True(t, updated.quitting)
					_, ok := mustMsg(t, cmd).(tea.QuitMsg)
					require.True(t, ok)
				})
			}
		})

		t.Run("OpenSelectedChatSwitchesToChatAndLoadsData", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.width = 100
			model.height = 40

			updatedModel, cmd := model.Update(openSelectedChatMsg{chatID: uuid.New()})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewChat, updated.currentView)
			require.True(t, updated.chat.loading)
			require.Len(t, mustBatchMsg(t, cmd), 2)
		})

		t.Run("OpenDraftChatSwitchesToDraftMode", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)

			updatedModel, cmd := model.Update(openDraftChatMsg{})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewChat, updated.currentView)
			require.True(t, updated.chat.draft)
			require.False(t, updated.chat.loading)
			require.Nil(t, cmd)
		})

		t.Run("ToggleModelPickerTogglesOverlay", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)

			updatedModel, cmd := model.Update(toggleModelPickerMsg{})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, overlayModelPicker, updated.overlay)
			require.NotNil(t, cmd)

			updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
			updated, cmd = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, overlayNone, updated.overlay)
			require.Nil(t, cmd)
		})

		t.Run("ToggleDiffDrawerTogglesOverlay", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat.chat = &chat

			updatedModel, cmd := model.Update(toggleDiffDrawerMsg{})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, overlayDiffDrawer, updated.overlay)
			require.Len(t, mustBatchMsg(t, cmd), 2)

			updatedModel, cmd = updated.Update(toggleDiffDrawerMsg{})
			updated, cmd = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, overlayNone, updated.overlay)
			require.Nil(t, cmd)
		})
	})

	t.Run("ChatView/MessageReceiving", func(t *testing.T) {
		t.Parallel()

		t.Run("ChatOpenedStoresChatAndClearsLoading", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			diffStatus := &codersdk.ChatDiffStatus{ChatID: uuid.New()}
			chat := testChat(codersdk.ChatStatusRunning)
			chat.DiffStatus = diffStatus

			updated, cmd := model.Update(chatOpenedMsg{chat: chat})
			require.Nil(t, cmd)
			require.NotNil(t, updated.chat)
			require.Equal(t, chat.ID, updated.chat.ID)
			require.Equal(t, codersdk.ChatStatusRunning, updated.chatStatus)
			require.Equal(t, diffStatus, updated.diffStatus)
			require.False(t, updated.loading)
			require.Nil(t, updated.err)
		})

		t.Run("ChatOpenedErrorSetsErrAndClearsLoading", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)

			updated, cmd := model.Update(chatOpenedMsg{err: xerrors.New("open failed")})
			require.Nil(t, cmd)
			require.NotNil(t, updated.err)
			require.Equal(t, "open failed", updated.err.Error())
			require.False(t, updated.loading)
		})

		t.Run("ChatHistoryStoresMessagesRebuildsBlocksAndTracksLastUsage", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			usageA := &codersdk.ChatMessageUsage{TotalTokens: int64Ref(10)}
			usageB := &codersdk.ChatMessageUsage{TotalTokens: int64Ref(20)}
			messages := []codersdk.ChatMessage{
				testMessage(1, codersdk.ChatMessageRoleUser, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "first"}),
				func() codersdk.ChatMessage {
					msg := testMessage(2, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "second"})
					msg.Usage = usageA
					return msg
				}(),
				func() codersdk.ChatMessage {
					msg := testMessage(3, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeReasoning, Text: "third"})
					msg.Usage = usageB
					return msg
				}(),
			}

			updated, cmd := model.Update(chatHistoryMsg{messages: messages})
			require.Nil(t, cmd)
			require.Equal(t, messages, updated.messages)
			require.Len(t, updated.blocks, 3)
			require.Equal(t, usageB, updated.lastUsage)
			require.False(t, updated.loading)
		})

		t.Run("ChatHistoryErrorSetsErrAndClearsLoading", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)

			updated, cmd := model.Update(chatHistoryMsg{err: xerrors.New("history failed")})
			require.Nil(t, cmd)
			require.NotNil(t, updated.err)
			require.Equal(t, "history failed", updated.err.Error())
			require.False(t, updated.loading)
		})
	})

	t.Run("ChatView/StreamEvents", func(t *testing.T) {
		t.Parallel()

		t.Run("MessagePartTextAppendsAndRebuildsBlocks", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)

			updated, _ := model.Update(chatStreamEventMsg{event: testTextPartEvent("hel")})
			updated, cmd := updated.Update(chatStreamEventMsg{event: testTextPartEvent("lo")})
			require.Nil(t, cmd)
			require.True(t, updated.accumulator.isPending())
			require.Len(t, updated.accumulator.parts, 1)
			require.Equal(t, "hello", updated.accumulator.parts[0].Text)
			require.Len(t, updated.blocks, 1)
			require.Equal(t, "hello", updated.blocks[0].text)
		})

		t.Run("MessagePartToolCallDeltaAccumulatesArgs", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)

			updated, _ := model.Update(chatStreamEventMsg{event: testToolCallDeltaEvent("tc-1", "search", `{"q":"hel`)})
			updated, cmd := updated.Update(chatStreamEventMsg{event: testToolCallDeltaEvent("tc-1", "search", `lo"}`)})
			require.Nil(t, cmd)
			require.Len(t, updated.accumulator.parts, 1)
			require.Equal(t, `{"q":"hello"}`, string(updated.accumulator.parts[0].Args))
			require.Len(t, updated.blocks, 1)
			require.Equal(t, blockToolCall, updated.blocks[0].kind)
			require.Equal(t, `{"q":"hello"}`, updated.blocks[0].args)
		})

		t.Run("MessageFinalizesAndResetsAccumulator", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.reconnecting = true
			model, _ = model.Update(chatStreamEventMsg{event: testTextPartEvent("partial")})
			usage := &codersdk.ChatMessageUsage{OutputTokens: int64Ref(7)}
			message := testMessage(9, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "final"})
			message.Usage = usage

			updated, cmd := model.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:    codersdk.ChatStreamEventTypeMessage,
				Message: &message,
			}})
			require.Nil(t, cmd)
			require.Len(t, updated.messages, 1)
			require.False(t, updated.accumulator.isPending())
			require.Nil(t, updated.accumulator.parts)
			require.Equal(t, usage, updated.lastUsage)
			require.False(t, updated.reconnecting)
			require.Len(t, updated.blocks, 1)
			require.Equal(t, "final", updated.blocks[0].text)
		})

		t.Run("StatusUpdatesChatStatus", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			chat := testChat(codersdk.ChatStatusWaiting)
			model.chat = &chat
			model.chatStatus = codersdk.ChatStatusWaiting

			updated, cmd := model.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:   codersdk.ChatStreamEventTypeStatus,
				Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatusRunning},
			}})
			require.Nil(t, cmd)
			require.Equal(t, codersdk.ChatStatusRunning, updated.chatStatus)
			require.NotNil(t, updated.chat)
			require.Equal(t, codersdk.ChatStatusRunning, updated.chat.Status)
		})

		t.Run("ErrorSetsErr", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)

			updated, cmd := model.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:  codersdk.ChatStreamEventTypeError,
				Error: &codersdk.ChatStreamError{Message: "stream blew up"},
			}})
			require.Nil(t, cmd)
			require.NotNil(t, updated.err)
			require.Equal(t, "stream error: stream blew up", updated.err.Error())
		})

		t.Run("QueueUpdateReplacesQueuedMessages", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			queued := []codersdk.ChatQueuedMessage{
				testQueuedMessage(1, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "queued text"}),
			}

			updated, cmd := model.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:           codersdk.ChatStreamEventTypeQueueUpdate,
				QueuedMessages: queued,
			}})
			require.Nil(t, cmd)
			require.Equal(t, queued, updated.queuedMessages)
			require.Len(t, updated.blocks, 1)
			require.Equal(t, "queued text", updated.blocks[0].text)
		})

		t.Run("RetrySetsReconnecting", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)

			updated, cmd := model.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:  codersdk.ChatStreamEventTypeRetry,
				Retry: &codersdk.ChatStreamRetry{Attempt: 2},
			}})
			require.Nil(t, cmd)
			require.True(t, updated.reconnecting)
		})

		t.Run("EOFStopsStreamingAndAttemptsReconnectWhenInterruptible", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(failingExperimentalClient())
			chat := testChat(codersdk.ChatStatusPending)
			model.chat = &chat
			model.chatStatus = codersdk.ChatStatusPending
			model.streaming = true

			updated, cmd := model.Update(chatStreamEventMsg{err: io.EOF})
			require.Nil(t, cmd)
			require.False(t, updated.streaming)
			require.True(t, updated.reconnecting)
			require.NotNil(t, updated.err)
		})

		t.Run("MessageEventsDeduplicateByID", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			message := testMessage(11, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "hello"})

			updated, _ := model.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:    codersdk.ChatStreamEventTypeMessage,
				Message: &message,
			}})
			updated, cmd := updated.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:    codersdk.ChatStreamEventTypeMessage,
				Message: &message,
			}})
			require.Nil(t, cmd)
			require.Len(t, updated.messages, 1)
		})
	})

	t.Run("ChatView/Sending", func(t *testing.T) {
		t.Parallel()

		t.Run("DeliveredMessageIsAddedAndBlocksRebuilt", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			message := testMessage(21, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "delivered"})

			updated, cmd := model.Update(messageSentMsg{resp: codersdk.CreateChatMessageResponse{Message: &message}})
			require.Nil(t, cmd)
			require.Len(t, updated.messages, 1)
			require.Len(t, updated.blocks, 1)
			require.Equal(t, "delivered", updated.blocks[0].text)
		})

		t.Run("QueuedResponseUpdatesQueuedMessages", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			queued := testQueuedMessage(22, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "queued"})

			updated, cmd := model.Update(messageSentMsg{resp: codersdk.CreateChatMessageResponse{
				Queued:        true,
				QueuedMessage: &queued,
			}})
			require.Nil(t, cmd)
			require.Len(t, updated.queuedMessages, 1)
			require.Len(t, updated.blocks, 1)
			require.Equal(t, "queued", updated.blocks[0].text)
		})

		t.Run("SendErrorPreservesComposerText", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.composer.SetValue("keep me")

			updated, cmd := model.Update(messageSentMsg{err: xerrors.New("send failed")})
			require.Nil(t, cmd)
			require.NotNil(t, updated.err)
			require.Equal(t, "send failed", updated.err.Error())
			require.Equal(t, "keep me", updated.composer.Value())
		})
	})

	t.Run("ChatView/SendMessageEnablesAutoFollow", func(t *testing.T) {
		t.Parallel()

		model := newTestChatViewModel(nil)
		model.draft = true
		model.autoFollow = false
		model.composer.SetValue("hello")

		updated, cmd := model.sendMessage()
		require.NotNil(t, cmd)
		require.True(t, updated.autoFollow)
		require.Empty(t, updated.composer.Value())
	})

	t.Run("ChatView/DraftPromotion", func(t *testing.T) {
		t.Parallel()

		t.Run("ChatCreatedPromotesDraft", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.draft = true
			model.streaming = true
			chat := testChat(codersdk.ChatStatusWaiting)

			updated, cmd := model.Update(chatCreatedMsg{chat: chat})
			require.Nil(t, cmd)
			require.NotNil(t, updated.chat)
			require.Equal(t, chat.ID, updated.chat.ID)
			require.False(t, updated.draft)
			require.Equal(t, codersdk.ChatStatusWaiting, updated.chatStatus)
			require.Nil(t, updated.err)
		})
	})

	t.Run("ChatView/Interrupts", func(t *testing.T) {
		t.Parallel()

		t.Run("InterruptedChatClearsInterruptingAndUpdatesStatus", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.interrupting = true
			chat := testChat(codersdk.ChatStatusCompleted)

			updated, cmd := model.Update(chatInterruptedMsg{chat: chat})
			require.Nil(t, cmd)
			require.False(t, updated.interrupting)
			require.NotNil(t, updated.chat)
			require.Equal(t, chat.ID, updated.chat.ID)
			require.Equal(t, codersdk.ChatStatusCompleted, updated.chatStatus)
		})

		t.Run("InterruptedChatErrorClearsInterruptingAndSetsErr", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.interrupting = true

			updated, cmd := model.Update(chatInterruptedMsg{err: xerrors.New("interrupt failed")})
			require.Nil(t, cmd)
			require.False(t, updated.interrupting)
			require.NotNil(t, updated.err)
			require.Equal(t, "interrupt failed", updated.err.Error())
		})
	})

	t.Run("ChatView/Keyboard", func(t *testing.T) {
		t.Parallel()

		t.Run("TabTogglesComposerFocus", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			require.True(t, model.composerFocused)

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
			require.False(t, updated.composerFocused)

			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab})
			require.True(t, updated.composerFocused)
		})

		t.Run("UpDownMoveSelectedBlockWithinBounds", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.composerFocused = false
			model.blocks = []chatBlock{{kind: blockText}, {kind: blockText}, {kind: blockText}}
			model.selectedBlock = 1

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
			require.Equal(t, 0, updated.selectedBlock)

			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyUp})
			require.Equal(t, 0, updated.selectedBlock)

			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
			require.Equal(t, 1, updated.selectedBlock)

			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
			require.Equal(t, 2, updated.selectedBlock)
		})

		t.Run("EnterAndSpaceToggleExpandedBlocks", func(t *testing.T) {
			t.Parallel()

			for name, key := range map[string]tea.KeyMsg{
				"Enter": {Type: tea.KeyEnter},
				"Space": {Type: tea.KeySpace},
			} {
				name := name
				key := key
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					model := newTestChatViewModel(nil)
					model.composerFocused = false
					model.blocks = []chatBlock{{kind: blockText}}
					model.selectedBlock = 0

					updated, _ := model.Update(key)
					require.True(t, updated.expandedBlocks[0])

					updated, _ = updated.Update(key)
					require.False(t, updated.expandedBlocks[0])
				})
			}
		})

		t.Run("CtrlPSendsToggleModelPickerMsg", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.composerFocused = false

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
			require.NotNil(t, cmd)
			require.Equal(t, model.composerFocused, updated.composerFocused)
			_, ok := mustMsg(t, cmd).(toggleModelPickerMsg)
			require.True(t, ok)
		})

		t.Run("CtrlPComposerFocusedSendsToggleModelPickerMsg", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.composer.SetValue("draft")

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
			require.NotNil(t, cmd)
			require.True(t, updated.composerFocused)
			require.Equal(t, "draft", updated.composer.Value())
			_, ok := mustMsg(t, cmd).(toggleModelPickerMsg)
			require.True(t, ok)
		})

		t.Run("CtrlDComposerFocusedSendsToggleDiffDrawerMsg", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.composer.SetValue("draft")

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
			require.NotNil(t, cmd)
			require.True(t, updated.composerFocused)
			require.Equal(t, "draft", updated.composer.Value())
			_, ok := mustMsg(t, cmd).(toggleDiffDrawerMsg)
			require.True(t, ok)
		})

		t.Run("CtrlDSendsToggleDiffDrawerMsg", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.composerFocused = false

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
			require.NotNil(t, cmd)
			require.Equal(t, model.composerFocused, updated.composerFocused)
			_, ok := mustMsg(t, cmd).(toggleDiffDrawerMsg)
			require.True(t, ok)
		})
	})

	t.Run("ChatView/ViewportAndComposer", func(t *testing.T) {
		t.Parallel()

		overflowingMessages := func(count int) []codersdk.ChatMessage {
			messages := make([]codersdk.ChatMessage, 0, count)
			for i := 0; i < count; i++ {
				role := codersdk.ChatMessageRoleUser
				if i%2 == 1 {
					role = codersdk.ChatMessageRoleAssistant
				}
				messages = append(messages, testMessage(
					int64(i+1),
					role,
					codersdk.ChatMessagePart{
						Type: codersdk.ChatMessagePartTypeText,
						Text: fmt.Sprintf("message %d %s", i+1, strings.Repeat("content ", 18)),
					},
				))
			}
			return messages
		}

		t.Run("AutoFollowDefaultsToTrue", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			require.True(t, model.autoFollow)
		})

		t.Run("ViewportNavigationScrolls", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.autoFollow = false
			model.messages = overflowingMessages(8)
			model.rebuildBlocks()

			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
			model.viewport.SetYOffset(0)

			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
			require.False(t, model.composerFocused)

			for i := 0; i < len(model.blocks)-1; i++ {
				model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
			}

			require.Greater(t, model.viewport.YOffset, 0)

			for i := 0; i < len(model.blocks)-1; i++ {
				model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
			}

			require.Equal(t, 0, model.selectedBlock)
			require.Equal(t, 0, model.viewport.YOffset)
		})

		t.Run("HistoryLoadsAtBottom", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
			model, _ = model.Update(chatHistoryMsg{messages: overflowingMessages(10)})

			require.True(t, model.autoFollow)
			require.True(t, model.viewport.AtBottom())
		})

		t.Run("StreamingAutoFollows", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
			model, _ = model.Update(chatHistoryMsg{messages: overflowingMessages(10)})

			yBefore := model.viewport.YOffset
			model, _ = model.Update(chatStreamEventMsg{event: testTextPartEvent(strings.Repeat("hello world ", 20))})
			model, _ = model.Update(chatStreamEventMsg{event: testTextPartEvent(strings.Repeat("more text ", 20))})

			require.True(t, model.autoFollow)
			require.True(t, model.viewport.AtBottom())
			require.GreaterOrEqual(t, model.viewport.YOffset, yBefore)
		})

		t.Run("ManualBrowsePausesFollow", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
			model, _ = model.Update(chatHistoryMsg{messages: overflowingMessages(10)})
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})

			for model.selectedBlock < len(model.blocks)-1 {
				model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
			}
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})

			require.False(t, model.autoFollow)

			yBefore := model.viewport.YOffset
			model, _ = model.Update(chatStreamEventMsg{event: testTextPartEvent(strings.Repeat("new content ", 20))})

			require.False(t, model.autoFollow)
			require.Equal(t, yBefore, model.viewport.YOffset)
		})

		t.Run("ResumeFollowAtBottom", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
			model, _ = model.Update(chatHistoryMsg{messages: overflowingMessages(10)})
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})

			for model.selectedBlock < len(model.blocks)-1 {
				model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
			}
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
			require.False(t, model.autoFollow)

			for model.selectedBlock < len(model.blocks)-1 {
				model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
			}

			require.True(t, model.autoFollow)
			require.True(t, model.viewport.AtBottom())
		})

		t.Run("ComposerWidthConstrained", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.loading = false
			model.draft = true
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

			require.Equal(t, max(10, 80-4), model.composer.Width)

			model.composer.SetValue(strings.Repeat("x", 200))
			view := model.View()
			for _, line := range strings.Split(view, "\n") {
				require.LessOrEqual(t, lipgloss.Width(line), model.width, "line too wide: %q", line)
			}
		})
	})

	t.Run("ChatView/RendererCaching", func(t *testing.T) {
		t.Parallel()

		t.Run("SameWidthReusesRenderer", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.width = 80

			rendererA := (&model).getOrCreateMarkdownRenderer(80)
			rendererB := (&model).getOrCreateMarkdownRenderer(80)
			require.NotNil(t, rendererA)
			require.Same(t, rendererA, rendererB)
		})

		t.Run("DifferentWidthRecreatesRenderer", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.width = 80

			rendererA := (&model).getOrCreateMarkdownRenderer(80)
			rendererB := (&model).getOrCreateMarkdownRenderer(60)
			require.NotNil(t, rendererA)
			require.NotNil(t, rendererB)
			require.NotSame(t, rendererA, rendererB)
		})
	})

	t.Run("ChatView/TranscriptSync", func(t *testing.T) {
		t.Parallel()

		newTranscriptModel := func() chatViewModel {
			model := newTestChatViewModel(nil)
			model.width = 80
			model.blocks = []chatBlock{
				{kind: blockText, role: codersdk.ChatMessageRoleUser, text: "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi"},
				{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "assistant reply"},
			}
			model.selectedBlock = 0
			model.composerFocused = false
			return model
		}

		t.Run("RepeatedViewDoesNotRefreshViewport", func(t *testing.T) {
			t.Parallel()

			model := newTranscriptModel()
			(&model).syncViewportContent()
			firstTranscript := model.lastTranscript
			require.NotEmpty(t, firstTranscript)

			(&model).syncViewportContent()
			require.Equal(t, firstTranscript, model.lastTranscript)
		})

		t.Run("BlockChangeRefreshesViewport", func(t *testing.T) {
			t.Parallel()

			model := newTranscriptModel()
			(&model).syncViewportContent()
			firstTranscript := model.lastTranscript

			model.blocks = append(model.blocks, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "new block"})
			(&model).syncViewportContent()
			require.NotEqual(t, firstTranscript, model.lastTranscript)
		})

		t.Run("SelectionChangeRefreshesViewport", func(t *testing.T) {
			t.Parallel()

			model := newTranscriptModel()
			(&model).syncViewportContent()
			firstTranscript := model.lastTranscript

			model.selectedBlock = 1
			(&model).syncViewportContent()
			require.NotEqual(t, firstTranscript, model.lastTranscript)
		})

		t.Run("WidthChangeRefreshesViewport", func(t *testing.T) {
			t.Parallel()

			model := newTranscriptModel()
			(&model).syncViewportContent()
			firstTranscript := model.lastTranscript

			model.width = 60
			(&model).syncViewportContent()
			require.NotEqual(t, firstTranscript, model.lastTranscript)
		})

		t.Run("ComposerFocusChangeRefreshesViewport", func(t *testing.T) {
			t.Parallel()

			model := newTranscriptModel()
			(&model).syncViewportContent()
			firstTranscript := model.lastTranscript

			model.composerFocused = true
			(&model).syncViewportContent()
			require.NotEqual(t, firstTranscript, model.lastTranscript)
		})
	})

	t.Run("ChatView/BlockCachePreservation", func(t *testing.T) {
		t.Parallel()

		t.Run("UnchangedBlocksKeepCache", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.width = 80
			model.messages = []codersdk.ChatMessage{
				testMessage(1, codersdk.ChatMessageRoleUser, codersdk.ChatMessagePart{
					Type: codersdk.ChatMessagePartTypeText,
					Text: "cached block content",
				}),
			}

			model.rebuildBlocks()
			require.Len(t, model.blocks, 1)
			cachedRender := model.blocks[0].cachedRender
			require.NotEmpty(t, cachedRender)

			model.rebuildBlocks()
			require.Len(t, model.blocks, 1)
			require.Equal(t, cachedRender, model.blocks[0].cachedRender)
		})

		t.Run("ChangedBlockLosesCache", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.width = 80
			model.messages = []codersdk.ChatMessage{
				testMessage(1, codersdk.ChatMessageRoleUser, codersdk.ChatMessagePart{
					Type: codersdk.ChatMessagePartTypeText,
					Text: "stable block",
				}),
			}
			model.accumulator = streamAccumulator{
				pending: true,
				role:    codersdk.ChatMessageRoleAssistant,
				parts: []codersdk.ChatMessagePart{{
					Type: codersdk.ChatMessagePartTypeText,
					Text: "partial response",
				}},
			}

			model.rebuildBlocks()
			require.Len(t, model.blocks, 2)
			cachedRender := model.blocks[len(model.blocks)-1].cachedRender
			require.NotEmpty(t, cachedRender)

			model.accumulator.parts[0].Text = "partial response updated"
			model.rebuildBlocks()
			require.Len(t, model.blocks, 2)
			require.NotEqual(t, cachedRender, model.blocks[len(model.blocks)-1].cachedRender)
		})
	})

	t.Run("StreamAccumulator", func(t *testing.T) {
		t.Parallel()

		t.Run("ApplyDeltaTextAppends", func(t *testing.T) {
			t.Parallel()

			var accumulator streamAccumulator
			accumulator.applyDelta(codersdk.ChatStreamMessagePart{
				Role: codersdk.ChatMessageRoleAssistant,
				Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "hel"},
			})
			accumulator.applyDelta(codersdk.ChatStreamMessagePart{
				Role: codersdk.ChatMessageRoleAssistant,
				Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "lo"},
			})

			require.Len(t, accumulator.parts, 1)
			require.Equal(t, "hello", accumulator.parts[0].Text)
		})

		t.Run("ApplyDeltaReasoningAppends", func(t *testing.T) {
			t.Parallel()

			var accumulator streamAccumulator
			accumulator.applyDelta(codersdk.ChatStreamMessagePart{
				Role: codersdk.ChatMessageRoleAssistant,
				Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeReasoning, Text: "thin"},
			})
			accumulator.applyDelta(codersdk.ChatStreamMessagePart{
				Role: codersdk.ChatMessageRoleAssistant,
				Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeReasoning, Text: "king"},
			})

			require.Len(t, accumulator.parts, 1)
			require.Equal(t, "thinking", accumulator.parts[0].Text)
		})

		t.Run("ApplyDeltaToolCallAccumulatesArgs", func(t *testing.T) {
			t.Parallel()

			var accumulator streamAccumulator
			accumulator.applyDelta(codersdk.ChatStreamMessagePart{
				Role: codersdk.ChatMessageRoleAssistant,
				Part: codersdk.ChatMessagePart{
					Type:       codersdk.ChatMessagePartTypeToolCall,
					ToolCallID: "tool-1",
					ToolName:   "search",
					ArgsDelta:  `{"query":"he`,
				},
			})
			accumulator.applyDelta(codersdk.ChatStreamMessagePart{
				Role: codersdk.ChatMessageRoleAssistant,
				Part: codersdk.ChatMessagePart{
					Type:       codersdk.ChatMessagePartTypeToolCall,
					ToolCallID: "tool-1",
					ToolName:   "search",
					ArgsDelta:  `llo"}`,
				},
			})

			require.Len(t, accumulator.parts, 1)
			require.Equal(t, `{"query":"hello"}`, string(accumulator.parts[0].Args))
		})

		t.Run("ResetClearsState", func(t *testing.T) {
			t.Parallel()

			accumulator := streamAccumulator{
				parts:      []codersdk.ChatMessagePart{{Type: codersdk.ChatMessagePartTypeText, Text: "hello"}},
				role:       codersdk.ChatMessageRoleAssistant,
				pending:    true,
				toolDeltas: map[string]string{"tool-1": `{}`},
			}

			accumulator.reset()
			require.Nil(t, accumulator.parts)
			require.Equal(t, codersdk.ChatMessageRole(""), accumulator.role)
			require.False(t, accumulator.pending)
			require.Nil(t, accumulator.toolDeltas)
		})

		t.Run("IsPendingAfterDelta", func(t *testing.T) {
			t.Parallel()

			var accumulator streamAccumulator
			accumulator.applyDelta(codersdk.ChatStreamMessagePart{
				Role: codersdk.ChatMessageRoleAssistant,
				Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "hello"},
			})

			require.True(t, accumulator.isPending())
		})
	})

	t.Run("ChatList", func(t *testing.T) {
		t.Parallel()

		t.Run("ChatsListedStoresChats", func(t *testing.T) {
			t.Parallel()

			model := newChatListModel(newTUIStyles())
			chats := []codersdk.Chat{testChat(codersdk.ChatStatusWaiting), testChat(codersdk.ChatStatusCompleted)}

			updated, cmd := model.Update(chatsListedMsg{chats: chats})
			require.Nil(t, cmd)
			require.Equal(t, chats, updated.chats)
			require.False(t, updated.loading)
		})

		t.Run("ChatsListedErrorStoresErr", func(t *testing.T) {
			t.Parallel()

			model := newChatListModel(newTUIStyles())

			updated, cmd := model.Update(chatsListedMsg{err: xerrors.New("list failed")})
			require.Nil(t, cmd)
			require.NotNil(t, updated.err)
			require.Equal(t, "list failed", updated.err.Error())
			require.False(t, updated.loading)
		})

		t.Run("DisplayRowsHideSubagentsUntilExpanded", func(t *testing.T) {
			t.Parallel()

			model := newChatListModel(newTUIStyles())
			model.loading = false

			parent := testChat(codersdk.ChatStatusRunning)
			parent.Title = "Parent chat"
			childA := testChat(codersdk.ChatStatusWaiting)
			childA.Title = "Subagent A"
			childA.ParentChatID = &parent.ID
			childB := testChat(codersdk.ChatStatusPending)
			childB.Title = "Subagent B"
			childB.ParentChatID = &parent.ID
			root := testChat(codersdk.ChatStatusCompleted)
			root.Title = "Standalone chat"

			model.chats = []codersdk.Chat{parent, childA, childB, root}

			rows := model.displayRows()
			require.Len(t, rows, 2)
			require.Equal(t, parent.ID, rows[0].chat.ID)
			require.Equal(t, 2, rows[0].childCount)
			require.False(t, rows[0].isExpanded)
			require.Equal(t, root.ID, rows[1].chat.ID)
			require.False(t, rows[1].isSubagent)

			output := plainText(model.View())
			require.Contains(t, output, "▶ Parent chat")
			require.Contains(t, output, "(2 subagents)")
			require.NotContains(t, output, childA.Title)
			require.NotContains(t, output, childB.Title)
		})

		t.Run("ExpandCollapseAndOpenSubagents", func(t *testing.T) {
			t.Parallel()

			model := newChatListModel(newTUIStyles())
			model.loading = false
			model.width = 100
			model.height = 10

			parent := testChat(codersdk.ChatStatusRunning)
			parent.Title = "Parent chat"
			child := testChat(codersdk.ChatStatusWaiting)
			child.Title = "Subagent chat"
			child.ParentChatID = &parent.ID
			root := testChat(codersdk.ChatStatusCompleted)
			root.Title = "Standalone chat"
			model.chats = []codersdk.Chat{parent, child, root}

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRight})
			require.Nil(t, cmd)
			require.True(t, updated.expanded[parent.ID])
			require.Len(t, updated.displayRows(), 3)
			require.Contains(t, plainText(updated.View()), "    Subagent chat")

			updated, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
			require.Nil(t, cmd)
			require.Equal(t, 1, updated.cursor)
			selected := updated.selectedChat()
			require.NotNil(t, selected)
			require.Equal(t, child.ID, selected.ID)

			updated, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
			msg := mustMsg(t, cmd)
			openMsg, ok := msg.(openSelectedChatMsg)
			require.True(t, ok)
			require.Equal(t, child.ID, openMsg.chatID)

			updated, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyLeft})
			require.Nil(t, cmd)
			require.False(t, updated.expanded[parent.ID])
			require.Equal(t, 0, updated.cursor)
			require.Len(t, updated.displayRows(), 2)
		})

		t.Run("ToggleKeyExpandsAndCollapsesParent", func(t *testing.T) {
			t.Parallel()

			model := newChatListModel(newTUIStyles())
			model.loading = false

			parent := testChat(codersdk.ChatStatusRunning)
			parent.Title = "Parent chat"
			child := testChat(codersdk.ChatStatusWaiting)
			child.Title = "Subagent chat"
			child.ParentChatID = &parent.ID
			model.chats = []codersdk.Chat{parent, child}

			updated, cmd := model.Update(keyRunes("x"))
			require.Nil(t, cmd)
			require.True(t, updated.expanded[parent.ID])
			require.Len(t, updated.displayRows(), 2)

			updated, cmd = updated.Update(keyRunes("x"))
			require.Nil(t, cmd)
			require.False(t, updated.expanded[parent.ID])
			require.Len(t, updated.displayRows(), 1)
		})

		t.Run("SearchIncludesMatchingSubagentUnderParent", func(t *testing.T) {
			t.Parallel()

			model := newChatListModel(newTUIStyles())
			model.loading = false

			parent := testChat(codersdk.ChatStatusRunning)
			parent.Title = "Parent chat"
			child := testChat(codersdk.ChatStatusWaiting)
			child.Title = "Subagent needle"
			child.ParentChatID = &parent.ID
			other := testChat(codersdk.ChatStatusCompleted)
			other.Title = "Other root"
			model.chats = []codersdk.Chat{parent, child, other}
			model.search.SetValue("needle")

			rows := model.displayRows()
			require.Len(t, rows, 2)
			require.Equal(t, parent.ID, rows[0].chat.ID)
			require.True(t, rows[0].isExpanded)
			require.Equal(t, child.ID, rows[1].chat.ID)
			require.True(t, rows[1].isSubagent)
		})

		t.Run("NavigationKeysMoveCursorWithinBounds", func(t *testing.T) {
			t.Parallel()

			chats := []codersdk.Chat{testChat(codersdk.ChatStatusWaiting), testChat(codersdk.ChatStatusPending), testChat(codersdk.ChatStatusCompleted)}
			for name, key := range map[string]tea.KeyMsg{
				"Up":   {Type: tea.KeyUp},
				"Down": {Type: tea.KeyDown},
				"J":    keyRunes("j"),
				"K":    keyRunes("k"),
			} {
				name := name
				key := key
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					model := newChatListModel(newTUIStyles())
					model.loading = false
					model.chats = chats
					model.cursor = 1

					updated, _ := model.Update(key)
					if name == "Up" || name == "K" {
						require.Equal(t, 0, updated.cursor)
						updated, _ = updated.Update(key)
						require.Equal(t, 0, updated.cursor)
					} else {
						require.Equal(t, 2, updated.cursor)
						updated, _ = updated.Update(key)
						require.Equal(t, 2, updated.cursor)
					}
				})
			}
		})

		t.Run("ViewKeepsSelectedChatVisible", func(t *testing.T) {
			t.Parallel()

			model := newChatListModel(newTUIStyles())
			model.loading = false
			model.width = 80
			model.height = 8
			model.chats = make([]codersdk.Chat, 8)
			for i := range model.chats {
				chat := testChat(codersdk.ChatStatusWaiting)
				chat.Title = fmt.Sprintf("chat %02d", i)
				model.chats[i] = chat
			}

			for range 6 {
				var cmd tea.Cmd
				model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyDown})
				require.Nil(t, cmd)
			}

			require.Equal(t, 6, model.cursor)
			require.Equal(t, 2, model.offset)

			listView := plainText(model.View())
			require.Contains(t, listView, "> chat 06")
			require.NotContains(t, listView, "chat 00")
			require.NotContains(t, listView, "chat 01")

			parent := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			parent.currentView = viewList
			parent.width = 80
			parent.height = 8
			parent.list = model

			parentView := plainText(parent.View())
			require.Contains(t, parentView, "Coder Chats")
			require.Contains(t, parentView, "> chat 06")

			for range 5 {
				var cmd tea.Cmd
				model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyUp})
				require.Nil(t, cmd)
			}

			require.Equal(t, 1, model.cursor)
			require.Equal(t, 1, model.offset)
			require.Contains(t, plainText(model.View()), "> chat 01")
		})

		t.Run("SlashFocusesSearch", func(t *testing.T) {
			t.Parallel()

			model := newChatListModel(newTUIStyles())
			model.loading = false

			updated, cmd := model.Update(keyRunes("/"))
			require.Nil(t, cmd)
			require.True(t, updated.searching)
		})

		t.Run("QTriggersQuit", func(t *testing.T) {
			t.Parallel()

			model := newChatListModel(newTUIStyles())
			model.loading = false

			updated, cmd := model.Update(keyRunes("q"))
			require.Equal(t, model.cursor, updated.cursor)
			_, ok := mustMsg(t, cmd).(tea.QuitMsg)
			require.True(t, ok)
		})
	})
}

func mustTUIModel(t testing.TB, model tea.Model, cmd tea.Cmd) expChatsTUIModel {
	t.Helper()

	updated, ok := model.(expChatsTUIModel)
	require.True(t, ok)
	require.Nil(t, cmd)
	return updated
}

func mustTUIModelWithCmd(t testing.TB, model tea.Model, cmd tea.Cmd) (expChatsTUIModel, tea.Cmd) {
	t.Helper()

	updated, ok := model.(expChatsTUIModel)
	require.True(t, ok)
	return updated, cmd
}

func mustMsg(t testing.TB, cmd tea.Cmd) tea.Msg {
	t.Helper()
	require.NotNil(t, cmd)
	return cmd()
}

func mustBatchMsg(t testing.TB, cmd tea.Cmd) tea.BatchMsg {
	t.Helper()

	msg := mustMsg(t, cmd)
	batch, ok := msg.(tea.BatchMsg)
	require.True(t, ok)
	return batch
}

func newTestChatViewModel(client *codersdk.ExperimentalClient) chatViewModel {
	return newChatViewModel(context.Background(), client, nil, nil, newTUIStyles())
}

func testChat(status codersdk.ChatStatus) codersdk.Chat {
	return codersdk.Chat{
		ID:        uuid.New(),
		Title:     "test chat",
		Status:    status,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func testMessage(id int64, role codersdk.ChatMessageRole, parts ...codersdk.ChatMessagePart) codersdk.ChatMessage {
	return codersdk.ChatMessage{
		ID:        id,
		ChatID:    uuid.New(),
		CreatedAt: time.Now(),
		Role:      role,
		Content:   parts,
	}
}

func testQueuedMessage(id int64, parts ...codersdk.ChatMessagePart) codersdk.ChatQueuedMessage {
	return codersdk.ChatQueuedMessage{
		ID:        id,
		ChatID:    uuid.New(),
		CreatedAt: time.Now(),
		Content:   parts,
	}
}

func testTextPartEvent(text string) codersdk.ChatStreamEvent {
	return codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role: codersdk.ChatMessageRoleAssistant,
			Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: text},
		},
	}
}

func testToolCallDeltaEvent(toolCallID string, toolName string, delta string) codersdk.ChatStreamEvent {
	return codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role: codersdk.ChatMessageRoleAssistant,
			Part: codersdk.ChatMessagePart{
				Type:       codersdk.ChatMessagePartTypeToolCall,
				ToolCallID: toolCallID,
				ToolName:   toolName,
				ArgsDelta:  delta,
			},
		},
	}
}

func failingExperimentalClient() *codersdk.ExperimentalClient {
	return codersdk.NewExperimentalClient(codersdk.New(&url.URL{}))
}

func keyRunes(value string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
}

func int64Ref(v int64) *int64 {
	return &v
}
