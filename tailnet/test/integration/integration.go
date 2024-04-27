package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

func NetworkSetupDefault(*testing.T) {}

func DERPMapTailscale(ctx context.Context, t *testing.T) *tailcfg.DERPMap {
	ctx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://controlplane.tailscale.com/derpmap/default", nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	dm := &tailcfg.DERPMap{}
	dec := json.NewDecoder(res.Body)
	err = dec.Decode(dm)
	require.NoError(t, err)

	return dm
}

func CoordinatorInMemory(t *testing.T, logger slog.Logger, dm *tailcfg.DERPMap) (coord tailnet.Coordinator, url string) {
	coord = tailnet.NewCoordinator(logger)
	var coordPtr atomic.Pointer[tailnet.Coordinator]
	coordPtr.Store(&coord)
	t.Cleanup(func() { _ = coord.Close() })

	csvc, err := tailnet.NewClientService(logger, &coordPtr, 10*time.Minute, func() *tailcfg.DERPMap {
		return dm
	})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idStr := strings.TrimPrefix(r.URL.Path, "/")
		id, err := uuid.Parse(idStr)
		if err != nil {
			httpapi.Write(r.Context(), w, http.StatusBadRequest, codersdk.Response{
				Message: "Bad agent id.",
				Detail:  err.Error(),
			})
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			httpapi.Write(r.Context(), w, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to accept websocket.",
				Detail:  err.Error(),
			})
			return
		}

		ctx, wsNetConn := codersdk.WebsocketNetConn(r.Context(), conn, websocket.MessageBinary)
		defer wsNetConn.Close()

		err = csvc.ServeConnV2(ctx, wsNetConn, tailnet.StreamID{
			Name: "client-" + id.String(),
			ID:   id,
			Auth: tailnet.SingleTailnetCoordinateeAuth{},
		})
		if err != nil && !xerrors.Is(err, io.EOF) && !xerrors.Is(err, context.Canceled) {
			_ = conn.Close(websocket.StatusInternalError, err.Error())
			return
		}
	}))
	t.Cleanup(srv.Close)

	return coord, srv.URL
}

func TailnetSetupDRPC(ctx context.Context, t *testing.T, logger slog.Logger,
	id, agentID uuid.UUID,
	coordinateURL string,
	dm *tailcfg.DERPMap,
) *tailnet.Conn {
	ip := tailnet.IPFromUUID(id)
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(ip, 128)},
		DERPMap:   dm,
		Logger:    logger,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	//nolint:bodyclose
	ws, _, err := websocket.Dial(ctx, coordinateURL+"/"+id.String(), nil)
	require.NoError(t, err)

	client, err := tailnet.NewDRPCClient(
		websocket.NetConn(ctx, ws, websocket.MessageBinary),
		logger,
	)
	require.NoError(t, err)

	coord, err := client.Coordinate(ctx)
	require.NoError(t, err)

	coordination := tailnet.NewRemoteCoordination(logger, coord, conn, agentID)
	t.Cleanup(func() { _ = coordination.Close() })
	return conn
}
