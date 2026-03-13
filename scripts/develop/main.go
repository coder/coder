// Command develop orchestrates the Coder development environment. It
// builds the binary, starts the API server and frontend dev server, sets
// up a first user, and handles graceful shutdown on signals.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

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
	defaultAPIPort  = 3000
	webPort         = 8080
	proxyPort       = 3010
	defaultPassword = "SomeSecurePassword!"
	healthTimeout   = 60 * time.Second
	shutdownTimeout = 15 * time.Second
)

func main() {
	var (
		port      int64
		agpl      bool
		accessURL string
		password  string
		useProxy  bool
		multiOrg  bool
		debug     bool
	)

	cmd := &serpent.Command{
		Use:   "develop",
		Short: "Orchestrate the Coder development environment.",
		Options: serpent.OptionSet{
			{
				Flag:        "port",
				Env:         "CODER_DEV_PORT",
				Default:     strconv.Itoa(defaultAPIPort),
				Description: "API server port.",
				Value:       serpent.Int64Of(&port),
			},
			{
				Flag:        "agpl",
				Env:         "CODER_BUILD_AGPL",
				Description: "Build AGPL-licensed code only.",
				Value:       serpent.BoolOf(&agpl),
			},
			{
				Flag:        "access-url",
				Env:         "CODER_DEV_ACCESS_URL",
				Description: "Override access URL.",
				Value:       serpent.StringOf(&accessURL),
			},
			{
				Flag:        "password",
				Env:         "CODER_DEV_ADMIN_PASSWORD",
				Default:     defaultPassword,
				Description: "Admin user password.",
				Value:       serpent.StringOf(&password),
			},
			{
				Flag:        "use-proxy",
				Description: "Start a workspace proxy.",
				Value:       serpent.BoolOf(&useProxy),
			},
			{
				Flag:        "multi-organization",
				Description: "Create a second organization.",
				Value:       serpent.BoolOf(&multiOrg),
			},
			{
				Flag:        "debug",
				Description: "Run under Delve debugger.",
				Value:       serpent.BoolOf(&debug),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			logger := slog.Make(sloghuman.Sink(inv.Stderr))
			cfg := &devConfig{
				apiPort:         int(port),
				agpl:            agpl,
				accessURL:       accessURL,
				password:        password,
				useProxy:        useProxy,
				multiOrg:        multiOrg,
				debug:           debug,
				serverExtraArgs: inv.Args,
			}
			if err := cfg.validate(); err != nil {
				return err
			}
			if err := cfg.resolvePaths(); err != nil {
				return err
			}
			return develop(inv.Context(), logger, cfg)
		},
	}

	err := cmd.Invoke(os.Args[1:]...).WithOS().Run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

type devConfig struct {
	apiPort    int
	agpl       bool
	accessURL  string
	password   string
	useProxy   bool
	multiOrg   bool
	debug      bool
	projectRoot string
	binaryPath  string
	configDir   string
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
	if c.apiPort < 1 || c.apiPort > 65535 {
		return xerrors.New("--port must be between 1 and 65535")
	}
	if c.apiPort == webPort {
		return xerrors.Errorf("--port %d conflicts with frontend dev server", webPort)
	}
	if c.useProxy && c.apiPort == proxyPort {
		return xerrors.Errorf("--port %d conflicts with workspace proxy", proxyPort)
	}
	return nil
}

func (c *devConfig) resolvePaths() error {
	if c.accessURL == "" {
		c.accessURL = fmt.Sprintf("http://127.0.0.1:%d", c.apiPort)
	}

	// Prevent inherited credentials from leaking into child processes.
	os.Unsetenv("CODER_SESSION_TOKEN")
	os.Unsetenv("CODER_URL")

	var err error
	c.projectRoot, err = os.Getwd()
	if err != nil {
		return xerrors.Errorf("getting working directory: %w", err)
	}
	c.binaryPath = filepath.Join(c.projectRoot, "build",
		fmt.Sprintf("coder_%s_%s", runtime.GOOS, runtime.GOARCH))
	c.configDir = filepath.Join(c.projectRoot, ".coderv2")
	return nil
}

func develop(ctx context.Context, logger slog.Logger, cfg *devConfig) error {
	if err := preflight(logger, cfg); err != nil {
		return err
	}
	if err := buildBinary(ctx, logger, cfg); err != nil {
		return xerrors.Errorf("build: %w", err)
	}

	ctx, stop := signal.NotifyContext(ctx, cli.StopSignals...)
	defer stop()

	var procs []*proc
	defer func() {
		shutdownProcs(logger, procs, shutdownTimeout)
	}()

	apiProc, err := startServer(ctx, logger, cfg, &procs)
	if err != nil {
		return err
	}
	procs = append(procs, apiProc)

	// The vite dev server proxies to the API and handles the case where
	// the API isn't ready yet, so start it in parallel.
	feProc, err := startProc(ctx, logger, "site", pnpmCmd(cfg))
	if err != nil {
		return xerrors.Errorf("starting frontend: %w", err)
	}
	procs = append(procs, feProc)

	apiURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.apiPort)
	if err := waitForHealthy(ctx, logger, apiURL, apiProc); err != nil {
		return err
	}

	client, err := setupFirstUser(ctx, logger, cfg, apiURL)
	if err != nil {
		return xerrors.Errorf("setup: %w", err)
	}

	if cfg.multiOrg {
		setupMultiOrg(ctx, logger, cfg, client, &procs)
	}
	if cfg.useProxy {
		setupWorkspaceProxy(ctx, logger, cfg, client, &procs)
	}

	printBanner(logger, cfg)

	// Block until a child exits or we get a signal.
	exited := mergeExits(procs)
	select {
	case p := <-exited:
		return xerrors.Errorf("process %q exited unexpectedly: %w", p.name, p.err)
	case <-ctx.Done():
		logger.Info(ctx, "signal received, shutting down")
		return nil
	}
}

func preflight(logger slog.Logger, cfg *devConfig) error {
	for _, dep := range []string{"go", "make", "pnpm"} {
		if _, err := exec.LookPath(dep); err != nil {
			return xerrors.Errorf("required dependency %q not found in PATH", dep)
		}
	}
	apiAddr := fmt.Sprintf("http://127.0.0.1:%d", cfg.apiPort)
	if isCoderRunning(apiAddr) {
		logger.Info(context.Background(), "Coder already running, exiting",
			slog.F("port", cfg.apiPort))
		os.Exit(0)
	}
	if isPortBusy(cfg.apiPort) {
		return xerrors.Errorf("port %d is already in use", cfg.apiPort)
	}
	if isPortBusy(webPort) {
		return xerrors.Errorf("port %d is already in use (frontend)", webPort)
	}
	return nil
}

func buildBinary(ctx context.Context, logger slog.Logger, cfg *devConfig) error {
	target := fmt.Sprintf("build/coder_%s_%s", runtime.GOOS, runtime.GOARCH)
	cmd := exec.CommandContext(ctx, "make", "-j", target)
	cmd.Dir = cfg.projectRoot
	w := &logWriter{logger: logger.Named("build")}
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Env = appendEnv(os.Environ(), "DEVELOP_IN_CODER", boolStr(developInCoder()))
	if cfg.agpl {
		cmd.Env = appendEnv(cmd.Env, "CODER_BUILD_AGPL", "1")
	}
	return cmd.Run()
}

func startServer(ctx context.Context, logger slog.Logger, cfg *devConfig, procs *[]*proc) (*proc, error) {
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
		return startServerDebug(ctx, logger, cfg, serverArgs, procs)
	}
	cmd := exec.CommandContext(ctx, cfg.binaryPath, serverArgs...)
	cmd.Dir = cfg.projectRoot
	cmd.Env = cleanChildEnv()
	return startProc(ctx, logger, "api", cmd)
}

func startServerDebug(ctx context.Context, logger slog.Logger, cfg *devConfig, serverArgs []string, procs *[]*proc) (*proc, error) {
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
		cmd := exec.CommandContext(egCtx,
			filepath.Join(cfg.projectRoot, "scripts", "build_go.sh"), buildArgs...)
		cmd.Dir = cfg.projectRoot
		w := &logWriter{logger: logger.Named("build-debug")}
		cmd.Stdout = w
		cmd.Stderr = w
		cmd.Env = cleanChildEnv()
		return cmd.Run()
	})
	eg.Go(func() error {
		goVer := strings.TrimPrefix(runtime.Version(), "go")
		cmd := exec.CommandContext(egCtx, "go", "install",
			"github.com/go-delve/delve/cmd/dlv@latest")
		cmd.Dir = cfg.projectRoot
		w := &logWriter{logger: logger.Named("dlv-install")}
		cmd.Stdout = w
		cmd.Stderr = w
		cmd.Env = appendEnv(os.Environ(), "GOBIN", dlvBinDir, "GOTOOLCHAIN", "go"+goVer)
		return cmd.Run()
	})
	if err := eg.Wait(); err != nil {
		return nil, xerrors.Errorf("debug build: %w", err)
	}

	// Start the debug binary, then attach dlv.
	srvCmd := exec.CommandContext(ctx, debugBin, serverArgs...)
	srvCmd.Dir = cfg.projectRoot
	srvCmd.Env = cleanChildEnv()
	apiProc, err := startProc(ctx, logger, "api", srvCmd)
	if err != nil {
		return nil, err
	}

	dlvCmd := exec.CommandContext(ctx, dlvBin, "attach", strconv.Itoa(srvCmd.Process.Pid),
		"--headless", "--continue", "--listen", "127.0.0.1:12345",
		"--accept-multiclient")
	dlvCmd.Dir = cfg.projectRoot
	dlvProc, err := startProc(ctx, logger, "dlv", dlvCmd)
	if err != nil {
		_ = srvCmd.Process.Kill()
		return nil, xerrors.Errorf("attaching dlv: %w", err)
	}
	*procs = append(*procs, dlvProc)
	logger.Info(ctx, "dlv listening", slog.F("addr", "127.0.0.1:12345"))
	return apiProc, nil
}

func waitForHealthy(ctx context.Context, logger slog.Logger, apiURL string, server *proc) error {
	logger.Info(ctx, "waiting for server to become ready")
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-server.done:
			return xerrors.Errorf("server exited before becoming healthy: %w", server.err)
		case <-ctx.Done():
			return xerrors.Errorf("server did not become ready in %s", healthTimeout)
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, "GET", apiURL+"/healthz", nil)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					logger.Info(ctx, "server is ready")
					return nil
				}
			}
		}
	}
}

func setupFirstUser(ctx context.Context, logger slog.Logger, cfg *devConfig, apiURL string) (*codersdk.Client, error) {
	serverURL, _ := url.Parse(apiURL)
	client := codersdk.New(serverURL)
	cfgRoot := config.Root(cfg.configDir)

	// Reuse an existing session if still valid.
	if token, err := cfgRoot.Session().Read(); err == nil && token != "" {
		client.SetSessionToken(token)
		if _, err := client.User(ctx, codersdk.Me); err == nil {
			logger.Info(ctx, "already logged in, skipping setup")
			return client, nil
		}
		client.SetSessionToken("")
	}

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
	logger.Info(ctx, "logged in", slog.F("email", "admin@coder.com"))

	// Member user is best-effort.
	_, err = client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
		Email:         "member@coder.com",
		Username:      "member",
		Name:          "Regular User",
		Password:      cfg.password,
		UserLoginType: codersdk.LoginTypePassword,
	})
	if err != nil {
		logger.Warn(ctx, "member user not created (may already exist)", slog.Error(err))
	} else {
		logger.Info(ctx, "created member user", slog.F("email", "member@coder.com"))
	}

	return client, nil
}

func setupMultiOrg(ctx context.Context, logger slog.Logger, cfg *devConfig, client *codersdk.Client, procs *[]*proc) {
	const orgName = "second-organization"

	org, err := client.OrganizationByName(ctx, orgName)
	if err != nil {
		logger.Info(ctx, "creating organization", slog.F("name", orgName))
		org, err = client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{Name: orgName})
		if err != nil {
			logger.Error(ctx, "failed to create org", slog.Error(err))
			return
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

	provCmd := exec.CommandContext(ctx, cfg.binaryPath,
		"--global-config", cfg.configDir,
		"provisionerd", "start",
		"--tag", "scope=organization",
		"--name", "second-org-daemon",
		"--org", orgName)
	provCmd.Dir = cfg.projectRoot
	provCmd.Env = cleanChildEnv()
	p, err := startProc(ctx, logger, "ext-provisioner", provCmd)
	if err != nil {
		logger.Error(ctx, "failed to start provisioner", slog.Error(err))
		return
	}
	*procs = append(*procs, p)
}

func setupWorkspaceProxy(ctx context.Context, logger slog.Logger, cfg *devConfig, client *codersdk.Client, procs *[]*proc) {
	_ = client.DeleteWorkspaceProxyByName(ctx, "local-proxy")

	resp, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
		Name:        "local-proxy",
		DisplayName: "Local Proxy",
		Icon:        "/emojis/1f4bb.png",
	})
	if err != nil {
		logger.Error(ctx, "failed to create proxy", slog.Error(err))
		return
	}

	proxyCmd := exec.CommandContext(ctx, cfg.binaryPath,
		"--global-config", cfg.configDir,
		"wsproxy", "server",
		"--dangerous-allow-cors-requests=true",
		"--http-address", fmt.Sprintf("localhost:%d", proxyPort),
		"--proxy-session-token", resp.ProxyToken,
		"--primary-access-url", fmt.Sprintf("http://localhost:%d", cfg.apiPort))
	proxyCmd.Dir = cfg.projectRoot
	proxyCmd.Env = cleanChildEnv()
	p, err := startProc(ctx, logger, "proxy", proxyCmd)
	if err != nil {
		logger.Error(ctx, "failed to start proxy", slog.Error(err))
		return
	}
	*procs = append(*procs, p)
}

func pnpmCmd(cfg *devConfig) *exec.Cmd {
	cmd := exec.Command("pnpm", "--dir", "./site", "dev", "--host")
	cmd.Dir = cfg.projectRoot
	cmd.Env = appendEnv(cleanChildEnv(),
		"PORT", strconv.Itoa(webPort),
		"CODER_HOST", fmt.Sprintf("http://127.0.0.1:%d", cfg.apiPort))
	return cmd
}

func printBanner(logger slog.Logger, cfg *devConfig) {
	ifaces := []string{"localhost"}
	if out, err := exec.Command("hostname", "-I").Output(); err == nil {
		for _, ip := range strings.Fields(string(out)) {
			ifaces = append(ifaces, ip)
		}
	}
	var b strings.Builder
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "====================================================================")
	fmt.Fprintln(&b, "==            Coder is now running in development mode.           ==")
	for _, h := range ifaces {
		fmt.Fprintf(&b, "==  API:    http://%s:%d\n", h, cfg.apiPort)
		fmt.Fprintf(&b, "==  Web UI: http://%s:%d\n", h, webPort)
		if cfg.useProxy {
			fmt.Fprintf(&b, "==  Proxy:  http://%s:%d\n", h, proxyPort)
		}
	}
	fmt.Fprintln(&b, "==")
	fmt.Fprintln(&b, "==  Use ./scripts/coder-dev.sh to talk to this instance!")
	fmt.Fprintf(&b, "==  alias cdr=%s/scripts/coder-dev.sh\n", cfg.projectRoot)
	fmt.Fprintln(&b, "====================================================================")
	logger.Info(context.Background(), b.String())
}

// proc tracks a running child process.
type proc struct {
	name string
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

func startProc(ctx context.Context, logger slog.Logger, name string, cmd *exec.Cmd) (*proc, error) {
	named := logger.Named(name)
	w := &logWriter{logger: named}
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Env = appendEnv(cmd.Env, "FORCE_COLOR", "1")

	named.Info(ctx, "starting", slog.F("cmd", strings.Join(cmd.Args, " ")))
	if err := cmd.Start(); err != nil {
		return nil, xerrors.Errorf("starting %s: %w", name, err)
	}
	p := &proc{name: name, cmd: cmd, done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// mergeExits returns a channel that receives the first proc to exit.
func mergeExits(procs []*proc) <-chan *proc {
	ch := make(chan *proc, 1)
	for _, p := range procs {
		go func() {
			<-p.done
			select {
			case ch <- p:
			default:
			}
		}()
	}
	return ch
}

// shutdownProcs sends SIGINT to each tracked process and waits up to
// timeout for them to exit before escalating to SIGKILL. On
// signal-triggered shutdown, children in the same process group will
// have already received the signal from the kernel, so the explicit
// SIGINT is redundant but harmless.
func shutdownProcs(logger slog.Logger, procs []*proc, timeout time.Duration) {
	if len(procs) == 0 {
		return
	}
	for _, p := range procs {
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Signal(os.Interrupt)
		}
	}
	allDone := make(chan struct{})
	go func() {
		for _, p := range procs {
			<-p.done
		}
		close(allDone)
	}()
	select {
	case <-allDone:
	case <-time.After(timeout):
		logger.Warn(context.Background(), "shutdown timeout, sending SIGKILL")
		for _, p := range procs {
			if p.cmd.Process != nil {
				_ = p.cmd.Process.Kill()
			}
		}
		<-allDone
	}
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

func isPortBusy(port int) bool {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

func isCoderRunning(baseURL string) bool {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(baseURL + "/api/v2/buildinfo")
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

func cleanChildEnv() []string {
	var out []string
	for _, e := range os.Environ() {
		if i := strings.IndexByte(e, '='); i >= 0 {
			k := e[:i]
			if k == "CODER_SESSION_TOKEN" || k == "CODER_URL" {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// appendEnv appends key=value pairs to an environment slice. The
// variadic args are consumed in pairs: key1, val1, key2, val2, etc.
func appendEnv(env []string, kvs ...string) []string {
	for i := 0; i+1 < len(kvs); i += 2 {
		env = append(env, kvs[i]+"="+kvs[i+1])
	}
	return env
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func developInCoder() bool {
	return os.Getenv("DEVELOP_IN_CODER") == "1" || os.Getenv("CODER_AGENT_URL") != ""
}
