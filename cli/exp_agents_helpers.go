package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

func fetchAllChatMessages(ctx context.Context, client *codersdk.ExperimentalClient, chatID uuid.UUID) ([]codersdk.ChatMessage, error) {
	var (
		allMessages []codersdk.ChatMessage
		opts        *codersdk.ChatMessagesPaginationOptions
	)

	for {
		resp, err := client.GetChatMessages(ctx, chatID, opts)
		if err != nil {
			return nil, err
		}

		allMessages = append(allMessages, resp.Messages...)
		if !resp.HasMore || len(resp.Messages) == 0 {
			break
		}

		opts = &codersdk.ChatMessagesPaginationOptions{
			BeforeID: resp.Messages[len(resp.Messages)-1].ID,
		}
	}

	slices.SortStableFunc(allMessages, func(a, b codersdk.ChatMessage) int {
		switch {
		case a.CreatedAt.Before(b.CreatedAt):
			return -1
		case a.CreatedAt.After(b.CreatedAt):
			return 1
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})

	return allMessages, nil
}

func compactTranscriptJSON(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}

	var builder bytes.Buffer
	if err := json.Compact(&builder, trimmed); err == nil {
		return builder.String()
	}

	return string(trimmed)
}

func renderChatDiffSummary(diff codersdk.ChatDiffContents, changes []codersdk.ChatGitChange) string {
	var lines []string
	if diff.Branch != nil && *diff.Branch != "" {
		lines = append(lines, fmt.Sprintf("Branch: %s", *diff.Branch))
	}
	if diff.PullRequestURL != nil && *diff.PullRequestURL != "" {
		lines = append(lines, fmt.Sprintf("PR: %s", *diff.PullRequestURL))
	}

	if len(changes) == 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "No changes detected.")
		return strings.Join(lines, "\n")
	}

	if len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, "Files changed:")
	for _, change := range changes {
		lines = append(lines, "  "+formatChatGitChange(change))
	}

	return strings.Join(lines, "\n")
}

func formatChatGitChange(change codersdk.ChatGitChange) string {
	path := change.FilePath
	if change.ChangeType == "renamed" && change.OldPath != nil && *change.OldPath != "" {
		path = fmt.Sprintf("%s → %s", *change.OldPath, change.FilePath)
	}

	return fmt.Sprintf("%-8s %s", change.ChangeType, path)
}
