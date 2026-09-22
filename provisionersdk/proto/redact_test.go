package proto_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestRedactRichParameterValues(t *testing.T) {
	t.Parallel()

	input := []*proto.RichParameterValue{
		{Name: "region", Value: "us-east-1"},
		{Name: "api_key", Value: "hunter2", Sensitive: true},
	}
	got := proto.RedactRichParameterValues(input)

	require.Len(t, got, 2)
	require.Equal(t, "us-east-1", got[0].Value)
	require.Equal(t, proto.RedactedValue, got[1].Value)
	require.True(t, got[1].Sensitive)
	// The input must not be mutated, since the caller still needs the real
	// value to run the job.
	require.Equal(t, "hunter2", input[1].Value)
}

func TestRedactVariableValues(t *testing.T) {
	t.Parallel()

	input := []*proto.VariableValue{
		{Name: "image", Value: "ubuntu"},
		{Name: "token", Value: "hunter2", Sensitive: true},
	}
	got := proto.RedactVariableValues(input)

	require.Len(t, got, 2)
	require.Equal(t, "ubuntu", got[0].Value)
	require.Equal(t, proto.RedactedValue, got[1].Value)
	require.Equal(t, "hunter2", input[1].Value)
}
