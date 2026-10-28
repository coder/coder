package terraform

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestProvisionEnvFoundationDefaults(t *testing.T) {
	// Environment-backed checks cannot run in parallel with t.Setenv.
	for _, key := range []string{
		"CODER_FOUNDATION_ID", "CODER_FOUNDATION_NAME", "CODER_FOUNDATION_TEMPLATE_IDS",
		"CODER_FOUNDATION_OUTPUT_METADATA", "CODER_FOUNDATION_SNAPSHOT_ID", "CODER_TEMPLATE_OUTPUTS",
		"CODER_WORKSPACE_OWNER_SESSION_TOKEN", "CODER_WORKSPACE_OWNER_OIDC_ACCESS_TOKEN",
		"CODER_WORKSPACE_OWNER_SSH_PRIVATE_KEY", "CODER_SESSION_TOKEN",
	} {
		t.Setenv(key, "private-marker")
	}
	for _, metadata := range []*proto.Metadata{
		{},
		{FoundationId: uuid.NewString(), FoundationName: "unbound"},
	} {
		env, err := provisionEnv(&proto.Config{}, metadata, nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, metadata.FoundationId, safeEnvironValue(env, "CODER_FOUNDATION_ID"))
		require.Equal(t, metadata.FoundationName, safeEnvironValue(env, "CODER_FOUNDATION_NAME"))
		require.Equal(t, "[]", safeEnvironValue(env, "CODER_FOUNDATION_TEMPLATE_IDS"))
		require.Empty(t, safeEnvironValue(env, "CODER_WORKSPACE_TEMPLATE_ID"))
		for _, value := range env {
			require.NotContains(t, value, "private-marker")
		}
	}
}

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
				TemplateId: uuid.NewString(), TemplateName: "workspace-template",
				FoundationId: uuid.NewString(), FoundationName: "shared-infrastructure",
				TemplateOutputs: tc.outputs, TemplateResourceRevisionId: uuid.NewString(),
				FoundationOutputMetadata: `{"secret":{"type":"string","sensitive":true}}`, FoundationSnapshotId: uuid.NewString(),
			}
			metadata.FoundationTemplateIds = []string{metadata.TemplateId}
			env, err := provisionEnv(&proto.Config{}, metadata, nil, nil, nil)
			require.NoError(t, err)
			wantOutputs := tc.outputs
			if wantOutputs == "" {
				wantOutputs = "{}"
			}
			require.Equal(t, wantOutputs, safeEnvironValue(env, "CODER_TEMPLATE_OUTPUTS"))
			require.Equal(t, metadata.TemplateResourceRevisionId, safeEnvironValue(env, "CODER_TEMPLATE_RESOURCE_REVISION_ID"))
			require.Equal(t, metadata.FoundationOutputMetadata, safeEnvironValue(env, "CODER_FOUNDATION_OUTPUT_METADATA"))
			require.Equal(t, metadata.FoundationSnapshotId, safeEnvironValue(env, "CODER_FOUNDATION_SNAPSHOT_ID"))
			require.False(t, tfEnvSafeToPrint["CODER_FOUNDATION_OUTPUT_METADATA"])
			require.Equal(t, metadata.TemplateId, safeEnvironValue(env, "CODER_WORKSPACE_TEMPLATE_ID"))
			require.Equal(t, metadata.TemplateName, safeEnvironValue(env, "CODER_WORKSPACE_TEMPLATE_NAME"))
			require.Equal(t, metadata.FoundationId, safeEnvironValue(env, "CODER_FOUNDATION_ID"))
			require.NotEqual(t, metadata.TemplateId, safeEnvironValue(env, "CODER_FOUNDATION_ID"))
			require.Equal(t, metadata.FoundationName, safeEnvironValue(env, "CODER_FOUNDATION_NAME"))
			require.JSONEq(t, `["`+metadata.TemplateId+`"]`, safeEnvironValue(env, "CODER_FOUNDATION_TEMPLATE_IDS"))
			require.Empty(t, safeEnvironValue(env, "CODER_WORKSPACE_OWNER_SSH_PRIVATE_KEY"))
			require.Empty(t, safeEnvironValue(env, "CODER_WORKSPACE_OWNER_SESSION_TOKEN"))
			require.Empty(t, safeEnvironValue(env, "CODER_WORKSPACE_OWNER_OIDC_ACCESS_TOKEN"))
			require.False(t, tfEnvSafeToPrint["CODER_TEMPLATE_OUTPUTS"])
		})
	}
}
