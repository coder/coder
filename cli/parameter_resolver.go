package cli

import "github.com/coder/coder/codersdk"

type WorkspaceCLIAction int

const (
	WorkspaceCreate WorkspaceCLIAction = iota
	WorkspaceStart
	WorkspaceUpdate
	WorkspaceRestart
)

type ParameterResolver struct {
	action                    WorkspaceCLIAction
	templateVersionParameters []codersdk.TemplateVersionParameter

	lastBuildParameters []codersdk.WorkspaceBuildParameter
	richParameters      []codersdk.WorkspaceBuildParameter
	buildOptions        []codersdk.WorkspaceBuildParameter
	richParametersFile  map[string]string

	alwaysPrompt       bool
	promptBuildOptions bool
}

func (pr *ParameterResolver) WithLastBuildParameters(params []codersdk.WorkspaceBuildParameter) *ParameterResolver {
	pr.lastBuildParameters = params
	return pr
}

func (pr *ParameterResolver) WithRichParameters(params []codersdk.WorkspaceBuildParameter) *ParameterResolver {
	pr.richParameters = params
	return pr
}

func (pr *ParameterResolver) WithBuildOptions(params []codersdk.WorkspaceBuildParameter) *ParameterResolver {
	pr.buildOptions = params
	return pr
}

func (pr *ParameterResolver) WithRichParametersFile(fileMap map[string]string) *ParameterResolver {
	pr.richParametersFile = fileMap
	return pr
}

func (pr *ParameterResolver) WithAlwaysPrompt(alwaysPrompt bool) *ParameterResolver {
	pr.alwaysPrompt = alwaysPrompt
	return pr
}

func (pr *ParameterResolver) WithPromptBuildOptions(promptBuildOptions bool) *ParameterResolver {
	pr.promptBuildOptions = promptBuildOptions
	return pr
}

func (pr *ParameterResolver) Resolve(action WorkspaceCLIAction, templateVersionParameters []codersdk.TemplateVersionParameter) ([]codersdk.WorkspaceBuildParameter, error) {
	panic("not implemented yet")
	/*
	   if Start or Restart {

	   	richParameters := make([]codersdk.WorkspaceBuildParameter, 0)
	   	if !args.PromptBuildOptions && len(args.BuildOptions) == 0 {
	   		return &buildParameters{
	   			richParameters: richParameters,
	   		}, nil
	   	}

	   	for _, templateVersionParameter := range templateVersionParameters {
	   		if !templateVersionParameter.Ephemeral {
	   			continue
	   		}

	   		buildOption, err := getParameterFromCommandLineOrInput(inv, args.PromptBuildOptions, args.BuildOptions, templateVersionParameter)
	   		if err != nil {
	   			return nil, err
	   		}
	   		richParameters = append(richParameters, buildOption)
	   	}

	   }

	   if Create or Update {

	   	parameterMapFromFile := map[string]string{}
	   	useParamFile := false
	   	if args.RichParameterFile != "" {
	   		useParamFile = true
	   		_, _ = fmt.Fprintln(inv.Stdout, cliui.DefaultStyles.Paragraph.Render("Attempting to read parameters from the rich parameter file.")+"\r\n")
	   		parameterMapFromFile, err = createParameterMapFromFile(args.RichParameterFile)
	   		if err != nil {
	   			return nil, err
	   		}
	   	}
	   	disclaimerPrinted := false
	   	richParameters := make([]codersdk.WorkspaceBuildParameter, 0)

	   PromptRichParamLoop:

	   	for _, templateVersionParameter := range templateVersionParameters {
	   		if !args.PromptBuildOptions && len(args.BuildOptions) == 0 && templateVersionParameter.Ephemeral {
	   			continue
	   		}

	   		if !disclaimerPrinted {
	   			_, _ = fmt.Fprintln(inv.Stdout, cliui.DefaultStyles.Paragraph.Render("This template has customizable parameters. Values can be changed after create, but may have unintended side effects (like data loss).")+"\r\n")
	   			disclaimerPrinted = true
	   		}

	   		// Param file is all or nothing
	   		if !useParamFile && !templateVersionParameter.Ephemeral {
	   			for _, e := range args.ExistingRichParams {
	   				if e.Name == templateVersionParameter.Name {
	   					// If the param already exists, we do not need to prompt it again.
	   					// The workspace scope will reuse params for each build.
	   					continue PromptRichParamLoop
	   				}
	   			}
	   		}

	   		if args.UpdateWorkspace && !templateVersionParameter.Mutable {
	   			// Check if the immutable parameter was used in the previous build. If so, then it isn't a fresh one
	   			// and the user should be warned.
	   			exists, err := workspaceBuildParameterExists(ctx, client, args.WorkspaceID, templateVersionParameter)
	   			if err != nil {
	   				return nil, err
	   			}

	   			if exists {
	   				_, _ = fmt.Fprintln(inv.Stdout, cliui.DefaultStyles.Warn.Render(fmt.Sprintf(`Parameter %q is not mutable, so can't be customized after workspace creation.`, templateVersionParameter.Name)))
	   				continue
	   			}
	   		}

	   		parameter, err := getParameter(inv, getParameterArgs{
	   			promptBuildOptions:       args.PromptBuildOptions,
	   			buildOptions:             args.BuildOptions,
	   			parameterMap:             parameterMapFromFile,
	   			templateVersionParameter: templateVersionParameter,
	   		})
	   		if err != nil {
	   			return nil, err
	   		}

	   		richParameters = append(richParameters, parameter)
	   	}

	   	if disclaimerPrinted {
	   		_, _ = fmt.Fprintln(inv.Stdout)
	   	}

	   }
	*/
}
