package agentsdk

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"

	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpc"
)

func (c *Client) RPC(ctx context.Context) (agentproto.DRPCAgentClient, error) {
	rpcURL, err := c.SDK.URL.Parse("/api/v2/workspaceagents/me/rpc")
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, xerrors.Errorf("create cookie jar: %w", err)
	}
	jar.SetCookies(rpcURL, []*http.Cookie{{
		Name:  codersdk.SessionTokenCookie,
		Value: c.SDK.SessionToken(),
	}})
	httpClient := &http.Client{
		Jar:       jar,
		Transport: c.SDK.HTTPClient.Transport,
	}
	// nolint:bodyclose
	conn, res, err := websocket.Dial(ctx, rpcURL.String(), &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, codersdk.ReadBodyAsError(res)
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageBinary)
	pingClosed := pingWebSocket(ctx, c.SDK.Logger(), conn, "RPC")

	nconn := &closeNetConn{
		Conn: wsNetConn,
		closeFunc: func() {
			cancelFunc()
			_ = conn.Close(websocket.StatusGoingAway, "Listen closed")
			<-pingClosed
		},
	}

	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	mux, err := yamux.Client(nconn, config)
	if err != nil {
		return nil, xerrors.Errorf("create yamux client: %w", err)
	}

	dconn := drpc.MultiplexedConn(mux)
	return agentproto.NewDRPCAgentClient(dconn), nil
}
