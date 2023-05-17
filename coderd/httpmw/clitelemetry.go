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
	var (
		mu sync.Mutex

		// We send telemetry at most once per minute.
		limiter = rate.NewLimiter(rate.Every(time.Minute), 1)
		queue   []telemetry.CLIInvocation
	)

	log = log.Named("cli-telemetry")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// No matter what, we proceed with the request.
			defer next.ServeHTTP(rw, r)

			payload := r.Header.Get(codersdk.CLITelemetryHeader)
			if payload == "" {
				return
			}

			byt, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				log.Error(
					r.Context(),
					"base64 decode",
					slog.F("error", err),
				)
				return
			}

			var inv telemetry.CLIInvocation
			err = json.Unmarshal(byt, &inv)
			if err != nil {
				log.Error(
					r.Context(),
					"unmarshal header",
					slog.Error(err),
				)
				return
			}

			// We do expensive work in a goroutine so we don't block the
			// request.
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
					"report sent", slog.F("count", len(queue)),
				)
				queue = queue[:0]
			}()
		})
	}
}
