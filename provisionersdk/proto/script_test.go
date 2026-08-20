package proto_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	provisionerproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func TestScriptOrderProtocolRoundTrip(t *testing.T) {
	t.Parallel()

	want := &provisionerproto.GraphComplete{
		Resources: []*provisionerproto.Resource{{
			Agents: []*provisionerproto.Agent{{
				Scripts: []*provisionerproto.Script{{
					ResourceAddress: "module.development.coder_script.install",
					Dependencies: []*provisionerproto.ScriptDependency{{
						PrerequisiteResourceAddress: "module.development.coder_script.clone",
						Requirement:                 provisionerproto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_COMPLETION,
					}},
				}},
			}},
		}},
		ScriptOrderDataSourceCount: 2,
		ScriptOrderRuleCount:       3,
	}

	data, err := proto.Marshal(want)
	require.NoError(t, err)

	got := &provisionerproto.GraphComplete{}
	require.NoError(t, proto.Unmarshal(data, got))
	require.True(t, proto.Equal(want, got))
}
