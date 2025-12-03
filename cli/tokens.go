package cli

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/slice"
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
				Description: "Create a scoped token",
				Command:     "coder tokens create --scope workspace:read --allow workspace:<uuid>",
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
			r.viewToken(),
			r.removeToken(),
		},
	}
	return cmd
}

func (r *RootCmd) createToken() *serpent.Command {
	var (
		tokenLifetime string
		name          string
		user          string
		scopes        []string
		allowList     []codersdk.APIAllowListTarget
	)
	cmd := &serpent.Command{
		Use:   "create",
		Short: "Create a token",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			userID := codersdk.Me
			if user != "" {
				userID = user
			}

			var parsedLifetime time.Duration

			tokenConfig, err := client.GetTokenConfig(inv.Context(), userID)
			if err != nil {
				return xerrors.Errorf("get token config: %w", err)
			}

			if tokenLifetime == "" {
				parsedLifetime = tokenConfig.MaxTokenLifetime
			} else {
				parsedLifetime, err = extendedParseDuration(tokenLifetime)
				if err != nil {
					return xerrors.Errorf("parse lifetime: %w", err)
				}

				if parsedLifetime > tokenConfig.MaxTokenLifetime {
					return xerrors.Errorf("lifetime (%s) is greater than the maximum allowed lifetime (%s)", parsedLifetime, tokenConfig.MaxTokenLifetime)
				}
			}

			req := codersdk.CreateTokenRequest{
				Lifetime:  parsedLifetime,
				TokenName: name,
			}
			if len(req.Scopes) == 0 {
				req.Scopes = slice.StringEnums[codersdk.APIKeyScope](scopes)
			}
			if len(allowList) > 0 {
				req.AllowList = append([]codersdk.APIAllowListTarget(nil), allowList...)
			}

			res, err := client.CreateToken(inv.Context(), userID, req)
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
			Description: "Duration for the token lifetime. Supports standard Go duration units (ns, us, ms, s, m, h) plus d (days) and y (years). Examples: 8h, 30d, 1y, 1d12h30m.",
			Value:       serpent.StringOf(&tokenLifetime),
		},
		{
			Flag:          "name",
			FlagShorthand: "n",
			Env:           "CODER_TOKEN_NAME",
			Description:   "Specify a human-readable name.",
			Value:         serpent.StringOf(&name),
		},
		{
			Flag:          "user",
			FlagShorthand: "u",
			Env:           "CODER_TOKEN_USER",
			Description:   "Specify the user to create the token for (Only works if logged in user is admin).",
			Value:         serpent.StringOf(&user),
		},
		{
			Flag:        "scope",
			Description: "Repeatable scope to attach to the token (e.g. workspace:read).",
			Value:       serpent.StringArrayOf(&scopes),
		},
		{
			Flag:        "allow",
			Description: "Repeatable allow-list entry (<type>:<uuid>, e.g. workspace:1234-...).",
			Value:       AllowListFlagOf(&allowList),
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
	Scopes    string    `json:"-" table:"scopes"`
	Allow     string    `json:"-" table:"allow list"`
	LastUsed  time.Time `json:"-" table:"last used"`
	ExpiresAt time.Time `json:"-" table:"expires at"`
	CreatedAt time.Time `json:"-" table:"created at"`
	Owner     string    `json:"-" table:"owner"`
}

func tokenListRowFromToken(token codersdk.APIKeyWithOwner) tokenListRow {
	return tokenListRowFromKey(token.APIKey, token.Username)
}

func tokenListRowFromKey(token codersdk.APIKey, owner string) tokenListRow {
	return tokenListRow{
		APIKey:    token,
		ID:        token.ID,
		TokenName: token.TokenName,
		Scopes:    joinScopes(token.Scopes),
		Allow:     joinAllowList(token.AllowList),
		LastUsed:  token.LastUsed,
		ExpiresAt: token.ExpiresAt,
		CreatedAt: token.CreatedAt,
		Owner:     owner,
	}
}

func joinScopes(scopes []codersdk.APIKeyScope) string {
	if len(scopes) == 0 {
		return ""
	}
	vals := slice.ToStrings(scopes)
	sort.Strings(vals)
	return strings.Join(vals, ", ")
}

func joinAllowList(entries []codersdk.APIAllowListTarget) string {
	if len(entries) == 0 {
		return ""
	}
	vals := make([]string, len(entries))
	for i, entry := range entries {
		vals[i] = entry.String()
	}
	sort.Strings(vals)
	return strings.Join(vals, ", ")
}

func (r *RootCmd) listTokens() *serpent.Command {
	// we only display the 'owner' column if the --all argument is passed in
	defaultCols := []string{"id", "name", "scopes", "allow list", "last used", "expires at", "created at"}
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

	cmd := &serpent.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List tokens",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			tokens, err := client.Tokens(inv.Context(), codersdk.Me, codersdk.TokensFilter{
				IncludeAll: all,
			})
			if err != nil {
				return xerrors.Errorf("list tokens: %w", err)
			}

			displayTokens = make([]tokenListRow, len(tokens))

			for i, token := range tokens {
				displayTokens[i] = tokenListRowFromToken(token)
			}

			out, err := formatter.Format(inv.Context(), displayTokens)
			if err != nil {
				return err
			}

			if out == "" {
				cliui.Info(inv.Stderr, "No tokens found.")
				return nil
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

func (r *RootCmd) viewToken() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]tokenListRow{}, []string{"id", "name", "scopes", "allow list", "last used", "expires at", "created at", "owner"}),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "view <name|id>",
		Short: "Display detailed information about a token",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			tokenName := inv.Args[0]
			token, err := client.APIKeyByName(inv.Context(), codersdk.Me, tokenName)
			if err != nil {
				maybeID := strings.Split(tokenName, "-")[0]
				token, err = client.APIKeyByID(inv.Context(), codersdk.Me, maybeID)
				if err != nil {
					return xerrors.Errorf("fetch api key by name or id: %w", err)
				}
			}

			row := tokenListRowFromKey(*token, "")
			out, err := formatter.Format(inv.Context(), []tokenListRow{row})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) removeToken() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "remove <name|id|token>",
		Aliases: []string{"delete"},
		Short:   "Delete a token",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			token, err := client.APIKeyByName(inv.Context(), codersdk.Me, inv.Args[0])
			if err != nil {
				// If it's a token, we need to extract the ID
				maybeID := strings.Split(inv.Args[0], "-")[0]
				token, err = client.APIKeyByID(inv.Context(), codersdk.Me, maybeID)
				if err != nil {
					return xerrors.Errorf("fetch api key by name or id: %w", err)
				}
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
