package wsbuilder

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
)

func TestBuilderDynamicProvisionerTagsDoesNotRequestSecretRequirements(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	names := []string{"region"}
	values := []string{"us-east"}

	render := &tagsPathRenderer{
		result: &dynamicparameters.RenderResult{
			Output: &preview.Output{
				WorkspaceTags: previewtypes.TagBlocks{{
					Tags: previewtypes.Tags{{
						Key:   previewtypes.StringLiteral("region"),
						Value: previewtypes.StringLiteral("us-east"),
					}},
				}},
			},
		},
	}

	builder := New(database.Workspace{
		ID:      uuid.New(),
		OwnerID: ownerID,
	}, database.WorkspaceTransitionStart, NoopUsageChecker{})
	builder.ctx = t.Context()
	builder.parameterRender = render
	builder.parameterNames = &names
	builder.parameterValues = &values
	builder.templateVersionJob = &database.ProvisionerJob{
		Tags: database.StringMap{
			provisionersdk.TagScope: provisionersdk.ScopeUser,
		},
	}

	tags, err := builder.getDynamicProvisionerTags()
	require.NoError(t, err)
	require.Equal(t, "us-east", tags["region"])
	require.Equal(t, ownerID.String(), tags[provisionersdk.TagOwner])
	require.Empty(t, render.opts, "tags path should not request secret requirements")
}

type tagsPathRenderer struct {
	result *dynamicparameters.RenderResult
	diags  hcl.Diagnostics
	opts   []dynamicparameters.RenderOption
}

func (r *tagsPathRenderer) Render(_ context.Context, _ uuid.UUID, _ map[string]string, opts ...dynamicparameters.RenderOption) (*dynamicparameters.RenderResult, hcl.Diagnostics) {
	r.opts = opts
	return r.result, r.diags
}

func (*tagsPathRenderer) Close() {}
