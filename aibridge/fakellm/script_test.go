package fakellm_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/fakellm"
)

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("groups turns at tool-call boundaries", func(t *testing.T) {
		t.Parallel()
		script := fakellm.MustParseString(`
			{"text": "let me check that"}
			{"think": "I need to check if the file exists. I'll use the execute tool"}
			{"tool_call": {"name": "execute", "args": {"command": "ls -l"}, "result": {"success": false, "output": "no such file or directory", "exit_code": 2}}}
			{"text": "nope it's not there. should I create it?"}
			{"tool_call": {"name": "user_choice", "args": {"options": ["yes", "no"]}, "result": {"choice": "yes"}}}
		`)

		require.Len(t, script.Turns, 2)

		turn1 := script.Turns[0]
		require.Equal(t, "let me check that", turn1.Text())
		require.True(t, turn1.FinishedToolCalls())
		require.Len(t, turn1.ToolCalls, 1)
		require.Equal(t, "execute", turn1.ToolCalls[0].Name)

		turn2 := script.Turns[1]
		require.Equal(t, "nope it's not there. should I create it?", turn2.Text())
		require.Len(t, turn2.ToolCalls, 1)
		require.Equal(t, "user_choice", turn2.ToolCalls[0].Name)
	})

	t.Run("requires tool_call result", func(t *testing.T) {
		t.Parallel()
		_, err := fakellm.ParseString(`{"tool_call": {"name": "execute", "args": {"command": "ls"}}}`)
		require.ErrorContains(t, err, "has no result")
	})

	t.Run("rejects a step with more than one field set", func(t *testing.T) {
		t.Parallel()
		_, err := fakellm.ParseString(`{"text": "hi", "think": "hmm"}`)
		require.ErrorContains(t, err, "exactly one of")
	})

	t.Run("rejects empty scripts", func(t *testing.T) {
		t.Parallel()
		_, err := fakellm.ParseString(``)
		require.ErrorContains(t, err, "no turns")
	})

	t.Run("error step is its own turn", func(t *testing.T) {
		t.Parallel()
		script := fakellm.MustParseString(`
			{"text": "before"}
			{"error": {"message": "rate limited"}}
			{"text": "after"}
		`)
		require.Len(t, script.Turns, 3)
		require.Nil(t, script.Turns[0].Err)
		require.NotNil(t, script.Turns[1].Err)
		require.Equal(t, "rate limited", script.Turns[1].Err.Message)
		require.Nil(t, script.Turns[2].Err)
	})
}
