package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

func (r *RootCmd) stdioHTTPCommand() *serpent.Command {
	var (
		port    string
		host    string
		timeout time.Duration
	)

	cmd := &serpent.Command{
		Use:   "stdio-http <command> [args...]",
		Short: "Run command and expose stdin/stdout/stderr over HTTP",
		Long: `Start an HTTP server that runs a command and exposes its stdio streams:
- POST requests to / send data to the command's stdin
- GET requests to / receive Server-Sent Events with stdout and stderr output`,
		Handler: func(inv *serpent.Invocation) error {
			if len(inv.Args) == 0 {
				return xerrors.Errorf("command is required")
			}

			cmdName := inv.Args[0]
			cmdArgs := inv.Args[1:]

			return handleStdioHTTP(inv, cmdName, cmdArgs, host, port, timeout)
		},
		Options: []serpent.Option{
			{
				Name:          "port",
				Description:   "Port to listen on.",
				Flag:          "port",
				FlagShorthand: "p",
				Default:       "8080",
				Value:         serpent.StringOf(&port),
			},
			{
				Name:        "host",
				Description: "Host to listen on.",
				Flag:        "host",
				Default:     "localhost",
				Value:       serpent.StringOf(&host),
			},
			{
				Name:        "timeout",
				Description: "Command timeout (0 means no timeout).",
				Flag:        "timeout",
				Default:     "0",
				Value:       serpent.DurationOf(&timeout),
			},
		},
	}

	return cmd
}

type stdioHTTPServer struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	// Channels to distribute stdout/stderr to multiple SSE connections
	stdoutCh chan []byte
	stderrCh chan []byte

	mu sync.RWMutex
	// Track active SSE connections
	stdoutSubscribers map[chan []byte]bool
	stderrSubscribers map[chan []byte]bool

	ctx    context.Context
	cancel context.CancelFunc
}

func handleStdioHTTP(inv *serpent.Invocation, cmdName string, cmdArgs []string, host, port string, timeout time.Duration) error {
	ctx, cancel := context.WithCancel(inv.Context())
	defer cancel()

	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	server := &stdioHTTPServer{
		stdoutCh:          make(chan []byte, 100),
		stderrCh:          make(chan []byte, 100),
		stdoutSubscribers: make(map[chan []byte]bool),
		stderrSubscribers: make(map[chan []byte]bool),
		ctx:               ctx,
		cancel:            cancel,
	}

	// Start the command
	cmdCtx, cmdCancel := context.WithCancel(ctx)
	defer cmdCancel()
	if err := server.startCommand(cmdName, cmdArgs); err != nil {
		return xerrors.Errorf("failed to start command: %w", err)
	}
	defer server.cleanup()

	// Start reading stdout/stderr
	go server.readStdout()
	go server.readStderr()
	go server.distributeStdout()
	go server.distributeStderr()

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/", server.handleRoot)

	addr := net.JoinHostPort(host, port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	cliui.Infof(inv.Stderr, "Starting HTTP server on http://%s", addr)
	cliui.Infof(inv.Stderr, "Command: %s %s", cmdName, strings.Join(cmdArgs, " "))
	cliui.Infof(inv.Stderr, "Endpoints:")
	cliui.Infof(inv.Stderr, "  POST / - Send data to command stdin")
	cliui.Infof(inv.Stderr, "  GET / - Stream command output (SSE with stdout/stderr events)")

	// Start HTTP server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	// Wait for context cancellation, command completion, or HTTP server error
	select {
	case <-ctx.Done():
		cliui.Infof(inv.Stderr, "Shutting down due to context cancellation")
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return xerrors.Errorf("HTTP server error: %w", err)
		}
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		cliui.Warnf(inv.Stderr, "HTTP server shutdown error: %v", err)
	}

	// Wait for command to finish
	if server.cmd.Process != nil {
		if err := server.cmd.(); err != nil {
			return xerrors.Errorf("kill command error: %w", err)
		}
	}

	return nil
}

func (s *stdioHTTPServer) startCommand(ctx context.Context, cmdName string, cmdArgs []string) error {
	s.cmd = exec.CommandContext(ctx, cmdName, cmdArgs...)

	var err error
	s.stdin, err = s.cmd.StdinPipe()
	if err != nil {
		return xerrors.Errorf("failed to create stdin pipe: %w", err)
	}

	s.stdout, err = s.cmd.StdoutPipe()
	if err != nil {
		return xerrors.Errorf("failed to create stdout pipe: %w", err)
	}

	s.stderr, err = s.cmd.StderrPipe()
	if err != nil {
		return xerrors.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := s.cmd.Start(); err != nil {
		return xerrors.Errorf("failed to start command: %w", err)
	}

	return nil
}

func (s *stdioHTTPServer) cleanup() {
	s.cancel()

	if s.stdin != nil {
		s.stdin.Close()
	}
	if s.stdout != nil {
		s.stdout.Close()
	}
	if s.stderr != nil {
		s.stderr.Close()
	}

	close(s.stdoutCh)
	close(s.stderrCh)
}

func (s *stdioHTTPServer) readStdout() {
	scanner := bufio.NewScanner(s.stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		select {
		case s.stdoutCh <- line:
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *stdioHTTPServer) readStderr() {
	scanner := bufio.NewScanner(s.stderr)
	for scanner.Scan() {
		line := scanner.Bytes()
		select {
		case s.stderrCh <- line:
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *stdioHTTPServer) distributeStdout() {
	for {
		select {
		case data, ok := <-s.stdoutCh:
			if !ok {
				return
			}
			s.mu.RLock()
			for ch := range s.stdoutSubscribers {
				select {
				case ch <- data:
				default:
					// Subscriber can't keep up, skip
				}
			}
			s.mu.RUnlock()
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *stdioHTTPServer) distributeStderr() {
	for {
		select {
		case data, ok := <-s.stderrCh:
			if !ok {
				return
			}
			s.mu.RLock()
			for ch := range s.stderrSubscribers {
				select {
				case ch <- data:
				default:
					// Subscriber can't keep up, skip
				}
			}
			s.mu.RUnlock()
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *stdioHTTPServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		// Read stdin data first
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		if s.stdin == nil {
			http.Error(w, "Command stdin not available", http.StatusServiceUnavailable)
			return
		}

		// Write to stdin
		_, err = s.stdin.Write(body)
		if err != nil {
			http.Error(w, "Failed to write to command stdin", http.StatusInternalServerError)
			return
		}

		// Start streaming SSE
		s.handleStream(w, r)
	case http.MethodGet:
		s.handleStream(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *stdioHTTPServer) handleStream(w http.ResponseWriter, r *http.Request) {
	s.setupSSE(w)

	stdoutCh := make(chan []byte, 10)
	stderrCh := make(chan []byte, 10)

	s.mu.Lock()
	s.stdoutSubscribers[stdoutCh] = true
	s.stderrSubscribers[stderrCh] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.stdoutSubscribers, stdoutCh)
		delete(s.stderrSubscribers, stderrCh)
		s.mu.Unlock()
		close(stdoutCh)
		close(stderrCh)
	}()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	for {
		select {
		case data := <-stdoutCh:
			fmt.Fprintf(w, "event: stdout\ndata: %s\n\n", string(data))
			flusher.Flush()
		case data := <-stderrCh:
			fmt.Fprintf(w, "event: stderr\ndata: %s\n\n", string(data))
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *stdioHTTPServer) setupSSE(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Headers", "Cache-Control")
}
