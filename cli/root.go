package cli

import (
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/kirsle/configdir"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/codersdk"
)

var (
	caret = cliui.Styles.Prompt.String()
)

const (
	varGlobalConfig = "global-config"
	varNoOpen       = "no-open"
	varForceTty     = "force-tty"
)

func Root() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "coder",
		Version:       buildinfo.Version(),
		SilenceErrors: true,
		SilenceUsage:  true,
		Long: `    ▄█▀    ▀█▄
     ▄▄ ▀▀▀  █▌   ██▀▀█▄          ▐█
 ▄▄██▀▀█▄▄▄  ██  ██      █▀▀█ ▐█▀▀██ ▄█▀▀█ █▀▀
█▌   ▄▌   ▐█ █▌  ▀█▄▄▄█▌ █  █ ▐█  ██ ██▀▀  █
     ██████▀▄█    ▀▀▀▀   ▀▀▀▀  ▀▀▀▀▀  ▀▀▀▀ ▀
  ` + lipgloss.NewStyle().Underline(true).Render("Self-hosted developer workspaces on your infra") + `

`,
		Example: cliui.Styles.Paragraph.Render(`Start Coder in "dev" mode. This dev-mode requires no further setup, and your local `+cliui.Styles.Code.Render("coder")+` CLI will be authenticated to talk to it. This makes it easy to experiment with Coder.`) + `

    ` + cliui.Styles.Code.Render("$ coder server --dev") + `
	` + cliui.Styles.Paragraph.Render("Get started by creating a template from an example.") + `

    ` + cliui.Styles.Code.Render("$ coder templates init"),
	}
	// Customizes the color of headings to make subcommands
	// more visually appealing.
	header := cliui.Styles.Placeholder
	cmd.SetUsageTemplate(strings.NewReplacer(
		`Usage:`, header.Render("Usage:"),
		`Examples:`, header.Render("Examples:"),
		`Available Commands:`, header.Render("Commands:"),
		`Global Flags:`, header.Render("Global Flags:"),
		`Flags:`, header.Render("Flags:"),
		`Additional help topics:`, header.Render("Additional help:"),
	).Replace(cmd.UsageTemplate()))
	cmd.SetVersionTemplate(versionTemplate())

	cmd.AddCommand(
		configSSH(),
		server(),
		login(),
		parameters(),
		templates(),
		users(),
		workspaces(),
		ssh(),
		workspaceTunnel(),
		gitssh(),
		publickey(),
		workspaceAgent(),
	)

	cmd.PersistentFlags().String(varGlobalConfig, configdir.LocalConfig("coderv2"), "Path to the global `coder` config directory")
	cmd.PersistentFlags().Bool(varForceTty, false, "Force the `coder` command to run as if connected to a TTY")
	err := cmd.PersistentFlags().MarkHidden(varForceTty)
	if err != nil {
		// This should never return an error, because we just added the `--force-tty`` flag prior to calling MarkHidden.
		panic(err)
	}
	cmd.PersistentFlags().Bool(varNoOpen, false, "Block automatically opening URLs in the browser.")
	err = cmd.PersistentFlags().MarkHidden(varNoOpen)
	if err != nil {
		panic(err)
	}

	return cmd
}

// createClient returns a new client from the command context.
// The configuration directory will be read from the global flag.
func createClient(cmd *cobra.Command) (*codersdk.Client, error) {
	root := createConfig(cmd)
	rawURL, err := root.URL().Read()
	if err != nil {
		return nil, err
	}
	serverURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	token, err := root.Session().Read()
	if err != nil {
		return nil, err
	}
	client := codersdk.New(serverURL)
	client.SessionToken = token
	return client, nil
}

// currentOrganization returns the currently active organization for the authenticated user.
func currentOrganization(cmd *cobra.Command, client *codersdk.Client) (codersdk.Organization, error) {
	orgs, err := client.OrganizationsByUser(cmd.Context(), codersdk.Me)
	if err != nil {
		return codersdk.Organization{}, nil
	}
	// For now, we won't use the config to set this.
	// Eventually, we will support changing using "coder switch <org>"
	return orgs[0], nil
}

// createConfig consumes the global configuration flag to produce a config root.
func createConfig(cmd *cobra.Command) config.Root {
	globalRoot, err := cmd.Flags().GetString(varGlobalConfig)
	if err != nil {
		panic(err)
	}
	return config.Root(globalRoot)
}

// isTTY returns whether the passed reader is a TTY or not.
// This accepts a reader to work with Cobra's "InOrStdin"
// function for simple testing.
func isTTY(cmd *cobra.Command) bool {
	// If the `--force-tty` command is available, and set,
	// assume we're in a tty. This is primarily for cases on Windows
	// where we may not be able to reliably detect this automatically (ie, tests)
	forceTty, err := cmd.Flags().GetBool(varForceTty)
	if forceTty && err == nil {
		return true
	}
	file, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(file.Fd())
}

func versionTemplate() string {
	template := `Coder {{printf "%s" .Version}}`
	buildTime, valid := buildinfo.Time()
	if valid {
		template += " " + buildTime.Format(time.UnixDate)
	}
	template += "\r\n" + buildinfo.ExternalURL()
	template += "\r\n"
	return template
}
