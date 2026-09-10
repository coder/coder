package cli

import (
	"os"
	"os/exec"
	"strings"

	"golang.org/x/xerrors"

	agplcli "github.com/coder/coder/v2/cli"
	"github.com/coder/serpent"
)

func (r *RootCmd) expLaunch() *serpent.Command {
	var gatewayURL, provider, model string

	return &serpent.Command{
		Use:   "launch <client> [-- <client args>]",
		Short: "Launch a coding agent configured to use AI Gateway (experimental)",
		Long: "Launches the client with its API pointed at this deployment's AI Gateway,\n" +
			"authenticated with your Coder session token.\n\n" +
			"Supported clients: claude.\n\n" +
			"Flags belonging to the client must come after `--`; flags before it are\n" +
			"parsed by Coder and rejected if unknown.",
		Middleware: serpent.RequireRangeArgs(1, -1),
		Options: serpent.OptionSet{
			{
				Flag:        "gateway-url",
				Env:         "CODER_AI_GATEWAY_URL",
				Description: "AI Gateway URL. Defaults to this deployment's gateway.",
				Value:       serpent.StringOf(&gatewayURL),
			},
			{
				Flag:        "provider",
				Default:     "anthropic",
				Description: "AI Gateway provider to route through.",
				Value:       serpent.StringOf(&provider),
			},
			{
				Flag:        "model",
				Description: "Model to pass to the client.",
				Value:       serpent.StringOf(&model),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			if inv.Args[0] != "claude" {
				return xerrors.Errorf("unknown client %q; supported: claude", inv.Args[0])
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			token := client.SessionToken()
			if token == "" {
				return xerrors.New("not logged in; run 'coder login'")
			}
			if gatewayURL == "" {
				gatewayURL = client.URL.JoinPath("/api/v2/ai-gateway").String()
			}
			gatewayURL = strings.TrimRight(gatewayURL, "/")

			bin, err := exec.LookPath("claude")
			if err != nil {
				return xerrors.Errorf("claude not found in PATH; install: https://docs.claude.com/en/docs/claude-code/setup")
			}

			var args []string
			if model != "" {
				args = append(args, "--model", model)
			}
			args = append(args, inv.Args[1:]...)

			//nolint:gosec // The binary is resolved from PATH by design.
			cmd := exec.CommandContext(inv.Context(), bin, args...)
			cmd.Env = append(scrubbedEnv(),
				"ANTHROPIC_BASE_URL="+gatewayURL+"/"+provider,
				"ANTHROPIC_AUTH_TOKEN="+token,
			)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = inv.Stdin, inv.Stdout, inv.Stderr

			if err := cmd.Run(); err != nil {
				exitErr := &exec.ExitError{}
				if xerrors.As(err, &exitErr) {
					return agplcli.ExitError(exitErr.ExitCode(), nil)
				}
				return err
			}
			return nil
		},
	}
}

// scrubbedEnv returns the environment without the variables that would
// re-route the client away from the gateway.
func scrubbedEnv() []string {
	all := os.Environ()
	env := make([]string, 0, len(all))
	for _, kv := range all {
		if strings.HasPrefix(kv, "ANTHROPIC_") || strings.HasPrefix(kv, "CLAUDE_CODE_USE_") {
			continue
		}
		env = append(env, kv)
	}
	return env
}
