package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

var featureColumns = []string{"Name", "Entitlement", "Enabled", "Limit", "Actual"}

func features() *cobra.Command {
	cmd := &cobra.Command{
		Short:   "List features",
		Use:     "features",
		Aliases: []string{"feature"},
	}
	cmd.AddCommand(
		featuresList(),
	)
	return cmd
}

func featuresList() *cobra.Command {
	var (
		columns      []string
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			entitlements, err := client.Entitlements(cmd.Context())
			if err != nil {
				return err
			}

			out := ""
			switch outputFormat {
			case "table", "":
				out, err = displayFeatures(columns, entitlements.Features)
				if err != nil {
					return xerrors.Errorf("render table: %w", err)
				}
			case "json":
				outBytes, err := json.Marshal(entitlements)
				if err != nil {
					return xerrors.Errorf("marshal features to JSON: %w", err)
				}

				out = string(outBytes)
			default:
				return xerrors.Errorf(`unknown output format %q, only "table" and "json" are supported`, outputFormat)
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}

	cmd.Flags().StringArrayVarP(&columns, "column", "c", featureColumns,
		fmt.Sprintf("Specify a column to filter in the table. Available columns are: %s",
			strings.Join(featureColumns, ", ")))
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format. Available formats are: table, json.")
	return cmd
}

type featureRow struct {
	Name        string `table:"name"`
	Entitlement string `table:"entitlement"`
	Enabled     bool   `table:"enabled"`
	Limit       *int64 `table:"limit"`
	Actual      *int64 `table:"actual"`
}

// displayFeatures will return a table displaying all features passed in.
// filterColumns must be a subset of the feature fields and will determine which
// columns to display
func displayFeatures(filterColumns []string, features map[string]codersdk.Feature) (string, error) {
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
