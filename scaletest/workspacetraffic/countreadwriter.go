package workspacetraffic

import (
	"context"
	"errors"
	"io"
	"time"

	"golang.org/x/xerrors"

	"nhooyr.io/websocket"
)

// countReadWriteCloser wraps an io.ReadWriteCloser and counts the number of bytes read and written.
type countReadWriteCloser struct {
	rwc          io.ReadWriteCloser
	readMetrics  ConnMetrics
	writeMetrics ConnMetrics
}

func (w *countReadWriteCloser) Close() error {
	return w.rwc.Close()
}

func (w *countReadWriteCloser) Read(p []byte) (int, error) {
	start := time.Now()
	n, err := w.rwc.Read(p)
	took := time.Since(start).Seconds()
	if reportableErr(err) {
		w.readMetrics.AddError(1)
	}
	w.readMetrics.ObserveLatency(took)
	if n > 0 {
		w.readMetrics.AddTotal(float64(n))
	}
	return n, err
}

func (w *countReadWriteCloser) Write(p []byte) (int, error) {
	start := time.Now()
	n, err := w.rwc.Write(p)
	took := time.Since(start).Seconds()
	if reportableErr(err) {
		w.writeMetrics.AddError(1)
	}
	w.writeMetrics.ObserveLatency(took)
	if n > 0 {
		w.writeMetrics.AddTotal(float64(n))
	}
	return n, err
}

// some errors we want to report in metrics; others we want to ignore
// such as websocket.StatusNormalClosure or context.Canceled
func reportableErr(err error) bool {
	if err == nil {
		return false
	}
	if xerrors.Is(err, io.EOF) {
		return false
	}
	if xerrors.Is(err, context.Canceled) {
		return false
	}
	var wsErr websocket.CloseError
	if errors.As(err, &wsErr) {
		return wsErr.Code != websocket.StatusNormalClosure
	}
	return false
}
