package coderd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestFoundationPublicOutputs(t *testing.T) {
	t.Parallel()
	var result codersdk.Foundation
	err := foundationPublicOutputs(json.RawMessage(`{
		"values":{"credential":"private-marker","number":9007199254740993,"nested":{"ratio":0.1234567890123456789}},
		"metadata":{"credential":{"type":"string","sensitive":true},"number":{"type":"number","sensitive":false}},
		"snapshot_id":"apply-run"
	}`), &result)
	require.NoError(t, err)
	require.NotContains(t, result.Outputs, "credential")
	require.True(t, result.OutputMetadata["credential"].Sensitive)
	require.Equal(t, "apply-run", result.OutputSnapshotID)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private-marker")
	require.Contains(t, string(encoded), "9007199254740993")
	require.Contains(t, string(encoded), "0.1234567890123456789")
}
