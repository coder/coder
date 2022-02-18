//go:build !embed
// +build !embed

package site

import (
	"io/fs"

	"testing/fstest"
)

var site fs.FS = fstest.MapFS{
	"out/test": &fstest.MapFile{
		Data: []byte("dummy file"),
	},
}
