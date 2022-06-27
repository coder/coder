package cli

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/kirsle/configdir"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/codersdk"
)

var (
	caret = cliui.Styles.Prompt.String()

	// Applied as annotations to workspace commands
	// so they display in a separated "help" section.
	workspaceCommand = map[string]string{
		"workspaces": " ",
	}
)

const (
	varURL             = "url"
	varToken           = "token"
	varAgentToken      = "agent-token"
	varAgentURL        = "agent-url"
	varGlobalConfig    = "global-config"
	varNoOpen          = "no-open"
	varForceTty        = "force-tty"
	notLoggedInMessage = "You are not logged in. Try logging in using 'coder login <url>'."

	envSessionToken = "CODER_SESSION_TOKEN"
)

func init() {
	// Customizes the color of headings to make subcommands more visually
	// appealing.
	header := cliui.Styles.Placeholder
	cobra.AddTemplateFunc("usageHeader", func(s string) string {
		return header.Render(s)
	})
}

func Root() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "coder",
		SilenceErrors: true,
		SilenceUsage:  true,
		Long: `Coder — A tool for provisioning self-hosted development environments.
`,
		Example: `  Start a Coder server.
  ` + cliui.Styles.Code.Render("$ coder server") + `

  Get started by creating a template from an example.
  ` + cliui.Styles.Code.Render("$ coder templates init"),
	}

	cmd.AddCommand(
		configSSH(),
		create(),
		deleteWorkspace(),
		dotfiles(),
		gitssh(),
		list(),
		login(),
		logout(),
		parameters(),
		portForward(),
		publickey(),
		resetPassword(),
		schedules(),
		server(),
		show(),
		ssh(),
		start(),
		state(),
		stop(),
		templates(),
		update(),
		users(),
		versionCmd(),
		wireguardPortForward(),
		workspaceAgent(),
	)

	cmd.SetUsageTemplate(usageTemplate())

	cmd.PersistentFlags().String(varURL, "", "Specify the URL to your deployment.")
	cliflag.String(cmd.PersistentFlags(), varToken, "", envSessionToken, "", fmt.Sprintf("Specify an authentication token. For security reasons setting %s is preferred.", envSessionToken))
	cliflag.String(cmd.PersistentFlags(), varAgentToken, "", "CODER_AGENT_TOKEN", "", "Specify an agent authentication token.")
	_ = cmd.PersistentFlags().MarkHidden(varAgentToken)
	cliflag.String(cmd.PersistentFlags(), varAgentURL, "", "CODER_AGENT_URL", "", "Specify the URL for an agent to access your deployment.")
	_ = cmd.PersistentFlags().MarkHidden(varAgentURL)
	cliflag.String(cmd.PersistentFlags(), varGlobalConfig, "", "CODER_CONFIG_DIR", configdir.LocalConfig("coderv2"), "Specify the path to the global `coder` config directory.")
	cmd.PersistentFlags().Bool(varForceTty, false, "Force the `coder` command to run as if connected to a TTY.")
	_ = cmd.PersistentFlags().MarkHidden(varForceTty)
	cmd.PersistentFlags().Bool(varNoOpen, false, "Block automatically opening URLs in the browser.")
	_ = cmd.PersistentFlags().MarkHidden(varNoOpen)

	return cmd
}

// versionCmd prints the coder version
func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Show coder version",
		Example: "coder version",
		RunE: func(cmd *cobra.Command, args []string) error {
			var str strings.Builder
			_, _ = str.WriteString(fmt.Sprintf("Coder %s", buildinfo.Version()))
			buildTime, valid := buildinfo.Time()
			if valid {
				_, _ = str.WriteString(" " + buildTime.Format(time.UnixDate))
			}
			_, _ = str.WriteString("\r\n" + buildinfo.ExternalURL() + "\r\n")

			_, _ = fmt.Fprint(cmd.OutOrStdout(), str.String())
			return nil
		},
	}
}

// createClient returns a new client from the command context.
// It reads from global configuration files if flags are not set.
func createClient(cmd *cobra.Command) (*codersdk.Client, error) {
	root := createConfig(cmd)
	rawURL, err := cmd.Flags().GetString(varURL)
	if err != nil || rawURL == "" {
		rawURL, err = root.URL().Read()
		if err != nil {
			// If the configuration files are absent, the user is logged out
			if os.IsNotExist(err) {
				return nil, xerrors.New(notLoggedInMessage)
			}
			return nil, err
		}
	}
	serverURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	token, err := cmd.Flags().GetString(varToken)
	if err != nil || token == "" {
		token, err = root.Session().Read()
		if err != nil {
			// If the configuration files are absent, the user is logged out
			if os.IsNotExist(err) {
				return nil, xerrors.New(notLoggedInMessage)
			}
			return nil, err
		}
	}
	client := codersdk.New(serverURL)
	client.SessionToken = strings.TrimSpace(token)
	return client, nil
}

// createAgentClient returns a new client from the command context.
// It works just like createClient, but uses the agent token and URL instead.
func createAgentClient(cmd *cobra.Command) (*codersdk.Client, error) {
	rawURL, err := cmd.Flags().GetString(varAgentURL)
	if err != nil {
		return nil, err
	}
	serverURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	token, err := cmd.Flags().GetString(varAgentToken)
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

// namedWorkspace fetches and returns a workspace by an identifier, which may be either
// a bare name (for a workspace owned by the current user) or a "user/workspace" combination,
// where user is either a username or UUID.
func namedWorkspace(cmd *cobra.Command, client *codersdk.Client, identifier string) (codersdk.Workspace, error) {
	parts := strings.Split(identifier, "/")

	var owner, name string
	switch len(parts) {
	case 1:
		owner = codersdk.Me
		name = parts[0]
	case 2:
		owner = parts[0]
		name = parts[1]
	default:
		return codersdk.Workspace{}, xerrors.Errorf("invalid workspace name: %q", identifier)
	}

	return client.WorkspaceByOwnerAndName(cmd.Context(), owner, name, codersdk.WorkspaceOptions{})
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

// isTTYOut returns whether the passed reader is a TTY or not.
// This accepts a reader to work with Cobra's "OutOrStdout"
// function for simple testing.
func isTTYOut(cmd *cobra.Command) bool {
	// If the `--force-tty` command is available, and set,
	// assume we're in a tty. This is primarily for cases on Windows
	// where we may not be able to reliably detect this automatically (ie, tests)
	forceTty, err := cmd.Flags().GetBool(varForceTty)
	if forceTty && err == nil {
		return true
	}
	file, ok := cmd.OutOrStdout().(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(file.Fd())
}

func usageTemplate() string {
	// usageHeader is defined in init().
	return `{{usageHeader "Usage:"}}
{{- if .Runnable}}
  {{.UseLine}}
{{end}}
{{- if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]
{{end}}

{{- if gt (len .Aliases) 0}}
{{usageHeader "Aliases:"}}
  {{.NameAndAliases}}
{{end}}

{{- if .HasExample}}
{{usageHeader "Get Started:"}}
{{.Example}}
{{end}}

{{- if .HasAvailableSubCommands}}
{{usageHeader "Commands:"}}
  {{- range .Commands}}
    {{- if (or (and .IsAvailableCommand (eq (len .Annotations) 0)) (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}
    {{- end}}
  {{- end}}
{{end}}

{{- if and (not .HasParent) .HasAvailableSubCommands}}
{{usageHeader "Workspace Commands:"}}
  {{- range .Commands}}
    {{- if (and .IsAvailableCommand (ne (index .Annotations "workspaces") ""))}}
  {{rpad .Name .NamePadding }} {{.Short}}
    {{- end}}
  {{- end}}
{{end}}

{{- if .HasAvailableLocalFlags}}
{{usageHeader "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}

{{- if .HasAvailableInheritedFlags}}
{{usageHeader "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}

{{- if .HasHelpSubCommands}}
{{usageHeader "Additional help topics:"}}
  {{- range .Commands}}
    {{- if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}
    {{- end}}
  {{- end}}
{{end}}

{{- if .HasAvailableSubCommands}}
Use "{{.CommandPath}} [command] --help" for more information about a command.
{{end}}`
}

// FormatCobraError colorizes and adds "--help" docs to cobra commands.
func FormatCobraError(err error, cmd *cobra.Command) string {
	helpErrMsg := fmt.Sprintf("Run '%s --help' for usage.", cmd.CommandPath())
	return cliui.Styles.Error.Render(err.Error() + "\n" + helpErrMsg)
}
