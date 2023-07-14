package workspacetraffic

import (
	"context"
	"errors"
	"golang.org/x/xerrors"
	"io"
	"nhooyr.io/websocket"
	"time"
)

// countReadWriteCloser wraps an io.ReadWriteCloser and counts the number of bytes read and written.
type countReadWriteCloser struct {
	ctx context.Context
	rwc io.ReadWriteCloser
	//metrics *Metrics
	readMetrics  ConnMetrics
	writeMetrics ConnMetrics
	//labels       []string
}

func (w *countReadWriteCloser) Close() error {
	return w.rwc.Close()
}

func (w *countReadWriteCloser) Read(p []byte) (int, error) {
	start := time.Now()
	n, err := w.rwc.Read(p)
	if reportableErr(err) {
		//w.metrics.ReadErrorsTotal.WithLabelValues(w.labels...).Inc()
		w.readMetrics.AddError(1)
	}
	//w.metrics.ReadLatencySeconds.WithLabelValues(w.labels...).Observe(time.Since(start).Seconds())
	w.readMetrics.ObserveLatency(time.Since(start).Seconds())
	if n > 0 {
		//w.metrics.BytesReadTotal.WithLabelValues(w.labels...).Add(float64(n))
		w.readMetrics.AddTotal(float64(n))
	}
	return n, err
}

func (w *countReadWriteCloser) Write(p []byte) (int, error) {
	start := time.Now()
	n, err := w.rwc.Write(p)
	if reportableErr(err) {
		//w.metrics.WriteErrorsTotal.WithLabelValues(w.labels...).Inc()
		w.writeMetrics.AddError(1)
	}
	//w.metrics.WriteLatencySeconds.WithLabelValues(w.labels...).Observe(time.Since(start).Seconds())
	w.writeMetrics.ObserveLatency(time.Since(start).Seconds())
	if n > 0 {
		//w.metrics.BytesWrittenTotal.WithLabelValues(w.labels...).Add(float64(n))
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
	if xerrors.Is(err, context.Canceled) {
		return false
	}
	var wsErr websocket.CloseError
	if errors.As(err, &wsErr) {
		return wsErr.Code != websocket.StatusNormalClosure
	}
	return false
}
