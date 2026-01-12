package cli

import (
	"archive/zip"
	"bytes"
	"context"
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

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
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
  - Coder deployment stats (aggregated workspace/session metrics)
  - Entitlements (if available)
  - Health settings (dismissed healthchecks)
  - Coder deployment Network troubleshooting information
  - Workspace list accessible to the user (sanitized)
  - Workspace configuration, parameters, and build logs
  - Template version and source code for the given workspace
  - Agent details (with environment variable sanitized)
  - Agent network diagnostics
  - Agent logs
  - License status
  - pprof profiling data (if --pprof is enabled)
` + cliui.Bold("Note: ") +
	cliui.Wrap("While we try to sanitize sensitive data from support bundles, we cannot guarantee that they do not contain information that you or your organization may consider sensitive.\n") +
	cliui.Bold("Please confirm that you will:\n") +
	"  - Review the support bundle before distribution\n" +
	"  - Only distribute it via trusted channels\n" +
	cliui.Bold("Continue? ")

func (r *RootCmd) supportBundle() *serpent.Command {
	var outputPath string
	var coderURLOverride string
	var workspacesTotalCap64 int64 = 10
	var templateName string
	var pprof bool
	cmd := &serpent.Command{
		Use:   "bundle <workspace> [<agent>]",
		Short: "Generate a support bundle to troubleshoot issues connecting to a workspace.",
		Long:  `This command generates a file containing detailed troubleshooting information about the Coder deployment and workspace connections. You must specify a single workspace (and optionally an agent name).`,
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(0, 2),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
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
				cliui.Warn(inv.Stderr, "Running inside Coder workspace; this can affect results!")
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
				wsID       uuid.UUID
				agtID      uuid.UUID
				templateID uuid.UUID
			)

			if len(inv.Args) == 0 {
				cliLog.Warn(inv.Context(), "no workspace specified")
				cliui.Warn(inv.Stderr, "No workspace specified. This will result in incomplete information.")
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

			// Resolve template by name if provided (captures active version)
			// Fallback: if canonical name lookup fails, match DisplayName (case-insensitive).
			if templateName != "" {
				id, err := resolveTemplateID(inv.Context(), client, templateName)
				if err != nil {
					return err
				}
				templateID = id
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
			if pprof {
				_, _ = fmt.Fprintln(inv.Stderr, "pprof data collection will take approximately 30 seconds...")
			}

			deps := support.Deps{
				Client: client,
				// Support adds a sink so we don't need to supply one ourselves.
				Log:                clientLog,
				WorkspaceID:        wsID,
				AgentID:            agtID,
				WorkspacesTotalCap: int(workspacesTotalCap64),
				TemplateID:         templateID,
				CollectPprof:       pprof,
			}

			bun, err := support.Run(inv.Context(), &deps)
			if err != nil {
				_ = os.Remove(outputPath) // best effort
				return xerrors.Errorf("create support bundle: %w", err)
			}

			summarizeBundle(inv, bun)
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
		{
			Flag:        "workspaces-total-cap",
			Env:         "CODER_SUPPORT_BUNDLE_WORKSPACES_TOTAL_CAP",
			Description: "Maximum number of workspaces to include in the support bundle. Set to 0 or negative value to disable the cap. Defaults to 10.",
			Value:       serpent.Int64Of(&workspacesTotalCap64),
		},
		{
			Flag:        "template",
			Env:         "CODER_SUPPORT_BUNDLE_TEMPLATE",
			Description: "Template name to include in the support bundle. Use org_name/template_name if template name is reused across multiple organizations.",
			Value:       serpent.StringOf(&templateName),
		},
		{
			Flag:        "pprof",
			Env:         "CODER_SUPPORT_BUNDLE_PPROF",
			Description: "Collect pprof profiling data from the Coder server and agent. Requires Coder server version 2.28.0 or newer.",
			Value:       serpent.BoolOf(&pprof),
		},
	}

	return cmd
}

// Resolve a template to its ID, supporting:
// - org/name form
// - slug or display name match (case-insensitive) across all memberships
func resolveTemplateID(ctx context.Context, client *codersdk.Client, templateArg string) (uuid.UUID, error) {
	orgPart := ""
	namePart := templateArg
	if slash := strings.IndexByte(templateArg, '/'); slash > 0 && slash < len(templateArg)-1 {
		orgPart = templateArg[:slash]
		namePart = templateArg[slash+1:]
	}

	resolveInOrg := func(orgID uuid.UUID) (codersdk.Template, bool, error) {
		if t, err := client.TemplateByName(ctx, orgID, namePart); err == nil {
			return t, true, nil
		}
		tpls, err := client.TemplatesByOrganization(ctx, orgID)
		if err != nil {
			return codersdk.Template{}, false, nil
		}
		for _, t := range tpls {
			if strings.EqualFold(t.Name, namePart) || strings.EqualFold(t.DisplayName, namePart) {
				return t, true, nil
			}
		}
		return codersdk.Template{}, false, nil
	}

	if orgPart != "" {
		org, err := client.OrganizationByName(ctx, orgPart)
		if err != nil {
			return uuid.Nil, xerrors.Errorf("get organization %q: %w", orgPart, err)
		}
		t, found, err := resolveInOrg(org.ID)
		if err != nil {
			return uuid.Nil, err
		}
		if !found {
			return uuid.Nil, xerrors.Errorf("template %q not found in organization %q", namePart, orgPart)
		}
		return t.ID, nil
	}

	orgs, err := client.OrganizationsByUser(ctx, codersdk.Me)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("get organizations: %w", err)
	}
	if len(orgs) == 1 {
		t, found, err := resolveInOrg(orgs[0].ID)
		if err != nil {
			return uuid.Nil, err
		}
		if !found {
			return uuid.Nil, xerrors.Errorf("template %q not found in your organization", namePart)
		}
		return t.ID, nil
	}

	var (
		foundTpl  codersdk.Template
		foundOrgs []string
	)
	for _, org := range orgs {
		if t, found, err := resolveInOrg(org.ID); err == nil && found {
			if len(foundOrgs) == 0 {
				foundTpl = t
			}
			foundOrgs = append(foundOrgs, org.Name)
		}
	}
	switch len(foundOrgs) {
	case 0:
		return uuid.Nil, xerrors.Errorf("template %q not found in your organizations", namePart)
	case 1:
		return foundTpl.ID, nil
	default:
		return uuid.Nil, xerrors.Errorf(
			"template %q found in multiple organizations (%s); use --template \"<org_name/%s>\" to target desired template.",
			namePart,
			strings.Join(foundOrgs, ", "),
			namePart,
		)
	}
}

// summarizeBundle makes a best-effort attempt to write a short summary
// of the support bundle to the user's terminal.
func summarizeBundle(inv *serpent.Invocation, bun *support.Bundle) {
	if bun == nil {
		cliui.Error(inv.Stdout, "No support bundle generated!")
		return
	}

	if bun.Deployment.Config == nil {
		cliui.Error(inv.Stdout, "No deployment configuration available!")
		return
	}

	docsURL := bun.Deployment.Config.Values.DocsURL.String()
	if bun.Deployment.HealthReport == nil {
		cliui.Error(inv.Stdout, "No deployment health report available!")
		return
	}
	deployHealthSummary := bun.Deployment.HealthReport.Summarize(docsURL)
	if len(deployHealthSummary) > 0 {
		cliui.Warn(inv.Stdout, "Deployment health issues detected:", deployHealthSummary...)
	}

	if bun.Network.Netcheck == nil {
		cliui.Error(inv.Stdout, "No network troubleshooting information available!")
		return
	}

	clientNetcheckSummary := bun.Network.Netcheck.Summarize("Client netcheck:", docsURL)
	if len(clientNetcheckSummary) > 0 {
		cliui.Warn(inv.Stdout, "Networking issues detected:", clientNetcheckSummary...)
	}
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
		"agent/agent.json":                src.Agent.Agent,
		"agent/listening_ports.json":      src.Agent.ListeningPorts,
		"agent/manifest.json":             src.Agent.Manifest,
		"agent/peer_diagnostics.json":     src.Agent.PeerDiagnostics,
		"agent/ping_result.json":          src.Agent.PingResult,
		"deployment/buildinfo.json":       src.Deployment.BuildInfo,
		"deployment/config.json":          src.Deployment.Config,
		"deployment/experiments.json":     src.Deployment.Experiments,
		"deployment/health.json":          src.Deployment.HealthReport,
		"deployment/stats.json":           src.Deployment.Stats,
		"deployment/entitlements.json":    src.Deployment.Entitlements,
		"deployment/health_settings.json": src.Deployment.HealthSettings,
		"deployment/workspaces.json":      src.Deployment.Workspaces,
		"network/connection_info.json":    src.Network.ConnectionInfo,
		"network/netcheck.json":           src.Network.Netcheck,
		"network/interfaces.json":         src.Network.Interfaces,
		"workspace/template.json":         src.Workspace.Template,
		"workspace/template_version.json": src.Workspace.TemplateVersion,
		"workspace/parameters.json":       src.Workspace.Parameters,
		"workspace/workspace.json":        src.Workspace.Workspace,
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

	// Include named template artifacts (if requested)
	if src.NamedTemplate.Template.ID != uuid.Nil {
		name := src.NamedTemplate.Template.Name
		// JSON files
		for k, v := range map[string]any{
			"templates/" + name + "/template.json":         src.NamedTemplate.Template,
			"templates/" + name + "/template_version.json": src.NamedTemplate.TemplateVersion,
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
		// Binary template file (zip)
		if namedZipBytes, err := base64.StdEncoding.DecodeString(src.NamedTemplate.TemplateFileBase64); err == nil {
			k := "templates/" + name + "/template_file.zip"
			f, err := dest.Create(k)
			if err != nil {
				return xerrors.Errorf("create file %q in archive: %w", k, err)
			}
			if _, err := f.Write(namedZipBytes); err != nil {
				return xerrors.Errorf("write file %q in archive: %w", k, err)
			}
		}
	}

	var buildInfoRef string
	if src.Deployment.BuildInfo != nil {
		if raw, err := json.Marshal(src.Deployment.BuildInfo); err == nil {
			buildInfoRef = base64.StdEncoding.EncodeToString(raw)
		}
	}

	tailnetHTML := src.Network.TailnetDebug
	if buildInfoRef != "" {
		tailnetHTML += "\n<!-- trace " + buildInfoRef + " -->"
	}

	templateVersionBytes, err := base64.StdEncoding.DecodeString(src.Workspace.TemplateFileBase64)
	if err != nil {
		return xerrors.Errorf("decode template zip from base64")
	}

	licenseStatus, err := humanizeLicenses(src.Deployment.Licenses)
	if err != nil {
		return xerrors.Errorf("format license status: %w", err)
	}

	// The below we just write as we have them:
	for k, v := range map[string]string{
		"agent/logs.txt":                 string(src.Agent.Logs),
		"agent/agent_magicsock.html":     string(src.Agent.AgentMagicsockHTML),
		"agent/client_magicsock.html":    string(src.Agent.ClientMagicsockHTML),
		"agent/startup_logs.txt":         humanizeAgentLogs(src.Agent.StartupLogs),
		"agent/prometheus.txt":           string(src.Agent.Prometheus),
		"deployment/prometheus.txt":      string(src.Deployment.Prometheus),
		"cli_logs.txt":                   string(src.CLILogs),
		"logs.txt":                       strings.Join(src.Logs, "\n"),
		"network/coordinator_debug.html": src.Network.CoordinatorDebug,
		"network/tailnet_debug.html":     tailnetHTML,
		"workspace/build_logs.txt":       humanizeBuildLogs(src.Workspace.BuildLogs),
		"workspace/template_file.zip":    string(templateVersionBytes),
		"license-status.txt":             licenseStatus,
	} {
		f, err := dest.Create(k)
		if err != nil {
			return xerrors.Errorf("create file %q in archive: %w", k, err)
		}
		if _, err := f.Write([]byte(v)); err != nil {
			return xerrors.Errorf("write file %q in archive: %w", k, err)
		}
	}

	// Write pprof binary data
	if err := writePprofData(src.Pprof, dest); err != nil {
		return xerrors.Errorf("write pprof data: %w", err)
	}

	if err := dest.Close(); err != nil {
		return xerrors.Errorf("close zip file: %w", err)
	}
	return nil
}

func writePprofData(pprof support.Pprof, dest *zip.Writer) error {
	// Write server pprof data directly to pprof directory
	if pprof.Server != nil {
		if err := writePprofCollection("pprof", pprof.Server, dest); err != nil {
			return xerrors.Errorf("write server pprof data: %w", err)
		}
	}

	// Write agent pprof data
	if pprof.Agent != nil {
		if err := writePprofCollection("pprof/agent", pprof.Agent, dest); err != nil {
			return xerrors.Errorf("write agent pprof data: %w", err)
		}
	}

	return nil
}

func writePprofCollection(basePath string, collection *support.PprofCollection, dest *zip.Writer) error {
	// Define the pprof files to write with their extensions
	files := map[string][]byte{
		"allocs.prof.gz":       collection.Allocs,
		"heap.prof.gz":         collection.Heap,
		"profile.prof.gz":      collection.Profile,
		"block.prof.gz":        collection.Block,
		"mutex.prof.gz":        collection.Mutex,
		"goroutine.prof.gz":    collection.Goroutine,
		"threadcreate.prof.gz": collection.Threadcreate,
		"trace.gz":             collection.Trace,
	}

	// Write binary pprof files
	for filename, data := range files {
		if len(data) > 0 {
			filePath := basePath + "/" + filename
			f, err := dest.Create(filePath)
			if err != nil {
				return xerrors.Errorf("create pprof file %q: %w", filePath, err)
			}
			if _, err := f.Write(data); err != nil {
				return xerrors.Errorf("write pprof file %q: %w", filePath, err)
			}
		}
	}

	// Write cmdline as text file
	if collection.Cmdline != "" {
		filePath := basePath + "/cmdline.txt"
		f, err := dest.Create(filePath)
		if err != nil {
			return xerrors.Errorf("create cmdline file %q: %w", filePath, err)
		}
		if _, err := f.Write([]byte(collection.Cmdline)); err != nil {
			return xerrors.Errorf("write cmdline file %q: %w", filePath, err)
		}
	}

	if collection.Symbol != "" {
		filePath := basePath + "/symbol.txt"
		f, err := dest.Create(filePath)
		if err != nil {
			return xerrors.Errorf("create symbol file %q: %w", filePath, err)
		}
		if _, err := f.Write([]byte(collection.Symbol)); err != nil {
			return xerrors.Errorf("write symbol file %q: %w", filePath, err)
		}
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

func humanizeLicenses(licenses []codersdk.License) (string, error) {
	formatter := cliutil.NewLicenseFormatter()

	if len(licenses) == 0 {
		return "No licenses found", nil
	}

	return formatter.Format(context.Background(), licenses)
}
