package coderd

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/go-chi/chi/v5"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd"
	agplaudit "github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/audit"
	"github.com/coder/coder/enterprise/audit/backends"
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
		AGPL:    coderd.New(options.Options),
		Options: options,

		activeUsers: codersdk.Feature{
			Entitlement: codersdk.EntitlementNotEntitled,
			Enabled:     false,
		},
		auditLogs:              codersdk.EntitlementNotEntitled,
		cancelEntitlementsLoop: cancelFunc,
	}
	oauthConfigs := &httpmw.OAuth2Configs{
		Github: options.GithubOAuth2Config,
		OIDC:   options.OIDCConfig,
	}
	apiKeyMiddleware := httpmw.ExtractAPIKey(options.Database, oauthConfigs, false)

	api.AGPL.APIHandler.Group(func(r chi.Router) {
		r.Get("/entitlements", api.entitlements)
		r.Route("/licenses", func(r chi.Router) {
			r.Use(apiKeyMiddleware)
			r.Post("/", api.postLicense)
			r.Get("/", api.licenses)
			r.Delete("/{id}", api.deleteLicense)
		})
	})

	err := api.updateEntitlements(ctx)
	if err != nil {
		return nil, xerrors.Errorf("update entitlements: %w", err)
	}
	api.closeLicenseSubscribe, err = api.Pubsub.Subscribe(pubSubEventLicenses, func(ctx context.Context, message []byte) {
		_ = api.updateEntitlements(ctx)
	})
	if err != nil {
		return nil, xerrors.Errorf("subscribe to license updates: %w", err)
	}
	go api.runEntitlementsLoop(ctx)

	return api, nil
}

type Options struct {
	*coderd.Options

	AuditLogging               bool
	EntitlementsUpdateInterval time.Duration
	Keys                       map[string]ed25519.PublicKey
}

type API struct {
	AGPL *coderd.API
	*Options

	closeLicenseSubscribe  func()
	cancelEntitlementsLoop func()
	mutex                  sync.RWMutex
	hasLicense             bool
	activeUsers            codersdk.Feature
	auditLogs              codersdk.Entitlement
}

func (api *API) Close() error {
	api.closeLicenseSubscribe()
	api.cancelEntitlementsLoop()
	return api.AGPL.Close()
}

func (api *API) runEntitlementsLoop(ctx context.Context) {
	ticker := time.NewTicker(api.EntitlementsUpdateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		err := api.updateEntitlements(ctx)
		if err != nil {
			api.Logger.Warn(ctx, "failed to get feature entitlements", slog.Error(err))
			continue
		}
	}
}

func (api *API) updateEntitlements(ctx context.Context) error {
	licenses, err := api.Database.GetUnexpiredLicenses(ctx)
	if err != nil {
		return err
	}
	api.mutex.Lock()
	defer api.mutex.Unlock()
	now := time.Now()

	// Default all entitlements to be disabled.
	hasLicense := false
	activeUsers := codersdk.Feature{
		Enabled:     false,
		Entitlement: codersdk.EntitlementNotEntitled,
	}
	auditLogs := codersdk.EntitlementNotEntitled

	// Here we loop through licenses to detect enabled features.
	for _, l := range licenses {
		claims, err := validateDBLicense(l, api.Keys)
		if err != nil {
			api.Logger.Debug(ctx, "skipping invalid license",
				slog.F("id", l.ID), slog.Error(err))
			continue
		}
		hasLicense = true
		entitlement := codersdk.EntitlementEntitled
		if now.After(claims.LicenseExpires.Time) {
			// if the grace period were over, the validation fails, so if we are after
			// LicenseExpires we must be in grace period.
			entitlement = codersdk.EntitlementGracePeriod
		}
		if claims.Features.UserLimit > 0 {
			activeUsers.Enabled = true
			activeUsers.Entitlement = entitlement
			currentLimit := int64(0)
			if activeUsers.Limit != nil {
				currentLimit = *activeUsers.Limit
			}
			limit := max(currentLimit, claims.Features.UserLimit)
			activeUsers.Limit = &limit
		}
		if claims.Features.AuditLog > 0 {
			auditLogs = entitlement
		}
	}

	if auditLogs != api.auditLogs {
		auditor := agplaudit.NewNop()
		// A flag could be added to the options that would allow disabling
		// enhanced audit logging here!
		if api.auditLogs == codersdk.EntitlementEntitled && api.AuditLogging {
			auditor = audit.NewAuditor(
				audit.DefaultFilter,
				backends.NewPostgres(api.Database, true),
				backends.NewSlog(api.Logger),
			)
		}
		api.AGPL.Auditor.Store(auditor)
	}

	api.hasLicense = hasLicense
	api.activeUsers = activeUsers
	api.auditLogs = auditLogs

	return nil
}

func (api *API) entitlements(rw http.ResponseWriter, r *http.Request) {
	api.mutex.RLock()
	hasLicense := api.hasLicense
	activeUsers := api.activeUsers
	auditLogs := api.auditLogs
	api.mutex.RUnlock()

	resp := codersdk.Entitlements{
		Features:   make(map[string]codersdk.Feature),
		Warnings:   make([]string, 0),
		HasLicense: hasLicense,
	}

	if activeUsers.Limit != nil {
		activeUserCount, err := api.Database.GetActiveUserCount(r.Context())
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Unable to query database",
				Detail:  err.Error(),
			})
			return
		}
		activeUsers.Actual = &activeUserCount
		if activeUserCount > *activeUsers.Limit {
			resp.Warnings = append(resp.Warnings,
				fmt.Sprintf(
					"Your deployment has %d active users but is only licensed for %d.",
					activeUserCount, *activeUsers.Limit))
		}
	}
	resp.Features[codersdk.FeatureUserLimit] = activeUsers

	// Audit logs
	resp.Features[codersdk.FeatureAuditLog] = codersdk.Feature{
		Entitlement: auditLogs,
		Enabled:     api.AuditLogging,
	}
	if auditLogs == codersdk.EntitlementGracePeriod && api.AuditLogging {
		resp.Warnings = append(resp.Warnings,
			"Audit logging is enabled but your license for this feature is expired.")
	}

	httpapi.Write(rw, http.StatusOK, resp)
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
