package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

func TestRenderMemoryTranscript(t *testing.T) {
	t.Parallel()

	message := func(t *testing.T, id int64, role database.ChatMessageRole, text string, revision int64) database.ChatMessage {
		t.Helper()
		encoded, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
		require.NoError(t, err)
		return database.ChatMessage{
			ID:             id,
			Role:           role,
			Visibility:     database.ChatMessageVisibilityBoth,
			Content:        pqtype.NullRawMessage{RawMessage: encoded.RawMessage, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			Revision:       revision,
		}
	}

	messages := []database.ChatMessage{
		message(t, 1, database.ChatMessageRoleUser, "old user detail", 3),
		message(t, 2, database.ChatMessageRoleTool, "tool output", 5),
		message(t, 3, database.ChatMessageRoleAssistant, "assistant restatement", 5),
		message(t, 4, database.ChatMessageRoleUser, "new user detail", 5),
	}
	transcript := renderMemoryTranscript(messages, 3)
	require.NotContains(t, transcript, "old user detail")
	require.NotContains(t, transcript, "tool output")
	require.NotContains(t, transcript, "assistant restatement")
	require.Contains(t, transcript, "new user detail")

	largeTranscript := renderMemoryTranscript([]database.ChatMessage{
		message(t, 5, database.ChatMessageRoleUser, strings.Repeat("界", memoryExtractionTranscriptMaxBytes)+"tail", 5),
	}, 3)
	require.LessOrEqual(t, len(largeTranscript), memoryExtractionTranscriptMaxBytes)
	require.True(t, utf8.ValidString(largeTranscript))
	require.True(t, strings.HasPrefix(largeTranscript, "[truncated] "))
	require.True(t, strings.HasSuffix(largeTranscript, "tail"))
}

func TestTurnUsedMemoryTools(t *testing.T) {
	t.Parallel()

	message := func(t *testing.T, role database.ChatMessageRole, parts []codersdk.ChatMessagePart) database.ChatMessage {
		t.Helper()
		content, err := chatprompt.MarshalParts(parts)
		require.NoError(t, err)
		return database.ChatMessage{
			Role:           role,
			Content:        pqtype.NullRawMessage{RawMessage: content.RawMessage, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			Revision:       5,
		}
	}

	t.Run("SuccessfulMatchingResult", func(t *testing.T) {
		t.Parallel()
		require.True(t, turnUsedMemoryTools([]database.ChatMessage{
			message(t, database.ChatMessageRoleAssistant, []codersdk.ChatMessagePart{
				codersdk.ChatMessageToolCall("save-1", chattool.SaveMemoryToolName, []byte(`{}`)),
			}),
			message(t, database.ChatMessageRoleTool, []codersdk.ChatMessagePart{
				codersdk.ChatMessageToolResult("save-1", chattool.SaveMemoryToolName, []byte(`{}`), false, false),
			}),
		}, 3))
	})

	t.Run("FailedSaveDoesNotSuppressExtraction", func(t *testing.T) {
		t.Parallel()
		require.False(t, turnUsedMemoryTools([]database.ChatMessage{
			message(t, database.ChatMessageRoleAssistant, []codersdk.ChatMessagePart{
				codersdk.ChatMessageToolCall("save-1", chattool.SaveMemoryToolName, []byte(`{}`)),
			}),
			message(t, database.ChatMessageRoleTool, []codersdk.ChatMessagePart{
				codersdk.ChatMessageToolResult("save-1", chattool.SaveMemoryToolName, []byte(`{"error":"memory limit reached"}`), true, false),
			}),
		}, 3))
	})
}

func TestNormalizeMemoryExtraction(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeMemoryExtraction(memoryExtractionUpsert{
		Name:        "Release_Notes",
		Description: "<project-memory>Durable release process</project-memory>",
		Body:        "<project-memory>Run the checklist.</project-memory>",
	})
	require.NoError(t, err)
	require.Equal(t, "release_notes", normalized.Name)
	require.Equal(t, "Durable release process", normalized.Description)
	require.Equal(t, "Run the checklist.", normalized.Body)

	_, err = normalizeMemoryExtraction(memoryExtractionUpsert{
		Name:        "invalid name",
		Description: "Description",
		Body:        "Body",
	})
	require.Error(t, err)
}

func TestExtractMemories(t *testing.T) {
	t.Parallel()

	newChat := func() database.Chat {
		return database.Chat{
			ID:                uuid.New(),
			OwnerID:           uuid.New(),
			OrganizationID:    uuid.New(),
			ProjectID:         uuid.NullUUID{UUID: uuid.New(), Valid: true},
			LastModelConfigID: uuid.New(),
			HistoryVersion:    6,
		}
	}
	newServer := func(t *testing.T, db database.Store, roundTripper http.RoundTripper) *Server {
		t.Helper()
		return &Server{
			db:                       db,
			logger:                   slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			clock:                    quartz.NewReal(),
			aibridgeTransportFactory: aibridgeTestFactoryPointer(&aibridgeTestFactory{rt: roundTripper}),
		}
	}
	message := func(t *testing.T, id int64, role database.ChatMessageRole, text string, revision int64) database.ChatMessage {
		t.Helper()
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
		require.NoError(t, err)
		return database.ChatMessage{
			ID:             id,
			Role:           role,
			Visibility:     database.ChatMessageVisibilityBoth,
			Content:        pqtype.NullRawMessage{RawMessage: content.RawMessage, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			Revision:       revision,
		}
	}
	expectModelResolution := func(db *dbmock.MockStore, chat database.Chat) {
		providerID := uuid.New()
		config := database.ChatModelConfig{
			ID:             chat.LastModelConfigID,
			Model:          "gpt-4o-mini",
			Enabled:        true,
			OrganizationID: chat.OrganizationID,
			AIProviderID:   uuid.NullUUID{UUID: providerID, Valid: true},
		}
		db.EXPECT().GetChatGatewayAPIKey(gomock.Any(), database.GetChatGatewayAPIKeyParams{
			UserID:    chat.OwnerID,
			TokenName: GatewayTokenName(chat.OwnerID),
		}).Return(database.APIKey{
			ID:        uuid.NewString(),
			ExpiresAt: time.Now().Add(syntheticAPIKeyLifetime),
		}, nil)
		db.EXPECT().GetEnabledChatModelConfigByID(gomock.Any(), chat.LastModelConfigID).Return(config, nil)
		db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(
			aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil,
		)
	}
	objectResponse := func(t *testing.T, object any) *http.Response {
		t.Helper()
		objectJSON, err := json.Marshal(object)
		require.NoError(t, err)
		responseJSON, err := json.Marshal(map[string]any{
			"id":         "resp_project_memory",
			"object":     "response",
			"created_at": 0,
			"status":     "completed",
			"model":      "gpt-4o-mini",
			"output": []map[string]any{{
				"id":   "msg_project_memory",
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{{
					"type": "output_text",
					"text": string(objectJSON),
				}},
			}},
			"usage": map[string]any{
				"input_tokens":  1,
				"output_tokens": 1,
				"total_tokens":  2,
			},
		})
		require.NoError(t, err)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(responseJSON))),
		}
	}

	t.Run("SkipsWhenCursorCurrent", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil)
		db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil)
		db.EXPECT().GetChatMemoryCursor(gomock.Any(), chat.ID).Return(
			database.ChatMemoryCursor{ChatID: chat.ID, HistoryVersion: chat.HistoryVersion}, nil,
		)

		newServer(t, db, nil).extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("SkipsModelCallWhenAgentSavedMemoryThisTurn", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		encoded, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageToolCall("call-1", chattool.SaveMemoryToolName, []byte(`{"name":"x"}`)),
		})
		require.NoError(t, err)
		saveCall := database.ChatMessage{
			ID:             2,
			Role:           database.ChatMessageRoleAssistant,
			Visibility:     database.ChatMessageVisibilityBoth,
			Content:        pqtype.NullRawMessage{RawMessage: encoded.RawMessage, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			Revision:       5,
		}
		resultContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult("call-1", chattool.SaveMemoryToolName, []byte(`{}`), false, false),
		})
		require.NoError(t, err)
		saveResult := database.ChatMessage{
			ID:             3,
			Role:           database.ChatMessageRoleTool,
			Visibility:     database.ChatMessageVisibilityBoth,
			Content:        pqtype.NullRawMessage{RawMessage: resultContent.RawMessage, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
			Revision:       5,
		}
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().GetChatMemoryCursor(gomock.Any(), chat.ID).Return(
				database.ChatMemoryCursor{ChatID: chat.ID, HistoryVersion: 3}, nil,
			),
			db.EXPECT().GetChatMessagesForPromptByChatID(gomock.Any(), chat.ID).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "remember this", 5),
				saveCall,
				saveResult,
			}, nil),
			// The cursor advances without loading memories or calling the model.
			db.EXPECT().UpsertChatMemoryCursor(gomock.Any(), database.UpsertChatMemoryCursorParams{
				ChatID:         chat.ID,
				HistoryVersion: chat.HistoryVersion,
			}).Return(database.ChatMemoryCursor{}, nil),
		)

		newServer(t, db, nil).extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("AppliesUpsertsAndAdvancesCursor", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		var capturedPrompt string
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			capturedPrompt = string(body)
			response := objectResponse(t, map[string]any{
				"upserts": []map[string]any{
					{
						"name":        "Release_Notes",
						"description": "<project-memory>Durable release process</project-memory>",
						"body":        "<project-memory>Run the release checklist.</project-memory>",
					},
					{
						"name":        "Bad Name!",
						"description": "Ignored",
						"body":        "Ignored",
					},
				},
			})
			response.Request = req
			return response, nil
		}))
		validUpsert := database.InsertChatProjectMemoryParams{
			ID:             uuid.NullUUID{},
			ProjectID:      chat.ProjectID.UUID,
			OrganizationID: chat.OrganizationID,
			Name:           "release_notes",
			Description:    "Durable release process",
			Body:           "Run the release checklist.",
			SourceChatID:   uuid.NullUUID{UUID: chat.ID, Valid: true},
			CreatedBy:      chat.OwnerID,
		}

		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().GetChatMemoryCursor(gomock.Any(), chat.ID).Return(
				database.ChatMemoryCursor{ChatID: chat.ID, HistoryVersion: 3}, nil,
			),
			db.EXPECT().GetChatMessagesForPromptByChatID(gomock.Any(), chat.ID).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "old detail", 2),
				message(t, 2, database.ChatMessageRoleUser, "new durable detail", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)
		gomock.InOrder(
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(0), nil),
			db.EXPECT().InsertChatProjectMemory(gomock.Any(), validUpsert).Return(database.ChatProjectMemory{}, nil),
			db.EXPECT().UpsertChatMemoryCursor(gomock.Any(), database.UpsertChatMemoryCursorParams{
				ChatID:         chat.ID,
				HistoryVersion: chat.HistoryVersion,
			}).Return(database.ChatMemoryCursor{}, nil),
		)

		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)

		require.Contains(t, capturedPrompt, "new durable detail")
		require.NotContains(t, capturedPrompt, "old detail")
		// Deletion is intentionally absent from the extraction schema.
		require.NotContains(t, capturedPrompt, `"deletes"`)
	})

	t.Run("NeverOverwritesExistingMemory", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			response := objectResponse(t, map[string]any{
				"upserts": []map[string]any{{
					"name":        "release_notes",
					"description": "Hallucinated rewrite",
					"body":        "Deploy day is Friday.",
				}},
			})
			response.Request = req
			return response, nil
		}))
		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().GetChatMemoryCursor(gomock.Any(), chat.ID).Return(
				database.ChatMemoryCursor{ChatID: chat.ID, HistoryVersion: 3}, nil,
			),
			db.EXPECT().GetChatMessagesForPromptByChatID(gomock.Any(), chat.ID).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "when do we deploy?", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)
		gomock.InOrder(
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(0), nil),
			db.EXPECT().InsertChatProjectMemory(gomock.Any(), database.InsertChatProjectMemoryParams{
				ID:             uuid.NullUUID{},
				ProjectID:      chat.ProjectID.UUID,
				OrganizationID: chat.OrganizationID,
				Name:           "release_notes",
				Description:    "Hallucinated rewrite",
				Body:           "Deploy day is Friday.",
				SourceChatID:   uuid.NullUUID{UUID: chat.ID, Valid: true},
				CreatedBy:      chat.OwnerID,
			}).Return(database.ChatProjectMemory{}, &pq.Error{Code: "23505"}),
			// A unique constraint conflict leaves the existing memory alone.
			db.EXPECT().UpsertChatMemoryCursor(gomock.Any(), database.UpsertChatMemoryCursorParams{
				ChatID:         chat.ID,
				HistoryVersion: chat.HistoryVersion,
			}).Return(database.ChatMemoryCursor{}, nil),
		)

		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("RespectsCapForNewNames", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			response := objectResponse(t, map[string]any{
				"upserts": []map[string]any{{
					"name":        "release_notes",
					"description": "Durable release process",
					"body":        "Run the release checklist.",
				}},
			})
			response.Request = req
			return response, nil
		}))

		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().GetChatMemoryCursor(gomock.Any(), chat.ID).Return(database.ChatMemoryCursor{}, sql.ErrNoRows),
			db.EXPECT().GetChatMessagesForPromptByChatID(gomock.Any(), chat.ID).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "new durable detail", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)
		gomock.InOrder(
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(chattool.MaxMemories), nil),
			db.EXPECT().UpsertChatMemoryCursor(gomock.Any(), database.UpsertChatMemoryCursorParams{
				ChatID:         chat.ID,
				HistoryVersion: chat.HistoryVersion,
			}).Return(database.ChatMemoryCursor{}, nil),
		)

		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("ModelFailureDoesNotAdvanceCursor", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := newChat()
		server := newServer(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"model failed"}}`)),
				Request:    req,
			}, nil
		}))

		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectByID(gomock.Any(), chat.ProjectID.UUID).Return(database.ChatProject{Name: "platform"}, nil),
			db.EXPECT().GetChatMemoryCursor(gomock.Any(), chat.ID).Return(database.ChatMemoryCursor{}, sql.ErrNoRows),
			db.EXPECT().GetChatMessagesForPromptByChatID(gomock.Any(), chat.ID).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "new durable detail", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)

		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})
}

func TestResolveMemoryScope(t *testing.T) {
	t.Parallel()
	t.Run("Personal", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		userID := uuid.New()
		db.EXPECT().GetUserChatPersonalMemoryEnabled(gomock.Any(), userID).Return("true", nil)
		server := &Server{db: db, logger: slogtest.Make(t, nil), configCache: newChatConfigCache(t.Context(), db, quartz.NewReal())}
		_, scope, ok := server.resolveMemoryScope(t.Context(), database.Chat{ID: uuid.New(), OwnerID: userID, OrganizationID: uuid.New()})
		require.True(t, ok)
		require.Equal(t, chattool.MemoryScopePersonal, scope.Kind)
	})
	t.Run("ToggleOffSkipsModelCall", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New(), OrganizationID: uuid.New(), HistoryVersion: 1}
		db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil)
		db.EXPECT().GetUserChatPersonalMemoryEnabled(gomock.Any(), chat.OwnerID).Return("false", nil)
		server := &Server{db: db, logger: slogtest.Make(t, nil), configCache: newChatConfigCache(t.Context(), db, quartz.NewReal())}
		server.extractMemories(t.Context(), slogtest.Make(t, nil), chat)
	})
	t.Run("Project", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		projectID := uuid.New()
		db.EXPECT().GetChatProjectByID(gomock.Any(), projectID).Return(database.ChatProject{Name: "platform"}, nil)
		server := &Server{db: db, logger: slogtest.Make(t, nil)}
		_, scope, ok := server.resolveMemoryScope(context.Background(), database.Chat{ID: uuid.New(), OwnerID: uuid.New(), OrganizationID: uuid.New(), ProjectID: uuid.NullUUID{UUID: projectID, Valid: true}})
		require.True(t, ok)
		require.Equal(t, chattool.MemoryScopeProject, scope.Kind)
		require.Equal(t, "platform", scope.Label)
	})
	t.Run("PersonalAbsentDefaultsEnabled", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		userID := uuid.New()
		db.EXPECT().GetUserChatPersonalMemoryEnabled(gomock.Any(), userID).Return("", sql.ErrNoRows)
		server := &Server{db: db, logger: slogtest.Make(t, nil), configCache: newChatConfigCache(t.Context(), db, quartz.NewReal())}
		_, _, ok := server.resolveMemoryScope(t.Context(), database.Chat{ID: uuid.New(), OwnerID: userID, OrganizationID: uuid.New()})
		require.True(t, ok)
	})
}
