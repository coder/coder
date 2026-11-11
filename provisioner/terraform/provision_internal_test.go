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

	// Grow the response until the bare GraphComplete still fits the limit
	// but the proto.Response envelope the session sends does not. The
	// guard must measure the envelope, or this case reaches the transport
	// and drops the connection.
	envelope := &proto.Response{Type: &proto.Response_Graph{Graph: response}}
	dependency := &proto.ScriptDependency{
		PrerequisiteResourceAddress: "module.setup." + strings.Repeat("x", 90) + ".coder_script.step",
		Requirement:                 proto.ScriptDependencyRequirement_SCRIPT_DEPENDENCY_REQUIREMENT_SUCCESS,
	}
	for protobuf.Size(envelope) <= drpcsdk.MaxMessageSize {
		script.Dependencies = append(script.Dependencies, dependency)
	}
	// Drop the entry that crossed the limit, then pad the last remaining
	// one a byte at a time so the envelope crosses by the smallest margin.
	last := protobuf.Clone(dependency).(*proto.ScriptDependency)
	script.Dependencies[len(script.Dependencies)-2] = last
	script.Dependencies = script.Dependencies[:len(script.Dependencies)-1]
	for protobuf.Size(envelope) <= drpcsdk.MaxMessageSize {
		last.PrerequisiteResourceAddress += "x"
	}
	require.LessOrEqual(t, protobuf.Size(response), drpcsdk.MaxMessageSize)
	require.Greater(t, protobuf.Size(envelope), drpcsdk.MaxMessageSize)

	err := checkGraphCompleteSize(response)
	require.ErrorContains(t, err, strconv.Itoa(protobuf.Size(envelope))+" bytes")
	require.ErrorContains(t, err, strconv.Itoa(drpcsdk.MaxMessageSize)+" byte message limit")
}
