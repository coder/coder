package gitauth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"
)

// OverrideVSCodeConfigs merges essential Coder settings with existing VS Code settings
// to ensure GIT_ASKPASS and Git authentication work properly with Coder.
func OverrideVSCodeConfigs(fs afero.Fs) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	// Define the essential settings that Coder needs to override
	coderSettings := map[string]interface{}{
		// This prevents VS Code from overriding GIT_ASKPASS, which
		// we use to automatically authenticate Git providers.
		"git.useIntegratedAskPass": false,
		// This prevents VS Code from using it's own GitHub authentication
		// which would circumvent cloning with Coder-configured providers.
		"github.gitAuthentication": false,
	}
	mutate := func(m map[string]interface{}) {
		// Merge Coder's essential settings with existing settings
		for key, value := range coderSettings {
			m[key] = value
		}
	}

	for _, configPath := range []string{
		// code-server's default configuration path.
		filepath.Join(xdg.DataHome, "code-server", "Machine", "settings.json"),
		// vscode-remote's default configuration path.
		filepath.Join(home, ".vscode-server", "data", "Machine", "settings.json"),
		// vscode-insiders' default configuration path.
		filepath.Join(home, ".vscode-insiders-server", "data", "Machine", "settings.json"),
		// cursor default configuration path.
		filepath.Join(home, ".cursor-server", "data", "Machine", "settings.json"),
		// windsurf default configuration path.
		filepath.Join(home, ".windsurf-server", "data", "Machine", "settings.json"),
		// vscodium default configuration path.
		filepath.Join(home, ".vscodium-server", "data", "Machine", "settings.json"),
	} {
		_, err := fs.Stat(configPath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return xerrors.Errorf("stat %q: %w", configPath, err)
			}

			// Create new settings file with only Coder's essential settings
			m := make(map[string]interface{})
			mutate(m)
			data, err := json.MarshalIndent(m, "", "\t")
			if err != nil {
				return xerrors.Errorf("marshal: %w", err)
			}

			err = fs.MkdirAll(filepath.Dir(configPath), 0o700)
			if err != nil {
				return xerrors.Errorf("mkdir all: %w", err)
			}

			err = afero.WriteFile(fs, configPath, data, 0o600)
			if err != nil {
				return xerrors.Errorf("write %q: %w", configPath, err)
			}
			continue
		}

		data, err := afero.ReadFile(fs, configPath)
		if err != nil {
			return xerrors.Errorf("read %q: %w", configPath, err)
		}
		mapping := map[string]interface{}{}
		err = json.Unmarshal(data, &mapping)
		if err != nil {
			return xerrors.Errorf("unmarshal %q: %w", configPath, err)
		}
		mutate(mapping)
		data, err = json.MarshalIndent(mapping, "", "\t")
		if err != nil {
			return xerrors.Errorf("marshal %q: %w", configPath, err)
		}
		err = afero.WriteFile(fs, configPath, data, 0o600)
		if err != nil {
			return xerrors.Errorf("write %q: %w", configPath, err)
		}
	}
	return nil
}
