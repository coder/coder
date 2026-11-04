package agentworkload

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

const outputHeartbeatInterval = 15 * time.Second

func (s *Supervisor) observeOutput(id uuid.UUID, offset int64) (codersdk.WorkloadOutput, <-chan struct{}, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return codersdk.WorkloadOutput{}, nil, http.StatusServiceUnavailable, xerrors.New("workload supervisor is detached")
	}
	output, status, err := s.outputLocked(id, offset)
	if err != nil {
		return output, nil, status, err
	}
	run := s.state.Runs[id]
	if run.changed == nil {
		run.changed = make(chan struct{})
	}
	return output, run.changed, status, nil
}

func (s *Supervisor) streamOutput(w http.ResponseWriter, r *http.Request, id uuid.UUID, offset int64) {
	output, changed, status, err := s.observeOutput(id, offset)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	encoder := json.NewEncoder(w)
	controller := http.NewResponseController(w)
	heartbeat := s.clock.NewTicker(outputHeartbeatInterval, "workload-output-heartbeat")
	defer heartbeat.Stop()

	var lastChange <-chan struct{}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		default:
		}
		// Reading the empty tail of a running batch must not emit a duplicate
		// snapshot. A terminal empty batch explicitly marks the end of output.
		if changed != lastChange || len(output.Events) > 0 || output.Workload.Terminal() {
			// Replace the server's whole-response timeout with a bounded write.
			if err := controller.SetWriteDeadline(time.Now().Add(operationTimeout)); err != nil && !xerrors.Is(err, http.ErrNotSupported) {
				return
			}
			if err := encoder.Encode(output); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
			lastChange = changed
		}
		if output.Workload.Terminal() && len(output.Events) == 0 {
			return
		}
		offset = output.NextOffset
		// Drain bounded batches immediately. Only an empty tail waits for a
		// notification; heartbeats never read or resend the workload snapshot.
		if len(output.Events) == 0 {
			waiting := true
			for waiting {
				select {
				case <-r.Context().Done():
					return
				case <-s.ctx.Done():
					return
				case <-changed:
					waiting = false
				case <-heartbeat.C:
					if err := controller.SetWriteDeadline(time.Now().Add(operationTimeout)); err != nil && !xerrors.Is(err, http.ErrNotSupported) {
						return
					}
					if _, err := io.WriteString(w, "\n"); err != nil {
						return
					}
					if err := controller.Flush(); err != nil {
						return
					}
				}
			}
		}
		output, changed, _, err = s.observeOutput(id, offset)
		if err != nil {
			// The response is committed. Closing lets the caller reconnect from
			// its last offset without mixing an HTTP error into the stream.
			return
		}
	}
}
