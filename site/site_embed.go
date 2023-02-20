//go:build embed
// +build embed

package site

import (
	"embed"
	"io/fs"
)

//go:embed all:out
//go:embed all:out/bin/*
var site embed.FS

func FS() fs.FS {
	// the out directory is where FE builds are created. It is in the same
	// directory as this file (package site).
	out, err := fs.Sub(site, "out")
	if err != nil {
		// This can't happen... Go would throw a compilation error.
		panic(err)
	}
	return out
}
