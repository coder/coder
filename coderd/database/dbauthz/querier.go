package dbauthz

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

func (q *querier) Ping(ctx context.Context) (time.Duration, error) {
	return q.db.Ping(ctx)
}

// InTx runs the given function in a transaction.
func (q *querier) InTx(function func(querier database.Store) error, txOpts *sql.TxOptions) error {
	return q.db.InTx(func(tx database.Store) error {
		// Wrap the transaction store in a querier.
		wrapped := New(tx, q.auth, q.log)
		return function(wrapped)
	}, txOpts)
}

func (q *querier) DeleteAPIKeyByID(ctx context.Context, id string) error {
	return deleteQ(q.log, q.auth, q.db.GetAPIKeyByID, q.db.DeleteAPIKeyByID)(ctx, id)
}

func (q *querier) GetAPIKeyByID(ctx context.Context, id string) (database.APIKey, error) {
	return fetch(q.log, q.auth, q.db.GetAPIKeyByID)(ctx, id)
}

func (q *querier) GetAPIKeysByLoginType(ctx context.Context, loginType database.LoginType) ([]database.APIKey, error) {
	return fetchWithPostFilter(q.auth, q.db.GetAPIKeysByLoginType)(ctx, loginType)
}

func (q *querier) GetAPIKeysLastUsedAfter(ctx context.Context, lastUsed time.Time) ([]database.APIKey, error) {
	return fetchWithPostFilter(q.auth, q.db.GetAPIKeysLastUsedAfter)(ctx, lastUsed)
}

func (q *querier) InsertAPIKey(ctx context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	return insert(q.log, q.auth,
		rbac.ResourceAPIKey.WithOwner(arg.UserID.String()),
		q.db.InsertAPIKey)(ctx, arg)
}

func (q *querier) UpdateAPIKeyByID(ctx context.Context, arg database.UpdateAPIKeyByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateAPIKeyByIDParams) (database.APIKey, error) {
		return q.db.GetAPIKeyByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateAPIKeyByID)(ctx, arg)
}

func (q *querier) InsertAuditLog(ctx context.Context, arg database.InsertAuditLogParams) (database.AuditLog, error) {
	return insert(q.log, q.auth, rbac.ResourceAuditLog, q.db.InsertAuditLog)(ctx, arg)
}

func (q *querier) GetAuditLogsOffset(ctx context.Context, arg database.GetAuditLogsOffsetParams) ([]database.GetAuditLogsOffsetRow, error) {
	// To optimize audit logs, we only check the global audit log permission once.
	// This is because we expect a large unbounded set of audit logs, and applying a SQL
	// filter would slow down the query for no benefit.
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceAuditLog); err != nil {
		return nil, err
	}
	return q.db.GetAuditLogsOffset(ctx, arg)
}

func (q *querier) GetFileByHashAndCreator(ctx context.Context, arg database.GetFileByHashAndCreatorParams) (database.File, error) {
	return fetch(q.log, q.auth, q.db.GetFileByHashAndCreator)(ctx, arg)
}

func (q *querier) GetFileByID(ctx context.Context, id uuid.UUID) (database.File, error) {
	return fetch(q.log, q.auth, q.db.GetFileByID)(ctx, id)
}

func (q *querier) InsertFile(ctx context.Context, arg database.InsertFileParams) (database.File, error) {
	return insert(q.log, q.auth, rbac.ResourceFile.WithOwner(arg.CreatedBy.String()), q.db.InsertFile)(ctx, arg)
}

func (q *querier) DeleteGroupByID(ctx context.Context, id uuid.UUID) error {
	return deleteQ(q.log, q.auth, q.db.GetGroupByID, q.db.DeleteGroupByID)(ctx, id)
}

func (q *querier) DeleteGroupMemberFromGroup(ctx context.Context, arg database.DeleteGroupMemberFromGroupParams) error {
	// Deleting a group member counts as updating a group.
	fetch := func(ctx context.Context, arg database.DeleteGroupMemberFromGroupParams) (database.Group, error) {
		return q.db.GetGroupByID(ctx, arg.GroupID)
	}
	return update(q.log, q.auth, fetch, q.db.DeleteGroupMemberFromGroup)(ctx, arg)
}

func (q *querier) InsertUserGroupsByName(ctx context.Context, arg database.InsertUserGroupsByNameParams) error {
	// This will add the user to all named groups. This counts as updating a group.
	// NOTE: instead of checking if the user has permission to update each group, we instead
	// check if the user has permission to update *a* group in the org.
	fetch := func(ctx context.Context, arg database.InsertUserGroupsByNameParams) (rbac.Objecter, error) {
		return rbac.ResourceGroup.InOrg(arg.OrganizationID), nil
	}
	return update(q.log, q.auth, fetch, q.db.InsertUserGroupsByName)(ctx, arg)
}

func (q *querier) DeleteGroupMembersByOrgAndUser(ctx context.Context, arg database.DeleteGroupMembersByOrgAndUserParams) error {
	// This will remove the user from all groups in the org. This counts as updating a group.
	// NOTE: instead of fetching all groups in the org with arg.UserID as a member, we instead
	// check if the caller has permission to update any group in the org.
	fetch := func(ctx context.Context, arg database.DeleteGroupMembersByOrgAndUserParams) (rbac.Objecter, error) {
		return rbac.ResourceGroup.InOrg(arg.OrganizationID), nil
	}
	return update(q.log, q.auth, fetch, q.db.DeleteGroupMembersByOrgAndUser)(ctx, arg)
}

func (q *querier) GetGroupByID(ctx context.Context, id uuid.UUID) (database.Group, error) {
	return fetch(q.log, q.auth, q.db.GetGroupByID)(ctx, id)
}

func (q *querier) GetGroupByOrgAndName(ctx context.Context, arg database.GetGroupByOrgAndNameParams) (database.Group, error) {
	return fetch(q.log, q.auth, q.db.GetGroupByOrgAndName)(ctx, arg)
}

func (q *querier) GetGroupMembers(ctx context.Context, groupID uuid.UUID) ([]database.User, error) {
	if _, err := q.GetGroupByID(ctx, groupID); err != nil { // AuthZ check
		return nil, err
	}
	return q.db.GetGroupMembers(ctx, groupID)
}

func (q *querier) InsertAllUsersGroup(ctx context.Context, organizationID uuid.UUID) (database.Group, error) {
	// This method creates a new group.
	return insert(q.log, q.auth, rbac.ResourceGroup.InOrg(organizationID), q.db.InsertAllUsersGroup)(ctx, organizationID)
}

func (q *querier) InsertGroup(ctx context.Context, arg database.InsertGroupParams) (database.Group, error) {
	return insert(q.log, q.auth, rbac.ResourceGroup.InOrg(arg.OrganizationID), q.db.InsertGroup)(ctx, arg)
}

func (q *querier) InsertGroupMember(ctx context.Context, arg database.InsertGroupMemberParams) error {
	fetch := func(ctx context.Context, arg database.InsertGroupMemberParams) (database.Group, error) {
		return q.db.GetGroupByID(ctx, arg.GroupID)
	}
	return update(q.log, q.auth, fetch, q.db.InsertGroupMember)(ctx, arg)
}

func (q *querier) UpdateGroupByID(ctx context.Context, arg database.UpdateGroupByIDParams) (database.Group, error) {
	fetch := func(ctx context.Context, arg database.UpdateGroupByIDParams) (database.Group, error) {
		return q.db.GetGroupByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateGroupByID)(ctx, arg)
}

func (q *querier) UpdateProvisionerJobWithCancelByID(ctx context.Context, arg database.UpdateProvisionerJobWithCancelByIDParams) error {
	job, err := q.db.GetProvisionerJobByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	switch job.Type {
	case database.ProvisionerJobTypeWorkspaceBuild:
		build, err := q.db.GetWorkspaceBuildByJobID(ctx, arg.ID)
		if err != nil {
			return err
		}
		workspace, err := q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
		if err != nil {
			return err
		}

		template, err := q.db.GetTemplateByID(ctx, workspace.TemplateID)
		if err != nil {
			return err
		}

		// Template can specify if cancels are allowed.
		// Would be nice to have a way in the rbac rego to do this.
		if !template.AllowUserCancelWorkspaceJobs {
			// Only owners can cancel workspace builds
			actor, ok := ActorFromContext(ctx)
			if !ok {
				return NoActorError
			}
			if !slice.Contains(actor.Roles.Names(), rbac.RoleOwner()) {
				return xerrors.Errorf("only owners can cancel workspace builds")
			}
		}

		err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace)
		if err != nil {
			return err
		}
	case database.ProvisionerJobTypeTemplateVersionDryRun, database.ProvisionerJobTypeTemplateVersionImport:
		// Authorized call to get template version.
		templateVersion, err := authorizedTemplateVersionFromJob(ctx, q, job)
		if err != nil {
			return err
		}

		if templateVersion.TemplateID.Valid {
			template, err := q.db.GetTemplateByID(ctx, templateVersion.TemplateID.UUID)
			if err != nil {
				return err
			}
			err = q.authorizeContext(ctx, rbac.ActionUpdate, templateVersion.RBACObject(template))
			if err != nil {
				return err
			}
		} else {
			err = q.authorizeContext(ctx, rbac.ActionUpdate, templateVersion.RBACObjectNoTemplate())
			if err != nil {
				return err
			}
		}
	default:
		return xerrors.Errorf("unknown job type: %q", job.Type)
	}
	return q.db.UpdateProvisionerJobWithCancelByID(ctx, arg)
}

func (q *querier) GetProvisionerJobByID(ctx context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
	job, err := q.db.GetProvisionerJobByID(ctx, id)
	if err != nil {
		return database.ProvisionerJob{}, err
	}

	switch job.Type {
	case database.ProvisionerJobTypeWorkspaceBuild:
		// Authorized call to get workspace build. If we can read the build, we
		// can read the job.
		_, err := q.GetWorkspaceBuildByJobID(ctx, id)
		if err != nil {
			return database.ProvisionerJob{}, err
		}
	case database.ProvisionerJobTypeTemplateVersionDryRun, database.ProvisionerJobTypeTemplateVersionImport:
		// Authorized call to get template version.
		_, err := authorizedTemplateVersionFromJob(ctx, q, job)
		if err != nil {
			return database.ProvisionerJob{}, err
		}
	default:
		return database.ProvisionerJob{}, xerrors.Errorf("unknown job type: %q", job.Type)
	}

	return job, nil
}

func (q *querier) GetProvisionerJobsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.ProvisionerJob, error) {
	// TODO: This is missing authorization and is incorrect. This call is used by telemetry, and by 1 http route.
	// That http handler should find a better way to fetch these jobs with easier rbac authz.
	return q.db.GetProvisionerJobsByIDs(ctx, ids)
}

func (q *querier) GetProvisionerLogsByIDBetween(ctx context.Context, arg database.GetProvisionerLogsByIDBetweenParams) ([]database.ProvisionerJobLog, error) {
	// Authorized read on job lets the actor also read the logs.
	_, err := q.GetProvisionerJobByID(ctx, arg.JobID)
	if err != nil {
		return nil, err
	}
	return q.db.GetProvisionerLogsByIDBetween(ctx, arg)
}

func (q *querier) GetLicenses(ctx context.Context) ([]database.License, error) {
	fetch := func(ctx context.Context, _ interface{}) ([]database.License, error) {
		return q.db.GetLicenses(ctx)
	}
	return fetchWithPostFilter(q.auth, fetch)(ctx, nil)
}

func (q *querier) InsertLicense(ctx context.Context, arg database.InsertLicenseParams) (database.License, error) {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceLicense); err != nil {
		return database.License{}, err
	}
	return q.db.InsertLicense(ctx, arg)
}

func (q *querier) InsertOrUpdateLogoURL(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceDeploymentConfig); err != nil {
		return err
	}
	return q.db.InsertOrUpdateLogoURL(ctx, value)
}

func (q *querier) InsertOrUpdateServiceBanner(ctx context.Context, value string) error {
	if err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceDeploymentConfig); err != nil {
		return err
	}
	return q.db.InsertOrUpdateServiceBanner(ctx, value)
}

func (q *querier) GetLicenseByID(ctx context.Context, id int32) (database.License, error) {
	return fetch(q.log, q.auth, q.db.GetLicenseByID)(ctx, id)
}

func (q *querier) DeleteLicense(ctx context.Context, id int32) (int32, error) {
	err := deleteQ(q.log, q.auth, q.db.GetLicenseByID, func(ctx context.Context, id int32) error {
		_, err := q.db.DeleteLicense(ctx, id)
		return err
	})(ctx, id)
	if err != nil {
		return -1, err
	}
	return id, nil
}

func (q *querier) GetDeploymentID(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetDeploymentID(ctx)
}

func (q *querier) GetLogoURL(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetLogoURL(ctx)
}

func (q *querier) GetServiceBanner(ctx context.Context) (string, error) {
	// No authz checks
	return q.db.GetServiceBanner(ctx)
}

func (q *querier) GetProvisionerDaemons(ctx context.Context) ([]database.ProvisionerDaemon, error) {
	fetch := func(ctx context.Context, _ interface{}) ([]database.ProvisionerDaemon, error) {
		return q.db.GetProvisionerDaemons(ctx)
	}
	return fetchWithPostFilter(q.auth, fetch)(ctx, nil)
}

func (q *querier) GetGroupsByOrganizationID(ctx context.Context, organizationID uuid.UUID) ([]database.Group, error) {
	return fetchWithPostFilter(q.auth, q.db.GetGroupsByOrganizationID)(ctx, organizationID)
}

func (q *querier) GetOrganizationByID(ctx context.Context, id uuid.UUID) (database.Organization, error) {
	return fetch(q.log, q.auth, q.db.GetOrganizationByID)(ctx, id)
}

func (q *querier) GetOrganizationByName(ctx context.Context, name string) (database.Organization, error) {
	return fetch(q.log, q.auth, q.db.GetOrganizationByName)(ctx, name)
}

func (q *querier) GetOrganizationIDsByMemberIDs(ctx context.Context, ids []uuid.UUID) ([]database.GetOrganizationIDsByMemberIDsRow, error) {
	// TODO: This should be rewritten to return a list of database.OrganizationMember for consistent RBAC objects.
	// Currently this row returns a list of org ids per user, which is challenging to check against the RBAC system.
	return fetchWithPostFilter(q.auth, q.db.GetOrganizationIDsByMemberIDs)(ctx, ids)
}

func (q *querier) GetOrganizationMemberByUserID(ctx context.Context, arg database.GetOrganizationMemberByUserIDParams) (database.OrganizationMember, error) {
	return fetch(q.log, q.auth, q.db.GetOrganizationMemberByUserID)(ctx, arg)
}

func (q *querier) GetOrganizationMembershipsByUserID(ctx context.Context, userID uuid.UUID) ([]database.OrganizationMember, error) {
	return fetchWithPostFilter(q.auth, q.db.GetOrganizationMembershipsByUserID)(ctx, userID)
}

func (q *querier) GetOrganizations(ctx context.Context) ([]database.Organization, error) {
	fetch := func(ctx context.Context, _ interface{}) ([]database.Organization, error) {
		return q.db.GetOrganizations(ctx)
	}
	return fetchWithPostFilter(q.auth, fetch)(ctx, nil)
}

func (q *querier) GetOrganizationsByUserID(ctx context.Context, userID uuid.UUID) ([]database.Organization, error) {
	return fetchWithPostFilter(q.auth, q.db.GetOrganizationsByUserID)(ctx, userID)
}

func (q *querier) InsertOrganization(ctx context.Context, arg database.InsertOrganizationParams) (database.Organization, error) {
	return insert(q.log, q.auth, rbac.ResourceOrganization, q.db.InsertOrganization)(ctx, arg)
}

func (q *querier) InsertOrganizationMember(ctx context.Context, arg database.InsertOrganizationMemberParams) (database.OrganizationMember, error) {
	// All roles are added roles. Org member is always implied.
	addedRoles := append(arg.Roles, rbac.RoleOrgMember(arg.OrganizationID))
	err := q.canAssignRoles(ctx, &arg.OrganizationID, addedRoles, []string{})
	if err != nil {
		return database.OrganizationMember{}, err
	}

	obj := rbac.ResourceOrganizationMember.InOrg(arg.OrganizationID).WithID(arg.UserID)
	return insert(q.log, q.auth, obj, q.db.InsertOrganizationMember)(ctx, arg)
}

func (q *querier) UpdateMemberRoles(ctx context.Context, arg database.UpdateMemberRolesParams) (database.OrganizationMember, error) {
	// Authorized fetch will check that the actor has read access to the org member since the org member is returned.
	member, err := q.GetOrganizationMemberByUserID(ctx, database.GetOrganizationMemberByUserIDParams{
		OrganizationID: arg.OrgID,
		UserID:         arg.UserID,
	})
	if err != nil {
		return database.OrganizationMember{}, err
	}

	// The org member role is always implied.
	impliedTypes := append(arg.GrantedRoles, rbac.RoleOrgMember(arg.OrgID))
	added, removed := rbac.ChangeRoleSet(member.Roles, impliedTypes)
	err = q.canAssignRoles(ctx, &arg.OrgID, added, removed)
	if err != nil {
		return database.OrganizationMember{}, err
	}

	return q.db.UpdateMemberRoles(ctx, arg)
}

func (q *querier) canAssignRoles(ctx context.Context, orgID *uuid.UUID, added, removed []string) error {
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return NoActorError
	}

	roleAssign := rbac.ResourceRoleAssignment
	shouldBeOrgRoles := false
	if orgID != nil {
		roleAssign = roleAssign.InOrg(*orgID)
		shouldBeOrgRoles = true
	}

	grantedRoles := append(added, removed...)
	// Validate that the roles being assigned are valid.
	for _, r := range grantedRoles {
		_, isOrgRole := rbac.IsOrgRole(r)
		if shouldBeOrgRoles && !isOrgRole {
			return xerrors.Errorf("Must only update org roles")
		}
		if !shouldBeOrgRoles && isOrgRole {
			return xerrors.Errorf("Must only update site wide roles")
		}

		// All roles should be valid roles
		if _, err := rbac.RoleByName(r); err != nil {
			return xerrors.Errorf("%q is not a supported role", r)
		}
	}

	if len(added) > 0 {
		if err := q.authorizeContext(ctx, rbac.ActionCreate, roleAssign); err != nil {
			return err
		}
	}

	if len(removed) > 0 {
		if err := q.authorizeContext(ctx, rbac.ActionDelete, roleAssign); err != nil {
			return err
		}
	}

	for _, roleName := range grantedRoles {
		if !rbac.CanAssignRole(actor.Roles, roleName) {
			return xerrors.Errorf("not authorized to assign role %q", roleName)
		}
	}

	return nil
}

func (q *querier) parameterRBACResource(ctx context.Context, scope database.ParameterScope, scopeID uuid.UUID) (rbac.Objecter, error) {
	var resource rbac.Objecter
	var err error
	switch scope {
	case database.ParameterScopeWorkspace:
		return q.db.GetWorkspaceByID(ctx, scopeID)
	case database.ParameterScopeImportJob:
		var version database.TemplateVersion
		version, err = q.db.GetTemplateVersionByJobID(ctx, scopeID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		resource = version.RBACObjectNoTemplate()

		var template database.Template
		template, err = q.db.GetTemplateByID(ctx, version.TemplateID.UUID)
		if err == nil {
			resource = version.RBACObject(template)
		} else if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return resource, nil
	case database.ParameterScopeTemplate:
		return q.db.GetTemplateByID(ctx, scopeID)
	default:
		return nil, xerrors.Errorf("Parameter scope %q unsupported", scope)
	}
}

func (q *querier) InsertParameterValue(ctx context.Context, arg database.InsertParameterValueParams) (database.ParameterValue, error) {
	resource, err := q.parameterRBACResource(ctx, arg.Scope, arg.ScopeID)
	if err != nil {
		return database.ParameterValue{}, err
	}

	err = q.authorizeContext(ctx, rbac.ActionUpdate, resource)
	if err != nil {
		return database.ParameterValue{}, err
	}

	return q.db.InsertParameterValue(ctx, arg)
}

func (q *querier) ParameterValue(ctx context.Context, id uuid.UUID) (database.ParameterValue, error) {
	parameter, err := q.db.ParameterValue(ctx, id)
	if err != nil {
		return database.ParameterValue{}, err
	}

	resource, err := q.parameterRBACResource(ctx, parameter.Scope, parameter.ScopeID)
	if err != nil {
		return database.ParameterValue{}, err
	}

	err = q.authorizeContext(ctx, rbac.ActionRead, resource)
	if err != nil {
		return database.ParameterValue{}, err
	}

	return parameter, nil
}

// ParameterValues is implemented as an all or nothing query. If the user is not
// able to read a single parameter value, then the entire query is denied.
// This should likely be revisited and see if the usage of this function cannot be changed.
func (q *querier) ParameterValues(ctx context.Context, arg database.ParameterValuesParams) ([]database.ParameterValue, error) {
	// This is a bit of a special case. Each parameter value returned might have a different scope. This could likely
	// be implemented in a more efficient manner.
	values, err := q.db.ParameterValues(ctx, arg)
	if err != nil {
		return nil, err
	}

	cached := make(map[uuid.UUID]bool)
	for _, value := range values {
		// If we already checked this scopeID, then we can skip it.
		// All scope ids are uuids of objects and universally unique.
		if allowed := cached[value.ScopeID]; allowed {
			continue
		}
		rbacObj, err := q.parameterRBACResource(ctx, value.Scope, value.ScopeID)
		if err != nil {
			return nil, err
		}
		err = q.authorizeContext(ctx, rbac.ActionRead, rbacObj)
		if err != nil {
			return nil, err
		}
		cached[value.ScopeID] = true
	}

	return values, nil
}

func (q *querier) GetParameterSchemasByJobID(ctx context.Context, jobID uuid.UUID) ([]database.ParameterSchema, error) {
	version, err := q.db.GetTemplateVersionByJobID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	object := version.RBACObjectNoTemplate()
	if version.TemplateID.Valid {
		tpl, err := q.db.GetTemplateByID(ctx, version.TemplateID.UUID)
		if err != nil {
			return nil, err
		}
		object = version.RBACObject(tpl)
	}

	err = q.authorizeContext(ctx, rbac.ActionRead, object)
	if err != nil {
		return nil, err
	}
	return q.db.GetParameterSchemasByJobID(ctx, jobID)
}

func (q *querier) GetParameterValueByScopeAndName(ctx context.Context, arg database.GetParameterValueByScopeAndNameParams) (database.ParameterValue, error) {
	resource, err := q.parameterRBACResource(ctx, arg.Scope, arg.ScopeID)
	if err != nil {
		return database.ParameterValue{}, err
	}

	err = q.authorizeContext(ctx, rbac.ActionRead, resource)
	if err != nil {
		return database.ParameterValue{}, err
	}

	return q.db.GetParameterValueByScopeAndName(ctx, arg)
}

func (q *querier) DeleteParameterValueByID(ctx context.Context, id uuid.UUID) error {
	parameter, err := q.db.ParameterValue(ctx, id)
	if err != nil {
		return err
	}

	resource, err := q.parameterRBACResource(ctx, parameter.Scope, parameter.ScopeID)
	if err != nil {
		return err
	}

	// A deleted param is still updating the underlying resource for the scope.
	err = q.authorizeContext(ctx, rbac.ActionUpdate, resource)
	if err != nil {
		return err
	}

	return q.db.DeleteParameterValueByID(ctx, id)
}

func (q *querier) GetPreviousTemplateVersion(ctx context.Context, arg database.GetPreviousTemplateVersionParams) (database.TemplateVersion, error) {
	// An actor can read the previous template version if they can read the related template.
	// If no linked template exists, we check if the actor can read *a* template.
	if !arg.TemplateID.Valid {
		if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTemplate.InOrg(arg.OrganizationID)); err != nil {
			return database.TemplateVersion{}, err
		}
	}
	if _, err := q.GetTemplateByID(ctx, arg.TemplateID.UUID); err != nil {
		return database.TemplateVersion{}, err
	}
	return q.db.GetPreviousTemplateVersion(ctx, arg)
}

func (q *querier) GetTemplateByID(ctx context.Context, id uuid.UUID) (database.Template, error) {
	return fetch(q.log, q.auth, q.db.GetTemplateByID)(ctx, id)
}

func (q *querier) GetTemplateByOrganizationAndName(ctx context.Context, arg database.GetTemplateByOrganizationAndNameParams) (database.Template, error) {
	return fetch(q.log, q.auth, q.db.GetTemplateByOrganizationAndName)(ctx, arg)
}

func (q *querier) GetTemplateVersionByID(ctx context.Context, tvid uuid.UUID) (database.TemplateVersion, error) {
	tv, err := q.db.GetTemplateVersionByID(ctx, tvid)
	if err != nil {
		return database.TemplateVersion{}, err
	}
	if !tv.TemplateID.Valid {
		// If no linked template exists, check if the actor can read a template in the organization.
		if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTemplate.InOrg(tv.OrganizationID)); err != nil {
			return database.TemplateVersion{}, err
		}
	} else if _, err := q.GetTemplateByID(ctx, tv.TemplateID.UUID); err != nil {
		// An actor can read the template version if they can read the related template.
		return database.TemplateVersion{}, err
	}
	return tv, nil
}

func (q *querier) GetTemplateVersionByJobID(ctx context.Context, jobID uuid.UUID) (database.TemplateVersion, error) {
	tv, err := q.db.GetTemplateVersionByJobID(ctx, jobID)
	if err != nil {
		return database.TemplateVersion{}, err
	}
	if !tv.TemplateID.Valid {
		// If no linked template exists, check if the actor can read a template in the organization.
		if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTemplate.InOrg(tv.OrganizationID)); err != nil {
			return database.TemplateVersion{}, err
		}
	} else if _, err := q.GetTemplateByID(ctx, tv.TemplateID.UUID); err != nil {
		// An actor can read the template version if they can read the related template.
		return database.TemplateVersion{}, err
	}
	return tv, nil
}

func (q *querier) GetTemplateVersionByTemplateIDAndName(ctx context.Context, arg database.GetTemplateVersionByTemplateIDAndNameParams) (database.TemplateVersion, error) {
	tv, err := q.db.GetTemplateVersionByTemplateIDAndName(ctx, arg)
	if err != nil {
		return database.TemplateVersion{}, err
	}
	if !tv.TemplateID.Valid {
		// If no linked template exists, check if the actor can read a template in the organization.
		if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTemplate.InOrg(tv.OrganizationID)); err != nil {
			return database.TemplateVersion{}, err
		}
	} else if _, err := q.GetTemplateByID(ctx, tv.TemplateID.UUID); err != nil {
		// An actor can read the template version if they can read the related template.
		return database.TemplateVersion{}, err
	}
	return tv, nil
}

func (q *querier) GetTemplateVersionParameters(ctx context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionParameter, error) {
	// An actor can read template version parameters if they can read the related template.
	tv, err := q.db.GetTemplateVersionByID(ctx, templateVersionID)
	if err != nil {
		return nil, err
	}

	var object rbac.Objecter
	template, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		object = rbac.ResourceTemplate.InOrg(tv.OrganizationID)
	} else {
		object = tv.RBACObject(template)
	}

	if err := q.authorizeContext(ctx, rbac.ActionRead, object); err != nil {
		return nil, err
	}
	return q.db.GetTemplateVersionParameters(ctx, templateVersionID)
}

func (q *querier) GetTemplateVersionVariables(ctx context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionVariable, error) {
	tv, err := q.db.GetTemplateVersionByID(ctx, templateVersionID)
	if err != nil {
		return nil, err
	}

	var object rbac.Objecter
	template, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		object = rbac.ResourceTemplate.InOrg(tv.OrganizationID)
	} else {
		object = tv.RBACObject(template)
	}

	if err := q.authorizeContext(ctx, rbac.ActionCreate, object); err != nil {
		return nil, err
	}
	return q.db.GetTemplateVersionVariables(ctx, templateVersionID)
}

func (q *querier) GetTemplateVersionsByIDs(ctx context.Context, ids []uuid.UUID) ([]database.TemplateVersion, error) {
	// TODO: This is so inefficient
	versions, err := q.db.GetTemplateVersionsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	checked := make(map[uuid.UUID]bool)
	for _, v := range versions {
		if _, ok := checked[v.TemplateID.UUID]; ok {
			continue
		}

		obj := v.RBACObjectNoTemplate()
		template, err := q.db.GetTemplateByID(ctx, v.TemplateID.UUID)
		if err == nil {
			obj = v.RBACObject(template)
		}
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err := q.authorizeContext(ctx, rbac.ActionRead, obj); err != nil {
			return nil, err
		}
		checked[v.TemplateID.UUID] = true
	}

	return versions, nil
}

func (q *querier) GetTemplateVersionsByTemplateID(ctx context.Context, arg database.GetTemplateVersionsByTemplateIDParams) ([]database.TemplateVersion, error) {
	// An actor can read template versions if they can read the related template.
	template, err := q.db.GetTemplateByID(ctx, arg.TemplateID)
	if err != nil {
		return nil, err
	}

	if err := q.authorizeContext(ctx, rbac.ActionRead, template); err != nil {
		return nil, err
	}

	return q.db.GetTemplateVersionsByTemplateID(ctx, arg)
}

func (q *querier) GetTemplateVersionsCreatedAfter(ctx context.Context, createdAt time.Time) ([]database.TemplateVersion, error) {
	// An actor can read execute this query if they can read all templates.
	if err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceTemplate.All()); err != nil {
		return nil, err
	}
	return q.db.GetTemplateVersionsCreatedAfter(ctx, createdAt)
}

func (q *querier) GetAuthorizedTemplates(ctx context.Context, arg database.GetTemplatesWithFilterParams, _ rbac.PreparedAuthorized) ([]database.Template, error) {
	// TODO Delete this function, all GetTemplates should be authorized. For now just call getTemplates on the authz querier.
	return q.GetTemplatesWithFilter(ctx, arg)
}

func (q *querier) GetTemplatesWithFilter(ctx context.Context, arg database.GetTemplatesWithFilterParams) ([]database.Template, error) {
	prep, err := prepareSQLFilter(ctx, q.auth, rbac.ActionRead, rbac.ResourceTemplate.Type)
	if err != nil {
		return nil, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	return q.db.GetAuthorizedTemplates(ctx, arg, prep)
}

func (q *querier) InsertTemplate(ctx context.Context, arg database.InsertTemplateParams) (database.Template, error) {
	obj := rbac.ResourceTemplate.InOrg(arg.OrganizationID)
	return insert(q.log, q.auth, obj, q.db.InsertTemplate)(ctx, arg)
}

func (q *querier) InsertTemplateVersion(ctx context.Context, arg database.InsertTemplateVersionParams) (database.TemplateVersion, error) {
	if !arg.TemplateID.Valid {
		// Making a new template version is the same permission as creating a new template.
		err := q.authorizeContext(ctx, rbac.ActionCreate, rbac.ResourceTemplate.InOrg(arg.OrganizationID))
		if err != nil {
			return database.TemplateVersion{}, err
		}
	} else {
		// Must do an authorized fetch to prevent leaking template ids this way.
		tpl, err := q.GetTemplateByID(ctx, arg.TemplateID.UUID)
		if err != nil {
			return database.TemplateVersion{}, err
		}
		// Check the create permission on the template.
		err = q.authorizeContext(ctx, rbac.ActionCreate, tpl)
		if err != nil {
			return database.TemplateVersion{}, err
		}
	}

	return q.db.InsertTemplateVersion(ctx, arg)
}

func (q *querier) UpdateTemplateACLByID(ctx context.Context, arg database.UpdateTemplateACLByIDParams) (database.Template, error) {
	// UpdateTemplateACL uses the ActionCreate action. Only users that can create the template
	// may update the ACL.
	fetch := func(ctx context.Context, arg database.UpdateTemplateACLByIDParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.ID)
	}
	return fetchAndQuery(q.log, q.auth, rbac.ActionCreate, fetch, q.db.UpdateTemplateACLByID)(ctx, arg)
}

func (q *querier) UpdateTemplateActiveVersionByID(ctx context.Context, arg database.UpdateTemplateActiveVersionByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateTemplateActiveVersionByIDParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateTemplateActiveVersionByID)(ctx, arg)
}

func (q *querier) SoftDeleteTemplateByID(ctx context.Context, id uuid.UUID) error {
	deleteF := func(ctx context.Context, id uuid.UUID) error {
		return q.db.UpdateTemplateDeletedByID(ctx, database.UpdateTemplateDeletedByIDParams{
			ID:        id,
			Deleted:   true,
			UpdatedAt: database.Now(),
		})
	}
	return deleteQ(q.log, q.auth, q.db.GetTemplateByID, deleteF)(ctx, id)
}

// Deprecated: use SoftDeleteTemplateByID instead.
func (q *querier) UpdateTemplateDeletedByID(ctx context.Context, arg database.UpdateTemplateDeletedByIDParams) error {
	return q.SoftDeleteTemplateByID(ctx, arg.ID)
}

func (q *querier) UpdateTemplateMetaByID(ctx context.Context, arg database.UpdateTemplateMetaByIDParams) (database.Template, error) {
	fetch := func(ctx context.Context, arg database.UpdateTemplateMetaByIDParams) (database.Template, error) {
		return q.db.GetTemplateByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateTemplateMetaByID)(ctx, arg)
}

func (q *querier) UpdateTemplateVersionByID(ctx context.Context, arg database.UpdateTemplateVersionByIDParams) error {
	template, err := q.db.GetTemplateByID(ctx, arg.TemplateID.UUID)
	if err != nil {
		return err
	}
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, template); err != nil {
		return err
	}
	return q.db.UpdateTemplateVersionByID(ctx, arg)
}

func (q *querier) UpdateTemplateVersionDescriptionByJobID(ctx context.Context, arg database.UpdateTemplateVersionDescriptionByJobIDParams) error {
	// An actor is allowed to update the template version description if they are authorized to update the template.
	tv, err := q.db.GetTemplateVersionByJobID(ctx, arg.JobID)
	if err != nil {
		return err
	}
	var obj rbac.Objecter
	if !tv.TemplateID.Valid {
		obj = rbac.ResourceTemplate.InOrg(tv.OrganizationID)
	} else {
		tpl, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
		if err != nil {
			return err
		}
		obj = tpl
	}
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, obj); err != nil {
		return err
	}
	return q.db.UpdateTemplateVersionDescriptionByJobID(ctx, arg)
}

func (q *querier) GetTemplateGroupRoles(ctx context.Context, id uuid.UUID) ([]database.TemplateGroup, error) {
	// An actor is authorized to read template group roles if they are authorized to read the template.
	template, err := q.db.GetTemplateByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := q.authorizeContext(ctx, rbac.ActionRead, template); err != nil {
		return nil, err
	}
	return q.db.GetTemplateGroupRoles(ctx, id)
}

func (q *querier) GetTemplateUserRoles(ctx context.Context, id uuid.UUID) ([]database.TemplateUser, error) {
	// An actor is authorized to query template user roles if they are authorized to read the template.
	template, err := q.db.GetTemplateByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := q.authorizeContext(ctx, rbac.ActionRead, template); err != nil {
		return nil, err
	}
	return q.db.GetTemplateUserRoles(ctx, id)
}

func (q *querier) DeleteAPIKeysByUserID(ctx context.Context, userID uuid.UUID) error {
	// TODO: This is not 100% correct because it omits apikey IDs.
	err := q.authorizeContext(ctx, rbac.ActionDelete,
		rbac.ResourceAPIKey.WithOwner(userID.String()))
	if err != nil {
		return err
	}
	return q.db.DeleteAPIKeysByUserID(ctx, userID)
}

func (q *querier) GetQuotaAllowanceForUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceUser.WithID(userID))
	if err != nil {
		return -1, err
	}
	return q.db.GetQuotaAllowanceForUser(ctx, userID)
}

func (q *querier) GetQuotaConsumedForUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	err := q.authorizeContext(ctx, rbac.ActionRead, rbac.ResourceUser.WithID(userID))
	if err != nil {
		return -1, err
	}
	return q.db.GetQuotaConsumedForUser(ctx, userID)
}

func (q *querier) GetUserByEmailOrUsername(ctx context.Context, arg database.GetUserByEmailOrUsernameParams) (database.User, error) {
	return fetch(q.log, q.auth, q.db.GetUserByEmailOrUsername)(ctx, arg)
}

func (q *querier) GetUserByID(ctx context.Context, id uuid.UUID) (database.User, error) {
	return fetch(q.log, q.auth, q.db.GetUserByID)(ctx, id)
}

func (q *querier) GetAuthorizedUserCount(ctx context.Context, arg database.GetFilteredUserCountParams, prepared rbac.PreparedAuthorized) (int64, error) {
	return q.db.GetAuthorizedUserCount(ctx, arg, prepared)
}

func (q *querier) GetFilteredUserCount(ctx context.Context, arg database.GetFilteredUserCountParams) (int64, error) {
	prep, err := prepareSQLFilter(ctx, q.auth, rbac.ActionRead, rbac.ResourceUser.Type)
	if err != nil {
		return -1, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	// TODO: This should be the only implementation.
	return q.GetAuthorizedUserCount(ctx, arg, prep)
}

func (q *querier) GetUsers(ctx context.Context, arg database.GetUsersParams) ([]database.GetUsersRow, error) {
	// TODO: We should use GetUsersWithCount with a better method signature.
	return fetchWithPostFilter(q.auth, q.db.GetUsers)(ctx, arg)
}

func (q *querier) GetUsersWithCount(ctx context.Context, arg database.GetUsersParams) ([]database.User, int64, error) {
	// TODO Implement this with a SQL filter. The count is incorrect without it.
	rowUsers, err := q.db.GetUsers(ctx, arg)
	if err != nil {
		return nil, -1, err
	}

	if len(rowUsers) == 0 {
		return []database.User{}, 0, nil
	}

	act, ok := ActorFromContext(ctx)
	if !ok {
		return nil, -1, NoActorError
	}

	// TODO: Is this correct? Should we return a restricted user?
	users := database.ConvertUserRows(rowUsers)
	users, err = rbac.Filter(ctx, q.auth, act, rbac.ActionRead, users)
	if err != nil {
		return nil, -1, err
	}

	return users, rowUsers[0].Count, nil
}

// TODO: Remove this and use a filter on GetUsers
func (q *querier) GetUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]database.User, error) {
	return fetchWithPostFilter(q.auth, q.db.GetUsersByIDs)(ctx, ids)
}

func (q *querier) InsertUser(ctx context.Context, arg database.InsertUserParams) (database.User, error) {
	// Always check if the assigned roles can actually be assigned by this actor.
	impliedRoles := append([]string{rbac.RoleMember()}, arg.RBACRoles...)
	err := q.canAssignRoles(ctx, nil, impliedRoles, []string{})
	if err != nil {
		return database.User{}, err
	}
	obj := rbac.ResourceUser
	return insert(q.log, q.auth, obj, q.db.InsertUser)(ctx, arg)
}

// TODO: Should this be in system.go?
func (q *querier) InsertUserLink(ctx context.Context, arg database.InsertUserLinkParams) (database.UserLink, error) {
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, rbac.ResourceUser.WithID(arg.UserID)); err != nil {
		return database.UserLink{}, err
	}
	return q.db.InsertUserLink(ctx, arg)
}

func (q *querier) SoftDeleteUserByID(ctx context.Context, id uuid.UUID) error {
	deleteF := func(ctx context.Context, id uuid.UUID) error {
		return q.db.UpdateUserDeletedByID(ctx, database.UpdateUserDeletedByIDParams{
			ID:      id,
			Deleted: true,
		})
	}
	return deleteQ(q.log, q.auth, q.db.GetUserByID, deleteF)(ctx, id)
}

// UpdateUserDeletedByID
// Deprecated: Delete this function in favor of 'SoftDeleteUserByID'. Deletes are
// irreversible.
func (q *querier) UpdateUserDeletedByID(ctx context.Context, arg database.UpdateUserDeletedByIDParams) error {
	fetch := func(ctx context.Context, arg database.UpdateUserDeletedByIDParams) (database.User, error) {
		return q.db.GetUserByID(ctx, arg.ID)
	}
	// This uses the rbac.ActionDelete action always as this function should always delete.
	// We should delete this function in favor of 'SoftDeleteUserByID'.
	return deleteQ(q.log, q.auth, fetch, q.db.UpdateUserDeletedByID)(ctx, arg)
}

func (q *querier) UpdateUserHashedPassword(ctx context.Context, arg database.UpdateUserHashedPasswordParams) error {
	user, err := q.db.GetUserByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, rbac.ActionUpdate, user.UserDataRBACObject())
	if err != nil {
		return err
	}

	return q.db.UpdateUserHashedPassword(ctx, arg)
}

func (q *querier) UpdateUserLastSeenAt(ctx context.Context, arg database.UpdateUserLastSeenAtParams) (database.User, error) {
	fetch := func(ctx context.Context, arg database.UpdateUserLastSeenAtParams) (database.User, error) {
		return q.db.GetUserByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateUserLastSeenAt)(ctx, arg)
}

func (q *querier) UpdateUserProfile(ctx context.Context, arg database.UpdateUserProfileParams) (database.User, error) {
	u, err := q.db.GetUserByID(ctx, arg.ID)
	if err != nil {
		return database.User{}, err
	}
	if err := q.authorizeContext(ctx, rbac.ActionUpdate, u.UserDataRBACObject()); err != nil {
		return database.User{}, err
	}
	return q.db.UpdateUserProfile(ctx, arg)
}

func (q *querier) UpdateUserStatus(ctx context.Context, arg database.UpdateUserStatusParams) (database.User, error) {
	fetch := func(ctx context.Context, arg database.UpdateUserStatusParams) (database.User, error) {
		return q.db.GetUserByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateUserStatus)(ctx, arg)
}

func (q *querier) DeleteGitSSHKey(ctx context.Context, userID uuid.UUID) error {
	return deleteQ(q.log, q.auth, q.db.GetGitSSHKey, q.db.DeleteGitSSHKey)(ctx, userID)
}

func (q *querier) GetGitSSHKey(ctx context.Context, userID uuid.UUID) (database.GitSSHKey, error) {
	return fetch(q.log, q.auth, q.db.GetGitSSHKey)(ctx, userID)
}

func (q *querier) InsertGitSSHKey(ctx context.Context, arg database.InsertGitSSHKeyParams) (database.GitSSHKey, error) {
	return insert(q.log, q.auth, rbac.ResourceUserData.WithOwner(arg.UserID.String()).WithID(arg.UserID), q.db.InsertGitSSHKey)(ctx, arg)
}

func (q *querier) UpdateGitSSHKey(ctx context.Context, arg database.UpdateGitSSHKeyParams) (database.GitSSHKey, error) {
	fetch := func(ctx context.Context, arg database.UpdateGitSSHKeyParams) (database.GitSSHKey, error) {
		return q.db.GetGitSSHKey(ctx, arg.UserID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateGitSSHKey)(ctx, arg)
}

func (q *querier) GetGitAuthLink(ctx context.Context, arg database.GetGitAuthLinkParams) (database.GitAuthLink, error) {
	return fetch(q.log, q.auth, q.db.GetGitAuthLink)(ctx, arg)
}

func (q *querier) InsertGitAuthLink(ctx context.Context, arg database.InsertGitAuthLinkParams) (database.GitAuthLink, error) {
	return insert(q.log, q.auth, rbac.ResourceUserData.WithOwner(arg.UserID.String()).WithID(arg.UserID), q.db.InsertGitAuthLink)(ctx, arg)
}

func (q *querier) UpdateGitAuthLink(ctx context.Context, arg database.UpdateGitAuthLinkParams) error {
	fetch := func(ctx context.Context, arg database.UpdateGitAuthLinkParams) (database.GitAuthLink, error) {
		return q.db.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{UserID: arg.UserID, ProviderID: arg.ProviderID})
	}
	return update(q.log, q.auth, fetch, q.db.UpdateGitAuthLink)(ctx, arg)
}

func (q *querier) UpdateUserLink(ctx context.Context, arg database.UpdateUserLinkParams) (database.UserLink, error) {
	fetch := func(ctx context.Context, arg database.UpdateUserLinkParams) (database.UserLink, error) {
		return q.db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    arg.UserID,
			LoginType: arg.LoginType,
		})
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateUserLink)(ctx, arg)
}

// UpdateUserRoles updates the site roles of a user. The validation for this function include more than
// just a basic RBAC check.
func (q *querier) UpdateUserRoles(ctx context.Context, arg database.UpdateUserRolesParams) (database.User, error) {
	// We need to fetch the user being updated to identify the change in roles.
	// This requires read access on the user in question, since the user is
	// returned from this function.
	user, err := fetch(q.log, q.auth, q.db.GetUserByID)(ctx, arg.ID)
	if err != nil {
		return database.User{}, err
	}

	// The member role is always implied.
	impliedTypes := append(arg.GrantedRoles, rbac.RoleMember())
	// If the changeset is nothing, less rbac checks need to be done.
	added, removed := rbac.ChangeRoleSet(user.RBACRoles, impliedTypes)
	err = q.canAssignRoles(ctx, nil, added, removed)
	if err != nil {
		return database.User{}, err
	}

	return q.db.UpdateUserRoles(ctx, arg)
}

func (q *querier) GetAuthorizedWorkspaces(ctx context.Context, arg database.GetWorkspacesParams, _ rbac.PreparedAuthorized) ([]database.GetWorkspacesRow, error) {
	// TODO Delete this function, all GetWorkspaces should be authorized. For now just call GetWorkspaces on the authz querier.
	return q.GetWorkspaces(ctx, arg)
}

func (q *querier) GetWorkspaces(ctx context.Context, arg database.GetWorkspacesParams) ([]database.GetWorkspacesRow, error) {
	prep, err := prepareSQLFilter(ctx, q.auth, rbac.ActionRead, rbac.ResourceWorkspace.Type)
	if err != nil {
		return nil, xerrors.Errorf("(dev error) prepare sql filter: %w", err)
	}
	return q.db.GetAuthorizedWorkspaces(ctx, arg, prep)
}

func (q *querier) GetLatestWorkspaceBuildByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) (database.WorkspaceBuild, error) {
	if _, err := q.GetWorkspaceByID(ctx, workspaceID); err != nil {
		return database.WorkspaceBuild{}, err
	}
	return q.db.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspaceID)
}

func (q *querier) GetLatestWorkspaceBuildsByWorkspaceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceBuild, error) {
	// This is not ideal as not all builds will be returned if the workspace cannot be read.
	// This should probably be handled differently? Maybe join workspace builds with workspace
	// ownership properties and filter on that.
	for _, id := range ids {
		_, err := q.GetWorkspaceByID(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	return q.db.GetLatestWorkspaceBuildsByWorkspaceIDs(ctx, ids)
}

func (q *querier) GetWorkspaceAgentByID(ctx context.Context, id uuid.UUID) (database.WorkspaceAgent, error) {
	if _, err := q.GetWorkspaceByAgentID(ctx, id); err != nil {
		return database.WorkspaceAgent{}, err
	}
	return q.db.GetWorkspaceAgentByID(ctx, id)
}

// GetWorkspaceAgentByInstanceID might want to be a system call? Unsure exactly,
// but this will fail. Need to figure out what AuthInstanceID is, and if it
// is essentially an auth token. But the caller using this function is not
// an authenticated user. So this authz check will fail.
func (q *querier) GetWorkspaceAgentByInstanceID(ctx context.Context, authInstanceID string) (database.WorkspaceAgent, error) {
	agent, err := q.db.GetWorkspaceAgentByInstanceID(ctx, authInstanceID)
	if err != nil {
		return database.WorkspaceAgent{}, err
	}
	_, err = q.GetWorkspaceByAgentID(ctx, agent.ID)
	if err != nil {
		return database.WorkspaceAgent{}, err
	}
	return agent, nil
}

// GetWorkspaceAgentsByResourceIDs is an all or nothing call. If the user cannot read
// a single agent, the entire call will fail.
func (q *querier) GetWorkspaceAgentsByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceAgent, error) {
	if _, ok := ActorFromContext(ctx); !ok {
		return nil, NoActorError
	}
	// TODO: Make this more efficient. This is annoying because all these resources should be owned by the same workspace.
	// So the authz check should just be 1 check, but we cannot do that easily here. We should see if all callers can
	// instead do something like GetWorkspaceAgentsByWorkspaceID.
	agents, err := q.db.GetWorkspaceAgentsByResourceIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	for _, a := range agents {
		// Check if we can fetch the workspace by the agent ID.
		_, err := q.GetWorkspaceByAgentID(ctx, a.ID)
		if err == nil {
			continue
		}
		if errors.Is(err, sql.ErrNoRows) && !errors.As(err, &NotAuthorizedError{}) {
			// The agent is not tied to a workspace, likely from an orphaned template version.
			// Just return it.
			continue
		}
		// Otherwise, we cannot read the workspace, so we cannot read the agent.
		return nil, err
	}
	return agents, nil
}

func (q *querier) UpdateWorkspaceAgentLifecycleStateByID(ctx context.Context, arg database.UpdateWorkspaceAgentLifecycleStateByIDParams) error {
	agent, err := q.db.GetWorkspaceAgentByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	workspace, err := q.db.GetWorkspaceByAgentID(ctx, agent.ID)
	if err != nil {
		return err
	}

	if err := q.authorizeContext(ctx, rbac.ActionUpdate, workspace); err != nil {
		return err
	}

	return q.db.UpdateWorkspaceAgentLifecycleStateByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceAgentStartupByID(ctx context.Context, arg database.UpdateWorkspaceAgentStartupByIDParams) error {
	agent, err := q.db.GetWorkspaceAgentByID(ctx, arg.ID)
	if err != nil {
		return err
	}

	workspace, err := q.db.GetWorkspaceByAgentID(ctx, agent.ID)
	if err != nil {
		return err
	}

	if err := q.authorizeContext(ctx, rbac.ActionUpdate, workspace); err != nil {
		return err
	}

	return q.db.UpdateWorkspaceAgentStartupByID(ctx, arg)
}

func (q *querier) GetWorkspaceAppByAgentIDAndSlug(ctx context.Context, arg database.GetWorkspaceAppByAgentIDAndSlugParams) (database.WorkspaceApp, error) {
	// If we can fetch the workspace, we can fetch the apps. Use the authorized call.
	if _, err := q.GetWorkspaceByAgentID(ctx, arg.AgentID); err != nil {
		return database.WorkspaceApp{}, err
	}

	return q.db.GetWorkspaceAppByAgentIDAndSlug(ctx, arg)
}

func (q *querier) GetWorkspaceAppsByAgentID(ctx context.Context, agentID uuid.UUID) ([]database.WorkspaceApp, error) {
	if _, err := q.GetWorkspaceByAgentID(ctx, agentID); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceAppsByAgentID(ctx, agentID)
}

// GetWorkspaceAppsByAgentIDs is an all or nothing call. If the user cannot read a single app, the entire call will fail.
func (q *querier) GetWorkspaceAppsByAgentIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceApp, error) {
	// TODO: This should be reworked. All these apps are likely owned by the same workspace, so we should be able to
	// do 1 authz call. We should refactor this to be GetWorkspaceAppsByWorkspaceID.
	for _, id := range ids {
		_, err := q.GetWorkspaceAgentByID(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	return q.db.GetWorkspaceAppsByAgentIDs(ctx, ids)
}

func (q *querier) GetWorkspaceBuildByID(ctx context.Context, buildID uuid.UUID) (database.WorkspaceBuild, error) {
	build, err := q.db.GetWorkspaceBuildByID(ctx, buildID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}
	if _, err := q.GetWorkspaceByID(ctx, build.WorkspaceID); err != nil {
		return database.WorkspaceBuild{}, err
	}
	return build, nil
}

func (q *querier) GetWorkspaceBuildByJobID(ctx context.Context, jobID uuid.UUID) (database.WorkspaceBuild, error) {
	build, err := q.db.GetWorkspaceBuildByJobID(ctx, jobID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}
	// Authorized fetch
	_, err = q.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}
	return build, nil
}

func (q *querier) GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx context.Context, arg database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (database.WorkspaceBuild, error) {
	if _, err := q.GetWorkspaceByID(ctx, arg.WorkspaceID); err != nil {
		return database.WorkspaceBuild{}, err
	}
	return q.db.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, arg)
}

func (q *querier) GetWorkspaceBuildParameters(ctx context.Context, workspaceBuildID uuid.UUID) ([]database.WorkspaceBuildParameter, error) {
	// Authorized call to get the workspace build. If we can read the build,
	// we can read the params.
	_, err := q.GetWorkspaceBuildByID(ctx, workspaceBuildID)
	if err != nil {
		return nil, err
	}

	return q.db.GetWorkspaceBuildParameters(ctx, workspaceBuildID)
}

func (q *querier) GetWorkspaceBuildsByWorkspaceID(ctx context.Context, arg database.GetWorkspaceBuildsByWorkspaceIDParams) ([]database.WorkspaceBuild, error) {
	if _, err := q.GetWorkspaceByID(ctx, arg.WorkspaceID); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceBuildsByWorkspaceID(ctx, arg)
}

func (q *querier) GetWorkspaceByAgentID(ctx context.Context, agentID uuid.UUID) (database.Workspace, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceByAgentID)(ctx, agentID)
}

func (q *querier) GetWorkspaceByID(ctx context.Context, id uuid.UUID) (database.Workspace, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceByID)(ctx, id)
}

func (q *querier) GetWorkspaceByOwnerIDAndName(ctx context.Context, arg database.GetWorkspaceByOwnerIDAndNameParams) (database.Workspace, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceByOwnerIDAndName)(ctx, arg)
}

func (q *querier) GetWorkspaceResourceByID(ctx context.Context, id uuid.UUID) (database.WorkspaceResource, error) {
	// TODO: Optimize this
	resource, err := q.db.GetWorkspaceResourceByID(ctx, id)
	if err != nil {
		return database.WorkspaceResource{}, err
	}

	_, err = q.GetProvisionerJobByID(ctx, resource.JobID)
	if err != nil {
		return database.WorkspaceResource{}, err
	}

	return resource, nil
}

// GetWorkspaceResourceMetadataByResourceIDs is an all or nothing call. If a single resource is not authorized, then
// an error is returned.
func (q *querier) GetWorkspaceResourceMetadataByResourceIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResourceMetadatum, error) {
	// TODO: This is very inefficient. Since all these resources are likely asscoiated with the same workspace.
	for _, id := range ids {
		// If we can read the resource, we can read the metadata.
		_, err := q.GetWorkspaceResourceByID(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	return q.db.GetWorkspaceResourceMetadataByResourceIDs(ctx, ids)
}

func (q *querier) GetWorkspaceResourcesByJobID(ctx context.Context, jobID uuid.UUID) ([]database.WorkspaceResource, error) {
	job, err := q.db.GetProvisionerJobByID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	var obj rbac.Objecter
	switch job.Type {
	case database.ProvisionerJobTypeTemplateVersionDryRun, database.ProvisionerJobTypeTemplateVersionImport:
		// We don't need to do an authorized check, but this helper function
		// handles the job type for us.
		// TODO: Do not duplicate auth checks.
		tv, err := authorizedTemplateVersionFromJob(ctx, q, job)
		if err != nil {
			return nil, err
		}
		if !tv.TemplateID.Valid {
			// Orphaned template version
			obj = tv.RBACObjectNoTemplate()
		} else {
			template, err := q.db.GetTemplateByID(ctx, tv.TemplateID.UUID)
			if err != nil {
				return nil, err
			}
			obj = template.RBACObject()
		}
	case database.ProvisionerJobTypeWorkspaceBuild:
		build, err := q.db.GetWorkspaceBuildByJobID(ctx, jobID)
		if err != nil {
			return nil, err
		}
		workspace, err := q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
		if err != nil {
			return nil, err
		}
		obj = workspace
	default:
		return nil, xerrors.Errorf("unknown job type: %s", job.Type)
	}

	if err := q.authorizeContext(ctx, rbac.ActionRead, obj); err != nil {
		return nil, err
	}
	return q.db.GetWorkspaceResourcesByJobID(ctx, jobID)
}

// GetWorkspaceResourcesByJobIDs is an all or nothing call. If a single resource is not authorized, then
// an error is returned.
func (q *querier) GetWorkspaceResourcesByJobIDs(ctx context.Context, ids []uuid.UUID) ([]database.WorkspaceResource, error) {
	// TODO: This is very inefficient. Since all these resources are likely asscoiated with the same workspace.
	for _, id := range ids {
		// If we can read the resource, we can read the metadata.
		_, err := q.GetProvisionerJobByID(ctx, id)
		if err != nil {
			return nil, err
		}
	}

	return q.db.GetWorkspaceResourcesByJobIDs(ctx, ids)
}

func (q *querier) InsertWorkspace(ctx context.Context, arg database.InsertWorkspaceParams) (database.Workspace, error) {
	obj := rbac.ResourceWorkspace.WithOwner(arg.OwnerID.String()).InOrg(arg.OrganizationID)
	return insert(q.log, q.auth, obj, q.db.InsertWorkspace)(ctx, arg)
}

func (q *querier) InsertWorkspaceBuild(ctx context.Context, arg database.InsertWorkspaceBuildParams) (database.WorkspaceBuild, error) {
	w, err := q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}

	var action rbac.Action = rbac.ActionUpdate
	if arg.Transition == database.WorkspaceTransitionDelete {
		action = rbac.ActionDelete
	}

	if err = q.authorizeContext(ctx, action, w); err != nil {
		return database.WorkspaceBuild{}, err
	}

	return q.db.InsertWorkspaceBuild(ctx, arg)
}

func (q *querier) InsertWorkspaceBuildParameters(ctx context.Context, arg database.InsertWorkspaceBuildParametersParams) error {
	// TODO: Optimize this. We always have the workspace and build already fetched.
	build, err := q.db.GetWorkspaceBuildByID(ctx, arg.WorkspaceBuildID)
	if err != nil {
		return err
	}

	workspace, err := q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace)
	if err != nil {
		return err
	}

	return q.db.InsertWorkspaceBuildParameters(ctx, arg)
}

func (q *querier) UpdateWorkspace(ctx context.Context, arg database.UpdateWorkspaceParams) (database.Workspace, error) {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return updateWithReturn(q.log, q.auth, fetch, q.db.UpdateWorkspace)(ctx, arg)
}

func (q *querier) UpdateWorkspaceAgentConnectionByID(ctx context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) error {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByAgentID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceAgentConnectionByID)(ctx, arg)
}

func (q *querier) InsertAgentStat(ctx context.Context, arg database.InsertAgentStatParams) (database.AgentStat, error) {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	// Not really sure what this is for.
	workspace, err := q.db.GetWorkspaceByID(ctx, arg.WorkspaceID)
	if err != nil {
		return database.AgentStat{}, err
	}
	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace)
	if err != nil {
		return database.AgentStat{}, err
	}
	return q.db.InsertAgentStat(ctx, arg)
}

func (q *querier) UpdateWorkspaceAppHealthByID(ctx context.Context, arg database.UpdateWorkspaceAppHealthByIDParams) error {
	// TODO: This is a workspace agent operation. Should users be able to query this?
	workspace, err := q.db.GetWorkspaceByWorkspaceAppID(ctx, arg.ID)
	if err != nil {
		return err
	}

	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace.RBACObject())
	if err != nil {
		return err
	}
	return q.db.UpdateWorkspaceAppHealthByID(ctx, arg)
}

func (q *querier) UpdateWorkspaceAutostart(ctx context.Context, arg database.UpdateWorkspaceAutostartParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceAutostartParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceAutostart)(ctx, arg)
}

func (q *querier) UpdateWorkspaceBuildByID(ctx context.Context, arg database.UpdateWorkspaceBuildByIDParams) (database.WorkspaceBuild, error) {
	build, err := q.db.GetWorkspaceBuildByID(ctx, arg.ID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}

	workspace, err := q.db.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return database.WorkspaceBuild{}, err
	}
	err = q.authorizeContext(ctx, rbac.ActionUpdate, workspace.RBACObject())
	if err != nil {
		return database.WorkspaceBuild{}, err
	}

	return q.db.UpdateWorkspaceBuildByID(ctx, arg)
}

func (q *querier) SoftDeleteWorkspaceByID(ctx context.Context, id uuid.UUID) error {
	return deleteQ(q.log, q.auth, q.db.GetWorkspaceByID, func(ctx context.Context, id uuid.UUID) error {
		return q.db.UpdateWorkspaceDeletedByID(ctx, database.UpdateWorkspaceDeletedByIDParams{
			ID:      id,
			Deleted: true,
		})
	})(ctx, id)
}

// Deprecated: Use SoftDeleteWorkspaceByID
func (q *querier) UpdateWorkspaceDeletedByID(ctx context.Context, arg database.UpdateWorkspaceDeletedByIDParams) error {
	// TODO deleteQ me, placeholder for database.Store
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceDeletedByIDParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	// This function is always used to deleteQ.
	return deleteQ(q.log, q.auth, fetch, q.db.UpdateWorkspaceDeletedByID)(ctx, arg)
}

func (q *querier) UpdateWorkspaceLastUsedAt(ctx context.Context, arg database.UpdateWorkspaceLastUsedAtParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceLastUsedAtParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceLastUsedAt)(ctx, arg)
}

func (q *querier) UpdateWorkspaceTTL(ctx context.Context, arg database.UpdateWorkspaceTTLParams) error {
	fetch := func(ctx context.Context, arg database.UpdateWorkspaceTTLParams) (database.Workspace, error) {
		return q.db.GetWorkspaceByID(ctx, arg.ID)
	}
	return update(q.log, q.auth, fetch, q.db.UpdateWorkspaceTTL)(ctx, arg)
}

func (q *querier) GetWorkspaceByWorkspaceAppID(ctx context.Context, workspaceAppID uuid.UUID) (database.Workspace, error) {
	return fetch(q.log, q.auth, q.db.GetWorkspaceByWorkspaceAppID)(ctx, workspaceAppID)
}

func authorizedTemplateVersionFromJob(ctx context.Context, q *querier, job database.ProvisionerJob) (database.TemplateVersion, error) {
	switch job.Type {
	case database.ProvisionerJobTypeTemplateVersionDryRun:
		// TODO: This is really unfortunate that we need to inspect the json
		// payload. We should fix this.
		tmp := struct {
			TemplateVersionID uuid.UUID `json:"template_version_id"`
		}{}
		err := json.Unmarshal(job.Input, &tmp)
		if err != nil {
			return database.TemplateVersion{}, xerrors.Errorf("dry-run unmarshal: %w", err)
		}
		// Authorized call to get template version.
		tv, err := q.GetTemplateVersionByID(ctx, tmp.TemplateVersionID)
		if err != nil {
			return database.TemplateVersion{}, err
		}
		return tv, nil
	case database.ProvisionerJobTypeTemplateVersionImport:
		// Authorized call to get template version.
		tv, err := q.GetTemplateVersionByJobID(ctx, job.ID)
		if err != nil {
			return database.TemplateVersion{}, err
		}
		return tv, nil
	default:
		return database.TemplateVersion{}, xerrors.Errorf("unknown job type: %q", job.Type)
	}
}
