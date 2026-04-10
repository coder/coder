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
				model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
				model.currentView = viewChat
				model.overlay = tt.overlay

				updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
				updated := mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, viewChat, updated.currentView)
				require.Equal(t, overlayNone, updated.overlay)
			})
		}

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

		for name, view := range map[string]tuiView{
			"List": viewList,
			"Chat": viewChat,
		} {
			t.Run("CtrlCQuitsFromAnyState/"+name, func(t *testing.T) {
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
					model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
					model.width, model.height = 100, 40
					updatedModel, cmd := model.Update(tt.msg)
					updated, cmd := mustTUIModelWithCmd(t, updatedModel, cmd)
					require.Equal(t, viewChat, updated.currentView)
					require.Equal(t, 39, updated.chat.height)
					require.Equal(t, 33, updated.chat.viewport.Height)
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
		t.Run("ReopensModelPickerAfterClosing", func(t *testing.T) {
			t.Parallel()
			model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
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
				model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
				model.currentView = viewChat
				model.width = 80
				model.height = 24
				updatedModel, cmd := model.Update(modelsListedMsg{catalog: twoModelCatalog})
				updated := mustTUIModel(t, updatedModel, cmd)

				updatedModel, cmd = updated.Update(toggleModelPickerMsg{})
				updated = mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, overlayModelPicker, updated.overlay)

				updatedModel, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
				updated = mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, overlayNone, updated.overlay)
			})

			t.Run("LoadErrorClosesOverlay", func(t *testing.T) {
				t.Parallel()
				model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
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
				model := newExpChatsTUIModel(context.Background(), nil, nil, nil, nil)
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
				updated = mustTUIModel(t, updatedModel, cmd)
				require.Equal(t, overlayNone, updated.overlay)
				require.NotNil(t, updated.chat.modelOverride)
				require.NotNil(t, updated.modelOverride)
				require.Equal(t, "openai:gpt-4.1", *updated.chat.modelOverride)
				require.Equal(t, "openai:gpt-4.1", *updated.modelOverride)
			})
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
				{name: "Draft/gitChangesMsg", msg: gitChangesMsg{generation: 1, chatID: uuid.New()}, draft: true},
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
			newToolRoundTripParts := func() []codersdk.ChatMessagePart {
				return []codersdk.ChatMessagePart{
					{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: "tool-1", ToolName: "search", Args: json.RawMessage(`{"q":"hello"}`)},
					{Type: codersdk.ChatMessagePartTypeToolResult, ToolCallID: "tool-1", ToolName: "search", Result: json.RawMessage(`{"ok":true}`)},
				}
			}

			tests := []struct {
				name         string
				messages     []codersdk.ChatMessage
				accumulator  streamAccumulator
				update       tea.Msg
				wantMessages int
				wantBlocks   int
				wantPending  bool
				wantText     string
				wantToolKind chatBlockKind
				wantToolID   string
			}{
				{
					name: "AccumulatorPending",
					update: func() tea.Msg {
						message := testMessage(12, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "final"})
						return chatStreamEventMsg{event: codersdk.ChatStreamEvent{Type: codersdk.ChatStreamEventTypeMessage, Message: &message}}
					}(),
					wantMessages: 1,
					wantBlocks:   1,
					wantText:     "final",
				},
				{
					name:         "FinalizedHistorySuppressesPendingToolDuplicate",
					messages:     []codersdk.ChatMessage{testMessage(1, codersdk.ChatMessageRoleAssistant, newToolRoundTripParts()...)},
					accumulator:  streamAccumulator{pending: true, role: codersdk.ChatMessageRoleAssistant, parts: newToolRoundTripParts()},
					wantMessages: 1,
					wantBlocks:   1,
					wantPending:  true,
					wantToolKind: blockToolResult,
					wantToolID:   "tool-1",
				},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					model := newTestChatViewModel(nil)
					if tt.name == "AccumulatorPending" {
						model, _ = model.Update(chatStreamEventMsg{event: testTextPartEvent("partial")})
					}
					model.messages = tt.messages
					model.accumulator = tt.accumulator
					model.rebuildBlocks()
					if tt.update != nil {
						var cmd tea.Cmd
						model, cmd = model.Update(tt.update)
						require.Nil(t, cmd)
					}
					require.Len(t, model.messages, tt.wantMessages)
					require.Len(t, model.blocks, tt.wantBlocks)
					require.Equal(t, tt.wantPending, model.accumulator.isPending())
					if tt.wantText != "" {
						require.Equal(t, tt.wantText, model.blocks[0].text)
					}
					if tt.wantToolID != "" {
						require.Equal(t, tt.wantToolKind, model.blocks[0].kind)
						require.Equal(t, tt.wantToolID, model.blocks[0].toolID)
					}
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
			tests := []struct {
				name  string
				setup func(chatViewModel) chatViewModel
				msg   tea.Msg
			}{
				{
					name: "messageSentMsg",
					setup: func(model chatViewModel) chatViewModel {
						chat := testChat(codersdk.ChatStatusCompleted)
						model.setChat(chat)
						return model
					},
					msg: messageSentMsg{err: xerrors.New("fail")},
				},
				{
					name: "chatCreatedMsg",
					setup: func(model chatViewModel) chatViewModel {
						model.draft = true
						return model
					},
					msg: chatCreatedMsg{err: xerrors.New("fail")},
				},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					model := tt.setup(newTestChatViewModel(nil))
					model.loading = false
					model.composer.SetValue("original")

					model, _ = model.sendMessage()
					require.Equal(t, "original", model.pendingComposerText)

					model.composer.SetValue("new input")

					model, _ = model.Update(tt.msg)
					require.Equal(t, "new input", model.composer.Value())
					require.Error(t, model.err)
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
				model.composer.SetValue("hello")
				updated, cmd := model.sendMessage()
				require.NotNil(t, cmd)
				require.Empty(t, updated.composer.Value())
				if tt.draft {
					msg, ok := mustMsg(t, cmd).(chatCreatedMsg)
					require.True(t, ok)
					require.NoError(t, msg.err)
					require.NotNil(t, createReq)
					require.NotNil(t, createReq.ModelConfigID)
					require.Equal(t, modelConfigID, *createReq.ModelConfigID)
					require.Equal(t, createdChat.ID, msg.chat.ID)
					return
				}
				msg, ok := mustMsg(t, cmd).(messageSentMsg)
				require.True(t, ok)
				require.NoError(t, msg.err)
				require.NotNil(t, messageReq)
				require.NotNil(t, messageReq.ModelConfigID)
				require.Equal(t, modelConfigID, *messageReq.ModelConfigID)
			})
		}
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
					require.Equal(t, 33, model.chat.viewport.Height)
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

		t.Run("WrappedComposerFitsTerminal", func(t *testing.T) {
			t.Parallel()
			model := applyWindowSize(t, newTestTUIModel(), 40, 18)
			model.currentView = viewChat
			model.chat.loading = false
			chat := testChat(codersdk.ChatStatusCompleted)
			model.chat.chat = &chat
			model.chat.chatStatus = chat.Status
			model.chat.messages = overflowingMessages(18)
			model.chat.rebuildBlocks()

			initialViewportHeight := model.chat.viewport.Height
			model.chat.composer.SetValue(strings.Repeat("wrapped input ", 14))
			model.chat.recalcViewportHeight()
			model.chat.syncViewportContent()

			require.Less(t, model.chat.viewport.Height, initialViewportHeight)
			require.LessOrEqual(t, strings.Count(model.View(), "\n")+1, 18)
		})

		t.Run("ViewShowsSingleStatusBarAndComposerDivider", func(t *testing.T) {
			t.Parallel()
			model := newTestChatViewModel(nil)
			model.loading = false
			model, _ = model.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
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
			model.chat.gitChanges = []codersdk.ChatGitChange{}
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

func TestExpAgents_View_LongInputFitsTerminal(t *testing.T) {
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
	lineCount := strings.Count(view, "\n") + 1

	require.LessOrEqual(t, lineCount, model.height)
	require.Less(t, model.viewport.Height, defaultViewportHeight)
	require.LessOrEqual(t, model.viewport.Height, 17)
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
