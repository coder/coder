package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	gossh "golang.org/x/crypto/ssh"
	gosshagent "golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/autobuild/notify"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/retry"
)

var (
	workspacePollInterval   = time.Minute
	autostopNotifyCountdown = []time.Duration{30 * time.Minute}
)

func (r *RootCmd) ssh() *clibase.Cmd {
	var (
		stdio          bool
		forwardAgent   bool
		forwardGPG     bool
		identityAgent  string
		wsPollInterval time.Duration
		waitEnum       string
		noWait         bool
		logDirPath     string
		remoteForward  string
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "ssh <workspace>",
		Short:       "Start a shell into a workspace",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) (retErr error) {
			// Before dialing the SSH server over TCP, capture Interrupt signals
			// so that if we are interrupted, we have a chance to tear down the
			// TCP session cleanly before exiting.  If we don't, then the TCP
			// session can persist for up to 72 hours, since we set a long
			// timeout on the Agent side of the connection.  In particular,
			// OpenSSH sends SIGHUP to terminate a proxy command.
			ctx, stop := inv.SignalNotifyContext(inv.Context(), InterruptSignals...)
			defer stop()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			logger := slog.Make() // empty logger
			defer func() {
				if retErr != nil {
					// catch and log all returned errors so we see them in the
					// log file (if there is one)
					logger.Error(ctx, "command exit", slog.Error(retErr))
				}
			}()

			// This WaitGroup solves for a race condition where we were logging
			// while closing the log file in a defer. It probably solves
			// others too.
			var wg sync.WaitGroup
			wg.Add(1)
			defer wg.Done()

			if logDirPath != "" {
				nonce, err := cryptorand.StringCharset(cryptorand.Lower, 5)
				if err != nil {
					return xerrors.Errorf("generate nonce: %w", err)
				}
				logFilePath := filepath.Join(
					logDirPath,
					fmt.Sprintf(
						"coder-ssh-%s-%s.log",
						// The time portion makes it easier to find the right
						// log file.
						time.Now().Format("20060102-150405"),
						// The nonce prevents collisions, as SSH invocations
						// frequently happen in parallel.
						nonce,
					),
				)
				logFile, err := os.OpenFile(
					logFilePath,
					os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_EXCL,
					0o600,
				)
				if err != nil {
					return xerrors.Errorf("error opening %s for logging: %w", logDirPath, err)
				}
				go func() {
					wg.Wait()
					_ = logFile.Close()
				}()

				logger = slog.Make(sloghuman.Sink(logFile))
				if r.verbose {
					logger = logger.Leveled(slog.LevelDebug)
				}

				// log HTTP requests
				client.SetLogger(logger)
			}

			if remoteForward != "" {
				isValid := validateRemoteForward(remoteForward)
				if !isValid {
					return xerrors.Errorf(`invalid format of remote-forward, expected: remote_port:local_address:local_port`)
				}
				if isValid && stdio {
					return xerrors.Errorf(`remote-forward can't be enabled in the stdio mode`)
				}
			}

			workspace, workspaceAgent, err := getWorkspaceAndAgent(ctx, inv, client, codersdk.Me, inv.Args[0])
			if err != nil {
				return err
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
			// The `--no-wait` flag is deprecated, but for now, check it.
			if noWait {
				wait = false
			}

			templateVersion, err := client.TemplateVersion(ctx, workspace.LatestBuild.TemplateVersionID)
			if err != nil {
				return err
			}

			var unsupportedWorkspace bool
			for _, warning := range templateVersion.Warnings {
				if warning == codersdk.TemplateVersionWarningUnsupportedWorkspaces {
					unsupportedWorkspace = true
					break
				}
			}

			if unsupportedWorkspace && isTTYErr(inv) {
				_, _ = fmt.Fprintln(inv.Stderr, "👋 Your workspace uses legacy parameters which are not supported anymore. Contact your administrator for assistance.")
			}

			updateWorkspaceBanner, outdated := verifyWorkspaceOutdated(client, workspace)
			if outdated && isTTYErr(inv) {
				_, _ = fmt.Fprintln(inv.Stderr, updateWorkspaceBanner)
			}

			// OpenSSH passes stderr directly to the calling TTY.
			// This is required in "stdio" mode so a connecting indicator can be displayed.
			err = cliui.Agent(ctx, inv.Stderr, workspaceAgent.ID, cliui.AgentOptions{
				Fetch:     client.WorkspaceAgent,
				FetchLogs: client.WorkspaceAgentLogsAfter,
				Wait:      wait,
			})
			if err != nil {
				if xerrors.Is(err, context.Canceled) {
					return cliui.Canceled
				}
			}

			if r.disableDirect {
				_, _ = fmt.Fprintln(inv.Stderr, "Direct connections disabled.")
			}
			conn, err := client.DialWorkspaceAgent(ctx, workspaceAgent.ID, &codersdk.DialWorkspaceAgentOptions{
				Logger:         logger,
				BlockEndpoints: r.disableDirect,
			})
			if err != nil {
				return xerrors.Errorf("dial agent: %w", err)
			}
			defer conn.Close()
			conn.AwaitReachable(ctx)

			stopPolling := tryPollWorkspaceAutostop(ctx, client, workspace)
			defer stopPolling()

			if stdio {
				rawSSH, err := conn.SSH(ctx)
				if err != nil {
					return xerrors.Errorf("connect SSH: %w", err)
				}
				defer rawSSH.Close()

				wg.Add(1)
				go func() {
					defer wg.Done()
					watchAndClose(ctx, func() error {
						return rawSSH.Close()
					}, logger, client, workspace)
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()
					// Ensure stdout copy closes incase stdin is closed
					// unexpectedly.
					defer rawSSH.Close()

					_, err := io.Copy(rawSSH, inv.Stdin)
					if err != nil {
						logger.Error(ctx, "copy stdin error", slog.Error(err))
					} else {
						logger.Debug(ctx, "copy stdin complete")
					}
				}()
				_, err = io.Copy(inv.Stdout, rawSSH)
				if err != nil {
					logger.Error(ctx, "copy stdout error", slog.Error(err))
				} else {
					logger.Debug(ctx, "copy stdout complete")
				}
				return nil
			}

			sshClient, err := conn.SSHClient(ctx)
			if err != nil {
				return xerrors.Errorf("ssh client: %w", err)
			}
			defer sshClient.Close()

			sshSession, err := sshClient.NewSession()
			if err != nil {
				return xerrors.Errorf("ssh session: %w", err)
			}
			defer sshSession.Close()

			wg.Add(1)
			go func() {
				defer wg.Done()
				watchAndClose(
					ctx,
					func() error {
						err := sshSession.Close()
						logger.Debug(ctx, "session close", slog.Error(err))
						err = sshClient.Close()
						logger.Debug(ctx, "client close", slog.Error(err))
						return nil
					},
					logger,
					client,
					workspace,
				)
			}()

			if identityAgent == "" {
				identityAgent = os.Getenv("SSH_AUTH_SOCK")
			}
			if forwardAgent && identityAgent != "" {
				err = gosshagent.ForwardToRemote(sshClient, identityAgent)
				if err != nil {
					return xerrors.Errorf("forward agent: %w", err)
				}
				err = gosshagent.RequestAgentForwarding(sshSession)
				if err != nil {
					return xerrors.Errorf("request agent forwarding failed: %w", err)
				}
			}

			if forwardGPG {
				if workspaceAgent.OperatingSystem == "windows" {
					return xerrors.New("GPG forwarding is not supported for Windows workspaces")
				}

				err = uploadGPGKeys(ctx, sshClient)
				if err != nil {
					return xerrors.Errorf("upload GPG public keys and ownertrust to workspace: %w", err)
				}
				closer, err := forwardGPGAgent(ctx, inv.Stderr, sshClient)
				if err != nil {
					return xerrors.Errorf("forward GPG socket: %w", err)
				}
				defer closer.Close()
			}

			if remoteForward != "" {
				localAddr, remoteAddr, err := parseRemoteForward(remoteForward)
				if err != nil {
					return err
				}

				closer, err := sshRemoteForward(ctx, inv.Stderr, sshClient, localAddr, remoteAddr)
				if err != nil {
					return xerrors.Errorf("ssh remote forward: %w", err)
				}
				defer closer.Close()
			}

			stdoutFile, validOut := inv.Stdout.(*os.File)
			stdinFile, validIn := inv.Stdin.(*os.File)
			if validOut && validIn && isatty.IsTerminal(stdoutFile.Fd()) {
				state, err := term.MakeRaw(int(stdinFile.Fd()))
				if err != nil {
					return err
				}
				defer func() {
					_ = term.Restore(int(stdinFile.Fd()), state)
				}()

				windowChange := listenWindowSize(ctx)
				go func() {
					for {
						select {
						case <-ctx.Done():
							return
						case <-windowChange:
						}
						width, height, err := term.GetSize(int(stdoutFile.Fd()))
						if err != nil {
							continue
						}
						_ = sshSession.WindowChange(height, width)
					}
				}()
			}

			err = sshSession.RequestPty("xterm-256color", 128, 128, gossh.TerminalModes{})
			if err != nil {
				return xerrors.Errorf("request pty: %w", err)
			}

			sshSession.Stdin = inv.Stdin
			sshSession.Stdout = inv.Stdout
			sshSession.Stderr = inv.Stderr

			err = sshSession.Shell()
			if err != nil {
				return xerrors.Errorf("start shell: %w", err)
			}

			// Put cancel at the top of the defer stack to initiate
			// shutdown of services.
			defer cancel()

			if validOut {
				// Set initial window size.
				width, height, err := term.GetSize(int(stdoutFile.Fd()))
				if err == nil {
					_ = sshSession.WindowChange(height, width)
				}
			}

			err = sshSession.Wait()
			if err != nil {
				// If the connection drops unexpectedly, we get an
				// ExitMissingError but no other error details, so try to at
				// least give the user a better message
				if errors.Is(err, &gossh.ExitMissingError{}) {
					return xerrors.New("SSH connection ended unexpectedly")
				}
				return xerrors.Errorf("session ended: %w", err)
			}

			return nil
		},
	}
	waitOption := clibase.Option{
		Flag:        "wait",
		Env:         "CODER_SSH_WAIT",
		Description: "Specifies whether or not to wait for the startup script to finish executing. Auto means that the agent startup script behavior configured in the workspace template is used.",
		Default:     "auto",
		Value:       clibase.EnumOf(&waitEnum, "yes", "no", "auto"),
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:        "stdio",
			Env:         "CODER_SSH_STDIO",
			Description: "Specifies whether to emit SSH output over stdin/stdout.",
			Value:       clibase.BoolOf(&stdio),
		},
		{
			Flag:          "forward-agent",
			FlagShorthand: "A",
			Env:           "CODER_SSH_FORWARD_AGENT",
			Description:   "Specifies whether to forward the SSH agent specified in $SSH_AUTH_SOCK.",
			Value:         clibase.BoolOf(&forwardAgent),
		},
		{
			Flag:          "forward-gpg",
			FlagShorthand: "G",
			Env:           "CODER_SSH_FORWARD_GPG",
			Description:   "Specifies whether to forward the GPG agent. Unsupported on Windows workspaces, but supports all clients. Requires gnupg (gpg, gpgconf) on both the client and workspace. The GPG agent must already be running locally and will not be started for you. If a GPG agent is already running in the workspace, it will be attempted to be killed.",
			Value:         clibase.BoolOf(&forwardGPG),
		},
		{
			Flag:        "identity-agent",
			Env:         "CODER_SSH_IDENTITY_AGENT",
			Description: "Specifies which identity agent to use (overrides $SSH_AUTH_SOCK), forward agent must also be enabled.",
			Value:       clibase.StringOf(&identityAgent),
		},
		{
			Flag:        "workspace-poll-interval",
			Env:         "CODER_WORKSPACE_POLL_INTERVAL",
			Description: "Specifies how often to poll for workspace automated shutdown.",
			Default:     "1m",
			Value:       clibase.DurationOf(&wsPollInterval),
		},
		waitOption,
		{
			Flag:        "no-wait",
			Env:         "CODER_SSH_NO_WAIT",
			Description: "Enter workspace immediately after the agent has connected. This is the default if the template has configured the agent startup script behavior as non-blocking.",
			Value:       clibase.BoolOf(&noWait),
			UseInstead:  []clibase.Option{waitOption},
		},
		{
			Flag:          "log-dir",
			Description:   "Specify the directory containing SSH diagnostic log files.",
			Env:           "CODER_SSH_LOG_DIR",
			FlagShorthand: "l",
			Value:         clibase.StringOf(&logDirPath),
		},
		{
			Flag:          "remote-forward",
			Description:   "Enable remote port forwarding (remote_port:local_address:local_port).",
			Env:           "CODER_SSH_REMOTE_FORWARD",
			FlagShorthand: "R",
			Value:         clibase.StringOf(&remoteForward),
		},
	}
	return cmd
}

// watchAndClose ensures closer is called if the context is canceled or
// the workspace reaches the stopped state.
//
// Watching the stopped state is a work-around for cases
// where the agent is not gracefully shut down and the
// connection is left open. If, for instance, the networking
// is stopped before the agent is shut down, the disconnect
// will usually not propagate.
//
// See: https://github.com/coder/coder/issues/6180
func watchAndClose(ctx context.Context, closer func() error, logger slog.Logger, client *codersdk.Client, workspace codersdk.Workspace) {
	// Ensure session is ended on both context cancellation
	// and workspace stop.
	defer func() {
		err := closer()
		if err != nil {
			logger.Error(ctx, "error closing session", slog.Error(err))
		}
	}()

startWatchLoop:
	for {
		logger.Debug(ctx, "connecting to the coder server to watch workspace events")
		var wsWatch <-chan codersdk.Workspace
		var err error
		for r := retry.New(time.Second, 15*time.Second); r.Wait(ctx); {
			wsWatch, err = client.WatchWorkspace(ctx, workspace.ID)
			if err == nil {
				break
			}
			if ctx.Err() != nil {
				logger.Debug(ctx, "context expired", slog.Error(ctx.Err()))
				return
			}
		}

		for {
			select {
			case <-ctx.Done():
				logger.Debug(ctx, "context expired", slog.Error(ctx.Err()))
				return
			case w, ok := <-wsWatch:
				if !ok {
					continue startWatchLoop
				}

				// Transitioning to stop or delete could mean that
				// the agent will still gracefully stop. If a new
				// build is starting, there's no reason to wait for
				// the agent, it should be long gone.
				if workspace.LatestBuild.ID != w.LatestBuild.ID && w.LatestBuild.Transition == codersdk.WorkspaceTransitionStart {
					logger.Info(ctx, "new build started")
					return
				}
				// Note, we only react to the stopped state here because we
				// want to give the agent a chance to gracefully shut down
				// during "stopping".
				if w.LatestBuild.Status == codersdk.WorkspaceStatusStopped {
					logger.Info(ctx, "workspace stopped")
					return
				}
			}
		}
	}
}

// getWorkspaceAgent returns the workspace and agent selected using either the
// `<workspace>[.<agent>]` syntax via `in` or picks a random workspace and agent
// if `shuffle` is true.
func getWorkspaceAndAgent(ctx context.Context, inv *clibase.Invocation, client *codersdk.Client, userID string, in string) (codersdk.Workspace, codersdk.WorkspaceAgent, error) { //nolint:revive
	var (
		workspace      codersdk.Workspace
		workspaceParts = strings.Split(in, ".")
		err            error
	)

	workspace, err = namedWorkspace(ctx, client, workspaceParts[0])
	if err != nil {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
	}

	if workspace.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.New("workspace must be in start transition to ssh")
	}
	if workspace.LatestBuild.Job.CompletedAt == nil {
		err := cliui.WorkspaceBuild(ctx, inv.Stderr, client, workspace.LatestBuild.ID)
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
		}
		// Fetch up-to-date build information after completion.
		workspace.LatestBuild, err = client.WorkspaceBuild(ctx, workspace.LatestBuild.ID)
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
		}
	}
	if workspace.LatestBuild.Transition == codersdk.WorkspaceTransitionDelete {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("workspace %q is being deleted", workspace.Name)
	}

	resources := workspace.LatestBuild.Resources

	agents := make([]codersdk.WorkspaceAgent, 0)
	for _, resource := range resources {
		agents = append(agents, resource.Agents...)
	}
	if len(agents) == 0 {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("workspace %q has no agents", workspace.Name)
	}
	var workspaceAgent codersdk.WorkspaceAgent
	if len(workspaceParts) >= 2 {
		for _, otherAgent := range agents {
			if otherAgent.Name != workspaceParts[1] {
				continue
			}
			workspaceAgent = otherAgent
			break
		}
		if workspaceAgent.ID == uuid.Nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("agent not found by name %q", workspaceParts[1])
		}
	}
	if workspaceAgent.ID == uuid.Nil {
		if len(agents) > 1 {
			workspaceAgent, err = cryptorand.Element(agents)
			if err != nil {
				return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
			}
		} else {
			workspaceAgent = agents[0]
		}
	}

	return workspace, workspaceAgent, nil
}

// Attempt to poll workspace autostop. We write a per-workspace lockfile to
// avoid spamming the user with notifications in case of multiple instances
// of the CLI running simultaneously.
func tryPollWorkspaceAutostop(ctx context.Context, client *codersdk.Client, workspace codersdk.Workspace) (stop func()) {
	lock := flock.New(filepath.Join(os.TempDir(), "coder-autostop-notify-"+workspace.ID.String()))
	conditionCtx, cancelCondition := context.WithCancel(ctx)
	condition := notifyCondition(conditionCtx, client, workspace.ID, lock)
	stopFunc := notify.Notify(condition, workspacePollInterval, autostopNotifyCountdown...)
	return func() {
		// With many "ssh" processes running, `lock.TryLockContext` can be hanging until the context canceled.
		// Without this cancellation, a CLI process with failed remote-forward could be hanging indefinitely.
		cancelCondition()
		stopFunc()
	}
}

// Notify the user if the workspace is due to shutdown.
func notifyCondition(ctx context.Context, client *codersdk.Client, workspaceID uuid.UUID, lock *flock.Flock) notify.Condition {
	return func(now time.Time) (deadline time.Time, callback func()) {
		// Keep trying to regain the lock.
		locked, err := lock.TryLockContext(ctx, workspacePollInterval)
		if err != nil || !locked {
			return time.Time{}, nil
		}

		ws, err := client.Workspace(ctx, workspaceID)
		if err != nil {
			return time.Time{}, nil
		}

		if ptr.NilOrZero(ws.TTLMillis) {
			return time.Time{}, nil
		}

		deadline = ws.LatestBuild.Deadline.Time
		callback = func() {
			ttl := deadline.Sub(now)
			var title, body string
			if ttl > time.Minute {
				title = fmt.Sprintf(`Workspace %s stopping soon`, ws.Name)
				body = fmt.Sprintf(
					`Your Coder workspace %s is scheduled to stop in %.0f mins`, ws.Name, ttl.Minutes())
			} else {
				title = fmt.Sprintf("Workspace %s stopping!", ws.Name)
				body = fmt.Sprintf("Your Coder workspace %s is stopping any time now!", ws.Name)
			}
			// notify user with a native system notification (best effort)
			_ = beeep.Notify(title, body, "")
		}
		return deadline.Truncate(time.Minute), callback
	}
}

// Verify if the user workspace is outdated and prepare an actionable message for user.
func verifyWorkspaceOutdated(client *codersdk.Client, workspace codersdk.Workspace) (string, bool) {
	if !workspace.Outdated {
		return "", false // workspace is up-to-date
	}

	workspaceLink := buildWorkspaceLink(client.URL, workspace)
	return fmt.Sprintf("👋 Your workspace is outdated! Update it here: %s\n", workspaceLink), true
}

// Build the user workspace link which navigates to the Coder web UI.
func buildWorkspaceLink(serverURL *url.URL, workspace codersdk.Workspace) *url.URL {
	return serverURL.ResolveReference(&url.URL{Path: fmt.Sprintf("@%s/%s", workspace.OwnerName, workspace.Name)})
}

// runLocal runs a command on the local machine.
func runLocal(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin

	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if exitErr := new(exec.ExitError); errors.As(err, &exitErr) {
			stderr = exitErr.Stderr
		}

		return out, xerrors.Errorf(
			"`%s %s` failed: stderr: %s\n\nstdout: %s\n\n%w",
			name,
			strings.Join(args, " "),
			bytes.TrimSpace(stderr),
			bytes.TrimSpace(out),
			err,
		)
	}

	return out, nil
}

// runRemoteSSH runs a command on a remote machine/workspace via SSH.
func runRemoteSSH(sshClient *gossh.Client, stdin io.Reader, cmd string) ([]byte, error) {
	sess, err := sshClient.NewSession()
	if err != nil {
		return nil, xerrors.Errorf("create SSH session")
	}
	defer sess.Close()

	stderr := bytes.NewBuffer(nil)
	sess.Stdin = stdin
	// On fish, this was outputting to stderr instead of stdout.
	// The tests pass differently on different Linux machines,
	// so it's best we capture the output of both.
	out, err := sess.CombinedOutput(cmd)
	if err != nil {
		return out, xerrors.Errorf(
			"`%s` failed: stderr: %s\n\nstdout: %s:\n\n%w",
			cmd,
			bytes.TrimSpace(stderr.Bytes()),
			bytes.TrimSpace(out),
			err,
		)
	}

	return out, nil
}

func uploadGPGKeys(ctx context.Context, sshClient *gossh.Client) error {
	// Check if the agent is running in the workspace already.
	//
	// Note: we don't support windows in the workspace for GPG forwarding so
	//       using shell commands is fine.
	//
	// Note: we sleep after killing the agent because it doesn't always die
	//       immediately.
	agentSocketBytes, err := runRemoteSSH(sshClient, nil, `sh -c '
set -eux
agent_socket=$(gpgconf --list-dir agent-socket)
echo "$agent_socket"
if [ -S "$agent_socket" ]; then
  echo "agent socket exists, attempting to kill it" >&2
  gpgconf --kill gpg-agent
  rm -f "$agent_socket"
  sleep 1
fi

test ! -S "$agent_socket"
'`)
	agentSocket := strings.TrimSpace(string(agentSocketBytes))
	if err != nil {
		return xerrors.Errorf("check if agent socket is running (check if %q exists): %w", agentSocket, err)
	}
	if agentSocket == "" {
		return xerrors.Errorf("agent socket path is empty, check the output of `gpgconf --list-dir agent-socket`")
	}

	// Read the user's public keys and ownertrust from GPG.
	pubKeyExport, err := runLocal(ctx, nil, "gpg", "--armor", "--export")
	if err != nil {
		return xerrors.Errorf("export local public keys from GPG: %w", err)
	}
	ownerTrustExport, err := runLocal(ctx, nil, "gpg", "--export-ownertrust")
	if err != nil {
		return xerrors.Errorf("export local ownertrust from GPG: %w", err)
	}

	// Import the public keys and ownertrust into the workspace.
	_, err = runRemoteSSH(sshClient, bytes.NewReader(pubKeyExport), "gpg --import")
	if err != nil {
		return xerrors.Errorf("import public keys into workspace: %w", err)
	}
	_, err = runRemoteSSH(sshClient, bytes.NewReader(ownerTrustExport), "gpg --import-ownertrust")
	if err != nil {
		return xerrors.Errorf("import ownertrust into workspace: %w", err)
	}

	// Kill the agent in the workspace if it was started by one of the above
	// commands.
	_, err = runRemoteSSH(sshClient, nil, fmt.Sprintf("gpgconf --kill gpg-agent && rm -f %q", agentSocket))
	if err != nil {
		return xerrors.Errorf("kill existing agent in workspace: %w", err)
	}

	return nil
}

func localGPGExtraSocket(ctx context.Context) (string, error) {
	localSocket, err := runLocal(ctx, nil, "gpgconf", "--list-dir", "agent-extra-socket")
	if err != nil {
		return "", xerrors.Errorf("get local GPG agent socket: %w", err)
	}

	return string(bytes.TrimSpace(localSocket)), nil
}

func remoteGPGAgentSocket(sshClient *gossh.Client) (string, error) {
	remoteSocket, err := runRemoteSSH(sshClient, nil, "gpgconf --list-dir agent-socket")
	if err != nil {
		return "", xerrors.Errorf("get remote GPG agent socket: %w", err)
	}

	return string(bytes.TrimSpace(remoteSocket)), nil
}
