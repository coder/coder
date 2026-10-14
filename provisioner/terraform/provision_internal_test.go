package terraform

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	protobuf "google.golang.org/protobuf/proto"

	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func TestCheckGraphCompleteSize(t *testing.T) {
	t.Parallel()

	script := &proto.Script{ResourceAddress: "coder_script.build"}
	response := &proto.GraphComplete{
		Resources: []*proto.Resource{{
			Agents: []*proto.Agent{{Scripts: []*proto.Script{script}}},
		}},
	}
	require.NoError(t, checkGraphCompleteSize(response))

	dependency := &proto.ScriptDependency{
		PrerequisiteResourceAddress: "module.setup." + strings.Repeat("x", 90) + ".coder_script.step",
		Requirement:                 proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS,
	}
	count := drpcsdk.MaxMessageSize/protobuf.Size(dependency) + 1
	for range count {
		script.Dependencies = append(script.Dependencies, dependency)
	}
	require.Greater(t, protobuf.Size(response), drpcsdk.MaxMessageSize)

	err := checkGraphCompleteSize(response)
	require.ErrorContains(t, err, strconv.Itoa(protobuf.Size(response))+" bytes")
	require.ErrorContains(t, err, strconv.Itoa(drpcsdk.MaxMessageSize)+" byte message limit")
	require.ErrorContains(t, err, "coder_script_order")
}
