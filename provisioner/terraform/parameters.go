package terraform

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
)

var terraformWithCoderParametersSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "data",
			LabelNames: []string{"coder_parameter", "*"},
		},
	},
}

func rawRichParameterNames(workdir string) ([]string, error) {
	entries, err := os.ReadDir(workdir)
	if err != nil {
		return nil, err
	}

	var coderParameterNames []string
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".tf") || !strings.HasSuffix(entry.Name(), ".tf.json") {
			continue
		}

		var parsedHCL *hcl.File
		var diags hcl.Diagnostics
		hclFilepath := path.Join(workdir, entry.Name())
		parser := hclparse.NewParser()
		if strings.HasSuffix(entry.Name(), ".tf.json") {
			parsedHCL, diags = parser.ParseJSONFile(hclFilepath)
		} else {
			parsedHCL, diags = parser.ParseHCLFile(hclFilepath)
		}
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
	return coderParameterNames, nil
}
