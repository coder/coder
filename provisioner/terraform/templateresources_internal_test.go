package terraform

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestProvisionEnvTemplateOutputs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, outputs string
	}{
		{name: "Default", outputs: ""},
		{name: "Nested", outputs: `{"object":{"list":[1,true,null,"a=b\nquoted\""],"integer":9007199254740993}}`},
		{name: "MaximumSnapshot", outputs: `{"value":"` + strings.Repeat("x", 64*1024-12) + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			metadata := &proto.Metadata{
				TemplateId: uuid.NewString(), TemplateName: "shared-infrastructure",
				TemplateOutputs: tc.outputs, TemplateResourceRevisionId: uuid.NewString(),
			}
			env, err := provisionEnv(&proto.Config{}, metadata, nil, nil, nil)
			require.NoError(t, err)
			wantOutputs := tc.outputs
			if wantOutputs == "" {
				wantOutputs = "{}"
			}
			require.Equal(t, wantOutputs, safeEnvironValue(env, "CODER_TEMPLATE_OUTPUTS"))
			require.Equal(t, metadata.TemplateResourceRevisionId, safeEnvironValue(env, "CODER_TEMPLATE_RESOURCE_REVISION_ID"))
			require.Equal(t, metadata.TemplateId, safeEnvironValue(env, "CODER_WORKSPACE_TEMPLATE_ID"))
			require.Equal(t, metadata.TemplateName, safeEnvironValue(env, "CODER_WORKSPACE_TEMPLATE_NAME"))
			require.Empty(t, safeEnvironValue(env, "CODER_WORKSPACE_OWNER_SESSION_TOKEN"))
			require.Empty(t, safeEnvironValue(env, "CODER_WORKSPACE_OWNER_OIDC_ACCESS_TOKEN"))
			require.False(t, tfEnvSafeToPrint["CODER_TEMPLATE_OUTPUTS"])
		})
	}
}
