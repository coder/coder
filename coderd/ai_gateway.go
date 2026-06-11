package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/policy"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	coderpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
)

// AIGatewayPoliciesRoutes mounts the policy CRUD routes. It is mounted by the
// enterprise API at /api/v2/aibridge/policies, gated by the AI Governance
// add-on (FeatureAIBridge).
func (api *API) AIGatewayPoliciesRoutes(r chi.Router) {
	r.Get("/", api.aiGatewayPoliciesList)
	r.Post("/", api.aiGatewayPoliciesCreate)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", api.aiGatewayPolicyGet)
		r.Patch("/", api.aiGatewayPolicyUpdate)
		r.Delete("/", api.aiGatewayPolicyDelete)
		r.Post("/versions", api.aiGatewayPolicyVersionCreate)
	})
}

// AIGatewayPipelinesRoutes mounts the pipeline CRUD routes at
// /api/v2/aibridge/pipelines.
func (api *API) AIGatewayPipelinesRoutes(r chi.Router) {
	r.Get("/", api.aiGatewayPipelinesList)
	r.Post("/", api.aiGatewayPipelinesCreate)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", api.aiGatewayPipelineGet)
		r.Patch("/", api.aiGatewayPipelineUpdate)
		r.Delete("/", api.aiGatewayPipelineDelete)
		r.Get("/versions", api.aiGatewayPipelineVersionsList)
		r.Post("/versions", api.aiGatewayPipelineVersionCreate)
		r.Patch("/members", api.aiGatewayPipelineMemberUpdate)
	})
}

func (api *API) publishAIGatewayPipelinesChanged(ctx context.Context) {
	if api.Pubsub == nil {
		return
	}
	if err := api.Pubsub.Publish(coderpubsub.AIGatewayPipelinesChangedChannel, nil); err != nil {
		api.Logger.Warn(ctx, "publish ai gateway pipelines changed event", slog.Error(err))
	}
}

// --- Policies ---------------------------------------------------------------

// @Summary List AI gateway policies
// @ID list-ai-gateway-policies
// @Security CoderSessionToken
// @Produce json
// @Tags AI Gateway
// @Success 200 {array} codersdk.AIGatewayPolicy
// @Router /api/v2/aibridge/policies [get]
func (api *API) aiGatewayPoliciesList(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := api.Database.GetAIGatewayPolicies(ctx, false)
	if err != nil {
		httpInternal(ctx, rw, api, "list AI gateway policies", err)
		return
	}
	out := make([]codersdk.AIGatewayPolicy, 0, len(rows))
	for _, row := range rows {
		versions, err := api.Database.GetAIGatewayPolicyVersionsByPolicyID(ctx, row.ID)
		if err != nil {
			httpInternal(ctx, rw, api, "list AI gateway policy versions", err)
			return
		}
		out = append(out, db2sdk.AIGatewayPolicy(row, versions))
	}
	httpapi.Write(ctx, rw, http.StatusOK, out)
}

// @Summary Get an AI gateway policy
// @ID get-an-ai-gateway-policy
// @Security CoderSessionToken
// @Produce json
// @Tags AI Gateway
// @Param id path string true "Policy ID" format(uuid)
// @Success 200 {object} codersdk.AIGatewayPolicy
// @Router /api/v2/aibridge/policies/{id} [get]
func (api *API) aiGatewayPolicyGet(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseUUIDParam(rw, r, ctx)
	if !ok {
		return
	}
	row, err := api.Database.GetAIGatewayPolicyByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway policy", err)
		return
	}
	versions, err := api.Database.GetAIGatewayPolicyVersionsByPolicyID(ctx, id)
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway policy versions", err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.AIGatewayPolicy(row, versions))
}

// @Summary Create an AI gateway policy
// @ID create-an-ai-gateway-policy
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Gateway
// @Param request body codersdk.CreateAIGatewayPolicyRequest true "Create policy request"
// @Success 201 {object} codersdk.AIGatewayPolicy
// @Router /api/v2/aibridge/policies [post]
func (api *API) aiGatewayPoliciesCreate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIGatewayPolicy](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	var req codersdk.CreateAIGatewayPolicyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if validations := req.Validate(); len(validations) > 0 {
		httpValidations(ctx, rw, "Invalid policy request.", validations)
		return
	}
	// Registration gate: the Rego must bind its declared kind's entrypoint rule
	// (and have a default verdict for decide). Runs in-process, no opa CLI.
	if err := policy.Validate(policy.Kind(req.Kind), req.Rego); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Policy failed validation.",
			Detail:  err.Error(),
		})
		return
	}

	var row database.AIGatewayPolicy
	err := api.Database.InTx(func(tx database.Store) error {
		var txErr error
		row, txErr = tx.InsertAIGatewayPolicy(ctx, database.InsertAIGatewayPolicyParams{
			ID:          uuid.New(),
			Name:        req.Name,
			DisplayName: sql.NullString{String: req.DisplayName, Valid: req.DisplayName != ""},
			Kind:        database.AIGatewayPolicyKind(req.Kind),
		})
		if txErr != nil {
			return txErr
		}
		ver, txErr := tx.InsertAIGatewayPolicyVersion(ctx, database.InsertAIGatewayPolicyVersionParams{
			ID:                  uuid.New(),
			PolicyID:            row.ID,
			VersionNumber:       1,
			Rego:                req.Rego,
			InputSchemaVersion:  int32(policy.CurrentInputSchemaVersion),
			OutputSchemaVersion: int32(policy.CurrentOutputSchemaVersion),
			Description:         sql.NullString{String: req.Description, Valid: req.Description != ""},
			CreatedBy:           auditableUserID(r),
		})
		if txErr != nil {
			return txErr
		}
		txErr = tx.UpdateAIGatewayPolicyActiveVersion(ctx, database.UpdateAIGatewayPolicyActiveVersionParams{
			ID:              row.ID,
			ActiveVersionID: ver.ID,
		})
		if txErr != nil {
			return txErr
		}
		row.ActiveVersionID = uuid.NullUUID{UUID: ver.ID, Valid: true}
		return nil
	}, &database.TxOptions{TxIdentifier: "create_ai_gateway_policy"})
	if err != nil {
		if database.IsUniqueViolation(err) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "A policy with that name already exists.",
				Detail:  err.Error(),
			})
			return
		}
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpInternal(ctx, rw, api, "create AI gateway policy", err)
		return
	}
	aReq.New = row
	api.publishAIGatewayPipelinesChanged(ctx)

	versions, err := api.Database.GetAIGatewayPolicyVersionsByPolicyID(ctx, row.ID)
	if err != nil {
		httpInternal(ctx, rw, api, "load AI gateway policy versions", err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.AIGatewayPolicy(row, versions))
}

// @Summary Create an AI gateway policy version
// @ID create-an-ai-gateway-policy-version
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Gateway
// @Param id path string true "Policy ID" format(uuid)
// @Param request body codersdk.CreateAIGatewayPolicyVersionRequest true "Create version request"
// @Success 201 {object} codersdk.AIGatewayPolicyVersion
// @Router /api/v2/aibridge/policies/{id}/versions [post]
func (api *API) aiGatewayPolicyVersionCreate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseUUIDParam(rw, r, ctx)
	if !ok {
		return
	}
	var req codersdk.CreateAIGatewayPolicyVersionRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if validations := req.Validate(); len(validations) > 0 {
		httpValidations(ctx, rw, "Invalid policy version request.", validations)
		return
	}

	pol, err := api.Database.GetAIGatewayPolicyByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway policy", err)
		return
	}
	if err := policy.Validate(policy.Kind(pol.Kind), req.Rego); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Policy failed validation.",
			Detail:  err.Error(),
		})
		return
	}

	var ver database.AIGatewayPolicyVersion
	err = api.Database.InTx(func(tx database.Store) error {
		existing, txErr := tx.GetAIGatewayPolicyVersionsByPolicyID(ctx, id)
		if txErr != nil {
			return txErr
		}
		next := int32(1)
		if len(existing) > 0 {
			next = existing[0].VersionNumber + 1
		}
		ver, txErr = tx.InsertAIGatewayPolicyVersion(ctx, database.InsertAIGatewayPolicyVersionParams{
			ID:                  uuid.New(),
			PolicyID:            id,
			VersionNumber:       next,
			Rego:                req.Rego,
			InputSchemaVersion:  int32(policy.CurrentInputSchemaVersion),
			OutputSchemaVersion: int32(policy.CurrentOutputSchemaVersion),
			Description:         sql.NullString{String: req.Description, Valid: req.Description != ""},
			CreatedBy:           auditableUserID(r),
		})
		if txErr != nil {
			return txErr
		}
		if req.Activate {
			if txErr = tx.UpdateAIGatewayPolicyActiveVersion(ctx, database.UpdateAIGatewayPolicyActiveVersionParams{
				ID:              id,
				ActiveVersionID: ver.ID,
			}); txErr != nil {
				return txErr
			}
			// Activation propagates to every referencing pipeline by minting a
			// new pipeline version on its tip. It only goes live (activates the
			// minted version) when the caller opts in with promote; otherwise
			// the mint is an unpromoted draft.
			return propagatePolicyVersion(ctx, tx, id, ver.ID, auditableUserID(r), req.Promote)
		}
		return nil
	}, &database.TxOptions{TxIdentifier: "create_ai_gateway_policy_version"})
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpInternal(ctx, rw, api, "create AI gateway policy version", err)
		return
	}
	if req.Activate {
		api.publishAIGatewayPipelinesChanged(ctx)
	}
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.AIGatewayPolicyVersion(ver))
}

// @Summary Update an AI gateway policy
// @ID update-an-ai-gateway-policy
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Gateway
// @Param id path string true "Policy ID" format(uuid)
// @Param request body codersdk.UpdateAIGatewayPolicyRequest true "Update policy request"
// @Success 200 {object} codersdk.AIGatewayPolicy
// @Router /api/v2/aibridge/policies/{id} [patch]
func (api *API) aiGatewayPolicyUpdate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIGatewayPolicy](rw, &audit.RequestParams{
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
	var req codersdk.UpdateAIGatewayPolicyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.IsEmpty() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "At least one field must be set."})
		return
	}

	old, err := api.Database.GetAIGatewayPolicyByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway policy", err)
		return
	}
	aReq.Old = old

	display := old.DisplayName
	if req.DisplayName != nil {
		display = sql.NullString{String: *req.DisplayName, Valid: *req.DisplayName != ""}
	}

	row, err := api.Database.UpdateAIGatewayPolicy(ctx, database.UpdateAIGatewayPolicyParams{
		ID:          id,
		DisplayName: display,
	})
	if err != nil {
		httpInternal(ctx, rw, api, "update AI gateway policy", err)
		return
	}
	if req.ActiveVersionID != nil {
		// Activating a version (including reverting to an older one) propagates
		// to every referencing pipeline by minting a new pipeline version on its
		// tip. It goes live only when promote is set; otherwise the mint is an
		// unpromoted draft and live posture is unchanged.
		err = api.Database.InTx(func(tx database.Store) error {
			if txErr := tx.UpdateAIGatewayPolicyActiveVersion(ctx, database.UpdateAIGatewayPolicyActiveVersionParams{
				ID:              id,
				ActiveVersionID: *req.ActiveVersionID,
			}); txErr != nil {
				return txErr
			}
			return propagatePolicyVersion(ctx, tx, id, *req.ActiveVersionID, auditableUserID(r), req.Promote)
		}, &database.TxOptions{TxIdentifier: "activate_ai_gateway_policy_version"})
		if err != nil {
			// A foreign-key violation means the version does not belong to this
			// policy.
			if database.IsForeignKeyViolation(err) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "active_version_id does not belong to this policy.",
				})
				return
			}
			httpInternal(ctx, rw, api, "set AI gateway policy active version", err)
			return
		}
		row.ActiveVersionID = uuid.NullUUID{UUID: *req.ActiveVersionID, Valid: true}
	}
	aReq.New = row
	api.publishAIGatewayPipelinesChanged(ctx)

	versions, err := api.Database.GetAIGatewayPolicyVersionsByPolicyID(ctx, id)
	if err != nil {
		httpInternal(ctx, rw, api, "load AI gateway policy versions", err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.AIGatewayPolicy(row, versions))
}

// @Summary Delete an AI gateway policy
// @ID delete-an-ai-gateway-policy
// @Security CoderSessionToken
// @Tags AI Gateway
// @Param id path string true "Policy ID" format(uuid)
// @Success 204
// @Router /api/v2/aibridge/policies/{id} [delete]
func (api *API) aiGatewayPolicyDelete(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIGatewayPolicy](rw, &audit.RequestParams{
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
	old, err := api.Database.GetAIGatewayPolicyByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway policy", err)
		return
	}
	aReq.Old = old

	// Block delete while a version is referenced by an active pipeline; the
	// operator must first remove it from the pipeline.
	count, err := api.Database.CountAIGatewayPolicyVersionsInActivePipelines(ctx, id)
	if err != nil {
		httpInternal(ctx, rw, api, "check AI gateway policy references", err)
		return
	}
	if count > 0 {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Policy is in use by an active pipeline; remove it from the pipeline first.",
		})
		return
	}

	if err := api.Database.DeleteAIGatewayPolicyByID(ctx, id); err != nil {
		httpInternal(ctx, rw, api, "delete AI gateway policy", err)
		return
	}
	deleted := old
	deleted.Deleted = true
	aReq.New = deleted
	api.publishAIGatewayPipelinesChanged(ctx)
	rw.WriteHeader(http.StatusNoContent)
}

// propagatePolicyVersion re-pins every active pipeline that references any
// version of policyID to newVersionID. For each affected pipeline it mints a new
// immutable pipeline version that copies the current membership, swapping only
// the entries that belong to this policy, then activates it. This keeps
// composition history while propagating an activated policy edit to what runs.
func propagatePolicyVersion(ctx context.Context, tx database.Store, policyID, newVersionID uuid.UUID, createdBy uuid.NullUUID, promote bool) error {
	policyVersions, err := tx.GetAIGatewayPolicyVersionsByPolicyID(ctx, policyID)
	if err != nil {
		return err
	}
	isThisPolicy := make(map[uuid.UUID]bool, len(policyVersions))
	for _, v := range policyVersions {
		isThisPolicy[v.ID] = true
	}

	pipelines, err := tx.GetAIGatewayPipelinesReferencingPolicy(ctx, policyID)
	if err != nil {
		return err
	}
	remapPolicy := func(pinned uuid.UUID) uuid.UUID {
		if isThisPolicy[pinned] {
			return newVersionID
		}
		return pinned
	}
	for _, pl := range pipelines {
		if !pl.ActiveVersionID.Valid {
			continue
		}
		if err := mintPipelineVersionWithMembers(ctx, tx, pl, createdBy, remapPolicy, identityRemap, promote); err != nil {
			return err
		}
	}
	return nil
}

// --- helpers ----------------------------------------------------------------

func parseUUIDParam(rw http.ResponseWriter, r *http.Request, ctx context.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid UUID in path."})
		return uuid.Nil, false
	}
	return id, true
}

func httpValidations(ctx context.Context, rw http.ResponseWriter, msg string, validations []codersdk.ValidationError) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message:     msg,
		Validations: validations,
	})
}

func httpInternal(ctx context.Context, rw http.ResponseWriter, api *API, msg string, err error) {
	api.Logger.Error(ctx, msg, slog.Error(err))
	httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
		Message: "Internal error: " + msg + ".",
		Detail:  err.Error(),
	})
}

// auditableUserID returns the requesting user's ID as a NullUUID for created_by
// columns. Routes are behind apiKeyMiddleware, so an API key is always present.
func auditableUserID(r *http.Request) uuid.NullUUID {
	return uuid.NullUUID{UUID: httpmw.APIKey(r).UserID, Valid: true}
}
