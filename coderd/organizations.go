package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get organization by ID
// @ID get-organization-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.Organization
// @Router /organizations/{organization} [get]
func (*API) organization(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

	httpapi.Write(ctx, rw, http.StatusOK, convertOrganization(organization))
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
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

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

	_, err := api.Database.GetOrganizationByName(ctx, req.Name)
	if err == nil {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Organization already exists with that name.",
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
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
			ID:          uuid.New(),
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
			Roles: []string{
				// TODO: When organizations are allowed to be created, we should
				// come back to determining the default role of the person who
				// creates the org. Until that happens, all users in an organization
				// should be just regular members.
				rbac.RoleOrgMember(),
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

	httpapi.Write(ctx, rw, http.StatusCreated, convertOrganization(organization))
}

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
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

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

	httpapi.Write(ctx, rw, http.StatusOK, convertOrganization(organization))
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
	ctx := r.Context()
	organization := httpmw.OrganizationParam(r)

	if organization.IsDefault {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Default organization cannot be deleted.",
		})
		return
	}

	err := api.Database.DeleteOrganization(ctx, organization.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting organization.",
			Detail:  fmt.Sprintf("delete organization: %s", err.Error()),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Organization has been deleted.",
	})
}

// convertOrganization consumes the database representation and outputs an API friendly representation.
func convertOrganization(organization database.Organization) codersdk.Organization {
	return codersdk.Organization{
		ID:          organization.ID,
		Name:        organization.Name,
		DisplayName: organization.DisplayName,
		Description: organization.Description,
		Icon:        organization.Icon,
		CreatedAt:   organization.CreatedAt,
		UpdatedAt:   organization.UpdatedAt,
		IsDefault:   organization.IsDefault,
	}
}
