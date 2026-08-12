package chatd

import (
	"context"
	"errors"
	"net"
	"sync"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

var errStreamPartsTransportClosed = xerrors.New("stream parts transport closed")

type streamPartsServerTransport interface {
	ReadControl(context.Context) (streamPartsControl, error)
	WriteEvents(context.Context, []codersdk.ChatStreamEvent) error
	Close() error
}

type streamPartsClientTransport interface {
	WriteControl(context.Context, streamPartsControl) error
	ReadEvents(context.Context) ([]codersdk.ChatStreamEvent, error)
	Close() error
}

type streamPartsWebSocketServerTransport struct {
	conn *websocket.Conn
}

func (t streamPartsWebSocketServerTransport) ReadControl(ctx context.Context) (streamPartsControl, error) {
	var control streamPartsControl
	if err := wsjson.Read(ctx, t.conn, &control); err != nil {
		return streamPartsControl{}, err
	}
	return control, nil
}

func (t streamPartsWebSocketServerTransport) WriteEvents(ctx context.Context, events []codersdk.ChatStreamEvent) error {
	return wsjson.Write(ctx, t.conn, events)
}

func (t streamPartsWebSocketServerTransport) Close() error {
	return t.conn.Close(websocket.StatusNormalClosure, "")
}

type streamPartsWebSocketClientTransport struct {
	conn *websocket.Conn
}

func (t streamPartsWebSocketClientTransport) WriteControl(ctx context.Context, control streamPartsControl) error {
	return wsjson.Write(ctx, t.conn, control)
}

func (t streamPartsWebSocketClientTransport) ReadEvents(ctx context.Context) ([]codersdk.ChatStreamEvent, error) {
	var batch []codersdk.ChatStreamEvent
	if err := wsjson.Read(ctx, t.conn, &batch); err != nil {
		return nil, err
	}
	return batch, nil
}

func (t streamPartsWebSocketClientTransport) Close() error {
	return t.conn.Close(websocket.StatusNormalClosure, "")
}

type streamPartsChannelPipe struct {
	controlCh chan streamPartsControl
	eventsCh  chan []codersdk.ChatStreamEvent
	done      chan struct{}
	closeOnce sync.Once
}

type streamPartsChannelServerTransport struct {
	pipe *streamPartsChannelPipe
}

type streamPartsChannelClientTransport struct {
	pipe *streamPartsChannelPipe
}

func newStreamPartsChannelTransportPair() (streamPartsServerTransport, streamPartsClientTransport) {
	pipe := &streamPartsChannelPipe{
		controlCh: make(chan streamPartsControl, 1),
		eventsCh:  make(chan []codersdk.ChatStreamEvent, 128),
		done:      make(chan struct{}),
	}
	return streamPartsChannelServerTransport{pipe: pipe}, streamPartsChannelClientTransport{pipe: pipe}
}

func (t streamPartsChannelServerTransport) ReadControl(ctx context.Context) (streamPartsControl, error) {
	select {
	case <-ctx.Done():
		return streamPartsControl{}, ctx.Err()
	case <-t.pipe.done:
		return streamPartsControl{}, errStreamPartsTransportClosed
	default:
	}
	select {
	case control := <-t.pipe.controlCh:
		return control, nil
	case <-ctx.Done():
		return streamPartsControl{}, ctx.Err()
	case <-t.pipe.done:
		return streamPartsControl{}, errStreamPartsTransportClosed
	}
}

func (t streamPartsChannelServerTransport) WriteEvents(ctx context.Context, events []codersdk.ChatStreamEvent) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.pipe.done:
		return errStreamPartsTransportClosed
	default:
	}
	select {
	case t.pipe.eventsCh <- events:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.pipe.done:
		return errStreamPartsTransportClosed
	}
}

func (t streamPartsChannelServerTransport) Close() error {
	return t.pipe.close()
}

func (t streamPartsChannelClientTransport) WriteControl(ctx context.Context, control streamPartsControl) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.pipe.done:
		return errStreamPartsTransportClosed
	default:
	}
	select {
	case t.pipe.controlCh <- control:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.pipe.done:
		return errStreamPartsTransportClosed
	}
}

func (t streamPartsChannelClientTransport) ReadEvents(ctx context.Context) ([]codersdk.ChatStreamEvent, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.pipe.done:
		return nil, errStreamPartsTransportClosed
	default:
	}
	select {
	case events := <-t.pipe.eventsCh:
		return events, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.pipe.done:
		return nil, errStreamPartsTransportClosed
	}
}

func (t streamPartsChannelClientTransport) Close() error {
	return t.pipe.close()
}

func (p *streamPartsChannelPipe) close() error {
	p.closeOnce.Do(func() {
		close(p.done)
	})
	return nil
}

type streamPartsTransportSession struct {
	ctx       context.Context
	cancel    context.CancelFunc
	transport streamPartsClientTransport
	parts     chan StreamPart
	closeOnce sync.Once
	closeErr  error
}

func newStreamPartsTransportSession(ctx context.Context, transport streamPartsClientTransport) *streamPartsTransportSession {
	sessionCtx, cancel := context.WithCancel(ctx)
	session := &streamPartsTransportSession{
		ctx:       sessionCtx,
		cancel:    cancel,
		transport: transport,
		parts:     make(chan StreamPart, 128),
	}
	go session.readLoop()
	return session
}

func (s *streamPartsTransportSession) SelectEpisode(ctx context.Context, historyVersion, generationAttempt int64) error {
	return s.transport.WriteControl(ctx, streamPartsControl{
		HistoryVersion:    historyVersion,
		GenerationAttempt: generationAttempt,
	})
}

func (s *streamPartsTransportSession) Parts() <-chan StreamPart {
	return s.parts
}

func (s *streamPartsTransportSession) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		s.closeErr = s.transport.Close()
		if streamPartsExpectedTransportClose(s.closeErr) {
			s.closeErr = nil
		}
	})
	return s.closeErr
}

func (s *streamPartsTransportSession) readLoop() {
	defer close(s.parts)
	for {
		batch, err := s.transport.ReadEvents(s.ctx)
		if err != nil {
			return
		}
		for _, event := range batch {
			part, ok := StreamPartFromEvent(event)
			if !ok {
				continue
			}
			select {
			case s.parts <- part:
			case <-s.ctx.Done():
				return
			}
		}
	}
}

type StreamPartsJSONSession struct {
	*streamPartsTransportSession
}

func NewStreamPartsJSONSession(ctx context.Context, conn *websocket.Conn) *StreamPartsJSONSession {
	return &StreamPartsJSONSession{
		streamPartsTransportSession: newStreamPartsTransportSession(ctx, streamPartsWebSocketClientTransport{conn: conn}),
	}
}

func streamPartsExpectedTransportClose(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, errStreamPartsTransportClosed) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, net.ErrClosed) {
		return true
	}
	switch websocket.CloseStatus(err) {
	case websocket.StatusNormalClosure, websocket.StatusGoingAway:
		return true
	default:
		return false
	}
}
