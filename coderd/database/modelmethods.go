package database

import (
	"database/sql"
	"encoding/hex"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/maps"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
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
	Members []GroupMemberTable `json:"members"`
}

// Auditable returns an object that can be used in audit logs.
// Covers both group and group member changes.
func (g Group) Auditable(members []GroupMember) AuditableGroup {
	membersTable := make([]GroupMemberTable, len(members))
	for i, member := range members {
		membersTable[i] = GroupMemberTable{
			UserID:  member.UserID,
			GroupID: member.GroupID,
		}
	}

	// consistent ordering
	sort.Slice(members, func(i, j int) bool {
		return members[i].UserID.String() < members[j].UserID.String()
	})

	return AuditableGroup{
		Group:   g,
		Members: membersTable,
	}
}

const EveryoneGroup = "Everyone"

func (w GetAuditLogsOffsetRow) RBACObject() rbac.Object {
	return w.AuditLog.RBACObject()
}

func (w AuditLog) RBACObject() rbac.Object {
	obj := rbac.ResourceAuditLog.WithID(w.ID)
	if w.OrganizationID != uuid.Nil {
		obj = obj.InOrg(w.OrganizationID)
	}

	return obj
}

func (w GetConnectionLogsOffsetRow) RBACObject() rbac.Object {
	return w.ConnectionLog.RBACObject()
}

func (w ConnectionLog) RBACObject() rbac.Object {
	obj := rbac.ResourceConnectionLog.WithID(w.ID)
	if w.OrganizationID != uuid.Nil {
		obj = obj.InOrg(w.OrganizationID)
	}

	return obj
}

// TaskTable converts a Task to it's reduced version.
// A more generalized solution is to use json marshaling to
// consistently keep these two structs in sync.
// That would be a lot of overhead, and a more costly unit test is
// written to make sure these match up.
func (t Task) TaskTable() TaskTable {
	return TaskTable{
		ID:                 t.ID,
		OrganizationID:     t.OrganizationID,
		OwnerID:            t.OwnerID,
		Name:               t.Name,
		DisplayName:        t.DisplayName,
		WorkspaceID:        t.WorkspaceID,
		TemplateVersionID:  t.TemplateVersionID,
		TemplateParameters: t.TemplateParameters,
		Prompt:             t.Prompt,
		CreatedAt:          t.CreatedAt,
		DeletedAt:          t.DeletedAt,
	}
}

func (t Task) RBACObject() rbac.Object {
	return t.TaskTable().RBACObject()
}

func (t TaskTable) RBACObject() rbac.Object {
	return rbac.ResourceTask.
		WithID(t.ID).
		WithOwner(t.OwnerID.String()).
		InOrg(t.OrganizationID)
}

func (s APIKeyScope) ToRBAC() rbac.ScopeName {
	switch s {
	case ApiKeyScopeCoderAll:
		return rbac.ScopeAll
	case ApiKeyScopeCoderApplicationConnect:
		return rbac.ScopeApplicationConnect
	default:
		// Allow low-level resource:action scopes to flow through to RBAC for
		// expansion via rbac.ExpandScope.
		return rbac.ScopeName(s)
	}
}

// APIKeyScopes represents a collection of individual API key scope names as
// stored in the database. Helper methods on this type are used to derive the
// RBAC scope that should be authorized for the key.
type APIKeyScopes []APIKeyScope

// WithAllowList wraps the scopes with a database allow list, producing an
// ExpandableScope that always enforces the allow list overlay when expanded.
func (s APIKeyScopes) WithAllowList(list AllowList) APIKeyScopeSet {
	return APIKeyScopeSet{Scopes: s, AllowList: list}
}

// Has returns true if the slice contains the provided scope.
func (s APIKeyScopes) Has(target APIKeyScope) bool {
	return slices.Contains(s, target)
}

// expandRBACScope merges the permissions of all scopes in the list into a
// single RBAC scope. If the list is empty, it defaults to rbac.ScopeAll for
// backward compatibility. This method is internal; use ScopeSet() to combine
// scopes with the API key's allow list for authorization.
func (s APIKeyScopes) expandRBACScope() (rbac.Scope, error) {
	// Default to ScopeAll for backward compatibility when no scopes provided.
	if len(s) == 0 {
		return rbac.Scope{}, xerrors.New("no scopes provided")
	}

	var merged rbac.Scope
	merged.Role = rbac.Role{
		// Identifier is informational; not used in policy evaluation.
		Identifier: rbac.RoleIdentifier{Name: "Scope_Multiple"},
		Site:       nil,
		User:       nil,
		ByOrgID:    map[string]rbac.OrgPermissions{},
	}

	// Collect allow lists for a union after expanding all scopes.
	allowLists := make([][]rbac.AllowListElement, 0, len(s))

	for _, s := range s {
		expanded, err := s.ToRBAC().Expand()
		if err != nil {
			return rbac.Scope{}, err
		}

		// Merge role permissions: union by simple concatenation.
		merged.Site = append(merged.Site, expanded.Site...)
		for orgID, perms := range expanded.ByOrgID {
			orgPerms := merged.ByOrgID[orgID]
			orgPerms.Org = append(orgPerms.Org, perms.Org...)
			orgPerms.Member = append(orgPerms.Member, perms.Member...)
			merged.ByOrgID[orgID] = orgPerms
		}
		merged.User = append(merged.User, expanded.User...)

		allowLists = append(allowLists, expanded.AllowIDList)
	}

	// De-duplicate permissions across Site/Org/User
	merged.Site = rbac.DeduplicatePermissions(merged.Site)
	merged.User = rbac.DeduplicatePermissions(merged.User)
	for orgID, perms := range merged.ByOrgID {
		perms.Org = rbac.DeduplicatePermissions(perms.Org)
		perms.Member = rbac.DeduplicatePermissions(perms.Member)
		merged.ByOrgID[orgID] = perms
	}

	union, err := rbac.UnionAllowLists(allowLists...)
	if err != nil {
		return rbac.Scope{}, err
	}
	merged.AllowIDList = union

	return merged, nil
}

// Name returns a human-friendly identifier for tracing/logging.
func (s APIKeyScopes) Name() rbac.RoleIdentifier {
	if len(s) == 0 {
		// Return all for backward compatibility.
		return rbac.RoleIdentifier{Name: string(ApiKeyScopeCoderAll)}
	}
	names := make([]string, 0, len(s))
	for _, s := range s {
		names = append(names, string(s))
	}
	return rbac.RoleIdentifier{Name: "scopes[" + strings.Join(names, "+") + "]"}
}

// APIKeyScopeSet merges expanded scopes with the API key's DB allow_list. If
// the DB allow_list is a wildcard or empty, the merged scope's allow list is
// unchanged. Otherwise, the DB allow_list overrides the merged AllowIDList to
// enforce the token's resource scoping consistently across all permissions.
type APIKeyScopeSet struct {
	Scopes    APIKeyScopes
	AllowList AllowList
}

var _ rbac.ExpandableScope = APIKeyScopeSet{}

func (s APIKeyScopeSet) Name() rbac.RoleIdentifier { return s.Scopes.Name() }

func (s APIKeyScopeSet) Expand() (rbac.Scope, error) {
	merged, err := s.Scopes.expandRBACScope()
	if err != nil {
		return rbac.Scope{}, err
	}
	merged.AllowIDList = rbac.IntersectAllowLists(merged.AllowIDList, s.AllowList)
	return merged, nil
}

// ScopeSet returns the scopes combined with the database allow list. It is the
// canonical way to expose an API key's effective scope for authorization.
func (k APIKey) ScopeSet() APIKeyScopeSet {
	return APIKeyScopeSet{
		Scopes:    k.Scopes,
		AllowList: k.AllowList,
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
	// #nosec G115 - Safe conversion for AutostartBlockDaysOfWeek which is 7 bits
	return ^uint8(t.AutostartBlockDaysOfWeek) & 0b01111111
}

func (TemplateVersion) RBACObject(template Template) rbac.Object {
	// Just use the parent template resource for controlling versions
	return template.RBACObject()
}

func (i InboxAlert) RBACObject() rbac.Object {
	return rbac.ResourceInboxAlert.
		WithID(i.ID).
		WithOwner(i.UserID.String())
}

// RBACObjectNoTemplate is for orphaned template versions.
func (v TemplateVersion) RBACObjectNoTemplate() rbac.Object {
	return rbac.ResourceTemplate.InOrg(v.OrganizationID)
}

func (g Group) RBACObject() rbac.Object {
	return rbac.ResourceGroup.WithID(g.ID).
		InOrg(g.OrganizationID).
		// Group members can read the group.
		WithGroupACL(map[string][]policy.Action{
			g.ID.String(): {
				policy.ActionRead,
			},
		})
}

func (g GetGroupsRow) RBACObject() rbac.Object {
	return g.Group.RBACObject()
}

func (gm GroupMember) RBACObject() rbac.Object {
	return rbac.ResourceGroupMember.WithID(gm.UserID).InOrg(gm.OrganizationID).WithOwner(gm.UserID.String())
}

// PrebuiltWorkspaceResource defines the interface for types that can be identified as prebuilt workspaces
// and converted to their corresponding prebuilt workspace RBAC object.
type PrebuiltWorkspaceResource interface {
	IsPrebuild() bool
	AsPrebuild() rbac.Object
}

// WorkspaceTable converts a Workspace to it's reduced version.
// A more generalized solution is to use json marshaling to
// consistently keep these two structs in sync.
// That would be a lot of overhead, and a more costly unit test is
// written to make sure these match up.
func (w Workspace) WorkspaceTable() WorkspaceTable {
	return WorkspaceTable{
		ID:                w.ID,
		CreatedAt:         w.CreatedAt,
		UpdatedAt:         w.UpdatedAt,
		OwnerID:           w.OwnerID,
		OrganizationID:    w.OrganizationID,
		TemplateID:        w.TemplateID,
		Deleted:           w.Deleted,
		Name:              w.Name,
		AutostartSchedule: w.AutostartSchedule,
		Ttl:               w.Ttl,
		LastUsedAt:        w.LastUsedAt,
		DormantAt:         w.DormantAt,
		DeletingAt:        w.DeletingAt,
		AutomaticUpdates:  w.AutomaticUpdates,
		Favorite:          w.Favorite,
		NextStartAt:       w.NextStartAt,
		GroupACL:          w.GroupACL,
		UserACL:           w.UserACL,
	}
}

func (w Workspace) RBACObject() rbac.Object {
	return w.WorkspaceTable().RBACObject()
}

// IsPrebuild returns true if the workspace is a prebuild workspace.
// A workspace is considered a prebuild if its owner is the prebuild system user.
func (w Workspace) IsPrebuild() bool {
	return w.OwnerID == PrebuildsSystemUserID
}

// AsPrebuild returns the RBAC object corresponding to the workspace type.
// If the workspace is a prebuild, it returns a prebuilt_workspace RBAC object.
// Otherwise, it returns a normal workspace RBAC object.
func (w Workspace) AsPrebuild() rbac.Object {
	if w.IsPrebuild() {
		return rbac.ResourcePrebuiltWorkspace.WithID(w.ID).
			InOrg(w.OrganizationID).
			WithOwner(w.OwnerID.String())
	}
	return w.RBACObject()
}

func (w WorkspaceTable) RBACObject() rbac.Object {
	if w.DormantAt.Valid {
		return w.DormantRBAC()
	}

	return rbac.ResourceWorkspace.WithID(w.ID).
		InOrg(w.OrganizationID).
		WithOwner(w.OwnerID.String()).
		WithGroupACL(w.GroupACL.RBACACL()).
		WithACLUserList(w.UserACL.RBACACL())
}

func (w WorkspaceTable) DormantRBAC() rbac.Object {
	return rbac.ResourceWorkspaceDormant.
		WithID(w.ID).
		InOrg(w.OrganizationID).
		WithOwner(w.OwnerID.String())
}

// IsPrebuild returns true if the workspace is a prebuild workspace.
// A workspace is considered a prebuild if its owner is the prebuild system user.
func (w WorkspaceTable) IsPrebuild() bool {
	return w.OwnerID == PrebuildsSystemUserID
}

// AsPrebuild returns the RBAC object corresponding to the workspace type.
// If the workspace is a prebuild, it returns a prebuilt_workspace RBAC object.
// Otherwise, it returns a normal workspace RBAC object.
func (w WorkspaceTable) AsPrebuild() rbac.Object {
	if w.IsPrebuild() {
		return rbac.ResourcePrebuiltWorkspace.WithID(w.ID).
			InOrg(w.OrganizationID).
			WithOwner(w.OwnerID.String())
	}
	return w.RBACObject()
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

func (m PaginatedOrganizationMembersRow) RBACObject() rbac.Object {
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
	return rbac.ResourceProvisionerDaemon.
		WithID(p.ID).
		InOrg(p.OrganizationID)
}

func (p GetProvisionerDaemonsWithStatusByOrganizationRow) RBACObject() rbac.Object {
	return p.ProvisionerDaemon.RBACObject()
}

func (p GetEligibleProvisionerDaemonsByProvisionerJobIDsRow) RBACObject() rbac.Object {
	return p.ProvisionerDaemon.RBACObject()
}

// RBACObject for a provisioner key is the same as a provisioner daemon.
// Keys == provisioners from a RBAC perspective.
func (p ProvisionerKey) RBACObject() rbac.Object {
	return rbac.ResourceProvisionerDaemon.
		WithID(p.ID).
		InOrg(p.OrganizationID)
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

func (t OAuth2ProviderAppToken) RBACObject() rbac.Object {
	return rbac.ResourceOauth2AppCodeToken.WithOwner(t.UserID.String()).WithID(t.ID)
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
			ID:             r.ID,
			Email:          r.Email,
			Username:       r.Username,
			Name:           r.Name,
			HashedPassword: r.HashedPassword,
			CreatedAt:      r.CreatedAt,
			UpdatedAt:      r.UpdatedAt,
			Status:         r.Status,
			RBACRoles:      r.RBACRoles,
			LoginType:      r.LoginType,
			AvatarURL:      r.AvatarURL,
			Deleted:        r.Deleted,
			LastSeenAt:     r.LastSeenAt,
			IsSystem:       r.IsSystem,
		}
	}

	return users
}

func ConvertWorkspaceRows(rows []GetWorkspacesRow) []Workspace {
	workspaces := make([]Workspace, len(rows))
	for i, r := range rows {
		workspaces[i] = Workspace{
			ID:                      r.ID,
			CreatedAt:               r.CreatedAt,
			UpdatedAt:               r.UpdatedAt,
			OwnerID:                 r.OwnerID,
			OrganizationID:          r.OrganizationID,
			TemplateID:              r.TemplateID,
			Deleted:                 r.Deleted,
			Name:                    r.Name,
			AutostartSchedule:       r.AutostartSchedule,
			Ttl:                     r.Ttl,
			LastUsedAt:              r.LastUsedAt,
			DormantAt:               r.DormantAt,
			DeletingAt:              r.DeletingAt,
			AutomaticUpdates:        r.AutomaticUpdates,
			Favorite:                r.Favorite,
			OwnerAvatarUrl:          r.OwnerAvatarUrl,
			OwnerUsername:           r.OwnerUsername,
			OrganizationName:        r.OrganizationName,
			OrganizationDisplayName: r.OrganizationDisplayName,
			OrganizationIcon:        r.OrganizationIcon,
			OrganizationDescription: r.OrganizationDescription,
			TemplateName:            r.TemplateName,
			TemplateDisplayName:     r.TemplateDisplayName,
			TemplateIcon:            r.TemplateIcon,
			TemplateDescription:     r.TemplateDescription,
			NextStartAt:             r.NextStartAt,
			TaskID:                  r.TaskID,
		}
	}

	return workspaces
}

func (g Group) IsEveryone() bool {
	return g.ID == g.OrganizationID
}

func (p ProvisionerJob) RBACObject() rbac.Object {
	switch p.Type {
	// Only acceptable for known job types at this time because template
	// admins may not be allowed to view new types.
	case ProvisionerJobTypeTemplateVersionImport, ProvisionerJobTypeTemplateVersionDryRun, ProvisionerJobTypeWorkspaceBuild:
		return rbac.ResourceProvisionerJobs.InOrg(p.OrganizationID)

	default:
		panic("developer error: unknown provisioner job type " + string(p.Type))
	}
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

func (k CryptoKey) ExpiresAt(keyDuration time.Duration) time.Time {
	return k.StartsAt.Add(keyDuration).UTC()
}

func (k CryptoKey) DecodeString() ([]byte, error) {
	return hex.DecodeString(k.Secret.String)
}

func (k CryptoKey) CanSign(now time.Time) bool {
	isAfterStart := !k.StartsAt.IsZero() && !now.Before(k.StartsAt)
	return isAfterStart && k.CanVerify(now)
}

func (k CryptoKey) CanVerify(now time.Time) bool {
	hasSecret := k.Secret.Valid
	isBeforeDeletion := !k.DeletesAt.Valid || now.Before(k.DeletesAt.Time)
	return hasSecret && isBeforeDeletion
}

func (r GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerRow) RBACObject() rbac.Object {
	return r.ProvisionerJob.RBACObject()
}

func (m WorkspaceAgentMemoryResourceMonitor) Debounce(
	by time.Duration,
	now time.Time,
	oldState, newState WorkspaceAgentMonitorState,
) (time.Time, bool) {
	if now.After(m.DebouncedUntil) &&
		oldState == WorkspaceAgentMonitorStateOK &&
		newState == WorkspaceAgentMonitorStateNOK {
		return now.Add(by), true
	}

	return m.DebouncedUntil, false
}

func (m WorkspaceAgentVolumeResourceMonitor) Debounce(
	by time.Duration,
	now time.Time,
	oldState, newState WorkspaceAgentMonitorState,
) (debouncedUntil time.Time, shouldNotify bool) {
	if now.After(m.DebouncedUntil) &&
		oldState == WorkspaceAgentMonitorStateOK &&
		newState == WorkspaceAgentMonitorStateNOK {
		return now.Add(by), true
	}

	return m.DebouncedUntil, false
}

func (s UserSecret) RBACObject() rbac.Object {
	return rbac.ResourceUserSecret.WithID(s.ID).WithOwner(s.UserID.String())
}

func (s AIBridgeInterception) RBACObject() rbac.Object {
	return rbac.ResourceAibridgeInterception.WithOwner(s.InitiatorID.String())
}

// WorkspaceIdentity contains the minimal workspace fields needed for agent API metadata/stats reporting
// and RBAC checks, without requiring a full database.Workspace object.
type WorkspaceIdentity struct {
	// Add any other fields needed for IsPrebuild() if it relies on workspace fields
	// Identity fields
	ID             uuid.UUID
	OwnerID        uuid.UUID
	OrganizationID uuid.UUID
	TemplateID     uuid.UUID

	// Display fields for logging/metrics
	Name          string
	OwnerUsername string
	TemplateName  string

	// Lifecycle fields needed for stats reporting
	AutostartSchedule sql.NullString
}

func (w WorkspaceIdentity) RBACObject() rbac.Object {
	return Workspace{
		ID:                w.ID,
		OwnerID:           w.OwnerID,
		OrganizationID:    w.OrganizationID,
		TemplateID:        w.TemplateID,
		Name:              w.Name,
		OwnerUsername:     w.OwnerUsername,
		TemplateName:      w.TemplateName,
		AutostartSchedule: w.AutostartSchedule,
	}.RBACObject()
}

// IsPrebuild returns true if the workspace is a prebuild workspace.
// A workspace is considered a prebuild if its owner is the prebuild system user.
func (w WorkspaceIdentity) IsPrebuild() bool {
	return w.OwnerID == PrebuildsSystemUserID
}

func (w WorkspaceIdentity) Equal(w2 WorkspaceIdentity) bool {
	return w.ID == w2.ID && w.OwnerID == w2.OwnerID && w.OrganizationID == w2.OrganizationID &&
		w.TemplateID == w2.TemplateID && w.Name == w2.Name && w.OwnerUsername == w2.OwnerUsername &&
		w.TemplateName == w2.TemplateName && w.AutostartSchedule == w2.AutostartSchedule
}

func WorkspaceIdentityFromWorkspace(w Workspace) WorkspaceIdentity {
	return WorkspaceIdentity{
		ID:                w.ID,
		OwnerID:           w.OwnerID,
		OrganizationID:    w.OrganizationID,
		TemplateID:        w.TemplateID,
		Name:              w.Name,
		OwnerUsername:     w.OwnerUsername,
		TemplateName:      w.TemplateName,
		AutostartSchedule: w.AutostartSchedule,
	}
}
