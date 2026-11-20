package audit

import (
	"context"
	"encoding/json"

	"cdr.dev/slog/v3"
)

type BackgroundSubsystem string

const (
	BackgroundSubsystemDormancy        BackgroundSubsystem = "dormancy"
	BackgroundSubsystemChatAutoArchive BackgroundSubsystem = "chat_auto_archive"
	// BackgroundSubsystemGroupSyncRewrite: Coder rewrote stored group sync
	// settings into valid form when they were read.
	BackgroundSubsystemGroupSyncRewrite BackgroundSubsystem = "group_sync_rewrite"
)

func BackgroundTaskFields(subsystem BackgroundSubsystem) map[string]string {
	return map[string]string{
		"automatic_actor":     "coder",
		"automatic_subsystem": string(subsystem),
	}
}

func BackgroundTaskFieldsBytes(ctx context.Context, logger slog.Logger, subsystem BackgroundSubsystem) []byte {
	af := BackgroundTaskFields(subsystem)

	wriBytes, err := json.Marshal(af)
	if err != nil {
		logger.Error(ctx, "marshal additional fields for background audit", slog.Error(err))
		return []byte("{}")
	}

	return wriBytes
}
