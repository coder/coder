package coderd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/provisionerdserver"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionerd/proto"
)

func (api *API) provisionerDaemonsEnabledMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		api.entitlementsMu.RLock()
		epd := api.entitlements.Features[codersdk.FeatureExternalProvisionerDaemons].Enabled
		api.entitlementsMu.RUnlock()

		if !epd {
			httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
				Message: "External provisioner daemons is an Enterprise feature. Contact sales!",
			})
			return
		}

		next.ServeHTTP(rw, r)
	})
}

// @Summary Get provisioner daemons
// @ID get-provisioner-daemons
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.ProvisionerDaemon
// @Router /organizations/{organization}/provisionerdaemons [get]
func (api *API) provisionerDaemons(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	daemons, err := api.Database.GetProvisionerDaemons(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner daemons.",
			Detail:  err.Error(),
		})
		return
	}
	if daemons == nil {
		daemons = []database.ProvisionerDaemon{}
	}
	daemons, err = coderd.AuthorizeFilter(api.AGPL.HTTPAuth, r, rbac.ActionRead, daemons)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner daemons.",
			Detail:  err.Error(),
		})
		return
	}
	apiDaemons := make([]codersdk.ProvisionerDaemon, 0)
	for _, daemon := range daemons {
		apiDaemons = append(apiDaemons, convertProvisionerDaemon(daemon))
	}
	httpapi.Write(ctx, rw, http.StatusOK, apiDaemons)
}

// Serves the provisioner daemon protobuf API over a WebSocket.
//
// @Summary Serve provisioner daemon
// @ID serve-provisioner-daemon
// @Security CoderSessionToken
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 101
// @Router /organizations/{organization}/provisionerdaemons/serve [get]
func (api *API) provisionerDaemonServe(rw http.ResponseWriter, r *http.Request) {
	tags := map[string]string{}
	if r.URL.Query().Has("tag") {
		for _, tag := range r.URL.Query()["tag"] {
			parts := strings.SplitN(tag, "=", 2)
			if len(parts) < 2 {
				httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
					Message: fmt.Sprintf("Invalid format for tag %q. Key and value must be separated with =.", tag),
				})
				return
			}
			tags[parts[0]] = parts[1]
		}
	}
	if !r.URL.Query().Has("provisioner") {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "The provisioner query parameter must be specified.",
		})
		return
	}

	provisionersMap := map[codersdk.ProvisionerType]struct{}{}
	for _, provisioner := range r.URL.Query()["provisioner"] {
		switch provisioner {
		case string(codersdk.ProvisionerTypeEcho):
			provisionersMap[codersdk.ProvisionerTypeEcho] = struct{}{}
		case string(codersdk.ProvisionerTypeTerraform):
			provisionersMap[codersdk.ProvisionerTypeTerraform] = struct{}{}
		default:
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Unknown provisioner type %q", provisioner),
			})
			return
		}
	}

	// Any authenticated user can create provisioner daemons scoped
	// for jobs that they own, but only authorized users can create
	// globally scoped provisioners that attach to all jobs.
	apiKey := httpmw.APIKey(r)
	tags = provisionerdserver.MutateTags(apiKey.UserID, tags)

	if tags[provisionerdserver.TagScope] == provisionerdserver.ScopeOrganization {
		if !api.AGPL.Authorize(r, rbac.ActionCreate, rbac.ResourceProvisionerDaemon) {
			httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
				Message: "You aren't allowed to create provisioner daemons for the organization.",
			})
			return
		}
	}

	provisioners := make([]database.ProvisionerType, 0)
	for p := range provisionersMap {
		switch p {
		case codersdk.ProvisionerTypeTerraform:
			provisioners = append(provisioners, database.ProvisionerTypeTerraform)
		case codersdk.ProvisionerTypeEcho:
			provisioners = append(provisioners, database.ProvisionerTypeEcho)
		}
	}

	name := namesgenerator.GetRandomName(1)
	daemon, err := api.Database.InsertProvisionerDaemon(r.Context(), database.InsertProvisionerDaemonParams{
		ID:           uuid.New(),
		CreatedAt:    database.Now(),
		Name:         name,
		Provisioners: provisioners,
		Tags:         tags,
	})
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error writing provisioner daemon.",
			Detail:  err.Error(),
		})
		return
	}

	rawTags, err := json.Marshal(daemon.Tags)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error marshaling daemon tags.",
			Detail:  err.Error(),
		})
		return
	}

	api.AGPL.WebsocketWaitMutex.Lock()
	api.AGPL.WebsocketWaitGroup.Add(1)
	api.AGPL.WebsocketWaitMutex.Unlock()
	defer api.AGPL.WebsocketWaitGroup.Done()

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
	err = proto.DRPCRegisterProvisionerDaemon(mux, &provisionerdserver.Server{
		AccessURL:    api.AccessURL,
		ID:           daemon.ID,
		Database:     api.Database,
		Pubsub:       api.Pubsub,
		Provisioners: daemon.Provisioners,
		Telemetry:    api.Telemetry,
		Auditor:      &api.AGPL.Auditor,
		Logger:       api.Logger.Named(fmt.Sprintf("provisionerd-%s", daemon.Name)),
		Tags:         rawTags,
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
		Tags:      daemon.Tags,
	}
	for _, provisionerType := range daemon.Provisioners {
		result.Provisioners = append(result.Provisioners, codersdk.ProvisionerType(provisionerType))
	}
	return result
}
