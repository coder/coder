package clilog_test

import (
	"bytes"
	"io"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clilog"
)

func TestDiscardOnPipeError(t *testing.T) {
	t.Parallel()

	const payload = "log entry"

	t.Run("DiscardsClosedPipe", func(t *testing.T) {
		t.Parallel()

		for _, target := range []error{
			io.ErrClosedPipe,
			syscall.EPIPE,
			xerrors.Errorf("wrapped: %w", io.ErrClosedPipe),
		} {
			fw := &fakeWriter{err: target}
			n, err := clilog.DiscardOnPipeError(fw).Write([]byte(payload))
			require.NoError(t, err, "%v should be discarded", target)
			assert.Equal(t, len(payload), n)
		}
	})

	t.Run("ReportsOtherErrors", func(t *testing.T) {
		t.Parallel()

		// os.ErrClosed stays reported: a write to a writer we closed ourselves
		// is worth surfacing.
		for _, target := range []error{os.ErrClosed, io.ErrShortWrite, xerrors.New("boom")} {
			fw := &fakeWriter{err: target}
			_, err := clilog.DiscardOnPipeError(fw).Write([]byte(payload))
			require.ErrorIs(t, err, target)
		}
	})

	t.Run("PassesThroughSuccess", func(t *testing.T) {
		t.Parallel()

		fw := &fakeWriter{}
		n, err := clilog.DiscardOnPipeError(fw).Write([]byte(payload))
		require.NoError(t, err)
		assert.Equal(t, len(payload), n)
		assert.Equal(t, payload, fw.buf.String())
	})
}

type fakeWriter struct {
	buf bytes.Buffer
	err error
}

func (f *fakeWriter) Write(p []byte) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.buf.Write(p)
}
