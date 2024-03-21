package tailnet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
	agpl "github.com/coder/coder/v2/tailnet"
)

type ClientService struct {
	*agpl.ClientService
}

// NewClientService returns a ClientService based on the given Coordinator pointer.  The pointer is
// loaded on each processed connection.
func NewClientService(
	logger slog.Logger,
	coordPtr *atomic.Pointer[agpl.Coordinator],
	derpMapUpdateFrequency time.Duration,
	derpMapFn func() *tailcfg.DERPMap,
) (
	*ClientService, error,
) {
	s, err := agpl.NewClientService(logger, coordPtr, derpMapUpdateFrequency, derpMapFn)
	if err != nil {
		return nil, err
	}
	return &ClientService{ClientService: s}, nil
}

func (s *ClientService) ServeMultiAgentClient(ctx context.Context, version string, conn net.Conn, id uuid.UUID) error {
	major, _, err := apiversion.Parse(version)
	if err != nil {
		s.Logger.Warn(ctx, "serve client called with unparsable version", slog.Error(err))
		return err
	}
	switch major {
	case 1:
		coord := *(s.CoordPtr.Load())
		sub := coord.ServeMultiAgent(id)
		return ServeWorkspaceProxy(ctx, conn, sub)
	case 2:
		auth := agpl.SingleTailnetCoordinateeAuth{}
		streamID := agpl.StreamID{
			Name: id.String(),
			ID:   id,
			Auth: auth,
		}
		return s.ServeConnV2(ctx, conn, streamID)
	default:
		s.Logger.Warn(ctx, "serve client called with unsupported version", slog.F("version", version))
		return xerrors.New("unsupported version")
	}
}

func ServeWorkspaceProxy(ctx context.Context, conn net.Conn, ma agpl.MultiAgentConn) error {
	go func() {
		err := forwardNodesToWorkspaceProxy(ctx, conn, ma)
		//nolint:staticcheck
		if err != nil {
			_ = conn.Close()
		}
	}()

	decoder := json.NewDecoder(conn)
	for {
		var msg wsproxysdk.CoordinateMessage
		err := decoder.Decode(&msg)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return xerrors.Errorf("read json: %w", err)
		}

		switch msg.Type {
		case wsproxysdk.CoordinateMessageTypeSubscribe:
			err := ma.SubscribeAgent(msg.AgentID)
			if err != nil {
				return xerrors.Errorf("subscribe agent: %w", err)
			}
		case wsproxysdk.CoordinateMessageTypeUnsubscribe:
			err := ma.UnsubscribeAgent(msg.AgentID)
			if err != nil {
				return xerrors.Errorf("unsubscribe agent: %w", err)
			}
		case wsproxysdk.CoordinateMessageTypeNodeUpdate:
			pn, err := agpl.NodeToProto(msg.Node)
			if err != nil {
				return err
			}
			err = ma.UpdateSelf(pn)
			if err != nil {
				return xerrors.Errorf("update self: %w", err)
			}

		default:
			return xerrors.Errorf("unknown message type %q", msg.Type)
		}
	}
}

func forwardNodesToWorkspaceProxy(ctx context.Context, conn net.Conn, ma agpl.MultiAgentConn) error {
	var lastData []byte
	for {
		resp, ok := ma.NextUpdate(ctx)
		if !ok {
			return xerrors.New("multiagent is closed")
		}
		nodes, err := agpl.OnlyNodeUpdates(resp)
		if err != nil {
			return xerrors.Errorf("failed to convert response: %w", err)
		}
		data, err := json.Marshal(wsproxysdk.CoordinateNodes{Nodes: nodes})
		if err != nil {
			return err
		}
		if bytes.Equal(lastData, data) {
			continue
		}

		// Set a deadline so that hung connections don't put back pressure on the system.
		// Node updates are tiny, so even the dinkiest connection can handle them if it's not hung.
		err = conn.SetWriteDeadline(time.Now().Add(agpl.WriteTimeout))
		if err != nil {
			// often, this is just because the connection is closed/broken, so only log at debug.
			return err
		}
		_, err = conn.Write(data)
		if err != nil {
			// often, this is just because the connection is closed/broken, so only log at debug.
			return err
		}

		// nhooyr.io/websocket has a bugged implementation of deadlines on a websocket net.Conn.  What they are
		// *supposed* to do is set a deadline for any subsequent writes to complete, otherwise the call to Write()
		// fails.  What nhooyr.io/websocket does is set a timer, after which it expires the websocket write context.
		// If this timer fires, then the next write will fail *even if we set a new write deadline*.  So, after
		// our successful write, it is important that we reset the deadline before it fires.
		err = conn.SetWriteDeadline(time.Time{})
		if err != nil {
			return err
		}
		lastData = data
	}
}
