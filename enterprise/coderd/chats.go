package coderd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// RelaySourceHeader marks replica-relayed stream requests.
const RelaySourceHeader = "X-Coder-Relay-Source-Replica"

const (
	authorizationHeader = "Authorization"
	cookieHeader        = "Cookie"
)

type relayEvent struct {
	Type codersdk.ServerSentEventType `json:"type"`
	Data json.RawMessage              `json:"data,omitempty"`
}

// newRemotePartsProvider creates a RemotePartsProvider that dials a remote
// replica's stream endpoint to fetch message_part events. It filters to only
// forward message_part events since durable events come via pubsub.
func newRemotePartsProvider(
	resolveReplicaAddress func(context.Context, uuid.UUID) (string, bool),
	replicaHTTPClient *http.Client,
	replicaID uuid.UUID,
) chatd.RemotePartsProvider {
	return func(
		ctx context.Context,
		chatID uuid.UUID,
		workerID uuid.UUID,
		requestHeader http.Header,
	) (
		[]codersdk.ChatStreamEvent,
		<-chan codersdk.ChatStreamEvent,
		func(),
		error,
	) {
		address, ok := resolveReplicaAddress(ctx, workerID)
		if !ok {
			return nil, nil, nil, xerrors.New("worker replica not found")
		}

		base, err := url.Parse(address)
		if err != nil {
			return nil, nil, nil, xerrors.Errorf("parse relay address %q: %w", address, err)
		}
		target, err := base.Parse(fmt.Sprintf("/api/v2/chats/%s/stream", chatID))
		if err != nil {
			return nil, nil, nil, xerrors.Errorf("build relay stream URL: %w", err)
		}
		switch target.Scheme {
		case "http":
			target.Scheme = "ws"
		case "https":
			target.Scheme = "wss"
		}

		conn, res, err := websocket.Dial(ctx, target.String(), &websocket.DialOptions{
			HTTPClient: replicaHTTPClient,
			HTTPHeader: relayHeaders(requestHeader, replicaID),
		})
		if err != nil {
			if res != nil {
				defer res.Body.Close()
				if responseErr := codersdk.ReadBodyAsError(res); responseErr != nil {
					err = responseErr
				}
			}
			return nil, nil, nil, xerrors.Errorf("dial relay stream: %w", err)
		}

		relayCtx, relayCancel := context.WithCancel(ctx)
		events := make(chan codersdk.ChatStreamEvent, 128)
		snapshot := make([]codersdk.ChatStreamEvent, 0)

		go func() {
			defer close(events)
			defer relayCancel()
			defer func() {
				_ = conn.Close(websocket.StatusNormalClosure, "")
			}()

			for {
				var envelope relayEvent
				err := wsjson.Read(relayCtx, conn, &envelope)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					switch websocket.CloseStatus(err) {
					case websocket.StatusNormalClosure, websocket.StatusGoingAway:
						return
					}
					select {
					case events <- relayError(chatID, xerrors.Errorf("read relay stream: %w", err)):
					case <-relayCtx.Done():
					}
					return
				}

				switch envelope.Type {
				case codersdk.ServerSentEventTypePing:
					continue
				case codersdk.ServerSentEventTypeData:
					var event codersdk.ChatStreamEvent
					if err := json.Unmarshal(envelope.Data, &event); err != nil {
						select {
						case events <- relayError(chatID, xerrors.Errorf("decode relay data event: %w", err)):
						case <-relayCtx.Done():
						}
						return
					}
					if event.ChatID == uuid.Nil {
						event.ChatID = chatID
					}
					// Only forward message_part events (durable events come via pubsub)
					if event.Type == codersdk.ChatStreamEventTypeMessagePart {
						// First events go to snapshot, then live stream
						if len(snapshot) < 100 {
							snapshot = append(snapshot, event)
						}
						select {
						case events <- event:
						case <-relayCtx.Done():
							return
						}
					}
				case codersdk.ServerSentEventTypeError:
					msg := "relay stream returned an error"
					if len(envelope.Data) > 0 {
						var response codersdk.Response
						if err := json.Unmarshal(envelope.Data, &response); err == nil {
							msg = formatRelayError(response)
						} else {
							msg = strings.TrimSpace(string(envelope.Data))
						}
					}
					select {
					case events <- relayError(chatID, xerrors.New(msg)):
					case <-relayCtx.Done():
					}
					return
				default:
					select {
					case events <- relayError(chatID, xerrors.Errorf("unknown relay event type %q", envelope.Type)):
					case <-relayCtx.Done():
					}
					return
				}
			}
		}()

		cancel := func() {
			relayCancel()
			_ = conn.Close(websocket.StatusNormalClosure, "")
		}
		return snapshot, events, cancel, nil
	}
}

func relayHeaders(source http.Header, replicaID uuid.UUID) http.Header {
	header := make(http.Header)
	if source != nil {
		for _, key := range []string{codersdk.SessionTokenHeader, authorizationHeader, cookieHeader} {
			for _, value := range source.Values(key) {
				header.Add(key, value)
			}
		}
	}
	header.Set(RelaySourceHeader, replicaID.String())
	return header
}

func relayError(chatID uuid.UUID, err error) codersdk.ChatStreamEvent {
	return codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeError,
		ChatID: chatID,
		Error: &codersdk.ChatStreamError{
			Message: err.Error(),
		},
	}
}

func formatRelayError(response codersdk.Response) string {
	message := strings.TrimSpace(response.Message)
	detail := strings.TrimSpace(response.Detail)
	switch {
	case message == "" && detail == "":
		return "relay stream returned an error"
	case message == "":
		return detail
	case detail == "":
		return message
	default:
		return fmt.Sprintf("%s: %s", message, detail)
	}
}
