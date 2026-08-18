package proto_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	agentproto "github.com/coder/coder/v2/agent/proto"
)

func TestScriptOrderManifestRoundTrip(t *testing.T) {
	t.Parallel()

	want := &agentproto.Manifest{
		Scripts: []*agentproto.WorkspaceAgentScript{{
			ResourceAddress: "module.development.coder_script.install",
			Dependencies: []*agentproto.WorkspaceAgentScriptDependency{{
				PrerequisiteResourceAddress: "module.development.coder_script.clone",
				Requirement:                 agentproto.WorkspaceAgentScriptDependency_REQUIREMENT_COMPLETION,
			}},
		}},
	}

	data, err := proto.Marshal(want)
	require.NoError(t, err)

	got := &agentproto.Manifest{}
	require.NoError(t, proto.Unmarshal(data, got))
	require.True(t, proto.Equal(want, got))
}
