package coderd

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Update organization
// @ID update-organization
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Param request body codersdk.UpdateOrganizationRequest true "Patch organization request"
// @Success 200 {object} codersdk.Organization
// @Router /organizations/{organization} [patch]
func (api *API) patchOrganization(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		organization      = httpmw.OrganizationParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Organization](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: organization.ID,
		})
	)
	aReq.Old = organization
	defer commitAudit()

	var req codersdk.UpdateOrganizationRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// "default" is a reserved name that always refers to the default org (much like the way we
	// use "me" for users).
	if req.Name == codersdk.DefaultOrganization {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Organization name %q is reserved.", codersdk.DefaultOrganization),
		})
		return
	}

	err := database.ReadModifyUpdate(api.Database, func(tx database.Store) error {
		var err error
		organization, err = tx.GetOrganizationByID(ctx, organization.ID)
		if err != nil {
			return err
		}

		updateOrgParams := database.UpdateOrganizationParams{
			UpdatedAt:   dbtime.Now(),
			ID:          organization.ID,
			Name:        organization.Name,
			DisplayName: organization.DisplayName,
			Description: organization.Description,
			Icon:        organization.Icon,
		}

		if req.Name != "" {
			updateOrgParams.Name = req.Name
		}
		if req.DisplayName != "" {
			updateOrgParams.DisplayName = req.DisplayName
		}
		if req.Description != nil {
			updateOrgParams.Description = *req.Description
		}
		if req.Icon != nil {
			updateOrgParams.Icon = *req.Icon
		}

		organization, err = tx.UpdateOrganization(ctx, updateOrgParams)
		if err != nil {
			return err
		}
		return nil
	})

	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if database.IsUniqueViolation(err) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Organization already exists with the name %q.", req.Name),
			Validations: []codersdk.ValidationError{{
				Field:  "name",
				Detail: "This value is already in use and should be unique.",
			}},
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating organization.",
			Detail:  fmt.Sprintf("update organization: %s", err.Error()),
		})
		return
	}

	aReq.New = organization
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.Organization(organization))
}

// @Summary Delete organization
// @ID delete-organization
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Success 200 {object} codersdk.Response
// @Router /organizations/{organization} [delete]
func (api *API) deleteOrganization(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		organization      = httpmw.OrganizationParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Organization](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionDelete,
			OrganizationID: organization.ID,
		})
	)
	aReq.Old = organization
	defer commitAudit()

	if organization.IsDefault {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Default organization cannot be deleted.",
		})
		return
	}

	err := api.Database.InTx(func(tx database.Store) error {
		err := tx.UpdateOrganizationDeletedByID(ctx, database.UpdateOrganizationDeletedByIDParams{
			ID:        organization.ID,
			UpdatedAt: dbtime.Now(),
		})
		if err != nil {
			return xerrors.Errorf("delete organization: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting organization.",
			Detail:  fmt.Sprintf("delete organization: %s", err.Error()),
		})
		return
	}

	aReq.New = database.Organization{}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Organization has been deleted.",
	})
}

// @Summary Create organization
// @ID create-organization
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Organizations
// @Param request body codersdk.CreateOrganizationRequest true "Create organization request"
// @Success 201 {object} codersdk.Organization
// @Router /organizations [post]
func (api *API) postOrganizations(rw http.ResponseWriter, r *http.Request) {
	var (
		// organizationID is required before the audit log entry is created.
		organizationID    = uuid.New()
		ctx               = r.Context()
		apiKey            = httpmw.APIKey(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Organization](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionCreate,
			OrganizationID: organizationID,
		})
	)
	aReq.Old = database.Organization{}
	defer commitAudit()

	var req codersdk.CreateOrganizationRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Name == codersdk.DefaultOrganization {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Organization name %q is reserved.", codersdk.DefaultOrganization),
		})
		return
	}

	_, err := api.Database.GetOrganizationByName(ctx, database.GetOrganizationByNameParams{
		Name:    req.Name,
		Deleted: false,
	})
	if err == nil {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Organization already exists with that name.",
		})
		return
	}
	if !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("Internal error fetching organization %q.", req.Name),
			Detail:  err.Error(),
		})
		return
	}

	var organization database.Organization
	err = api.Database.InTx(func(tx database.Store) error {
		if req.DisplayName == "" {
			req.DisplayName = req.Name
		}

		organization, err = tx.InsertOrganization(ctx, database.InsertOrganizationParams{
			ID:          organizationID,
			Name:        req.Name,
			DisplayName: req.DisplayName,
			Description: req.Description,
			Icon:        req.Icon,
			CreatedAt:   dbtime.Now(),
			UpdatedAt:   dbtime.Now(),
		})
		if err != nil {
			return xerrors.Errorf("create organization: %w", err)
		}
		_, err = tx.InsertOrganizationMember(ctx, database.InsertOrganizationMemberParams{
			OrganizationID: organization.ID,
			UserID:         apiKey.UserID,
			CreatedAt:      dbtime.Now(),
			UpdatedAt:      dbtime.Now(),
			Roles:          []string{
				// TODO: When organizations are allowed to be created, we should
				// come back to determining the default role of the person who
				// creates the org. Until that happens, all users in an organization
				// should be just regular members.
			},
		})
		if err != nil {
			return xerrors.Errorf("create organization admin: %w", err)
		}

		_, err = tx.InsertAllUsersGroup(ctx, organization.ID)
		if err != nil {
			return xerrors.Errorf("create %q group: %w", database.EveryoneGroup, err)
		}
		return nil
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error inserting organization member.",
			Detail:  err.Error(),
		})
		return
	}

	aReq.New = organization
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.Organization(organization))
}
