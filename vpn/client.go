package vpn

import (
	"context"
	"net/http"
	"net/netip"
	"net/url"

	"golang.org/x/xerrors"
	"tailscale.com/net/dns"
	"tailscale.com/net/netmon"
	"tailscale.com/wgengine/router"

	"github.com/google/uuid"
	"github.com/tailscale/wireguard-go/tun"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

type Conn interface {
	CurrentWorkspaceState() (tailnet.WorkspaceUpdate, error)
	GetPeerDiagnostics(peerID uuid.UUID) tailnet.PeerDiagnostics
	Close() error
}

type vpnConn struct {
	*tailnet.Conn

	cancelFn    func()
	controller  *tailnet.Controller
	updatesCtrl *tailnet.TunnelAllWorkspaceUpdatesController
}

func (c *vpnConn) CurrentWorkspaceState() (tailnet.WorkspaceUpdate, error) {
	return c.updatesCtrl.CurrentState()
}

func (c *vpnConn) Close() error {
	c.cancelFn()
	<-c.controller.Closed()
	return c.Conn.Close()
}

type client struct{}

type Client interface {
	NewConn(ctx context.Context, serverURL *url.URL, token string, options *Options) (Conn, error)
}

func NewClient() Client {
	return &client{}
}

type Options struct {
	Headers          http.Header
	Logger           slog.Logger
	DNSConfigurator  dns.OSConfigurator
	Router           router.Router
	TUNDevice        tun.Device
	WireguardMonitor *netmon.Monitor
	UpdateHandler    tailnet.UpdatesHandler
}

func (*client) NewConn(initCtx context.Context, serverURL *url.URL, token string, options *Options) (vpnC Conn, err error) {
	if options == nil {
		options = &Options{}
	}

	if options.Headers == nil {
		options.Headers = http.Header{}
	}

	headers := options.Headers
	sdk := codersdk.New(serverURL)
	sdk.SetSessionToken(token)
	sdk.HTTPClient.Transport = &codersdk.HeaderTransport{
		Transport: http.DefaultTransport,
		Header:    headers,
	}

	// New context, separate from initCtx. We don't want to cancel the
	// connection if initCtx is canceled.
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	rpcURL, err := sdk.URL.Parse("/api/v2/tailnet")
	if err != nil {
		return nil, xerrors.Errorf("parse rpc url: %w", err)
	}

	me, err := sdk.User(initCtx, codersdk.Me)
	if err != nil {
		return nil, xerrors.Errorf("get user: %w", err)
	}

	connInfo, err := workspacesdk.New(sdk).AgentConnectionInfoGeneric(initCtx)
	if err != nil {
		return nil, xerrors.Errorf("get connection info: %w", err)
	}
	// default to DNS suffix of "coder" if the server hasn't set it (might be too old).
	dnsNameOptions := tailnet.DNSNameOptions{Suffix: "coder"}
	if connInfo.HostnameSuffix != "" {
		dnsNameOptions.Suffix = connInfo.HostnameSuffix
	}

	headers.Set(codersdk.SessionTokenHeader, token)
	dialer := workspacesdk.NewWebsocketDialer(options.Logger, rpcURL, &websocket.DialOptions{
		HTTPClient:      sdk.HTTPClient,
		HTTPHeader:      headers,
		CompressionMode: websocket.CompressionDisabled,
	}, workspacesdk.WithWorkspaceUpdates(&proto.WorkspaceUpdatesRequest{
		WorkspaceOwnerId: tailnet.UUIDToByteSlice(me.ID),
	}))

	ip := tailnet.CoderServicePrefix.RandomAddr()
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:           []netip.Prefix{netip.PrefixFrom(ip, 128)},
		DERPMap:             connInfo.DERPMap,
		DERPHeader:          &headers,
		DERPForceWebSockets: connInfo.DERPForceWebSockets,
		Logger:              options.Logger,
		BlockEndpoints:      connInfo.DisableDirectConnections,
		DNSConfigurator:     options.DNSConfigurator,
		Router:              options.Router,
		TUNDev:              options.TUNDevice,
		WireguardMonitor:    options.WireguardMonitor,
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet: %w", err)
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()

	clk := quartz.NewReal()
	controller := tailnet.NewController(options.Logger, dialer)
	coordCtrl := tailnet.NewTunnelSrcCoordController(options.Logger, conn)
	controller.ResumeTokenCtrl = tailnet.NewBasicResumeTokenController(options.Logger, clk)
	controller.CoordCtrl = coordCtrl
	controller.DERPCtrl = tailnet.NewBasicDERPController(options.Logger, conn)
	updatesCtrl := tailnet.NewTunnelAllWorkspaceUpdatesController(
		options.Logger,
		coordCtrl,
		tailnet.WithDNS(conn, me.Username, dnsNameOptions),
		tailnet.WithHandler(options.UpdateHandler),
	)
	controller.WorkspaceUpdatesCtrl = updatesCtrl
	controller.Run(ctx)

	options.Logger.Debug(ctx, "running tailnet API v2+ connector")

	select {
	case <-initCtx.Done():
		return nil, xerrors.Errorf("timed out waiting for coordinator and derp map: %w", initCtx.Err())
	case err = <-dialer.Connected():
		if err != nil {
			options.Logger.Error(ctx, "failed to connect to tailnet v2+ API", slog.Error(err))
			return nil, xerrors.Errorf("start connector: %w", err)
		}
		options.Logger.Debug(ctx, "connected to tailnet v2+ API")
	}

	return &vpnConn{
		Conn:        conn,
		cancelFn:    cancel,
		controller:  controller,
		updatesCtrl: updatesCtrl,
	}, nil
}
