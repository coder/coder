//go:build !slim

package cli

import (
	"fmt"
	"os/signal"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/scaletest/llmmock"
	"github.com/coder/serpent"
)

func (*RootCmd) scaletestLLMMock() *serpent.Command {
	var (
		hostAddress  string
		apiPort      int64
		purgeAtCount int64
	)
	cmd := &serpent.Command{
		Use:   "llm-mock",
		Short: "Start a mock LLM API server for testing",
		Long: `Start a mock LLM API server that simulates OpenAI and Anthropic APIs with an HTTP API
server that can be used to query intercepted requests and purge stored data.`,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			notifyCtx, stop := signal.NotifyContext(ctx, StopSignals...)
			defer stop()
			ctx = notifyCtx

			logger := slog.Make(sloghuman.Sink(inv.Stderr)).Leveled(slog.LevelInfo)
			config := llmmock.Config{
				HostAddress: hostAddress,
				APIPort:     int(apiPort),
				Logger:      logger,
			}
			srv := new(llmmock.Server)

			if err := srv.Start(ctx, config); err != nil {
				return xerrors.Errorf("start mock LLM server: %w", err)
			}
			defer func() {
				_ = srv.Stop()
			}()

			_, _ = fmt.Fprintf(inv.Stdout, "Mock LLM API server started on %s\n", srv.APIAddress())
			_, _ = fmt.Fprintf(inv.Stdout, "  OpenAI endpoint: %s/v1/chat/completions\n", srv.APIAddress())
			_, _ = fmt.Fprintf(inv.Stdout, "  Anthropic endpoint: %s/v1/messages\n", srv.APIAddress())
			_, _ = fmt.Fprintf(inv.Stdout, "  Query API: %s/api/requests\n", srv.APIAddress())
			if purgeAtCount > 0 {
				_, _ = fmt.Fprintf(inv.Stdout, "  Auto-purge when request count reaches %d\n", purgeAtCount)
			}

			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					_, _ = fmt.Fprintf(inv.Stdout, "\nTotal requests received since last purge: %d\n", srv.RequestCount())
					return nil
				case <-ticker.C:
					count := srv.RequestCount()
					if count > 0 {
						_, _ = fmt.Fprintf(inv.Stdout, "Requests received: %d\n", count)
					}

					if purgeAtCount > 0 && int64(count) >= purgeAtCount {
						_, _ = fmt.Fprintf(inv.Stdout, "Request count (%d) reached threshold (%d). Purging...\n", count, purgeAtCount)
						srv.Purge()
						continue
					}
				}
			}
		},
	}

	cmd.Options = []serpent.Option{
		{
			Flag:        "host-address",
			Env:         "CODER_SCALETEST_LLM_MOCK_HOST_ADDRESS",
			Default:     "localhost",
			Description: "Host address to bind the mock LLM API server.",
			Value:       serpent.StringOf(&hostAddress),
		},
		{
			Flag:        "api-port",
			Env:         "CODER_SCALETEST_LLM_MOCK_API_PORT",
			Description: "Port for the HTTP API server. Uses a random port if not specified.",
			Value:       serpent.Int64Of(&apiPort),
		},
		{
			Flag:        "purge-at-count",
			Env:         "CODER_SCALETEST_LLM_MOCK_PURGE_AT_COUNT",
			Default:     "100000",
			Description: "Maximum number of requests to keep before auto-purging. Set to 0 to disable.",
			Value:       serpent.Int64Of(&purgeAtCount),
		},
	}

	return cmd
}
