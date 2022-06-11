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
	bumpDescriptionLong = `To extend the autostop deadline for a workspace.`
)

func bump() *cobra.Command {
	bumpCmd := &cobra.Command{
		Args:        cobra.RangeArgs(1, 2),
		Annotations: workspaceCommand,
		Use:         "bump <workspace-name> <duration>",
		Short:       "Extend the autostop deadline for a workspace.",
		Long:        bumpDescriptionLong,
		Example:     "coder bump my-workspace 90m",
		RunE: func(cmd *cobra.Command, args []string) error {
			bumpDuration, err := tryParseDuration(args[1])
			if err != nil {
				return err
			}

			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			newDeadline := time.Now().Add(bumpDuration)

			if newDeadline.Before(workspace.LatestBuild.Deadline) {
				_, _ = fmt.Fprintf(
					cmd.OutOrStdout(),
					"The proposed deadline is %s before the current deadline.\n",
					workspace.LatestBuild.Deadline.Sub(newDeadline).Round(time.Minute),
				)
				return nil
			}

			if err := client.PutExtendWorkspace(cmd.Context(), workspace.ID, codersdk.PutExtendWorkspaceRequest{
				Deadline: newDeadline,
			}); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(
				cmd.OutOrStdout(),
				"Workspace %q will now stop at %s\n", workspace.Name,
				newDeadline.Format(time.RFC822),
			)

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
