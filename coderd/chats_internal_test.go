package coderd

import (
	"database/sql"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/codersdk"
)

func TestParseChatGitReferenceOutput(t *testing.T) {
	t.Parallel()

	branch, origin := parseChatGitReferenceOutput("feature/chat-diff\nhttps://github.com/coder/coder.git\n")
	require.Equal(t, "feature/chat-diff", branch)
	require.Equal(t, "https://github.com/coder/coder.git", origin)

	branch, origin = parseChatGitReferenceOutput("single-line-only")
	require.Empty(t, branch)
	require.Empty(t, origin)
}

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

type assertionError string

func (e assertionError) Error() string {
	return string(e)
}
