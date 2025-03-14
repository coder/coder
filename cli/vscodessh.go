package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/serpent"
)

// vscodeSSH is used by the Coder VS Code extension to establish
// a connection to a workspace.
//
// This command needs to remain stable for compatibility with
// various VS Code versions, so it's kept separate from our
// standard SSH command.
func (r *RootCmd) vscodeSSH() *serpent.Command {
	var (
		sessionTokenFile    string
		urlFile             string
		logDir              string
		networkInfoDir      string
		networkInfoInterval time.Duration
		waitEnum            string
	)
	cmd := &serpent.Command{
		// A SSH config entry is added by the VS Code extension that
		// passes %h to ProxyCommand. The prefix of `coder-vscode--`
		// is a magical string represented in our VS Code extension.
		// It's not important here, only the delimiter `--` is.
		Use:        "vscodessh <coder-vscode--<owner>--<workspace>--<agent?>>",
		Hidden:     true,
		Middleware: serpent.RequireNArgs(1),
		Handler: func(inv *serpent.Invocation) error {
			if networkInfoDir == "" {
				return xerrors.New("network-info-dir must be specified")
			}
			if sessionTokenFile == "" {
				return xerrors.New("session-token-file must be specified")
			}
			if urlFile == "" {
				return xerrors.New("url-file must be specified")
			}

			fs, ok := inv.Context().Value("fs").(afero.Fs)
			if !ok {
				fs = afero.NewOsFs()
			}

			sessionToken, err := afero.ReadFile(fs, sessionTokenFile)
			if err != nil {
				return xerrors.Errorf("read session token: %w", err)
			}
			rawURL, err := afero.ReadFile(fs, urlFile)
			if err != nil {
				return xerrors.Errorf("read url: %w", err)
			}
			serverURL, err := url.Parse(string(rawURL))
			if err != nil {
				return xerrors.Errorf("parse url: %w", err)
			}

			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			client := codersdk.New(serverURL)
			client.SetSessionToken(string(sessionToken))

			// This adds custom headers to the request!
			err = r.configureClient(ctx, client, serverURL, inv)
			if err != nil {
				return xerrors.Errorf("set client: %w", err)
			}

			parts := strings.Split(inv.Args[0], "--")
			if len(parts) < 3 {
				return xerrors.Errorf("invalid argument format. must be: coder-vscode--<owner>--<name>--<agent?>")
			}
			owner := parts[1]
			name := parts[2]
			if len(parts) > 3 {
				name += "." + parts[3]
			}

			// Set autostart to false because it's assumed the VS Code extension
			// will call this command after the workspace is started.
			autostart := false

			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, autostart, fmt.Sprintf("%s/%s", owner, name))
			if err != nil {
				return xerrors.Errorf("find workspace and agent: %w", err)
			}

			// Select the startup script behavior based on template configuration or flags.
			var wait bool
			switch waitEnum {
			case "yes":
				wait = true
			case "no":
				wait = false
			case "auto":
				for _, script := range workspaceAgent.Scripts {
					if script.StartBlocksLogin {
						wait = true
						break
					}
				}
			default:
				return xerrors.Errorf("unknown wait value %q", waitEnum)
			}

			appearanceCfg, err := client.Appearance(ctx)
			if err != nil {
				var sdkErr *codersdk.Error
				if !(xerrors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound) {
					return xerrors.Errorf("get appearance config: %w", err)
				}
				appearanceCfg.DocsURL = codersdk.DefaultDocsURL()
			}

			err = cliui.Agent(ctx, inv.Stderr, workspaceAgent.ID, cliui.AgentOptions{
				Fetch:     client.WorkspaceAgent,
				FetchLogs: client.WorkspaceAgentLogsAfter,
				Wait:      wait,
				DocsURL:   appearanceCfg.DocsURL,
			})
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return cliui.Canceled
				}
			}

			// Use a stripped down writer that doesn't sync, otherwise you get
			// "failed to sync sloghuman: sync /dev/stderr: The handle is
			// invalid" on Windows. Syncing isn't required for stdout/stderr
			// anyways.
			logger := inv.Logger.AppendSinks(sloghuman.Sink(slogWriter{w: inv.Stderr})).Leveled(slog.LevelDebug)
			if logDir != "" {
				logFilePath := filepath.Join(logDir, fmt.Sprintf("%d.log", os.Getppid()))
				logFile, err := fs.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY, 0o600)
				if err != nil {
					return xerrors.Errorf("open log file %q: %w", logFilePath, err)
				}
				dc := cliutil.DiscardAfterClose(logFile)
				defer dc.Close()
				logger = logger.AppendSinks(sloghuman.Sink(dc))
			}
			if r.disableDirect {
				logger.Info(ctx, "direct connections disabled")
			}
			agentConn, err := workspacesdk.New(client).
				DialAgent(ctx, workspaceAgent.ID, &workspacesdk.DialAgentOptions{
					Logger:         logger,
					BlockEndpoints: r.disableDirect,
				})
			if err != nil {
				return xerrors.Errorf("dial workspace agent: %w", err)
			}
			defer agentConn.Close()

			agentConn.AwaitReachable(ctx)

			closeUsage := client.UpdateWorkspaceUsageWithBodyContext(ctx, workspace.ID, codersdk.PostWorkspaceUsageRequest{
				AgentID: workspaceAgent.ID,
				AppName: codersdk.UsageAppNameVscode,
			})
			defer closeUsage()

			rawSSH, err := agentConn.SSH(ctx)
			if err != nil {
				return err
			}
			defer rawSSH.Close()

			// Copy SSH traffic over stdio.
			go func() {
				_, _ = io.Copy(inv.Stdout, rawSSH)
			}()
			go func() {
				_, _ = io.Copy(rawSSH, inv.Stdin)
			}()

			errCh, err := setStatsCallback(ctx, agentConn, logger, networkInfoDir, networkInfoInterval)
			if err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return nil
			case err := <-errCh:
				return err
			}
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "network-info-dir",
			Description: "Specifies a directory to write network information periodically.",
			Value:       serpent.StringOf(&networkInfoDir),
		},
		{
			Flag:        "log-dir",
			Description: "Specifies a directory to write logs to.",
			Value:       serpent.StringOf(&logDir),
		},
		{
			Flag:        "session-token-file",
			Description: "Specifies a file that contains a session token.",
			Value:       serpent.StringOf(&sessionTokenFile),
		},
		{
			Flag:        "url-file",
			Description: "Specifies a file that contains the Coder URL.",
			Value:       serpent.StringOf(&urlFile),
		},
		{
			Flag:        "network-info-interval",
			Description: "Specifies the interval to update network information.",
			Default:     "5s",
			Value:       serpent.DurationOf(&networkInfoInterval),
		},
		{
			Flag:        "wait",
			Description: "Specifies whether or not to wait for the startup script to finish executing. Auto means that the agent startup script behavior configured in the workspace template is used.",
			Default:     "auto",
			Value:       serpent.EnumOf(&waitEnum, "yes", "no", "auto"),
		},
	}
	return cmd
}

// slogWriter wraps an io.Writer and removes all other methods (such as Sync),
// which may cause undesired/broken behavior.
type slogWriter struct {
	w io.Writer
}

var _ io.Writer = slogWriter{}

func (s slogWriter) Write(p []byte) (n int, err error) {
	return s.w.Write(p)
}
