// Package docgenenv normalizes the process environment so the documentation
// generators produce identical output regardless of the host they run on.
package docgenenv

import (
	"os"
	"strings"
)

// Prepare clears CODER_* variables and pins the cache, config, and temp
// directories. Without this, defaults derived from os.UserCacheDir and the
// config directory embed the generating host's home directory in the docs.
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
