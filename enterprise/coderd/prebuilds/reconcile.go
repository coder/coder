package prebuilds

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"

	"cdr.dev/slog"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

type StoreReconciler struct {
	store      database.Store
	cfg        codersdk.PrebuildsConfig
	pubsub     pubsub.Pubsub
	logger     slog.Logger
	clock      quartz.Clock
	registerer prometheus.Registerer
	metrics    *MetricsCollector
	notifEnq   notifications.Enqueuer

	cancelFn          context.CancelCauseFunc
	running           atomic.Bool
	stopped           atomic.Bool
	done              chan struct{}
	provisionNotifyCh chan database.ProvisionerJob
}

var _ prebuilds.ReconciliationOrchestrator = &StoreReconciler{}

func NewStoreReconciler(store database.Store,
	ps pubsub.Pubsub,
	cfg codersdk.PrebuildsConfig,
	logger slog.Logger,
	clock quartz.Clock,
	registerer prometheus.Registerer,
	notifEnq notifications.Enqueuer,
) *StoreReconciler {
	reconciler := &StoreReconciler{
		store:             store,
		pubsub:            ps,
		logger:            logger,
		cfg:               cfg,
		clock:             clock,
		registerer:        registerer,
		notifEnq:          notifEnq,
		done:              make(chan struct{}, 1),
		provisionNotifyCh: make(chan database.ProvisionerJob, 10),
	}

	if registerer != nil {
		reconciler.metrics = NewMetricsCollector(store, logger, reconciler)
		if err := registerer.Register(reconciler.metrics); err != nil {
			// If the registerer fails to register the metrics collector, it's not fatal.
			logger.Error(context.Background(), "failed to register prometheus metrics", slog.Error(err))
		}
	}

	return reconciler
}

func (c *StoreReconciler) Run(ctx context.Context) {
	reconciliationInterval := c.cfg.ReconciliationInterval.Value()
	if reconciliationInterval <= 0 { // avoids a panic
		reconciliationInterval = 5 * time.Minute
	}

	c.logger.Info(ctx, "starting reconciler",
		slog.F("interval", reconciliationInterval),
		slog.F("backoff_interval", c.cfg.ReconciliationBackoffInterval.String()),
		slog.F("backoff_lookback", c.cfg.ReconciliationBackoffLookback.String()))

	var wg sync.WaitGroup
	ticker := c.clock.NewTicker(reconciliationInterval)
	defer ticker.Stop()
	defer func() {
		wg.Wait()
		c.done <- struct{}{}
	}()

	// nolint:gocritic // Reconciliation Loop needs Prebuilds Orchestrator permissions.
	ctx, cancel := context.WithCancelCause(dbauthz.AsPrebuildsOrchestrator(ctx))
	c.cancelFn = cancel

	// Start updating metrics in the background.
	if c.metrics != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.metrics.BackgroundFetch(ctx, metricsUpdateInterval, metricsUpdateTimeout)
		}()
	}

	// Everything is in place, reconciler can now be considered as running.
	//
	// NOTE: without this atomic bool, Stop might race with Run for the c.cancelFn above.
	c.running.Store(true)

	// Publish provisioning jobs outside of database transactions.
	// A connection is held while a database transaction is active; PGPubsub also tries to acquire a new connection on
	// Publish, so we can exhaust available connections.
	//
	// A single worker dequeues from the channel, which should be sufficient.
	// If any messages are missed due to congestion or errors, provisionerdserver has a backup polling mechanism which
	// will periodically pick up any queued jobs (see poll(time.Duration) in coderd/provisionerdserver/acquirer.go).
	go func() {
		for {
			select {
			case <-c.done:
				return
			case <-ctx.Done():
				return
			case job := <-c.provisionNotifyCh:
				err := provisionerjobs.PostJob(c.pubsub, job)
				if err != nil {
					c.logger.Error(ctx, "failed to post provisioner job to pubsub", slog.Error(err))
				}
			}
		}
	}()

	for {
		select {
		// TODO: implement pubsub listener to allow reconciling a specific template imperatively once it has been changed,
		//		 instead of waiting for the next reconciliation interval
		case <-ticker.C:
			// Trigger a new iteration on each tick.
			err := c.ReconcileAll(ctx)
			if err != nil {
				c.logger.Error(context.Background(), "reconciliation failed", slog.Error(err))
			}
		case <-ctx.Done():
			// nolint:gocritic // it's okay to use slog.F() for an error in this case
			// because we want to differentiate two different types of errors: ctx.Err() and context.Cause()
			c.logger.Warn(
				context.Background(),
				"reconciliation loop exited",
				slog.Error(ctx.Err()),
				slog.F("cause", context.Cause(ctx)),
			)
			return
		}
	}
}

func (c *StoreReconciler) Stop(ctx context.Context, cause error) {
	defer c.running.Store(false)

	if cause != nil {
		c.logger.Error(context.Background(), "stopping reconciler due to an error", slog.Error(cause))
	} else {
		c.logger.Info(context.Background(), "gracefully stopping reconciler")
	}

	// If previously stopped (Swap returns previous value), then short-circuit.
	//
	// NOTE: we need to *prospectively* mark this as stopped to prevent Stop being called multiple times and causing problems.
	if c.stopped.Swap(true) {
		return
	}

	// Unregister the metrics collector.
	if c.metrics != nil && c.registerer != nil {
		if !c.registerer.Unregister(c.metrics) {
			// The API doesn't allow us to know why the de-registration failed, but it's not very consequential.
			// The only time this would be an issue is if the premium license is removed, leading to the feature being
			// disabled (and consequently this Stop method being called), and then adding a new license which enables the
			// feature again. If the metrics cannot be registered, it'll log an error from NewStoreReconciler.
			c.logger.Warn(context.Background(), "failed to unregister metrics collector")
		}
	}

	// If the reconciler is not running, there's nothing else to do.
	if !c.running.Load() {
		return
	}

	if c.cancelFn != nil {
		c.cancelFn(cause)
	}

	select {
	// Give up waiting for control loop to exit.
	case <-ctx.Done():
		// nolint:gocritic // it's okay to use slog.F() for an error in this case
		// because we want to differentiate two different types of errors: ctx.Err() and context.Cause()
		c.logger.Error(
			context.Background(),
			"reconciler stop exited prematurely",
			slog.Error(ctx.Err()),
			slog.F("cause", context.Cause(ctx)),
		)
	// Wait for the control loop to exit.
	case <-c.done:
		c.logger.Info(context.Background(), "reconciler stopped")
	}
}

// ReconcileAll will attempt to resolve the desired vs actual state of all templates which have presets with prebuilds configured.
//
// NOTE:
//
// This function will kick of n provisioner jobs, based on the calculated state modifications.
//
// These provisioning jobs are fire-and-forget. We DO NOT wait for the prebuilt workspaces to complete their
// provisioning. As a consequence, it's possible that another reconciliation run will occur, which will mean that
// multiple preset versions could be reconciling at once. This may mean some temporary over-provisioning, but the
// reconciliation loop will bring these resources back into their desired numbers in an EVENTUALLY-consistent way.
//
// For example: we could decide to provision 1 new instance in this reconciliation.
// While that workspace is being provisioned, another template version is created which means this same preset will
// be reconciled again, leading to another workspace being provisioned. Two workspace builds will be occurring
// simultaneously for the same preset, but once both jobs have completed the reconciliation loop will notice the
// extraneous instance and delete it.
func (c *StoreReconciler) ReconcileAll(ctx context.Context) error {
	logger := c.logger.With(slog.F("reconcile_context", "all"))

	select {
	case <-ctx.Done():
		logger.Warn(context.Background(), "reconcile exiting prematurely; context done", slog.Error(ctx.Err()))
		return nil
	default:
	}

	logger.Debug(ctx, "starting reconciliation")

	err := c.WithReconciliationLock(ctx, logger, func(ctx context.Context, db database.Store) error {
		snapshot, err := c.SnapshotState(ctx, db)
		if err != nil {
			return xerrors.Errorf("determine current snapshot: %w", err)
		}

		c.reportHardLimitedPresets(snapshot)

		if len(snapshot.Presets) == 0 {
			logger.Debug(ctx, "no templates found with prebuilds configured")
			return nil
		}

		var eg errgroup.Group
		// Reconcile presets in parallel. Each preset in its own goroutine.
		for _, preset := range snapshot.Presets {
			ps, err := snapshot.FilterByPreset(preset.ID)
			if err != nil {
				logger.Warn(ctx, "failed to find preset snapshot", slog.Error(err), slog.F("preset_id", preset.ID.String()))
				continue
			}

			eg.Go(func() error {
				// Pass outer context.
				err = c.ReconcilePreset(ctx, *ps)
				if err != nil {
					logger.Error(
						ctx,
						"failed to reconcile prebuilds for preset",
						slog.Error(err),
						slog.F("preset_id", preset.ID),
					)
				}
				// DO NOT return error otherwise the tx will end.
				return nil
			})
		}

		// Release lock only when all preset reconciliation goroutines are finished.
		return eg.Wait()
	})
	if err != nil {
		logger.Error(ctx, "failed to reconcile", slog.Error(err))
	}

	return err
}

func (c *StoreReconciler) reportHardLimitedPresets(snapshot *prebuilds.GlobalSnapshot) {
	// presetsMap is a map from key (orgName:templateName:presetName) to list of corresponding presets.
	// Multiple versions of a preset can exist with the same orgName, templateName, and presetName,
	// because templates can have multiple versions — or deleted templates can share the same name.
	presetsMap := make(map[hardLimitedPresetKey][]database.GetTemplatePresetsWithPrebuildsRow)
	for _, preset := range snapshot.Presets {
		key := hardLimitedPresetKey{
			orgName:      preset.OrganizationName,
			templateName: preset.TemplateName,
			presetName:   preset.Name,
		}

		presetsMap[key] = append(presetsMap[key], preset)
	}

	// Report a preset as hard-limited only if all the following conditions are met:
	// - The preset is marked as hard-limited
	// - The preset is using the active version of its template, and the template has not been deleted
	//
	// The second condition is important because a hard-limited preset that has become outdated is no longer relevant.
	// Its associated prebuilt workspaces were likely deleted, and it's not meaningful to continue reporting it
	// as hard-limited to the admin.
	//
	// This approach accounts for all relevant scenarios:
	// Scenario #1: The admin created a new template version with the same preset names.
	// Scenario #2: The admin created a new template version and renamed the presets.
	// Scenario #3: The admin deleted a template version that contained hard-limited presets.
	//
	// In all of these cases, only the latest and non-deleted presets will be reported.
	// All other presets will be ignored and eventually removed from Prometheus.
	isPresetHardLimited := make(map[hardLimitedPresetKey]bool)
	for key, presets := range presetsMap {
		for _, preset := range presets {
			if preset.UsingActiveVersion && !preset.Deleted && snapshot.IsHardLimited(preset.ID) {
				isPresetHardLimited[key] = true
				break
			}
		}
	}

	c.metrics.registerHardLimitedPresets(isPresetHardLimited)
}

// SnapshotState captures the current state of all prebuilds across templates.
func (c *StoreReconciler) SnapshotState(ctx context.Context, store database.Store) (*prebuilds.GlobalSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var state prebuilds.GlobalSnapshot

	err := store.InTx(func(db database.Store) error {
		// TODO: implement template-specific reconciliations later
		presetsWithPrebuilds, err := db.GetTemplatePresetsWithPrebuilds(ctx, uuid.NullUUID{})
		if err != nil {
			return xerrors.Errorf("failed to get template presets with prebuilds: %w", err)
		}
		if len(presetsWithPrebuilds) == 0 {
			return nil
		}

		allRunningPrebuilds, err := db.GetRunningPrebuiltWorkspaces(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get running prebuilds: %w", err)
		}

		allPrebuildsInProgress, err := db.CountInProgressPrebuilds(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get prebuilds in progress: %w", err)
		}

		presetsBackoff, err := db.GetPresetsBackoff(ctx, c.clock.Now().Add(-c.cfg.ReconciliationBackoffLookback.Value()))
		if err != nil {
			return xerrors.Errorf("failed to get backoffs for presets: %w", err)
		}

		hardLimitedPresets, err := db.GetPresetsAtFailureLimit(ctx, c.cfg.FailureHardLimit.Value())
		if err != nil {
			return xerrors.Errorf("failed to get hard limited presets: %w", err)
		}

		state = prebuilds.NewGlobalSnapshot(
			presetsWithPrebuilds,
			allRunningPrebuilds,
			allPrebuildsInProgress,
			presetsBackoff,
			hardLimitedPresets,
		)
		return nil
	}, &database.TxOptions{
		Isolation:    sql.LevelRepeatableRead, // This mirrors the MVCC snapshotting Postgres does when using CTEs
		ReadOnly:     true,
		TxIdentifier: "prebuilds_state_determination",
	})

	return &state, err
}

func (c *StoreReconciler) ReconcilePreset(ctx context.Context, ps prebuilds.PresetSnapshot) error {
	logger := c.logger.With(
		slog.F("template_id", ps.Preset.TemplateID.String()),
		slog.F("template_name", ps.Preset.TemplateName),
		slog.F("template_version_id", ps.Preset.TemplateVersionID),
		slog.F("template_version_name", ps.Preset.TemplateVersionName),
		slog.F("preset_id", ps.Preset.ID),
		slog.F("preset_name", ps.Preset.Name),
	)

	// If the preset reached the hard failure limit for the first time during this iteration:
	// - Mark it as hard-limited in the database
	// - Send notifications to template admins
	// - Continue execution, we disallow only creation operation for hard-limited presets. Deletion is allowed.
	if ps.Preset.PrebuildStatus != database.PrebuildStatusHardLimited && ps.IsHardLimited {
		logger.Warn(ctx, "preset is hard limited, notifying template admins")

		err := c.store.UpdatePresetPrebuildStatus(ctx, database.UpdatePresetPrebuildStatusParams{
			Status:   database.PrebuildStatusHardLimited,
			PresetID: ps.Preset.ID,
		})
		if err != nil {
			return xerrors.Errorf("failed to update preset prebuild status: %w", err)
		}

		err = c.notifyPrebuildFailureLimitReached(ctx, ps)
		if err != nil {
			logger.Error(ctx, "failed to notify that number of prebuild failures reached the limit", slog.Error(err))
		}
	}

	state := ps.CalculateState()
	actions, err := c.CalculateActions(ctx, ps)
	if err != nil {
		logger.Error(ctx, "failed to calculate actions for preset", slog.Error(err))
		return err
	}

	fields := []any{
		slog.F("desired", state.Desired), slog.F("actual", state.Actual),
		slog.F("extraneous", state.Extraneous), slog.F("starting", state.Starting),
		slog.F("stopping", state.Stopping), slog.F("deleting", state.Deleting),
		slog.F("eligible", state.Eligible),
	}

	levelFn := logger.Debug
	levelFn(ctx, "calculated reconciliation state for preset", fields...)

	var multiErr multierror.Error
	for _, action := range actions {
		err = c.executeReconciliationAction(ctx, logger, ps, action)
		if err != nil {
			logger.Error(ctx, "failed to execute action", "type", action.ActionType, slog.Error(err))
			multiErr.Errors = append(multiErr.Errors, err)
		}
	}
	return multiErr.ErrorOrNil()
}

func (c *StoreReconciler) notifyPrebuildFailureLimitReached(ctx context.Context, ps prebuilds.PresetSnapshot) error {
	// nolint:gocritic // Necessary to query all the required data.
	ctx = dbauthz.AsSystemRestricted(ctx)

	// Send notification to template admins.
	if c.notifEnq == nil {
		c.logger.Warn(ctx, "notification enqueuer not set, cannot send prebuild is hard limited notification(s)")
		return nil
	}

	templateAdmins, err := c.store.GetUsers(ctx, database.GetUsersParams{
		RbacRole: []string{codersdk.RoleTemplateAdmin},
	})
	if err != nil {
		return xerrors.Errorf("fetch template admins: %w", err)
	}

	for _, templateAdmin := range templateAdmins {
		if _, err := c.notifEnq.EnqueueWithData(ctx, templateAdmin.ID, notifications.PrebuildFailureLimitReached,
			map[string]string{
				"org":              ps.Preset.OrganizationName,
				"template":         ps.Preset.TemplateName,
				"template_version": ps.Preset.TemplateVersionName,
				"preset":           ps.Preset.Name,
			},
			map[string]any{},
			"prebuilds_reconciler",
			// Associate this notification with all the related entities.
			ps.Preset.TemplateID, ps.Preset.TemplateVersionID, ps.Preset.ID, ps.Preset.OrganizationID,
		); err != nil {
			c.logger.Error(ctx,
				"failed to send notification",
				slog.Error(err),
				slog.F("template_admin_id", templateAdmin.ID.String()),
			)

			continue
		}
	}

	return nil
}

func (c *StoreReconciler) CalculateActions(ctx context.Context, snapshot prebuilds.PresetSnapshot) ([]*prebuilds.ReconciliationActions, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return snapshot.CalculateActions(c.clock, c.cfg.ReconciliationBackoffInterval.Value())
}

func (c *StoreReconciler) WithReconciliationLock(
	ctx context.Context,
	logger slog.Logger,
	fn func(ctx context.Context, db database.Store) error,
) error {
	// This tx holds a global lock, which prevents any other coderd replica from starting a reconciliation and
	// possibly getting an inconsistent view of the state.
	//
	// The lock MUST be held until ALL modifications have been effected.
	//
	// It is run with RepeatableRead isolation, so it's effectively snapshotting the data at the start of the tx.
	//
	// This is a read-only tx, so returning an error (i.e. causing a rollback) has no impact.
	return c.store.InTx(func(db database.Store) error {
		start := c.clock.Now()

		// Try to acquire the lock. If we can't get it, another replica is handling reconciliation.
		acquired, err := db.TryAcquireLock(ctx, database.LockIDReconcilePrebuilds)
		if err != nil {
			// This is a real database error, not just lock contention
			logger.Error(ctx, "failed to acquire reconciliation lock due to database error", slog.Error(err))
			return err
		}
		if !acquired {
			// Normal case: another replica has the lock
			return nil
		}

		logger.Debug(ctx,
			"acquired top-level reconciliation lock",
			slog.F("acquire_wait_secs", fmt.Sprintf("%.4f", c.clock.Since(start).Seconds())),
		)

		return fn(ctx, db)
	}, &database.TxOptions{
		Isolation:    sql.LevelRepeatableRead,
		ReadOnly:     true,
		TxIdentifier: "prebuilds",
	})
}

// executeReconciliationAction executes a reconciliation action on the given preset snapshot.
//
// The action can be of different types (create, delete, backoff), and may internally include
// multiple items to process, for example, a delete action can contain multiple prebuild IDs to delete,
// and a create action includes a count of prebuilds to create.
//
// This method handles logging at appropriate levels and performs the necessary operations
// according to the action type. It returns an error if any part of the action fails.
func (c *StoreReconciler) executeReconciliationAction(ctx context.Context, logger slog.Logger, ps prebuilds.PresetSnapshot, action *prebuilds.ReconciliationActions) error {
	levelFn := logger.Debug

	// Nothing has to be done.
	if !ps.Preset.UsingActiveVersion && action.IsNoop() {
		logger.Debug(ctx, "skipping reconciliation for preset - nothing has to be done",
			slog.F("template_id", ps.Preset.TemplateID.String()), slog.F("template_name", ps.Preset.TemplateName),
			slog.F("template_version_id", ps.Preset.TemplateVersionID.String()), slog.F("template_version_name", ps.Preset.TemplateVersionName),
			slog.F("preset_id", ps.Preset.ID.String()), slog.F("preset_name", ps.Preset.Name))
		return nil
	}

	// nolint:gocritic // ReconcilePreset needs Prebuilds Orchestrator permissions.
	prebuildsCtx := dbauthz.AsPrebuildsOrchestrator(ctx)

	fields := []any{
		slog.F("action_type", action.ActionType), slog.F("create_count", action.Create),
		slog.F("delete_count", len(action.DeleteIDs)), slog.F("to_delete", action.DeleteIDs),
	}
	levelFn(ctx, "calculated reconciliation action for preset", fields...)

	switch {
	case action.ActionType == prebuilds.ActionTypeBackoff:
		levelFn = logger.Warn
	// Log at info level when there's a change to be effected.
	case action.ActionType == prebuilds.ActionTypeCreate && action.Create > 0:
		levelFn = logger.Info
	case action.ActionType == prebuilds.ActionTypeDelete && len(action.DeleteIDs) > 0:
		levelFn = logger.Info
	}

	switch action.ActionType {
	case prebuilds.ActionTypeBackoff:
		// If there is anything to backoff for (usually a cycle of failed prebuilds), then log and bail out.
		levelFn(ctx, "template prebuild state retrieved, backing off",
			append(fields,
				slog.F("backoff_until", action.BackoffUntil.Format(time.RFC3339)),
				slog.F("backoff_secs", math.Round(action.BackoffUntil.Sub(c.clock.Now()).Seconds())),
			)...)

		return nil

	case prebuilds.ActionTypeCreate:
		// Unexpected things happen (i.e. bugs or bitflips); let's defend against disastrous outcomes.
		// See https://blog.robertelder.org/causes-of-bit-flips-in-computer-memory/.
		// This is obviously not comprehensive protection against this sort of problem, but this is one essential check.
		desired := ps.Preset.DesiredInstances.Int32
		if action.Create > desired {
			logger.Critical(ctx, "determined excessive count of prebuilds to create; clamping to desired count",
				slog.F("create_count", action.Create), slog.F("desired_count", desired))

			action.Create = desired
		}

		// If preset is hard-limited, and it's a create operation, log it and exit early.
		// Creation operation is disallowed for hard-limited preset.
		if ps.IsHardLimited && action.Create > 0 {
			logger.Warn(ctx, "skipping hard limited preset for create operation")
			return nil
		}

		var multiErr multierror.Error
		for range action.Create {
			if err := c.createPrebuiltWorkspace(prebuildsCtx, uuid.New(), ps.Preset.TemplateID, ps.Preset.ID); err != nil {
				logger.Error(ctx, "failed to create prebuild", slog.Error(err))
				multiErr.Errors = append(multiErr.Errors, err)
			}
		}

		return multiErr.ErrorOrNil()

	case prebuilds.ActionTypeDelete:
		var multiErr multierror.Error
		for _, id := range action.DeleteIDs {
			if err := c.deletePrebuiltWorkspace(prebuildsCtx, id, ps.Preset.TemplateID, ps.Preset.ID); err != nil {
				logger.Error(ctx, "failed to delete prebuild", slog.Error(err))
				multiErr.Errors = append(multiErr.Errors, err)
			}
		}

		return multiErr.ErrorOrNil()

	default:
		return xerrors.Errorf("unknown action type: %v", action.ActionType)
	}
}

func (c *StoreReconciler) createPrebuiltWorkspace(ctx context.Context, prebuiltWorkspaceID uuid.UUID, templateID uuid.UUID, presetID uuid.UUID) error {
	name, err := prebuilds.GenerateName()
	if err != nil {
		return xerrors.Errorf("failed to generate unique prebuild ID: %w", err)
	}

	return c.store.InTx(func(db database.Store) error {
		template, err := db.GetTemplateByID(ctx, templateID)
		if err != nil {
			return xerrors.Errorf("failed to get template: %w", err)
		}

		now := c.clock.Now()

		minimumWorkspace, err := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
			ID:                prebuiltWorkspaceID,
			CreatedAt:         now,
			UpdatedAt:         now,
			OwnerID:           prebuilds.SystemUserID,
			OrganizationID:    template.OrganizationID,
			TemplateID:        template.ID,
			Name:              name,
			LastUsedAt:        c.clock.Now(),
			AutomaticUpdates:  database.AutomaticUpdatesNever,
			AutostartSchedule: sql.NullString{},
			Ttl:               sql.NullInt64{},
			NextStartAt:       sql.NullTime{},
		})
		if err != nil {
			return xerrors.Errorf("insert workspace: %w", err)
		}

		// We have to refetch the workspace for the joined in fields.
		workspace, err := db.GetWorkspaceByID(ctx, minimumWorkspace.ID)
		if err != nil {
			return xerrors.Errorf("get workspace by ID: %w", err)
		}

		c.logger.Info(ctx, "attempting to create prebuild", slog.F("name", name),
			slog.F("workspace_id", prebuiltWorkspaceID.String()), slog.F("preset_id", presetID.String()))

		return c.provision(ctx, db, prebuiltWorkspaceID, template, presetID, database.WorkspaceTransitionStart, workspace)
	}, &database.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  false,
	})
}

func (c *StoreReconciler) deletePrebuiltWorkspace(ctx context.Context, prebuiltWorkspaceID uuid.UUID, templateID uuid.UUID, presetID uuid.UUID) error {
	return c.store.InTx(func(db database.Store) error {
		workspace, err := db.GetWorkspaceByID(ctx, prebuiltWorkspaceID)
		if err != nil {
			return xerrors.Errorf("get workspace by ID: %w", err)
		}

		template, err := db.GetTemplateByID(ctx, templateID)
		if err != nil {
			return xerrors.Errorf("failed to get template: %w", err)
		}

		if workspace.OwnerID != prebuilds.SystemUserID {
			return xerrors.Errorf("prebuilt workspace is not owned by prebuild user anymore, probably it was claimed")
		}

		c.logger.Info(ctx, "attempting to delete prebuild",
			slog.F("workspace_id", prebuiltWorkspaceID.String()), slog.F("preset_id", presetID.String()))

		return c.provision(ctx, db, prebuiltWorkspaceID, template, presetID, database.WorkspaceTransitionDelete, workspace)
	}, &database.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  false,
	})
}

func (c *StoreReconciler) provision(
	ctx context.Context,
	db database.Store,
	prebuildID uuid.UUID,
	template database.Template,
	presetID uuid.UUID,
	transition database.WorkspaceTransition,
	workspace database.Workspace,
) error {
	tvp, err := db.GetPresetParametersByTemplateVersionID(ctx, template.ActiveVersionID)
	if err != nil {
		return xerrors.Errorf("fetch preset details: %w", err)
	}

	var params []codersdk.WorkspaceBuildParameter
	for _, param := range tvp {
		// TODO: don't fetch in the first place.
		if param.TemplateVersionPresetID != presetID {
			continue
		}

		params = append(params, codersdk.WorkspaceBuildParameter{
			Name:  param.Name,
			Value: param.Value,
		})
	}

	builder := wsbuilder.New(workspace, transition).
		Reason(database.BuildReasonInitiator).
		Initiator(prebuilds.SystemUserID).
		MarkPrebuild()

	if transition != database.WorkspaceTransitionDelete {
		// We don't specify the version for a delete transition,
		// because the prebuilt workspace may have been created using an older template version.
		// If the version isn't explicitly set, the builder will automatically use the version
		// from the last workspace build — which is the desired behavior.
		builder = builder.VersionID(template.ActiveVersionID)

		// We only inject the required params when the prebuild is being created.
		// This mirrors the behavior of regular workspace deletion (see cli/delete.go).
		builder = builder.TemplateVersionPresetID(presetID)
		builder = builder.RichParameterValues(params)
	}

	_, provisionerJob, _, err := builder.Build(
		ctx,
		db,
		func(_ policy.Action, _ rbac.Objecter) bool {
			return true // TODO: harden?
		},
		audit.WorkspaceBuildBaggage{},
	)
	if err != nil {
		return xerrors.Errorf("provision workspace: %w", err)
	}

	if provisionerJob == nil {
		return nil
	}

	// Publish provisioner job event outside of transaction.
	select {
	case c.provisionNotifyCh <- *provisionerJob:
	default: // channel full, drop the message; provisioner will pick this job up later with its periodic check, though.
		c.logger.Warn(ctx, "provisioner job notification queue full, dropping",
			slog.F("job_id", provisionerJob.ID), slog.F("prebuild_id", prebuildID.String()))
	}

	c.logger.Info(ctx, "prebuild job scheduled", slog.F("transition", transition),
		slog.F("prebuild_id", prebuildID.String()), slog.F("preset_id", presetID.String()),
		slog.F("job_id", provisionerJob.ID))

	return nil
}

// ForceMetricsUpdate forces the metrics collector, if defined, to update its state (we cache the metrics state to
// reduce load on the database).
func (c *StoreReconciler) ForceMetricsUpdate(ctx context.Context) error {
	if c.metrics == nil {
		return nil
	}

	return c.metrics.UpdateState(ctx, time.Second*10)
}

func (c *StoreReconciler) TrackResourceReplacement(ctx context.Context, workspaceID, buildID uuid.UUID, replacements []*sdkproto.ResourceReplacement) {
	// nolint:gocritic // Necessary to query all the required data.
	ctx = dbauthz.AsSystemRestricted(ctx)
	// Since this may be called in a fire-and-forget fashion, we need to give up at some point.
	trackCtx, trackCancel := context.WithTimeout(ctx, time.Minute)
	defer trackCancel()

	if err := c.trackResourceReplacement(trackCtx, workspaceID, buildID, replacements); err != nil {
		c.logger.Error(ctx, "failed to track resource replacement", slog.Error(err))
	}
}

// nolint:revive // Shut up it's fine.
func (c *StoreReconciler) trackResourceReplacement(ctx context.Context, workspaceID, buildID uuid.UUID, replacements []*sdkproto.ResourceReplacement) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	workspace, err := c.store.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return xerrors.Errorf("fetch workspace %q: %w", workspaceID.String(), err)
	}

	build, err := c.store.GetWorkspaceBuildByID(ctx, buildID)
	if err != nil {
		return xerrors.Errorf("fetch workspace build %q: %w", buildID.String(), err)
	}

	// The first build will always be the prebuild.
	prebuild, err := c.store.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
		WorkspaceID: workspaceID, BuildNumber: 1,
	})
	if err != nil {
		return xerrors.Errorf("fetch prebuild: %w", err)
	}

	// This should not be possible, but defend against it.
	if !prebuild.TemplateVersionPresetID.Valid || prebuild.TemplateVersionPresetID.UUID == uuid.Nil {
		return xerrors.Errorf("no preset used in prebuild for workspace %q", workspaceID.String())
	}

	prebuildPreset, err := c.store.GetPresetByID(ctx, prebuild.TemplateVersionPresetID.UUID)
	if err != nil {
		return xerrors.Errorf("fetch template preset for template version ID %q: %w", prebuild.TemplateVersionID.String(), err)
	}

	claimant, err := c.store.GetUserByID(ctx, workspace.OwnerID) // At this point, the workspace is owned by the new owner.
	if err != nil {
		return xerrors.Errorf("fetch claimant %q: %w", workspace.OwnerID.String(), err)
	}

	// Use the claiming build here (not prebuild) because both should be equivalent, and we might as well spot inconsistencies now.
	templateVersion, err := c.store.GetTemplateVersionByID(ctx, build.TemplateVersionID)
	if err != nil {
		return xerrors.Errorf("fetch template version %q: %w", build.TemplateVersionID.String(), err)
	}

	org, err := c.store.GetOrganizationByID(ctx, workspace.OrganizationID)
	if err != nil {
		return xerrors.Errorf("fetch org %q: %w", workspace.OrganizationID.String(), err)
	}

	// Track resource replacement in Prometheus metric.
	if c.metrics != nil {
		c.metrics.trackResourceReplacement(org.Name, workspace.TemplateName, prebuildPreset.Name)
	}

	// Send notification to template admins.
	if c.notifEnq == nil {
		c.logger.Warn(ctx, "notification enqueuer not set, cannot send resource replacement notification(s)")
		return nil
	}

	repls := make(map[string]string, len(replacements))
	for _, repl := range replacements {
		repls[repl.GetResource()] = strings.Join(repl.GetPaths(), ", ")
	}

	templateAdmins, err := c.store.GetUsers(ctx, database.GetUsersParams{
		RbacRole: []string{codersdk.RoleTemplateAdmin},
	})
	if err != nil {
		return xerrors.Errorf("fetch template admins: %w", err)
	}

	var notifErr error
	for _, templateAdmin := range templateAdmins {
		if _, err := c.notifEnq.EnqueueWithData(ctx, templateAdmin.ID, notifications.TemplateWorkspaceResourceReplaced,
			map[string]string{
				"org":                 org.Name,
				"workspace":           workspace.Name,
				"template":            workspace.TemplateName,
				"template_version":    templateVersion.Name,
				"preset":              prebuildPreset.Name,
				"workspace_build_num": fmt.Sprintf("%d", build.BuildNumber),
				"claimant":            claimant.Username,
			},
			map[string]any{
				"replacements": repls,
			}, "prebuilds_reconciler",
			// Associate this notification with all the related entities.
			workspace.ID, workspace.OwnerID, workspace.TemplateID, templateVersion.ID, prebuildPreset.ID, workspace.OrganizationID,
		); err != nil {
			notifErr = errors.Join(xerrors.Errorf("send notification to %q: %w", templateAdmin.ID.String(), err))
			continue
		}
	}

	return notifErr
}
