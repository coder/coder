package database

import (
	"sort"
	"strconv"
	"time"

	"golang.org/x/exp/maps"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
)

type WorkspaceStatus string

const (
	WorkspaceStatusPending   WorkspaceStatus = "pending"
	WorkspaceStatusStarting  WorkspaceStatus = "starting"
	WorkspaceStatusRunning   WorkspaceStatus = "running"
	WorkspaceStatusStopping  WorkspaceStatus = "stopping"
	WorkspaceStatusStopped   WorkspaceStatus = "stopped"
	WorkspaceStatusFailed    WorkspaceStatus = "failed"
	WorkspaceStatusCanceling WorkspaceStatus = "canceling"
	WorkspaceStatusCanceled  WorkspaceStatus = "canceled"
	WorkspaceStatusDeleting  WorkspaceStatus = "deleting"
	WorkspaceStatusDeleted   WorkspaceStatus = "deleted"
)

func (s WorkspaceStatus) Valid() bool {
	switch s {
	case WorkspaceStatusPending, WorkspaceStatusStarting, WorkspaceStatusRunning,
		WorkspaceStatusStopping, WorkspaceStatusStopped, WorkspaceStatusFailed,
		WorkspaceStatusCanceling, WorkspaceStatusCanceled, WorkspaceStatusDeleting,
		WorkspaceStatusDeleted:
		return true
	default:
		return false
	}
}

type WorkspaceAgentStatus string

// This is also in codersdk/workspaceagents.go and should be kept in sync.
const (
	WorkspaceAgentStatusConnecting   WorkspaceAgentStatus = "connecting"
	WorkspaceAgentStatusConnected    WorkspaceAgentStatus = "connected"
	WorkspaceAgentStatusDisconnected WorkspaceAgentStatus = "disconnected"
	WorkspaceAgentStatusTimeout      WorkspaceAgentStatus = "timeout"
)

func (s WorkspaceAgentStatus) Valid() bool {
	switch s {
	case WorkspaceAgentStatusConnecting, WorkspaceAgentStatusConnected,
		WorkspaceAgentStatusDisconnected, WorkspaceAgentStatusTimeout:
		return true
	default:
		return false
	}
}

type AuditableOrganizationMember struct {
	OrganizationMember
	Username string `json:"username"`
}

func (m OrganizationMember) Auditable(username string) AuditableOrganizationMember {
	return AuditableOrganizationMember{
		OrganizationMember: m,
		Username:           username,
	}
}

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

const EveryoneGroup = "Everyone"

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
	return rbac.ResourceApiKey.WithIDString(k.ID).
		WithOwner(k.UserID.String())
}

func (t Template) RBACObject() rbac.Object {
	return rbac.ResourceTemplate.WithID(t.ID).
		InOrg(t.OrganizationID).
		WithACLUserList(t.UserACL).
		WithGroupACL(t.GroupACL)
}

func (t GetFileTemplatesRow) RBACObject() rbac.Object {
	return rbac.ResourceTemplate.WithID(t.TemplateID).
		InOrg(t.TemplateOrganizationID).
		WithACLUserList(t.UserACL).
		WithGroupACL(t.GroupACL)
}

func (t Template) DeepCopy() Template {
	cpy := t
	cpy.UserACL = maps.Clone(t.UserACL)
	cpy.GroupACL = maps.Clone(t.GroupACL)
	return cpy
}

// AutostartAllowedDays returns the inverse of 'AutostartBlockDaysOfWeek'.
// It is more useful to have the days that are allowed to autostart from a UX
// POV. The database prefers the 0 value being 'all days allowed'.
func (t Template) AutostartAllowedDays() uint8 {
	// Just flip the binary 0s to 1s and vice versa.
	// There is an extra day with the 8th bit that needs to be zeroed.
	return ^uint8(t.AutostartBlockDaysOfWeek) & 0b01111111
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

func (w GetWorkspaceByAgentIDRow) RBACObject() rbac.Object {
	return w.Workspace.RBACObject()
}

func (w Workspace) RBACObject() rbac.Object {
	// If a workspace is locked it cannot be accessed.
	if w.DormantAt.Valid {
		return w.DormantRBAC()
	}

	return rbac.ResourceWorkspace.WithID(w.ID).
		InOrg(w.OrganizationID).
		WithOwner(w.OwnerID.String())
}

func (w Workspace) DormantRBAC() rbac.Object {
	return rbac.ResourceWorkspaceDormant.
		WithID(w.ID).
		InOrg(w.OrganizationID).
		WithOwner(w.OwnerID.String())
}

func (m OrganizationMember) RBACObject() rbac.Object {
	return rbac.ResourceOrganizationMember.
		WithID(m.UserID).
		InOrg(m.OrganizationID).
		WithOwner(m.UserID.String())
}

func (m OrganizationMembersRow) RBACObject() rbac.Object {
	return m.OrganizationMember.RBACObject()
}

func (m GetOrganizationIDsByMemberIDsRow) RBACObject() rbac.Object {
	// TODO: This feels incorrect as we are really returning a list of orgmembers.
	// This return type should be refactored to return a list of orgmembers, not this
	// special type.
	return rbac.ResourceUserObject(m.UserID)
}

func (o Organization) RBACObject() rbac.Object {
	return rbac.ResourceOrganization.
		WithID(o.ID).
		InOrg(o.ID)
}

func (p ProvisionerDaemon) RBACObject() rbac.Object {
	return rbac.ResourceProvisionerDaemon.WithID(p.ID)
}

func (w WorkspaceProxy) RBACObject() rbac.Object {
	return rbac.ResourceWorkspaceProxy.
		WithID(w.ID)
}

func (w WorkspaceProxy) IsPrimary() bool {
	return w.Name == "primary"
}

func (f File) RBACObject() rbac.Object {
	return rbac.ResourceFile.
		WithID(f.ID).
		WithOwner(f.CreatedBy.String())
}

// RBACObject returns the RBAC object for the site wide user resource.
func (u User) RBACObject() rbac.Object {
	return rbac.ResourceUserObject(u.ID)
}

func (u GetUsersRow) RBACObject() rbac.Object {
	return rbac.ResourceUserObject(u.ID)
}

func (u GitSSHKey) RBACObject() rbac.Object        { return rbac.ResourceUserObject(u.UserID) }
func (u ExternalAuthLink) RBACObject() rbac.Object { return rbac.ResourceUserObject(u.UserID) }
func (u UserLink) RBACObject() rbac.Object         { return rbac.ResourceUserObject(u.UserID) }

func (u ExternalAuthLink) OAuthToken() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  u.OAuthAccessToken,
		RefreshToken: u.OAuthRefreshToken,
		Expiry:       u.OAuthExpiry,
	}
}

func (l License) RBACObject() rbac.Object {
	return rbac.ResourceLicense.WithIDString(strconv.FormatInt(int64(l.ID), 10))
}

func (c OAuth2ProviderAppCode) RBACObject() rbac.Object {
	return rbac.ResourceOauth2AppCodeToken.WithOwner(c.UserID.String())
}

func (OAuth2ProviderAppSecret) RBACObject() rbac.Object {
	return rbac.ResourceOauth2AppSecret
}

func (OAuth2ProviderApp) RBACObject() rbac.Object {
	return rbac.ResourceOauth2App
}

func (a GetOAuth2ProviderAppsByUserIDRow) RBACObject() rbac.Object {
	return a.OAuth2ProviderApp.RBACObject()
}

type WorkspaceAgentConnectionStatus struct {
	Status           WorkspaceAgentStatus `json:"status"`
	FirstConnectedAt *time.Time           `json:"first_connected_at"`
	LastConnectedAt  *time.Time           `json:"last_connected_at"`
	DisconnectedAt   *time.Time           `json:"disconnected_at"`
}

func (a WorkspaceAgent) Status(inactiveTimeout time.Duration) WorkspaceAgentConnectionStatus {
	connectionTimeout := time.Duration(a.ConnectionTimeoutSeconds) * time.Second

	status := WorkspaceAgentConnectionStatus{
		Status: WorkspaceAgentStatusDisconnected,
	}
	if a.FirstConnectedAt.Valid {
		status.FirstConnectedAt = &a.FirstConnectedAt.Time
	}
	if a.LastConnectedAt.Valid {
		status.LastConnectedAt = &a.LastConnectedAt.Time
	}
	if a.DisconnectedAt.Valid {
		status.DisconnectedAt = &a.DisconnectedAt.Time
	}

	switch {
	case !a.FirstConnectedAt.Valid:
		switch {
		case connectionTimeout > 0 && dbtime.Now().Sub(a.CreatedAt) > connectionTimeout:
			// If the agent took too long to connect the first time,
			// mark it as timed out.
			status.Status = WorkspaceAgentStatusTimeout
		default:
			// If the agent never connected, it's waiting for the compute
			// to start up.
			status.Status = WorkspaceAgentStatusConnecting
		}
	// We check before instead of after because last connected at and
	// disconnected at can be equal timestamps in tight-timed tests.
	case !a.DisconnectedAt.Time.Before(a.LastConnectedAt.Time):
		// If we've disconnected after our last connection, we know the
		// agent is no longer connected.
		status.Status = WorkspaceAgentStatusDisconnected
	case dbtime.Now().Sub(a.LastConnectedAt.Time) > inactiveTimeout:
		// The connection died without updating the last connected.
		status.Status = WorkspaceAgentStatusDisconnected
		// Client code needs an accurate disconnected at if the agent has been inactive.
		status.DisconnectedAt = &a.LastConnectedAt.Time
	case a.LastConnectedAt.Valid:
		// The agent should be assumed connected if it's under inactivity timeouts
		// and last connected at has been properly set.
		status.Status = WorkspaceAgentStatusConnected
	}

	return status
}

func ConvertUserRows(rows []GetUsersRow) []User {
	users := make([]User, len(rows))
	for i, r := range rows {
		users[i] = User{
			ID:              r.ID,
			Email:           r.Email,
			Username:        r.Username,
			HashedPassword:  r.HashedPassword,
			CreatedAt:       r.CreatedAt,
			UpdatedAt:       r.UpdatedAt,
			Status:          r.Status,
			RBACRoles:       r.RBACRoles,
			LoginType:       r.LoginType,
			AvatarURL:       r.AvatarURL,
			Deleted:         r.Deleted,
			LastSeenAt:      r.LastSeenAt,
			ThemePreference: r.ThemePreference,
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
			DormantAt:         r.DormantAt,
			DeletingAt:        r.DeletingAt,
			AutomaticUpdates:  r.AutomaticUpdates,
			Favorite:          r.Favorite,
		}
	}

	return workspaces
}

func (g Group) IsEveryone() bool {
	return g.ID == g.OrganizationID
}

func (p ProvisionerJob) Finished() bool {
	return p.CanceledAt.Valid || p.CompletedAt.Valid
}

func (p ProvisionerJob) FinishedAt() time.Time {
	if p.CompletedAt.Valid {
		return p.CompletedAt.Time
	}

	if p.CanceledAt.Valid {
		return p.CanceledAt.Time
	}

	return time.Time{}
}

func (r CustomRole) RoleIdentifier() rbac.RoleIdentifier {
	return rbac.RoleIdentifier{
		Name:           r.Name,
		OrganizationID: r.OrganizationID.UUID,
	}
}

func (r GetAuthorizationUserRolesRow) RoleNames() ([]rbac.RoleIdentifier, error) {
	names := make([]rbac.RoleIdentifier, 0, len(r.Roles))
	for _, role := range r.Roles {
		value, err := rbac.RoleNameFromString(role)
		if err != nil {
			return nil, xerrors.Errorf("convert role %q: %w", role, err)
		}
		names = append(names, value)
	}
	return names, nil
}
