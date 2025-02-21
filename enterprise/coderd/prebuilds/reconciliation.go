package prebuilds

import (
	"math"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/slice"
)

type reconciliationState struct {
	presets             []database.GetTemplatePresetsWithPrebuildsRow
	runningPrebuilds    []database.GetRunningPrebuildsRow
	prebuildsInProgress []database.GetPrebuildsInProgressRow
}

type presetState struct {
	preset     database.GetTemplatePresetsWithPrebuildsRow
	running    []database.GetRunningPrebuildsRow
	inProgress []database.GetPrebuildsInProgressRow
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
}

func newReconciliationState(presets []database.GetTemplatePresetsWithPrebuildsRow, runningPrebuilds []database.GetRunningPrebuildsRow, prebuildsInProgress []database.GetPrebuildsInProgressRow) reconciliationState {
	return reconciliationState{presets: presets, runningPrebuilds: runningPrebuilds, prebuildsInProgress: prebuildsInProgress}
}

func (s reconciliationState) filterByPreset(presetID uuid.UUID) (*presetState, error) {
	preset, found := slice.Find(s.presets, func(preset database.GetTemplatePresetsWithPrebuildsRow) bool {
		return preset.PresetID == presetID
	})
	if !found {
		return nil, xerrors.Errorf("no preset found with ID %q", presetID)
	}

	running := slice.Filter(s.runningPrebuilds, func(prebuild database.GetRunningPrebuildsRow) bool {
		if !prebuild.DesiredPresetID.Valid && !prebuild.CurrentPresetID.Valid {
			return false
		}
		return prebuild.CurrentPresetID.UUID == preset.PresetID &&
			prebuild.TemplateVersionID == preset.TemplateVersionID // Not strictly necessary since presets are 1:1 with template versions, but no harm in being extra safe.
	})

	// These aren't preset-specific, but they need to inhibit all presets of this template from operating since they could
	// be in-progress builds which might impact another preset. For example, if a template goes from no defined prebuilds to defined prebuilds
	// and back, or a template is updated from one version to another.
	inProgress := slice.Filter(s.prebuildsInProgress, func(prebuild database.GetPrebuildsInProgressRow) bool {
		return prebuild.TemplateVersionID == preset.TemplateVersionID
	})

	return &presetState{
		preset:     preset,
		running:    running,
		inProgress: inProgress,
	}, nil
}

func (p presetState) calculateActions() (*reconciliationActions, error) {
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

	// Bail early to avoid scheduling new prebuilds while operations are in progress.
	if (toCreate+toDelete) > 0 && (starting+stopping+deleting) > 0 {
		// TODO: move up
		//c.logger.Warn(ctx, "prebuild operations in progress, skipping reconciliation",
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

		var victims []uuid.UUID
		for i := 0; i < int(extraneous); i++ {
			if i >= len(p.running) {
				// This should never happen.
				// TODO: move up
				//c.logger.Warn(ctx, "unexpected reconciliation state; extraneous count exceeds running prebuilds count!",
				//	slog.F("running_count", len(p.running)),
				//	slog.F("extraneous", extraneous))
				continue
			}

			victims = append(victims, p.running[i].WorkspaceID)
		}

		actions.deleteIDs = append(actions.deleteIDs, victims...)

		// TODO: move up
		//c.logger.Warn(ctx, "found extra prebuilds running, picking random victim(s)",
		//	slog.F("template_id", p.preset.TemplateID.String()), slog.F("desired", desired), slog.F("actual", actual), slog.F("extra", extraneous),
		//	slog.F("victims", victims))

		// Prevent the rest of the reconciliation from completing
		return actions, nil
	}

	// If the template has become deleted or deprecated since the last reconciliation, we need to ensure we
	// scale those prebuilds down to zero.
	if p.preset.Deleted || p.preset.Deprecated {
		toCreate = 0
		toDelete = int(actual + outdated)
	}

	actions.create = int32(toCreate)

	if toDelete > 0 && len(p.running) != toDelete {
		// TODO: move up
		//c.logger.Warn(ctx, "mismatch between running prebuilds and expected deletion count!",
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
