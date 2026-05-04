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
