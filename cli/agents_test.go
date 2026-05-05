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

func TestAgents(t *testing.T) {
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

		client := newTestExperimentalClient(t, func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(rw).Encode(catalog)
		})
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
		tests := []struct {
			name    string
			overlay tuiOverlay
		}{
			{"ModelPicker", overlayModelPicker},
			{"DiffDrawer", overlayDiffDrawer},
		}
		for _, tt := range tests {
			t.Run("EscFromOverlayClosesIt/"+tt.name, func(t *testing.T) {
				t.Parallel()
				model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
				model.currentView = viewChat
				model.overlay = tt.overlay

				updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
				updated, _ := mustTUIModelWithCmd(t, updatedModel, cmd)
				require.Equal(t, viewChat, updated.currentView)
				require.Equal(t, overlayNone, updated.overlay)
			})
		}

		t.Run("AdditionalOverlayCloseKeys", func(t *testing.T) {
			t.Parallel()
			tests := []struct {
				name    string
				overlay tuiOverlay
				key     tea.KeyMsg
			}{
				{name: "ModelPicker/KeyEscape", overlay: overlayModelPicker, key: tea.KeyMsg{Type: tea.KeyEscape}},
				{name: "ModelPicker/CtrlOpenBracket", overlay: overlayModelPicker, key: tea.KeyMsg{Type: tea.KeyCtrlOpenBracket}},
				{name: "DiffDrawer/KeyEscape", overlay: overlayDiffDrawer, key: tea.KeyMsg{Type: tea.KeyEscape}},
				{name: "DiffDrawer/CtrlOpenBracket", overlay: overlayDiffDrawer, key: tea.KeyMsg{Type: tea.KeyCtrlOpenBracket}},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
					model.currentView = viewChat
					model.overlay = tt.overlay

					updatedModel, cmd := model.Update(tt.key)
					updated, _ := mustTUIModelWithCmd(t, updatedModel, cmd)
					require.Equal(t, viewChat, updated.currentView)
					require.Equal(t, overlayNone, updated.overlay)
				})
			}
		})

		t.Run("EscFromChatViewReturnsToListAndRefreshes", func(t *testing.T) {
			t.Parallel()
			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
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
			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
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
			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
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

		t.Run("EscFromSearchClearsFilterAndRestoresListNavigation", func(t *testing.T) {
			t.Parallel()
			chats := []codersdk.Chat{
				{ID: uuid.New(), Title: "alpha", Status: codersdk.ChatStatusCompleted, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				{ID: uuid.New(), Title: "beta", Status: codersdk.ChatStatusCompleted, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				{ID: uuid.New(), Title: "gamma", Status: codersdk.ChatStatusCompleted, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			}
			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
			updatedModel, cmd := model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
			model = mustTUIModel(t, updatedModel, cmd)
			model.currentView = viewList
			model.list.loading = false
			model.list.chats = chats

			updatedModel, cmd = model.Update(keyRunes("/"))
			updated := mustTUIModel(t, updatedModel, cmd)
			require.True(t, updated.list.searching)

			updatedModel, cmd = updated.Update(keyRunes("b"))
			updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, "b", updated.list.search.Value())

			updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated = mustTUIModel(t, updatedModel, cmd)
			require.False(t, updated.quitting)
			require.False(t, updated.list.searching)
			require.Empty(t, updated.list.search.Value())

			updatedModel, cmd = updated.Update(keyRunes("j"))
			updated = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, 1, updated.list.cursor)

			updatedModel, cmd = updated.Update(keyRunes("k"))
			updated = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, 0, updated.list.cursor)
		})

		for name, view := range map[string]tuiView{
			"List": viewList,
			"Chat": viewChat,
		} {
			t.Run("CtrlCQuitsFromAnyState/"+name, func(t *testing.T) {
				t.Parallel()
				model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
				model.currentView = view

				updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
				updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
				require.True(t, updated.quitting)
				_, ok := mustMsg(t, cmd).(tea.QuitMsg)
				require.True(t, ok)
			})
		}

		t.Run("OpenChatSwitchesView", func(t *testing.T) {
			t.Parallel()
			tests := []struct {
				name         string
				msg          tea.Msg
				draft        bool
				wantLoading  bool
				wantBatchLen int
			}{
				{name: "SelectedChat", msg: openSelectedChatMsg{chatID: uuid.New()}, wantLoading: true, wantBatchLen: 3},
				{name: "DraftChat", msg: openDraftChatMsg{}, draft: true},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
					model.width, model.height = 100, 40
					updatedModel, cmd := model.Update(tt.msg)
					updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
					require.Equal(t, viewChat, updated.currentView)
					require.Equal(t, 40, updated.chat.height)
					require.Equal(t, 34, updated.chat.viewport.Height)
					if tt.draft {
						require.True(t, updated.chat.draft)
						require.False(t, updated.chat.loading)
						require.True(t, updated.chat.metadataResolved)
						require.True(t, updated.chat.historyResolved)
						require.Nil(t, cmd)
						return
					}
					require.Equal(t, tt.wantLoading, updated.chat.loading)
					require.Len(t, mustBatchMsg(t, cmd), tt.wantBatchLen)
				})
			}
		})
		t.Run("EscFromChatViewRestoresListHeaderAndPadsTerminal", func(t *testing.T) {
			t.Parallel()
			assertReturnToList := func(t testing.TB, model chatsTUIModel) {
				t.Helper()
				updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
				updated, _ := mustTUIModelWithCmd(t, updatedModel, cmd)
				require.Equal(t, viewList, updated.currentView)
				firstLine, _, _ := strings.Cut(plainText(updated.View()), "\n")
				require.Equal(t, "Coder Chats", firstLine)
				require.Equal(t, updated.height, countRenderedLines(plainText(updated.View())))
			}

			t.Run("SelectedChat", func(t *testing.T) {
				t.Parallel()
				model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
				updatedModel, cmd := model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
				model = mustTUIModel(t, updatedModel, cmd)
				model.list.loading = false
				model.list.chats = []codersdk.Chat{testChat(codersdk.ChatStatusCompleted)}
				chatID := uuid.New()

				updatedModel, cmd = model.Update(openSelectedChatMsg{chatID: chatID})
				model, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
				openedChat := testChat(codersdk.ChatStatusCompleted)
				openedChat.ID = chatID
				openedChat.Title = "Existing chat"
				updatedModel, cmd = model.Update(chatOpenedMsg{generation: model.chat.chatGeneration, chatID: chatID, chat: openedChat})
				model = mustTUIModel(t, updatedModel, cmd)
				require.Contains(t, plainText(model.View()), "Existing chat")

				assertReturnToList(t, model)
			})

			t.Run("DraftChat", func(t *testing.T) {
				t.Parallel()
				model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
				updatedModel, cmd := model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
				model = mustTUIModel(t, updatedModel, cmd)
				model.list.loading = false
				model.list.chats = []codersdk.Chat{testChat(codersdk.ChatStatusCompleted)}

				updatedModel, cmd = model.Update(openDraftChatMsg{})
				model, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
				require.Contains(t, plainText(model.View()), "New Chat (draft)")

				assertReturnToList(t, model)
			})
		})
		t.Run("ChatViewOmitsListHeaderAndLoadingSpinner", func(t *testing.T) {
			t.Parallel()

			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
			updatedModel, cmd := model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
			model = mustTUIModel(t, updatedModel, cmd)
			model.currentView = viewChat
			model.list.loading = true
			model.chat.loading = false

			chat := testChat(codersdk.ChatStatusCompleted)
			chat.Title = "Existing chat"
			model.chat.chat = &chat
			model.chat.chatStatus = chat.Status
			model.chat.messages = []codersdk.ChatMessage{
				testMessage(1, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{
					Type: codersdk.ChatMessagePartTypeText,
					Text: "assistant reply",
				}),
			}
			model.chat.rebuildBlocks()

			view := plainText(model.View())
			firstLine, _, _ := strings.Cut(view, "\n")
			require.Contains(t, firstLine, "Existing chat")
			require.NotContains(t, view, "Coder Chats")
			require.NotContains(t, view, "Loading chats")
		})

		t.Run("ReopensModelPickerAfterClosing", func(t *testing.T) {
			t.Parallel()
			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
			model.currentView = viewChat
			catalog := codersdk.ChatModelsResponse{
				Providers: []codersdk.ChatModelProvider{{
					Provider:  "provider",
					Available: true,
					Models: []codersdk.ChatModel{{
						ID:          "provider:model-a",
						Provider:    "provider",
						Model:       "model-a",
						DisplayName: "Model A",
					}},
				}},
			}
			model.catalog = &catalog
			model.chat.modelPickerFlat = catalog.Providers[0].Models
			updatedModel, cmd := model.Update(toggleModelPickerMsg{})
			updated := mustTUIModel(t, updatedModel, cmd)
			updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
			updated = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, overlayNone, updated.overlay)

			updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
			updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, overlayModelPicker, updated.overlay)
		})

		t.Run("ModelPickerBehavior", func(t *testing.T) {
			t.Parallel()
			twoModelCatalog := codersdk.ChatModelsResponse{
				Providers: []codersdk.ChatModelProvider{{
					Provider:  "openai",
					Available: true,
					Models: []codersdk.ChatModel{
						{ID: "openai:gpt-4o", Provider: "openai", Model: "gpt-4o", DisplayName: "GPT-4o"},
						{ID: "openai:gpt-4.1", Provider: "openai", Model: "gpt-4.1", DisplayName: "GPT-4.1"},
					},
				}},
			}

			t.Run("CancelClosesOverlay", func(t *testing.T) {
				t.Parallel()
				model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
				model.currentView = viewChat
				model.width = 80
				model.height = 24
				updatedModel, cmd := model.Update(modelsListedMsg{catalog: twoModelCatalog})
				updated, _ := mustTUIModelWithCmd(t, updatedModel, cmd)

				updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
				updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
				require.Equal(t, overlayModelPicker, updated.overlay)

				updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
				updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
				require.Equal(t, overlayNone, updated.overlay)
			})

			t.Run("EscClosesPickerWithoutLeavingChat", func(t *testing.T) {
				t.Parallel()
				model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
				model.currentView = viewChat
				model.width = 80
				model.height = 24
				model.chat.draft = true
				model.chat.composerFocused = true
				model.chat.composer.SetValue("keep draft")
				updatedModel, cmd := model.Update(modelsListedMsg{catalog: twoModelCatalog})
				updated := mustTUIModel(t, updatedModel, cmd)

				updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
				updated = mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, overlayModelPicker, updated.overlay)

				updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
				updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
				// ClearScreen cmd is expected
				require.Equal(t, overlayNone, updated.overlay)
				require.Equal(t, viewChat, updated.currentView)
				require.Equal(t, "keep draft", updated.chat.composer.Value())
			})

			t.Run("AdditionalCloseKeysClosePickerWithoutLeavingChat", func(t *testing.T) {
				t.Parallel()
				tests := []struct {
					name string
					key  tea.KeyMsg
				}{
					{name: "CtrlP", key: tea.KeyMsg{Type: tea.KeyCtrlP}},
					{name: "Q", key: keyRunes("q")},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						t.Parallel()
						model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
						model.currentView = viewChat
						model.width = 80
						model.height = 24
						model.chat.draft = true
						model.chat.composerFocused = true
						model.chat.composer.SetValue("keep draft")
						updatedModel, cmd := model.Update(modelsListedMsg{catalog: twoModelCatalog})
						updated := mustTUIModel(t, updatedModel, cmd)

						updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
						updated = mustTUIModel(t, updatedModel, cmd)
						require.Equal(t, overlayModelPicker, updated.overlay)

						updatedModel, cmd = updated.Update(tt.key)
						updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
						// ClearScreen cmd is expected
						require.Equal(t, overlayNone, updated.overlay)
						require.Equal(t, viewChat, updated.currentView)
						require.Equal(t, "keep draft", updated.chat.composer.Value())
					})
				}
			})

			t.Run("EnterSelectsModelWithoutSendingDraft", func(t *testing.T) {
				t.Parallel()
				model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
				model.currentView = viewChat
				model.width = 80
				model.height = 24
				model.chat.draft = true
				model.chat.composerFocused = true
				model.chat.composer.SetValue("keep draft")
				updatedModel, cmd := model.Update(modelsListedMsg{catalog: twoModelCatalog})
				updated := mustTUIModel(t, updatedModel, cmd)

				updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
				updated = mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, overlayModelPicker, updated.overlay)

				updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
				updated = mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, 1, updated.chat.modelPickerCursor)

				updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
				updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
				// ClearScreen cmd is expected
				require.Equal(t, overlayNone, updated.overlay)
				require.NotNil(t, updated.chat.modelOverride)
				require.NotNil(t, updated.modelOverride)
				require.Equal(t, "openai:gpt-4.1", *updated.chat.modelOverride)
				require.Equal(t, "openai:gpt-4.1", *updated.modelOverride)
				require.Equal(t, "keep draft", updated.chat.composer.Value())
				require.False(t, updated.chat.creatingChat)
			})

			t.Run("LoadErrorClosesOverlay", func(t *testing.T) {
				t.Parallel()
				model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
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
			})

			t.Run("ScrollAndSelectModel", func(t *testing.T) {
				t.Parallel()
				model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
				model.currentView = viewChat
				model.width = 80
				model.height = 24
				updatedModel, cmd := model.Update(modelsListedMsg{catalog: twoModelCatalog})
				updated := mustTUIModel(t, updatedModel, cmd)

				updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
				updated = mustTUIModel(t, updatedModel, cmd)

				for range 4 {
					updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
					updated = mustTUIModel(t, updatedModel, cmd)
				}
				require.Equal(t, 1, updated.chat.modelPickerCursor)

				updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
				updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
				require.Equal(t, overlayNone, updated.overlay)
				require.NotNil(t, updated.chat.modelOverride)
				require.NotNil(t, updated.modelOverride)
				require.Equal(t, "openai:gpt-4.1", *updated.chat.modelOverride)
				require.Equal(t, "openai:gpt-4.1", *updated.modelOverride)
			})
		})

		t.Run("DiffDrawerLoadingState", func(t *testing.T) {
			t.Parallel()
			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
			model.currentView = viewChat
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat.chat = &chat

			updatedModel, cmd := model.Update(toggleDiffDrawerMsg{})
			updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Equal(t, overlayDiffDrawer, updated.overlay)
			require.NotNil(t, cmd)
			require.Contains(t, plainText(updated.View()), "Loading diff")
		})

		t.Run("DiffDrawerErrorState", func(t *testing.T) {
			t.Parallel()
			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
			model.currentView = viewChat
			model.width = 80
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat.chat = &chat

			updatedModel, cmd := model.Update(toggleDiffDrawerMsg{})
			updated, _ := mustTUIModelWithCmd(t, updatedModel, cmd)

			updatedModel, cmd = updated.Update(diffContentsMsg{err: xerrors.New("connection refused")})
			updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
			require.Contains(t, plainText(updated.View()), "connection refused")
		})

		t.Run("DiffDrawerMemoizesSummary", func(t *testing.T) {
			t.Parallel()
			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
			model.currentView = viewChat
			model.width = 80
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat.chat = &chat
			generation := model.chat.chatGeneration

			// A successful diffContentsMsg pre-renders the summary
			// and the lipgloss-styled body so View() redraws do not
			// re-parse or re-style the full diff on every keypress
			// (see chatViewModel.diffSummary and diffStyledBody).
			diff := codersdk.ChatDiffContents{
				ChatID: chat.ID,
				Diff:   "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new",
			}
			updatedModel, cmd := model.Update(diffContentsMsg{generation: generation, chatID: chat.ID, diff: diff})
			updated, _ := mustTUIModelWithCmd(t, updatedModel, cmd)
			require.NotNil(t, updated.chat.diffContents)
			require.Equal(t, "1 file changed:\n  modified a.txt (+1 -1)", updated.chat.diffSummary)
			require.NotEmpty(t, updated.chat.diffStyledBody)
			// The cached styled body still contains the diff text
			// verbatim: lipgloss wraps lines in escape codes without
			// replacing them, so every original line of the input
			// diff must survive the round-trip.
			require.Contains(t, plainText(updated.chat.diffStyledBody), "diff --git a/a.txt b/a.txt")
			require.Contains(t, plainText(updated.chat.diffStyledBody), "+new")

			// setChat clears both caches so a new chat does not
			// inherit stale render output from the previous session.
			(&updated.chat).setChat(testChat(codersdk.ChatStatusCompleted))
			require.Empty(t, updated.chat.diffSummary)
			require.Empty(t, updated.chat.diffStyledBody)
			require.Nil(t, updated.chat.diffContents)
		})

		t.Run("OverlayDismissedOnViewSwitch", func(t *testing.T) {
			t.Parallel()
			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
			model.currentView = viewChat
			model.overlay = overlayModelPicker

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			updated, _ := mustTUIModelWithCmd(t, updatedModel, cmd)
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

			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
			model.currentView = viewChat
			model.overlay = overlayModelPicker
			model.catalog = &catalog
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat.chat = &chat

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

			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
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
			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
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
		setup := func(metadataResolved, historyResolved bool) chatViewModel {
			model := newTestChatViewModel(nil)
			model.loading, model.metadataResolved, model.historyResolved = true, metadataResolved, historyResolved
			return model
		}
		t.Run("ChatOpenedSuccessAndError", func(t *testing.T) {
			t.Parallel()
			diffStatus := &codersdk.ChatDiffStatus{ChatID: uuid.New()}
			chat := testChat(codersdk.ChatStatusRunning)
			chat.DiffStatus = diffStatus
			chat.PlanMode = codersdk.ChatPlanModePlan
			updated, cmd := setup(false, true).Update(chatOpenedMsg{chat: chat})
			require.NotNil(t, cmd)
			require.Equal(t, chat.ID, updated.chat.ID)
			require.Equal(t, codersdk.ChatStatusRunning, updated.chatStatus)
			require.Equal(t, diffStatus, updated.diffStatus)
			require.Equal(t, codersdk.ChatPlanModePlan, updated.planMode)
			require.False(t, updated.loading)
			require.Nil(t, updated.err)
			updated, cmd = setup(false, true).Update(chatOpenedMsg{err: xerrors.New("open failed")})
			require.Nil(t, cmd)
			require.Equal(t, "open failed", updated.err.Error())
			require.False(t, updated.loading)
		})
		t.Run("ChatHistorySuccessAndError", func(t *testing.T) {
			t.Parallel()
			usageA := &codersdk.ChatMessageUsage{TotalTokens: int64Ref(10)}
			usageB := &codersdk.ChatMessageUsage{TotalTokens: int64Ref(20)}
			second := testMessage(2, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "second"})
			second.Usage = usageA
			third := testMessage(3, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeReasoning, Text: "third"})
			third.Usage = usageB
			messages := []codersdk.ChatMessage{testMessage(1, codersdk.ChatMessageRoleUser, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "first"}), second, third}
			updated, cmd := setup(true, false).Update(chatHistoryMsg{messages: messages})
			require.Nil(t, cmd)
			require.Equal(t, messages, updated.messages)
			require.Len(t, updated.blocks, 3)
			require.Equal(t, usageB, updated.lastUsage)
			require.False(t, updated.loading)
			updated, cmd = setup(true, false).Update(chatHistoryMsg{err: xerrors.New("history failed")})
			require.Nil(t, cmd)
			require.Equal(t, "history failed", updated.err.Error())
			require.False(t, updated.loading)
		})
		t.Run("OpenHistoryBothSucceedOutOfOrder", func(t *testing.T) {
			t.Parallel()
			model := setup(false, false)
			model, _ = model.Update(chatHistoryMsg{messages: []codersdk.ChatMessage{testMessage(1, codersdk.ChatMessageRoleUser, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "hi"})}})
			require.True(t, model.loading)
			require.Nil(t, model.err)
			model.streaming = true
			chat := testChat(codersdk.ChatStatusCompleted)
			model, _ = model.Update(chatOpenedMsg{chat: chat})
			require.False(t, model.loading)
			require.Nil(t, model.err)
			require.Len(t, model.messages, 1)
		})
		t.Run("OpenHistoryBothFail", func(t *testing.T) {
			t.Parallel()
			model := setup(false, false)
			model, _ = model.Update(chatOpenedMsg{err: xerrors.New("open err")})
			require.True(t, model.loading)
			model, _ = model.Update(chatHistoryMsg{err: xerrors.New("history err")})
			require.False(t, model.loading)
			require.Equal(t, "open err", model.err.Error())
		})
		t.Run("StaleAsyncMessagesAreDropped", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			chat := testChat(codersdk.ChatStatusCompleted)
			model.setChat(chat)
			model.chatGeneration = 1
			model.loading = false

			before := model
			updated, cmd := model.Update(chatOpenedMsg{
				generation: 0,
				chatID:     uuid.New(),
				chat:       testChat(codersdk.ChatStatusRunning),
			})
			require.Nil(t, cmd)
			require.Equal(t, before.chat, updated.chat)
			require.Equal(t, before.loading, updated.loading)
			require.Equal(t, before.messages, updated.messages)
			require.Equal(t, before.err, updated.err)
		})

		t.Run("StaleSessionMessagesAreDroppedByGeneration", func(t *testing.T) {
			t.Parallel()
			type staleGenerationCase struct {
				name  string
				msg   tea.Msg
				draft bool
			}
			type staleGenerationSnapshot struct {
				loading             bool
				err                 error
				chat                *codersdk.Chat
				pendingComposerText string
				composerValue       string
				messages            []codersdk.ChatMessage
				draft               bool
				creatingChat        bool
				interrupting        bool
				queuedMessages      []codersdk.ChatQueuedMessage
			}

			startingState := func(draft bool) chatViewModel {
				model := newTestChatViewModel(nil)
				model.chatGeneration = 2
				model.loading = false
				model.pendingComposerText = "pending"
				if draft {
					model.draft = true
					model.composer.SetValue("draft text")
					return model
				}
				model.creatingChat = true
				model.interrupting = true
				model.setChat(testChat(codersdk.ChatStatusCompleted))
				model.composer.SetValue("current")
				return model
			}
			snapshot := func(model chatViewModel) staleGenerationSnapshot {
				return staleGenerationSnapshot{
					loading:             model.loading,
					err:                 model.err,
					chat:                model.chat,
					pendingComposerText: model.pendingComposerText,
					composerValue:       model.composer.Value(),
					messages:            model.messages,
					draft:               model.draft,
					creatingChat:        model.creatingChat,
					interrupting:        model.interrupting,
					queuedMessages:      model.queuedMessages,
				}
			}

			tests := []staleGenerationCase{
				{name: "WriteSide/chatCreatedMsg", msg: chatCreatedMsg{generation: 1, chat: testChat(codersdk.ChatStatusRunning)}},
				{name: "WriteSide/messageSentMsg", msg: messageSentMsg{generation: 1, resp: codersdk.CreateChatMessageResponse{}}},
				{name: "WriteSide/chatInterruptedMsg", msg: chatInterruptedMsg{generation: 1, chat: testChat(codersdk.ChatStatusCompleted)}},
				{name: "Draft/chatOpenedMsg", msg: chatOpenedMsg{generation: 1, chatID: uuid.New(), chat: testChat(codersdk.ChatStatusCompleted)}, draft: true},
				{name: "Draft/chatHistoryMsg", msg: chatHistoryMsg{generation: 1, chatID: uuid.New(), messages: []codersdk.ChatMessage{testMessage(1, codersdk.ChatMessageRoleUser, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "hi"})}}, draft: true},
				{name: "Draft/chatStreamEventMsg", msg: chatStreamEventMsg{generation: 1, chatID: uuid.New(), event: testTextPartEvent("stale")}, draft: true},
				{name: "Draft/diffContentsMsg", msg: diffContentsMsg{generation: 1, chatID: uuid.New()}, draft: true},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					model := startingState(tt.draft)
					before := snapshot(model)
					updated, cmd := model.Update(tt.msg)
					require.Nil(t, cmd)
					require.Equal(t, before, snapshot(updated))
				})
			}
		})

		t.Run("ErrorThenRetrySucceeds", func(t *testing.T) {
			t.Parallel()
			tests := []struct {
				name         string
				errMsg       tea.Msg
				retryMsg     tea.Msg
				needsClient  bool
				composerText string
				wantBlocks   int
				wantRetryCmd bool
			}{
				{name: "ChatOpened", errMsg: chatOpenedMsg{err: xerrors.New("open failed")}, retryMsg: chatOpenedMsg{chat: testChat(codersdk.ChatStatusRunning)}},
				{name: "History", errMsg: chatHistoryMsg{err: xerrors.New("history failed")}, retryMsg: chatHistoryMsg{messages: []codersdk.ChatMessage{testMessage(1, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "recovered"})}}, wantBlocks: 1},
				{name: "Send", needsClient: true, composerText: "keep me", errMsg: messageSentMsg{err: xerrors.New("send failed")}, wantRetryCmd: true},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					model := newTestChatViewModel(nil)
					if tt.needsClient {
						model = newTestChatViewModel(failingExperimentalClient())
						model.loading = false
						chat := testChat(codersdk.ChatStatusCompleted)
						model.chat = &chat
						model.chatStatus = chat.Status
						model.composer.SetValue(tt.composerText)
					}
					updated, cmd := model.Update(tt.errMsg)
					require.Nil(t, cmd)
					require.Error(t, updated.err)
					if tt.retryMsg != nil {
						updated, cmd = updated.Update(tt.retryMsg)
						require.Nil(t, updated.err)
						switch retryMsg := tt.retryMsg.(type) {
						case chatOpenedMsg:
							require.NotNil(t, cmd)
							require.NotNil(t, updated.chat)
							require.Equal(t, retryMsg.chat.ID, updated.chat.ID)
						case chatHistoryMsg:
							require.Nil(t, cmd)
							require.Equal(t, retryMsg.messages, updated.messages)
							require.Len(t, updated.blocks, tt.wantBlocks)
						}
					}
					if !tt.wantRetryCmd {
						return
					}
					require.Equal(t, tt.composerText, updated.composer.Value())
					require.Contains(t, updated.View(), "send failed")
					updated.composer.SetValue("retry me")
					retried, retryCmd := updated.sendMessage()
					require.NotNil(t, retryCmd)
					require.True(t, retried.autoFollow)
					require.Empty(t, retried.composer.Value())
					_, ok := mustMsg(t, retryCmd).(messageSentMsg)
					require.True(t, ok)
				})
			}
		})
		t.Run("ChatHistoryEdgeCases", func(t *testing.T) {
			t.Parallel()
			cases := []struct {
				name     string
				messages []codersdk.ChatMessage
				wantNil  bool
			}{
				{name: "NilMessages", wantNil: true},
				{name: "EmptyMessages", messages: []codersdk.ChatMessage{}, wantNil: false},
			}
			for _, tt := range cases {
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
						updated, _ = model.Update(chatHistoryMsg{messages: tt.messages})
					})
					require.Equal(t, tt.wantNil, updated.messages == nil)
					if !tt.wantNil {
						require.Empty(t, updated.messages)
					}
					require.Empty(t, updated.blocks)
				})
			}
		})
	})

	t.Run("ChatView/StreamEvents", func(t *testing.T) {
		t.Parallel()
		applyStream := func(model chatViewModel, event codersdk.ChatStreamEvent) (chatViewModel, tea.Cmd) {
			return model.Update(chatStreamEventMsg{event: event})
		}
		messageEvent := func(message codersdk.ChatMessage) codersdk.ChatStreamEvent {
			return codersdk.ChatStreamEvent{Type: codersdk.ChatStreamEventTypeMessage, Message: &message}
		}
		usage := &codersdk.ChatMessageUsage{OutputTokens: int64Ref(7)}
		finalMessage := testMessage(9, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "final"})
		finalMessage.Usage = usage
		for _, tt := range []struct {
			name                string
			seedEvents          []codersdk.ChatStreamEvent
			reconnecting        bool
			event               codersdk.ChatStreamEvent
			wantMessages        int
			wantAccumulatorText string
			wantAccumulatorArgs string
			wantBlockKind       chatBlockKind
			wantBlockText       string
			wantBlockArgs       string
			wantUsage           *codersdk.ChatMessageUsage
		}{
			{
				name:                "MessagePartTextAppendsAndRebuildsBlocks",
				seedEvents:          []codersdk.ChatStreamEvent{testTextPartEvent("hel")},
				event:               testTextPartEvent("lo"),
				wantAccumulatorText: "hello",
				wantBlockText:       "hello",
			},
			{
				name:                "MessagePartToolCallDeltaAccumulatesArgs",
				seedEvents:          []codersdk.ChatStreamEvent{testToolCallDeltaEvent("tc-1", "search", `{"q":"hel`)},
				event:               testToolCallDeltaEvent("tc-1", "search", `lo"}`),
				wantAccumulatorArgs: `{"q":"hello"}`,
				wantBlockKind:       blockToolCall,
				wantBlockArgs:       `{"q":"hello"}`,
			},
			{
				name:          "MessageFinalizesAndResetsAccumulator",
				seedEvents:    []codersdk.ChatStreamEvent{testTextPartEvent("partial")},
				reconnecting:  true,
				event:         messageEvent(finalMessage),
				wantMessages:  1,
				wantBlockText: "final",
				wantUsage:     usage,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				model := newTestChatViewModel(nil)
				model.reconnecting = tt.reconnecting
				for _, event := range tt.seedEvents {
					model, _ = applyStream(model, event)
				}
				var cmd tea.Cmd
				model, cmd = applyStream(model, tt.event)
				require.Nil(t, cmd)
				assertStreamCase(t, model, tt.wantMessages, tt.wantAccumulatorText, tt.wantAccumulatorArgs, tt.wantBlockKind, tt.wantBlockText, tt.wantBlockArgs, tt.wantUsage)
			})
		}
		t.Run("StatusEventRouting", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			chat := testChat(codersdk.ChatStatusWaiting)
			model.chat, model.activeChatID, model.chatStatus = &chat, chat.ID, chat.Status
			updated, cmd := model.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:   codersdk.ChatStreamEventTypeStatus,
				ChatID: chat.ID,
				Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatusRunning},
			}})
			require.NotNil(t, cmd)
			require.Equal(t, codersdk.ChatStatusRunning, updated.chatStatus)
			require.Equal(t, codersdk.ChatStatusRunning, updated.chat.Status)
			chat.Status = codersdk.ChatStatusWaiting
			model.chatStatus = codersdk.ChatStatusWaiting
			updated, cmd = model.Update(chatStreamEventMsg{event: codersdk.ChatStreamEvent{
				Type:   codersdk.ChatStreamEventTypeStatus,
				ChatID: uuid.New(),
				Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatusRunning},
			}})
			require.Nil(t, cmd)
			require.Equal(t, codersdk.ChatStatusWaiting, updated.chatStatus)
			require.Equal(t, codersdk.ChatStatusWaiting, updated.chat.Status)
		})
		t.Run("ErrorSetsErr", func(t *testing.T) {
			t.Parallel()
			updated, cmd := applyStream(newTestChatViewModel(nil), codersdk.ChatStreamEvent{
				Type:  codersdk.ChatStreamEventTypeError,
				Error: &codersdk.ChatError{Message: "stream blew up"},
			})
			require.Nil(t, cmd)
			require.Equal(t, "stream error: stream blew up", updated.err.Error())
		})
		queuedMessages := []codersdk.ChatQueuedMessage{
			testQueuedMessage(1, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "queued text"}),
		}
		existingMessages := []codersdk.ChatMessage{
			testMessage(1, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "existing"}),
		}
		for _, tt := range []struct {
			name               string
			messages           []codersdk.ChatMessage
			event              codersdk.ChatStreamEvent
			wantMessages       []codersdk.ChatMessage
			wantQueuedMessages []codersdk.ChatQueuedMessage
			wantBlockText      string
		}{
			{
				name:               "QueueUpdateReplacesQueuedMessages",
				event:              codersdk.ChatStreamEvent{Type: codersdk.ChatStreamEventTypeQueueUpdate, QueuedMessages: queuedMessages},
				wantQueuedMessages: queuedMessages,
				wantBlockText:      "queued text",
			},
			{
				name:          "StreamEventWithNilPartIsIgnored",
				messages:      existingMessages,
				event:         codersdk.ChatStreamEvent{Type: codersdk.ChatStreamEventTypeMessagePart},
				wantMessages:  existingMessages,
				wantBlockText: "existing",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				model := newTestChatViewModel(nil)
				model.messages = tt.messages
				model.rebuildBlocks()
				updated, cmd := applyStream(model, tt.event)
				require.Nil(t, cmd)
				model = updated
				require.Equal(t, tt.wantMessages, model.messages)
				require.Equal(t, tt.wantQueuedMessages, model.queuedMessages)
				require.Len(t, model.blocks, 1)
				require.Equal(t, tt.wantBlockText, model.blocks[0].text)
				require.False(t, model.accumulator.isPending())
			})
		}
		t.Run("StreamEventErrorShowsInView", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			model = mustChatViewUpdate(t, model, tea.WindowSizeMsg{Width: 120, Height: 12})
			model.loading = false
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat = &chat
			model.chatStatus = chat.Status
			model.messages = []codersdk.ChatMessage{
				testMessage(1, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "existing response"}),
			}
			model.rebuildBlocks()
			updated := mustChatViewUpdate(t, model, chatStreamEventMsg{err: xerrors.New("websocket closed")})
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
			model = mustChatViewUpdate(t, model, tea.WindowSizeMsg{Width: 120, Height: 12})
			view := plainText(model.View())
			require.Contains(t, view, "New Chat (draft)")
			require.Contains(t, view, "Loading chat...")
			require.Contains(t, view, "Type a message")
			require.Contains(t, view, "esc: back")
		})
		t.Run("MultipleStreamErrorsOnlyShowLatest", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			model = mustChatViewUpdate(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})
			model.loading = false
			updated := mustChatViewUpdate(t, model, chatStreamEventMsg{err: xerrors.New("first error")})
			updated = mustChatViewUpdate(t, updated, chatStreamEventMsg{err: xerrors.New("second error")})
			view := updated.View()
			require.Contains(t, view, "second error")
			require.NotContains(t, view, "first error")
		})
		t.Run("StreamAccumulatorFinalToolCallUpsertsExistingPart", func(t *testing.T) {
			t.Parallel()
			newToolCallDelta := func(toolCallID, toolName, argsDelta string) codersdk.ChatStreamMessagePart {
				return codersdk.ChatStreamMessagePart{
					Role: codersdk.ChatMessageRoleAssistant,
					Part: codersdk.ChatMessagePart{
						Type:       codersdk.ChatMessagePartTypeToolCall,
						ToolCallID: toolCallID,
						ToolName:   toolName,
						ArgsDelta:  argsDelta,
					},
				}
			}
			newFinalToolCall := func(toolCallID, toolName, args string) codersdk.ChatStreamMessagePart {
				return codersdk.ChatStreamMessagePart{
					Role: codersdk.ChatMessageRoleAssistant,
					Part: codersdk.ChatMessagePart{
						Type:       codersdk.ChatMessagePartTypeToolCall,
						ToolCallID: toolCallID,
						ToolName:   toolName,
						Args:       json.RawMessage(args),
					},
				}
			}
			cases := []struct {
				name  string
				seed  []codersdk.ChatStreamMessagePart
				final codersdk.ChatStreamMessagePart
				want  []codersdk.ChatMessagePart
			}{
				{
					name: "ReplaceExistingToolCall",
					seed: []codersdk.ChatStreamMessagePart{
						newToolCallDelta("tc-1", "search", `{"q":"hel`),
					},
					final: newFinalToolCall("tc-1", "search", `{"q":"hello"}`),
					want: []codersdk.ChatMessagePart{{
						Type:       codersdk.ChatMessagePartTypeToolCall,
						ToolCallID: "tc-1",
						ToolName:   "search",
						Args:       json.RawMessage(`{"q":"hello"}`),
					}},
				},
				{
					name: "AppendNewToolCallID",
					seed: []codersdk.ChatStreamMessagePart{
						newToolCallDelta("tc-1", "search", `{"q":"hel`),
					},
					final: newFinalToolCall("tc-2", "lookup", `{"id":"42"}`),
					want: []codersdk.ChatMessagePart{
						{
							Type:       codersdk.ChatMessagePartTypeToolCall,
							ToolCallID: "tc-1",
							ToolName:   "search",
							Args:       json.RawMessage(`{"q":"hel`),
						},
						{
							Type:       codersdk.ChatMessagePartTypeToolCall,
							ToolCallID: "tc-2",
							ToolName:   "lookup",
							Args:       json.RawMessage(`{"id":"42"}`),
						},
					},
				},
			}
			for _, tt := range cases {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					var accumulator streamAccumulator
					for _, delta := range tt.seed {
						accumulator.applyDelta(delta)
					}
					accumulator.applyDelta(tt.final)
					require.True(t, accumulator.pending)
					require.Equal(t, codersdk.ChatMessageRoleAssistant, accumulator.role)
					require.Equal(t, tt.want, accumulator.parts)
				})
			}
		})
		t.Run("MessageDeduplication", func(t *testing.T) {
			t.Parallel()
			toolRoundTripParts := []codersdk.ChatMessagePart{
				{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: "tool-1", ToolName: "search", Args: json.RawMessage(`{"q":"hello"}`)},
				{Type: codersdk.ChatMessagePartTypeToolResult, ToolCallID: "tool-1", ToolName: "search", Result: json.RawMessage(`{"ok":true}`)},
			}
			model := newTestChatViewModel(nil)
			model.messages = []codersdk.ChatMessage{testMessage(1, codersdk.ChatMessageRoleAssistant, toolRoundTripParts...)}
			model.accumulator = streamAccumulator{pending: true, role: codersdk.ChatMessageRoleAssistant, parts: toolRoundTripParts}
			model.rebuildBlocks()
			require.Len(t, model.messages, 1)
			require.Len(t, model.blocks, 1)
			require.True(t, model.accumulator.isPending())
			require.Equal(t, blockToolResult, model.blocks[0].kind)
			require.Equal(t, "tool-1", model.blocks[0].toolID)
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
			require.NotNil(t, cmd)
			require.False(t, updated.streaming)
			require.True(t, updated.reconnecting)
		})
		t.Run("MessageEventsDeduplicateByID", func(t *testing.T) {
			t.Parallel()
			message := testMessage(11, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "hello"})
			model, _ := applyStream(newTestChatViewModel(nil), messageEvent(message))
			model, cmd := applyStream(model, messageEvent(message))
			require.Nil(t, cmd)
			require.Len(t, model.messages, 1)
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
			client := newTestExperimentalClient(t, func(rw http.ResponseWriter, req *http.Request) {
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
			})

			model := newTestChatViewModel(client)
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

		t.Run("SendCreateErrorHandling", func(t *testing.T) {
			t.Parallel()
			tests := []struct {
				name, composerText, wantComposer                 string
				draft, setChat, useSend, typeNewInput, wantRetry bool
				errMsg                                           tea.Msg
			}{
				{name: "send preserves existing composer text", composerText: "keep me", wantComposer: "keep me", errMsg: messageSentMsg{err: xerrors.New("send failed")}},
				{name: "send restores pending text", composerText: "my message", wantComposer: "my message", setChat: true, useSend: true, errMsg: messageSentMsg{err: xerrors.New("network error")}},
				{name: "create restores pending text", composerText: "first message", wantComposer: "first message", draft: true, useSend: true, errMsg: chatCreatedMsg{err: xerrors.New("create failed")}},
				{name: "create error allows retry", composerText: "keep draft", wantComposer: "keep draft", draft: true, wantRetry: true, errMsg: chatCreatedMsg{err: xerrors.New("create failed")}},
				{name: "messageSent error does not overwrite newer input", composerText: "original", wantComposer: "new input", setChat: true, useSend: true, typeNewInput: true, errMsg: messageSentMsg{err: xerrors.New("fail")}},
				{name: "chatCreated error does not overwrite newer input", composerText: "original", wantComposer: "new input", draft: true, useSend: true, typeNewInput: true, errMsg: chatCreatedMsg{err: xerrors.New("fail")}},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					var client *codersdk.ExperimentalClient
					if tt.wantRetry {
						client = failingExperimentalClient()
					}
					model := newTestChatViewModel(client)
					model.loading = false
					if tt.draft {
						model.draft = true
					}
					if tt.setChat {
						chat := testChat(codersdk.ChatStatusCompleted)
						model.setChat(chat)
					}
					model.composer.SetValue(tt.composerText)
					if tt.useSend {
						model, _ = model.sendMessage()
						require.Empty(t, model.composer.Value())
						require.Equal(t, tt.composerText, model.pendingComposerText)
						if tt.draft {
							require.True(t, model.creatingChat)
						}
					}
					if tt.typeNewInput {
						model.composer.SetValue("new input")
					}
					updated, cmd := model.Update(tt.errMsg)
					require.Nil(t, cmd)
					model = updated
					require.Error(t, model.err)
					switch msg := tt.errMsg.(type) {
					case messageSentMsg:
						require.Equal(t, msg.err.Error(), model.err.Error())
					case chatCreatedMsg:
						require.Equal(t, msg.err.Error(), model.err.Error())
					}
					require.Equal(t, tt.wantComposer, model.composer.Value())
					if tt.wantRetry {
						require.True(t, model.draft)
						require.Contains(t, model.View(), "create failed")
						model.composer.SetValue("retry draft")
						retried, retryCmd := model.sendMessage()
						require.NotNil(t, retryCmd)
						require.True(t, retried.draft)
						require.Empty(t, retried.composer.Value())
						_, ok := mustMsg(t, retryCmd).(chatCreatedMsg)
						require.True(t, ok)
					}
					if tt.draft && tt.useSend {
						require.False(t, model.creatingChat)
					}
				})
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
	})

	t.Run("ChatView/ModelOverrideMapsCanonicalModelID", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			draft bool
		}{
			{name: "DraftCreateReturnsChatCreatedMsg", draft: true},
			{name: "SendMessageReturnsMessageSentMsg", draft: false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				modelConfigID := uuid.New()
				organizationID := uuid.New()
				modelOverride := "provider:model"
				createdChat := testChat(codersdk.ChatStatusWaiting)
				chat := testChat(codersdk.ChatStatusCompleted)
				var createReq *codersdk.CreateChatRequest
				var messageReq *codersdk.CreateChatMessageRequest
				client := newTestExperimentalClient(t, func(rw http.ResponseWriter, req *http.Request) {
					rw.Header().Set("Content-Type", "application/json")
					switch {
					case req.Method == http.MethodGet && req.URL.Path == "/api/experimental/chats/model-configs":
						require.NoError(t, json.NewEncoder(rw).Encode([]codersdk.ChatModelConfig{{ID: modelConfigID, Provider: "provider", Model: "model"}}))
					case req.Method == http.MethodPost && req.URL.Path == "/api/experimental/chats":
						createReq = new(codersdk.CreateChatRequest)
						require.NoError(t, json.NewDecoder(req.Body).Decode(createReq))
						rw.WriteHeader(http.StatusCreated)
						require.NoError(t, json.NewEncoder(rw).Encode(createdChat))
					case req.Method == http.MethodPost && req.URL.Path == fmt.Sprintf("/api/experimental/chats/%s/messages", chat.ID):
						messageReq = new(codersdk.CreateChatMessageRequest)
						require.NoError(t, json.NewDecoder(req.Body).Decode(messageReq))
						require.NoError(t, json.NewEncoder(rw).Encode(codersdk.CreateChatMessageResponse{}))
					default:
						t.Fatalf("unexpected %s %s", req.Method, req.URL.Path)
					}
				})
				model := newTestChatViewModel(client)
				if tt.draft {
					model.draft = true
				} else {
					model.setChat(chat)
				}
				model.loading = false
				model.modelOverride = &modelOverride
				model.organizationID = organizationID
				model.planMode = codersdk.ChatPlanModePlan
				model.composer.SetValue("hello")
				updated, cmd := model.sendMessage()
				require.NotNil(t, cmd)
				require.Empty(t, updated.composer.Value())
				if tt.draft {
					msg, ok := mustMsg(t, cmd).(chatCreatedMsg)
					require.True(t, ok)
					require.NoError(t, msg.err)
					require.NotNil(t, createReq)
					require.Equal(t, organizationID, createReq.OrganizationID)
					require.NotNil(t, createReq.ModelConfigID)
					require.Equal(t, modelConfigID, *createReq.ModelConfigID)
					require.Equal(t, codersdk.ChatPlanModePlan, createReq.PlanMode)
					require.Equal(t, createdChat.ID, msg.chat.ID)
					return
				}
				msg, ok := mustMsg(t, cmd).(messageSentMsg)
				require.True(t, ok)
				require.NoError(t, msg.err)
				require.NotNil(t, messageReq)
				require.NotNil(t, messageReq.ModelConfigID)
				require.Equal(t, modelConfigID, *messageReq.ModelConfigID)
				require.NotNil(t, messageReq.PlanMode)
				require.Equal(t, codersdk.ChatPlanModePlan, *messageReq.PlanMode)
			})
		}
	})
	t.Run("ChatView/SendMessageExplicitlyClearsPlanMode", func(t *testing.T) {
		t.Parallel()
		chat := testChat(codersdk.ChatStatusCompleted)
		var messageReq *codersdk.CreateChatMessageRequest
		client := newTestExperimentalClient(t, func(rw http.ResponseWriter, req *http.Request) {
			rw.Header().Set("Content-Type", "application/json")
			switch {
			case req.Method == http.MethodPost && req.URL.Path == fmt.Sprintf("/api/experimental/chats/%s/messages", chat.ID):
				messageReq = new(codersdk.CreateChatMessageRequest)
				require.NoError(t, json.NewDecoder(req.Body).Decode(messageReq))
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.CreateChatMessageResponse{}))
			default:
				t.Fatalf("unexpected %s %s", req.Method, req.URL.Path)
			}
		})
		model := newTestChatViewModel(client)
		model.setChat(chat)
		model.loading = false
		model.composer.SetValue("hello")

		updated, cmd := model.sendMessage()
		require.NotNil(t, cmd)
		require.Empty(t, updated.composer.Value())

		msg, ok := mustMsg(t, cmd).(messageSentMsg)
		require.True(t, ok)
		require.NoError(t, msg.err)
		require.NotNil(t, messageReq)
		require.NotNil(t, messageReq.PlanMode)
		require.Empty(t, *messageReq.PlanMode)
	})

	t.Run("ChatView/ChatCreatedPromotesDraft", func(t *testing.T) {
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

	t.Run("ChatView/Interrupts", func(t *testing.T) {
		t.Parallel()
		newInterruptModel := func(status codersdk.ChatStatus) chatViewModel {
			model := newTestChatViewModel(failingExperimentalClient())
			model.setChat(testChat(status))
			return model
		}

		t.Run("InterruptedChatClearsInterruptingAndUpdatesStatus", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			model.interrupting = true
			chat := testChat(codersdk.ChatStatusCompleted)

			updated, cmd := model.Update(chatInterruptedMsg{chat: chat})
			require.Nil(t, cmd)
			require.False(t, updated.interrupting)
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
			require.Equal(t, "interrupt failed", updated.err.Error())
		})

		tests := []struct {
			name                 string
			chatStatus           codersdk.ChatStatus
			alreadyInterrupting  bool
			expectedInterrupting bool
		}{
			{name: "DoubleInterrupt", chatStatus: codersdk.ChatStatusRunning, alreadyInterrupting: true, expectedInterrupting: true},
			{name: "IdleChat", chatStatus: codersdk.ChatStatusCompleted},
		}
		for _, tt := range tests {
			t.Run("CtrlXNoOpCases/"+tt.name, func(t *testing.T) {
				t.Parallel()
				model := newInterruptModel(tt.chatStatus)
				model.interrupting = tt.alreadyInterrupting

				updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
				require.Nil(t, cmd)
				require.Equal(t, tt.expectedInterrupting, updated.interrupting)
			})
		}

		t.Run("CtrlXInterruptsRunningChat", func(t *testing.T) {
			t.Parallel()
			model := newInterruptModel(codersdk.ChatStatusRunning)
			require.True(t, model.composerFocused)

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
			require.NotNil(t, cmd)
			require.True(t, updated.interrupting)
			require.True(t, updated.composerFocused)
		})

		t.Run("TabKeepsFocusSwitchBehaviorWhileRunningChat", func(t *testing.T) {
			t.Parallel()
			model := newInterruptModel(codersdk.ChatStatusRunning)
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
			model.setChat(testChat(codersdk.ChatStatusRunning))
			model.loading = false

			view := plainText(model.View())
			require.Contains(t, view, "ctrl+x: interrupt")
			require.NotContains(t, view, "ctrl+i: interrupt")
		})

		t.Run("ViewShowsPlanModeBadge", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			model, _ = model.Update(tea.WindowSizeMsg{Width: 140, Height: 12})
			model.loading = false
			execView := model.View()
			require.Contains(t, plainText(execView), "mode: exec")
			require.Contains(t, execView, model.styles.modeBadgeExec.Render("exec"))

			model.planMode = codersdk.ChatPlanModePlan
			planView := model.View()
			view := plainText(planView)
			require.Contains(t, view, "mode: plan")
			require.Contains(t, planView, model.styles.modeBadgePlan.Render("plan"))
			require.Contains(t, view, "shift+tab: switch mode")
		})

		t.Run("PlanModeUpdateErrorRollsBackLocalModeAndShowsBanner", func(t *testing.T) {
			t.Parallel()

			for _, tt := range []struct {
				name    string
				current codersdk.ChatPlanMode
				want    codersdk.ChatPlanMode
			}{
				{name: "BackToCode", current: codersdk.ChatPlanModePlan, want: ""},
				{name: "BackToPlan", current: "", want: codersdk.ChatPlanModePlan},
			} {
				tt := tt
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					model := newTestChatViewModel(nil)
					model, _ = model.Update(tea.WindowSizeMsg{Width: 140, Height: 12})
					model.setChat(testChat(codersdk.ChatStatusCompleted))
					model.planMode = tt.current

					updated, cmd := model.Update(chatPlanModeUpdatedMsg{err: xerrors.New("update failed")})
					require.Nil(t, cmd)
					require.Equal(t, tt.want, updated.planMode)
					require.EqualError(t, updated.err, "update failed")
					require.Contains(t, plainText(updated.View()), "update failed")
				})
			}
		})
	})

	t.Run("ChatView/Keyboard", func(t *testing.T) {
		t.Parallel()
		t.Run("KeyboardShortcutRouting", func(t *testing.T) {
			t.Parallel()
			isToggleModelPicker := func(msg tea.Msg) bool { _, ok := msg.(toggleModelPickerMsg); return ok }
			isToggleDiffDrawer := func(msg tea.Msg) bool { _, ok := msg.(toggleDiffDrawerMsg); return ok }
			tests := []struct {
				name            string
				key             tea.KeyType
				composerFocused bool
				composerValue   string
				assert          func(tea.Msg) bool
			}{
				{name: "CtrlP/Focused", key: tea.KeyCtrlP, composerFocused: true, composerValue: "draft", assert: isToggleModelPicker},
				{name: "CtrlP/Unfocused", key: tea.KeyCtrlP, assert: isToggleModelPicker},
				{name: "CtrlD/Focused", key: tea.KeyCtrlD, composerFocused: true, composerValue: "draft", assert: isToggleDiffDrawer},
				{name: "CtrlD/Unfocused", key: tea.KeyCtrlD, assert: isToggleDiffDrawer},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					model := newTestChatViewModel(nil)
					model.composerFocused = tt.composerFocused
					model.composer.SetValue(tt.composerValue)

					updated, cmd := model.Update(tea.KeyMsg{Type: tt.key})
					require.NotNil(t, cmd)
					require.Equal(t, tt.composerFocused, updated.composerFocused)
					require.Equal(t, tt.composerValue, updated.composer.Value())
					require.True(t, tt.assert(mustMsg(t, cmd)))
				})
			}
		})

		t.Run("ShiftTabTogglesPlanMode", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			model.composer.SetValue("draft")

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
			require.Nil(t, cmd)
			require.Equal(t, codersdk.ChatPlanModePlan, updated.planMode)
			require.Equal(t, "draft", updated.composer.Value())

			updated, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
			require.Nil(t, cmd)
			require.Empty(t, updated.planMode)
			require.Equal(t, "draft", updated.composer.Value())
		})

		t.Run("TabOnlySwitchesComposerFocus", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			model.planMode = codersdk.ChatPlanModePlan

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyTab})
			require.Nil(t, cmd)
			require.Equal(t, codersdk.ChatPlanModePlan, updated.planMode)
			require.False(t, updated.composerFocused)
		})

		t.Run("ShiftTabDraftChatDefersPlanModePersistence", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			model.draft = true

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
			require.Nil(t, cmd)
			require.Equal(t, codersdk.ChatPlanModePlan, updated.planMode)
		})

		t.Run("ShiftTabExistingChatUpdatesPlanModeImmediately", func(t *testing.T) {
			t.Parallel()
			chat := testChat(codersdk.ChatStatusCompleted)
			var requests []codersdk.UpdateChatRequest
			client := newTestExperimentalClient(t, func(rw http.ResponseWriter, req *http.Request) {
				switch {
				case req.Method == http.MethodPatch && req.URL.Path == fmt.Sprintf("/api/experimental/chats/%s", chat.ID):
					var updateReq codersdk.UpdateChatRequest
					require.NoError(t, json.NewDecoder(req.Body).Decode(&updateReq))
					requests = append(requests, updateReq)
					rw.WriteHeader(http.StatusNoContent)
				default:
					t.Fatalf("unexpected %s %s", req.Method, req.URL.Path)
				}
			})
			model := newTestChatViewModel(client)
			model.setChat(chat)

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
			require.NotNil(t, cmd)
			require.Equal(t, codersdk.ChatPlanModePlan, updated.planMode)

			msg, ok := mustMsg(t, cmd).(chatPlanModeUpdatedMsg)
			require.True(t, ok)
			require.NoError(t, msg.err)
			require.Len(t, requests, 1)
			require.NotNil(t, requests[0].PlanMode)
			require.Equal(t, codersdk.ChatPlanModePlan, *requests[0].PlanMode)

			updated, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
			require.NotNil(t, cmd)
			require.Empty(t, updated.planMode)

			msg, ok = mustMsg(t, cmd).(chatPlanModeUpdatedMsg)
			require.True(t, ok)
			require.NoError(t, msg.err)
			require.Len(t, requests, 2)
			require.NotNil(t, requests[1].PlanMode)
			require.Empty(t, *requests[1].PlanMode)
		})

		t.Run("CtrlPFromListViewDoesNotOpenModelPicker", func(t *testing.T) {
			t.Parallel()
			model := newTestTUIModel()
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
		applyWindowSize := func(t *testing.T, model chatsTUIModel, width int, height int) chatsTUIModel {
			t.Helper()
			updatedModel, cmd := model.Update(tea.WindowSizeMsg{Width: width, Height: height})
			return mustTUIModel(t, updatedModel, cmd)
		}
		scrollableModel := func(t *testing.T, keys ...tea.KeyType) chatViewModel {
			t.Helper()
			model := newTestChatViewModel(nil)
			model.loading = false
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat = &chat
			model.chatStatus = chat.Status
			model = mustChatViewUpdate(t, model, tea.WindowSizeMsg{Width: 80, Height: 20})
			model.messages = overflowingMessages(24)
			model.rebuildBlocks()
			model = mustChatViewUpdate(t, model, tea.KeyMsg{Type: tea.KeyTab})
			require.False(t, model.composerFocused)
			require.True(t, model.autoFollow)
			require.True(t, model.viewport.AtBottom())
			require.Greater(t, model.viewport.YOffset, 0)
			for _, key := range keys {
				model = mustChatViewUpdate(t, model, tea.KeyMsg{Type: key})
			}
			return model
		}
		streamMessage := func(id int64) chatStreamEventMsg {
			message := testMessage(
				id,
				codersdk.ChatMessageRoleAssistant,
				codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: strings.Repeat("new content ", 24)},
			)
			return chatStreamEventMsg{event: codersdk.ChatStreamEvent{Type: codersdk.ChatStreamEventTypeMessage, Message: &message}}
		}
		updateView := func(model chatViewModel, msg tea.Msg) chatViewModel {
			updated, _ := model.Update(msg)
			return updated
		}
		t.Run("ViewportHeights", func(t *testing.T) {
			t.Parallel()
			tests := []struct {
				name               string
				height             int
				viewChat           bool
				messageCount       int
				wantChatHeight     int
				wantViewportHeight int
			}{
				{"Standard", 40, false, 0, 39, 33},
				{"MinimumZero", 5, false, 0, -1, 0},
				{"ViewFitsTerminal", 40, true, 24, -1, -1},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					model := applyWindowSize(t, newTestTUIModel(), 80, tt.height)
					if tt.viewChat {
						model.currentView = viewChat
						model.chat.loading = false
						model.chat, _ = model.chat.Update(model.childWindowSizeMsg())
						chat := testChat(codersdk.ChatStatusCompleted)
						model.chat.chat, model.chat.chatStatus = &chat, chat.Status
						model.chat.messages = overflowingMessages(tt.messageCount)
						model.chat.rebuildBlocks()
						require.LessOrEqual(t, strings.Count(model.View(), "\n")+1, tt.height)
						return
					}
					if tt.wantChatHeight >= 0 {
						require.Equal(t, tt.wantChatHeight, model.chat.height)
					}
					if tt.wantViewportHeight >= 0 {
						require.Equal(t, tt.wantViewportHeight, model.chat.viewport.Height)
					}
				})
			}
		})
		t.Run("WrappedComposerFitsTerminal", func(t *testing.T) {
			t.Parallel()
			model := applyWindowSize(t, newTestTUIModel(), 40, 18)
			model.currentView = viewChat
			model.chat.loading = false
			model.chat, _ = model.chat.Update(model.childWindowSizeMsg())
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat.chat = &chat
			model.chat.chatStatus = chat.Status
			model.chat.messages = overflowingMessages(18)
			model.chat.rebuildBlocks()
			initialViewportHeight := model.chat.viewport.Height
			model.chat.composer.SetValue(strings.Repeat("wrapped input ", 14))
			model.chat.recalcViewportHeight()
			model.chat.syncViewportContent()
			view := plainText(model.View())
			lines := strings.Split(view, "\n")
			require.LessOrEqual(t, model.chat.viewport.Height, initialViewportHeight)
			require.LessOrEqual(t, len(lines), 18)
			require.NotEmpty(t, strings.TrimSpace(lines[len(lines)-1]))
		})
		t.Run("ViewShowsSingleStatusBarAndComposerDivider", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			model.loading = false
			model = mustChatViewUpdate(t, model, tea.WindowSizeMsg{Width: 60, Height: 14})
			chat := testChat(codersdk.ChatStatusWaiting)
			model.chat = &chat
			model.chatStatus = chat.Status
			model.messages = []codersdk.ChatMessage{
				testMessage(1, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "existing response"}),
			}
			model.rebuildBlocks()
			view := plainText(model.View())
			require.NotContains(t, view, "Status: waiting")
			require.Equal(t, 1, strings.Count(view, "waiting"))
			lines := strings.Split(view, "\n")
			composerLine := -1
			for i, line := range lines {
				if strings.Contains(line, "> ") {
					composerLine = i
					break
				}
			}
			require.Greater(t, composerLine, 1)
			require.Contains(t, lines[composerLine-1], "────")
		})
		t.Run("ScrollNavigation", func(t *testing.T) {
			t.Parallel()
			type yOffsetCheck int
			const (
				ySkip yOffsetCheck = iota
				yLess
				yGreater
				yEqual
				yHalfUp
				yHalfDown
			)
			const skip = -1
			tests := []struct {
				name              string
				preKeys           []tea.KeyType
				key               tea.KeyType
				yCheck            yOffsetCheck
				wantAutoFollow    int
				wantBeforeBottom  int
				wantAfterBottom   int
				wantBeforeYOffset int
				wantAfterYOffset  int
			}{
				{"ScrollUpDecreasesYOffset", nil, tea.KeyUp, yLess, 0, skip, skip, skip, skip},
				{"ScrollDownIncreasesYOffset", []tea.KeyType{tea.KeyUp}, tea.KeyDown, yGreater, skip, skip, skip, skip, skip},
				{"ScrollUpAtTopIsNoOp", []tea.KeyType{tea.KeyHome}, tea.KeyUp, yEqual, skip, skip, skip, 0, skip},
				{"ScrollDownAtBottomReEnablesAutoFollow", []tea.KeyType{tea.KeyUp}, tea.KeyDown, yGreater, 1, 0, 1, skip, skip},
				{"PageUpScrollsHalfViewport", nil, tea.KeyPgUp, yHalfUp, 0, skip, skip, skip, skip},
				{"PageDownScrollsHalfViewport", []tea.KeyType{tea.KeyPgUp}, tea.KeyPgDown, yHalfDown, skip, skip, skip, skip, skip},
				{"HomeJumpsToTop", nil, tea.KeyHome, ySkip, 0, skip, skip, skip, 0},
				{"EndJumpsToBottomAndEnablesAutoFollow", []tea.KeyType{tea.KeyHome}, tea.KeyEnd, ySkip, 1, 0, 1, skip, skip},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					before := scrollableModel(t, tt.preKeys...)
					after := mustChatViewUpdate(t, before, tea.KeyMsg{Type: tt.key})
					assertScrollNavigationCase(t, before, after, tt.wantBeforeYOffset, tt.wantAfterYOffset, tt.wantAutoFollow, tt.wantBeforeBottom, tt.wantAfterBottom, int(tt.yCheck))
				})
			}
		})
		t.Run("AutoFollowOnContentUpdates", func(t *testing.T) {
			t.Parallel()
			tests := []struct {
				name                string
				preKeys             []tea.KeyType
				messageID           int64
				wantAutoFollow      bool
				wantAtBottom        bool
				wantPreserveYOffset bool
			}{
				{"SetContentPreservesScrollPosition", []tea.KeyType{tea.KeyUp}, 1001, false, false, true},
				{"NewMessageAutoFollowsWhenAtBottom", nil, 1002, true, true, false},
				{"NewMessageDoesNotAutoFollowWhenScrolledUp", []tea.KeyType{tea.KeyUp}, 1003, false, false, true},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					before := scrollableModel(t, tt.preKeys...)
					after := updateView(before, streamMessage(tt.messageID))
					require.Equal(t, tt.wantAutoFollow, after.autoFollow)
					require.Equal(t, tt.wantAtBottom, after.viewport.AtBottom())
					if tt.wantPreserveYOffset {
						require.Equal(t, before.viewport.YOffset, after.viewport.YOffset)
						return
					}
					require.GreaterOrEqual(t, after.viewport.YOffset, before.viewport.YOffset)
				})
			}
		})
		t.Run("StreamingAutoFollows", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			model = mustChatViewUpdate(t, model, tea.WindowSizeMsg{Width: 80, Height: 10})
			model = updateView(model, chatHistoryMsg{messages: overflowingMessages(10)})
			before := model.viewport.YOffset
			model = updateView(model, chatStreamEventMsg{event: testTextPartEvent(strings.Repeat("hello world ", 20))})
			model = updateView(model, chatStreamEventMsg{event: testTextPartEvent(strings.Repeat("more text ", 20))})
			require.True(t, model.autoFollow)
			require.True(t, model.viewport.AtBottom())
			require.GreaterOrEqual(t, model.viewport.YOffset, before)
		})
	})
	t.Run("ChatView/StatePersistence", func(t *testing.T) {
		t.Parallel()
		t.Run("ComposerTextSurvivesOverlayToggle", func(t *testing.T) {
			t.Parallel()
			model := newTestTUIModel()
			model.currentView = viewChat
			model.chat.loading = false
			catalog := codersdk.ChatModelsResponse{Providers: []codersdk.ChatModelProvider{{
				Provider:  "provider",
				Available: true,
				Models:    []codersdk.ChatModel{{ID: uuid.New().String(), Provider: "provider", Model: "model-a", DisplayName: "Model A"}},
			}}}
			model.catalog = &catalog
			model.chat.modelPickerFlat = catalog.Providers[0].Models
			model.chat.composer.SetValue("keep this draft")
			updatedModel, cmd := model.Update(toggleModelPickerMsg{})
			model = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, "keep this draft", model.chat.composer.Value())
			updatedModel, cmd = model.Update(toggleModelPickerMsg{})
			model = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, "keep this draft", model.chat.composer.Value())
		})

		t.Run("ComposerTextSurvivesFocusSwitch", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			model.composer.SetValue("keep this draft")

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
			require.False(t, updated.composerFocused)
			require.Equal(t, "keep this draft", updated.composer.Value())

			updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab})
			require.True(t, updated.composerFocused)
			require.Equal(t, "keep this draft", updated.composer.Value())
		})

		t.Run("ViewportScrollSurvivesOverlayToggle", func(t *testing.T) {
			t.Parallel()
			model := newTestTUIModel()
			model.currentView = viewChat
			model.chat.loading = false
			updatedModel, cmd := model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
			model = mustTUIModel(t, updatedModel, cmd)
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat.setChat(chat)
			model.chat.messages = overflowingMessages(10)
			diff := codersdk.ChatDiffContents{ChatID: chat.ID, Diff: "diff --git a/file b/file"}
			model.chat.diffContents = &diff
			model.chat.rebuildBlocks()
			model.chat.composerFocused = false
			(&model.chat).syncViewportContent()
			model.chat.viewport.GotoBottom()
			updatedModel, cmd = model.Update(tea.KeyMsg{Type: tea.KeyUp})
			model = mustTUIModel(t, updatedModel, cmd)
			require.False(t, model.chat.viewport.AtBottom())
			yBefore := model.chat.viewport.YOffset
			updatedModel, cmd = model.Update(toggleDiffDrawerMsg{})
			model = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, overlayDiffDrawer, model.overlay)
			require.Equal(t, yBefore, model.chat.viewport.YOffset)

			updatedModel, cmd = model.Update(toggleDiffDrawerMsg{})
			model = mustTUIModel(t, updatedModel, cmd)
			require.Equal(t, overlayNone, model.overlay)
			require.Equal(t, yBefore, model.chat.viewport.YOffset)
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

			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
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
			updated, _ = mustTUIModelWithCmd(t, updatedModel, cmd)
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
			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
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
			model := newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
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

	t.Run("SpinnerTickOnlyRefreshesWhenVisible", func(t *testing.T) {
		t.Parallel()

		model := newTestChatViewModel(nil)
		model = mustChatViewUpdate(t, model, tea.WindowSizeMsg{Width: 80, Height: 10})
		chat := testChat(codersdk.ChatStatusRunning)
		model.chat = &chat
		model.chatStatus = chat.Status
		model.messages = overflowingMessages(18)
		model.rebuildBlocks()

		visibleTranscript := model.lastTranscript
		updated, cmd := model.Update(model.spinner.Tick())
		require.NotNil(t, cmd)
		require.NotEqual(t, visibleTranscript, updated.lastTranscript)

		updated.viewport.LineUp(3)
		updated.autoFollow = false
		require.False(t, updated.viewport.AtBottom())

		hiddenTranscript := updated.lastTranscript
		hiddenYOffset := updated.viewport.YOffset
		updated, cmd = updated.Update(updated.spinner.Tick())
		require.NotNil(t, cmd)
		require.Equal(t, hiddenTranscript, updated.lastTranscript)
		require.Equal(t, hiddenYOffset, updated.viewport.YOffset)
	})

	t.Run("AskUserQuestion", func(t *testing.T) {
		t.Parallel()
		mustAskArgs := func(t testing.TB, questions ...parsedAskQuestion) string {
			t.Helper()
			payloadQuestions := make([]map[string]any, 0, len(questions))
			for _, question := range questions {
				options := make([]map[string]string, 0, len(question.Options))
				for _, option := range question.Options {
					options = append(options, map[string]string{
						"label": option.Label,
						"value": option.Value,
					})
				}
				payloadQuestions = append(payloadQuestions, map[string]any{
					"header":   question.Header,
					"question": question.Question,
					"options":  options,
				})
			}
			output, err := json.Marshal(map[string]any{"questions": payloadQuestions})
			require.NoError(t, err)
			return string(output)
		}
		askToolCall := func(t testing.TB, toolCallID string, questions ...parsedAskQuestion) codersdk.ChatStreamToolCall {
			t.Helper()
			return codersdk.ChatStreamToolCall{
				ToolCallID: toolCallID,
				ToolName:   "ask_user_question",
				Args:       mustAskArgs(t, questions...),
			}
		}
		message := func(parts ...codersdk.ChatMessagePart) codersdk.ChatMessage {
			return codersdk.ChatMessage{Content: parts}
		}
		toolCallPart := func(toolCallID, toolName, args string) codersdk.ChatMessagePart {
			return codersdk.ChatMessagePart{
				Type:       codersdk.ChatMessagePartTypeToolCall,
				ToolCallID: toolCallID,
				ToolName:   toolName,
				Args:       rawJSON(args),
			}
		}
		toolResultPart := func(toolCallID, toolName, result string) codersdk.ChatMessagePart {
			return codersdk.ChatMessagePart{
				Type:       codersdk.ChatMessagePartTypeToolResult,
				ToolCallID: toolCallID,
				ToolName:   toolName,
				Result:     rawJSON(result),
			}
		}
		firstQuestion := parsedAskQuestion{
			Header:   "Review plan",
			Question: "What should happen next?",
			Options: []parsedAskOption{
				{Label: "Approve", Value: "approve"},
				{Label: "Reject", Value: "reject"},
			},
		}
		secondQuestion := parsedAskQuestion{
			Header:   "Reason",
			Question: "Why?",
			Options: []parsedAskOption{
				{Label: "Speed", Value: "speed"},
				{Label: "Quality", Value: "quality"},
			},
		}

		t.Run("ParseToolCall", func(t *testing.T) {
			t.Parallel()

			t.Run("ValidJSONWithOptions", func(t *testing.T) {
				t.Parallel()

				state, err := parseAskUserQuestionToolCall(askToolCall(t, "tool-1", firstQuestion, secondQuestion))
				require.NoError(t, err)
				require.Equal(t, "tool-1", state.ToolCallID)
				require.Equal(t, []parsedAskQuestion{firstQuestion, secondQuestion}, state.Questions)
				require.Empty(t, state.Answers)
				require.Zero(t, state.CurrentIndex)
				require.Zero(t, state.OptionCursor)
			})

			t.Run("EmptyOrMissingQuestionsReturnsError", func(t *testing.T) {
				t.Parallel()

				for _, tt := range []struct {
					name string
					args string
				}{
					{name: "MissingQuestions", args: `{}`},
					{name: "EmptyQuestions", args: `{"questions":[]}`},
				} {
					tt := tt
					t.Run(tt.name, func(t *testing.T) {
						t.Parallel()

						state, err := parseAskUserQuestionToolCall(codersdk.ChatStreamToolCall{
							ToolCallID: "tool-1",
							ToolName:   "ask_user_question",
							Args:       tt.args,
						})
						require.Nil(t, state)
						require.ErrorContains(t, err, "at least one question")
					})
				}
			})

			t.Run("MalformedJSONReturnsError", func(t *testing.T) {
				t.Parallel()

				state, err := parseAskUserQuestionToolCall(codersdk.ChatStreamToolCall{
					ToolCallID: "tool-1",
					ToolName:   "ask_user_question",
					Args:       `{"questions":[`,
				})
				require.Nil(t, state)
				require.ErrorContains(t, err, "parse ask_user_question args")
			})
		})

		t.Run("BuildToolResult", func(t *testing.T) {
			t.Parallel()

			t.Run("AnswersMarshalToJSON", func(t *testing.T) {
				t.Parallel()

				output, err := buildAskUserQuestionToolResult(&askUserQuestionState{
					Answers: []askQuestionAnswer{{
						Header:      firstQuestion.Header,
						Question:    firstQuestion.Question,
						Answer:      "approve",
						OptionLabel: "Approve",
						Freeform:    false,
					}},
				})
				require.NoError(t, err)
				require.JSONEq(t, `{"answers":[{"header":"Review plan","question":"What should happen next?","answer":"approve","option_label":"Approve","freeform":false}]}`, string(output))
			})

			t.Run("NoAnswersUsesEmptyArray", func(t *testing.T) {
				t.Parallel()

				output, err := buildAskUserQuestionToolResult(&askUserQuestionState{})
				require.NoError(t, err)
				require.JSONEq(t, `{"answers":[]}`, string(output))
			})
		})

		t.Run("FindPending", func(t *testing.T) {
			t.Parallel()

			t.Run("NoMessagesReturnsNil", func(t *testing.T) {
				t.Parallel()

				state, err := findPendingAskUserQuestion(nil)
				require.NoError(t, err)
				require.Nil(t, state)
			})

			t.Run("ServerToolResultStillReturnsPendingState", func(t *testing.T) {
				t.Parallel()

				messages := []codersdk.ChatMessage{
					message(toolCallPart("tool-1", "ask_user_question", mustAskArgs(t, firstQuestion))),
					message(toolResultPart("tool-1", "ask_user_question", mustAskArgs(t, firstQuestion))),
				}
				state, err := findPendingAskUserQuestion(messages)
				require.NoError(t, err)
				require.NotNil(t, state)
				require.Equal(t, "tool-1", state.ToolCallID)
				require.Equal(t, []parsedAskQuestion{firstQuestion}, state.Questions)
			})

			t.Run("UserAnsweredToolCallReturnsNil", func(t *testing.T) {
				t.Parallel()

				messages := []codersdk.ChatMessage{
					message(toolCallPart("tool-1", "ask_user_question", mustAskArgs(t, firstQuestion))),
					message(toolResultPart("tool-1", "ask_user_question", `{"answers":[{"answer":"approve"}]}`)),
				}
				state, err := findPendingAskUserQuestion(messages)
				require.NoError(t, err)
				require.Nil(t, state)
			})

			t.Run("UnmatchedToolCallReturnsParsedState", func(t *testing.T) {
				t.Parallel()

				messages := []codersdk.ChatMessage{
					message(toolCallPart("tool-1", "ask_user_question", mustAskArgs(t, firstQuestion, secondQuestion))),
					message(codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "assistant reply"}),
				}
				state, err := findPendingAskUserQuestion(messages)
				require.NoError(t, err)
				require.NotNil(t, state)
				require.Equal(t, "tool-1", state.ToolCallID)
				require.Equal(t, []parsedAskQuestion{firstQuestion, secondQuestion}, state.Questions)
			})

			t.Run("NonAskUserQuestionToolCallReturnsNil", func(t *testing.T) {
				t.Parallel()

				messages := []codersdk.ChatMessage{
					message(toolCallPart("tool-1", "search_docs", `{"query":"overlay"}`)),
				}
				state, err := findPendingAskUserQuestion(messages)
				require.NoError(t, err)
				require.Nil(t, state)
			})
		})

		t.Run("HandleStreamEventActionRequired", func(t *testing.T) {
			t.Parallel()

			t.Run("AskUserQuestionShowsOverlay", func(t *testing.T) {
				t.Parallel()

				model := newTestChatViewModel(nil)
				updated, cmd := model.handleStreamEvent(codersdk.ChatStreamEvent{
					Type: codersdk.ChatStreamEventTypeActionRequired,
					ActionRequired: &codersdk.ChatStreamActionRequired{
						ToolCalls: []codersdk.ChatStreamToolCall{askToolCall(t, "tool-1", firstQuestion)},
					},
				})
				require.NotNil(t, updated.pendingAskUserQuestion)
				require.Equal(t, "tool-1", updated.pendingAskUserQuestion.ToolCallID)
				require.Equal(t, []parsedAskQuestion{firstQuestion}, updated.pendingAskUserQuestion.Questions)
				showMsg, ok := mustMsg(t, cmd).(showAskUserQuestionMsg)
				require.True(t, ok)
				require.Same(t, updated.pendingAskUserQuestion, showMsg.state)
			})

			t.Run("NonAskUserQuestionToolCallIsIgnored", func(t *testing.T) {
				t.Parallel()

				model := newTestChatViewModel(nil)
				updated, cmd := model.handleStreamEvent(codersdk.ChatStreamEvent{
					Type: codersdk.ChatStreamEventTypeActionRequired,
					ActionRequired: &codersdk.ChatStreamActionRequired{
						ToolCalls: []codersdk.ChatStreamToolCall{{
							ToolCallID: "tool-1",
							ToolName:   "search_docs",
							Args:       `{"query":"overlay"}`,
						}},
					},
				})
				require.Nil(t, updated.pendingAskUserQuestion)
				require.Nil(t, cmd)
			})

			t.Run("MalformedArgsReturnErrorEvent", func(t *testing.T) {
				t.Parallel()

				model := newTestChatViewModel(nil)
				model.activeChatID = uuid.New()
				model.chatGeneration = 7
				updated, cmd := model.handleStreamEvent(codersdk.ChatStreamEvent{
					Type: codersdk.ChatStreamEventTypeActionRequired,
					ActionRequired: &codersdk.ChatStreamActionRequired{
						ToolCalls: []codersdk.ChatStreamToolCall{{
							ToolCallID: "tool-1",
							ToolName:   "ask_user_question",
							Args:       `{"questions":[`,
						}},
					},
				})
				require.Nil(t, updated.pendingAskUserQuestion)
				streamMsg, ok := mustMsg(t, cmd).(chatStreamEventMsg)
				require.True(t, ok)
				require.Equal(t, uint64(7), streamMsg.generation)
				require.Equal(t, model.activeChatID, streamMsg.chatID)
				require.Equal(t, codersdk.ChatStreamEventTypeError, streamMsg.event.Type)
				require.NotNil(t, streamMsg.event.Error)
				require.Contains(t, streamMsg.event.Error.Message, "failed to parse ask_user_question")

				updated = mustChatViewUpdate(t, updated, streamMsg)
				require.EqualError(t, updated.err, "stream error: "+streamMsg.event.Error.Message)
			})
		})

		t.Run("HandleStreamEventStatusRequiresAction", func(t *testing.T) {
			t.Parallel()

			t.Run("RecoversFromMessages", func(t *testing.T) {
				t.Parallel()

				chat := testChat(codersdk.ChatStatusRunning)
				model := newTestChatViewModel(nil)
				model.chat, model.activeChatID, model.chatStatus = &chat, chat.ID, chat.Status
				model.messages = []codersdk.ChatMessage{
					message(toolCallPart("tool-1", "ask_user_question", mustAskArgs(t, firstQuestion))),
					message(toolResultPart("tool-1", "ask_user_question", mustAskArgs(t, firstQuestion))),
				}

				updated, cmd := model.handleStreamEvent(codersdk.ChatStreamEvent{
					Type:   codersdk.ChatStreamEventTypeStatus,
					ChatID: chat.ID,
					Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatusRequiresAction},
				})
				require.Equal(t, codersdk.ChatStatusRequiresAction, updated.chatStatus)
				require.NotNil(t, updated.pendingAskUserQuestion)
				require.Equal(t, "tool-1", updated.pendingAskUserQuestion.ToolCallID)
				require.Equal(t, []parsedAskQuestion{firstQuestion}, updated.pendingAskUserQuestion.Questions)
				showMsg, ok := mustMsg(t, cmd).(showAskUserQuestionMsg)
				require.True(t, ok)
				require.Same(t, updated.pendingAskUserQuestion, showMsg.state)
			})

			t.Run("RecoversFromAccumulatorBeforeFinalMessage", func(t *testing.T) {
				t.Parallel()

				chat := testChat(codersdk.ChatStatusRunning)
				model := newTestChatViewModel(nil)
				model.chat, model.activeChatID, model.chatStatus = &chat, chat.ID, chat.Status
				model.accumulator.parts = []codersdk.ChatMessagePart{
					toolCallPart("tool-1", "ask_user_question", mustAskArgs(t, firstQuestion, secondQuestion)),
				}
				model.accumulator.pending = true

				updated, cmd := model.handleStreamEvent(codersdk.ChatStreamEvent{
					Type:   codersdk.ChatStreamEventTypeStatus,
					ChatID: chat.ID,
					Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatusRequiresAction},
				})
				require.Equal(t, codersdk.ChatStatusRequiresAction, updated.chatStatus)
				require.NotNil(t, updated.pendingAskUserQuestion)
				require.Equal(t, "tool-1", updated.pendingAskUserQuestion.ToolCallID)
				require.Equal(t, []parsedAskQuestion{firstQuestion, secondQuestion}, updated.pendingAskUserQuestion.Questions)
				showMsg, ok := mustMsg(t, cmd).(showAskUserQuestionMsg)
				require.True(t, ok)
				require.Same(t, updated.pendingAskUserQuestion, showMsg.state)
			})
		})

		t.Run("OverlayLifecycle", func(t *testing.T) {
			t.Parallel()

			newOverlayState := func() *askUserQuestionState {
				return newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion})
			}

			t.Run("ShowOpensOverlay", func(t *testing.T) {
				t.Parallel()

				state := newOverlayState()
				model := newTestTUIModel()
				model.currentView = viewChat

				updatedModel, cmd := model.Update(showAskUserQuestionMsg{state: state})
				updated := mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, overlayAskUserQuestion, updated.overlay)
				require.Same(t, state, updated.chat.pendingAskUserQuestion)
			})

			t.Run("HideClosesOverlay", func(t *testing.T) {
				t.Parallel()

				state := newOverlayState()
				model := newTestTUIModel()
				model.currentView = viewChat
				model.overlay = overlayAskUserQuestion
				model.chat.pendingAskUserQuestion = state

				updatedModel, cmd := model.Update(hideAskUserQuestionMsg{})
				updated := mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, overlayNone, updated.overlay)
				require.Same(t, state, updated.chat.pendingAskUserQuestion)
			})

			t.Run("EscapeDoesNotCloseOverlay", func(t *testing.T) {
				t.Parallel()

				state := newOverlayState()
				model := newTestTUIModel()
				model.currentView = viewChat
				model.overlay = overlayAskUserQuestion
				model.chat.pendingAskUserQuestion = state

				updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
				updated := mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, overlayAskUserQuestion, updated.overlay)
				require.Same(t, state, updated.chat.pendingAskUserQuestion)
			})

			t.Run("SuccessfulSubmitClearsOverlay", func(t *testing.T) {
				t.Parallel()

				state := newOverlayState()
				model := newTestTUIModel()
				model.currentView = viewChat
				model.overlay = overlayAskUserQuestion
				model.chat.pendingAskUserQuestion = state
				model.chat.activeChatID = uuid.New()
				model.chat.chatGeneration = 11

				updatedModel, cmd := model.Update(toolResultsSubmittedMsg{
					generation: 11,
					chatID:     model.chat.activeChatID,
				})
				updated := mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, overlayNone, updated.overlay)
				require.Nil(t, updated.chat.pendingAskUserQuestion)
			})

			t.Run("SubmitErrorKeepsOverlayOpen", func(t *testing.T) {
				t.Parallel()

				state := newOverlayState()
				state.Submitting = true
				model := newTestTUIModel()
				model.currentView = viewChat
				model.overlay = overlayAskUserQuestion
				model.chat.pendingAskUserQuestion = state
				model.chat.activeChatID = uuid.New()
				model.chat.chatGeneration = 11

				updatedModel, cmd := model.Update(toolResultsSubmittedMsg{
					generation: 11,
					chatID:     model.chat.activeChatID,
					err:        xerrors.New("submit failed"),
				})
				updated := mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, overlayAskUserQuestion, updated.overlay)
				require.NotNil(t, updated.chat.pendingAskUserQuestion)
				require.False(t, updated.chat.pendingAskUserQuestion.Submitting)
				require.EqualError(t, updated.chat.pendingAskUserQuestion.Error, "submit failed")
			})

			t.Run("StaleSubmitIsIgnored", func(t *testing.T) {
				t.Parallel()

				state := newOverlayState()
				state.Submitting = true
				model := newTestTUIModel()
				model.currentView = viewChat
				model.overlay = overlayAskUserQuestion
				model.chat.pendingAskUserQuestion = state
				model.chat.activeChatID = uuid.New()
				model.chat.chatGeneration = 11

				updatedModel, cmd := model.Update(toolResultsSubmittedMsg{
					generation: 10,
					chatID:     model.chat.activeChatID,
				})
				updated := mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, overlayAskUserQuestion, updated.overlay)
				require.Same(t, state, updated.chat.pendingAskUserQuestion)
				require.True(t, updated.chat.pendingAskUserQuestion.Submitting)
				require.NoError(t, updated.chat.pendingAskUserQuestion.Error)
			})
		})

		t.Run("KeyHandling", func(t *testing.T) {
			t.Parallel()

			t.Run("UpAndDownNavigateOptions", func(t *testing.T) {
				t.Parallel()

				state := newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion})
				model := newTestTUIModel()
				model.chat.pendingAskUserQuestion = state

				require.Nil(t, model.handleAskUserQuestionKey(tea.KeyMsg{Type: tea.KeyDown}))
				require.Equal(t, 1, state.OptionCursor)
				require.Nil(t, model.handleAskUserQuestionKey(tea.KeyMsg{Type: tea.KeyUp}))
				require.Zero(t, state.OptionCursor)
				require.Nil(t, model.handleAskUserQuestionKey(tea.KeyMsg{Type: tea.KeyUp}))
				require.Equal(t, len(firstQuestion.Options), state.OptionCursor)
			})

			t.Run("EnterOnOptionRecordsAnswerAndAdvances", func(t *testing.T) {
				t.Parallel()

				state := newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion, secondQuestion})
				state.OptionCursor = 1
				model := newTestTUIModel()
				model.chat.pendingAskUserQuestion = state

				cmd := model.handleAskUserQuestionKey(tea.KeyMsg{Type: tea.KeyEnter})
				require.Nil(t, cmd)
				require.Len(t, state.Answers, 1)
				require.Equal(t, askQuestionAnswer{
					Header:      firstQuestion.Header,
					Question:    firstQuestion.Question,
					Answer:      "reject",
					OptionLabel: "Reject",
					Freeform:    false,
				}, state.Answers[0])
				require.Equal(t, 1, state.CurrentIndex)
				require.Zero(t, state.OptionCursor)
			})

			t.Run("EnterOnOtherEntersFreeformMode", func(t *testing.T) {
				t.Parallel()

				state := newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion})
				state.OptionCursor = len(firstQuestion.Options)
				model := newTestTUIModel()
				model.chat.pendingAskUserQuestion = state

				cmd := model.handleAskUserQuestionKey(tea.KeyMsg{Type: tea.KeyEnter})
				require.Nil(t, cmd)
				require.True(t, state.OtherMode)
				require.Empty(t, state.OtherInput.Value())
			})

			t.Run("EscapeInFreeformModeExitsOnlyInput", func(t *testing.T) {
				t.Parallel()

				state := newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion})
				state.OtherMode = true
				state.OtherInput.Focus()
				state.OtherInput.SetValue("typed answer")
				model := newTestTUIModel()
				model.currentView = viewChat
				model.overlay = overlayAskUserQuestion
				model.chat.pendingAskUserQuestion = state

				updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
				updated := mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, overlayAskUserQuestion, updated.overlay)
				require.NotNil(t, updated.chat.pendingAskUserQuestion)
				require.False(t, updated.chat.pendingAskUserQuestion.OtherMode)
				require.Equal(t, "typed answer", updated.chat.pendingAskUserQuestion.OtherInput.Value())
			})

			t.Run("LeftOrHMovesBackToPreviousQuestion", func(t *testing.T) {
				t.Parallel()

				state := newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion, secondQuestion})
				state.CurrentIndex = 1
				state.OptionCursor = 1
				state.Error = xerrors.New("temporary error")
				state.Answers = []askQuestionAnswer{{
					Header:      firstQuestion.Header,
					Question:    firstQuestion.Question,
					Answer:      "approve",
					OptionLabel: "Approve",
					Freeform:    false,
				}}
				model := newTestTUIModel()
				model.chat.pendingAskUserQuestion = state

				cmd := model.handleAskUserQuestionKey(keyRunes("h"))
				require.Nil(t, cmd)
				require.Zero(t, state.CurrentIndex)
				require.Zero(t, state.OptionCursor)
				require.False(t, state.OtherMode)
				require.Nil(t, state.Error)
				require.Empty(t, state.Answers)
			})
		})

		t.Run("RecordAskAnswer", func(t *testing.T) {
			t.Parallel()

			model := newChatsTUIModel(context.Background(), failingExperimentalClient(), nil, nil, nil, uuid.Nil)
			model.chat.activeChatID = uuid.New()
			model.chat.chatGeneration = 4
			state := newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion})
			state.OtherMode = true
			state.OtherInput.Focus()
			state.OtherInput.SetValue("custom answer")
			model.chat.pendingAskUserQuestion = state

			cmd := model.recordAskAnswer("custom answer", "", true)
			require.NotNil(t, cmd)
			require.True(t, state.Submitting)
			require.Len(t, state.Answers, 1)
			require.Equal(t, askQuestionAnswer{
				Header:   firstQuestion.Header,
				Question: firstQuestion.Question,
				Answer:   "custom answer",
				Freeform: true,
			}, state.Answers[0])
			require.False(t, state.OtherMode)
			require.Empty(t, state.OtherInput.Value())
		})

		t.Run("ComposerBlocksEnterWhileQuestionPending", func(t *testing.T) {
			t.Parallel()

			baseline := newTestChatViewModel(failingExperimentalClient())
			baseline.draft = true
			baseline.loading = false
			baseline.composer.SetValue("send this")

			updated, cmd := baseline.Update(tea.KeyMsg{Type: tea.KeyEnter})
			require.NotNil(t, cmd)
			require.True(t, updated.creatingChat)
			require.Empty(t, updated.composer.Value())

			blocked := newTestChatViewModel(failingExperimentalClient())
			blocked.draft = true
			blocked.loading = false
			blocked.composer.SetValue("send this")
			blocked.pendingAskUserQuestion = newAskUserQuestionState("tool-1", []parsedAskQuestion{firstQuestion})

			updated, cmd = blocked.Update(tea.KeyMsg{Type: tea.KeyEnter})
			require.Nil(t, cmd)
			require.False(t, updated.creatingChat)
			require.Equal(t, "send this", updated.composer.Value())
			require.Empty(t, updated.pendingComposerText)
		})
	})

	t.Run("ChatList", func(t *testing.T) {
		t.Parallel()
		newChat := func(status codersdk.ChatStatus, title string, parent *uuid.UUID) codersdk.Chat {
			chat := testChat(status)
			chat.Title, chat.ParentChatID = title, parent
			return chat
		}
		newList := func(chats ...codersdk.Chat) chatListModel {
			model := newReadyChatListModel()
			model.chats = chats
			return model
		}
		mustUpdate := func(t testing.TB, model chatListModel, msg tea.Msg) chatListModel {
			t.Helper()
			updated, cmd := model.Update(msg)
			require.Nil(t, cmd)
			return updated
		}
		requireRows := func(t testing.TB, rows []chatDisplayRow, wantChats []codersdk.Chat, wantDepths ...int) {
			t.Helper()
			require.Len(t, rows, len(wantChats))
			for i, want := range wantChats {
				require.Equal(t, want.ID, rows[i].chat.ID)
				require.Equal(t, wantDepths[i], rows[i].depth)
			}
		}
		t.Run("ChatsListedUpdatesState", func(t *testing.T) {
			t.Parallel()
			for _, tt := range []struct {
				name      string
				msg       chatsListedMsg
				wantChats int
				wantErr   string
			}{
				{name: "StoresChats", msg: chatsListedMsg{chats: []codersdk.Chat{testChat(codersdk.ChatStatusWaiting), testChat(codersdk.ChatStatusCompleted)}}, wantChats: 2},
				{name: "StoresErr", msg: chatsListedMsg{err: xerrors.New("list failed")}, wantErr: "list failed"},
			} {
				updated, cmd := newChatListModel(newTUIStyles()).Update(tt.msg)
				require.Nilf(t, cmd, tt.name)
				require.Falsef(t, updated.loading, tt.name)
				require.Lenf(t, updated.chats, tt.wantChats, tt.name)
				if tt.wantErr == "" {
					require.NoErrorf(t, updated.err, tt.name)
					continue
				}
				require.EqualErrorf(t, updated.err, tt.wantErr, tt.name)
			}
		})
		t.Run("ParentExpansionAndCollapse", func(t *testing.T) {
			t.Parallel()
			parent := newChat(codersdk.ChatStatusRunning, "Parent chat", nil)
			childA := newChat(codersdk.ChatStatusWaiting, "Subagent A", &parent.ID)
			childB := newChat(codersdk.ChatStatusPending, "Subagent B", &parent.ID)
			root := newChat(codersdk.ChatStatusCompleted, "Standalone chat", nil)
			model := newList(parent, childA, childB, root)
			requireRows(t, model.displayRows(), []codersdk.Chat{parent, root}, 0, 0)
			output := plainText(model.View())
			require.Contains(t, output, "▶ Parent chat")
			require.Contains(t, output, "(2 subagents)")
			require.NotContains(t, output, childA.Title)
			require.NotContains(t, output, childB.Title)
			model = mustUpdate(t, model, tea.KeyMsg{Type: tea.KeyRight})
			require.True(t, model.expanded[parent.ID])
			requireRows(t, model.displayRows(), []codersdk.Chat{parent, childA, childB, root}, 0, 1, 1, 0)
			model = mustUpdate(t, model, keyRunes("x"))
			require.False(t, model.expanded[parent.ID])
			requireRows(t, model.displayRows(), []codersdk.Chat{parent, root}, 0, 0)
			model = mustUpdate(t, model, keyRunes("x"))
			require.True(t, model.expanded[parent.ID])
			model = mustUpdate(t, model, tea.KeyMsg{Type: tea.KeyLeft})
			require.False(t, model.expanded[parent.ID])
			require.Zero(t, model.cursor)
		})
		t.Run("NestedExpansionNavigationAndOpenSelectedChat", func(t *testing.T) {
			t.Parallel()
			parent := newChat(codersdk.ChatStatusRunning, "Parent chat", nil)
			child := newChat(codersdk.ChatStatusWaiting, "Child subagent", &parent.ID)
			grandchild := newChat(codersdk.ChatStatusPending, "Grandchild subagent", &child.ID)
			root := newChat(codersdk.ChatStatusCompleted, "Standalone chat", nil)
			model := newList(parent, child, grandchild, root)
			model.width, model.height = 100, 10
			model = mustUpdate(t, model, tea.KeyMsg{Type: tea.KeyRight})
			require.True(t, model.expanded[parent.ID])
			requireRows(t, model.displayRows(), []codersdk.Chat{parent, child, root}, 0, 1, 0)
			model = mustUpdate(t, model, tea.KeyMsg{Type: tea.KeyDown})
			selected := model.selectedChat()
			require.NotNil(t, selected)
			require.Equal(t, child.ID, selected.ID)
			model = mustUpdate(t, model, tea.KeyMsg{Type: tea.KeyRight})
			require.True(t, model.expanded[child.ID])
			requireRows(t, model.displayRows(), []codersdk.Chat{parent, child, grandchild, root}, 0, 1, 2, 0)
			require.Contains(t, plainText(model.View()), "Grandchild subagent")
			model = mustUpdate(t, model, tea.KeyMsg{Type: tea.KeyDown})
			selected = model.selectedChat()
			require.NotNil(t, selected)
			require.Equal(t, grandchild.ID, selected.ID)
			model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			openMsg, ok := mustMsg(t, cmd).(openSelectedChatMsg)
			require.True(t, ok)
			require.Equal(t, grandchild.ID, openMsg.chatID)
			model = mustUpdate(t, model, tea.KeyMsg{Type: tea.KeyLeft})
			require.False(t, model.expanded[child.ID])
			require.Equal(t, child.ID, model.selectedChat().ID)
			model = mustUpdate(t, model, tea.KeyMsg{Type: tea.KeyLeft})
			require.False(t, model.expanded[parent.ID])
			require.Equal(t, parent.ID, model.selectedChat().ID)
		})
		t.Run("SearchIncludesVisibleAncestorChain", func(t *testing.T) {
			t.Parallel()
			for _, depth := range []int{1, 2} {
				model := newReadyChatListModel()
				chain := make([]codersdk.Chat, 0, depth+1)
				wantDepths := make([]int, 0, depth+1)
				var parentID *uuid.UUID
				for d := 0; d <= depth; d++ {
					title := "Root chat"
					if d > 0 {
						title = fmt.Sprintf("Subagent depth %d", d)
					}
					if d == depth {
						title += " needle"
					}
					chain = append(chain, newChat(codersdk.ChatStatusWaiting, title, parentID))
					parentID = &chain[len(chain)-1].ID
					wantDepths = append(wantDepths, d)
				}
				model.chats = append([]codersdk.Chat{}, chain...)
				model.chats = append(model.chats, newChat(codersdk.ChatStatusCompleted, "Other root", nil))
				model.search.SetValue("needle")
				rows := model.displayRows()
				requireRows(t, rows, chain, wantDepths...)
				for i, row := range rows {
					require.Equalf(t, i < depth, row.isExpanded, "depth=%d row=%d", depth, i)
				}
			}
		})
		t.Run("NavigationKeysMoveCursorWithinBounds", func(t *testing.T) {
			t.Parallel()
			chats := []codersdk.Chat{testChat(codersdk.ChatStatusWaiting), testChat(codersdk.ChatStatusPending), testChat(codersdk.ChatStatusCompleted)}
			for _, tt := range []struct {
				name string
				key  tea.KeyMsg
				want int
			}{
				{name: "Up", key: tea.KeyMsg{Type: tea.KeyUp}, want: 0},
				{name: "Down", key: tea.KeyMsg{Type: tea.KeyDown}, want: 2},
				{name: "J", key: keyRunes("j"), want: 2},
				{name: "K", key: keyRunes("k"), want: 0},
			} {
				model := newList(chats...)
				model.cursor = 1
				model = mustUpdate(t, model, tt.key)
				require.Equalf(t, tt.want, model.cursor, tt.name)
				model = mustUpdate(t, model, tt.key)
				require.Equalf(t, tt.want, model.cursor, tt.name)
			}
		})
		t.Run("ViewKeepsSelectedChatVisible", func(t *testing.T) {
			t.Parallel()
			model := newReadyChatListModel()
			model.width, model.height = 80, 8
			for i := range 8 {
				model.chats = append(model.chats, newChat(codersdk.ChatStatusWaiting, fmt.Sprintf("chat %02d", i), nil))
			}
			for range 6 {
				model = mustUpdate(t, model, tea.KeyMsg{Type: tea.KeyDown})
			}
			require.Equal(t, 2, model.offset)
			listView := plainText(model.View())
			require.Contains(t, listView, "> chat 06")
			require.NotContains(t, listView, "chat 00")
			parent := newTestTUIModel()
			parent.width, parent.height, parent.list = 80, 8, model
			parentView := plainText(parent.View())
			require.Contains(t, parentView, "Coder Chats")
			require.Contains(t, parentView, "> chat 06")
			for range 5 {
				model = mustUpdate(t, model, tea.KeyMsg{Type: tea.KeyUp})
			}
			require.Equal(t, 1, model.offset)
			require.Contains(t, plainText(model.View()), "> chat 01")
		})
		t.Run("EmptyListDisplaysNoChatsMessage", func(t *testing.T) {
			t.Parallel()
			updated, cmd := newChatListModel(newTUIStyles()).Update(chatsListedMsg{chats: []codersdk.Chat{}})
			require.Nil(t, cmd)
			require.Contains(t, plainText(updated.View()), "No chats yet")
		})
	})
}

func TestAgents_View_LongInputFitsTerminal(t *testing.T) {
	t.Parallel()
	model := newTestChatViewModel(nil)
	model.width, model.height = 80, 24
	model.setComposerWidth()
	model.recalcViewportHeight()
	model.syncViewportContent()
	chat := testChat(codersdk.ChatStatusCompleted)
	model.chat = &chat
	model.chatStatus = chat.Status
	model.messages = overflowingMessages(24)
	model.rebuildBlocks()

	defaultViewportHeight := model.viewport.Height
	model.composer.SetValue(strings.Repeat("a", 250))
	model.recalcViewportHeight()
	model.syncViewportContent()

	view := plainText(model.View())
	lines := strings.Split(view, "\n")

	require.LessOrEqual(t, len(lines), model.height)
	require.LessOrEqual(t, model.viewport.Height, defaultViewportHeight)
	require.NotEmpty(t, strings.TrimSpace(lines[len(lines)-1]))
}

func mustTUIModel(t testing.TB, model tea.Model, cmd tea.Cmd) chatsTUIModel {
	t.Helper()
	updated, ok := model.(chatsTUIModel)
	require.True(t, ok)
	require.Nil(t, cmd)
	return updated
}

func mustTUIModelWithCmd(t testing.TB, model tea.Model, cmd tea.Cmd) (chatsTUIModel, tea.Cmd) {
	t.Helper()
	updated, ok := model.(chatsTUIModel)
	require.True(t, ok)
	return updated, cmd
}

func mustChatViewUpdate(t testing.TB, model chatViewModel, msg tea.Msg) chatViewModel {
	t.Helper()
	updated, cmd := model.Update(msg)
	require.Nil(t, cmd)
	return updated
}

func mustMsg(t testing.TB, cmd tea.Cmd) tea.Msg { t.Helper(); require.NotNil(t, cmd); return cmd() }

func mustBatchMsg(t testing.TB, cmd tea.Cmd) tea.BatchMsg {
	t.Helper()
	batch, ok := mustMsg(t, cmd).(tea.BatchMsg)
	require.True(t, ok)
	return batch
}

func assertStreamCase(t testing.TB, model chatViewModel, wantMessages int, wantAccumulatorText, wantAccumulatorArgs string, wantBlockKind chatBlockKind, wantBlockText, wantBlockArgs string, wantUsage *codersdk.ChatMessageUsage) {
	t.Helper()
	wantPending := wantAccumulatorText != "" || wantAccumulatorArgs != ""
	require.Len(t, model.messages, wantMessages)
	require.Equal(t, wantPending, model.accumulator.isPending())
	if wantPending {
		require.Len(t, model.accumulator.parts, 1)
		if wantAccumulatorText != "" {
			require.Equal(t, wantAccumulatorText, model.accumulator.parts[0].Text)
		}
		if wantAccumulatorArgs != "" {
			require.Equal(t, wantAccumulatorArgs, string(model.accumulator.parts[0].Args))
		}
	} else {
		require.Empty(t, model.accumulator.parts)
	}
	require.Len(t, model.blocks, 1)
	require.Equal(t, wantBlockKind, model.blocks[0].kind)
	if wantBlockText != "" {
		require.Equal(t, wantBlockText, model.blocks[0].text)
	}
	if wantBlockArgs != "" {
		require.Equal(t, wantBlockArgs, model.blocks[0].args)
	}
	require.Equal(t, wantUsage, model.lastUsage)
	require.False(t, model.reconnecting)
}

func assertScrollNavigationCase(t testing.TB, before chatViewModel, after chatViewModel, wantBeforeYOffset int, wantAfterYOffset int, wantAutoFollow int, wantBeforeBottom int, wantAfterBottom int, yCheck int) {
	t.Helper()
	if wantAfterYOffset == 0 && wantBeforeYOffset == -1 {
		require.NotZero(t, before.viewport.YOffset)
	}
	if wantBeforeYOffset != -1 {
		require.Equal(t, wantBeforeYOffset, before.viewport.YOffset)
	}
	if wantAfterYOffset != -1 {
		require.Equal(t, wantAfterYOffset, after.viewport.YOffset)
	}
	if wantAutoFollow != -1 {
		require.Equal(t, wantAutoFollow == 1, after.autoFollow)
	}
	if wantBeforeBottom != -1 {
		require.Equal(t, wantBeforeBottom == 1, before.viewport.AtBottom())
	}
	if wantAfterBottom != -1 {
		require.Equal(t, wantAfterBottom == 1, after.viewport.AtBottom())
	}
	switch yCheck {
	case 1:
		require.Less(t, after.viewport.YOffset, before.viewport.YOffset)
	case 2:
		require.Greater(t, after.viewport.YOffset, before.viewport.YOffset)
	case 3:
		require.Equal(t, before.viewport.YOffset, after.viewport.YOffset)
	case 4:
		halfView := before.viewport.Height / 2
		require.InDelta(t, float64(before.viewport.YOffset-halfView), float64(after.viewport.YOffset), 1)
	case 5:
		halfView := before.viewport.Height / 2
		require.InDelta(t, float64(before.viewport.YOffset+halfView), float64(after.viewport.YOffset), 1)
	}
}

// newTestChatViewModel creates a chatViewModel for reducer tests.
// The returned model has chatGeneration=0, so test messages with
// default generation=0 pass the generation guard.
func newTestChatViewModel(client *codersdk.ExperimentalClient) chatViewModel {
	return newChatViewModel(context.Background(), client, nil, nil, uuid.Nil, newTUIStyles())
}

func newTestTUIModel() chatsTUIModel {
	return newChatsTUIModel(context.Background(), nil, nil, nil, nil, uuid.Nil)
}

func newReadyChatListModel() chatListModel {
	model := newChatListModel(newTUIStyles())
	model.loading = false
	return model
}

func newTestExperimentalClient(t testing.TB, handler http.HandlerFunc) *codersdk.ExperimentalClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	return codersdk.NewExperimentalClient(codersdk.New(serverURL))
}

func overflowingMessages(count int) []codersdk.ChatMessage {
	messages := make([]codersdk.ChatMessage, 0, count)
	for i := 0; i < count; i++ {
		role := codersdk.ChatMessageRoleUser
		if i%2 == 1 {
			role = codersdk.ChatMessageRoleAssistant
		}
		messages = append(messages, testMessage(int64(i+1), role, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: fmt.Sprintf("message %d %s", i+1, strings.Repeat("content ", 18))}))
	}
	return messages
}

func testChat(status codersdk.ChatStatus) codersdk.Chat {
	return codersdk.Chat{ID: uuid.New(), Title: "test chat", Status: status, CreatedAt: time.Now(), UpdatedAt: time.Now()}
}

func testMessage(id int64, role codersdk.ChatMessageRole, parts ...codersdk.ChatMessagePart) codersdk.ChatMessage {
	return codersdk.ChatMessage{ID: id, ChatID: uuid.New(), CreatedAt: time.Now(), Role: role, Content: parts}
}

func testQueuedMessage(id int64, parts ...codersdk.ChatMessagePart) codersdk.ChatQueuedMessage {
	return codersdk.ChatQueuedMessage{ID: id, ChatID: uuid.New(), CreatedAt: time.Now(), Content: parts}
}

func testTextPartEvent(text string) codersdk.ChatStreamEvent {
	return codersdk.ChatStreamEvent{Type: codersdk.ChatStreamEventTypeMessagePart, MessagePart: &codersdk.ChatStreamMessagePart{
		Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: text},
	}}
}

func testToolCallDeltaEvent(toolCallID, toolName, delta string) codersdk.ChatStreamEvent {
	return codersdk.ChatStreamEvent{Type: codersdk.ChatStreamEventTypeMessagePart, MessagePart: &codersdk.ChatStreamMessagePart{
		Role: codersdk.ChatMessageRoleAssistant,
		Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: toolCallID, ToolName: toolName, ArgsDelta: delta},
	}}
}

func failingExperimentalClient() *codersdk.ExperimentalClient {
	return codersdk.NewExperimentalClient(codersdk.New(&url.URL{}))
}

func keyRunes(value string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)} }

func int64Ref(v int64) *int64 {
	return &v
}
