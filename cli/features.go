package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coder/coder/cli/cliui"
	"github.com/jedib0t/go-pretty/v6/table"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

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
			err := cliui.ValidateColumns(featureColumns, columns)
			if err != nil {
				return err
			}
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
				out = displayFeatures(columns, entitlements.Features)
			case "json":
				outBytes, err := json.Marshal(entitlements)
				if err != nil {
					return xerrors.Errorf("marshal users to JSON: %w", err)
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

// displayFeatures will return a table displaying all features passed in.
// filterColumns must be a subset of the feature fields and will determine which
// columns to display
func displayFeatures(filterColumns []string, features map[string]codersdk.Feature) string {
	tableWriter := cliui.Table()
	header := table.Row{}
	for _, h := range featureColumns {
		header = append(header, h)
	}
	tableWriter.AppendHeader(header)
	tableWriter.SetColumnConfigs(cliui.FilterTableColumns(header, filterColumns))
	tableWriter.SortBy([]table.SortBy{{
		Name: "username",
	}})
	for name, feat := range features {
		tableWriter.AppendRow(table.Row{
			name,
			feat.Entitlement,
			feat.Enabled,
			intOrNil(feat.Limit),
			intOrNil(feat.Actual),
		})
	}
	return tableWriter.Render()
}

func intOrNil(i *int64) string {
	if i == nil {
		return ""
	}
	return fmt.Sprintf("%d", *i)
}
