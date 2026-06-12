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
		minStreamDuration   time.Duration
		maxStreamDuration   time.Duration
		responsePayloadSize int64
		toolCallsPerTurn    int64
		toolCallCommand     string

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

			if (minStreamDuration > 0) != (maxStreamDuration > 0) {
				return xerrors.New("--min-stream-duration and --max-stream-duration must be set together")
			}
			if minStreamDuration > maxStreamDuration {
				return xerrors.New("--min-stream-duration must not exceed --max-stream-duration")
			}

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
				MinStreamDuration:   minStreamDuration,
				MaxStreamDuration:   maxStreamDuration,
				ResponsePayloadSize: int(responsePayloadSize),
				ToolCallsPerTurn:    int(toolCallsPerTurn),
				ToolCallCommand:     toolCallCommand,
				TraceEnable:         traceEnable,
			}
			srv := new(llmmock.Server)

			if err := srv.Start(ctx, config); err != nil {
				return xerrors.Errorf("start mock LLM server: %w", err)
			}
			defer func() {
				if err := srv.Stop(); err != nil {
					logger.Error(ctx, "failed to stop mock LLM server", slog.Error(err))
				}
			}()

			_, _ = fmt.Fprintf(inv.Stdout, "Mock LLM API server started on %s\n", srv.APIAddress())
			_, _ = fmt.Fprintf(inv.Stdout, "  OpenAI endpoint: %s/v1/chat/completions\n", srv.APIAddress())
			_, _ = fmt.Fprintf(inv.Stdout, "  OpenAI responses endpoint: %s/v1/responses\n", srv.APIAddress())
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
			Flag:        "min-stream-duration",
			Env:         "CODER_SCALETEST_LLM_MOCK_MIN_STREAM_DURATION",
			Default:     "0s",
			Description: "Minimum duration to stream a text response over (e.g., 5s, 10s). Set with max-stream-duration to pace response chunks.",
			Value:       serpent.DurationOf(&minStreamDuration),
		},
		{
			Flag:        "max-stream-duration",
			Env:         "CODER_SCALETEST_LLM_MOCK_MAX_STREAM_DURATION",
			Default:     "0s",
			Description: "Maximum duration to stream a text response over (e.g., 10s, 30s). Set with min-stream-duration to pace response chunks.",
			Value:       serpent.DurationOf(&maxStreamDuration),
		},
		{
			Flag:        "response-payload-size",
			Env:         "CODER_SCALETEST_LLM_MOCK_RESPONSE_PAYLOAD_SIZE",
			Default:     "0",
			Description: "Size in bytes of the response payload. If 0, uses default context-aware responses.",
			Value:       serpent.Int64Of(&responsePayloadSize),
		},
		{
			Flag:        "tool-calls-per-turn",
			Env:         "CODER_SCALETEST_LLM_MOCK_TOOL_CALLS_PER_TURN",
			Default:     "0",
			Description: "Number of execute tool calls to emit per user turn. Set to 0 for text-only responses. OpenAI Chat Completions only.",
			Value:       serpent.Int64Of(&toolCallsPerTurn),
		},
		{
			Flag:        "tool-call-command",
			Env:         "CODER_SCALETEST_LLM_MOCK_TOOL_CALL_COMMAND",
			Default:     "echo scaletest",
			Description: "Shell command sent in each mock execute tool call when tool calls are enabled. OpenAI Chat Completions only.",
			Value:       serpent.StringOf(&toolCallCommand),
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
