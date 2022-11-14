package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/provisionerdserver"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionerd/proto"
	"github.com/coder/coder/provisionersdk"
)

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
	daemons, err = AuthorizeFilter(api.HTTPAuth, r, rbac.ActionRead, daemons)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner daemons.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, daemons)
}

// ListenProvisionerDaemon is an in-memory connection to a provisionerd.  Useful when starting coderd and provisionerd
// in the same process.
func (api *API) ListenProvisionerDaemon(ctx context.Context, acquireJobDebounce time.Duration) (client proto.DRPCProvisionerDaemonClient, err error) {
	clientSession, serverSession := provisionersdk.TransportPipe()
	defer func() {
		if err != nil {
			_ = clientSession.Close()
			_ = serverSession.Close()
		}
	}()

	name := namesgenerator.GetRandomName(1)
	daemon, err := api.Database.InsertProvisionerDaemon(ctx, database.InsertProvisionerDaemonParams{
		ID:           uuid.New(),
		CreatedAt:    database.Now(),
		Name:         name,
		Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho, database.ProvisionerTypeTerraform},
	})
	if err != nil {
		return nil, xerrors.Errorf("insert provisioner daemon %q: %w", name, err)
	}

	mux := drpcmux.New()
	err = proto.DRPCRegisterProvisionerDaemon(mux, &provisionerdserver.Server{
		AccessURL:          api.AccessURL,
		ID:                 daemon.ID,
		Database:           api.Database,
		Pubsub:             api.Pubsub,
		Provisioners:       daemon.Provisioners,
		Telemetry:          api.Telemetry,
		Logger:             api.Logger.Named(fmt.Sprintf("provisionerd-%s", daemon.Name)),
		AcquireJobDebounce: acquireJobDebounce,
		QuotaCommitter:     &api.QuotaCommitter,
	})
	if err != nil {
		return nil, err
	}
	server := drpcserver.NewWithOptions(mux, drpcserver.Options{
		Log: func(err error) {
			if xerrors.Is(err, io.EOF) {
				return
			}
			api.Logger.Debug(ctx, "drpc server error", slog.Error(err))
		},
	})
	go func() {
		err := server.Serve(ctx, serverSession)
		if err != nil && !xerrors.Is(err, io.EOF) {
			api.Logger.Debug(ctx, "provisioner daemon disconnected", slog.Error(err))
		}
		// close the sessions so we don't leak goroutines serving them.
		_ = clientSession.Close()
		_ = serverSession.Close()
	}()

	return proto.NewDRPCProvisionerDaemonClient(provisionersdk.Conn(clientSession)), nil
}
