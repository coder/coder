// Command develop orchestrates the Coder development environment. It
// builds the binary, starts the API server and frontend dev server,
// sets up a first user, and handles graceful shutdown on signals.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

const (
	defaultAPIPort   = "3000"
	defaultWebPort   = "8080"
	defaultProxyPort = "3010"
	defaultPassword  = "SomeSecurePassword!"
	healthTimeout    = 60 * time.Second
	shutdownTimeout  = 15 * time.Second
)

func main() {
	var cfg devConfig

	cmd := &serpent.Command{
		Use:   "develop",
		Short: "Orchestrate the Coder development environment.",
		Options: serpent.OptionSet{
			{
				Flag:        "port",
				Env:         "CODER_DEV_PORT",
				Default:     defaultAPIPort,
				Description: "API server port.",
				Value:       serpent.Int64Of(&cfg.apiPort),
			},
			{
				Flag:        "web-port",
				Env:         "CODER_DEV_WEB_PORT",
				Default:     defaultWebPort,
				Description: "Frontend dev server port.",
				Value:       serpent.Int64Of(&cfg.webPort),
			},
			{
				Flag:        "proxy-port",
				Env:         "CODER_DEV_PROXY_PORT",
				Default:     defaultProxyPort,
				Description: "Workspace proxy port.",
				Value:       serpent.Int64Of(&cfg.proxyPort),
			},
			{
				Flag:        "agpl",
				Env:         "CODER_BUILD_AGPL",
				Description: "Build AGPL-licensed code only.",
				Value:       serpent.BoolOf(&cfg.agpl),
			},
			{
				Flag:        "access-url",
				Env:         "CODER_DEV_ACCESS_URL",
				Description: "Override access URL.",
				Value:       serpent.StringOf(&cfg.accessURL),
			},
			{
				Flag:        "password",
				Env:         "CODER_DEV_ADMIN_PASSWORD",
				Default:     defaultPassword,
				Description: "Admin user password.",
				Value:       serpent.StringOf(&cfg.password),
			},
			{
				Flag:        "use-proxy",
				Description: "Start a workspace proxy.",
				Value:       serpent.BoolOf(&cfg.useProxy),
			},
			{
				Flag:        "multi-organization",
				Description: "Create a second organization.",
				Value:       serpent.BoolOf(&cfg.multiOrg),
			},
			{
				Flag:        "debug",
				Description: "Run under Delve debugger.",
				Value:       serpent.BoolOf(&cfg.debug),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			cfg.serverExtraArgs = inv.Args

			logger := slog.Make(sloghuman.Sink(inv.Stderr))
			if err := cfg.validate(); err != nil {
				return err
			}
			if err := cfg.resolveEnv(); err != nil {
				return err
			}
			return develop(inv.Context(), logger, &cfg)
		},
	}

	err := cmd.Invoke(os.Args[1:]...).WithOS().Run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

type devConfig struct {
	apiPort     int64
	webPort     int64
	proxyPort   int64
	agpl        bool
	accessURL   string
	password    string
	useProxy    bool
	multiOrg    bool
	debug       bool
	projectRoot string
	binaryPath  string
	configDir   string
	childEnv    []string
	// Extra args after flags forwarded to "coder server".
	serverExtraArgs []string
}

func (c *devConfig) validate() error {
	if c.agpl && c.useProxy {
		return xerrors.New("cannot use both --agpl and --use-proxy")
	}
	if c.agpl && c.multiOrg {
		return xerrors.New("cannot use both --agpl and --multi-organization")
	}
	for _, p := range []struct {
		name string
		val  int64
	}{
		{"--port", c.apiPort},
		{"--web-port", c.webPort},
		{"--proxy-port", c.proxyPort},
	} {
		if p.val < 1 || p.val > 65535 {
			return xerrors.Errorf("%s must be between 1 and 65535", p.name)
		}
	}
	if c.apiPort == c.webPort {
		return xerrors.Errorf("--port %d conflicts with frontend dev server", c.webPort)
	}
	if c.useProxy && c.apiPort == c.proxyPort {
		return xerrors.Errorf("--port %d conflicts with workspace proxy", c.proxyPort)
	}
	if c.useProxy && c.webPort == c.proxyPort {
		return xerrors.Errorf("--web-port %d conflicts with --proxy-port", c.webPort)
	}
	return nil
}

// resolveEnv sets defaults, unsets leaked credentials, resolves
// filesystem paths, and computes the child process environment.
func (c *devConfig) resolveEnv() error {
	if c.accessURL == "" {
		c.accessURL = fmt.Sprintf("http://127.0.0.1:%d", c.apiPort)
	}

	// Prevent inherited credentials from leaking into child
	// processes or being picked up by config reads.
	_ = os.Unsetenv("CODER_SESSION_TOKEN")
	_ = os.Unsetenv("CODER_URL")

	var err error
	c.projectRoot, err = os.Getwd()
	if err != nil {
		return xerrors.Errorf("getting working directory: %w", err)
	}
	c.binaryPath = filepath.Join(c.projectRoot, "build",
		fmt.Sprintf("coder_%s_%s", runtime.GOOS, runtime.GOARCH))
	c.configDir = filepath.Join(c.projectRoot, ".coderv2")

	// Compute once, reused by cmd().
	c.childEnv = filterEnv(os.Environ(), "CODER_SESSION_TOKEN", "CODER_URL")

	return nil
}

// cmd builds an exec.Cmd rooted in the project directory with a
// clean child environment. The context controls process lifetime.
func (c *devConfig) cmd(ctx context.Context, bin string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = c.projectRoot
	cmd.Env = slices.Clone(c.childEnv)
	return cmd
}

// filterEnv returns env with any variables whose key matches
// exclude removed.
func filterEnv(env []string, exclude ...string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if !slices.Contains(exclude, k) {
			out = append(out, e)
		}
	}
	return out
}

// procGroup tracks child processes using an errgroup. When any
// child exits, the errgroup cancels its derived context, aborting
// all downstream operations. Graceful shutdown is handled by
// cmd.Cancel/WaitDelay on each command.
type procGroup struct {
	eg     *errgroup.Group
	ctx    context.Context
	logger slog.Logger
}

func newProcGroup(ctx context.Context, logger slog.Logger) *procGroup {
	eg, ctx := errgroup.WithContext(ctx)
	return &procGroup{eg: eg, ctx: ctx, logger: logger}
}

// Start registers a long-running command with the group. It sets up
// graceful shutdown (SIGINT on context cancel, SIGKILL after
// timeout), wires stdout/stderr to structured logging, starts the
// process, and registers a goroutine that waits for it to exit.
func (g *procGroup) Start(name string, cmd *exec.Cmd) error {
	// Guard against nil env: appending to nil creates a non-nil
	// slice that exec.Cmd treats as an explicit (empty) env.
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, "FORCE_COLOR=1")

	// Run in a new process group so signals reach the entire
	// child tree (e.g. pnpm → vite), not just the direct child.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Graceful shutdown: SIGINT the process group on context
	// cancel, escalate to SIGKILL after WaitDelay.
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}
	cmd.WaitDelay = shutdownTimeout

	named := g.logger.Named(name)
	w := &logWriter{logger: named}
	cmd.Stdout = w
	cmd.Stderr = w

	named.Info(g.ctx, "starting", slog.F("cmd", strings.Join(cmd.Args, " ")))
	if err := cmd.Start(); err != nil {
		return xerrors.Errorf("starting %s: %w", name, err)
	}

	g.eg.Go(func() error {
		err := cmd.Wait()
		if err != nil {
			return xerrors.Errorf("process %q exited: %w", name, err)
		}
		// Clean exit is still unexpected for a long-running dev
		// process. Report it so the orchestrator shuts down.
		return xerrors.Errorf("process %q exited unexpectedly", name)
	})
	return nil
}

// Wait blocks until all started processes have exited.
func (g *procGroup) Wait() error { return g.eg.Wait() }

// Ctx returns the errgroup's derived context. It cancels when the
// parent context fires (signal) or any child process exits.
func (g *procGroup) Ctx() context.Context { return g.ctx }

// poll calls cond every interval until it returns a value and true,
// or the context is canceled. If cond returns a non-nil error,
// polling stops immediately.
func poll[T any](ctx context.Context, interval time.Duration, cond func(ctx context.Context) (T, bool, error)) (T, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		case <-ticker.C:
			v, done, err := cond(ctx)
			if err != nil {
				return v, err
			}
			if done {
				return v, nil
			}
		}
	}
}

func develop(ctx context.Context, logger slog.Logger, cfg *devConfig) error {
	sigCtx, stop := signal.NotifyContext(ctx, cli.StopSignals...)
	defer stop()

	if err := preflight(sigCtx, logger, cfg); err != nil {
		return err
	}
	if err := buildBinary(sigCtx, logger, cfg); err != nil {
		return xerrors.Errorf("build: %w", err)
	}

	// Wrap in a cancelable context so deferred cleanup can
	// trigger graceful shutdown on early return.
	cancelCtx, cancelAll := context.WithCancel(sigCtx)

	group := newProcGroup(cancelCtx, logger)
	defer func() {
		cancelAll()
		_ = group.Wait()
	}()

	ctx = group.Ctx()

	if err := startServer(cfg, group); err != nil {
		return err
	}

	// The vite dev server proxies to the API and handles the
	// case where the API isn't ready yet, so start it in parallel.
	if err := group.Start("site", pnpmCmd(ctx, cfg)); err != nil {
		return xerrors.Errorf("starting frontend: %w", err)
	}

	apiURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.apiPort)
	if err := waitForHealthy(ctx, logger, apiURL); err != nil {
		return err
	}

	client, err := setupFirstUser(ctx, logger, cfg, apiURL)
	if err != nil {
		return xerrors.Errorf("setup: %w", err)
	}

	if cfg.multiOrg {
		if err := setupMultiOrg(ctx, logger, cfg, client, group); err != nil {
			logger.Warn(ctx, "multi-org setup failed, continuing",
				slog.Error(err))
		}
	}

	if err := setupDockerTemplate(ctx, logger, cfg, client); err != nil {
		logger.Warn(ctx, "docker template setup failed, continuing", slog.Error(err))
	}

	if cfg.useProxy {
		if err := setupWorkspaceProxy(ctx, cfg, client, group); err != nil {
			logger.Warn(ctx, "proxy setup failed, continuing",
				slog.Error(err))
		}
	}

	printBanner(ctx, logger, cfg)

	// Block until a signal fires or a child process exits.
	<-ctx.Done()

	waitErr := group.Wait()

	// If a signal triggered shutdown, process exit errors are
	// expected (SIGINT deaths). Report clean shutdown.
	if sigCtx.Err() != nil {
		logger.Info(ctx, "signal received, shutting down")
		return nil
	}
	return waitErr
}

func preflight(ctx context.Context, logger slog.Logger, cfg *devConfig) error {
	// Source lib.sh to run its dependency checks (bash 4+, GNU
	// getopt, make 4+) and then check command dependencies,
	// matching the original develop.sh. Prints helpful install
	// instructions on failure and exits non-zero.
	libSh := filepath.Join(cfg.projectRoot, "scripts", "lib.sh")
	libCheck := exec.CommandContext(ctx, "bash", "-c", //nolint:gosec // libSh is a project-relative path, not user input
		"source "+libSh+" && dependencies curl git go jq make pnpm")
	libCheck.Stdout = os.Stderr
	libCheck.Stderr = os.Stderr
	if err := libCheck.Run(); err != nil {
		return xerrors.New("dependency check failed, see above")
	}
	apiAddr := fmt.Sprintf("http://127.0.0.1:%d", cfg.apiPort)
	if isCoderRunning(ctx, apiAddr) {
		logger.Info(ctx, "coder is already running on this port",
			slog.F("port", cfg.apiPort))
		return nil
	}
	if isPortBusy(ctx, cfg.apiPort) {
		return xerrors.Errorf("port %d is already in use", cfg.apiPort)
	}
	if isPortBusy(ctx, cfg.webPort) {
		return xerrors.Errorf("port %d is already in use (frontend)", cfg.webPort)
	}
	if cfg.useProxy && isPortBusy(ctx, cfg.proxyPort) {
		return xerrors.Errorf("port %d is already in use (proxy)", cfg.proxyPort)
	}
	return nil
}

// buildBinary uses os.Environ() directly (not cfg.cmd()) because
// the build needs the full unfiltered parent environment.
func buildBinary(ctx context.Context, logger slog.Logger, cfg *devConfig) error {
	target := fmt.Sprintf("build/coder_%s_%s", runtime.GOOS, runtime.GOARCH)
	cmd := exec.CommandContext(ctx, "make", "-j", target)
	cmd.Dir = cfg.projectRoot
	w := &logWriter{logger: logger.Named("build")}
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Env = append(os.Environ(),
		"DEVELOP_IN_CODER="+shellBool(developInCoder()),
		"MAKE_TIMED=1",
	)
	if cfg.agpl {
		cmd.Env = append(cmd.Env, "CODER_BUILD_AGPL=1")
	}
	return cmd.Run()
}

func startServer(cfg *devConfig, group *procGroup) error {
	serverArgs := []string{
		"--global-config", cfg.configDir,
		"server",
		"--http-address", fmt.Sprintf("0.0.0.0:%d", cfg.apiPort),
		"--swagger-enable",
		"--access-url", cfg.accessURL,
		"--dangerous-allow-cors-requests=true",
		"--enable-terraform-debug-mode",
	}
	serverArgs = append(serverArgs, cfg.serverExtraArgs...)

	if cfg.debug {
		return startServerDebug(cfg, serverArgs, group)
	}
	cmd := cfg.cmd(group.Ctx(), cfg.binaryPath, serverArgs...)
	return group.Start("api", cmd)
}

func startServerDebug(cfg *devConfig, serverArgs []string, group *procGroup) error {
	ctx := group.Ctx()
	logger := group.logger

	debugBin := filepath.Join(cfg.projectRoot, "build",
		fmt.Sprintf("coder_debug_%s_%s", runtime.GOOS, runtime.GOARCH))
	dlvBinDir := filepath.Join(cfg.projectRoot, "build", ".bin")
	dlvBin := filepath.Join(dlvBinDir, "dlv")

	// Build debug binary and install dlv in parallel.
	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		buildArgs := []string{
			"--os", runtime.GOOS, "--arch", runtime.GOARCH,
			"--output", debugBin, "--debug",
		}
		if cfg.agpl {
			buildArgs = append(buildArgs, "--agpl")
		}
		cmd := cfg.cmd(egCtx,
			filepath.Join(cfg.projectRoot, "scripts", "build_go.sh"),
			buildArgs...)
		w := &logWriter{logger: logger.Named("build-debug")}
		cmd.Stdout = w
		cmd.Stderr = w
		return cmd.Run()
	})
	eg.Go(func() error {
		goVer := strings.TrimPrefix(runtime.Version(), "go")
		cmd := cfg.cmd(egCtx, "go", "install",
			"github.com/go-delve/delve/cmd/dlv@latest")
		cmd.Env = append(cmd.Env,
			"GOBIN="+dlvBinDir, "GOTOOLCHAIN=go"+goVer)
		w := &logWriter{logger: logger.Named("dlv-install")}
		cmd.Stdout = w
		cmd.Stderr = w
		return cmd.Run()
	})
	if err := eg.Wait(); err != nil {
		return xerrors.Errorf("debug build: %w", err)
	}

	srvCmd := cfg.cmd(ctx, debugBin, serverArgs...)
	if err := group.Start("api", srvCmd); err != nil {
		return err
	}

	dlvCmd := cfg.cmd(ctx, dlvBin, "attach",
		strconv.Itoa(srvCmd.Process.Pid),
		"--headless", "--continue",
		"--listen", "127.0.0.1:12345",
		"--accept-multiclient")
	if err := group.Start("dlv", dlvCmd); err != nil {
		return xerrors.Errorf("attaching dlv: %w", err)
	}
	logger.Info(ctx, "delve debugger listening", slog.F("addr", "127.0.0.1:12345"))
	return nil
}

func waitForHealthy(ctx context.Context, logger slog.Logger, apiURL string) error {
	logger.Info(ctx, "waiting for server to become ready")
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()

	_, err := poll(ctx, 500*time.Millisecond,
		func(ctx context.Context) (struct{}, bool, error) {
			req, err := http.NewRequestWithContext(
				ctx, "GET", apiURL+"/healthz", nil)
			if err != nil {
				return struct{}{}, false, nil
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return struct{}{}, false, nil
			}
			_ = resp.Body.Close()
			return struct{}{}, resp.StatusCode == http.StatusOK, nil
		})
	if err != nil {
		return xerrors.Errorf("server did not become ready in %s: %w", healthTimeout, err)
	}
	logger.Info(ctx, "server is ready to accept connections")
	return nil
}

func setupFirstUser(ctx context.Context, logger slog.Logger, cfg *devConfig, apiURL string) (*codersdk.Client, error) {
	serverURL, _ := url.Parse(apiURL)
	client := codersdk.New(serverURL)
	cfgRoot := config.Root(cfg.configDir)

	// Try reusing an existing session.
	loggedIn := false
	if token, err := cfgRoot.Session().Read(); err == nil && token != "" {
		client.SetSessionToken(token)
		if _, err := client.User(ctx, codersdk.Me); err == nil {
			loggedIn = true
		} else {
			client.SetSessionToken("")
		}
	}

	if !loggedIn {
		hasUser, err := client.HasFirstUser(ctx)
		if err != nil {
			return nil, xerrors.Errorf("checking first user: %w", err)
		}
		if !hasUser {
			logger.Info(ctx, "creating first user",
				slog.F("email", "admin@coder.com"),
				slog.F("password", cfg.password))
			_, err := client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
				Email:    "admin@coder.com",
				Username: "admin",
				Name:     "Admin User",
				Password: cfg.password,
			})
			if err != nil {
				return nil, xerrors.Errorf("creating first user: %w", err)
			}
		}

		loginResp, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{
			Email:    "admin@coder.com",
			Password: cfg.password,
		})
		if err != nil {
			return nil, xerrors.Errorf("login: %w", err)
		}
		client.SetSessionToken(loginResp.SessionToken)

		if err := cfgRoot.Session().Write(loginResp.SessionToken); err != nil {
			return nil, xerrors.Errorf("writing session: %w", err)
		}
		if err := cfgRoot.URL().Write(apiURL); err != nil {
			return nil, xerrors.Errorf("writing url: %w", err)
		}
	}
	logger.Info(ctx, "authenticated as admin user", slog.F("email", "admin@coder.com"))

	// Look up the default org for member creation.
	defaultOrg, err := client.OrganizationByName(ctx, codersdk.DefaultOrganization)
	if err != nil {
		return nil, xerrors.Errorf("looking up default org: %w", err)
	}

	// Member user is best-effort.
	if _, err := client.User(ctx, "member"); err != nil {
		_, err = client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "member@coder.com",
			Username:        "member",
			Name:            "Regular User",
			Password:        cfg.password,
			UserLoginType:   codersdk.LoginTypePassword,
			OrganizationIDs: []uuid.UUID{defaultOrg.ID},
		})
		if err != nil {
			logger.Warn(ctx, "failed to create member user", slog.Error(err))
		} else {
			logger.Info(ctx, "created member user", slog.F("email", "member@coder.com"))
		}
	}

	return client, nil
}

func setupMultiOrg(ctx context.Context, logger slog.Logger, cfg *devConfig, client *codersdk.Client, group *procGroup) error {
	const orgName = "second-organization"

	org, err := client.OrganizationByName(ctx, orgName)
	if err != nil {
		logger.Info(ctx, "creating organization",
			slog.F("name", orgName))
		org, err = client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{Name: orgName})
		if err != nil {
			return xerrors.Errorf("creating org: %w", err)
		}
	}

	members, err := client.OrganizationMembers(ctx, org.ID)
	if err == nil {
		found := false
		for _, m := range members {
			if m.Username == "member" {
				found = true
				break
			}
		}
		if !found {
			if _, err := client.PostOrganizationMember(ctx, org.ID, "member"); err != nil {
				logger.Warn(ctx, "failed to add member to org", slog.Error(err))
			}
		}
	}

	cmd := cfg.cmd(ctx, cfg.binaryPath,
		"--global-config", cfg.configDir,
		"provisionerd", "start",
		"--tag", "scope=organization",
		"--name", "second-org-daemon",
		"--org", orgName)
	return group.Start("ext-provisioner", cmd)
}

func setupWorkspaceProxy(ctx context.Context, cfg *devConfig, client *codersdk.Client, group *procGroup) error {
	_ = client.DeleteWorkspaceProxyByName(ctx, "local-proxy")

	resp, err := client.CreateWorkspaceProxy(ctx,
		codersdk.CreateWorkspaceProxyRequest{
			Name:        "local-proxy",
			DisplayName: "Local Proxy",
			Icon:        "/emojis/1f4bb.png",
		})
	if err != nil {
		return xerrors.Errorf("creating proxy: %w", err)
	}

	cmd := cfg.cmd(ctx, cfg.binaryPath,
		"--global-config", cfg.configDir,
		"wsproxy", "server",
		"--dangerous-allow-cors-requests=true",
		"--http-address", fmt.Sprintf("localhost:%d", cfg.proxyPort),
		"--proxy-session-token", resp.ProxyToken,
		"--primary-access-url", fmt.Sprintf("http://localhost:%d", cfg.apiPort))
	return group.Start("proxy", cmd)
}

// setupDockerTemplate creates a Docker template from the embedded
// starter example when Docker is available and the template does
// not already exist. Uses the SDK's ExampleID field so the server
// resolves the template archive internally, no CLI shelling needed.
func setupDockerTemplate(ctx context.Context, logger slog.Logger, cfg *devConfig, client *codersdk.Client) error {
	// Check if Docker is available.
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		logger.Debug(ctx, "docker not available, skipping template setup")
		return nil
	}

	// Resolve Docker host for template variables.
	dockerHost := ""
	if out, err := exec.CommandContext(ctx, "docker", "context", "inspect",
		"--format", "{{ .Endpoints.docker.Host }}").Output(); err == nil {
		dockerHost = strings.TrimSpace(string(out))
	}

	if err := createTemplateInOrg(ctx, logger, client, codersdk.DefaultOrganization, dockerHost); err != nil {
		return err
	}

	if cfg.multiOrg {
		if err := createTemplateInOrg(ctx, logger, client, "second-organization", dockerHost); err != nil {
			logger.Warn(ctx, "failed to create docker template in second org",
				slog.Error(err))
		}
	}

	return nil
}

// waitForVersion polls until a template version's provisioner job
// reaches a terminal state.
func waitForVersion(ctx context.Context, client *codersdk.Client, id uuid.UUID) (codersdk.TemplateVersion, error) {
	return poll(ctx, 500*time.Millisecond,
		func(ctx context.Context) (codersdk.TemplateVersion, bool, error) {
			v, err := client.TemplateVersion(ctx, id)
			if err != nil {
				return v, false, err
			}
			switch v.Job.Status {
			case codersdk.ProvisionerJobSucceeded:
				return v, true, nil
			case codersdk.ProvisionerJobFailed:
				return v, false, xerrors.Errorf("job failed: %s", v.Job.Error)
			case codersdk.ProvisionerJobCanceled:
				return v, false, xerrors.New("job was canceled")
			default:
				return v, false, nil // Still pending/running.
			}
		})
}

// createTemplateInOrg ensures the "docker" template exists in the
// given org, creating it from the embedded example if needed.
func createTemplateInOrg(ctx context.Context, logger slog.Logger, client *codersdk.Client, orgName string, dockerHost string) error {
	org, err := client.OrganizationByName(ctx, orgName)
	if err != nil {
		return xerrors.Errorf("looking up org %q: %w", orgName, err)
	}
	if _, err := client.TemplateByName(ctx, org.ID, "docker"); err == nil {
		logger.Debug(ctx, "docker template already exists",
			slog.F("org", orgName))
		return nil
	}
	version, err := client.CreateTemplateVersion(ctx, org.ID,
		codersdk.CreateTemplateVersionRequest{
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			ExampleID:     "docker",
			Provisioner:   codersdk.ProvisionerTypeTerraform,
			UserVariableValues: []codersdk.VariableValue{
				{Name: "docker_arch", Value: runtime.GOARCH},
				{Name: "docker_host", Value: dockerHost},
			},
		})
	if err != nil {
		return xerrors.Errorf("creating version: %w", err)
	}
	version, err = waitForVersion(ctx, client, version.ID)
	if err != nil {
		return err
	}
	_, err = client.CreateTemplate(ctx, org.ID,
		codersdk.CreateTemplateRequest{
			Name:      "docker",
			VersionID: version.ID,
		})
	if err != nil {
		return xerrors.Errorf("creating template: %w", err)
	}
	logger.Info(ctx, "docker template created in org",
		slog.F("org", orgName))
	return nil
}

func pnpmCmd(ctx context.Context, cfg *devConfig) *exec.Cmd {
	cmd := cfg.cmd(ctx, "pnpm", "--dir", "./site", "dev", "--host")
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("PORT=%d", cfg.webPort),
		fmt.Sprintf("CODER_HOST=http://127.0.0.1:%d", cfg.apiPort),
	)
	return cmd
}

func printBanner(ctx context.Context, logger slog.Logger, cfg *devConfig) {
	ifaces := []string{"localhost"}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				ifaces = append(ifaces, ipnet.IP.String())
			}
		}
	}
	var b strings.Builder
	w := 64
	line := func(content string) {
		_, _ = fmt.Fprintf(&b, "║ %-*s ║\n", w, content)
	}
	divider := "╔" + strings.Repeat("═", w+2) + "╗"
	bottom := "╚" + strings.Repeat("═", w+2) + "╝"

	_, _ = fmt.Fprintln(&b)
	_, _ = fmt.Fprintln(&b, divider)
	line("")
	line("           Coder is now running in development mode.")
	line("")
	for _, h := range ifaces {
		line(fmt.Sprintf("API:    http://%s:%d", h, cfg.apiPort))
		line(fmt.Sprintf("Web UI: http://%s:%d", h, cfg.webPort))
		if cfg.useProxy {
			line(fmt.Sprintf("Proxy:  http://%s:%d", h, cfg.proxyPort))
		}
	}
	line("")
	line("Use ./scripts/coder-dev.sh to talk to this instance!")
	line(fmt.Sprintf("  alias cdr=%s/scripts/coder-dev.sh", cfg.projectRoot))
	line("")
	_, _ = fmt.Fprintln(&b, bottom)
	logger.Info(ctx, b.String())
}

// logWriter adapts an slog.Logger into an io.Writer. Each complete
// line of text written is logged at Info level. Partial lines are
// buffered until a newline arrives. Safe for concurrent use.
type logWriter struct {
	logger slog.Logger
	mu     sync.Mutex
	buf    []byte
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(w.buf[:idx])
		w.buf = w.buf[idx+1:]
		if line != "" {
			w.logger.Info(context.Background(), line)
		}
	}
	return len(p), nil
}

func isPortBusy(ctx context.Context, port int64) bool {
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func isCoderRunning(ctx context.Context, baseURL string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/v2/buildinfo", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var info struct{ Version string }
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return false
	}
	return info.Version != ""
}

// shellBool returns "1" for true and "0" for false (shell convention).
func shellBool(b bool) string { //nolint:revive // trivial bool-to-string helper
	if b {
		return "1"
	}
	return "0"
}

func developInCoder() bool {
	return os.Getenv("DEVELOP_IN_CODER") == "1" || os.Getenv("CODER_AGENT_URL") != ""
}
