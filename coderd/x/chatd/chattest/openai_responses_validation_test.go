package chattest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

func TestValidateResponsesAPIInput(t *testing.T) {
	t.Parallel()

	t.Run("valid reasoning and web search references", func(t *testing.T) {
		t.Parallel()

		errResp := chattest.ValidateResponsesAPIInput([]interface{}{
			map[string]interface{}{"type": "item_reference", "id": "rs_valid"},
			map[string]interface{}{"type": "item_reference", "id": "ws_valid"},
		})
		require.Nil(t, errResp)
	})

	t.Run("rejects web search without reasoning", func(t *testing.T) {
		t.Parallel()

		errResp := chattest.ValidateResponsesAPIInput([]interface{}{
			map[string]interface{}{"type": "item_reference", "id": "ws_orphan"},
		})
		require.NotNil(t, errResp)
		require.Equal(t, 400, errResp.StatusCode)
		require.Contains(t, errResp.Message, "web_search_call")
		require.Contains(t, errResp.Message, "reasoning")
	})

	t.Run("valid function call and output", func(t *testing.T) {
		t.Parallel()

		errResp := chattest.ValidateResponsesAPIInput([]interface{}{
			map[string]interface{}{"type": "function_call", "call_id": "call_valid"},
			map[string]interface{}{"type": "function_call_output", "call_id": "call_valid"},
		})
		require.Nil(t, errResp)
	})

	t.Run("rejects function call without output", func(t *testing.T) {
		t.Parallel()

		errResp := chattest.ValidateResponsesAPIInput([]interface{}{
			map[string]interface{}{"type": "function_call", "call_id": "call_orphan"},
		})
		require.NotNil(t, errResp)
		require.Contains(t, errResp.Message, "No tool output found for function call call_orphan")
	})

	t.Run("rejects output before function call", func(t *testing.T) {
		t.Parallel()

		errResp := chattest.ValidateResponsesAPIInput([]interface{}{
			map[string]interface{}{"type": "function_call_output", "call_id": "call_late"},
			map[string]interface{}{"type": "function_call", "call_id": "call_late"},
		})
		require.NotNil(t, errResp)
		require.Contains(t, errResp.Message, "Tool output found without preceding function call call_late")
	})

	t.Run("rejects duplicate function call", func(t *testing.T) {
		t.Parallel()

		errResp := chattest.ValidateResponsesAPIInput([]interface{}{
			map[string]interface{}{"type": "function_call", "call_id": "call_duplicate"},
			map[string]interface{}{"type": "function_call", "call_id": "call_duplicate"},
			map[string]interface{}{"type": "function_call_output", "call_id": "call_duplicate"},
		})
		require.NotNil(t, errResp)
		require.Contains(t, errResp.Message, "Duplicate function call found for call_id call_duplicate")
	})

	t.Run("rejects duplicate function call output", func(t *testing.T) {
		t.Parallel()

		errResp := chattest.ValidateResponsesAPIInput([]interface{}{
			map[string]interface{}{"type": "function_call", "call_id": "call_duplicate_output"},
			map[string]interface{}{"type": "function_call_output", "call_id": "call_duplicate_output"},
			map[string]interface{}{"type": "function_call_output", "call_id": "call_duplicate_output"},
		})
		require.NotNil(t, errResp)
		require.Contains(t, errResp.Message, "Duplicate tool output found for function call call_duplicate_output")
	})

	t.Run("classifies item reference by prefix without type field", func(t *testing.T) {
		t.Parallel()

		errResp := chattest.ValidateResponsesAPIInput([]interface{}{
			map[string]interface{}{"id": "rs_prefix_only"},
			map[string]interface{}{"id": "ws_prefix_only"},
		})
		require.Nil(t, errResp)
	})
}
