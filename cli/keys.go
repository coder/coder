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
				"Here is your API key. 🪄",
			))
			cmd.Println()
			cmd.Println(cliui.Styles.Code.Render(strings.TrimSpace(res.Key)))
			cmd.Println()
			cmd.Println(cliui.Styles.Wrap.Render(
				fmt.Sprintf("You can use this API key by setting --%s CLI flag, the %s environment variable, or the %q HTTP header.", varToken, envSessionToken, codersdk.SessionTokenKey),
			))

			return nil
		},
	}

	return cmd
}

type keyRow struct {
	ID        string    `table:"ID"`
	LastUsed  time.Time `table:"Last Used"`
	ExpiresAt time.Time `table:"Expires At"`
	CreatedAt time.Time `table:"Created At"`
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

			keys, err := client.GetMachineKeys(cmd.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("create machine key: %w", err)
			}

			if len(keys) == 0 {
				cmd.Println(cliui.Styles.Wrap.Render(
					"No machine keys found.",
				))
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
