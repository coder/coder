package drpcsdk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"storj.io/drpc"
	"storj.io/drpc/drpcserver"

	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestNewServerRecoversPanics(t *testing.T) {
	t.Parallel()

	const (
		panicRPC   = "/test.Service/Panic"
		echoRPC    = "/test.Service/Echo"
		panicValue = "sensitive panic details"
	)

	ctx := testutil.Context(t, testutil.WaitShort)
	serverCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	client, listener := drpcsdk.MemTransportPipe()
	defer func() {
		_ = client.Close()
		_ = listener.Close()
	}()

	handler := testHandlerFunc(func(stream drpc.Stream, rpc string) error {
		switch rpc {
		case panicRPC:
			panic(panicValue)
		case echoRPC:
			var message string
			if err := stream.MsgRecv(&message, stringEncoding{}); err != nil {
				return err
			}
			return stream.MsgSend(&message, stringEncoding{})
		default:
			return xerrors.Errorf("unexpected RPC %q", rpc)
		}
	})
	server := drpcsdk.NewServer(testutil.NewFakeSink(t).Logger(), handler, drpcserver.Options{
		Manager: drpcsdk.DefaultDRPCOptions(nil),
	})
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Serve(serverCtx, listener)
	}()

	request, response := "request", ""
	err := client.Invoke(ctx, panicRPC, stringEncoding{}, &request, &response)
	require.EqualError(t, err, "internal error: panic serving dRPC request")
	require.NotContains(t, err.Error(), panicValue)

	request, response = "healthy", ""
	err = client.Invoke(ctx, echoRPC, stringEncoding{}, &request, &response)
	require.NoError(t, err)
	require.Equal(t, request, response)

	cancel()
	require.NoError(t, testutil.RequireReceive(ctx, t, serverDone))
}

type testHandlerFunc func(drpc.Stream, string) error

func (f testHandlerFunc) HandleRPC(stream drpc.Stream, rpc string) error {
	return f(stream, rpc)
}

type stringEncoding struct{}

func (stringEncoding) Marshal(message drpc.Message) ([]byte, error) {
	value, ok := message.(*string)
	if !ok {
		return nil, xerrors.Errorf("marshal %T: expected *string", message)
	}
	return []byte(*value), nil
}

func (stringEncoding) Unmarshal(data []byte, message drpc.Message) error {
	value, ok := message.(*string)
	if !ok {
		return xerrors.Errorf("unmarshal %T: expected *string", message)
	}
	*value = string(data)
	return nil
}
