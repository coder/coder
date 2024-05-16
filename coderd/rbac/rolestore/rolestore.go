package rolestore

import (
	"encoding/json"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
)

func ConvertDBRole(dbRole database.CustomRole) (rbac.Role, error) {
	role := rbac.Role{
		Name:        dbRole.Name,
		DisplayName: dbRole.DisplayName,
		Site:        nil,
		Org:         nil,
		User:        nil,
	}

	err := json.Unmarshal(dbRole.SitePermissions, &role.Site)
	if err != nil {
		return role, xerrors.Errorf("unmarshal site permissions: %w", err)
	}

	err = json.Unmarshal(dbRole.OrgPermissions, &role.Org)
	if err != nil {
		return role, xerrors.Errorf("unmarshal org permissions: %w", err)
	}

	err = json.Unmarshal(dbRole.UserPermissions, &role.User)
	if err != nil {
		return role, xerrors.Errorf("unmarshal user permissions: %w", err)
	}

	return role, nil
}
