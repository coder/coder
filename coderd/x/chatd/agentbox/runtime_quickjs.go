package agentbox

import (
	_ "embed"
)

// LanguageJavaScript runs scripts with quickjs-ng. The std and os modules
// are exposed as globals, and the mcp global from prelude.js is defined
// before the script runs. The script runs from /script, so ES modules
// staged in the box are imported by absolute /box/... path.
const LanguageJavaScript = "javascript"

//go:embed runtimes/quickjs/qjs-wasi.wasm
var quickjsModule []byte

//go:embed runtimes/quickjs/prelude.js
var quickjsPrelude []byte

func init() {
	registerRuntime(guestRuntime{
		name:         LanguageJavaScript,
		module:       quickjsModule,
		entry:        "main.js",
		prelude:      quickjsPrelude,
		preludeEntry: "prelude.js",
		argv: func(scriptPath, preludePath string, args []string) []string {
			// argv[0] must be present or qjs enters its REPL. --std makes
			// the std and os modules available as globals; -I evaluates
			// the prelude as a classic script first.
			return append([]string{"qjs", "--std", "-I", preludePath, scriptPath}, args...)
		},
	})
}
