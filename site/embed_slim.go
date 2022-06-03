//go:build !embed
// +build !embed

package site

import (
	"net/http"
)

type APIResponse struct {
	StatusCode int
	Message    string
}

func Handler() http.Handler {
	return http.NotFoundHandler()
}

func WithAPIResponse(ctx context.Context, _ APIResponse) context.Context {
	return ctx
}
