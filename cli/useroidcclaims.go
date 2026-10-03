package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) userOIDCClaims() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat([]claimRow{}, []string{"key", "value"}),
			func(data any) (any, error) {
				resp, ok := data.(codersdk.OIDCClaimsResponse)
				if !ok {
					return nil, xerrors.Errorf("expected type %T, got %T", resp, data)
				}
				rows := make([]claimRow, 0, len(resp.Claims))
				for k, v := range resp.Claims {
					rows = append(rows, claimRow{
						Key:   k,
						Value: fmt.Sprintf("%v", v),
					})
				}
				return rows, nil
			},
		),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "oidc-claims",
		Short: "Display the OIDC claims for the authenticated user.",
		Long: FormatExamples(
			Example{
				Description: "Display your OIDC claims",
				Command:     "coder users oidc-claims",
			},
			Example{
				Description: "Display your OIDC claims as JSON",
				Command:     "coder users oidc-claims -o json",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			resp, err := client.UserOIDCClaims(inv.Context())
			if err != nil {
				return xerrors.Errorf("get oidc claims: %w", err)
			}

			out, err := formatter.Format(inv.Context(), resp)
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

type claimRow struct {
	Key   string `json:"-" table:"key,default_sort"`
	Value string `json:"-" table:"value"`
}
