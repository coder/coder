package proto_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	agentproto "github.com/coder/coder/v2/agent/proto"
)

func TestScriptDependenciesRoundTrip(t *testing.T) {
	t.Parallel()

	id, logSourceID := uuid.New(), uuid.New()
	want := &agentproto.Manifest{
		Scripts: []*agentproto.WorkspaceAgentScript{{
			Id:              id[:],
			LogSourceId:     logSourceID[:],
			DisplayName:     "Install tools",
			RunOnStart:      true,
			ResourceAddress: "coder_script.install_tools",
			Dependencies: []*agentproto.WorkspaceAgentScriptDependency{
				{
					PrerequisiteResourceAddress: "coder_script.clone_repo",
					Requirement:                 agentproto.WorkspaceAgentScriptDependency_REQUIREMENT_SUCCESS,
				},
				{
					PrerequisiteResourceAddress: "coder_script.warm_cache",
					Requirement:                 agentproto.WorkspaceAgentScriptDependency_REQUIREMENT_COMPLETION,
				},
			},
		}},
	}

	data, err := proto.Marshal(want)
	require.NoError(t, err)

	got := &agentproto.Manifest{}
	require.NoError(t, proto.Unmarshal(data, got))
	require.True(t, proto.Equal(want, got))
}

// A script with neither new field set must encode to the same bytes it did
// before fields 11 and 12 existed, so an older agent sees nothing new.
// Walking the tags instead of comparing against a recorded byte string
// keeps the test stable when unrelated fields are added later.
func TestUnorderedScriptEncodesWithoutNewFields(t *testing.T) {
	t.Parallel()

	id, logSourceID := uuid.New(), uuid.New()
	script := &agentproto.WorkspaceAgentScript{
		Id:          id[:],
		LogSourceId: logSourceID[:],
		Script:      "git clone ...",
		RunOnStart:  true,
		Timeout:     durationpb.New(2 * time.Minute),
		DisplayName: "Clone repo",
	}
	data, err := proto.Marshal(script)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		require.Positive(t, n)
		require.LessOrEqual(t, int(num), 10, "field %d must not be encoded when it is not set", num)
		data = data[n:]

		n = protowire.ConsumeFieldValue(num, typ, data)
		require.Positive(t, n)
		data = data[n:]
	}
}
