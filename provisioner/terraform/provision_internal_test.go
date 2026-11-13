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
	// Binary search for the smallest dependency count whose envelope
	// exceeds the limit. protobuf.Size is O(n), so growing one entry at a
	// time is quadratic and takes minutes under the race detector.
	all := make([]*proto.ScriptDependency, drpcsdk.MaxMessageSize/protobuf.Size(dependency)+1)
	for i := range all {
		all[i] = dependency
	}
	sizeWith := func(n int) int {
		script.Dependencies = all[:n]
		return protobuf.Size(envelope)
	}
	lo, hi := 0, len(all)
	require.Greater(t, sizeWith(hi), drpcsdk.MaxMessageSize)
	for lo < hi {
		mid := (lo + hi) / 2
		if sizeWith(mid) > drpcsdk.MaxMessageSize {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	// Drop the entry that crossed the limit, then pad the last remaining
	// one so the envelope crosses by the smallest margin. Jump most of the
	// way in one step, then finish a byte at a time.
	last := protobuf.Clone(dependency).(*proto.ScriptDependency)
	script.Dependencies = append([]*proto.ScriptDependency(nil), all[:lo-1]...)
	script.Dependencies[len(script.Dependencies)-1] = last
	if gap := drpcsdk.MaxMessageSize - protobuf.Size(envelope) - 8; gap > 0 {
		last.PrerequisiteResourceAddress += strings.Repeat("x", gap)
	}
	for protobuf.Size(envelope) <= drpcsdk.MaxMessageSize {
		last.PrerequisiteResourceAddress += "x"
	}
	require.LessOrEqual(t, protobuf.Size(response), drpcsdk.MaxMessageSize)
	require.Greater(t, protobuf.Size(envelope), drpcsdk.MaxMessageSize)

	err := checkGraphCompleteSize(response)
	require.ErrorContains(t, err, strconv.Itoa(protobuf.Size(envelope))+" bytes")
	require.ErrorContains(t, err, strconv.Itoa(drpcsdk.MaxMessageSize)+" byte message limit")
}
