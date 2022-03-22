package cli

import (
	"context"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

func parameters() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "parameters",
		Aliases: []string{"params"},
	}

	cmd.AddCommand(parameterCreate(), parameterList(), parameterDelete())

	return cmd
}

func parseScopeAndID(ctx context.Context, client *codersdk.Client, organization codersdk.Organization, rawScope string, name string) (codersdk.ParameterScope, string, error) {
	scope, err := parseParameterScope(rawScope)
	if err != nil {
		return scope, "", err
	}
	var scopeID string
	switch scope {
	case codersdk.ParameterOrganization:
		if name == "" {
			scopeID = organization.ID
		} else {
			org, err := client.OrganizationByName(ctx, "", name)
			if err != nil {
				return scope, "", err
			}
			scopeID = org.ID
		}
	case codersdk.ParameterProject:
		project, err := client.ProjectByName(ctx, organization.ID, name)
		if err != nil {
			return scope, "", err
		}
		scopeID = project.ID.String()
	case codersdk.ParameterUser:
		user, err := client.User(ctx, name)
		if err != nil {
			return scope, "", err
		}
		scopeID = user.ID
	case codersdk.ParameterWorkspace:
		workspace, err := client.WorkspaceByName(ctx, "", name)
		if err != nil {
			return scope, "", err
		}
		scopeID = workspace.ID.String()
	}
	return scope, scopeID, nil
}

func parseParameterScope(scope string) (codersdk.ParameterScope, error) {
	switch scope {
	case string(codersdk.ParameterOrganization):
		return codersdk.ParameterOrganization, nil
	case string(codersdk.ParameterProject):
		return codersdk.ParameterProject, nil
	case string(codersdk.ParameterUser):
		return codersdk.ParameterUser, nil
	case string(codersdk.ParameterWorkspace):
		return codersdk.ParameterWorkspace, nil
	}
	return codersdk.ParameterOrganization, xerrors.Errorf("no scope found by name %q", scope)
}
