package chattool_test

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

func TestNewContext(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, opts chattool.NewContextOptions) map[string]any {
		t.Helper()
		resp, err := chattool.NewContext(opts).Run(context.Background(), fantasy.ToolCall{Input: `{}`})
		require.NoError(t, err)
		// Both outcomes are plain results: the refusal must stay structured
		// rather than be flattened into an error string.
		require.False(t, resp.IsError)
		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		return result
	}

	t.Run("Schedules", func(t *testing.T) {
		t.Parallel()
		result := run(t, chattool.NewContextOptions{})
		require.Equal(t, "scheduled", result["status"])
		require.Equal(t, "A new context window will start without summarizing conversation history.", result["message"])
	})
	t.Run("RefusesWhenRolloverDisabled", func(t *testing.T) {
		t.Parallel()
		result := run(t, chattool.NewContextOptions{RolloverDisabled: true})
		require.Equal(t, "rollover_disabled", result["code"])
		require.NotContains(t, result, "status")
	})
}
