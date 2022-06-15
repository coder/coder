package cli

import (
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

func parameters() *cobra.Command {
	cmd := &cobra.Command{
		Short:   "List parameters for a given scope",
		Use:     "parameters",
		Aliases: []string{"params"},
	}
	cmd.AddCommand(
		parameterList(),
	)
	return cmd
}

// displayParameters will return a table displaying all parameters passed in.
// filterColumns must be a subset of the parameter fields and will determine which
// columns to display
func displayParameters(filterColumns []string, params ...codersdk.Parameter) string {
	tableWriter := cliui.Table()
	header := table.Row{"id", "scope", "scope id", "name", "source scheme", "destination scheme", "created at", "updated at"}
	tableWriter.AppendHeader(header)
	tableWriter.SetColumnConfigs(cliui.FilterTableColumns(header, filterColumns))
	tableWriter.SortBy([]table.SortBy{{
		Name: "name",
	}})
	for _, param := range params {
		//fmt.Println(param, filterColumns)
		tableWriter.AppendRow(table.Row{
			param.ID.String(),
			param.Scope,
			param.ScopeID.String(),
			param.Name,
			param.SourceScheme,
			param.DestinationScheme,
			param.CreatedAt,
			param.UpdatedAt,
		})
	}
	return tableWriter.Render()
}
