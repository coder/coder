package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

const ttlDescriptionLong = `To have your workspace stop automatically after a configurable interval has passed.
Minimum TTL is 1 minute.
`

func ttl() *cobra.Command {
	ttlCmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "ttl [command]",
		Short:       "Schedule a workspace to automatically stop after a configurable interval",
		Long:        ttlDescriptionLong,
		Example:     "coder ttl set my-workspace 8h30m",
	}

	ttlCmd.AddCommand(ttlShow())
	ttlCmd.AddCommand(ttlset())
	ttlCmd.AddCommand(ttlunset())

	return ttlCmd
}

func ttlShow() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "show <workspace_name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			if workspace.TTLMillis == nil || *workspace.TTLMillis == 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "not set\n")
				return nil
			}

			dur := time.Duration(*workspace.TTLMillis) * time.Millisecond
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", dur)

			return nil
		},
	}
	return cmd
}

func ttlset() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "set <workspace_name> <ttl>",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			ttl, err := time.ParseDuration(args[1])
			if err != nil {
				return xerrors.Errorf("parse ttl: %w", err)
			}

			truncated := ttl.Truncate(time.Minute)

			if truncated == 0 {
				return xerrors.Errorf("ttl must be at least 1m")
			}

			if truncated != ttl {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "warning: ttl rounded down to %s\n", truncated)
			}

			if changed, newDeadline := changedNewDeadline(workspace, truncated); changed {
				// For the purposes of the user, "less than a minute" is essentially the same as "immediately".
				timeRemaining := time.Until(newDeadline).Truncate(time.Minute)
				humanRemaining := "in " + timeRemaining.String()
				if timeRemaining <= 0 {
					humanRemaining = "immediately"
				}
				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text: fmt.Sprintf(
						"Workspace %q will be stopped %s. Are you sure?",
						workspace.Name,
						humanRemaining,
					),
					Default:   "yes",
					IsConfirm: true,
				})
				if errors.Is(err, cliui.Canceled) {
					return nil
				}
			}

			millis := truncated.Milliseconds()
			if err = client.UpdateWorkspaceTTL(cmd.Context(), workspace.ID, codersdk.UpdateWorkspaceTTLRequest{
				TTLMillis: &millis,
			}); err != nil {
				return xerrors.Errorf("update workspace ttl: %w", err)
			}

			return nil
		},
	}

	return cmd
}

func ttlunset() *cobra.Command {
	return &cobra.Command{
		Use:  "unset <workspace_name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			err = client.UpdateWorkspaceTTL(cmd.Context(), workspace.ID, codersdk.UpdateWorkspaceTTLRequest{
				TTLMillis: nil,
			})
			if err != nil {
				return xerrors.Errorf("update workspace ttl: %w", err)
			}

			_, _ = fmt.Fprint(cmd.OutOrStdout(), "ttl unset\n", workspace.Name)

			return nil
		},
	}
}

func changedNewDeadline(ws codersdk.Workspace, newTTL time.Duration) (changed bool, newDeadline time.Time) {
	if ws.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
		// not running
		return false, newDeadline
	}

	if ws.LatestBuild.Job.CompletedAt == nil {
		// still building
		return false, newDeadline
	}

	newDeadline = ws.LatestBuild.Job.CompletedAt.Add(newTTL)
	return true, newDeadline
}
