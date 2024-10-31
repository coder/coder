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

func BackgroundTaskFields(ctx context.Context, logger slog.Logger, subsystem BackgroundSubsystem) []byte {
	af := map[string]string{
		"automatic_actor":     "coder",
		"automatic_subsystem": string(subsystem),
	}

	wriBytes, err := json.Marshal(af)
	if err != nil {
		logger.Error(ctx, "marshal additional fields for dormancy audit", slog.Error(err))
		return []byte("{}")
	}

	return wriBytes
}
