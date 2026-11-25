package proto_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	provisionerproto "github.com/coder/coder/v2/provisionersdk/proto"
)

func TestScriptDependenciesRoundTrip(t *testing.T) {
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
				Devcontainers: []*provisionerproto.Devcontainer{{
					Scripts: []*provisionerproto.Script{{
						ResourceAddress: "coder_script.b",
						Dependencies: []*provisionerproto.ScriptDependency{{
							PrerequisiteResourceAddress: "coder_script.a",
							Requirement:                 provisionerproto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS,
						}},
					}},
				}},
			}},
		}},
	}

	data, err := proto.Marshal(want)
	require.NoError(t, err)

	got := &provisionerproto.GraphComplete{}
	require.NoError(t, proto.Unmarshal(data, got))
	require.True(t, proto.Equal(want, got))
}

// A script with neither new field set must encode to the same bytes it did
// before fields 10 and 11 existed. Walking the tags instead of comparing
// against a recorded byte string keeps the test stable when unrelated
// fields are added later.
func TestScriptWithoutNewFieldsEncodesAsBefore(t *testing.T) {
	t.Parallel()

	script := &provisionerproto.Script{
		DisplayName:    "Clone repo",
		Script:         "git clone ...",
		RunOnStart:     true,
		TimeoutSeconds: 120,
	}
	data, err := proto.Marshal(script)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		require.Positive(t, n)
		require.LessOrEqual(t, int(num), 9, "field %d must not be encoded when it is not set", num)
		data = data[n:]

		n = protowire.ConsumeFieldValue(num, typ, data)
		require.Positive(t, n)
		data = data[n:]
	}
}
