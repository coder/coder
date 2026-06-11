package policy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/policy"
)

func TestEnvelope_Builds(t *testing.T) {
	t.Parallel()

	_, err := policy.PreAuthEnvelope{
		Headers:    map[string]any{"authorization": "Bearer x"},
		Credential: map[string]any{"x_api_key": "k"},
	}.Build()
	require.NoError(t, err)

	_, err = policy.PreReqEnvelope{Request: []byte(`{"model":"gpt-4o"}`)}.Build()
	require.NoError(t, err)

	_, err = policy.PreToolEnvelope{
		PreReqEnvelope: policy.PreReqEnvelope{Request: []byte(`{}`)},
		ToolCall:       policy.ToolCall{Name: "bash"},
	}.Build()
	require.NoError(t, err)
}
