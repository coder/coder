package databasefake

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

// New returns an in-memory fake of the database.
func New() database.Store {
	return &fakeQuerier{
		mutex: &sync.RWMutex{},
		data: &data{
			apiKeys:             make([]database.APIKey, 0),
			agentStats:          make([]database.AgentStat, 0),
			organizationMembers: make([]database.OrganizationMember, 0),
			organizations:       make([]database.Organization, 0),
			users:               make([]database.User, 0),

			auditLogs:                      make([]database.AuditLog, 0),
			files:                          make([]database.File, 0),
			gitSSHKey:                      make([]database.GitSSHKey, 0),
			parameterSchemas:               make([]database.ParameterSchema, 0),
			parameterValues:                make([]database.ParameterValue, 0),
			provisionerDaemons:             make([]database.ProvisionerDaemon, 0),
			provisionerJobAgents:           make([]database.WorkspaceAgent, 0),
			provisionerJobLogs:             make([]database.ProvisionerJobLog, 0),
			provisionerJobResources:        make([]database.WorkspaceResource, 0),
			provisionerJobResourceMetadata: make([]database.WorkspaceResourceMetadatum, 0),
			provisionerJobs:                make([]database.ProvisionerJob, 0),
			templateVersions:               make([]database.TemplateVersion, 0),
			templates:                      make([]database.Template, 0),
			workspaceBuilds:                make([]database.WorkspaceBuild, 0),
			workspaceApps:                  make([]database.WorkspaceApp, 0),
			workspaces:                     make([]database.Workspace, 0),
			licenses:                       make([]database.License, 0),
		},
	}
}

type rwMutex interface {
	Lock()
	RLock()
	Unlock()
	RUnlock()
}

// inTxMutex is a no op, since inside a transaction we are already locked.
type inTxMutex struct{}

func (inTxMutex) Lock()    {}
func (inTxMutex) RLock()   {}
func (inTxMutex) Unlock()  {}
func (inTxMutex) RUnlock() {}

// fakeQuerier replicates database functionality to enable quick testing.
type fakeQuerier struct {
	mutex rwMutex
	*data
}

type data struct {
	// Legacy tables
	apiKeys             []database.APIKey
	organizations       []database.Organization
	organizationMembers []database.OrganizationMember
	users               []database.User
	userLinks           []database.UserLink

	// New tables
	agentStats                     []database.AgentStat
	auditLogs                      []database.AuditLog
	files                          []database.File
	gitSSHKey                      []database.GitSSHKey
	parameterSchemas               []database.ParameterSchema
	parameterValues                []database.ParameterValue
	provisionerDaemons             []database.ProvisionerDaemon
	provisionerJobAgents           []database.WorkspaceAgent
	provisionerJobLogs             []database.ProvisionerJobLog
	provisionerJobResources        []database.WorkspaceResource
	provisionerJobResourceMetadata []database.WorkspaceResourceMetadatum
	provisionerJobs                []database.ProvisionerJob
	templateVersions               []database.TemplateVersion
	templates                      []database.Template
	workspaceBuilds                []database.WorkspaceBuild
	workspaceApps                  []database.WorkspaceApp
	workspaces                     []database.Workspace
	licenses                       []database.License

	deploymentID  string
	lastLicenseID int32
}

// InTx doesn't rollback data properly for in-memory yet.
func (q *fakeQuerier) InTx(fn func(database.Store) error) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return fn(&fakeQuerier{mutex: inTxMutex{}, data: q.data})
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
func (*fakeQuerier) DeleteOldAgentStats(_ context.Context) error {
	// no-op
	return nil
}

func (q *fakeQuerier) InsertAgentStat(_ context.Context, p database.InsertAgentStatParams) (database.AgentStat, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	stat := database.AgentStat{
		ID:          p.ID,
		CreatedAt:   p.CreatedAt,
		WorkspaceID: p.WorkspaceID,
		AgentID:     p.AgentID,
		UserID:      p.UserID,
		Payload:     p.Payload,
		TemplateID:  p.TemplateID,
	}
	q.agentStats = append(q.agentStats, stat)
	return stat, nil
}

func (q *fakeQuerier) GetTemplateDAUs(_ context.Context, templateID uuid.UUID) ([]database.GetTemplateDAUsRow, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	counts := make(map[time.Time]map[string]struct{})

	for _, as := range q.agentStats {
		if as.TemplateID != templateID {
			continue
		}

		date := as.CreatedAt.Truncate(time.Hour * 24)
		dateEntry := counts[date]
		if dateEntry == nil {
			dateEntry = make(map[string]struct{})
		}
		counts[date] = dateEntry

		dateEntry[as.UserID.String()] = struct{}{}
	}

	countKeys := maps.Keys(counts)
	sort.Slice(countKeys, func(i, j int) bool {
		return countKeys[i].Before(countKeys[j])
	})

	var rs []database.GetTemplateDAUsRow
	for _, key := range countKeys {
		rs = append(rs, database.GetTemplateDAUsRow{
			Date:   key,
			Amount: int64(len(counts[key])),
		})
	}

	return rs, nil
}

func (q *fakeQuerier) ParameterValue(_ context.Context, id uuid.UUID) (database.ParameterValue, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, parameterValue := range q.parameterValues {
		if parameterValue.ID.String() != id.String() {
			continue
		}
		return parameterValue, nil
	}
	return database.ParameterValue{}, sql.ErrNoRows
}

func (q *fakeQuerier) DeleteParameterValueByID(_ context.Context, id uuid.UUID) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, parameterValue := range q.parameterValues {
		if parameterValue.ID.String() != id.String() {
			continue
		}
		q.parameterValues[index] = q.parameterValues[len(q.parameterValues)-1]
		q.parameterValues = q.parameterValues[:len(q.parameterValues)-1]
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

func (q *fakeQuerier) GetAPIKeysLastUsedAfter(_ context.Context, after time.Time) ([]database.APIKey, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	apiKeys := make([]database.APIKey, 0)
	for _, key := range q.apiKeys {
		if key.LastUsed.After(after) {
			apiKeys = append(apiKeys, key)
		}
	}
	return apiKeys, nil
}

func (q *fakeQuerier) DeleteAPIKeyByID(_ context.Context, id string) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, apiKey := range q.apiKeys {
		if apiKey.ID != id {
			continue
		}
		q.apiKeys[index] = q.apiKeys[len(q.apiKeys)-1]
		q.apiKeys = q.apiKeys[:len(q.apiKeys)-1]
		return nil
	}
	return sql.ErrNoRows
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

func (q *fakeQuerier) GetActiveUserCount(_ context.Context) (int64, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	active := int64(0)
	for _, u := range q.users {
		if u.Status == database.UserStatusActive {
			active++
		}
	}
	return active, nil
}

func (q *fakeQuerier) GetUsers(_ context.Context, params database.GetUsersParams) ([]database.User, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// Avoid side-effect of sorting.
	users := make([]database.User, len(q.users))
	copy(users, q.users)

	// Database orders by created_at
	slices.SortFunc(users, func(a, b database.User) bool {
		if a.CreatedAt.Equal(b.CreatedAt) {
			// Technically the postgres database also orders by uuid. So match
			// that behavior
			return a.ID.String() < b.ID.String()
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})

	if params.AfterID != uuid.Nil {
		found := false
		for i, v := range users {
			if v.ID == params.AfterID {
				// We want to return all users after index i.
				users = users[i+1:]
				found = true
				break
			}
		}

		// If no users after the time, then we return an empty list.
		if !found {
			return nil, sql.ErrNoRows
		}
	}

	if params.Search != "" {
		tmp := make([]database.User, 0, len(users))
		for i, user := range users {
			if strings.Contains(strings.ToLower(user.Email), strings.ToLower(params.Search)) {
				tmp = append(tmp, users[i])
			} else if strings.Contains(strings.ToLower(user.Username), strings.ToLower(params.Search)) {
				tmp = append(tmp, users[i])
			}
		}
		users = tmp
	}

	if len(params.Status) > 0 {
		usersFilteredByStatus := make([]database.User, 0, len(users))
		for i, user := range users {
			if slice.ContainsCompare(params.Status, user.Status, func(a, b database.UserStatus) bool {
				return strings.EqualFold(string(a), string(b))
			}) {
				usersFilteredByStatus = append(usersFilteredByStatus, users[i])
			}
		}
		users = usersFilteredByStatus
	}

	if len(params.RbacRole) > 0 && !slice.Contains(params.RbacRole, rbac.RoleMember()) {
		usersFilteredByRole := make([]database.User, 0, len(users))
		for i, user := range users {
			if slice.OverlapCompare(params.RbacRole, user.RBACRoles, strings.EqualFold) {
				usersFilteredByRole = append(usersFilteredByRole, users[i])
			}
		}
		users = usersFilteredByRole
	}

	if params.OffsetOpt > 0 {
		if int(params.OffsetOpt) > len(users)-1 {
			return nil, sql.ErrNoRows
		}
		users = users[params.OffsetOpt:]
	}

	if params.LimitOpt > 0 {
		if int(params.LimitOpt) > len(users) {
			params.LimitOpt = int32(len(users))
		}
		users = users[:params.LimitOpt]
	}

	return users, nil
}

func (q *fakeQuerier) GetUsersByIDs(_ context.Context, ids []uuid.UUID) ([]database.User, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	users := make([]database.User, 0)
	for _, user := range q.users {
		for _, id := range ids {
			if user.ID.String() != id.String() {
				continue
			}
			users = append(users, user)
		}
	}
	return users, nil
}

func (q *fakeQuerier) GetAuthorizationUserRoles(_ context.Context, userID uuid.UUID) (database.GetAuthorizationUserRolesRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var user *database.User
	roles := make([]string, 0)
	for _, u := range q.users {
		if u.ID == userID {
			u := u
			roles = append(roles, u.RBACRoles...)
			roles = append(roles, "member")
			user = &u
			break
		}
	}

	for _, mem := range q.organizationMembers {
		if mem.UserID == userID {
			roles = append(roles, mem.Roles...)
			roles = append(roles, "organization-member:"+mem.OrganizationID.String())
		}
	}

	if user == nil {
		return database.GetAuthorizationUserRolesRow{}, sql.ErrNoRows
	}

	return database.GetAuthorizationUserRolesRow{
		ID:       userID,
		Username: user.Username,
		Status:   user.Status,
		Roles:    roles,
	}, nil
}

func (q *fakeQuerier) GetWorkspaces(_ context.Context, arg database.GetWorkspacesParams) ([]database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaces := make([]database.Workspace, 0)
	for _, workspace := range q.workspaces {
		if arg.OwnerID != uuid.Nil && workspace.OwnerID != arg.OwnerID {
			continue
		}
		if arg.OwnerUsername != "" {
			owner, err := q.GetUserByID(context.Background(), workspace.OwnerID)
			if err == nil && !strings.EqualFold(arg.OwnerUsername, owner.Username) {
				continue
			}
		}
		if arg.TemplateName != "" {
			template, err := q.GetTemplateByID(context.Background(), workspace.TemplateID)
			if err == nil && !strings.EqualFold(arg.TemplateName, template.Name) {
				continue
			}
		}
		if !arg.Deleted && workspace.Deleted {
			continue
		}

		if arg.Name != "" && !strings.Contains(strings.ToLower(workspace.Name), strings.ToLower(arg.Name)) {
			continue
		}
		if len(arg.TemplateIds) > 0 {
			match := false
			for _, id := range arg.TemplateIds {
				if workspace.TemplateID == id {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		workspaces = append(workspaces, workspace)
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

	var found *database.Workspace
	for _, workspace := range q.workspaces {
		workspace := workspace
		if workspace.OwnerID != arg.OwnerID {
			continue
		}
		if !strings.EqualFold(workspace.Name, arg.Name) {
			continue
		}
		if workspace.Deleted != arg.Deleted {
			continue
		}

		// Return the most recent workspace with the given name
		if found == nil || workspace.CreatedAt.After(found.CreatedAt) {
			found = &workspace
		}
	}
	if found != nil {
		return *found, nil
	}
	return database.Workspace{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceAppsByAgentID(_ context.Context, id uuid.UUID) ([]database.WorkspaceApp, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	apps := make([]database.WorkspaceApp, 0)
	for _, app := range q.workspaceApps {
		if app.AgentID == id {
			apps = append(apps, app)
		}
	}
	if len(apps) == 0 {
		return nil, sql.ErrNoRows
	}
	return apps, nil
}

func (q *fakeQuerier) GetWorkspaceAppsCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceApp, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	apps := make([]database.WorkspaceApp, 0)
	for _, app := range q.workspaceApps {
		if app.CreatedAt.After(after) {
			apps = append(apps, app)
		}
	}
	return apps, nil
}

func (q *fakeQuerier) GetWorkspaceAppsByAgentIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceApp, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	apps := make([]database.WorkspaceApp, 0)
	for _, app := range q.workspaceApps {
		for _, id := range ids {
			if app.AgentID.String() == id.String() {
				apps = append(apps, app)
				break
			}
		}
	}
	return apps, nil
}

func (q *fakeQuerier) GetWorkspaceOwnerCountsByTemplateIDs(_ context.Context, templateIDs []uuid.UUID) ([]database.GetWorkspaceOwnerCountsByTemplateIDsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	counts := map[uuid.UUID]map[uuid.UUID]struct{}{}
	for _, templateID := range templateIDs {
		counts[templateID] = map[uuid.UUID]struct{}{}
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

	for _, history := range q.workspaceBuilds {
		if history.ID.String() == id.String() {
			return history, nil
		}
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceBuildByJobID(_ context.Context, jobID uuid.UUID) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, build := range q.workspaceBuilds {
		if build.JobID.String() == jobID.String() {
			return build, nil
		}
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetLatestWorkspaceBuildByWorkspaceID(_ context.Context, workspaceID uuid.UUID) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var row database.WorkspaceBuild
	var buildNum int32 = -1
	for _, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.WorkspaceID.String() == workspaceID.String() && workspaceBuild.BuildNumber > buildNum {
			row = workspaceBuild
			buildNum = workspaceBuild.BuildNumber
		}
	}
	if buildNum == -1 {
		return database.WorkspaceBuild{}, sql.ErrNoRows
	}
	return row, nil
}

func (q *fakeQuerier) GetLatestWorkspaceBuilds(_ context.Context) ([]database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	builds := make(map[uuid.UUID]database.WorkspaceBuild)
	buildNumbers := make(map[uuid.UUID]int32)
	for _, workspaceBuild := range q.workspaceBuilds {
		id := workspaceBuild.WorkspaceID
		if workspaceBuild.BuildNumber > buildNumbers[id] {
			builds[id] = workspaceBuild
			buildNumbers[id] = workspaceBuild.BuildNumber
		}
	}
	var returnBuilds []database.WorkspaceBuild
	for i, n := range buildNumbers {
		if n > 0 {
			b := builds[i]
			returnBuilds = append(returnBuilds, b)
		}
	}
	if len(returnBuilds) == 0 {
		return nil, sql.ErrNoRows
	}
	return returnBuilds, nil
}

func (q *fakeQuerier) GetLatestWorkspaceBuildsByWorkspaceIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	builds := make(map[uuid.UUID]database.WorkspaceBuild)
	buildNumbers := make(map[uuid.UUID]int32)
	for _, workspaceBuild := range q.workspaceBuilds {
		for _, id := range ids {
			if id.String() == workspaceBuild.WorkspaceID.String() && workspaceBuild.BuildNumber > buildNumbers[id] {
				builds[id] = workspaceBuild
				buildNumbers[id] = workspaceBuild.BuildNumber
			}
		}
	}
	var returnBuilds []database.WorkspaceBuild
	for i, n := range buildNumbers {
		if n > 0 {
			b := builds[i]
			returnBuilds = append(returnBuilds, b)
		}
	}
	if len(returnBuilds) == 0 {
		return nil, sql.ErrNoRows
	}
	return returnBuilds, nil
}

func (q *fakeQuerier) GetWorkspaceBuildByWorkspaceID(_ context.Context,
	params database.GetWorkspaceBuildByWorkspaceIDParams,
) ([]database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	history := make([]database.WorkspaceBuild, 0)
	for _, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.WorkspaceID.String() == params.WorkspaceID.String() {
			history = append(history, workspaceBuild)
		}
	}

	// Order by build_number
	slices.SortFunc(history, func(a, b database.WorkspaceBuild) bool {
		// use greater than since we want descending order
		return a.BuildNumber > b.BuildNumber
	})

	if params.AfterID != uuid.Nil {
		found := false
		for i, v := range history {
			if v.ID == params.AfterID {
				// We want to return all builds after index i.
				history = history[i+1:]
				found = true
				break
			}
		}

		// If no builds after the time, then we return an empty list.
		if !found {
			return nil, sql.ErrNoRows
		}
	}

	if params.OffsetOpt > 0 {
		if int(params.OffsetOpt) > len(history)-1 {
			return nil, sql.ErrNoRows
		}
		history = history[params.OffsetOpt:]
	}

	if params.LimitOpt > 0 {
		if int(params.LimitOpt) > len(history) {
			params.LimitOpt = int32(len(history))
		}
		history = history[:params.LimitOpt]
	}

	if len(history) == 0 {
		return nil, sql.ErrNoRows
	}
	return history, nil
}

func (q *fakeQuerier) GetWorkspaceBuildByWorkspaceIDAndName(_ context.Context, arg database.GetWorkspaceBuildByWorkspaceIDAndNameParams) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspaceBuild := range q.workspaceBuilds {
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

func (q *fakeQuerier) GetWorkspaceBuildByWorkspaceIDAndBuildNumber(_ context.Context, arg database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.WorkspaceID.String() != arg.WorkspaceID.String() {
			continue
		}
		if workspaceBuild.BuildNumber != arg.BuildNumber {
			continue
		}
		return workspaceBuild, nil
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceBuildsCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaceBuilds := make([]database.WorkspaceBuild, 0)
	for _, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.CreatedAt.After(after) {
			workspaceBuilds = append(workspaceBuilds, workspaceBuild)
		}
	}
	return workspaceBuilds, nil
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

func (q *fakeQuerier) ParameterValues(_ context.Context, arg database.ParameterValuesParams) ([]database.ParameterValue, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	parameterValues := make([]database.ParameterValue, 0)
	for _, parameterValue := range q.parameterValues {
		if len(arg.Scopes) > 0 {
			if !slice.Contains(arg.Scopes, parameterValue.Scope) {
				continue
			}
		}
		if len(arg.ScopeIds) > 0 {
			if !slice.Contains(arg.ScopeIds, parameterValue.ScopeID) {
				continue
			}
		}

		if len(arg.Ids) > 0 {
			if !slice.Contains(arg.Ids, parameterValue.ID) {
				continue
			}
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

	for _, template := range q.templates {
		if template.ID.String() == id.String() {
			return template, nil
		}
	}
	return database.Template{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetTemplateByOrganizationAndName(_ context.Context, arg database.GetTemplateByOrganizationAndNameParams) (database.Template, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, template := range q.templates {
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

func (q *fakeQuerier) UpdateTemplateMetaByID(_ context.Context, arg database.UpdateTemplateMetaByIDParams) error {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for idx, tpl := range q.templates {
		if tpl.ID != arg.ID {
			continue
		}
		tpl.UpdatedAt = database.Now()
		tpl.Name = arg.Name
		tpl.Description = arg.Description
		tpl.Icon = arg.Icon
		tpl.MaxTtl = arg.MaxTtl
		tpl.MinAutostartInterval = arg.MinAutostartInterval
		q.templates[idx] = tpl
		return nil
	}

	return sql.ErrNoRows
}

func (q *fakeQuerier) GetTemplatesWithFilter(_ context.Context, arg database.GetTemplatesWithFilterParams) ([]database.Template, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var templates []database.Template
	for _, template := range q.templates {
		if template.Deleted != arg.Deleted {
			continue
		}
		if arg.OrganizationID != uuid.Nil && template.OrganizationID != arg.OrganizationID {
			continue
		}

		if arg.ExactName != "" && !strings.EqualFold(template.Name, arg.ExactName) {
			continue
		}

		if len(arg.Ids) > 0 {
			match := false
			for _, id := range arg.Ids {
				if template.ID == id {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		templates = append(templates, template)
	}
	if len(templates) > 0 {
		slices.SortFunc(templates, func(i, j database.Template) bool {
			if i.Name != j.Name {
				return i.Name < j.Name
			}
			return i.ID.String() < j.ID.String()
		})
		return templates, nil
	}

	return nil, sql.ErrNoRows
}

func (q *fakeQuerier) GetTemplateVersionsByTemplateID(_ context.Context, arg database.GetTemplateVersionsByTemplateIDParams) (version []database.TemplateVersion, err error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, templateVersion := range q.templateVersions {
		if templateVersion.TemplateID.UUID.String() != arg.TemplateID.String() {
			continue
		}
		version = append(version, templateVersion)
	}

	// Database orders by created_at
	slices.SortFunc(version, func(a, b database.TemplateVersion) bool {
		if a.CreatedAt.Equal(b.CreatedAt) {
			// Technically the postgres database also orders by uuid. So match
			// that behavior
			return a.ID.String() < b.ID.String()
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})

	if arg.AfterID != uuid.Nil {
		found := false
		for i, v := range version {
			if v.ID == arg.AfterID {
				// We want to return all users after index i.
				version = version[i+1:]
				found = true
				break
			}
		}

		// If no users after the time, then we return an empty list.
		if !found {
			return nil, sql.ErrNoRows
		}
	}

	if arg.OffsetOpt > 0 {
		if int(arg.OffsetOpt) > len(version)-1 {
			return nil, sql.ErrNoRows
		}
		version = version[arg.OffsetOpt:]
	}

	if arg.LimitOpt > 0 {
		if int(arg.LimitOpt) > len(version) {
			arg.LimitOpt = int32(len(version))
		}
		version = version[:arg.LimitOpt]
	}

	if len(version) == 0 {
		return nil, sql.ErrNoRows
	}

	return version, nil
}

func (q *fakeQuerier) GetTemplateVersionsCreatedAfter(_ context.Context, after time.Time) ([]database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	versions := make([]database.TemplateVersion, 0)
	for _, version := range q.templateVersions {
		if version.CreatedAt.After(after) {
			versions = append(versions, version)
		}
	}
	return versions, nil
}

func (q *fakeQuerier) GetTemplateVersionByTemplateIDAndName(_ context.Context, arg database.GetTemplateVersionByTemplateIDAndNameParams) (database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, templateVersion := range q.templateVersions {
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

	for _, templateVersion := range q.templateVersions {
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

	for _, templateVersion := range q.templateVersions {
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
	for _, parameterSchema := range q.parameterSchemas {
		if parameterSchema.JobID.String() != jobID.String() {
			continue
		}
		parameters = append(parameters, parameterSchema)
	}
	if len(parameters) == 0 {
		return nil, sql.ErrNoRows
	}
	sort.Slice(parameters, func(i, j int) bool {
		return parameters[i].Index < parameters[j].Index
	})
	return parameters, nil
}

func (q *fakeQuerier) GetParameterSchemasCreatedAfter(_ context.Context, after time.Time) ([]database.ParameterSchema, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	parameters := make([]database.ParameterSchema, 0)
	for _, parameterSchema := range q.parameterSchemas {
		if parameterSchema.CreatedAt.After(after) {
			parameters = append(parameters, parameterSchema)
		}
	}
	return parameters, nil
}

func (q *fakeQuerier) GetParameterValueByScopeAndName(_ context.Context, arg database.GetParameterValueByScopeAndNameParams) (database.ParameterValue, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, parameterValue := range q.parameterValues {
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

func (q *fakeQuerier) GetTemplates(_ context.Context) ([]database.Template, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	templates := slices.Clone(q.templates)
	slices.SortFunc(templates, func(i, j database.Template) bool {
		if i.Name != j.Name {
			return i.Name < j.Name
		}
		return i.ID.String() < j.ID.String()
	})

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
	for i := len(q.provisionerJobAgents) - 1; i >= 0; i-- {
		agent := q.provisionerJobAgents[i]
		if agent.AuthToken == authToken {
			return agent, nil
		}
	}
	return database.WorkspaceAgent{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceAgentByID(_ context.Context, id uuid.UUID) (database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// The schema sorts this by created at, so we iterate the array backwards.
	for i := len(q.provisionerJobAgents) - 1; i >= 0; i-- {
		agent := q.provisionerJobAgents[i]
		if agent.ID == id {
			return agent, nil
		}
	}
	return database.WorkspaceAgent{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceAgentByInstanceID(_ context.Context, instanceID string) (database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// The schema sorts this by created at, so we iterate the array backwards.
	for i := len(q.provisionerJobAgents) - 1; i >= 0; i-- {
		agent := q.provisionerJobAgents[i]
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
	for _, agent := range q.provisionerJobAgents {
		for _, resourceID := range resourceIDs {
			if agent.ResourceID != resourceID {
				continue
			}
			workspaceAgents = append(workspaceAgents, agent)
		}
	}
	return workspaceAgents, nil
}

func (q *fakeQuerier) GetWorkspaceAgentsCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaceAgents := make([]database.WorkspaceAgent, 0)
	for _, agent := range q.provisionerJobAgents {
		if agent.CreatedAt.After(after) {
			workspaceAgents = append(workspaceAgents, agent)
		}
	}
	return workspaceAgents, nil
}

func (q *fakeQuerier) GetWorkspaceAppByAgentIDAndName(_ context.Context, arg database.GetWorkspaceAppByAgentIDAndNameParams) (database.WorkspaceApp, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, app := range q.workspaceApps {
		if app.AgentID != arg.AgentID {
			continue
		}
		if app.Name != arg.Name {
			continue
		}
		return app, nil
	}
	return database.WorkspaceApp{}, sql.ErrNoRows
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
		if provisionerJob.ID != id {
			continue
		}
		return provisionerJob, nil
	}
	return database.ProvisionerJob{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceResourceByID(_ context.Context, id uuid.UUID) (database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, resource := range q.provisionerJobResources {
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
	for _, resource := range q.provisionerJobResources {
		if resource.JobID.String() != jobID.String() {
			continue
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func (q *fakeQuerier) GetWorkspaceResourcesCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	resources := make([]database.WorkspaceResource, 0)
	for _, resource := range q.provisionerJobResources {
		if resource.CreatedAt.After(after) {
			resources = append(resources, resource)
		}
	}
	return resources, nil
}

func (q *fakeQuerier) GetWorkspaceResourceMetadataCreatedAfter(ctx context.Context, after time.Time) ([]database.WorkspaceResourceMetadatum, error) {
	resources, err := q.GetWorkspaceResourcesCreatedAfter(ctx, after)
	if err != nil {
		return nil, err
	}
	resourceIDs := map[uuid.UUID]struct{}{}
	for _, resource := range resources {
		resourceIDs[resource.ID] = struct{}{}
	}
	metadata := make([]database.WorkspaceResourceMetadatum, 0)
	for _, m := range q.provisionerJobResourceMetadata {
		_, ok := resourceIDs[m.WorkspaceResourceID]
		if !ok {
			continue
		}
		metadata = append(metadata, m)
	}
	return metadata, nil
}

func (q *fakeQuerier) GetWorkspaceResourceMetadataByResourceID(_ context.Context, id uuid.UUID) ([]database.WorkspaceResourceMetadatum, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	metadata := make([]database.WorkspaceResourceMetadatum, 0)
	for _, metadatum := range q.provisionerJobResourceMetadata {
		if metadatum.WorkspaceResourceID.String() == id.String() {
			metadata = append(metadata, metadatum)
		}
	}
	return metadata, nil
}

func (q *fakeQuerier) GetWorkspaceResourceMetadataByResourceIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceResourceMetadatum, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	metadata := make([]database.WorkspaceResourceMetadatum, 0)
	for _, metadatum := range q.provisionerJobResourceMetadata {
		for _, id := range ids {
			if metadatum.WorkspaceResourceID.String() == id.String() {
				metadata = append(metadata, metadatum)
			}
		}
	}
	return metadata, nil
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

func (q *fakeQuerier) GetProvisionerJobsCreatedAfter(_ context.Context, after time.Time) ([]database.ProvisionerJob, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	jobs := make([]database.ProvisionerJob, 0)
	for _, job := range q.provisionerJobs {
		if job.CreatedAt.After(after) {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

func (q *fakeQuerier) GetProvisionerLogsByIDBetween(_ context.Context, arg database.GetProvisionerLogsByIDBetweenParams) ([]database.ProvisionerJobLog, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	logs := make([]database.ProvisionerJobLog, 0)
	for _, jobLog := range q.provisionerJobLogs {
		if jobLog.JobID.String() != arg.JobID.String() {
			continue
		}
		if !arg.CreatedBefore.IsZero() && jobLog.CreatedAt.After(arg.CreatedBefore) {
			continue
		}
		if !arg.CreatedAfter.IsZero() && jobLog.CreatedAt.Before(arg.CreatedAfter) {
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

	if arg.LifetimeSeconds == 0 {
		arg.LifetimeSeconds = 86400
	}

	//nolint:gosimple
	key := database.APIKey{
		ID:              arg.ID,
		LifetimeSeconds: arg.LifetimeSeconds,
		HashedSecret:    arg.HashedSecret,
		IPAddress:       arg.IPAddress,
		UserID:          arg.UserID,
		ExpiresAt:       arg.ExpiresAt,
		CreatedAt:       arg.CreatedAt,
		UpdatedAt:       arg.UpdatedAt,
		LastUsed:        arg.LastUsed,
		LoginType:       arg.LoginType,
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
	q.parameterValues = append(q.parameterValues, parameterValue)
	return parameterValue, nil
}

func (q *fakeQuerier) InsertTemplate(_ context.Context, arg database.InsertTemplateParams) (database.Template, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if arg.MinAutostartInterval == 0 {
		arg.MinAutostartInterval = int64(time.Hour)
	}

	//nolint:gosimple
	template := database.Template{
		ID:                   arg.ID,
		CreatedAt:            arg.CreatedAt,
		UpdatedAt:            arg.UpdatedAt,
		OrganizationID:       arg.OrganizationID,
		Name:                 arg.Name,
		Provisioner:          arg.Provisioner,
		ActiveVersionID:      arg.ActiveVersionID,
		Description:          arg.Description,
		MaxTtl:               arg.MaxTtl,
		MinAutostartInterval: arg.MinAutostartInterval,
		CreatedBy:            arg.CreatedBy,
	}
	q.templates = append(q.templates, template)
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
		Readme:         arg.Readme,
		JobID:          arg.JobID,
		CreatedBy:      arg.CreatedBy,
	}
	q.templateVersions = append(q.templateVersions, version)
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
	q.provisionerJobLogs = append(q.provisionerJobLogs, logs...)
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
		Index:                    arg.Index,
	}
	q.parameterSchemas = append(q.parameterSchemas, param)
	return param, nil
}

func (q *fakeQuerier) InsertProvisionerDaemon(_ context.Context, arg database.InsertProvisionerDaemonParams) (database.ProvisionerDaemon, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	daemon := database.ProvisionerDaemon{
		ID:           arg.ID,
		CreatedAt:    arg.CreatedAt,
		Name:         arg.Name,
		Provisioners: arg.Provisioners,
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
		Directory:            arg.Directory,
		StartupScript:        arg.StartupScript,
		InstanceMetadata:     arg.InstanceMetadata,
		ResourceMetadata:     arg.ResourceMetadata,
	}

	q.provisionerJobAgents = append(q.provisionerJobAgents, agent)
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
	q.provisionerJobResources = append(q.provisionerJobResources, resource)
	return resource, nil
}

func (q *fakeQuerier) InsertWorkspaceResourceMetadata(_ context.Context, arg database.InsertWorkspaceResourceMetadataParams) (database.WorkspaceResourceMetadatum, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	metadatum := database.WorkspaceResourceMetadatum{
		WorkspaceResourceID: arg.WorkspaceResourceID,
		Key:                 arg.Key,
		Value:               arg.Value,
		Sensitive:           arg.Sensitive,
	}
	q.provisionerJobResourceMetadata = append(q.provisionerJobResourceMetadata, metadatum)
	return metadatum, nil
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
		LoginType:      arg.LoginType,
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
		user.AvatarURL = arg.AvatarURL
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

func (q *fakeQuerier) UpdateUserHashedPassword(_ context.Context, arg database.UpdateUserHashedPasswordParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, user := range q.users {
		if user.ID != arg.ID {
			continue
		}
		user.HashedPassword = arg.HashedPassword
		q.users[i] = user
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) InsertWorkspace(_ context.Context, arg database.InsertWorkspaceParams) (database.Workspace, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	workspace := database.Workspace{
		ID:                arg.ID,
		CreatedAt:         arg.CreatedAt,
		UpdatedAt:         arg.UpdatedAt,
		OwnerID:           arg.OwnerID,
		OrganizationID:    arg.OrganizationID,
		TemplateID:        arg.TemplateID,
		Name:              arg.Name,
		AutostartSchedule: arg.AutostartSchedule,
		Ttl:               arg.Ttl,
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
		BuildNumber:       arg.BuildNumber,
		Transition:        arg.Transition,
		InitiatorID:       arg.InitiatorID,
		JobID:             arg.JobID,
		ProvisionerState:  arg.ProvisionerState,
		Deadline:          arg.Deadline,
		Reason:            arg.Reason,
	}
	q.workspaceBuilds = append(q.workspaceBuilds, workspaceBuild)
	return workspaceBuild, nil
}

func (q *fakeQuerier) InsertWorkspaceApp(_ context.Context, arg database.InsertWorkspaceAppParams) (database.WorkspaceApp, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// nolint:gosimple
	workspaceApp := database.WorkspaceApp{
		ID:           arg.ID,
		AgentID:      arg.AgentID,
		CreatedAt:    arg.CreatedAt,
		Name:         arg.Name,
		Icon:         arg.Icon,
		Command:      arg.Command,
		Url:          arg.Url,
		RelativePath: arg.RelativePath,
	}
	q.workspaceApps = append(q.workspaceApps, workspaceApp)
	return workspaceApp, nil
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
		apiKey.IPAddress = arg.IPAddress
		q.apiKeys[index] = apiKey
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateTemplateActiveVersionByID(_ context.Context, arg database.UpdateTemplateActiveVersionByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, template := range q.templates {
		if template.ID != arg.ID {
			continue
		}
		template.ActiveVersionID = arg.ActiveVersionID
		template.UpdatedAt = arg.UpdatedAt
		q.templates[index] = template
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateTemplateDeletedByID(_ context.Context, arg database.UpdateTemplateDeletedByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, template := range q.templates {
		if template.ID != arg.ID {
			continue
		}
		template.Deleted = arg.Deleted
		template.UpdatedAt = arg.UpdatedAt
		q.templates[index] = template
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateTemplateVersionByID(_ context.Context, arg database.UpdateTemplateVersionByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, templateVersion := range q.templateVersions {
		if templateVersion.ID != arg.ID {
			continue
		}
		templateVersion.TemplateID = arg.TemplateID
		templateVersion.UpdatedAt = arg.UpdatedAt
		q.templateVersions[index] = templateVersion
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateTemplateVersionDescriptionByJobID(_ context.Context, arg database.UpdateTemplateVersionDescriptionByJobIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, templateVersion := range q.templateVersions {
		if templateVersion.JobID != arg.JobID {
			continue
		}
		templateVersion.Readme = arg.Readme
		templateVersion.UpdatedAt = arg.UpdatedAt
		q.templateVersions[index] = templateVersion
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateProvisionerDaemonByID(_ context.Context, arg database.UpdateProvisionerDaemonByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, daemon := range q.provisionerDaemons {
		if arg.ID != daemon.ID {
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

	for index, agent := range q.provisionerJobAgents {
		if agent.ID != arg.ID {
			continue
		}
		agent.FirstConnectedAt = arg.FirstConnectedAt
		agent.LastConnectedAt = arg.LastConnectedAt
		agent.DisconnectedAt = arg.DisconnectedAt
		agent.UpdatedAt = arg.UpdatedAt
		q.provisionerJobAgents[index] = agent
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceAgentVersionByID(_ context.Context, arg database.UpdateWorkspaceAgentVersionByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, agent := range q.provisionerJobAgents {
		if agent.ID != arg.ID {
			continue
		}

		agent.Version = arg.Version
		q.provisionerJobAgents[index] = agent
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateProvisionerJobByID(_ context.Context, arg database.UpdateProvisionerJobByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, job := range q.provisionerJobs {
		if arg.ID != job.ID {
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
		if arg.ID != job.ID {
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
		if arg.ID != job.ID {
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

func (q *fakeQuerier) UpdateWorkspace(_ context.Context, arg database.UpdateWorkspaceParams) (database.Workspace, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, workspace := range q.workspaces {
		if workspace.Deleted || workspace.ID != arg.ID {
			continue
		}
		for _, other := range q.workspaces {
			if other.Deleted || other.ID == workspace.ID || workspace.OwnerID != other.OwnerID {
				continue
			}
			if other.Name == arg.Name {
				return database.Workspace{}, &pq.Error{Code: "23505", Message: "duplicate key value violates unique constraint"}
			}
		}

		workspace.Name = arg.Name
		q.workspaces[i] = workspace

		return workspace, nil
	}

	return database.Workspace{}, sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceAutostart(_ context.Context, arg database.UpdateWorkspaceAutostartParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, workspace := range q.workspaces {
		if workspace.ID != arg.ID {
			continue
		}
		workspace.AutostartSchedule = arg.AutostartSchedule
		q.workspaces[index] = workspace
		return nil
	}

	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceTTL(_ context.Context, arg database.UpdateWorkspaceTTLParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, workspace := range q.workspaces {
		if workspace.ID != arg.ID {
			continue
		}
		workspace.Ttl = arg.Ttl
		q.workspaces[index] = workspace
		return nil
	}

	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceLastUsedAt(_ context.Context, arg database.UpdateWorkspaceLastUsedAtParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, workspace := range q.workspaces {
		if workspace.ID != arg.ID {
			continue
		}
		workspace.LastUsedAt = arg.LastUsedAt
		q.workspaces[index] = workspace
		return nil
	}

	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceBuildByID(_ context.Context, arg database.UpdateWorkspaceBuildByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.ID != arg.ID {
			continue
		}
		workspaceBuild.UpdatedAt = arg.UpdatedAt
		workspaceBuild.ProvisionerState = arg.ProvisionerState
		workspaceBuild.Deadline = arg.Deadline
		q.workspaceBuilds[index] = workspaceBuild
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceDeletedByID(_ context.Context, arg database.UpdateWorkspaceDeletedByIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, workspace := range q.workspaces {
		if workspace.ID != arg.ID {
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
	q.gitSSHKey = append(q.gitSSHKey, gitSSHKey)
	return gitSSHKey, nil
}

func (q *fakeQuerier) GetGitSSHKey(_ context.Context, userID uuid.UUID) (database.GitSSHKey, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, key := range q.gitSSHKey {
		if key.UserID == userID {
			return key, nil
		}
	}
	return database.GitSSHKey{}, sql.ErrNoRows
}

func (q *fakeQuerier) UpdateGitSSHKey(_ context.Context, arg database.UpdateGitSSHKeyParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, key := range q.gitSSHKey {
		if key.UserID.String() != arg.UserID.String() {
			continue
		}
		key.UpdatedAt = arg.UpdatedAt
		key.PrivateKey = arg.PrivateKey
		key.PublicKey = arg.PublicKey
		q.gitSSHKey[index] = key
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) DeleteGitSSHKey(_ context.Context, userID uuid.UUID) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, key := range q.gitSSHKey {
		if key.UserID.String() != userID.String() {
			continue
		}
		q.gitSSHKey[index] = q.gitSSHKey[len(q.gitSSHKey)-1]
		q.gitSSHKey = q.gitSSHKey[:len(q.gitSSHKey)-1]
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) GetAuditLogsBefore(_ context.Context, arg database.GetAuditLogsBeforeParams) ([]database.AuditLog, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	logs := make([]database.AuditLog, 0)
	start := database.AuditLog{}

	if arg.ID != uuid.Nil {
		for _, alog := range q.auditLogs {
			if alog.ID == arg.ID {
				start = alog
				break
			}
		}
	} else {
		start.ID = uuid.New()
		start.Time = arg.StartTime
	}

	if start.ID == uuid.Nil {
		return nil, sql.ErrNoRows
	}

	// q.auditLogs are already sorted by time DESC, so no need to sort after the fact.
	for _, alog := range q.auditLogs {
		if alog.Time.Before(start.Time) {
			logs = append(logs, alog)
		}

		if len(logs) >= int(arg.RowLimit) {
			break
		}
	}

	return logs, nil
}

func (q *fakeQuerier) InsertAuditLog(_ context.Context, arg database.InsertAuditLogParams) (database.AuditLog, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	alog := database.AuditLog(arg)

	q.auditLogs = append(q.auditLogs, alog)
	slices.SortFunc(q.auditLogs, func(a, b database.AuditLog) bool {
		return a.Time.Before(b.Time)
	})

	return alog, nil
}

func (q *fakeQuerier) InsertDeploymentID(_ context.Context, id string) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.deploymentID = id
	return nil
}

func (q *fakeQuerier) GetDeploymentID(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.deploymentID, nil
}

func (q *fakeQuerier) InsertLicense(
	_ context.Context, arg database.InsertLicenseParams,
) (database.License, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	l := database.License{
		ID:         q.lastLicenseID + 1,
		UploadedAt: arg.UploadedAt,
		JWT:        arg.JWT,
		Exp:        arg.Exp,
	}
	q.lastLicenseID = l.ID
	q.licenses = append(q.licenses, l)
	return l, nil
}

func (q *fakeQuerier) GetLicenses(_ context.Context) ([]database.License, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	results := append([]database.License{}, q.licenses...)
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	return results, nil
}

func (q *fakeQuerier) GetUnexpiredLicenses(_ context.Context) ([]database.License, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	now := time.Now()
	var results []database.License
	for _, l := range q.licenses {
		if l.Exp.After(now) {
			results = append(results, l)
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	return results, nil
}

func (q *fakeQuerier) DeleteLicense(_ context.Context, id int32) (int32, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, l := range q.licenses {
		if l.ID == id {
			q.licenses[index] = q.licenses[len(q.licenses)-1]
			q.licenses = q.licenses[:len(q.licenses)-1]
			return id, nil
		}
	}
	return 0, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserLinkByLinkedID(_ context.Context, id string) (database.UserLink, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, link := range q.userLinks {
		if link.LinkedID == id {
			return link, nil
		}
	}
	return database.UserLink{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserLinkByUserIDLoginType(_ context.Context, params database.GetUserLinkByUserIDLoginTypeParams) (database.UserLink, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, link := range q.userLinks {
		if link.UserID == params.UserID && link.LoginType == params.LoginType {
			return link, nil
		}
	}
	return database.UserLink{}, sql.ErrNoRows
}

func (q *fakeQuerier) InsertUserLink(_ context.Context, args database.InsertUserLinkParams) (database.UserLink, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	//nolint:gosimple
	link := database.UserLink{
		UserID:            args.UserID,
		LoginType:         args.LoginType,
		LinkedID:          args.LinkedID,
		OAuthAccessToken:  args.OAuthAccessToken,
		OAuthRefreshToken: args.OAuthRefreshToken,
		OAuthExpiry:       args.OAuthExpiry,
	}

	q.userLinks = append(q.userLinks, link)

	return link, nil
}

func (q *fakeQuerier) UpdateUserLinkedID(_ context.Context, params database.UpdateUserLinkedIDParams) (database.UserLink, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for i, link := range q.userLinks {
		if link.UserID == params.UserID && link.LoginType == params.LoginType {
			link.LinkedID = params.LinkedID

			q.userLinks[i] = link
			return link, nil
		}
	}

	return database.UserLink{}, sql.ErrNoRows
}

func (q *fakeQuerier) UpdateUserLink(_ context.Context, params database.UpdateUserLinkParams) (database.UserLink, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for i, link := range q.userLinks {
		if link.UserID == params.UserID && link.LoginType == params.LoginType {
			link.OAuthAccessToken = params.OAuthAccessToken
			link.OAuthRefreshToken = params.OAuthRefreshToken
			link.OAuthExpiry = params.OAuthExpiry

			q.userLinks[i] = link
			return link, nil
		}
	}

	return database.UserLink{}, sql.ErrNoRows
}
