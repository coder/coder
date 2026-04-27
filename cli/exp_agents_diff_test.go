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
		// error in the TUI. Drive the subtests from the shared codersdk
		// constants so a server-side rewording automatically flows
		// through the test matrix.
		for _, message := range []string{
			codersdk.ChatGitWatchNoWorkspaceMessage,
			codersdk.ChatGitWatchWorkspaceNotFoundMessage,
			codersdk.ChatGitWatchWorkspaceNoAgentsMessage,
			codersdk.ChatGitWatchAgentStateMessage(codersdk.WorkspaceAgentConnecting),
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

	t.Run("IgnoresForbiddenWatcherFallbackErrors", func(t *testing.T) {
		t.Parallel()

		// authorizeChatWorkspaceExec in coderd/exp_chats.go returns 403
		// when the chat owner's workspace exec permission is revoked.
		// The remote /diff endpoint does not re-check workspace
		// permissions, so fetchChatDiffContents must swallow the 403
		// and fall back to the empty remote diff just like it does for
		// the 400 variants above. Without this subtest, removing the
		// `case http.StatusForbidden` branch in
		// shouldIgnoreLocalDiffFallbackError would silently regress.
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
				rw.WriteHeader(http.StatusForbidden)
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.Response{Message: "forbidden"}))
			default:
				http.NotFound(rw, r)
			}
		}))

		diff, err := fetchChatDiffContents(ctx, client, chatID)
		require.NoError(t, err)
		require.Equal(t, chatID, diff.ChatID)
		require.Empty(t, diff.Diff)
	})

	t.Run("IgnoresNotFoundWatcherFallbackErrors", func(t *testing.T) {
		t.Parallel()

		// watchChatGit in coderd/exp_chats.go returns 404 for missing
		// chats (httpapi.ResourceNotFound). The remote /diff endpoint
		// already handles the missing-chat case on its own, so
		// fetchChatDiffContents must swallow the 404 from /stream/git
		// and fall back to whatever the remote diff returned, the
		// same way it does for the 400 and 403 variants above.
		// Without this subtest, removing the `case http.StatusNotFound`
		// branch in shouldIgnoreLocalDiffFallbackError would silently
		// regress (mirrors the 403 coverage added for DEREM-16).
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
				rw.WriteHeader(http.StatusNotFound)
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.Response{Message: "not found"}))
			default:
				http.NotFound(rw, r)
			}
		}))

		diff, err := fetchChatDiffContents(ctx, client, chatID)
		require.NoError(t, err)
		require.Equal(t, chatID, diff.ChatID)
		require.Empty(t, diff.Diff)
	})

	t.Run("BackfillsRemoteMetadataWhenLocalDiffIsSingleRepo", func(t *testing.T) {
		t.Parallel()

		// The scenario this PR was written for: a chat has remote
		// metadata (provider, pull-request URL, etc.) but the server
		// returns an empty Diff because the remote watcher has not
		// observed changes yet. The CLI fetches the local watcher
		// diff and must carry the remote metadata forward so the
		// Diff overlay still shows the PR URL / origin.
		ctx := t.Context()
		chatID := uuid.New()
		path := fmt.Sprintf("/api/experimental/chats/%s", chatID)
		remoteBranch := "feature/remote-branch"
		remoteOrigin := "https://github.com/coder/coder.git"
		remotePR := "https://github.com/coder/coder/pull/42"
		remoteProvider := "github"
		client := newTestExperimentalClient(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case path + "/diff":
				rw.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.ChatDiffContents{
					ChatID:         chatID,
					Provider:       &remoteProvider,
					RemoteOrigin:   &remoteOrigin,
					Branch:         &remoteBranch,
					PullRequestURL: &remotePR,
				}))
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
				// Return exactly one repo so buildLocalChatDiffContents
				// sets Branch/RemoteOrigin, which is the signal that
				// fetchChatDiffContents uses to backfill missing
				// metadata from the remote response (Provider, PR URL)
				// without overwriting fields the local watcher
				// already populated.
				require.NoError(t, json.NewEncoder(writer).Encode(codersdk.WorkspaceAgentGitServerMessage{
					Type: codersdk.WorkspaceAgentGitServerMessageTypeChanges,
					Repositories: []codersdk.WorkspaceAgentRepoChanges{{
						RepoRoot:     "/workspace/repo",
						Branch:       "feature/local-branch",
						RemoteOrigin: "https://github.com/coder/local.git",
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

		// The aggregated diff comes from the local watcher.
		require.Contains(t, diff.Diff, "diff --git a/a.txt b/a.txt")
		require.Contains(t, diff.Diff, "+new")

		// Branch and RemoteOrigin were populated by the single-repo
		// local watcher result, so they must NOT be overwritten by
		// the remote response.
		require.NotNil(t, diff.Branch)
		require.Equal(t, "feature/local-branch", *diff.Branch)
		require.NotNil(t, diff.RemoteOrigin)
		require.Equal(t, "https://github.com/coder/local.git", *diff.RemoteOrigin)

		// Provider and PullRequestURL were nil on the local diff,
		// so they must be backfilled from the remote metadata.
		require.NotNil(t, diff.Provider)
		require.Equal(t, remoteProvider, *diff.Provider)
		require.NotNil(t, diff.PullRequestURL)
		require.Equal(t, remotePR, *diff.PullRequestURL)
	})

	t.Run("BackfillsRemoteMetadataWhenSingleRepoHasBlankBranchAndOrigin", func(t *testing.T) {
		t.Parallel()

		// A single contributing repo can legitimately be in detached
		// HEAD with no origin remote configured: buildLocalChatDiffContents
		// then leaves both Branch and RemoteOrigin nil even though
		// exactly one repository produced the aggregated diff. Before
		// the singleRepo flag was introduced, the gate on
		// `localDiff.Branch != nil || localDiff.RemoteOrigin != nil`
		// skipped the backfill in this case and the drawer silently
		// lost remote Provider/PullRequestURL. fetchChatDiffContents
		// must now use the explicit singleRepo signal so remote
		// metadata still flows through, and must also populate the
		// nil Branch/RemoteOrigin from the remote response to keep the
		// drawer display consistent with all other single-repo diffs.
		ctx := t.Context()
		chatID := uuid.New()
		path := fmt.Sprintf("/api/experimental/chats/%s", chatID)
		remoteBranch := "feature/remote-branch"
		remoteOrigin := "https://github.com/coder/coder.git"
		remotePR := "https://github.com/coder/coder/pull/42"
		remoteProvider := "github"
		client := newTestExperimentalClient(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case path + "/diff":
				rw.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(rw).Encode(codersdk.ChatDiffContents{
					ChatID:         chatID,
					Provider:       &remoteProvider,
					RemoteOrigin:   &remoteOrigin,
					Branch:         &remoteBranch,
					PullRequestURL: &remotePR,
				}))
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
				// Exactly one repository contributes, but both
				// Branch and RemoteOrigin are empty (detached HEAD,
				// no origin remote). buildLocalChatDiffContents
				// still flags this as singleRepo=true, so the
				// backfill must run and populate every nil field
				// from the remote response.
				require.NoError(t, json.NewEncoder(writer).Encode(codersdk.WorkspaceAgentGitServerMessage{
					Type: codersdk.WorkspaceAgentGitServerMessageTypeChanges,
					Repositories: []codersdk.WorkspaceAgentRepoChanges{{
						RepoRoot:     "/workspace/repo",
						Branch:       "",
						RemoteOrigin: "",
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

		// The aggregated diff still comes from the local watcher.
		require.Contains(t, diff.Diff, "diff --git a/a.txt b/a.txt")
		require.Contains(t, diff.Diff, "+new")

		// Every remote-only field is backfilled because
		// buildLocalChatDiffContents flagged the aggregate as
		// singleRepo=true even with blank branch/origin.
		require.NotNil(t, diff.Branch)
		require.Equal(t, remoteBranch, *diff.Branch)
		require.NotNil(t, diff.RemoteOrigin)
		require.Equal(t, remoteOrigin, *diff.RemoteOrigin)
		require.NotNil(t, diff.Provider)
		require.Equal(t, remoteProvider, *diff.Provider)
		require.NotNil(t, diff.PullRequestURL)
		require.Equal(t, remotePR, *diff.PullRequestURL)
	})

	t.Run("IgnoresWatcherMessageTooBigCloses", func(t *testing.T) {
		t.Parallel()

		// agentgit caps each repository's UnifiedDiff at ~3 MiB and a
		// Changes payload aggregates every repo plus metadata, so a
		// realistic multi-repo workspace can legitimately produce a
		// payload that exceeds the client's websocket read limit.
		// When that happens coder/websocket closes the connection
		// with StatusMessageTooBig. fetchChatDiffContents must map
		// that specific close status onto errLocalDiffWatchClosed
		// and fall back to the remote empty diff rather than
		// surfacing a hard error to the TUI. Without this subtest,
		// removing the StatusMessageTooBig branch in
		// fetchLocalChatDiffContents or the errLocalDiffWatchClosed
		// branch in shouldIgnoreLocalDiffFallbackError would
		// silently regress the large-multi-repo case this feature is
		// meant to improve.
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
				// Drain the refresh before closing so the client
				// surfaces the close status from its next Read, not
				// an unrelated write error.
				_, _, err = conn.Read(ctx)
				require.NoError(t, err)
				require.NoError(t, conn.Close(websocket.StatusMessageTooBig, "too big"))
			default:
				http.NotFound(rw, r)
			}
		}))

		diff, err := fetchChatDiffContents(ctx, client, chatID)
		require.NoError(t, err)
		require.Equal(t, chatID, diff.ChatID)
		require.Empty(t, diff.Diff)
	})

	t.Run("IgnoresWatcherGoingAwayCloses", func(t *testing.T) {
		t.Parallel()

		// The coderd watchChatGit proxy always closes the client
		// stream with StatusGoingAway regardless of why the
		// upstream agent->coderd hop failed. In particular, when
		// that hop's 4 MiB read limit (workspacesdk/agentconn.go)
		// is exceeded, the agent closes its end with
		// StatusMessageTooBig but the proxy does not propagate
		// that status, so the client only observes
		// StatusGoingAway. That is the exact scenario this PR's
		// 32 MiB client read limit is meant to handle, so the
		// TUI must degrade to the remote empty diff for
		// StatusGoingAway just like it does for
		// StatusMessageTooBig. Without this subtest, narrowing
		// the close-status match back to StatusMessageTooBig
		// only would silently regress multi-repo worktrees whose
		// aggregate Changes payload sits between the 4 MiB
		// upstream limit and the 32 MiB client limit.
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
				_, _, err = conn.Read(ctx)
				require.NoError(t, err)
				require.NoError(t, conn.Close(websocket.StatusGoingAway, "proxy tear-down"))
			default:
				http.NotFound(rw, r)
			}
		}))

		diff, err := fetchChatDiffContents(ctx, client, chatID)
		require.NoError(t, err)
		require.Equal(t, chatID, diff.ChatID)
		require.Empty(t, diff.Diff)
	})

	t.Run("SurfacesUnexpectedWatcherCloseErrors", func(t *testing.T) {
		t.Parallel()

		// The StatusMessageTooBig fallback is intentionally narrow:
		// a generic websocket close (for example the server
		// crashing and closing with StatusInternalError) should
		// surface as an error rather than silently degrading,
		// because that would hide real protocol regressions behind
		// the best-effort fallback. This subtest pins that
		// distinction so a future attempt to blanket-ignore every
		// close reason immediately breaks the test.
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
				_, _, err = conn.Read(ctx)
				require.NoError(t, err)
				require.NoError(t, conn.Close(websocket.StatusInternalError, "boom"))
			default:
				http.NotFound(rw, r)
			}
		}))

		_, err := fetchChatDiffContents(ctx, client, chatID)
		require.Error(t, err)
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
		diff, singleRepo := buildLocalChatDiffContents(chatID, []codersdk.WorkspaceAgentRepoChanges{
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
		// Branch/RemoteOrigin only make sense for a single repo. The
		// singleRepo flag must be false so callers know not to
		// backfill remote metadata onto a multi-repo aggregate.
		require.Equal(t, chatID, diff.ChatID)
		require.Contains(t, diff.Diff, "diff --git a/a.txt b/a.txt")
		require.Contains(t, diff.Diff, "diff --git a/z.txt b/z.txt")
		require.Less(t, strings.Index(diff.Diff, "a.txt"), strings.Index(diff.Diff, "z.txt"))
		require.Nil(t, diff.Branch)
		require.Nil(t, diff.RemoteOrigin)
		require.False(t, singleRepo)
	})

	t.Run("ReturnsEmptyForNoRepositories", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		// No repos: exercise the early-return in buildLocalChatDiffContents
		// so the empty case is mechanically covered. singleRepo must
		// be false because no repository contributed any diff.
		for _, repos := range [][]codersdk.WorkspaceAgentRepoChanges{nil, {}} {
			diff, singleRepo := buildLocalChatDiffContents(chatID, repos)
			require.Equal(t, chatID, diff.ChatID)
			require.Empty(t, diff.Diff)
			require.Nil(t, diff.Branch)
			require.Nil(t, diff.RemoteOrigin)
			require.False(t, singleRepo)
		}
	})

	t.Run("SkipsRemovedAndEmptyRepositories", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		// Removed repos (Removed=true) and repos with whitespace-only
		// UnifiedDiff must not contribute to the aggregated diff. With
		// a single contributing repo, the per-repo Branch and
		// RemoteOrigin should still propagate to the result and
		// singleRepo must be true because only one repository
		// contributed.
		diff, singleRepo := buildLocalChatDiffContents(chatID, []codersdk.WorkspaceAgentRepoChanges{
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
		require.True(t, singleRepo)
	})

	t.Run("ReturnsEmptyWhenAllRepositoriesAreSkipped", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		// If every repo is removed or empty, buildLocalChatDiffContents
		// returns the empty remote-diff shape so the caller falls back
		// to the placeholder overlay instead of rendering a diff-less
		// summary. singleRepo must be false because no repository
		// contributed any diff content.
		diff, singleRepo := buildLocalChatDiffContents(chatID, []codersdk.WorkspaceAgentRepoChanges{
			{RepoRoot: "/workspace/removed", Removed: true, UnifiedDiff: "diff --git a/removed.txt b/removed.txt\n+removed\n"},
			{RepoRoot: "/workspace/empty"},
		})

		require.Equal(t, chatID, diff.ChatID)
		require.Empty(t, diff.Diff)
		require.Nil(t, diff.Branch)
		require.Nil(t, diff.RemoteOrigin)
		require.False(t, singleRepo)
	})
}
