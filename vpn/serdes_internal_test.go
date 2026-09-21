package vpn

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/coder/coder/v2/testutil"
)

func TestSerdes_StopClosesAfterResponse(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		manager, tunnel := net.Pipe()
		defer manager.Close()
		sendCh := make(chan *TunnelMessage)
		recvCh := make(chan *ManagerMessage)
		s := newSerdes(ctx, testutil.Logger(t), tunnel, sendCh, recvCh)
		s.start()
		defer func() {
			cancel()
			require.NoError(t, s.Close())
		}()

		stop := &TunnelMessage{
			Rpc: &RPC{ResponseTo: 1},
			Msg: &TunnelMessage_Stop{Stop: &StopResponse{Success: true}},
		}
		sendCh <- stop
		synctest.Wait()

		// Keep cancellation pending, with another sender parked while the
		// writer is blocked writing Stop. Stop must be the final frame.
		go func() {
			select {
			case <-ctx.Done():
			case sendCh <- &TunnelMessage{Msg: &TunnelMessage_Log{Log: &Log{Message: "late log"}}}:
			}
		}()
		synctest.Wait()

		var length uint32
		require.NoError(t, binary.Read(manager, binary.BigEndian, &length))
		payload := make([]byte, length)
		_, err := io.ReadFull(manager, payload)
		require.NoError(t, err)
		got := new(TunnelMessage)
		require.NoError(t, proto.Unmarshal(payload, got))
		require.True(t, proto.Equal(stop, got))
		require.ErrorIs(t, binary.Read(manager, binary.BigEndian, &length), io.EOF)
	})
}
