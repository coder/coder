package coderd

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coder/coder/v2/provisionersdk"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/moby/moby/pkg/namesgenerator"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionerd/proto"
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

type provisionerDaemonAuth struct {
	psk        string
	authorizer rbac.Authorizer
}

// authorize returns mutated tags and true if the given HTTP request is authorized to access the provisioner daemon
// protobuf API, and returns nil, false otherwise.
func (p *provisionerDaemonAuth) authorize(r *http.Request, tags map[string]string) (map[string]string, bool) {
	ctx := r.Context()
	apiKey, ok := httpmw.APIKeyOptional(r)
	if ok {
		tags = provisionersdk.MutateTags(apiKey.UserID, tags)
		if tags[provisionersdk.TagScope] == provisionersdk.ScopeUser {
			// Any authenticated user can create provisioner daemons scoped
			// for jobs that they own,
			return tags, true
		}
		ua := httpmw.UserAuthorization(r)
		if err := p.authorizer.Authorize(ctx, ua.Actor, rbac.ActionCreate, rbac.ResourceProvisionerDaemon); err == nil {
			// User is allowed to create provisioner daemons
			return tags, true
		}
	}

	// Check for PSK
	if p.psk != "" {
		psk := r.Header.Get(codersdk.ProvisionerDaemonPSK)
		if subtle.ConstantTimeCompare([]byte(p.psk), []byte(psk)) == 1 {
			// If using PSK auth, the daemon is, by definition, scoped to the organization.
			tags[provisionersdk.TagScope] = provisionersdk.ScopeOrganization
			return tags, true
		}
	}
	return nil, false
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
	ctx := r.Context()

	tags := map[string]string{}
	if r.URL.Query().Has("tag") {
		for _, tag := range r.URL.Query()["tag"] {
			parts := strings.SplitN(tag, "=", 2)
			if len(parts) < 2 {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: fmt.Sprintf("Invalid format for tag %q. Key and value must be separated with =.", tag),
				})
				return
			}
			tags[parts[0]] = parts[1]
		}
	}
	if !r.URL.Query().Has("provisioner") {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The provisioner query parameter must be specified.",
		})
		return
	}

	id, _ := uuid.Parse(r.URL.Query().Get("id"))
	if id == uuid.Nil {
		id = uuid.New()
	}

	provisionersMap := map[codersdk.ProvisionerType]struct{}{}
	for _, provisioner := range r.URL.Query()["provisioner"] {
		switch provisioner {
		case string(codersdk.ProvisionerTypeEcho):
			provisionersMap[codersdk.ProvisionerTypeEcho] = struct{}{}
		case string(codersdk.ProvisionerTypeTerraform):
			provisionersMap[codersdk.ProvisionerTypeTerraform] = struct{}{}
		default:
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Unknown provisioner type %q", provisioner),
			})
			return
		}
	}

	name := namesgenerator.GetRandomName(10)
	if vals, ok := r.URL.Query()["name"]; ok && len(vals) > 0 {
		name = vals[0]
	} else {
		api.Logger.Warn(ctx, "unnamed provisioner daemon")
	}

	tags, authorized := api.provisionerDaemonAuth.authorize(r, tags)
	if !authorized {
		api.Logger.Warn(ctx, "unauthorized provisioner daemon serve request", slog.F("tags", tags))
		httpapi.Write(ctx, rw, http.StatusForbidden,
			codersdk.Response{Message: "You aren't allowed to create provisioner daemons"})
		return
	}
	api.Logger.Debug(ctx, "provisioner authorized", slog.F("tags", tags))
	if err := provisionerdserver.Tags(tags).Valid(); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Given tags are not acceptable to the service",
			Validations: []codersdk.ValidationError{
				{Field: "tags", Detail: err.Error()},
			},
		})
		return
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

	log := api.Logger.With(
		slog.F("name", name),
		slog.F("provisioners", provisioners),
		slog.F("tags", tags),
	)

	api.AGPL.WebsocketWaitMutex.Lock()
	api.AGPL.WebsocketWaitGroup.Add(1)
	api.AGPL.WebsocketWaitMutex.Unlock()
	defer api.AGPL.WebsocketWaitGroup.Done()

	tep := telemetry.ConvertExternalProvisioner(id, tags, provisioners)
	api.Telemetry.Report(&telemetry.Snapshot{ExternalProvisioners: []telemetry.ExternalProvisioner{tep}})
	defer func() {
		tep.ShutdownAt = ptr.Ref(time.Now())
		api.Telemetry.Report(&telemetry.Snapshot{ExternalProvisioners: []telemetry.ExternalProvisioner{tep}})
	}()

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		// Need to disable compression to avoid a data-race.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		if !xerrors.Is(err, context.Canceled) {
			log.Error(ctx, "accept provisioner websocket conn", slog.Error(err))
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
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
	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()
	session, err := yamux.Server(wsNetConn, config)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("multiplex server: %s", err))
		return
	}
	mux := drpcmux.New()
	logger := api.Logger.Named(fmt.Sprintf("ext-provisionerd-%s", name))
	logger.Info(ctx, "starting external provisioner daemon")
	srv, err := provisionerdserver.NewServer(
		api.ctx,
		api.AccessURL,
		id,
		logger,
		provisioners,
		tags,
		api.Database,
		api.Pubsub,
		api.AGPL.Acquirer,
		api.Telemetry,
		trace.NewNoopTracerProvider().Tracer("noop"),
		&api.AGPL.QuotaCommitter,
		&api.AGPL.Auditor,
		api.AGPL.TemplateScheduleStore,
		api.AGPL.UserQuietHoursScheduleStore,
		api.DeploymentValues,
		provisionerdserver.Options{
			ExternalAuthConfigs: api.ExternalAuthConfigs,
			OIDCConfig:          api.OIDCConfig,
		},
	)
	if err != nil {
		if !xerrors.Is(err, context.Canceled) {
			log.Error(ctx, "create provisioner daemon server", slog.Error(err))
		}
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("create provisioner daemon server: %s", err))
		return
	}
	err = proto.DRPCRegisterProvisionerDaemon(mux, srv)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("drpc register provisioner daemon: %s", err))
		return
	}
	server := drpcserver.NewWithOptions(mux, drpcserver.Options{
		Log: func(err error) {
			if xerrors.Is(err, io.EOF) {
				return
			}
			logger.Debug(ctx, "drpc server error", slog.Error(err))
		},
	})
	err = server.Serve(ctx, session)
	logger.Info(ctx, "provisioner daemon disconnected", slog.Error(err))
	if err != nil && !xerrors.Is(err, io.EOF) {
		_ = conn.Close(websocket.StatusInternalError, httpapi.WebsocketCloseSprintf("serve: %s", err))
		return
	}
	_ = conn.Close(websocket.StatusGoingAway, "")
}

func convertProvisionerDaemon(daemon database.ProvisionerDaemon) codersdk.ProvisionerDaemon {
	result := codersdk.ProvisionerDaemon{
		ID:         daemon.ID,
		CreatedAt:  daemon.CreatedAt,
		LastSeenAt: codersdk.NullTime{NullTime: daemon.LastSeenAt},
		Name:       daemon.Name,
		Tags:       daemon.Tags,
		Version:    daemon.Version,
	}
	for _, provisionerType := range daemon.Provisioners {
		result.Provisioners = append(result.Provisioners, codersdk.ProvisionerType(provisionerType))
	}
	return result
}

// wsNetConn wraps net.Conn created by websocket.NetConn(). Cancel func
// is called if a read or write error is encountered.
type wsNetConn struct {
	cancel context.CancelFunc
	net.Conn
}

func (c *wsNetConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *wsNetConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *wsNetConn) Close() error {
	defer c.cancel()
	return c.Conn.Close()
}

// websocketNetConn wraps websocket.NetConn and returns a context that
// is tied to the parent context and the lifetime of the conn. Any error
// during read or write will cancel the context, but not close the
// conn. Close should be called to release context resources.
func websocketNetConn(ctx context.Context, conn *websocket.Conn, msgType websocket.MessageType) (context.Context, net.Conn) {
	ctx, cancel := context.WithCancel(ctx)
	nc := websocket.NetConn(ctx, conn, msgType)
	return ctx, &wsNetConn{
		cancel: cancel,
		Conn:   nc,
	}
}
