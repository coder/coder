package chatd

import (
	"cmp"

	"github.com/coder/coder/v2/codersdk"
)

// Limits are the deployment chat limits.
type Limits struct {
	// MaxStepsPerTurn is the maximum number of steps in a chat turn.
	MaxStepsPerTurn int
	// MaxGenerationRetries is the maximum number of consecutive retries
	// after a model generation fails.
	MaxGenerationRetries int
	// MaxQueuedMessagesPerChat is the maximum number of messages that can
	// be queued in a chat.
	MaxQueuedMessagesPerChat int
	// MaxAttachmentsPerChat is the maximum number of files linked to a
	// chat.
	MaxAttachmentsPerChat int
	// MaxPromptBytes is the maximum size in bytes of the deployment system
	// prompt, the plan mode instructions, and each user's custom prompt.
	MaxPromptBytes int
	// MaxConcurrentRecordingUploads is the maximum number of virtual
	// desktop recordings that the server stores at the same time.
	MaxConcurrentRecordingUploads int
}

// LimitsFromConfig converts the deployment chat configuration to Limits.
func LimitsFromConfig(cfg codersdk.ChatConfig) Limits {
	return Limits{
		MaxStepsPerTurn:               int(cfg.MaxStepsPerTurn.Value()),
		MaxGenerationRetries:          int(cfg.MaxGenerationRetries.Value()),
		MaxQueuedMessagesPerChat:      int(cfg.MaxQueuedMessagesPerChat.Value()),
		MaxAttachmentsPerChat:         int(cfg.MaxAttachmentsPerChat.Value()),
		MaxPromptBytes:                int(cfg.MaxPromptBytes.Value()),
		MaxConcurrentRecordingUploads: int(cfg.MaxConcurrentRecordingUploads.Value()),
	}
}

// withDefaults sets each zero field to its codersdk default.
func (l Limits) withDefaults() Limits {
	return Limits{
		MaxStepsPerTurn:               cmp.Or(l.MaxStepsPerTurn, codersdk.DefaultChatMaxStepsPerTurn),
		MaxGenerationRetries:          cmp.Or(l.MaxGenerationRetries, codersdk.DefaultChatMaxGenerationRetries),
		MaxQueuedMessagesPerChat:      cmp.Or(l.MaxQueuedMessagesPerChat, codersdk.DefaultChatMaxQueuedMessagesPerChat),
		MaxAttachmentsPerChat:         cmp.Or(l.MaxAttachmentsPerChat, codersdk.DefaultChatMaxAttachmentsPerChat),
		MaxPromptBytes:                cmp.Or(l.MaxPromptBytes, codersdk.DefaultChatMaxPromptBytes),
		MaxConcurrentRecordingUploads: cmp.Or(l.MaxConcurrentRecordingUploads, codersdk.DefaultChatMaxConcurrentRecordingUploads),
	}
}
