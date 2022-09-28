package coderd

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

func (api *API) templateACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	template := httpmw.TemplateParam(r)
	if !api.Authorize(r, rbac.ActionRead, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

	users, err := api.Database.GetTemplateUserRoles(ctx, template.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	users, err = coderd.AuthorizeFilter(api.AGPL.HTTPAuth, r, rbac.ActionRead, users)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching users.",
			Detail:  err.Error(),
		})
		return
	}

	groups, err := api.Database.GetTemplateGroupRoles(ctx, template.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	role, ok := template.GroupACL()[database.AllUsersGroup]
	if ok {
		group, err := api.Database.GetGroupByOrgAndName(ctx, database.GetGroupByOrgAndNameParams{
			OrganizationID: template.OrganizationID,
			Name:           database.AllUsersGroup,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		groups = append(groups, database.TemplateGroup{
			Group: group,
			Role:  role,
		})
	}

	groups, err = coderd.AuthorizeFilter(api.AGPL.HTTPAuth, r, rbac.ActionRead, groups)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching users.",
			Detail:  err.Error(),
		})
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

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.TemplateACL{
		Users:  convertTemplateUsers(users, organizationIDsByUserID),
		Groups: convertTemplateGroups(groups),
	})
}

func (api *API) patchTemplateACL(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx      = r.Context()
		template = httpmw.TemplateParam(r)
	)

	// Only users who are able to create templates (aka template admins)
	// are able to control permissions.
	if !api.Authorize(r, rbac.ActionCreate, template) {
		httpapi.ResourceNotFound(rw)
		return
	}

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
		if len(req.UserPerms) > 0 {
			userACL := template.UserACL()
			for k, v := range req.UserPerms {
				// A user with an empty string implies
				// deletion.
				if v == "" {
					delete(userACL, k)
					continue
				}
				userACL[k] = database.TemplateRole(v)
			}

			err := tx.UpdateTemplateUserACLByID(r.Context(), template.ID, userACL)
			if err != nil {
				return xerrors.Errorf("update template user ACL: %w", err)
			}
		}

		if len(req.GroupPerms) > 0 {
			allUsersGroup, err := tx.GetGroupByOrgAndName(ctx, database.GetGroupByOrgAndNameParams{
				OrganizationID: template.OrganizationID,
				Name:           database.AllUsersGroup,
			})
			if err != nil {
				return xerrors.Errorf("get allUsers group: %w", err)
			}

			groupACL := template.GroupACL()
			for k, v := range req.GroupPerms {
				if k == allUsersGroup.ID.String() {
					k = database.AllUsersGroup
				}
				// An id with an empty string implies
				// deletion.
				if v == "" {
					delete(groupACL, k)
					continue
				}
				groupACL[k] = database.TemplateRole(v)
			}

			err = tx.UpdateTemplateGroupACLByID(r.Context(), template.ID, groupACL)
			if err != nil {
				return xerrors.Errorf("update template user ACL: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Successfully updated template ACL list.",
	})
}

// nolint TODO fix stupid flag.
func validateTemplateACLPerms(ctx context.Context, db database.Store, perms map[string]codersdk.TemplateRole, field string, isUser bool) []codersdk.ValidationError {
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
			User: convertUser(tu.User, orgIDsByUserIDs[tu.User.ID]),
			Role: codersdk.TemplateRole(tu.Role),
		})
	}

	return users
}

func convertTemplateGroups(tgs []database.TemplateGroup) []codersdk.TemplateGroup {
	groups := make([]codersdk.TemplateGroup, 0, len(tgs))

	for _, tg := range tgs {
		groups = append(groups, codersdk.TemplateGroup{
			Group: convertGroup(tg.Group, nil),
			Role:  codersdk.TemplateRole(tg.Role),
		})
	}

	return groups
}

func validateTemplateRole(role codersdk.TemplateRole) error {
	dbRole := convertSDKTemplateRole(role)
	if dbRole == "" && role != codersdk.TemplateRoleDeleted {
		return xerrors.Errorf("role %q is not a valid Template role", role)
	}

	return nil
}

func convertSDKTemplateRole(role codersdk.TemplateRole) database.TemplateRole {
	switch role {
	case codersdk.TemplateRoleAdmin:
		return database.TemplateRoleAdmin
	case codersdk.TemplateRoleView:
		return database.TemplateRoleRead
	}

	return ""
}
