package cli //nolint:testpackage // Tests unexported local diff fallback helpers.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestFetchChatDiffContents(t *testing.T) {
	t.Parallel()

	t.Run("FallsBackToLocalGitWatcher", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		chatID := uuid.New()
		path := fmt.Sprintf("/api/experimental/chats/%s", chatID)
		client := newTestExperimentalClient(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case path + "/diff":
				rw.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.ChatDiffContents{ChatID: chatID}))
			case path + "/stream/git":
				conn, err := websocket.Accept(rw, r, nil)
				require.NoError(t, err)
				defer conn.Close(websocket.StatusNormalClosure, "")

				_, payload, err := conn.Read(ctx)
				require.NoError(t, err)
				var refresh codersdk.WorkspaceAgentGitClientMessage
				require.NoError(t, json.Unmarshal(payload, &refresh))
				require.Equal(t, codersdk.WorkspaceAgentGitClientMessageTypeRefresh, refresh.Type)

				writer, err := conn.Writer(ctx, websocket.MessageText)
				require.NoError(t, err)
				require.NoError(t, json.NewEncoder(writer).Encode(codersdk.WorkspaceAgentGitServerMessage{
					Type: codersdk.WorkspaceAgentGitServerMessageTypeChanges,
					Repositories: []codersdk.WorkspaceAgentRepoChanges{{
						RepoRoot:     "/workspace/repo",
						Branch:       "feature/local-diff",
						RemoteOrigin: "https://github.com/coder/coder.git",
						UnifiedDiff:  "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n",
					}},
				}))
				require.NoError(t, writer.Close())
			default:
				http.NotFound(rw, r)
			}
		}))

		diff, err := fetchChatDiffContents(ctx, client, chatID)
		require.NoError(t, err)
		require.NotNil(t, diff.Branch)
		require.Equal(t, "feature/local-diff", *diff.Branch)
		require.NotNil(t, diff.RemoteOrigin)
		require.Equal(t, "https://github.com/coder/coder.git", *diff.RemoteOrigin)
		require.Contains(t, diff.Diff, "diff --git a/a.txt b/a.txt")
		require.Contains(t, diff.Diff, "+new")
	})

	t.Run("IgnoresTimedOutWatcherFallbackErrors", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.IntervalMedium)
		defer cancel()

		handlerDone := make(chan struct{})
		chatID := uuid.New()
		path := fmt.Sprintf("/api/experimental/chats/%s", chatID)
		client := newTestExperimentalClient(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case path + "/diff":
				rw.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.ChatDiffContents{ChatID: chatID}))
			case path + "/stream/git":
				defer close(handlerDone)

				conn, err := websocket.Accept(rw, r, nil)
				require.NoError(t, err)
				defer conn.Close(websocket.StatusNormalClosure, "")

				_, payload, err := conn.Read(r.Context())
				require.NoError(t, err)
				var refresh codersdk.WorkspaceAgentGitClientMessage
				require.NoError(t, json.Unmarshal(payload, &refresh))
				require.Equal(t, codersdk.WorkspaceAgentGitClientMessageTypeRefresh, refresh.Type)

				time.Sleep(testutil.IntervalSlow)
			default:
				http.NotFound(rw, r)
			}
		}))

		diff, err := fetchChatDiffContents(ctx, client, chatID)
		require.NoError(t, err)
		require.Equal(t, chatID, diff.ChatID)
		require.Empty(t, diff.Diff)
		require.Eventually(t, func() bool {
			select {
			case <-handlerDone:
				return true
			default:
				return false
			}
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("IgnoresMissingWorkspaceFallbackErrors", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		chatID := uuid.New()
		path := fmt.Sprintf("/api/experimental/chats/%s", chatID)
		client := newTestExperimentalClient(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case path + "/diff":
				rw.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.ChatDiffContents{ChatID: chatID}))
			case path + "/stream/git":
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusBadRequest)
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.Response{Message: "Chat has no workspace to watch."}))
			default:
				http.NotFound(rw, r)
			}
		}))

		diff, err := fetchChatDiffContents(ctx, client, chatID)
		require.NoError(t, err)
		require.Equal(t, chatID, diff.ChatID)
		require.Empty(t, diff.Diff)
	})
}

func TestBuildLocalChatDiffContents(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	diff := buildLocalChatDiffContents(chatID, []codersdk.WorkspaceAgentRepoChanges{
		{
			RepoRoot:    "/workspace/z-repo",
			UnifiedDiff: "diff --git a/z.txt b/z.txt\n+z\n",
		},
		{
			RepoRoot:     "/workspace/a-repo",
			Branch:       "feature/local",
			RemoteOrigin: "https://github.com/coder/coder.git",
			UnifiedDiff:  "diff --git a/a.txt b/a.txt\n+a\n",
		},
	})

	require.Equal(t, chatID, diff.ChatID)
	require.Contains(t, diff.Diff, "diff --git a/a.txt b/a.txt")
	require.Contains(t, diff.Diff, "diff --git a/z.txt b/z.txt")
	require.Less(t, strings.Index(diff.Diff, "a.txt"), strings.Index(diff.Diff, "z.txt"))
	require.Nil(t, diff.Branch)
	require.Nil(t, diff.RemoteOrigin)
}
