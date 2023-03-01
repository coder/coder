package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
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
	var tokenLifetime time.Duration
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

// tokenListRow is the type provided to the OutputFormatter.
type tokenListRow struct {
	// For JSON format:
	codersdk.APIKey `table:"-"`

	// For table format:
	ID        string    `json:"-" table:"id,default_sort"`
	LastUsed  time.Time `json:"-" table:"last used"`
	ExpiresAt time.Time `json:"-" table:"expires at"`
	CreatedAt time.Time `json:"-" table:"created at"`
	Owner     string    `json:"-" table:"owner"`
}

func tokenListRowFromToken(token codersdk.APIKeyWithOwner) tokenListRow {
	return tokenListRow{
		APIKey:    token.APIKey,
		ID:        token.ID,
		LastUsed:  token.LastUsed,
		ExpiresAt: token.ExpiresAt,
		CreatedAt: token.CreatedAt,
		Owner:     token.Username,
	}
}

func listTokens() *cobra.Command {
	// we only display the 'owner' column if the --all argument is passed in
	defaultCols := []string{"id", "last used", "expires at", "created at"}
	if slices.Contains(os.Args, "-a") || slices.Contains(os.Args, "--all") {
		defaultCols = append(defaultCols, "owner")
	}

	var (
		all           bool
		displayTokens []tokenListRow
		formatter     = cliui.NewOutputFormatter(
			cliui.TableFormat([]tokenListRow{}, defaultCols),
			cliui.JSONFormat(),
		)
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

			tokens, err := client.Tokens(cmd.Context(), codersdk.Me, codersdk.TokensFilter{
				IncludeAll: all,
			})
			if err != nil {
				return xerrors.Errorf("list tokens: %w", err)
			}

			if len(tokens) == 0 {
				cmd.Println(cliui.Styles.Wrap.Render(
					"No tokens found.",
				))
			}

			displayTokens = make([]tokenListRow, len(tokens))

			for i, token := range tokens {
				displayTokens[i] = tokenListRowFromToken(token)
			}

			out, err := formatter.Format(cmd.Context(), displayTokens)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}

	cmd.Flags().BoolVarP(&all, "all", "a", false,
		"Specifies whether all users' tokens will be listed or not (must have Owner role to see all tokens).")

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
