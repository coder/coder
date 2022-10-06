package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func keys() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "keys",
		Short:   "Manage machine keys",
		Long:    "Machine keys are used to authenticate automated clients to Coder.",
		Aliases: []string{"key"},
		Example: formatExamples(
			example{
				Description: "Create a machine key for CI/CD scripts",
				Command:     "coder keys create",
			},
			example{
				Description: "List your machine keys",
				Command:     "coder keys ls",
			},
			example{
				Description: "Remove a key by ID",
				Command:     "coder keys rm WuoWs4ZsMX",
			},
		),
	}
	cmd.AddCommand(
		createKey(),
		listKeys(),
		removeKey(),
	)

	return cmd
}

func createKey() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a machine key",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			res, err := client.CreateMachineKey(cmd.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("create machine key: %w", err)
			}

			cmd.Println(cliui.Styles.Wrap.Render(
				"This is your API key for authenticating to Coder in automated services. 🪄",
			))
			cmd.Println()
			cmd.Println(cliui.Styles.Code.Render(strings.TrimSpace(res.Key)))
			cmd.Println()
			cmd.Println(cliui.Styles.Wrap.Render(
				"You can use this API key by setting --%s CLI flag, the %s environment variable, or the %s HTTP header.",
			))

			return nil
		},
	}

	return cmd
}

type keyRow struct {
	ID        string    `table:"id"`
	LastUsed  time.Time `json:"last_used"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func listKeys() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List machine keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			keys, err := client.ListMachineKeys(cmd.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("create machine key: %w", err)
			}

			var rows []keyRow
			for _, key := range keys {
				rows = append(rows, keyRow{
					ID:        key.ID,
					LastUsed:  key.LastUsed,
					ExpiresAt: key.ExpiresAt,
					CreatedAt: key.CreatedAt,
				})
			}

			out, err := cliui.DisplayTable(rows, "", nil)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}

	return cmd
}

func removeKey() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove [id]",
		Aliases: []string{"rm"},
		Short:   "Delete a machine key",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			err = client.DeleteAPIKey(cmd.Context(), codersdk.Me, args[0])
			if err != nil {
				return xerrors.Errorf("delete api key: %w", err)
			}

			cmd.Println(cliui.Styles.Wrap.Render(
				"API key has been deleted.",
			))

			return nil
		},
	}

	return cmd
}
