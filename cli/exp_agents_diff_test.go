package cli //nolint:testpackage // Tests unexported local diff fallback helpers.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

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

		ctx := t.Context()
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

		ctx, cancel := context.WithTimeout(t.Context(), testutil.IntervalMedium)
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

				// Keep the WebSocket open until the client disconnects
				// (either from fetchChatDiffContents hitting its watch
				// timeout or test cleanup closing the connection)
				// instead of sleeping for a fixed duration. The second
				// Read blocks on the socket and unblocks with an error
				// when the peer closes the connection, so this handler
				// drains cleanly without time.Sleep (see WORKFLOWS.md).
				_, _, _ = conn.Read(r.Context())
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

		// Each message here matches a 400 response that watchChatGit can
		// return when the chat cannot be observed through the workspace
		// agent. fetchChatDiffContents should swallow the error and fall
		// back to the empty remote diff instead of surfacing a hard
		// error in the TUI.
		for _, message := range []string{
			"Chat has no workspace to watch.",
			"Chat workspace not found.",
			"Chat workspace has no agents.",
			`Agent state is "connecting", it must be in the "connected" state.`,
		} {
			t.Run(message, func(t *testing.T) {
				t.Parallel()

				ctx := t.Context()
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
						require.NoError(t, json.NewEncoder(rw).Encode(codersdk.Response{Message: message}))
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
	})

	t.Run("ReturnsRemoteDiffWithoutDialingWatcher", func(t *testing.T) {
		t.Parallel()

		// When the remote /diff endpoint returns a non-empty diff the
		// CLI short-circuits the WebSocket fallback. If the git stream
		// handler ever fires, the test fails the request explicitly so
		// an inverted condition regresses loudly.
		ctx := t.Context()
		chatID := uuid.New()
		path := fmt.Sprintf("/api/experimental/chats/%s", chatID)
		branch := "feature/remote"
		prURL := "https://example.com/pr/1"
		remoteDiff := codersdk.ChatDiffContents{
			ChatID:         chatID,
			Branch:         &branch,
			PullRequestURL: &prURL,
			Diff:           "diff --git a/remote.txt b/remote.txt\n--- a/remote.txt\n+++ b/remote.txt\n@@ -1 +1 @@\n-old\n+new\n",
		}
		client := newTestExperimentalClient(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case path + "/diff":
				rw.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(rw).Encode(remoteDiff))
			case path + "/stream/git":
				t.Errorf("local git watcher should not be dialed when the remote diff is non-empty")
				rw.WriteHeader(http.StatusInternalServerError)
			default:
				http.NotFound(rw, r)
			}
		}))

		got, err := fetchChatDiffContents(ctx, client, chatID)
		require.NoError(t, err)
		require.Equal(t, chatID, got.ChatID)
		require.Equal(t, remoteDiff.Diff, got.Diff)
		require.NotNil(t, got.Branch)
		require.Equal(t, branch, *got.Branch)
		require.NotNil(t, got.PullRequestURL)
		require.Equal(t, prURL, *got.PullRequestURL)
	})

	t.Run("PropagatesRemoteDiffAPIErrors", func(t *testing.T) {
		t.Parallel()

		// A 500 from /diff is a hard failure that the CLI must surface
		// rather than silently fall back. The local watcher must not
		// be dialed when the remote endpoint returned an error.
		ctx := t.Context()
		chatID := uuid.New()
		path := fmt.Sprintf("/api/experimental/chats/%s", chatID)
		client := newTestExperimentalClient(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case path + "/diff":
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusInternalServerError)
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.Response{Message: "boom"}))
			case path + "/stream/git":
				t.Errorf("local git watcher should not be dialed when /diff errors")
				rw.WriteHeader(http.StatusInternalServerError)
			default:
				http.NotFound(rw, r)
			}
		}))

		_, err := fetchChatDiffContents(ctx, client, chatID)
		require.Error(t, err)
		sdkErr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())
	})

	t.Run("SurfacesNonIgnorableWatcherErrors", func(t *testing.T) {
		t.Parallel()

		// A 500 from the git stream is not in the ignorable set, so
		// fetchChatDiffContents must return it verbatim instead of
		// silently collapsing to the empty remote diff.
		ctx := t.Context()
		chatID := uuid.New()
		path := fmt.Sprintf("/api/experimental/chats/%s", chatID)
		client := newTestExperimentalClient(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case path + "/diff":
				rw.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.ChatDiffContents{ChatID: chatID}))
			case path + "/stream/git":
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusInternalServerError)
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.Response{Message: "internal git watcher failure"}))
			default:
				http.NotFound(rw, r)
			}
		}))

		_, err := fetchChatDiffContents(ctx, client, chatID)
		require.Error(t, err)
		sdkErr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())
	})
}

func TestBuildLocalChatDiffContents(t *testing.T) {
	t.Parallel()

	t.Run("SortsMultipleReposByRepoRoot", func(t *testing.T) {
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

		// Multi-repo aggregation drops the per-repo metadata because
		// Branch/RemoteOrigin only make sense for a single repo.
		require.Equal(t, chatID, diff.ChatID)
		require.Contains(t, diff.Diff, "diff --git a/a.txt b/a.txt")
		require.Contains(t, diff.Diff, "diff --git a/z.txt b/z.txt")
		require.Less(t, strings.Index(diff.Diff, "a.txt"), strings.Index(diff.Diff, "z.txt"))
		require.Nil(t, diff.Branch)
		require.Nil(t, diff.RemoteOrigin)
	})

	t.Run("ReturnsEmptyForNoRepositories", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		// No repos: exercise the early-return in buildLocalChatDiffContents
		// so the empty case is mechanically covered.
		for _, repos := range [][]codersdk.WorkspaceAgentRepoChanges{nil, {}} {
			diff := buildLocalChatDiffContents(chatID, repos)
			require.Equal(t, chatID, diff.ChatID)
			require.Empty(t, diff.Diff)
			require.Nil(t, diff.Branch)
			require.Nil(t, diff.RemoteOrigin)
		}
	})

	t.Run("SkipsRemovedAndEmptyRepositories", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		// Removed repos (Removed=true) and repos with whitespace-only
		// UnifiedDiff must not contribute to the aggregated diff. With
		// a single contributing repo, the per-repo Branch and
		// RemoteOrigin should still propagate to the result.
		diff := buildLocalChatDiffContents(chatID, []codersdk.WorkspaceAgentRepoChanges{
			{
				RepoRoot:    "/workspace/removed",
				Removed:     true,
				UnifiedDiff: "diff --git a/removed.txt b/removed.txt\n+removed\n",
			},
			{
				RepoRoot:    "/workspace/empty",
				UnifiedDiff: "   \n",
			},
			{
				RepoRoot:     "/workspace/only",
				Branch:       "feature/only",
				RemoteOrigin: "https://github.com/coder/coder.git",
				UnifiedDiff:  "diff --git a/only.txt b/only.txt\n+only\n",
			},
		})

		require.Equal(t, chatID, diff.ChatID)
		require.Contains(t, diff.Diff, "diff --git a/only.txt b/only.txt")
		require.NotContains(t, diff.Diff, "removed.txt")
		require.NotContains(t, diff.Diff, "empty")
		require.NotNil(t, diff.Branch)
		require.Equal(t, "feature/only", *diff.Branch)
		require.NotNil(t, diff.RemoteOrigin)
		require.Equal(t, "https://github.com/coder/coder.git", *diff.RemoteOrigin)
	})

	t.Run("ReturnsEmptyWhenAllRepositoriesAreSkipped", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		// If every repo is removed or empty, buildLocalChatDiffContents
		// returns the empty remote-diff shape so the caller falls back
		// to the placeholder overlay instead of rendering a diff-less
		// summary.
		diff := buildLocalChatDiffContents(chatID, []codersdk.WorkspaceAgentRepoChanges{
			{RepoRoot: "/workspace/removed", Removed: true, UnifiedDiff: "diff --git a/removed.txt b/removed.txt\n+removed\n"},
			{RepoRoot: "/workspace/empty"},
		})

		require.Equal(t, chatID, diff.ChatID)
		require.Empty(t, diff.Diff)
		require.Nil(t, diff.Branch)
		require.Nil(t, diff.RemoteOrigin)
	})
}
