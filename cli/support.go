package cli

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/support"
	"github.com/coder/serpent"
)

func (r *RootCmd) support() *serpent.Command {
	supportCmd := &serpent.Command{
		Use:   "support",
		Short: "Commands for troubleshooting issues with a Coder deployment.",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.supportBundle(),
		},
	}
	return supportCmd
}

var supportBundleBlurb = cliui.Bold("This will collect the following information:\n") +
	`  - Coder deployment version
  - Coder deployment Configuration (sanitized), including enabled experiments
  - Coder deployment health snapshot
  - Coder deployment Network troubleshooting information
  - Workspace configuration, parameters, and build logs
  - Template version and source code for the given workspace
  - Agent details (with environment variable sanitized)
  - Agent network diagnostics
  - Agent logs
` + cliui.Bold("Note: ") +
	cliui.Wrap("While we try to sanitize sensitive data from support bundles, we cannot guarantee that they do not contain information that you or your organization may consider sensitive.\n") +
	cliui.Bold("Please confirm that you will:\n") +
	"  - Review the support bundle before distribution\n" +
	"  - Only distribute it via trusted channels\n" +
	cliui.Bold("Continue? ")

func (r *RootCmd) supportBundle() *serpent.Command {
	var outputPath string
	var coderURLOverride string
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "bundle <workspace> [<agent>]",
		Short: "Generate a support bundle to troubleshoot issues connecting to a workspace.",
		Long:  `This command generates a file containing detailed troubleshooting information about the Coder deployment and workspace connections. You must specify a single workspace (and optionally an agent name).`,
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(0, 2),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			var cliLogBuf bytes.Buffer
			cliLogW := sloghuman.Sink(&cliLogBuf)
			cliLog := slog.Make(cliLogW).Leveled(slog.LevelDebug)
			if r.verbose {
				cliLog = cliLog.AppendSinks(sloghuman.Sink(inv.Stderr))
			}
			ans, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      supportBundleBlurb,
				Secret:    false,
				IsConfirm: true,
			})
			if err != nil || ans != cliui.ConfirmYes {
				return err
			}
			if skip, _ := inv.ParsedFlags().GetBool("yes"); skip {
				cliLog.Debug(inv.Context(), "user auto-confirmed")
			} else {
				cliLog.Debug(inv.Context(), "user confirmed manually", slog.F("answer", ans))
			}

			vi := defaultVersionInfo()
			cliLog.Debug(inv.Context(), "version info",
				slog.F("version", vi.Version),
				slog.F("build_time", vi.BuildTime),
				slog.F("external_url", vi.ExternalURL),
				slog.F("slim", vi.Slim),
				slog.F("agpl", vi.AGPL),
				slog.F("boring_crypto", vi.BoringCrypto),
			)
			cliLog.Debug(inv.Context(), "invocation", slog.F("args", strings.Join(os.Args, " ")))

			// Check if we're running inside a workspace
			if val, found := os.LookupEnv("CODER"); found && val == "true" {
				_, _ = fmt.Fprintln(inv.Stderr, "Running inside Coder workspace; this can affect results!")
				cliLog.Debug(inv.Context(), "running inside coder workspace")
			}

			if coderURLOverride != "" && coderURLOverride != client.URL.String() {
				u, err := url.Parse(coderURLOverride)
				if err != nil {
					return xerrors.Errorf("invalid value for Coder URL override: %w", err)
				}
				_, _ = fmt.Fprintf(inv.Stderr, "Overrode Coder URL to %q; this can affect results!\n", coderURLOverride)
				cliLog.Debug(inv.Context(), "coder url overridden", slog.F("url", coderURLOverride))
				client.URL = u
			}

			var (
				wsID  uuid.UUID
				agtID uuid.UUID
			)

			if len(inv.Args) == 0 {
				cliLog.Warn(inv.Context(), "no workspace specified")
				_, _ = fmt.Fprintln(inv.Stderr, "Warning: no workspace specified. This will result in incomplete information.")
			} else {
				ws, err := namedWorkspace(inv.Context(), client, inv.Args[0])
				if err != nil {
					return xerrors.Errorf("invalid workspace: %w", err)
				}
				cliLog.Debug(inv.Context(), "found workspace",
					slog.F("workspace_name", ws.Name),
					slog.F("workspace_id", ws.ID),
				)
				wsID = ws.ID
				agentName := ""
				if len(inv.Args) > 1 {
					agentName = inv.Args[1]
				}

				agt, found := findAgent(agentName, ws.LatestBuild.Resources)
				if !found {
					cliLog.Warn(inv.Context(), "could not find agent in workspace", slog.F("agent_name", agentName))
				} else {
					cliLog.Debug(inv.Context(), "found workspace agent",
						slog.F("agent_name", agt.Name),
						slog.F("agent_id", agt.ID),
					)
					agtID = agt.ID
				}
			}

			if outputPath == "" {
				cwd, err := filepath.Abs(".")
				if err != nil {
					return xerrors.Errorf("could not determine current working directory: %w", err)
				}
				fname := fmt.Sprintf("coder-support-%d.zip", time.Now().Unix())
				outputPath = filepath.Join(cwd, fname)
			}
			cliLog.Debug(inv.Context(), "output path", slog.F("path", outputPath))

			w, err := os.Create(outputPath)
			if err != nil {
				return xerrors.Errorf("create output file: %w", err)
			}
			zwr := zip.NewWriter(w)
			defer zwr.Close()

			clientLog := slog.Make().Leveled(slog.LevelDebug)
			if r.verbose {
				clientLog.AppendSinks(sloghuman.Sink(inv.Stderr))
			}
			deps := support.Deps{
				Client: client,
				// Support adds a sink so we don't need to supply one ourselves.
				Log:         clientLog,
				WorkspaceID: wsID,
				AgentID:     agtID,
			}

			bun, err := support.Run(inv.Context(), &deps)
			if err != nil {
				_ = os.Remove(outputPath) // best effort
				return xerrors.Errorf("create support bundle: %w", err)
			}
			bun.CLILogs = cliLogBuf.Bytes()

			if err := writeBundle(bun, zwr); err != nil {
				_ = os.Remove(outputPath) // best effort
				return xerrors.Errorf("write support bundle to %s: %w", outputPath, err)
			}
			_, _ = fmt.Fprintln(inv.Stderr, "Wrote support bundle to "+outputPath)
			return nil
		},
	}
	cmd.Options = serpent.OptionSet{
		cliui.SkipPromptOption(),
		{
			Flag:          "output-file",
			FlagShorthand: "O",
			Env:           "CODER_SUPPORT_BUNDLE_OUTPUT_FILE",
			Description:   "File path for writing the generated support bundle. Defaults to coder-support-$(date +%s).zip.",
			Value:         serpent.StringOf(&outputPath),
		},
		{
			Flag:        "url-override",
			Env:         "CODER_SUPPORT_BUNDLE_URL_OVERRIDE",
			Description: "Override the URL to your Coder deployment. This may be useful, for example, if you need to troubleshoot a specific Coder replica.",
			Value:       serpent.StringOf(&coderURLOverride),
		},
	}

	return cmd
}

func findAgent(agentName string, haystack []codersdk.WorkspaceResource) (*codersdk.WorkspaceAgent, bool) {
	for _, res := range haystack {
		for _, agt := range res.Agents {
			if agentName == "" {
				// just return the first
				return &agt, true
			}
			if agt.Name == agentName {
				return &agt, true
			}
		}
	}
	return nil, false
}

func writeBundle(src *support.Bundle, dest *zip.Writer) error {
	// We JSON-encode the following:
	for k, v := range map[string]any{
		"deployment/buildinfo.json":       src.Deployment.BuildInfo,
		"deployment/config.json":          src.Deployment.Config,
		"deployment/experiments.json":     src.Deployment.Experiments,
		"deployment/health.json":          src.Deployment.HealthReport,
		"network/netcheck.json":           src.Network.Netcheck,
		"workspace/workspace.json":        src.Workspace.Workspace,
		"agent/agent.json":                src.Agent.Agent,
		"agent/listening_ports.json":      src.Agent.ListeningPorts,
		"agent/manifest.json":             src.Agent.Manifest,
		"agent/peer_diagnostics.json":     src.Agent.PeerDiagnostics,
		"agent/ping_result.json":          src.Agent.PingResult,
		"workspace/template.json":         src.Workspace.Template,
		"workspace/template_version.json": src.Workspace.TemplateVersion,
		"workspace/parameters.json":       src.Workspace.Parameters,
	} {
		f, err := dest.Create(k)
		if err != nil {
			return xerrors.Errorf("create file %q in archive: %w", k, err)
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "    ")
		if err := enc.Encode(v); err != nil {
			return xerrors.Errorf("write json to %q: %w", k, err)
		}
	}

	templateVersionBytes, err := base64.StdEncoding.DecodeString(src.Workspace.TemplateFileBase64)
	if err != nil {
		return xerrors.Errorf("decode template zip from base64")
	}

	// The below we just write as we have them:
	for k, v := range map[string]string{
		"network/coordinator_debug.html": src.Network.CoordinatorDebug,
		"network/tailnet_debug.html":     src.Network.TailnetDebug,
		"workspace/build_logs.txt":       humanizeBuildLogs(src.Workspace.BuildLogs),
		"agent/logs.txt":                 string(src.Agent.Logs),
		"agent/agent_magicsock.html":     string(src.Agent.AgentMagicsockHTML),
		"agent/client_magicsock.html":    string(src.Agent.ClientMagicsockHTML),
		"agent/startup_logs.txt":         humanizeAgentLogs(src.Agent.StartupLogs),
		"agent/prometheus.txt":           string(src.Agent.Prometheus),
		"workspace/template_file.zip":    string(templateVersionBytes),
		"logs.txt":                       strings.Join(src.Logs, "\n"),
		"cli_logs.txt":                   string(src.CLILogs),
	} {
		f, err := dest.Create(k)
		if err != nil {
			return xerrors.Errorf("create file %q in archive: %w", k, err)
		}
		if _, err := f.Write([]byte(v)); err != nil {
			return xerrors.Errorf("write file %q in archive: %w", k, err)
		}
	}
	if err := dest.Close(); err != nil {
		return xerrors.Errorf("close zip file: %w", err)
	}
	return nil
}

func humanizeAgentLogs(ls []codersdk.WorkspaceAgentLog) string {
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 2, 1, ' ', 0)
	for _, l := range ls {
		_, _ = fmt.Fprintf(tw, "%s\t[%s]\t%s\n",
			l.CreatedAt.Format("2006-01-02 15:04:05.000"), // for consistency with slog
			string(l.Level),
			l.Output,
		)
	}
	_ = tw.Flush()
	return buf.String()
}

func humanizeBuildLogs(ls []codersdk.ProvisionerJobLog) string {
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 2, 1, ' ', 0)
	for _, l := range ls {
		_, _ = fmt.Fprintf(tw, "%s\t[%s]\t%s\t%s\t%s\n",
			l.CreatedAt.Format("2006-01-02 15:04:05.000"), // for consistency with slog
			string(l.Level),
			string(l.Source),
			l.Stage,
			l.Output,
		)
	}
	_ = tw.Flush()
	return buf.String()
}
