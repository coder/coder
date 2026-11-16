package agentmcp

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Changing pluginDataHashLen orphans existing plugin data.
const pluginDataHashLen = 12

// PluginDataDir returns <home>/.coder/plugin-data/<name>-<hash> without
// creating it. The hash covers the cleaned installDir, so two installations
// of the same plugin name get different directories.
func PluginDataDir(home, name, installDir string) (string, error) {
	if err := workspacesdk.ValidatePluginName(name); err != nil {
		return "", xerrors.Errorf("plugin data dir: %w", err)
	}
	sum := sha256.Sum256([]byte(filepath.Clean(installDir)))
	return filepath.Join(home, ".coder", "plugin-data", name+"-"+hex.EncodeToString(sum[:])[:pluginDataHashLen]), nil
}
