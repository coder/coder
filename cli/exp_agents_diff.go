package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

const localChatDiffWatchTimeout = 5 * time.Second

// localChatDiffReadLimit bounds the size of the Changes message the
// client is willing to receive from the chat git watcher. agentgit
// caps each repository's UnifiedDiff at ~3 MiB (maxTotalDiffSize),
// and a Changes payload can aggregate many repos plus metadata, so
// 4 MiB is too tight for realistic multi-repo worktrees. 32 MiB
// covers ~10 maxed-out repos; pathological payloads beyond that still
// fall back to the remote empty diff via errLocalDiffWatchClosed /
// shouldIgnoreLocalDiffFallbackError.
const localChatDiffReadLimit = 32 << 20 // 32 MiB

// errLocalDiffWatchClosed is returned when the chat git watcher
// websocket closes during the Changes read loop with one of the
// known-safe close statuses:
//
//   - StatusMessageTooBig: the Changes payload exceeded our local
//     32 MiB client read limit (localChatDiffReadLimit).
//   - StatusGoingAway: the coderd watchChatGit proxy tore the
//     client stream down. This is the status the proxy always uses
//     in coderd/exp_chats.go, so it also covers the upstream 4 MiB
//     read limit on agent->coderd messages (see
//     workspacesdk/agentconn.go): when that limit is exceeded the
//     agent closes with StatusMessageTooBig, but the proxy does not
//     propagate that status and the client only ever observes
//     StatusGoingAway.
//
// Both cases degrade to the remote empty diff returned by /diff:
// the local watcher is a supplementary enrichment source that
// cannot improve on the remote when its stream is cut short. Other
// close statuses (StatusInternalError, StatusProtocolError, ...)
// and non-close read errors still surface as hard errors so real
// protocol regressions are not hidden behind the fallback.
var errLocalDiffWatchClosed = xerrors.New("chat git watcher connection closed before delivering a Changes message")

func fetchChatDiffContents(
	ctx context.Context,
	client *codersdk.ExperimentalClient,
	chatID uuid.UUID,
) (codersdk.ChatDiffContents, error) {
	remoteDiff, err := client.GetChatDiffContents(ctx, chatID)
	if err != nil {
		return codersdk.ChatDiffContents{}, err
	}
	if strings.TrimSpace(remoteDiff.Diff) != "" {
		return remoteDiff, nil
	}

	localDiff, localSingleRepo, err := fetchLocalChatDiffContents(ctx, client, chatID)
	if err != nil {
		if shouldIgnoreLocalDiffFallbackError(err) {
			return remoteDiff, nil
		}
		return codersdk.ChatDiffContents{}, err
	}
	if strings.TrimSpace(localDiff.Diff) == "" {
		return remoteDiff, nil
	}

	// Backfill metadata from the remote diff only when the local
	// watcher produced a single contributing repository. Gate this on
	// the explicit single-repo signal from buildLocalChatDiffContents
	// rather than on Branch/RemoteOrigin being non-nil, because a
	// single contributing repo can legitimately have an empty branch
	// (detached HEAD) or no origin remote and we still want remote
	// fields like Provider/PullRequestURL to flow through. Multi-repo
	// aggregates cannot be described by a single remote's metadata, so
	// we leave them alone.
	if localSingleRepo {
		if localDiff.Provider == nil {
			localDiff.Provider = remoteDiff.Provider
		}
		if localDiff.RemoteOrigin == nil {
			localDiff.RemoteOrigin = remoteDiff.RemoteOrigin
		}
		if localDiff.Branch == nil {
			localDiff.Branch = remoteDiff.Branch
		}
		if localDiff.PullRequestURL == nil {
			localDiff.PullRequestURL = remoteDiff.PullRequestURL
		}
	}
	return localDiff, nil
}

// fetchLocalChatDiffContents returns the aggregated local-watcher diff
// and a singleRepo flag that indicates whether that aggregate came from
// exactly one contributing repository. The caller uses singleRepo to
// decide whether it is safe to backfill remote-only metadata onto the
// local diff. All error paths return singleRepo=false.
//
// This intentionally bypasses wsjson.NewStream and reads the websocket
// directly so we can inspect the close status: an oversized Changes
// payload must degrade to the remote empty diff via
// errLocalDiffWatchClosed + shouldIgnoreLocalDiffFallbackError,
// but wsjson.Decoder swallows the read error (logs at debug) and
// closes the channel, which would collapse that specific case into
// the same generic "connection closed" bucket as server crashes or
// decode failures. Reading directly lets us narrowly fall back only
// for read-limit violations while still surfacing real protocol
// regressions.
func fetchLocalChatDiffContents(
	parentCtx context.Context,
	client *codersdk.ExperimentalClient,
	chatID uuid.UUID,
) (codersdk.ChatDiffContents, bool, error) {
	ctx, cancel := context.WithTimeout(parentCtx, localChatDiffWatchTimeout)
	defer cancel()

	conn, err := dialChatGit(ctx, client, chatID)
	if err != nil {
		return codersdk.ChatDiffContents{}, false, err
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	conn.SetReadLimit(localChatDiffReadLimit)

	refreshPayload, err := json.Marshal(codersdk.WorkspaceAgentGitClientMessage{
		Type: codersdk.WorkspaceAgentGitClientMessageTypeRefresh,
	})
	if err != nil {
		return codersdk.ChatDiffContents{}, false, xerrors.Errorf("marshal git refresh: %w", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, refreshPayload); err != nil {
		return codersdk.ChatDiffContents{}, false, xerrors.Errorf("request git refresh: %w", err)
	}

	for {
		msgType, payload, err := conn.Read(ctx)
		if err != nil {
			// Context expiration gets its own wrapping so it threads
			// cleanly through shouldIgnoreLocalDiffFallbackError's
			// context.DeadlineExceeded case.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return codersdk.ChatDiffContents{}, false, xerrors.Errorf("watch chat git: %w", ctxErr)
			}
			// A Changes payload that exceeds localChatDiffReadLimit
			// causes coder/websocket to close the connection with
			// StatusMessageTooBig. The coderd watchChatGit proxy
			// also always closes the client with StatusGoingAway
			// (see coderd/exp_chats.go), which is how we observe
			// the upstream 4 MiB agent->coderd read-limit breach:
			// the agent closes its own hop with StatusMessageTooBig,
			// but the proxy does not propagate that status, so the
			// client only ever sees StatusGoingAway. Map both onto
			// the narrow sentinel so shouldIgnoreLocalDiffFallbackError
			// can degrade to the remote empty diff instead of
			// surfacing a hard error. Every other close status
			// (StatusInternalError, StatusProtocolError, ...) and
			// every non-close read error still propagates so real
			// protocol regressions reach the user.
			switch websocket.CloseStatus(err) {
			case websocket.StatusMessageTooBig, websocket.StatusGoingAway:
				return codersdk.ChatDiffContents{}, false, errLocalDiffWatchClosed
			}
			return codersdk.ChatDiffContents{}, false, xerrors.Errorf("read git watch: %w", err)
		}
		// Ignore unexpected frame types instead of erroring; the
		// watcher only emits text frames today and a future binary
		// heartbeat should not break the overlay.
		if msgType != websocket.MessageText {
			continue
		}
		var msg codersdk.WorkspaceAgentGitServerMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			return codersdk.ChatDiffContents{}, false, xerrors.Errorf("decode git watch message: %w", err)
		}
		switch msg.Type {
		case codersdk.WorkspaceAgentGitServerMessageTypeError:
			message := strings.TrimSpace(msg.Message)
			if message == "" {
				message = "git watch returned an unknown error"
			}
			return codersdk.ChatDiffContents{}, false, xerrors.New(message)
		case codersdk.WorkspaceAgentGitServerMessageTypeChanges:
			diff, singleRepo := buildLocalChatDiffContents(chatID, msg.Repositories)
			return diff, singleRepo, nil
		}
	}
}

// dialChatGit opens the chat git-watcher WebSocket. We dial the socket
// manually instead of using codersdk.Client.Dial because that helper
// closes the HTTP response body before surfacing the error, which
// prevents codersdk.ReadBodyAsError from extracting the status code and
// message that shouldIgnoreLocalDiffFallbackError needs to decide
// whether to degrade to the empty remote diff. Keep this handrolled
// path as long as the shared helper has that limitation.
func dialChatGit(
	ctx context.Context,
	client *codersdk.ExperimentalClient,
	chatID uuid.UUID,
) (*websocket.Conn, error) {
	requestURL, err := client.URL.Parse(
		fmt.Sprintf("/api/experimental/chats/%s/stream/git", chatID),
	)
	if err != nil {
		return nil, err
	}

	dialOptions := &websocket.DialOptions{
		HTTPClient:      client.HTTPClient,
		CompressionMode: websocket.CompressionDisabled,
	}
	client.SessionTokenProvider.SetDialOption(dialOptions)

	conn, resp, err := websocket.Dial(ctx, requestURL.String(), dialOptions)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		if resp != nil {
			return nil, codersdk.ReadBodyAsError(resp)
		}
		return nil, err
	}
	return conn, nil
}

// buildLocalChatDiffContents aggregates the local watcher's
// per-repository changes into a single ChatDiffContents. The returned
// singleRepo flag is true iff the aggregated diff came from exactly
// one contributing repository (one repo with a non-empty UnifiedDiff
// that has not been removed). Callers use this flag to decide whether
// it is safe to backfill remote-only metadata onto the local diff:
// multi-repo aggregates cannot be described by a single remote's
// branch/origin/PR URL, but a single-repo aggregate can even when the
// contributing repo has an empty branch (detached HEAD) or no origin
// remote configured.
func buildLocalChatDiffContents(
	chatID uuid.UUID,
	repositories []codersdk.WorkspaceAgentRepoChanges,
) (codersdk.ChatDiffContents, bool) {
	result := codersdk.ChatDiffContents{ChatID: chatID}
	if len(repositories) == 0 {
		return result, false
	}

	repositories = slices.Clone(repositories)
	slices.SortFunc(repositories, func(a, b codersdk.WorkspaceAgentRepoChanges) int {
		return strings.Compare(a.RepoRoot, b.RepoRoot)
	})

	diffSegments := make([]string, 0, len(repositories))
	diffRepositories := make([]codersdk.WorkspaceAgentRepoChanges, 0, len(repositories))
	for _, repo := range repositories {
		if repo.Removed || strings.TrimSpace(repo.UnifiedDiff) == "" {
			continue
		}
		diffRepositories = append(diffRepositories, repo)
		diffSegments = append(diffSegments, strings.TrimRight(repo.UnifiedDiff, "\n"))
	}
	if len(diffSegments) == 0 {
		return result, false
	}

	result.Diff = strings.Join(diffSegments, "\n")
	singleRepo := len(diffRepositories) == 1
	if singleRepo {
		if branch := strings.TrimSpace(diffRepositories[0].Branch); branch != "" {
			result.Branch = &branch
		}
		if origin := strings.TrimSpace(diffRepositories[0].RemoteOrigin); origin != "" {
			result.RemoteOrigin = &origin
		}
	}
	return result, singleRepo
}

func shouldIgnoreLocalDiffFallbackError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// A watcher stream closed with StatusMessageTooBig or
	// StatusGoingAway is a best-effort degradation point: the
	// remote /diff endpoint already returns the empty placeholder
	// in this case, so fall back to it instead of surfacing a hard
	// error. See errLocalDiffWatchClosed for the rationale on why
	// those two close statuses are safe while others still surface.
	if errors.Is(err, errLocalDiffWatchClosed) {
		return true
	}

	sdkErr, ok := codersdk.AsError(err)
	if !ok {
		return false
	}

	switch sdkErr.StatusCode() {
	case http.StatusNotFound:
		return true
	case http.StatusForbidden:
		// authorizeChatWorkspaceExec returns 403 when the chat owner's
		// workspace permissions have been revoked. The remote diff
		// endpoint (getChatDiffContents) does not re-check workspace
		// permissions, so degrade to its empty response the same way
		// we do for the 400 variants below.
		return true
	case http.StatusBadRequest:
		// These correspond to the 400 responses from watchChatGit in
		// coderd/exp_chats.go when the chat cannot be observed through
		// a workspace agent (no workspace bound, workspace deleted, no
		// agents, or an agent that is not yet connected). Each should
		// fall back to the empty remote diff the same way a missing
		// chat (404) does instead of surfacing a hard error.
		// codersdk.IsChatGitWatchFallbackMessage keeps this list
		// mechanically linked to the server-side messages.
		return codersdk.IsChatGitWatchFallbackMessage(sdkErr.Message)
	default:
		return false
	}
}
