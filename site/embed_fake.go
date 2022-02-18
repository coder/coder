//go:build !embed
// +build !embed

package site

import (
	"io/fs"

	"testing/fstest"
)

var site fs.FS = fstest.MapFS{
	// Create a fake filesystem for go tests, so that we can avoid
	// building the frontend
	"out/test": &fstest.MapFile{
		Data: []byte("dummy file"),
	},
}
