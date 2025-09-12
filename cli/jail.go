package cli

import (
	jailcli "github.com/coder/jail/cli"
	"github.com/coder/serpent"
)

func (r *RootCmd) jail() *serpent.Command {
	var config jailcli.Config

	return &serpent.Command{
		Use:   "jail -- <command>",
		Short: "Monitor and restrict HTTP/HTTPS requests from processes",
		Long: `coder jail creates an isolated network environment for the target process,
intercepting all HTTP/HTTPS traffic through a transparent proxy that enforces
user-defined rules.

Examples:
  # Allow only requests to github.com
  coder jail --allow "github.com" -- curl https://github.com

  # Monitor all requests to specific domains (allow only those)
  coder jail --allow "github.com/api/issues/*" --allow "GET,HEAD github.com" -- npm install

  # Block everything by default (implicit)`,
		Options: serpent.OptionSet{
			{
				Name:        "allow",
				Flag:        "allow",
				Env:         "JAIL_ALLOW",
				Description: "Allow rule (can be specified multiple times). Format: 'pattern' or 'METHOD[,METHOD] pattern'.",
				Value:       serpent.StringArrayOf(&config.AllowStrings),
			},
			{
				Name:        "log-level",
				Flag:        "log-level",
				Env:         "JAIL_LOG_LEVEL",
				Description: "Set log level (error, warn, info, debug).",
				Default:     "warn",
				Value:       serpent.StringOf(&config.LogLevel),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			return jailcli.Run(inv.Context(), config, inv.Args)
		},
	}
}
