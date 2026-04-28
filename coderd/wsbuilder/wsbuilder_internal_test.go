package wsbuilder

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
)

func TestBuilderDynamicProvisionerTagsIgnoresUnsatisfiedSecretRequirements(t *testing.T) {
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
			SecretRequirements: []codersdk.SecretRequirementStatus{{
				Kind:        codersdk.SecretRequirementKindEnv,
				Label:       "GITHUB_TOKEN",
				HelpMessage: "Add a GitHub PAT",
				Satisfied:   false,
			}},
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
}

type tagsPathRenderer struct {
	result *dynamicparameters.RenderResult
	diags  hcl.Diagnostics
}

func (r *tagsPathRenderer) Render(context.Context, uuid.UUID, map[string]string) (*dynamicparameters.RenderResult, hcl.Diagnostics) {
	return r.result, r.diags
}

func (*tagsPathRenderer) Close() {}
