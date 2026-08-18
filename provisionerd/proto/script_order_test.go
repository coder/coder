package proto_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	provisionerdproto "github.com/coder/coder/v2/provisionerd/proto"
)

func TestScriptOrderUsageProtocolRoundTrip(t *testing.T) {
	t.Parallel()

	want := &provisionerdproto.CompletedJob{
		Type: &provisionerdproto.CompletedJob_TemplateImport_{
			TemplateImport: &provisionerdproto.CompletedJob_TemplateImport{
				ScriptOrderDataSourceCount: 2,
				ScriptOrderRuleCount:       3,
			},
		},
	}

	data, err := proto.Marshal(want)
	require.NoError(t, err)

	got := &provisionerdproto.CompletedJob{}
	require.NoError(t, proto.Unmarshal(data, got))
	require.True(t, proto.Equal(want, got))
}
