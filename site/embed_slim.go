//go:build slim
// +build slim

package site

import (
	"net/http"
)

func DefaultHandler() http.Handler {
	return http.NotFoundHandler()
}
