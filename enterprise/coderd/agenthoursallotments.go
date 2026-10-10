package coderd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// agentHoursOvergrantError reports that an allotment would push its tier
// past AgentHoursAllotmentMaxBps.
type agentHoursOvergrantError struct {
	// availableBps is the largest allotment the target can hold: the
	// unallotted share plus the target's current allotment.
	availableBps  int64
	unallottedBps int64
}

func (e agentHoursOvergrantError) Error() string {
	return fmt.Sprintf("allotment exceeds the %d available basis points", e.availableBps)
}

// checkAgentHoursAllotment returns an agentHoursOvergrantError when adding
// requestedBps to othersBps, the tier's total without the target's own row,
// exceeds the cap.
func checkAgentHoursAllotment(othersBps int64, currentBps, requestedBps int32) error {
	available := max(codersdk.AgentHoursAllotmentMaxBps-othersBps, 0)
	if int64(requestedBps) > available {
		return agentHoursOvergrantError{
			availableBps:  available,
			unallottedBps: max(available-int64(currentBps), 0),
		}
	}
	return nil
}

// agentHoursAllotmentBody decodes a codersdk.UpsertAgentHoursAllotmentRequest
// from any valid JSON, keeping the allotment raw, so that a fractional or
// quoted value or a body that is not an object gets a field validation error
// instead of a decode error naming Go types.
type agentHoursAllotmentBody struct {
	allotmentBps json.RawMessage
}

func (b *agentHoursAllotmentBody) UnmarshalJSON(data []byte) error {
	var req struct {
		AllotmentBps json.RawMessage `json:"allotment_bps"`
	}
	if json.Unmarshal(data, &req) == nil {
		b.allotmentBps = req.AllotmentBps
	}
	return nil
}

// readAgentHoursAllotment reads a codersdk.UpsertAgentHoursAllotmentRequest.
func readAgentHoursAllotment(ctx context.Context, rw http.ResponseWriter, r *http.Request) (int32, bool) {
	var body agentHoursAllotmentBody
	if !httpapi.Read(ctx, rw, r, &body) {
		return 0, false
	}
	bps, err := strconv.ParseInt(string(body.allotmentBps), 10, 32)
	if err == nil && bps >= 1 && bps <= codersdk.AgentHoursAllotmentMaxBps {
		return int32(bps), true
	}
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Invalid Agent Hours allotment.",
		Validations: []codersdk.ValidationError{{
			Field:  "allotment_bps",
			Detail: fmt.Sprintf("Must be an integer between 1 and %d.", codersdk.AgentHoursAllotmentMaxBps),
		}},
	})
	return 0, false
}

func overgrantDetail(e agentHoursOvergrantError) string {
	if e.availableBps == e.unallottedBps {
		return fmt.Sprintf("Only %s is unallotted.", database.FormatAllotmentBps(e.unallottedBps))
	}
	return fmt.Sprintf("Only %s is unallotted, so this allotment can be at most %s.",
		database.FormatAllotmentBps(e.unallottedBps), database.FormatAllotmentBps(e.availableBps))
}

// writeAgentHoursAllotmentTxError writes the response for an error returned
// by an allotment upsert transaction.
func (api *API) writeAgentHoursAllotmentTxError(ctx context.Context, rw http.ResponseWriter, err error) {
	var overgrant agentHoursOvergrantError
	if errors.As(err, &overgrant) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Agent Hours allotments cannot exceed 100% in total.",
			Detail:  overgrantDetail(overgrant),
			Validations: []codersdk.ValidationError{{
				Field:  "allotment_bps",
				Detail: fmt.Sprintf("Must not exceed %d.", overgrant.availableBps),
			}},
		})
		return
	}
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	api.Logger.Error(ctx, "upsert agent hours allotment", slog.Error(err))
	httpapi.InternalServerError(rw, err)
}

// @Summary Get Agent Hours organization allotments
// @ID get-agent-hours-organization-allotments
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {array} codersdk.AgentHoursOrganizationAllotment
// @Router /api/v2/agent-hours/allotments [get]
func (api *API) agentHoursOrganizationAllotments(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}

	rows, err := api.Database.GetAgentHoursOrganizationAllotments(ctx)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, slice.List(rows, db2sdk.AgentHoursOrganizationAllotmentRow))
}

// @Summary Upsert Agent Hours organization allotment
// @ID upsert-agent-hours-organization-allotment
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.UpsertAgentHoursAllotmentRequest true "Upsert Agent Hours allotment request"
// @Success 200 {object} codersdk.AgentHoursOrganizationAllotment
// @Router /api/v2/organizations/{organization}/agent-hours/allotment [put]
func (api *API) upsertAgentHoursOrganizationAllotment(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		org               = httpmw.OrganizationParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableAgentHoursOrganizationAllotment](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: org.ID,
		})
	)
	defer commitAudit()

	// Authorize before taking the lock so unauthorized callers cannot
	// stall writers.
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}
	if org.Deleted {
		httpapi.ResourceNotFound(rw)
		return
	}

	allotmentBps, ok := readAgentHoursAllotment(ctx, rw, r)
	if !ok {
		return
	}

	var (
		old     database.AgentHoursOrganizationAllotment
		hadRow  bool
		updated database.AgentHoursOrganizationAllotment
	)
	err := api.Database.InTx(func(tx database.Store) error {
		// The lock is its own statement so the total read below sees rows
		// committed by writers that held the lock before us.
		if err := tx.AcquireLock(ctx, database.LockIDAgentHoursOrganizationAllotments); err != nil {
			return xerrors.Errorf("acquire lock: %w", err)
		}
		//nolint:gocritic // The over-grant guard must count every organization's allotment.
		rows, err := tx.GetAgentHoursOrganizationAllotments(dbauthz.AsSystemRestricted(ctx))
		if err != nil {
			return xerrors.Errorf("get organization allotments: %w", err)
		}
		var others int64
		for _, row := range rows {
			if row.OrganizationID == org.ID {
				old = database.AgentHoursOrganizationAllotment{
					OrganizationID: row.OrganizationID,
					AllotmentBps:   row.AllotmentBps,
					CreatedAt:      row.CreatedAt,
					UpdatedAt:      row.UpdatedAt,
				}
				hadRow = true
				continue
			}
			others += int64(row.AllotmentBps)
		}
		if hadRow && old.AllotmentBps == allotmentBps {
			updated = old
			return nil
		}
		if err := checkAgentHoursAllotment(others, old.AllotmentBps, allotmentBps); err != nil {
			return err
		}
		updated, err = tx.UpsertAgentHoursOrganizationAllotment(ctx, database.UpsertAgentHoursOrganizationAllotmentParams{
			OrganizationID: org.ID,
			AllotmentBps:   allotmentBps,
		})
		if err != nil {
			return xerrors.Errorf("upsert organization allotment: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		api.writeAgentHoursAllotmentTxError(ctx, rw, err)
		return
	}
	// An unchanged allotment leaves Old and New unset, which skips the audit
	// entry.
	switch {
	case !hadRow:
		aReq.Action = database.AuditActionCreate
		aReq.New = updated.Auditable(org.Name)
	case old.AllotmentBps != updated.AllotmentBps:
		aReq.Old = old.Auditable(org.Name)
		aReq.New = updated.Auditable(org.Name)
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.AgentHoursOrganizationAllotment(updated, org))
}

// @Summary Delete Agent Hours organization allotment
// @ID delete-agent-hours-organization-allotment
// @Security CoderSessionToken
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 204
// @Router /api/v2/organizations/{organization}/agent-hours/allotment [delete]
func (api *API) deleteAgentHoursOrganizationAllotment(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		org               = httpmw.OrganizationParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableAgentHoursOrganizationAllotment](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionDelete,
			OrganizationID: org.ID,
		})
	)
	defer commitAudit()

	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}
	if org.Deleted {
		httpapi.ResourceNotFound(rw)
		return
	}

	deleted, err := api.Database.DeleteAgentHoursOrganizationAllotment(ctx, org.ID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "delete agent hours organization allotment", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.Old = deleted.Auditable(org.Name)

	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Get Agent Hours group allotments
// @ID get-agent-hours-group-allotments
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.AgentHoursGroupAllotments
// @Router /api/v2/organizations/{organization}/agent-hours/group-allotments [get]
func (api *API) agentHoursGroupAllotments(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceGroup.InOrg(org.ID)) {
		httpapi.Forbidden(rw)
		return
	}
	if org.Deleted {
		httpapi.ResourceNotFound(rw)
		return
	}

	var resp codersdk.AgentHoursGroupAllotments
	orgAllotment, err := api.Database.GetAgentHoursOrganizationAllotment(ctx, org.ID)
	switch {
	case err == nil:
		resp.OrganizationAllotmentBps = &orgAllotment.AllotmentBps
	case !httpapi.Is404Error(err):
		httpapi.InternalServerError(rw, err)
		return
	}

	rows, err := api.Database.GetAgentHoursGroupAllotmentsByOrganizationID(ctx, org.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	resp.Groups = slice.List(rows, db2sdk.AgentHoursGroupAllotmentRow)
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// groupOrganizationActive writes a 404 and returns false when the group's
// organization is soft-deleted, matching the organization tier.
func (api *API) groupOrganizationActive(ctx context.Context, rw http.ResponseWriter, group database.Group) bool {
	//nolint:gocritic // The caller is already authorized to update the group; group managers may not be able to read its organization.
	org, err := api.Database.GetOrganizationByID(dbauthz.AsSystemRestricted(ctx), group.OrganizationID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return false
	}
	if org.Deleted {
		httpapi.ResourceNotFound(rw)
		return false
	}
	return true
}

// @Summary Upsert Agent Hours group allotment
// @ID upsert-agent-hours-group-allotment
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param group path string true "Group ID" format(uuid)
// @Param request body codersdk.UpsertAgentHoursAllotmentRequest true "Upsert Agent Hours allotment request"
// @Success 200 {object} codersdk.AgentHoursGroupAllotment
// @Router /api/v2/groups/{group}/agent-hours/allotment [put]
func (api *API) upsertAgentHoursGroupAllotment(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		group             = httpmw.GroupParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableAgentHoursGroupAllotment](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: group.OrganizationID,
		})
	)
	defer commitAudit()

	// Authorize before taking the lock so unauthorized callers cannot
	// stall writers.
	if !api.Authorize(r, policy.ActionUpdate, group) {
		httpapi.Forbidden(rw)
		return
	}
	if !api.groupOrganizationActive(ctx, rw, group) {
		return
	}

	allotmentBps, ok := readAgentHoursAllotment(ctx, rw, r)
	if !ok {
		return
	}

	var (
		old     database.AgentHoursGroupAllotment
		hadRow  bool
		updated database.AgentHoursGroupAllotment
	)
	err := api.Database.InTx(func(tx database.Store) error {
		// The lock is its own statement so the total read below sees rows
		// committed by writers that held the lock before us.
		if err := tx.AcquireLock(ctx, database.LockIDAgentHoursGroupAllotments(group.OrganizationID)); err != nil {
			return xerrors.Errorf("acquire lock: %w", err)
		}
		//nolint:gocritic // The over-grant guard must count every group's allotment, whatever the caller can read.
		rows, err := tx.GetAgentHoursGroupAllotmentsByOrganizationID(dbauthz.AsSystemRestricted(ctx), group.OrganizationID)
		if err != nil {
			return xerrors.Errorf("get group allotments: %w", err)
		}
		var others int64
		for _, row := range rows {
			if row.GroupID == group.ID {
				old = database.AgentHoursGroupAllotment{
					GroupID:      row.GroupID,
					AllotmentBps: row.AllotmentBps,
					CreatedAt:    row.CreatedAt,
					UpdatedAt:    row.UpdatedAt,
				}
				hadRow = true
				continue
			}
			others += int64(row.AllotmentBps)
		}
		if hadRow && old.AllotmentBps == allotmentBps {
			updated = old
			return nil
		}
		if err := checkAgentHoursAllotment(others, old.AllotmentBps, allotmentBps); err != nil {
			return err
		}
		updated, err = tx.UpsertAgentHoursGroupAllotment(ctx, database.UpsertAgentHoursGroupAllotmentParams{
			GroupID:      group.ID,
			AllotmentBps: allotmentBps,
		})
		if err != nil {
			return xerrors.Errorf("upsert group allotment: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		api.writeAgentHoursAllotmentTxError(ctx, rw, err)
		return
	}
	switch {
	case !hadRow:
		aReq.Action = database.AuditActionCreate
		aReq.New = updated.Auditable(group.Name)
	case old.AllotmentBps != updated.AllotmentBps:
		aReq.Old = old.Auditable(group.Name)
		aReq.New = updated.Auditable(group.Name)
	}

	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.AgentHoursGroupAllotment(updated, group))
}

// @Summary Delete Agent Hours group allotment
// @ID delete-agent-hours-group-allotment
// @Security CoderSessionToken
// @Tags Enterprise
// @Param group path string true "Group ID" format(uuid)
// @Success 204
// @Router /api/v2/groups/{group}/agent-hours/allotment [delete]
func (api *API) deleteAgentHoursGroupAllotment(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		group             = httpmw.GroupParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AuditableAgentHoursGroupAllotment](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionDelete,
			OrganizationID: group.OrganizationID,
		})
	)
	defer commitAudit()

	if !api.Authorize(r, policy.ActionUpdate, group) {
		httpapi.Forbidden(rw)
		return
	}
	if !api.groupOrganizationActive(ctx, rw, group) {
		return
	}

	deleted, err := api.Database.DeleteAgentHoursGroupAllotment(ctx, group.ID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		api.Logger.Error(ctx, "delete agent hours group allotment", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.Old = deleted.Auditable(group.Name)

	rw.WriteHeader(http.StatusNoContent)
}
