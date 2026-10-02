package agentmcp

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

// PluginDataDir returns the PLUGIN_DATA directory for the plugin named
// name installed at root: <home>/.coder/plugin-data/<name>-<hash>, where
// hash is the first 12 hex characters of sha256(root). Keying by root
// keeps two installations of the same plugin name from sharing state.
func PluginDataDir(home, name, root string) string {
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(home, ".coder", "plugin-data", name+"-"+hex.EncodeToString(sum[:])[:12])
}
