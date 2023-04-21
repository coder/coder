package cli

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) tokens() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:   "tokens",
		Short: "Manage personal access tokens",
		Long: "Tokens are used to authenticate automated clients to Coder.\n" + formatExamples(
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
		Aliases: []string{"token"},
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.createToken(),
			r.listTokens(),
			r.removeToken(),
		},
	}
	return cmd
}

func (r *RootCmd) createToken() *clibase.Cmd {
	var (
		tokenLifetime time.Duration
		name          string
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "create",
		Short: "Create a token",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			res, err := client.CreateToken(inv.Context(), codersdk.Me, codersdk.CreateTokenRequest{
				Lifetime:  tokenLifetime,
				TokenName: name,
			})
			if err != nil {
				return xerrors.Errorf("create tokens: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stdout, res.Key)

			return nil
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:        "lifetime",
			Env:         "CODER_TOKEN_LIFETIME",
			Description: "Specify a duration for the lifetime of the token.",
			Default:     (time.Hour * 24 * 30).String(),
			Value:       clibase.DurationOf(&tokenLifetime),
		},
		{
			Flag:          "name",
			FlagShorthand: "n",
			Env:           "CODER_TOKEN_NAME",
			Description:   "Specify a human-readable name.",
			Value:         clibase.StringOf(&name),
		},
	}

	return cmd
}

// tokenListRow is the type provided to the OutputFormatter.
type tokenListRow struct {
	// For JSON format:
	codersdk.APIKey `table:"-"`

	// For table format:
	ID        string    `json:"-" table:"id,default_sort"`
	TokenName string    `json:"token_name" table:"name"`
	LastUsed  time.Time `json:"-" table:"last used"`
	ExpiresAt time.Time `json:"-" table:"expires at"`
	CreatedAt time.Time `json:"-" table:"created at"`
	Owner     string    `json:"-" table:"owner"`
}

func tokenListRowFromToken(token codersdk.APIKeyWithOwner) tokenListRow {
	return tokenListRow{
		APIKey:    token.APIKey,
		ID:        token.ID,
		TokenName: token.TokenName,
		LastUsed:  token.LastUsed,
		ExpiresAt: token.ExpiresAt,
		CreatedAt: token.CreatedAt,
		Owner:     token.Username,
	}
}

func (r *RootCmd) listTokens() *clibase.Cmd {
	// we only display the 'owner' column if the --all argument is passed in
	defaultCols := []string{"id", "name", "last used", "expires at", "created at"}
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

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List tokens",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			tokens, err := client.Tokens(inv.Context(), codersdk.Me, codersdk.TokensFilter{
				IncludeAll: all,
			})
			if err != nil {
				return xerrors.Errorf("list tokens: %w", err)
			}

			if len(tokens) == 0 {
				cliui.Infof(
					inv.Stdout,
					"No tokens found.\n",
				)
			}

			displayTokens = make([]tokenListRow, len(tokens))

			for i, token := range tokens {
				displayTokens[i] = tokenListRowFromToken(token)
			}

			out, err := formatter.Format(inv.Context(), displayTokens)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:          "all",
			FlagShorthand: "a",
			Description:   "Specifies whether all users' tokens will be listed or not (must have Owner role to see all tokens).",
			Value:         clibase.BoolOf(&all),
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) removeToken() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:     "remove <name>",
		Aliases: []string{"delete"},
		Short:   "Delete a token",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			token, err := client.APIKeyByName(inv.Context(), codersdk.Me, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("fetch api key by name %s: %w", inv.Args[0], err)
			}

			err = client.DeleteAPIKey(inv.Context(), codersdk.Me, token.ID)
			if err != nil {
				return xerrors.Errorf("delete api key: %w", err)
			}

			cliui.Infof(
				inv.Stdout,
				"Token has been deleted.",
			)

			return nil
		},
	}

	return cmd
}
