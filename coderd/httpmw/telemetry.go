package httpmw

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"tailscale.com/tstime/rate"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/codersdk"
)

func ReportCLITelemetry(log slog.Logger, rep telemetry.Reporter) func(http.Handler) http.Handler {
	var mu sync.Mutex

	var (
		// We send telemetry at most once per minute.
		limiter = rate.NewLimiter(rate.Every(time.Minute), 1)
		queue   []telemetry.CLIInvocation
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			defer next.ServeHTTP(rw, r)
			payload := r.Header.Get(codersdk.CLITelemetryHeader)

			// We do simple checks and processing outside of the goroutine
			// to avoid the overhead of an additional goroutine on every
			// request.
			if payload == "" {
				return
			}

			byt, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				log.Error(
					r.Context(),
					"base64 decode CLI telemetry header",
					slog.F("error", err),
				)
				return
			}

			var inv telemetry.CLIInvocation
			err = json.Unmarshal(byt, &inv)
			if err != nil {
				log.Error(
					r.Context(),
					"unmarshal CLI telemetry header",
					slog.Error(err),
				)
				return
			}

			go func() {
				mu.Lock()
				defer mu.Unlock()

				queue = append(queue, inv)
				if !limiter.Allow() && len(queue) < 1024 {
					return
				}
				rep.Report(&telemetry.Snapshot{
					CLIInvocations: queue,
				})
				log.Debug(
					r.Context(),
					"reported CLI telemetry", slog.F("count", len(queue)),
				)
				queue = queue[:0]
			}()
		})
	}
}
