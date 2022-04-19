package peerbroker

import (
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"sync"

	"github.com/pion/webrtc/v3"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker/proto"
)

// ConnSettingsFunc returns initialization options for a connection
type ConnSettingsFunc func(ctx context.Context) ([]webrtc.ICEServer, *peer.ConnOptions, error)

// Listen consumes the transport as the server-side of the PeerBroker dRPC service.
// The Accept function must be serviced, or new connections will hang.
func Listen(connListener net.Listener, connSettingsFunc ConnSettingsFunc) (*Listener, error) {
	if connSettingsFunc == nil {
		connSettingsFunc = func(ctx context.Context) ([]webrtc.ICEServer, *peer.ConnOptions, error) {
			return []webrtc.ICEServer{}, nil, nil
		}
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	listener := &Listener{
		connectionChannel:  make(chan *peer.Conn),
		connectionListener: connListener,

		closeFunc: cancelFunc,
		closed:    make(chan struct{}),
	}

	mux := drpcmux.New()
	err := proto.DRPCRegisterPeerBroker(mux, &peerBrokerService{
		connSettingsFunc: connSettingsFunc,

		listener: listener,
	})
	if err != nil {
		return nil, xerrors.Errorf("register peer broker: %w", err)
	}
	srv := drpcserver.New(mux)
	go func() {
		err := srv.Serve(ctx, connListener)
		_ = listener.closeWithError(err)
	}()

	return listener, nil
}

type Listener struct {
	connectionChannel  chan *peer.Conn
	connectionListener net.Listener

	closeFunc  context.CancelFunc
	closed     chan struct{}
	closeMutex sync.Mutex
	closeError error
}

// Accept blocks until a connection arrives or the listener is closed.
func (l *Listener) Accept() (*peer.Conn, error) {
	select {
	case <-l.closed:
		return nil, l.closeError
	case conn := <-l.connectionChannel:
		return conn, nil
	}
}

// Close ends the listener. This will block all new WebRTC connections
// from establishing, but will not close active connections.
func (l *Listener) Close() error {
	return l.closeWithError(io.EOF)
}

func (l *Listener) closeWithError(err error) error {
	l.closeMutex.Lock()
	defer l.closeMutex.Unlock()

	if l.isClosed() {
		return l.closeError
	}

	_ = l.connectionListener.Close()
	l.closeError = err
	l.closeFunc()
	close(l.closed)

	return nil
}

func (l *Listener) isClosed() bool {
	select {
	case <-l.closed:
		return true
	default:
		return false
	}
}

// Implements the PeerBroker service protobuf definition.
type peerBrokerService struct {
	listener *Listener

	connSettingsFunc ConnSettingsFunc
}

// NegotiateConnection negotiates a WebRTC connection.
func (b *peerBrokerService) NegotiateConnection(stream proto.DRPCPeerBroker_NegotiateConnectionStream) error {
	iceServers, connOptions, err := b.connSettingsFunc(stream.Context())
	if err != nil {
		return xerrors.Errorf("get connection settings: %w", err)
	}
	peerConn, err := peer.Server(iceServers, connOptions)
	if err != nil {
		return xerrors.Errorf("create peer connection: %w", err)
	}
	select {
	case <-b.listener.closed:
		return peerConn.CloseWithError(b.listener.closeError)
	case b.listener.connectionChannel <- peerConn:
	}
	go func() {
		defer stream.Close()
		for {
			select {
			case <-peerConn.Closed():
				return
			case sessionDescription := <-peerConn.LocalSessionDescription():
				err = stream.Send(&proto.Exchange{
					Message: &proto.Exchange_Sdp{
						Sdp: &proto.WebRTCSessionDescription{
							SdpType: int32(sessionDescription.Type),
							Sdp:     sessionDescription.SDP,
						},
					},
				})
				if err != nil {
					_ = peerConn.CloseWithError(xerrors.Errorf("send local session description: %w", err))
					return
				}
			case iceCandidate := <-peerConn.LocalCandidate():
				err = stream.Send(&proto.Exchange{
					Message: &proto.Exchange_IceCandidate{
						IceCandidate: iceCandidate.Candidate,
					},
				})
				if err != nil {
					_ = peerConn.CloseWithError(xerrors.Errorf("send local candidate: %w", err))
					return
				}
			}
		}
	}()
	for {
		clientToServerMessage, err := stream.Recv()
		if err != nil {
			// p2p connections should never die if this stream does due
			// to proper closure or context cancellation!
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return nil
			}
			return peerConn.CloseWithError(xerrors.Errorf("recv: %w", err))
		}

		switch {
		case clientToServerMessage.GetSdp() != nil:
			peerConn.SetRemoteSessionDescription(webrtc.SessionDescription{
				Type: webrtc.SDPType(clientToServerMessage.GetSdp().SdpType),
				SDP:  clientToServerMessage.GetSdp().Sdp,
			})
		case clientToServerMessage.GetIceCandidate() != "":
			peerConn.AddRemoteCandidate(webrtc.ICECandidateInit{
				Candidate: clientToServerMessage.GetIceCandidate(),
			})
		default:
			return peerConn.CloseWithError(xerrors.Errorf("unhandled message: %s", reflect.TypeOf(clientToServerMessage).String()))
		}
	}
}
