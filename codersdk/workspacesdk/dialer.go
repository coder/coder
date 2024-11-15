package workspacesdk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
)

var permanentErrorStatuses = []int{
	http.StatusConflict,   // returned if client/agent connections disabled (browser only)
	http.StatusBadRequest, // returned if API mismatch
	http.StatusNotFound,   // returned if user doesn't have permission or agent doesn't exist
}

type WebsocketDialer struct {
	logger            slog.Logger
	dialOptions       *websocket.DialOptions
	url               *url.URL
	resumeTokenFailed bool
	connected         chan error
	isFirst           bool
}

func (w *WebsocketDialer) Dial(ctx context.Context, r tailnet.ResumeTokenController,
) (
	tailnet.ControlProtocolClients, error,
) {
	w.logger.Debug(ctx, "dialing Coder tailnet v2+ API")

	u := new(url.URL)
	*u = *w.url
	if r != nil && !w.resumeTokenFailed {
		if token, ok := r.Token(); ok {
			q := u.Query()
			q.Set("resume_token", token)
			u.RawQuery = q.Encode()
			w.logger.Debug(ctx, "using resume token on dial")
		}
	}

	// nolint:bodyclose
	ws, res, err := websocket.Dial(ctx, u.String(), w.dialOptions)
	if w.isFirst {
		if res != nil && slices.Contains(permanentErrorStatuses, res.StatusCode) {
			err = codersdk.ReadBodyAsError(res)
			// A bit more human-readable help in the case the API version was rejected
			var sdkErr *codersdk.Error
			if xerrors.As(err, &sdkErr) {
				if sdkErr.Message == AgentAPIMismatchMessage &&
					sdkErr.StatusCode() == http.StatusBadRequest {
					sdkErr.Helper = fmt.Sprintf(
						"Ensure your client release version (%s, different than the API version) matches the server release version",
						buildinfo.Version())
				}
			}
			w.connected <- err
			return tailnet.ControlProtocolClients{}, err
		}
		w.isFirst = false
		close(w.connected)
	}
	if err != nil {
		bodyErr := codersdk.ReadBodyAsError(res)
		var sdkErr *codersdk.Error
		if xerrors.As(bodyErr, &sdkErr) {
			for _, v := range sdkErr.Validations {
				if v.Field == "resume_token" {
					// Unset the resume token for the next attempt
					w.logger.Warn(ctx, "failed to dial tailnet v2+ API: server replied invalid resume token; unsetting for next connection attempt")
					w.resumeTokenFailed = true
					return tailnet.ControlProtocolClients{}, err
				}
			}
		}
		if !errors.Is(err, context.Canceled) {
			w.logger.Error(ctx, "failed to dial tailnet v2+ API", slog.Error(err), slog.F("sdk_err", sdkErr))
		}
		return tailnet.ControlProtocolClients{}, err
	}
	w.resumeTokenFailed = false

	client, err := tailnet.NewDRPCClient(
		websocket.NetConn(context.Background(), ws, websocket.MessageBinary),
		w.logger,
	)
	if err != nil {
		w.logger.Debug(ctx, "failed to create DRPCClient", slog.Error(err))
		_ = ws.Close(websocket.StatusInternalError, "")
		return tailnet.ControlProtocolClients{}, err
	}
	coord, err := client.Coordinate(context.Background())
	if err != nil {
		w.logger.Debug(ctx, "failed to create Coordinate RPC", slog.Error(err))
		_ = ws.Close(websocket.StatusInternalError, "")
		return tailnet.ControlProtocolClients{}, err
	}

	derps := &tailnet.DERPFromDRPCWrapper{}
	derps.Client, err = client.StreamDERPMaps(context.Background(), &proto.StreamDERPMapsRequest{})
	if err != nil {
		w.logger.Debug(ctx, "failed to create DERPMap stream", slog.Error(err))
		_ = ws.Close(websocket.StatusInternalError, "")
		return tailnet.ControlProtocolClients{}, err
	}

	return tailnet.ControlProtocolClients{
		Closer:      client.DRPCConn(),
		Coordinator: coord,
		DERP:        derps,
		ResumeToken: client,
		Telemetry:   client,
	}, nil
}

func (w *WebsocketDialer) Connected() <-chan error {
	return w.connected
}

func NewWebsocketDialer(logger slog.Logger, u *url.URL, opts *websocket.DialOptions) *WebsocketDialer {
	return &WebsocketDialer{
		logger:      logger,
		dialOptions: opts,
		url:         u,
		connected:   make(chan error, 1),
		isFirst:     true,
	}
}
