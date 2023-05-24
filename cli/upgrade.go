package cli

import (
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) upgrade() *clibase.Cmd {
	var (
		scriptURL string
	)
	cmd := &clibase.Cmd{
		Use:   "upgrade",
		Short: "Upgrade the Coder CLI to match the version of a deployment.",
		Options: clibase.OptionSet{
			{
				Flag:        "install-script-url",
				Description: "Download URL of install script (useful for testing).",
				Value:       clibase.StringOf(&scriptURL),
				Default:     "https://coder.com/install.sh",
				Hidden:      true,
			},
		},
		Handler: func(inv *clibase.Invocation) error {
			var (
				ctx       = inv.Context()
				serverURL = r.clientURL.String()
			)

			var err error
			if serverURL == "" {
				serverURL, err = r.createConfig().URL().Read()
				if err != nil || strings.TrimSpace(serverURL) == "" {
					_, _ = fmt.Fprintln(inv.Stderr, cliui.Styles.Error.Render(
						"No deployment URL provided. You must either login using",
						cliui.Styles.Code.Render("coder login"),
						cliui.Styles.Error.Render("or specify a URL with the"),
						cliui.Styles.Code.Render("--url"),
						cliui.Styles.Error.Render("flag."),
					))

					return xerrors.Errorf("read config url: %w", err)
				}
			}

			uri, err := url.Parse(serverURL)
			if err != nil {
				return xerrors.Errorf("parse url: %w", err)
			}

			client := codersdk.New(uri)
			serverInfo, err := client.BuildInfo(ctx)
			if err != nil {
				return xerrors.Errorf("build info: %w", err)
			}

			version := semver.Canonical(serverInfo.Version)
			version = strings.TrimSuffix(version, "-devel")
			version = strings.TrimPrefix(version, "v")

			if runtime.GOOS == "windows" {
				_, _ = fmt.Fprintln(inv.Stdout,
					cliui.Styles.Code.Render("coder upgrade "),
					"is not currently supported on Windows.\n",
					fmt.Sprintf("Download the installer at %s to upgrade your client.", cliui.Styles.Keyword.Render(windowsInstallerURL(version))),
				)
				return nil
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Detected server version %q, downloading version %q from %s\n", serverInfo.Version, version, scriptURL)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, scriptURL, nil)
			if err != nil {
				return xerrors.Errorf("new http request: %w", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return xerrors.Errorf("fetch install script: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return xerrors.Errorf("unexpected error code %d while fetching install script: %w", resp.StatusCode, err)
			}

			// nolint: gosec
			cmd := exec.Command("sh", "-s", "--", "--version", version)
			// TODO: should output print the script prior to executing it and require
			// the user to confirm they want to run it?
			cmd.Stdin = resp.Body
			cmd.Stdout = inv.Stdout
			cmd.Stderr = inv.Stderr
			err = cmd.Run()
			if err != nil {
				return xerrors.Errorf("run install script: %w", err)
			}

			return nil
		},
	}

	return cmd
}

func windowsInstallerURL(version string) string {
	return fmt.Sprintf(
		"https://github.com/coder/coder/releases/download/v%s/coder_%s_windows_%s_installer.exe",
		version,
		version,
		runtime.GOARCH,
	)
}
