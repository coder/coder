package coderd

import (
	"fmt"
	"io"
	"net/http"

	"cdr.dev/slog"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionerd/proto"
)

func (api *API) postProvisionerDaemon(rw http.ResponseWriter, r *http.Request) {
	if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceProvisionerDaemon) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.CreateProvisionerDaemonRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}

	provisioner, err := api.Database.InsertProvisionerDaemon(r.Context(), database.InsertProvisionerDaemonParams{
		ID:           uuid.New(),
		CreatedAt:    database.Now(),
		Name:         req.Name,
		Provisioners: []database.ProvisionerType{database.ProvisionerTypeTerraform},
		AuthToken:    uuid.NullUUID{Valid: true, UUID: uuid.New()},
	})
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error inserting provisioner daemon.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusCreated, convertProvisionerDaemon(provisioner))
}

// Serves the provisioner daemon protobuf API over a WebSocket.
func (api *API) provisionerDaemonsListen(rw http.ResponseWriter, r *http.Request) {
	daemon := httpmw.ProvisionerDaemon(r)
	api.Logger.Warn(r.Context(), "daemon connected", slog.F("daemon", daemon.Name))

	api.websocketWaitMutex.Lock()
	api.websocketWaitGroup.Add(1)
	api.websocketWaitMutex.Unlock()
	defer api.websocketWaitGroup.Done()

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Internal error accepting websocket connection.",
			Detail:  err.Error(),
		})
		return
	}
	// Align with the frame size of yamux.
	conn.SetReadLimit(256 * 1024)

	// Multiplexes the incoming connection using yamux.
	// This allows multiple function calls to occur over
	// the same connection.
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Server(websocket.NetConn(r.Context(), conn, websocket.MessageBinary), config)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("multiplex server: %s", err))
		return
	}
	mux := drpcmux.New()
	err = proto.DRPCRegisterProvisionerDaemon(mux, &provisionerdServer{
		AccessURL:    api.AccessURL,
		ID:           daemon.ID,
		Database:     api.Database,
		Pubsub:       api.Pubsub,
		Provisioners: daemon.Provisioners,
		Telemetry:    api.Telemetry,
		Logger:       api.Logger.Named(fmt.Sprintf("provisionerd-%s", daemon.Name)),
	})
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("drpc register provisioner daemon: %s", err))
		return
	}
	server := drpcserver.NewWithOptions(mux, drpcserver.Options{
		Log: func(err error) {
			if xerrors.Is(err, io.EOF) {
				return
			}
			api.Logger.Debug(r.Context(), "drpc server error", slog.Error(err))
		},
	})
	err = server.Serve(r.Context(), session)
	if err != nil && !xerrors.Is(err, io.EOF) {
		api.Logger.Debug(r.Context(), "provisioner daemon disconnected", slog.Error(err))
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("serve: %s", err))
		return
	}
	_ = conn.Close(websocket.StatusGoingAway, "")
}

func convertProvisionerDaemon(daemon database.ProvisionerDaemon) codersdk.ProvisionerDaemon {
	result := codersdk.ProvisionerDaemon{
		ID:        daemon.ID,
		CreatedAt: daemon.CreatedAt,
		UpdatedAt: daemon.UpdatedAt,
		Name:      daemon.Name,
	}
	for _, provisionerType := range daemon.Provisioners {
		result.Provisioners = append(result.Provisioners, codersdk.ProvisionerType(provisionerType))
	}
	if daemon.AuthToken.Valid {
		result.AuthToken = &daemon.AuthToken.UUID
	}
	return result
}
