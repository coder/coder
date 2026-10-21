package coderd

import (
	"bytes"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get organization skill settings
// @ID get-organization-skill-settings
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Success 200 {object} codersdk.OrganizationSkillSettings
// @Router /api/experimental/organizations/{organization}/skill-settings [get]
// @x-apidocgen {"skip": true}
func (api *API) getOrganizationSkillSettings(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	organization := httpmw.OrganizationParam(r)
	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.OrganizationSkillSettings{
		MembersCanDisableSkills: organization.MembersCanDisableSkills,
	})
}

// @Summary Update organization skill settings
// @ID update-organization-skill-settings
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Param request body codersdk.UpdateOrganizationSkillSettingsRequest true "Update organization skill settings request"
// @Success 200 {object} codersdk.OrganizationSkillSettings
// @Router /api/experimental/organizations/{organization}/skill-settings [put]
// @x-apidocgen {"skip": true}
func (api *API) putOrganizationSkillSettings(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		organization      = httpmw.OrganizationParam(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Organization](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionWrite,
			OrganizationID: organization.ID,
		})
	)
	defer commitAudit()
	aReq.Old = organization
	if !api.Authorize(r, policy.ActionUpdate, rbac.ResourceOrganizationSkill.InOrg(organization.ID)) {
		httpapi.Write(ctx, rw, http.StatusForbidden, organizationSkillForbidden("update organization skill settings"))
		return
	}

	var req codersdk.UpdateOrganizationSkillSettingsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	updated, err := api.Database.UpdateOrganizationMembersCanDisableSkills(ctx, database.UpdateOrganizationMembersCanDisableSkillsParams{
		ID:                      organization.ID,
		MembersCanDisableSkills: req.MembersCanDisableSkills,
		UpdatedAt:               dbtime.Now(),
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	aReq.New = updated

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.OrganizationSkillSettings{
		MembersCanDisableSkills: updated.MembersCanDisableSkills,
	})
}

// @Summary Get organization skill preferences for a member
// @ID get-organization-skill-preferences-for-a-member
// @Security CoderSessionToken
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.OrganizationSkillPreferences
// @Router /api/experimental/organizations/{organization}/members/{user}/skill-preferences [get]
// @x-apidocgen {"skip": true}
func (api *API) getOrganizationSkillPreferences(rw http.ResponseWriter, r *http.Request) { //nolint:revive // Method name matches route.
	ctx := r.Context()
	member := httpmw.OrganizationMemberParam(r)

	prefs, err := api.Database.GetOrganizationSkillMemberPreferences(ctx, database.GetOrganizationSkillMemberPreferencesParams{
		OrganizationID: member.OrganizationID,
		UserID:         member.UserID,
	})
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, organizationSkillPreferences(prefs))
}

// @Summary Update organization skill preferences for a member
// @ID update-organization-skill-preferences-for-a-member
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID or name"
// @Param user path string true "User ID, name, or me"
// @Param request body codersdk.UpdateOrganizationSkillPreferencesRequest true "Update organization skill preferences request"
// @Success 200 {object} codersdk.OrganizationSkillPreferences
// @Router /api/experimental/organizations/{organization}/members/{user}/skill-preferences [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchOrganizationSkillPreferences(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx          = r.Context()
		organization = httpmw.OrganizationParam(r)
		member       = httpmw.OrganizationMemberParam(r)
	)
	if !api.Authorize(r, policy.ActionUpdatePersonal, rbac.ResourceUserObject(member.UserID)) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if !organization.MembersCanDisableSkills {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Members cannot turn off organization skills in this organization.",
			Detail:  "An organization admin must let members turn off organization skills first.",
		})
		return
	}

	var req codersdk.UpdateOrganizationSkillPreferencesRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.AllSkillsDisabled == nil && len(req.Skills) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid request to update organization skill preferences.",
			Detail:  "Set all_skills_disabled or at least one entry in skills.",
		})
		return
	}

	skillIDs, validations, err := api.parseOrganizationSkillPreferenceIDs(r, organization.ID, req.Skills)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update organization skill preferences.",
			Validations: validations,
		})
		return
	}

	err = api.Database.InTx(func(tx database.Store) error {
		if req.AllSkillsDisabled != nil {
			err := tx.UpsertOrganizationMemberSkillSettings(ctx, database.UpsertOrganizationMemberSkillSettingsParams{
				OrganizationID:    member.OrganizationID,
				UserID:            member.UserID,
				AllSkillsDisabled: *req.AllSkillsDisabled,
			})
			if err != nil {
				return xerrors.Errorf("upsert organization member skill settings: %w", err)
			}
		}
		for _, skill := range skillIDs {
			if skill.enabled {
				err := tx.DeleteOrganizationMemberDisabledSkill(ctx, database.DeleteOrganizationMemberDisabledSkillParams{
					OrganizationID: member.OrganizationID,
					UserID:         member.UserID,
					SkillID:        skill.id,
				})
				if err != nil {
					return xerrors.Errorf("delete organization member disabled skill: %w", err)
				}
				continue
			}
			err := tx.InsertOrganizationMemberDisabledSkill(ctx, database.InsertOrganizationMemberDisabledSkillParams{
				OrganizationID: member.OrganizationID,
				UserID:         member.UserID,
				SkillID:        skill.id,
			})
			if err != nil {
				return xerrors.Errorf("insert organization member disabled skill: %w", err)
			}
		}
		return nil
	}, nil)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	prefs, err := api.Database.GetOrganizationSkillMemberPreferences(ctx, database.GetOrganizationSkillMemberPreferencesParams{
		OrganizationID: member.OrganizationID,
		UserID:         member.UserID,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, organizationSkillPreferences(prefs))
}

type organizationSkillPreferenceChange struct {
	id      uuid.UUID
	enabled bool
}

// parseOrganizationSkillPreferenceIDs validates that every key names an
// organization skill of the organization that the caller can read, and
// returns the changes sorted by skill ID.
func (api *API) parseOrganizationSkillPreferenceIDs(r *http.Request, organizationID uuid.UUID, choices map[string]bool) ([]organizationSkillPreferenceChange, []codersdk.ValidationError, error) {
	if len(choices) == 0 {
		return nil, nil, nil
	}
	rows, err := api.Database.ListOrganizationSkillMetadataByOrganizationID(r.Context(), organizationID)
	if err != nil {
		return nil, nil, xerrors.Errorf("list organization skill metadata: %w", err)
	}
	readable := make(map[uuid.UUID]struct{}, len(rows))
	for _, row := range rows {
		readable[row.ID] = struct{}{}
	}

	var (
		changes     = make([]organizationSkillPreferenceChange, 0, len(choices))
		validations []codersdk.ValidationError
	)
	for key, enabled := range choices {
		field := "skills." + key
		id, err := uuid.Parse(key)
		if err != nil {
			validations = append(validations, codersdk.ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("%q is not a valid skill ID.", key),
			})
			continue
		}
		if _, ok := readable[id]; !ok {
			validations = append(validations, codersdk.ValidationError{
				Field:  field,
				Detail: "No organization skill with this ID exists in this organization.",
			})
			continue
		}
		changes = append(changes, organizationSkillPreferenceChange{id: id, enabled: enabled})
	}
	slices.SortFunc(validations, func(a, b codersdk.ValidationError) int {
		return strings.Compare(a.Field, b.Field)
	})
	slices.SortFunc(changes, func(a, b organizationSkillPreferenceChange) int {
		return bytes.Compare(a.id[:], b.id[:])
	})
	return changes, validations, nil
}

func organizationSkillPreferences(prefs database.GetOrganizationSkillMemberPreferencesRow) codersdk.OrganizationSkillPreferences {
	disabled := prefs.DisabledSkillIDs
	if disabled == nil {
		disabled = []uuid.UUID{}
	}
	return codersdk.OrganizationSkillPreferences{
		MembersCanDisableSkills: prefs.MembersCanDisableSkills,
		AllSkillsDisabled:       prefs.AllSkillsDisabled,
		DisabledSkillIDs:        disabled,
	}
}
