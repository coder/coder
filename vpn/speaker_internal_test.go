package vpn

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/proto"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

// TestSpeaker_RawPeer tests the speaker with a peer that we simulate by directly making reads and
// writes to the other end of the pipe. There should be at least one test that does this, rather
// than use 2 speakers so that we don't have a bug where we don't adhere to the stated protocol, but
// both sides have the bug and can still communicate.
func TestSpeaker_RawPeer(t *testing.T) {
	t.Parallel()
	mp, tp := net.Pipe()
	defer mp.Close()
	defer tp.Close()
	ctx := testutil.Context(t, testutil.WaitShort)
	// We're going to use deadlines for this test so that we don't hang the main test thread if
	// the speaker misbehaves.
	err := mp.SetReadDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	err = mp.SetWriteDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	logger := testutil.Logger(t)
	var tun *speaker[*TunnelMessage, *ManagerMessage, ManagerMessage]
	errCh := make(chan error, 1)
	go func() {
		s, err := newSpeaker[*TunnelMessage, *ManagerMessage](ctx, logger, tp, SpeakerRoleTunnel, SpeakerRoleManager)
		tun = s
		errCh <- err
	}()

	expectedHandshake := "codervpn tunnel 1.1\n"

	b := make([]byte, 256)
	n, err := mp.Read(b)
	require.NoError(t, err)
	require.Equal(t, expectedHandshake, string(b[:n]))

	_, err = mp.Write([]byte("codervpn manager 1.3,2.1\n"))
	require.NoError(t, err)

	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.NoError(t, err)
	tun.start()

	// send a message and verify it follows protocol for encoding
	testutil.RequireSendCtx(ctx, t, tun.sendCh, &TunnelMessage{
		Msg: &TunnelMessage_Start{
			Start: &StartResponse{},
		},
	})

	var msgLen uint32
	err = binary.Read(mp, binary.BigEndian, &msgLen)
	require.NoError(t, err)
	msgBuf := make([]byte, msgLen)
	n, err = mp.Read(msgBuf)
	require.NoError(t, err)
	// #nosec G115 - Safe conversion of read bytes count to uint32 for comparison with message length
	require.Equal(t, msgLen, uint32(n))
	msg := new(TunnelMessage)
	err = proto.Unmarshal(msgBuf, msg)
	require.NoError(t, err)
	_, ok := msg.Msg.(*TunnelMessage_Start)
	require.True(t, ok)

	// Should close the pipe on close of the speaker.
	err = tun.Close()
	require.NoError(t, err)
	_, err = mp.Read(b)
	require.ErrorIs(t, err, io.EOF)
}

func TestSpeaker_HandshakeRWFailure(t *testing.T) {
	t.Parallel()
	mp, tp := net.Pipe()
	// immediately close the pipe, so we'll get read & write failures on handshake
	_ = mp.Close()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	var tun *speaker[*TunnelMessage, *ManagerMessage, ManagerMessage]
	errCh := make(chan error, 1)
	go func() {
		s, err := newSpeaker[*TunnelMessage, *ManagerMessage](
			ctx, logger.Named("tun"), tp, SpeakerRoleTunnel, SpeakerRoleManager,
		)
		tun = s
		errCh <- err
	}()
	err := testutil.RequireRecvCtx(ctx, t, errCh)
	require.ErrorContains(t, err, "handshake failed")
	require.Nil(t, tun)
}

func TestSpeaker_HandshakeCtxDone(t *testing.T) {
	t.Parallel()
	mp, tp := net.Pipe()
	defer mp.Close()
	defer tp.Close()
	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	var tun *speaker[*TunnelMessage, *ManagerMessage, ManagerMessage]
	errCh := make(chan error, 1)
	go func() {
		s, err := newSpeaker[*TunnelMessage, *ManagerMessage](
			ctx, logger.Named("tun"), tp, SpeakerRoleTunnel, SpeakerRoleManager,
		)
		tun = s
		errCh <- err
	}()
	cancel()
	err := testutil.RequireRecvCtx(testCtx, t, errCh)
	require.ErrorContains(t, err, "handshake failed")
	require.Nil(t, tun)
}

func TestSpeaker_OversizeHandshake(t *testing.T) {
	t.Parallel()
	mp, tp := net.Pipe()
	defer mp.Close()
	defer tp.Close()
	ctx := testutil.Context(t, testutil.WaitShort)
	// We're going to use deadlines for this test so that we don't hang the main test thread if
	// the speaker misbehaves.
	err := mp.SetReadDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	err = mp.SetWriteDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	var tun *speaker[*TunnelMessage, *ManagerMessage, ManagerMessage]
	errCh := make(chan error, 1)
	go func() {
		s, err := newSpeaker[*TunnelMessage, *ManagerMessage](ctx, logger, tp, SpeakerRoleTunnel, SpeakerRoleManager)
		tun = s
		errCh <- err
	}()

	expectedHandshake := "codervpn tunnel 1.1\n"

	b := make([]byte, 256)
	n, err := mp.Read(b)
	require.NoError(t, err)
	require.Equal(t, expectedHandshake, string(b[:n]))

	badHandshake := strings.Repeat("bad", 256)
	_, err = mp.Write([]byte(badHandshake))
	require.Error(t, err) // other side closes when we write too much

	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.ErrorContains(t, err, "handshake failed")
	require.Nil(t, tun)
}

func TestSpeaker_HandshakeInvalid(t *testing.T) {
	t.Parallel()
	// nolint: paralleltest // no longer need to reinitialize loop vars in go 1.22
	for _, tc := range []struct {
		name, handshake string
	}{
		{name: "preamble", handshake: "ssh manager 1.1\n"},
		{name: "2components", handshake: "ssh manager\n"},
		{name: "newmajors", handshake: "codervpn manager 2.0,3.0\n"},
		{name: "0version", handshake: "codervpn 0.1 manager\n"},
		{name: "unknown_role", handshake: "codervpn 1.1 supervisor\n"},
		{name: "unexpected_role", handshake: "codervpn 1.1 tunnel\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mp, tp := net.Pipe()
			defer mp.Close()
			defer tp.Close()
			ctx := testutil.Context(t, testutil.WaitShort)
			// We're going to use deadlines for this test so that we don't hang the main test thread if
			// the speaker misbehaves.
			err := mp.SetReadDeadline(time.Now().Add(testutil.WaitShort))
			require.NoError(t, err)
			err = mp.SetWriteDeadline(time.Now().Add(testutil.WaitShort))
			require.NoError(t, err)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
			var tun *speaker[*TunnelMessage, *ManagerMessage, ManagerMessage]
			errCh := make(chan error, 1)
			go func() {
				s, err := newSpeaker[*TunnelMessage, *ManagerMessage](ctx, logger, tp, SpeakerRoleTunnel, SpeakerRoleManager)
				tun = s
				errCh <- err
			}()

			_, err = mp.Write([]byte(tc.handshake))
			require.NoError(t, err)

			expectedHandshake := "codervpn tunnel 1.1\n"
			b := make([]byte, 256)
			n, err := mp.Read(b)
			require.NoError(t, err)
			require.Equal(t, expectedHandshake, string(b[:n]))

			err = testutil.RequireRecvCtx(ctx, t, errCh)
			require.ErrorContains(t, err, "validate header")
			require.Nil(t, tun)
		})
	}
}

// TestSpeaker_RawPeer tests the speaker with a peer that we simulate by directly making reads and
// writes to the other end of the pipe. There should be at least one test that does this, rather
// than use 2 speakers so that we don't have a bug where we don't adhere to the stated protocol, but
// both sides have the bug and can still communicate.
func TestSpeaker_CorruptMessage(t *testing.T) {
	t.Parallel()
	mp, tp := net.Pipe()
	defer mp.Close()
	defer tp.Close()
	ctx := testutil.Context(t, testutil.WaitShort)
	// We're going to use deadlines for this test so that we don't hang the main test thread if
	// the speaker misbehaves.
	err := mp.SetReadDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	err = mp.SetWriteDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	var tun *speaker[*TunnelMessage, *ManagerMessage, ManagerMessage]
	errCh := make(chan error, 1)
	go func() {
		s, err := newSpeaker[*TunnelMessage, *ManagerMessage](ctx, logger, tp, SpeakerRoleTunnel, SpeakerRoleManager)
		tun = s
		errCh <- err
	}()

	expectedHandshake := "codervpn tunnel 1.1\n"

	b := make([]byte, 256)
	n, err := mp.Read(b)
	require.NoError(t, err)
	require.Equal(t, expectedHandshake, string(b[:n]))

	_, err = mp.Write([]byte("codervpn manager 1.0\n"))
	require.NoError(t, err)

	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.NoError(t, err)
	tun.start()

	err = binary.Write(mp, binary.BigEndian, uint32(10))
	require.NoError(t, err)
	n, err = mp.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	require.NoError(t, err)
	require.EqualValues(t, 10, n)

	// it should hang up on us if we write nonsense
	_, err = mp.Read(b)
	require.ErrorIs(t, err, io.EOF)
}

func TestSpeaker_unaryRPC_mainline(t *testing.T) {
	t.Parallel()
	ctx, tun, mgr := setupSpeakers(t)

	errCh := make(chan error, 1)
	var resp *TunnelMessage
	go func() {
		r, err := mgr.unaryRPC(ctx, &ManagerMessage{
			Msg: &ManagerMessage_Start{
				Start: &StartRequest{
					CoderUrl: "https://coder.example.com",
				},
			},
		})
		resp = r
		errCh <- err
	}()
	req := testutil.RequireRecvCtx(ctx, t, tun.requests)
	require.NotEqualValues(t, 0, req.msg.GetRpc().GetMsgId())
	require.Equal(t, "https://coder.example.com", req.msg.GetStart().GetCoderUrl())
	err := req.sendReply(&TunnelMessage{
		Msg: &TunnelMessage_Start{
			Start: &StartResponse{},
		},
	})
	require.NoError(t, err)
	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.NoError(t, err)
	_, ok := resp.Msg.(*TunnelMessage_Start)
	require.True(t, ok)

	// closing the manager should close the tun.requests channel
	err = mgr.Close()
	require.NoError(t, err)
	select {
	case _, ok := <-tun.requests:
		require.False(t, ok)
	case <-ctx.Done():
		t.Fatal("timed out waiting for requests to close")
	}
}

func TestSpeaker_unaryRPC_canceled(t *testing.T) {
	t.Parallel()
	testCtx, tun, mgr := setupSpeakers(t)

	ctx, cancel := context.WithCancel(testCtx)
	defer cancel()
	errCh := make(chan error, 1)
	var resp *TunnelMessage
	go func() {
		r, err := mgr.unaryRPC(ctx, &ManagerMessage{
			Msg: &ManagerMessage_Start{
				Start: &StartRequest{
					CoderUrl: "https://coder.example.com",
				},
			},
		})
		resp = r
		errCh <- err
	}()
	req := testutil.RequireRecvCtx(testCtx, t, tun.requests)
	require.NotEqualValues(t, 0, req.msg.GetRpc().GetMsgId())
	require.Equal(t, "https://coder.example.com", req.msg.GetStart().GetCoderUrl())

	cancel()
	err := testutil.RequireRecvCtx(testCtx, t, errCh)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, resp)

	err = req.sendReply(&TunnelMessage{
		Msg: &TunnelMessage_Start{
			Start: &StartResponse{},
		},
	})
	require.NoError(t, err)
}

func TestSpeaker_unaryRPC_hung_up(t *testing.T) {
	t.Parallel()
	testCtx, tun, mgr := setupSpeakers(t)

	ctx, cancel := context.WithCancel(testCtx)
	defer cancel()
	errCh := make(chan error, 1)
	var resp *TunnelMessage
	go func() {
		r, err := mgr.unaryRPC(ctx, &ManagerMessage{
			Msg: &ManagerMessage_Start{
				Start: &StartRequest{
					CoderUrl: "https://coder.example.com",
				},
			},
		})
		resp = r
		errCh <- err
	}()
	req := testutil.RequireRecvCtx(testCtx, t, tun.requests)
	require.NotEqualValues(t, 0, req.msg.GetRpc().GetMsgId())
	require.Equal(t, "https://coder.example.com", req.msg.GetStart().GetCoderUrl())

	// When: Tunnel closes instead of replying.
	err := tun.Close()
	require.NoError(t, err)
	// Then: we should get an error on the RPC.
	err = testutil.RequireRecvCtx(testCtx, t, errCh)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Nil(t, resp)
}

func TestSpeaker_unaryRPC_sendLoop(t *testing.T) {
	t.Parallel()
	testCtx, tun, mgr := setupSpeakers(t)

	ctx, cancel := context.WithCancel(testCtx)
	defer cancel()

	// When: Tunnel closes before we send the RPC
	err := tun.Close()
	require.NoError(t, err)

	// When: serdes sendloop is closed
	// Send a message from the manager. This closes the manager serdes sendloop, since it will error
	// when writing the message to the (closed) pipe.
	testutil.RequireSendCtx(ctx, t, mgr.sendCh, &ManagerMessage{
		Msg: &ManagerMessage_GetPeerUpdate{},
	})

	// When: we send an RPC
	errCh := make(chan error, 1)
	var resp *TunnelMessage
	go func() {
		r, err := mgr.unaryRPC(ctx, &ManagerMessage{
			Msg: &ManagerMessage_Start{
				Start: &StartRequest{
					CoderUrl: "https://coder.example.com",
				},
			},
		})
		resp = r
		errCh <- err
	}()

	// Then: we should get an error on the RPC.
	err = testutil.RequireRecvCtx(testCtx, t, errCh)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Nil(t, resp)
}

func setupSpeakers(t *testing.T) (
	context.Context, *speaker[*TunnelMessage, *ManagerMessage, ManagerMessage], *speaker[*ManagerMessage, *TunnelMessage, TunnelMessage],
) {
	mp, tp := net.Pipe()
	t.Cleanup(func() { _ = mp.Close() })
	t.Cleanup(func() { _ = tp.Close() })
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	var tun *speaker[*TunnelMessage, *ManagerMessage, ManagerMessage]
	var mgr *speaker[*ManagerMessage, *TunnelMessage, TunnelMessage]
	errCh := make(chan error, 2)
	go func() {
		s, err := newSpeaker[*TunnelMessage, *ManagerMessage](
			ctx, logger.Named("tun"), tp, SpeakerRoleTunnel, SpeakerRoleManager,
		)
		tun = s
		errCh <- err
	}()
	go func() {
		s, err := newSpeaker[*ManagerMessage, *TunnelMessage](
			ctx, logger.Named("mgr"), mp, SpeakerRoleManager, SpeakerRoleTunnel,
		)
		mgr = s
		errCh <- err
	}()
	err := testutil.RequireRecvCtx(ctx, t, errCh)
	require.NoError(t, err)
	err = testutil.RequireRecvCtx(ctx, t, errCh)
	require.NoError(t, err)
	tun.start()
	mgr.start()
	return ctx, tun, mgr
}
