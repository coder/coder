package acl

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

type UpdateValidator[Role codersdk.WorkspaceRole | codersdk.TemplateRole] interface {
	// Users should return a map from user UUIDs (as strings) to the role they
	// are being assigned. Additionally, it should return a string that will be
	// used as the field name for the ValidationErrors returned from Validate.
	Users() (map[string]Role, string)
	// Groups should return a map from group UUIDs (as strings) to the role they
	// are being assigned. Additionally, it should return a string that will be
	// used as the field name for the ValidationErrors returned from Validate.
	Groups() (map[string]Role, string)
	// ValidateRole should return an error that will be used in the
	// ValidationError if the role is invalid for the corresponding resource type.
	ValidateRole(role Role) error
}

func Validate[Role codersdk.WorkspaceRole | codersdk.TemplateRole](
	ctx context.Context,
	db database.Store,
	v UpdateValidator[Role],
) []codersdk.ValidationError {
	// nolint:gocritic // Validate requires full read access to users and groups
	ctx = dbauthz.AsSystemRestricted(ctx)
	var validErrs []codersdk.ValidationError

	groupRoles, groupsField := v.Groups()
	groupIDs := make([]uuid.UUID, 0, len(groupRoles))
	for idStr, role := range groupRoles {
		// Validate the provided role names
		if err := v.ValidateRole(role); err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field:  groupsField,
				Detail: err.Error(),
			})
		}
		// Validate that the IDs are UUIDs
		id, err := uuid.Parse(idStr)
		if err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field:  groupsField,
				Detail: fmt.Sprintf("%v is not a valid UUID.", idStr),
			})
			continue
		}
		// Don't check if the ID exists when setting the role to
		// WorkspaceRoleDeleted or TemplateRoleDeleted. They might've existing at
		// some point and got deleted. If we report that as an error here then they
		// can't be removed.
		if string(role) == "" {
			continue
		}
		groupIDs = append(groupIDs, id)
	}

	// Validate that the groups exist
	groupValidation, err := db.ValidateGroupIDs(ctx, groupIDs)
	if err != nil {
		validErrs = append(validErrs, codersdk.ValidationError{
			Field:  groupsField,
			Detail: fmt.Sprintf("failed to validate group IDs: %v", err.Error()),
		})
	}
	if !groupValidation.Ok {
		for _, id := range groupValidation.InvalidGroupIds {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field:  groupsField,
				Detail: fmt.Sprintf("group with ID %v does not exist", id),
			})
		}
	}

	userRoles, usersField := v.Users()
	userIDs := make([]uuid.UUID, 0, len(userRoles))
	for idStr, role := range userRoles {
		// Validate the provided role names
		if err := v.ValidateRole(role); err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field:  usersField,
				Detail: err.Error(),
			})
		}
		// Validate that the IDs are UUIDs
		id, err := uuid.Parse(idStr)
		if err != nil {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field:  usersField,
				Detail: fmt.Sprintf("%v is not a valid UUID.", idStr),
			})
			continue
		}
		// Don't check if the ID exists when setting the role to
		// WorkspaceRoleDeleted or TemplateRoleDeleted. They might've existing at
		// some point and got deleted. If we report that as an error here then they
		// can't be removed.
		if string(role) == "" {
			continue
		}
		userIDs = append(userIDs, id)
	}

	// Validate that the groups exist
	userValidation, err := db.ValidateUserIDs(ctx, userIDs)
	if err != nil {
		validErrs = append(validErrs, codersdk.ValidationError{
			Field:  usersField,
			Detail: fmt.Sprintf("failed to validate user IDs: %v", err.Error()),
		})
	}
	if !userValidation.Ok {
		for _, id := range userValidation.InvalidUserIds {
			validErrs = append(validErrs, codersdk.ValidationError{
				Field:  usersField,
				Detail: fmt.Sprintf("user with ID %v does not exist", id),
			})
		}
	}

	return validErrs
}
