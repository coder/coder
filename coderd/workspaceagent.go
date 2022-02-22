package coderd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"nhooyr.io/websocket"

	"github.com/coder/coder/httpapi"
	"github.com/coder/coder/httpmw"
)

func (api *api) workspaceAgentServe(rw http.ResponseWriter, r *http.Request) {
	workspaceAgent := httpmw.WorkspaceAgent(r)
	workspaceHistory, err := api.Database.GetWorkspaceHistoryByID(r.Context(), workspaceAgent.WorkspaceHistoryID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace history: %s", err),
		})
		return
	}
	api.websocketWaitGroup.Add(1)
	defer api.websocketWaitGroup.Done()
	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("accept websocket: %s", err),
		})
		return
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	netConn := websocket.NetConn(r.Context(), conn, websocket.MessageBinary)
	err = agentListener(api, netConn, workspaceHistory.WorkspaceID.String())
	if err != nil {
		_ = conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
}

func agentDialer(api *api, conn net.Conn, workspaceID string) error {
	streamID := uuid.New().String()
	decoder := json.NewDecoder(conn)
	cancelSubscribe, err := api.Pubsub.Subscribe(agentPubsubOutID(workspaceID), func(ctx context.Context, message []byte) {
		if len(message) < len(streamID) {
			return
		}
		gotStreamID := message[0:len(streamID)]
		if string(gotStreamID) != streamID {
			return
		}
		message = message[len(streamID):]
		_, _ = conn.Write(message)
	})
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	defer cancelSubscribe()

	for {
		var m json.RawMessage
		err := decoder.Decode(&m)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("decoding: %w", err)
		}
		data, _ := m.MarshalJSON()
		data = append([]byte(streamID), data...)
		err = api.Pubsub.Publish(agentPubsubInID(workspaceID), data)
		if err != nil {
			return fmt.Errorf("publish: %w", err)
		}

	}
	return nil
}

func agentListener(api *api, conn net.Conn, workspaceID string) error {
	api.agentBrokerMutex.Lock()
	if oldConn, ok := api.agentBrokerConnections[workspaceID]; ok {
		_ = oldConn.Close()
	}
	api.agentBrokerConnections[workspaceID] = conn
	api.agentBrokerMutex.Unlock()
	c := yamux.DefaultConfig()
	c.LogOutput = io.Discard
	session, err := yamux.Client(conn, c)
	if err != nil {
		return fmt.Errorf("create yamux client: %w", err)
	}
	var (
		streams      = map[string]*yamux.Stream{}
		streamLock   sync.Mutex
		longIDLength = len(uuid.New().String())
	)
	cancelSubscribe, err := api.Pubsub.Subscribe(agentPubsubInID(workspaceID), func(ctx context.Context, message []byte) {
		if len(message) < longIDLength {
			return
		}
		streamID := string(message[0:longIDLength])
		message = message[longIDLength:]

		streamLock.Lock()
		defer streamLock.Unlock()

		stream, ok := streams[streamID]
		if !ok {
			var err error
			stream, err = session.OpenStream()
			if err != nil {
				return
			}
			streams[streamID] = stream
			go func() {
				defer func() {
					streamLock.Lock()
					defer streamLock.Unlock()
					delete(streams, streamID)
				}()
				decoder := json.NewDecoder(stream)
				for {
					var m json.RawMessage
					err = decoder.Decode(&m)
					if err != nil {
						return
					}
					data, _ := m.MarshalJSON()
					data = append([]byte(streamID), data...)
					err = api.Pubsub.Publish(agentPubsubOutID(workspaceID), data)
					if err != nil {
						return
					}
				}
			}()
		}
		_, _ = stream.Write(message)
	})
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	defer cancelSubscribe()
	<-session.CloseChan()
	return nil
}

func agentPubsubOutID(workspaceID string) string {
	return workspaceID + "-out"
}

func agentPubsubInID(workspaceID string) string {
	return workspaceID + "-in"
}
