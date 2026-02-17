package coderd

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

func TestParseGitHubRepositoryOrigin(t *testing.T) {
	t.Parallel()

	owner, repo, normalized, ok := parseGitHubRepositoryOrigin("https://github.com/coder/coder.git")
	require.True(t, ok)
	require.Equal(t, "coder", owner)
	require.Equal(t, "coder", repo)
	require.Equal(t, "https://github.com/coder/coder", normalized)

	owner, repo, normalized, ok = parseGitHubRepositoryOrigin("git@github.com:coder/coder.git")
	require.True(t, ok)
	require.Equal(t, "coder", owner)
	require.Equal(t, "coder", repo)
	require.Equal(t, "https://github.com/coder/coder", normalized)

	owner, repo, normalized, ok = parseGitHubRepositoryOrigin("https://gitlab.com/coder/coder")
	require.False(t, ok)
	require.Empty(t, owner)
	require.Empty(t, repo)
	require.Empty(t, normalized)
}

func TestResolveExternalAuthProviderType(t *testing.T) {
	t.Parallel()

	api := &API{
		Options: &Options{
			ExternalAuthConfigs: []*externalauth.Config{
				{
					Type:  "github",
					Regex: regexp.MustCompile(`github\.com`),
				},
			},
		},
	}

	provider := api.resolveExternalAuthProviderType("https://github.com/coder/coder")
	require.Equal(t, "github", provider)

	provider = api.resolveExternalAuthProviderType("https://gitlab.com/coder/coder")
	require.Empty(t, provider)
}

func TestShouldRefreshChatDiffStatus(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	freshStatus := database.ChatDiffStatus{
		RefreshedAt: sql.NullTime{Time: now.Add(-time.Minute), Valid: true},
		StaleAt:     now.Add(time.Minute),
	}
	staleStatus := database.ChatDiffStatus{
		RefreshedAt: sql.NullTime{Time: now.Add(-time.Minute), Valid: true},
		StaleAt:     now.Add(-time.Second),
	}

	require.False(t, shouldRefreshChatDiffStatus(freshStatus, now, false))
	require.True(t, shouldRefreshChatDiffStatus(staleStatus, now, false))
	require.True(t, shouldRefreshChatDiffStatus(freshStatus, now, true))
	require.True(t, shouldRefreshChatDiffStatus(database.ChatDiffStatus{}, now, false))
}

func TestFilterChatsByWorkspaceID(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.New()
	otherWorkspaceID := uuid.New()

	matchingChat := database.Chat{
		ID:          uuid.New(),
		WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
	}
	otherWorkspaceChat := database.Chat{
		ID:          uuid.New(),
		WorkspaceID: uuid.NullUUID{UUID: otherWorkspaceID, Valid: true},
	}
	noWorkspaceChat := database.Chat{
		ID:          uuid.New(),
		WorkspaceID: uuid.NullUUID{},
	}

	filtered := filterChatsByWorkspaceID(
		[]database.Chat{matchingChat, otherWorkspaceChat, noWorkspaceChat},
		workspaceID,
	)

	require.Len(t, filtered, 1)
	require.Equal(t, matchingChat.ID, filtered[0].ID)
}

func TestChatWorkspaceAuditStatus(t *testing.T) {
	t.Parallel()

	t.Run("ResponderError", func(t *testing.T) {
		t.Parallel()

		err := httperror.NewResponseError(http.StatusBadRequest, codersdk.Response{
			Message: "invalid request",
		})
		require.Equal(t, http.StatusBadRequest, chatWorkspaceAuditStatus(err))
	})

	t.Run("GenericError", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, http.StatusInternalServerError, chatWorkspaceAuditStatus(assertionError("boom")))
	})
}

func TestSynthesizeChatWorkspaceRequestPreservesMetadata(t *testing.T) {
	t.Parallel()

	requestID := uuid.New()
	metadata := chatWorkspaceRequestMetadata{
		Header:     http.Header{"User-Agent": []string{"coder-test-agent"}},
		RemoteAddr: "203.0.113.42:9999",
		RequestID:  requestID.String(),
	}

	req, err := synthesizeChatWorkspaceRequest(
		context.Background(),
		"http://localhost/api/v2/chats/workspace",
		metadata,
	)
	require.NoError(t, err)
	require.Equal(t, metadata.RemoteAddr, req.RemoteAddr)
	require.Equal(t, metadata.Header.Get("User-Agent"), req.Header.Get("User-Agent"))
	require.Equal(t, requestID, httpmw.RequestID(req))
}

func TestSynthesizeChatWorkspaceRequestFallsBackToGeneratedRequestID(t *testing.T) {
	t.Parallel()

	req, err := synthesizeChatWorkspaceRequest(
		context.Background(),
		"http://localhost/api/v2/chats/workspace",
		chatWorkspaceRequestMetadata{RequestID: "not-a-uuid"},
	)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, httpmw.RequestID(req))
}

func TestConvertChatMessagesSkipsWorkspaceMetadata(t *testing.T) {
	t.Parallel()

	messages := []database.ChatMessage{
		{
			ID:   1,
			Role: "user",
		},
		{
			ID:     2,
			Role:   chatWorkspaceRequestMetadataRole,
			Hidden: true,
		},
	}

	converted := convertChatMessages(messages)
	require.Len(t, converted, 1)
	require.Equal(t, int64(1), converted[0].ID)
}

func TestChatWorkspaceRequestMetadataFromRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "http://example.test/chats", nil)
	req.Header.Set("User-Agent", "coder-test-agent")
	req.RemoteAddr = "203.0.113.42:9999"

	requestID := uuid.New()
	req = req.WithContext(httpmw.WithRequestID(context.Background(), requestID))

	metadata := chatWorkspaceRequestMetadataFromRequest(req)
	require.Equal(t, "203.0.113.42:9999", metadata.RemoteAddr)
	require.Equal(t, requestID.String(), metadata.RequestID)
	require.Equal(t, "coder-test-agent", metadata.Header.Get("User-Agent"))
}

type assertionError string

func (e assertionError) Error() string {
	return string(e)
}
