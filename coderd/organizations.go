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

	if !api.Authorize(rw, r, rbac.ActionRead, rbac.ResourceOrganization.
		InOrg(organization.ID).
		WithID(organization.ID.String())) {
		return
	}

	httpapi.Write(rw, http.StatusOK, convertOrganization(organization))
}

func (api *API) postOrganizations(rw http.ResponseWriter, r *http.Request) {
	apiKey := httpmw.APIKey(r)
	// Create organization uses the organization resource without an OrgID.
	// This means you need the site wide permission to make a new organization.
	if !api.Authorize(rw, r, rbac.ActionCreate,
		rbac.ResourceOrganization) {
		return
	}

	var req codersdk.CreateOrganizationRequest
	if !httpapi.Read(rw, r, &req) {
		return
	}

	_, err := api.Database.GetOrganizationByName(r.Context(), req.Name)
	if err == nil {
		httpapi.Write(rw, http.StatusConflict, httpapi.Response{
			Message: "organization already exists with that name",
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization: %s", err.Error()),
		})
		return
	}

	var organization database.Organization
	err = api.Database.InTx(func(db database.Store) error {
		organization, err = api.Database.InsertOrganization(r.Context(), database.InsertOrganizationParams{
			ID:        uuid.New(),
			Name:      req.Name,
			CreatedAt: database.Now(),
			UpdatedAt: database.Now(),
		})
		if err != nil {
			return xerrors.Errorf("create organization: %w", err)
		}
		_, err = api.Database.InsertOrganizationMember(r.Context(), database.InsertOrganizationMemberParams{
			OrganizationID: organization.ID,
			UserID:         apiKey.UserID,
			CreatedAt:      database.Now(),
			UpdatedAt:      database.Now(),
			Roles: []string{
				// Also assign member role incase they get demoted from admin
				rbac.RoleOrgMember(organization.ID),
				rbac.RoleOrgAdmin(organization.ID),
			},
		})
		if err != nil {
			return xerrors.Errorf("create organization member: %w", err)
		}
		return nil
	})
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: err.Error(),
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
