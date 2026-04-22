package chatd

import (
	"encoding/json"
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

func TestComputerUseTargetEligibilityError_ClassifiesInvalidOpenAIOptions(t *testing.T) {
	t.Parallel()

	target := computerUseTarget{
		provider: "openai",
		model:    "computer-use-preview-2025-03-11",
		config: database.ChatModelConfig{
			Provider: "openai",
			Model:    "computer-use-preview-2025-03-11",
			Enabled:  true,
			Options:  json.RawMessage(`{"provider_options":`),
		},
	}

	err := computerUseTargetEligibilityError(target, func(string) bool { return true })
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse computer use model call config")

	classified := chaterror.Classify(err)
	require.Equal(t, chaterror.KindConfig, classified.Kind)
	require.Equal(t, "openai", classified.Provider)
}
