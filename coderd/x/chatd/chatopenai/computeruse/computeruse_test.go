package computeruse_test

import (
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai/computeruse"
)

func TestComputerUseTool(t *testing.T) {
	t.Parallel()

	tool := computeruse.Tool()
	require.True(t, computeruse.IsTool(tool))
	require.Equal(t, "computer", tool.GetName())
}

func TestComputerUseResultProviderMetadata(t *testing.T) {
	t.Parallel()

	t.Run("SuccessfulImage", func(t *testing.T) {
		t.Parallel()

		metadata := computeruse.ResultProviderMetadata(
			fantasy.NewImageResponse([]byte("png"), "image/png"),
		)
		outputOptions, ok := metadata[fantasyopenai.Name].(*fantasyopenai.ComputerCallOutputOptions)
		require.True(t, ok)
		require.Equal(t, "original", outputOptions.Detail)
	})

	tests := []struct {
		name     string
		response fantasy.ToolResponse
	}{
		{name: "Error", response: fantasy.NewTextErrorResponse("failed")},
		{name: "Text", response: fantasy.NewTextResponse("ok")},
		{name: "EmptyImage", response: fantasy.NewImageResponse(nil, "image/png")},
		{
			name:     "NonImageMediaType",
			response: fantasy.NewImageResponse([]byte("png"), "application/octet-stream"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			metadata := computeruse.ResultProviderMetadata(tt.response)
			require.Nil(t, metadata)
		})
	}
}

func TestDesktopActionsWrapsPointerActionsWithModifiers(t *testing.T) {
	t.Parallel()

	input, err := computeruse.ParseInput(`{
		"call_id":"call_click_modifier",
		"actions":[{"type":"click","button":"left","x":70,"y":80,"keys":["ctrl","shift"]}]
	}`)
	require.NoError(t, err)

	actions, err := computeruse.DesktopActions(input, 1440, 900)
	require.NoError(t, err)
	require.Len(t, actions, 5)

	require.Equal(t, "key_down", actions[0].Action.Action)
	require.NotNil(t, actions[0].Action.Text)
	require.Equal(t, "ctrl", *actions[0].Action.Text)
	require.Equal(t, []string{"ctrl"}, actions[0].ReleaseKeysOnFailure)

	require.Equal(t, "key_down", actions[1].Action.Action)
	require.NotNil(t, actions[1].Action.Text)
	require.Equal(t, "shift", *actions[1].Action.Text)
	require.Equal(t, []string{"ctrl", "shift"}, actions[1].ReleaseKeysOnFailure)

	require.Equal(t, "left_click", actions[2].Action.Action)
	require.Equal(t, []string{"ctrl", "shift"}, actions[2].ReleaseKeysOnFailure)

	require.Equal(t, "key_up", actions[3].Action.Action)
	require.NotNil(t, actions[3].Action.Text)
	require.Equal(t, "shift", *actions[3].Action.Text)
	require.Equal(t, []string{"ctrl", "shift"}, actions[3].ReleaseKeysOnFailure)

	require.Equal(t, "key_up", actions[4].Action.Action)
	require.NotNil(t, actions[4].Action.Text)
	require.Equal(t, "ctrl", *actions[4].Action.Text)
	require.Equal(t, []string{"ctrl"}, actions[4].ReleaseKeysOnFailure)
}

func TestDesktopActionsMarksFinalDragReleaseForCleanup(t *testing.T) {
	t.Parallel()

	input, err := computeruse.ParseInput(`{
		"call_id":"call_drag",
		"actions":[{"type":"drag","path":[{"x":1,"y":2},{"x":3,"y":4}]}]
	}`)
	require.NoError(t, err)

	actions, err := computeruse.DesktopActions(input, 1440, 900)
	require.NoError(t, err)
	require.Len(t, actions, 4)
	require.Equal(t, "left_mouse_down", actions[1].Action.Action)
	require.True(t, actions[1].ReleaseMouseOnFailure)
	require.Equal(t, "left_mouse_up", actions[3].Action.Action)
	require.True(t, actions[3].ReleaseMouseOnFailure)
}

func TestDesktopActionsDefaultsEmptyClickButtonToLeft(t *testing.T) {
	t.Parallel()

	input, err := computeruse.ParseInput(`{
		"call_id":"call_empty_button",
		"actions":[{"type":"click","x":70,"y":80}]
	}`)
	require.NoError(t, err)

	actions, err := computeruse.DesktopActions(input, 1440, 900)
	require.NoError(t, err)
	require.Len(t, actions, 1)
	require.Equal(t, "left_click", actions[0].Action.Action)
}

func TestDesktopActionsMapsBackForwardClickButtons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		button  string
		wantKey string
	}{
		{name: "Back", button: "back", wantKey: "alt+Left"},
		{name: "Forward", button: "forward", wantKey: "alt+Right"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input, err := computeruse.ParseInput(`{
				"call_id":"call_side_button",
				"actions":[{"type":"click","button":"` + tt.button + `","x":70,"y":80}]
			}`)
			require.NoError(t, err)

			actions, err := computeruse.DesktopActions(input, 1440, 900)
			require.NoError(t, err)
			require.Len(t, actions, 2)
			require.Equal(t, "mouse_move", actions[0].Action.Action)
			require.Equal(t, "key", actions[1].Action.Action)
			require.NotNil(t, actions[1].Action.Text)
			require.Equal(t, tt.wantKey, *actions[1].Action.Text)
		})
	}
}

func TestDesktopActionsRejectsUnsupportedDoubleClickButton(t *testing.T) {
	t.Parallel()

	input, err := computeruse.ParseInput(`{
		"call_id":"call_double_click",
		"actions":[{"type":"double_click","button":"right","x":70,"y":80}]
	}`)
	require.NoError(t, err)

	_, err = computeruse.DesktopActions(input, 1440, 900)
	require.Error(t, err)
	require.Contains(t, err.Error(), `unsupported OpenAI double-click button "right"`)
}

func TestDesktopActionsConvertsScrollPixelsToWheelClicks(t *testing.T) {
	t.Parallel()

	input, err := computeruse.ParseInput(`{
		"call_id":"call_scroll",
		"actions":[{"type":"scroll","x":70,"y":80,"scroll_y":401,"scroll_x":-99}]
	}`)
	require.NoError(t, err)

	actions, err := computeruse.DesktopActions(input, 1440, 900)
	require.NoError(t, err)
	require.Len(t, actions, 3)

	vertical := actions[1].Action
	require.NotNil(t, vertical.ScrollAmount)
	require.NotNil(t, vertical.ScrollDirection)
	require.Equal(t, "down", *vertical.ScrollDirection)
	require.Equal(t, 5, *vertical.ScrollAmount)

	horizontal := actions[2].Action
	require.NotNil(t, horizontal.ScrollAmount)
	require.NotNil(t, horizontal.ScrollDirection)
	require.Equal(t, "left", *horizontal.ScrollDirection)
	require.Equal(t, 1, *horizontal.ScrollAmount)
}
