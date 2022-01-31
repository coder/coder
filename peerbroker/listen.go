package peerbroker

import (
	"context"
	"errors"
	"io"
	"reflect"
	"sync"

	"github.com/pion/webrtc/v3"
	"golang.org/x/xerrors"
	"storj.io/drpc"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker/proto"
)

// Listen consumes the transport as the server-side of the PeerBroker dRPC service.
// The Accept function must be serviced, or new connections will hang.
func Listen(transport drpc.Transport, opts *peer.ConnOptions) (*Listener, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	listener := &Listener{
		connectionChannel: make(chan *peer.Conn),

		closeFunc: cancelFunc,
		closed:    make(chan struct{}),
	}

	mux := drpcmux.New()
	err := proto.DRPCRegisterPeerBroker(mux, &peerBrokerService{
		connOptions: opts,

		listener: listener,
	})
	if err != nil {
		return nil, xerrors.Errorf("register peer broker: %w", err)
	}
	srv := drpcserver.New(mux)
	go func() {
		err := srv.ServeOne(ctx, transport)
		_ = listener.closeWithError(err)
	}()

	return listener, nil
}

type Listener struct {
	connectionChannel chan *peer.Conn

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

	connOptions *peer.ConnOptions
}

// NegotiateConnection negotiates a WebRTC connection.
func (b *peerBrokerService) NegotiateConnection(stream proto.DRPCPeerBroker_NegotiateConnectionStream) error {
	// Start with no ICE servers. They can be sent by the client if provided.
	peerConn, err := peer.Server([]webrtc.ICEServer{}, b.connOptions)
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
				err = stream.Send(&proto.NegotiateConnection_ServerToClient{
					Message: &proto.NegotiateConnection_ServerToClient_Answer{
						Answer: &proto.WebRTCSessionDescription{
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
				err = stream.Send(&proto.NegotiateConnection_ServerToClient{
					Message: &proto.NegotiateConnection_ServerToClient_IceCandidate{
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
			if errors.Is(err, io.EOF) {
				break
			}
			return peerConn.CloseWithError(xerrors.Errorf("recv: %w", err))
		}

		switch {
		case clientToServerMessage.GetOffer() != nil:
			peerConn.SetRemoteSessionDescription(webrtc.SessionDescription{
				Type: webrtc.SDPType(clientToServerMessage.GetOffer().SdpType),
				SDP:  clientToServerMessage.GetOffer().Sdp,
			})
		case clientToServerMessage.GetServers() != nil:
			// Convert protobuf ICE servers to the WebRTC type.
			iceServers := make([]webrtc.ICEServer, 0, len(clientToServerMessage.GetServers().Servers))
			for _, iceServer := range clientToServerMessage.GetServers().Servers {
				iceServers = append(iceServers, webrtc.ICEServer{
					URLs:           iceServer.Urls,
					Username:       iceServer.Username,
					Credential:     iceServer.Credential,
					CredentialType: webrtc.ICECredentialType(iceServer.CredentialType),
				})
			}
			err = peerConn.SetConfiguration(webrtc.Configuration{
				ICEServers: iceServers,
			})
			if err != nil {
				return peerConn.CloseWithError(xerrors.Errorf("set ice configuration: %w", err))
			}
		case clientToServerMessage.GetIceCandidate() != "":
			peerConn.AddRemoteCandidate(webrtc.ICECandidateInit{
				Candidate: clientToServerMessage.GetIceCandidate(),
			})
		default:
			return peerConn.CloseWithError(xerrors.Errorf("unhandled message: %s", reflect.TypeOf(clientToServerMessage).String()))
		}
	}

	return nil
}
