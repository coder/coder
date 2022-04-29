package databasefake

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
)

// New returns an in-memory fake of the database.
func New() database.Store {
	return &fakeQuerier{
		apiKeys:             make([]database.APIKey, 0),
		organizations:       make([]database.Organization, 0),
		organizationMembers: make([]database.OrganizationMember, 0),
		users:               make([]database.User, 0),

		files:                  make([]database.File, 0),
		parameterValue:         make([]database.ParameterValue, 0),
		parameterSchema:        make([]database.ParameterSchema, 0),
		template:               make([]database.Template, 0),
		templateVersion:        make([]database.TemplateVersion, 0),
		provisionerDaemons:     make([]database.ProvisionerDaemon, 0),
		provisionerJobs:        make([]database.ProvisionerJob, 0),
		provisionerJobLog:      make([]database.ProvisionerJobLog, 0),
		workspaces:             make([]database.Workspace, 0),
		provisionerJobResource: make([]database.WorkspaceResource, 0),
		workspaceBuild:         make([]database.WorkspaceBuild, 0),
		provisionerJobAgent:    make([]database.WorkspaceAgent, 0),
		GitSSHKey:              make([]database.GitSSHKey, 0),
	}
}

// fakeQuerier replicates database functionality to enable quick testing.
type fakeQuerier struct {
	mutex sync.RWMutex

	// Legacy tables
	apiKeys             []database.APIKey
	organizations       []database.Organization
	organizationMembers []database.OrganizationMember
	users               []database.User

	// New tables
	files                  []database.File
	parameterValue         []database.ParameterValue
	parameterSchema        []database.ParameterSchema
	template               []database.Template
	templateVersion        []database.TemplateVersion
	provisionerDaemons     []database.ProvisionerDaemon
	provisionerJobs        []database.ProvisionerJob
	provisionerJobAgent    []database.WorkspaceAgent
	provisionerJobResource []database.WorkspaceResource
	provisionerJobLog      []database.ProvisionerJobLog
	workspaces             []database.Workspace
	workspaceBuild         []database.WorkspaceBuild
	GitSSHKey              []database.GitSSHKey
}

// InTx doesn't rollback data properly for in-memory yet.
func (q *fakeQuerier) InTx(fn func(database.Store) error) error {
	return fn(q)
}

func (q *fakeQuerier) AcquireProvisionerJob(_ context.Context, arg database.AcquireProvisionerJobParams) (database.ProvisionerJob, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, provisionerJob := range q.provisionerJobs {
		if provisionerJob.StartedAt.Valid {
			continue
		}
		found := false
		for _, provisionerType := range arg.Types {
			if provisionerJob.Provisioner != provisionerType {
				continue
			}
			found = true
			break
		}
		if !found {
			continue
		}
		provisionerJob.StartedAt = arg.StartedAt
		provisionerJob.UpdatedAt = arg.StartedAt.Time
		provisionerJob.WorkerID = arg.WorkerID
		q.provisionerJobs[index] = provisionerJob
		return provisionerJob, nil
	}
	return database.ProvisionerJob{}, sql.ErrNoRows
}

func (q *fakeQuerier) DeleteParameterValueByID(_ context.Context, id uuid.UUID) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, parameterValue := range q.parameterValue {
		if parameterValue.ID.String() != id.String() {
			continue
		}
		q.parameterValue[index] = q.parameterValue[len(q.parameterValue)-1]
		q.parameterValue = q.parameterValue[:len(q.parameterValue)-1]
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) GetAPIKeyByID(_ context.Context, id string) (database.APIKey, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, apiKey := range q.apiKeys {
		if apiKey.ID == id {
			return apiKey, nil
		}
	}
	return database.APIKey{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetFileByHash(_ context.Context, hash string) (database.File, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, file := range q.files {
		if file.Hash == hash {
			return file, nil
		}
	}
	return database.File{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserByEmailOrUsername(_ context.Context, arg database.GetUserByEmailOrUsernameParams) (database.User, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, user := range q.users {
		if user.Email == arg.Email || user.Username == arg.Username {
			return user, nil
		}
	}
	return database.User{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserByID(_ context.Context, id uuid.UUID) (database.User, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, user := range q.users {
		if user.ID == id {
			return user, nil
		}
	}
	return database.User{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserCount(_ context.Context) (int64, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return int64(len(q.users)), nil
}

func (q *fakeQuerier) GetUsers(_ context.Context, params database.GetUsersParams) ([]database.User, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	users := q.users
	// Database orders by created_at
	sort.Slice(users, func(i, j int) bool {
		if users[i].CreatedAt.Equal(users[j].CreatedAt) {
			// Technically the postgres database also orders by uuid. So match
			// that behavior
			return users[i].ID.String() < users[j].ID.String()
		}
		return users[i].CreatedAt.Before(users[j].CreatedAt)
	})

	if params.AfterUser != uuid.Nil {
		found := false
		for i := range users {
			if users[i].ID == params.AfterUser {
				// We want to return all users after index i.
				if i+1 >= len(users) {
					return []database.User{}, nil
				}
				users = users[i+1:]
				found = true
				break
			}
		}

		// If no users after the time, then we return an empty list.
		if !found {
			return []database.User{}, nil
		}
	}

	if params.Search != "" {
		tmp := make([]database.User, 0, len(users))
		for i, user := range users {
			if strings.Contains(user.Email, params.Search) {
				tmp = append(tmp, users[i])
			} else if strings.Contains(user.Username, params.Search) {
				tmp = append(tmp, users[i])
			}
		}
		users = tmp
	}

	if params.Status != "" {
		usersFilteredByStatus := make([]database.User, 0, len(users))
		for i, user := range users {
			if params.Status == string(user.Status) {
				usersFilteredByStatus = append(usersFilteredByStatus, users[i])
			}
		}
		users = usersFilteredByStatus
	}

	if params.OffsetOpt > 0 {
		if int(params.OffsetOpt) > len(users)-1 {
			return []database.User{}, nil
		}
		users = users[params.OffsetOpt:]
	}

	if params.LimitOpt > 0 {
		if int(params.LimitOpt) > len(users) {
			params.LimitOpt = int32(len(users))
		}
		users = users[:params.LimitOpt]
	}

	tmp := make([]database.User, len(users))
	copy(tmp, users)

	return tmp, nil
}

func (q *fakeQuerier) GetWorkspacesByTemplateID(_ context.Context, arg database.GetWorkspacesByTemplateIDParams) ([]database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaces := make([]database.Workspace, 0)
	for _, workspace := range q.workspaces {
		if workspace.TemplateID.String() != arg.TemplateID.String() {
			continue
		}
		if workspace.Deleted != arg.Deleted {
			continue
		}
		workspaces = append(workspaces, workspace)
	}
	if len(workspaces) == 0 {
		return nil, sql.ErrNoRows
	}
	return workspaces, nil
}

func (q *fakeQuerier) GetWorkspaceByID(_ context.Context, id uuid.UUID) (database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspace := range q.workspaces {
		if workspace.ID.String() == id.String() {
			return workspace, nil
		}
	}
	return database.Workspace{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceByOwnerIDAndName(_ context.Context, arg database.GetWorkspaceByOwnerIDAndNameParams) (database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspace := range q.workspaces {
		if workspace.OwnerID != arg.OwnerID {
			continue
		}
		if !strings.EqualFold(workspace.Name, arg.Name) {
			continue
		}
		if workspace.Deleted != arg.Deleted {
			continue
		}
		return workspace, nil
	}
	return database.Workspace{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceOwnerCountsByTemplateIDs(_ context.Context, templateIDs []uuid.UUID) ([]database.GetWorkspaceOwnerCountsByTemplateIDsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	counts := map[uuid.UUID]map[uuid.UUID]struct{}{}
	for _, templateID := range templateIDs {
		found := false
		for _, workspace := range q.workspaces {
			if workspace.TemplateID != templateID {
				continue
			}
			if workspace.Deleted {
				continue
			}
			countByOwnerID, ok := counts[templateID]
			if !ok {
				countByOwnerID = map[uuid.UUID]struct{}{}
			}
			countByOwnerID[workspace.OwnerID] = struct{}{}
			counts[templateID] = countByOwnerID
			found = true
			break
		}
		if !found {
			counts[templateID] = map[uuid.UUID]struct{}{}
		}
	}
	res := make([]database.GetWorkspaceOwnerCountsByTemplateIDsRow, 0)
	for key, value := range counts {
		res = append(res, database.GetWorkspaceOwnerCountsByTemplateIDsRow{
			TemplateID: key,
			Count:      int64(len(value)),
		})
	}
	if len(res) == 0 {
		return nil, sql.ErrNoRows
	}
	return res, nil
}

func (q *fakeQuerier) GetWorkspaceBuildByID(_ context.Context, id uuid.UUID) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, history := range q.workspaceBuild {
		if history.ID.String() == id.String() {
			return history, nil
		}
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceBuildByJobID(_ context.Context, jobID uuid.UUID) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, build := range q.workspaceBuild {
		if build.JobID.String() == jobID.String() {
			return build, nil
		}
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceBuildByWorkspaceIDWithoutAfter(_ context.Context, workspaceID uuid.UUID) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspaceBuild := range q.workspaceBuild {
		if workspaceBuild.WorkspaceID.String() != workspaceID.String() {
			continue
		}
		if !workspaceBuild.AfterID.Valid {
			return workspaceBuild, nil
		}
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceBuildsByWorkspaceIDsWithoutAfter(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	builds := make([]database.WorkspaceBuild, 0)
	for _, workspaceBuild := range q.workspaceBuild {
		for _, id := range ids {
			if id.String() != workspaceBuild.WorkspaceID.String() {
				continue
			}
			builds = append(builds, workspaceBuild)
		}
	}
	if len(builds) == 0 {
		return nil, sql.ErrNoRows
	}
	return builds, nil
}

func (q *fakeQuerier) GetWorkspaceBuildByWorkspaceID(_ context.Context, workspaceID uuid.UUID) ([]database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	history := make([]database.WorkspaceBuild, 0)
	for _, workspaceBuild := range q.workspaceBuild {
		if workspaceBuild.WorkspaceID.String() == workspaceID.String() {
			history = append(history, workspaceBuild)
		}
	}
	if len(history) == 0 {
		return nil, sql.ErrNoRows
	}
	return history, nil
}

func (q *fakeQuerier) GetWorkspaceBuildByWorkspaceIDAndName(_ context.Context, arg database.GetWorkspaceBuildByWorkspaceIDAndNameParams) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspaceBuild := range q.workspaceBuild {
		if workspaceBuild.WorkspaceID.String() != arg.WorkspaceID.String() {
			continue
		}
		if !strings.EqualFold(workspaceBuild.Name, arg.Name) {
			continue
		}
		return workspaceBuild, nil
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspacesByOrganizationID(_ context.Context, req database.GetWorkspacesByOrganizationIDParams) ([]database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaces := make([]database.Workspace, 0)
	for _, workspace := range q.workspaces {
		if workspace.OrganizationID != req.OrganizationID {
			continue
		}
		if workspace.Deleted != req.Deleted {
			continue
		}
		workspaces = append(workspaces, workspace)
	}
	if len(workspaces) == 0 {
		return nil, sql.ErrNoRows
	}
	return workspaces, nil
}

func (q *fakeQuerier) GetWorkspacesByOwnerID(_ context.Context, req database.GetWorkspacesByOwnerIDParams) ([]database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaces := make([]database.Workspace, 0)
	for _, workspace := range q.workspaces {
		if workspace.OwnerID != req.OwnerID {
			continue
		}
		if workspace.Deleted != req.Deleted {
			continue
		}
		workspaces = append(workspaces, workspace)
	}
	if len(workspaces) == 0 {
		return nil, sql.ErrNoRows
	}
	return workspaces, nil
}

func (q *fakeQuerier) GetOrganizations(_ context.Context) ([]database.Organization, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if len(q.organizations) == 0 {
		return nil, sql.ErrNoRows
	}
	return q.organizations, nil
}

func (q *fakeQuerier) GetOrganizationByID(_ context.Context, id uuid.UUID) (database.Organization, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, organization := range q.organizations {
		if organization.ID == id {
			return organization, nil
		}
	}
	return database.Organization{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetOrganizationByName(_ context.Context, name string) (database.Organization, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, organization := range q.organizations {
		if organization.Name == name {
			return organization, nil
		}
	}
	return database.Organization{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetOrganizationsByUserID(_ context.Context, userID uuid.UUID) ([]database.Organization, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	organizations := make([]database.Organization, 0)
	for _, organizationMember := range q.organizationMembers {
		if organizationMember.UserID != userID {
			continue
		}
		for _, organization := range q.organizations {
			if organization.ID != organizationMember.OrganizationID {
				continue
			}
			organizations = append(organizations, organization)
		}
	}
	if len(organizations) == 0 {
		return nil, sql.ErrNoRows
	}
	return organizations, nil
}

func (q *fakeQuerier) GetParameterValuesByScope(_ context.Context, arg database.GetParameterValuesByScopeParams) ([]database.ParameterValue, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	parameterValues := make([]database.ParameterValue, 0)
	for _, parameterValue := range q.parameterValue {
		if parameterValue.Scope != arg.Scope {
			continue
		}
		if parameterValue.ScopeID != arg.ScopeID {
			continue
		}
		parameterValues = append(parameterValues, parameterValue)
	}
	if len(parameterValues) == 0 {
		return nil, sql.ErrNoRows
	}
	return parameterValues, nil
}

func (q *fakeQuerier) GetTemplateByID(_ context.Context, id uuid.UUID) (database.Template, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, template := range q.template {
		if template.ID.String() == id.String() {
			return template, nil
		}
	}
	return database.Template{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetTemplateByOrganizationAndName(_ context.Context, arg database.GetTemplateByOrganizationAndNameParams) (database.Template, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, template := range q.template {
		if template.OrganizationID != arg.OrganizationID {
			continue
		}
		if !strings.EqualFold(template.Name, arg.Name) {
			continue
		}
		if template.Deleted != arg.Deleted {
			continue
		}
		return template, nil
	}
	return database.Template{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetTemplateVersionsByTemplateID(_ context.Context, templateID uuid.UUID) ([]database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	version := make([]database.TemplateVersion, 0)
	for _, templateVersion := range q.templateVersion {
		if templateVersion.TemplateID.UUID.String() != templateID.String() {
			continue
		}
		version = append(version, templateVersion)
	}
	if len(version) == 0 {
		return nil, sql.ErrNoRows
	}
	return version, nil
}

func (q *fakeQuerier) GetTemplateVersionByTemplateIDAndName(_ context.Context, arg database.GetTemplateVersionByTemplateIDAndNameParams) (database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, templateVersion := range q.templateVersion {
		if templateVersion.TemplateID != arg.TemplateID {
			continue
		}
		if !strings.EqualFold(templateVersion.Name, arg.Name) {
			continue
		}
		return templateVersion, nil
	}
	return database.TemplateVersion{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetTemplateVersionByID(_ context.Context, templateVersionID uuid.UUID) (database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, templateVersion := range q.templateVersion {
		if templateVersion.ID.String() != templateVersionID.String() {
			continue
		}
		return templateVersion, nil
	}
	return database.TemplateVersion{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetTemplateVersionByJobID(_ context.Context, jobID uuid.UUID) (database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, templateVersion := range q.templateVersion {
		if templateVersion.JobID.String() != jobID.String() {
			continue
		}
		return templateVersion, nil
	}
	return database.TemplateVersion{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetParameterSchemasByJobID(_ context.Context, jobID uuid.UUID) ([]database.ParameterSchema, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	parameters := make([]database.ParameterSchema, 0)
	for _, parameterSchema := range q.parameterSchema {
		if parameterSchema.JobID.String() != jobID.String() {
			continue
		}
		parameters = append(parameters, parameterSchema)
	}
	if len(parameters) == 0 {
		return nil, sql.ErrNoRows
	}
	return parameters, nil
}

func (q *fakeQuerier) GetParameterValueByScopeAndName(_ context.Context, arg database.GetParameterValueByScopeAndNameParams) (database.ParameterValue, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, parameterValue := range q.parameterValue {
		if parameterValue.Scope != arg.Scope {
			continue
		}
		if parameterValue.ScopeID != arg.ScopeID {
			continue
		}
		if parameterValue.Name != arg.Name {
			continue
		}
		return parameterValue, nil
	}
	return database.ParameterValue{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetTemplatesByOrganization(_ context.Context, arg database.GetTemplatesByOrganizationParams) ([]database.Template, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	templates := make([]database.Template, 0)
	for _, template := range q.template {
		if template.Deleted != arg.Deleted {
			continue
		}
		if template.OrganizationID != arg.OrganizationID {
			continue
		}
		templates = append(templates, template)
	}
	if len(templates) == 0 {
		return nil, sql.ErrNoRows
	}
	return templates, nil
}

func (q *fakeQuerier) GetTemplatesByIDs(_ context.Context, ids []uuid.UUID) ([]database.Template, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	templates := make([]database.Template, 0)
	for _, template := range q.template {
		for _, id := range ids {
			if template.ID.String() != id.String() {
				continue
			}
			templates = append(templates, template)
		}
	}
	if len(templates) == 0 {
		return nil, sql.ErrNoRows
	}
	return templates, nil
}

func (q *fakeQuerier) GetOrganizationMemberByUserID(_ context.Context, arg database.GetOrganizationMemberByUserIDParams) (database.OrganizationMember, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, organizationMember := range q.organizationMembers {
		if organizationMember.OrganizationID != arg.OrganizationID {
			continue
		}
		if organizationMember.UserID != arg.UserID {
			continue
		}
		return organizationMember, nil
	}
	return database.OrganizationMember{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetOrganizationIDsByMemberIDs(_ context.Context, ids []uuid.UUID) ([]database.GetOrganizationIDsByMemberIDsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	getOrganizationIDsByMemberIDRows := make([]database.GetOrganizationIDsByMemberIDsRow, 0, len(ids))
	for _, userID := range ids {
		userOrganizationIDs := make([]uuid.UUID, 0)
		for _, membership := range q.organizationMembers {
			if membership.UserID == userID {
				userOrganizationIDs = append(userOrganizationIDs, membership.OrganizationID)
			}
		}
		getOrganizationIDsByMemberIDRows = append(getOrganizationIDsByMemberIDRows, database.GetOrganizationIDsByMemberIDsRow{
			UserID:          userID,
			OrganizationIDs: userOrganizationIDs,
		})
	}
	if len(getOrganizationIDsByMemberIDRows) == 0 {
		return nil, sql.ErrNoRows
	}
	return getOrganizationIDsByMemberIDRows, nil
}

func (q *fakeQuerier) GetOrganizationMembershipsByUserID(_ context.Context, userID uuid.UUID) ([]database.OrganizationMember, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var memberships []database.OrganizationMember
	for _, organizationMember := range q.organizationMembers {
		mem := organizationMember
		if mem.UserID != userID {
			continue
		}
		memberships = append(memberships, mem)
	}
	return memberships, nil
}

func (q *fakeQuerier) UpdateMemberRoles(_ context.Context, arg database.UpdateMemberRolesParams) (database.OrganizationMember, error) {
	for i, mem := range q.organizationMembers {
		if mem.UserID == arg.UserID && mem.OrganizationID == arg.OrgID {
			uniqueRoles := make([]string, 0, len(arg.GrantedRoles))
			exist := make(map[string]struct{})
			for _, r := range arg.GrantedRoles {
				if _, ok := exist[r]; ok {
					continue
				}
				exist[r] = struct{}{}
				uniqueRoles = append(uniqueRoles, r)
			}
			sort.Strings(uniqueRoles)

			mem.Roles = uniqueRoles
			q.organizationMembers[i] = mem
			return mem, nil
		}
	}
	return database.OrganizationMember{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetProvisionerDaemons(_ context.Context) ([]database.ProvisionerDaemon, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if len(q.provisionerDaemons) == 0 {
		return nil, sql.ErrNoRows
	}
	return q.provisionerDaemons, nil
}

func (q *fakeQuerier) GetWorkspaceAgentByAuthToken(_ context.Context, authToken uuid.UUID) (database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// The schema sorts this by created at, so we iterate the array backwards.
	for i := len(q.provisionerJobAgent) - 1; i >= 0; i-- {
		agent := q.provisionerJobAgent[i]
		if agent.AuthToken.String() == authToken.String() {
			return agent, nil
		}
	}
	return database.WorkspaceAgent{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceAgentByID(_ context.Context, id uuid.UUID) (database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// The schema sorts this by created at, so we iterate the array backwards.
	for i := len(q.provisionerJobAgent) - 1; i >= 0; i-- {
		agent := q.provisionerJobAgent[i]
		if agent.ID.String() == id.String() {
			return agent, nil
		}
	}
	return database.WorkspaceAgent{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceAgentByInstanceID(_ context.Context, instanceID string) (database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// The schema sorts this by created at, so we iterate the array backwards.
	for i := len(q.provisionerJobAgent) - 1; i >= 0; i-- {
		agent := q.provisionerJobAgent[i]
		if agent.AuthInstanceID.Valid && agent.AuthInstanceID.String == instanceID {
			return agent, nil
		}
	}
	return database.WorkspaceAgent{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceAgentsByResourceIDs(_ context.Context, resourceIDs []uuid.UUID) ([]database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaceAgents := make([]database.WorkspaceAgent, 0)
	for _, agent := range q.provisionerJobAgent {
		for _, resourceID := range resourceIDs {
			if agent.ResourceID.String() != resourceID.String() {
				continue
			}
			workspaceAgents = append(workspaceAgents, agent)
		}
	}
	if len(workspaceAgents) == 0 {
		return nil, sql.ErrNoRows
	}
	return workspaceAgents, nil
}

func (q *fakeQuerier) GetProvisionerDaemonByID(_ context.Context, id uuid.UUID) (database.ProvisionerDaemon, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, provisionerDaemon := range q.provisionerDaemons {
		if provisionerDaemon.ID.String() != id.String() {
			continue
		}
		return provisionerDaemon, nil
	}
	return database.ProvisionerDaemon{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetProvisionerJobByID(_ context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, provisionerJob := range q.provisionerJobs {
		if provisionerJob.ID.String() != id.String() {
			continue
		}
		return provisionerJob, nil
	}
	return database.ProvisionerJob{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceResourceByID(_ context.Context, id uuid.UUID) (database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, resource := range q.provisionerJobResource {
		if resource.ID.String() == id.String() {
			return resource, nil
		}
	}
	return database.WorkspaceResource{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceResourcesByJobID(_ context.Context, jobID uuid.UUID) ([]database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	resources := make([]database.WorkspaceResource, 0)
	for _, resource := range q.provisionerJobResource {
		if resource.JobID.String() != jobID.String() {
			continue
		}
		resources = append(resources, resource)
	}
	if len(resources) == 0 {
		return nil, sql.ErrNoRows
	}
	return resources, nil
}

func (q *fakeQuerier) GetProvisionerJobsByIDs(_ context.Context, ids []uuid.UUID) ([]database.ProvisionerJob, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	jobs := make([]database.ProvisionerJob, 0)
	for _, job := range q.provisionerJobs {
		for _, id := range ids {
			if id.String() == job.ID.String() {
				jobs = append(jobs, job)
				break
			}
		}
	}
	if len(jobs) == 0 {
		return nil, sql.ErrNoRows
	}

	return jobs, nil
}

func (q *fakeQuerier) GetProvisionerLogsByIDBetween(_ context.Context, arg database.GetProvisionerLogsByIDBetweenParams) ([]database.ProvisionerJobLog, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	logs := make([]database.ProvisionerJobLog, 0)
	for _, jobLog := range q.provisionerJobLog {
		if jobLog.JobID.String() != arg.JobID.String() {
			continue
		}
		if jobLog.CreatedAt.After(arg.CreatedBefore) {
			continue
		}
		if jobLog.CreatedAt.Before(arg.CreatedAfter) {
			continue
		}
		logs = append(logs, jobLog)
	}
	if len(logs) == 0 {
		return nil, sql.ErrNoRows
	}
	return logs, nil
}

func (q *fakeQuerier) InsertAPIKey(_ context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	key := database.APIKey{
		ID:                arg.ID,
		HashedSecret:      arg.HashedSecret,
		UserID:            arg.UserID,
		ExpiresAt:         arg.ExpiresAt,
		CreatedAt:         arg.CreatedAt,
		UpdatedAt:         arg.UpdatedAt,
		LastUsed:          arg.LastUsed,
		LoginType:         arg.LoginType,
		OAuthAccessToken:  arg.OAuthAccessToken,
		OAuthRefreshToken: arg.OAuthRefreshToken,
		OAuthIDToken:      arg.OAuthIDToken,
		OAuthExpiry:       arg.OAuthExpiry,
	}
	q.apiKeys = append(q.apiKeys, key)
	return key, nil
}

func (q *fakeQuerier) InsertFile(_ context.Context, arg database.InsertFileParams) (database.File, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	file := database.File{
		Hash:      arg.Hash,
		CreatedAt: arg.CreatedAt,
		CreatedBy: arg.CreatedBy,
		Mimetype:  arg.Mimetype,
		Data:      arg.Data,
	}
	q.files = append(q.files, file)
	return file, nil
}

func (q *fakeQuerier) InsertOrganization(_ context.Context, arg database.InsertOrganizationParams) (database.Organization, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	organization := database.Organization{
		ID:        arg.ID,
		Name:      arg.Name,
		CreatedAt: arg.CreatedAt,
		UpdatedAt: arg.UpdatedAt,
	}
	q.organizations = append(q.organizations, organization)
	return organization, nil
}

func (q *fakeQuerier) InsertOrganizationMember(_ context.Context, arg database.InsertOrganizationMemberParams) (database.OrganizationMember, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	organizationMember := database.OrganizationMember{
		OrganizationID: arg.OrganizationID,
		UserID:         arg.UserID,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		Roles:          arg.Roles,
	}
	q.organizationMembers = append(q.organizationMembers, organizationMember)
	return organizationMember, nil
}

func (q *fakeQuerier) InsertParameterValue(_ context.Context, arg database.InsertParameterValueParams) (database.ParameterValue, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	parameterValue := database.ParameterValue{
		ID:                arg.ID,
		Name:              arg.Name,
		CreatedAt:         arg.CreatedAt,
		UpdatedAt:         arg.UpdatedAt,
		Scope:             arg.Scope,
		ScopeID:           arg.ScopeID,
		SourceScheme:      arg.SourceScheme,
		SourceValue:       arg.SourceValue,
		DestinationScheme: arg.DestinationScheme,
	}
	q.parameterValue = append(q.parameterValue, parameterValue)
	return parameterValue, nil
}

func (q *fakeQuerier) InsertTemplate(_ context.Context, arg database.InsertTemplateParams) (database.Template, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	template := database.Template{
		ID:              arg.ID,
		CreatedAt:       arg.CreatedAt,
		UpdatedAt:       arg.UpdatedAt,
		OrganizationID:  arg.OrganizationID,
		Name:            arg.Name,
		Provisioner:     arg.Provisioner,
		ActiveVersionID: arg.ActiveVersionID,
	}
	q.template = append(q.template, template)
	return template, nil
}

func (q *fakeQuerier) InsertTemplateVersion(_ context.Context, arg database.InsertTemplateVersionParams) (database.TemplateVersion, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	version := database.TemplateVersion{
		ID:             arg.ID,
		TemplateID:     arg.TemplateID,
		OrganizationID: arg.OrganizationID,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		Name:           arg.Name,
		Description:    arg.Description,
		JobID:          arg.JobID,
	}
	q.templateVersion = append(q.templateVersion, version)
	return version, nil
}

func (q *fakeQuerier) InsertProvisionerJobLogs(_ context.Context, arg database.InsertProvisionerJobLogsParams) ([]database.ProvisionerJobLog, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	logs := make([]database.ProvisionerJobLog, 0)
	for index, output := range arg.Output {
		logs = append(logs, database.ProvisionerJobLog{
			JobID:     arg.JobID,
			ID:        arg.ID[index],
			CreatedAt: arg.CreatedAt[index],
			Source:    arg.Source[index],
			Level:     arg.Level[index],
			Stage:     arg.Stage[index],
			Output:    output,
		})
	}
	q.provisionerJobLog = append(q.provisionerJobLog, logs...)
	return logs, nil
}

func (q *fakeQuerier) InsertParameterSchema(_ context.Context, arg database.InsertParameterSchemaParams) (database.ParameterSchema, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	param := database.ParameterSchema{
		ID:                       arg.ID,
		CreatedAt:                arg.CreatedAt,
		JobID:                    arg.JobID,
		Name:                     arg.Name,
		Description:              arg.Description,
		DefaultSourceScheme:      arg.DefaultSourceScheme,
		DefaultSourceValue:       arg.DefaultSourceValue,
		AllowOverrideSource:      arg.AllowOverrideSource,
		DefaultDestinationScheme: arg.DefaultDestinationScheme,
		AllowOverrideDestination: arg.AllowOverrideDestination,
		DefaultRefresh:           arg.DefaultRefresh,
		RedisplayValue:           arg.RedisplayValue,
		ValidationError:          arg.ValidationError,
		ValidationCondition:      arg.ValidationCondition,
		ValidationTypeSystem:     arg.ValidationTypeSystem,
		ValidationValueType:      arg.ValidationValueType,
	}
	q.parameterSchema = append(q.parameterSchema, param)
	return param, nil
}

func (q *fakeQuerier) InsertProvisionerDaemon(_ context.Context, arg database.InsertProvisionerDaemonParams) (database.ProvisionerDaemon, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	daemon := database.ProvisionerDaemon{
		ID:             arg.ID,
		CreatedAt:      arg.CreatedAt,
		OrganizationID: arg.OrganizationID,
		Name:           arg.Name,
		Provisioners:   arg.Provisioners,
	}
	q.provisionerDaemons = append(q.provisionerDaemons, daemon)
	return daemon, nil
}

func (q *fakeQuerier) InsertProvisionerJob(_ context.Context, arg database.InsertProvisionerJobParams) (database.ProvisionerJob, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	job := database.ProvisionerJob{
		ID:             arg.ID,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		OrganizationID: arg.OrganizationID,
		InitiatorID:    arg.InitiatorID,
		Provisioner:    arg.Provisioner,
		StorageMethod:  arg.StorageMethod,
		StorageSource:  arg.StorageSource,
		Type:           arg.Type,
		Input:          arg.Input,
	}
	q.provisionerJobs = append(q.provisionerJobs, job)
	return job, nil
}

func (q *fakeQuerier) InsertWorkspaceAgent(_ context.Context, arg database.InsertWorkspaceAgentParams) (database.WorkspaceAgent, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	agent := database.WorkspaceAgent{
		ID:                   arg.ID,
		CreatedAt:            arg.CreatedAt,
		UpdatedAt:            arg.UpdatedAt,
		ResourceID:           arg.ResourceID,
		AuthToken:            arg.AuthToken,
		AuthInstanceID:       arg.AuthInstanceID,
		EnvironmentVariables: arg.EnvironmentVariables,
		Name:                 arg.Name,
		Architecture:         arg.Architecture,
		OperatingSystem:      arg.OperatingSystem,
		StartupScript:        arg.StartupScript,
		InstanceMetadata:     arg.InstanceMetadata,
		ResourceMetadata:     arg.ResourceMetadata,
	}
	q.provisionerJobAgent = append(q.provisionerJobAgent, agent)
	return agent, nil
}

func (q *fakeQuerier) InsertWorkspaceResource(_ context.Context, arg database.InsertWorkspaceResourceParams) (database.WorkspaceResource, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	resource := database.WorkspaceResource{
		ID:         arg.ID,
		CreatedAt:  arg.CreatedAt,
		JobID:      arg.JobID,
		Transition: arg.Transition,
		Type:       arg.Type,
		Name:       arg.Name,
	}
	q.provisionerJobResource = append(q.provisionerJobResource, resource)
	return resource, nil
}

func (q *fakeQuerier) InsertUser(_ context.Context, arg database.InsertUserParams) (database.User, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	user := database.User{
		ID:             arg.ID,
		Email:          arg.Email,
		HashedPassword: arg.HashedPassword,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		Username:       arg.Username,
		Status:         database.UserStatusActive,
		RBACRoles:      arg.RBACRoles,
	}
	q.users = append(q.users, user)
	return user, nil
}

func (q *fakeQuerier) UpdateUserRoles(_ context.Context, arg database.UpdateUserRolesParams) (database.User, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, user := range q.users {
		if user.ID != arg.ID {
			continue
		}

		// Set new roles
		user.RBACRoles = arg.GrantedRoles
		// Remove duplicates and sort
		uniqueRoles := make([]string, 0, len(user.RBACRoles))
		exist := make(map[string]struct{})
		for _, r := range user.RBACRoles {
			if _, ok := exist[r]; ok {
				continue
			}
			exist[r] = struct{}{}
			uniqueRoles = append(uniqueRoles, r)
		}
		sort.Strings(uniqueRoles)
		user.RBACRoles = uniqueRoles

		q.users[index] = user
		return user, nil
	}
	return database.User{}, sql.ErrNoRows
}

func (q *fakeQuerier) UpdateUserProfile(_ context.Context, arg database.UpdateUserProfileParams) (database.User, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, user := range q.users {
		if user.ID != arg.ID {
			continue
		}
		user.Email = arg.Email
		user.Username = arg.Username
		q.users[index] = user
		return user, nil
	}
	return database.User{}, sql.ErrNoRows
}

func (q *fakeQuerier) UpdateUserStatus(_ context.Context, arg database.UpdateUserStatusParams) (database.User, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, user := range q.users {
		if user.ID != arg.ID {
			continue
		}
		user.Status = arg.Status
		user.UpdatedAt = arg.UpdatedAt
		q.users[index] = user
		return user, nil
	}
	return database.User{}, sql.ErrNoRows
}

func (q *fakeQuerier) InsertWorkspace(_ context.Context, arg database.InsertWorkspaceParams) (database.Workspace, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	workspace := database.Workspace{
		ID:             arg.ID,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		OwnerID:        arg.OwnerID,
		OrganizationID: arg.OrganizationID,
		TemplateID:     arg.TemplateID,
		Name:           arg.Name,
	}
	q.workspaces = append(q.workspaces, workspace)
	return workspace, nil
}

func (q *fakeQuerier) InsertWorkspaceBuild(_ context.Context, arg database.InsertWorkspaceBuildParams) (database.WorkspaceBuild, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	workspaceBuild := database.WorkspaceBuild{
		ID:                arg.ID,
		CreatedAt:         arg.CreatedAt,
		UpdatedAt:         arg.UpdatedAt,
		WorkspaceID:       arg.WorkspaceID,
		Name:              arg.Name,
		TemplateVersionID: arg.TemplateVersionID,
		BeforeID:          arg.BeforeID,
		Transition:        arg.Transition,
		InitiatorID:       arg.InitiatorID,
		JobID:             arg.JobID,
		ProvisionerState:  arg.ProvisionerState,
	}
	q.workspaceBuild = append(q.workspaceBuild, workspaceBuild)
	return workspaceBuild, nil
}

func (q *fakeQuerier) UpdateAPIKeyByID(_ context.Context, arg database.UpdateAPIKeyByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, apiKey := range q.apiKeys {
		if apiKey.ID != arg.ID {
			continue
		}
		apiKey.LastUsed = arg.LastUsed
		apiKey.ExpiresAt = arg.ExpiresAt
		apiKey.OAuthAccessToken = arg.OAuthAccessToken
		apiKey.OAuthRefreshToken = arg.OAuthRefreshToken
		apiKey.OAuthExpiry = arg.OAuthExpiry
		q.apiKeys[index] = apiKey
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateTemplateActiveVersionByID(_ context.Context, arg database.UpdateTemplateActiveVersionByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, template := range q.template {
		if template.ID.String() != arg.ID.String() {
			continue
		}
		template.ActiveVersionID = arg.ActiveVersionID
		q.template[index] = template
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateTemplateDeletedByID(_ context.Context, arg database.UpdateTemplateDeletedByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, template := range q.template {
		if template.ID.String() != arg.ID.String() {
			continue
		}
		template.Deleted = arg.Deleted
		q.template[index] = template
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateTemplateVersionByID(_ context.Context, arg database.UpdateTemplateVersionByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, templateVersion := range q.templateVersion {
		if templateVersion.ID.String() != arg.ID.String() {
			continue
		}
		templateVersion.TemplateID = arg.TemplateID
		templateVersion.UpdatedAt = arg.UpdatedAt
		q.templateVersion[index] = templateVersion
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateProvisionerDaemonByID(_ context.Context, arg database.UpdateProvisionerDaemonByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, daemon := range q.provisionerDaemons {
		if arg.ID.String() != daemon.ID.String() {
			continue
		}
		daemon.UpdatedAt = arg.UpdatedAt
		daemon.Provisioners = arg.Provisioners
		q.provisionerDaemons[index] = daemon
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceAgentConnectionByID(_ context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, agent := range q.provisionerJobAgent {
		if agent.ID.String() != arg.ID.String() {
			continue
		}
		agent.FirstConnectedAt = arg.FirstConnectedAt
		agent.LastConnectedAt = arg.LastConnectedAt
		agent.DisconnectedAt = arg.DisconnectedAt
		q.provisionerJobAgent[index] = agent
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateProvisionerJobByID(_ context.Context, arg database.UpdateProvisionerJobByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, job := range q.provisionerJobs {
		if arg.ID.String() != job.ID.String() {
			continue
		}
		job.UpdatedAt = arg.UpdatedAt
		q.provisionerJobs[index] = job
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateProvisionerJobWithCancelByID(_ context.Context, arg database.UpdateProvisionerJobWithCancelByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, job := range q.provisionerJobs {
		if arg.ID.String() != job.ID.String() {
			continue
		}
		job.CanceledAt = arg.CanceledAt
		q.provisionerJobs[index] = job
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateProvisionerJobWithCompleteByID(_ context.Context, arg database.UpdateProvisionerJobWithCompleteByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, job := range q.provisionerJobs {
		if arg.ID.String() != job.ID.String() {
			continue
		}
		job.UpdatedAt = arg.UpdatedAt
		job.CompletedAt = arg.CompletedAt
		job.Error = arg.Error
		q.provisionerJobs[index] = job
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceAutostart(_ context.Context, arg database.UpdateWorkspaceAutostartParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, workspace := range q.workspaces {
		if workspace.ID.String() != arg.ID.String() {
			continue
		}
		workspace.AutostartSchedule = arg.AutostartSchedule
		q.workspaces[index] = workspace
		return nil
	}

	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceAutostop(_ context.Context, arg database.UpdateWorkspaceAutostopParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, workspace := range q.workspaces {
		if workspace.ID.String() != arg.ID.String() {
			continue
		}
		workspace.AutostopSchedule = arg.AutostopSchedule
		q.workspaces[index] = workspace
		return nil
	}

	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceBuildByID(_ context.Context, arg database.UpdateWorkspaceBuildByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, workspaceBuild := range q.workspaceBuild {
		if workspaceBuild.ID.String() != arg.ID.String() {
			continue
		}
		workspaceBuild.UpdatedAt = arg.UpdatedAt
		workspaceBuild.AfterID = arg.AfterID
		workspaceBuild.ProvisionerState = arg.ProvisionerState
		q.workspaceBuild[index] = workspaceBuild
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceDeletedByID(_ context.Context, arg database.UpdateWorkspaceDeletedByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, workspace := range q.workspaces {
		if workspace.ID.String() != arg.ID.String() {
			continue
		}
		workspace.Deleted = arg.Deleted
		q.workspaces[index] = workspace
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) InsertGitSSHKey(_ context.Context, arg database.InsertGitSSHKeyParams) (database.GitSSHKey, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	gitSSHKey := database.GitSSHKey{
		UserID:     arg.UserID,
		CreatedAt:  arg.CreatedAt,
		UpdatedAt:  arg.UpdatedAt,
		PrivateKey: arg.PrivateKey,
		PublicKey:  arg.PublicKey,
	}
	q.GitSSHKey = append(q.GitSSHKey, gitSSHKey)
	return gitSSHKey, nil
}

func (q *fakeQuerier) GetGitSSHKey(_ context.Context, userID uuid.UUID) (database.GitSSHKey, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, key := range q.GitSSHKey {
		if key.UserID == userID {
			return key, nil
		}
	}
	return database.GitSSHKey{}, sql.ErrNoRows
}

func (q *fakeQuerier) UpdateGitSSHKey(_ context.Context, arg database.UpdateGitSSHKeyParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, key := range q.GitSSHKey {
		if key.UserID.String() != arg.UserID.String() {
			continue
		}
		key.UpdatedAt = arg.UpdatedAt
		key.PrivateKey = arg.PrivateKey
		key.PublicKey = arg.PublicKey
		q.GitSSHKey[index] = key
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) DeleteGitSSHKey(_ context.Context, userID uuid.UUID) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, key := range q.GitSSHKey {
		if key.UserID.String() != userID.String() {
			continue
		}
		q.GitSSHKey[index] = q.GitSSHKey[len(q.GitSSHKey)-1]
		q.GitSSHKey = q.GitSSHKey[:len(q.GitSSHKey)-1]
		return nil
	}
	return sql.ErrNoRows
}
