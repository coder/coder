package coderd

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get template available acl users/groups
// @ID get-template-available-acl-usersgroups
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param template path string true "Template ID" format(uuid)
// @Success 200 {array} codersdk.ACLAvailable
// @Router /templates/{template}/acl/available [get]
func (api *API) templateAvailablePermissions(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx      = r.Context()
		template = httpmw.TemplateParam(r)
	)

	// Requires update permission on the template to list all avail users/groups
	// for assignment.
	if !api.Authorize(r, rbac.ActionUpdate, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	// We have to use the system restricted context here because the caller
	// might not have permission to read all users.
	// nolint:gocritic
	users, _, ok := api.AGPL.GetUsers(rw, r.WithContext(dbauthz.AsSystemRestricted(ctx)))
	if !ok {
		return
	}

	// Perm check is the template update check.
	// nolint:gocritic
	groups, err := api.Database.GetGroupsByOrganizationID(dbauthz.AsSystemRestricted(ctx), template.OrganizationID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	sdkGroups := make([]codersdk.Group, 0, len(groups))
	for _, group := range groups {
		// nolint:gocritic
		members, err := api.Database.GetGroupMembers(dbauthz.AsSystemRestricted(ctx), group.ID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}

		sdkGroups = append(sdkGroups, db2sdk.Group(group, members))
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ACLAvailable{
		// TODO: @emyrk we should return a MinimalUser here instead of a full user.
		// The FE requires the `email` field, so this cannot be done without
		// a UI change.
		Users:  db2sdk.ReducedUsers(users),
		Groups: sdkGroups,
	})
}

// @Summary Get template ACLs
// @ID get-template-acls
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param template path string true "Template ID" format(uuid)
// @Success 200 {array} codersdk.TemplateUser
// @Router /templates/{template}/acl [get]
func (api *API) templateACL(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx      = r.Context()
		template = httpmw.TemplateParam(r)
	)

	users, err := api.Database.GetTemplateUserRoles(ctx, template.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	dbGroups, err := api.Database.GetTemplateGroupRoles(ctx, template.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	userIDs := make([]uuid.UUID, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}

	orgIDsByMemberIDsRows, err := api.Database.GetOrganizationIDsByMemberIDs(r.Context(), userIDs)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}

	organizationIDsByUserID := map[uuid.UUID][]uuid.UUID{}
	for _, organizationIDsByMemberIDsRow := range orgIDsByMemberIDsRows {
		organizationIDsByUserID[organizationIDsByMemberIDsRow.UserID] = organizationIDsByMemberIDsRow.OrganizationIDs
	}

	groups := make([]codersdk.TemplateGroup, 0, len(dbGroups))
	for _, group := range dbGroups {
		var members []database.User

		// This is a bit of a hack. The caller might not have permission to do this,
		// but they can read the acl list if the function got this far. So we let
		// them read the group members.
		// We should probably at least return more truncated user data here.
		// nolint:gocritic
		members, err = api.Database.GetGroupMembers(dbauthz.AsSystemRestricted(ctx), group.ID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		groups = append(groups, codersdk.TemplateGroup{
			Group: db2sdk.Group(group.Group, members),
			Role:  convertToTemplateRole(group.Actions),
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.TemplateACL{
		Users:  convertTemplateUsers(users, organizationIDsByUserID),
		Groups: groups,
	})
}

// @Summary Update template ACL
// @ID update-template-acl
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param template path string true "Template ID" format(uuid)
// @Param request body codersdk.UpdateTemplateACL true "Update template request"
// @Success 200 {object} codersdk.Response
// @Router /templates/{template}/acl [patch]
func (api *API) patchTemplateACL(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		template          = httpmw.TemplateParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.Template](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	defer commitAudit()
	aReq.Old = template

	var req codersdk.UpdateTemplateACL
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	validErrs := validateTemplateACLPerms(ctx, api.Database, req.UserPerms, "user_perms", true)
	validErrs = append(validErrs,
		validateTemplateACLPerms(ctx, api.Database, req.GroupPerms, "group_perms", false)...)

	if len(validErrs) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update template metadata!",
			Validations: validErrs,
		})
		return
	}

	err := api.Database.InTx(func(tx database.Store) error {
		var err error
		template, err = tx.GetTemplateByID(ctx, template.ID)
		if err != nil {
			return xerrors.Errorf("get template by ID: %w", err)
		}

		if len(req.UserPerms) > 0 {
			for id, role := range req.UserPerms {
				// A user with an empty string implies
				// deletion.
				if role == "" {
					delete(template.UserACL, id)
					continue
				}
				template.UserACL[id] = convertSDKTemplateRole(role)
			}
		}

		if len(req.GroupPerms) > 0 {
			for id, role := range req.GroupPerms {
				// An id with an empty string implies
				// deletion.
				if role == "" {
					delete(template.GroupACL, id)
					continue
				}
				template.GroupACL[id] = convertSDKTemplateRole(role)
			}
		}

		err = tx.UpdateTemplateACLByID(ctx, database.UpdateTemplateACLByIDParams{
			ID:       template.ID,
			UserACL:  template.UserACL,
			GroupACL: template.GroupACL,
		})
		if err != nil {
			return xerrors.Errorf("update template ACL by ID: %w", err)
		}
		template, err = tx.GetTemplateByID(ctx, template.ID)
		if err != nil {
			return xerrors.Errorf("get updated template by ID: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = template

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Successfully updated template ACL list.",
	})
}

// nolint TODO fix stupid flag.
func validateTemplateACLPerms(ctx context.Context, db database.Store, perms map[string]codersdk.TemplateRole, field string, isUser bool) []codersdk.ValidationError {
	// Validate requires full read access to users and groups
	// nolint:gocritic
	ctx = dbauthz.AsSystemRestricted(ctx)
	var validErrs []codersdk.ValidationError
	for k, v := range perms {
		if err := validateTemplateRole(v); err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{Field: field, Detail: err.Error()})
			continue
		}

		id, err := uuid.Parse(k)
		if err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{Field: field, Detail: "ID " + k + "must be a valid UUID."})
			continue
		}

		if isUser {
			// This could get slow if we get a ton of user perm updates.
			_, err = db.GetUserByID(ctx, id)
			if err != nil {
				validErrs = append(validErrs, codersdk.ValidationError{Field: field, Detail: fmt.Sprintf("Failed to find resource with ID %q: %v", k, err.Error())})
				continue
			}
		} else {
			// This could get slow if we get a ton of group perm updates.
			_, err = db.GetGroupByID(ctx, id)
			if err != nil {
				validErrs = append(validErrs, codersdk.ValidationError{Field: field, Detail: fmt.Sprintf("Failed to find resource with ID %q: %v", k, err.Error())})
				continue
			}
		}
	}

	return validErrs
}

func convertTemplateUsers(tus []database.TemplateUser, orgIDsByUserIDs map[uuid.UUID][]uuid.UUID) []codersdk.TemplateUser {
	users := make([]codersdk.TemplateUser, 0, len(tus))

	for _, tu := range tus {
		users = append(users, codersdk.TemplateUser{
			User: db2sdk.User(tu.User, orgIDsByUserIDs[tu.User.ID]),
			Role: convertToTemplateRole(tu.Actions),
		})
	}

	return users
}

func validateTemplateRole(role codersdk.TemplateRole) error {
	actions := convertSDKTemplateRole(role)
	if actions == nil && role != codersdk.TemplateRoleDeleted {
		return xerrors.Errorf("role %q is not a valid Template role", role)
	}

	return nil
}

func convertToTemplateRole(actions []rbac.Action) codersdk.TemplateRole {
	switch {
	case len(actions) == 1 && actions[0] == rbac.ActionRead:
		return codersdk.TemplateRoleUse
	case len(actions) == 1 && actions[0] == rbac.WildcardSymbol:
		return codersdk.TemplateRoleAdmin
	}

	return ""
}

func convertSDKTemplateRole(role codersdk.TemplateRole) []rbac.Action {
	switch role {
	case codersdk.TemplateRoleAdmin:
		return []rbac.Action{rbac.WildcardSymbol}
	case codersdk.TemplateRoleUse:
		return []rbac.Action{rbac.ActionRead}
	}

	return nil
}

// TODO reduce the duplication across all of these.
func (api *API) templateRBACEnabledMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		api.entitlementsMu.RLock()
		rbac := api.entitlements.Features[codersdk.FeatureTemplateRBAC].Enabled
		api.entitlementsMu.RUnlock()

		if !rbac {
			httpapi.RouteNotFound(rw)
			return
		}

		next.ServeHTTP(rw, r)
	})
}

func (api *API) moonsEnabledMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Entitlement must be enabled.
		api.entitlementsMu.RLock()
		proxy := api.entitlements.Features[codersdk.FeatureWorkspaceProxy].Enabled
		api.entitlementsMu.RUnlock()
		if !proxy {
			httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
				Message: "External workspace proxies is an Enterprise feature. Contact sales!",
			})
			return
		}

		next.ServeHTTP(rw, r)
	})
}
