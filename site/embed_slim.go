//go:build !embed
// +build !embed

package site

import (
	"net/http"
)

func DefaultHandler() http.Handler {
	return http.NotFoundHandler()
}
