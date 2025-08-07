package aibridged

import (
	"net/http"

	"cdr.dev/slog"
)

type Model struct {
	Provider, ModelName string
}

type Session[Req any] interface {
	Init(logger slog.Logger, baseURL, key string, tracker Tracker, toolMgr ToolManager) (id string)
	LastUserPrompt(req Req) (*string, error)
	Model(req *Req) Model
	Execute(req *Req, w http.ResponseWriter, r *http.Request) error
	Close() error
}
