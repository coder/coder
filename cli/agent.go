package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
	"gopkg.in/natefinch/lumberjack.v2"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/agent/reaper"
	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/codersdk/agentsdk"
)

func workspaceAgent() *cobra.Command {
	var (
		auth         string
		logDir       string
		pprofAddress string
		noReap       bool
	)
	cmd := &cobra.Command{
		Use: "agent",
		// This command isn't useful to manually execute.
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			rawURL, err := cmd.Flags().GetString(varAgentURL)
			if err != nil {
				return xerrors.Errorf("CODER_AGENT_URL must be set: %w", err)
			}
			coderURL, err := url.Parse(rawURL)
			if err != nil {
				return xerrors.Errorf("parse %q: %w", rawURL, err)
			}

			isLinux := runtime.GOOS == "linux"

			// Spawn a reaper so that we don't accumulate a ton
			// of zombie processes.
			if reaper.IsInitProcess() && !noReap && isLinux {
				logWriter := &lumberjack.Logger{
					Filename: filepath.Join(logDir, "coder-agent-init.log"),
					MaxSize:  5, // MB
				}
				defer logWriter.Close()
				logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()), sloghuman.Sink(logWriter)).Leveled(slog.LevelDebug)

				logger.Info(ctx, "spawning reaper process")
				// Do not start a reaper on the child process. It's important
				// to do this else we fork bomb ourselves.
				args := append(os.Args, "--no-reap")
				err := reaper.ForkReap(
					reaper.WithExecArgs(args...),
					reaper.WithCatchSignals(InterruptSignals...),
				)
				if err != nil {
					logger.Error(ctx, "failed to reap", slog.Error(err))
					return xerrors.Errorf("fork reap: %w", err)
				}

				logger.Info(ctx, "reaper process exiting")
				return nil
			}

			// Handle interrupt signals to allow for graceful shutdown,
			// note that calling stopNotify disables the signal handler
			// and the next interrupt will terminate the program (you
			// probably want cancel instead).
			//
			// Note that we don't want to handle these signals in the
			// process that runs as PID 1, that's why we do this after
			// the reaper forked.
			ctx, stopNotify := signal.NotifyContext(ctx, InterruptSignals...)
			defer stopNotify()

			// dumpHandler does signal handling, so we call it after the
			// reaper.
			go dumpHandler(ctx)

			ljLogger := &lumberjack.Logger{
				Filename: filepath.Join(logDir, "coder-agent.log"),
				MaxSize:  5, // MB
			}
			defer ljLogger.Close()
			logWriter := &closeWriter{w: ljLogger}
			defer logWriter.Close()

			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()), sloghuman.Sink(logWriter)).Leveled(slog.LevelDebug)

			version := buildinfo.Version()
			logger.Info(ctx, "starting agent",
				slog.F("url", coderURL),
				slog.F("auth", auth),
				slog.F("version", version),
			)
			client := agentsdk.New(coderURL)
			client.SDK.Logger = logger
			// Set a reasonable timeout so requests can't hang forever!
			client.SDK.HTTPClient.Timeout = 10 * time.Second

			// Enable pprof handler
			// This prevents the pprof import from being accidentally deleted.
			_ = pprof.Handler
			pprofSrvClose := serveHandler(ctx, logger, nil, pprofAddress, "pprof")
			defer pprofSrvClose()

			// exchangeToken returns a session token.
			// This is abstracted to allow for the same looping condition
			// regardless of instance identity auth type.
			var exchangeToken func(context.Context) (agentsdk.AuthenticateResponse, error)
			switch auth {
			case "token":
				token, err := cmd.Flags().GetString(varAgentToken)
				if err != nil {
					return xerrors.Errorf("CODER_AGENT_TOKEN must be set for token auth: %w", err)
				}
				client.SetSessionToken(token)
			case "google-instance-identity":
				// This is *only* done for testing to mock client authentication.
				// This will never be set in a production scenario.
				var gcpClient *metadata.Client
				gcpClientRaw := ctx.Value("gcp-client")
				if gcpClientRaw != nil {
					gcpClient, _ = gcpClientRaw.(*metadata.Client)
				}
				exchangeToken = func(ctx context.Context) (agentsdk.AuthenticateResponse, error) {
					return client.AuthGoogleInstanceIdentity(ctx, "", gcpClient)
				}
			case "aws-instance-identity":
				// This is *only* done for testing to mock client authentication.
				// This will never be set in a production scenario.
				var awsClient *http.Client
				awsClientRaw := ctx.Value("aws-client")
				if awsClientRaw != nil {
					awsClient, _ = awsClientRaw.(*http.Client)
					if awsClient != nil {
						client.SDK.HTTPClient = awsClient
					}
				}
				exchangeToken = func(ctx context.Context) (agentsdk.AuthenticateResponse, error) {
					return client.AuthAWSInstanceIdentity(ctx)
				}
			case "azure-instance-identity":
				// This is *only* done for testing to mock client authentication.
				// This will never be set in a production scenario.
				var azureClient *http.Client
				azureClientRaw := ctx.Value("azure-client")
				if azureClientRaw != nil {
					azureClient, _ = azureClientRaw.(*http.Client)
					if azureClient != nil {
						client.SDK.HTTPClient = azureClient
					}
				}
				exchangeToken = func(ctx context.Context) (agentsdk.AuthenticateResponse, error) {
					return client.AuthAzureInstanceIdentity(ctx)
				}
			}

			executablePath, err := os.Executable()
			if err != nil {
				return xerrors.Errorf("getting os executable: %w", err)
			}
			err = os.Setenv("PATH", fmt.Sprintf("%s%c%s", os.Getenv("PATH"), filepath.ListSeparator, filepath.Dir(executablePath)))
			if err != nil {
				return xerrors.Errorf("add executable to $PATH: %w", err)
			}

			closer := agent.New(agent.Options{
				Client: client,
				Logger: logger,
				LogDir: logDir,
				ExchangeToken: func(ctx context.Context) (string, error) {
					if exchangeToken == nil {
						return client.SDK.SessionToken(), nil
					}
					resp, err := exchangeToken(ctx)
					if err != nil {
						return "", err
					}
					client.SetSessionToken(resp.SessionToken)
					return resp.SessionToken, nil
				},
				EnvironmentVariables: map[string]string{
					"GIT_ASKPASS": executablePath,
				},
			})
			<-ctx.Done()
			return closer.Close()
		},
	}

	cliflag.StringVarP(cmd.Flags(), &auth, "auth", "", "CODER_AGENT_AUTH", "token", "Specify the authentication type to use for the agent")
	cliflag.StringVarP(cmd.Flags(), &logDir, "log-dir", "", "CODER_AGENT_LOG_DIR", os.TempDir(), "Specify the location for the agent log files")
	cliflag.StringVarP(cmd.Flags(), &pprofAddress, "pprof-address", "", "CODER_AGENT_PPROF_ADDRESS", "127.0.0.1:6060", "The address to serve pprof.")
	cliflag.BoolVarP(cmd.Flags(), &noReap, "no-reap", "", "", false, "Do not start a process reaper.")
	return cmd
}

func serveHandler(ctx context.Context, logger slog.Logger, handler http.Handler, addr, name string) (closeFunc func()) {
	logger.Debug(ctx, "http server listening", slog.F("addr", addr), slog.F("name", name))

	// ReadHeaderTimeout is purposefully not enabled. It caused some issues with
	// websockets over the dev tunnel.
	// See: https://github.com/coder/coder/pull/3730
	//nolint:gosec
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !xerrors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, "http server listen", slog.F("name", name), slog.Error(err))
		}
	}()

	return func() {
		_ = srv.Close()
	}
}

// closeWriter is a wrapper around an io.WriteCloser that prevents
// writes after Close. This is necessary because lumberjack will
// re-open the file on write.
type closeWriter struct {
	w      io.WriteCloser
	mu     sync.Mutex // Protects following.
	closed bool
}

func (c *closeWriter) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	return c.w.Close()
}

func (c *closeWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, io.ErrClosedPipe
	}
	return c.w.Write(p)
}
