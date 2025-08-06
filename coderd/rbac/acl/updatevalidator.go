package acl

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

type ACLUpdateValidator[Role codersdk.WorkspaceRole | codersdk.TemplateRole] interface {
	Users() (map[string]Role, string)
	Groups() (map[string]Role, string)
	ValidateRole(role Role) error
}

func Validate[T codersdk.WorkspaceRole | codersdk.TemplateRole](
	ctx context.Context,
	db database.Store,
	v ACLUpdateValidator[T],
) []codersdk.ValidationError {
	// nolint:gocritic // Validate requires full read access to users and groups
	ctx = dbauthz.AsSystemRestricted(ctx)
	var validErrs []codersdk.ValidationError

	groupPerms, groupsField := v.Groups()
	groupIDs := make([]uuid.UUID, 0, len(groupPerms))
	for idStr, role := range groupPerms {
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
				Detail: idStr + "is not a valid UUID.",
			})
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

	userPerms, usersField := v.Users()
	userIDs := make([]uuid.UUID, 0, len(userPerms))
	for idStr, role := range userPerms {
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
				Detail: idStr + "is not a valid UUID.",
			})
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
