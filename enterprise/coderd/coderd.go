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
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/enterprise/derpmesh"
	"github.com/coder/coder/enterprise/replicasync"
	"github.com/coder/coder/enterprise/tailnet"
	"github.com/coder/coder/provisionerd/proto"
	agpltailnet "github.com/coder/coder/tailnet"
)

// New constructs an Enterprise coderd API instance.
// This handler is designed to wrap the AGPL Coder code and
// layer Enterprise functionality on top as much as possible.
func New(ctx context.Context, options *Options) (*API, error) {
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
		AGPL:                   coderd.New(options.Options),
		Options:                options,
		cancelEntitlementsLoop: cancelFunc,
	}

	api.AGPL.Options.SetUserGroups = api.setUserGroups

	oauthConfigs := &httpmw.OAuth2Configs{
		Github: options.GithubOAuth2Config,
		OIDC:   options.OIDCConfig,
	}
	apiKeyMiddleware := httpmw.ExtractAPIKey(httpmw.ExtractAPIKeyConfig{
		DB:              options.Database,
		OAuth2Configs:   oauthConfigs,
		RedirectToLogin: false,
	})

	api.AGPL.APIHandler.Group(func(r chi.Router) {
		r.Get("/entitlements", api.serveEntitlements)
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
			r.Use(
				apiKeyMiddleware,
			)
			r.Get("/", api.appearance)
			r.Put("/", api.putAppearance)
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
	var err error
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

	EntitlementsUpdateInterval time.Duration
	Keys                       map[string]ed25519.PublicKey
}

type API struct {
	AGPL *coderd.API
	*Options

	// Detects multiple Coder replicas running at the same time.
	replicaManager *replicasync.Manager
	// Meshes DERP connections from multiple replicas.
	derpMesh *derpmesh.Mesh

	cancelEntitlementsLoop func()
	entitlementsMu         sync.RWMutex
	entitlements           codersdk.Entitlements
}

func (api *API) Close() error {
	api.cancelEntitlementsLoop()
	_ = api.replicaManager.Close()
	_ = api.derpMesh.Close()
	return api.AGPL.Close()
}

func (api *API) updateEntitlements(ctx context.Context) error {
	api.entitlementsMu.Lock()
	defer api.entitlementsMu.Unlock()

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
		})
	if err != nil {
		return err
	}

	if entitlements.RequireTelemetry && !api.DeploymentConfig.Telemetry.Enable.Value {
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

	entitlements.Experimental = api.DeploymentConfig.Experimental.Value || len(api.AGPL.Experiments) != 0

	featureChanged := func(featureName codersdk.FeatureName) (changed bool, enabled bool) {
		if api.entitlements.Features == nil {
			return true, entitlements.Features[featureName].Enabled
		}
		oldFeature := api.entitlements.Features[featureName]
		newFeature := entitlements.Features[featureName]
		if oldFeature.Enabled != newFeature.Enabled {
			return true, newFeature.Enabled
		}
		return false, newFeature.Enabled
	}

	if changed, enabled := featureChanged(codersdk.FeatureAuditLog); changed {
		auditor := agplaudit.NewNop()
		if enabled {
			auditor = api.AGPL.Options.Auditor
		}
		api.AGPL.Auditor.Store(&auditor)
	}

	if changed, enabled := featureChanged(codersdk.FeatureBrowserOnly); changed {
		var handler func(rw http.ResponseWriter) bool
		if enabled {
			handler = api.shouldBlockNonBrowserConnections
		}
		api.AGPL.WorkspaceClientCoordinateOverride.Store(&handler)
	}

	if changed, enabled := featureChanged(codersdk.FeatureTemplateRBAC); changed {
		if enabled {
			committer := committer{Database: api.Database}
			ptr := proto.QuotaCommitter(&committer)
			api.AGPL.QuotaCommitter.Store(&ptr)
		} else {
			api.AGPL.QuotaCommitter.Store(nil)
		}
	}

	if changed, enabled := featureChanged(codersdk.FeatureHighAvailability); changed {
		coordinator := agpltailnet.NewCoordinator()
		if enabled {
			haCoordinator, err := tailnet.NewCoordinator(api.Logger, api.Pubsub)
			if err != nil {
				api.Logger.Error(ctx, "unable to set up high availability coordinator", slog.Error(err))
				// If we try to setup the HA coordinator and it fails, nothing
				// is actually changing.
				changed = false
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
			api.derpMesh.SetAddresses([]string{}, false)
			api.replicaManager.SetCallback(func() {
				// If the amount of replicas change, so should our entitlements.
				// This is to display a warning in the UI if the user is unlicensed.
				_ = api.updateEntitlements(ctx)
			})
		}

		// Recheck changed in case the HA coordinator failed to set up.
		if changed {
			oldCoordinator := *api.AGPL.TailnetCoordinator.Swap(&coordinator)
			err := oldCoordinator.Close()
			if err != nil {
				api.Logger.Error(ctx, "close old tailnet coordinator", slog.Error(err))
			}
		}
	}

	api.entitlements = entitlements

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
