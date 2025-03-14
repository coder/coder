package gitauth
import (
	"fmt"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"github.com/adrg/xdg"
	"github.com/spf13/afero"
)
// OverrideVSCodeConfigs overwrites a few properties to consume
// GIT_ASKPASS from the host instead of VS Code-specific authentication.
func OverrideVSCodeConfigs(fs afero.Fs) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	mutate := func(m map[string]interface{}) {
		// This prevents VS Code from overriding GIT_ASKPASS, which
		// we use to automatically authenticate Git providers.
		m["git.useIntegratedAskPass"] = false
		// This prevents VS Code from using it's own GitHub authentication
		// which would circumvent cloning with Coder-configured providers.
		m["github.gitAuthentication"] = false
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
				return fmt.Errorf("stat %q: %w", configPath, err)
			}
			m := map[string]interface{}{}
			mutate(m)
			data, err := json.MarshalIndent(m, "", "\t")
			if err != nil {
				return fmt.Errorf("marshal: %w", err)
			}
			err = fs.MkdirAll(filepath.Dir(configPath), 0o700)
			if err != nil {
				return fmt.Errorf("mkdir all: %w", err)
			}
			err = afero.WriteFile(fs, configPath, data, 0o600)
			if err != nil {
				return fmt.Errorf("write %q: %w", configPath, err)
			}
			continue
		}
		data, err := afero.ReadFile(fs, configPath)
		if err != nil {
			return fmt.Errorf("read %q: %w", configPath, err)
		}
		mapping := map[string]interface{}{}
		err = json.Unmarshal(data, &mapping)
		if err != nil {
			return fmt.Errorf("unmarshal %q: %w", configPath, err)
		}
		mutate(mapping)
		data, err = json.MarshalIndent(mapping, "", "\t")
		if err != nil {
			return fmt.Errorf("marshal %q: %w", configPath, err)
		}
		err = afero.WriteFile(fs, configPath, data, 0o600)
		if err != nil {
			return fmt.Errorf("write %q: %w", configPath, err)
		}
	}
	return nil
}
