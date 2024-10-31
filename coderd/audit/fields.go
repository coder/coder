package audit

import (
	"context"
	"encoding/json"

	"cdr.dev/slog"
)

type BackgroundSubsystem string

const (
	BackgroundSubsystemDormancy BackgroundSubsystem = "dormancy"
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
		logger.Error(ctx, "marshal additional fields for dormancy audit", slog.Error(err))
		return []byte("{}")
	}

	return wriBytes
}
