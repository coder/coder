package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func tokens() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tokens",
		Short:   "Manage personal access tokens",
		Long:    "Tokens are used to authenticate automated clients to Coder.",
		Aliases: []string{"token"},
		Example: formatExamples(
			example{
				Description: "Create a token for automation",
				Command:     "coder tokens create",
			},
			example{
				Description: "List your tokens",
				Command:     "coder tokens ls",
			},
			example{
				Description: "Remove a token by ID",
				Command:     "coder tokens rm WuoWs4ZsMX",
			},
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		createToken(),
		listTokens(),
		removeToken(),
	)

	return cmd
}

func createToken() *cobra.Command {
	var (
		tokenLifetime time.Duration
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			res, err := client.CreateToken(cmd.Context(), codersdk.Me, codersdk.CreateTokenRequest{
				Lifetime: tokenLifetime,
			})
			if err != nil {
				return xerrors.Errorf("create tokens: %w", err)
			}

			cmd.Println(cliui.Styles.Wrap.Render(
				"Here is your token. 🪄",
			))
			cmd.Println()
			cmd.Println(cliui.Styles.Code.Render(strings.TrimSpace(res.Key)))
			cmd.Println()
			cmd.Println(cliui.Styles.Wrap.Render(
				fmt.Sprintf("You can use this token by setting the --%s CLI flag, the %s environment variable, or the %q HTTP header.", varToken, envSessionToken, codersdk.SessionTokenHeader),
			))

			return nil
		},
	}

	cliflag.DurationVarP(cmd.Flags(), &tokenLifetime, "lifetime", "", "CODER_TOKEN_LIFETIME", 30*24*time.Hour, "Specify a duration for the lifetime of the token.")

	return cmd
}

func listTokens() *cobra.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]codersdk.APIKey{}, nil),
		cliui.JSONFormat(),
	)

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			keys, err := client.Tokens(cmd.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("create tokens: %w", err)
			}

			if len(keys) == 0 {
				cmd.Println(cliui.Styles.Wrap.Render(
					"No tokens found.",
				))
			}

			out, err := formatter.Format(cmd.Context(), keys)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}

	formatter.AttachFlags(cmd)
	return cmd
}

func removeToken() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove [id]",
		Aliases: []string{"rm"},
		Short:   "Delete a token",
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
				"Token has been deleted.",
			))

			return nil
		},
	}

	return cmd
}
