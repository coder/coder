package cli

import (
	"context"

	"github.com/google/uuid"
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

func parseScopeAndID(ctx context.Context, client *codersdk.Client, organization codersdk.Organization, rawScope string, name string) (codersdk.ParameterScope, uuid.UUID, error) {
	scope, err := parseParameterScope(rawScope)
	if err != nil {
		return scope, uuid.Nil, err
	}

	var scopeID uuid.UUID
	switch scope {
	case codersdk.ParameterOrganization:
		if name == "" {
			scopeID = organization.ID
		} else {
			org, err := client.OrganizationByName(ctx, codersdk.Me, name)
			if err != nil {
				return scope, uuid.Nil, err
			}
			scopeID = org.ID
		}
	case codersdk.ParameterTemplate:
		template, err := client.TemplateByName(ctx, organization.ID, name)
		if err != nil {
			return scope, uuid.Nil, err
		}
		scopeID = template.ID
	case codersdk.ParameterUser:
		uid, _ := uuid.Parse(name)
		user, err := client.User(ctx, uid)
		if err != nil {
			return scope, uuid.Nil, err
		}
		scopeID = user.ID
	case codersdk.ParameterWorkspace:
		workspace, err := client.WorkspaceByOwnerAndName(ctx, organization.ID, codersdk.Me, name)
		if err != nil {
			return scope, uuid.Nil, err
		}
		scopeID = workspace.ID
	}

	return scope, scopeID, nil
}

func parseParameterScope(scope string) (codersdk.ParameterScope, error) {
	switch scope {
	case string(codersdk.ParameterOrganization):
		return codersdk.ParameterOrganization, nil
	case string(codersdk.ParameterTemplate):
		return codersdk.ParameterTemplate, nil
	case string(codersdk.ParameterUser):
		return codersdk.ParameterUser, nil
	case string(codersdk.ParameterWorkspace):
		return codersdk.ParameterWorkspace, nil
	}
	return codersdk.ParameterOrganization, xerrors.Errorf("no scope found by name %q", scope)
}
