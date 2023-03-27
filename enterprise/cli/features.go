package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) features() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Short:   "List Enterprise features",
		Use:     "features",
		Aliases: []string{"feature"},
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.featuresList(),
		},
	}
	return cmd
}

func (r *RootCmd) featuresList() *clibase.Cmd {
	var (
		featureColumns = []string{"Name", "Entitlement", "Enabled", "Limit", "Actual"}
		columns        []string
		outputFormat   string
	)
	client := new(codersdk.Client)

	cmd := &clibase.Cmd{
		Use:     "list",
		Aliases: []string{"ls"},
		Middleware: clibase.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			entitlements, err := client.Entitlements(inv.Context())
			var apiError *codersdk.Error
			if errors.As(err, &apiError) && apiError.StatusCode() == http.StatusNotFound {
				return xerrors.New("You are on the AGPL licensed version of Coder that does not have Enterprise functionality!")
			}
			if err != nil {
				return err
			}

			// This uses custom formatting as the JSON output outputs an object
			// as opposed to a list from the table output.
			out := ""
			switch outputFormat {
			case "table", "":
				out, err = displayFeatures(columns, entitlements.Features)
				if err != nil {
					return xerrors.Errorf("render table: %w", err)
				}
			case "json":
				buf := new(bytes.Buffer)
				enc := json.NewEncoder(buf)
				enc.SetIndent("", "  ")
				err = enc.Encode(entitlements)
				if err != nil {
					return xerrors.Errorf("marshal features to JSON: %w", err)
				}
				out = buf.String()
			default:
				return xerrors.Errorf(`unknown output format %q, only "table" and "json" are supported`, outputFormat)
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:          "column",
			FlagShorthand: "c",
			Description: fmt.Sprintf("Specify a column to filter in the table. Available columns are: %s.",
				strings.Join(featureColumns, ", "),
			),
			Default: strings.Join(featureColumns, ","),
			Value:   clibase.StringArrayOf(&columns),
		},
		{
			Flag:          "output",
			FlagShorthand: "o",
			Description:   "Output format. Available formats are: table, json.",
			Default:       "table",
			Value:         clibase.StringOf(&outputFormat),
		},
	}

	return cmd
}

type featureRow struct {
	Name        codersdk.FeatureName `table:"name,default_sort"`
	Entitlement string               `table:"entitlement"`
	Enabled     bool                 `table:"enabled"`
	Limit       *int64               `table:"limit"`
	Actual      *int64               `table:"actual"`
}

// displayFeatures will return a table displaying all features passed in.
// filterColumns must be a subset of the feature fields and will determine which
// columns to display
func displayFeatures(filterColumns []string, features map[codersdk.FeatureName]codersdk.Feature) (string, error) {
	rows := make([]featureRow, 0, len(features))
	for name, feat := range features {
		rows = append(rows, featureRow{
			Name:        name,
			Entitlement: string(feat.Entitlement),
			Enabled:     feat.Enabled,
			Limit:       feat.Limit,
			Actual:      feat.Actual,
		})
	}

	return cliui.DisplayTable(rows, "name", filterColumns)
}
