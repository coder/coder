//go:build embed
// +build embed

package site

import "embed"

// The `embed` package ignores recursively including directories
// that prefix with `_`. Wildcarding nested is janky, but seems to
// work quite well for edge-cases.
//go:embed out/_next/*/*/*/*
//go:embed out/_next/*/*/*
//go:embed out
var site embed.FS
