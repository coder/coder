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

	"github.com/google/uuid"
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

func TestRenderProjectMemoryTranscript(t *testing.T) {
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
		message(t, 3, database.ChatMessageRoleAssistant, "durable assistant detail", 5),
	}
	transcript := renderProjectMemoryTranscript(messages, 3)
	require.NotContains(t, transcript, "old user detail")
	require.NotContains(t, transcript, "tool output")
	require.Contains(t, transcript, "durable assistant detail")
}

func TestNormalizeProjectMemoryExtraction(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeProjectMemoryExtraction(projectMemoryExtractionUpsert{
		Name:        "Release_Notes",
		Type:        database.ChatProjectMemoryTypeProject,
		Description: "<project-memory>Durable release process</project-memory>",
		Body:        "<project-memory>Run the checklist.</project-memory>",
	})
	require.NoError(t, err)
	require.Equal(t, "release_notes", normalized.Name)
	require.Equal(t, "Durable release process", normalized.Description)
	require.Equal(t, "Run the checklist.", normalized.Body)

	_, err = normalizeProjectMemoryExtraction(projectMemoryExtractionUpsert{
		Name:        "invalid name",
		Type:        database.ChatProjectMemoryTypeProject,
		Description: "Description",
		Body:        "Body",
	})
	require.Error(t, err)
}

func TestExtractProjectMemories(t *testing.T) {
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
		db.EXPECT().GetChatProjectMemoryCursor(gomock.Any(), chat.ID).Return(
			database.ChatProjectMemoryCursor{ChatID: chat.ID, HistoryVersion: chat.HistoryVersion}, nil,
		)

		newServer(t, db, nil).extractProjectMemories(t.Context(), slogtest.Make(t, nil), chat)
	})

	t.Run("AppliesUpsertsAndDeletesAndAdvancesCursor", func(t *testing.T) {
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
						"type":        database.ChatProjectMemoryTypeProject,
						"description": "<project-memory>Durable release process</project-memory>",
						"body":        "<project-memory>Run the release checklist.</project-memory>",
					},
					{
						"name":        "Bad Name!",
						"type":        database.ChatProjectMemoryTypeProject,
						"description": "Ignored",
						"body":        "Ignored",
					},
				},
				"deletes": []string{" Old_Memory "},
			})
			response.Request = req
			return response, nil
		}))
		validUpsert := database.UpsertChatProjectMemoryByNameParams{
			ProjectID:      chat.ProjectID.UUID,
			OrganizationID: chat.OrganizationID,
			Type:           database.ChatProjectMemoryTypeProject,
			Name:           "release_notes",
			Description:    "Durable release process",
			Body:           "Run the release checklist.",
			SourceChatID:   uuid.NullUUID{UUID: chat.ID, Valid: true},
			CreatedBy:      chat.OwnerID,
		}

		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectMemoryCursor(gomock.Any(), chat.ID).Return(
				database.ChatProjectMemoryCursor{ChatID: chat.ID, HistoryVersion: 3}, nil,
			),
			db.EXPECT().GetChatMessagesForPromptByChatID(gomock.Any(), chat.ID).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "old detail", 2),
				message(t, 2, database.ChatMessageRoleAssistant, "new durable detail", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)
		gomock.InOrder(
			db.EXPECT().GetChatProjectMemoryByName(gomock.Any(), database.GetChatProjectMemoryByNameParams{
				ProjectID: chat.ProjectID.UUID,
				Name:      "release_notes",
			}).Return(database.GetChatProjectMemoryByNameRow{}, nil),
			db.EXPECT().UpsertChatProjectMemoryByName(gomock.Any(), validUpsert).Return(database.ChatProjectMemory{}, nil),
			db.EXPECT().DeleteChatProjectMemoryByName(gomock.Any(), database.DeleteChatProjectMemoryByNameParams{
				ProjectID: chat.ProjectID.UUID,
				Name:      "old_memory",
			}).Return(nil),
			db.EXPECT().UpsertChatProjectMemoryCursor(gomock.Any(), database.UpsertChatProjectMemoryCursorParams{
				ChatID:         chat.ID,
				HistoryVersion: chat.HistoryVersion,
			}).Return(database.ChatProjectMemoryCursor{}, nil),
		)

		server.extractProjectMemories(t.Context(), slogtest.Make(t, nil), chat)

		require.Contains(t, capturedPrompt, "new durable detail")
		require.NotContains(t, capturedPrompt, "old detail")
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
					"type":        database.ChatProjectMemoryTypeProject,
					"description": "Durable release process",
					"body":        "Run the release checklist.",
				}},
				"deletes": []string{},
			})
			response.Request = req
			return response, nil
		}))

		gomock.InOrder(
			db.EXPECT().GetChatByID(gomock.Any(), chat.ID).Return(chat, nil),
			db.EXPECT().GetChatProjectMemoryCursor(gomock.Any(), chat.ID).Return(database.ChatProjectMemoryCursor{}, sql.ErrNoRows),
			db.EXPECT().GetChatMessagesForPromptByChatID(gomock.Any(), chat.ID).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "new durable detail", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)
		gomock.InOrder(
			db.EXPECT().GetChatProjectMemoryByName(gomock.Any(), database.GetChatProjectMemoryByNameParams{
				ProjectID: chat.ProjectID.UUID,
				Name:      "release_notes",
			}).Return(database.GetChatProjectMemoryByNameRow{}, sql.ErrNoRows),
			db.EXPECT().CountChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(int64(chattool.MaxProjectMemories), nil),
			db.EXPECT().UpsertChatProjectMemoryCursor(gomock.Any(), database.UpsertChatProjectMemoryCursorParams{
				ChatID:         chat.ID,
				HistoryVersion: chat.HistoryVersion,
			}).Return(database.ChatProjectMemoryCursor{}, nil),
		)

		server.extractProjectMemories(t.Context(), slogtest.Make(t, nil), chat)
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
			db.EXPECT().GetChatProjectMemoryCursor(gomock.Any(), chat.ID).Return(database.ChatProjectMemoryCursor{}, sql.ErrNoRows),
			db.EXPECT().GetChatMessagesForPromptByChatID(gomock.Any(), chat.ID).Return([]database.ChatMessage{
				message(t, 1, database.ChatMessageRoleUser, "new durable detail", 5),
			}, nil),
			db.EXPECT().GetChatProjectMemoriesByProjectID(gomock.Any(), chat.ProjectID.UUID).Return(nil, nil),
		)
		expectModelResolution(db, chat)

		server.extractProjectMemories(t.Context(), slogtest.Make(t, nil), chat)
	})
}

func TestMaybeExtractProjectMemoriesAsyncSkips(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		chat        database.Chat
		experiments codersdk.Experiments
	}{
		{
			name: "ParentChat",
			chat: database.Chat{
				ID:           uuid.New(),
				ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
				ProjectID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
			},
			experiments: codersdk.Experiments{codersdk.ExperimentChatProjects},
		},
		{
			name:        "NoProject",
			chat:        database.Chat{ID: uuid.New()},
			experiments: codersdk.Experiments{codersdk.ExperimentChatProjects},
		},
		{
			name: "ExperimentDisabled",
			chat: database.Chat{
				ID:        uuid.New(),
				ProjectID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			serverCtx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			server := &Server{
				ctx:         serverCtx,
				cancel:      cancel,
				db:          db,
				experiments: tt.experiments,
			}

			server.maybeExtractProjectMemoriesAsync(t.Context(), slogtest.Make(t, nil), tt.chat)
		})
	}
}
