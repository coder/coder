package agentworkload

import (
	"encoding/binary"
	"io"
	"math"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

type retainedOutput struct {
	Events     []codersdk.WorkloadEvent `json:"events"`
	Truncated  bool                     `json:"truncated"`
	BaseOffset int64                    `json:"base_offset,omitempty"`
}

func outputName(id uuid.UUID) string { return id.String() + ".output.json" }

func (o retainedOutput) valid() bool {
	if len(o.Events) > maxOutputEvents || o.BaseOffset < 0 || o.BaseOffset > math.MaxInt64-int64(len(o.Events)) {
		return false
	}
	size := 0
	for _, event := range o.Events {
		if (event.Stream != "stdout" && event.Stream != "stderr") || len(event.Data) > 16<<10 {
			return false
		}
		size += len(event.Data)
	}
	return size <= maxOutputBytes
}

// notifyLocked wakes observers without retaining output for individual readers.
// Both output reads and subscriptions use s.mu to avoid losing a wakeup.
func (*Supervisor) notifyLocked(run *record) {
	if run.Workload.Terminal() && run.inputCancel != nil {
		run.inputCancel()
	}
	if run.changed != nil {
		close(run.changed)
	}
	run.changed = make(chan struct{})
}

func (s *Supervisor) truncate(run *record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !run.output.Truncated {
		run.output.Truncated = true
		s.notifyLocked(run)
	}
}

// Decode Docker's ordered multiplexed stream without allocating a frame-sized
// buffer. Large or malicious frame sizes cannot cause unbounded allocations.
func (s *Supervisor) collect(run *record, reader io.Reader) error {
	if run.Request.TTY {
		return s.collectTTY(run, reader)
	}
	var header [8]byte
	var buffer [16 << 10]byte
	for {
		_, err := io.ReadFull(reader, header[:])
		if err == io.EOF {
			return nil
		}
		if err != nil {
			s.truncate(run)
			return err
		}
		stream := "stdout"
		if header[0] == 2 {
			stream = "stderr"
		} else if header[0] != 1 {
			s.truncate(run)
			return xerrors.New("invalid Docker output stream")
		}
		remaining := int64(binary.BigEndian.Uint32(header[4:]))
		for remaining > 0 {
			n, err := io.ReadFull(reader, buffer[:min(remaining, int64(len(buffer)))])
			if n > 0 {
				s.appendOutput(run, stream, buffer[:n])
			}
			if err != nil {
				s.truncate(run)
				return err
			}
			remaining -= int64(n)
		}
	}
}

// A TTY merges stderr into stdout and has no Docker multiplex headers. Read,
// rather than ReadFull, forwards short prompts without waiting for more output.
func (s *Supervisor) collectTTY(run *record, reader io.Reader) error {
	var buffer [16 << 10]byte
	for {
		n, err := reader.Read(buffer[:])
		if n > 0 {
			s.appendOutput(run, "stdout", buffer[:n])
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			s.truncate(run)
			return err
		}
	}
}

func (s *Supervisor) appendOutput(run *record, stream string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run.Request.Stdin {
		// Eviction only truncates readers that actually fall behind. A live
		// reader at the tail keeps receiving output beyond the archive cap.
		for len(run.output.Events) > 0 && (len(run.output.Events) >= maxOutputEvents || run.outputBytes+len(data) > maxOutputBytes) {
			run.outputBytes -= len(run.output.Events[0].Data)
			run.output.Events[0] = codersdk.WorkloadEvent{}
			run.output.Events = run.output.Events[1:]
			run.output.BaseOffset++
		}
	}
	if len(run.output.Events) >= maxOutputEvents || run.outputBytes >= maxOutputBytes {
		if !run.output.Truncated {
			run.output.Truncated = true
			s.notifyLocked(run)
		}
		return
	}
	n := min(len(data), maxOutputBytes-run.outputBytes)
	if n < len(data) {
		run.output.Truncated = true
	}
	run.output.Events = append(run.output.Events, codersdk.WorkloadEvent{Stream: stream, Data: append([]byte(nil), data[:n]...)})
	run.outputBytes += n
	s.notifyLocked(run)
}
