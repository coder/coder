package dynamicparameters

import (
	"context"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"

	"github.com/coder/coder/v2/provisioner/sandbox"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
)

// Sandbox templates contain only fixed values and need no Terraform metadata.
type sandboxRenderer struct{}

func (*sandboxRenderer) Render(_ context.Context, _ uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	if len(values) > 0 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Sandbox templates do not support parameter values.",
		})
	}
	tags := make(previewtypes.Tags, 0)
	for key, value := range (sandbox.Manifest{}).Tags() {
		tags = append(tags, previewtypes.Tag{
			Key:   previewtypes.StringLiteral(key),
			Value: previewtypes.StringLiteral(value),
		})
	}
	return &preview.Output{
		Parameters:    []previewtypes.Parameter{},
		WorkspaceTags: previewtypes.TagBlocks{{Tags: tags}},
	}, diags
}

func (*sandboxRenderer) Close() {}
