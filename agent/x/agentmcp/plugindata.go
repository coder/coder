package agentmcp

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// pluginDataHashLen is the number of hex characters of the installDir hash in
// a plugin data directory name. Changing it orphans existing plugin
// data.
const pluginDataHashLen = 12

// PluginDataDir returns the data directory path,
// <home>/.coder/plugin-data/<name>-<hash>, for the plugin named name
// installed at installDir. The hash covers the cleaned installDir, so
// two installations of the same plugin name get different directories.
// The directory is not created. name must be a valid Agent Plugins
// manifest name.
func PluginDataDir(home, name, installDir string) (string, error) {
	if err := workspacesdk.ValidatePluginName(name); err != nil {
		return "", xerrors.Errorf("plugin data dir: %w", err)
	}
	sum := sha256.Sum256([]byte(filepath.Clean(installDir)))
	return filepath.Join(home, ".coder", "plugin-data", name+"-"+hex.EncodeToString(sum[:])[:pluginDataHashLen]), nil
}
