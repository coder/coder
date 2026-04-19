package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/websocket"
)

const localChatDiffWatchTimeout = 5 * time.Second

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

	localDiff, err := fetchLocalChatDiffContents(ctx, client, chatID)
	if err != nil {
		if shouldIgnoreLocalDiffFallbackError(err) {
			return remoteDiff, nil
		}
		return codersdk.ChatDiffContents{}, err
	}
	if strings.TrimSpace(localDiff.Diff) == "" {
		return remoteDiff, nil
	}

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
	return localDiff, nil
}

func fetchLocalChatDiffContents(
	parentCtx context.Context,
	client *codersdk.ExperimentalClient,
	chatID uuid.UUID,
) (codersdk.ChatDiffContents, error) {
	ctx, cancel := context.WithTimeout(parentCtx, localChatDiffWatchTimeout)
	defer cancel()

	conn, err := dialChatGit(ctx, client, chatID)
	if err != nil {
		return codersdk.ChatDiffContents{}, err
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	conn.SetReadLimit(1 << 22)

	stream := wsjson.NewStream[
		codersdk.WorkspaceAgentGitServerMessage,
		codersdk.WorkspaceAgentGitClientMessage,
	](conn, websocket.MessageText, websocket.MessageText, client.Logger())
	if err := stream.Send(codersdk.WorkspaceAgentGitClientMessage{
		Type: codersdk.WorkspaceAgentGitClientMessageTypeRefresh,
	}); err != nil {
		return codersdk.ChatDiffContents{}, xerrors.Errorf("request git refresh: %w", err)
	}

	messages := stream.Chan()
	for {
		select {
		case <-ctx.Done():
			return codersdk.ChatDiffContents{}, xerrors.Errorf("watch chat git: %w", ctx.Err())
		case msg, ok := <-messages:
			if !ok {
				if ctx.Err() != nil {
					return codersdk.ChatDiffContents{}, xerrors.Errorf("watch chat git: %w", ctx.Err())
				}
				return codersdk.ChatDiffContents{}, xerrors.New("git watch connection closed")
			}
			switch msg.Type {
			case codersdk.WorkspaceAgentGitServerMessageTypeError:
				message := strings.TrimSpace(msg.Message)
				if message == "" {
					message = "git watch returned an unknown error"
				}
				return codersdk.ChatDiffContents{}, xerrors.New(message)
			case codersdk.WorkspaceAgentGitServerMessageTypeChanges:
				return buildLocalChatDiffContents(chatID, msg.Repositories), nil
			}
		}
	}
}

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

func buildLocalChatDiffContents(
	chatID uuid.UUID,
	repositories []codersdk.WorkspaceAgentRepoChanges,
) codersdk.ChatDiffContents {
	result := codersdk.ChatDiffContents{ChatID: chatID}
	if len(repositories) == 0 {
		return result
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
		return result
	}

	result.Diff = strings.Join(diffSegments, "\n")
	if len(diffRepositories) == 1 {
		if branch := strings.TrimSpace(diffRepositories[0].Branch); branch != "" {
			result.Branch = &branch
		}
		if origin := strings.TrimSpace(diffRepositories[0].RemoteOrigin); origin != "" {
			result.RemoteOrigin = &origin
		}
	}
	return result
}

func shouldIgnoreLocalDiffFallbackError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	sdkErr, ok := codersdk.AsError(err)
	if !ok {
		return false
	}

	switch sdkErr.StatusCode() {
	case http.StatusNotFound:
		return true
	case http.StatusBadRequest:
		message := strings.ToLower(strings.TrimSpace(sdkErr.Message))
		return strings.Contains(message, "chat has no workspace to watch") ||
			strings.Contains(message, "chat workspace has no agents")
	default:
		return false
	}
}
