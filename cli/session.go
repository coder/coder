package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
)

func session() *cobra.Command {
	return &cobra.Command{
		Use:   "session",
		Short: "Print out information about your current session",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Session token is %s\n", cliui.Styles.Code.Render(strings.TrimSpace(client.SessionToken)))
			return nil
		},
	}
}
