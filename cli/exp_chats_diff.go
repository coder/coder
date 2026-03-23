package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

type chatDiffOutput struct {
	GitChanges []codersdk.ChatGitChange  `json:"git_changes"`
	Diff       codersdk.ChatDiffContents `json:"diff"`
}

func (r *RootCmd) chatsDiff() *serpent.Command {
	var (
		stat   bool
		raw    bool
		output string
	)

	return &serpent.Command{
		Use:   "diff <chat-id>",
		Short: "Show the diff for a chat.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Name:        "stat",
				Flag:        "stat",
				Description: "Show a compact file summary.",
				Value:       serpent.BoolOf(&stat),
			},
			{
				Name:        "raw",
				Flag:        "raw",
				Description: "Show the raw unified diff.",
				Value:       serpent.BoolOf(&raw),
			},
			{
				Name:          "output",
				Flag:          "output",
				FlagShorthand: "o",
				Default:       "text",
				Description:   "Output format.",
				Value:         serpent.EnumOf(&output, "text", "json"),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			chatID, err := parseChatID(inv)
			if err != nil {
				return err
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx := inv.Context()

			if output == "json" {
				changes, err := client.GetChatGitChanges(ctx, chatID)
				if err != nil {
					return xerrors.Errorf("get chat git changes %s: %w", chatID, err)
				}

				diff, err := client.GetChatDiffContents(ctx, chatID)
				if err != nil {
					return xerrors.Errorf("get chat diff %s: %w", chatID, err)
				}

				encoded, err := json.MarshalIndent(chatDiffOutput{
					GitChanges: changes,
					Diff:       diff,
				}, "", "  ")
				if err != nil {
					return xerrors.Errorf("marshal chat diff output: %w", err)
				}

				_, err = fmt.Fprintln(inv.Stdout, string(encoded))
				return err
			}

			if raw {
				diff, err := client.GetChatDiffContents(ctx, chatID)
				if err != nil {
					return xerrors.Errorf("get chat diff %s: %w", chatID, err)
				}

				_, err = fmt.Fprint(inv.Stdout, diff.Diff)
				return err
			}

			changes, err := client.GetChatGitChanges(ctx, chatID)
			if err != nil {
				return xerrors.Errorf("get chat git changes %s: %w", chatID, err)
			}

			var out string
			if stat {
				out = renderChatDiffStat(changes)
			} else {
				diff, err := client.GetChatDiffContents(ctx, chatID)
				if err != nil {
					return xerrors.Errorf("get chat diff %s: %w", chatID, err)
				}
				out = renderChatDiffSummary(diff, changes)
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
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

func renderChatDiffStat(changes []codersdk.ChatGitChange) string {
	if len(changes) == 0 {
		return "No changes detected."
	}

	lines := make([]string, len(changes))
	for i, change := range changes {
		lines[i] = formatChatGitChange(change)
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
