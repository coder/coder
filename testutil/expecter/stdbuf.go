package expecter

import (
	"bytes"
	"io"
	"sync"

	"golang.org/x/xerrors"
)

// stdbuf is like a buffered stdout, it buffers writes until read.
type stdbuf struct {
	r io.Reader

	mu   sync.Mutex // Protects following.
	b    []byte
	more chan struct{}
	err  error
}

func newStdbuf() *stdbuf {
	return &stdbuf{more: make(chan struct{}, 1)}
}

func (b *stdbuf) ReadAll() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.err != nil {
		return nil
	}
	p := append([]byte(nil), b.b...)
	b.b = b.b[len(b.b):]
	return p
}

func (b *stdbuf) Read(p []byte) (int, error) {
	if b.r == nil {
		return b.readOrWaitForMore(p)
	}

	n, err := b.r.Read(p)
	if xerrors.Is(err, io.EOF) {
		b.r = nil
		err = nil
		if n == 0 {
			return b.readOrWaitForMore(p)
		}
	}
	return n, err
}

func (b *stdbuf) readOrWaitForMore(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Deplete channel so that more check
	// is for future input into buffer.
	select {
	case <-b.more:
	default:
	}

	if len(b.b) == 0 {
		if b.err != nil {
			return 0, b.err
		}

		b.mu.Unlock()
		<-b.more
		b.mu.Lock()
	}

	b.r = bytes.NewReader(b.b)
	b.b = b.b[len(b.b):]

	return b.r.Read(p)
}

func (b *stdbuf) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.err != nil {
		return 0, b.err
	}

	b.b = append(b.b, p...)

	select {
	case b.more <- struct{}{}:
	default:
	}

	return len(p), nil
}

func (b *stdbuf) Close() error {
	return b.closeErr(nil)
}

func (b *stdbuf) closeErr(err error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return err
	}
	if err == nil {
		b.err = io.EOF
	} else {
		b.err = err
	}
	close(b.more)
	return err
}
