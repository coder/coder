// aigateway-mock records selected request headers and serves AI provider fixtures.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/http/httpguts"
	"golang.org/x/tools/txtar"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/fixtures"
)

const maxRequestBytes = 1 << 20

func main() {
	listen := flag.String("listen", "127.0.0.1:18081", "HTTP listen address")
	var headers []string
	flag.Func("header", "Additional header to record (repeatable; values may be sensitive)", func(name string) error {
		if !httpguts.ValidHeaderFieldName(name) {
			return xerrors.Errorf("invalid header name %q", name)
		}
		headers = append(headers, name)
		return nil
	})
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, *listen, headers, os.Stdout, os.Stderr)
	stop()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "aigateway-mock: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, address string, headers []string, output, diagnostics io.Writer) error {
	handler, err := newHandler(output, headers)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return xerrors.Errorf("listen: %w", err)
	}
	defer listener.Close()
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	stop := context.AfterFunc(ctx, func() { _ = server.Close() })
	defer stop()
	_, _ = fmt.Fprintf(diagnostics, "AI Gateway mock upstream listening on http://%s\n", listener.Addr())
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return xerrors.Errorf("serve: %w", err)
	}
	return nil
}

type requestRecord struct {
	Path    string      `json:"path"`
	Stream  bool        `json:"stream"`
	Headers http.Header `json:"headers"`
}

func newHandler(output io.Writer, extraHeaders []string) (http.Handler, error) {
	extra := make(map[string]bool, len(extraHeaders))
	for _, name := range extraHeaders {
		extra[strings.ToLower(name)] = true
	}
	var outputMu sync.Mutex
	encoder := json.NewEncoder(output)
	mux := http.NewServeMux()
	for _, route := range []struct {
		path      string
		blocking  []byte
		streaming []byte
	}{
		{"/chat/completions", fixtures.OaiChatSimple, fixtures.OaiChatSimple},
		{"/responses", fixtures.OaiResponsesBlockingSimple, fixtures.OaiResponsesStreamingSimple},
		{"/messages", fixtures.AntSimple, fixtures.AntSimple},
	} {
		blocking, err := fixtureSection(route.blocking, "non-streaming")
		if err != nil {
			return nil, xerrors.Errorf("load %s: %w", route.path, err)
		}
		streaming, err := fixtureSection(route.streaming, "streaming")
		if err != nil {
			return nil, xerrors.Errorf("load %s: %w", route.path, err)
		}
		handle := func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
			if err != nil {
				status := http.StatusBadRequest
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					status = http.StatusRequestEntityTooLarge
				}
				http.Error(w, "cannot read request body", status)
				return
			}
			var request *struct {
				Stream bool `json:"stream"`
			}
			if err := json.Unmarshal(body, &request); err != nil || request == nil {
				http.Error(w, "expected a JSON object with an optional boolean stream field", http.StatusBadRequest)
				return
			}
			record := requestRecord{Path: r.URL.Path, Stream: request.Stream, Headers: make(http.Header)}
			for name, values := range r.Header {
				lower := strings.ToLower(name)
				if strings.HasPrefix(lower, "x-ai-bridge-actor-") || strings.HasPrefix(lower, "x-smoke-") || extra[lower] {
					record.Headers[name] = values
				}
			}
			outputMu.Lock()
			err = encoder.Encode(record)
			outputMu.Unlock()
			if err != nil {
				http.Error(w, "cannot record request", http.StatusInternalServerError)
				return
			}
			response := blocking
			w.Header().Set("Content-Type", "application/json")
			if request.Stream {
				response = streaming
				w.Header().Set("Content-Type", "text/event-stream")
			}
			// Replay the complete SSE fixture without simulating token timing.
			_, _ = w.Write(response)
		}
		mux.HandleFunc("POST "+route.path, handle)
		mux.HandleFunc("POST /v1"+route.path, handle)
	}
	return mux, nil
}

func fixtureSection(data []byte, name string) ([]byte, error) {
	for _, file := range txtar.Parse(data).Files {
		if file.Name == name {
			return file.Data, nil
		}
	}
	return nil, xerrors.Errorf("fixture missing %q section", name)
}
