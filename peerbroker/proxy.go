package peerbroker

import (
	"context"
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
	"github.com/coder/coder/database"
	"github.com/coder/coder/peerbroker/proto"
)

var streamIDLength = len(uuid.NewString())

// ProxyOptions customizes the proxy behavior.
type ProxyOptions struct {
	ChannelID string
	Logger    slog.Logger
	Pubsub    database.Pubsub
}

// ProxyDial passes streams through the connection listener over pubsub.
func ProxyDial(ctx context.Context, connListener net.Listener, options ProxyOptions) error {
	mux := drpcmux.New()
	err := proto.DRPCRegisterPeerBroker(mux, &proxyDial{
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

// ProxyListen passes streams from the pubsub to the client provided.
func ProxyListen(client proto.DRPCPeerBrokerClient, options ProxyOptions) (io.Closer, error) {
	proxyListen := &proxyListen{
		channelID:  options.ChannelID,
		logger:     options.Logger,
		pubsub:     options.Pubsub,
		connection: client,
		streams:    make(map[string]proto.DRPCPeerBroker_NegotiateConnectionClient),
	}
	return proxyListen, proxyListen.listen()
}

type proxyDial struct {
	channelID string
	pubsub    database.Pubsub
	logger    slog.Logger
}

func (p *proxyDial) NegotiateConnection(stream proto.DRPCPeerBroker_NegotiateConnectionStream) error {
	streamID := uuid.NewString()
	var err error
	closeSubscribe, err := p.pubsub.Subscribe(proxyInID(p.channelID), func(ctx context.Context, message []byte) {
		err := p.onServerMessage(streamID, stream, message)
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
		data = append([]byte(streamID), data...)
		err = p.pubsub.Publish(proxyOutID(p.channelID), data)
		if err != nil {
			return xerrors.Errorf("publish: %w", err)
		}
	}
	return nil
}

func (p *proxyDial) onServerMessage(streamID string, stream proto.DRPCPeerBroker_NegotiateConnectionStream, message []byte) error {
	if len(message) < streamIDLength {
		return xerrors.Errorf("got message length %d < %d", len(message), streamIDLength)
	}
	serverStreamID := string(message[0:streamIDLength])
	if serverStreamID != streamID {
		// It's not trying to communicate with this stream!
		return nil
	}
	var msg proto.NegotiateConnection_ServerToClient
	err := protobuf.Unmarshal(message[streamIDLength:], &msg)
	if err != nil {
		return xerrors.Errorf("unmarshal message: %w", err)
	}
	err = stream.Send(&msg)
	if err != nil {
		return xerrors.Errorf("send message: %w", err)
	}
	return nil
}

type proxyListen struct {
	channelID string
	pubsub    database.Pubsub
	logger    slog.Logger

	connection     proto.DRPCPeerBrokerClient
	closeMutex     sync.Mutex
	closeSubscribe func()
	streamMutex    sync.Mutex
	streams        map[string]proto.DRPCPeerBroker_NegotiateConnectionClient
}

func (p *proxyListen) listen() error {
	var err error
	p.closeSubscribe, err = p.pubsub.Subscribe(proxyOutID(p.channelID), func(ctx context.Context, message []byte) {
		err := p.onClientMessage(ctx, message)
		if err != nil {
			p.logger.Debug(ctx, "failed to accept client message", slog.Error(err))
		}
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *proxyListen) onClientMessage(ctx context.Context, message []byte) error {
	if len(message) < streamIDLength {
		return xerrors.Errorf("got message length %d < %d", len(message), streamIDLength)
	}
	var err error
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

			err = p.onServerMessage(streamID, stream)
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

	var msg proto.NegotiateConnection_ClientToServer
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

func (p *proxyListen) onServerMessage(streamID string, stream proto.DRPCPeerBroker_NegotiateConnectionClient) error {
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
		data = append([]byte(streamID), data...)
		err = p.pubsub.Publish(proxyInID(p.channelID), data)
		if err != nil {
			return xerrors.Errorf("publish: %w", err)
		}
	}
	return nil
}

func (p *proxyListen) Close() error {
	p.streamMutex.Lock()
	defer p.streamMutex.Unlock()
	p.closeSubscribe()
	return nil
}

func proxyOutID(channelID string) string {
	return fmt.Sprintf("%s-out", channelID)
}

func proxyInID(channelID string) string {
	return fmt.Sprintf("%s-in", channelID)
}
