package vpn

import (
	"context"
	"encoding/binary"
	"io"
	"sync"

	"google.golang.org/protobuf/proto"

	"cdr.dev/slog"
)

// MaxLength is the largest possible CoderVPN Protocol message size. This is set
// so that a misbehaving peer can't cause us to allocate a huge amount of memory.
const MaxLength = 0x1000000 // 16MiB

// serdes SERializes and DESerializes protobuf messages to and from the conn.
type serdes[S rpcMessage, R receivableRPCMessage[RR], RR any] struct {
	ctx       context.Context
	logger    slog.Logger
	conn      io.ReadWriteCloser
	sendCh    <-chan S
	recvCh    chan<- R
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func (s *serdes[_, R, RR]) recvLoop() {
	s.logger.Debug(s.ctx, "starting recvLoop")
	defer s.closeIdempotent()
	defer close(s.recvCh)
	for {
		var length uint32
		if err := binary.Read(s.conn, binary.BigEndian, &length); err != nil {
			s.logger.Debug(s.ctx, "failed to read length", slog.Error(err))
			return
		}
		if length > MaxLength {
			s.logger.Critical(s.ctx, "message length exceeds max",
				slog.F("length", length))
			return
		}
		s.logger.Debug(s.ctx, "about to read message", slog.F("length", length))
		mb := make([]byte, length)
		if n, err := io.ReadFull(s.conn, mb); err != nil {
			s.logger.Debug(s.ctx, "failed to read message",
				slog.Error(err),
				slog.F("num_bytes_read", n))
			return
		}
		msg := R(new(RR))
		if err := proto.Unmarshal(mb, msg); err != nil {
			s.logger.Critical(s.ctx, "failed to unmarshal message", slog.Error(err))
			return
		}
		select {
		case s.recvCh <- msg:
			s.logger.Debug(s.ctx, "passed received message to speaker")
		case <-s.ctx.Done():
			s.logger.Debug(s.ctx, "recvLoop canceled", slog.Error(s.ctx.Err()))
		}
	}
}

func (s *serdes[S, _, _]) sendLoop() {
	s.logger.Debug(s.ctx, "starting sendLoop")
	defer s.closeIdempotent()
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Debug(s.ctx, "sendLoop canceled", slog.Error(s.ctx.Err()))
			return
		case msg, ok := <-s.sendCh:
			if !ok {
				s.logger.Debug(s.ctx, "sendCh closed")
				return
			}
			mb, err := proto.Marshal(msg)
			if err != nil {
				s.logger.Critical(s.ctx, "failed to marshal message", slog.Error(err))
				return
			}
			// #nosec G115 - Safe conversion as protobuf message length is expected to be within uint32 range
			if err := binary.Write(s.conn, binary.BigEndian, uint32(len(mb))); err != nil {
				s.logger.Debug(s.ctx, "failed to write length", slog.Error(err))
				return
			}
			if _, err := s.conn.Write(mb); err != nil {
				s.logger.Debug(s.ctx, "failed to write message", slog.Error(err))
				return
			}
		}
	}
}

func (s *serdes[_, _, _]) closeIdempotent() {
	s.closeOnce.Do(func() {
		if err := s.conn.Close(); err != nil {
			s.logger.Error(s.ctx, "failed to close connection", slog.Error(err))
		} else {
			s.logger.Info(s.ctx, "closed connection")
		}
	})
}

// Close closes the serdes
// nolint: revive
func (s *serdes[_, _, _]) Close() error {
	s.closeIdempotent()
	s.wg.Wait()
	return nil
}

// start starts the goroutines that serialize and deserialize to the conn.
// nolint: revive
func (s *serdes[_, _, _]) start() {
	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		s.recvLoop()
	}()
	go func() {
		defer s.wg.Done()
		s.sendLoop()
	}()
}

func newSerdes[S rpcMessage, R receivableRPCMessage[RR], RR any](
	ctx context.Context, logger slog.Logger, conn io.ReadWriteCloser,
	sendCh <-chan S, recvCh chan<- R,
) *serdes[S, R, RR] {
	return &serdes[S, R, RR]{
		ctx:    ctx,
		logger: logger.Named("serdes"),
		conn:   conn,
		sendCh: sendCh,
		recvCh: recvCh,
	}
}
