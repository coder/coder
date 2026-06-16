package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// resolvedMember is a request membership with the kind resolved from its policy
// version.
type resolvedMember struct {
	policyVersionID uuid.UUID
	name            string
	hook            database.AIGatewayHook
	kind            database.AIGatewayPolicyKind
	failMode        database.AIGatewayFailMode
	enabled         bool
}

// resolvePipelinePolicies fetches each referenced policy version's kind and
// enforces kind-validity-by-hook plus per-hook cardinality. It returns a
// codersdk.Response describing the problem (and ok=false) on a client error.
func resolvePipelinePolicies(ctx context.Context, db database.Store, reqs []codersdk.AIGatewayPipelinePolicyRequest) ([]resolvedMember, *codersdk.Response) {
	out := make([]resolvedMember, 0, len(reqs))
	// Track single-instance kinds per hook.
	type hk struct {
		hook database.AIGatewayHook
		kind database.AIGatewayPolicyKind
	}
	seen := make(map[hk]bool)

	for i, req := range reqs {
		ver, err := db.GetAIGatewayPolicyVersionByID(ctx, req.PolicyVersionID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &codersdk.Response{Message: fmt.Sprintf("policies[%d]: policy version not found", i)}
		}
		if err != nil {
			return nil, &codersdk.Response{Message: "Internal error resolving policy version.", Detail: err.Error()}
		}
		pol, err := db.GetAIGatewayPolicyByID(ctx, ver.PolicyID)
		if err != nil {
			return nil, &codersdk.Response{Message: "Internal error resolving policy.", Detail: err.Error()}
		}

		hook := database.AIGatewayHook(req.Hook)
		kind := pol.Kind
		if !aiGatewayHookAllowsKind(hook, kind) {
			return nil, &codersdk.Response{Message: fmt.Sprintf("policies[%d]: kind %q is not valid at hook %q", i, kind, hook)}
		}
		// annotate/route/transform are capped at one per hook.
		if kind != database.AIGatewayPolicyKindDecide {
			key := hk{hook, kind}
			if seen[key] {
				return nil, &codersdk.Response{Message: fmt.Sprintf("policies[%d]: at most one %q policy per hook", i, kind)}
			}
			seen[key] = true
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		out = append(out, resolvedMember{
			policyVersionID: req.PolicyVersionID,
			name:            pol.Name,
			hook:            hook,
			kind:            kind,
			failMode:        database.AIGatewayFailMode(req.FailMode),
			enabled:         enabled,
		})
	}
	return out, nil
}

// checkMemberNameCollision rejects a pipeline version whose member policy names
// and guardrail names overlap. Names key the first level of input.annotations
// (the host-owned namespace), so a policy and a guardrail sharing a name would
// collide there. Cross-table uniqueness cannot be a DB index, so it is checked
// here at version-create time.
func checkMemberNameCollision(members []resolvedMember, guardrails []resolvedGuardrail) *codersdk.Response {
	names := make(map[string]struct{}, len(members))
	for _, m := range members {
		names[m.name] = struct{}{}
	}
	for _, g := range guardrails {
		if _, dup := names[g.name]; dup {
			return &codersdk.Response{
				Message: fmt.Sprintf("member name %q is used by both a policy and a guardrail; names must be unique within a pipeline because they namespace annotations", g.name),
			}
		}
	}
	return nil
}

func aiGatewayHookAllowsKind(hook database.AIGatewayHook, kind database.AIGatewayPolicyKind) bool {
	switch hook {
	case database.AIGatewayHookPreAuth:
		return kind == database.AiGatewayPolicyKindAnnotate || kind == database.AIGatewayPolicyKindDecide
	case database.AIGatewayHookPreReq:
		return true
	case database.AIGatewayHookPreTool:
		// The request is already dispatched at pre-tool, so route and transform
		// do not apply; only annotate and decide gate a tool call.
		return kind == database.AiGatewayPolicyKindAnnotate || kind == database.AIGatewayPolicyKindDecide
	default:
		return false
	}
}

func insertPipelineMembers(ctx context.Context, tx database.Store, versionID uuid.UUID, members []resolvedMember) error {
	for _, m := range members {
		if _, err := tx.InsertAIGatewayPipelineVersionPolicy(ctx, database.InsertAIGatewayPipelineVersionPolicyParams{
			ID:                uuid.New(),
			PipelineVersionID: versionID,
			PolicyVersionID:   m.policyVersionID,
			Hook:              m.hook,
			Kind:              m.kind,
			FailMode:          m.failMode,
			Enabled:           m.enabled,
		}); err != nil {
			return err
		}
	}
	return nil
}

// resolvedGuardrail is a request guardrail membership validated for insertion.
type resolvedGuardrail struct {
	guardrailVersionID uuid.UUID
	name               string
	hook               database.AIGatewayHook
	failMode           database.AIGatewayFailMode
	networkTimeoutMs   int32
	enabled            bool
}

// resolvePipelineGuardrails validates each referenced guardrail version exists.
// Guardrails have no kind and no per-hook cardinality cap (many concurrent
// guardrails per hook are allowed), so this only checks existence and fills
// defaults. It returns a codersdk.Response (ok=false) on a client error.
func resolvePipelineGuardrails(ctx context.Context, db database.Store, reqs []codersdk.AIGatewayPipelineGuardrailRequest) ([]resolvedGuardrail, *codersdk.Response) {
	out := make([]resolvedGuardrail, 0, len(reqs))
	for i, req := range reqs {
		ver, err := db.GetAIGatewayGuardrailVersionByID(ctx, req.GuardrailVersionID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &codersdk.Response{Message: fmt.Sprintf("guardrails[%d]: guardrail version not found", i)}
		} else if err != nil {
			return nil, &codersdk.Response{Message: "Internal error resolving guardrail version.", Detail: err.Error()}
		}
		gr, err := db.GetAIGatewayGuardrailByID(ctx, ver.GuardrailID)
		if err != nil {
			return nil, &codersdk.Response{Message: "Internal error resolving guardrail.", Detail: err.Error()}
		}
		timeout := int32(2000)
		if req.NetworkTimeoutMS != nil {
			timeout = *req.NetworkTimeoutMS
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		out = append(out, resolvedGuardrail{
			guardrailVersionID: req.GuardrailVersionID,
			name:               gr.Name,
			hook:               database.AIGatewayHook(req.Hook),
			failMode:           database.AIGatewayFailMode(req.FailMode),
			networkTimeoutMs:   timeout,
			enabled:            enabled,
		})
	}
	return out, nil
}

func insertPipelineGuardrails(ctx context.Context, tx database.Store, versionID uuid.UUID, members []resolvedGuardrail) error {
	for _, m := range members {
		if _, err := tx.InsertAIGatewayPipelineVersionGuardrail(ctx, database.InsertAIGatewayPipelineVersionGuardrailParams{
			ID:                 uuid.New(),
			PipelineVersionID:  versionID,
			GuardrailVersionID: m.guardrailVersionID,
			Hook:               m.hook,
			FailMode:           m.failMode,
			NetworkTimeoutMs:   m.networkTimeoutMs,
			Enabled:            m.enabled,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (api *API) loadPipelineSDK(ctx context.Context, row database.AIGatewayPipeline) (codersdk.AIGatewayPipeline, error) {
	// The tip (latest) version is loaded so the SDK can report minted-but-
	// unpromoted drift (latest ahead of active) for the two-stage rollout UX.
	allVersions, err := api.Database.GetAIGatewayPipelineVersionsByPipelineID(ctx, row.ID)
	if err != nil {
		return codersdk.AIGatewayPipeline{}, err
	}
	var (
		latest           *database.AIGatewayPipelineVersion
		latestMembers    []database.AIGatewayPipelineVersionPolicy
		latestGuardrails []database.AIGatewayPipelineVersionGuardrail
	)
	if len(allVersions) > 0 {
		// Ordered version_number DESC, so the first row is the tip. Its full
		// membership is loaded so the UI can base edits on the tip (the staged
		// draft lineage), not the active version. Basing an edit on the active
		// version would drop members added in an unpromoted draft.
		latest = &allVersions[0]
		latestMembers, err = api.Database.GetAIGatewayPipelineVersionPolicies(ctx, latest.ID)
		if err != nil {
			return codersdk.AIGatewayPipeline{}, err
		}
		latestGuardrails, err = api.Database.GetAIGatewayPipelineVersionGuardrails(ctx, latest.ID)
		if err != nil {
			return codersdk.AIGatewayPipeline{}, err
		}
	}

	if !row.ActiveVersionID.Valid {
		return db2sdk.AIGatewayPipelineWithLatest(row, nil, nil, nil, latest, latestMembers, latestGuardrails), nil
	}

	activeID := row.ActiveVersionID.UUID
	var (
		members    []database.AIGatewayPipelineVersionPolicy
		guardrails []database.AIGatewayPipelineVersionGuardrail
	)
	if latest != nil && latest.ID == activeID {
		// The active version is the tip; reuse the membership already loaded.
		members, guardrails = latestMembers, latestGuardrails
	} else {
		members, err = api.Database.GetAIGatewayPipelineVersionPolicies(ctx, activeID)
		if err != nil {
			return codersdk.AIGatewayPipeline{}, err
		}
		guardrails, err = api.Database.GetAIGatewayPipelineVersionGuardrails(ctx, activeID)
		if err != nil {
			return codersdk.AIGatewayPipeline{}, err
		}
	}
	ver := database.AIGatewayPipelineVersion{ID: activeID, PipelineID: row.ID}
	return db2sdk.AIGatewayPipelineWithLatest(row, &ver, members, guardrails, latest, latestMembers, latestGuardrails), nil
}

// @Summary List AI gateway pipelines
// @ID list-ai-gateway-pipelines
// @Security CoderSessionToken
// @Produce json
// @Tags AI Gateway
// @Success 200 {array} codersdk.AIGatewayPipeline
// @Router /api/v2/aibridge/pipelines [get]
func (api *API) aiGatewayPipelinesList(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := api.Database.GetAIGatewayPipelines(ctx, database.GetAIGatewayPipelinesParams{
		IncludeDisabled: true,
	})
	if err != nil {
		httpInternal(ctx, rw, api, "list AI gateway pipelines", err)
		return
	}
	out := make([]codersdk.AIGatewayPipeline, 0, len(rows))
	for _, row := range rows {
		sdk, err := api.loadPipelineSDK(ctx, row)
		if err != nil {
			httpInternal(ctx, rw, api, "load AI gateway pipeline", err)
			return
		}
		out = append(out, sdk)
	}
	httpapi.Write(ctx, rw, http.StatusOK, out)
}

// @Summary Get an AI gateway pipeline
// @ID get-an-ai-gateway-pipeline
// @Security CoderSessionToken
// @Produce json
// @Tags AI Gateway
// @Param id path string true "Pipeline ID" format(uuid)
// @Success 200 {object} codersdk.AIGatewayPipeline
// @Router /api/v2/aibridge/pipelines/{id} [get]
func (api *API) aiGatewayPipelineGet(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseUUIDParam(rw, r, ctx)
	if !ok {
		return
	}
	row, err := api.Database.GetAIGatewayPipelineByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway pipeline", err)
		return
	}
	sdk, err := api.loadPipelineSDK(ctx, row)
	if err != nil {
		httpInternal(ctx, rw, api, "load AI gateway pipeline", err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, sdk)
}

// @Summary List AI gateway pipeline versions
// @ID list-ai-gateway-pipeline-versions
// @Security CoderSessionToken
// @Produce json
// @Tags AI Gateway
// @Param id path string true "Pipeline ID" format(uuid)
// @Success 200 {array} codersdk.AIGatewayPipelineVersion
// @Router /api/v2/aibridge/pipelines/{id}/versions [get]
func (api *API) aiGatewayPipelineVersionsList(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseUUIDParam(rw, r, ctx)
	if !ok {
		return
	}
	if _, err := api.Database.GetAIGatewayPipelineByID(ctx, id); errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	} else if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway pipeline", err)
		return
	}
	versions, err := api.Database.GetAIGatewayPipelineVersionsByPipelineID(ctx, id)
	if err != nil {
		httpInternal(ctx, rw, api, "list AI gateway pipeline versions", err)
		return
	}
	out := make([]codersdk.AIGatewayPipelineVersion, 0, len(versions))
	for _, ver := range versions {
		members, err := api.Database.GetAIGatewayPipelineVersionPolicies(ctx, ver.ID)
		if err != nil {
			httpInternal(ctx, rw, api, "load AI gateway pipeline version policies", err)
			return
		}
		guardrails, err := api.Database.GetAIGatewayPipelineVersionGuardrails(ctx, ver.ID)
		if err != nil {
			httpInternal(ctx, rw, api, "load AI gateway pipeline version guardrails", err)
			return
		}
		out = append(out, db2sdk.AIGatewayPipelineVersion(ver, members, guardrails))
	}
	httpapi.Write(ctx, rw, http.StatusOK, out)
}

// @Summary Create an AI gateway pipeline
// @ID create-an-ai-gateway-pipeline
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Gateway
// @Param request body codersdk.CreateAIGatewayPipelineRequest true "Create pipeline request"
// @Success 201 {object} codersdk.AIGatewayPipeline
// @Router /api/v2/aibridge/pipelines [post]
func (api *API) aiGatewayPipelinesCreate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIGatewayPipeline](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()

	var req codersdk.CreateAIGatewayPipelineRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if validations := req.Validate(); len(validations) > 0 {
		httpValidations(ctx, rw, "Invalid pipeline request.", validations)
		return
	}
	if _, err := api.Database.GetAIProviderByID(ctx, req.ProviderID); errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "provider_id does not reference a live provider."})
		return
	} else if err != nil {
		httpInternal(ctx, rw, api, "get provider", err)
		return
	}
	members, problem := resolvePipelinePolicies(ctx, api.Database, req.Policies)
	if problem != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *problem)
		return
	}
	guardrails, problem := resolvePipelineGuardrails(ctx, api.Database, req.Guardrails)
	if problem != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *problem)
		return
	}
	if problem := checkMemberNameCollision(members, guardrails); problem != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *problem)
		return
	}

	var row database.AIGatewayPipeline
	err := api.Database.InTx(func(tx database.Store) error {
		var txErr error
		row, txErr = tx.InsertAIGatewayPipeline(ctx, database.InsertAIGatewayPipelineParams{
			ID:         uuid.New(),
			ProviderID: req.ProviderID,
			Enabled:    req.Enabled,
		})
		if txErr != nil {
			return txErr
		}
		ver, txErr := tx.InsertAIGatewayPipelineVersion(ctx, database.InsertAIGatewayPipelineVersionParams{
			ID:            uuid.New(),
			PipelineID:    row.ID,
			VersionNumber: 1,
			CreatedBy:     auditableUserID(r),
		})
		if txErr != nil {
			return txErr
		}
		if txErr = insertPipelineMembers(ctx, tx, ver.ID, members); txErr != nil {
			return txErr
		}
		if txErr = insertPipelineGuardrails(ctx, tx, ver.ID, guardrails); txErr != nil {
			return txErr
		}
		txErr = tx.UpdateAIGatewayPipelineActiveVersion(ctx, database.UpdateAIGatewayPipelineActiveVersionParams{
			ID:              row.ID,
			ActiveVersionID: ver.ID,
		})
		if txErr != nil {
			return txErr
		}
		row.ActiveVersionID = uuid.NullUUID{UUID: ver.ID, Valid: true}
		return nil
	}, &database.TxOptions{TxIdentifier: "create_ai_gateway_pipeline"})
	if err != nil {
		if database.IsUniqueViolation(err) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "This provider already has a pipeline.",
				Detail:  err.Error(),
			})
			return
		}
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpInternal(ctx, rw, api, "create AI gateway pipeline", err)
		return
	}
	aReq.New = row
	api.publishAIGatewayPipelinesChanged(ctx)

	sdk, err := api.loadPipelineSDK(ctx, row)
	if err != nil {
		httpInternal(ctx, rw, api, "load AI gateway pipeline", err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, sdk)
}

// @Summary Create an AI gateway pipeline version
// @ID create-an-ai-gateway-pipeline-version
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Gateway
// @Param id path string true "Pipeline ID" format(uuid)
// @Param request body codersdk.CreateAIGatewayPipelineVersionRequest true "Create version request"
// @Success 201 {object} codersdk.AIGatewayPipelineVersion
// @Router /api/v2/aibridge/pipelines/{id}/versions [post]
func (api *API) aiGatewayPipelineVersionCreate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseUUIDParam(rw, r, ctx)
	if !ok {
		return
	}
	var req codersdk.CreateAIGatewayPipelineVersionRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if validations := req.Validate(); len(validations) > 0 {
		httpValidations(ctx, rw, "Invalid pipeline version request.", validations)
		return
	}
	if _, err := api.Database.GetAIGatewayPipelineByID(ctx, id); errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	} else if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway pipeline", err)
		return
	}
	members, problem := resolvePipelinePolicies(ctx, api.Database, req.Policies)
	if problem != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *problem)
		return
	}
	guardrails, problem := resolvePipelineGuardrails(ctx, api.Database, req.Guardrails)
	if problem != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *problem)
		return
	}
	if problem := checkMemberNameCollision(members, guardrails); problem != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *problem)
		return
	}

	var ver database.AIGatewayPipelineVersion
	err := api.Database.InTx(func(tx database.Store) error {
		existing, err := tx.GetAIGatewayPipelineVersionsByPipelineID(ctx, id)
		if err != nil {
			return err
		}
		next := int32(1)
		if len(existing) > 0 {
			next = existing[0].VersionNumber + 1
		}
		ver, err = tx.InsertAIGatewayPipelineVersion(ctx, database.InsertAIGatewayPipelineVersionParams{
			ID:            uuid.New(),
			PipelineID:    id,
			VersionNumber: next,
			CreatedBy:     auditableUserID(r),
		})
		if err != nil {
			return err
		}
		if err = insertPipelineMembers(ctx, tx, ver.ID, members); err != nil {
			return err
		}
		if err = insertPipelineGuardrails(ctx, tx, ver.ID, guardrails); err != nil {
			return err
		}
		if req.Activate {
			return tx.UpdateAIGatewayPipelineActiveVersion(ctx, database.UpdateAIGatewayPipelineActiveVersionParams{
				ID:              id,
				ActiveVersionID: ver.ID,
			})
		}
		return nil
	}, &database.TxOptions{TxIdentifier: "create_ai_gateway_pipeline_version"})
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpInternal(ctx, rw, api, "create AI gateway pipeline version", err)
		return
	}
	if req.Activate {
		api.publishAIGatewayPipelinesChanged(ctx)
	}
	members2, err := api.Database.GetAIGatewayPipelineVersionPolicies(ctx, ver.ID)
	if err != nil {
		httpInternal(ctx, rw, api, "load AI gateway pipeline version members", err)
		return
	}
	guardrails2, err := api.Database.GetAIGatewayPipelineVersionGuardrails(ctx, ver.ID)
	if err != nil {
		httpInternal(ctx, rw, api, "load AI gateway pipeline version guardrails", err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.AIGatewayPipelineVersion(ver, members2, guardrails2))
}

// @Summary Update an AI gateway pipeline
// @ID update-an-ai-gateway-pipeline
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Gateway
// @Param id path string true "Pipeline ID" format(uuid)
// @Param request body codersdk.UpdateAIGatewayPipelineRequest true "Update pipeline request"
// @Success 200 {object} codersdk.AIGatewayPipeline
// @Router /api/v2/aibridge/pipelines/{id} [patch]
func (api *API) aiGatewayPipelineUpdate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIGatewayPipeline](rw, &audit.RequestParams{
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
	var req codersdk.UpdateAIGatewayPipelineRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.IsEmpty() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "At least one field must be set."})
		return
	}
	old, err := api.Database.GetAIGatewayPipelineByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway pipeline", err)
		return
	}
	aReq.Old = old

	enabled := old.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row, err := api.Database.UpdateAIGatewayPipeline(ctx, database.UpdateAIGatewayPipelineParams{
		ID:      id,
		Enabled: enabled,
	})
	if err != nil {
		httpInternal(ctx, rw, api, "update AI gateway pipeline", err)
		return
	}
	if req.ActiveVersionID != nil {
		if err := api.Database.UpdateAIGatewayPipelineActiveVersion(ctx, database.UpdateAIGatewayPipelineActiveVersionParams{
			ID:              id,
			ActiveVersionID: *req.ActiveVersionID,
		}); err != nil {
			if database.IsForeignKeyViolation(err) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "active_version_id does not belong to this pipeline."})
				return
			}
			httpInternal(ctx, rw, api, "set AI gateway pipeline active version", err)
			return
		}
		row.ActiveVersionID = uuid.NullUUID{UUID: *req.ActiveVersionID, Valid: true}
	}
	aReq.New = row
	api.publishAIGatewayPipelinesChanged(ctx)

	sdk, err := api.loadPipelineSDK(ctx, row)
	if err != nil {
		httpInternal(ctx, rw, api, "load AI gateway pipeline", err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, sdk)
}

// @Summary Enable or disable an AI gateway pipeline member
// @ID enable-or-disable-an-ai-gateway-pipeline-member
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags AI Gateway
// @Param id path string true "Pipeline ID" format(uuid)
// @Param request body codersdk.UpdateAIGatewayPipelineMemberRequest true "Update member request"
// @Success 200 {object} codersdk.AIGatewayPipeline
// @Router /api/v2/aibridge/pipelines/{id}/members [patch]
func (api *API) aiGatewayPipelineMemberUpdate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseUUIDParam(rw, r, ctx)
	if !ok {
		return
	}
	var req codersdk.UpdateAIGatewayPipelineMemberRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if validations := req.Validate(); len(validations) > 0 {
		httpValidations(ctx, rw, "Invalid pipeline member request.", validations)
		return
	}

	row, err := api.Database.GetAIGatewayPipelineByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway pipeline", err)
		return
	}
	// Enable/disable applies to the live (active) version so it takes effect
	// immediately. A member that exists only in an unpromoted draft cannot be
	// paused live until it is promoted.
	if !row.ActiveVersionID.Valid {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Pipeline has no active version to update."})
		return
	}

	if req.PolicyVersionID != nil {
		err = api.Database.UpdateAIGatewayPipelineVersionPolicyEnabled(ctx, database.UpdateAIGatewayPipelineVersionPolicyEnabledParams{
			PipelineVersionID: row.ActiveVersionID.UUID,
			PolicyVersionID:   *req.PolicyVersionID,
			Hook:              database.AIGatewayHook(req.Hook),
			Enabled:           req.Enabled,
		})
	} else {
		err = api.Database.UpdateAIGatewayPipelineVersionGuardrailEnabled(ctx, database.UpdateAIGatewayPipelineVersionGuardrailEnabledParams{
			PipelineVersionID:  row.ActiveVersionID.UUID,
			GuardrailVersionID: *req.GuardrailVersionID,
			Hook:               database.AIGatewayHook(req.Hook),
			Enabled:            req.Enabled,
		})
	}
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpInternal(ctx, rw, api, "update AI gateway pipeline member", err)
		return
	}
	api.publishAIGatewayPipelinesChanged(ctx)

	sdk, err := api.loadPipelineSDK(ctx, row)
	if err != nil {
		httpInternal(ctx, rw, api, "load AI gateway pipeline", err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, sdk)
}

// @Summary Delete an AI gateway pipeline
// @ID delete-an-ai-gateway-pipeline
// @Security CoderSessionToken
// @Tags AI Gateway
// @Param id path string true "Pipeline ID" format(uuid)
// @Success 204
// @Router /api/v2/aibridge/pipelines/{id} [delete]
func (api *API) aiGatewayPipelineDelete(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.AIGatewayPipeline](rw, &audit.RequestParams{
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
	old, err := api.Database.GetAIGatewayPipelineByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpInternal(ctx, rw, api, "get AI gateway pipeline", err)
		return
	}
	aReq.Old = old
	if err := api.Database.DeleteAIGatewayPipelineByID(ctx, id); err != nil {
		httpInternal(ctx, rw, api, "delete AI gateway pipeline", err)
		return
	}
	deleted := old
	deleted.Deleted = true
	deleted.Enabled = false
	aReq.New = deleted
	api.publishAIGatewayPipelinesChanged(ctx)
	rw.WriteHeader(http.StatusNoContent)
}
