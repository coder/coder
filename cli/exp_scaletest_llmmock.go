//go:build !slim

package cli

import (
	"fmt"
	"os/signal"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/scaletest/llmmock"
	"github.com/coder/serpent"
)

func (*RootCmd) scaletestLLMMock() *serpent.Command {
	var (
		address             string
		artificialLatency   time.Duration
		responsePayloadSize int64

		pprofEnable  bool
		pprofAddress string

		traceEnable bool
	)
	cmd := &serpent.Command{
		Use:   "llm-mock",
		Short: "Start a mock LLM API server for testing",
		Long:  `Start a mock LLM API server that simulates OpenAI and Anthropic APIs`,
		Handler: func(inv *serpent.Invocation) error {
			ctx, stop := signal.NotifyContext(inv.Context(), StopSignals...)
			defer stop()

			logger := slog.Make(sloghuman.Sink(inv.Stderr)).Leveled(slog.LevelInfo)

			if pprofEnable {
				closePprof := ServeHandler(ctx, logger, nil, pprofAddress, "pprof")
				defer closePprof()
				logger.Info(ctx, "pprof server started", slog.F("address", pprofAddress))
			}

			config := llmmock.Config{
				Address:             address,
				Logger:              logger,
				ArtificialLatency:   artificialLatency,
				ResponsePayloadSize: int(responsePayloadSize),
				PprofEnable:         pprofEnable,
				PprofAddress:        pprofAddress,
				TraceEnable:         traceEnable,
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

			<-ctx.Done()
			return nil
		},
	}

	cmd.Options = []serpent.Option{
		{
			Flag:        "address",
			Env:         "CODER_SCALETEST_LLM_MOCK_ADDRESS",
			Default:     "localhost",
			Description: "Address to bind the mock LLM API server. Can include a port (e.g., 'localhost:8080' or ':8080'). Uses a random port if no port is specified.",
			Value:       serpent.StringOf(&address),
		},
		{
			Flag:        "artificial-latency",
			Env:         "CODER_SCALETEST_LLM_MOCK_ARTIFICIAL_LATENCY",
			Default:     "0s",
			Description: "Artificial latency to add to each response (e.g., 100ms, 1s). Simulates slow upstream processing.",
			Value:       serpent.DurationOf(&artificialLatency),
		},
		{
			Flag:        "response-payload-size",
			Env:         "CODER_SCALETEST_LLM_MOCK_RESPONSE_PAYLOAD_SIZE",
			Default:     "0",
			Description: "Size in bytes of the response payload. If 0, uses default context-aware responses.",
			Value:       serpent.Int64Of(&responsePayloadSize),
		},
		{
			Flag:        "pprof-enable",
			Env:         "CODER_SCALETEST_LLM_MOCK_PPROF_ENABLE",
			Default:     "false",
			Description: "Serve pprof metrics on the address defined by pprof-address.",
			Value:       serpent.BoolOf(&pprofEnable),
		},
		{
			Flag:        "pprof-address",
			Env:         "CODER_SCALETEST_LLM_MOCK_PPROF_ADDRESS",
			Default:     "127.0.0.1:6060",
			Description: "The bind address to serve pprof.",
			Value:       serpent.StringOf(&pprofAddress),
		},
		{
			Flag:        "trace-enable",
			Env:         "CODER_SCALETEST_LLM_MOCK_TRACE_ENABLE",
			Default:     "false",
			Description: "Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.",
			Value:       serpent.BoolOf(&traceEnable),
		},
	}

	return cmd
}
