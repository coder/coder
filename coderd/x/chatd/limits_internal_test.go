package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
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

func TestNewSetsZeroLimitsToDefaults(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	require.Equal(t, Limits{
		MaxStepsPerTurn:               codersdk.DefaultChatMaxStepsPerTurn,
		MaxGenerationRetries:          codersdk.DefaultChatMaxGenerationRetries,
		MaxQueuedMessagesPerChat:      codersdk.DefaultChatMaxQueuedMessagesPerChat,
		MaxAttachmentsPerChat:         codersdk.DefaultChatMaxAttachmentsPerChat,
		MaxPromptBytes:                codersdk.DefaultChatMaxPromptBytes,
		MaxConcurrentRecordingUploads: codersdk.DefaultChatMaxConcurrentRecordingUploads,
	}, server.chatLimits)
	require.Equal(t, codersdk.DefaultChatMaxConcurrentRecordingUploads, cap(server.recordingSem))
}
