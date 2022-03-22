package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func parameterList() *cobra.Command {
	return &cobra.Command{
		Use: "list <scope> <scope-id>",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			name := ""
			if len(args) >= 2 {
				name = args[1]
			}
			scope, scopeID, err := parseScopeAndID(cmd.Context(), client, organization, args[0], name)
			if err != nil {
				return err
			}
			params, err := client.Parameters(cmd.Context(), scope, scopeID)
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 4, ' ', 0)
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\n",
				color.HiBlackString("Parameter"),
				color.HiBlackString("Created"),
				color.HiBlackString("Scheme"))
			for _, param := range params {
				_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\n",
					color.New(color.FgHiCyan).Sprint(param.Name),
					color.WhiteString(param.UpdatedAt.Format("January 2, 2006")),
					color.New(color.FgHiWhite).Sprint(param.DestinationScheme))
			}
			return writer.Flush()
		},
	}
}
