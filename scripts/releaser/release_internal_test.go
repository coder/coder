package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTagMessage(t *testing.T) {
	t.Parallel()

	v := version{Major: 2, Minor: 38, Patch: 4}
	require.Equal(t, "Release v2.38.4", tagMessage(v, releaseOptions{}))
	require.Equal(t,
		"Release v2.38.4\n\nRequested-by: @alice\nRequested-via: https://github.com/coder/coder/actions/runs/1",
		tagMessage(v, releaseOptions{RequestedBy: "alice", RequestedByRunURL: "https://github.com/coder/coder/actions/runs/1"}),
	)
}
