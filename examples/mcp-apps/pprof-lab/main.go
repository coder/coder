// Command pprof-lab serves net/http/pprof alongside a control panel that
// starts and stops pathological workloads, so the resulting profiles can be
// explored with pprof tooling.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/coder/coder/examples/mcp-apps/pprof-lab/lab"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:6060", "listen address")
	flag.Parse()

	if err := run(*addr); err != nil {
		log.Fatalf("pprof-lab: %v", err)
	}
}

// run serves the lab on addr until SIGINT or SIGTERM, then drains the HTTP
// server and stops every scenario.
func run(addr string) error {
	// Mutex and block profiles are empty unless sampling is enabled. 1 in 5
	// mutex contention events and every block event of 100us or longer are
	// recorded.
	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(1e5)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reg := lab.NewRegistry()
	defer reg.StopAll()

	srv := &http.Server{
		Addr:              addr,
		Handler:           NewHandler(reg, time.Now()),
		ReadHeaderTimeout: 10 * time.Second,
		// net/http/pprof extends the write deadline by the requested
		// `seconds` on /debug/pprof/profile and /trace, and rejects requests
		// whose `seconds` exceeds WriteTimeout. This caps a single CPU or
		// trace capture at 60s.
		WriteTimeout: 60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on http://%s/", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Printf("shutting down: %v", context.Cause(ctx))
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
