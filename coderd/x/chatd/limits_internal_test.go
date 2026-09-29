package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestLimitsFromConfig(t *testing.T) {
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
}
