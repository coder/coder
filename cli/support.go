package cli

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/support"
)

func (r *RootCmd) support() *clibase.Cmd {
	supportCmd := &clibase.Cmd{
		Use:   "support",
		Short: "Commands for troubleshooting issues with a Coder deployment.",
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Hidden: true, // TODO: un-hide once the must-haves from #12160 are completed.
		Children: []*clibase.Cmd{
			r.supportBundle(),
		},
	}
	return supportCmd
}

func (r *RootCmd) supportBundle() *clibase.Cmd {
	var outputPath string
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "bundle <workspace> [<agent>]",
		Short: "Generate a support bundle to troubleshoot issues connecting to a workspace.",
		Long:  `This command generates a file containing detailed troubleshooting information about the Coder deployment and workspace connections. You must specify a single workspace (and optionally an agent name).`,
		Middleware: clibase.Chain(
			clibase.RequireRangeArgs(0, 2),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			var (
				log = slog.Make(sloghuman.Sink(inv.Stderr)).
					Leveled(slog.LevelDebug)
				deps = support.Deps{
					Client: client,
					Log:    log,
				}
			)

			if len(inv.Args) == 0 {
				return xerrors.Errorf("must specify workspace name")
			}
			ws, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("invalid workspace: %w", err)
			}

			deps.WorkspaceID = ws.ID

			agentName := ""
			if len(inv.Args) > 1 {
				agentName = inv.Args[1]
			}

			agt, found := findAgent(agentName, ws.LatestBuild.Resources)
			if !found {
				return xerrors.Errorf("could not find agent named %q for workspace", agentName)
			}

			deps.AgentID = agt.ID

			if outputPath == "" {
				cwd, err := filepath.Abs(".")
				if err != nil {
					return xerrors.Errorf("could not determine current working directory: %w", err)
				}
				fname := fmt.Sprintf("coder-support-%d.zip", time.Now().Unix())
				outputPath = filepath.Join(cwd, fname)
			}

			w, err := os.Create(outputPath)
			if err != nil {
				return xerrors.Errorf("create output file: %w", err)
			}
			zwr := zip.NewWriter(w)
			defer zwr.Close()

			bun, err := support.Run(inv.Context(), &deps)
			if err != nil {
				_ = os.Remove(outputPath) // best effort
				return xerrors.Errorf("create support bundle: %w", err)
			}

			if err := writeBundle(bun, zwr); err != nil {
				_ = os.Remove(outputPath) // best effort
				return xerrors.Errorf("write support bundle to %s: %w", outputPath, err)
			}
			return nil
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:          "output",
			FlagShorthand: "o",
			Env:           "CODER_SUPPORT_BUNDLE_OUTPUT",
			Description:   "File path for writing the generated support bundle. Defaults to coder-support-$(date +%s).zip.",
			Value:         clibase.StringOf(&outputPath),
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
	for k, v := range map[string]any{
		"deployment/buildinfo.json":       src.Deployment.BuildInfo,
		"deployment/config.json":          src.Deployment.Config,
		"deployment/experiments.json":     src.Deployment.Experiments,
		"deployment/health.json":          src.Deployment.HealthReport,
		"network/netcheck_local.json":     src.Network.NetcheckLocal,
		"network/netcheck_remote.json":    src.Network.NetcheckRemote,
		"workspace/workspace.json":        src.Workspace.Workspace,
		"workspace/agent.json":            src.Workspace.Agent,
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

	for k, v := range map[string]string{
		"network/coordinator_debug.html":   src.Network.CoordinatorDebug,
		"network/tailnet_debug.html":       src.Network.TailnetDebug,
		"workspace/build_logs.txt":         humanizeBuildLogs(src.Workspace.BuildLogs),
		"workspace/agent_startup_logs.txt": humanizeAgentLogs(src.Workspace.AgentStartupLogs),
		"workspace/template_file.zip":      string(templateVersionBytes),
		"logs.txt":                         strings.Join(src.Logs, "\n"),
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
