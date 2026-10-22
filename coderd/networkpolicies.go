package coderd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/networkpolicy"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

type networkPolicyHTTPError struct {
	status  int
	message string
}

func (e *networkPolicyHTTPError) Error() string { return e.message }
func networkPolicyConflict(message string) error {
	return &networkPolicyHTTPError{http.StatusConflict, message}
}
func networkPolicyBadRequest(message string) error {
	return &networkPolicyHTTPError{http.StatusBadRequest, message}
}

func (api *API) networkPolicyError(rw http.ResponseWriter, r *http.Request, err error) {
	status, message := http.StatusInternalServerError, "Internal error processing network policy."
	var requestErr *networkPolicyHTTPError
	switch {
	case errors.As(err, &requestErr):
		status, message = requestErr.status, requestErr.message
	case errors.Is(err, sql.ErrNoRows), dbauthz.IsNotAuthorizedError(err):
		status, message = http.StatusNotFound, "Network policy resource not found."
	case database.IsUniqueViolation(err):
		status, message = http.StatusConflict, "Network policy resource already exists."
	default:
		api.Logger.Error(r.Context(), "network policy request", slog.Error(err))
	}
	httpapi.Write(r.Context(), rw, status, codersdk.Response{Message: message})
}

func networkPolicyByRequest(ctx context.Context, r *http.Request, fetch func(context.Context, uuid.UUID) (database.NetworkPolicy, error)) (database.NetworkPolicy, error) {
	id, err := uuid.Parse(chi.URLParam(r, "networkpolicy"))
	if err != nil {
		return database.NetworkPolicy{}, sql.ErrNoRows
	}
	p, err := fetch(ctx, id)
	if err != nil {
		return p, err
	}
	if p.OrganizationID != httpmw.OrganizationParam(r).ID {
		return p, sql.ErrNoRows
	}
	return p, nil
}

// @Summary List network policies
// @ID list-network-policies
// @Security CoderSessionToken
// @Tags Network policies
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.NetworkPolicy
// @Router /api/experimental/organizations/{organization}/network-policies [get]
// @x-apidocgen {"skip": true}
func (api *API) networkPolicies(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	policies, err := api.Database.GetNetworkPoliciesByOrganizationID(ctx, httpmw.OrganizationParam(r).ID)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	result := make([]codersdk.NetworkPolicy, 0, len(policies))
	for _, p := range policies {
		detail, err := networkPolicyDetail(ctx, api.Database, p)
		if err != nil {
			api.networkPolicyError(rw, r, err)
			return
		}
		result = append(result, detail)
	}
	httpapi.Write(ctx, rw, http.StatusOK, result)
}

// @Summary List administrable templates for network policies
// @ID list-network-policy-templates
// @Security CoderSessionToken
// @Tags Network policies
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.Template
// @Router /api/experimental/organizations/{organization}/network-policies/templates [get]
// @x-apidocgen {"skip": true}
func (api *API) networkPolicyTemplateList(rw http.ResponseWriter, r *http.Request) {
	templates, err := api.Database.GetTemplatesWithFilter(r.Context(), database.GetTemplatesWithFilterParams{
		OrganizationID: httpmw.OrganizationParam(r).ID,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		api.networkPolicyError(rw, r, err)
		return
	}
	administrable := make([]database.Template, 0, len(templates))
	for _, template := range templates {
		if api.Authorize(r, policy.ActionUpdate, template.RBACObject()) {
			administrable = append(administrable, template)
		}
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, api.convertTemplates(administrable))
}

// @Summary Get network policy
// @ID get-network-policy
// @Security CoderSessionToken
// @Tags Network policies
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param networkpolicy path string true "Network policy ID" format(uuid)
// @Success 200 {object} codersdk.NetworkPolicy
// @Router /api/experimental/organizations/{organization}/network-policies/{networkpolicy} [get]
// @x-apidocgen {"skip": true}
func (api *API) networkPolicyByID(rw http.ResponseWriter, r *http.Request) {
	p, err := networkPolicyByRequest(r.Context(), r, api.Database.GetNetworkPolicyByID)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	detail, err := networkPolicyDetail(r.Context(), api.Database, p)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, detail)
}

// networkPolicyTemplates checks the complete proposed set before any mutation.
// The database wrapper also checks existing and historical attachments.
func (api *API) networkPolicyTemplates(ctx context.Context, tx database.Store, r *http.Request, ids []uuid.UUID) error {
	org := httpmw.OrganizationParam(r).ID
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return networkPolicyBadRequest("Template IDs must be unique.")
		}
		seen[id] = true
		t, err := tx.GetTemplateByID(ctx, id)
		if err != nil {
			return err
		}
		if t.OrganizationID != org || t.Deleted || !api.Authorize(r, policy.ActionUpdate, t.RBACObject()) {
			return sql.ErrNoRows
		}
	}
	return nil
}

func (api *API) networkPolicyModulePath() string {
	if api.NetworkPolicyModulePath != "" {
		return api.NetworkPolicyModulePath
	}
	if api.DeploymentValues != nil {
		return api.DeploymentValues.NetworkPolicyModulePath.String()
	}
	return ""
}

func (api *API) networkPolicyArchive(hcl string) ([]byte, error) {
	if len(hcl) > 1<<20 {
		return nil, networkPolicyBadRequest("Policy HCL exceeds 1 MiB.")
	}
	if err := networkpolicy.ValidateHCL(hcl); err != nil {
		return nil, networkPolicyBadRequest(err.Error())
	}
	modulePath := api.networkPolicyModulePath()
	if modulePath == "" {
		return nil, &networkPolicyHTTPError{http.StatusServiceUnavailable, "The trusted network policy module is not configured."}
	}
	archive, err := networkpolicy.Archive(hcl, os.DirFS(modulePath))
	if err != nil {
		return nil, xerrors.Errorf("package trusted policy module: %w", err)
	}
	return archive, nil
}

func insertNetworkPolicyRevision(ctx context.Context, tx database.Store, p database.NetworkPolicy, user uuid.UUID, hcl string, archive []byte) (database.NetworkPolicyRevision, error) {
	hash := sha256.Sum256(archive)
	file, err := tx.GetFileByHashAndCreator(ctx, database.GetFileByHashAndCreatorParams{Hash: hex.EncodeToString(hash[:]), CreatedBy: user})
	if errors.Is(err, sql.ErrNoRows) {
		file, err = tx.InsertFile(ctx, database.InsertFileParams{ID: uuid.New(), Hash: hex.EncodeToString(hash[:]), CreatedBy: user, CreatedAt: dbtime.Now(), Mimetype: tarMimeType, Data: archive})
	}
	if err != nil {
		return database.NetworkPolicyRevision{}, err
	}
	revisions, err := tx.GetNetworkPolicyRevisionsByPolicyID(ctx, p.ID)
	if err != nil {
		return database.NetworkPolicyRevision{}, err
	}
	var number int64 = 1
	for _, revision := range revisions {
		number = max(number, revision.RevisionNumber+1)
	}
	return tx.InsertNetworkPolicyRevision(ctx, database.InsertNetworkPolicyRevisionParams{ID: uuid.New(), PolicyID: p.ID, OrganizationID: p.OrganizationID, RevisionNumber: number, Hcl: hcl, FileID: file.ID, CreatedBy: user, CreatedAt: dbtime.Now()})
}

// @Summary Create network policy
// @ID create-network-policy
// @Security CoderSessionToken
// @Tags Network policies
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.CreateNetworkPolicyRequest true "Policy"
// @Success 201 {object} codersdk.NetworkPolicy
// @Router /api/experimental/organizations/{organization}/network-policies [post]
// @x-apidocgen {"skip": true}
func (api *API) postNetworkPolicy(rw http.ResponseWriter, r *http.Request) {
	var req codersdk.CreateNetworkPolicyRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	archive, err := api.networkPolicyArchive(req.HCL)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	ctx := r.Context()
	var result codersdk.NetworkPolicy
	err = api.Database.InTx(func(tx database.Store) error {
		if len(req.TemplateIDs) == 0 && !api.Authorize(r, policy.ActionCreate, rbac.ResourceTemplate.InOrg(httpmw.OrganizationParam(r).ID)) {
			return sql.ErrNoRows
		}
		if err := api.networkPolicyTemplates(ctx, tx, r, req.TemplateIDs); err != nil {
			return err
		}
		now := dbtime.Now()
		// The complete initial attachment set was authorized above. Elevation
		// allows template ACL admins to create without organization-wide rights.
		//nolint:gocritic // Authorized all proposed templates before creating the empty policy.
		createCtx := dbauthz.AsSystemRestricted(ctx)
		p, err := tx.InsertNetworkPolicy(createCtx, database.InsertNetworkPolicyParams{ID: uuid.New(), OrganizationID: httpmw.OrganizationParam(r).ID, Name: req.Name, Description: req.Description, CreatedBy: httpmw.APIKey(r).UserID, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		for _, id := range req.TemplateIDs {
			_, err = tx.InsertNetworkPolicyAttachment(createCtx, database.InsertNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: id, OrganizationID: p.OrganizationID, CreatedAt: now})
			if err != nil {
				return err
			}
		}
		_, err = insertNetworkPolicyRevision(ctx, tx, p, httpmw.APIKey(r).UserID, req.HCL, archive)
		if err != nil {
			return err
		}
		result, err = networkPolicyDetail(ctx, tx, p)
		return err
	}, nil)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, result)
}

func networkPolicyCheckGeneration(p database.NetworkPolicy, expected int64) error {
	if p.DeletionRequestedAt.Valid || p.DeletedAt.Valid {
		return networkPolicyConflict("Network policy deletion has been requested; mutations are blocked.")
	}
	if p.Generation != expected {
		return networkPolicyConflict("Network policy has changed. Reload before retrying.")
	}
	return nil
}

// @Summary Update network policy
// @ID update-network-policy
// @Security CoderSessionToken
// @Tags Network policies
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param networkpolicy path string true "Network policy ID" format(uuid)
// @Param request body codersdk.UpdateNetworkPolicyRequest true "Metadata"
// @Success 200 {object} codersdk.NetworkPolicy
// @Router /api/experimental/organizations/{organization}/network-policies/{networkpolicy} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchNetworkPolicy(rw http.ResponseWriter, r *http.Request) {
	var req codersdk.UpdateNetworkPolicyRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	ctx := r.Context()
	var result codersdk.NetworkPolicy
	err := api.Database.InTx(func(tx database.Store) error {
		p, err := networkPolicyByRequest(ctx, r, tx.GetNetworkPolicyByIDForUpdate)
		if err != nil {
			return err
		}
		if err = networkPolicyCheckGeneration(p, req.ExpectedGeneration); err != nil {
			return err
		}
		p, err = tx.UpdateNetworkPolicy(ctx, database.UpdateNetworkPolicyParams{ID: p.ID, Name: req.Name, Description: req.Description, UpdatedAt: dbtime.Now()})
		if err != nil {
			return err
		}
		result, err = networkPolicyDetail(ctx, tx, p)
		return err
	}, nil)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, result)
}

// @Summary Save network policy revision
// @ID create-network-policy-revision
// @Security CoderSessionToken
// @Tags Network policies
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param networkpolicy path string true "Network policy ID" format(uuid)
// @Param request body codersdk.CreateNetworkPolicyRevisionRequest true "Revision"
// @Success 201 {object} codersdk.NetworkPolicyRevision
// @Router /api/experimental/organizations/{organization}/network-policies/{networkpolicy}/revisions [post]
// @x-apidocgen {"skip": true}
func (api *API) postNetworkPolicyRevision(rw http.ResponseWriter, r *http.Request) {
	var req codersdk.CreateNetworkPolicyRevisionRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	archive, err := api.networkPolicyArchive(req.HCL)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	ctx := r.Context()
	var revision database.NetworkPolicyRevision
	err = api.Database.InTx(func(tx database.Store) error {
		p, err := networkPolicyByRequest(ctx, r, tx.GetNetworkPolicyByIDForUpdate)
		if err != nil {
			return err
		}
		if err = networkPolicyCheckGeneration(p, req.ExpectedGeneration); err != nil {
			return err
		}
		revision, err = insertNetworkPolicyRevision(ctx, tx, p, httpmw.APIKey(r).UserID, req.HCL, archive)
		if err != nil {
			return err
		}
		_, err = tx.UpdateNetworkPolicy(ctx, database.UpdateNetworkPolicyParams{ID: p.ID, Name: p.Name, Description: p.Description, UpdatedAt: dbtime.Now()})
		return err
	}, nil)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, convertNetworkPolicyRevision(revision))
}

// @Summary Replace network policy template attachments
// @ID set-network-policy-templates
// @Security CoderSessionToken
// @Tags Network policies
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param networkpolicy path string true "Network policy ID" format(uuid)
// @Param request body codersdk.SetNetworkPolicyTemplatesRequest true "Attachments"
// @Success 200 {object} codersdk.NetworkPolicy
// @Router /api/experimental/organizations/{organization}/network-policies/{networkpolicy}/templates [put]
// @x-apidocgen {"skip": true}
func (api *API) putNetworkPolicyTemplates(rw http.ResponseWriter, r *http.Request) {
	var req codersdk.SetNetworkPolicyTemplatesRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	ctx := r.Context()
	var result codersdk.NetworkPolicy
	var jobs []database.ProvisionerJob
	err := api.Database.InTx(func(tx database.Store) error {
		p, err := networkPolicyByRequest(ctx, r, tx.GetNetworkPolicyByIDForUpdate)
		if err != nil {
			return err
		}
		if err = networkPolicyCheckGeneration(p, req.ExpectedGeneration); err != nil {
			return err
		}
		if err = api.networkPolicyTemplates(ctx, tx, r, req.TemplateIDs); err != nil {
			return err
		}
		old, err := tx.GetNetworkPolicyAttachmentsByPolicyID(ctx, p.ID)
		if err != nil {
			return err
		}
		targets, err := tx.GetNetworkPolicyTargetsByPolicyID(ctx, p.ID)
		if err != nil {
			return err
		}
		ids := make([]uuid.UUID, 0, len(targets))
		for _, target := range targets {
			ids = append(ids, target.ID)
		}
		targets, err = lockNetworkPolicyTargets(ctx, tx, p, ids)
		if err != nil {
			return err
		}
		var revision database.NetworkPolicyRevision
		if p.PublishedRevisionID.Valid {
			revision, err = tx.GetNetworkPolicyRevisionByID(ctx, p.PublishedRevisionID.UUID)
			if err != nil {
				return err
			}
		}
		// Preserve old-set authorization even after the last attachment is
		// removed. New-set authorization and every target lock are held above.
		//nolint:gocritic // The complete old and new template sets are authorized.
		systemCtx := dbauthz.AsSystemRestricted(ctx)
		for _, attachment := range old {
			if slices.Contains(req.TemplateIDs, attachment.TemplateID) {
				continue
			}
			count, err := tx.DeleteNetworkPolicyAttachment(systemCtx, database.DeleteNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: attachment.TemplateID})
			if err != nil {
				return err
			}
			if count == 0 {
				return networkPolicyConflict("Network policy attachments cannot change while a target operation is active.")
			}
		}
		for _, id := range req.TemplateIDs {
			if slices.ContainsFunc(old, func(a database.NetworkPolicyAttachment) bool { return a.TemplateID == id }) {
				continue
			}
			_, err = tx.InsertNetworkPolicyAttachment(systemCtx, database.InsertNetworkPolicyAttachmentParams{
				PolicyID:       p.ID,
				TemplateID:     id,
				OrganizationID: p.OrganizationID,
				CreatedAt:      dbtime.Now(),
			})
			if err != nil {
				return err
			}
		}
		if p.PublishedRevisionID.Valid {
			jobs, _, err = queueNetworkPolicyJobs(systemCtx, tx, p, revision, targets, httpmw.APIKey(r).UserID, codersdk.NetworkPolicyOperationApply)
			if err != nil {
				return err
			}
		}
		p, err = tx.UpdateNetworkPolicy(systemCtx, database.UpdateNetworkPolicyParams{ID: p.ID, Name: p.Name, Description: p.Description, UpdatedAt: dbtime.Now()})
		if err != nil {
			return err
		}
		result, err = networkPolicyDetail(systemCtx, tx, p)
		return err
	}, nil)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	api.notifyNetworkPolicyJobs(ctx, jobs)
	httpapi.Write(ctx, rw, http.StatusOK, result)
}

// @Summary Create network policy target
// @ID create-network-policy-target
// @Security CoderSessionToken
// @Tags Network policies
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param networkpolicy path string true "Network policy ID" format(uuid)
// @Param request body codersdk.CreateNetworkPolicyTargetRequest true "Target"
// @Success 201 {object} codersdk.NetworkPolicyTarget
// @Router /api/experimental/organizations/{organization}/network-policies/{networkpolicy}/targets [post]
// @x-apidocgen {"skip": true}
func (api *API) postNetworkPolicyTarget(rw http.ResponseWriter, r *http.Request) {
	var req codersdk.CreateNetworkPolicyTargetRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	if strings.TrimSpace(req.Cluster) == "" || strings.TrimSpace(req.Context) == "" || strings.TrimSpace(req.Namespace) == "" {
		api.networkPolicyError(rw, r, networkPolicyBadRequest("Cluster, context, and namespace must be explicit."))
		return
	}
	ctx := r.Context()
	var target database.NetworkPolicyTarget
	err := api.Database.InTx(func(tx database.Store) error {
		p, err := networkPolicyByRequest(ctx, r, tx.GetNetworkPolicyByIDForUpdate)
		if err != nil {
			return err
		}
		if err = networkPolicyCheckGeneration(p, req.ExpectedGeneration); err != nil {
			return err
		}
		provisionerTags := provisionersdk.MutateTags(httpmw.APIKey(r).UserID, req.ProvisionerTags)
		// This capability is server-owned and cannot be removed by API input.
		provisionerTags[provisionersdk.TagNetworkPolicy] = "1"
		tags, err := json.Marshal(provisionerTags)
		if err != nil {
			return err
		}
		// Target writes are system-only after full composite policy authorization.
		//nolint:gocritic // The policy lock above authorizes every attachment.
		target, err = tx.InsertNetworkPolicyTarget(dbauthz.AsSystemRestricted(ctx), database.InsertNetworkPolicyTargetParams{ID: uuid.New(), PolicyID: p.ID, OrganizationID: p.OrganizationID, Cluster: req.Cluster, Context: req.Context, Namespace: req.Namespace, Tags: tags, CreatedAt: dbtime.Now(), UpdatedAt: dbtime.Now()})
		if err != nil {
			return err
		}
		_, err = tx.UpdateNetworkPolicy(ctx, database.UpdateNetworkPolicyParams{ID: p.ID, Name: p.Name, Description: p.Description, UpdatedAt: dbtime.Now()})
		return err
	}, nil)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, convertNetworkPolicyTarget(target))
}

// @Summary Publish or plan network policy
// @ID publish-network-policy
// @Security CoderSessionToken
// @Tags Network policies
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param networkpolicy path string true "Network policy ID" format(uuid)
// @Param request body codersdk.PublishNetworkPolicyRequest true "Publication"
// @Success 202 {array} codersdk.NetworkPolicyRun
// @Router /api/experimental/organizations/{organization}/network-policies/{networkpolicy}/publications [post]
// @x-apidocgen {"skip": true}
func (api *API) postNetworkPolicyPublication(rw http.ResponseWriter, r *http.Request) {
	var req codersdk.PublishNetworkPolicyRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	if req.Operation != codersdk.NetworkPolicyOperationApply && req.Operation != codersdk.NetworkPolicyOperationPlan {
		api.networkPolicyError(rw, r, networkPolicyBadRequest("Operation must be apply or plan."))
		return
	}
	if req.RevisionID == uuid.Nil || len(req.TargetIDs) == 0 {
		api.networkPolicyError(rw, r, networkPolicyBadRequest("A revision and explicit targets are required."))
		return
	}
	ids := slices.Clone(req.TargetIDs)
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	if len(slices.Compact(ids)) != len(req.TargetIDs) {
		api.networkPolicyError(rw, r, networkPolicyBadRequest("Target IDs must be unique."))
		return
	}
	ctx := r.Context()
	jobs := make([]database.ProvisionerJob, 0, len(ids))
	result := make([]codersdk.NetworkPolicyRun, 0, len(ids))
	err := api.Database.InTx(func(tx database.Store) error {
		p, err := networkPolicyByRequest(ctx, r, tx.GetNetworkPolicyByIDForUpdate)
		if err != nil {
			return err
		}
		if err = networkPolicyCheckGeneration(p, req.ExpectedGeneration); err != nil {
			return err
		}
		revision, err := tx.GetNetworkPolicyRevisionByID(ctx, req.RevisionID)
		if err != nil {
			return err
		}
		if revision.PolicyID != p.ID || revision.OrganizationID != p.OrganizationID {
			return sql.ErrNoRows
		}
		targets, err := lockNetworkPolicyTargets(ctx, tx, p, ids)
		if err != nil {
			return err
		}
		jobs, result, err = queueNetworkPolicyJobs(ctx, tx, p, revision, targets, httpmw.APIKey(r).UserID, req.Operation)
		if err != nil {
			return err
		}
		if req.Operation == codersdk.NetworkPolicyOperationApply {
			_, err = tx.PublishNetworkPolicyRevision(ctx, database.PublishNetworkPolicyRevisionParams{ID: p.ID, PublishedRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, UpdatedAt: dbtime.Now()})
		} else {
			_, err = tx.UpdateNetworkPolicy(ctx, database.UpdateNetworkPolicyParams{ID: p.ID, Name: p.Name, Description: p.Description, UpdatedAt: dbtime.Now()})
		}
		return err
	}, nil)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	api.notifyNetworkPolicyJobs(ctx, jobs)
	httpapi.Write(ctx, rw, http.StatusAccepted, result)
}

func (api *API) notifyNetworkPolicyJobs(ctx context.Context, jobs []database.ProvisionerJob) {
	for _, job := range jobs {
		if err := provisionerjobs.PostJob(api.Pubsub, job); err != nil {
			api.Logger.Error(ctx, "notify network policy provisioner job", slog.Error(err))
		}
	}
}

// lockNetworkPolicyTargets follows the policy lock, in stable UUID order.
func lockNetworkPolicyTargets(ctx context.Context, tx database.Store, p database.NetworkPolicy, ids []uuid.UUID) ([]database.NetworkPolicyTarget, error) {
	ids = slices.Clone(ids)
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	targets := make([]database.NetworkPolicyTarget, 0, len(ids))
	for _, id := range ids {
		target, err := tx.GetNetworkPolicyTargetByIDForUpdate(ctx, id)
		if err != nil {
			return nil, err
		}
		if target.PolicyID != p.ID || target.OrganizationID != p.OrganizationID {
			return nil, sql.ErrNoRows
		}
		if target.ActiveJobID.Valid {
			return nil, networkPolicyConflict("A target already has an active or uncertain operation.")
		}
		targets = append(targets, target)
	}
	return targets, nil
}

// queueNetworkPolicyJobs requires policy and target locks and complete template
// authorization. Jobs snapshot the attachment set inside the same transaction.
func queueNetworkPolicyJobs(ctx context.Context, tx database.Store, p database.NetworkPolicy, revision database.NetworkPolicyRevision, targets []database.NetworkPolicyTarget, userID uuid.UUID, operation codersdk.NetworkPolicyOperation) ([]database.ProvisionerJob, []codersdk.NetworkPolicyRun, error) {
	//nolint:gocritic // Callers authorize all policy attachments and hold the policy lock.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	jobs := make([]database.ProvisionerJob, 0, len(targets))
	runs := make([]codersdk.NetworkPolicyRun, 0, len(targets))
	traceMetadata, err := json.Marshal(tracing.MetadataFromContext(ctx))
	if err != nil {
		return nil, nil, xerrors.Errorf("marshal job trace metadata: %w", err)
	}
	for _, target := range targets {
		if operation == codersdk.NetworkPolicyOperationDestroy {
			source, err := networkpolicy.DestroyProvenance(systemCtx, tx, target)
			if err != nil {
				return nil, nil, err
			}
			if source == nil || source.RevisionID != revision.ID {
				return nil, nil, xerrors.New("destroy revision does not match retained state")
			}
			target.StateRunID = uuid.NullUUID{UUID: source.ID, Valid: true}
		} else if p.DeletionRequestedAt.Valid || p.DeletedAt.Valid {
			return nil, nil, networkPolicyConflict("Network policy deletion has been requested.")
		}
		runID, jobID := uuid.New(), uuid.New()
		input, err := json.Marshal(provisionerdserver.NetworkPolicyJob{NetworkPolicyRunID: runID})
		if err != nil {
			return nil, nil, err
		}
		var tags map[string]string
		if err := json.Unmarshal(target.Tags, &tags); err != nil {
			return nil, nil, xerrors.Errorf("decode target tags: %w", err)
		}
		if tags == nil {
			tags = make(map[string]string)
		}
		// Reassert the capability for targets saved before this server version.
		tags[provisionersdk.TagNetworkPolicy] = "1"
		// Destroy requires a newer worker, without changing plan/apply routing.
		delete(tags, provisionersdk.TagNetworkPolicyDestroy)
		if operation == codersdk.NetworkPolicyOperationDestroy {
			tags[provisionersdk.TagNetworkPolicyDestroy] = "1"
		}
		now := dbtime.Now()
		job, err := tx.InsertProvisionerJob(systemCtx, database.InsertProvisionerJobParams{
			ID:             jobID,
			OrganizationID: p.OrganizationID,
			CreatedAt:      now,
			UpdatedAt:      now,
			InitiatorID:    userID,
			Provisioner:    database.ProvisionerTypeTerraform,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         revision.FileID,
			Type:           database.ProvisionerJobTypeNetworkPolicy,
			Input:          input,
			Tags:           tags,
			TraceMetadata:  pqtype.NullRawMessage{Valid: true, RawMessage: traceMetadata},
			LogsOverflowed: false,
		})
		if err != nil {
			return nil, nil, err
		}
		target, err = tx.StartNetworkPolicyTargetJob(systemCtx, database.StartNetworkPolicyTargetJobParams{
			ID:                target.ID,
			JobID:             uuid.NullUUID{UUID: jobID, Valid: true},
			DesiredRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true},
			Generation:        target.Generation,
			StateRunID:        target.StateRunID,
			Status:            "pending",
			UpdatedAt:         now,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, networkPolicyConflict("A target already has an active operation or changed.")
		}
		if err != nil {
			return nil, nil, err
		}
		run, err := tx.InsertNetworkPolicyRun(systemCtx, database.InsertNetworkPolicyRunParams{
			ID:             runID,
			JobID:          jobID,
			PolicyID:       p.ID,
			TargetID:       target.ID,
			RevisionID:     revision.ID,
			OrganizationID: p.OrganizationID,
			Generation:     target.Generation,
			Operation:      string(operation),
			CreatedAt:      now,
		})
		if err != nil {
			return nil, nil, err
		}
		jobs = append(jobs, job)
		runs = append(runs, convertNetworkPolicyRun(run, job))
	}
	return jobs, runs, nil
}

// @Summary Get network policy run logs
// @ID get-network-policy-run-logs
// @Security CoderSessionToken
// @Tags Network policies
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param networkpolicy path string true "Network policy ID" format(uuid)
// @Param run path string true "Run ID" format(uuid)
// @Param after query integer false "Return logs after this ID"
// @Success 200 {array} codersdk.ProvisionerJobLog
// @Router /api/experimental/organizations/{organization}/network-policies/{networkpolicy}/runs/{run}/logs [get]
// @x-apidocgen {"skip": true}
func (api *API) networkPolicyRunLogs(rw http.ResponseWriter, r *http.Request) {
	p, err := networkPolicyByRequest(r.Context(), r, api.Database.GetNetworkPolicyByID)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "run"))
	if err != nil {
		api.networkPolicyError(rw, r, sql.ErrNoRows)
		return
	}
	run, err := api.Database.GetNetworkPolicyRunByID(r.Context(), id)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	if run.PolicyID != p.ID || run.OrganizationID != p.OrganizationID {
		api.networkPolicyError(rw, r, sql.ErrNoRows)
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), run.JobID)
	if err != nil {
		api.networkPolicyError(rw, r, err)
		return
	}
	api.provisionerJobLogs(rw, r, job)
}

func networkPolicyDetail(ctx context.Context, tx database.Store, p database.NetworkPolicy) (codersdk.NetworkPolicy, error) {
	result := codersdk.NetworkPolicy{ID: p.ID, OrganizationID: p.OrganizationID, Name: p.Name, Description: p.Description, Generation: p.Generation, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Revisions: []codersdk.NetworkPolicyRevision{}, TemplateIDs: []uuid.UUID{}, Targets: []codersdk.NetworkPolicyTarget{}, Runs: []codersdk.NetworkPolicyRun{}}
	if p.DeletionRequestedAt.Valid {
		result.DeletionRequestedAt = &p.DeletionRequestedAt.Time
	}
	if p.DeletedAt.Valid {
		result.DeletedAt = &p.DeletedAt.Time
	}
	revisions, err := tx.GetNetworkPolicyRevisionsByPolicyID(ctx, p.ID)
	if err != nil {
		return result, err
	}
	for _, revision := range revisions {
		result.Revisions = append(result.Revisions, convertNetworkPolicyRevision(revision))
	}
	attachments, err := tx.GetNetworkPolicyAttachmentsByPolicyID(ctx, p.ID)
	if err != nil {
		return result, err
	}
	for _, attachment := range attachments {
		result.TemplateIDs = append(result.TemplateIDs, attachment.TemplateID)
	}
	targets, err := tx.GetNetworkPolicyTargetsByPolicyID(ctx, p.ID)
	if err != nil {
		return result, err
	}
	targetIndex := make(map[uuid.UUID]int, len(targets))
	for i, target := range targets {
		targetIndex[target.ID] = i
		result.Targets = append(result.Targets, convertNetworkPolicyTarget(target))
	}
	runs, err := tx.GetNetworkPolicyRunsByPolicyID(ctx, p.ID)
	if err != nil {
		return result, err
	}
	for _, run := range runs {
		job, err := tx.GetProvisionerJobByID(ctx, run.JobID)
		if err != nil {
			return result, err
		}
		if index, ok := targetIndex[run.TargetID]; ok && job.ErrorCode.String == "NETWORK_POLICY_STATE_UNKNOWN" {
			target := targets[index]
			if target.ActiveJobID.Valid && target.ActiveJobID.UUID == job.ID && target.LastJobID.Valid && target.LastJobID.UUID == job.ID {
				result.Targets[index].Status = "unknown"
				result.Targets[index].LastError = job.Error.String
				result.Targets[index].Drifted = true
			}
		}
		converted := convertNetworkPolicyRun(run, job)
		if run.Operation == string(codersdk.NetworkPolicyOperationPlan) && job.CompletedAt.Valid {
			converted.Plan, err = networkPolicyPlanReport(ctx, tx, job.ID)
			if err != nil {
				return result, err
			}
		}
		result.Runs = append(result.Runs, converted)
	}
	return result, nil
}

// networkPolicyPlanReport provides a bounded preview of persisted plan output.
// Full output remains available through the authenticated run logs endpoint.
func networkPolicyPlanReport(ctx context.Context, tx database.Store, jobID uuid.UUID) (string, error) {
	logs, err := tx.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{JobID: jobID})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	const maxReportBytes = 64 << 10
	var report strings.Builder
	for _, log := range logs {
		remaining := maxReportBytes - report.Len()
		if len(log.Output)+1 > remaining {
			_, _ = report.WriteString(log.Output[:min(len(log.Output), remaining)])
			_, _ = report.WriteString("\nReport truncated. Fetch run logs for complete output.\n")
			break
		}
		_, _ = report.WriteString(log.Output)
		_ = report.WriteByte('\n')
	}
	return report.String(), nil
}

func convertNetworkPolicyRevision(r database.NetworkPolicyRevision) codersdk.NetworkPolicyRevision {
	return codersdk.NetworkPolicyRevision{ID: r.ID, PolicyID: r.PolicyID, HCL: r.Hcl, CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy}
}
func networkPolicyUUID(id uuid.NullUUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	return &id.UUID
}
func convertNetworkPolicyTarget(t database.NetworkPolicyTarget) codersdk.NetworkPolicyTarget {
	var tags map[string]string
	_ = json.Unmarshal(t.Tags, &tags)
	result := codersdk.NetworkPolicyTarget{ID: t.ID, PolicyID: t.PolicyID, Name: t.Cluster + "/" + t.Context + "/" + t.Namespace, Cluster: t.Cluster, Context: t.Context, Namespace: t.Namespace, ProvisionerTags: tags, DesiredRevisionID: networkPolicyUUID(t.DesiredRevisionID), AppliedRevisionID: networkPolicyUUID(t.AppliedRevisionID), Status: t.Status, Drifted: t.Drifted, LastJobID: networkPolicyUUID(t.LastJobID), LastError: t.LastError, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
	if t.DestroyedAt.Valid {
		result.DestroyedAt = &t.DestroyedAt.Time
	}
	return result
}
func convertNetworkPolicyRun(run database.NetworkPolicyRun, job database.ProvisionerJob) codersdk.NetworkPolicyRun {
	result := codersdk.NetworkPolicyRun{TemplateIDs: run.TemplateIDs, ID: run.ID, PolicyID: run.PolicyID, TargetID: run.TargetID, RevisionID: run.RevisionID, JobID: run.JobID, Operation: codersdk.NetworkPolicyOperation(run.Operation), Status: string(job.JobStatus), Error: job.Error.String, CreatedAt: run.CreatedAt}
	if job.CompletedAt.Valid {
		result.CompletedAt = &job.CompletedAt.Time
	}
	return result
}
