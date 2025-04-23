package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"github.com/spf13/afero"
	gossh "golang.org/x/crypto/ssh"
	gosshagent "golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
	"golang.org/x/xerrors"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"tailscale.com/tailcfg"
	"tailscale.com/types/netlogtype"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/coderd/autobuild/notify"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/pty"
	"github.com/coder/quartz"
	"github.com/coder/retry"
	"github.com/coder/serpent"
)

const (
	disableUsageApp = "disable"
)

var (
	workspacePollInterval   = time.Minute
	autostopNotifyCountdown = []time.Duration{30 * time.Minute}
	// gracefulShutdownTimeout is the timeout, per item in the stack of things to close
	gracefulShutdownTimeout = 2 * time.Second
	workspaceNameRe         = regexp.MustCompile(`[/.]+|--`)
)

func (r *RootCmd) ssh() *serpent.Command {
	var (
		stdio               bool
		hostPrefix          string
		hostnameSuffix      string
		forwardAgent        bool
		forwardGPG          bool
		identityAgent       string
		wsPollInterval      time.Duration
		waitEnum            string
		noWait              bool
		logDirPath          string
		remoteForwards      []string
		env                 []string
		usageApp            string
		disableAutostart    bool
		appearanceConfig    codersdk.AppearanceConfig
		networkInfoDir      string
		networkInfoInterval time.Duration

		containerName string
		containerUser string
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "ssh <workspace>",
		Short:       "Start a shell into a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
			initAppearance(client, &appearanceConfig),
		),
		Handler: func(inv *serpent.Invocation) (retErr error) {
			// Before dialing the SSH server over TCP, capture Interrupt signals
			// so that if we are interrupted, we have a chance to tear down the
			// TCP session cleanly before exiting.  If we don't, then the TCP
			// session can persist for up to 72 hours, since we set a long
			// timeout on the Agent side of the connection.  In particular,
			// OpenSSH sends SIGHUP to terminate a proxy command.
			ctx, stop := inv.SignalNotifyContext(inv.Context(), StopSignals...)
			defer stop()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			// Prevent unnecessary logs from the stdlib from messing up the TTY.
			// See: https://github.com/coder/coder/issues/13144
			log.SetOutput(io.Discard)

			logger := inv.Logger
			defer func() {
				if retErr != nil {
					// catch and log all returned errors so we see them in the
					// log file (if there is one)
					logger.Error(ctx, "command exit", slog.Error(retErr))
				}
			}()

			// In stdio mode, we can't allow any writes to stdin or stdout
			// because they are used by the SSH protocol.
			stdioReader, stdioWriter := inv.Stdin, inv.Stdout
			if stdio {
				inv.Stdin = stdioErrLogReader{inv.Logger}
				inv.Stdout = inv.Stderr
			}

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
				logFileBaseName := fmt.Sprintf(
					"coder-ssh-%s-%s",
					// The time portion makes it easier to find the right
					// log file.
					time.Now().Format("20060102-150405"),
					// The nonce prevents collisions, as SSH invocations
					// frequently happen in parallel.
					nonce,
				)
				if stdio {
					// The VS Code extension obtains the PID of the SSH process to
					// find the log file associated with a SSH session.
					//
					// We get the parent PID because it's assumed `ssh` is calling this
					// command via the ProxyCommand SSH option.
					logFileBaseName += fmt.Sprintf("-%d", os.Getppid())
				}
				logFileBaseName += ".log"

				logFilePath := filepath.Join(logDirPath, logFileBaseName)
				logFile, err := os.OpenFile(
					logFilePath,
					os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_EXCL,
					0o600,
				)
				if err != nil {
					return xerrors.Errorf("error opening %s for logging: %w", logDirPath, err)
				}
				dc := cliutil.DiscardAfterClose(logFile)
				go func() {
					wg.Wait()
					_ = dc.Close()
				}()

				logger = logger.AppendSinks(sloghuman.Sink(dc))
				if r.verbose {
					logger = logger.Leveled(slog.LevelDebug)
				}

				// log HTTP requests
				client.SetLogger(logger)
			}
			stack := newCloserStack(ctx, logger, quartz.NewReal())
			defer stack.close(nil)

			for _, remoteForward := range remoteForwards {
				isValid := validateRemoteForward(remoteForward)
				if !isValid {
					return xerrors.Errorf(`invalid format of remote-forward, expected: remote_port:local_address:local_port`)
				}
				if isValid && stdio {
					return xerrors.Errorf(`remote-forward can't be enabled in the stdio mode`)
				}
			}

			var parsedEnv [][2]string
			for _, e := range env {
				k, v, ok := strings.Cut(e, "=")
				if !ok {
					return xerrors.Errorf("invalid environment variable setting %q", e)
				}
				parsedEnv = append(parsedEnv, [2]string{k, v})
			}

			deploymentSSHConfig := codersdk.SSHConfigResponse{
				HostnamePrefix: hostPrefix,
				HostnameSuffix: hostnameSuffix,
			}

			workspace, workspaceAgent, err := findWorkspaceAndAgentByHostname(
				ctx, inv, client,
				inv.Args[0], deploymentSSHConfig, disableAutostart)
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
				FetchInterval: 0,
				Fetch:         client.WorkspaceAgent,
				FetchLogs:     client.WorkspaceAgentLogsAfter,
				Wait:          wait,
				DocsURL:       appearanceConfig.DocsURL,
			})
			if err != nil {
				if xerrors.Is(err, context.Canceled) {
					return cliui.ErrCanceled
				}
				return err
			}

			if r.disableDirect {
				_, _ = fmt.Fprintln(inv.Stderr, "Direct connections disabled.")
			}
			conn, err := workspacesdk.New(client).
				DialAgent(ctx, workspaceAgent.ID, &workspacesdk.DialAgentOptions{
					Logger:          logger,
					BlockEndpoints:  r.disableDirect,
					EnableTelemetry: !r.disableNetworkTelemetry,
				})
			if err != nil {
				return xerrors.Errorf("dial agent: %w", err)
			}
			if err = stack.push("agent conn", conn); err != nil {
				return err
			}
			conn.AwaitReachable(ctx)

			if containerName != "" {
				cts, err := client.WorkspaceAgentListContainers(ctx, workspaceAgent.ID, nil)
				if err != nil {
					return xerrors.Errorf("list containers: %w", err)
				}
				if len(cts.Containers) == 0 {
					cliui.Info(inv.Stderr, "No containers found!")
					cliui.Info(inv.Stderr, "Tip: Agent container integration is experimental and not enabled by default.")
					cliui.Info(inv.Stderr, "     To enable it, set CODER_AGENT_DEVCONTAINERS_ENABLE=true in your template.")
					return nil
				}
				var found bool
				for _, c := range cts.Containers {
					if c.FriendlyName == containerName || c.ID == containerName {
						found = true
						break
					}
				}
				if !found {
					availableContainers := make([]string, len(cts.Containers))
					for i, c := range cts.Containers {
						availableContainers[i] = c.FriendlyName
					}
					cliui.Errorf(inv.Stderr, "Container not found: %q\nAvailable containers: %v", containerName, availableContainers)
					return nil
				}
			}

			stopPolling := tryPollWorkspaceAutostop(ctx, client, workspace)
			defer stopPolling()

			usageAppName := getUsageAppName(usageApp)
			if usageAppName != "" {
				closeUsage := client.UpdateWorkspaceUsageWithBodyContext(ctx, workspace.ID, codersdk.PostWorkspaceUsageRequest{
					AgentID: workspaceAgent.ID,
					AppName: usageAppName,
				})
				defer closeUsage()
			}

			if stdio {
				rawSSH, err := conn.SSH(ctx)
				if err != nil {
					return xerrors.Errorf("connect SSH: %w", err)
				}
				copier := newRawSSHCopier(logger, rawSSH, stdioReader, stdioWriter)
				if err = stack.push("rawSSHCopier", copier); err != nil {
					return err
				}

				var errCh <-chan error
				if networkInfoDir != "" {
					errCh, err = setStatsCallback(ctx, conn, logger, networkInfoDir, networkInfoInterval)
					if err != nil {
						return err
					}
				}

				wg.Add(1)
				go func() {
					defer wg.Done()
					watchAndClose(ctx, func() error {
						stack.close(xerrors.New("watchAndClose"))
						return nil
					}, logger, client, workspace, errCh)
				}()
				copier.copy(&wg)
				return nil
			}

			sshClient, err := conn.SSHClient(ctx)
			if err != nil {
				return xerrors.Errorf("ssh client: %w", err)
			}
			if err = stack.push("ssh client", sshClient); err != nil {
				return err
			}

			sshSession, err := sshClient.NewSession()
			if err != nil {
				return xerrors.Errorf("ssh session: %w", err)
			}
			if err = stack.push("sshSession", sshSession); err != nil {
				return err
			}

			var errCh <-chan error
			if networkInfoDir != "" {
				errCh, err = setStatsCallback(ctx, conn, logger, networkInfoDir, networkInfoInterval)
				if err != nil {
					return err
				}
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				watchAndClose(
					ctx,
					func() error {
						stack.close(xerrors.New("watchAndClose"))
						return nil
					},
					logger,
					client,
					workspace,
					errCh,
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
				if err = stack.push("forwardGPGAgent", closer); err != nil {
					return err
				}
			}

			if len(remoteForwards) > 0 {
				for _, remoteForward := range remoteForwards {
					localAddr, remoteAddr, err := parseRemoteForward(remoteForward)
					if err != nil {
						return err
					}

					closer, err := sshRemoteForward(ctx, inv.Stderr, sshClient, localAddr, remoteAddr)
					if err != nil {
						return xerrors.Errorf("ssh remote forward: %w", err)
					}
					if err = stack.push("sshRemoteForward", closer); err != nil {
						return err
					}
				}
			}

			stdinFile, validIn := inv.Stdin.(*os.File)
			stdoutFile, validOut := inv.Stdout.(*os.File)
			if validIn && validOut && isatty.IsTerminal(stdinFile.Fd()) && isatty.IsTerminal(stdoutFile.Fd()) {
				inState, err := pty.MakeInputRaw(stdinFile.Fd())
				if err != nil {
					return err
				}
				defer func() {
					_ = pty.RestoreTerminal(stdinFile.Fd(), inState)
				}()
				outState, err := pty.MakeOutputRaw(stdoutFile.Fd())
				if err != nil {
					return err
				}
				defer func() {
					_ = pty.RestoreTerminal(stdoutFile.Fd(), outState)
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

			for _, kv := range parsedEnv {
				if err := sshSession.Setenv(kv[0], kv[1]); err != nil {
					return xerrors.Errorf("setenv: %w", err)
				}
			}

			if containerName != "" {
				for k, v := range map[string]string{
					agentssh.ContainerEnvironmentVariable:     containerName,
					agentssh.ContainerUserEnvironmentVariable: containerUser,
				} {
					if err := sshSession.Setenv(k, v); err != nil {
						return xerrors.Errorf("setenv: %w", err)
					}
				}
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
			conn.SendDisconnectedTelemetry()
			if err != nil {
				if exitErr := (&gossh.ExitError{}); errors.As(err, &exitErr) {
					// Clear the error since it's not useful beyond
					// reporting status.
					return ExitError(exitErr.ExitStatus(), nil)
				}
				// If the connection drops unexpectedly, we get an
				// ExitMissingError but no other error details, so try to at
				// least give the user a better message
				if errors.Is(err, &gossh.ExitMissingError{}) {
					return ExitError(255, xerrors.New("SSH connection ended unexpectedly"))
				}
				return xerrors.Errorf("session ended: %w", err)
			}

			return nil
		},
	}
	waitOption := serpent.Option{
		Flag:        "wait",
		Env:         "CODER_SSH_WAIT",
		Description: "Specifies whether or not to wait for the startup script to finish executing. Auto means that the agent startup script behavior configured in the workspace template is used.",
		Default:     "auto",
		Value:       serpent.EnumOf(&waitEnum, "yes", "no", "auto"),
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "stdio",
			Env:         "CODER_SSH_STDIO",
			Description: "Specifies whether to emit SSH output over stdin/stdout.",
			Value:       serpent.BoolOf(&stdio),
		},
		{
			Flag:        "ssh-host-prefix",
			Env:         "CODER_SSH_SSH_HOST_PREFIX",
			Description: "Strip this prefix from the provided hostname to determine the workspace name. This is useful when used as part of an OpenSSH proxy command.",
			Value:       serpent.StringOf(&hostPrefix),
		},
		{
			Flag:        "hostname-suffix",
			Env:         "CODER_SSH_HOSTNAME_SUFFIX",
			Description: "Strip this suffix from the provided hostname to determine the workspace name. This is useful when used as part of an OpenSSH proxy command. The suffix must be specified without a leading . character.",
			Value:       serpent.StringOf(&hostnameSuffix),
		},
		{
			Flag:          "forward-agent",
			FlagShorthand: "A",
			Env:           "CODER_SSH_FORWARD_AGENT",
			Description:   "Specifies whether to forward the SSH agent specified in $SSH_AUTH_SOCK.",
			Value:         serpent.BoolOf(&forwardAgent),
		},
		{
			Flag:          "forward-gpg",
			FlagShorthand: "G",
			Env:           "CODER_SSH_FORWARD_GPG",
			Description:   "Specifies whether to forward the GPG agent. Unsupported on Windows workspaces, but supports all clients. Requires gnupg (gpg, gpgconf) on both the client and workspace. The GPG agent must already be running locally and will not be started for you. If a GPG agent is already running in the workspace, it will be attempted to be killed.",
			Value:         serpent.BoolOf(&forwardGPG),
		},
		{
			Flag:        "identity-agent",
			Env:         "CODER_SSH_IDENTITY_AGENT",
			Description: "Specifies which identity agent to use (overrides $SSH_AUTH_SOCK), forward agent must also be enabled.",
			Value:       serpent.StringOf(&identityAgent),
		},
		{
			Flag:        "workspace-poll-interval",
			Env:         "CODER_WORKSPACE_POLL_INTERVAL",
			Description: "Specifies how often to poll for workspace automated shutdown.",
			Default:     "1m",
			Value:       serpent.DurationOf(&wsPollInterval),
		},
		waitOption,
		{
			Flag:        "no-wait",
			Env:         "CODER_SSH_NO_WAIT",
			Description: "Enter workspace immediately after the agent has connected. This is the default if the template has configured the agent startup script behavior as non-blocking.",
			Value:       serpent.BoolOf(&noWait),
			UseInstead:  []serpent.Option{waitOption},
		},
		{
			Flag:          "log-dir",
			Description:   "Specify the directory containing SSH diagnostic log files.",
			Env:           "CODER_SSH_LOG_DIR",
			FlagShorthand: "l",
			Value:         serpent.StringOf(&logDirPath),
		},
		{
			Flag:          "remote-forward",
			Description:   "Enable remote port forwarding (remote_port:local_address:local_port).",
			Env:           "CODER_SSH_REMOTE_FORWARD",
			FlagShorthand: "R",
			Value:         serpent.StringArrayOf(&remoteForwards),
		},
		{
			Flag:          "env",
			Description:   "Set environment variable(s) for session (key1=value1,key2=value2,...).",
			Env:           "CODER_SSH_ENV",
			FlagShorthand: "e",
			Value:         serpent.StringArrayOf(&env),
		},
		{
			Flag:        "usage-app",
			Description: "Specifies the usage app to use for workspace activity tracking.",
			Env:         "CODER_SSH_USAGE_APP",
			Value:       serpent.StringOf(&usageApp),
			Hidden:      true,
		},
		{
			Flag:        "network-info-dir",
			Description: "Specifies a directory to write network information periodically.",
			Value:       serpent.StringOf(&networkInfoDir),
		},
		{
			Flag:        "network-info-interval",
			Description: "Specifies the interval to update network information.",
			Default:     "5s",
			Value:       serpent.DurationOf(&networkInfoInterval),
		},
		{
			Flag:          "container",
			FlagShorthand: "c",
			Description:   "Specifies a container inside the workspace to connect to.",
			Value:         serpent.StringOf(&containerName),
			Hidden:        true, // Hidden until this features is at least in beta.
		},
		{
			Flag:        "container-user",
			Description: "When connecting to a container, specifies the user to connect as.",
			Value:       serpent.StringOf(&containerUser),
			Hidden:      true, // Hidden until this features is at least in beta.
		},
		sshDisableAutostartOption(serpent.BoolOf(&disableAutostart)),
	}
	return cmd
}

// findWorkspaceAndAgentByHostname parses the hostname from the commandline and finds the workspace and agent it
// corresponds to, taking into account any name prefixes or suffixes configured (e.g. myworkspace.coder, or
// vscode-coder--myusername--myworkspace).
func findWorkspaceAndAgentByHostname(
	ctx context.Context, inv *serpent.Invocation, client *codersdk.Client,
	hostname string, config codersdk.SSHConfigResponse, disableAutostart bool,
) (
	codersdk.Workspace, codersdk.WorkspaceAgent, error,
) {
	// for suffixes, we don't explicitly get the . and must add it. This is to ensure that the suffix is always
	// interpreted as a dotted label in DNS names, not just any string suffix. That is, a suffix of 'coder' will
	// match a hostname like 'en.coder', but not 'encoder'.
	qualifiedSuffix := "." + config.HostnameSuffix

	switch {
	case config.HostnamePrefix != "" && strings.HasPrefix(hostname, config.HostnamePrefix):
		hostname = strings.TrimPrefix(hostname, config.HostnamePrefix)
	case config.HostnameSuffix != "" && strings.HasSuffix(hostname, qualifiedSuffix):
		hostname = strings.TrimSuffix(hostname, qualifiedSuffix)
	}
	hostname = normalizeWorkspaceInput(hostname)
	return getWorkspaceAndAgent(ctx, inv, client, !disableAutostart, hostname)
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
func watchAndClose(ctx context.Context, closer func() error, logger slog.Logger, client *codersdk.Client, workspace codersdk.Workspace, errCh <-chan error) {
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
			case err := <-errCh:
				logger.Error(ctx, "failed to collect network stats", slog.Error(err))
				return
			}
		}
	}
}

// getWorkspaceAgent returns the workspace and agent selected using either the
// `<workspace>[.<agent>]` syntax via `in`.
// If autoStart is true, the workspace will be started if it is not already running.
func getWorkspaceAndAgent(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, autostart bool, input string) (codersdk.Workspace, codersdk.WorkspaceAgent, error) { //nolint:revive
	var (
		workspace codersdk.Workspace
		// The input will be `owner/name.agent`
		// The agent is optional.
		workspaceParts = strings.Split(input, ".")
		err            error
	)

	workspace, err = namedWorkspace(ctx, client, workspaceParts[0])
	if err != nil {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
	}

	if workspace.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
		if !autostart {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.New("workspace must be started")
		}
		// Autostart the workspace for the user.
		// For some failure modes, return a better message.
		if workspace.LatestBuild.Transition == codersdk.WorkspaceTransitionDelete {
			// Any sort of deleting status, we should reject with a nicer error.
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("workspace %q is deleted", workspace.Name)
		}
		if workspace.LatestBuild.Job.Status == codersdk.ProvisionerJobFailed {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{},
				xerrors.Errorf("workspace %q is in failed state, unable to autostart the workspace", workspace.Name)
		}
		// The workspace needs to be stopped before we can start it.
		// It cannot be in any pending or failed state.
		if workspace.LatestBuild.Status != codersdk.WorkspaceStatusStopped {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{},
				xerrors.Errorf("workspace must be started; was unable to autostart as the last build job is %q, expected %q",
					workspace.LatestBuild.Status,
					codersdk.WorkspaceStatusStopped,
				)
		}

		// Start workspace based on the last build parameters.
		// It's possible for a workspace build to fail due to the template requiring starting
		// workspaces with the active version.
		_, _ = fmt.Fprintf(inv.Stderr, "Workspace was stopped, starting workspace to allow connecting to %q...\n", workspace.Name)
		_, err = startWorkspace(inv, client, workspace, workspaceParameterFlags{}, buildFlags{}, WorkspaceStart)
		if cerr, ok := codersdk.AsError(err); ok {
			switch cerr.StatusCode() {
			case http.StatusConflict:
				_, _ = fmt.Fprintln(inv.Stderr, "Unable to start the workspace due to conflict, the workspace may be starting, retrying without autostart...")
				return getWorkspaceAndAgent(ctx, inv, client, false, input)

			case http.StatusForbidden:
				_, err = startWorkspace(inv, client, workspace, workspaceParameterFlags{}, buildFlags{}, WorkspaceUpdate)
				if err != nil {
					return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("start workspace with active template version: %w", err)
				}
				_, _ = fmt.Fprintln(inv.Stdout, "Unable to start the workspace with template version from last build. Your workspace has been updated to the current active template version.")
			}
		} else if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, xerrors.Errorf("start workspace with current template version: %w", err)
		}

		// Refresh workspace state so that `outdated`, `build`,`template_*` fields are up-to-date.
		workspace, err = namedWorkspace(ctx, client, workspaceParts[0])
		if err != nil {
			return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
		}
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

	var agentName string
	if len(workspaceParts) >= 2 {
		agentName = workspaceParts[1]
	}
	workspaceAgent, err := getWorkspaceAgent(workspace, agentName)
	if err != nil {
		return codersdk.Workspace{}, codersdk.WorkspaceAgent{}, err
	}

	return workspace, workspaceAgent, nil
}

func getWorkspaceAgent(workspace codersdk.Workspace, agentName string) (workspaceAgent codersdk.WorkspaceAgent, err error) {
	resources := workspace.LatestBuild.Resources

	agents := make([]codersdk.WorkspaceAgent, 0)
	for _, resource := range resources {
		agents = append(agents, resource.Agents...)
	}
	if len(agents) == 0 {
		return codersdk.WorkspaceAgent{}, xerrors.Errorf("workspace %q has no agents", workspace.Name)
	}
	if agentName != "" {
		for _, otherAgent := range agents {
			if otherAgent.Name != agentName {
				continue
			}
			workspaceAgent = otherAgent
			break
		}
		if workspaceAgent.ID == uuid.Nil {
			return codersdk.WorkspaceAgent{}, xerrors.Errorf("agent not found by name %q", agentName)
		}
	}
	if workspaceAgent.ID == uuid.Nil {
		if len(agents) > 1 {
			workspaceAgent, err = cryptorand.Element(agents)
			if err != nil {
				return codersdk.WorkspaceAgent{}, err
			}
		} else {
			workspaceAgent = agents[0]
		}
	}
	return workspaceAgent, nil
}

// Attempt to poll workspace autostop. We write a per-workspace lockfile to
// avoid spamming the user with notifications in case of multiple instances
// of the CLI running simultaneously.
func tryPollWorkspaceAutostop(ctx context.Context, client *codersdk.Client, workspace codersdk.Workspace) (stop func()) {
	lock := flock.New(filepath.Join(os.TempDir(), "coder-autostop-notify-"+workspace.ID.String()))
	conditionCtx, cancelCondition := context.WithCancel(ctx)
	condition := notifyCondition(conditionCtx, client, workspace.ID, lock)
	notifier := notify.New(condition, workspacePollInterval, autostopNotifyCountdown)
	return func() {
		// With many "ssh" processes running, `lock.TryLockContext` can be hanging until the context canceled.
		// Without this cancellation, a CLI process with failed remote-forward could be hanging indefinitely.
		cancelCondition()
		notifier.Close()
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

type closerWithName struct {
	name   string
	closer io.Closer
}

type closerStack struct {
	sync.Mutex
	closers []closerWithName
	closed  bool
	logger  slog.Logger
	err     error
	allDone chan struct{}

	// for testing
	clock quartz.Clock
}

func newCloserStack(ctx context.Context, logger slog.Logger, clock quartz.Clock) *closerStack {
	cs := &closerStack{
		logger:  logger,
		allDone: make(chan struct{}),
		clock:   clock,
	}
	go cs.closeAfterContext(ctx)
	return cs
}

func (c *closerStack) closeAfterContext(ctx context.Context) {
	<-ctx.Done()
	c.close(ctx.Err())
}

func (c *closerStack) close(err error) {
	c.Lock()
	if c.closed {
		c.Unlock()
		<-c.allDone
		return
	}
	c.closed = true
	c.err = err
	c.Unlock()
	defer close(c.allDone)
	if len(c.closers) == 0 {
		return
	}

	// We are going to work down the stack in order.  If things close quickly, we trigger the
	// closers serially, in order. `done` is a channel that indicates the nth closer is done
	// closing, and we should trigger the (n-1) closer.  However, if things take too long we don't
	// want to wait, so we also start a ticker that works down the stack and sends on `done` as
	// well.
	next := len(c.closers) - 1
	// here we make the buffer 2x the number of closers because we could write once for it being
	// actually done and once via the countdown for each closer
	done := make(chan int, len(c.closers)*2)
	startNext := func() {
		go func(i int) {
			defer func() { done <- i }()
			cwn := c.closers[i]
			cErr := cwn.closer.Close()
			c.logger.Debug(context.Background(),
				"closed item from stack", slog.F("name", cwn.name), slog.Error(cErr))
		}(next)
		next--
	}
	done <- len(c.closers) // kick us off right away

	// start a ticking countdown in case we hang/don't close quickly
	countdown := len(c.closers) - 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.clock.TickerFunc(ctx, gracefulShutdownTimeout, func() error {
		if countdown < 0 {
			return nil
		}
		done <- countdown
		countdown--
		return nil
	}, "closerStack")

	for n := range done { // the nth closer is done
		if n == 0 {
			return
		}
		if n-1 == next {
			startNext()
		}
	}
}

func (c *closerStack) push(name string, closer io.Closer) error {
	c.Lock()
	if c.closed {
		c.Unlock()
		// since we're refusing to push it on the stack, close it now
		err := closer.Close()
		c.logger.Error(context.Background(),
			"closed item rejected push", slog.F("name", name), slog.Error(err))
		return xerrors.Errorf("already closed: %w", c.err)
	}
	c.closers = append(c.closers, closerWithName{name: name, closer: closer})
	c.Unlock()
	return nil
}

// rawSSHCopier handles copying raw SSH data between the conn and the pair (r, w).
type rawSSHCopier struct {
	conn   *gonet.TCPConn
	logger slog.Logger
	r      io.Reader
	w      io.Writer

	done chan struct{}
}

func newRawSSHCopier(logger slog.Logger, conn *gonet.TCPConn, r io.Reader, w io.Writer) *rawSSHCopier {
	return &rawSSHCopier{conn: conn, logger: logger, r: r, w: w, done: make(chan struct{})}
}

func (c *rawSSHCopier) copy(wg *sync.WaitGroup) {
	defer close(c.done)
	logCtx := context.Background()
	wg.Add(1)
	go func() {
		defer wg.Done()
		// We close connections using CloseWrite instead of Close, so that the SSH server sees the
		// closed connection while reading, and shuts down cleanly.  This will trigger the io.Copy
		// in the server-to-client direction to also be closed and the copy() routine will exit.
		// This ensures that we don't leave any state in the server, like forwarded ports if
		// copy() were to return and the underlying tailnet connection torn down before the TCP
		// session exits. This is a bit of a hack to block shut down at the application layer, since
		// we can't serialize the TCP and tailnet layers shutting down.
		//
		// Of course, if the underlying transport is broken, io.Copy will still return.
		defer func() {
			cwErr := c.conn.CloseWrite()
			c.logger.Debug(logCtx, "closed raw SSH connection for writing", slog.Error(cwErr))
		}()

		_, err := io.Copy(c.conn, c.r)
		if err != nil {
			c.logger.Error(logCtx, "copy stdin error", slog.Error(err))
		} else {
			c.logger.Debug(logCtx, "copy stdin complete")
		}
	}()
	_, err := io.Copy(c.w, c.conn)
	if err != nil {
		c.logger.Error(logCtx, "copy stdout error", slog.Error(err))
	} else {
		c.logger.Debug(logCtx, "copy stdout complete")
	}
}

func (c *rawSSHCopier) Close() error {
	err := c.conn.CloseWrite()

	// give the copy() call a chance to return on a timeout, so that we don't
	// continue tearing down and close the underlying netstack before the SSH
	// session has a chance to gracefully shut down.
	t := time.NewTimer(5 * time.Second)
	defer t.Stop()
	select {
	case <-c.done:
	case <-t.C:
	}
	return err
}

func sshDisableAutostartOption(src *serpent.Bool) serpent.Option {
	return serpent.Option{
		Flag:        "disable-autostart",
		Description: "Disable starting the workspace automatically when connecting via SSH.",
		Env:         "CODER_SSH_DISABLE_AUTOSTART",
		Value:       src,
		Default:     "false",
	}
}

type stdioErrLogReader struct {
	l slog.Logger
}

func (r stdioErrLogReader) Read(_ []byte) (int, error) {
	r.l.Error(context.Background(), "reading from stdin in stdio mode is not allowed")
	return 0, io.EOF
}

func getUsageAppName(usageApp string) codersdk.UsageAppName {
	if usageApp == disableUsageApp {
		return ""
	}

	allowedUsageApps := []string{
		string(codersdk.UsageAppNameSSH),
		string(codersdk.UsageAppNameVscode),
		string(codersdk.UsageAppNameJetbrains),
	}
	if slices.Contains(allowedUsageApps, usageApp) {
		return codersdk.UsageAppName(usageApp)
	}

	return codersdk.UsageAppNameSSH
}

func setStatsCallback(
	ctx context.Context,
	agentConn *workspacesdk.AgentConn,
	logger slog.Logger,
	networkInfoDir string,
	networkInfoInterval time.Duration,
) (<-chan error, error) {
	fs, ok := ctx.Value("fs").(afero.Fs)
	if !ok {
		fs = afero.NewOsFs()
	}
	if err := fs.MkdirAll(networkInfoDir, 0o700); err != nil {
		return nil, xerrors.Errorf("mkdir: %w", err)
	}

	// The VS Code extension obtains the PID of the SSH process to
	// read files to display logs and network info.
	//
	// We get the parent PID because it's assumed `ssh` is calling this
	// command via the ProxyCommand SSH option.
	pid := os.Getppid()

	// The VS Code extension obtains the PID of the SSH process to
	// read the file below which contains network information to display.
	//
	// We get the parent PID because it's assumed `ssh` is calling this
	// command via the ProxyCommand SSH option.
	networkInfoFilePath := filepath.Join(networkInfoDir, fmt.Sprintf("%d.json", pid))

	var (
		firstErrTime time.Time
		errCh        = make(chan error, 1)
	)
	cb := func(start, end time.Time, virtual, _ map[netlogtype.Connection]netlogtype.Counts) {
		sendErr := func(tolerate bool, err error) {
			logger.Error(ctx, "collect network stats", slog.Error(err))
			// Tolerate up to 1 minute of errors.
			if tolerate {
				if firstErrTime.IsZero() {
					logger.Info(ctx, "tolerating network stats errors for up to 1 minute")
					firstErrTime = time.Now()
				}
				if time.Since(firstErrTime) < time.Minute {
					return
				}
			}

			select {
			case errCh <- err:
			default:
			}
		}

		stats, err := collectNetworkStats(ctx, agentConn, start, end, virtual)
		if err != nil {
			sendErr(true, err)
			return
		}

		rawStats, err := json.Marshal(stats)
		if err != nil {
			sendErr(false, err)
			return
		}
		err = afero.WriteFile(fs, networkInfoFilePath, rawStats, 0o600)
		if err != nil {
			sendErr(false, err)
			return
		}

		firstErrTime = time.Time{}
	}

	now := time.Now()
	cb(now, now.Add(time.Nanosecond), map[netlogtype.Connection]netlogtype.Counts{}, map[netlogtype.Connection]netlogtype.Counts{})
	agentConn.SetConnStatsCallback(networkInfoInterval, 2048, cb)
	return errCh, nil
}

type sshNetworkStats struct {
	P2P              bool               `json:"p2p"`
	Latency          float64            `json:"latency"`
	PreferredDERP    string             `json:"preferred_derp"`
	DERPLatency      map[string]float64 `json:"derp_latency"`
	UploadBytesSec   int64              `json:"upload_bytes_sec"`
	DownloadBytesSec int64              `json:"download_bytes_sec"`
}

func collectNetworkStats(ctx context.Context, agentConn *workspacesdk.AgentConn, start, end time.Time, counts map[netlogtype.Connection]netlogtype.Counts) (*sshNetworkStats, error) {
	latency, p2p, pingResult, err := agentConn.Ping(ctx)
	if err != nil {
		return nil, err
	}
	node := agentConn.Node()
	derpMap := agentConn.DERPMap()
	derpLatency := map[string]float64{}

	// Convert DERP region IDs to friendly names for display in the UI.
	for rawRegion, latency := range node.DERPLatency {
		regionParts := strings.SplitN(rawRegion, "-", 2)
		regionID, err := strconv.Atoi(regionParts[0])
		if err != nil {
			continue
		}
		region, found := derpMap.Regions[regionID]
		if !found {
			// It's possible that a workspace agent is using an old DERPMap
			// and reports regions that do not exist. If that's the case,
			// report the region as unknown!
			region = &tailcfg.DERPRegion{
				RegionID:   regionID,
				RegionName: fmt.Sprintf("Unnamed %d", regionID),
			}
		}
		// Convert the microseconds to milliseconds.
		derpLatency[region.RegionName] = latency * 1000
	}

	totalRx := uint64(0)
	totalTx := uint64(0)
	for _, stat := range counts {
		totalRx += stat.RxBytes
		totalTx += stat.TxBytes
	}
	// Tracking the time since last request is required because
	// ExtractTrafficStats() resets its counters after each call.
	dur := end.Sub(start)
	uploadSecs := float64(totalTx) / dur.Seconds()
	downloadSecs := float64(totalRx) / dur.Seconds()

	// Sometimes the preferred DERP doesn't match the one we're actually
	// connected with. Perhaps because the agent prefers a different DERP and
	// we're using that server instead.
	preferredDerpID := node.PreferredDERP
	if pingResult.DERPRegionID != 0 {
		preferredDerpID = pingResult.DERPRegionID
	}
	preferredDerp, ok := derpMap.Regions[preferredDerpID]
	preferredDerpName := fmt.Sprintf("Unnamed %d", preferredDerpID)
	if ok {
		preferredDerpName = preferredDerp.RegionName
	}
	if _, ok := derpLatency[preferredDerpName]; !ok {
		derpLatency[preferredDerpName] = 0
	}

	return &sshNetworkStats{
		P2P:              p2p,
		Latency:          float64(latency.Microseconds()) / 1000,
		PreferredDERP:    preferredDerpName,
		DERPLatency:      derpLatency,
		UploadBytesSec:   int64(uploadSecs),
		DownloadBytesSec: int64(downloadSecs),
	}, nil
}

// Converts workspace name input to owner/workspace.agent format
// Possible valid input formats:
// workspace
// workspace.agent
// owner/workspace
// owner--workspace
// owner/workspace--agent
// owner/workspace.agent
// owner--workspace--agent
// owner--workspace.agent
func normalizeWorkspaceInput(input string) string {
	// Split on "/", "--", and "."
	parts := workspaceNameRe.Split(input, -1)

	switch len(parts) {
	case 1:
		return input // "workspace"
	case 2:
		// Either "owner/workspace" or "workspace.agent"
		if strings.Contains(input, "/") || strings.Contains(input, "--") {
			return fmt.Sprintf("%s/%s", parts[0], parts[1]) // owner/workspace
		}
		return fmt.Sprintf("%s.%s", parts[0], parts[1]) // workspace.agent
	case 3:
		return fmt.Sprintf("%s/%s.%s", parts[0], parts[1], parts[2]) // "owner/workspace.agent"
	default:
		return input // Fallback
	}
}
