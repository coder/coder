package agentworkload

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

// Optional interfaces leave output-only runtimes unchanged.
type inputRuntime interface {
	AttachInput(context.Context, string) (io.ReadCloser, error)
}

type resizeRuntime interface {
	Resize(context.Context, string, uint16, uint16) error
}

type inputAttachment interface {
	WriteContext(context.Context, []byte) (int, error)
	CloseWrite() error
}

func (s *Supervisor) claimInput(id uuid.UUID) (*record, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.failed {
		return nil, http.StatusServiceUnavailable, xerrors.New("workload supervisor is detached or unavailable")
	}
	run, exists := s.state.Runs[id]
	if !exists {
		return nil, http.StatusNotFound, xerrors.New("workload not found")
	}
	if !run.Request.Stdin {
		return nil, http.StatusConflict, xerrors.New("workload was not created with stdin")
	}
	if run.Workload.Terminal() || run.CancelRequested || run.inputClosed || run.inputCtx.Err() != nil {
		return nil, http.StatusConflict, xerrors.New("workload is no longer accepting input")
	}
	if run.inputController {
		return nil, http.StatusConflict, xerrors.New("workload already has an input controller")
	}
	run.inputController = true
	// Add under the same lock as Close so shutdown joins upgraded connections.
	s.wg.Add(1)
	return run, http.StatusOK, nil
}

func (s *Supervisor) waitInput(ctx context.Context, run *record) (inputAttachment, error) {
	for {
		s.mu.Lock()
		if ctx.Err() != nil || run.Workload.Terminal() || run.CancelRequested {
			s.mu.Unlock()
			return nil, xerrors.New("workload is no longer accepting input")
		}
		if run.input != nil {
			input := run.input
			s.mu.Unlock()
			return input, nil
		}
		if run.Workload.State == "running" {
			s.mu.Unlock()
			return nil, xerrors.New("workload runtime does not support input")
		}
		if run.changed == nil {
			run.changed = make(chan struct{})
		}
		changed := run.changed
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, xerrors.New("workload input attachment interrupted")
		case <-changed:
		}
	}
}

func (s *Supervisor) inputHTTP(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	run, status, err := s.claimInput(id)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	defer func() {
		s.mu.Lock()
		run.inputController = false
		s.mu.Unlock()
		s.wg.Done()
	}()
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(run.inputCtx, cancel)
	defer stop()
	defer cancel()
	input, err := s.waitInput(ctx, run)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(32 << 10) // A 16 KiB payload plus base64 and JSON overhead.
	stopClose := context.AfterFunc(ctx, func() { _ = conn.CloseNow() })
	defer stopClose()
	defer conn.CloseNow()

	// Keep reading while Docker writes, so a disconnect cancels a blocked write.
	// The unbuffered handoff never retains or replays a queue of keystrokes.
	messages := make(chan []byte)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		defer cancel()
		for {
			kind, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if kind != websocket.MessageText {
				_ = conn.Close(websocket.StatusUnsupportedData, "input must be JSON text")
				return
			}
			select {
			case messages <- data:
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() { cancel(); <-readDone }()
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-messages:
			var message codersdk.WorkloadInput
			if err := decodeJSON(bytes.NewReader(data), &message); err != nil || !validInput(message, run.Request.TTY) {
				_ = conn.Close(websocket.StatusPolicyViolation, "invalid workload input")
				return
			}
			if err := s.sendInput(ctx, run, input, message); err != nil {
				// Neither input bytes nor daemon errors belong in diagnostics.
				_ = conn.Close(websocket.StatusInternalError, "workload input failed; input was not replayed")
				return
			}
			if message.Type == "eof" {
				s.mu.Lock()
				run.inputClosed = true
				s.mu.Unlock()
				_ = conn.Close(websocket.StatusNormalClosure, "stdin closed")
				return
			}
		}
	}
}

func validInput(message codersdk.WorkloadInput, tty bool) bool {
	switch message.Type {
	case "data":
		return len(message.Data) > 0 && len(message.Data) <= 16<<10 && message.Width == 0 && message.Height == 0
	case "resize":
		return tty && len(message.Data) == 0 && message.Width > 0 && message.Height > 0
	case "eof":
		return len(message.Data) == 0 && message.Width == 0 && message.Height == 0
	default:
		return false
	}
}

func (s *Supervisor) sendInput(ctx context.Context, run *record, input inputAttachment, message codersdk.WorkloadInput) error {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	switch message.Type {
	case "resize":
		rt, ok := s.runtime.(resizeRuntime)
		if !ok {
			return xerrors.New("runtime does not support terminal resize")
		}
		return rt.Resize(ctx, containerName(run.Request.ID), message.Width, message.Height)
	case "eof":
		if !run.Request.TTY {
			return input.CloseWrite()
		}
		// Docker does not propagate a half-close to a TTY. Send its terminal
		// EOF character instead, without closing the shared output stream.
		message.Data = []byte{4}
	}
	n, err := input.WriteContext(ctx, message.Data)
	if err == nil && n != len(message.Data) {
		return io.ErrShortWrite
	}
	return err
}
