package config

import (
	"errors"
	"net/url"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/gitauth"

	_ "embed"
)

//go:embed server.yaml
var defaultServer string

// Server represents a parsed server configuration.
type Server struct {
	GitAuth []*gitauth.Config
}

// ParseServer creates or consumes a server config by path.
// If one does not exist, it will create one. If it fails to create,
// a warning will appear but the server will not fail to start.
// This is to prevent blocking execution on readonly file-systems
// that didn't provide a default config.
func ParseServer(cmd *cobra.Command, accessURL *url.URL, path string) (*Server, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		err = os.WriteFile(path, []byte(defaultServer), 0600)
		if err != nil {
			cmd.Printf("%s Unable to write the default config file: %s", cliui.Styles.Warn.Render("Warning:"), err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		data = []byte(defaultServer)
	}
	var server struct {
		GitAuth []*gitauth.YAML `yaml:"gitauth"`
	}
	err = yaml.Unmarshal(data, &server)
	if err != nil {
		return nil, err
	}
	configs, err := gitauth.ConvertYAML(server.GitAuth, accessURL)
	if err != nil {
		return nil, err
	}
	return &Server{
		GitAuth: configs,
	}, nil
}
