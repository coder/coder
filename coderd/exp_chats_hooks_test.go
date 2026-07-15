package coderd_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agenthooks"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestPostChatsInitialPromptHookErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		statusCode  int
		response    string
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "deny",
			statusCode:  http.StatusOK,
			response:    `{"permission":{"decision":"deny"},"user_message":"blocked by policy"}`,
			wantStatus:  http.StatusForbidden,
			wantMessage: "blocked by policy",
		},
		{
			name:       "dispatch failure",
			statusCode: http.StatusInternalServerError,
			wantStatus: http.StatusBadGateway,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			requests := make(chan agenthooks.Request, 2)
			consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request agenthooks.Request
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				requests <- request
				w.WriteHeader(test.statusCode)
				if test.response != "" {
					_, err := w.Write([]byte(test.response))
					require.NoError(t, err)
				}
			}))
			t.Cleanup(consumer.Close)

			client, db := newChatClientWithDatabase(t, func(opts *coderdtest.Options) {
				opts.ChatWorkerDisabled = true
				require.NoError(t, opts.DeploymentValues.AI.Chat.HookURL.Set(consumer.URL))
				opts.DeploymentValues.AI.Chat.HookSecret = serpent.String("test-hook-secret-32-bytes-minimum!!")
				opts.DeploymentValues.AI.Chat.HookTimeout = serpent.Duration(time.Second)
				opts.DeploymentValues.AI.Chat.HookEnabled = serpent.Bool(true)
			})
			user := coderdtest.CreateFirstUser(t, client.Client)
			model := createAdditionalChatModelConfig(t, client, "openai", "gpt-4.1")
			ctx := testutil.Context(t, testutil.WaitLong)

			_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: user.OrganizationID,
				ModelConfigID:  &model.ID,
				Content: []codersdk.ChatInputPart{{
					Type: codersdk.ChatInputPartTypeText,
					Text: "blocked prompt",
				}},
			})
			sdkErr := coderdtest.SDKError(t, err)
			require.Equal(t, test.wantStatus, sdkErr.StatusCode())
			if test.wantMessage != "" {
				require.Equal(t, test.wantMessage, sdkErr.Message)
			}
			request := testutil.RequireReceive(ctx, t, requests)
			require.Equal(t, agenthooks.EventUserPromptSubmit, request.Type)
			require.NotEqual(t, uuid.Nil, request.Meta.ChatID)
			_, err = db.GetChatByID(dbauthz.AsSystemRestricted(ctx), request.Meta.ChatID)
			require.ErrorIs(t, err, sql.ErrNoRows)
		})
	}
}
