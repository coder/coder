package apidump

import (
	"io"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/xerrors"
)

// streamingBodyDumper wraps an io.ReadCloser and writes all data to a dump file
// as it's read, preserving streaming behavior.
type streamingBodyDumper struct {
	body       io.ReadCloser
	dumpPath   string
	headerData []byte
	logger     func(err error)

	once    sync.Once
	file    *os.File
	initErr error
}

func (s *streamingBodyDumper) init() {
	s.once.Do(func() {
		if err := os.MkdirAll(filepath.Dir(s.dumpPath), 0o755); err != nil {
			s.initErr = xerrors.Errorf("create dump dir: %w", err)
			return
		}
		f, err := os.Create(s.dumpPath)
		if err != nil {
			s.initErr = xerrors.Errorf("create dump file: %w", err)
			return
		}
		s.file = f
		// Write headers first.
		if _, err := s.file.Write(s.headerData); err != nil {
			s.initErr = xerrors.Errorf("write headers: %w", err)
			_ = s.file.Close() // best-effort cleanup on header write failure
			s.file = nil
		}
	})
}

func (s *streamingBodyDumper) Read(p []byte) (int, error) {
	n, err := s.body.Read(p)
	if n > 0 {
		s.init()
		if s.initErr != nil && s.logger != nil {
			s.logger(s.initErr)
		}
		if s.file != nil {
			// Write raw bytes as they stream through.
			_, _ = s.file.Write(p[:n])
		}
	}
	return n, err
}

func (s *streamingBodyDumper) Close() error {
	// Ensure init() has completed to avoid racing with Read().
	s.init()
	var closeErr error
	if s.file != nil {
		closeErr = s.file.Close()
	}
	bodyErr := s.body.Close()
	if bodyErr != nil {
		return bodyErr
	}
	return closeErr
}
