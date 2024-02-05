package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/codersdk"
)

// workspaceParameterFlags are used by commands processing rich parameters and/or build options.
type workspaceParameterFlags struct {
	promptBuildOptions bool
	buildOptions       []string

	richParameterFile string
	richParameters    []string

	promptRichParameters bool
}

func (wpf *workspaceParameterFlags) allOptions() []clibase.Option {
	options := append(wpf.cliBuildOptions(), wpf.cliParameters()...)
	return append(options, wpf.alwaysPrompt())
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

func (wpf *workspaceParameterFlags) alwaysPrompt() clibase.Option {
	return clibase.Option{
		Flag:        "always-prompt",
		Description: "Always prompt all parameters. Does not pull parameter values from existing workspace.",
		Value:       clibase.BoolOf(&wpf.promptRichParameters),
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
