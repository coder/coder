// Package docgenenv normalizes the process environment so documentation
// generators produce host-independent output.
package docgenenv

import (
	"os"
	"strings"
)

// Prepare clears CODER_* variables and pins the cache, config, and temp
// directories so generated docs don't embed the generating host's home
// directory.
func Prepare() {
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "CODER_") {
			continue
		}
		name, _, _ := strings.Cut(env, "=")
		if err := os.Unsetenv(name); err != nil {
			panic(err)
		}
	}

	mustSetenv("CLIDOCGEN_CACHE_DIRECTORY", "~/.cache")
	mustSetenv("CLIDOCGEN_CONFIG_DIRECTORY", "~/.config/coderv2")
	mustSetenv("TMPDIR", "/tmp")
}

func mustSetenv(key, value string) {
	if err := os.Setenv(key, value); err != nil {
		panic(err)
	}
}
