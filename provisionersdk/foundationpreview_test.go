package provisionersdk_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

func TestFoundationPlanPreview(t *testing.T) {
	t.Parallel()
	preview := provisionersdk.FoundationPlanPreview([]byte(`{
		"format_version":"1.2",
		"variables":{"password":{"value":"NEVER-EXPOSE-VARIABLE"}},
		"resource_changes":[{"change":{"before":"NEVER-EXPOSE-RESOURCE"}}],
		"output_changes":{
			"unchanged":{"actions":["no-op"],"after":"NEVER-EXPOSE-UNCHANGED"},
			"network":{"actions":["update"],"before":"NEVER-EXPOSE-BEFORE","after":"NEVER-EXPOSE-AFTER","after_unknown":false},
			"token":{"actions":["delete"],"before":"NEVER-EXPOSE-SECRET","before_sensitive":true},
			"future":{"actions":["create"],"after_unknown":{"NEVER-EXPOSE-MARKER-KEY":[false,true]},"after_sensitive":[false,{"key":true}]}
		}
	}`))
	require.True(t, preview.Available)
	require.Equal(t, []codersdk.FoundationOutputChange{
		{Name: "future", Actions: []string{"create"}, Unknown: true, Sensitive: true},
		{Name: "network", Actions: []string{"update"}},
		{Name: "token", Actions: []string{"delete"}, Sensitive: true},
	}, preview.OutputChanges)
	require.Empty(t, preview.ConsumerReferences)
	data, err := json.Marshal(preview)
	require.NoError(t, err)
	require.NotContains(t, string(data), "NEVER-EXPOSE")
	require.Contains(t, preview.Advisory, "not a compatibility or safety guarantee")
}

func TestFoundationPlanPreviewUnavailable(t *testing.T) {
	t.Parallel()
	for name, plan := range map[string]string{
		"absent":         "",
		"malformed":      `{"secret":"NEVER-EXPOSE"`,
		"not_plan":       `{"secret":"NEVER-EXPOSE"}`,
		"null":           `null`,
		"future_format":  `{"format_version":"2.0"}`,
		"invalid_name":   `{"format_version":"1.2","output_changes":{"NEVER-EXPOSE PASSWORD":{"actions":["create"]}}}`,
		"invalid_action": `{"format_version":"1.2","output_changes":{"token":{"actions":["NEVER-EXPOSE"]}}}`,
		"invalid_marker": `{"format_version":"1.2","output_changes":{"token":{"actions":["create"],"after_unknown":"NEVER-EXPOSE"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			preview := provisionersdk.FoundationPlanPreview([]byte(plan))
			require.False(t, preview.Available)
			encoded, err := json.Marshal(preview)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "NEVER-EXPOSE")
			require.NotEmpty(t, preview.Advisory)
		})
	}
}

func TestFoundationPlanPreviewNoOutputs(t *testing.T) {
	t.Parallel()
	preview := provisionersdk.FoundationPlanPreview([]byte(`{"format_version":"1.2"}`))
	require.True(t, preview.Available)
	require.Empty(t, preview.OutputChanges)
	require.NotNil(t, preview.OutputChanges)
	require.Contains(t, preview.Advisory, "No detected references")
}
