package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

// workspaceParameterFlags are used by commands processing rich parameters and/or build options.
type workspaceParameterFlags struct {
	promptBuildOptions bool
	buildOptions       []string

	richParameterFile string
	richParameters    []string
}

func (wpf *workspaceParameterFlags) cliBuildOptions() []clibase.Option {
	return clibase.OptionSet{
		{
			Flag:        "build-option",
			Env:         "CODER_BUILD_OPTION",
			Description: `Build option value in the format "name=value".`,
			Value:       clibase.StringArrayOf(&wpf.buildOptions),
		},
		{
			Flag:        "build-options",
			Description: "Prompt for one-time build options defined with ephemeral parameters.",
			Value:       clibase.BoolOf(&wpf.promptBuildOptions),
		},
	}
}

func (wpf *workspaceParameterFlags) cliParameters() []clibase.Option {
	return clibase.OptionSet{
		clibase.Option{
			Flag:        "parameter",
			Env:         "CODER_RICH_PARAMETER",
			Description: `Rich parameter value in the format "name=value".`,
			Value:       clibase.StringArrayOf(&wpf.richParameters),
		},
		clibase.Option{
			Flag:        "rich-parameter-file",
			Env:         "CODER_RICH_PARAMETER_FILE",
			Description: "Specify a file path with values for rich parameters defined in the template.",
			Value:       clibase.StringOf(&wpf.richParameterFile),
		},
	}
}

func asWorkspaceBuildParameters(nameValuePairs []string) ([]codersdk.WorkspaceBuildParameter, error) {
	var params []codersdk.WorkspaceBuildParameter
	for _, nameValue := range nameValuePairs {
		split := strings.SplitN(nameValue, "=", 2)
		if len(split) < 2 {
			return nil, xerrors.Errorf("format key=value expected, but got %s", nameValue)
		}
		params = append(params, codersdk.WorkspaceBuildParameter{
			Name:  split[0],
			Value: split[1],
		})
	}
	return params, nil
}

func parseParameterMapFile(parameterFile string) (map[string]string, error) {
	parameterFileContents, err := os.ReadFile(parameterFile)
	if err != nil {
		return nil, err
	}

	mapStringInterface := make(map[string]interface{})
	err = yaml.Unmarshal(parameterFileContents, &mapStringInterface)
	if err != nil {
		return nil, err
	}

	parameterMap := map[string]string{}
	for k, v := range mapStringInterface {
		switch val := v.(type) {
		case string, bool, int:
			parameterMap[k] = fmt.Sprintf("%v", val)
		case []interface{}:
			b, err := json.Marshal(&val)
			if err != nil {
				return nil, err
			}
			parameterMap[k] = string(b)
		default:
			return nil, xerrors.Errorf("invalid parameter type: %T", v)
		}
	}
	return parameterMap, nil
}

type getParameterArgs struct {
	templateVersionParameter codersdk.TemplateVersionParameter

	promptBuildOptions bool
	buildOptions       []codersdk.WorkspaceBuildParameter
	parameterMap       map[string]string
}

func getParameter(inv *clibase.Invocation, args getParameterArgs) (codersdk.WorkspaceBuildParameter, error) {
	if args.parameterMap != nil {
		if parameterValue, ok := args.parameterMap[args.templateVersionParameter.Name]; ok {
			return codersdk.WorkspaceBuildParameter{
				Name:  args.templateVersionParameter.Name,
				Value: parameterValue,
			}, nil
		}
	}
	return getParameterFromCommandLineOrInput(inv, args.promptBuildOptions, args.buildOptions, args.templateVersionParameter)
}

//nolint:revive
func getParameterFromCommandLineOrInput(inv *clibase.Invocation, promptBuildOptions bool, buildOptions []codersdk.WorkspaceBuildParameter, templateVersionParameter codersdk.TemplateVersionParameter) (codersdk.WorkspaceBuildParameter, error) {
	if templateVersionParameter.Ephemeral {
		for _, bo := range buildOptions {
			if bo.Name == templateVersionParameter.Name {
				return codersdk.WorkspaceBuildParameter{
					Name:  templateVersionParameter.Name,
					Value: bo.Value,
				}, nil
			}
		}
	}

	parameterValue, err := cliui.RichParameter(inv, templateVersionParameter)
	if err != nil {
		return codersdk.WorkspaceBuildParameter{}, err
	}
	return codersdk.WorkspaceBuildParameter{
		Name:  templateVersionParameter.Name,
		Value: parameterValue,
	}, nil
}
