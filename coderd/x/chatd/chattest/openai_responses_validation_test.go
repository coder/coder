package chattest_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

func TestValidateResponsesAPIInput_Pass(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		items []interface{}
	}{
		{
			name:  "empty",
			items: nil,
		},
		{
			name: "user only",
			items: []interface{}{
				map[string]interface{}{"type": "message", "role": "user"},
			},
		},
		{
			name: "reasoning then web_search reference",
			items: []interface{}{
				map[string]interface{}{"type": "message", "role": "user"},
				map[string]interface{}{"type": "item_reference", "id": "rs_001"},
				map[string]interface{}{"type": "item_reference", "id": "ws_001"},
				map[string]interface{}{"type": "message", "role": "assistant"},
			},
		},
		{
			name: "function_call paired with function_call_output",
			items: []interface{}{
				map[string]interface{}{"type": "message", "role": "user"},
				map[string]interface{}{
					"type":      "function_call",
					"call_id":   "call_001",
					"name":      "add",
					"arguments": `{"a":1}`,
				},
				map[string]interface{}{
					"type":    "function_call_output",
					"call_id": "call_001",
					"output":  "2",
				},
			},
		},
		{
			name: "reasoning reference standalone is allowed",
			items: []interface{}{
				map[string]interface{}{"type": "item_reference", "id": "rs_001"},
				map[string]interface{}{"type": "message", "role": "assistant"},
			},
		},
		{
			// Fantasy emits item_reference entries with only "id"
			// set because the openai-go SDK marks the type field
			// as `omitzero`. The validator must still detect the
			// pairing rule based on the bare-{id} shape.
			name: "implicit item_reference shape (no type field)",
			items: []interface{}{
				map[string]interface{}{"role": "user", "content": "hi"},
				map[string]interface{}{"id": "rs_001"},
				map[string]interface{}{"id": "ws_001"},
				map[string]interface{}{"role": "user", "content": "more"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Nil(t, chattest.ValidateResponsesAPIInput(tc.items))
		})
	}
}

func TestValidateResponsesAPIInput_RejectsWebSearchWithoutReasoning(t *testing.T) {
	t.Parallel()

	items := []interface{}{
		map[string]interface{}{"type": "message", "role": "user"},
		map[string]interface{}{"type": "item_reference", "id": "ws_001"},
		map[string]interface{}{"type": "message", "role": "assistant"},
	}

	err := chattest.ValidateResponsesAPIInput(items)
	require.NotNil(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.Contains(t, err.Message, "ws_001")
	require.Contains(t, err.Message, "web_search_call")
	require.Contains(t, err.Message, "reasoning")
}

func TestValidateResponsesAPIInput_RejectsImplicitWebSearchWithoutReasoning(t *testing.T) {
	t.Parallel()

	// Bare-{id} shape (no "type" field). Validator must still
	// detect the orphan web_search_call.
	items := []interface{}{
		map[string]interface{}{"role": "user", "content": "hi"},
		map[string]interface{}{"id": "ws_001"},
		map[string]interface{}{"role": "user", "content": "more"},
	}

	err := chattest.ValidateResponsesAPIInput(items)
	require.NotNil(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.Contains(t, err.Message, "ws_001")
}

func TestValidateResponsesAPIInput_RejectsWebSearchAfterUnrelatedItem(t *testing.T) {
	t.Parallel()

	// reasoning preceded the web_search but a message intervenes.
	items := []interface{}{
		map[string]interface{}{"type": "item_reference", "id": "rs_001"},
		map[string]interface{}{"type": "message", "role": "user"},
		map[string]interface{}{"type": "item_reference", "id": "ws_001"},
	}

	err := chattest.ValidateResponsesAPIInput(items)
	require.NotNil(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
}

func TestValidateResponsesAPIInput_RejectsFunctionCallWithoutOutput(t *testing.T) {
	t.Parallel()

	items := []interface{}{
		map[string]interface{}{"type": "message", "role": "user"},
		map[string]interface{}{
			"type":      "function_call",
			"call_id":   "call_xyz",
			"name":      "add",
			"arguments": "{}",
		},
		map[string]interface{}{"type": "message", "role": "user"},
	}

	err := chattest.ValidateResponsesAPIInput(items)
	require.NotNil(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.Contains(t, err.Message, "call_xyz")
	require.Contains(t, err.Message, "No tool output found")
}
