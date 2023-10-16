package httpmw

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"golang.org/x/exp/maps"
	"tailscale.com/tstime/rate"

	"cdr.dev/slog"
	clitelemetry "github.com/coder/coder/v2/cli/telemetry"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/codersdk"
)

func ReportCLITelemetry(log slog.Logger, rep telemetry.Reporter) func(http.Handler) http.Handler {
	var (
		mu sync.Mutex

		// We send telemetry at most once per minute.
		limiter = rate.NewLimiter(rate.Every(time.Minute), 1)

		// We map by timestamp to deduplicate invocations, since one invocation
		// will send multiple requests, each with a duplicate header. It's still
		// possible for duplicates to reach the telemetry service since requests
		// can get processed by different coderds, but our analysis tools
		// will deduplicate by timestamp as well.
		//
		// This approach just helps us reduce storage and ingest fees, and doesn't
		// change the correctness.
		queue = make(map[string]clitelemetry.Invocation)
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
					slog.Error(err),
				)
				return
			}

			var inv clitelemetry.Invocation
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

				queue[inv.InvokedAt.String()] = inv
				if !limiter.Allow() && len(queue) < 1024 {
					return
				}
				rep.Report(&telemetry.Snapshot{
					CLIInvocations: maps.Values(queue),
				})
				log.Debug(
					r.Context(),
					"report sent", slog.F("count", len(queue)),
				)
				maps.Clear(queue)
			}()
		})
	}
}
