package agentbox

import (
	"crypto/sha256"
	"encoding/hex"
)

// guestRuntime describes one embedded WASI command module that executes guest
// source code. Every runtime receives the script at /script/main.js (or the
// runtime's own entry name) on a read-only mount and the box directory at
// /box.
type guestRuntime struct {
	// name is the language identifier accepted by RunRequest.Language.
	name string
	// module is the WASI preview1 command module.
	module []byte
	// entry is the file name of the script inside the /script mount.
	entry string
	// argv builds the guest argv for a script at scriptPath with the
	// caller's extra arguments.
	argv func(scriptPath string, args []string) []string
}

// runtimes maps language names to embedded runtimes. Adding a language is
// one file registering itself here in an init function.
var runtimes = map[string]guestRuntime{}

func registerRuntime(rt guestRuntime) {
	if _, dup := runtimes[rt.name]; dup {
		panic("agentbox: duplicate runtime " + rt.name)
	}
	runtimes[rt.name] = rt
}

// EmbeddedRuntimeSHA256 returns the hex SHA-256 of the embedded module for
// language, or an empty string for an unknown language.
func EmbeddedRuntimeSHA256(language string) string {
	rt, ok := runtimes[language]
	if !ok {
		return ""
	}
	sum := sha256.Sum256(rt.module)
	return hex.EncodeToString(sum[:])
}
