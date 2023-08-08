package cli

import (
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/codersdk"
)

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

	richParameters     []codersdk.WorkspaceBuildParameter
	richParametersFile map[string]string
	buildOptions       []codersdk.WorkspaceBuildParameter

	promptRichParameters bool
	promptBuildOptions   bool
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

func (pr *ParameterResolver) WithPromptRichParameters(promptRichParameters bool) *ParameterResolver {
	pr.promptRichParameters = promptRichParameters
	return pr
}

func (pr *ParameterResolver) WithPromptBuildOptions(promptBuildOptions bool) *ParameterResolver {
	pr.promptBuildOptions = promptBuildOptions
	return pr
}

func (pr *ParameterResolver) Resolve(inv *clibase.Invocation, action WorkspaceCLIAction, templateVersionParameters []codersdk.TemplateVersionParameter) ([]codersdk.WorkspaceBuildParameter, error) {
	var staged []codersdk.WorkspaceBuildParameter
	var err error

	staged = pr.resolveWithParametersMapFile(staged)
	staged = pr.resolveWithCommandLineOrEnv(staged)
	if err = pr.verifyConstraints(staged); err != nil {
		return nil, err
	}
	staged = pr.resolveWithLastBuildParameters(staged)
	staged = pr.resolveWithInput(staged, inv, action)
	return staged, nil
}

func (pr *ParameterResolver) resolveWithParametersMapFile(resolved []codersdk.WorkspaceBuildParameter) []codersdk.WorkspaceBuildParameter {
	for name, value := range pr.richParametersFile {
		for i, r := range resolved {
			if r.Name == name {
				resolved[i].Value = value
				goto done
			}
		}

		resolved = append(resolved, codersdk.WorkspaceBuildParameter{
			Name:  name,
			Value: value,
		})
	done:
	}
	return resolved
}

func (pr *ParameterResolver) resolveWithCommandLineOrEnv(resolved []codersdk.WorkspaceBuildParameter) []codersdk.WorkspaceBuildParameter {
	for _, richParameter := range pr.richParameters {
		for i, r := range resolved {
			if r.Name == richParameter.Name {
				resolved[i].Value = richParameter.Value
				goto richParameterDone
			}
		}

		resolved = append(resolved, richParameter)
	richParameterDone:
	}

	if pr.promptBuildOptions {
		for _, buildOption := range pr.buildOptions {
			for i, r := range resolved {
				if r.Name == buildOption.Name {
					resolved[i].Value = buildOption.Value
					goto buildOptionDone
				}
			}

			resolved = append(resolved, buildOption)
		buildOptionDone:
		}
	}
	return resolved
}

func (pr *ParameterResolver) resolveWithLastBuildParameters(resolved []codersdk.WorkspaceBuildParameter) []codersdk.WorkspaceBuildParameter {
	for _, buildParameter := range pr.lastBuildParameters {
		for i, r := range resolved {
			if r.Name == buildParameter.Name {
				resolved[i].Value = buildParameter.Value
				goto done
			}
		}

		resolved = append(resolved, buildParameter)
	done:
	}
	return resolved
}

func (pr *ParameterResolver) resolveWithInput(resolved []codersdk.WorkspaceBuildParameter, iv *clibase.Invocation, action WorkspaceCLIAction) []codersdk.WorkspaceBuildParameter {
	// update == then skip if in last build parameters unless prompt-all, build options
	// update == immutable

	panic("not implemented yet")

	return resolved
}

func (pr *ParameterResolver) verifyConstraints(resolved []codersdk.WorkspaceBuildParameter) error {
	panic("not implemented yet")
}
