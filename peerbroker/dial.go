package peerbroker

import (
	"reflect"

	"github.com/pion/webrtc/v3"
	"golang.org/x/xerrors"

	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker/proto"
)

// Dial consumes the PeerBroker gRPC connection negotiation stream to produce a WebRTC peered connection.
func Dial(stream proto.DRPCPeerBroker_NegotiateConnectionClient, iceServers []webrtc.ICEServer, opts *peer.ConnOpts) (*peer.Conn, error) {
	// Convert WebRTC ICE servers to the protobuf type.
	protoIceServers := make([]*proto.WebRTCICEServer, 0, len(iceServers))
	for _, iceServer := range iceServers {
		var credentialString string
		if value, ok := iceServer.Credential.(string); ok {
			credentialString = value
		}
		protoIceServers = append(protoIceServers, &proto.WebRTCICEServer{
			Urls:           iceServer.URLs,
			Username:       iceServer.Username,
			Credential:     credentialString,
			CredentialType: int32(iceServer.CredentialType),
		})
	}
	if len(protoIceServers) > 0 {
		// Send ICE servers to connect with.
		// Client sends ICE servers so clients can determine the node
		// servers will meet at. eg. us-west1.coder.com could be a TURN server.
		err := stream.Send(&proto.NegotiateConnection_ClientToServer{
			Message: &proto.NegotiateConnection_ClientToServer_Servers{
				Servers: &proto.WebRTCICEServers{
					Servers: protoIceServers,
				},
			},
		})
		if err != nil {
			return nil, xerrors.Errorf("write ice servers: %w", err)
		}
	}

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
				err = stream.Send(&proto.NegotiateConnection_ClientToServer{
					Message: &proto.NegotiateConnection_ClientToServer_Offer{
						Offer: &proto.WebRTCSessionDescription{
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
				err = stream.Send(&proto.NegotiateConnection_ClientToServer{
					Message: &proto.NegotiateConnection_ClientToServer_IceCandidate{
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
			case serverToClientMessage.GetAnswer() != nil:
				peerConn.SetRemoteSessionDescription(webrtc.SessionDescription{
					Type: webrtc.SDPType(serverToClientMessage.GetAnswer().SdpType),
					SDP:  serverToClientMessage.GetAnswer().Sdp,
				})
			case serverToClientMessage.GetIceCandidate() != "":
				err = peerConn.AddRemoteCandidate(webrtc.ICECandidateInit{
					Candidate: serverToClientMessage.GetIceCandidate(),
				})
				if err != nil {
					_ = peerConn.CloseWithError(xerrors.Errorf("add remote candidate: %w", err))
					return
				}
			default:
				_ = peerConn.CloseWithError(xerrors.Errorf("unhandled message: %s", reflect.TypeOf(serverToClientMessage).String()))
				return
			}
		}
	}()

	return peerConn, nil
}
