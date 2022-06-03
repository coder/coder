//go:build !embed
// +build !embed

package site

import (
	"embed"
	"io/fs"
)

var slim embed.FS

func FS() fs.FS {
	return slim
}
