package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/aibridge/guardrail/adapters"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// AIGatewayGuardrailsRoutes mounts the guardrail CRUD routes at
// /api/v2/aibridge/guardrails, gated by the AI Governance add-on
// (FeatureAIBridge).
func (api *API) AIGatewayGuardrailsRoutes(r chi.Router) {
	r.Get("/", api.aiGatewayGuardrailsList)
	r.Post("/", api.aiGatewayGuardrailsCreate)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", api.aiGatewayGuardrailGet)
		r.Patch("/", api.aiGatewayGuardrailUpdate)
		r.Delete("/", api.aiGatewayGuardrailDelete)
		r.Post("/versions", api.aiGatewayGuardrailVersionCreate)
	})
}

// @Summary List AI gateway guardrails
// @ID list-ai-gateway-guardrails
// @Security CoderSessionToken
// @Produce json
// @Tags AI Gateway
// @Success 200 {array} codersdk.AIGatewayGuardrail
// @Router /api/v2/aibridge/guardrails [get]
func (api *API) aiGatewayGuardrailsList(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := api.Database.GetAIGatewayGuardrails(ctx, false)
	if err != nil {
		httpInternal(ctx, rw, api, "list AI gateway guardrails", err)
		return
	}
	out := make([]codersdk.AIGatewayGuardrail, 0, len(rows))
	for _, row := range rows {
		versions, err := api.Database.GetAIGatewayGuardrailVersionsByGuardrailID(ctx, row.ID)
		if err != nil {
			httpInternal(ctx, rw, api, "list AI gateway guardrail versions", err)
			return
		}
		out = append(out, db2sdk.AIGatewayGuardrail(row, versions))
	}
	httpapi.Write(ctx, rw, http.StatusOK, out)
}

// @Summary Get an AI gateway guardrail
// @ID get-an-ai-gateway-guardrail
// @Security CoderSessionToken
// @Produce json
// @Tags AI Gateway
// @Param id path string true "Guardrail ID" format(uuid)
// @Success 200 {object} codersdk.AIGatewayGuardrail
// @Router /api/v2/aibridge/guardrails/{id} [get]
func (api *API) aiGatewayGuardrailGet(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseUUIDParam(rw, r, ctx)
	if !ok {
		return
	}
	row, err := api.Database.GetAIGatewayGuardrailByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway guardrail", err)
		return
	}
	versions, err := api.Database.GetAIGatewayGuardrailVersionsByGuardrailID(ctx, id)
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway guardrail versions", err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.AIGatewayGuardrail(row, versions))
}

// @Summary Create an AI gateway guardrail
// @ID create-an-ai-gateway-guardrail
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Gateway
// @Param request body codersdk.CreateAIGatewayGuardrailRequest true "Create guardrail request"
// @Success 201 {object} codersdk.AIGatewayGuardrail
// @Router /api/v2/aibridge/guardrails [post]
func (api *API) aiGatewayGuardrailsCreate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIGatewayGuardrail](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	var req codersdk.CreateAIGatewayGuardrailRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if validations := req.Validate(); len(validations) > 0 {
		httpValidations(ctx, rw, "Invalid guardrail request.", validations)
		return
	}
	// Registration gate: the adapter must be known and the config well-formed.
	if err := adapters.Validate(req.AdapterType, req.Config); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Guardrail failed validation.",
			Detail:  err.Error(),
		})
		return
	}

	var row database.AIGatewayGuardrail
	err := api.Database.InTx(func(tx database.Store) error {
		var txErr error
		row, txErr = tx.InsertAIGatewayGuardrail(ctx, database.InsertAIGatewayGuardrailParams{
			ID:          uuid.New(),
			Name:        req.Name,
			DisplayName: sql.NullString{String: req.DisplayName, Valid: req.DisplayName != ""},
			AdapterType: req.AdapterType,
		})
		if txErr != nil {
			return txErr
		}
		ver, txErr := tx.InsertAIGatewayGuardrailVersion(ctx, database.InsertAIGatewayGuardrailVersionParams{
			ID:            uuid.New(),
			GuardrailID:   row.ID,
			VersionNumber: 1,
			Config:        req.Config,
			Credential:    req.Credential,
			Description:   sql.NullString{String: req.Description, Valid: req.Description != ""},
			CreatedBy:     auditableUserID(r),
		})
		if txErr != nil {
			return txErr
		}
		txErr = tx.UpdateAIGatewayGuardrailActiveVersion(ctx, database.UpdateAIGatewayGuardrailActiveVersionParams{
			ID:              row.ID,
			ActiveVersionID: ver.ID,
		})
		if txErr != nil {
			return txErr
		}
		row.ActiveVersionID = uuid.NullUUID{UUID: ver.ID, Valid: true}
		return nil
	}, &database.TxOptions{TxIdentifier: "create_ai_gateway_guardrail"})
	if err != nil {
		if database.IsUniqueViolation(err) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "A guardrail with that name already exists.",
				Detail:  err.Error(),
			})
			return
		}
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpInternal(ctx, rw, api, "create AI gateway guardrail", err)
		return
	}
	aReq.New = row
	api.publishAIGatewayPipelinesChanged(ctx)

	versions, err := api.Database.GetAIGatewayGuardrailVersionsByGuardrailID(ctx, row.ID)
	if err != nil {
		httpInternal(ctx, rw, api, "load AI gateway guardrail versions", err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.AIGatewayGuardrail(row, versions))
}

// @Summary Create an AI gateway guardrail version
// @ID create-an-ai-gateway-guardrail-version
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Gateway
// @Param id path string true "Guardrail ID" format(uuid)
// @Param request body codersdk.CreateAIGatewayGuardrailVersionRequest true "Create version request"
// @Success 201 {object} codersdk.AIGatewayGuardrailVersion
// @Router /api/v2/aibridge/guardrails/{id}/versions [post]
func (api *API) aiGatewayGuardrailVersionCreate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseUUIDParam(rw, r, ctx)
	if !ok {
		return
	}
	var req codersdk.CreateAIGatewayGuardrailVersionRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if validations := req.Validate(); len(validations) > 0 {
		httpValidations(ctx, rw, "Invalid guardrail version request.", validations)
		return
	}

	g, err := api.Database.GetAIGatewayGuardrailByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway guardrail", err)
		return
	}
	if err := adapters.Validate(g.AdapterType, req.Config); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Guardrail failed validation.",
			Detail:  err.Error(),
		})
		return
	}

	var ver database.AIGatewayGuardrailVersion
	err = api.Database.InTx(func(tx database.Store) error {
		existing, txErr := tx.GetAIGatewayGuardrailVersionsByGuardrailID(ctx, id)
		if txErr != nil {
			return txErr
		}
		next := int32(1)
		if len(existing) > 0 {
			next = existing[0].VersionNumber + 1
		}
		ver, txErr = tx.InsertAIGatewayGuardrailVersion(ctx, database.InsertAIGatewayGuardrailVersionParams{
			ID:            uuid.New(),
			GuardrailID:   id,
			VersionNumber: next,
			Config:        req.Config,
			Credential:    req.Credential,
			Description:   sql.NullString{String: req.Description, Valid: req.Description != ""},
			CreatedBy:     auditableUserID(r),
		})
		if txErr != nil {
			return txErr
		}
		if req.Activate {
			if txErr = tx.UpdateAIGatewayGuardrailActiveVersion(ctx, database.UpdateAIGatewayGuardrailActiveVersionParams{
				ID:              id,
				ActiveVersionID: ver.ID,
			}); txErr != nil {
				return txErr
			}
			return propagateGuardrailVersion(ctx, tx, id, ver.ID, auditableUserID(r), req.Promote)
		}
		return nil
	}, &database.TxOptions{TxIdentifier: "create_ai_gateway_guardrail_version"})
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpInternal(ctx, rw, api, "create AI gateway guardrail version", err)
		return
	}
	if req.Activate {
		api.publishAIGatewayPipelinesChanged(ctx)
	}
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.AIGatewayGuardrailVersion(ver))
}

// @Summary Update an AI gateway guardrail
// @ID update-an-ai-gateway-guardrail
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Gateway
// @Param id path string true "Guardrail ID" format(uuid)
// @Param request body codersdk.UpdateAIGatewayGuardrailRequest true "Update guardrail request"
// @Success 200 {object} codersdk.AIGatewayGuardrail
// @Router /api/v2/aibridge/guardrails/{id} [patch]
func (api *API) aiGatewayGuardrailUpdate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIGatewayGuardrail](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()

	id, ok := parseUUIDParam(rw, r, ctx)
	if !ok {
		return
	}
	var req codersdk.UpdateAIGatewayGuardrailRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.IsEmpty() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "At least one field must be set."})
		return
	}

	old, err := api.Database.GetAIGatewayGuardrailByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway guardrail", err)
		return
	}
	aReq.Old = old

	display := old.DisplayName
	if req.DisplayName != nil {
		display = sql.NullString{String: *req.DisplayName, Valid: *req.DisplayName != ""}
	}
	enabled := old.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	row, err := api.Database.UpdateAIGatewayGuardrail(ctx, database.UpdateAIGatewayGuardrailParams{
		ID:          id,
		DisplayName: display,
		Enabled:     enabled,
	})
	if err != nil {
		httpInternal(ctx, rw, api, "update AI gateway guardrail", err)
		return
	}
	if req.ActiveVersionID != nil {
		err = api.Database.InTx(func(tx database.Store) error {
			if txErr := tx.UpdateAIGatewayGuardrailActiveVersion(ctx, database.UpdateAIGatewayGuardrailActiveVersionParams{
				ID:              id,
				ActiveVersionID: *req.ActiveVersionID,
			}); txErr != nil {
				return txErr
			}
			return propagateGuardrailVersion(ctx, tx, id, *req.ActiveVersionID, auditableUserID(r), req.Promote)
		}, &database.TxOptions{TxIdentifier: "activate_ai_gateway_guardrail_version"})
		if err != nil {
			if database.IsForeignKeyViolation(err) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "active_version_id does not belong to this guardrail.",
				})
				return
			}
			httpInternal(ctx, rw, api, "set AI gateway guardrail active version", err)
			return
		}
		row.ActiveVersionID = uuid.NullUUID{UUID: *req.ActiveVersionID, Valid: true}
	}
	aReq.New = row
	api.publishAIGatewayPipelinesChanged(ctx)

	versions, err := api.Database.GetAIGatewayGuardrailVersionsByGuardrailID(ctx, id)
	if err != nil {
		httpInternal(ctx, rw, api, "load AI gateway guardrail versions", err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.AIGatewayGuardrail(row, versions))
}

// @Summary Delete an AI gateway guardrail
// @ID delete-an-ai-gateway-guardrail
// @Security CoderSessionToken
// @Tags AI Gateway
// @Param id path string true "Guardrail ID" format(uuid)
// @Success 204
// @Router /api/v2/aibridge/guardrails/{id} [delete]
func (api *API) aiGatewayGuardrailDelete(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIGatewayGuardrail](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
	)
	defer commitAudit()

	id, ok := parseUUIDParam(rw, r, ctx)
	if !ok {
		return
	}
	old, err := api.Database.GetAIGatewayGuardrailByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway guardrail", err)
		return
	}
	aReq.Old = old

	count, err := api.Database.CountAIGatewayGuardrailVersionsInActivePipelines(ctx, id)
	if err != nil {
		httpInternal(ctx, rw, api, "check AI gateway guardrail references", err)
		return
	}
	if count > 0 {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Guardrail is in use by an active pipeline; remove it from the pipeline first.",
		})
		return
	}

	if err := api.Database.DeleteAIGatewayGuardrailByID(ctx, id); err != nil {
		httpInternal(ctx, rw, api, "delete AI gateway guardrail", err)
		return
	}
	deleted := old
	deleted.Deleted = true
	aReq.New = deleted
	api.publishAIGatewayPipelinesChanged(ctx)
	rw.WriteHeader(http.StatusNoContent)
}

// propagateGuardrailVersion re-pins every active pipeline that references any
// version of guardrailID to newVersionID, minting a new pipeline version that
// preserves all other members (policies and other guardrails). Mirrors
// propagatePolicyVersion.
func propagateGuardrailVersion(ctx context.Context, tx database.Store, guardrailID, newVersionID uuid.UUID, createdBy uuid.NullUUID, promote bool) error {
	versions, err := tx.GetAIGatewayGuardrailVersionsByGuardrailID(ctx, guardrailID)
	if err != nil {
		return err
	}
	isThisGuardrail := make(map[uuid.UUID]bool, len(versions))
	for _, v := range versions {
		isThisGuardrail[v.ID] = true
	}

	pipelines, err := tx.GetAIGatewayPipelinesReferencingGuardrail(ctx, guardrailID)
	if err != nil {
		return err
	}
	remapGuardrail := func(pinned uuid.UUID) uuid.UUID {
		if isThisGuardrail[pinned] {
			return newVersionID
		}
		return pinned
	}
	for _, pl := range pipelines {
		if !pl.ActiveVersionID.Valid {
			continue
		}
		if err := mintPipelineVersionWithMembers(ctx, tx, pl, createdBy, identityRemap, remapGuardrail, promote); err != nil {
			return err
		}
	}
	return nil
}

func identityRemap(id uuid.UUID) uuid.UUID { return id }

// mintPipelineVersionWithMembers creates a new immutable version of pl that
// copies the members of pl's TIP (latest) version, remapping each member's
// pinned version id through the given functions. When promote is true it also
// activates the minted version; otherwise the version is an unpromoted draft on
// the pipeline's lineage and live posture is unchanged (explicit two-stage
// rollout).
//
// Members are sourced from the tip, not the active version, so successive
// edits accumulate as one linear draft lineage and the tip is always the
// testable "everything staged so far". Copying BOTH member kinds is essential:
// a pipeline version holds policies and guardrails together, so dropping either
// when propagating one would silently remove the other from what runs.
func mintPipelineVersionWithMembers(ctx context.Context, tx database.Store, pl database.AIGatewayPipeline, createdBy uuid.NullUUID, remapPolicyVersion, remapGuardrailVersion func(uuid.UUID) uuid.UUID, promote bool) error {
	existing, err := tx.GetAIGatewayPipelineVersionsByPipelineID(ctx, pl.ID)
	if err != nil {
		return err
	}
	// Source members from the tip (latest) version. Fall back to the active
	// version if, defensively, the pipeline has no version rows.
	source := pl.ActiveVersionID.UUID
	next := int32(1)
	if len(existing) > 0 {
		source = existing[0].ID
		next = existing[0].VersionNumber + 1
	}
	policyMembers, err := tx.GetAIGatewayPipelineVersionPolicies(ctx, source)
	if err != nil {
		return err
	}
	guardrailMembers, err := tx.GetAIGatewayPipelineVersionGuardrails(ctx, source)
	if err != nil {
		return err
	}
	newPV, err := tx.InsertAIGatewayPipelineVersion(ctx, database.InsertAIGatewayPipelineVersionParams{
		ID:            uuid.New(),
		PipelineID:    pl.ID,
		VersionNumber: next,
		CreatedBy:     createdBy,
	})
	if err != nil {
		return err
	}
	for _, m := range policyMembers {
		if _, err := tx.InsertAIGatewayPipelineVersionPolicy(ctx, database.InsertAIGatewayPipelineVersionPolicyParams{
			ID:                uuid.New(),
			PipelineVersionID: newPV.ID,
			PolicyVersionID:   remapPolicyVersion(m.PolicyVersionID),
			Hook:              m.Hook,
			Kind:              m.Kind,
			FailMode:          m.FailMode,
			Enabled:           m.Enabled,
		}); err != nil {
			return err
		}
	}
	for _, m := range guardrailMembers {
		if _, err := tx.InsertAIGatewayPipelineVersionGuardrail(ctx, database.InsertAIGatewayPipelineVersionGuardrailParams{
			ID:                 uuid.New(),
			PipelineVersionID:  newPV.ID,
			GuardrailVersionID: remapGuardrailVersion(m.GuardrailVersionID),
			Hook:               m.Hook,
			Mode:               m.Mode,
			FailMode:           m.FailMode,
			NetworkTimeoutMs:   m.NetworkTimeoutMs,
			Enabled:            m.Enabled,
		}); err != nil {
			return err
		}
	}
	if !promote {
		return nil
	}
	return tx.UpdateAIGatewayPipelineActiveVersion(ctx, database.UpdateAIGatewayPipelineActiveVersionParams{
		ID:              pl.ID,
		ActiveVersionID: newPV.ID,
	})
}
