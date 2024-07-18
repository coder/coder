package coderd

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/coderd/database"
	agplportsharing "github.com/coder/coder/v2/coderd/portsharing"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/enterprise/coderd/portsharing"

	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd"
	agplaudit "github.com/coder/coder/v2/coderd/audit"
	agpldbauthz "github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	agplschedule "github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/dbauthz"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/coderd/proxyhealth"
	"github.com/coder/coder/v2/enterprise/coderd/schedule"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/enterprise/derpmesh"
	"github.com/coder/coder/v2/enterprise/replicasync"
	"github.com/coder/coder/v2/enterprise/tailnet"
	"github.com/coder/coder/v2/provisionerd/proto"
	agpltailnet "github.com/coder/coder/v2/tailnet"
)

// New constructs an Enterprise coderd API instance.
// This handler is designed to wrap the AGPL Coder code and
// layer Enterprise functionality on top as much as possible.
func New(ctx context.Context, options *Options) (_ *API, err error) {
	if options.EntitlementsUpdateInterval == 0 {
		options.EntitlementsUpdateInterval = 10 * time.Minute
	}
	if options.LicenseKeys == nil {
		options.LicenseKeys = Keys
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
	if options.ReplicaErrorGracePeriod == 0 {
		// This will prevent the error from being shown for a minute
		// from when an additional replica was started.
		options.ReplicaErrorGracePeriod = time.Minute
	}

	ctx, cancelFunc := context.WithCancel(ctx)

	if options.ExternalTokenEncryption == nil {
		options.ExternalTokenEncryption = make([]dbcrypt.Cipher, 0)
	}
	// Database encryption is an enterprise feature, but as checking license entitlements
	// depends on the database, we end up in a chicken-and-egg situation. To avoid this,
	// we always enable it but only soft-enforce it.
	if len(options.ExternalTokenEncryption) > 0 {
		var keyDigests []string
		for _, cipher := range options.ExternalTokenEncryption {
			keyDigests = append(keyDigests, cipher.HexDigest())
		}
		options.Logger.Info(ctx, "database encryption enabled", slog.F("keys", keyDigests))
	}

	cryptDB, err := dbcrypt.New(ctx, options.Database, options.ExternalTokenEncryption...)
	if err != nil {
		cancelFunc()
		// If we fail to initialize the database, it's likely that the
		// database is encrypted with an unknown external token encryption key.
		// This is a fatal error.
		var derr *dbcrypt.DecryptFailedError
		if xerrors.As(err, &derr) {
			return nil, xerrors.Errorf("database encrypted with unknown key, either add the key or see https://coder.com/docs/admin/encryption#disabling-encryption: %w", derr)
		}
		return nil, xerrors.Errorf("init database encryption: %w", err)
	}
	options.Database = cryptDB
	api := &API{
		ctx:     ctx,
		cancel:  cancelFunc,
		Options: options,
		provisionerDaemonAuth: &provisionerDaemonAuth{
			psk:        options.ProvisionerDaemonPSK,
			authorizer: options.Authorizer,
		},
	}
	// This must happen before coderd initialization!
	options.PostAuthAdditionalHeadersFunc = api.writeEntitlementWarningsHeader
	api.AGPL = coderd.New(options.Options)
	defer func() {
		if err != nil {
			_ = api.Close()
		}
	}()

	api.AGPL.Options.ParseLicenseClaims = func(rawJWT string) (email string, trial bool, err error) {
		c, err := license.ParseClaims(rawJWT, Keys)
		if err != nil {
			return "", false, err
		}
		return c.Subject, c.Trial, nil
	}
	api.AGPL.Options.SetUserGroups = api.setUserGroups
	api.AGPL.Options.SetUserSiteRoles = api.setUserSiteRoles
	api.AGPL.SiteHandler.RegionsFetcher = func(ctx context.Context) (any, error) {
		// If the user can read the workspace proxy resource, return that.
		// If not, always default to the regions.
		actor, ok := agpldbauthz.ActorFromContext(ctx)
		if ok && api.Authorizer.Authorize(ctx, actor, policy.ActionRead, rbac.ResourceWorkspaceProxy) == nil {
			return api.fetchWorkspaceProxies(ctx)
		}
		return api.fetchRegions(ctx)
	}
	api.tailnetService, err = tailnet.NewClientService(agpltailnet.ClientServiceOptions{
		Logger:                  api.Logger.Named("tailnetclient"),
		CoordPtr:                &api.AGPL.TailnetCoordinator,
		DERPMapUpdateFrequency:  api.Options.DERPMapUpdateFrequency,
		DERPMapFn:               api.AGPL.DERPMap,
		NetworkTelemetryHandler: api.AGPL.NetworkTelemetryBatcher.Handler,
	})
	if err != nil {
		api.Logger.Fatal(api.ctx, "failed to initialize tailnet client service", slog.Error(err))
	}

	oauthConfigs := &httpmw.OAuth2Configs{
		Github: options.GithubOAuth2Config,
		OIDC:   options.OIDCConfig,
	}
	apiKeyMiddleware := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB:                            options.Database,
		OAuth2Configs:                 oauthConfigs,
		RedirectToLogin:               false,
		DisableSessionExpiryRefresh:   options.DeploymentValues.Sessions.DisableExpiryRefresh.Value(),
		Optional:                      false,
		SessionTokenFunc:              nil, // Default behavior
		PostAuthAdditionalHeadersFunc: options.PostAuthAdditionalHeadersFunc,
	})
	apiKeyMiddlewareOptional := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB:                            options.Database,
		OAuth2Configs:                 oauthConfigs,
		RedirectToLogin:               false,
		DisableSessionExpiryRefresh:   options.DeploymentValues.Sessions.DisableExpiryRefresh.Value(),
		Optional:                      true,
		SessionTokenFunc:              nil, // Default behavior
		PostAuthAdditionalHeadersFunc: options.PostAuthAdditionalHeadersFunc,
	})

	deploymentID, err := options.Database.GetDeploymentID(ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to get deployment ID: %w", err)
	}

	api.AGPL.RefreshEntitlements = func(ctx context.Context) error {
		return api.refreshEntitlements(ctx)
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
			r.Post("/refresh-entitlements", api.postRefreshEntitlements)
			r.Post("/", api.postLicense)
			r.Get("/", api.licenses)
			r.Delete("/{id}", api.deleteLicense)
		})
		r.Route("/applications/reconnecting-pty-signed-token", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Post("/", api.reconnectingPTYSignedToken)
		})
		r.Route("/workspaceproxies", func(r chi.Router) {
			r.Use(
				api.RequireFeatureMW(codersdk.FeatureWorkspaceProxy),
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
				r.Post("/app-stats", api.workspaceProxyReportAppStats)
				r.Post("/register", api.workspaceProxyRegister)
				r.Post("/deregister", api.workspaceProxyDeregister)
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
		r.Route("/organizations/{organization}/provisionerkeys", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				httpmw.ExtractOrganizationParam(api.Database),
				api.RequireFeatureMW(codersdk.FeatureMultipleOrganizations),
				httpmw.RequireExperiment(api.AGPL.Experiments, codersdk.ExperimentMultiOrganization),
			)
			r.Get("/", api.provisionerKeys)
			r.Post("/", api.postProvisionerKey)
			r.Route("/{provisionerkey}", func(r chi.Router) {
				r.Use(
					httpmw.ExtractProvisionerKeyParam(options.Database),
				)
				r.Delete("/", api.deleteProvisionerKey)
			})
		})
		// TODO: provisioner daemons are not scoped to organizations in the database, so placing them
		// under an organization route doesn't make sense.  In order to allow the /serve endpoint to
		// work with a pre-shared key (PSK) without an API key, these routes will simply ignore the
		// value of {organization}.  That is, the route will work with any organization ID, whether or
		// not it exits.  This doesn't leak any information about the existence of organizations, so is
		// fine from a security perspective, but might be a little surprising.
		//
		// We may in future decide to scope provisioner daemons to organizations, so we'll keep the API
		// route as is.
		r.Route("/organizations/{organization}/provisionerdaemons", func(r chi.Router) {
			r.Use(
				api.provisionerDaemonsEnabledMW,
				apiKeyMiddlewareOptional,
				httpmw.ExtractProvisionerDaemonAuthenticated(httpmw.ExtractProvisionerAuthConfig{
					DB:       api.Database,
					Optional: true,
				}, api.ProvisionerDaemonPSK),
				// Either a user auth or provisioner auth is required
				// to move forward.
				httpmw.RequireAPIKeyOrProvisionerDaemonAuth(),
				httpmw.ExtractOrganizationParam(api.Database),
			)
			r.With(apiKeyMiddleware).Get("/", api.provisionerDaemons)
			r.With(apiKeyMiddlewareOptional).Get("/serve", api.provisionerDaemonServe)
		})
		r.Route("/templates/{template}/acl", func(r chi.Router) {
			r.Use(
				api.templateRBACEnabledMW,
				apiKeyMiddleware,
				httpmw.ExtractTemplateParam(api.Database),
			)
			r.Get("/available", api.templateAvailablePermissions)
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
				r.Use(httpmw.ExtractUserParam(options.Database))
				r.Get("/", api.workspaceQuota)
			})
		})
		r.Route("/appearance", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(
					apiKeyMiddlewareOptional,
					httpmw.ExtractWorkspaceAgentAndLatestBuild(httpmw.ExtractWorkspaceAgentAndLatestBuildConfig{
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
				api.autostopRequirementEnabledMW,
				apiKeyMiddleware,
				httpmw.ExtractUserParam(options.Database),
			)

			r.Get("/", api.userQuietHoursSchedule)
			r.Put("/", api.putUserQuietHoursSchedule)
		})
		r.Route("/integrations", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				api.jfrogEnabledMW,
			)

			r.Post("/jfrog/xray-scan", api.postJFrogXrayScan)
			r.Get("/jfrog/xray-scan", api.jFrogXrayScan)
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

	meshTLSConfig, err := replicasync.CreateDERPMeshTLSConfig(options.AccessURL.Hostname(), options.TLSCertificates)
	if err != nil {
		return nil, xerrors.Errorf("create DERP mesh TLS config: %w", err)
	}
	// We always want to run the replica manager even if we don't have DERP
	// enabled, since it's used to detect other coder servers for licensing.
	api.replicaManager, err = replicasync.New(ctx, options.Logger, options.Database, options.Pubsub, &replicasync.Options{
		ID:             api.AGPL.ID,
		RelayAddress:   options.DERPServerRelayAddress,
		RegionID:       int32(options.DERPServerRegionID),
		TLSConfig:      meshTLSConfig,
		UpdateInterval: options.ReplicaSyncUpdateInterval,
	})
	if err != nil {
		return nil, xerrors.Errorf("initialize replica: %w", err)
	}
	if api.DERPServer != nil {
		api.derpMesh = derpmesh.New(options.Logger.Named("derpmesh"), api.DERPServer, meshTLSConfig)
	}

	// Moon feature init. Proxyhealh is a go routine to periodically check
	// the health of all workspace proxies.
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

	// Wire this up to healthcheck.
	var fetchUpdater healthcheck.WorkspaceProxiesFetchUpdater = &workspaceProxiesFetchUpdater{
		fetchFunc:  api.fetchWorkspaceProxies,
		updateFunc: api.ProxyHealth.ForceUpdate,
	}
	api.AGPL.WorkspaceProxiesFetchUpdater.Store(&fetchUpdater)

	err = api.PrometheusRegistry.Register(&api.licenseMetricsCollector)
	if err != nil {
		return nil, xerrors.Errorf("unable to register license metrics collector")
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

	ExternalTokenEncryption []dbcrypt.Cipher

	// Used for high availability.
	ReplicaSyncUpdateInterval time.Duration
	ReplicaErrorGracePeriod   time.Duration
	DERPServerRelayAddress    string
	DERPServerRegionID        int

	// Used for user quiet hours schedules.
	DefaultQuietHoursSchedule string // cron schedule, if empty user quiet hours schedules are disabled

	EntitlementsUpdateInterval time.Duration
	ProxyHealthInterval        time.Duration
	LicenseKeys                map[string]ed25519.PublicKey

	// optional pre-shared key for authentication of external provisioner daemons
	ProvisionerDaemonPSK string

	CheckInactiveUsersCancelFunc func()
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

	provisionerDaemonAuth *provisionerDaemonAuth

	licenseMetricsCollector license.MetricsCollector
	tailnetService          *tailnet.ClientService
}

// writeEntitlementWarningsHeader writes the entitlement warnings to the response header
// for all authenticated users with roles. If there are no warnings, this header will not be written.
//
// This header is used by the CLI to display warnings to the user without having
// to make additional requests!
func (api *API) writeEntitlementWarningsHeader(a rbac.Subject, header http.Header) {
	roles, err := a.Roles.Expand()
	if err != nil {
		return
	}
	nonMemberRoles := 0
	for _, role := range roles {
		// The member role is implied, and not assignable.
		// If there is no display name, then the role is also unassigned.
		// This is not the ideal logic, but works for now.
		if role.Identifier == rbac.RoleMember() || (role.DisplayName == "") {
			continue
		}
		nonMemberRoles++
	}
	if nonMemberRoles == 0 {
		// Don't show entitlement warnings if the user
		// has no roles. This is a normal user!
		return
	}
	api.entitlementsMu.RLock()
	defer api.entitlementsMu.RUnlock()
	for _, warning := range api.entitlements.Warnings {
		header.Add(codersdk.EntitlementsWarningHeader, warning)
	}
}

func (api *API) Close() error {
	// Replica manager should be closed first. This is because the replica
	// manager updates the replica's table in the database when it closes.
	// This tells other Coderds that it is now offline.
	if api.replicaManager != nil {
		_ = api.replicaManager.Close()
	}
	api.cancel()
	if api.derpMesh != nil {
		_ = api.derpMesh.Close()
	}

	if api.Options.CheckInactiveUsersCancelFunc != nil {
		api.Options.CheckInactiveUsersCancelFunc()
	}
	return api.AGPL.Close()
}

func (api *API) updateEntitlements(ctx context.Context) error {
	api.entitlementsUpdateMu.Lock()
	defer api.entitlementsUpdateMu.Unlock()

	replicas := api.replicaManager.AllPrimary()
	agedReplicas := make([]database.Replica, 0, len(replicas))
	for _, replica := range replicas {
		// If a replica is less than the update interval old, we don't
		// want to display a warning. In the open-source version of Coder,
		// Kubernetes Pods will start up before shutting down the other,
		// and we don't want to display a warning in that case.
		//
		// Only display warnings for long-lived replicas!
		if dbtime.Now().Sub(replica.StartedAt) < api.ReplicaErrorGracePeriod {
			continue
		}
		agedReplicas = append(agedReplicas, replica)
	}

	entitlements, err := license.Entitlements(
		ctx, api.Database,
		len(agedReplicas), len(api.ExternalAuthConfigs), api.LicenseKeys, map[codersdk.FeatureName]bool{
			codersdk.FeatureAuditLog:                   api.AuditLogging,
			codersdk.FeatureBrowserOnly:                api.BrowserOnly,
			codersdk.FeatureSCIM:                       len(api.SCIMAPIKey) != 0,
			codersdk.FeatureMultipleExternalAuth:       len(api.ExternalAuthConfigs) > 1,
			codersdk.FeatureTemplateRBAC:               api.RBAC,
			codersdk.FeatureExternalTokenEncryption:    len(api.ExternalTokenEncryption) > 0,
			codersdk.FeatureExternalProvisionerDaemons: true,
			codersdk.FeatureAdvancedTemplateScheduling: true,
			codersdk.FeatureWorkspaceProxy:             true,
			codersdk.FeatureUserRoleManagement:         true,
			codersdk.FeatureAccessControl:              true,
			codersdk.FeatureControlSharedPorts:         true,
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
			committer := committer{
				Log:      api.Logger.Named("quota_committer"),
				Database: api.Database,
			}
			qcPtr := proto.QuotaCommitter(&committer)
			api.AGPL.QuotaCommitter.Store(&qcPtr)
		} else {
			api.AGPL.QuotaCommitter.Store(nil)
		}
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureAdvancedTemplateScheduling); shouldUpdate(initial, changed, enabled) {
		if enabled {
			templateStore := schedule.NewEnterpriseTemplateScheduleStore(api.AGPL.UserQuietHoursScheduleStore)
			templateStoreInterface := agplschedule.TemplateScheduleStore(templateStore)
			api.AGPL.TemplateScheduleStore.Store(&templateStoreInterface)

			if api.DefaultQuietHoursSchedule == "" {
				api.Logger.Warn(ctx, "template autostop requirement will default to UTC midnight as the default user quiet hours schedule. Set a custom default quiet hours schedule using CODER_QUIET_HOURS_DEFAULT_SCHEDULE to avoid this warning")
				api.DefaultQuietHoursSchedule = "CRON_TZ=UTC 0 0 * * *"
			}
			quietHoursStore, err := schedule.NewEnterpriseUserQuietHoursScheduleStore(api.DefaultQuietHoursSchedule, api.DeploymentValues.UserQuietHoursSchedule.AllowUserCustom.Value())
			if err != nil {
				api.Logger.Error(ctx, "unable to set up enterprise user quiet hours schedule store, template autostop requirements will not be applied to workspace builds", slog.Error(err))
			} else {
				api.AGPL.UserQuietHoursScheduleStore.Store(&quietHoursStore)
			}
		} else {
			templateStore := agplschedule.NewAGPLTemplateScheduleStore()
			api.AGPL.TemplateScheduleStore.Store(&templateStore)
			quietHoursStore := agplschedule.NewAGPLUserQuietHoursScheduleStore()
			api.AGPL.UserQuietHoursScheduleStore.Store(&quietHoursStore)
		}
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureHighAvailability); shouldUpdate(initial, changed, enabled) {
		var coordinator agpltailnet.Coordinator
		// If HA is enabled, but the database is in-memory, we can't actually
		// run HA and the PG coordinator. So throw a log line, and continue to use
		// the in memory AGPL coordinator.
		if enabled && api.DeploymentValues.InMemoryDatabase.Value() {
			api.Logger.Warn(ctx, "high availability is enabled, but cannot be configured due to the database being set to in-memory")
		}
		if enabled && !api.DeploymentValues.InMemoryDatabase.Value() {
			haCoordinator, err := tailnet.NewPGCoord(api.ctx, api.Logger, api.Pubsub, api.Database)
			if err != nil {
				api.Logger.Error(ctx, "unable to set up high availability coordinator", slog.Error(err))
				// If we try to setup the HA coordinator and it fails, nothing
				// is actually changing.
			} else {
				coordinator = haCoordinator
			}

			api.replicaManager.SetCallback(func() {
				// Only update DERP mesh if the built-in server is enabled.
				if api.Options.DeploymentValues.DERP.Server.Enable {
					addresses := make([]string, 0)
					for _, replica := range api.replicaManager.Regional() {
						// Don't add replicas with an empty relay address.
						if replica.RelayAddress == "" {
							continue
						}
						addresses = append(addresses, replica.RelayAddress)
					}
					api.derpMesh.SetAddresses(addresses, false)
				}
				_ = api.updateEntitlements(ctx)
			})
		} else {
			coordinator = agpltailnet.NewCoordinator(api.Logger)
			if api.Options.DeploymentValues.DERP.Server.Enable {
				api.derpMesh.SetAddresses([]string{}, false)
			}
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

	if initial, changed, enabled := featureChanged(codersdk.FeatureWorkspaceProxy); shouldUpdate(initial, changed, enabled) {
		if enabled {
			fn := derpMapper(api.Logger, api.ProxyHealth)
			api.AGPL.DERPMapper.Store(&fn)
		} else {
			api.AGPL.DERPMapper.Store(nil)
		}
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureAccessControl); shouldUpdate(initial, changed, enabled) {
		var acs agpldbauthz.AccessControlStore = agpldbauthz.AGPLTemplateAccessControlStore{}
		if enabled {
			acs = dbauthz.EnterpriseTemplateAccessControlStore{}
		}
		api.AGPL.AccessControlStore.Store(&acs)
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureAppearance); shouldUpdate(initial, changed, enabled) {
		if enabled {
			f := newAppearanceFetcher(
				api.Database,
				api.DeploymentValues.Support.Links.Value,
			)
			api.AGPL.AppearanceFetcher.Store(&f)
		} else {
			api.AGPL.AppearanceFetcher.Store(&appearance.DefaultFetcher)
		}
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureControlSharedPorts); shouldUpdate(initial, changed, enabled) {
		var ps agplportsharing.PortSharer = agplportsharing.DefaultPortSharer
		if enabled {
			ps = portsharing.NewEnterprisePortSharer()
		}
		api.AGPL.PortSharer.Store(&ps)
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureCustomRoles); shouldUpdate(initial, changed, enabled) {
		var handler coderd.CustomRoleHandler = &enterpriseCustomRoleHandler{API: api, Enabled: enabled}
		api.AGPL.CustomRoleHandler.Store(&handler)
	}

	if initial, changed, enabled := featureChanged(codersdk.FeatureMultipleOrganizations); shouldUpdate(initial, changed, enabled) {
		var handler coderd.CustomRoleHandler = &enterpriseCustomRoleHandler{API: api, Enabled: enabled}
		api.AGPL.CustomRoleHandler.Store(&handler)
	}

	// External token encryption is soft-enforced
	featureExternalTokenEncryption := entitlements.Features[codersdk.FeatureExternalTokenEncryption]
	featureExternalTokenEncryption.Enabled = len(api.ExternalTokenEncryption) > 0
	if featureExternalTokenEncryption.Enabled && featureExternalTokenEncryption.Entitlement != codersdk.EntitlementEntitled {
		msg := fmt.Sprintf("%s is enabled (due to setting external token encryption keys) but your license is not entitled to this feature.", codersdk.FeatureExternalTokenEncryption.Humanize())
		api.Logger.Warn(ctx, msg)
		entitlements.Warnings = append(entitlements.Warnings, msg)
	}
	entitlements.Features[codersdk.FeatureExternalTokenEncryption] = featureExternalTokenEncryption

	api.entitlementsMu.Lock()
	defer api.entitlementsMu.Unlock()
	api.entitlements = entitlements
	api.licenseMetricsCollector.Entitlements.Store(&entitlements)
	api.AGPL.SiteHandler.Entitlements.Store(&entitlements)
	return nil
}

// getProxyDERPStartingRegionID returns the starting region ID that should be
// used for workspace proxies. A proxy's actual region ID is the return value
// from this function + it's RegionID field.
//
// Two ints are returned, the first is the starting region ID for proxies, and
// the second is the maximum region ID that already exists in the DERP map.
func getProxyDERPStartingRegionID(derpMap *tailcfg.DERPMap) (sID int64, mID int64) {
	var maxRegionID int64
	for _, region := range derpMap.Regions {
		rid := int64(region.RegionID)
		if rid > maxRegionID {
			maxRegionID = rid
		}
	}
	if maxRegionID < 0 {
		maxRegionID = 0
	}

	// Round to the nearest 10,000 with a sufficient buffer of at least 2,000.
	// The buffer allows for future "fixed" regions to be added to the base DERP
	// map without conflicting with proxy region IDs (standard DERP maps usually
	// use incrementing IDs for new regions).
	//
	// Example:
	//  maxRegionID = -2_000 -> startingRegionID = 10_000
	//  maxRegionID = 8_000 -> startingRegionID = 10_000
	//  maxRegionID = 8_500 -> startingRegionID = 20_000
	//  maxRegionID = 12_000 -> startingRegionID = 20_000
	//  maxRegionID = 20_000 -> startingRegionID = 30_000
	const roundStartingRegionID = 10_000
	const startingRegionIDBuffer = 2_000
	// Add the buffer first.
	startingRegionID := maxRegionID + startingRegionIDBuffer
	// Round UP to the nearest 10,000. Go's math.Ceil rounds up to the nearest
	// integer, so we need to divide by 10,000 first and then multiply by
	// 10,000.
	startingRegionID = int64(math.Ceil(float64(startingRegionID)/roundStartingRegionID) * roundStartingRegionID)
	// This should never be hit but it's here just in case.
	if startingRegionID < roundStartingRegionID {
		startingRegionID = roundStartingRegionID
	}

	return startingRegionID, maxRegionID
}

var (
	lastDerpConflictMutex sync.Mutex
	lastDerpConflictLog   time.Time
)

func derpMapper(logger slog.Logger, proxyHealth *proxyhealth.ProxyHealth) func(*tailcfg.DERPMap) *tailcfg.DERPMap {
	return func(derpMap *tailcfg.DERPMap) *tailcfg.DERPMap {
		derpMap = derpMap.Clone()

		// Find the starting region ID that we'll use for proxies. This must be
		// deterministic based on the derp map.
		startingRegionID, largestRegionID := getProxyDERPStartingRegionID(derpMap)
		if largestRegionID >= 1<<32 {
			// Enforce an upper bound on the region ID. This shouldn't be hit in
			// practice, but it's a good sanity check.
			lastDerpConflictMutex.Lock()
			shouldLog := lastDerpConflictLog.IsZero() || time.Since(lastDerpConflictLog) > time.Minute
			if shouldLog {
				lastDerpConflictLog = time.Now()
			}
			lastDerpConflictMutex.Unlock()
			if shouldLog {
				logger.Warn(
					context.Background(),
					"existing DERP region IDs are too large, proxy region IDs will not be populated in the derp map. Please ensure that all DERP region IDs are less than 2^32",
					slog.F("largest_region_id", largestRegionID),
					slog.F("max_region_id", int64(1<<32-1)),
				)
				return derpMap
			}
		}

		// Add all healthy proxies to the DERP map.
		statusMap := proxyHealth.HealthStatus()
	statusLoop:
		for _, status := range statusMap {
			if status.Status != proxyhealth.Healthy || !status.Proxy.DerpEnabled {
				// Only add healthy proxies with DERP enabled to the DERP map.
				continue
			}

			u, err := url.Parse(status.Proxy.Url)
			if err != nil {
				// Not really any need to log, the proxy should be unreachable
				// anyways and filtered out by the above condition.
				continue
			}
			port := u.Port()
			if port == "" {
				port = "80"
				if u.Scheme == "https" {
					port = "443"
				}
			}
			portInt, err := strconv.Atoi(port)
			if err != nil {
				// Not really any need to log, the proxy should be unreachable
				// anyways and filtered out by the above condition.
				continue
			}

			// Sanity check that the region ID and code is unique.
			//
			// This should be impossible to hit as the IDs are enforced to be
			// unique by the database and the computed ID is greater than any
			// existing ID in the DERP map.
			regionID := int(startingRegionID) + int(status.Proxy.RegionID)
			regionCode := fmt.Sprintf("coder_%s", strings.ToLower(status.Proxy.Name))
			regionName := status.Proxy.DisplayName
			if regionName == "" {
				regionName = status.Proxy.Name
			}
			for _, r := range derpMap.Regions {
				if r.RegionID == regionID || r.RegionCode == regionCode {
					// Log a warning if we haven't logged one in the last
					// minute.
					lastDerpConflictMutex.Lock()
					shouldLog := lastDerpConflictLog.IsZero() || time.Since(lastDerpConflictLog) > time.Minute
					if shouldLog {
						lastDerpConflictLog = time.Now()
					}
					lastDerpConflictMutex.Unlock()
					if shouldLog {
						logger.Warn(context.Background(),
							"proxy region ID or code conflict, ignoring workspace proxy for DERP map",
							slog.F("proxy_id", status.Proxy.ID),
							slog.F("proxy_name", status.Proxy.Name),
							slog.F("proxy_display_name", status.Proxy.DisplayName),
							slog.F("proxy_url", status.Proxy.Url),
							slog.F("proxy_region_id", status.Proxy.RegionID),
							slog.F("proxy_computed_region_id", regionID),
							slog.F("proxy_computed_region_code", regionCode),
						)
					}

					continue statusLoop
				}
			}

			derpMap.Regions[regionID] = &tailcfg.DERPRegion{
				// EmbeddedRelay ONLY applies to the primary.
				EmbeddedRelay: false,
				RegionID:      regionID,
				RegionCode:    regionCode,
				RegionName:    regionName,
				Nodes: []*tailcfg.DERPNode{
					{
						Name:      fmt.Sprintf("%da", regionID),
						RegionID:  regionID,
						HostName:  u.Hostname(),
						DERPPort:  portInt,
						STUNPort:  -1,
						ForceHTTP: u.Scheme == "http",
					},
				},
			}
		}

		return derpMap
	}
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

	defer func() {
		// If this function ends, it means the context was canceled and this
		// coderd is shutting down. In this case, post a pubsub message to
		// tell other coderd's to resync their entitlements. This is required to
		// make sure things like replica counts are updated in the UI.
		// Ignore the error, as this is just a best effort. If it fails,
		// the system will eventually recover as replicas timeout
		// if their heartbeats stop. The best effort just tries to update the
		// UI faster if it succeeds.
		_ = api.Pubsub.Publish(PubsubEventLicenses, []byte("going away"))
	}()
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

func (api *API) Authorize(r *http.Request, action policy.Action, object rbac.Objecter) bool {
	return api.AGPL.HTTPAuth.Authorize(r, action, object)
}
