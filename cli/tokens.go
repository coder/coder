package cli

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) tokens() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "tokens",
		Short: "Manage personal access tokens",
		Long: "Tokens are used to authenticate automated clients to Coder.\n" + FormatExamples(
			Example{
				Description: "Create a token for automation",
				Command:     "coder tokens create",
			},
			Example{
				Description: "List your tokens",
				Command:     "coder tokens ls",
			},
			Example{
				Description: "Remove a token by ID",
				Command:     "coder tokens rm WuoWs4ZsMX",
			},
		),
		Aliases: []string{"token"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.createToken(),
			r.listTokens(),
			r.removeToken(),
		},
	}
	return cmd
}

func (r *RootCmd) createToken() *serpent.Command {
	var (
		tokenLifetime time.Duration
		name          string
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "create",
		Short: "Create a token",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
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

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "lifetime",
			Env:         "CODER_TOKEN_LIFETIME",
			Description: "Specify a duration for the lifetime of the token.",
			Default:     (time.Hour * 24 * 30).String(),
			Value:       serpent.DurationOf(&tokenLifetime),
		},
		{
			Flag:          "name",
			FlagShorthand: "n",
			Env:           "CODER_TOKEN_NAME",
			Description:   "Specify a human-readable name.",
			Value:         serpent.StringOf(&name),
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

func (r *RootCmd) listTokens() *serpent.Command {
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
	cmd := &serpent.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List tokens",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
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

	cmd.Options = serpent.OptionSet{
		{
			Flag:          "all",
			FlagShorthand: "a",
			Description:   "Specifies whether all users' tokens will be listed or not (must have Owner role to see all tokens).",
			Value:         serpent.BoolOf(&all),
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) removeToken() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:     "remove <name>",
		Aliases: []string{"delete"},
		Short:   "Delete a token",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
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
