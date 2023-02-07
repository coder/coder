package database

import (
	"sort"
	"strconv"

	"github.com/coder/coder/coderd/rbac"
)

type AuditableGroup struct {
	Group
	Members []GroupMember `json:"members"`
}

// Auditable returns an object that can be used in audit logs.
// Covers both group and group member changes.
func (g Group) Auditable(users []User) AuditableGroup {
	members := make([]GroupMember, 0, len(users))
	for _, u := range users {
		members = append(members, GroupMember{
			UserID:  u.ID,
			GroupID: g.ID,
		})
	}

	// consistent ordering
	sort.Slice(members, func(i, j int) bool {
		return members[i].UserID.String() < members[j].UserID.String()
	})

	return AuditableGroup{
		Group:   g,
		Members: members,
	}
}

const AllUsersGroup = "Everyone"

func (s APIKeyScope) ToRBAC() rbac.ScopeName {
	switch s {
	case APIKeyScopeAll:
		return rbac.ScopeAll
	case APIKeyScopeApplicationConnect:
		return rbac.ScopeApplicationConnect
	default:
		panic("developer error: unknown scope type " + string(s))
	}
}

func (k APIKey) RBACObject() rbac.Object {
	return rbac.ResourceAPIKey.WithIDString(k.ID).
		WithOwner(k.UserID.String())
}

func (t Template) RBACObject() rbac.Object {
	return rbac.ResourceTemplate.WithID(t.ID).
		InOrg(t.OrganizationID).
		WithACLUserList(t.UserACL).
		WithGroupACL(t.GroupACL)
}

func (TemplateVersion) RBACObject(template Template) rbac.Object {
	// Just use the parent template resource for controlling versions
	return template.RBACObject()
}

// RBACObjectNoTemplate is for orphaned template versions.
func (v TemplateVersion) RBACObjectNoTemplate() rbac.Object {
	return rbac.ResourceTemplate.InOrg(v.OrganizationID)
}

func (g Group) RBACObject() rbac.Object {
	return rbac.ResourceGroup.WithID(g.ID).
		InOrg(g.OrganizationID)
}

func (w Workspace) RBACObject() rbac.Object {
	return rbac.ResourceWorkspace.WithID(w.ID).
		InOrg(w.OrganizationID).
		WithOwner(w.OwnerID.String())
}

func (w Workspace) ExecutionRBAC() rbac.Object {
	return rbac.ResourceWorkspaceExecution.
		WithID(w.ID).
		InOrg(w.OrganizationID).
		WithOwner(w.OwnerID.String())
}

func (w Workspace) ApplicationConnectRBAC() rbac.Object {
	return rbac.ResourceWorkspaceApplicationConnect.
		WithID(w.ID).
		InOrg(w.OrganizationID).
		WithOwner(w.OwnerID.String())
}

func (m OrganizationMember) RBACObject() rbac.Object {
	return rbac.ResourceOrganizationMember.
		WithID(m.UserID).
		InOrg(m.OrganizationID)
}

func (m GetOrganizationIDsByMemberIDsRow) RBACObject() rbac.Object {
	// TODO: This feels incorrect as we are really returning a list of orgmembers.
	// This return type should be refactored to return a list of orgmembers, not this
	// special type.
	return rbac.ResourceUser.WithID(m.UserID)
}

func (o Organization) RBACObject() rbac.Object {
	return rbac.ResourceOrganization.
		WithID(o.ID).
		InOrg(o.ID)
}

func (p ProvisionerDaemon) RBACObject() rbac.Object {
	return rbac.ResourceProvisionerDaemon.WithID(p.ID)
}

func (f File) RBACObject() rbac.Object {
	return rbac.ResourceFile.
		WithID(f.ID).
		WithOwner(f.CreatedBy.String())
}

// RBACObject returns the RBAC object for the site wide user resource.
// If you are trying to get the RBAC object for the UserData, use
// u.UserDataRBACObject() instead.
func (u User) RBACObject() rbac.Object {
	return rbac.ResourceUser.WithID(u.ID)
}

func (u User) UserDataRBACObject() rbac.Object {
	return rbac.ResourceUserData.WithID(u.ID).WithOwner(u.ID.String())
}

func (u GetUsersRow) RBACObject() rbac.Object {
	return rbac.ResourceUser.WithID(u.ID)
}

func (u GitSSHKey) RBACObject() rbac.Object {
	return rbac.ResourceUserData.WithID(u.UserID).WithOwner(u.UserID.String())
}

func (u GitAuthLink) RBACObject() rbac.Object {
	// I assume UserData is ok?
	return rbac.ResourceUserData.WithID(u.UserID).WithOwner(u.UserID.String())
}

func (u UserLink) RBACObject() rbac.Object {
	// I assume UserData is ok?
	return rbac.ResourceUserData.WithOwner(u.UserID.String()).WithID(u.UserID)
}

func (l License) RBACObject() rbac.Object {
	return rbac.ResourceLicense.WithIDString(strconv.FormatInt(int64(l.ID), 10))
}

func ConvertUserRows(rows []GetUsersRow) []User {
	users := make([]User, len(rows))
	for i, r := range rows {
		users[i] = User{
			ID:             r.ID,
			Email:          r.Email,
			Username:       r.Username,
			HashedPassword: r.HashedPassword,
			CreatedAt:      r.CreatedAt,
			UpdatedAt:      r.UpdatedAt,
			Status:         r.Status,
			RBACRoles:      r.RBACRoles,
			LoginType:      r.LoginType,
			AvatarURL:      r.AvatarURL,
			Deleted:        r.Deleted,
			LastSeenAt:     r.LastSeenAt,
		}
	}

	return users
}

func ConvertWorkspaceRows(rows []GetWorkspacesRow) []Workspace {
	workspaces := make([]Workspace, len(rows))
	for i, r := range rows {
		workspaces[i] = Workspace{
			ID:                r.ID,
			CreatedAt:         r.CreatedAt,
			UpdatedAt:         r.UpdatedAt,
			OwnerID:           r.OwnerID,
			OrganizationID:    r.OrganizationID,
			TemplateID:        r.TemplateID,
			Deleted:           r.Deleted,
			Name:              r.Name,
			AutostartSchedule: r.AutostartSchedule,
			Ttl:               r.Ttl,
			LastUsedAt:        r.LastUsedAt,
		}
	}

	return workspaces
}
