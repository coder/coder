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
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/coder/coder/v2/coderd/telemetry"
)

func main() {
	port := flag.String("port", "8081", "Port to listen on")
	flag.Parse()

	enc := json.NewEncoder(os.Stdout)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /snapshot", func(w http.ResponseWriter, r *http.Request) {
		var snapshot telemetry.Snapshot
		if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		output := map[string]any{
			"type":    "snapshot",
			"version": r.Header.Get(telemetry.VersionHeader),
			"data":    snapshot,
		}
		_ = enc.Encode(output)

		w.WriteHeader(http.StatusAccepted)
	})

	mux.HandleFunc("POST /deployment", func(w http.ResponseWriter, r *http.Request) {
		var deployment telemetry.Deployment
		if err := json.NewDecoder(r.Body).Decode(&deployment); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		output := map[string]any{
			"type":    "deployment",
			"version": r.Header.Get(telemetry.VersionHeader),
			"data":    deployment,
		}
		_ = enc.Encode(output)

		w.WriteHeader(http.StatusAccepted)
	})

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
