package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog/v3"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/x/workspacerelay"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// workspaceRelayMetrics counts what the relay sees. An operator reads them
// in log mode to learn what enforcing would break.
type workspaceRelayMetrics struct {
	requests *prometheus.CounterVec
	leases   *prometheus.CounterVec
}

func newWorkspaceRelayMetrics(reg prometheus.Registerer) *workspaceRelayMetrics {
	m := &workspaceRelayMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "workspace_relay",
			Name:      "requests_total",
			Help:      "Requests made with a workspace session token, by whether they came through the workspace's daemon (relayed), without a lease (bare) or with one that is not valid for the workspace (invalid_lease), and by whether they were allowed or refused.",
		}, []string{"route", "outcome"}),
		leases: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "workspace_relay",
			Name:      "leases_total",
			Help:      "Leases that workspace daemons asked for: issued to a new holder (claimed), to the same holder (renewed), to another daemon instance after the holder lapsed (takeover), or refused.",
		}, []string{"result"}),
	}
	reg.MustRegister(m.requests, m.leases)
	return m
}

// workspaceRelayCheck is asked about every API key that authenticates a
// request. It decides for the session tokens of workspaces only: such a token
// must arrive with a lease of the workspace's daemon.
func (api *API) workspaceRelayCheck(ctx context.Context, r *http.Request, key database.APIKey) *codersdk.Response {
	if api.WorkspaceRelayMode == workspacerelay.ModeOff {
		return nil
	}
	// A token name is the user's to choose, so a token a user made is not
	// a workspace's whatever it is called.
	if key.LoginType == database.LoginTypeToken {
		return nil
	}
	_, workspaceID, ok := provisionerdserver.ParseWorkspaceSessionTokenName(key.TokenName)
	if !ok {
		return nil
	}

	route := "bare"
	var claims workspacerelay.LeaseClaims
	if lease := r.Header.Get(workspacerelay.LeaseHeader); lease != "" {
		var err error
		claims, err = workspacerelay.VerifyLease(ctx, api.AppSigningKeyCache, lease, api.Clock.Now())
		switch {
		case err != nil:
			route = "invalid_lease"
			api.Logger.Debug(ctx, "workspace relay lease is not valid", slog.F("workspace_id", workspaceID), slog.Error(err))
		case claims.WorkspaceID != workspaceID:
			// A lease of one workspace vouches for nothing in another.
			route = "invalid_lease"
		default:
			route = "relayed"
		}
	}

	if route == "relayed" {
		api.workspaceRelayMetrics.requests.WithLabelValues(route, "allowed").Inc()
		// The request log says which daemon vouched for the request.
		if rl := loggermw.RequestLoggerFromContext(ctx); rl != nil {
			rl.WithFields(
				slog.F("workspace_relay_workspace_id", claims.WorkspaceID),
				slog.F("workspace_relay_agent_id", claims.AgentID),
				slog.F("workspace_relay_generation", claims.Generation),
			)
		}
		return nil
	}

	if api.WorkspaceRelayMode != workspacerelay.ModeEnforce {
		api.workspaceRelayMetrics.requests.WithLabelValues(route, "allowed").Inc()
		api.Logger.Warn(ctx, "workspace session token used without going through the workspace's daemon: this request would be refused if the workspace relay were enforced",
			slog.F("workspace_id", workspaceID),
			slog.F("api_key_id", key.ID),
			slog.F("route", route),
			slog.F("method", r.Method),
			slog.F("path", r.URL.Path),
			slog.F("remote_addr", r.RemoteAddr),
		)
		return nil
	}
	api.workspaceRelayMetrics.requests.WithLabelValues(route, "refused").Inc()
	api.Logger.Warn(ctx, "refused a workspace session token that did not come through the workspace's daemon",
		slog.F("workspace_id", workspaceID),
		slog.F("api_key_id", key.ID),
		slog.F("route", route),
		slog.F("method", r.Method),
		slog.F("path", r.URL.Path),
		slog.F("remote_addr", r.RemoteAddr),
	)
	return &codersdk.Response{
		Message: "This session token belongs to a workspace and is only accepted through that workspace's daemon.",
		Detail:  "Inside the workspace, use the address in CODER_URL, which the workspace's daemon serves. The token does not work anywhere else.",
	}
}

// relayAIGateway applies the relay to AI Gateway, which authenticates Coder
// tokens by itself and so never asks workspaceRelayCheck.
func (api *API) relayAIGateway(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// The lease is for the control plane. It is not passed on to a
		// model provider.
		defer r.Header.Del(workspacerelay.LeaseHeader)
		if api.WorkspaceRelayMode == workspacerelay.ModeOff {
			next.ServeHTTP(rw, r)
			return
		}
		keyID, _, err := httpmw.SplitAPIToken(agplaibridge.ExtractAuthToken(r.Header))
		if err != nil {
			// AI Gateway says what is wrong with the token.
			next.ServeHTTP(rw, r)
			return
		}
		//nolint:gocritic // The key is looked up to see whether it is a workspace's. AI Gateway authenticates it.
		key, err := api.Database.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), keyID)
		if err != nil {
			next.ServeHTTP(rw, r)
			return
		}
		if refusal := api.workspaceRelayCheck(ctx, r, key); refusal != nil {
			httpapi.Write(ctx, rw, http.StatusUnauthorized, *refusal)
			return
		}
		r.Header.Del(workspacerelay.LeaseHeader)
		next.ServeHTTP(rw, r)
	})
}

// WorkspaceRelayAIGateway returns the middleware that applies the workspace
// relay to AI Gateway's routes.
func (api *API) WorkspaceRelayAIGateway() func(http.Handler) http.Handler {
	return api.relayAIGateway
}

// @Summary Obtain a workspace relay lease (experimental)
// @ID obtain-workspace-relay-lease
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.WorkspaceRelayLeaseRequest true "Lease request"
// @Success 200 {object} agentsdk.WorkspaceRelayLease
// @Router /api/v2/workspaceagents/me/relay-lease [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentRelayLease(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if api.WorkspaceRelayMode == workspacerelay.ModeOff {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "The workspace relay is off.",
		})
		return
	}
	var req agentsdk.WorkspaceRelayLeaseRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	agent := httpmw.WorkspaceAgent(r)
	build := httpmw.LatestBuild(r)
	now := api.Clock.Now()
	if err := workspacerelay.VerifyHolder(req.PublicKey, req.Signature, req.Timestamp, now); err != nil {
		api.workspaceRelayMetrics.leases.WithLabelValues("refused").Inc()
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The request for a lease is not signed by the key it names.",
			Detail:  err.Error(),
		})
		return
	}

	//nolint:gocritic // The daemon was authenticated by its own token. Its relay is not a user's to claim.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	previous, err := api.Database.GetWorkspaceAgentRelayByAgentID(sysCtx, agent.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}
	held := err == nil
	relay, err := api.Database.ClaimWorkspaceAgentRelay(sysCtx, database.ClaimWorkspaceAgentRelayParams{
		WorkspaceAgentID: agent.ID,
		PublicKey:        req.PublicKey,
		Now:              now,
		LapsedBefore:     now.Add(-workspacerelay.DefaultLeaseTTL),
	})
	if errors.Is(err, sql.ErrNoRows) {
		// Somebody else holds the relay and is alive. This is what a
		// copy of the daemon's token gets.
		api.workspaceRelayMetrics.leases.WithLabelValues("refused").Inc()
		api.Logger.Warn(ctx, "refused a workspace relay lease: another instance of the daemon holds the relay",
			slog.F("workspace_id", build.WorkspaceID),
			slog.F("agent_id", agent.ID),
			slog.F("generation", previous.Generation),
			slog.F("remote_addr", r.RemoteAddr),
		)
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Another instance of this workspace's daemon holds the relay.",
			Detail:  "It can be taken over once that instance has stopped renewing its lease.",
		})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	switch {
	case !held, relay.Generation != previous.Generation && previous.RenewedAt.Unix() == 0:
		// The first instance, or the next one after the holder gave the
		// relay up when it stopped.
		api.workspaceRelayMetrics.leases.WithLabelValues("claimed").Inc()
	case relay.Generation != previous.Generation:
		// Either the daemon restarted, or somebody with a copy of its
		// token waited for it to stop. The control plane cannot tell.
		api.workspaceRelayMetrics.leases.WithLabelValues("takeover").Inc()
		api.Logger.Warn(ctx, "another instance of a workspace daemon took over the relay",
			slog.F("workspace_id", build.WorkspaceID),
			slog.F("agent_id", agent.ID),
			slog.F("generation", relay.Generation),
			slog.F("previous_holder_renewed_at", previous.RenewedAt),
			slog.F("remote_addr", r.RemoteAddr),
		)
	default:
		api.workspaceRelayMetrics.leases.WithLabelValues("renewed").Inc()
	}

	lease, expiresAt, err := workspacerelay.IssueLease(ctx, api.AppSigningKeyCache, now, workspacerelay.DefaultLeaseTTL, build.WorkspaceID, agent.ID, relay.Generation)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, agentsdk.WorkspaceRelayLease{
		Lease:      lease,
		ExpiresAt:  expiresAt,
		Generation: relay.Generation,
		Enforced:   api.WorkspaceRelayMode == workspacerelay.ModeEnforce,
	})
}

// @Summary Release a workspace relay lease (experimental)
// @ID release-workspace-relay-lease
// @Security CoderSessionToken
// @Accept json
// @Tags Agents
// @Param request body agentsdk.WorkspaceRelayLeaseRequest true "Lease request"
// @Success 204
// @Router /api/v2/workspaceagents/me/relay-lease [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteWorkspaceAgentRelayLease(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req agentsdk.WorkspaceRelayLeaseRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	agent := httpmw.WorkspaceAgent(r)
	if err := workspacerelay.VerifyHolder(req.PublicKey, req.Signature, req.Timestamp, api.Clock.Now()); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The request is not signed by the key it names.",
			Detail:  err.Error(),
		})
		return
	}
	//nolint:gocritic // The daemon was authenticated by its own token. Its relay is not a user's to release.
	err := api.Database.ReleaseWorkspaceAgentRelay(dbauthz.AsSystemRestricted(ctx), database.ReleaseWorkspaceAgentRelayParams{
		WorkspaceAgentID: agent.ID,
		PublicKey:        req.PublicKey,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}
