package chatd

import "github.com/coder/coder/v2/codersdk"

// Limits are the deployment-configurable ceilings on chat turns and on
// the payloads chatd stores. A zero field means "use the default" so
// callers can leave fields unset; New and LimitsFromConfig resolve them.
type Limits struct {
	MaxStepsPerTurn               int
	MaxGenerationRetries          int
	MaxQueuedMessagesPerChat      int
	MaxAttachmentsPerChat         int
	MaxPromptBytes                int
	MaxDynamicToolsPerChat        int
	MaxToolOutputBytes            int
	MaxConcurrentRecordingUploads int
	DebugMaxTextRunes             int
	DebugMaxBodyBytes             int
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
		MaxDynamicToolsPerChat:        int(cfg.MaxDynamicToolsPerChat.Value()),
		MaxToolOutputBytes:            int(cfg.MaxToolOutputBytes.Value()),
		MaxConcurrentRecordingUploads: int(cfg.MaxConcurrentRecordingUploads.Value()),
		DebugMaxTextRunes:             int(cfg.DebugMaxTextRunes.Value()),
		DebugMaxBodyBytes:             int(cfg.DebugMaxBodyBytes.Value()),
	}.withDefaults()
}

func (l Limits) withDefaults() Limits {
	return Limits{
		MaxStepsPerTurn:               limitOrDefault(l.MaxStepsPerTurn, codersdk.DefaultChatMaxStepsPerTurn),
		MaxGenerationRetries:          limitOrDefault(l.MaxGenerationRetries, codersdk.DefaultChatMaxGenerationRetries),
		MaxQueuedMessagesPerChat:      limitOrDefault(l.MaxQueuedMessagesPerChat, codersdk.DefaultChatMaxQueuedMessagesPerChat),
		MaxAttachmentsPerChat:         limitOrDefault(l.MaxAttachmentsPerChat, codersdk.DefaultChatMaxAttachmentsPerChat),
		MaxPromptBytes:                limitOrDefault(l.MaxPromptBytes, codersdk.DefaultChatMaxPromptBytes),
		MaxDynamicToolsPerChat:        limitOrDefault(l.MaxDynamicToolsPerChat, codersdk.DefaultChatMaxDynamicToolsPerChat),
		MaxToolOutputBytes:            limitOrDefault(l.MaxToolOutputBytes, codersdk.DefaultChatMaxToolOutputBytes),
		MaxConcurrentRecordingUploads: limitOrDefault(l.MaxConcurrentRecordingUploads, codersdk.DefaultChatMaxConcurrentRecordingUploads),
		DebugMaxTextRunes:             limitOrDefault(l.DebugMaxTextRunes, codersdk.DefaultChatDebugMaxTextRunes),
		DebugMaxBodyBytes:             limitOrDefault(l.DebugMaxBodyBytes, codersdk.DefaultChatDebugMaxBodyBytes),
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
