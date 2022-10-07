package coderd

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-chi/chi/v5"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd"
	agplaudit "github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/workspacequota"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/audit"
	"github.com/coder/coder/enterprise/audit/backends"
	"github.com/coder/coder/enterprise/coderd/license"
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
	ctx, cancelFunc := context.WithCancel(ctx)
	api := &API{
		AGPL:                   coderd.New(options.Options),
		Options:                options,
		cancelEntitlementsLoop: cancelFunc,
	}
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
		r.Route("/licenses", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Post("/", api.postLicense)
			r.Get("/", api.licenses)
			r.Delete("/{id}", api.deleteLicense)
		})
		r.Route("/workspace-quota", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Route("/{user}", func(r chi.Router) {
				r.Use(httpmw.ExtractUserParam(options.Database))
				r.Get("/", api.workspaceQuota)
			})
		})
	})

	if len(options.SCIMAPIKey) != 0 {
		api.AGPL.RootHandler.Route("/scim/v2", func(r chi.Router) {
			r.Use(api.scimEnabledMW)
			r.Post("/Users", api.scimPostUser)
			r.Route("/Users", func(r chi.Router) {
				r.Get("/", api.scimGetUsers)
				r.Post("/", api.scimPostUser)
				r.Get("/{id}", api.scimGetUser)
				r.Patch("/{id}", api.scimPatchUser)
			})
		})
	}

	err := api.updateEntitlements(ctx)
	if err != nil {
		return nil, xerrors.Errorf("update entitlements: %w", err)
	}
	go api.runEntitlementsLoop(ctx)

	return api, nil
}

type Options struct {
	*coderd.Options

	AuditLogging bool
	// Whether to block non-browser connections.
	BrowserOnly        bool
	SCIMAPIKey         []byte
	UserWorkspaceQuota int

	EntitlementsUpdateInterval time.Duration
	Keys                       map[string]ed25519.PublicKey
}

type API struct {
	AGPL *coderd.API
	*Options

	cancelEntitlementsLoop func()
	entitlementsMu         sync.RWMutex
	entitlements           codersdk.Entitlements
}

func (api *API) Close() error {
	api.cancelEntitlementsLoop()
	return api.AGPL.Close()
}

func (api *API) updateEntitlements(ctx context.Context) error {
	api.entitlementsMu.Lock()
	defer api.entitlementsMu.Unlock()

	entitlements, err := license.Entitlements(ctx, api.Database, api.Logger, api.Keys, map[string]bool{
		codersdk.FeatureAuditLog:       api.AuditLogging,
		codersdk.FeatureBrowserOnly:    api.BrowserOnly,
		codersdk.FeatureSCIM:           len(api.SCIMAPIKey) != 0,
		codersdk.FeatureWorkspaceQuota: api.UserWorkspaceQuota != 0,
	})
	if err != nil {
		return err
	}

	featureChanged := func(featureName string) (changed bool, enabled bool) {
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
			auditor = audit.NewAuditor(
				audit.DefaultFilter,
				backends.NewPostgres(api.Database, true),
				backends.NewSlog(api.Logger),
			)
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

	if changed, enabled := featureChanged(codersdk.FeatureWorkspaceQuota); changed {
		enforcer := workspacequota.NewNop()
		if enabled {
			enforcer = NewEnforcer(api.Options.UserWorkspaceQuota)
		}
		api.AGPL.WorkspaceQuotaEnforcer.Store(&enforcer)
	}

	api.entitlements = entitlements

	return nil
}

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
