package agentbox

import (
	_ "embed"
)

// LanguageJavaScript runs scripts with quickjs-ng. The std and os modules
// are exposed as globals. The script runs from /script, so ES modules
// staged in the box are imported by absolute /box/... path.
const LanguageJavaScript = "javascript"

//go:embed runtimes/quickjs/qjs-wasi.wasm
var quickjsModule []byte

func init() {
	registerRuntime(guestRuntime{
		name:   LanguageJavaScript,
		module: quickjsModule,
		entry:  "main.js",
		argv: func(scriptPath string, args []string) []string {
			// argv[0] must be present or qjs enters its REPL. --std makes
			// the std and os modules available as globals.
			return append([]string{"qjs", "--std", scriptPath}, args...)
		},
	})
}
