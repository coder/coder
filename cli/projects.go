package cli

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/xlab/treeprint"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/database"
)

func projects() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "projects",
		Aliases: []string{"project"},
		Example: `
  - Create a project for developers to create workspaces

    ` + color.New(color.FgHiMagenta).Sprint("$ coder projects create") + `

  - Make changes to your project, and plan the changes
 
    ` + color.New(color.FgHiMagenta).Sprint("$ coder projects plan <name>") + `

  - Update the project. Your developers can update their workspaces

    ` + color.New(color.FgHiMagenta).Sprint("$ coder projects update <name>"),
	}
	cmd.AddCommand(projectCreate())
	cmd.AddCommand(projectPlan())
	cmd.AddCommand(projectUpdate())

	return cmd
}

func displayProjectImportInfo(cmd *cobra.Command, parameterSchemas []coderd.ParameterSchema, parameterValues []coderd.ComputedParameterValue, resources []coderd.ProjectImportJobResource) error {
	schemaByID := map[string]coderd.ParameterSchema{}
	for _, schema := range parameterSchemas {
		schemaByID[schema.ID.String()] = schema
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n  %s\n\n", color.HiBlackString("Parameters"))
	for _, value := range parameterValues {
		schema, ok := schemaByID[value.SchemaID.String()]
		if !ok {
			return xerrors.Errorf("schema not found: %s", value.Name)
		}
		displayValue := value.SourceValue
		if !schema.RedisplayValue {
			displayValue = "<redacted>"
		}
		output := fmt.Sprintf("%s %s %s", color.HiCyanString(value.Name), color.HiBlackString("="), displayValue)
		if value.DefaultSourceValue {
			output += " (default value)"
		} else if value.Scope != database.ParameterScopeImportJob {
			output += fmt.Sprintf(" (inherited from %s)", value.Scope)
		}

		root := treeprint.NewWithRoot(output)
		if schema.Description != "" {
			root.AddBranch(fmt.Sprintf("%s\n%s", color.HiBlackString("Description"), schema.Description))
		}
		if schema.AllowOverrideSource {
			root.AddBranch(fmt.Sprintf("%s Users can customize this value!", color.HiYellowString("+")))
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "    "+strings.Join(strings.Split(root.String(), "\n"), "\n    "))
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n\n", color.HiBlackString("Resources"))
	for _, resource := range resources {
		transition := color.HiGreenString("start")
		if resource.Transition == database.WorkspaceTransitionStop {
			transition = color.HiRedString("stop")
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    %s %s on %s\n\n", color.HiCyanString(resource.Type), color.HiCyanString(resource.Name), transition)
	}
	return nil
}
