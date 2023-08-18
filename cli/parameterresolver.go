package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
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
	staged = pr.resolveWithLastBuildParameters(staged, templateVersionParameters)
	if err = pr.verifyConstraints(staged, action, templateVersionParameters); err != nil {
		return nil, err
	}
	if staged, err = pr.resolveWithInput(staged, inv, action, templateVersionParameters); err != nil {
		return nil, err
	}
	return staged, nil
}

func (pr *ParameterResolver) resolveWithParametersMapFile(resolved []codersdk.WorkspaceBuildParameter) []codersdk.WorkspaceBuildParameter {
next:
	for name, value := range pr.richParametersFile {
		for i, r := range resolved {
			if r.Name == name {
				resolved[i].Value = value
				continue next
			}
		}

		resolved = append(resolved, codersdk.WorkspaceBuildParameter{
			Name:  name,
			Value: value,
		})
	}
	return resolved
}

func (pr *ParameterResolver) resolveWithCommandLineOrEnv(resolved []codersdk.WorkspaceBuildParameter) []codersdk.WorkspaceBuildParameter {
nextRichParameter:
	for _, richParameter := range pr.richParameters {
		for i, r := range resolved {
			if r.Name == richParameter.Name {
				resolved[i].Value = richParameter.Value
				continue nextRichParameter
			}
		}

		resolved = append(resolved, richParameter)
	}

nextBuildOption:
	for _, buildOption := range pr.buildOptions {
		for i, r := range resolved {
			if r.Name == buildOption.Name {
				resolved[i].Value = buildOption.Value
				continue nextBuildOption
			}
		}

		resolved = append(resolved, buildOption)
	}
	return resolved
}

func (pr *ParameterResolver) resolveWithLastBuildParameters(resolved []codersdk.WorkspaceBuildParameter, templateVersionParameters []codersdk.TemplateVersionParameter) []codersdk.WorkspaceBuildParameter {
	if pr.promptRichParameters {
		return resolved // don't pull parameters from last build
	}

next:
	for _, buildParameter := range pr.lastBuildParameters {
		tvp := findTemplateVersionParameter(buildParameter, templateVersionParameters)
		if tvp == nil {
			continue // it looks like this parameter is not present anymore
		}

		if tvp.Ephemeral {
			continue // ephemeral parameters should not be passed to consecutive builds
		}

		if !tvp.Mutable {
			continue // immutables should not be passed to consecutive builds
		}

		for i, r := range resolved {
			if r.Name == buildParameter.Name {
				resolved[i].Value = buildParameter.Value
				continue next
			}
		}

		resolved = append(resolved, buildParameter)
	}
	return resolved
}

func (pr *ParameterResolver) verifyConstraints(resolved []codersdk.WorkspaceBuildParameter, action WorkspaceCLIAction, templateVersionParameters []codersdk.TemplateVersionParameter) error {
	for _, r := range resolved {
		tvp := findTemplateVersionParameter(r, templateVersionParameters)
		if tvp == nil {
			return xerrors.Errorf("parameter %q is not present in the template", r.Name)
		}

		if tvp.Ephemeral && !pr.promptBuildOptions && findWorkspaceBuildParameter(tvp.Name, pr.buildOptions) == nil {
			return xerrors.Errorf("ephemeral parameter %q can be used only with --build-options or --build-option flag", r.Name)
		}

		if !tvp.Mutable && action != WorkspaceCreate {
			return xerrors.Errorf("parameter %q is immutable and cannot be updated", r.Name)
		}
	}
	return nil
}

func (pr *ParameterResolver) resolveWithInput(resolved []codersdk.WorkspaceBuildParameter, inv *clibase.Invocation, action WorkspaceCLIAction, templateVersionParameters []codersdk.TemplateVersionParameter) ([]codersdk.WorkspaceBuildParameter, error) {
	for _, tvp := range templateVersionParameters {
		p := findWorkspaceBuildParameter(tvp.Name, resolved)
		if p != nil {
			continue
		}
		// Parameter has not been resolved yet, so CLI needs to determine if user should input it.

		firstTimeUse := pr.isFirstTimeUse(tvp.Name)

		if (tvp.Ephemeral && pr.promptBuildOptions) ||
			(action == WorkspaceCreate && tvp.Required) ||
			(action == WorkspaceCreate && !tvp.Ephemeral) ||
			(action == WorkspaceUpdate && tvp.Required) ||
			(action == WorkspaceUpdate && !tvp.Mutable && firstTimeUse) ||
			(action == WorkspaceUpdate && tvp.Mutable && !tvp.Ephemeral && pr.promptRichParameters) {
			parameterValue, err := cliui.RichParameter(inv, tvp)
			if err != nil {
				return nil, err
			}

			resolved = append(resolved, codersdk.WorkspaceBuildParameter{
				Name:  tvp.Name,
				Value: parameterValue,
			})
		} else if action == WorkspaceUpdate && !tvp.Mutable && !firstTimeUse {
			_, _ = fmt.Fprintln(inv.Stdout, cliui.DefaultStyles.Warn.Render(fmt.Sprintf("Parameter %q is not mutable, and cannot be customized after workspace creation.", tvp.Name)))
		}
	}
	return resolved, nil
}

func (pr *ParameterResolver) isFirstTimeUse(parameterName string) bool {
	return findWorkspaceBuildParameter(parameterName, pr.lastBuildParameters) == nil
}

func findTemplateVersionParameter(workspaceBuildParameter codersdk.WorkspaceBuildParameter, templateVersionParameters []codersdk.TemplateVersionParameter) *codersdk.TemplateVersionParameter {
	for _, tvp := range templateVersionParameters {
		if tvp.Name == workspaceBuildParameter.Name {
			return &tvp
		}
	}
	return nil
}

func findWorkspaceBuildParameter(parameterName string, params []codersdk.WorkspaceBuildParameter) *codersdk.WorkspaceBuildParameter {
	for _, p := range params {
		if p.Name == parameterName {
			return &p
		}
	}
	return nil
}
