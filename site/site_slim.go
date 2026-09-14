//go:build !embed
// +build !embed

package site

import (
	"io/fs"
	"testing/fstest"
	"time"
)

func FS() fs.FS {
	// This is required to contain an index.html file for unit tests.
	// Our unit tests frequently just hit `/` and expect to get a 200.
	// So a valid index.html file should be expected to be served.
	return fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data:    []byte("Slim build of Coder, does not contain the frontend static files."),
			ModTime: time.Now(),
		},
	}
}
