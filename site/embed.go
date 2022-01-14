//go:build !embed
// +build !embed

package site

import (
	"net/http"
)

// Handler returns a default handler when the site is not
// statically embedded.
func Handler() http.Handler {
	return http.NotFoundHandler()
}
