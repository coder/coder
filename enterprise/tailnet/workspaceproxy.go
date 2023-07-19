package tailnet

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/enterprise/wsproxy/wsproxysdk"
	agpl "github.com/coder/coder/tailnet"
)

func ServeWorkspaceProxy(ctx context.Context, conn net.Conn, ma agpl.MultiAgentConn) error {
	go func() {
		err := forwardNodesToWorkspaceProxy(ctx, conn, ma)
		if err != nil {
			_ = conn.Close()
		}
	}()

	decoder := json.NewDecoder(conn)
	for {
		var msg wsproxysdk.CoordinateMessage
		err := decoder.Decode(&msg)
		if err != nil {
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
			err := ma.UpdateSelf(msg.Node)
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
		nodes, ok := ma.NextUpdate(ctx)
		if !ok {
			return xerrors.New("multiagent is closed")
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
