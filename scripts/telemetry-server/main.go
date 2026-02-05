// telemetry-server is a standalone HTTP server that receives telemetry
// snapshots and prints them as a JSON stream to stdout. This is useful for
// local development. Test with scripts/develop.sh by setting:
//
//	CODER_TELEMETRY_ENABLE=true CODER_TELEMETRY_URL=http://127.0.0.1:8081
//
// Usage:
//
//	go run ./scripts/telemetry-server [--port 8081]
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	port := flag.String("port", "8081", "Port to listen on")
	flag.Parse()

	enc := json.NewEncoder(os.Stdout)

	mux := http.NewServeMux()

	handleTelemetry := func(telemetryType string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			output := map[string]any{
				"type":    telemetryType,
				"version": r.Header.Get("X-Telemetry-Version"),
				"data":    json.RawMessage(body),
			}
			_ = enc.Encode(output)

			w.WriteHeader(http.StatusAccepted)
		}
	}

	mux.HandleFunc("POST /snapshot", handleTelemetry("snapshot"))
	mux.HandleFunc("POST /deployment", handleTelemetry("deployment"))

	addr := net.JoinHostPort("127.0.0.1", *port)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		_ = server.Close()
	}()

	_, _ = fmt.Fprintf(os.Stdout, "Mock telemetry server listening on %s\n", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
