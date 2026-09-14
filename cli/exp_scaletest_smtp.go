//go:build !slim

package cli

import (
	"fmt"
	"os/signal"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/scaletest/smtpmock"
	"github.com/coder/serpent"
)

func (*RootCmd) scaletestSMTP() *serpent.Command {
	var (
		hostAddress  string
		smtpPort     int64
		apiPort      int64
		purgeAtCount int64
	)
	cmd := &serpent.Command{
		Use:   "smtp",
		Short: "Start a mock SMTP server for testing",
		Long: `Start a mock SMTP server with an HTTP API server that can be used to purge
messages and get messages by email.`,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			notifyCtx, stop := signal.NotifyContext(ctx, StopSignals...)
			defer stop()
			ctx = notifyCtx

			logger := slog.Make(sloghuman.Sink(inv.Stderr)).Leveled(slog.LevelInfo)
			config := smtpmock.Config{
				HostAddress: hostAddress,
				SMTPPort:    int(smtpPort),
				APIPort:     int(apiPort),
				Logger:      logger,
			}
			srv := new(smtpmock.Server)

			if err := srv.Start(ctx, config); err != nil {
				return xerrors.Errorf("start mock SMTP server: %w", err)
			}
			defer func() {
				_ = srv.Stop()
			}()

			_, _ = fmt.Fprintf(inv.Stdout, "Mock SMTP server started on %s\n", srv.SMTPAddress())
			_, _ = fmt.Fprintf(inv.Stdout, "HTTP API server started on %s\n", srv.APIAddress())
			if purgeAtCount > 0 {
				_, _ = fmt.Fprintf(inv.Stdout, "  Auto-purge when message count reaches %d\n", purgeAtCount)
			}

			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					_, _ = fmt.Fprintf(inv.Stdout, "\nTotal messages received since last purge: %d\n", srv.MessageCount())
					return nil
				case <-ticker.C:
					count := srv.MessageCount()
					if count > 0 {
						_, _ = fmt.Fprintf(inv.Stdout, "Messages received: %d\n", count)
					}

					if purgeAtCount > 0 && int64(count) >= purgeAtCount {
						_, _ = fmt.Fprintf(inv.Stdout, "Message count (%d) reached threshold (%d). Purging...\n", count, purgeAtCount)
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
			Env:         "CODER_SCALETEST_SMTP_HOST_ADDRESS",
			Default:     "localhost",
			Description: "Host address to bind the mock SMTP and API servers.",
			Value:       serpent.StringOf(&hostAddress),
		},
		{
			Flag:        "smtp-port",
			Env:         "CODER_SCALETEST_SMTP_PORT",
			Description: "Port for the mock SMTP server. Uses a random port if not specified.",
			Value:       serpent.Int64Of(&smtpPort),
		},
		{
			Flag:        "api-port",
			Env:         "CODER_SCALETEST_SMTP_API_PORT",
			Description: "Port for the HTTP API server. Uses a random port if not specified.",
			Value:       serpent.Int64Of(&apiPort),
		},
		{
			Flag:        "purge-at-count",
			Env:         "CODER_SCALETEST_SMTP_PURGE_AT_COUNT",
			Default:     "100000",
			Description: "Maximum number of messages to keep before auto-purging. Set to 0 to disable.",
			Value:       serpent.Int64Of(&purgeAtCount),
		},
	}

	return cmd
}
