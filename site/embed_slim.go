//go:build slim
// +build slim

package site

import (
	"net/http"

	"cdr.dev/slog"
)

func Handler(logger slog.Logger) http.Handler {
	return http.NotFoundHandler()
}
