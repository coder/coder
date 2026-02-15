package workspacesdk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/websocket"
)

var permanentErrorStatuses = []int{
	http.StatusConflict,            // returned if client/agent connections disabled (browser only)
	http.StatusBadRequest,          // returned if API mismatch
	http.StatusNotFound,            // returned if user doesn't have permission or agent doesn't exist
	http.StatusInternalServerError, // returned if database is not reachable,
	http.StatusForbidden,           // returned if user is not authorized
	// StatusUnauthorized is only a permanent error if the error is not due to
	// an invalid resume token. See `checkResumeTokenFailure`.
	http.StatusUnauthorized,
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

	// onConnectAuthRequired is called when the server returns 403
	// with a connect-auth requirement. If set, the dialer retries
	// with the returned proof header.
	onConnectAuthRequired func() (string, error)
	connectProof          string
}

// checkResumeTokenFailure checks if the parsed error indicates a resume token failure
// and updates the resumeTokenFailed flag accordingly. Returns true if a resume token
// failure was detected.
func (w *WebsocketDialer) checkResumeTokenFailure(ctx context.Context, sdkErr *codersdk.Error) bool {
	if sdkErr == nil {
		return false
	}

	for _, v := range sdkErr.Validations {
		if v.Field == "resume_token" {
			w.logger.Warn(ctx, "failed to dial tailnet v2+ API: server replied invalid resume token; unsetting for next connection attempt")
			w.resumeTokenFailed = true
			return true
		}
	}
	return false
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

	// If we have a connect-auth proof from a previous attempt, add
	// it to the dial options.
	dialOpts := w.dialOptions
	if w.connectProof != "" {
		dialOpts = cloneDialOptions(w.dialOptions)
		if dialOpts.HTTPHeader == nil {
			dialOpts.HTTPHeader = make(http.Header)
		}
		dialOpts.HTTPHeader.Set(codersdk.ConnectProofHeader, w.connectProof)
	}

	// nolint:bodyclose
	ws, res, err := websocket.Dial(ctx, u.String(), dialOpts)

	// Check for connect-auth requirement on every dial attempt
	// (not just the first). This ensures reconnects after the
	// 30-second proof expiry obtain a fresh Touch ID proof
	// rather than reusing a stale one.
	if res != nil && isConnectAuthResponse(res) && w.onConnectAuthRequired != nil {
		res.Body.Close()
		// Clear the stale proof so the next attempt starts fresh.
		w.connectProof = ""
		proof, cbErr := w.onConnectAuthRequired()
		if cbErr != nil {
			if w.isFirst {
				w.connected <- cbErr
			}
			return tailnet.ControlProtocolClients{}, cbErr
		}
		if proof != "" {
			w.connectProof = proof
			return tailnet.ControlProtocolClients{}, xerrors.New("connect-auth: retrying with proof")
		}
	}

	if w.isFirst {
		if res != nil && slices.Contains(permanentErrorStatuses, res.StatusCode) {
			err = codersdk.ReadBodyAsError(res)
			var sdkErr *codersdk.Error
			if xerrors.As(err, &sdkErr) {
				// Check for resume token failure first
				if w.checkResumeTokenFailure(ctx, sdkErr) {
					return tailnet.ControlProtocolClients{}, err
				}

				// A bit more human-readable help in the case the API version was rejected
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
			if w.checkResumeTokenFailure(ctx, sdkErr) {
				return tailnet.ControlProtocolClients{}, err
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

// isConnectAuthResponse checks whether an HTTP response indicates
// that the server requires connect-auth (Secure Enclave proof).
// Uses the Coder-Connect-Auth-Required header for reliable
// detection rather than parsing error message strings.
func isConnectAuthResponse(res *http.Response) bool {
	if res == nil {
		return false
	}
	return res.StatusCode == http.StatusForbidden &&
		res.Header.Get(codersdk.ConnectAuthRequiredHeader) == "true"
}

// cloneDialOptions creates a copy of the dial options so we can
// add headers without mutating the original. The HTTPHeader map
// is deep-copied to avoid sharing state.
func cloneDialOptions(opts *websocket.DialOptions) *websocket.DialOptions {
	if opts == nil {
		return &websocket.DialOptions{}
	}
	clone := *opts
	if opts.HTTPHeader != nil {
		clone.HTTPHeader = opts.HTTPHeader.Clone()
	}
	return &clone
}
