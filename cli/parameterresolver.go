package cli
import (
	"errors"
	"fmt"
	"strings"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil/levenshtein"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)
type WorkspaceCLIAction int
const (
	WorkspaceCreate WorkspaceCLIAction = iota
	WorkspaceStart
	WorkspaceUpdate
	WorkspaceRestart
)
type ParameterResolver struct {
	lastBuildParameters       []codersdk.WorkspaceBuildParameter
	sourceWorkspaceParameters []codersdk.WorkspaceBuildParameter
	richParameters         []codersdk.WorkspaceBuildParameter
	richParametersDefaults map[string]string
	richParametersFile     map[string]string
	ephemeralParameters    []codersdk.WorkspaceBuildParameter
	promptRichParameters      bool
	promptEphemeralParameters bool
}
func (pr *ParameterResolver) WithLastBuildParameters(params []codersdk.WorkspaceBuildParameter) *ParameterResolver {
	pr.lastBuildParameters = params
	return pr
}
func (pr *ParameterResolver) WithSourceWorkspaceParameters(params []codersdk.WorkspaceBuildParameter) *ParameterResolver {
	pr.sourceWorkspaceParameters = params
	return pr
}
func (pr *ParameterResolver) WithRichParameters(params []codersdk.WorkspaceBuildParameter) *ParameterResolver {
	pr.richParameters = params
	return pr
}
func (pr *ParameterResolver) WithEphemeralParameters(params []codersdk.WorkspaceBuildParameter) *ParameterResolver {
	pr.ephemeralParameters = params
	return pr
}
func (pr *ParameterResolver) WithRichParametersFile(fileMap map[string]string) *ParameterResolver {
	pr.richParametersFile = fileMap
	return pr
}
func (pr *ParameterResolver) WithRichParametersDefaults(params []codersdk.WorkspaceBuildParameter) *ParameterResolver {
	if pr.richParametersDefaults == nil {
		pr.richParametersDefaults = make(map[string]string)
	}
	for _, p := range params {
		pr.richParametersDefaults[p.Name] = p.Value
	}
	return pr
}
func (pr *ParameterResolver) WithPromptRichParameters(promptRichParameters bool) *ParameterResolver {
	pr.promptRichParameters = promptRichParameters
	return pr
}
func (pr *ParameterResolver) WithPromptEphemeralParameters(promptEphemeralParameters bool) *ParameterResolver {
	pr.promptEphemeralParameters = promptEphemeralParameters
	return pr
}
func (pr *ParameterResolver) Resolve(inv *serpent.Invocation, action WorkspaceCLIAction, templateVersionParameters []codersdk.TemplateVersionParameter) ([]codersdk.WorkspaceBuildParameter, error) {
	var staged []codersdk.WorkspaceBuildParameter
	var err error
	staged = pr.resolveWithParametersMapFile(staged)
	staged = pr.resolveWithCommandLineOrEnv(staged)
	staged = pr.resolveWithSourceBuildParameters(staged, templateVersionParameters)
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
nextEphemeralParameter:
	for _, ephemeralParameter := range pr.ephemeralParameters {
		for i, r := range resolved {
			if r.Name == ephemeralParameter.Name {
				resolved[i].Value = ephemeralParameter.Value
				continue nextEphemeralParameter
			}
		}
		resolved = append(resolved, ephemeralParameter)
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
		if len(tvp.Options) > 0 && !isValidTemplateParameterOption(buildParameter, tvp.Options) {
			continue // do not propagate invalid options
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
func (pr *ParameterResolver) resolveWithSourceBuildParameters(resolved []codersdk.WorkspaceBuildParameter, templateVersionParameters []codersdk.TemplateVersionParameter) []codersdk.WorkspaceBuildParameter {
next:
	for _, buildParameter := range pr.sourceWorkspaceParameters {
		tvp := findTemplateVersionParameter(buildParameter, templateVersionParameters)
		if tvp == nil {
			continue // it looks like this parameter is not present anymore
		}
		if tvp.Ephemeral {
			continue // ephemeral parameters should not be passed to consecutive builds
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
			return templateVersionParametersNotFound(r.Name, templateVersionParameters)
		}
		if tvp.Ephemeral && !pr.promptEphemeralParameters && findWorkspaceBuildParameter(tvp.Name, pr.ephemeralParameters) == nil {
			return fmt.Errorf("ephemeral parameter %q can be used only with --prompt-ephemeral-parameters or --ephemeral-parameter flag", r.Name)
		}
		if !tvp.Mutable && action != WorkspaceCreate {
			return fmt.Errorf("parameter %q is immutable and cannot be updated", r.Name)
		}
	}
	return nil
}
func (pr *ParameterResolver) resolveWithInput(resolved []codersdk.WorkspaceBuildParameter, inv *serpent.Invocation, action WorkspaceCLIAction, templateVersionParameters []codersdk.TemplateVersionParameter) ([]codersdk.WorkspaceBuildParameter, error) {
	for _, tvp := range templateVersionParameters {
		p := findWorkspaceBuildParameter(tvp.Name, resolved)
		if p != nil {
			continue
		}
		// Parameter has not been resolved yet, so CLI needs to determine if user should input it.
		firstTimeUse := pr.isFirstTimeUse(tvp.Name)
		promptParameterOption := pr.isLastBuildParameterInvalidOption(tvp)
		if (tvp.Ephemeral && pr.promptEphemeralParameters) ||
			(action == WorkspaceCreate && tvp.Required) ||
			(action == WorkspaceCreate && !tvp.Ephemeral) ||
			(action == WorkspaceUpdate && promptParameterOption) ||
			(action == WorkspaceUpdate && tvp.Mutable && tvp.Required) ||
			(action == WorkspaceUpdate && !tvp.Mutable && firstTimeUse) ||
			(tvp.Mutable && !tvp.Ephemeral && pr.promptRichParameters) {
			parameterValue, err := cliui.RichParameter(inv, tvp, pr.richParametersDefaults)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, codersdk.WorkspaceBuildParameter{
				Name:  tvp.Name,
				Value: parameterValue,
			})
		} else if action == WorkspaceUpdate && !tvp.Mutable && !firstTimeUse {
			_, _ = fmt.Fprintln(inv.Stdout, pretty.Sprint(cliui.DefaultStyles.Warn, fmt.Sprintf("Parameter %q is not mutable, and cannot be customized after workspace creation.", tvp.Name)))
		}
	}
	return resolved, nil
}
func (pr *ParameterResolver) isFirstTimeUse(parameterName string) bool {
	return findWorkspaceBuildParameter(parameterName, pr.lastBuildParameters) == nil
}
func (pr *ParameterResolver) isLastBuildParameterInvalidOption(templateVersionParameter codersdk.TemplateVersionParameter) bool {
	if len(templateVersionParameter.Options) == 0 {
		return false
	}
	for _, buildParameter := range pr.lastBuildParameters {
		if buildParameter.Name == templateVersionParameter.Name {
			return !isValidTemplateParameterOption(buildParameter, templateVersionParameter.Options)
		}
	}
	return false
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
func isValidTemplateParameterOption(buildParameter codersdk.WorkspaceBuildParameter, options []codersdk.TemplateVersionParameterOption) bool {
	for _, opt := range options {
		if opt.Value == buildParameter.Value {
			return true
		}
	}
	return false
}
func templateVersionParametersNotFound(unknown string, params []codersdk.TemplateVersionParameter) error {
	var sb strings.Builder
	_, _ = sb.WriteString(fmt.Sprintf("parameter %q is not present in the template.", unknown))
	// Going with a fairly generous edit distance
	maxDist := len(unknown) / 2
	var paramNames []string
	for _, p := range params {
		paramNames = append(paramNames, p.Name)
	}
	matches := levenshtein.Matches(unknown, maxDist, paramNames...)
	if len(matches) > 0 {
		_, _ = sb.WriteString(fmt.Sprintf("\nDid you mean: %s", strings.Join(matches, ", ")))
	}
	return fmt.Errorf(sb.String())
}
