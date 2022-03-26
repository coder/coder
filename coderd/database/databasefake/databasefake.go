package databasefake

import (
	"context"
	"database/sql"
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
		project:                make([]database.Project, 0),
		projectVersion:         make([]database.ProjectVersion, 0),
		provisionerDaemons:     make([]database.ProvisionerDaemon, 0),
		provisionerJobs:        make([]database.ProvisionerJob, 0),
		provisionerJobLog:      make([]database.ProvisionerJobLog, 0),
		workspace:              make([]database.Workspace, 0),
		provisionerJobResource: make([]database.WorkspaceResource, 0),
		workspaceBuild:         make([]database.WorkspaceBuild, 0),
		provisionerJobAgent:    make([]database.WorkspaceAgent, 0),
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
	project                []database.Project
	projectVersion         []database.ProjectVersion
	provisionerDaemons     []database.ProvisionerDaemon
	provisionerJobs        []database.ProvisionerJob
	provisionerJobAgent    []database.WorkspaceAgent
	provisionerJobResource []database.WorkspaceResource
	provisionerJobLog      []database.ProvisionerJobLog
	workspace              []database.Workspace
	workspaceBuild         []database.WorkspaceBuild
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

func (q *fakeQuerier) GetUserByID(_ context.Context, id string) (database.User, error) {
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

func (q *fakeQuerier) GetWorkspacesByProjectID(_ context.Context, arg database.GetWorkspacesByProjectIDParams) ([]database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaces := make([]database.Workspace, 0)
	for _, workspace := range q.workspace {
		if workspace.ProjectID.String() != arg.ProjectID.String() {
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

	for _, workspace := range q.workspace {
		if workspace.ID.String() == id.String() {
			return workspace, nil
		}
	}
	return database.Workspace{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceByUserIDAndName(_ context.Context, arg database.GetWorkspaceByUserIDAndNameParams) (database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspace := range q.workspace {
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

func (q *fakeQuerier) GetWorkspaceOwnerCountsByProjectIDs(_ context.Context, projectIDs []uuid.UUID) ([]database.GetWorkspaceOwnerCountsByProjectIDsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	counts := map[string]map[string]struct{}{}
	for _, projectID := range projectIDs {
		found := false
		for _, workspace := range q.workspace {
			if workspace.ProjectID.String() != projectID.String() {
				continue
			}
			if workspace.Deleted {
				continue
			}
			countByOwnerID, ok := counts[projectID.String()]
			if !ok {
				countByOwnerID = map[string]struct{}{}
			}
			countByOwnerID[workspace.OwnerID] = struct{}{}
			counts[projectID.String()] = countByOwnerID
			found = true
			break
		}
		if !found {
			counts[projectID.String()] = map[string]struct{}{}
		}
	}
	res := make([]database.GetWorkspaceOwnerCountsByProjectIDsRow, 0)
	for key, value := range counts {
		uid := uuid.MustParse(key)
		res = append(res, database.GetWorkspaceOwnerCountsByProjectIDsRow{
			ProjectID: uid,
			Count:     int64(len(value)),
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

func (q *fakeQuerier) GetWorkspacesByUserID(_ context.Context, req database.GetWorkspacesByUserIDParams) ([]database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaces := make([]database.Workspace, 0)
	for _, workspace := range q.workspace {
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

func (q *fakeQuerier) GetOrganizationByID(_ context.Context, id string) (database.Organization, error) {
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

func (q *fakeQuerier) GetOrganizationsByUserID(_ context.Context, userID string) ([]database.Organization, error) {
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

func (q *fakeQuerier) GetProjectByID(_ context.Context, id uuid.UUID) (database.Project, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, project := range q.project {
		if project.ID.String() == id.String() {
			return project, nil
		}
	}
	return database.Project{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetProjectByOrganizationAndName(_ context.Context, arg database.GetProjectByOrganizationAndNameParams) (database.Project, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, project := range q.project {
		if project.OrganizationID != arg.OrganizationID {
			continue
		}
		if !strings.EqualFold(project.Name, arg.Name) {
			continue
		}
		if project.Deleted != arg.Deleted {
			continue
		}
		return project, nil
	}
	return database.Project{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetProjectVersionsByProjectID(_ context.Context, projectID uuid.UUID) ([]database.ProjectVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	version := make([]database.ProjectVersion, 0)
	for _, projectVersion := range q.projectVersion {
		if projectVersion.ProjectID.UUID.String() != projectID.String() {
			continue
		}
		version = append(version, projectVersion)
	}
	if len(version) == 0 {
		return nil, sql.ErrNoRows
	}
	return version, nil
}

func (q *fakeQuerier) GetProjectVersionByProjectIDAndName(_ context.Context, arg database.GetProjectVersionByProjectIDAndNameParams) (database.ProjectVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, projectVersion := range q.projectVersion {
		if projectVersion.ProjectID.UUID.String() != arg.ProjectID.UUID.String() {
			continue
		}
		if !strings.EqualFold(projectVersion.Name, arg.Name) {
			continue
		}
		return projectVersion, nil
	}
	return database.ProjectVersion{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetProjectVersionByID(_ context.Context, projectVersionID uuid.UUID) (database.ProjectVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, projectVersion := range q.projectVersion {
		if projectVersion.ID.String() != projectVersionID.String() {
			continue
		}
		return projectVersion, nil
	}
	return database.ProjectVersion{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetProjectVersionByJobID(_ context.Context, jobID uuid.UUID) (database.ProjectVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, projectVersion := range q.projectVersion {
		if projectVersion.JobID.String() != jobID.String() {
			continue
		}
		return projectVersion, nil
	}
	return database.ProjectVersion{}, sql.ErrNoRows
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

func (q *fakeQuerier) GetProjectsByOrganization(_ context.Context, arg database.GetProjectsByOrganizationParams) ([]database.Project, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	projects := make([]database.Project, 0)
	for _, project := range q.project {
		if project.Deleted != arg.Deleted {
			continue
		}
		if project.OrganizationID != arg.OrganizationID {
			continue
		}
		projects = append(projects, project)
	}
	if len(projects) == 0 {
		return nil, sql.ErrNoRows
	}
	return projects, nil
}

func (q *fakeQuerier) GetProjectsByIDs(_ context.Context, ids []uuid.UUID) ([]database.Project, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	projects := make([]database.Project, 0)
	for _, project := range q.project {
		for _, id := range ids {
			if project.ID.String() != id.String() {
				continue
			}
			projects = append(projects, project)
		}
	}
	if len(projects) == 0 {
		return nil, sql.ErrNoRows
	}
	return projects, nil
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

	for _, agent := range q.provisionerJobAgent {
		if agent.AuthToken.String() == authToken.String() {
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

func (q *fakeQuerier) GetWorkspaceAgentByResourceID(_ context.Context, resourceID uuid.UUID) (database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, agent := range q.provisionerJobAgent {
		if agent.ResourceID.String() == resourceID.String() {
			return agent, nil
		}
	}
	return database.WorkspaceAgent{}, sql.ErrNoRows
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
		ID:               arg.ID,
		HashedSecret:     arg.HashedSecret,
		UserID:           arg.UserID,
		Application:      arg.Application,
		Name:             arg.Name,
		LastUsed:         arg.LastUsed,
		ExpiresAt:        arg.ExpiresAt,
		CreatedAt:        arg.CreatedAt,
		UpdatedAt:        arg.UpdatedAt,
		LoginType:        arg.LoginType,
		OIDCAccessToken:  arg.OIDCAccessToken,
		OIDCRefreshToken: arg.OIDCRefreshToken,
		OIDCIDToken:      arg.OIDCIDToken,
		OIDCExpiry:       arg.OIDCExpiry,
		DevurlToken:      arg.DevurlToken,
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

func (q *fakeQuerier) InsertProject(_ context.Context, arg database.InsertProjectParams) (database.Project, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	project := database.Project{
		ID:              arg.ID,
		CreatedAt:       arg.CreatedAt,
		UpdatedAt:       arg.UpdatedAt,
		OrganizationID:  arg.OrganizationID,
		Name:            arg.Name,
		Provisioner:     arg.Provisioner,
		ActiveVersionID: arg.ActiveVersionID,
	}
	q.project = append(q.project, project)
	return project, nil
}

func (q *fakeQuerier) InsertProjectVersion(_ context.Context, arg database.InsertProjectVersionParams) (database.ProjectVersion, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	version := database.ProjectVersion{
		ID:             arg.ID,
		ProjectID:      arg.ProjectID,
		OrganizationID: arg.OrganizationID,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		Name:           arg.Name,
		Description:    arg.Description,
		JobID:          arg.JobID,
	}
	q.projectVersion = append(q.projectVersion, version)
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
		Address:    arg.Address,
		Type:       arg.Type,
		Name:       arg.Name,
		AgentID:    arg.AgentID,
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
		Name:           arg.Name,
		LoginType:      arg.LoginType,
		HashedPassword: arg.HashedPassword,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		Username:       arg.Username,
	}
	q.users = append(q.users, user)
	return user, nil
}

func (q *fakeQuerier) InsertWorkspace(_ context.Context, arg database.InsertWorkspaceParams) (database.Workspace, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	workspace := database.Workspace{
		ID:        arg.ID,
		CreatedAt: arg.CreatedAt,
		UpdatedAt: arg.UpdatedAt,
		OwnerID:   arg.OwnerID,
		ProjectID: arg.ProjectID,
		Name:      arg.Name,
	}
	q.workspace = append(q.workspace, workspace)
	return workspace, nil
}

func (q *fakeQuerier) InsertWorkspaceBuild(_ context.Context, arg database.InsertWorkspaceBuildParams) (database.WorkspaceBuild, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	workspaceBuild := database.WorkspaceBuild{
		ID:               arg.ID,
		CreatedAt:        arg.CreatedAt,
		UpdatedAt:        arg.UpdatedAt,
		WorkspaceID:      arg.WorkspaceID,
		Name:             arg.Name,
		ProjectVersionID: arg.ProjectVersionID,
		BeforeID:         arg.BeforeID,
		Transition:       arg.Transition,
		Initiator:        arg.Initiator,
		JobID:            arg.JobID,
		ProvisionerState: arg.ProvisionerState,
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
		apiKey.OIDCAccessToken = arg.OIDCAccessToken
		apiKey.OIDCRefreshToken = arg.OIDCRefreshToken
		apiKey.OIDCExpiry = arg.OIDCExpiry
		q.apiKeys[index] = apiKey
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateProjectActiveVersionByID(_ context.Context, arg database.UpdateProjectActiveVersionByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, project := range q.project {
		if project.ID.String() != arg.ID.String() {
			continue
		}
		project.ActiveVersionID = arg.ActiveVersionID
		q.project[index] = project
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateProjectDeletedByID(_ context.Context, arg database.UpdateProjectDeletedByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, project := range q.project {
		if project.ID.String() != arg.ID.String() {
			continue
		}
		project.Deleted = arg.Deleted
		q.project[index] = project
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateProjectVersionByID(_ context.Context, arg database.UpdateProjectVersionByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, projectVersion := range q.projectVersion {
		if projectVersion.ID.String() != arg.ID.String() {
			continue
		}
		projectVersion.ProjectID = arg.ProjectID
		projectVersion.UpdatedAt = arg.UpdatedAt
		q.projectVersion[index] = projectVersion
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

	for index, workspace := range q.workspace {
		if workspace.ID.String() != arg.ID.String() {
			continue
		}
		workspace.Deleted = arg.Deleted
		q.workspace[index] = workspace
		return nil
	}
	return sql.ErrNoRows
}
