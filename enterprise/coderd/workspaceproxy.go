package coderd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	agpl "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/coderd/proxyhealth"
	"github.com/coder/coder/v2/enterprise/replicasync"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
)

// whitelistedCryptoKeyFeatures is a list of crypto key features that are
// allowed to be queried with workspace proxies.
var whitelistedCryptoKeyFeatures = []database.CryptoKeyFeature{
	database.CryptoKeyFeatureWorkspaceAppsToken,
	database.CryptoKeyFeatureWorkspaceAppsAPIKey,
}

// forceWorkspaceProxyHealthUpdate forces an update of the proxy health.
// This is useful when a proxy is created or deleted. Errors will be logged.
func (api *API) forceWorkspaceProxyHealthUpdate(ctx context.Context) {
	if err := api.ProxyHealth.ForceUpdate(ctx); err != nil && !database.IsQueryCanceledError(err) && !xerrors.Is(err, context.Canceled) {
		api.Logger.Error(ctx, "force proxy health update", slog.Error(err))
	}
}

// NOTE: this doesn't need a swagger definition since AGPL already has one, and
// this route overrides the AGPL one.
func (api *API) regions(rw http.ResponseWriter, r *http.Request) {
	regions, err := api.fetchRegions(r.Context())
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, regions)
}

func (api *API) fetchRegions(ctx context.Context) (codersdk.RegionsResponse[codersdk.Region], error) {
	//nolint:gocritic // this intentionally requests resources that users
	// cannot usually access in order to give them a full list of available
	// regions. Regions are just a data subset of proxies.
	ctx = dbauthz.AsSystemRestricted(ctx)
	proxies, err := api.fetchWorkspaceProxies(ctx)
	if err != nil {
		return codersdk.RegionsResponse[codersdk.Region]{}, err
	}

	regions := make([]codersdk.Region, 0, len(proxies.Regions))
	for i := range proxies.Regions {
		// Ignore deleted and DERP-only proxies.
		if proxies.Regions[i].Deleted || proxies.Regions[i].DerpOnly {
			continue
		}
		// Append the inner region data.
		regions = append(regions, proxies.Regions[i].Region)
	}

	return codersdk.RegionsResponse[codersdk.Region]{
		Regions: regions,
	}, nil
}

// @Summary Update workspace proxy
// @ID update-workspace-proxy
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param workspaceproxy path string true "Proxy ID or name" format(uuid)
// @Param request body codersdk.PatchWorkspaceProxy true "Update workspace proxy request"
// @Success 200 {object} codersdk.WorkspaceProxy
// @Router /workspaceproxies/{workspaceproxy} [patch]
func (api *API) patchWorkspaceProxy(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		proxy             = httpmw.WorkspaceProxyParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.WorkspaceProxy](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	aReq.Old = proxy
	defer commitAudit()

	var req codersdk.PatchWorkspaceProxy
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var hashedSecret []byte
	var fullToken string
	if req.RegenerateToken {
		var err error
		fullToken, hashedSecret, err = generateWorkspaceProxyToken(proxy.ID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
	}

	deploymentIDStr, err := api.Database.GetDeploymentID(ctx)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	var updatedProxy database.WorkspaceProxy
	if proxy.ID.String() == deploymentIDStr {
		// User is editing the default primary proxy.
		var ok bool
		updatedProxy, ok = api.patchPrimaryWorkspaceProxy(req, rw, r)
		if !ok {
			return
		}
	} else {
		updatedProxy, err = api.Database.UpdateWorkspaceProxy(ctx, database.UpdateWorkspaceProxyParams{
			Name:        req.Name,
			DisplayName: req.DisplayName,
			Icon:        req.Icon,
			ID:          proxy.ID,
			// If hashedSecret is nil or empty, this will not update the secret.
			TokenHashedSecret: hashedSecret,
		})
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
	}

	aReq.New = updatedProxy
	status, ok := api.ProxyHealth.HealthStatus()[updatedProxy.ID]
	if !ok {
		// The proxy should have some status, but just in case.
		status.Status = proxyhealth.Unknown
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.UpdateWorkspaceProxyResponse{
		Proxy:      convertProxy(updatedProxy, status),
		ProxyToken: fullToken,
	})

	// Update the proxy cache.
	go api.forceWorkspaceProxyHealthUpdate(api.ctx)
}

// patchPrimaryWorkspaceProxy handles the special case of updating the default
func (api *API) patchPrimaryWorkspaceProxy(req codersdk.PatchWorkspaceProxy, rw http.ResponseWriter, r *http.Request) (database.WorkspaceProxy, bool) {
	var (
		ctx   = r.Context()
		proxy = httpmw.WorkspaceProxyParam(r)
	)

	// User is editing the default primary proxy.
	if req.Name != "" && req.Name != proxy.Name {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot update name of default primary proxy, did you mean to update the 'display name'?",
			Validations: []codersdk.ValidationError{
				{Field: "name", Detail: "Cannot update name of default primary proxy"},
			},
		})
		return database.WorkspaceProxy{}, false
	}
	if req.DisplayName == "" && req.Icon == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "No update arguments provided. Nothing to do.",
			Validations: []codersdk.ValidationError{
				{Field: "display_name", Detail: "No value provided."},
				{Field: "icon", Detail: "No value provided."},
			},
		})
		return database.WorkspaceProxy{}, false
	}

	args := database.UpsertDefaultProxyParams{
		DisplayName: req.DisplayName,
		IconUrl:     req.Icon,
	}
	if req.DisplayName == "" || req.Icon == "" {
		// If the user has not specified an update value, use the existing value.
		existing, err := api.Database.GetDefaultProxyConfig(ctx)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return database.WorkspaceProxy{}, false
		}
		if req.DisplayName == "" {
			args.DisplayName = existing.DisplayName
		}
		if req.Icon == "" {
			args.IconUrl = existing.IconUrl
		}
	}

	err := api.Database.UpsertDefaultProxy(ctx, args)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return database.WorkspaceProxy{}, false
	}

	// Use the primary region to fetch the default proxy values.
	updatedProxy, err := api.AGPL.PrimaryWorkspaceProxy(ctx)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return database.WorkspaceProxy{}, false
	}
	return updatedProxy, true
}

// @Summary Delete workspace proxy
// @ID delete-workspace-proxy
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param workspaceproxy path string true "Proxy ID or name" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /workspaceproxies/{workspaceproxy} [delete]
func (api *API) deleteWorkspaceProxy(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		proxy             = httpmw.WorkspaceProxyParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.WorkspaceProxy](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
	)
	aReq.Old = proxy
	defer commitAudit()

	if proxy.IsPrimary() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Cannot delete primary proxy",
		})
		return
	}

	err := api.Database.UpdateWorkspaceProxyDeleted(ctx, database.UpdateWorkspaceProxyDeletedParams{
		ID:      proxy.ID,
		Deleted: true,
	})
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = database.WorkspaceProxy{}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Proxy has been deleted!",
	})

	// Update the proxy health cache to remove this proxy.
	go api.forceWorkspaceProxyHealthUpdate(api.ctx)
}

// @Summary Get workspace proxy
// @ID get-workspace-proxy
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param workspaceproxy path string true "Proxy ID or name" format(uuid)
// @Success 200 {object} codersdk.WorkspaceProxy
// @Router /workspaceproxies/{workspaceproxy} [get]
func (api *API) workspaceProxy(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx   = r.Context()
		proxy = httpmw.WorkspaceProxyParam(r)
	)

	httpapi.Write(ctx, rw, http.StatusOK, convertProxy(proxy, api.ProxyHealth.HealthStatus()[proxy.ID]))
}

// @Summary Create workspace proxy
// @ID create-workspace-proxy
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.CreateWorkspaceProxyRequest true "Create workspace proxy request"
// @Success 201 {object} codersdk.WorkspaceProxy
// @Router /workspaceproxies [post]
func (api *API) postWorkspaceProxy(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.WorkspaceProxy](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	var req codersdk.CreateWorkspaceProxyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if strings.ToLower(req.Name) == "primary" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: `The name "primary" is reserved for the primary region.`,
			Detail:  "Cannot name a workspace proxy 'primary'.",
			Validations: []codersdk.ValidationError{
				{
					Field:  "name",
					Detail: "Reserved name",
				},
			},
		})
		return
	}

	id := uuid.New()
	fullToken, hashedSecret, err := generateWorkspaceProxyToken(id)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	proxy, err := api.Database.InsertWorkspaceProxy(ctx, database.InsertWorkspaceProxyParams{
		ID:                id,
		Name:              req.Name,
		DisplayName:       req.DisplayName,
		Icon:              req.Icon,
		TokenHashedSecret: hashedSecret,
		// Enabled by default, but will be disabled on register if the proxy has
		// it disabled.
		DerpEnabled: true,
		// Disabled by default, but blah blah blah.
		DerpOnly:  false,
		CreatedAt: dbtime.Now(),
		UpdatedAt: dbtime.Now(),
	})
	if database.IsUniqueViolation(err, database.UniqueWorkspaceProxiesLowerNameIndex) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Workspace proxy with name %q already exists.", req.Name),
		})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	api.Telemetry.Report(&telemetry.Snapshot{
		WorkspaceProxies: []telemetry.WorkspaceProxy{telemetry.ConvertWorkspaceProxy(proxy)},
	})

	aReq.New = proxy
	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.UpdateWorkspaceProxyResponse{
		Proxy: convertProxy(proxy, proxyhealth.ProxyStatus{
			Proxy:     proxy,
			CheckedAt: time.Now(),
			Status:    proxyhealth.Unregistered,
		}),
		ProxyToken: fullToken,
	})

	// Update the proxy health cache to include this new proxy.
	go api.forceWorkspaceProxyHealthUpdate(api.ctx)
}

// nolint:revive
func validateProxyURL(u string) error {
	p, err := url.Parse(u)
	if err != nil {
		return err
	}
	if p.Scheme != "http" && p.Scheme != "https" {
		return xerrors.New("scheme must be http or https")
	}
	if !(p.Path == "/" || p.Path == "") {
		return xerrors.New("path must be empty or /")
	}
	return nil
}

// @Summary Get workspace proxies
// @ID get-workspace-proxies
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {array} codersdk.RegionsResponse[codersdk.WorkspaceProxy]
// @Router /workspaceproxies [get]
func (api *API) workspaceProxies(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	proxies, err := api.fetchWorkspaceProxies(r.Context())
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
				Message: "You are not authorized to use this endpoint.",
			})
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, proxies)
}

func (api *API) fetchWorkspaceProxies(ctx context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
	proxies, err := api.Database.GetWorkspaceProxies(ctx)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return codersdk.RegionsResponse[codersdk.WorkspaceProxy]{}, err
	}

	// Add the primary as well
	primaryProxy, err := api.AGPL.PrimaryWorkspaceProxy(ctx)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return codersdk.RegionsResponse[codersdk.WorkspaceProxy]{}, err
	}
	proxies = append([]database.WorkspaceProxy{primaryProxy}, proxies...)

	statues := api.ProxyHealth.HealthStatus()
	return codersdk.RegionsResponse[codersdk.WorkspaceProxy]{
		Regions: convertProxies(proxies, statues),
	}, nil
}

// @Summary Issue signed workspace app token
// @ID issue-signed-workspace-app-token
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body workspaceapps.IssueTokenRequest true "Issue signed app token request"
// @Success 201 {object} wsproxysdk.IssueSignedAppTokenResponse
// @Router /workspaceproxies/me/issue-signed-app-token [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceProxyIssueSignedAppToken(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// NOTE: this endpoint will return JSON on success, but will (usually)
	// return a self-contained HTML error page on failure. The external proxy
	// should forward any non-201 response to the client.

	var req workspaceapps.IssueTokenRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// userReq is a http request from the user on the other side of the proxy.
	// Although the workspace proxy is making this call, we want to use the user's
	// authorization context to create the token.
	//
	// We can use the existing request context for all tracing/logging purposes.
	// Any workspace proxy auth uses different context keys so we don't need to
	// worry about that.
	userReq, err := http.NewRequestWithContext(ctx, "GET", req.AppRequest.BasePath, nil)
	if err != nil {
		// This should never happen
		httpapi.InternalServerError(rw, xerrors.Errorf("[DEV ERROR] new request: %w", err))
		return
	}
	userReq.Header.Set(codersdk.SessionTokenHeader, req.SessionToken)

	// Exchange the token.
	token, tokenStr, ok := api.AGPL.WorkspaceAppsProvider.Issue(ctx, rw, userReq, req)
	if !ok {
		return
	}
	if token == nil {
		httpapi.InternalServerError(rw, xerrors.New("nil token after calling token provider"))
		return
	}

	httpapi.Write(ctx, rw, http.StatusCreated, wsproxysdk.IssueSignedAppTokenResponse{
		SignedTokenStr: tokenStr,
	})
}

// @Summary Report workspace app stats
// @ID report-workspace-app-stats
// @Security CoderSessionToken
// @Accept json
// @Tags Enterprise
// @Param request body wsproxysdk.ReportAppStatsRequest true "Report app stats request"
// @Success 204
// @Router /workspaceproxies/me/app-stats [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceProxyReportAppStats(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = httpmw.WorkspaceProxy(r) // Ensure the proxy is authenticated.

	var req wsproxysdk.ReportAppStatsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	api.Logger.Debug(ctx, "report app stats", slog.F("stats", req.Stats))

	reporter := api.WorkspaceAppsStatsCollectorOptions.Reporter
	if err := reporter.ReportAppStats(ctx, req.Stats); err != nil {
		api.Logger.Error(ctx, "report app stats failed", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

// workspaceProxyRegister is used to register a new workspace proxy. When a proxy
// comes online, it will announce itself to this endpoint. This updates its values
// in the database and returns a signed token that can be used to authenticate
// tokens.
//
// This is called periodically by the proxy in the background (every 30s per
// replica) to ensure that the proxy is still registered and the corresponding
// replica table entry is refreshed.
//
// @Summary Register workspace proxy
// @ID register-workspace-proxy
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body wsproxysdk.RegisterWorkspaceProxyRequest true "Register workspace proxy request"
// @Success 201 {object} wsproxysdk.RegisterWorkspaceProxyResponse
// @Router /workspaceproxies/me/register [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceProxyRegister(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx   = r.Context()
		proxy = httpmw.WorkspaceProxy(r)
	)

	var req wsproxysdk.RegisterWorkspaceProxyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// NOTE: we previously enforced version checks when registering, but this
	// will cause proxies to enter crash loop backoff if the server is updated
	// and the proxy is not. Most releases do not make backwards-incompatible
	// changes to the proxy API, so instead of blocking requests we will show
	// healthcheck warnings.

	if err := validateProxyURL(req.AccessURL); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "URL is invalid.",
			Detail:  err.Error(),
		})
		return
	}

	if req.WildcardHostname != "" {
		if _, err := appurl.CompileHostnamePattern(req.WildcardHostname); err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Wildcard URL is invalid.",
				Detail:  err.Error(),
			})
			return
		}
	}

	if req.ReplicaID == uuid.Nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Replica ID is invalid.",
		})
		return
	}

	if req.DerpOnly && !req.DerpEnabled {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "DerpOnly cannot be true when DerpEnabled is false.",
		})
		return
	}

	startingRegionID, _ := getProxyDERPStartingRegionID(api.Options.BaseDERPMap)
	// #nosec G115 - Safe conversion as DERP region IDs are small integers expected to be within int32 range
	regionID := int32(startingRegionID) + proxy.RegionID

	err := api.Database.InTx(func(db database.Store) error {
		// First, update the proxy's values in the database.
		_, err := db.RegisterWorkspaceProxy(ctx, database.RegisterWorkspaceProxyParams{
			ID:               proxy.ID,
			Url:              req.AccessURL,
			DerpEnabled:      req.DerpEnabled,
			DerpOnly:         req.DerpOnly,
			WildcardHostname: req.WildcardHostname,
			Version:          req.Version,
		})
		if err != nil {
			return xerrors.Errorf("register workspace proxy: %w", err)
		}

		// Second, find the replica that corresponds to this proxy and refresh
		// it if it exists. If it doesn't exist, create it.
		now := time.Now()
		replica, err := db.GetReplicaByID(ctx, req.ReplicaID)
		if err == nil {
			// Replica exists, update it.
			if replica.StoppedAt.Valid && !replica.StartedAt.IsZero() {
				// If the replica deregistered, it shouldn't be able to
				// re-register before restarting.
				// TODO: sadly this results in 500 when it should be 400
				return xerrors.Errorf("replica %s is marked stopped", replica.ID)
			}

			replica, err = db.UpdateReplica(ctx, database.UpdateReplicaParams{
				ID:              replica.ID,
				UpdatedAt:       now,
				StartedAt:       replica.StartedAt,
				StoppedAt:       replica.StoppedAt,
				RelayAddress:    req.ReplicaRelayAddress,
				RegionID:        regionID,
				Hostname:        req.ReplicaHostname,
				Version:         req.Version,
				Error:           req.ReplicaError,
				DatabaseLatency: 0,
				Primary:         false,
			})
			if err != nil {
				return xerrors.Errorf("update replica: %w", err)
			}
		} else if xerrors.Is(err, sql.ErrNoRows) {
			// Replica doesn't exist, create it.
			replica, err = db.InsertReplica(ctx, database.InsertReplicaParams{
				ID:              req.ReplicaID,
				CreatedAt:       now,
				StartedAt:       now,
				UpdatedAt:       now,
				Hostname:        req.ReplicaHostname,
				RegionID:        regionID,
				RelayAddress:    req.ReplicaRelayAddress,
				Version:         req.Version,
				DatabaseLatency: 0,
				Primary:         false,
			})
			if err != nil {
				return xerrors.Errorf("insert replica: %w", err)
			}
		} else {
			return xerrors.Errorf("get replica: %w", err)
		}

		return nil
	}, nil)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// Update replica sync and notify all other replicas to update their
	// replica list.
	err = api.replicaManager.PublishUpdate()
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	replicaUpdateCtx, replicaUpdateCancel := context.WithTimeout(ctx, 5*time.Second)
	defer replicaUpdateCancel()
	err = api.replicaManager.UpdateNow(replicaUpdateCtx)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// Find sibling regions to respond with for derpmesh.
	siblings := api.replicaManager.InRegion(regionID)
	siblingsRes := make([]codersdk.Replica, 0, len(siblings))
	for _, replica := range siblings {
		if replica.ID == req.ReplicaID {
			continue
		}
		siblingsRes = append(siblingsRes, convertReplica(replica))
	}

	httpapi.Write(ctx, rw, http.StatusCreated, wsproxysdk.RegisterWorkspaceProxyResponse{
		DERPMeshKey:         api.DERPServer.MeshKey(),
		DERPRegionID:        regionID,
		DERPMap:             api.AGPL.DERPMap(),
		DERPForceWebSockets: api.DeploymentValues.DERP.Config.ForceWebSockets.Value(),
		SiblingReplicas:     siblingsRes,
	})

	go api.forceWorkspaceProxyHealthUpdate(api.ctx)
}

// workspaceProxyCryptoKeys is used to fetch signing keys for the workspace proxy.
//
// This is called periodically by the proxy in the background (every 10m per
// replica) to ensure that the proxy has the latest signing keys.
//
// @Summary Get workspace proxy crypto keys
// @ID get-workspace-proxy-crypto-keys
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param feature query string true "Feature key"
// @Success 200 {object} wsproxysdk.CryptoKeysResponse
// @Router /workspaceproxies/me/crypto-keys [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceProxyCryptoKeys(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	feature := database.CryptoKeyFeature(r.URL.Query().Get("feature"))
	if feature == "" {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing feature query parameter.",
		})
		return
	}

	if !slices.Contains(whitelistedCryptoKeyFeatures, feature) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Invalid feature: %q", feature),
		})
		return
	}

	keys, err := api.Database.GetCryptoKeysByFeature(ctx, feature)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, wsproxysdk.CryptoKeysResponse{
		CryptoKeys: db2sdk.CryptoKeys(keys),
	})
}

// @Summary Deregister workspace proxy
// @ID deregister-workspace-proxy
// @Security CoderSessionToken
// @Accept json
// @Tags Enterprise
// @Param request body wsproxysdk.DeregisterWorkspaceProxyRequest true "Deregister workspace proxy request"
// @Success 204
// @Router /workspaceproxies/me/deregister [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceProxyDeregister(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req wsproxysdk.DeregisterWorkspaceProxyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	err := api.Database.InTx(func(db database.Store) error {
		now := time.Now()
		replica, err := db.GetReplicaByID(ctx, req.ReplicaID)
		if err != nil {
			return xerrors.Errorf("get replica: %w", err)
		}

		if replica.StoppedAt.Valid && !replica.StartedAt.IsZero() {
			// TODO: sadly this results in 500 when it should be 400
			return xerrors.Errorf("replica %s is already marked stopped", replica.ID)
		}

		replica, err = db.UpdateReplica(ctx, database.UpdateReplicaParams{
			ID:        replica.ID,
			UpdatedAt: now,
			StartedAt: replica.StartedAt,
			StoppedAt: sql.NullTime{
				Valid: true,
				Time:  now,
			},
			RelayAddress:    replica.RelayAddress,
			RegionID:        replica.RegionID,
			Hostname:        replica.Hostname,
			Version:         replica.Version,
			Error:           replica.Error,
			DatabaseLatency: replica.DatabaseLatency,
			Primary:         replica.Primary,
		})
		if err != nil {
			return xerrors.Errorf("update replica: %w", err)
		}

		return nil
	}, nil)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// Publish a replicasync event with a nil ID so every replica (yes, even the
	// current replica) will refresh its replicas list.
	err = api.Pubsub.Publish(replicasync.PubsubEvent, []byte(uuid.Nil.String()))
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
	go api.forceWorkspaceProxyHealthUpdate(api.ctx)
}

// reconnectingPTYSignedToken issues a signed app token for use when connecting
// to the reconnecting PTY websocket on an external workspace proxy. This is set
// by the client as a query parameter when connecting.
//
// @Summary Issue signed app token for reconnecting PTY
// @ID issue-signed-app-token-for-reconnecting-pty
// @Security CoderSessionToken
// @Tags Enterprise
// @Accept json
// @Produce json
// @Param request body codersdk.IssueReconnectingPTYSignedTokenRequest true "Issue reconnecting PTY signed token request"
// @Success 200 {object} codersdk.IssueReconnectingPTYSignedTokenResponse
// @Router /applications/reconnecting-pty-signed-token [post]
// @x-apidocgen {"skip": true}
func (api *API) reconnectingPTYSignedToken(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	if !api.Authorize(r, policy.ActionCreate, apiKey) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.IssueReconnectingPTYSignedTokenRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	u, err := url.Parse(req.URL)
	if err == nil && u.Scheme != "ws" && u.Scheme != "wss" {
		err = xerrors.Errorf("invalid URL scheme %q, expected 'ws' or 'wss'", u.Scheme)
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid URL.",
			Detail:  err.Error(),
		})
		return
	}

	// Assert the URL is a valid reconnecting-pty URL.
	expectedPath := fmt.Sprintf("/api/v2/workspaceagents/%s/pty", req.AgentID.String())
	if u.Path != expectedPath {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid URL path.",
			Detail:  "The provided URL is not a valid reconnecting PTY endpoint URL.",
		})
		return
	}

	scheme, err := api.AGPL.ValidWorkspaceAppHostname(ctx, u.Host, agpl.ValidWorkspaceAppHostnameOpts{
		// Only allow the proxy access URL as a hostname since we don't need a
		// ticket for the primary dashboard URL terminal.
		AllowPrimaryAccessURL: false,
		AllowPrimaryWildcard:  false,
		AllowProxyAccessURL:   true,
		AllowProxyWildcard:    false,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to verify hostname in URL.",
			Detail:  err.Error(),
		})
		return
	}
	if scheme == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid hostname in URL.",
			Detail:  "The hostname must be the primary wildcard app hostname, a workspace proxy access URL or a workspace proxy wildcard app hostname.",
		})
		return
	}

	_, tokenStr, ok := api.AGPL.WorkspaceAppsProvider.Issue(ctx, rw, r, workspaceapps.IssueTokenRequest{
		AppRequest: workspaceapps.Request{
			AccessMethod:  workspaceapps.AccessMethodTerminal,
			BasePath:      u.Path,
			AgentNameOrID: req.AgentID.String(),
		},
		SessionToken: httpmw.APITokenFromRequest(r),
		// The following fields aren't required as long as the request is authed
		// with a valid API key, which we know since this endpoint is protected
		// by auth middleware already.
		PathAppBaseURL: "",
		AppHostname:    "",
		// The following fields are empty for terminal apps.
		AppPath:  "",
		AppQuery: "",
	})
	if !ok {
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.IssueReconnectingPTYSignedTokenResponse{
		SignedToken: tokenStr,
	})
}

func generateWorkspaceProxyToken(id uuid.UUID) (token string, hashed []byte, err error) {
	secret, err := cryptorand.HexString(64)
	if err != nil {
		return "", nil, xerrors.Errorf("generate token: %w", err)
	}
	hashedSecret := sha256.Sum256([]byte(secret))
	fullToken := fmt.Sprintf("%s:%s", id, secret)
	return fullToken, hashedSecret[:], nil
}

func convertProxies(p []database.WorkspaceProxy, statuses map[uuid.UUID]proxyhealth.ProxyStatus) []codersdk.WorkspaceProxy {
	resp := make([]codersdk.WorkspaceProxy, 0, len(p))
	for _, proxy := range p {
		resp = append(resp, convertProxy(proxy, statuses[proxy.ID]))
	}
	return resp
}

func convertRegion(proxy database.WorkspaceProxy, status proxyhealth.ProxyStatus) codersdk.Region {
	return codersdk.Region{
		ID:               proxy.ID,
		Name:             proxy.Name,
		DisplayName:      proxy.DisplayName,
		IconURL:          proxy.Icon,
		Healthy:          status.Status == proxyhealth.Healthy,
		PathAppURL:       proxy.Url,
		WildcardHostname: proxy.WildcardHostname,
	}
}

func convertProxy(p database.WorkspaceProxy, status proxyhealth.ProxyStatus) codersdk.WorkspaceProxy {
	now := dbtime.Now()
	if p.IsPrimary() {
		// Primary is always healthy since the primary serves the api that this
		// is returned from.
		u, _ := url.Parse(p.Url)
		status = proxyhealth.ProxyStatus{
			Proxy:     p,
			ProxyHost: u.Host,
			Status:    proxyhealth.Healthy,
			Report:    codersdk.ProxyHealthReport{},
			CheckedAt: now,
		}
		// For primary, created at / updated at are always 'now'
		p.CreatedAt = now
		p.UpdatedAt = now
	}
	if status.Status == "" {
		status.Status = proxyhealth.Unknown
	}
	if status.Report.Errors == nil {
		status.Report.Errors = make([]string, 0)
	}
	if status.Report.Warnings == nil {
		status.Report.Warnings = make([]string, 0)
	}
	return codersdk.WorkspaceProxy{
		Region:      convertRegion(p, status),
		DerpEnabled: p.DerpEnabled,
		DerpOnly:    p.DerpOnly,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
		Deleted:     p.Deleted,
		Version:     p.Version,
		Status: codersdk.WorkspaceProxyStatus{
			Status:    codersdk.ProxyHealthStatus(status.Status),
			Report:    status.Report,
			CheckedAt: status.CheckedAt,
		},
	}
}

// workspaceProxiesFetchUpdater implements healthcheck.WorkspaceProxyFetchUpdater
// in an actually useful and meaningful way.
type workspaceProxiesFetchUpdater struct {
	fetchFunc  func(context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error)
	updateFunc func(context.Context) error
}

func (w *workspaceProxiesFetchUpdater) Fetch(ctx context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
	//nolint:gocritic // Need perms to read all workspace proxies.
	authCtx := dbauthz.AsSystemRestricted(ctx)
	return w.fetchFunc(authCtx)
}

func (w *workspaceProxiesFetchUpdater) Update(ctx context.Context) error {
	return w.updateFunc(ctx)
}
