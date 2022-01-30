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
			case localNegotiation := <-peerConn.LocalNegotiation():
				err = stream.Send(&proto.NegotiateConnection_ClientToServer{
					Message: &proto.NegotiateConnection_ClientToServer_Negotiation{
						Negotiation: convertLocalNegotiation(localNegotiation),
					},
				})
				if err != nil {
					_ = peerConn.CloseWithError(xerrors.Errorf("send local session description: %w", err))
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
			if serverToClientMessage.GetNegotiation() == nil {
				_ = peerConn.CloseWithError(xerrors.Errorf("unhandled message: %s", reflect.TypeOf(serverToClientMessage).String()))
				return
			}

			err = peerConn.AddRemoteNegotiation(convertProtoNegotiation(serverToClientMessage.Negotiation))
			if err != nil {
				_ = peerConn.CloseWithError(xerrors.Errorf("add remote negotiation: %w", err))
				return
			}
		}
	}()

	return peerConn, nil
}

func convertLocalNegotiation(localNegotiation peer.Negotiation) *proto.Negotiation {
	protoNegotation := &proto.Negotiation{}
	if localNegotiation.SessionDescription != nil {
		protoNegotation.SessionDescription = &proto.WebRTCSessionDescription{
			SdpType: int32(localNegotiation.SessionDescription.Type),
			Sdp:     localNegotiation.SessionDescription.SDP,
		}
	}
	if len(localNegotiation.ICECandidates) > 0 {
		iceCandidates := make([]string, 0, len(localNegotiation.ICECandidates))
		for _, iceCandidate := range localNegotiation.ICECandidates {
			iceCandidates = append(iceCandidates, iceCandidate.Candidate)
		}
		protoNegotation.IceCandidates = iceCandidates
	}
	return protoNegotation
}

func convertProtoNegotiation(protoNegotiation *proto.Negotiation) peer.Negotiation {
	localNegotiation := peer.Negotiation{}
	if protoNegotiation.SessionDescription != nil {
		localNegotiation.SessionDescription = &webrtc.SessionDescription{
			Type: webrtc.SDPType(protoNegotiation.SessionDescription.SdpType),
			SDP:  protoNegotiation.SessionDescription.Sdp,
		}
	}
	if len(protoNegotiation.IceCandidates) > 0 {
		candidates := make([]webrtc.ICECandidateInit, 0, len(protoNegotiation.IceCandidates))
		for _, iceCandidate := range protoNegotiation.IceCandidates {
			candidates = append(candidates, webrtc.ICECandidateInit{
				Candidate: iceCandidate,
			})
		}
		localNegotiation.ICECandidates = candidates
	}
	return localNegotiation
}
