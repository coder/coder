package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

const (
	bumpDescriptionLong = `To extend the autostop deadline for a workspace.
	If no unit is specified in the duration, we assume minutes.
	`
	defaultBumpDuration = 90 * time.Minute
)

func bump() *cobra.Command {
	bumpCmd := &cobra.Command{
		Args:        cobra.RangeArgs(1, 2),
		Annotations: workspaceCommand,
		Use:         "bump workspace [duration]",
		Short:       "Extend the autostop deadline for a workspace.",
		Long:        bumpDescriptionLong,
		Example:     "coder bump my-workspace 90m",
		RunE: func(cmd *cobra.Command, args []string) error {
			bumpDuration := defaultBumpDuration
			if len(args) > 1 {
				d, err := tryParseDuration(args[1])
				if err != nil {
					return err
				}
				bumpDuration = d
			}
			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return xerrors.Errorf("get current org: %w", err)
			}

			workspace, err := client.WorkspaceByOwnerAndName(cmd.Context(), organization.ID, codersdk.Me, args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			if workspace.LatestBuild.Deadline.IsZero() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "no deadline set\n")
				return nil
			}

			newDeadline := workspace.LatestBuild.Deadline.Add(bumpDuration)
			if err := client.PutExtendWorkspace(cmd.Context(), workspace.ID, codersdk.PutExtendWorkspaceRequest{
				Deadline: newDeadline,
			}); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Workspace %q will now stop at %s\n", workspace.Name, newDeadline.Format(time.RFC3339))

			return nil
		},
	}

	return bumpCmd
}

func tryParseDuration(raw string) (time.Duration, error) {
	// If the user input a raw number, assume minutes
	if isDigit(raw) {
		raw = raw + "m"
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	return d, nil
}

func isDigit(s string) bool {
	return strings.IndexFunc(s, func(c rune) bool {
		return c < '0' || c > '9'
	}) == -1
}
