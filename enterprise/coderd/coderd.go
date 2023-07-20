package coderd

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd"
	agplaudit "github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	agplschedule "github.com/coder/coder/coderd/schedule"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/enterprise/coderd/proxyhealth"
	"github.com/coder/coder/enterprise/coderd/schedule"
	"github.com/coder/coder/enterprise/derpmesh"
	"github.com/coder/coder/enterprise/replicasync"
	"github.com/coder/coder/enterprise/tailnet"
	"github.com/coder/coder/provisionerd/proto"
	agpltailnet "github.com/coder/coder/tailnet"
)

// New constructs an Enterprise coderd API instance.
// This handler is designed to wrap the AGPL Coder code and
// layer Enterprise functionality on top as much as possible.
func New(ctx context.Context, options *Options) (_ *API, err error) {
	if options.EntitlementsUpdateInterval == 0 {
		options.EntitlementsUpdateInterval = 10 * time.Minute
	}
	if options.Keys == nil {
		options.Keys = Keys
	}
	if options.Options == nil {
		options.Options = &coderd.Options{}
	}
	if options.PrometheusRegistry == nil {
		options.PrometheusRegistry = prometheus.NewRegistry()
	}
	if options.Options.Authorizer == nil {
		options.Options.Authorizer = rbac.NewCachingAuthorizer(options.PrometheusRegistry)
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	api := &API{
		ctx:    ctx,
		cancel: cancelFunc,

		AGPL:    coderd.New(options.Options),
		Options: options,
	}
	defer func() {
		if err != nil {
			_ = api.Close()
		}
	}()

	api.AGPL.Options.SetUserGroups = api.setUserGroups
	api.AGPL.SiteHandler.AppearanceFetcher = api.fetchAppearanceConfig
	api.AGPL.SiteHandler.RegionsFetcher = func(ctx context.Context) (any, error) {
		// If the user can read the workspace proxy resource, return that.
		// If not, always default to the regions.
		actor, ok := dbauthz.ActorFromContext(ctx)
		if ok && api.Authorizer.Authorize(ctx, actor, rbac.ActionRead, rbac.ResourceWorkspaceProxy) == nil {
			return api.fetchWorkspaceProxies(ctx)
		}
		return api.fetchRegions(ctx)
	}

	oauthConfigs := &httpmw.OAuth2Configs{
		Github: options.GithubOAuth2Config,
		OIDC:   options.OIDCConfig,
	}
	apiKeyMiddleware := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB:                          options.Database,
		OAuth2Configs:               oauthConfigs,
		RedirectToLogin:             false,
		DisableSessionExpiryRefresh: options.DeploymentValues.DisableSessionExpiryRefresh.Value(),
		Optional:                    false,
		SessionTokenFunc:            nil, // Default behavior
	})
	apiKeyMiddlewareOptional := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB:                          options.Database,
		OAuth2Configs:               oauthConfigs,
		RedirectToLogin:             false,
		DisableSessionExpiryRefresh: options.DeploymentValues.DisableSessionExpiryRefresh.Value(),
		Optional:                    true,
		SessionTokenFunc:            nil, // Default behavior
	})

	deploymentID, err := options.Database.GetDeploymentID(ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to get deployment ID: %w", err)
	}

	api.AGPL.APIHandler.Group(func(r chi.Router) {
		r.Get("/entitlements", api.serveEntitlements)
		// /regions overrides the AGPL /regions endpoint
		r.Group(func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Get("/regions", api.regions)
		})
		r.Route("/replicas", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Get("/", api.replicas)
		})
		r.Route("/licenses", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Post("/", api.postLicense)
			r.Get("/", api.licenses)
			r.Delete("/{id}", api.deleteLicense)
		})
		r.Route("/applications/reconnecting-pty-signed-token", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Post("/", api.reconnectingPTYSignedToken)
		})

		r.With(
			apiKeyMiddlewareOptional,
			httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
				DB:       options.Database,
				Optional: true,
			}),
			httpmw.RequireAPIKeyOrWorkspaceProxyAuth(),
		).Get("/workspaceagents/{workspaceagent}/legacy", api.agentIsLegacy)
		r.Route("/workspaceproxies", func(r chi.Router) {
			r.Use(
				api.moonsEnabledMW,
			)
			r.Group(func(r chi.Router) {
				r.Use(
					apiKeyMiddleware,
				)
				r.Post("/", api.postWorkspaceProxy)
				r.Get("/", api.workspaceProxies)
			})
			r.Route("/me", func(r chi.Router) {
				r.Use(
					httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
						DB:       options.Database,
						Optional: false,
					}),
				)
				r.Get("/coordinate", api.workspaceProxyCoordinate)
				r.Post("/issue-signed-app-token", api.workspaceProxyIssueSignedAppToken)
				r.Post("/register", api.workspaceProxyRegister)
				r.Post("/goingaway", api.workspaceProxyGoingAway)
			})
			r.Route("/{workspaceproxy}", func(r chi.Router) {
				r.Use(
					apiKeyMiddleware,
					httpmw.ExtractWorkspaceProxyParam(api.Database, deploymentID, api.AGPL.PrimaryWorkspaceProxy),
				)

				r.Get("/", api.workspaceProxy)
				r.Patch("/", api.patchWorkspaceProxy)
				r.Delete("/", api.deleteWorkspaceProxy)
			})
		})
		r.Route("/organizations/{organization}/groups", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				api.templateRBACEnabledMW,
				httpmw.ExtractOrganizationParam(api.Database),
			)
			r.Post("/", api.postGroupByOrganization)
			r.Get("/", api.groupsByOrganization)
			r.Route("/{groupName}", func(r chi.Router) {
				r.Use(
					httpmw.ExtractGroupByNameParam(api.Database),
				)

				r.Get("/", api.groupByOrganization)
			})
		})
		r.Route("/organizations/{organization}/provisionerdaemons", func(r chi.Router) {
			r.Use(
				api.provisionerDaemonsEnabledMW,
				apiKeyMiddleware,
				httpmw.ExtractOrganizationParam(api.Database),
			)
			r.Get("/", api.provisionerDaemons)
			r.Get("/serve", api.provisionerDaemonServe)
		})
		r.Route("/templates/{template}/acl", func(r chi.Router) {
			r.Use(
				api.templateRBACEnabledMW,
				apiKeyMiddleware,
				httpmw.ExtractTemplateParam(api.Database),
			)
			r.Get("/", api.templateACL)
			r.Patch("/", api.patchTemplateACL)
		})
		r.Route("/groups/{group}", func(r chi.Router) {
			r.Use(
				api.templateRBACEnabledMW,
				apiKeyMiddleware,
				httpmw.ExtractGroupParam(api.Database),
			)
			r.Get("/", api.group)
			r.Patch("/", api.patchGroup)
			r.Delete("/", api.deleteGroup)
		})
		r.Route("/workspace-quota", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
			)
			r.Route("/{user}", func(r chi.Router) {
				r.Use(httpmw.ExtractUserParam(options.Database, false))
				r.Get("/", api.workspaceQuota)
			})
		})
		r.Route("/appearance", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(
					apiKeyMiddlewareOptional,
					httpmw.ExtractWorkspaceAgent(httpmw.ExtractWorkspaceAgentConfig{
						DB:       options.Database,
						Optional: true,
					}),
					httpmw.RequireAPIKeyOrWorkspaceAgent(),
				)
				r.Get("/", api.appearance)
			})
			r.Group(func(r chi.Router) {
				r.Use(
					apiKeyMiddleware,
				)
				r.Put("/", api.putAppearance)
			})
		})
		r.Route("/users/{user}/quiet-hours", func(r chi.Router) {
			r.Use(
				api.restartRequirementEnabledMW,
				apiKeyMiddleware,
				httpmw.ExtractUserParam(options.Database, false),
			)

			r.Get("/", api.userQuietHoursSchedule)
			r.Put("/", api.putUserQuietHoursSchedule)
		})
	})

	if len(options.SCIMAPIKey) != 0 {
		api.AGPL.RootHandler.Route("/scim/v2", func(r chi.Router) {
			r.Use(
				api.scimEnabledMW,
			)
			r.Post("/Users", api.scimPostUser)
			r.Route("/Users", func(r chi.Router) {
				r.Get("/", api.scimGetUsers)
				r.Post("/", api.scimPostUser)
				r.Get("/{id}", api.scimGetUser)
				r.Patch("/{id}", api.scimPatchUser)
			})
		})
	}

	meshRootCA := x509.NewCertPool()
	for _, certificate := range options.TLSCertificates {
		for _, certificatePart := range certificate.Certificate {
			certificate, err := x509.ParseCertificate(certificatePart)
			if err != nil {
				return nil, xerrors.Errorf("parse certificate %s: %w", certificate.Subject.CommonName, err)
			}
			meshRootCA.AddCert(certificate)
		}
	}
	// This TLS configuration spoofs access from the access URL hostname
	// assuming that the certificates provided will cover that hostname.
	//
	// Replica sync and DERP meshing require accessing replicas via their
	// internal IP addresses, and if TLS is configured we use the same
	// certificates.
	meshTLSConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: options.TLSCertificates,
		RootCAs:      meshRootCA,
		ServerName:   options.AccessURL.Hostname(),
	}
	api.replicaManager, err = replicasync.New(ctx, options.Logger, options.Database, options.Pubsub, &replicasync.Options{
		ID:           api.AGPL.ID,
		RelayAddress: options.DERPServerRelayAddress,
		RegionID:     int32(options.DERPServerRegionID),
		TLSConfig:    meshTLSConfig,
	})
	if err != nil {
		return nil, xerrors.Errorf("initialize replica: %w", err)
	}
	api.derpMesh = derpmesh.New(options.Logger.Named("derpmesh"), api.DERPServer, meshTLSConfig)

	if api.AGPL.Experiments.Enabled(codersdk.ExperimentMoons) {
		// Proxy health is a moon feature.
		api.ProxyHealth, err = proxyhealth.New(&proxyhealth.Options{
			Interval:   options.ProxyHealthInterval,
			DB:         api.Database,
			Logger:     options.Logger.Named("proxyhealth"),
			Client:     api.HTTPClient,
			Prometheus: api.PrometheusRegistry,
		})
		if err != nil {
			return nil, xerrors.Errorf("initialize proxy health: %w", err)
		}
		go api.ProxyHealth.Run(ctx)
		// Force the initial loading of the cache. Do this in a go routine in case
		// the calls to the workspace proxies hang and this takes some time.
		go api.forceWorkspaceProxyHealthUpdate(ctx)

		// Use proxy health to return the healthy workspace proxy hostnames.
		f := api.ProxyHealth.ProxyHosts
		api.AGPL.WorkspaceProxyHostsFn.Store(&f)
	}

	err = api.updateEntitlements(ctx)
	if err != nil {
		return nil, xerrors.Errorf("update entitlements: %w", err)
	}
	go api.runEntitlementsLoop(ctx)

	return api, nil
}

type Options struct {
	*coderd.Options

	RBAC         bool
	AuditLogging bool
	// Whether to block non-browser connections.
	BrowserOnly bool
	SCIMAPIKey  []byte

	// Used for high availability.
	DERPServerRelayAddress string
	DERPServerRegionID     int

	// Used for user quiet hours schedules.
	DefaultQuietHoursSchedule string // cron schedule, if empty user quiet hours schedules are disabled

	EntitlementsUpdateInterval time.Duration
	ProxyHealthInterval        time.Duration
	Keys                       map[string]ed25519.PublicKey
}

type API struct {
	AGPL *coderd.API
	*Options

	// ctx is canceled immediately on shutdown, it can be used to abort
	// interruptible tasks.
	ctx    context.Context
	cancel context.CancelFunc

	// Detects multiple Coder replicas running at the same time.
	replicaManager *replicasync.Manager
	// Meshes DERP connections from multiple replicas.
	derpMesh *derpmesh.Mesh
	// ProxyHealth checks the reachability of all workspace proxies.
	ProxyHealth *proxyhealth.ProxyHealth

	entitlementsUpdateMu sync.Mutex
	entitlementsMu       sync.RWMutex
	entitlements         codersdk.Entitlements
}

func (api *API) Close() error {
	api.cancel()
	if api.replicaManager != nil {
		_ = api.replicaManager.Close()
	}
	if api.derpMesh != nil {
		_ = api.derpMesh.Close()
	}
	return api.AGPL.Close()
}

func (api *API) updateEntitlements(ctx context.Context) error {
	api.entitlementsUpdateMu.Lock()
	defer api.entitlementsUpdateMu.Unlock()

	entitlements, err := license.Entitlements(
		ctx, api.Database,
		api.Logger, len(api.replicaManager.All()), len(api.GitAuthConfigs), api.Keys, map[codersdk.FeatureName]bool{
			codersdk.FeatureAuditLog:                   api.AuditLogging,
			codersdk.FeatureBrowserOnly:                api.BrowserOnly,
			codersdk.FeatureSCIM:                       len(api.SCIMAPIKey) != 0,
			codersdk.FeatureHighAvailability:           api.DERPServerRelayAddress != "",
			codersdk.FeatureMultipleGitAuth:            len(api.GitAuthConfigs) > 1,
			codersdk.FeatureTemplateRBAC:               api.RBAC,
			codersdk.FeatureExternalProvisionerDaemons: true,
			codersdk.FeatureAdvancedTemplateScheduling: true,
			// FeatureTemplateRestartRequirement depends on
			// FeatureAdvancedTemplateScheduling.
			codersdk.FeatureTemplateRestartRequirement: api.DefaultQuietHoursSchedule != "",
			codersdk.FeatureWorkspaceProxy:             true,
		})
	if err != nil {
		return err
	}

	if entitlements.RequireTelemetry && !api.DeploymentValues.Telemetry.Enable.Value() {
		// We can't fail because then the user couldn't remove the offending
		// license w/o a restart.
		//
		// We don't simply append to entitlement.Errors since we don't want any
		// enterprise features enabled.
		api.entitlements.Errors = []string{
			"License requires telemetry but telemetry is disabled",
		}
		api.Logger.Error(ctx, "license requires telemetry enabled")
		return nil
	}

	if entitlements.Features[codersdk.FeatureTemplateRestartRequirement].Enabled && !entitlements.Features[codersdk.FeatureAdvancedTemplateScheduling].Enabled {
		api.entitlements.Errors = []string{
			`Your license is entitled to the feature "template restart ` +
				`requirement" (and you have it enabled by setting the ` +
				"default quiet hours schedule), but you are not entitled to " +
				`the dependency feature "advanced template scheduling". ` +
				"Please contact support for a new license.",
		}
		api.Logger.Error(ctx, "license is entitled to template restart requirement but not advanced template scheduling")
		return nil
	}

	featureChanged := func(featureName codersdk.FeatureName) (initial, changed, enabled bool) {
		if api.entitlements.Features == nil {
			return true, false, entitlements.Features[featureName].Enabled
		}
		oldFeature := api.entitlements.Features[featureName]
		newFeature := entitlements.Features[featureName]
		if oldFeature.Enabled != newFeature.Enabled {
			return false, true, newFeature.Enabled
		}
		return false, false, newFeature.Enabled
	}

	shouldUpdate := func(initial, changed, enabled bool) bool {
		// Avoid an initial tick on startup unless the feature is enabled.
		return changed || (initial && enabled)
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureAuditLog); shouldUpdate(initial, changed, enabled) {
		auditor := agplaudit.NewNop()
		if enabled {
			auditor = api.AGPL.Options.Auditor
		}
		api.AGPL.Auditor.Store(&auditor)
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureBrowserOnly); shouldUpdate(initial, changed, enabled) {
		var handler func(rw http.ResponseWriter) bool
		if enabled {
			handler = api.shouldBlockNonBrowserConnections
		}
		api.AGPL.WorkspaceClientCoordinateOverride.Store(&handler)
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureTemplateRBAC); shouldUpdate(initial, changed, enabled) {
		if enabled {
			committer := committer{Database: api.Database}
			ptr := proto.QuotaCommitter(&committer)
			api.AGPL.QuotaCommitter.Store(&ptr)
		} else {
			api.AGPL.QuotaCommitter.Store(nil)
		}
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureAdvancedTemplateScheduling); shouldUpdate(initial, changed, enabled) {
		if enabled {
			templateStore := schedule.NewEnterpriseTemplateScheduleStore()
			templateStoreInterface := agplschedule.TemplateScheduleStore(templateStore)
			api.AGPL.TemplateScheduleStore.Store(&templateStoreInterface)
		} else {
			templateStore := agplschedule.NewAGPLTemplateScheduleStore()
			api.AGPL.TemplateScheduleStore.Store(&templateStore)
		}
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureTemplateRestartRequirement); shouldUpdate(initial, changed, enabled) {
		if enabled {
			templateStore := *(api.AGPL.TemplateScheduleStore.Load())
			enterpriseTemplateStore, ok := templateStore.(*schedule.EnterpriseTemplateScheduleStore)
			if !ok {
				api.Logger.Error(ctx, "unable to set up enterprise template schedule store, template restart requirements will not be applied to workspace builds")
			}
			enterpriseTemplateStore.UseRestartRequirement.Store(true)

			quietHoursStore, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(api.DefaultQuietHoursSchedule)
			if err != nil {
				api.Logger.Error(ctx, "unable to set up enterprise user quiet hours schedule store, template restart requirements will not be applied to workspace builds", slog.Error(err))
			} else {
				api.AGPL.UserQuietHoursScheduleStore.Store(&quietHoursStore)
			}
		} else {
			if api.DefaultQuietHoursSchedule != "" {
				api.Logger.Warn(ctx, "template restart requirements are not enabled (due to setting default quiet hours schedule) as your license is not entitled to this feature")
			}

			templateStore := *(api.AGPL.TemplateScheduleStore.Load())
			enterpriseTemplateStore, ok := templateStore.(*schedule.EnterpriseTemplateScheduleStore)
			if ok {
				enterpriseTemplateStore.UseRestartRequirement.Store(false)
			}

			quietHoursStore := agplschedule.NewAGPLUserQuietHoursScheduleStore()
			api.AGPL.UserQuietHoursScheduleStore.Store(&quietHoursStore)
		}
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureHighAvailability); shouldUpdate(initial, changed, enabled) {
		var coordinator agpltailnet.Coordinator
		if enabled {
			var haCoordinator agpltailnet.Coordinator
			if api.AGPL.Experiments.Enabled(codersdk.ExperimentTailnetHACoordinator) {
				haCoordinator, err = tailnet.NewCoordinator(api.Logger, api.Pubsub)
			} else {
				haCoordinator, err = tailnet.NewPGCoord(api.ctx, api.Logger, api.Pubsub, api.Database)
			}
			if err != nil {
				api.Logger.Error(ctx, "unable to set up high availability coordinator", slog.Error(err))
				// If we try to setup the HA coordinator and it fails, nothing
				// is actually changing.
			} else {
				coordinator = haCoordinator
			}

			api.replicaManager.SetCallback(func() {
				addresses := make([]string, 0)
				for _, replica := range api.replicaManager.Regional() {
					addresses = append(addresses, replica.RelayAddress)
				}
				api.derpMesh.SetAddresses(addresses, false)
				_ = api.updateEntitlements(ctx)
			})
		} else {
			coordinator = agpltailnet.NewCoordinator(api.Logger)
			api.derpMesh.SetAddresses([]string{}, false)
			api.replicaManager.SetCallback(func() {
				// If the amount of replicas change, so should our entitlements.
				// This is to display a warning in the UI if the user is unlicensed.
				_ = api.updateEntitlements(ctx)
			})
		}

		// Recheck changed in case the HA coordinator failed to set up.
		if coordinator != nil {
			oldCoordinator := *api.AGPL.TailnetCoordinator.Swap(&coordinator)
			err := oldCoordinator.Close()
			if err != nil {
				api.Logger.Error(ctx, "close old tailnet coordinator", slog.Error(err))
			}
		}
	}

	api.entitlementsMu.Lock()
	defer api.entitlementsMu.Unlock()
	api.entitlements = entitlements
	api.AGPL.SiteHandler.Entitlements.Store(&entitlements)

	return nil
}

// @Summary Get entitlements
// @ID get-entitlements
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {object} codersdk.Entitlements
// @Router /entitlements [get]
func (api *API) serveEntitlements(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	api.entitlementsMu.RLock()
	entitlements := api.entitlements
	api.entitlementsMu.RUnlock()
	httpapi.Write(ctx, rw, http.StatusOK, entitlements)
}

func (api *API) runEntitlementsLoop(ctx context.Context) {
	eb := backoff.NewExponentialBackOff()
	eb.MaxElapsedTime = 0 // retry indefinitely
	b := backoff.WithContext(eb, ctx)
	updates := make(chan struct{}, 1)
	subscribed := false

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// pass
		}
		if !subscribed {
			cancel, err := api.Pubsub.Subscribe(PubsubEventLicenses, func(_ context.Context, _ []byte) {
				// don't block.  If the channel is full, drop the event, as there is a resync
				// scheduled already.
				select {
				case updates <- struct{}{}:
					// pass
				default:
					// pass
				}
			})
			if err != nil {
				api.Logger.Warn(ctx, "failed to subscribe to license updates", slog.Error(err))
				select {
				case <-ctx.Done():
					return
				case <-time.After(b.NextBackOff()):
				}
				continue
			}
			// nolint: revive
			defer cancel()
			subscribed = true
			api.Logger.Debug(ctx, "successfully subscribed to pubsub")
		}

		api.Logger.Debug(ctx, "syncing licensed entitlements")
		err := api.updateEntitlements(ctx)
		if err != nil {
			api.Logger.Warn(ctx, "failed to get feature entitlements", slog.Error(err))
			time.Sleep(b.NextBackOff())
			continue
		}
		b.Reset()
		api.Logger.Debug(ctx, "synced licensed entitlements")

		select {
		case <-ctx.Done():
			return
		case <-time.After(api.EntitlementsUpdateInterval):
			continue
		case <-updates:
			api.Logger.Debug(ctx, "got pubsub update")
			continue
		}
	}
}

func (api *API) Authorize(r *http.Request, action rbac.Action, object rbac.Objecter) bool {
	return api.AGPL.HTTPAuth.Authorize(r, action, object)
}
