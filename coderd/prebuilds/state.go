package prebuilds

import (
	"math"
	"slices"
	"time"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/database"
)

func (p PresetState) CalculateActions(clock quartz.Clock, backoffInterval time.Duration) (*ReconciliationActions, error) {

	switch {
	//case inBackoffPeriod():
	//	return handleBackoff()
	//case isDeprecated():
	//	return handleDeprecatedTemplateVersion()
	case p.Preset.UsingActiveVersion:
		handleActiveTemplateVersion()
		//case x:
		//	handleInactiveTemplateVersion()
	}
}

func (p PresetState) handleActiveTemplateVersion() (*ReconciliationActions, error) {
	var (
		actual                       int32 // Running prebuilds for active version.
		desired                      int32 // Active template version's desired instances as defined in preset.
		eligible                     int32 // Prebuilds which can be claimed.
		outdated                     int32 // Prebuilds which no longer match the active template version.
		extraneous                   int32 // Extra running prebuilds for active version (somehow).
		starting, stopping, deleting int32 // Prebuilds currently being provisioned up or down.
	)

	actual = int32(len(p.Running))
	desired = p.Preset.DesiredInstances
	extraneous = max(actual-p.Preset.DesiredInstances, 0)

	for _, prebuild := range p.Running {
		if prebuild.Ready {
			eligible++
		}
	}

	// In-progress builds are common across all presets belonging to a given template.
	// In other words: these values will be identical across all presets belonging to this template.
	// TODO: put in a helper method?
	for _, progress := range p.InProgress {
		num := progress.Count
		switch progress.Transition {
		case database.WorkspaceTransitionStart:
			starting += num
		case database.WorkspaceTransitionStop:
			stopping += num
		case database.WorkspaceTransitionDelete:
			deleting += num
		}
	}

	var (
		toCreate = int(math.Max(0, float64(
			desired-(actual+starting)), // The number of prebuilds currently being stopped (should be 0)
		))
		//toDelete = int(math.Max(0, float64(
		//	outdated- // The number of prebuilds running above the desired count for active version
		//		deleting), // The number of prebuilds currently being deleted
		//))

		actions = &ReconciliationActions{
			Actual:     actual,
			Desired:    desired,
			Eligible:   eligible,
			Outdated:   outdated,
			Extraneous: extraneous,
			Starting:   starting,
			Stopping:   stopping,
			Deleting:   deleting,
		}
	)

	// It's possible that an operator could stop/start prebuilds which interfere with the reconciliation loop, so
	// we check if there are somehow more prebuilds than we expect, and then pick random victims to be deleted.
	if extraneous > 0 {
		// Sort running IDs by creation time so we always delete the oldest prebuilds.
		// In general, we want fresher prebuilds (imagine a mono-repo is cloned; newer is better).
		slices.SortFunc(p.Running, func(a, b database.GetRunningPrebuildsRow) int {
			if a.CreatedAt.Before(b.CreatedAt) {
				return -1
			}
			if a.CreatedAt.After(b.CreatedAt) {
				return 1
			}

			return 0
		})

		for i := 0; i < int(extraneous); i++ {
			if i >= len(p.Running) {
				// This should never happen.
				// TODO: move up
				// c.logger.Warn(ctx, "unexpected reconciliation state; extraneous count exceeds running prebuilds count!",
				//	slog.F("running_count", len(p.Running)),
				//	slog.F("extraneous", extraneous))
				continue
			}

			actions.DeleteIDs = append(actions.DeleteIDs, p.Running[i].WorkspaceID)
		}

		// TODO: move up
		// c.logger.Warn(ctx, "found extra prebuilds running, picking random victim(s)",
		//	slog.F("template_id", p.Preset.TemplateID.String()), slog.F("desired", desired), slog.F("actual", actual), slog.F("extra", extraneous),
		//	slog.F("victims", victims))

		// Prevent the rest of the reconciliation from completing
		return actions, nil
	}

	actions.Create = int32(toCreate)

	return actions, nil
}

func (p PresetState) CalculateActionsV0(clock quartz.Clock, backoffInterval time.Duration) (*ReconciliationActions, error) {
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

	if p.Preset.UsingActiveVersion {
		actual = int32(len(p.Running))
		desired = p.Preset.DesiredInstances
	}

	for _, prebuild := range p.Running {
		if p.Preset.UsingActiveVersion {
			if prebuild.Ready {
				eligible++
			}

			extraneous = int32(math.Max(float64(actual-p.Preset.DesiredInstances), 0))
		}

		if prebuild.TemplateVersionID == p.Preset.TemplateVersionID && !p.Preset.UsingActiveVersion {
			outdated++
		}
	}

	// In-progress builds are common across all presets belonging to a given template.
	// In other words: these values will be identical across all presets belonging to this template.
	for _, progress := range p.InProgress {
		num := progress.Count
		switch progress.Transition {
		case database.WorkspaceTransitionStart:
			starting += num
		case database.WorkspaceTransitionStop:
			stopping += num
		case database.WorkspaceTransitionDelete:
			deleting += num
		}
	}

	var (
		toCreate = int(math.Max(0, float64(
			desired-(actual+starting)), // The number of prebuilds currently being stopped (should be 0)
		))
		toDelete = int(math.Max(0, float64(
			outdated- // The number of prebuilds running above the desired count for active version
				deleting), // The number of prebuilds currently being deleted
		))

		actions = &ReconciliationActions{
			Actual:     actual,
			Desired:    desired,
			Eligible:   eligible,
			Outdated:   outdated,
			Extraneous: extraneous,
			Starting:   starting,
			Stopping:   stopping,
			Deleting:   deleting,
		}
	)

	// If the template has become deleted or deprecated since the last reconciliation, we need to ensure we
	// scale those prebuilds down to zero.
	if p.Preset.Deleted || p.Preset.Deprecated {
		toCreate = 0
		toDelete = int(actual + outdated)
		actions.Desired = 0
	}

	// We backoff when the last build failed, to give the operator some time to investigate the issue and to not provision
	// a tonne of prebuilds (_n_ on each reconciliation iteration).
	if p.Backoff != nil && p.Backoff.NumFailed > 0 {
		actions.Failed = p.Backoff.NumFailed

		backoffUntil := p.Backoff.LastBuildAt.Add(time.Duration(p.Backoff.NumFailed) * backoffInterval)

		if clock.Now().Before(backoffUntil) {
			actions.Create = 0
			actions.DeleteIDs = nil
			actions.BackoffUntil = backoffUntil

			// Return early here; we should not perform any reconciliation actions if we're in a backoff period.
			return actions, nil
		}
	}

	// It's possible that an operator could stop/start prebuilds which interfere with the reconciliation loop, so
	// we check if there are somehow more prebuilds than we expect, and then pick random victims to be deleted.
	if extraneous > 0 {
		// Sort running IDs by creation time so we always delete the oldest prebuilds.
		// In general, we want fresher prebuilds (imagine a mono-repo is cloned; newer is better).
		slices.SortFunc(p.Running, func(a, b database.GetRunningPrebuildsRow) int {
			if a.CreatedAt.Before(b.CreatedAt) {
				return -1
			}
			if a.CreatedAt.After(b.CreatedAt) {
				return 1
			}

			return 0
		})

		for i := 0; i < int(extraneous); i++ {
			if i >= len(p.Running) {
				// This should never happen.
				// TODO: move up
				// c.logger.Warn(ctx, "unexpected reconciliation state; extraneous count exceeds running prebuilds count!",
				//	slog.F("running_count", len(p.Running)),
				//	slog.F("extraneous", extraneous))
				continue
			}

			actions.DeleteIDs = append(actions.DeleteIDs, p.Running[i].WorkspaceID)
		}

		// TODO: move up
		// c.logger.Warn(ctx, "found extra prebuilds running, picking random victim(s)",
		//	slog.F("template_id", p.Preset.TemplateID.String()), slog.F("desired", desired), slog.F("actual", actual), slog.F("extra", extraneous),
		//	slog.F("victims", victims))

		// Prevent the rest of the reconciliation from completing
		return actions, nil
	}

	actions.Create = int32(toCreate)

	// if toDelete > 0 && len(p.Running) != toDelete {
	// TODO: move up
	// c.logger.Warn(ctx, "mismatch between running prebuilds and expected deletion count!",
	//	slog.F("template_id", s.preset.TemplateID.String()), slog.F("running", len(p.Running)), slog.F("to_delete", toDelete))
	// }

	// TODO: implement lookup to not perform same action on workspace multiple times in $period
	// 		 i.e. a workspace cannot be deleted for some reason, which continually makes it eligible for deletion
	for i := 0; i < toDelete; i++ {
		if i >= len(p.Running) {
			// TODO: move up
			// Above warning will have already addressed this.
			continue
		}

		actions.DeleteIDs = append(actions.DeleteIDs, p.Running[i].WorkspaceID)
	}

	return actions, nil
}
