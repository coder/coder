package workspacesdk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/websocket"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
)

var permanentErrorStatuses = []int{
	http.StatusConflict,            // returned if client/agent connections disabled (browser only)
	http.StatusBadRequest,          // returned if API mismatch
	http.StatusNotFound,            // returned if user doesn't have permission or agent doesn't exist
	http.StatusInternalServerError, // returned if database is not reachable,
}

type WebsocketDialer struct {
	logger      slog.Logger
	dialOptions *websocket.DialOptions
	url         *url.URL
	// workspaceUpdatesReq != nil means that the dialer should call the WorkspaceUpdates RPC and
	// return the corresponding client
	workspaceUpdatesReq *proto.WorkspaceUpdatesRequest

	resumeTokenFailed bool
	connected         chan error
	isFirst           bool
}

type WebsocketDialerOption func(*WebsocketDialer)

func WithWorkspaceUpdates(req *proto.WorkspaceUpdatesRequest) WebsocketDialerOption {
	return func(w *WebsocketDialer) {
		w.workspaceUpdatesReq = req
	}
}

func (w *WebsocketDialer) Dial(ctx context.Context, r tailnet.ResumeTokenController,
) (
	tailnet.ControlProtocolClients, error,
) {
	w.logger.Debug(ctx, "dialing Coder tailnet v2+ API")

	u := new(url.URL)
	*u = *w.url
	q := u.Query()
	if r != nil && !w.resumeTokenFailed {
		if token, ok := r.Token(); ok {
			q.Set("resume_token", token)
			w.logger.Debug(ctx, "using resume token on dial")
		}
	}
	// The current version includes additions
	//
	// 2.1 GetAnnouncementBanners on the Agent API (version locked to Tailnet API)
	// 2.2 PostTelemetry on the Tailnet API
	// 2.3 RefreshResumeToken, WorkspaceUpdates
	//
	// Resume tokens and telemetry are optional, and fail gracefully.  So we use version 2.0 for
	// maximum compatibility if we don't need WorkspaceUpdates. If we do, we use 2.3.
	if w.workspaceUpdatesReq != nil {
		q.Add("version", "2.3")
	} else {
		q.Add("version", "2.0")
	}
	u.RawQuery = q.Encode()

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

				if sdkErr.Message == codersdk.DatabaseNotReachable &&
					sdkErr.StatusCode() == http.StatusInternalServerError {
					err = xerrors.Errorf("%w: %v", codersdk.ErrDatabaseNotReachable, err)
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

	var updates tailnet.WorkspaceUpdatesClient
	if w.workspaceUpdatesReq != nil {
		updates, err = client.WorkspaceUpdates(context.Background(), w.workspaceUpdatesReq)
		if err != nil {
			w.logger.Debug(ctx, "failed to create WorkspaceUpdates stream", slog.Error(err))
			_ = ws.Close(websocket.StatusInternalError, "")
			return tailnet.ControlProtocolClients{}, err
		}
	}

	return tailnet.ControlProtocolClients{
		Closer:           client.DRPCConn(),
		Coordinator:      coord,
		DERP:             derps,
		ResumeToken:      client,
		Telemetry:        client,
		WorkspaceUpdates: updates,
	}, nil
}

func (w *WebsocketDialer) Connected() <-chan error {
	return w.connected
}

func NewWebsocketDialer(
	logger slog.Logger, u *url.URL, websocketOptions *websocket.DialOptions,
	dialerOptions ...WebsocketDialerOption,
) *WebsocketDialer {
	w := &WebsocketDialer{
		logger:      logger,
		dialOptions: websocketOptions,
		url:         u,
		connected:   make(chan error, 1),
		isFirst:     true,
	}
	for _, o := range dialerOptions {
		o(w)
	}
	return w
}
