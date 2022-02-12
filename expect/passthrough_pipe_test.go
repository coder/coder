package expect_test

import (
	"errors"
	"io"

	//"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	. "github.com/coder/coder/expect"
)

func TestPassthroughPipe(t *testing.T) {
	t.Parallel()

	pipeReader, pipeWriter := io.Pipe()

	passthroughPipe, err := NewPassthroughPipe(pipeReader)
	require.NoError(t, err)

	err = passthroughPipe.SetReadDeadline(time.Now().Add(time.Hour))
	require.NoError(t, err)

	pipeError := xerrors.New("pipe error")
	err = pipeWriter.CloseWithError(pipeError)
	require.NoError(t, err)

	p := make([]byte, 1)
	_, err = passthroughPipe.Read(p)
	require.Equal(t, err, pipeError)
}
