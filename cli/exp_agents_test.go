package cli //nolint:testpackage // Tests unexported chat TUI reducers.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

func TestExpAgents(t *testing.T) {
	t.Parallel()

	t.Run("ResolveModel", func(t *testing.T) {
		t.Parallel()

		catalog := codersdk.ChatModelsResponse{
			Providers: []codersdk.ChatModelProvider{{
				Provider:  "openai",
				Available: true,
				Models: []codersdk.ChatModel{{
					ID:          "openai:gpt-4o",
					Provider:    "openai",
					Model:       "gpt-4o",
					DisplayName: "GPT-4o",
				}},
			}},
		}

		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(rw).Encode(catalog)
		}))
		t.Cleanup(server.Close)

		serverURL, err := url.Parse(server.URL)
		require.NoError(t, err)

		client := codersdk.NewExperimentalClient(codersdk.New(serverURL))
		tests := []struct {
			name  string
			input string
			want  string
		}{
			{name: "ExactID", input: "openai:gpt-4o", want: "openai:gpt-4o"},
			{name: "ProviderModel", input: "openai/gpt-4o", want: "openai:gpt-4o"},
			{name: "DisplayName", input: "GPT-4o", want: "openai:gpt-4o"},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				resolved, err := resolveModel(context.Background(), client, tt.input)
				require.NoError(t, err)
				require.NotNil(t, resolved)
				require.Equal(t, tt.want, *resolved)
			})
		}
	})

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

		t.Run("EscFromChatViewAdvancesGeneration", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat
			model.overlay = overlayNone
			model.chatGeneration = 4
			model.chat.chatGeneration = 4

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, uint64(5), updated.chatGeneration)
			require.Equal(t, uint64(5), updated.chat.chatGeneration)
			require.True(t, updated.chat.matchesGeneration(updated.chatGeneration))
			require.NotNil(t, cmd)
		})

		t.Run("EscFromChatViewRejectsLateChatLoadMessages", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat
			model.overlay = overlayNone
			model.chatGeneration = 4
			model.chat.chatGeneration = 4
			model.chat.chat = &codersdk.Chat{ID: uuid.New(), Title: "current chat"}

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.NotNil(t, cmd)

			staleChat := codersdk.Chat{ID: uuid.New(), Title: "stale chat"}
			updatedModel, cmd = updated.Update(chatOpenedMsg{generation: 4, chatID: staleChat.ID, chat: staleChat})
			updated = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, "current chat", updated.chat.chat.Title)

			staleMessages := []codersdk.ChatMessage{testMessage(
				1,
				codersdk.ChatMessageRoleUser,
				codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "stale"},
			)}
			updatedModel, cmd = updated.Update(chatHistoryMsg{generation: 4, chatID: staleChat.ID, messages: staleMessages})
			updated = mustTUIModel(t, updatedModel, cmd)
			require.Empty(t, updated.chat.messages)
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
						require.True(t, updated.chat.metadataResolved)
						require.True(t, updated.chat.historyResolved)
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
						Provider:  "openai",
						Available: true,
						Models: []codersdk.ChatModel{
							{ID: "openai:gpt-4o", Provider: "openai", Model: "gpt-4o", DisplayName: "GPT-4o"},
							{ID: "openai:gpt-4.1", Provider: "openai", Model: "gpt-4.1", DisplayName: "GPT-4.1"},
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
					name: "LoadingPlaceholder",
					run: func(t *testing.T, model expChatsTUIModel) {
						t.Helper()

						model.currentView = viewChat
						model.width = 80
						model.height = 24

						updatedModel, cmd := model.Update(toggleModelPickerMsg{})
						updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
						require.Equal(t, overlayModelPicker, updated.overlay)
						require.NotNil(t, cmd)
						require.Contains(t, plainText(updated.View()), "Loading models...")
					},
				},
				{
					name: "RebuildsFlatListFromCachedCatalog",
					run: func(t *testing.T, model expChatsTUIModel) {
						t.Helper()

						catalog := twoModelCatalog()
						model.currentView = viewChat
						model.catalog = &catalog
						model.chat.modelPickerFlat = nil

						updatedModel, cmd := model.Update(toggleModelPickerMsg{})
						updated := mustTUIModel(t, updatedModel, cmd)
						require.Equal(t, overlayModelPicker, updated.overlay)
						require.Len(t, updated.chat.modelPickerFlat, 2)
						require.Equal(t, catalog.Providers[0].Models, updated.chat.modelPickerFlat)
					},
				},
				{
					name: "LoadErrorClosesOverlay",
					run: func(t *testing.T, model expChatsTUIModel) {
						t.Helper()

						model.currentView = viewChat
						model.width = 80
						model.height = 24

						updatedModel, cmd := model.Update(toggleModelPickerMsg{})
						updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
						require.Equal(t, overlayModelPicker, updated.overlay)
						require.NotNil(t, cmd)
						require.Contains(t, plainText(updated.View()), "Loading models...")

						updatedModel, cmd = updated.Update(modelsListedMsg{err: xerrors.New("model discovery failed")})
						updated = mustTUIModel(t, updatedModel, cmd)
						require.Equal(t, overlayNone, updated.overlay)
						require.NotContains(t, plainText(updated.View()), "Loading models...")
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

		t.Run("DiffDrawerNoOpWithoutActiveChat", func(t *testing.T) {
			t.Parallel()

			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
			model.currentView = viewChat
			model.chat.draft = true
			model.chat.loading = false

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			msg, ok := mustMsg(t, cmd).(toggleDiffDrawerMsg)
			require.True(t, ok)

			updatedModel, cmd = updated.Update(msg)
			updated = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, overlayNone, updated.overlay)
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
			model.loading = true
			model.metadataResolved = false
			model.historyResolved = true
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
			model.loading = true
			model.metadataResolved = false
			model.historyResolved = true

			updated, cmd := model.Update(chatOpenedMsg{err: xerrors.New("open failed")})
			require.Nil(t, cmd)
			require.NotNil(t, updated.err)
			require.Equal(t, "open failed", updated.err.Error())
			require.False(t, updated.loading)
		})

		t.Run("ChatHistoryStoresMessagesRebuildsBlocksAndTracksLastUsage", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.loading = true
			model.metadataResolved = true
			model.historyResolved = false
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
			model.loading = true
			model.metadataResolved = true
			model.historyResolved = false

			updated, cmd := model.Update(chatHistoryMsg{err: xerrors.New("history failed")})
			require.Nil(t, cmd)
			require.NotNil(t, updated.err)
			require.Equal(t, "history failed", updated.err.Error())
			require.False(t, updated.loading)
		})

		t.Run("OpenHistoryReadiness", func(t *testing.T) {
			t.Parallel()

			t.Run("OpenFailsThenHistorySucceeds", func(t *testing.T) {
				t.Parallel()

				model := newTestChatViewModel(nil)
				model.loading = true
				model.metadataResolved = false
				model.historyResolved = false

				model, _ = model.Update(chatOpenedMsg{err: xerrors.New("open failed")})
				require.True(t, model.loading)
				require.True(t, model.metadataResolved)
				require.False(t, model.historyResolved)

				model, _ = model.Update(chatHistoryMsg{messages: []codersdk.ChatMessage{
					testMessage(1, codersdk.ChatMessageRoleUser, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "hi"}),
				}})
				require.False(t, model.loading)
				require.NotNil(t, model.err)
				require.Equal(t, "open failed", model.err.Error())
				require.Len(t, model.messages, 1)
			})

			t.Run("HistoryFailsThenOpenSucceeds", func(t *testing.T) {
				t.Parallel()

				model := newTestChatViewModel(nil)
				model.loading = true
				model.metadataResolved = false
				model.historyResolved = false

				model, _ = model.Update(chatHistoryMsg{err: xerrors.New("history failed")})
				require.True(t, model.loading)
				require.False(t, model.metadataResolved)
				require.True(t, model.historyResolved)

				chat := testChat(codersdk.ChatStatusCompleted)
				model, _ = model.Update(chatOpenedMsg{chat: chat})
				require.False(t, model.loading)
				require.NotNil(t, model.err)
				require.Equal(t, "history failed", model.err.Error())
				require.NotNil(t, model.chat)
			})

			t.Run("BothSucceedOutOfOrder", func(t *testing.T) {
				t.Parallel()

				model := newTestChatViewModel(nil)
				model.loading = true
				model.metadataResolved = false
				model.historyResolved = false

				model, _ = model.Update(chatHistoryMsg{messages: []codersdk.ChatMessage{
					testMessage(1, codersdk.ChatMessageRoleUser, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "hi"}),
				}})
				require.True(t, model.loading)
				require.Nil(t, model.err)

				model.streaming = true
				chat := testChat(codersdk.ChatStatusCompleted)
				model, _ = model.Update(chatOpenedMsg{chat: chat})
				require.False(t, model.loading)
				require.Nil(t, model.err)
				require.NotNil(t, model.chat)
				require.Len(t, model.messages, 1)
			})

			t.Run("OpenThenEmptyHistoryStartsStream", func(t *testing.T) {
				t.Parallel()

				chat := testChat(codersdk.ChatStatusRunning)
				streamQueryCh := make(chan string, 1)
				streamErrCh := make(chan error, 1)
				server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
					wantPath := fmt.Sprintf("/api/experimental/chats/%s/stream", chat.ID)
					if req.URL.Path != wantPath {
						select {
						case streamErrCh <- xerrors.Errorf("stream path %q, want %q", req.URL.Path, wantPath):
						default:
						}
						rw.WriteHeader(http.StatusNotFound)
						return
					}

					conn, err := websocket.Accept(rw, req, nil)
					if err != nil {
						select {
						case streamErrCh <- err:
						default:
						}
						return
					}
					defer conn.Close(websocket.StatusNormalClosure, "")

					select {
					case streamQueryCh <- req.URL.RawQuery:
					default:
					}
				}))
				defer server.Close()

				serverURL, err := url.Parse(server.URL)
				require.NoError(t, err)

				model := newTestChatViewModel(codersdk.NewExperimentalClient(codersdk.New(serverURL)))
				model.loading = true
				model.metadataResolved = false
				model.historyResolved = false

				model, cmd := model.Update(chatOpenedMsg{chat: chat})
				require.NotNil(t, cmd)
				require.True(t, model.loading)
				require.False(t, model.streaming)

				updated, cmd := model.Update(chatHistoryMsg{messages: nil})
				defer updated.stopStream()
				require.NotNil(t, cmd)
				require.False(t, updated.loading)
				require.True(t, updated.streaming)
				require.NotNil(t, updated.streamCloser)
				require.NotNil(t, updated.streamEventCh)

				select {
				case err := <-streamErrCh:
					require.NoError(t, err)
				case query := <-streamQueryCh:
					require.Empty(t, query)
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for chat stream connection")
				}
			})

			t.Run("BothFail", func(t *testing.T) {
				t.Parallel()

				model := newTestChatViewModel(nil)
				model.loading = true
				model.metadataResolved = false
				model.historyResolved = false

				model, _ = model.Update(chatOpenedMsg{err: xerrors.New("open err")})
				require.True(t, model.loading)

				model, _ = model.Update(chatHistoryMsg{err: xerrors.New("history err")})
				require.False(t, model.loading)
				require.NotNil(t, model.err)
				require.Equal(t, "open err", model.err.Error())
			})
		})

		t.Run("StaleAsyncMessagesAreDropped", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name string
				msg  tea.Msg
			}{
				{
					name: "chatOpenedMsg",
					msg: chatOpenedMsg{
						chatID: uuid.New(),
						chat:   testChat(codersdk.ChatStatusCompleted),
					},
				},
				{
					name: "chatHistoryMsg",
					msg: chatHistoryMsg{
						chatID: uuid.New(),
						messages: []codersdk.ChatMessage{testMessage(
							1,
							codersdk.ChatMessageRoleUser,
							codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "hi"},
						)},
					},
				},
				{
					name: "gitChangesMsg",
					msg:  gitChangesMsg{chatID: uuid.New()},
				},
				{
					name: "diffContentsMsg",
					msg:  diffContentsMsg{chatID: uuid.New()},
				},
			}

			for _, tt := range tests {
				tt := tt
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newTestChatViewModel(nil)
					chat := testChat(codersdk.ChatStatusCompleted)
					model.setChat(chat)
					model.chatGeneration = 1
					model.loading = false

					before := model
					model, cmd := model.Update(tt.msg)
					require.Nil(t, cmd)
					require.Equal(t, before.loading, model.loading)
					require.Equal(t, before.messages, model.messages)
					require.Equal(t, before.err, model.err)
				})
			}
		})

		t.Run("WriteSideStaleSessionMessagesAreDropped", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name string
				msg  tea.Msg
			}{
				{
					name: "chatCreatedMsg",
					msg:  chatCreatedMsg{generation: 1, chat: testChat(codersdk.ChatStatusRunning)},
				},
				{
					name: "messageSentMsg",
					msg:  messageSentMsg{generation: 1, resp: codersdk.CreateChatMessageResponse{}},
				},
				{
					name: "chatInterruptedMsg",
					msg:  chatInterruptedMsg{generation: 1, chat: testChat(codersdk.ChatStatusCompleted)},
				},
			}

			for _, tt := range tests {
				tt := tt
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newTestChatViewModel(nil)
					model.chatGeneration = 2
					model.creatingChat = true
					model.interrupting = true
					chat := testChat(codersdk.ChatStatusCompleted)
					model.setChat(chat)
					model.loading = false
					model.pendingComposerText = "pending"
					model.composer.SetValue("current")

					before := model
					model, cmd := model.Update(tt.msg)
					require.Nil(t, cmd)
					require.Equal(t, before.loading, model.loading)
					require.Equal(t, before.chat, model.chat)
					require.Equal(t, before.draft, model.draft)
					require.Equal(t, before.err, model.err)
					require.Equal(t, before.pendingComposerText, model.pendingComposerText)
					require.Equal(t, before.creatingChat, model.creatingChat)
					require.Equal(t, before.interrupting, model.interrupting)
					require.Equal(t, before.queuedMessages, model.queuedMessages)
					require.Equal(t, before.composer.Value(), model.composer.Value())
				})
			}
		})

		t.Run("DraftSessionRejectsStaleTrafficByGeneration", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name string
				msg  tea.Msg
			}{
				{
					name: "chatOpenedMsg",
					msg:  chatOpenedMsg{generation: 1, chatID: uuid.New(), chat: testChat(codersdk.ChatStatusCompleted)},
				},
				{
					name: "chatHistoryMsg",
					msg: chatHistoryMsg{generation: 1, chatID: uuid.New(), messages: []codersdk.ChatMessage{testMessage(
						1,
						codersdk.ChatMessageRoleUser,
						codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "hi"},
					)}},
				},
				{
					name: "chatStreamEventMsg",
					msg:  chatStreamEventMsg{generation: 1, chatID: uuid.New(), event: testTextPartEvent("stale")},
				},
				{
					name: "gitChangesMsg",
					msg:  gitChangesMsg{generation: 1, chatID: uuid.New()},
				},
				{
					name: "diffContentsMsg",
					msg:  diffContentsMsg{generation: 1, chatID: uuid.New()},
				},
			}

			for _, tt := range tests {
				tt := tt
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newTestChatViewModel(nil)
					model.draft = true
					model.loading = false
					model.chatGeneration = 2
					model.pendingComposerText = "pending"
					model.composer.SetValue("draft text")

					before := model
					model, cmd := model.Update(tt.msg)
					require.Nil(t, cmd)
					require.Equal(t, before.loading, model.loading)
					require.Equal(t, before.messages, model.messages)
					require.Equal(t, before.err, model.err)
					require.Equal(t, before.chat, model.chat)
					require.Equal(t, before.pendingComposerText, model.pendingComposerText)
					require.Equal(t, before.composer.Value(), model.composer.Value())
					require.True(t, model.draft)
				})
			}
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
			model.activeChatID = chat.ID
			model.chatStatus = codersdk.ChatStatusWaiting

			updated, cmd := model.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:   codersdk.ChatStreamEventTypeStatus,
				ChatID: chat.ID,
				Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatusRunning},
			}})
			require.NotNil(t, cmd)
			require.Equal(t, codersdk.ChatStatusRunning, updated.chatStatus)
			require.NotNil(t, updated.chat)
			require.Equal(t, codersdk.ChatStatusRunning, updated.chat.Status)
		})

		t.Run("StatusFromDifferentChatIsIgnored", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			chat := testChat(codersdk.ChatStatusWaiting)
			model.chat = &chat
			model.activeChatID = chat.ID
			model.chatStatus = codersdk.ChatStatusWaiting

			updated, cmd := model.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:   codersdk.ChatStreamEventTypeStatus,
				ChatID: uuid.New(),
				Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatusRunning},
			}})
			require.Nil(t, cmd)
			require.Equal(t, codersdk.ChatStatusWaiting, updated.chatStatus)
			require.NotNil(t, updated.chat)
			require.Equal(t, codersdk.ChatStatusWaiting, updated.chat.Status)
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
			model.loading = true
			model.metadataResolved = false
			model.historyResolved = false
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

		t.Run("MessageDeduplication", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name string
				run  func(t *testing.T)
			}{
				{"AccumulatorPending", func(t *testing.T) {
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
				}},
				{"FinalizedHistorySuppressesPendingToolDuplicate", func(t *testing.T) {
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
				}},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					tt.run(t)
				})
			}
		})

		t.Run("StaleStreamEventsAreDroppedByGeneration", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			chat := testChat(codersdk.ChatStatusRunning)
			model.setChat(chat)
			model.chatGeneration = 1
			model.streaming = true

			staleMsg := chatStreamEventMsg{
				chatID: uuid.New(),
				event:  testTextPartEvent("should be ignored"),
			}

			updated, cmd := model.Update(staleMsg)
			require.Nil(t, cmd)
			require.Empty(t, updated.accumulator.parts)
			require.Equal(t, model.chatStatus, updated.chatStatus)
			require.Equal(t, model.blocks, updated.blocks)
		})

		t.Run("IntentionalCloseSkipsReconnect", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			chat := testChat(codersdk.ChatStatusRunning)
			model.setChat(chat)
			model.streaming = true

			model.stopStream()
			require.True(t, model.intentionalClose)

			eofMsg := chatStreamEventMsg{
				chatID: chat.ID,
				err:    io.EOF,
			}
			updated, cmd := model.Update(eofMsg)
			require.Nil(t, cmd)
			require.False(t, updated.streaming)
			require.False(t, updated.reconnecting)
			require.False(t, updated.intentionalClose)
			require.NoError(t, updated.err)
		})

		t.Run("EOFStopsStreamingAndAttemptsReconnectWhenInterruptible", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(failingExperimentalClient())
			chat := testChat(codersdk.ChatStatusPending)
			model.setChat(chat)
			model.streaming = true

			updated, cmd := model.Update(chatStreamEventMsg{chatID: chat.ID, err: io.EOF})
			require.Nil(t, cmd)
			require.False(t, updated.streaming)
			require.True(t, updated.reconnecting)
			require.NotNil(t, updated.err)
		})

		t.Run("EOFAttemptsReconnectWhenWaiting", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(failingExperimentalClient())
			chat := testChat(codersdk.ChatStatusWaiting)
			model.setChat(chat)
			model.streaming = true

			updated, cmd := model.Update(chatStreamEventMsg{chatID: chat.ID, err: io.EOF})
			require.NotNil(t, cmd)
			require.False(t, updated.streaming)
			require.True(t, updated.reconnecting)
			require.NotNil(t, updated.err)
		})

		t.Run("EOFReconnectClearsPendingAccumulator", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(failingExperimentalClient())
			chat := testChat(codersdk.ChatStatusPending)
			model.setChat(chat)
			model.messages = []codersdk.ChatMessage{
				testMessage(1, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "persisted"}),
			}
			model.accumulator = streamAccumulator{
				pending: true,
				role:    codersdk.ChatMessageRoleAssistant,
				parts: []codersdk.ChatMessagePart{{
					Type: codersdk.ChatMessagePartTypeText,
					Text: "partial",
				}},
			}
			model.rebuildBlocks()
			require.Len(t, model.blocks, 2)
			require.Equal(t, "partial", model.blocks[1].text)
			model.streaming = true

			updated, cmd := model.Update(chatStreamEventMsg{chatID: chat.ID, err: io.EOF})
			require.Nil(t, cmd)
			require.False(t, updated.streaming)
			require.True(t, updated.reconnecting)
			require.NotNil(t, updated.err)
			require.False(t, updated.accumulator.isPending())
			require.Nil(t, updated.accumulator.parts)
			require.Len(t, updated.blocks, 1)
			require.Equal(t, "persisted", updated.blocks[0].text)
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

		t.Run("DisconnectedSendRestartsStream", func(t *testing.T) {
			t.Parallel()

			chat := testChat(codersdk.ChatStatusCompleted)
			message := testMessage(22, codersdk.ChatMessageRoleUser, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "sent"})
			streamQueryCh := make(chan string, 1)
			streamErrCh := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				wantPath := fmt.Sprintf("/api/experimental/chats/%s/stream", chat.ID)
				if req.URL.Path != wantPath {
					select {
					case streamErrCh <- xerrors.Errorf("stream path %q, want %q", req.URL.Path, wantPath):
					default:
					}
					rw.WriteHeader(http.StatusNotFound)
					return
				}

				conn, err := websocket.Accept(rw, req, nil)
				if err != nil {
					select {
					case streamErrCh <- err:
					default:
					}
					return
				}
				defer conn.Close(websocket.StatusNormalClosure, "")

				select {
				case streamQueryCh <- req.URL.RawQuery:
				default:
				}
			}))
			defer server.Close()

			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)

			model := newTestChatViewModel(codersdk.NewExperimentalClient(codersdk.New(serverURL)))
			model.setChat(chat)

			updated, cmd := model.Update(messageSentMsg{resp: codersdk.CreateChatMessageResponse{Message: &message}})
			defer updated.stopStream()
			require.NotNil(t, cmd)
			require.True(t, updated.streaming)
			require.NotNil(t, updated.streamCloser)
			require.NotNil(t, updated.streamEventCh)
			require.Len(t, updated.messages, 1)

			select {
			case err := <-streamErrCh:
				require.NoError(t, err)
			case query := <-streamQueryCh:
				require.Equal(t, fmt.Sprintf("after_id=%d", message.ID), query)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for restarted chat stream connection")
			}
		})

		t.Run("ActiveStreamDoesNotReconnectOnSend", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			chat := testChat(codersdk.ChatStatusCompleted)
			message := testMessage(24, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "delivered"})
			model.setChat(chat)
			model.streaming = true

			updated, cmd := model.Update(messageSentMsg{resp: codersdk.CreateChatMessageResponse{Message: &message}})
			require.Nil(t, cmd)
			require.True(t, updated.streaming)
			require.Len(t, updated.messages, 1)
		})

		t.Run("SuccessfulSendClearsPreviousError", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.err = xerrors.New("stale send failed")
			message := testMessage(23, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "delivered"})

			updated, cmd := model.Update(messageSentMsg{resp: codersdk.CreateChatMessageResponse{Message: &message}})
			require.Nil(t, cmd)
			require.Nil(t, updated.err)
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

		t.Run("SendErrorRestoresComposer", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			chat := testChat(codersdk.ChatStatusCompleted)
			model.setChat(chat)
			model.loading = false
			model.composer.SetValue("my message")

			model, _ = model.sendMessage()
			require.Empty(t, model.composer.Value())
			require.Equal(t, "my message", model.pendingComposerText)

			model, _ = model.Update(messageSentMsg{err: xerrors.New("network error")})
			require.Equal(t, "my message", model.composer.Value())
			require.Error(t, model.err)
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

		t.Run("CreateErrorRestoresComposer", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.draft = true
			model.loading = false
			model.composer.SetValue("first message")

			model, _ = model.sendMessage()
			require.Empty(t, model.composer.Value())
			require.Equal(t, "first message", model.pendingComposerText)
			require.True(t, model.creatingChat)

			model, _ = model.Update(chatCreatedMsg{err: xerrors.New("create failed")})
			require.Equal(t, "first message", model.composer.Value())
			require.False(t, model.creatingChat)
			require.Error(t, model.err)
		})

		t.Run("PendingComposerTextRestoresOnlyIntoEmptyComposer", func(t *testing.T) {
			t.Parallel()

			t.Run("messageSentMsg", func(t *testing.T) {
				t.Parallel()

				model := newTestChatViewModel(nil)
				chat := testChat(codersdk.ChatStatusCompleted)
				model.setChat(chat)
				model.loading = false
				model.composer.SetValue("original")

				model, _ = model.sendMessage()
				require.Equal(t, "original", model.pendingComposerText)

				model.composer.SetValue("new input")

				model, _ = model.Update(messageSentMsg{err: xerrors.New("fail")})
				require.Equal(t, "new input", model.composer.Value())
				require.Error(t, model.err)
			})

			t.Run("chatCreatedMsg", func(t *testing.T) {
				t.Parallel()

				model := newTestChatViewModel(nil)
				model.draft = true
				model.loading = false
				model.composer.SetValue("draft text")

				model, _ = model.sendMessage()
				require.Equal(t, "draft text", model.pendingComposerText)

				model.composer.SetValue("edited")

				model, _ = model.Update(chatCreatedMsg{err: xerrors.New("fail")})
				require.Equal(t, "edited", model.composer.Value())
				require.Error(t, model.err)
			})
		})

		t.Run("EnterWhileLoadingDoesNotDispatchOrClearComposer", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.loading = true
			model.composer.SetValue("my text")

			updated, cmd := model.sendMessage()
			require.Nil(t, cmd)
			require.Equal(t, "my text", updated.composer.Value())
			require.Empty(t, updated.pendingComposerText)
		})

		t.Run("EnterWithoutLoadedChatDoesNotDispatchOrClearComposer", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.loading = false
			model.draft = false
			model.chat = nil
			model.composer.SetValue("my text")

			updated, cmd := model.sendMessage()
			require.Nil(t, cmd)
			require.Equal(t, "my text", updated.composer.Value())
			require.Empty(t, updated.pendingComposerText)
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

		t.Run("DuplicateDraftCreateIsIgnored", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model.draft = true
			model.loading = false
			model.creatingChat = true
			model.composer.SetValue("hello")

			updated, cmd := model.sendMessage()
			require.Nil(t, cmd)
			require.Equal(t, "hello", updated.composer.Value())
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

	t.Run("ChatView/ModelOverrideMapsCanonicalModelIDForDraftCreate", func(t *testing.T) {
		t.Parallel()

		modelConfigID := uuid.New()
		createdChat := testChat(codersdk.ChatStatusWaiting)
		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			rw.Header().Set("Content-Type", "application/json")
			switch {
			case req.Method == http.MethodGet && req.URL.Path == "/api/experimental/chats/model-configs":
				require.NoError(t, json.NewEncoder(rw).Encode([]codersdk.ChatModelConfig{{
					ID:       modelConfigID,
					Provider: "provider",
					Model:    "model",
				}}))
			case req.Method == http.MethodPost && req.URL.Path == "/api/experimental/chats":
				var createReq codersdk.CreateChatRequest
				require.NoError(t, json.NewDecoder(req.Body).Decode(&createReq))
				require.NotNil(t, createReq.ModelConfigID)
				require.Equal(t, modelConfigID, *createReq.ModelConfigID)
				rw.WriteHeader(http.StatusCreated)
				require.NoError(t, json.NewEncoder(rw).Encode(createdChat))
			default:
				t.Fatalf("unexpected %s %s", req.Method, req.URL.Path)
			}
		}))
		defer server.Close()

		serverURL, err := url.Parse(server.URL)
		require.NoError(t, err)

		client := codersdk.NewExperimentalClient(codersdk.New(serverURL))
		model := newTestChatViewModel(client)
		model.draft = true
		model.loading = false
		modelOverride := "provider:model"
		model.modelOverride = &modelOverride
		model.composer.SetValue("hello")

		updated, cmd := model.sendMessage()
		require.NotNil(t, cmd)
		require.Empty(t, updated.composer.Value())

		msg, ok := mustMsg(t, cmd).(chatCreatedMsg)
		require.True(t, ok)
		require.NoError(t, msg.err)
		require.Equal(t, createdChat.ID, msg.chat.ID)
	})

	t.Run("ChatView/ModelOverrideMapsCanonicalModelIDForSendMessage", func(t *testing.T) {
		t.Parallel()

		modelConfigID := uuid.New()
		chat := testChat(codersdk.ChatStatusCompleted)
		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			rw.Header().Set("Content-Type", "application/json")
			wantPath := fmt.Sprintf("/api/experimental/chats/%s/messages", chat.ID)
			switch {
			case req.Method == http.MethodGet && req.URL.Path == "/api/experimental/chats/model-configs":
				require.NoError(t, json.NewEncoder(rw).Encode([]codersdk.ChatModelConfig{{
					ID:       modelConfigID,
					Provider: "provider",
					Model:    "model",
				}}))
			case req.Method == http.MethodPost && req.URL.Path == wantPath:
				var messageReq codersdk.CreateChatMessageRequest
				require.NoError(t, json.NewDecoder(req.Body).Decode(&messageReq))
				require.NotNil(t, messageReq.ModelConfigID)
				require.Equal(t, modelConfigID, *messageReq.ModelConfigID)
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.CreateChatMessageResponse{}))
			default:
				t.Fatalf("unexpected %s %s", req.Method, req.URL.Path)
			}
		}))
		defer server.Close()

		serverURL, err := url.Parse(server.URL)
		require.NoError(t, err)

		client := codersdk.NewExperimentalClient(codersdk.New(serverURL))
		model := newTestChatViewModel(client)
		model.chat = &chat
		model.chatStatus = chat.Status
		model.loading = false
		modelOverride := "provider:model"
		model.modelOverride = &modelOverride
		model.composer.SetValue("hello")

		updated, cmd := model.sendMessage()
		require.NotNil(t, cmd)
		require.Empty(t, updated.composer.Value())

		msg, ok := mustMsg(t, cmd).(messageSentMsg)
		require.True(t, ok)
		require.NoError(t, msg.err)
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

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
			require.Nil(t, cmd)
			require.True(t, updated.interrupting)
		})

		t.Run("InterruptOnIdleChatIsNoOp", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(failingExperimentalClient())
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat = &chat
			model.chatStatus = codersdk.ChatStatusCompleted

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
			require.Nil(t, cmd)
			require.False(t, updated.interrupting)
		})

		t.Run("CtrlXInterruptsRunningChat", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(failingExperimentalClient())
			chat := testChat(codersdk.ChatStatusRunning)
			model.chat = &chat
			model.chatStatus = codersdk.ChatStatusRunning
			require.True(t, model.composerFocused)

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
			require.NotNil(t, cmd)
			require.True(t, updated.interrupting)
			require.True(t, updated.composerFocused)
		})

		t.Run("TabKeepsFocusSwitchBehaviorWhileRunningChat", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(failingExperimentalClient())
			chat := testChat(codersdk.ChatStatusRunning)
			model.chat = &chat
			model.chatStatus = codersdk.ChatStatusRunning
			require.True(t, model.composerFocused)

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyTab})
			require.Nil(t, cmd)
			require.False(t, updated.interrupting)
			require.False(t, updated.composerFocused)
		})

		t.Run("ViewShowsCtrlXInterruptHelp", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model, _ = model.Update(tea.WindowSizeMsg{Width: 140, Height: 12})
			chat := testChat(codersdk.ChatStatusRunning)
			model.chat = &chat
			model.chatStatus = codersdk.ChatStatusRunning
			model.loading = false

			view := plainText(model.View())
			require.Contains(t, view, "ctrl+x: interrupt")
			require.NotContains(t, view, "ctrl+i: interrupt")
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

	t.Run("ChatView/ViewportScrolling", func(t *testing.T) {
		t.Parallel()

		applyWindowSize := func(t *testing.T, model expChatsTUIModel, width int, height int) expChatsTUIModel {
			t.Helper()

			updatedModel, cmd := model.Update(tea.WindowSizeMsg{Width: width, Height: height})
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

			model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyTab})
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

		t.Run("ViewportHeights", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name    string
				height  int
				prepare func(*expChatsTUIModel)
				assert  func(t *testing.T, model expChatsTUIModel)
			}{
				{"Standard", 40, nil, func(t *testing.T, model expChatsTUIModel) {
					require.Equal(t, 39, model.chat.height)
					require.Equal(t, 31, model.chat.viewport.Height)
				}},
				{"MinimumZero", 5, nil, func(t *testing.T, model expChatsTUIModel) {
					require.Equal(t, 0, model.chat.viewport.Height)
				}},
				{"ViewFitsTerminal", 40, func(model *expChatsTUIModel) {
					model.currentView = viewChat
					model.chat.loading = false
					chat := testChat(codersdk.ChatStatusCompleted)
					model.chat.chat = &chat
					model.chat.chatStatus = chat.Status
					model.chat.messages = overflowingMessages(24)
					model.chat.rebuildBlocks()
				}, func(t *testing.T, model expChatsTUIModel) {
					require.LessOrEqual(t, strings.Count(model.View(), "\n")+1, 40)
				}},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := applyWindowSize(t, newTestTUIModel(), 80, tt.height)
					if tt.prepare != nil {
						tt.prepare(&model)
					}
					tt.assert(t, model)
				})
			}
		})

		t.Run("ScrollNavigation", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name   string
				setup  func(t *testing.T, model chatViewModel) chatViewModel
				key    tea.KeyType
				assert func(t *testing.T, before chatViewModel, after chatViewModel)
			}{
				{"ScrollUpDecreasesYOffset", nil, tea.KeyUp, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.False(t, after.autoFollow)
					require.Less(t, after.viewport.YOffset, before.viewport.YOffset)
				}},
				{"ScrollDownIncreasesYOffset", func(t *testing.T, model chatViewModel) chatViewModel {
					updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
					return updated
				}, tea.KeyDown, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.Greater(t, after.viewport.YOffset, before.viewport.YOffset)
				}},
				{"ScrollUpAtTopIsNoOp", func(t *testing.T, model chatViewModel) chatViewModel {
					updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyHome})
					return updated
				}, tea.KeyUp, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.Equal(t, 0, before.viewport.YOffset)
					require.Equal(t, before.viewport.YOffset, after.viewport.YOffset)
				}},
				{"ScrollDownAtBottomReEnablesAutoFollow", func(t *testing.T, model chatViewModel) chatViewModel {
					updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
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
				{"PageDownScrollsHalfViewport", func(t *testing.T, model chatViewModel) chatViewModel {
					updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
					return updated
				}, tea.KeyPgDown, func(t *testing.T, before chatViewModel, after chatViewModel) {
					halfView := before.viewport.Height / 2
					require.InDelta(t, float64(before.viewport.YOffset+halfView), float64(after.viewport.YOffset), 1)
				}},
				{"HomeJumpsToTop", nil, tea.KeyHome, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.NotZero(t, before.viewport.YOffset)
					require.Equal(t, 0, after.viewport.YOffset)
					require.False(t, after.autoFollow)
				}},
				{"EndJumpsToBottomAndEnablesAutoFollow", func(t *testing.T, model chatViewModel) chatViewModel {
					updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyHome})
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

		t.Run("AutoFollowOnContentUpdates", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name   string
				setup  func(chatViewModel) chatViewModel
				update func(chatViewModel) chatViewModel
				assert func(t *testing.T, before chatViewModel, after chatViewModel)
			}{
				{"SetContentPreservesScrollPosition", func(model chatViewModel) chatViewModel {
					updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
					return updated
				}, func(model chatViewModel) chatViewModel {
					updated, _ := model.Update(streamMessage(1001))
					return updated
				}, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.False(t, after.autoFollow)
					require.Equal(t, before.viewport.YOffset, after.viewport.YOffset)
				}},
				{"NewMessageAutoFollowsWhenAtBottom", nil, func(model chatViewModel) chatViewModel {
					updated, _ := model.Update(streamMessage(1002))
					return updated
				}, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.True(t, after.autoFollow)
					require.True(t, after.viewport.AtBottom())
					require.GreaterOrEqual(t, after.viewport.YOffset, before.viewport.YOffset)
				}},
				{"NewMessageDoesNotAutoFollowWhenScrolledUp", func(model chatViewModel) chatViewModel {
					updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
					return updated
				}, func(model chatViewModel) chatViewModel {
					updated, _ := model.Update(streamMessage(1003))
					return updated
				}, func(t *testing.T, before chatViewModel, after chatViewModel) {
					require.False(t, after.autoFollow)
					require.False(t, after.viewport.AtBottom())
					require.Equal(t, before.viewport.YOffset, after.viewport.YOffset)
				}},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					model := newScrollableModel(t)
					if tt.setup != nil {
						model = tt.setup(model)
					}
					before := model
					after := tt.update(model)
					tt.assert(t, before, after)
				})
			}
		})

		t.Run("StreamingAutoFollows", func(t *testing.T) {
			t.Parallel()

			model := newTestChatViewModel(nil)
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
			model, _ = model.Update(chatHistoryMsg{messages: overflowingMessages(10)})
			before := model.viewport.YOffset

			model, _ = model.Update(chatStreamEventMsg{event: testTextPartEvent(strings.Repeat("hello world ", 20))})
			model, _ = model.Update(chatStreamEventMsg{event: testTextPartEvent(strings.Repeat("more text ", 20))})

			require.True(t, model.autoFollow)
			require.True(t, model.viewport.AtBottom())
			require.GreaterOrEqual(t, model.viewport.YOffset, before)
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

					model := baseChatModel()
					model.catalog = &catalog
					model.chat.modelPickerFlat = catalog.Providers[0].Models
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
					model := baseChatModel()
					updatedModel, cmd := model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
					updated := mustTUIModel(t, updatedModel, cmd)
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

			firstModelID := "provider:model-a"
			secondModelID := "provider:model-b"
			catalog := codersdk.ChatModelsResponse{
				Providers: []codersdk.ChatModelProvider{{
					Provider:  "provider",
					Available: true,
					Models: []codersdk.ChatModel{
						{
							ID:          firstModelID,
							Provider:    "provider",
							Model:       "model-a",
							DisplayName: "Model A",
						},
						{
							ID:          secondModelID,
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

		t.Run("ChatsListedUpdatesState", func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name   string
				msg    chatsListedMsg
				assert func(t *testing.T, updated chatListModel)
			}{
				{"StoresChats", chatsListedMsg{chats: []codersdk.Chat{testChat(codersdk.ChatStatusWaiting), testChat(codersdk.ChatStatusCompleted)}}, func(t *testing.T, updated chatListModel) {
					require.Len(t, updated.chats, 2)
					require.NoError(t, updated.err)
				}},
				{"StoresErr", chatsListedMsg{err: xerrors.New("list failed")}, func(t *testing.T, updated chatListModel) {
					require.EqualError(t, updated.err, "list failed")
				}},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					updated, cmd := newChatListModel(newTUIStyles()).Update(tt.msg)
					require.Nil(t, cmd)
					require.False(t, updated.loading)
					tt.assert(t, updated)
				})
			}
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

			model := newReadyChatListModel()
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

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
			require.True(t, updated.expanded[parent.ID])
			require.Contains(t, plainText(updated.View()), "    Subagent chat")

			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
			require.Equal(t, child.ID, updated.selectedChat().ID)

			updated, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
			openMsg, ok := mustMsg(t, cmd).(openSelectedChatMsg)
			require.True(t, ok)
			require.Equal(t, child.ID, openMsg.chatID)

			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyLeft})
			require.False(t, updated.expanded[parent.ID])
			require.Equal(t, 0, updated.cursor)
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

		t.Run("SearchIncludesAncestorChainForMatchingGrandchild", func(t *testing.T) {
			t.Parallel()

			model := newChatListModel(newTUIStyles())
			model.loading = false

			root := testChat(codersdk.ChatStatusRunning)
			root.Title = "Root chat"
			child := testChat(codersdk.ChatStatusWaiting)
			child.Title = "Child subagent"
			child.ParentChatID = &root.ID
			grandchild := testChat(codersdk.ChatStatusPending)
			grandchild.Title = "Grandchild needle"
			grandchild.ParentChatID = &child.ID
			other := testChat(codersdk.ChatStatusCompleted)
			other.Title = "Other root"
			model.chats = []codersdk.Chat{root, child, grandchild, other}
			model.search.SetValue("needle")

			rows := model.displayRows()
			require.Len(t, rows, 3)
			require.Equal(t, root.ID, rows[0].chat.ID)
			require.True(t, rows[0].isExpanded)
			require.Equal(t, child.ID, rows[1].chat.ID)
			require.True(t, rows[1].isExpanded)
			require.Equal(t, grandchild.ID, rows[2].chat.ID)
			require.Equal(t, 2, rows[2].depth)
		})

		t.Run("DisplayRowsRenderNestedSubagentsRecursively", func(t *testing.T) {
			t.Parallel()

			model := newChatListModel(newTUIStyles())
			model.loading = false

			parent := testChat(codersdk.ChatStatusRunning)
			parent.Title = "Parent chat"
			child := testChat(codersdk.ChatStatusWaiting)
			child.Title = "Child subagent"
			child.ParentChatID = &parent.ID
			grandchild := testChat(codersdk.ChatStatusPending)
			grandchild.Title = "Grandchild subagent"
			grandchild.ParentChatID = &child.ID
			model.chats = []codersdk.Chat{parent, child, grandchild}
			model.expanded[parent.ID] = true
			model.expanded[child.ID] = true

			rows := model.displayRows()
			require.Len(t, rows, 3)
			require.Equal(t, parent.ID, rows[0].chat.ID)
			require.Equal(t, 0, rows[0].depth)
			require.Equal(t, child.ID, rows[1].chat.ID)
			require.True(t, rows[1].isSubagent)
			require.Equal(t, 1, rows[1].depth)
			require.Equal(t, 1, rows[1].childCount)
			require.True(t, rows[1].isExpanded)
			require.Equal(t, grandchild.ID, rows[2].chat.ID)
			require.True(t, rows[2].isSubagent)
			require.Equal(t, 2, rows[2].depth)

			output := plainText(model.View())
			require.Contains(t, output, "▼ Child subagent")
			require.Contains(t, output, "Grandchild subagent")
		})

		t.Run("ExpandKeyExpandsNestedSubagent", func(t *testing.T) {
			t.Parallel()

			model := newReadyChatListModel()
			model.width = 100
			model.height = 10

			parent := testChat(codersdk.ChatStatusRunning)
			parent.Title = "Parent chat"
			child := testChat(codersdk.ChatStatusWaiting)
			child.Title = "Child subagent"
			child.ParentChatID = &parent.ID
			grandchild := testChat(codersdk.ChatStatusPending)
			grandchild.Title = "Grandchild subagent"
			grandchild.ParentChatID = &child.ID
			model.chats = []codersdk.Chat{parent, child, grandchild}

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
			require.True(t, updated.expanded[parent.ID])

			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
			require.Equal(t, child.ID, updated.selectedChat().ID)

			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRight})
			require.True(t, updated.expanded[child.ID])
			require.Contains(t, plainText(updated.View()), "Grandchild subagent")
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

			model := newReadyChatListModel()
			model.width = 80
			model.height = 8
			for i := range 8 {
				chat := testChat(codersdk.ChatStatusWaiting)
				chat.Title = fmt.Sprintf("chat %02d", i)
				model.chats = append(model.chats, chat)
			}

			for range 6 {
				model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
			}
			require.Equal(t, 2, model.offset)
			listView := plainText(model.View())
			require.Contains(t, listView, "> chat 06")
			require.NotContains(t, listView, "chat 00")

			parent := newTestTUIModel()
			parent.width = 80
			parent.height = 8
			parent.list = model
			parentView := plainText(parent.View())
			require.Contains(t, parentView, "Coder Chats")
			require.Contains(t, parentView, "> chat 06")

			for range 5 {
				model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
			}
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

// newTestChatViewModel creates a chatViewModel for reducer tests.
// The returned model has chatGeneration=0, so test messages with
// default generation=0 pass the generation guard.
func newTestChatViewModel(client *codersdk.ExperimentalClient) chatViewModel {
	return newChatViewModel(context.Background(), client, nil, nil, newTUIStyles())
}

func newTestTUIModel() expChatsTUIModel {
	return newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
}

func baseChatModel() expChatsTUIModel {
	model := newTestTUIModel()
	model.currentView = viewChat
	model.chat.loading = false
	return model
}

func newReadyChatListModel() chatListModel {
	model := newChatListModel(newTUIStyles())
	model.loading = false
	return model
}

func overflowingMessages(count int) []codersdk.ChatMessage {
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
