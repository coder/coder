package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
)

func parameterCreate() *cobra.Command {
	var (
		name   string
		value  string
		scheme string
	)
	cmd := &cobra.Command{
		Use:     "create <scope> [name]",
		Aliases: []string{"mk"},
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			scopeName := ""
			if len(args) >= 2 {
				scopeName = args[1]
			}
			scope, scopeID, err := parseScopeAndID(cmd.Context(), client, organization, args[0], scopeName)
			if err != nil {
				return err
			}
			scheme, err := parseParameterScheme(scheme)
			if err != nil {
				return err
			}
			_, err = client.CreateParameter(cmd.Context(), scope, scopeID, codersdk.CreateParameterRequest{
				Name:              name,
				SourceValue:       value,
				SourceScheme:      database.ParameterSourceSchemeData,
				DestinationScheme: scheme,
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Printf("Created!\n")
			return nil
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "Name for a parameter.")
	_ = cmd.MarkFlagRequired("name")
	cmd.Flags().StringVarP(&value, "value", "v", "", "Value for a parameter.")
	_ = cmd.MarkFlagRequired("value")
	cmd.Flags().StringVarP(&scheme, "scheme", "s", "var", `Scheme for the parameter ("var" or "env").`)

	return cmd
}

func parseParameterScheme(scheme string) (database.ParameterDestinationScheme, error) {
	switch scheme {
	case "env":
		return database.ParameterDestinationSchemeEnvironmentVariable, nil
	case "var":
		return database.ParameterDestinationSchemeProvisionerVariable, nil
	}
	return database.ParameterDestinationSchemeNone, xerrors.Errorf("scheme %q not recognized", scheme)
}
