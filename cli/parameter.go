package cli
import (
	"errors"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"gopkg.in/yaml.v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)
// workspaceParameterFlags are used by commands processing rich parameters and/or build options.
type workspaceParameterFlags struct {
	promptEphemeralParameters bool
	ephemeralParameters []string
	richParameterFile     string
	richParameters        []string
	richParameterDefaults []string
	promptRichParameters bool
}
func (wpf *workspaceParameterFlags) allOptions() []serpent.Option {
	options := append(wpf.cliEphemeralParameters(), wpf.cliParameters()...)
	options = append(options, wpf.cliParameterDefaults()...)
	return append(options, wpf.alwaysPrompt())
}
func (wpf *workspaceParameterFlags) cliEphemeralParameters() []serpent.Option {
	return serpent.OptionSet{
		// Deprecated - replaced with ephemeral-parameter
		{
			Flag:        "build-option",
			Env:         "CODER_BUILD_OPTION",
			Description: `Build option value in the format "name=value".`,
			UseInstead:  []serpent.Option{{Flag: "ephemeral-parameter"}},
			Value:       serpent.StringArrayOf(&wpf.ephemeralParameters),
		},
		// Deprecated - replaced with prompt-ephemeral-parameters
		{
			Flag:        "build-options",
			Description: "Prompt for one-time build options defined with ephemeral parameters.",
			UseInstead:  []serpent.Option{{Flag: "prompt-ephemeral-parameters"}},
			Value:       serpent.BoolOf(&wpf.promptEphemeralParameters),
		},
		{
			Flag:        "ephemeral-parameter",
			Env:         "CODER_EPHEMERAL_PARAMETER",
			Description: `Set the value of ephemeral parameters defined in the template. The format is "name=value".`,
			Value:       serpent.StringArrayOf(&wpf.ephemeralParameters),
		},
		{
			Flag:        "prompt-ephemeral-parameters",
			Env:         "CODER_PROMPT_EPHEMERAL_PARAMETERS",
			Description: "Prompt to set values of ephemeral parameters defined in the template. If a value has been set via --ephemeral-parameter, it will not be prompted for.",
			Value:       serpent.BoolOf(&wpf.promptEphemeralParameters),
		},
	}
}
func (wpf *workspaceParameterFlags) cliParameters() []serpent.Option {
	return serpent.OptionSet{
		serpent.Option{
			Flag:        "parameter",
			Env:         "CODER_RICH_PARAMETER",
			Description: `Rich parameter value in the format "name=value".`,
			Value:       serpent.StringArrayOf(&wpf.richParameters),
		},
		serpent.Option{
			Flag:        "rich-parameter-file",
			Env:         "CODER_RICH_PARAMETER_FILE",
			Description: "Specify a file path with values for rich parameters defined in the template. The file should be in YAML format, containing key-value pairs for the parameters.",
			Value:       serpent.StringOf(&wpf.richParameterFile),
		},
	}
}
func (wpf *workspaceParameterFlags) cliParameterDefaults() []serpent.Option {
	return serpent.OptionSet{
		serpent.Option{
			Flag:        "parameter-default",
			Env:         "CODER_RICH_PARAMETER_DEFAULT",
			Description: `Rich parameter default values in the format "name=value".`,
			Value:       serpent.StringArrayOf(&wpf.richParameterDefaults),
		},
	}
}
func (wpf *workspaceParameterFlags) alwaysPrompt() serpent.Option {
	return serpent.Option{
		Flag:        "always-prompt",
		Description: "Always prompt all parameters. Does not pull parameter values from existing workspace.",
		Value:       serpent.BoolOf(&wpf.promptRichParameters),
	}
}
func asWorkspaceBuildParameters(nameValuePairs []string) ([]codersdk.WorkspaceBuildParameter, error) {
	var params []codersdk.WorkspaceBuildParameter
	for _, nameValue := range nameValuePairs {
		split := strings.SplitN(nameValue, "=", 2)
		if len(split) < 2 {
			return nil, fmt.Errorf("format key=value expected, but got %s", nameValue)
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
			return nil, fmt.Errorf("invalid parameter type: %T", v)
		}
	}
	return parameterMap, nil
}
// buildFlags contains options relating to troubleshooting provisioner jobs.
type buildFlags struct {
	provisionerLogDebug bool
}
func (bf *buildFlags) cliOptions() []serpent.Option {
	return []serpent.Option{
		{
			Flag: "provisioner-log-debug",
			Description: `Sets the provisioner log level to debug.
This will print additional information about the build process.
This is useful for troubleshooting build issues.`,
			Value:  serpent.BoolOf(&bf.provisionerLogDebug),
			Hidden: true,
		},
	}
}
