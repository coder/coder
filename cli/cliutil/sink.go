package cliutil

import (
	"io"
	"sync"
)

type discardAfterClose struct {
	sync.Mutex
	wc     io.WriteCloser
	closed bool
}

// DiscardAfterClose is an io.WriteCloser that discards writes after it is closed without errors.
// It is useful as a target for a slog.Sink such that an underlying WriteCloser, like a file, can
// be cleaned up without race conditions from still-active loggers.
func DiscardAfterClose(wc io.WriteCloser) io.WriteCloser {
	return &discardAfterClose{wc: wc}
}

func (d *discardAfterClose) Write(p []byte) (n int, err error) {
	d.Lock()
	defer d.Unlock()
	if d.closed {
		return len(p), nil
	}
	return d.wc.Write(p)
}

func (d *discardAfterClose) Close() error {
	d.Lock()
	defer d.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	return d.wc.Close()
}
