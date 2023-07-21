package cli

import (
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

// workspaceParameterFlags are used by "start", "restart", and "update".
type workspaceParameterFlags struct {
	buildOptions bool
}

func (wpf *workspaceParameterFlags) options() []clibase.Option {
	return clibase.OptionSet{
		{
			Flag:        "build-options",
			Description: "Prompt for one-time build options defined with ephemeral parameters.",
			Value:       clibase.BoolOf(&wpf.buildOptions),
		},
	}
}

func (r *RootCmd) start() *clibase.Cmd {
	var parameterFlags workspaceParameterFlags

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "start <workspace>",
		Short:       "Start a workspace",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Options: append(parameterFlags.options(), cliui.SkipPromptOption()),
		Handler: func(inv *clibase.Invocation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}

			template, err := client.Template(inv.Context(), workspace.TemplateID)
			if err != nil {
				return err
			}

			buildParams, err := prepStartWorkspace(inv, client, prepStartWorkspaceArgs{
				Template:     template,
				BuildOptions: parameterFlags.buildOptions,
			})
			if err != nil {
				return err
			}

			build, err := client.CreateWorkspaceBuild(inv.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition:          codersdk.WorkspaceTransitionStart,
				RichParameterValues: buildParams.richParameters,
			})
			if err != nil {
				return err
			}

			err = cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, build.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(inv.Stdout, "\nThe %s workspace has been started at %s!\n", cliui.DefaultStyles.Keyword.Render(workspace.Name), cliui.DefaultStyles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
			return nil
		},
	}
	return cmd
}

type prepStartWorkspaceArgs struct {
	Template     codersdk.Template
	BuildOptions bool
}

func prepStartWorkspace(inv *clibase.Invocation, client *codersdk.Client, args prepStartWorkspaceArgs) (*buildParameters, error) {
	ctx := inv.Context()

	templateVersion, err := client.TemplateVersion(ctx, args.Template.ActiveVersionID)
	if err != nil {
		return nil, err
	}

	templateVersionParameters, err := client.TemplateVersionRichParameters(inv.Context(), templateVersion.ID)
	if err != nil {
		return nil, xerrors.Errorf("get template version rich parameters: %w", err)
	}

	richParameters := make([]codersdk.WorkspaceBuildParameter, 0)
	if !args.BuildOptions {
		return &buildParameters{
			richParameters: richParameters,
		}, nil
	}

	for _, templateVersionParameter := range templateVersionParameters {
		if !templateVersionParameter.Ephemeral {
			continue
		}

		parameterValue, err := cliui.RichParameter(inv, templateVersionParameter)
		if err != nil {
			return nil, err
		}

		richParameters = append(richParameters, codersdk.WorkspaceBuildParameter{
			Name:  templateVersionParameter.Name,
			Value: parameterValue,
		})
	}

	return &buildParameters{
		richParameters: richParameters,
	}, nil
}
