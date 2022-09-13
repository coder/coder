package coderd

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/coder/coder/enterprise/audit/backends"

	"github.com/cenkalti/backoff/v4"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	agpl "github.com/coder/coder/coderd"
	agplAudit "github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/features"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/audit"
)

type Enablements struct {
	AuditLogs bool
}

type featuresService struct {
	logger         slog.Logger
	database       database.Store
	pubsub         database.Pubsub
	keys           map[string]ed25519.PublicKey
	enablements    Enablements
	resyncInterval time.Duration
	// enabledImplementations includes an "enabled" implementation of every feature.  This is
	// initialized at start of day and remains static.  The consequence of this is that these things
	// are hanging around using memory even if not licensed or in use, but it greatly simplifies the
	// logic because we don't have to bother creating and destroying them as entitlements change.
	// If we have a particularly memory-hungry feature in future, we might wish to reconsider this
	// choice.
	enabledImplementations agpl.FeatureInterfaces

	mu           sync.RWMutex
	entitlements entitlements
}

// newFeaturesService creates a FeaturesService and starts it.  It will continue running for the
// duration of the passed ctx.
func newFeaturesService(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	pubsub database.Pubsub,
	enablements Enablements,
) features.Service {
	fs := &featuresService{
		logger:      logger,
		database:    db,
		pubsub:      pubsub,
		keys:        keys,
		enablements: enablements,
		enabledImplementations: agpl.FeatureInterfaces{
			Auditor: audit.NewAuditor(
				audit.DefaultFilter,
				backends.NewPostgres(db, true),
				backends.NewSlog(logger),
			),
		},
		resyncInterval: 10 * time.Minute,
		entitlements: entitlements{
			activeUsers: numericalEntitlement{
				entitlementLimit: entitlementLimit{
					unlimited: true,
				},
			},
		},
	}
	go fs.syncEntitlements(ctx)
	return fs
}

func (s *featuresService) EntitlementsAPI(rw http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	e := s.entitlements
	s.mu.RUnlock()

	resp := codersdk.Entitlements{
		Features:   make(map[string]codersdk.Feature),
		Warnings:   make([]string, 0),
		HasLicense: e.hasLicense,
	}

	// User limit
	uf := codersdk.Feature{
		Entitlement: e.activeUsers.state.toSDK(),
		Enabled:     true,
	}
	if !e.activeUsers.unlimited {
		n, err := s.database.GetActiveUserCount(r.Context())
		if err != nil {
			httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Unable to query database",
				Detail:  err.Error(),
			})
			return
		}
		uf.Actual = &n
		uf.Limit = &e.activeUsers.limit
		if n > e.activeUsers.limit {
			resp.Warnings = append(resp.Warnings,
				fmt.Sprintf(
					"Your deployment has %d active users but is only licensed for %d.",
					n, e.activeUsers.limit))
		}
	}
	resp.Features[codersdk.FeatureUserLimit] = uf

	// Audit logs
	resp.Features[codersdk.FeatureAuditLog] = codersdk.Feature{
		Entitlement: e.auditLogs.state.toSDK(),
		Enabled:     s.enablements.AuditLogs,
	}
	if e.auditLogs.state == gracePeriod && s.enablements.AuditLogs {
		resp.Warnings = append(resp.Warnings,
			"Audit logging is enabled but your license for this feature is expired.")
	}

	httpapi.Write(rw, http.StatusOK, resp)
}

type entitlementState int

const (
	notEntitled entitlementState = iota
	gracePeriod
	entitled
)

type entitlementLimit struct {
	unlimited bool
	limit     int64
}

type entitlement struct {
	state entitlementState
}

func (s entitlementState) toSDK() codersdk.Entitlement {
	switch s {
	case notEntitled:
		return codersdk.EntitlementNotEntitled
	case gracePeriod:
		return codersdk.EntitlementGracePeriod
	case entitled:
		return codersdk.EntitlementEntitled
	default:
		panic("unknown entitlementState")
	}
}

type numericalEntitlement struct {
	entitlement
	entitlementLimit
}

type entitlements struct {
	hasLicense  bool
	activeUsers numericalEntitlement
	auditLogs   entitlement
}

func (s *featuresService) getEntitlements(ctx context.Context) (entitlements, error) {
	licenses, err := s.database.GetUnexpiredLicenses(ctx)
	if err != nil {
		return entitlements{}, err
	}
	now := time.Now()
	e := entitlements{
		activeUsers: numericalEntitlement{
			entitlementLimit: entitlementLimit{
				unlimited: true,
			},
		},
	}
	for _, l := range licenses {
		claims, err := validateDBLicense(l, s.keys)
		if err != nil {
			s.logger.Debug(ctx, "skipping invalid license",
				slog.F("id", l.ID), slog.Error(err))
			continue
		}
		e.hasLicense = true
		thisEntitlement := entitled
		if now.After(claims.LicenseExpires.Time) {
			// if the grace period were over, the validation fails, so if we are after
			// LicenseExpires we must be in grace period.
			thisEntitlement = gracePeriod
		}
		if claims.Features.UserLimit > 0 {
			e.activeUsers.state = thisEntitlement
			e.activeUsers.unlimited = false
			e.activeUsers.limit = max(e.activeUsers.limit, claims.Features.UserLimit)
		}
		if claims.Features.AuditLog > 0 {
			e.auditLogs.state = thisEntitlement
		}
	}
	return e, nil
}

func (s *featuresService) syncEntitlements(ctx context.Context) {
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
			cancel, err := s.pubsub.Subscribe(PubSubEventLicenses, func(_ context.Context, _ []byte) {
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
				s.logger.Warn(ctx, "failed to subscribe to license updates", slog.Error(err))
				time.Sleep(b.NextBackOff())
				continue
			}
			// nolint: revive
			defer cancel()
			subscribed = true
			s.logger.Debug(ctx, "successfully subscribed to pubsub")
		}

		s.logger.Info(ctx, "syncing licensed entitlements")
		ents, err := s.getEntitlements(ctx)
		if err != nil {
			s.logger.Warn(ctx, "failed to get feature entitlements", slog.Error(err))
			time.Sleep(b.NextBackOff())
			continue
		}
		b.Reset()

		s.mu.Lock()
		s.entitlements = ents
		s.mu.Unlock()
		s.logger.Debug(ctx, "synced licensed entitlements")

		select {
		case <-ctx.Done():
			return
		case <-time.After(s.resyncInterval):
			continue
		case <-updates:
			s.logger.Debug(ctx, "got pubsub update")
			continue
		}
	}
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (s *featuresService) Get(ps any) error {
	if reflect.TypeOf(ps).Kind() != reflect.Pointer {
		return xerrors.New("input must be pointer to struct")
	}
	vs := reflect.ValueOf(ps).Elem()
	if vs.Kind() != reflect.Struct {
		return xerrors.New("input must be pointer to struct")
	}
	// grab a local copy of entitlements so that we have a consistent set, but aren't keeping it
	// locked from updates while we process.
	s.mu.RLock()
	ent := s.entitlements
	s.mu.RUnlock()

	for i := 0; i < vs.NumField(); i++ {
		vf := vs.Field(i)
		tf := vf.Type()
		if tf.Kind() != reflect.Interface {
			return xerrors.Errorf("fields of input struct must be interfaces: %s", tf.String())
		}

		err := s.setImplementation(ent, vf, tf)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *featuresService) setImplementation(ent entitlements, vf reflect.Value, tf reflect.Type) error {
	// c.f. https://stackoverflow.com/questions/7132848/how-to-get-the-reflect-type-of-an-interface
	switch tf {
	case reflect.TypeOf((*agplAudit.Auditor)(nil)).Elem():
		// Audit logging
		if !s.enablements.AuditLogs || ent.auditLogs.state == notEntitled {
			vf.Set(reflect.ValueOf(agpl.DisabledImplementations.Auditor))
			return nil
		}
		vf.Set(reflect.ValueOf(s.enabledImplementations.Auditor))
		return nil
	default:
		return xerrors.Errorf("unable to find implementation of interface %s", tf.String())
	}
}
