package prebuilds

import (
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/slice"
)

type reconciliationState struct {
	presets             []database.GetTemplatePresetsWithPrebuildsRow
	runningPrebuilds    []database.GetRunningPrebuildsRow
	prebuildsInProgress []database.GetPrebuildsInProgressRow
	backoffs            []database.GetPresetsBackoffRow
}

type presetState struct {
	preset     database.GetTemplatePresetsWithPrebuildsRow
	running    []database.GetRunningPrebuildsRow
	inProgress []database.GetPrebuildsInProgressRow
	backoff    *database.GetPresetsBackoffRow
}

type reconciliationActions struct {
	actual                       int32       // Running prebuilds for active version.
	desired                      int32       // Active template version's desired instances as defined in preset.
	eligible                     int32       // Prebuilds which can be claimed.
	outdated                     int32       // Prebuilds which no longer match the active template version.
	extraneous                   int32       // Extra running prebuilds for active version (somehow).
	starting, stopping, deleting int32       // Prebuilds currently being provisioned up or down.
	create                       int32       // The number of prebuilds required to be created to reconcile required state.
	deleteIDs                    []uuid.UUID // IDs of running prebuilds required to be deleted to reconcile required state.
	backoffUntil                 time.Time   // The time to wait until before trying to provision a new prebuild.
}

func newReconciliationState(presets []database.GetTemplatePresetsWithPrebuildsRow, runningPrebuilds []database.GetRunningPrebuildsRow,
	prebuildsInProgress []database.GetPrebuildsInProgressRow, backoffs []database.GetPresetsBackoffRow,
) reconciliationState {
	return reconciliationState{presets: presets, runningPrebuilds: runningPrebuilds, prebuildsInProgress: prebuildsInProgress, backoffs: backoffs}
}

func (s reconciliationState) filterByPreset(presetID uuid.UUID) (*presetState, error) {
	preset, found := slice.Find(s.presets, func(preset database.GetTemplatePresetsWithPrebuildsRow) bool {
		return preset.PresetID == presetID
	})
	if !found {
		return nil, xerrors.Errorf("no preset found with ID %q", presetID)
	}

	running := slice.Filter(s.runningPrebuilds, func(prebuild database.GetRunningPrebuildsRow) bool {
		if !prebuild.CurrentPresetID.Valid {
			return false
		}
		return prebuild.CurrentPresetID.UUID == preset.PresetID &&
			prebuild.TemplateVersionID == preset.TemplateVersionID // Not strictly necessary since presets are 1:1 with template versions, but no harm in being extra safe.
	})

	// These aren't preset-specific, but they need to inhibit all presets of this template from operating since they could
	// be in-progress builds which might impact another preset. For example, if a template goes from no defined prebuilds to defined prebuilds
	// and back, or a template is updated from one version to another.
	// We group by the template so that all prebuilds being provisioned for a prebuild are inhibited if any prebuild for
	// any preset in that template are in progress, to prevent clobbering.
	inProgress := slice.Filter(s.prebuildsInProgress, func(prebuild database.GetPrebuildsInProgressRow) bool {
		return prebuild.TemplateID == preset.TemplateID
	})

	var backoff *database.GetPresetsBackoffRow
	backoffs := slice.Filter(s.backoffs, func(row database.GetPresetsBackoffRow) bool {
		return row.PresetID == preset.PresetID
	})
	if len(backoffs) == 1 {
		backoff = &backoffs[0]
	}

	return &presetState{
		preset:     preset,
		running:    running,
		inProgress: inProgress,
		backoff:    backoff,
	}, nil
}

func (p presetState) calculateActions(backoffInterval time.Duration) (*reconciliationActions, error) {
	// TODO: align workspace states with how we represent them on the FE and the CLI
	//	     right now there's some slight differences which can lead to additional prebuilds being created

	// TODO: add mechanism to prevent prebuilds being reconciled from being claimable by users; i.e. if a prebuild is
	// 		 about to be deleted, it should not be deleted if it has been claimed - beware of TOCTOU races!

	var (
		actual                       int32 // Running prebuilds for active version.
		desired                      int32 // Active template version's desired instances as defined in preset.
		eligible                     int32 // Prebuilds which can be claimed.
		outdated                     int32 // Prebuilds which no longer match the active template version.
		extraneous                   int32 // Extra running prebuilds for active version (somehow).
		starting, stopping, deleting int32 // Prebuilds currently being provisioned up or down.
	)

	if p.preset.UsingActiveVersion {
		actual = int32(len(p.running))
		desired = p.preset.DesiredInstances
	}

	for _, prebuild := range p.running {
		if p.preset.UsingActiveVersion {
			if prebuild.Ready {
				eligible++
			}

			extraneous = int32(math.Max(float64(actual-p.preset.DesiredInstances), 0))
		}

		if prebuild.TemplateVersionID == p.preset.TemplateVersionID && !p.preset.UsingActiveVersion {
			outdated++
		}
	}

	// In-progress builds are common across all presets belonging to a given template.
	// In other words: these values will be identical across all presets belonging to this template.
	for _, progress := range p.inProgress {
		switch progress.Transition {
		case database.WorkspaceTransitionStart:
			starting++
		case database.WorkspaceTransitionStop:
			stopping++
		case database.WorkspaceTransitionDelete:
			deleting++
		}
	}

	var (
		toCreate = int(math.Max(0, float64(
			desired- // The number specified in the preset
				(actual+starting)- // The current number of prebuilds (or builds in-flight)
				stopping), // The number of prebuilds currently being stopped (should be 0)
		))
		toDelete = int(math.Max(0, float64(
			outdated- // The number of prebuilds running above the desired count for active version
				deleting), // The number of prebuilds currently being deleted
		))

		actions = &reconciliationActions{
			actual:     actual,
			desired:    desired,
			eligible:   eligible,
			outdated:   outdated,
			extraneous: extraneous,
			starting:   starting,
			stopping:   stopping,
			deleting:   deleting,
		}
	)

	// If the template has become deleted or deprecated since the last reconciliation, we need to ensure we
	// scale those prebuilds down to zero.
	if p.preset.Deleted || p.preset.Deprecated {
		toCreate = 0
		toDelete = int(actual + outdated)
		actions.desired = 0
	}

	// We backoff when the last build failed, to give the operator some time to investigate the issue and to not provision
	// a tonne of prebuilds (_n_ on each reconciliation iteration).
	if p.backoff != nil && p.backoff.NumFailed > 0 {
		backoffUntil := p.backoff.LastBuildAt.Add(time.Duration(p.backoff.NumFailed) * backoffInterval)

		if dbtime.Now().Before(backoffUntil) {
			actions.create = 0
			actions.deleteIDs = nil
			actions.backoffUntil = backoffUntil

			// Return early here; we should not perform any reconciliation actions if we're in a backoff period.
			return actions, nil
		}
	}

	// Bail early to avoid scheduling new prebuilds while operations are in progress.
	// TODO: optimization: we should probably be able to create prebuilds while others are deleting for a given preset.
	if (toCreate+toDelete) > 0 && (starting+stopping+deleting) > 0 {
		// TODO: move up
		// c.logger.Warn(ctx, "prebuild operations in progress, skipping reconciliation",
		//	slog.F("template_id", p.preset.TemplateID.String()), slog.F("starting", starting),
		//	slog.F("stopping", stopping), slog.F("deleting", deleting),
		//	slog.F("wanted_to_create", create), slog.F("wanted_to_delete", toDelete))
		return actions, nil
	}

	// It's possible that an operator could stop/start prebuilds which interfere with the reconciliation loop, so
	// we check if there are somehow more prebuilds than we expect, and then pick random victims to be deleted.
	if extraneous > 0 {
		// Sort running IDs by creation time so we always delete the oldest prebuilds.
		// In general, we want fresher prebuilds (imagine a mono-repo is cloned; newer is better).
		slices.SortFunc(p.running, func(a, b database.GetRunningPrebuildsRow) int {
			if a.CreatedAt.Before(b.CreatedAt) {
				return -1
			}
			if a.CreatedAt.After(b.CreatedAt) {
				return 1
			}

			return 0
		})

		for i := 0; i < int(extraneous); i++ {
			if i >= len(p.running) {
				// This should never happen.
				// TODO: move up
				// c.logger.Warn(ctx, "unexpected reconciliation state; extraneous count exceeds running prebuilds count!",
				//	slog.F("running_count", len(p.running)),
				//	slog.F("extraneous", extraneous))
				continue
			}

			actions.deleteIDs = append(actions.deleteIDs, p.running[i].WorkspaceID)
		}

		// TODO: move up
		// c.logger.Warn(ctx, "found extra prebuilds running, picking random victim(s)",
		//	slog.F("template_id", p.preset.TemplateID.String()), slog.F("desired", desired), slog.F("actual", actual), slog.F("extra", extraneous),
		//	slog.F("victims", victims))

		// Prevent the rest of the reconciliation from completing
		return actions, nil
	}

	actions.create = int32(toCreate)

	if toDelete > 0 && len(p.running) != toDelete {
		// TODO: move up
		// c.logger.Warn(ctx, "mismatch between running prebuilds and expected deletion count!",
		//	slog.F("template_id", s.preset.TemplateID.String()), slog.F("running", len(p.running)), slog.F("to_delete", toDelete))
	}

	// TODO: implement lookup to not perform same action on workspace multiple times in $period
	// 		 i.e. a workspace cannot be deleted for some reason, which continually makes it eligible for deletion
	for i := 0; i < toDelete; i++ {
		if i >= len(p.running) {
			// TODO: move up
			// Above warning will have already addressed this.
			continue
		}

		actions.deleteIDs = append(actions.deleteIDs, p.running[i].WorkspaceID)
	}

	return actions, nil
}
