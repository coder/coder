package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

func (api *API) organization(rw http.ResponseWriter, r *http.Request) {
	organization := httpmw.OrganizationParam(r)

	if !api.Authorize(r, rbac.ActionRead, rbac.ResourceOrganization.
		InOrg(organization.ID)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	httpapi.Write(rw, http.StatusOK, convertOrganization(organization))
}

func (api *API) postOrganizations(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	// Create organization uses the organization resource without an OrgID.
	// This means you need the site wide permission to make a new organization.
	if !api.Authorize(r, rbac.ActionCreate, rbac.ResourceOrganization) {
		httpapi.Forbidden(rw)
		return
	}

	var req codersdk.CreateOrganizationRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	_, err := api.Database.GetOrganizationByName(r.Context(), req.Name)
	if err == nil {
		httpapi.Write(rw, http.StatusConflict, codersdk.Response{
			Message: "Organization already exists with that name.",
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("Internal error fetching organization %q.", req.Name),
			Detail:  err.Error(),
		})
		return
	}

	var organization database.Organization
	err = api.Database.InTx(func(store database.Store) error {
		organization, err = store.InsertOrganization(r.Context(), database.InsertOrganizationParams{
			ID:        uuid.New(),
			Name:      req.Name,
			CreatedAt: database.Now(),
			UpdatedAt: database.Now(),
		})
		if err != nil {
			return xerrors.Errorf("create organization: %w", err)
		}
		_, err = store.InsertOrganizationMember(r.Context(), database.InsertOrganizationMemberParams{
			OrganizationID: organization.ID,
			UserID:         apiKey.UserID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			Roles: []string{
				rbac.RoleOrgAdmin(organization.ID),
			},
		})
		if err != nil {
			return xerrors.Errorf("create organization admin: %w", err)
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error inserting organization member.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(rw, http.StatusCreated, convertOrganization(organization))
}

// convertOrganization consumes the database representation and outputs an API friendly representation.
func convertOrganization(organization database.Organization) codersdk.Organization {
	return codersdk.Organization{
		ID:        organization.ID,
		Name:      organization.Name,
		CreatedAt: organization.CreatedAt,
		UpdatedAt: organization.UpdatedAt,
	}
}
