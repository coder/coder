package chatd

import (
	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

// ChatPersonalModelOverrideKeyPrefix is the user config key prefix for
// chat personal model overrides. Keys carry the organization UUID between
// the prefix and the context:
// chat_personal_model_override:<org-uuid>:{context}. Values under this
// prefix should be parsed with ParseChatPersonalModelOverride so malformed
// values use one fallback.
const ChatPersonalModelOverrideKeyPrefix = "chat_personal_model_override:"

// ChatPersonalModelOverrideKey returns the user config key for a chat
// personal model override context in the given organization. Values stored
// at the returned key should use ParseChatPersonalModelOverride so
// malformed values fall back safely.
func ChatPersonalModelOverrideKey(
	orgID uuid.UUID,
	overrideContext codersdk.ChatPersonalModelOverrideContext,
) string {
	return ChatPersonalModelOverrideKeyPrefix + orgID.String() + ":" + string(overrideContext)
}

// ParsedChatPersonalModelOverride is a parsed personal model override value.
// When Malformed is true, Mode is the provided default and ModelConfigID is
// uuid.Nil.
type ParsedChatPersonalModelOverride struct {
	Mode            codersdk.ChatPersonalModelOverrideMode
	ModelConfigID   uuid.UUID
	ReasoningEffort *string
	Malformed       bool
}

// ParseChatPersonalModelOverride parses a stored personal model override.
// Empty values return defaultMode without marking the value malformed.
// Malformed values return defaultMode, uuid.Nil, and Malformed true.
func ParseChatPersonalModelOverride(
	raw string,
	defaultMode codersdk.ChatPersonalModelOverrideMode,
) ParsedChatPersonalModelOverride {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ParsedChatPersonalModelOverride{Mode: defaultMode}
	}

	switch trimmed {
	case string(codersdk.ChatPersonalModelOverrideModeChatDefault):
		return ParsedChatPersonalModelOverride{
			Mode: codersdk.ChatPersonalModelOverrideModeChatDefault,
		}
	case string(codersdk.ChatPersonalModelOverrideModeDeploymentDefault):
		return ParsedChatPersonalModelOverride{
			Mode: codersdk.ChatPersonalModelOverrideModeDeploymentDefault,
		}
	}

	mode, rawModelConfigID, ok := strings.Cut(trimmed, ":")
	if !ok || mode != string(codersdk.ChatPersonalModelOverrideModeModel) {
		return ParsedChatPersonalModelOverride{
			Mode:      defaultMode,
			Malformed: true,
		}
	}
	rawID, rawEffort, hasEffort := strings.Cut(rawModelConfigID, ":")
	modelConfigID, err := uuid.Parse(rawID)
	if err != nil || (hasEffort && rawEffort == "") {
		return ParsedChatPersonalModelOverride{
			Mode:      defaultMode,
			Malformed: true,
		}
	}
	parsed := ParsedChatPersonalModelOverride{
		Mode:          codersdk.ChatPersonalModelOverrideModeModel,
		ModelConfigID: modelConfigID,
	}
	if hasEffort {
		parsed.ReasoningEffort = &rawEffort
	}
	return parsed
}
