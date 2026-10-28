package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

func convertFoundation(f database.Foundation) codersdk.Foundation {
	tags := map[string]string{}
	_ = json.Unmarshal(f.ProvisionerTags, &tags)
	return codersdk.Foundation{ID: f.ID, OrganizationID: f.OrganizationID, Name: f.Name, PreventDestroyWithWorkspaces: f.PreventDestroyWithWorkspaces, ProvisionerTags: tags, TemplateID: templateResourceUUID(f.TemplateID), Status: "unconfigured", Revisions: []codersdk.TemplateResourceRevision{}, Runs: []codersdk.TemplateResourceRun{}, Outputs: map[string]interface{}{}, Resources: []codersdk.FoundationResource{}, ResourcesAvailable: true}
}

func (api *API) writeFoundation(rw http.ResponseWriter, r *http.Request, f database.Foundation, status int) {
	var result codersdk.Foundation
	err := api.Database.InTx(func(tx database.Store) error {
		var err error
		result, err = foundationSnapshot(r.Context(), tx, f)
		return err
	}, &database.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	httpapi.Write(r.Context(), rw, status, result)
}

// @Summary List Foundations
// @ID list-foundations
// @Security CoderSessionToken
// @Tags Foundations
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.Foundation
// @Router /api/v2/organizations/{organization}/foundations [get]
// @x-apidocgen {"skip": true}
func (api *API) foundations(rw http.ResponseWriter, r *http.Request) {
	result := []codersdk.Foundation{}
	err := api.Database.InTx(func(tx database.Store) error {
		rows, err := tx.GetFoundationsByOrganizationID(r.Context(), httpmw.OrganizationParam(r).ID)
		if err != nil {
			return err
		}
		for _, f := range rows {
			snapshot, err := foundationSnapshot(r.Context(), tx, f)
			if err != nil {
				return err
			}
			result = append(result, snapshot)
		}
		return nil
	}, &database.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, result)
}

// @Summary Create a Foundation
// @ID create-foundation
// @Security CoderSessionToken
// @Tags Foundations
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.CreateFoundationRequest true "Settings"
// @Success 201 {object} codersdk.Foundation
// @Router /api/v2/organizations/{organization}/foundations [post]
// @x-apidocgen {"skip": true}
func (api *API) postFoundation(rw http.ResponseWriter, r *http.Request) {
	orgID := httpmw.OrganizationParam(r).ID
	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceFoundation.InOrg(orgID)) {
		httpapi.ResourceNotFound(rw)
		return
	}
	ar, commit := audit.InitRequest[database.Foundation](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, OrganizationID: orgID, Action: database.AuditActionCreate})
	defer commit()
	var req codersdk.CreateFoundationRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	tags, err := foundationTags(req.ProvisionerTags)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	now := dbtime.Now()
	f, err := api.Database.InsertFoundation(r.Context(), database.InsertFoundationParams{ID: uuid.New(), OrganizationID: orgID, Name: req.Name, ProvisionerTags: tags, PreventDestroyWithWorkspaces: optionalFoundationBool(req.PreventDestroyWithWorkspaces), CreatedAt: now, UpdatedAt: now})
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	ar.New = f
	httpapi.Write(r.Context(), rw, http.StatusCreated, convertFoundation(f))
}

// foundationTags prevents user settings from weakening platform routing fences.
func foundationTags(tags map[string]string) (json.RawMessage, error) {
	if tags == nil {
		tags = map[string]string{}
	}
	for key := range tags {
		if key == provisionersdk.TagScope || key == provisionersdk.TagOwner || key == provisionersdk.TagTemplateResources || strings.HasPrefix(key, "coder.") {
			return nil, &templateResourceHTTPError{http.StatusBadRequest, "Reserved provisioner tags cannot be configured on a Foundation."}
		}
	}
	return json.Marshal(tags)
}

// @Summary Get a Foundation
// @ID get-foundation
// @Security CoderSessionToken
// @Tags Foundations
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param foundation path string true "Foundation ID" format(uuid)
// @Success 200 {object} codersdk.Foundation
// @Router /api/v2/organizations/{organization}/foundations/{foundation} [get]
// @x-apidocgen {"skip": true}
func (api *API) foundation(rw http.ResponseWriter, r *http.Request) {
	f, ok := api.foundationParam(rw, r, policy.ActionRead)
	if !ok {
		return
	}
	api.writeFoundation(rw, r, f, http.StatusOK)
}

// @Summary Update Foundation settings
// @ID update-foundation
// @Security CoderSessionToken
// @Tags Foundations
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param foundation path string true "Foundation ID" format(uuid)
// @Param request body codersdk.UpdateFoundationRequest true "Settings"
// @Success 200 {object} codersdk.Foundation
// @Router /api/v2/organizations/{organization}/foundations/{foundation} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchFoundation(rw http.ResponseWriter, r *http.Request) {
	f, ok := api.foundationParam(rw, r, policy.ActionUpdate)
	if !ok {
		return
	}
	ar, commit := audit.InitRequest[database.Foundation](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, OrganizationID: f.OrganizationID, Action: database.AuditActionWrite})
	defer commit()
	ar.Old = f
	var req codersdk.UpdateFoundationRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	tags, err := foundationTags(req.ProvisionerTags)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	f, err = api.Database.UpdateFoundation(r.Context(), database.UpdateFoundationParams{ID: f.ID, Name: req.Name, ProvisionerTags: tags, PreventDestroyWithWorkspaces: optionalFoundationBool(req.PreventDestroyWithWorkspaces), UpdatedAt: dbtime.Now()})
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	ar.New = f
	api.writeFoundation(rw, r, f, http.StatusOK)
}

func optionalFoundationBool(value *bool) sql.NullBool {
	if value == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *value, Valid: true}
}

func guardFoundationDeletion(ctx context.Context, tx database.Store, f database.Foundation) error {
	state, err := tx.GetTemplateResourceStateForUpdate(ctx, f.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && (state.ActiveRunID.Valid || state.AppliedRevisionID.Valid || (state.Status != "unconfigured" && state.Status != "destroyed")) {
		return templateResourceConflict("Destroy Foundation infrastructure before deleting it. Unknown outcomes require manual reconciliation.")
	}
	// Cleanup, not consumer existence, gates deletion. Historical build inputs
	// survive soft deletion so remaining workspaces can still stop or delete.
	return nil
}

// @Summary Delete a cleaned-up Foundation
// @ID delete-foundation
// @Security CoderSessionToken
// @Tags Foundations
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param foundation path string true "Foundation ID" format(uuid)
// @Success 200 {object} codersdk.Response
// @Router /api/v2/organizations/{organization}/foundations/{foundation} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteFoundation(rw http.ResponseWriter, r *http.Request) {
	f, ok := api.foundationParam(rw, r, policy.ActionDelete)
	if !ok {
		return
	}
	ar, commit := audit.InitRequest[database.Foundation](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, OrganizationID: f.OrganizationID, Action: database.AuditActionDelete})
	defer commit()
	ar.Old = f
	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.LockFoundation(r.Context(), f.ID); err != nil {
			return err
		}
		fresh, err := tx.GetFoundationByID(r.Context(), f.ID)
		if err != nil {
			return err
		}
		if fresh.Deleted {
			return sql.ErrNoRows
		}
		if err := guardFoundationDeletion(r.Context(), tx, fresh); err != nil {
			return err
		}
		ar.New, err = tx.DeleteFoundation(r.Context(), database.DeleteFoundationParams{ID: f.ID, UpdatedAt: dbtime.Now()})
		return err
	}, nil)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{Message: "Foundation deleted."})
}

// @Summary Get a template's Foundation
// @ID get-template-foundation
// @Security CoderSessionToken
// @Tags Foundations
// @Produce json
// @Param template path string true "Template ID" format(uuid)
// @Success 200 {object} codersdk.Foundation
// @Router /api/v2/templates/{template}/foundation [get]
// @x-apidocgen {"skip": true}
func (api *API) templateFoundation(rw http.ResponseWriter, r *http.Request) {
	f, err := api.Database.GetFoundationByTemplateID(r.Context(), uuid.NullUUID{UUID: httpmw.TemplateParam(r).ID, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(r.Context(), rw, http.StatusOK, nil)
		return
	}
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	api.writeFoundation(rw, r, f, http.StatusOK)
}

// @Summary Bind a Foundation to a template
// @ID set-template-foundation
// @Security CoderSessionToken
// @Tags Foundations
// @Accept json
// @Produce json
// @Param template path string true "Template ID" format(uuid)
// @Param request body codersdk.SetTemplateFoundationRequest true "Foundation"
// @Success 200 {object} codersdk.Foundation
// @Router /api/v2/templates/{template}/foundation [put]
// @x-apidocgen {"skip": true}
func (api *API) putTemplateFoundation(rw http.ResponseWriter, r *http.Request) {
	template := httpmw.TemplateParam(r)
	if !api.Authorize(r, policy.ActionUpdate, template.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	var req codersdk.SetTemplateFoundationRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	f, err := api.Database.GetFoundationByID(r.Context(), req.FoundationID)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	if f.Deleted || f.OrganizationID != template.OrganizationID || !api.Authorize(r, policy.ActionUse, f.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	ar, commit := audit.InitRequest[database.Foundation](rw, &audit.RequestParams{Audit: *api.Auditor.Load(), Log: api.Logger, Request: r, OrganizationID: f.OrganizationID, Action: database.AuditActionWrite})
	defer commit()
	ar.Old = f
	err = api.Database.InTx(func(tx database.Store) error {
		//nolint:gocritic // Template update and Foundation use permissions were checked above.
		systemCtx := dbauthz.AsSystemRestricted(r.Context())
		if err := tx.LockTemplateResourceTemplate(systemCtx, template.ID); err != nil {
			return err
		}
		freshTemplate, err := tx.GetTemplateByID(r.Context(), template.ID)
		if err != nil {
			return err
		}
		if freshTemplate.Deleted {
			return sql.ErrNoRows
		}
		bound, err := tx.GetFoundationByTemplateID(systemCtx, uuid.NullUUID{UUID: template.ID, Valid: true})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && bound.ID != f.ID {
			return templateResourceConflict("The template already has a Foundation. Remove it after cleanup before binding another.")
		}
		if err := tx.LockFoundation(r.Context(), f.ID); err != nil {
			return err
		}
		fresh, err := tx.GetFoundationByID(r.Context(), f.ID)
		if err != nil {
			return err
		}
		if fresh.Deleted {
			return sql.ErrNoRows
		}
		if fresh.TemplateID.Valid && fresh.TemplateID.UUID != template.ID {
			return templateResourceConflict("The Foundation is already bound to another template.")
		}
		state, err := tx.GetTemplateResourceStateForUpdate(r.Context(), f.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && (state.ActiveRunID.Valid || state.Status == "unknown") {
			return templateResourceConflict("Cannot change binding while a Foundation run is active or quarantined.")
		}
		f, err = tx.BindFoundation(r.Context(), database.BindFoundationParams{ID: f.ID, TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true}, UpdatedAt: dbtime.Now()})
		return err
	}, nil)
	if err != nil {
		api.templateResourceError(rw, r, err)
		return
	}
	ar.New = f
	api.writeFoundation(rw, r, f, http.StatusOK)
}
