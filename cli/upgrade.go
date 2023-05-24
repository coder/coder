package cli

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) upgrade() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:     "upgrade",
		Short:   "Upgrade the Coder CLI to match the version of a deployment.",
		Options: clibase.OptionSet{},
		Handler: func(inv *clibase.Invocation) error {
			var (
				ctx  = inv.Context()
				curl = r.clientURL.String()
			)

			var err error
			if curl == "" {
				curl, err = r.createConfig().URL().Read()
				if err != nil {
					return xerrors.Errorf("read config url: %w", err)
				}
			}

			uri, err := url.Parse(curl)
			if err != nil {
				return xerrors.Errorf("parse url: %w", err)
			}

			client := codersdk.New(uri)
			serverInfo, err := client.BuildInfo(ctx)
			if err != nil {
				return xerrors.Errorf("build info: %w", err)
			}

			resp, err := http.Get("https://coder.com/install.sh")
			if err != nil {
				return xerrors.Errorf("fetch install script: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return xerrors.Errorf("unexpected error code %d while fetching install script: %w", resp.StatusCode, err)
			}

			version := semver.Canonical(serverInfo.Version)
			version = strings.TrimSuffix(version, "-devel")
			version = strings.TrimPrefix(version, "v")

			// nolint: gosec
			cmd := exec.Command("sh", "-s", "--", "--version", version)
			cmd.Stdin = resp.Body
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err = cmd.Run()
			if err != nil {
				return xerrors.Errorf("run install script: %w", err)
			}

			return nil
		},
	}

	return cmd
}
