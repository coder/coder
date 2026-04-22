package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
)

func TestComputerUseTargetFromConfig_RejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := computerUseTargetFromConfig(database.ChatModelConfig{
		Provider: "openai-compat",
		Model:    "computer-use-preview-2025-03-11",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `computer use provider "openai-compat" is not supported`)

	classified := chaterror.Classify(err)
	require.Equal(t, chaterror.KindConfig, classified.Kind)
	require.Equal(t, "openai-compat", classified.Provider)
}
