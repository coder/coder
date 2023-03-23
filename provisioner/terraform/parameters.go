package terraform

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"

	"github.com/coder/coder/provisionersdk/proto"
)

var terraformWithCoderParametersSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "data",
			LabelNames: []string{"coder_parameter", "*"},
		},
	},
}

func orderResources(workdir string, state State) (*State, error) {
	entries, err := os.ReadDir(workdir)
	if err != nil {
		return nil, err
	}

	var coderParameterNames []string
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}
		hclFilepath := path.Join(workdir, entry.Name())

		parser := hclparse.NewParser()
		parsedHCL, diags := parser.ParseHCLFile(hclFilepath)
		if diags.HasErrors() {
			return nil, hcl.Diagnostics{
				{
					Severity: hcl.DiagError,
					Summary:  "Failed to parse HCL file",
					Detail:   fmt.Sprintf("parser.ParseHCLFile can't parse %q file", hclFilepath),
				},
			}
		}

		content, _, _ := parsedHCL.Body.PartialContent(terraformWithCoderParametersSchema)
		for _, block := range content.Blocks {
			if block.Type == "data" && block.Labels[0] == "coder_parameter" && len(block.Labels) == 2 {
				coderParameterNames = append(coderParameterNames, block.Labels[1])
			}
		}
	}

	if len(coderParameterNames) != len(state.Parameters) {
		return &state, nil // Return the original state, parameters will be order alphabetically.
	}

	var orderedParameters []*proto.RichParameter
	for _, coderParameterName := range coderParameterNames {
		for _, p := range orderedParameters {
			if p.Name != coderParameterName {
				continue
			}
			orderedParameters = append(orderedParameters, p)
		}
	}

	if len(orderedParameters) != len(state.Parameters) {
		return &state, nil // Return the original state, most likely a parameter was placed twice.
	}

	state.Parameters = orderedParameters
	return &state, nil
}
