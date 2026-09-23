package chatd

import "github.com/coder/coder/v2/codersdk"

// Limits are the deployment-configurable ceilings on chat turns and on
// the payloads chatd stores. A zero field means "use the default" so
// callers can leave fields unset; New and LimitsFromConfig resolve them.
type Limits struct {
	// MaxStepsPerTurn bounds the model and tool steps one turn may run.
	MaxStepsPerTurn int
	// MaxGenerationRetries bounds how many times a turn, or one nested
	// advisor call, retries a model call that failed with a transient
	// provider error.
	MaxGenerationRetries int
	// MaxQueuedMessagesPerChat bounds the user messages waiting in a
	// chat's queue while a turn runs.
	MaxQueuedMessagesPerChat int
	// MaxAttachmentsPerChat is the number of most recent attachments a
	// chat keeps; older files are removed when new ones are linked.
	MaxAttachmentsPerChat int
	// MaxPromptBytes bounds the deployment system prompt, plan mode
	// instructions, and per-user custom prompts.
	MaxPromptBytes int
	// MaxConcurrentRecordingUploads bounds the virtual desktop recordings
	// chatd stores concurrently.
	MaxConcurrentRecordingUploads int
}

// LimitsFromConfig resolves the configured chat limits, substituting
// the codersdk defaults for any value that is not positive.
func LimitsFromConfig(cfg codersdk.ChatConfig) Limits {
	return Limits{
		MaxStepsPerTurn:               int(cfg.MaxStepsPerTurn.Value()),
		MaxGenerationRetries:          int(cfg.MaxGenerationRetries.Value()),
		MaxQueuedMessagesPerChat:      int(cfg.MaxQueuedMessagesPerChat.Value()),
		MaxAttachmentsPerChat:         int(cfg.MaxAttachmentsPerChat.Value()),
		MaxPromptBytes:                int(cfg.MaxPromptBytes.Value()),
		MaxConcurrentRecordingUploads: int(cfg.MaxConcurrentRecordingUploads.Value()),
	}.withDefaults()
}

func (l Limits) withDefaults() Limits {
	return Limits{
		MaxStepsPerTurn:               limitOrDefault(l.MaxStepsPerTurn, codersdk.DefaultChatMaxStepsPerTurn),
		MaxGenerationRetries:          limitOrDefault(l.MaxGenerationRetries, codersdk.DefaultChatMaxGenerationRetries),
		MaxQueuedMessagesPerChat:      limitOrDefault(l.MaxQueuedMessagesPerChat, codersdk.DefaultChatMaxQueuedMessagesPerChat),
		MaxAttachmentsPerChat:         limitOrDefault(l.MaxAttachmentsPerChat, codersdk.DefaultChatMaxAttachmentsPerChat),
		MaxPromptBytes:                limitOrDefault(l.MaxPromptBytes, codersdk.DefaultChatMaxPromptBytes),
		MaxConcurrentRecordingUploads: limitOrDefault(l.MaxConcurrentRecordingUploads, codersdk.DefaultChatMaxConcurrentRecordingUploads),
	}
}

// limits returns the effective chat limits. Servers built as bare
// literals in tests carry zero limits, so defaults are applied here too.
func (p *Server) limits() Limits {
	return p.chatLimits.withDefaults()
}

func limitOrDefault(value, def int) int {
	if value <= 0 {
		return def
	}
	return value
}
