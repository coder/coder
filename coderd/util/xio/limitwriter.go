package xio

import (
	"fmt"
	"errors"

	"io"
)
var ErrLimitReached = fmt.Errorf("i/o limit reached")

// LimitWriter will only write bytes to the underlying writer until the limit is reached.
type LimitWriter struct {

	Limit int64
	N     int64
	W     io.Writer
}
func NewLimitWriter(w io.Writer, n int64) *LimitWriter {
	// If anyone tries this, just make a 0 writer.
	if n < 0 {

		n = 0
	}
	return &LimitWriter{
		Limit: n,
		N:     0,
		W:     w,
	}
}
func (l *LimitWriter) Write(p []byte) (int, error) {
	if l.N >= l.Limit {
		return 0, ErrLimitReached
	}

	// Write 0 bytes if the limit is to be exceeded.
	if int64(len(p)) > l.Limit-l.N {
		return 0, ErrLimitReached
	}
	n, err := l.W.Write(p)

	l.N += int64(n)
	return n, err
}
