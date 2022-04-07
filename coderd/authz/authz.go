package authz

import (
	"context"
	"errors"
	"strings"

	"github.com/coder/coder/coderd/authz/rbac"
)

var ErrUnauthorized = errors.New("unauthorized")

func Authorize(ctx context.Context, subj Subject, action rbac.Operation, res Object) error {
	if res.ObjectType == "" {
		return ErrUnauthorized
	}

	// Own actions only succeed if the subject owns the resource.
	if !isAll(action) {
		// Before we reject this, the user could be an admin with "-all" perms.
		err := Authorize(ctx, subj, rbac.Operation(strings.ReplaceAll(string(action), "-own", "-all")), res)
		if err == nil {
			return nil
		}
		if res.Owner != subj.ID() {
			return ErrUnauthorized
		}
	}

	if SiteEnforcer.RolesHavePermission(subj.Roles(), res.ObjectType, action) {
		return nil
	}

	if res.OrgOwner != "" {
		orgRoles, err := subj.OrgRoles(ctx, res.OrgOwner)
		if err == nil {
			if OrganizationEnforcer.RolesHavePermission(orgRoles, res.ObjectType, action) {
				return nil
			}
		}
	}

	return ErrUnauthorized
}

func isAll(action rbac.Operation) bool {
	return strings.HasSuffix(string(action), "-all")
}
