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
