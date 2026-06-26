package chatd

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

type streamPartsControl struct {
	HistoryVersion    int64 `json:"history_version"`
	GenerationAttempt int64 `json:"generation_attempt"`
}

type streamPartsEndpoint struct {
	chatID uuid.UUID
	buffer *messagepartbuffer.Buffer
	logger slog.Logger
}

// ServeStreamPartsAuthorized serves the internal episode-selected parts stream
// for an already authorized chat route.
func (p *Server) ServeStreamPartsAuthorized(rw http.ResponseWriter, r *http.Request, chat database.Chat) error {
	if p == nil || p.messagePartBuffer == nil {
		return xerrors.New("message part buffer is not configured")
	}
	endpoint := streamPartsEndpoint{
		chatID: chat.ID,
		buffer: p.messagePartBuffer,
		logger: p.logger.Named("chat_stream_parts").With(slog.F("chat_id", chat.ID)),
	}
	return endpoint.serveWebSocket(rw, r)
}

func (e streamPartsEndpoint) serveWebSocket(rw http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		return xerrors.Errorf("accept parts websocket: %w", err)
	}
	transport := streamPartsWebSocketServerTransport{conn: conn}
	defer func() {
		_ = transport.Close()
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go httpapi.HeartbeatClose(ctx, e.logger, cancel, conn)

	return e.serve(ctx, transport)
}

func (e streamPartsEndpoint) serve(ctx context.Context, transport streamPartsServerTransport) error {
	if e.buffer == nil {
		return xerrors.New("message part buffer is not configured")
	}
	if transport == nil {
		return xerrors.New("stream parts transport is not configured")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	controlCh := make(chan streamPartsControl, 1)
	errCh := make(chan error, 1)
	go func() {
		for {
			control, err := transport.ReadControl(ctx)
			if err != nil {
				select {
				case errCh <- err:
				case <-ctx.Done():
				}
				return
			}
			select {
			case controlCh <- control:
			case <-ctx.Done():
				return
			}
		}
	}()

	var (
		parts        <-chan messagepartbuffer.Part
		partCancel   func()
		partCancelFn context.CancelFunc
		selected     streamPartsControl
		lastSeq      int64
	)
	defer func() {
		if partCancel != nil {
			partCancel()
		}
		if partCancelFn != nil {
			partCancelFn()
		}
	}()

	selectEpisode := func(control streamPartsControl) error {
		if partCancel != nil {
			partCancel()
			partCancel = nil
		}
		if partCancelFn != nil {
			partCancelFn()
			partCancelFn = nil
		}
		parts = nil
		selected = control
		lastSeq = 0
		partCtx, cancel := context.WithCancel(ctx)
		ch, cancelSub, err := e.buffer.SubscribeToEpisode(partCtx, messagepartbuffer.Key{
			ChatID:            e.chatID,
			HistoryVersion:    control.HistoryVersion,
			GenerationAttempt: control.GenerationAttempt,
		})
		if err != nil {
			cancel()
			return err
		}
		partCancelFn = cancel
		partCancel = cancelSub
		parts = ch
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			if ctx.Err() != nil || streamPartsExpectedTransportClose(err) {
				return nil
			}
			return err
		case control := <-controlCh:
			if err := selectEpisode(control); err != nil {
				return err
			}
		case part, ok := <-parts:
			if !ok {
				parts = nil
				continue
			}
			if part.Seq != lastSeq+1 {
				return xerrors.Errorf("message part sequence gap: got %d after %d", part.Seq, lastSeq)
			}
			lastSeq = part.Seq
			event := codersdk.ChatStreamEvent{
				Type:   codersdk.ChatStreamEventTypeMessagePart,
				ChatID: e.chatID,
				MessagePart: &codersdk.ChatStreamMessagePart{
					Role:              part.Role,
					Part:              part.MessagePart,
					HistoryVersion:    selected.HistoryVersion,
					GenerationAttempt: selected.GenerationAttempt,
					Seq:               part.Seq,
				},
			}
			if err := transport.WriteEvents(ctx, []codersdk.ChatStreamEvent{event}); err != nil {
				if ctx.Err() != nil || streamPartsExpectedTransportClose(err) {
					return nil
				}
				return err
			}
		}
	}
}

func StreamPartFromEvent(event codersdk.ChatStreamEvent) (StreamPart, bool) {
	if event.Type != codersdk.ChatStreamEventTypeMessagePart || event.MessagePart == nil {
		return StreamPart{}, false
	}
	return StreamPart{
		HistoryVersion:    event.MessagePart.HistoryVersion,
		GenerationAttempt: event.MessagePart.GenerationAttempt,
		Seq:               event.MessagePart.Seq,
		Role:              event.MessagePart.Role,
		Part:              event.MessagePart.Part,
	}, true
}
