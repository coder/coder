package peerbroker

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	protobuf "google.golang.org/protobuf/proto"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/peerbroker/proto"
)

var (
	// Each NegotiateConnection() function call spawns a new stream.
	streamIDLength = len(uuid.NewString())
	// We shouldn't PubSub anything larger than this!
	maxPayloadSizeBytes = 8192
)

// ProxyOptions provides values to configure a proxy.
type ProxyOptions struct {
	ChannelID string
	Logger    slog.Logger
	Pubsub    database.Pubsub
}

// ProxyDial writes client negotiation streams over PubSub.
//
// PubSub is used to geodistribute WebRTC handshakes. All negotiation
// messages are small in size (<=8KB), and we don't require delivery
// guarantees because connections can always be renegotiated.
//
//	┌────────────────────┐   ┌─────────────────────────────┐
//	│      coderd        │   │          coderd             │
//
// ┌─────────────────────┐   │/<agent-id>/connect │   │    /<agent-id>/listen       │
// │       client        │   │                    │   │                             │   ┌─────┐
// │                     ├──►│Creates a stream ID │◄─►│Subscribe() to the <agent-id>│◄──┤agent│
// │NegotiateConnection()│   │and Publish() to the│   │channel. Parse the stream ID │   └─────┘
// └─────────────────────┘   │<agent-id> channel: │   │from payloads to create new  │
//
//	│                    │   │NegotiateConnection() streams│
//	│<stream-id><payload>│   │or write to existing ones.   │
//	└────────────────────┘   └─────────────────────────────┘
func ProxyDial(client proto.DRPCPeerBrokerClient, options ProxyOptions) (io.Closer, error) {
	proxyDial := &proxyDial{
		channelID:  options.ChannelID,
		logger:     options.Logger,
		pubsub:     options.Pubsub,
		connection: client,
		streams:    make(map[string]proto.DRPCPeerBroker_NegotiateConnectionClient),
	}
	return proxyDial, proxyDial.listen()
}

// ProxyListen accepts client negotiation streams over PubSub and writes them to the listener
// as new NegotiateConnection() streams.
func ProxyListen(ctx context.Context, connListener net.Listener, options ProxyOptions) error {
	mux := drpcmux.New()
	err := proto.DRPCRegisterPeerBroker(mux, &proxyListen{
		channelID: options.ChannelID,
		pubsub:    options.Pubsub,
		logger:    options.Logger,
	})
	if err != nil {
		return xerrors.Errorf("register peer broker: %w", err)
	}
	server := drpcserver.New(mux)
	err = server.Serve(ctx, connListener)
	if err != nil {
		if errors.Is(err, yamux.ErrSessionShutdown) {
			return nil
		}
		return xerrors.Errorf("serve: %w", err)
	}
	return nil
}

type proxyListen struct {
	channelID string
	pubsub    database.Pubsub
	logger    slog.Logger
}

func (p *proxyListen) NegotiateConnection(stream proto.DRPCPeerBroker_NegotiateConnectionStream) error {
	streamID := uuid.NewString()
	var err error
	closeSubscribe, err := p.pubsub.Subscribe(proxyInID(p.channelID), func(ctx context.Context, message []byte) {
		err := p.onServerToClientMessage(streamID, stream, message)
		if err != nil {
			p.logger.Debug(ctx, "failed to accept server message", slog.Error(err))
		}
	})
	if err != nil {
		return xerrors.Errorf("subscribe: %w", err)
	}
	defer closeSubscribe()
	for {
		clientToServerMessage, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return xerrors.Errorf("recv: %w", err)
		}
		data, err := protobuf.Marshal(clientToServerMessage)
		if err != nil {
			return xerrors.Errorf("marshal: %w", err)
		}
		if len(data) > maxPayloadSizeBytes {
			return xerrors.Errorf("maximum payload size %d exceeded", maxPayloadSizeBytes)
		}
		data = append([]byte(streamID), data...)
		err = p.pubsub.Publish(proxyOutID(p.channelID), marshal(data))
		if err != nil {
			return xerrors.Errorf("publish: %w", err)
		}
	}
	return nil
}

func (*proxyListen) onServerToClientMessage(streamID string, stream proto.DRPCPeerBroker_NegotiateConnectionStream, message []byte) error {
	var err error
	message, err = unmarshal(message)
	if err != nil {
		return xerrors.Errorf("decode: %w", err)
	}
	if len(message) < streamIDLength {
		return xerrors.Errorf("got message length %d < %d", len(message), streamIDLength)
	}
	serverStreamID := string(message[0:streamIDLength])
	if serverStreamID != streamID {
		// It's not trying to communicate with this stream!
		return nil
	}
	var msg proto.Exchange
	err = protobuf.Unmarshal(message[streamIDLength:], &msg)
	if err != nil {
		return xerrors.Errorf("unmarshal message: %w", err)
	}
	err = stream.Send(&msg)
	if err != nil {
		return xerrors.Errorf("send message: %w", err)
	}
	return nil
}

type proxyDial struct {
	channelID string
	pubsub    database.Pubsub
	logger    slog.Logger

	connection     proto.DRPCPeerBrokerClient
	closeSubscribe func()
	streamMutex    sync.Mutex
	streams        map[string]proto.DRPCPeerBroker_NegotiateConnectionClient
}

func (p *proxyDial) listen() error {
	var err error
	p.closeSubscribe, err = p.pubsub.Subscribe(proxyOutID(p.channelID), func(ctx context.Context, message []byte) {
		err := p.onClientToServerMessage(ctx, message)
		if err != nil {
			p.logger.Debug(ctx, "failed to accept client message", slog.Error(err))
		}
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *proxyDial) onClientToServerMessage(ctx context.Context, message []byte) error {
	var err error
	message, err = unmarshal(message)
	if err != nil {
		return xerrors.Errorf("decode: %w", err)
	}
	if len(message) < streamIDLength {
		return xerrors.Errorf("got message length %d < %d", len(message), streamIDLength)
	}
	streamID := string(message[0:streamIDLength])
	p.streamMutex.Lock()
	stream, ok := p.streams[streamID]
	if !ok {
		stream, err = p.connection.NegotiateConnection(ctx)
		if err != nil {
			p.streamMutex.Unlock()
			return xerrors.Errorf("negotiate connection: %w", err)
		}
		p.streams[streamID] = stream
		go func() {
			defer stream.Close()

			err := p.onServerToClientMessage(streamID, stream)
			if err != nil {
				p.logger.Debug(ctx, "failed to accept server message", slog.Error(err))
			}
		}()
		go func() {
			<-stream.Context().Done()
			p.streamMutex.Lock()
			delete(p.streams, streamID)
			p.streamMutex.Unlock()
		}()
	}
	p.streamMutex.Unlock()

	var msg proto.Exchange
	err = protobuf.Unmarshal(message[streamIDLength:], &msg)
	if err != nil {
		return xerrors.Errorf("unmarshal message: %w", err)
	}
	err = stream.Send(&msg)
	if err != nil {
		return xerrors.Errorf("write message: %w", err)
	}
	return nil
}

func (p *proxyDial) onServerToClientMessage(streamID string, stream proto.DRPCPeerBroker_NegotiateConnectionClient) error {
	for {
		serverToClientMessage, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if errors.Is(err, context.Canceled) {
				break
			}
			return xerrors.Errorf("recv: %w", err)
		}
		data, err := protobuf.Marshal(serverToClientMessage)
		if err != nil {
			return xerrors.Errorf("marshal: %w", err)
		}
		if len(data) > maxPayloadSizeBytes {
			return xerrors.Errorf("maximum payload size %d exceeded", maxPayloadSizeBytes)
		}
		data = append([]byte(streamID), data...)
		err = p.pubsub.Publish(proxyInID(p.channelID), marshal(data))
		if err != nil {
			return xerrors.Errorf("publish: %w", err)
		}
	}
	return nil
}

func (p *proxyDial) Close() error {
	p.streamMutex.Lock()
	defer p.streamMutex.Unlock()
	p.closeSubscribe()
	return nil
}

// base64 needs to be used here to keep the pubsub messages in UTF-8 range.
// PostgreSQL cannot handle non UTF-8 messages over pubsub.
func marshal(data []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(data))
}

func unmarshal(data []byte) ([]byte, error) {
	return base64.StdEncoding.DecodeString(string(data))
}

func proxyOutID(channelID string) string {
	return fmt.Sprintf("%s-out", channelID)
}

func proxyInID(channelID string) string {
	return fmt.Sprintf("%s-in", channelID)
}
