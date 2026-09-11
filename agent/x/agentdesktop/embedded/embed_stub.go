//go:build !portabledesktop_embed || !linux || !(amd64 || arm64)

package embedded

// Builds without the tag, or for platforms without a release, embed nothing
// so the agent falls back to a portabledesktop found on PATH.
var embeddedBinary []byte
