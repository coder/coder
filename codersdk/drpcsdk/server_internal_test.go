package drpcsdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"storj.io/drpc"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/testutil"
)

func TestRecoverHandler(t *testing.T) {
	t.Parallel()

	t.Run("Panic", func(t *testing.T) {
		t.Parallel()

		const panicValue = "sensitive panic details"
		sink := testutil.NewFakeSink(t)
		handler := &recoverHandler{
			logger: sink.Logger(),
			handler: handlerFunc(func(drpc.Stream, string) error {
				panic(panicValue)
			}),
		}

		err := handler.HandleRPC(contextStream{ctx: t.Context()}, "/test.Service/Panic")
		require.Error(t, err)
		require.True(t, drpc.InternalError.Has(err))
		require.NotContains(t, err.Error(), panicValue)

		entries := sink.Entries()
		require.Len(t, entries, 1)
		require.Equal(t, slog.LevelError, entries[0].Level)
		require.Equal(t, "panic serving dRPC request (recovered)", entries[0].Message)
		require.Equal(t, "/test.Service/Panic", fieldValue(entries[0].Fields, "rpc"))
		require.Equal(t, panicValue, fieldValue(entries[0].Fields, "panic"))
		stackValue := fieldValue(entries[0].Fields, "stack")
		stack, ok := stackValue.(string)
		require.True(t, ok, "stack field must be a string, got %T", stackValue)
		require.Contains(t, stack, "goroutine ")
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()

		expected := xerrors.New("handler error")
		handler := &recoverHandler{
			handler: handlerFunc(func(drpc.Stream, string) error {
				return expected
			}),
		}

		err := handler.HandleRPC(contextStream{ctx: t.Context()}, "/test.Service/Error")
		require.ErrorIs(t, err, expected)
	})
}

type handlerFunc func(drpc.Stream, string) error

func (f handlerFunc) HandleRPC(stream drpc.Stream, rpc string) error {
	return f(stream, rpc)
}

type contextStream struct {
	ctx context.Context
}

func (s contextStream) Context() context.Context                { return s.ctx }
func (contextStream) MsgSend(drpc.Message, drpc.Encoding) error { return nil }
func (contextStream) MsgRecv(drpc.Message, drpc.Encoding) error { return nil }
func (contextStream) CloseSend() error                          { return nil }
func (contextStream) Close() error                              { return nil }

func fieldValue(fields slog.Map, name string) any {
	for _, field := range fields {
		if field.Name == name {
			return field.Value
		}
	}
	return nil
}
