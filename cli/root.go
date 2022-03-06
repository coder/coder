package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/kirsle/configdir"
	"github.com/manifoldco/promptui"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/codersdk"
)

var (
	caret = color.HiBlackString(">")
)

const (
	varGlobalConfig = "global-config"
	varNoOpen       = "no-open"
	varForceTty     = "force-tty"
)

func Root() *cobra.Command {
	cmd := &cobra.Command{
		Use: "coder",
		Long: `    ▄█▀    ▀█▄
     ▄▄ ▀▀▀  █▌   ██▀▀█▄          ▐█
 ▄▄██▀▀█▄▄▄  ██  ██      █▀▀█ ▐█▀▀██ ▄█▀▀█ █▀▀
█▌   ▄▌   ▐█ █▌  ▀█▄▄▄█▌ █  █ ▐█  ██ ██▀▀  █
     ██████▀▄█    ▀▀▀▀   ▀▀▀▀  ▀▀▀▀▀  ▀▀▀▀ ▀
  ` + color.New(color.Underline).Sprint("Self-hosted developer workspaces on your infra") + `

`,
		Example: `
  - Create a project for developers to create workspaces

    ` + color.New(color.FgHiMagenta).Sprint("$ coder projects create <directory>") + `		

  - Create a workspace for a specific project

    ` + color.New(color.FgHiMagenta).Sprint("$ coder workspaces create <project>") + `
	
  - Maintain consistency by updating a workspace

    ` + color.New(color.FgHiMagenta).Sprint("$ coder workspaces update <workspace>"),
	}
	// Customizes the color of headings to make subcommands
	// more visually appealing.
	header := color.New(color.FgHiBlack)
	cmd.SetUsageTemplate(strings.NewReplacer(
		`Usage:`, header.Sprint("Usage:"),
		`Examples:`, header.Sprint("Examples:"),
		`Available Commands:`, header.Sprint("Commands:"),
		`Global Flags:`, header.Sprint("Global Flags:"),
		`Flags:`, header.Sprint("Flags:"),
		`Additional help topics:`, header.Sprint("Additional help:"),
	).Replace(cmd.UsageTemplate()))

	cmd.AddCommand(login())
	cmd.AddCommand(projects())
	cmd.AddCommand(workspaces())
	cmd.AddCommand(users())

	cmd.PersistentFlags().String(varGlobalConfig, configdir.LocalConfig("coder"), "Path to the global `coder` config directory")
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
func currentOrganization(cmd *cobra.Command, client *codersdk.Client) (coderd.Organization, error) {
	orgs, err := client.OrganizationsByUser(cmd.Context(), "me")
	if err != nil {
		return coderd.Organization{}, nil
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

	reader := cmd.InOrStdin()
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(file.Fd())
}

func prompt(cmd *cobra.Command, prompt *promptui.Prompt) (string, error) {
	prompt.Stdin = io.NopCloser(cmd.InOrStdin())
	prompt.Stdout = readWriteCloser{
		Writer: cmd.OutOrStdout(),
	}

	// The prompt library displays defaults in a jarring way for the user
	// by attempting to autocomplete it. This sets no default enabling us
	// to customize the display.
	defaultValue := prompt.Default
	if !prompt.IsConfirm {
		prompt.Default = ""
	}

	// Rewrite the confirm template to remove bold, and fit to the Coder style.
	confirmEnd := fmt.Sprintf("[y/%s] ", color.New(color.Bold).Sprint("N"))
	if prompt.Default == "y" {
		confirmEnd = fmt.Sprintf("[%s/n] ", color.New(color.Bold).Sprint("Y"))
	}
	confirm := color.HiBlackString("?") + ` {{ . }} ` + confirmEnd

	// Customize to remove bold.
	valid := color.HiBlackString("?") + " {{ . }} "
	if defaultValue != "" {
		valid += fmt.Sprintf("(%s) ", defaultValue)
	}

	success := valid
	invalid := valid
	if prompt.IsConfirm {
		success = confirm
		invalid = confirm
	}

	prompt.Templates = &promptui.PromptTemplates{
		Confirm: confirm,
		Success: success,
		Invalid: invalid,
		Valid:   valid,
	}
	oldValidate := prompt.Validate
	if oldValidate != nil {
		// Override the validate function to pass our default!
		prompt.Validate = func(s string) error {
			if s == "" {
				s = defaultValue
			}
			return oldValidate(s)
		}
	}
	value, err := prompt.Run()
	if value == "" && !prompt.IsConfirm {
		value = defaultValue
	}

	return value, err
}

// readWriteCloser fakes reads, writes, and closing!
type readWriteCloser struct {
	io.Reader
	io.Writer
	io.Closer
}
