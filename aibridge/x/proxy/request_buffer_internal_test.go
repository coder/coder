package proxy

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	codertestutil "github.com/coder/coder/v2/testutil"
)

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

type trackedBody struct {
	io.Reader
	closes, eofs int
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		b.eofs++
	}
	return n, err
}
func (b *trackedBody) Close() error { b.closes++; return nil }

func TestRequestBufferReaders(t *testing.T) {
	t.Parallel()
	for _, terminal := range []error{io.EOF, io.ErrClosedPipe} {
		t.Run(terminal.Error(), func(t *testing.T) {
			t.Parallel()
			ctx := codertestutil.Context(t, codertestutil.WaitLong)
			reading, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			calls := 0
			b := &requestBuffer{source: readerFunc(func(p []byte) (int, error) {
				calls++
				if calls == 1 {
					return copy(p, "prefix"), nil
				}
				close(reading)
				<-release
				return copy(p, "suffix"), terminal
			})}
			first, err := b.getBody()
			require.NoError(t, err)
			defer first.Close()
			prefix := make([]byte, len("prefix"))
			_, err = io.ReadFull(first, prefix)
			require.NoError(t, err)
			assertComplete := func(t assert.TestingT, payload []byte, err error) {
				assert.Equal(t, "prefixsuffix", string(payload))
				if errors.Is(terminal, io.EOF) {
					assert.NoError(t, err)
				} else {
					assert.ErrorIs(t, err, terminal)
				}
			}
			blocked, err := b.getBody()
			require.NoError(t, err)
			defer blocked.Close()
			done := make(chan struct{}, 2)
			go func() {
				payload, err := io.ReadAll(blocked)
				assertComplete(t, payload, err)
				done <- struct{}{}
			}()
			codertestutil.TryReceive(ctx, t, reading)
			second, err := b.getBody()
			require.NoError(t, err)
			defer second.Close()
			go func() {
				_, err := io.ReadFull(second, prefix)
				assert.NoError(t, err)
				assert.Equal(t, "prefix", string(prefix))
				done <- struct{}{}
			}()
			// A cached prefix must remain readable while another reader is
			// blocked waiting for input at the buffered end.
			codertestutil.RequireReceive(ctx, t, done)
			release <- struct{}{}
			payload, err := io.ReadAll(second)
			require.Equal(t, "suffix", string(payload))
			if errors.Is(terminal, io.EOF) {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, terminal)
			}
			codertestutil.RequireReceive(ctx, t, done)
			replay, err := b.getBody()
			require.NoError(t, err)
			defer replay.Close()
			payload, err = io.ReadAll(replay)
			assertComplete(t, payload, err)
			require.Equal(t, 2, calls, "input bytes must be read from the source only once")
		})
	}
}

func TestRequestBufferBodyOwnership(t *testing.T) {
	t.Parallel()
	source := &trackedBody{Reader: strings.NewReader("request")}
	buffer := &requestBuffer{source: source}
	first, err := buffer.getBody()
	require.NoError(t, err)
	require.NoError(t, first.Close())
	require.Zero(t, source.closes, "attempt readers must not close the server-owned input")
	require.Zero(t, source.eofs, "creating and closing a reader must not consume input")
	replay, err := buffer.getBody()
	require.NoError(t, err)
	defer replay.Close()
	payload, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.Equal(t, "request", string(payload))
	require.Zero(t, source.closes)
}

func TestRequestBufferEmptyBody(t *testing.T) {
	t.Parallel()
	buffer := &requestBuffer{err: io.EOF}
	body, err := buffer.getBody()
	require.NoError(t, err)
	require.Equal(t, http.NoBody, body)
}
