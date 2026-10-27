package coderd

import (
	"net/http"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
)

// validateTemplateResourceSource keeps state in Coder rather than an external backend.
func validateTemplateResourceSource(source string) error {
	return validateTemplateResourceFile([]byte(source), "main.tf")
}

func validateTemplateResourceFile(source []byte, name string) error {
	parser := hclparse.NewParser()
	var file *hcl.File
	var diagnostics hcl.Diagnostics
	if strings.HasSuffix(name, ".tf.json") {
		file, diagnostics = parser.ParseJSON(source, name)
	} else {
		file, diagnostics = parser.ParseHCL(source, name)
	}
	if diagnostics.HasErrors() {
		return &templateResourceHTTPError{http.StatusBadRequest, "Invalid Terraform configuration: " + diagnostics.Error()}
	}
	root, _, diagnostics := file.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "terraform"}}})
	if diagnostics.HasErrors() {
		return &templateResourceHTTPError{http.StatusBadRequest, "Invalid Terraform configuration: " + diagnostics.Error()}
	}
	for _, block := range root.Blocks {
		settings, _, diagnostics := block.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
			{Type: "backend", LabelNames: []string{"type"}},
			{Type: "cloud"},
		}})
		if diagnostics.HasErrors() {
			return &templateResourceHTTPError{http.StatusBadRequest, "Invalid Terraform configuration: " + diagnostics.Error()}
		}
		if len(settings.Blocks) != 0 {
			return &templateResourceHTTPError{http.StatusBadRequest, "Shared resource state is managed by Coder. Terraform backend and cloud blocks are not supported."}
		}
	}
	return nil
}
