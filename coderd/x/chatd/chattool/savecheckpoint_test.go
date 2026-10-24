package chattool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

func TestSaveCheckpoint(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, checkpoint string) fantasy.ToolResponse {
		t.Helper()
		input, err := json.Marshal(map[string]string{"checkpoint": checkpoint})
		require.NoError(t, err)
		resp, err := chattool.SaveCheckpoint().Run(context.Background(), fantasy.ToolCall{Input: string(input)})
		require.NoError(t, err)
		return resp
	}

	t.Run("Saves", func(t *testing.T) {
		t.Parallel()
		resp := run(t, "goal: ship it")
		require.False(t, resp.IsError)
		require.JSONEq(t, `{"saved":true,"bytes":13}`, resp.Content)
	})
	t.Run("AcceptsLimit", func(t *testing.T) {
		t.Parallel()
		resp := run(t, strings.Repeat("a", chattool.MaxCheckpointBytes))
		require.False(t, resp.IsError)
	})
	t.Run("RejectsOverLimit", func(t *testing.T) {
		t.Parallel()
		resp := run(t, strings.Repeat("a", chattool.MaxCheckpointBytes+1))
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "8192 bytes")
	})
	t.Run("RejectsBlank", func(t *testing.T) {
		t.Parallel()
		resp := run(t, " \n\t")
		require.True(t, resp.IsError)
	})
}
