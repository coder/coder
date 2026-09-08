//go:build !desktop_runtime || !linux || !(amd64 || arm64)

package desktopruntime

// Builds without the desktop_runtime tag, or for platforms without a
// runtime, embed nothing so the agent falls back to an external desktop
// implementation.
const archiveName = ""

var archive []byte
