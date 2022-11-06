package cli

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof" //nolint: gosec
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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
	"github.com/coder/coder/codersdk"
)

func workspaceAgent() *cobra.Command {
	var (
		auth         string
		pprofEnabled bool
		pprofAddress string
		noReap       bool
	)
	cmd := &cobra.Command{
		Use: "agent",
		// This command isn't useful to manually execute.
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			rawURL, err := cmd.Flags().GetString(varAgentURL)
			if err != nil {
				return xerrors.Errorf("CODER_AGENT_URL must be set: %w", err)
			}
			coderURL, err := url.Parse(rawURL)
			if err != nil {
				return xerrors.Errorf("parse %q: %w", rawURL, err)
			}

			logWriter := &lumberjack.Logger{
				Filename: filepath.Join(os.TempDir(), "coder-agent.log"),
				MaxSize:  5, // MB
			}
			defer logWriter.Close()
			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()), sloghuman.Sink(logWriter)).Leveled(slog.LevelDebug)

			isLinux := runtime.GOOS == "linux"

			// Spawn a reaper so that we don't accumulate a ton
			// of zombie processes.
			if reaper.IsInitProcess() && !noReap && isLinux {
				logger.Info(cmd.Context(), "spawning reaper process")
				// Do not start a reaper on the child process. It's important
				// to do this else we fork bomb ourselves.
				args := append(os.Args, "--no-reap")
				err := reaper.ForkReap(reaper.WithExecArgs(args...))
				if err != nil {
					logger.Error(cmd.Context(), "failed to reap", slog.Error(err))
					return xerrors.Errorf("fork reap: %w", err)
				}

				logger.Info(cmd.Context(), "reaper process exiting")
				return nil
			}

			version := buildinfo.Version()
			logger.Info(cmd.Context(), "starting agent",
				slog.F("url", coderURL),
				slog.F("auth", auth),
				slog.F("version", version),
			)
			client := codersdk.New(coderURL)
			// Set a reasonable timeout so requests can't hang forever!
			client.HTTPClient.Timeout = 10 * time.Second

			if pprofEnabled {
				srvClose := serveHandler(cmd.Context(), logger, nil, pprofAddress, "pprof")
				defer srvClose()
			} else {
				// If pprof wasn't enabled at startup, allow a
				// `kill -USR1 $agent_pid` to start it (on Unix).
				srvClose := agentStartPPROFOnUSR1(cmd.Context(), logger, pprofAddress)
				defer srvClose()
			}

			// exchangeToken returns a session token.
			// This is abstracted to allow for the same looping condition
			// regardless of instance identity auth type.
			var exchangeToken func(context.Context) (codersdk.WorkspaceAgentAuthenticateResponse, error)
			switch auth {
			case "token":
				token, err := cmd.Flags().GetString(varAgentToken)
				if err != nil {
					return xerrors.Errorf("CODER_AGENT_TOKEN must be set for token auth: %w", err)
				}
				client.SessionToken = token
			case "google-instance-identity":
				// This is *only* done for testing to mock client authentication.
				// This will never be set in a production scenario.
				var gcpClient *metadata.Client
				gcpClientRaw := cmd.Context().Value("gcp-client")
				if gcpClientRaw != nil {
					gcpClient, _ = gcpClientRaw.(*metadata.Client)
				}
				exchangeToken = func(ctx context.Context) (codersdk.WorkspaceAgentAuthenticateResponse, error) {
					return client.AuthWorkspaceGoogleInstanceIdentity(ctx, "", gcpClient)
				}
			case "aws-instance-identity":
				// This is *only* done for testing to mock client authentication.
				// This will never be set in a production scenario.
				var awsClient *http.Client
				awsClientRaw := cmd.Context().Value("aws-client")
				if awsClientRaw != nil {
					awsClient, _ = awsClientRaw.(*http.Client)
					if awsClient != nil {
						client.HTTPClient = awsClient
					}
				}
				exchangeToken = func(ctx context.Context) (codersdk.WorkspaceAgentAuthenticateResponse, error) {
					return client.AuthWorkspaceAWSInstanceIdentity(ctx)
				}
			case "azure-instance-identity":
				// This is *only* done for testing to mock client authentication.
				// This will never be set in a production scenario.
				var azureClient *http.Client
				azureClientRaw := cmd.Context().Value("azure-client")
				if azureClientRaw != nil {
					azureClient, _ = azureClientRaw.(*http.Client)
					if azureClient != nil {
						client.HTTPClient = azureClient
					}
				}
				exchangeToken = func(ctx context.Context) (codersdk.WorkspaceAgentAuthenticateResponse, error) {
					return client.AuthWorkspaceAzureInstanceIdentity(ctx)
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
				ExchangeToken: func(ctx context.Context) (string, error) {
					if exchangeToken == nil {
						return client.SessionToken, nil
					}
					resp, err := exchangeToken(ctx)
					if err != nil {
						return "", err
					}
					client.SessionToken = resp.SessionToken
					return resp.SessionToken, nil
				},
				EnvironmentVariables: map[string]string{
					"GIT_ASKPASS": executablePath,
				},
			})
			<-cmd.Context().Done()
			return closer.Close()
		},
	}

	cliflag.StringVarP(cmd.Flags(), &auth, "auth", "", "CODER_AGENT_AUTH", "token", "Specify the authentication type to use for the agent")
	cliflag.BoolVarP(cmd.Flags(), &pprofEnabled, "pprof-enable", "", "CODER_AGENT_PPROF_ENABLE", false, "Enable serving pprof metrics on the address defined by --pprof-address.")
	cliflag.BoolVarP(cmd.Flags(), &noReap, "no-reap", "", "", false, "Do not start a process reaper.")
	cliflag.StringVarP(cmd.Flags(), &pprofAddress, "pprof-address", "", "CODER_AGENT_PPROF_ADDRESS", "127.0.0.1:6060", "The address to serve pprof.")
	return cmd
}
