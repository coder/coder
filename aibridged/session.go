package aibridged

import (
	"net/http"

	"cdr.dev/slog"
)

type Model struct {
	Provider, ModelName string
}

// Session describes a (potentially) stateful interaction with an AI provider.
type Session interface {
	Init(logger slog.Logger, baseURL, key string, tracker Tracker, toolMgr ToolManager) (id string)
	LastUserPrompt() (*string, error)
	Model() Model
	ProcessRequest(w http.ResponseWriter, r *http.Request) error
	Close() error
}
