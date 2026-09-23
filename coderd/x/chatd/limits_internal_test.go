package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestLimitsFromConfig(t *testing.T) {
	t.Parallel()

	t.Run("ZeroConfigUsesDefaults", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, Limits{
			MaxStepsPerTurn:               codersdk.DefaultChatMaxStepsPerTurn,
			MaxGenerationRetries:          codersdk.DefaultChatMaxGenerationRetries,
			MaxQueuedMessagesPerChat:      codersdk.DefaultChatMaxQueuedMessagesPerChat,
			MaxAttachmentsPerChat:         codersdk.DefaultChatMaxAttachmentsPerChat,
			MaxPromptBytes:                codersdk.DefaultChatMaxPromptBytes,
			MaxConcurrentRecordingUploads: codersdk.DefaultChatMaxConcurrentRecordingUploads,
		}, LimitsFromConfig(codersdk.ChatConfig{}))
	})

	t.Run("ConfiguredValuesWin", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.ChatConfig{
			MaxStepsPerTurn:               serpent.Int64(7),
			MaxGenerationRetries:          serpent.Int64(3),
			MaxQueuedMessagesPerChat:      serpent.Int64(2),
			MaxAttachmentsPerChat:         serpent.Int64(9),
			MaxPromptBytes:                serpent.Int64(1024),
			MaxConcurrentRecordingUploads: serpent.Int64(1),
		}
		require.Equal(t, Limits{
			MaxStepsPerTurn:               7,
			MaxGenerationRetries:          3,
			MaxQueuedMessagesPerChat:      2,
			MaxAttachmentsPerChat:         9,
			MaxPromptBytes:                1024,
			MaxConcurrentRecordingUploads: 1,
		}, LimitsFromConfig(cfg))
	})

	t.Run("NegativeFallsBackToDefault", func(t *testing.T) {
		t.Parallel()
		limits := Limits{MaxStepsPerTurn: -1, MaxPromptBytes: 10}.withDefaults()
		require.Equal(t, codersdk.DefaultChatMaxStepsPerTurn, limits.MaxStepsPerTurn)
		require.Equal(t, 10, limits.MaxPromptBytes)
	})
}
