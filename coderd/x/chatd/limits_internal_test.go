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
			MaxDynamicToolsPerChat:        codersdk.DefaultChatMaxDynamicToolsPerChat,
			MaxToolOutputBytes:            codersdk.DefaultChatMaxToolOutputBytes,
			MaxConcurrentRecordingUploads: codersdk.DefaultChatMaxConcurrentRecordingUploads,
			DebugMaxTextRunes:             codersdk.DefaultChatDebugMaxTextRunes,
			DebugMaxBodyBytes:             codersdk.DefaultChatDebugMaxBodyBytes,
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
			MaxDynamicToolsPerChat:        serpent.Int64(5),
			MaxToolOutputBytes:            serpent.Int64(2048),
			MaxConcurrentRecordingUploads: serpent.Int64(1),
			DebugMaxTextRunes:             serpent.Int64(100),
			DebugMaxBodyBytes:             serpent.Int64(200),
		}
		require.Equal(t, Limits{
			MaxStepsPerTurn:               7,
			MaxGenerationRetries:          3,
			MaxQueuedMessagesPerChat:      2,
			MaxAttachmentsPerChat:         9,
			MaxPromptBytes:                1024,
			MaxDynamicToolsPerChat:        5,
			MaxToolOutputBytes:            2048,
			MaxConcurrentRecordingUploads: 1,
			DebugMaxTextRunes:             100,
			DebugMaxBodyBytes:             200,
		}, LimitsFromConfig(cfg))
	})

	t.Run("NegativeFallsBackToDefault", func(t *testing.T) {
		t.Parallel()
		limits := Limits{MaxStepsPerTurn: -1, MaxToolOutputBytes: 10}.withDefaults()
		require.Equal(t, codersdk.DefaultChatMaxStepsPerTurn, limits.MaxStepsPerTurn)
		require.Equal(t, 10, limits.MaxToolOutputBytes)
	})
}
