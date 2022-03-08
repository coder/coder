package peerbroker

import (
	"reflect"

	"github.com/pion/webrtc/v3"
	"golang.org/x/xerrors"

	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker/proto"
)

// Dial consumes the PeerBroker gRPC connection negotiation stream to produce a WebRTC peered connection.
func Dial(stream proto.DRPCPeerBroker_NegotiateConnectionClient, iceServers []webrtc.ICEServer, opts *peer.ConnOptions) (*peer.Conn, error) {
	peerConn, err := peer.Client(iceServers, opts)
	if err != nil {
		return nil, xerrors.Errorf("create peer connection: %w", err)
	}
	go func() {
		defer stream.Close()
		// Exchanging messages from the peer connection to negotiate a connection.
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
	go func() {
		// Exchanging messages from the server to negotiate a connection.
		for {
			serverToClientMessage, err := stream.Recv()
			if err != nil {
				_ = peerConn.CloseWithError(xerrors.Errorf("recv: %w", err))
				return
			}

			switch {
			case serverToClientMessage.GetSdp() != nil:
				peerConn.SetRemoteSessionDescription(webrtc.SessionDescription{
					Type: webrtc.SDPType(serverToClientMessage.GetSdp().SdpType),
					SDP:  serverToClientMessage.GetSdp().Sdp,
				})
			case serverToClientMessage.GetIceCandidate() != "":
				peerConn.AddRemoteCandidate(webrtc.ICECandidateInit{
					Candidate: serverToClientMessage.GetIceCandidate(),
				})
			default:
				_ = peerConn.CloseWithError(xerrors.Errorf("unhandled message: %s", reflect.TypeOf(serverToClientMessage).String()))
				return
			}
		}
	}()

	return peerConn, nil
}
