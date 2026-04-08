package cli //nolint:testpackage // Tests unexported chat TUI reducers.

import (
	"context"
	"encoding/json"
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
			require.Len(t, batch, 3)
			require.Equal(t, viewChat, model.currentView)
		})

		t.Run("EscFromOverlayClosesIt", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name    string
				overlay tuiOverlay
			}{
				{"ModelPicker", overlayModelPicker},
				{"DiffDrawer", overlayDiffDrawer},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
					model.currentView = viewChat
					model.overlay = tt.overlay

					updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
					updated := mustTUIModel(t, updatedModel, cmd)
					require.Equal(t, viewChat, updated.currentView)
					require.Equal(t, overlayNone, updated.overlay)
				})
			}
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

		t.Run("OpenChatSwitchesView", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name   string
				msg    tea.Msg
				assert func(t *testing.T, updated expChatsTUIModel, cmd tea.Cmd)
			}{
				{
					"SelectedChat",
					openSelectedChatMsg{chatID: uuid.New()},
					func(t *testing.T, updated expChatsTUIModel, cmd tea.Cmd) {
						t.Helper()
						require.Equal(t, viewChat, updated.currentView)
						require.True(t, updated.chat.loading)
						require.Equal(t, 39, updated.chat.height)
						require.Equal(t, 31, updated.chat.viewport.Height)
						require.Len(t, mustBatchMsg(t, cmd), 3)
					},
				},
				{
					"DraftChat",
					openDraftChatMsg{},
					func(t *testing.T, updated expChatsTUIModel, cmd tea.Cmd) {
						t.Helper()
						require.Equal(t, viewChat, updated.currentView)
						require.True(t, updated.chat.draft)
						require.False(t, updated.chat.loading)
						require.Equal(t, 39, updated.chat.height)
						require.Equal(t, 31, updated.chat.viewport.Height)
						require.Nil(t, cmd)
					},
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
					model.width = 100
					model.height = 40

					updatedModel, cmd := model.Update(tt.msg)
					updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
					tt.assert(t, updated, cmd)
				})
			}
		})

		t.Run("OverlayToggling", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name  string
				setup func(*expChatsTUIModel)
				run   func(t *testing.T, model expChatsTUIModel)
			}{
				{
					name: "ModelPicker",
					run: func(t *testing.T, model expChatsTUIModel) {
						t.Helper()

						updatedModel, cmd := model.Update(toggleModelPickerMsg{})
						updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
						require.Equal(t, overlayModelPicker, updated.overlay)
						require.NotNil(t, cmd)

						updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
						updated, cmd = mustTUIModelWithCmd(t, updatedModel, cmd)
						require.Equal(t, overlayNone, updated.overlay)
						require.Nil(t, cmd)
					},
				},
				{
					name: "DiffDrawer",
					setup: func(model *expChatsTUIModel) {
						chat := testChat(codersdk.ChatStatusCompleted)
						model.chat.chat = &chat
					},
					run: func(t *testing.T, model expChatsTUIModel) {
						t.Helper()

						updatedModel, cmd := model.Update(toggleDiffDrawerMsg{})
						updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
						require.Equal(t, overlayDiffDrawer, updated.overlay)
						require.Len(t, mustBatchMsg(t, cmd), 2)

						updatedModel, cmd = updated.Update(toggleDiffDrawerMsg{})
						updated, cmd = mustTUIModelWithCmd(t, updatedModel, cmd)
						require.Equal(t, overlayNone, updated.overlay)
						require.Nil(t, cmd)
					},
				},
				{
					name: "RapidToggle",
					setup: func(model *expChatsTUIModel) {
						catalog := codersdk.ChatModelsResponse{
							Providers: []codersdk.ChatModelProvider{{
								Provider:  "provider",
								Available: true,
								Models: []codersdk.ChatModel{{
									ID:          uuid.New().String(),
									Provider:    "provider",
									Model:       "model-a",
									DisplayName: "Model A",
								}},
							}},
						}
						model.currentView = viewChat
						model.catalog = &catalog
					},
					run: func(t *testing.T, model expChatsTUIModel) {
						t.Helper()

						expectedStates := []tuiOverlay{overlayModelPicker, overlayNone, overlayModelPicker, overlayNone}
						updated := model
						for _, expected := range expectedStates {
							updatedModel, cmd := updated.Update(toggleModelPickerMsg{})
							updated = mustTUIModel(t, updatedModel, cmd)
							require.Equal(t, expected, updated.overlay)
						}
					},
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
					if tt.setup != nil {
						tt.setup(&model)
					}

					tt.run(t, model)
				})
			}
		})

		t.Run("ModelPickerBehavior", func(t *testing.T) {
			t.Parallel()

			twoModelCatalog := func() codersdk.ChatModelsResponse {
				return codersdk.ChatModelsResponse{
					Providers: []codersdk.ChatModelProvider{{
						Provider:  "OpenAI",
						Available: true,
						Models: []codersdk.ChatModel{
							{ID: uuid.NewString(), Model: "gpt-4o", DisplayName: "GPT-4o"},
							{ID: uuid.NewString(), Model: "gpt-4.1", DisplayName: "GPT-4.1"},
						},
					}},
				}
			}

			tests := []struct {
				name string
				run  func(t *testing.T, model expChatsTUIModel)
			}{
				{
					name: "CursorBoundsCheck",
					run: func(t *testing.T, model expChatsTUIModel) {
						t.Helper()

						updatedModel, cmd := model.Update(modelsListedMsg{catalog: twoModelCatalog()})
						updated := mustTUIModel(t, updatedModel, cmd)

						updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
						updated = mustTUIModel(t, updatedModel, cmd)

						for range 4 {
							updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
							updated = mustTUIModel(t, updatedModel, cmd)
						}

						require.Equal(t, 1, updated.chat.modelPickerCursor)
					},
				},
				{
					name: "EmptyCatalog",
					run: func(t *testing.T, model expChatsTUIModel) {
						t.Helper()

						updatedModel, cmd := model.Update(modelsListedMsg{catalog: codersdk.ChatModelsResponse{}})
						updated := mustTUIModel(t, updatedModel, cmd)

						updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
						updated = mustTUIModel(t, updatedModel, cmd)

						require.Equal(t, overlayModelPicker, updated.overlay)
						require.NotPanics(t, func() {
							output := plainText(updated.View())
							require.Contains(t, output, "Select Model")
							require.Contains(t, output, "No models available.")
						})
					},
				},
				{
					name: "ReOpenPreservesCursor",
					run: func(t *testing.T, model expChatsTUIModel) {
						t.Helper()

						updatedModel, cmd := model.Update(modelsListedMsg{catalog: twoModelCatalog()})
						updated := mustTUIModel(t, updatedModel, cmd)

						updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
						updated = mustTUIModel(t, updatedModel, cmd)

						updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
						updated = mustTUIModel(t, updatedModel, cmd)
						require.Equal(t, 1, updated.chat.modelPickerCursor)

						updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
						updated = mustTUIModel(t, updatedModel, cmd)
						require.Equal(t, overlayNone, updated.overlay)

						updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
						updated = mustTUIModel(t, updatedModel, cmd)
						require.Equal(t, overlayModelPicker, updated.overlay)
						require.Equal(t, 1, updated.chat.modelPickerCursor)
					},
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
					tt.run(t, model)
				})
			}
		})

		t.Run("DiffDrawerLoadingState", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat.chat = &chat

			updatedModel, cmd := model.Update(toggleDiffDrawerMsg{})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, overlayDiffDrawer, updated.overlay)
			require.Len(t, mustBatchMsg(t, cmd), 2)
			require.Contains(t, plainText(updated.View()), "Loading diff")
		})

		t.Run("DiffDrawerErrorState", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat
			model.width = 80
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat.chat = &chat

			updatedModel, cmd := model.Update(toggleDiffDrawerMsg{})
			updated, _ := mustTUIModelWithCmd(t, updatedModel, cmd)

			updatedModel, cmd = updated.Update(gitChangesMsg{err: xerrors.New("connection refused")})
			updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Contains(t, plainText(updated.View()), "connection refused")
		})

		t.Run("OverlayDismissedOnViewSwitch", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat
			model.overlay = overlayModelPicker

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated := mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, viewChat, updated.currentView)
			require.Equal(t, overlayNone, updated.overlay)

			updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated, cmd = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewList, updated.currentView)
			require.Equal(t, overlayNone, updated.overlay)
			require.True(t, updated.list.loading)
			require.NotNil(t, cmd)
		})

		t.Run("OverlaysMutuallyExclusive", func(t *testing.T) {
			t.Parallel()

			catalog := codersdk.ChatModelsResponse{
				Providers: []codersdk.ChatModelProvider{{
					Provider:  "provider",
					Available: true,
					Models: []codersdk.ChatModel{{
						ID:          uuid.New().String(),
						Provider:    "provider",
						Model:       "model-a",
						DisplayName: "Model A",
					}},
				}},
			}

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat
			model.overlay = overlayModelPicker
			model.catalog = &catalog
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat.chat = &chat
			model.chat.gitChanges = []codersdk.ChatGitChange{}

			updatedModel, cmd := model.Update(toggleDiffDrawerMsg{})
			updated, _ := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, overlayDiffDrawer, updated.overlay)

			updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
			updated = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, overlayModelPicker, updated.overlay)
		})

		t.Run("OverlayBlocksViewKeys", func(t *testing.T) {
			t.Parallel()

			catalog := codersdk.ChatModelsResponse{
				Providers: []codersdk.ChatModelProvider{{
					Provider:  "provider",
					Available: true,
					Models: []codersdk.ChatModel{{
						ID:          uuid.New().String(),
						Provider:    "provider",
						Model:       "model-a",
						DisplayName: "Model A",
					}},
				}},
			}

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat
			model.catalog = &catalog
			model.chat.modelPickerFlat = catalog.Providers[0].Models

			updatedModel, cmd := model.Update(toggleModelPickerMsg{})
			updated := mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, overlayModelPicker, updated.overlay)

			updatedModel, cmd = updated.Update(keyRunes("n"))
			updated = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, viewChat, updated.currentView)
			require.Equal(t, overlayModelPicker, updated.overlay)
			require.False(t, updated.chat.draft)
		})

		t.Run("RapidViewSwitching", func(t *testing.T) {
			t.Parallel()

			firstChatID := uuid.New()
			secondChatID := uuid.New()
			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.width = 100
			model.height = 40

			updatedModel, cmd := model.Update(openSelectedChatMsg{chatID: firstChatID})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewChat, updated.currentView)
			require.True(t, updated.chat.loading)
			require.Nil(t, updated.chat.chat)
			require.Empty(t, updated.chat.messages)
			require.Len(t, mustBatchMsg(t, cmd), 3)

			firstChat := testChat(codersdk.ChatStatusCompleted)
			firstChat.ID = firstChatID
			updated.chat.chat = &firstChat
			updated.chat.loading = false
			updated.chat.messages = []codersdk.ChatMessage{testMessage(1, codersdk.ChatMessageRoleUser, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "from chat A"})}
			updated.chat.composer.SetValue("stale draft")

			updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewList, updated.currentView)
			require.True(t, updated.list.loading)

			updatedModel, cmd = updated.Update(openSelectedChatMsg{chatID: secondChatID})
			updated, cmd = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewChat, updated.currentView)
			require.True(t, updated.chat.loading)
			require.Nil(t, updated.chat.chat)
			require.Empty(t, updated.chat.messages)
			require.Empty(t, updated.chat.composer.Value())
			require.False(t, updated.chat.draft)
			require.Len(t, mustBatchMsg(t, cmd), 3)

			updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewList, updated.currentView)
			require.True(t, updated.list.loading)
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
			require.NotNil(t, cmd)
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

		t.Run("ErrorThenRetrySucceeds", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name string
				run  func(t *testing.T)
			}{
				{"ChatOpened", func(t *testing.T) {
					model := newTestChatViewModel(nil)

					updated, cmd := model.Update(chatOpenedMsg{err: xerrors.New("open failed")})
					require.Nil(t, cmd)
					require.NotNil(t, updated.err)

					chat := testChat(codersdk.ChatStatusRunning)
					updated, cmd = updated.Update(chatOpenedMsg{chat: chat})
					require.NotNil(t, cmd)
					require.NotNil(t, updated.chat)
					require.Equal(t, chat.ID, updated.chat.ID)
					require.Nil(t, updated.err)
				}},
				{"History", func(t *testing.T) {
					model := newTestChatViewModel(nil)

					updated, cmd := model.Update(chatHistoryMsg{err: xerrors.New("history failed")})
					require.Nil(t, cmd)
					require.NotNil(t, updated.err)

					messages := []codersdk.ChatMessage{
						testMessage(1, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "recovered"}),
					}
					updated, cmd = updated.Update(chatHistoryMsg{messages: messages})
					require.Nil(t, cmd)
					require.Equal(t, messages, updated.messages)
					require.Len(t, updated.blocks, 1)
					require.Nil(t, updated.err)
				}},
				{"Send", func(t *testing.T) {
					model := newTestChatViewModel(failingExperimentalClient())
					model.loading = false
					chat := testChat(codersdk.ChatStatusCompleted)
					model.chat = &chat
					model.chatStatus = chat.Status
					model.composer.SetValue("keep me")

					updated, cmd := model.Update(messageSentMsg{err: xerrors.New("send failed")})
					require.Nil(t, cmd)
					require.Equal(t, "keep me", updated.composer.Value())
					require.Contains(t, updated.View(), "send failed")

					updated.composer.SetValue("retry me")
					retried, retryCmd := updated.sendMessage()
					require.NotNil(t, retryCmd)
					require.True(t, retried.autoFollow)
					require.Empty(t, retried.composer.Value())
					_, ok := mustMsg(t, retryCmd).(messageSentMsg)
					require.True(t, ok)
				}},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					tt.run(t)
				})
			}
		})

		t.Run("ChatHistoryEdgeCases", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name    string
				input   []codersdk.ChatMessage
				wantNil bool
			}{
				{"NilMessages", nil, true},
				{"EmptyMessages", []codersdk.ChatMessage{}, false},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newTestChatViewModel(nil)
					model.messages = []codersdk.ChatMessage{
						testMessage(1, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "existing"}),
					}
					model.rebuildBlocks()
					require.Len(t, model.blocks, 1)

					var updated chatViewModel
					require.NotPanics(t, func() {
						updated, _ = model.Update(chatHistoryMsg{messages: tt.input})
					})
					if tt.wantNil {
						require.Nil(t, updated.messages)
					} else {
						require.NotNil(t, updated.messages)
						require.Empty(t, updated.messages)
					}
					require.Empty(t, updated.blocks)
				})
			}
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
			require.NotNil(t, cmd)
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
			require.NotNil(t, cmd)
			require.True(t, updated.reconnecting)
		})

		t.Run("StreamEventWithNilPartIsIgnored", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.messages = []codersdk.ChatMessage{
				testMessage(1, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "existing"}),
			}
			model.rebuildBlocks()

			updated, cmd := model.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:        codersdk.ChatStreamEventTypeMessagePart,
				MessagePart: nil,
			}})
			require.Nil(t, cmd)
			require.Equal(t, model.messages, updated.messages)
			require.Equal(t, model.blocks, updated.blocks)
			require.False(t, updated.accumulator.isPending())
		})

		t.Run("StreamEventErrorShowsInView", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
			model.loading = false
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat = &chat
			model.chatStatus = chat.Status
			model.messages = []codersdk.ChatMessage{
				testMessage(1, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "existing response"}),
			}
			model.rebuildBlocks()

			updated, cmd := model.Update(chatStreamEventMsg{err: xerrors.New("websocket closed")})
			require.Nil(t, cmd)
			require.NotNil(t, updated.err)

			view := plainText(updated.View())
			require.Contains(t, view, chat.Title)
			require.Contains(t, view, "existing response")
			require.Contains(t, view, "websocket closed")
			require.Contains(t, view, "Type a message")
			require.Contains(t, view, "esc: back")
		})

		t.Run("LoadingViewKeepsChatChrome", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})

			view := plainText(model.View())
			require.Contains(t, view, "New Chat (draft)")
			require.Contains(t, view, "Loading chat...")
			require.Contains(t, view, "Type a message")
			require.Contains(t, view, "esc: back")
		})

		t.Run("MultipleStreamErrorsOnlyShowLatest", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
			model.loading = false

			updated, _ := model.Update(chatStreamEventMsg{err: xerrors.New("first error")})
			updated, cmd := updated.Update(chatStreamEventMsg{err: xerrors.New("second error")})
			require.Nil(t, cmd)
			require.NotNil(t, updated.err)
			view := updated.View()
			require.Contains(t, view, "second error")
			require.NotContains(t, view, "first error")
		})

		t.Run("MessageDeduplicationWithAccumulatorPending", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)

			updated, _ := model.Update(chatStreamEventMsg{event: testTextPartEvent("partial")})
			message := testMessage(12, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "final"})

			updated, cmd := updated.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:    codersdk.ChatStreamEventTypeMessage,
				Message: &message,
			}})
			require.Nil(t, cmd)
			require.Len(t, updated.messages, 1)
			require.False(t, updated.accumulator.isPending())
			require.Len(t, updated.blocks, 1)
			require.Equal(t, "final", updated.blocks[0].text)
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

		t.Run("CreateChatErrorAllowsRetry", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(failingExperimentalClient())
			model.loading = false
			model.draft = true
			model.composer.SetValue("keep draft")

			updated, cmd := model.Update(chatCreatedMsg{err: xerrors.New("create failed")})
			require.Nil(t, cmd)
			require.True(t, updated.draft)
			require.Contains(t, updated.View(), "create failed")

			updated.composer.SetValue("retry draft")
			retried, retryCmd := updated.sendMessage()
			require.NotNil(t, retryCmd)
			require.True(t, retried.draft)
			require.Empty(t, retried.composer.Value())
			_, ok := mustMsg(t, retryCmd).(chatCreatedMsg)
			require.True(t, ok)
		})

		t.Run("BlankComposerDoesNotSend", func(t *testing.T) {
			t.Parallel()

			for _, value := range []string{"", "   "} {
				model := newTestChatViewModel(nil)
				model.composer.SetValue(value)

				updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
				require.Nil(t, cmd)
				require.Equal(t, value, updated.composer.Value())
			}
		})

		t.Run("SendClearsComposerText", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.draft = true
			model.composer.SetValue("hello")

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			require.NotNil(t, cmd)
			require.Empty(t, updated.composer.Value())
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

		t.Run("DoubleInterruptIsNoOp", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(failingExperimentalClient())
			chat := testChat(codersdk.ChatStatusRunning)
			model.chat = &chat
			model.chatStatus = codersdk.ChatStatusRunning
			model.interrupting = true

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlI})
			require.Nil(t, cmd)
			require.True(t, updated.interrupting)
		})

		t.Run("InterruptOnIdleChatIsNoOp", func(t *testing.T) {
			t.Parallel()

			// This also covers Ctrl-I on a completed chat.
			model := newTestChatViewModel(failingExperimentalClient())
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat = &chat
			model.chatStatus = codersdk.ChatStatusCompleted

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlI})
			require.Nil(t, cmd)
			require.False(t, updated.interrupting)
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

		t.Run("CtrlPSendsToggleModelPickerMsg", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name            string
				composerFocused bool
				composerValue   string
			}{
				{name: "Unfocused", composerFocused: false},
				{name: "Focused", composerFocused: true, composerValue: "draft"},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newTestChatViewModel(nil)
					model.composerFocused = tt.composerFocused
					if tt.composerValue != "" {
						model.composer.SetValue(tt.composerValue)
					}

					updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
					require.NotNil(t, cmd)
					require.Equal(t, tt.composerFocused, updated.composerFocused)
					if tt.composerValue != "" {
						require.Equal(t, tt.composerValue, updated.composer.Value())
					}
					_, ok := mustMsg(t, cmd).(toggleModelPickerMsg)
					require.True(t, ok)
				})
			}
		})

		t.Run("CtrlDSendsToggleDiffDrawerMsg", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name            string
				composerFocused bool
				composerValue   string
			}{
				{name: "Focused", composerFocused: true, composerValue: "draft"},
				{name: "Unfocused", composerFocused: false},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newTestChatViewModel(nil)
					model.composerFocused = tt.composerFocused
					if tt.composerValue != "" {
						model.composer.SetValue(tt.composerValue)
					}

					updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
					require.NotNil(t, cmd)
					require.Equal(t, tt.composerFocused, updated.composerFocused)
					if tt.composerValue != "" {
						require.Equal(t, tt.composerValue, updated.composer.Value())
					}
					_, ok := mustMsg(t, cmd).(toggleDiffDrawerMsg)
					require.True(t, ok)
				})
			}
		})

		t.Run("CtrlPFromListViewDoesNotOpenModelPicker", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewList
			model.list.loading = false

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
			updated := mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, viewList, updated.currentView)
			require.Equal(t, overlayNone, updated.overlay)
		})
	})

	t.Run("ChatView/Navigation", func(t *testing.T) {
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

		newScrollableModel := func(t *testing.T) chatViewModel {
			t.Helper()

			model := newTestChatViewModel(nil)
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
			model.messages = overflowingMessages(10)
			model.rebuildBlocks()
			model.composerFocused = false
			model.autoFollow = true
			(&model).syncViewportContent()
			model.viewport.GotoBottom()
			return model
		}

		t.Run("PageKeysScrollHalfViewport", func(t *testing.T) {
			t.Parallel()

			model := newScrollableModel(t)
			before := model.viewport.YOffset
			halfView := model.viewport.Height / 2

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
			require.False(t, updated.autoFollow)
			require.InDelta(t, float64(before-halfView), float64(updated.viewport.YOffset), 1)

			before = updated.viewport.YOffset
			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyPgDown})
			require.InDelta(t, float64(before+halfView), float64(updated.viewport.YOffset), 1)
		})

		t.Run("HomeEndJumpToEdges", func(t *testing.T) {
			t.Parallel()

			model := newScrollableModel(t)

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyHome})
			require.Equal(t, 0, updated.viewport.YOffset)
			require.False(t, updated.autoFollow)

			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnd})
			require.True(t, updated.viewport.AtBottom())
			require.True(t, updated.autoFollow)
		})

		t.Run("RunningChatShowsSpinnerIndicator", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.width = 80
			model.accumulator = streamAccumulator{pending: true, role: codersdk.ChatMessageRoleAssistant}

			(&model).syncViewportContent()
			require.Contains(t, model.lastTranscript, "Thinking")
		})

		t.Run("ReconnectingChatShowsReconnectIndicator", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.width = 80
			model.reconnecting = true

			(&model).syncViewportContent()
			require.Contains(t, model.lastTranscript, "Reconnecting")
		})

		t.Run("FinalizedHistorySuppressesPendingToolDuplicate", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.messages = []codersdk.ChatMessage{testMessage(
				1,
				codersdk.ChatMessageRoleAssistant,
				codersdk.ChatMessagePart{
					Type:       codersdk.ChatMessagePartTypeToolCall,
					ToolCallID: "tool-1",
					ToolName:   "search",
					Args:       json.RawMessage(`{"q":"hello"}`),
				},
				codersdk.ChatMessagePart{
					Type:       codersdk.ChatMessagePartTypeToolResult,
					ToolCallID: "tool-1",
					ToolName:   "search",
					Result:     json.RawMessage(`{"ok":true}`),
				},
			)}
			model.accumulator = streamAccumulator{
				pending: true,
				role:    codersdk.ChatMessageRoleAssistant,
				parts: []codersdk.ChatMessagePart{
					{
						Type:       codersdk.ChatMessagePartTypeToolCall,
						ToolCallID: "tool-1",
						ToolName:   "search",
						Args:       json.RawMessage(`{"q":"hello"}`),
					},
					{
						Type:       codersdk.ChatMessagePartTypeToolResult,
						ToolCallID: "tool-1",
						ToolName:   "search",
						Result:     json.RawMessage(`{"ok":true}`),
					},
				},
			}

			model.rebuildBlocks()
			require.Len(t, model.blocks, 1)
			require.Equal(t, blockToolResult, model.blocks[0].kind)
			require.Equal(t, "tool-1", model.blocks[0].toolID)
		})
	})

	t.Run("ChatView/ViewportScrolling", func(t *testing.T) {
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
						Text: fmt.Sprintf(
							"message %d %s",
							i+1,
							strings.Repeat("content ", 18),
						),
					},
				))
			}
			return messages
		}

		applyWindowSize := func(
			t *testing.T,
			model expChatsTUIModel,
			width int,
			height int,
		) expChatsTUIModel {
			t.Helper()

			updatedModel, cmd := model.Update(
				tea.WindowSizeMsg{Width: width, Height: height},
			)
			return mustTUIModel(t, updatedModel, cmd)
		}

		newScrollableModel := func(t *testing.T) chatViewModel {
			t.Helper()

			model := newTestChatViewModel(nil)
			model.loading = false
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat = &chat
			model.chatStatus = chat.Status
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
			model.messages = overflowingMessages(24)
			model.rebuildBlocks()

			var cmd tea.Cmd
			model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyTab})
			require.Nil(t, cmd)
			require.False(t, model.composerFocused)
			require.True(t, model.autoFollow)
			require.True(t, model.viewport.AtBottom())
			require.Greater(t, model.viewport.YOffset, 0)
			return model
		}

		streamMessage := func(id int64) chatStreamEventMsg {
			message := testMessage(
				id,
				codersdk.ChatMessageRoleAssistant,
				codersdk.ChatMessagePart{
					Type: codersdk.ChatMessagePartTypeText,
					Text: strings.Repeat("new content ", 24),
				},
			)
			return chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:    codersdk.ChatStreamEventTypeMessage,
				Message: &message,
			}}
		}

		t.Run("ViewportHeightCalculation", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name       string
				height     int
				assertions func(t *testing.T, model expChatsTUIModel)
			}{
				{"Standard", 40, func(t *testing.T, model expChatsTUIModel) {
					require.Equal(t, 31, model.chat.viewport.Height)
				}},
				{"MinimumZero", 5, func(t *testing.T, model expChatsTUIModel) {
					require.GreaterOrEqual(t, model.chat.viewport.Height, 0)
					require.Equal(t, 0, model.chat.viewport.Height)
				}},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					var model expChatsTUIModel
					require.NotPanics(t, func() {
						model = applyWindowSize(
							t,
							newExpChatsTUIModel(context.Background(), nil, nil, nil, nil),
							80,
							tt.height,
						)
					})
					tt.assertions(t, model)
				})
			}
		})

		t.Run("ViewportHeightAccountsForRootTitle", func(t *testing.T) {
			t.Parallel()

			model := applyWindowSize(
				t,
				newExpChatsTUIModel(context.Background(), nil, nil, nil, nil),
				80,
				40,
			)

			require.Equal(t, 39, model.chat.height)
		})

		t.Run("ScrollNavigation", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name   string
				setup  func(t *testing.T, m chatViewModel) chatViewModel
				key    tea.KeyType
				assert func(t *testing.T, before chatViewModel, after chatViewModel)
			}{
				{"ScrollUpDecreasesYOffset", nil, tea.KeyUp, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.False(t, after.autoFollow)
					require.Less(t, after.viewport.YOffset, before.viewport.YOffset)
				}},
				{"ScrollDownIncreasesYOffset", func(t *testing.T, m chatViewModel) chatViewModel {
					updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
					return updated
				}, tea.KeyDown, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.Greater(t, after.viewport.YOffset, before.viewport.YOffset)
				}},
				{"ScrollUpAtTopIsNoOp", func(t *testing.T, m chatViewModel) chatViewModel {
					updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyHome})
					return updated
				}, tea.KeyUp, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.Equal(t, 0, before.viewport.YOffset)
					require.Equal(t, before.viewport.YOffset, after.viewport.YOffset)
				}},
				{"ScrollDownAtBottomReEnablesAutoFollow", func(t *testing.T, m chatViewModel) chatViewModel {
					updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
					require.False(t, updated.autoFollow)
					return updated
				}, tea.KeyDown, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.False(t, before.viewport.AtBottom())
					require.True(t, after.viewport.AtBottom())
					require.True(t, after.autoFollow)
				}},
				{"PageUpScrollsHalfViewport", nil, tea.KeyPgUp, func(t *testing.T, before chatViewModel, after chatViewModel) {
					halfView := before.viewport.Height / 2
					require.False(t, after.autoFollow)
					require.InDelta(t, float64(before.viewport.YOffset-halfView), float64(after.viewport.YOffset), 1)
				}},
				{"PageDownScrollsHalfViewport", func(t *testing.T, m chatViewModel) chatViewModel {
					updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
					return updated
				}, tea.KeyPgDown, func(t *testing.T, before chatViewModel, after chatViewModel) {
					halfView := before.viewport.Height / 2
					require.InDelta(t, float64(before.viewport.YOffset+halfView), float64(after.viewport.YOffset), 1)
				}},
				{"HomeJumpsToTop", nil, tea.KeyHome, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.NotEqual(t, 0, before.viewport.YOffset)
					require.Equal(t, 0, after.viewport.YOffset)
					require.False(t, after.autoFollow)
				}},
				{"EndJumpsToBottomAndEnablesAutoFollow", func(t *testing.T, m chatViewModel) chatViewModel {
					updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyHome})
					return updated
				}, tea.KeyEnd, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.False(t, before.viewport.AtBottom())
					require.True(t, after.viewport.AtBottom())
					require.True(t, after.autoFollow)
				}},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newScrollableModel(t)
					if tt.setup != nil {
						model = tt.setup(t, model)
					}
					before := model

					after, _ := model.Update(tea.KeyMsg{Type: tt.key})
					tt.assert(t, before, after)
				})
			}
		})

		t.Run("SetContentPreservesScrollPosition", func(t *testing.T) {
			t.Parallel()

			model := newScrollableModel(t)
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
			before := model.viewport.YOffset

			updated, _ := model.Update(streamMessage(1001))
			require.False(t, updated.autoFollow)
			require.Equal(t, before, updated.viewport.YOffset)
		})

		t.Run("NewMessageAutoFollowsWhenAtBottom", func(t *testing.T) {
			t.Parallel()

			model := newScrollableModel(t)
			before := model.viewport.YOffset

			updated, _ := model.Update(streamMessage(1002))
			require.True(t, updated.autoFollow)
			require.True(t, updated.viewport.AtBottom())
			require.GreaterOrEqual(t, updated.viewport.YOffset, before)
		})

		t.Run("NewMessageDoesNotAutoFollowWhenScrolledUp", func(t *testing.T) {
			t.Parallel()

			model := newScrollableModel(t)
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
			before := model.viewport.YOffset

			updated, _ := model.Update(streamMessage(1003))
			require.False(t, updated.autoFollow)
			require.False(t, updated.viewport.AtBottom())
			require.Equal(t, before, updated.viewport.YOffset)
		})

		t.Run("TotalViewHeightDoesNotExceedTerminalHeight", func(t *testing.T) {
			t.Parallel()

			model := applyWindowSize(
				t,
				newExpChatsTUIModel(context.Background(), nil, nil, nil, nil),
				80,
				40,
			)
			model.currentView = viewChat
			model.chat.loading = false
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat.chat = &chat
			model.chat.chatStatus = chat.Status
			model.chat.messages = overflowingMessages(24)
			model.chat.rebuildBlocks()

			view := model.View()
			lineCount := strings.Count(view, "\n") + 1
			require.LessOrEqual(t, lineCount, 40)
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

	t.Run("ChatView/StatePersistence", func(t *testing.T) {
		t.Parallel()

		t.Run("StateSurvivesTransition", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name string
				run  func(t *testing.T)
			}{
				{"ComposerTextOverlayToggle", func(t *testing.T) {
					catalog := codersdk.ChatModelsResponse{
						Providers: []codersdk.ChatModelProvider{{
							Provider:  "provider",
							Available: true,
							Models: []codersdk.ChatModel{{
								ID:          uuid.New().String(),
								Provider:    "provider",
								Model:       "model-a",
								DisplayName: "Model A",
							}},
						}},
					}

					model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
					model.currentView = viewChat
					model.catalog = &catalog
					model.chat.modelPickerFlat = catalog.Providers[0].Models
					model.chat.loading = false
					model.chat.composer.SetValue("keep this draft")

					updatedModel, cmd := model.Update(toggleModelPickerMsg{})
					updated := mustTUIModel(t, updatedModel, cmd)
					require.Equal(t, "keep this draft", updated.chat.composer.Value())

					updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
					updated = mustTUIModel(t, updatedModel, cmd)
					require.Equal(t, "keep this draft", updated.chat.composer.Value())
				}},
				{"ComposerTextFocusSwitch", func(t *testing.T) {
					model := newTestChatViewModel(nil)
					model.composer.SetValue("keep this draft")

					updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
					require.False(t, updated.composerFocused)
					require.Equal(t, "keep this draft", updated.composer.Value())

					updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab})
					require.True(t, updated.composerFocused)
					require.Equal(t, "keep this draft", updated.composer.Value())
				}},
				{"ViewportScrollOverlayToggle", func(t *testing.T) {
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

					model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
					model.currentView = viewChat
					updatedModel, cmd := model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
					updated := mustTUIModel(t, updatedModel, cmd)
					updated.chat.loading = false
					chat := testChat(codersdk.ChatStatusCompleted)
					updated.chat.chat = &chat
					updated.chat.messages = overflowingMessages(10)
					updated.chat.gitChanges = []codersdk.ChatGitChange{}
					diff := codersdk.ChatDiffContents{ChatID: chat.ID, Diff: "diff --git a/file b/file"}
					updated.chat.diffContents = &diff
					updated.chat.rebuildBlocks()
					updated.chat.composerFocused = false
					updated.chat.autoFollow = true
					(&updated.chat).syncViewportContent()
					updated.chat.viewport.GotoBottom()

					updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyUp})
					updated = mustTUIModel(t, updatedModel, cmd)
					require.False(t, updated.chat.viewport.AtBottom())
					yBefore := updated.chat.viewport.YOffset

					updatedModel, cmd = updated.Update(toggleDiffDrawerMsg{})
					updated = mustTUIModel(t, updatedModel, cmd)
					require.Equal(t, overlayDiffDrawer, updated.overlay)
					require.Equal(t, yBefore, updated.chat.viewport.YOffset)

					updatedModel, cmd = updated.Update(toggleDiffDrawerMsg{})
					updated = mustTUIModel(t, updatedModel, cmd)
					require.Equal(t, overlayNone, updated.overlay)
					require.Equal(t, yBefore, updated.chat.viewport.YOffset)
				}},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					tt.run(t)
				})
			}
		})

		t.Run("SelectedModelSurvivesPickerReopen", func(t *testing.T) {
			t.Parallel()

			firstModelID := uuid.New()
			secondModelID := uuid.New()
			catalog := codersdk.ChatModelsResponse{
				Providers: []codersdk.ChatModelProvider{{
					Provider:  "provider",
					Available: true,
					Models: []codersdk.ChatModel{
						{
							ID:          firstModelID.String(),
							Provider:    "provider",
							Model:       "model-a",
							DisplayName: "Model A",
						},
						{
							ID:          secondModelID.String(),
							Provider:    "provider",
							Model:       "model-b",
							DisplayName: "Model B",
						},
					},
				}},
			}

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat

			updatedModel, cmd := model.Update(modelsListedMsg{catalog: catalog})
			updated := mustTUIModel(t, updatedModel, cmd)

			updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
			updated = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, overlayModelPicker, updated.overlay)

			updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
			updated = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, 1, updated.chat.modelPickerCursor)

			updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
			updated = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, overlayNone, updated.overlay)
			require.NotNil(t, updated.chat.modelOverride)
			require.NotNil(t, updated.modelOverride)
			require.Equal(t, secondModelID, *updated.chat.modelOverride)
			require.Equal(t, secondModelID, *updated.modelOverride)

			updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
			updated = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, overlayModelPicker, updated.overlay)
			require.Equal(t, 1, updated.chat.modelPickerCursor)
			require.NotNil(t, updated.chat.modelOverride)
			require.Equal(t, secondModelID, *updated.chat.modelOverride)
		})
	})

	t.Run("ChatView/ChatLifecycle", func(t *testing.T) {
		t.Parallel()

		t.Run("DraftChatSwitchBackToListDoesNotCrash", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)

			updatedModel, cmd := model.Update(openDraftChatMsg{})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewChat, updated.currentView)
			require.True(t, updated.chat.draft)
			require.Nil(t, cmd)

			updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewList, updated.currentView)
			require.Equal(t, overlayNone, updated.overlay)
			require.True(t, updated.list.loading)
			require.False(t, updated.chat.streaming)
		})

		t.Run("StreamingChatSwitchBackToList", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat
			chat := testChat(codersdk.ChatStatusRunning)
			model.chat.chat = &chat
			model.chat.chatStatus = codersdk.ChatStatusRunning
			model.chat.streaming = true
			model.chat.streamCloser = io.NopCloser(strings.NewReader("stream"))

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewList, updated.currentView)
			require.True(t, updated.list.loading)
			require.False(t, updated.chat.streaming)
			require.Nil(t, updated.chat.streamCloser)
			require.NotNil(t, cmd)
		})

		t.Run("ReOpenSameChatAfterEsc", func(t *testing.T) {
			t.Parallel()

			chatID := uuid.New()
			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.width = 100
			model.height = 40

			updatedModel, cmd := model.Update(openSelectedChatMsg{chatID: chatID})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewChat, updated.currentView)
			require.Len(t, mustBatchMsg(t, cmd), 3)

			openedChat := testChat(codersdk.ChatStatusCompleted)
			openedChat.ID = chatID
			updated.chat.chat = &openedChat
			updated.chat.loading = false
			updated.chat.messages = []codersdk.ChatMessage{testMessage(1, codersdk.ChatMessageRoleUser, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "stale message"})}
			updated.chat.composer.SetValue("stale draft")

			updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewList, updated.currentView)
			require.True(t, updated.list.loading)

			updatedModel, cmd = updated.Update(openSelectedChatMsg{chatID: chatID})
			updated, cmd = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, viewChat, updated.currentView)
			require.True(t, updated.chat.loading)
			require.Nil(t, updated.chat.chat)
			require.Empty(t, updated.chat.messages)
			require.Empty(t, updated.chat.composer.Value())
			require.Len(t, mustBatchMsg(t, cmd), 3)
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

		t.Run("TranscriptRefreshRules", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name      string
				mutate    func(m *chatViewModel)
				expectNew bool
			}{
				{"RepeatedViewNoRefresh", func(m *chatViewModel) {}, false},
				{"BlockChange", func(m *chatViewModel) {
					m.blocks = append(m.blocks, chatBlock{kind: blockText, role: codersdk.ChatMessageRoleAssistant, text: "new block"})
				}, true},
				{"SelectionChange", func(m *chatViewModel) { m.selectedBlock = 1 }, true},
				{"WidthChange", func(m *chatViewModel) { m.width = 60 }, true},
				{"ComposerFocusChange", func(m *chatViewModel) { m.composerFocused = true }, true},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newTranscriptModel()
					(&model).syncViewportContent()
					firstTranscript := model.lastTranscript
					require.NotEmpty(t, firstTranscript)

					tt.mutate(&model)
					(&model).syncViewportContent()
					if tt.expectNew {
						require.NotEqual(t, firstTranscript, model.lastTranscript)
					} else {
						require.Equal(t, firstTranscript, model.lastTranscript)
					}
				})
			}
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

		t.Run("EmptyListDisplaysNoChatsMessage", func(t *testing.T) {
			t.Parallel()

			model := newChatListModel(newTUIStyles())

			updated, cmd := model.Update(chatsListedMsg{chats: []codersdk.Chat{}})
			require.Nil(t, cmd)
			require.Contains(t, plainText(updated.View()), "No chats yet")
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
