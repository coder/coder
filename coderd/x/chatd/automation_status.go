package chatd

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// AutomationPausedReasons returns the conditions that stop each automation
// in rows from running even while it is enabled. Automations without a
// reason have no map entry. ctx must carry the caller's authorization: rows
// were read as the caller, and their run statuses are read as the caller
// too. The result is an advisory snapshot that publishing does not consult;
// admission rechecks every condition.
func (p *Server) AutomationPausedReasons(ctx context.Context, rows []database.ChatAutomation) (map[uuid.UUID][]codersdk.ChatAutomationPausedReason, error) {
	reasons := make(map[uuid.UUID][]codersdk.ChatAutomationPausedReason)
	if len(rows) == 0 {
		return reasons, nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	statuses, err := p.db.GetChatAutomationRunStatusesByIDs(ctx, ids)
	if err != nil {
		return nil, xerrors.Errorf("get chat automation run statuses: %w", err)
	}
	statusByID := make(map[uuid.UUID]database.GetChatAutomationRunStatusesByIDsRow, len(statuses))
	for _, status := range statuses {
		statusByID[status.ID] = status
	}

	type ownerSubject struct {
		subject  rbac.Subject
		inactive bool
	}
	type modelKey struct {
		ownerID, organizationID, modelConfigID uuid.UUID
	}
	experimentOn := make(map[uuid.UUID]bool)
	subjects := make(map[uuid.UUID]ownerSubject)
	modelAvailable := make(map[modelKey]bool)

	for _, row := range rows {
		status, ok := statusByID[row.ID]
		if !ok {
			// Deleted after the caller read it; nothing runs.
			continue
		}
		ownerInactive := !status.OwnerActive
		on, ok := experimentOn[row.OwnerID]
		if !ok {
			on = AutomationsEnabled(ctx, p.experimentEvaluator, row.OwnerID)
			experimentOn[row.OwnerID] = on
		}
		targetUnavailable := row.TargetMode == database.ChatAutomationTargetModeExistingChat && !status.TargetAvailable

		modelUnavailable := false
		if row.TargetMode == database.ChatAutomationTargetModeNewChat && !ownerInactive {
			owner, ok := subjects[row.OwnerID]
			if !ok {
				owner.subject, err = automationOwnerSubject(ctx, p.db, row.OwnerID)
				switch {
				case errors.Is(err, ErrAutomationOwnerInactive):
					// The owner became inactive after the status read.
					owner.inactive = true
				case err != nil:
					return nil, err
				}
				subjects[row.OwnerID] = owner
			}
			if owner.inactive {
				ownerInactive = true
			} else {
				key := modelKey{ownerID: row.OwnerID, organizationID: row.OrganizationID, modelConfigID: row.NewChatModelConfigID.UUID}
				available, ok := modelAvailable[key]
				if !ok {
					err := checkAutomationModel(ctx, p.db, owner.subject, row)
					switch {
					case errors.Is(err, ErrAutomationModelUnavailable):
					case err != nil:
						return nil, err
					default:
						available = true
					}
					modelAvailable[key] = available
				}
				modelUnavailable = !available
			}
		}

		var rowReasons []codersdk.ChatAutomationPausedReason
		if ownerInactive {
			rowReasons = append(rowReasons, codersdk.ChatAutomationPausedReasonOwnerInactive)
		}
		if !on {
			rowReasons = append(rowReasons, codersdk.ChatAutomationPausedReasonExperimentDisabled)
		}
		if targetUnavailable {
			rowReasons = append(rowReasons, codersdk.ChatAutomationPausedReasonTargetUnavailable)
		}
		if modelUnavailable {
			rowReasons = append(rowReasons, codersdk.ChatAutomationPausedReasonModelUnavailable)
		}
		if len(rowReasons) > 0 {
			reasons[row.ID] = rowReasons
		}
	}
	return reasons, nil
}
