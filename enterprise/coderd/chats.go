package coderd

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

// RelaySourceHeader marks replica-relayed stream requests.
const RelaySourceHeader = "X-Coder-Relay-Source-Replica"

const (
	authorizationHeader = "Authorization"
	cookieHeader        = "Cookie"
)

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

		baseURL, err := url.Parse(address)
		if err != nil {
			return nil, nil, nil, xerrors.Errorf("parse relay address %q: %w", address, err)
		}
		relayCtx, relayCancel := context.WithCancel(ctx)
		sdkClient := codersdk.New(baseURL)
		sdkClient.HTTPClient = replicaHTTPClient
		sdkClient.SessionTokenProvider = relayHeaderTokenProvider{
			header: relayHeaders(requestHeader, replicaID),
		}
		sourceEvents, sourceStream, err := sdkClient.StreamChat(relayCtx, chatID)
		if err != nil {
			relayCancel()
			return nil, nil, nil, xerrors.Errorf("dial relay stream: %w", err)
		}

		snapshot := make([]codersdk.ChatStreamEvent, 0, 100)

		// Wait briefly for the first event to handle the common
		// case where the remote side has buffered parts but hasn't
		// flushed them to the WebSocket yet.
		const drainTimeout = time.Second
		drainTimer := time.NewTimer(drainTimeout)
		defer drainTimer.Stop()

	drainInitial:
		for len(snapshot) < cap(snapshot) {
			select {
			case <-relayCtx.Done():
				_ = sourceStream.Close()
				relayCancel()
				return nil, nil, nil, xerrors.Errorf("dial relay stream: %w", relayCtx.Err())
			case event, ok := <-sourceEvents:
				if !ok {
					break drainInitial
				}
				if event.Type != codersdk.ChatStreamEventTypeMessagePart {
					continue
				}
				snapshot = append(snapshot, event)
				// After getting the first event, switch to
				// non-blocking drain for remaining buffered events.
				drainTimer.Stop()
				drainTimer.Reset(0)
			case <-drainTimer.C:
				break drainInitial
			}
		}

		events := make(chan codersdk.ChatStreamEvent, 128)

		go func() {
			defer close(events)
			defer relayCancel()
			defer func() {
				_ = sourceStream.Close()
			}()

			// No need to re-send snapshot events — they're
			// returned to the caller directly.
			for {
				select {
				case <-relayCtx.Done():
					return
				case event, ok := <-sourceEvents:
					if !ok {
						return
					}
					if event.Type != codersdk.ChatStreamEventTypeMessagePart {
						continue
					}
					select {
					case events <- event:
					case <-relayCtx.Done():
						return
					}
				}
			}
		}()

		cancel := func() {
			relayCancel()
			_ = sourceStream.Close()
		}
		return snapshot, events, cancel, nil
	}
}

type relayHeaderTokenProvider struct {
	header http.Header
}

func (p relayHeaderTokenProvider) AsRequestOption() codersdk.RequestOption {
	return func(req *http.Request) {
		for key, values := range p.header {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	}
}

func (p relayHeaderTokenProvider) SetDialOption(opts *websocket.DialOptions) {
	if opts.HTTPHeader == nil {
		opts.HTTPHeader = make(http.Header)
	}
	for key, values := range p.header {
		for _, value := range values {
			opts.HTTPHeader.Add(key, value)
		}
	}
}

func (p relayHeaderTokenProvider) GetSessionToken() string {
	return p.header.Get(codersdk.SessionTokenHeader)
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
