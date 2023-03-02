package dbfake

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

// FakeDatabase is helpful for knowing if the underlying db is an in memory fake
// database. This is only in the databasefake package, so will only be used
// by unit tests.
type FakeDatabase interface {
	IsFakeDB()
}

var errDuplicateKey = &pq.Error{
	Code:    "23505",
	Message: "duplicate key value violates unique constraint",
}

// New returns an in-memory fake of the database.
func New() database.Store {
	return &fakeQuerier{
		mutex: &sync.RWMutex{},
		data: &data{
			apiKeys:                   make([]database.APIKey, 0),
			organizationMembers:       make([]database.OrganizationMember, 0),
			organizations:             make([]database.Organization, 0),
			users:                     make([]database.User, 0),
			gitAuthLinks:              make([]database.GitAuthLink, 0),
			groups:                    make([]database.Group, 0),
			groupMembers:              make([]database.GroupMember, 0),
			auditLogs:                 make([]database.AuditLog, 0),
			files:                     make([]database.File, 0),
			gitSSHKey:                 make([]database.GitSSHKey, 0),
			parameterSchemas:          make([]database.ParameterSchema, 0),
			parameterValues:           make([]database.ParameterValue, 0),
			provisionerDaemons:        make([]database.ProvisionerDaemon, 0),
			workspaceAgents:           make([]database.WorkspaceAgent, 0),
			provisionerJobLogs:        make([]database.ProvisionerJobLog, 0),
			workspaceResources:        make([]database.WorkspaceResource, 0),
			workspaceResourceMetadata: make([]database.WorkspaceResourceMetadatum, 0),
			provisionerJobs:           make([]database.ProvisionerJob, 0),
			templateVersions:          make([]database.TemplateVersion, 0),
			templates:                 make([]database.Template, 0),
			workspaceAgentStats:       make([]database.WorkspaceAgentStat, 0),
			workspaceBuilds:           make([]database.WorkspaceBuild, 0),
			workspaceApps:             make([]database.WorkspaceApp, 0),
			workspaces:                make([]database.Workspace, 0),
			licenses:                  make([]database.License, 0),
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
	workspaceAgentStats       []database.WorkspaceAgentStat
	auditLogs                 []database.AuditLog
	files                     []database.File
	gitAuthLinks              []database.GitAuthLink
	gitSSHKey                 []database.GitSSHKey
	groupMembers              []database.GroupMember
	groups                    []database.Group
	licenses                  []database.License
	parameterSchemas          []database.ParameterSchema
	parameterValues           []database.ParameterValue
	provisionerDaemons        []database.ProvisionerDaemon
	provisionerJobLogs        []database.ProvisionerJobLog
	provisionerJobs           []database.ProvisionerJob
	replicas                  []database.Replica
	templateVersions          []database.TemplateVersion
	templateVersionParameters []database.TemplateVersionParameter
	templateVersionVariables  []database.TemplateVersionVariable
	templates                 []database.Template
	workspaceAgents           []database.WorkspaceAgent
	workspaceApps             []database.WorkspaceApp
	workspaceBuilds           []database.WorkspaceBuild
	workspaceBuildParameters  []database.WorkspaceBuildParameter
	workspaceResourceMetadata []database.WorkspaceResourceMetadatum
	workspaceResources        []database.WorkspaceResource
	workspaces                []database.Workspace

	deploymentID    string
	derpMeshKey     string
	lastUpdateCheck []byte
	serviceBanner   []byte
	logoURL         string
	lastLicenseID   int32
}

func validateDatabaseTypeWithValid(v reflect.Value) (handled bool, err error) {
	if v.Kind() == reflect.Struct {
		return false, nil
	}

	if v.CanInterface() {
		if !strings.Contains(v.Type().PkgPath(), "coderd/database") {
			return true, nil
		}
		if valid, ok := v.Interface().(interface{ Valid() bool }); ok {
			if !valid.Valid() {
				return true, xerrors.Errorf("invalid %s: %q", v.Type().Name(), v.Interface())
			}
		}
		return true, nil
	}
	return false, nil
}

// validateDatabaseType uses reflect to check if struct properties are types
// with a Valid() bool function set. If so, call it and return an error
// if false.
//
// Note that we only check immediate values and struct fields. We do not
// recurse into nested structs.
func validateDatabaseType(args interface{}) error {
	v := reflect.ValueOf(args)

	// Note: database.Null* types don't have a Valid method, we skip them here
	// because their embedded types may have a Valid method and we don't want
	// to bother with checking both that the Valid field is true and that the
	// type it embeds validates to true. We would need to check:
	//
	//	dbNullEnum.Valid && dbNullEnum.Enum.Valid()
	if strings.HasPrefix(v.Type().Name(), "Null") {
		return nil
	}

	if ok, err := validateDatabaseTypeWithValid(v); ok {
		return err
	}
	switch v.Kind() {
	case reflect.Struct:
		var errs []string
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			if ok, err := validateDatabaseTypeWithValid(field); ok && err != nil {
				errs = append(errs, fmt.Sprintf("%s.%s: %s", v.Type().Name(), v.Type().Field(i).Name, err.Error()))
			}
		}
		if len(errs) > 0 {
			return xerrors.Errorf("invalid database type fields:\n\t%s", strings.Join(errs, "\n\t"))
		}
	default:
		panic(fmt.Sprintf("unhandled type: %s", v.Type().Name()))
	}
	return nil
}

func (fakeQuerier) IsFakeDB() {}
func (*fakeQuerier) Ping(_ context.Context) (time.Duration, error) {
	return 0, nil
}

// InTx doesn't rollback data properly for in-memory yet.
func (q *fakeQuerier) InTx(fn func(database.Store) error, _ *sql.TxOptions) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return fn(&fakeQuerier{mutex: inTxMutex{}, data: q.data})
}

func (q *fakeQuerier) AcquireProvisionerJob(_ context.Context, arg database.AcquireProvisionerJobParams) (database.ProvisionerJob, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.ProvisionerJob{}, err
	}

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
		tags := map[string]string{}
		if arg.Tags != nil {
			err := json.Unmarshal(arg.Tags, &tags)
			if err != nil {
				return provisionerJob, xerrors.Errorf("unmarshal: %w", err)
			}
		}

		missing := false
		for key, value := range provisionerJob.Tags {
			provided, found := tags[key]
			if !found {
				missing = true
				break
			}
			if provided != value {
				missing = true
				break
			}
		}
		if missing {
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

func (*fakeQuerier) DeleteOldWorkspaceAgentStats(_ context.Context) error {
	// no-op
	return nil
}

func (q *fakeQuerier) InsertWorkspaceAgentStat(_ context.Context, p database.InsertWorkspaceAgentStatParams) (database.WorkspaceAgentStat, error) {
	if err := validateDatabaseType(p); err != nil {
		return database.WorkspaceAgentStat{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	stat := database.WorkspaceAgentStat{
		ID:                 p.ID,
		CreatedAt:          p.CreatedAt,
		WorkspaceID:        p.WorkspaceID,
		AgentID:            p.AgentID,
		UserID:             p.UserID,
		ConnectionsByProto: p.ConnectionsByProto,
		ConnectionCount:    p.ConnectionCount,
		RxPackets:          p.RxPackets,
		RxBytes:            p.RxBytes,
		TxPackets:          p.TxPackets,
		TxBytes:            p.TxBytes,
		TemplateID:         p.TemplateID,
	}
	q.workspaceAgentStats = append(q.workspaceAgentStats, stat)
	return stat, nil
}

func (q *fakeQuerier) GetTemplateDAUs(_ context.Context, templateID uuid.UUID) ([]database.GetTemplateDAUsRow, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	seens := make(map[time.Time]map[uuid.UUID]struct{})

	for _, as := range q.workspaceAgentStats {
		if as.TemplateID != templateID {
			continue
		}

		date := as.CreatedAt.Truncate(time.Hour * 24)

		dateEntry := seens[date]
		if dateEntry == nil {
			dateEntry = make(map[uuid.UUID]struct{})
		}
		dateEntry[as.UserID] = struct{}{}
		seens[date] = dateEntry
	}

	seenKeys := maps.Keys(seens)
	sort.Slice(seenKeys, func(i, j int) bool {
		return seenKeys[i].Before(seenKeys[j])
	})

	var rs []database.GetTemplateDAUsRow
	for _, key := range seenKeys {
		ids := seens[key]
		for id := range ids {
			rs = append(rs, database.GetTemplateDAUsRow{
				Date:   key,
				UserID: id,
			})
		}
	}

	return rs, nil
}

func (q *fakeQuerier) GetDeploymentDAUs(_ context.Context) ([]database.GetDeploymentDAUsRow, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	seens := make(map[time.Time]map[uuid.UUID]struct{})

	for _, as := range q.workspaceAgentStats {
		date := as.CreatedAt.Truncate(time.Hour * 24)

		dateEntry := seens[date]
		if dateEntry == nil {
			dateEntry = make(map[uuid.UUID]struct{})
		}
		dateEntry[as.UserID] = struct{}{}
		seens[date] = dateEntry
	}

	seenKeys := maps.Keys(seens)
	sort.Slice(seenKeys, func(i, j int) bool {
		return seenKeys[i].Before(seenKeys[j])
	})

	var rs []database.GetDeploymentDAUsRow
	for _, key := range seenKeys {
		ids := seens[key]
		for id := range ids {
			rs = append(rs, database.GetDeploymentDAUsRow{
				Date:   key,
				UserID: id,
			})
		}
	}

	return rs, nil
}

func (q *fakeQuerier) GetTemplateAverageBuildTime(ctx context.Context, arg database.GetTemplateAverageBuildTimeParams) (database.GetTemplateAverageBuildTimeRow, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.GetTemplateAverageBuildTimeRow{}, err
	}

	var emptyRow database.GetTemplateAverageBuildTimeRow
	var (
		startTimes  []float64
		stopTimes   []float64
		deleteTimes []float64
	)
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	for _, wb := range q.workspaceBuilds {
		version, err := q.getTemplateVersionByIDNoLock(ctx, wb.TemplateVersionID)
		if err != nil {
			return emptyRow, err
		}
		if version.TemplateID != arg.TemplateID {
			continue
		}

		job, err := q.getProvisionerJobByIDNoLock(ctx, wb.JobID)
		if err != nil {
			return emptyRow, err
		}
		if job.CompletedAt.Valid {
			took := job.CompletedAt.Time.Sub(job.StartedAt.Time).Seconds()
			switch wb.Transition {
			case database.WorkspaceTransitionStart:
				startTimes = append(startTimes, took)
			case database.WorkspaceTransitionStop:
				stopTimes = append(stopTimes, took)
			case database.WorkspaceTransitionDelete:
				deleteTimes = append(deleteTimes, took)
			}
		}
	}

	tryPercentile := func(fs []float64, p float64) float64 {
		if len(fs) == 0 {
			return -1
		}
		sort.Float64s(fs)
		return fs[int(float64(len(fs))*p/100)]
	}

	var row database.GetTemplateAverageBuildTimeRow
	row.Delete50, row.Delete95 = tryPercentile(deleteTimes, 50), tryPercentile(deleteTimes, 95)
	row.Stop50, row.Stop95 = tryPercentile(stopTimes, 50), tryPercentile(stopTimes, 95)
	row.Start50, row.Start95 = tryPercentile(startTimes, 50), tryPercentile(startTimes, 95)
	return row, nil
}

func (q *fakeQuerier) ParameterValue(_ context.Context, id uuid.UUID) (database.ParameterValue, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, parameterValue := range q.parameterValues {
		if parameterValue.ID != id {
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
		if parameterValue.ID != id {
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

func (q *fakeQuerier) GetAPIKeysByLoginType(_ context.Context, t database.LoginType) ([]database.APIKey, error) {
	if err := validateDatabaseType(t); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	apiKeys := make([]database.APIKey, 0)
	for _, key := range q.apiKeys {
		if key.LoginType == t {
			apiKeys = append(apiKeys, key)
		}
	}
	return apiKeys, nil
}

func (q *fakeQuerier) GetAPIKeysByUserID(_ context.Context, params database.GetAPIKeysByUserIDParams) ([]database.APIKey, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	apiKeys := make([]database.APIKey, 0)
	for _, key := range q.apiKeys {
		if key.UserID == params.UserID && key.LoginType == params.LoginType {
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

func (q *fakeQuerier) DeleteAPIKeysByUserID(_ context.Context, userID uuid.UUID) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i := len(q.apiKeys) - 1; i >= 0; i-- {
		if q.apiKeys[i].UserID == userID {
			q.apiKeys = append(q.apiKeys[:i], q.apiKeys[i+1:]...)
		}
	}

	return nil
}

func (q *fakeQuerier) GetFileByHashAndCreator(_ context.Context, arg database.GetFileByHashAndCreatorParams) (database.File, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.File{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, file := range q.files {
		if file.Hash == arg.Hash && file.CreatedBy == arg.CreatedBy {
			return file, nil
		}
	}
	return database.File{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetFileByID(_ context.Context, id uuid.UUID) (database.File, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, file := range q.files {
		if file.ID == id {
			return file, nil
		}
	}
	return database.File{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserByEmailOrUsername(_ context.Context, arg database.GetUserByEmailOrUsernameParams) (database.User, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.User{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, user := range q.users {
		if !user.Deleted && (strings.EqualFold(user.Email, arg.Email) || strings.EqualFold(user.Username, arg.Username)) {
			return user, nil
		}
	}
	return database.User{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetUserByID(_ context.Context, id uuid.UUID) (database.User, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getUserByIDNoLock(id)
}

// getUserByIDNoLock is used by other functions in the database fake.
func (q *fakeQuerier) getUserByIDNoLock(id uuid.UUID) (database.User, error) {
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

	existing := int64(0)
	for _, u := range q.users {
		if !u.Deleted {
			existing++
		}
	}
	return existing, nil
}

func (q *fakeQuerier) GetActiveUserCount(_ context.Context) (int64, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	active := int64(0)
	for _, u := range q.users {
		if u.Status == database.UserStatusActive && !u.Deleted {
			active++
		}
	}
	return active, nil
}

func (q *fakeQuerier) GetFilteredUserCount(ctx context.Context, arg database.GetFilteredUserCountParams) (int64, error) {
	if err := validateDatabaseType(arg); err != nil {
		return 0, err
	}
	count, err := q.GetAuthorizedUserCount(ctx, arg, nil)
	return count, err
}

func (q *fakeQuerier) GetAuthorizedUserCount(ctx context.Context, params database.GetFilteredUserCountParams, prepared rbac.PreparedAuthorized) (int64, error) {
	if err := validateDatabaseType(params); err != nil {
		return 0, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// Call this to match the same function calls as the SQL implementation.
	if prepared != nil {
		_, err := prepared.CompileToSQL(ctx, rbac.ConfigWithoutACL())
		if err != nil {
			return -1, err
		}
	}

	users := make([]database.User, 0, len(q.users))

	for _, user := range q.users {
		// If the filter exists, ensure the object is authorized.
		if prepared != nil && prepared.Authorize(ctx, user.RBACObject()) != nil {
			continue
		}

		users = append(users, user)
	}

	// Filter out deleted since they should never be returned..
	tmp := make([]database.User, 0, len(users))
	for _, user := range users {
		if !user.Deleted {
			tmp = append(tmp, user)
		}
	}
	users = tmp

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

	return int64(len(users)), nil
}

func (q *fakeQuerier) UpdateUserDeletedByID(_ context.Context, params database.UpdateUserDeletedByIDParams) error {
	if err := validateDatabaseType(params); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, u := range q.users {
		if u.ID == params.ID {
			u.Deleted = params.Deleted
			q.users[i] = u
			return nil
		}
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) GetUsers(_ context.Context, params database.GetUsersParams) ([]database.GetUsersRow, error) {
	if err := validateDatabaseType(params); err != nil {
		return nil, err
	}

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

	// Filter out deleted since they should never be returned..
	tmp := make([]database.User, 0, len(users))
	for _, user := range users {
		if !user.Deleted {
			tmp = append(tmp, user)
		}
	}
	users = tmp

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
			return []database.GetUsersRow{}, nil
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

	beforePageCount := len(users)

	if params.OffsetOpt > 0 {
		if int(params.OffsetOpt) > len(users)-1 {
			return []database.GetUsersRow{}, nil
		}
		users = users[params.OffsetOpt:]
	}

	if params.LimitOpt > 0 {
		if int(params.LimitOpt) > len(users) {
			params.LimitOpt = int32(len(users))
		}
		users = users[:params.LimitOpt]
	}

	return convertUsers(users, int64(beforePageCount)), nil
}

func convertUsers(users []database.User, count int64) []database.GetUsersRow {
	rows := make([]database.GetUsersRow, len(users))
	for i, u := range users {
		rows[i] = database.GetUsersRow{
			ID:             u.ID,
			Email:          u.Email,
			Username:       u.Username,
			HashedPassword: u.HashedPassword,
			CreatedAt:      u.CreatedAt,
			UpdatedAt:      u.UpdatedAt,
			Status:         u.Status,
			RBACRoles:      u.RBACRoles,
			LoginType:      u.LoginType,
			AvatarURL:      u.AvatarURL,
			Deleted:        u.Deleted,
			LastSeenAt:     u.LastSeenAt,
			Count:          count,
		}
	}

	return rows
}

func (q *fakeQuerier) GetUsersByIDs(_ context.Context, ids []uuid.UUID) ([]database.User, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	users := make([]database.User, 0)
	for _, user := range q.users {
		for _, id := range ids {
			if user.ID != id {
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

	var groups []string
	for _, member := range q.groupMembers {
		if member.UserID == userID {
			groups = append(groups, member.GroupID.String())
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
		Groups:   groups,
	}, nil
}

func (q *fakeQuerier) GetWorkspaces(ctx context.Context, arg database.GetWorkspacesParams) ([]database.GetWorkspacesRow, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	// A nil auth filter means no auth filter.
	workspaceRows, err := q.GetAuthorizedWorkspaces(ctx, arg, nil)
	return workspaceRows, err
}

//nolint:gocyclo
func (q *fakeQuerier) GetAuthorizedWorkspaces(ctx context.Context, arg database.GetWorkspacesParams, prepared rbac.PreparedAuthorized) ([]database.GetWorkspacesRow, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if prepared != nil {
		// Call this to match the same function calls as the SQL implementation.
		_, err := prepared.CompileToSQL(ctx, rbac.ConfigWithoutACL())
		if err != nil {
			return nil, err
		}
	}

	workspaces := make([]database.Workspace, 0)
	for _, workspace := range q.workspaces {
		if arg.OwnerID != uuid.Nil && workspace.OwnerID != arg.OwnerID {
			continue
		}

		if arg.OwnerUsername != "" {
			owner, err := q.getUserByIDNoLock(workspace.OwnerID)
			if err == nil && !strings.EqualFold(arg.OwnerUsername, owner.Username) {
				continue
			}
		}

		if arg.TemplateName != "" {
			template, err := q.getTemplateByIDNoLock(ctx, workspace.TemplateID)
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

		if arg.Status != "" {
			build, err := q.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
			if err != nil {
				return nil, xerrors.Errorf("get latest build: %w", err)
			}

			job, err := q.GetProvisionerJobByID(ctx, build.JobID)
			if err != nil {
				return nil, xerrors.Errorf("get provisioner job: %w", err)
			}

			switch database.WorkspaceStatus(arg.Status) {
			case database.WorkspaceStatusPending:
				if !job.StartedAt.Valid {
					continue
				}

			case database.WorkspaceStatusStarting:
				if !job.StartedAt.Valid &&
					!job.CanceledAt.Valid &&
					job.CompletedAt.Valid &&
					time.Since(job.UpdatedAt) > 30*time.Second ||
					build.Transition != database.WorkspaceTransitionStart {
					continue
				}

			case database.WorkspaceStatusRunning:
				if !job.CompletedAt.Valid &&
					job.CanceledAt.Valid &&
					job.Error.Valid ||
					build.Transition != database.WorkspaceTransitionStart {
					continue
				}

			case database.WorkspaceStatusStopping:
				if !job.StartedAt.Valid &&
					!job.CanceledAt.Valid &&
					job.CompletedAt.Valid &&
					time.Since(job.UpdatedAt) > 30*time.Second ||
					build.Transition != database.WorkspaceTransitionStop {
					continue
				}

			case database.WorkspaceStatusStopped:
				if !job.CompletedAt.Valid &&
					job.CanceledAt.Valid &&
					job.Error.Valid ||
					build.Transition != database.WorkspaceTransitionStop {
					continue
				}

			case database.WorkspaceStatusFailed:
				if (!job.CanceledAt.Valid && !job.Error.Valid) ||
					(!job.CompletedAt.Valid && !job.Error.Valid) {
					continue
				}

			case database.WorkspaceStatusCanceling:
				if !job.CanceledAt.Valid && job.CompletedAt.Valid {
					continue
				}

			case database.WorkspaceStatusCanceled:
				if !job.CanceledAt.Valid && !job.CompletedAt.Valid {
					continue
				}

			case database.WorkspaceStatusDeleted:
				if !job.StartedAt.Valid &&
					job.CanceledAt.Valid &&
					!job.CompletedAt.Valid &&
					time.Since(job.UpdatedAt) > 30*time.Second ||
					build.Transition != database.WorkspaceTransitionDelete {
					continue
				}

			case database.WorkspaceStatusDeleting:
				if !job.CompletedAt.Valid &&
					job.CanceledAt.Valid &&
					job.Error.Valid &&
					build.Transition != database.WorkspaceTransitionDelete {
					continue
				}

			default:
				return nil, xerrors.Errorf("unknown workspace status in filter: %q", arg.Status)
			}
		}

		if arg.HasAgent != "" {
			build, err := q.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
			if err != nil {
				return nil, xerrors.Errorf("get latest build: %w", err)
			}

			job, err := q.GetProvisionerJobByID(ctx, build.JobID)
			if err != nil {
				return nil, xerrors.Errorf("get provisioner job: %w", err)
			}

			workspaceResources, err := q.GetWorkspaceResourcesByJobID(ctx, job.ID)
			if err != nil {
				return nil, xerrors.Errorf("get workspace resources: %w", err)
			}

			var workspaceResourceIDs []uuid.UUID
			for _, wr := range workspaceResources {
				workspaceResourceIDs = append(workspaceResourceIDs, wr.ID)
			}

			workspaceAgents, err := q.GetWorkspaceAgentsByResourceIDs(ctx, workspaceResourceIDs)
			if err != nil {
				return nil, xerrors.Errorf("get workspace agents: %w", err)
			}

			var hasAgentMatched bool
			for _, wa := range workspaceAgents {
				if mapAgentStatus(wa, arg.AgentInactiveDisconnectTimeoutSeconds) == arg.HasAgent {
					hasAgentMatched = true
				}
			}

			if !hasAgentMatched {
				continue
			}
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

		// If the filter exists, ensure the object is authorized.
		if prepared != nil && prepared.Authorize(ctx, workspace.RBACObject()) != nil {
			continue
		}
		workspaces = append(workspaces, workspace)
	}

	beforePageCount := len(workspaces)

	if arg.Offset > 0 {
		if int(arg.Offset) > len(workspaces) {
			return []database.GetWorkspacesRow{}, nil
		}
		workspaces = workspaces[arg.Offset:]
	}
	if arg.Limit > 0 {
		if int(arg.Limit) > len(workspaces) {
			return convertToWorkspaceRows(workspaces, int64(beforePageCount)), nil
		}
		workspaces = workspaces[:arg.Limit]
	}

	return convertToWorkspaceRows(workspaces, int64(beforePageCount)), nil
}

// mapAgentStatus determines the agent status based on different timestamps like created_at, last_connected_at, disconnected_at, etc.
// The function must be in sync with: coderd/workspaceagents.go:convertWorkspaceAgent.
func mapAgentStatus(dbAgent database.WorkspaceAgent, agentInactiveDisconnectTimeoutSeconds int64) string {
	var status string
	connectionTimeout := time.Duration(dbAgent.ConnectionTimeoutSeconds) * time.Second
	switch {
	case !dbAgent.FirstConnectedAt.Valid:
		switch {
		case connectionTimeout > 0 && database.Now().Sub(dbAgent.CreatedAt) > connectionTimeout:
			// If the agent took too long to connect the first time,
			// mark it as timed out.
			status = "timeout"
		default:
			// If the agent never connected, it's waiting for the compute
			// to start up.
			status = "connecting"
		}
	case dbAgent.DisconnectedAt.Time.After(dbAgent.LastConnectedAt.Time):
		// If we've disconnected after our last connection, we know the
		// agent is no longer connected.
		status = "disconnected"
	case database.Now().Sub(dbAgent.LastConnectedAt.Time) > time.Duration(agentInactiveDisconnectTimeoutSeconds)*time.Second:
		// The connection died without updating the last connected.
		status = "disconnected"
	case dbAgent.LastConnectedAt.Valid:
		// The agent should be assumed connected if it's under inactivity timeouts
		// and last connected at has been properly set.
		status = "connected"
	default:
		panic("unknown agent status: " + status)
	}
	return status
}

func convertToWorkspaceRows(workspaces []database.Workspace, count int64) []database.GetWorkspacesRow {
	rows := make([]database.GetWorkspacesRow, len(workspaces))
	for i, w := range workspaces {
		rows[i] = database.GetWorkspacesRow{
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
			Count:             count,
		}
	}
	return rows
}

func (q *fakeQuerier) GetWorkspaceByID(_ context.Context, id uuid.UUID) (database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspace := range q.workspaces {
		if workspace.ID == id {
			return workspace, nil
		}
	}
	return database.Workspace{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceByAgentID(_ context.Context, agentID uuid.UUID) (database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var agent database.WorkspaceAgent
	for _, _agent := range q.workspaceAgents {
		if _agent.ID == agentID {
			agent = _agent
			break
		}
	}
	if agent.ID == uuid.Nil {
		return database.Workspace{}, sql.ErrNoRows
	}

	var resource database.WorkspaceResource
	for _, _resource := range q.workspaceResources {
		if _resource.ID == agent.ResourceID {
			resource = _resource
			break
		}
	}
	if resource.ID == uuid.Nil {
		return database.Workspace{}, sql.ErrNoRows
	}

	var build database.WorkspaceBuild
	for _, _build := range q.workspaceBuilds {
		if _build.JobID == resource.JobID {
			build = _build
			break
		}
	}
	if build.ID == uuid.Nil {
		return database.Workspace{}, sql.ErrNoRows
	}

	for _, workspace := range q.workspaces {
		if workspace.ID == build.WorkspaceID {
			return workspace, nil
		}
	}

	return database.Workspace{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceByOwnerIDAndName(_ context.Context, arg database.GetWorkspaceByOwnerIDAndNameParams) (database.Workspace, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Workspace{}, err
	}

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

func (q *fakeQuerier) GetWorkspaceByWorkspaceAppID(_ context.Context, workspaceAppID uuid.UUID) (database.Workspace, error) {
	if err := validateDatabaseType(workspaceAppID); err != nil {
		return database.Workspace{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspaceApp := range q.workspaceApps {
		workspaceApp := workspaceApp
		if workspaceApp.ID == workspaceAppID {
			return q.GetWorkspaceByAgentID(context.Background(), workspaceApp.AgentID)
		}
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
			if app.AgentID == id {
				apps = append(apps, app)
				break
			}
		}
	}
	return apps, nil
}

func (q *fakeQuerier) GetWorkspaceBuildByID(_ context.Context, id uuid.UUID) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, history := range q.workspaceBuilds {
		if history.ID == id {
			return history, nil
		}
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceBuildByJobID(_ context.Context, jobID uuid.UUID) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, build := range q.workspaceBuilds {
		if build.JobID == jobID {
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
		if workspaceBuild.WorkspaceID == workspaceID && workspaceBuild.BuildNumber > buildNum {
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
			if id == workspaceBuild.WorkspaceID && workspaceBuild.BuildNumber > buildNumbers[id] {
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

func (q *fakeQuerier) GetWorkspaceBuildsByWorkspaceID(_ context.Context,
	params database.GetWorkspaceBuildsByWorkspaceIDParams,
) ([]database.WorkspaceBuild, error) {
	if err := validateDatabaseType(params); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	history := make([]database.WorkspaceBuild, 0)
	for _, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.CreatedAt.Before(params.Since) {
			continue
		}
		if workspaceBuild.WorkspaceID == params.WorkspaceID {
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

func (q *fakeQuerier) GetWorkspaceBuildByWorkspaceIDAndBuildNumber(_ context.Context, arg database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (database.WorkspaceBuild, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceBuild{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.WorkspaceID != arg.WorkspaceID {
			continue
		}
		if workspaceBuild.BuildNumber != arg.BuildNumber {
			continue
		}
		return workspaceBuild, nil
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceBuildParameters(_ context.Context, workspaceBuildID uuid.UUID) ([]database.WorkspaceBuildParameter, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	params := make([]database.WorkspaceBuildParameter, 0)
	for _, param := range q.workspaceBuildParameters {
		if param.WorkspaceBuildID != workspaceBuildID {
			continue
		}
		params = append(params, param)
	}
	return params, nil
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
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

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

		if len(arg.IDs) > 0 {
			if !slice.Contains(arg.IDs, parameterValue.ID) {
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

func (q *fakeQuerier) GetTemplateByID(ctx context.Context, id uuid.UUID) (database.Template, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getTemplateByIDNoLock(ctx, id)
}

func (q *fakeQuerier) getTemplateByIDNoLock(_ context.Context, id uuid.UUID) (database.Template, error) {
	for _, template := range q.templates {
		if template.ID == id {
			return template, nil
		}
	}
	return database.Template{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetTemplateByOrganizationAndName(_ context.Context, arg database.GetTemplateByOrganizationAndNameParams) (database.Template, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Template{}, err
	}

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

func (q *fakeQuerier) UpdateTemplateMetaByID(_ context.Context, arg database.UpdateTemplateMetaByIDParams) (database.Template, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Template{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for idx, tpl := range q.templates {
		if tpl.ID != arg.ID {
			continue
		}
		tpl.UpdatedAt = database.Now()
		tpl.Name = arg.Name
		tpl.DisplayName = arg.DisplayName
		tpl.Description = arg.Description
		tpl.Icon = arg.Icon
		tpl.DefaultTTL = arg.DefaultTTL
		q.templates[idx] = tpl
		return tpl, nil
	}

	return database.Template{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetTemplatesWithFilter(ctx context.Context, arg database.GetTemplatesWithFilterParams) ([]database.Template, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	return q.GetAuthorizedTemplates(ctx, arg, nil)
}

func (q *fakeQuerier) GetAuthorizedTemplates(ctx context.Context, arg database.GetTemplatesWithFilterParams, prepared rbac.PreparedAuthorized) ([]database.Template, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// Call this to match the same function calls as the SQL implementation.
	if prepared != nil {
		_, err := prepared.CompileToSQL(ctx, rbac.ConfigWithACL())
		if err != nil {
			return nil, err
		}
	}

	var templates []database.Template
	for _, template := range q.templates {
		if prepared != nil && prepared.Authorize(ctx, template.RBACObject()) != nil {
			continue
		}

		if template.Deleted != arg.Deleted {
			continue
		}
		if arg.OrganizationID != uuid.Nil && template.OrganizationID != arg.OrganizationID {
			continue
		}

		if arg.ExactName != "" && !strings.EqualFold(template.Name, arg.ExactName) {
			continue
		}

		if len(arg.IDs) > 0 {
			match := false
			for _, id := range arg.IDs {
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
	if err := validateDatabaseType(arg); err != nil {
		return version, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, templateVersion := range q.templateVersions {
		if templateVersion.TemplateID.UUID != arg.TemplateID {
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
	if err := validateDatabaseType(arg); err != nil {
		return database.TemplateVersion{}, err
	}

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

func (q *fakeQuerier) GetTemplateVersionParameters(_ context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionParameter, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	parameters := make([]database.TemplateVersionParameter, 0)
	for _, param := range q.templateVersionParameters {
		if param.TemplateVersionID != templateVersionID {
			continue
		}
		parameters = append(parameters, param)
	}
	return parameters, nil
}

func (q *fakeQuerier) GetTemplateVersionVariables(_ context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionVariable, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	variables := make([]database.TemplateVersionVariable, 0)
	for _, variable := range q.templateVersionVariables {
		if variable.TemplateVersionID != templateVersionID {
			continue
		}
		variables = append(variables, variable)
	}
	return variables, nil
}

func (q *fakeQuerier) GetTemplateVersionByID(ctx context.Context, templateVersionID uuid.UUID) (database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getTemplateVersionByIDNoLock(ctx, templateVersionID)
}

func (q *fakeQuerier) getTemplateVersionByIDNoLock(_ context.Context, templateVersionID uuid.UUID) (database.TemplateVersion, error) {
	for _, templateVersion := range q.templateVersions {
		if templateVersion.ID != templateVersionID {
			continue
		}
		return templateVersion, nil
	}
	return database.TemplateVersion{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetTemplateVersionsByIDs(_ context.Context, ids []uuid.UUID) ([]database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	versions := make([]database.TemplateVersion, 0)
	for _, version := range q.templateVersions {
		for _, id := range ids {
			if id == version.ID {
				versions = append(versions, version)
				break
			}
		}
	}
	if len(versions) == 0 {
		return nil, sql.ErrNoRows
	}

	return versions, nil
}

func (q *fakeQuerier) GetTemplateVersionByJobID(_ context.Context, jobID uuid.UUID) (database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, templateVersion := range q.templateVersions {
		if templateVersion.JobID != jobID {
			continue
		}
		return templateVersion, nil
	}
	return database.TemplateVersion{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetPreviousTemplateVersion(_ context.Context, arg database.GetPreviousTemplateVersionParams) (database.TemplateVersion, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.TemplateVersion{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var currentTemplateVersion database.TemplateVersion
	for _, templateVersion := range q.templateVersions {
		if templateVersion.TemplateID != arg.TemplateID {
			continue
		}
		if templateVersion.Name != arg.Name {
			continue
		}
		if templateVersion.OrganizationID != arg.OrganizationID {
			continue
		}
		currentTemplateVersion = templateVersion
		break
	}

	previousTemplateVersions := make([]database.TemplateVersion, 0)
	for _, templateVersion := range q.templateVersions {
		if templateVersion.ID == currentTemplateVersion.ID {
			continue
		}
		if templateVersion.OrganizationID != arg.OrganizationID {
			continue
		}
		if templateVersion.TemplateID != currentTemplateVersion.TemplateID {
			continue
		}

		if templateVersion.CreatedAt.Before(currentTemplateVersion.CreatedAt) {
			previousTemplateVersions = append(previousTemplateVersions, templateVersion)
		}
	}

	if len(previousTemplateVersions) == 0 {
		return database.TemplateVersion{}, sql.ErrNoRows
	}

	sort.Slice(previousTemplateVersions, func(i, j int) bool {
		return previousTemplateVersions[i].CreatedAt.After(previousTemplateVersions[j].CreatedAt)
	})

	return previousTemplateVersions[0], nil
}

func (q *fakeQuerier) GetParameterSchemasByJobID(_ context.Context, jobID uuid.UUID) ([]database.ParameterSchema, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	parameters := make([]database.ParameterSchema, 0)
	for _, parameterSchema := range q.parameterSchemas {
		if parameterSchema.JobID != jobID {
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
	if err := validateDatabaseType(arg); err != nil {
		return database.ParameterValue{}, err
	}

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

func (q *fakeQuerier) GetTemplateUserRoles(_ context.Context, id uuid.UUID) ([]database.TemplateUser, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var template database.Template
	for _, t := range q.templates {
		if t.ID == id {
			template = t
			break
		}
	}

	if template.ID == uuid.Nil {
		return nil, sql.ErrNoRows
	}

	users := make([]database.TemplateUser, 0, len(template.UserACL))
	for k, v := range template.UserACL {
		user, err := q.getUserByIDNoLock(uuid.MustParse(k))
		if err != nil && xerrors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get user by ID: %w", err)
		}
		// We don't delete users from the map if they
		// get deleted so just skip.
		if xerrors.Is(err, sql.ErrNoRows) {
			continue
		}

		if user.Deleted || user.Status == database.UserStatusSuspended {
			continue
		}

		users = append(users, database.TemplateUser{
			User:    user,
			Actions: v,
		})
	}

	return users, nil
}

func (q *fakeQuerier) GetTemplateGroupRoles(_ context.Context, id uuid.UUID) ([]database.TemplateGroup, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var template database.Template
	for _, t := range q.templates {
		if t.ID == id {
			template = t
			break
		}
	}

	if template.ID == uuid.Nil {
		return nil, sql.ErrNoRows
	}

	groups := make([]database.TemplateGroup, 0, len(template.GroupACL))
	for k, v := range template.GroupACL {
		group, err := q.GetGroupByID(context.Background(), uuid.MustParse(k))
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get group by ID: %w", err)
		}
		// We don't delete groups from the map if they
		// get deleted so just skip.
		if xerrors.Is(err, sql.ErrNoRows) {
			continue
		}

		groups = append(groups, database.TemplateGroup{
			Group:   group,
			Actions: v,
		})
	}

	return groups, nil
}

func (q *fakeQuerier) GetOrganizationMemberByUserID(_ context.Context, arg database.GetOrganizationMemberByUserIDParams) (database.OrganizationMember, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.OrganizationMember{}, err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return database.OrganizationMember{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

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
	for i := len(q.workspaceAgents) - 1; i >= 0; i-- {
		agent := q.workspaceAgents[i]
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
	for i := len(q.workspaceAgents) - 1; i >= 0; i-- {
		agent := q.workspaceAgents[i]
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
	for i := len(q.workspaceAgents) - 1; i >= 0; i-- {
		agent := q.workspaceAgents[i]
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
	for _, agent := range q.workspaceAgents {
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
	for _, agent := range q.workspaceAgents {
		if agent.CreatedAt.After(after) {
			workspaceAgents = append(workspaceAgents, agent)
		}
	}
	return workspaceAgents, nil
}

func (q *fakeQuerier) GetWorkspaceAppByAgentIDAndSlug(_ context.Context, arg database.GetWorkspaceAppByAgentIDAndSlugParams) (database.WorkspaceApp, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceApp{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, app := range q.workspaceApps {
		if app.AgentID != arg.AgentID {
			continue
		}
		if app.Slug != arg.Slug {
			continue
		}
		return app, nil
	}
	return database.WorkspaceApp{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetProvisionerJobByID(ctx context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getProvisionerJobByIDNoLock(ctx, id)
}

func (q *fakeQuerier) getProvisionerJobByIDNoLock(_ context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
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

	for _, resource := range q.workspaceResources {
		if resource.ID == id {
			return resource, nil
		}
	}
	return database.WorkspaceResource{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetWorkspaceResourcesByJobID(_ context.Context, jobID uuid.UUID) ([]database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	resources := make([]database.WorkspaceResource, 0)
	for _, resource := range q.workspaceResources {
		if resource.JobID != jobID {
			continue
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func (q *fakeQuerier) GetWorkspaceResourcesByJobIDs(_ context.Context, jobIDs []uuid.UUID) ([]database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	resources := make([]database.WorkspaceResource, 0)
	for _, resource := range q.workspaceResources {
		for _, jobID := range jobIDs {
			if resource.JobID != jobID {
				continue
			}
			resources = append(resources, resource)
		}
	}
	return resources, nil
}

func (q *fakeQuerier) GetWorkspaceResourcesCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	resources := make([]database.WorkspaceResource, 0)
	for _, resource := range q.workspaceResources {
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

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	metadata := make([]database.WorkspaceResourceMetadatum, 0)
	for _, m := range q.workspaceResourceMetadata {
		_, ok := resourceIDs[m.WorkspaceResourceID]
		if !ok {
			continue
		}
		metadata = append(metadata, m)
	}
	return metadata, nil
}

func (q *fakeQuerier) GetWorkspaceResourceMetadataByResourceIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceResourceMetadatum, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	metadata := make([]database.WorkspaceResourceMetadatum, 0)
	for _, metadatum := range q.workspaceResourceMetadata {
		for _, id := range ids {
			if metadatum.WorkspaceResourceID == id {
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
			if id == job.ID {
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
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	logs := make([]database.ProvisionerJobLog, 0)
	for _, jobLog := range q.provisionerJobLogs {
		if jobLog.JobID != arg.JobID {
			continue
		}
		if arg.CreatedBefore != 0 && jobLog.ID > arg.CreatedBefore {
			continue
		}
		if arg.CreatedAfter != 0 && jobLog.ID < arg.CreatedAfter {
			continue
		}
		logs = append(logs, jobLog)
	}
	return logs, nil
}

func (q *fakeQuerier) InsertAPIKey(_ context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.APIKey{}, err
	}

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
		Scope:           arg.Scope,
	}
	q.apiKeys = append(q.apiKeys, key)
	return key, nil
}

func (q *fakeQuerier) InsertFile(_ context.Context, arg database.InsertFileParams) (database.File, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.File{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	file := database.File{
		ID:        arg.ID,
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
	if err := validateDatabaseType(arg); err != nil {
		return database.Organization{}, err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return database.OrganizationMember{}, err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return database.ParameterValue{}, err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return database.Template{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	template := database.Template{
		ID:                           arg.ID,
		CreatedAt:                    arg.CreatedAt,
		UpdatedAt:                    arg.UpdatedAt,
		OrganizationID:               arg.OrganizationID,
		Name:                         arg.Name,
		Provisioner:                  arg.Provisioner,
		ActiveVersionID:              arg.ActiveVersionID,
		Description:                  arg.Description,
		DefaultTTL:                   arg.DefaultTTL,
		CreatedBy:                    arg.CreatedBy,
		UserACL:                      arg.UserACL,
		GroupACL:                     arg.GroupACL,
		DisplayName:                  arg.DisplayName,
		Icon:                         arg.Icon,
		AllowUserCancelWorkspaceJobs: arg.AllowUserCancelWorkspaceJobs,
	}
	q.templates = append(q.templates, template)
	return template, nil
}

func (q *fakeQuerier) InsertTemplateVersion(_ context.Context, arg database.InsertTemplateVersionParams) (database.TemplateVersion, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.TemplateVersion{}, err
	}

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

func (q *fakeQuerier) InsertTemplateVersionParameter(_ context.Context, arg database.InsertTemplateVersionParameterParams) (database.TemplateVersionParameter, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.TemplateVersionParameter{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	param := database.TemplateVersionParameter{
		TemplateVersionID:   arg.TemplateVersionID,
		Name:                arg.Name,
		Description:         arg.Description,
		Type:                arg.Type,
		Mutable:             arg.Mutable,
		DefaultValue:        arg.DefaultValue,
		Icon:                arg.Icon,
		Options:             arg.Options,
		ValidationError:     arg.ValidationError,
		ValidationRegex:     arg.ValidationRegex,
		ValidationMin:       arg.ValidationMin,
		ValidationMax:       arg.ValidationMax,
		ValidationMonotonic: arg.ValidationMonotonic,
	}
	q.templateVersionParameters = append(q.templateVersionParameters, param)
	return param, nil
}

func (q *fakeQuerier) InsertTemplateVersionVariable(_ context.Context, arg database.InsertTemplateVersionVariableParams) (database.TemplateVersionVariable, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.TemplateVersionVariable{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	variable := database.TemplateVersionVariable{
		TemplateVersionID: arg.TemplateVersionID,
		Name:              arg.Name,
		Description:       arg.Description,
		Type:              arg.Type,
		Value:             arg.Value,
		DefaultValue:      arg.DefaultValue,
		Required:          arg.Required,
		Sensitive:         arg.Sensitive,
	}
	q.templateVersionVariables = append(q.templateVersionVariables, variable)
	return variable, nil
}

func (q *fakeQuerier) InsertProvisionerJobLogs(_ context.Context, arg database.InsertProvisionerJobLogsParams) ([]database.ProvisionerJobLog, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	logs := make([]database.ProvisionerJobLog, 0)
	id := int64(1)
	if len(q.provisionerJobLogs) > 0 {
		id = q.provisionerJobLogs[len(q.provisionerJobLogs)-1].ID
	}
	for index, output := range arg.Output {
		id++
		logs = append(logs, database.ProvisionerJobLog{
			ID:        id,
			JobID:     arg.JobID,
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
	if err := validateDatabaseType(arg); err != nil {
		return database.ParameterSchema{}, err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return database.ProvisionerDaemon{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	daemon := database.ProvisionerDaemon{
		ID:           arg.ID,
		CreatedAt:    arg.CreatedAt,
		Name:         arg.Name,
		Provisioners: arg.Provisioners,
		Tags:         arg.Tags,
	}
	q.provisionerDaemons = append(q.provisionerDaemons, daemon)
	return daemon, nil
}

func (q *fakeQuerier) InsertProvisionerJob(_ context.Context, arg database.InsertProvisionerJobParams) (database.ProvisionerJob, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.ProvisionerJob{}, err
	}

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
		FileID:         arg.FileID,
		Type:           arg.Type,
		Input:          arg.Input,
		Tags:           arg.Tags,
	}
	q.provisionerJobs = append(q.provisionerJobs, job)
	return job, nil
}

func (q *fakeQuerier) InsertWorkspaceAgent(_ context.Context, arg database.InsertWorkspaceAgentParams) (database.WorkspaceAgent, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceAgent{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	agent := database.WorkspaceAgent{
		ID:                       arg.ID,
		CreatedAt:                arg.CreatedAt,
		UpdatedAt:                arg.UpdatedAt,
		ResourceID:               arg.ResourceID,
		AuthToken:                arg.AuthToken,
		AuthInstanceID:           arg.AuthInstanceID,
		EnvironmentVariables:     arg.EnvironmentVariables,
		Name:                     arg.Name,
		Architecture:             arg.Architecture,
		OperatingSystem:          arg.OperatingSystem,
		Directory:                arg.Directory,
		StartupScript:            arg.StartupScript,
		InstanceMetadata:         arg.InstanceMetadata,
		ResourceMetadata:         arg.ResourceMetadata,
		ConnectionTimeoutSeconds: arg.ConnectionTimeoutSeconds,
		TroubleshootingURL:       arg.TroubleshootingURL,
		MOTDFile:                 arg.MOTDFile,
		LifecycleState:           database.WorkspaceAgentLifecycleStateCreated,
	}

	q.workspaceAgents = append(q.workspaceAgents, agent)
	return agent, nil
}

func (q *fakeQuerier) InsertWorkspaceResource(_ context.Context, arg database.InsertWorkspaceResourceParams) (database.WorkspaceResource, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceResource{}, err
	}

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
		Hide:       arg.Hide,
		Icon:       arg.Icon,
		DailyCost:  arg.DailyCost,
	}
	q.workspaceResources = append(q.workspaceResources, resource)
	return resource, nil
}

func (q *fakeQuerier) InsertWorkspaceResourceMetadata(_ context.Context, arg database.InsertWorkspaceResourceMetadataParams) ([]database.WorkspaceResourceMetadatum, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	metadata := make([]database.WorkspaceResourceMetadatum, 0)
	id := int64(1)
	if len(q.workspaceResourceMetadata) > 0 {
		id = q.workspaceResourceMetadata[len(q.workspaceResourceMetadata)-1].ID
	}
	for index, key := range arg.Key {
		id++
		value := arg.Value[index]
		metadata = append(metadata, database.WorkspaceResourceMetadatum{
			ID:                  id,
			WorkspaceResourceID: arg.WorkspaceResourceID,
			Key:                 key,
			Value: sql.NullString{
				String: value,
				Valid:  value != "",
			},
			Sensitive: arg.Sensitive[index],
		})
	}
	q.workspaceResourceMetadata = append(q.workspaceResourceMetadata, metadata...)
	return metadata, nil
}

func (q *fakeQuerier) InsertUser(_ context.Context, arg database.InsertUserParams) (database.User, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.User{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, user := range q.users {
		if user.Username == arg.Username && !user.Deleted {
			return database.User{}, errDuplicateKey
		}
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return database.User{}, err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return database.User{}, err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return database.User{}, err
	}

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

func (q *fakeQuerier) UpdateUserLastSeenAt(_ context.Context, arg database.UpdateUserLastSeenAtParams) (database.User, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.User{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, user := range q.users {
		if user.ID != arg.ID {
			continue
		}
		user.LastSeenAt = arg.LastSeenAt
		user.UpdatedAt = arg.UpdatedAt
		q.users[index] = user
		return user, nil
	}
	return database.User{}, sql.ErrNoRows
}

func (q *fakeQuerier) UpdateUserHashedPassword(_ context.Context, arg database.UpdateUserHashedPasswordParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return database.Workspace{}, err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceBuild{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	workspaceBuild := database.WorkspaceBuild{
		ID:                arg.ID,
		CreatedAt:         arg.CreatedAt,
		UpdatedAt:         arg.UpdatedAt,
		WorkspaceID:       arg.WorkspaceID,
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

func (q *fakeQuerier) InsertWorkspaceBuildParameters(_ context.Context, arg database.InsertWorkspaceBuildParametersParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, name := range arg.Name {
		q.workspaceBuildParameters = append(q.workspaceBuildParameters, database.WorkspaceBuildParameter{
			WorkspaceBuildID: arg.WorkspaceBuildID,
			Name:             name,
			Value:            arg.Value[index],
		})
	}
	return nil
}

func (q *fakeQuerier) InsertWorkspaceApp(_ context.Context, arg database.InsertWorkspaceAppParams) (database.WorkspaceApp, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceApp{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if arg.SharingLevel == "" {
		arg.SharingLevel = database.AppSharingLevelOwner
	}

	// nolint:gosimple
	workspaceApp := database.WorkspaceApp{
		ID:                   arg.ID,
		AgentID:              arg.AgentID,
		CreatedAt:            arg.CreatedAt,
		Slug:                 arg.Slug,
		DisplayName:          arg.DisplayName,
		Icon:                 arg.Icon,
		Command:              arg.Command,
		Url:                  arg.Url,
		External:             arg.External,
		Subdomain:            arg.Subdomain,
		SharingLevel:         arg.SharingLevel,
		HealthcheckUrl:       arg.HealthcheckUrl,
		HealthcheckInterval:  arg.HealthcheckInterval,
		HealthcheckThreshold: arg.HealthcheckThreshold,
		Health:               arg.Health,
	}
	q.workspaceApps = append(q.workspaceApps, workspaceApp)
	return workspaceApp, nil
}

func (q *fakeQuerier) UpdateWorkspaceAppHealthByID(_ context.Context, arg database.UpdateWorkspaceAppHealthByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, app := range q.workspaceApps {
		if app.ID != arg.ID {
			continue
		}
		app.Health = arg.Health
		q.workspaceApps[index] = app
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateAPIKeyByID(_ context.Context, arg database.UpdateAPIKeyByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

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

func (q *fakeQuerier) UpdateTemplateACLByID(_ context.Context, arg database.UpdateTemplateACLByIDParams) (database.Template, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Template{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, template := range q.templates {
		if template.ID == arg.ID {
			template.GroupACL = arg.GroupACL
			template.UserACL = arg.UserACL

			q.templates[i] = template
			return template, nil
		}
	}

	return database.Template{}, sql.ErrNoRows
}

func (q *fakeQuerier) UpdateTemplateVersionByID(_ context.Context, arg database.UpdateTemplateVersionByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

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

func (q *fakeQuerier) UpdateTemplateVersionGitAuthProvidersByJobID(_ context.Context, arg database.UpdateTemplateVersionGitAuthProvidersByJobIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, templateVersion := range q.templateVersions {
		if templateVersion.JobID != arg.JobID {
			continue
		}
		templateVersion.GitAuthProviders = arg.GitAuthProviders
		templateVersion.UpdatedAt = arg.UpdatedAt
		q.templateVersions[index] = templateVersion
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceAgentConnectionByID(_ context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, agent := range q.workspaceAgents {
		if agent.ID != arg.ID {
			continue
		}
		agent.FirstConnectedAt = arg.FirstConnectedAt
		agent.LastConnectedAt = arg.LastConnectedAt
		agent.DisconnectedAt = arg.DisconnectedAt
		agent.UpdatedAt = arg.UpdatedAt
		q.workspaceAgents[index] = agent
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceAgentStartupByID(_ context.Context, arg database.UpdateWorkspaceAgentStartupByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, agent := range q.workspaceAgents {
		if agent.ID != arg.ID {
			continue
		}

		agent.Version = arg.Version
		agent.ExpandedDirectory = arg.ExpandedDirectory
		q.workspaceAgents[index] = agent
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateProvisionerJobByID(_ context.Context, arg database.UpdateProvisionerJobByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, job := range q.provisionerJobs {
		if arg.ID != job.ID {
			continue
		}
		job.CanceledAt = arg.CanceledAt
		job.CompletedAt = arg.CompletedAt
		q.provisionerJobs[index] = job
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) UpdateProvisionerJobWithCompleteByID(_ context.Context, arg database.UpdateProvisionerJobWithCompleteByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return database.Workspace{}, err
	}

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
				return database.Workspace{}, errDuplicateKey
			}
		}

		workspace.Name = arg.Name
		q.workspaces[i] = workspace

		return workspace, nil
	}

	return database.Workspace{}, sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceAutostart(_ context.Context, arg database.UpdateWorkspaceAutostartParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

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

func (q *fakeQuerier) UpdateWorkspaceBuildByID(_ context.Context, arg database.UpdateWorkspaceBuildByIDParams) (database.WorkspaceBuild, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceBuild{}, err
	}

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
		return workspaceBuild, nil
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceBuildCostByID(_ context.Context, arg database.UpdateWorkspaceBuildCostByIDParams) (database.WorkspaceBuild, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceBuild{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.ID != arg.ID {
			continue
		}
		workspaceBuild.DailyCost = arg.DailyCost
		q.workspaceBuilds[index] = workspaceBuild
		return workspaceBuild, nil
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *fakeQuerier) UpdateWorkspaceDeletedByID(_ context.Context, arg database.UpdateWorkspaceDeletedByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

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
	if err := validateDatabaseType(arg); err != nil {
		return database.GitSSHKey{}, err
	}

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

func (q *fakeQuerier) UpdateGitSSHKey(_ context.Context, arg database.UpdateGitSSHKeyParams) (database.GitSSHKey, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.GitSSHKey{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, key := range q.gitSSHKey {
		if key.UserID != arg.UserID {
			continue
		}
		key.UpdatedAt = arg.UpdatedAt
		key.PrivateKey = arg.PrivateKey
		key.PublicKey = arg.PublicKey
		q.gitSSHKey[index] = key
		return key, nil
	}
	return database.GitSSHKey{}, sql.ErrNoRows
}

func (q *fakeQuerier) InsertGroupMember(_ context.Context, arg database.InsertGroupMemberParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, member := range q.groupMembers {
		if member.GroupID == arg.GroupID &&
			member.UserID == arg.UserID {
			return errDuplicateKey
		}
	}

	//nolint:gosimple
	q.groupMembers = append(q.groupMembers, database.GroupMember{
		GroupID: arg.GroupID,
		UserID:  arg.UserID,
	})

	return nil
}

func (q *fakeQuerier) DeleteGroupMemberFromGroup(_ context.Context, arg database.DeleteGroupMemberFromGroupParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, member := range q.groupMembers {
		if member.UserID == arg.UserID && member.GroupID == arg.GroupID {
			q.groupMembers = append(q.groupMembers[:i], q.groupMembers[i+1:]...)
		}
	}
	return nil
}

func (q *fakeQuerier) InsertUserGroupsByName(_ context.Context, arg database.InsertUserGroupsByNameParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	var groupIDs []uuid.UUID
	for _, group := range q.groups {
		for _, groupName := range arg.GroupNames {
			if group.Name == groupName {
				groupIDs = append(groupIDs, group.ID)
			}
		}
	}

	for _, groupID := range groupIDs {
		q.groupMembers = append(q.groupMembers, database.GroupMember{
			UserID:  arg.UserID,
			GroupID: groupID,
		})
	}

	return nil
}

func (q *fakeQuerier) DeleteGroupMembersByOrgAndUser(_ context.Context, arg database.DeleteGroupMembersByOrgAndUserParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	newMembers := q.groupMembers[:0]
	for _, member := range q.groupMembers {
		if member.UserID == arg.UserID {
			for _, group := range q.groups {
				if group.ID == member.GroupID && group.OrganizationID == arg.OrganizationID {
					continue
				}

				newMembers = append(newMembers, member)
			}
		}
	}
	q.groupMembers = newMembers

	return nil
}

func (q *fakeQuerier) UpdateGroupByID(_ context.Context, arg database.UpdateGroupByIDParams) (database.Group, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Group{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, group := range q.groups {
		if group.ID == arg.ID {
			group.Name = arg.Name
			group.AvatarURL = arg.AvatarURL
			group.QuotaAllowance = arg.QuotaAllowance
			q.groups[i] = group
			return group, nil
		}
	}
	return database.Group{}, sql.ErrNoRows
}

func (q *fakeQuerier) DeleteGitSSHKey(_ context.Context, userID uuid.UUID) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, key := range q.gitSSHKey {
		if key.UserID != userID {
			continue
		}
		q.gitSSHKey[index] = q.gitSSHKey[len(q.gitSSHKey)-1]
		q.gitSSHKey = q.gitSSHKey[:len(q.gitSSHKey)-1]
		return nil
	}
	return sql.ErrNoRows
}

func (q *fakeQuerier) GetAuditLogsOffset(_ context.Context, arg database.GetAuditLogsOffsetParams) ([]database.GetAuditLogsOffsetRow, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	logs := make([]database.GetAuditLogsOffsetRow, 0, arg.Limit)

	// q.auditLogs are already sorted by time DESC, so no need to sort after the fact.
	for _, alog := range q.auditLogs {
		if arg.Offset > 0 {
			arg.Offset--
			continue
		}
		if arg.Action != "" && !strings.Contains(string(alog.Action), arg.Action) {
			continue
		}
		if arg.ResourceType != "" && !strings.Contains(string(alog.ResourceType), arg.ResourceType) {
			continue
		}
		if arg.ResourceID != uuid.Nil && alog.ResourceID != arg.ResourceID {
			continue
		}
		if arg.Username != "" {
			user, err := q.getUserByIDNoLock(alog.UserID)
			if err == nil && !strings.EqualFold(arg.Username, user.Username) {
				continue
			}
		}
		if arg.Email != "" {
			user, err := q.getUserByIDNoLock(alog.UserID)
			if err == nil && !strings.EqualFold(arg.Email, user.Email) {
				continue
			}
		}
		if !arg.DateFrom.IsZero() {
			if alog.Time.Before(arg.DateFrom) {
				continue
			}
		}
		if !arg.DateTo.IsZero() {
			if alog.Time.After(arg.DateTo) {
				continue
			}
		}
		if arg.BuildReason != "" {
			workspaceBuild, err := q.GetWorkspaceBuildByID(context.Background(), alog.ResourceID)
			if err == nil && !strings.EqualFold(arg.BuildReason, string(workspaceBuild.Reason)) {
				continue
			}
		}

		user, err := q.getUserByIDNoLock(alog.UserID)
		userValid := err == nil

		logs = append(logs, database.GetAuditLogsOffsetRow{
			ID:               alog.ID,
			RequestID:        alog.RequestID,
			OrganizationID:   alog.OrganizationID,
			Ip:               alog.Ip,
			UserAgent:        alog.UserAgent,
			ResourceType:     alog.ResourceType,
			ResourceID:       alog.ResourceID,
			ResourceTarget:   alog.ResourceTarget,
			ResourceIcon:     alog.ResourceIcon,
			Action:           alog.Action,
			Diff:             alog.Diff,
			StatusCode:       alog.StatusCode,
			AdditionalFields: alog.AdditionalFields,
			UserID:           alog.UserID,
			UserUsername:     sql.NullString{String: user.Username, Valid: userValid},
			UserEmail:        sql.NullString{String: user.Email, Valid: userValid},
			UserCreatedAt:    sql.NullTime{Time: user.CreatedAt, Valid: userValid},
			UserStatus:       database.NullUserStatus{UserStatus: user.Status, Valid: userValid},
			UserRoles:        user.RBACRoles,
			Count:            0,
		})

		if len(logs) >= int(arg.Limit) {
			break
		}
	}

	count := int64(len(logs))
	for i := range logs {
		logs[i].Count = count
	}

	return logs, nil
}

func (q *fakeQuerier) InsertAuditLog(_ context.Context, arg database.InsertAuditLogParams) (database.AuditLog, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.AuditLog{}, err
	}

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

func (q *fakeQuerier) InsertDERPMeshKey(_ context.Context, id string) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.derpMeshKey = id
	return nil
}

func (q *fakeQuerier) GetDERPMeshKey(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.derpMeshKey, nil
}

func (q *fakeQuerier) InsertOrUpdateLastUpdateCheck(_ context.Context, data string) error {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	q.lastUpdateCheck = []byte(data)
	return nil
}

func (q *fakeQuerier) GetLastUpdateCheck(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if q.lastUpdateCheck == nil {
		return "", sql.ErrNoRows
	}
	return string(q.lastUpdateCheck), nil
}

func (q *fakeQuerier) InsertOrUpdateServiceBanner(_ context.Context, data string) error {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	q.serviceBanner = []byte(data)
	return nil
}

func (q *fakeQuerier) GetServiceBanner(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if q.serviceBanner == nil {
		return "", sql.ErrNoRows
	}

	return string(q.serviceBanner), nil
}

func (q *fakeQuerier) InsertOrUpdateLogoURL(_ context.Context, data string) error {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	q.logoURL = data
	return nil
}

func (q *fakeQuerier) GetLogoURL(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if q.logoURL == "" {
		return "", sql.ErrNoRows
	}

	return q.logoURL, nil
}

func (q *fakeQuerier) InsertLicense(
	_ context.Context, arg database.InsertLicenseParams,
) (database.License, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.License{}, err
	}

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

func (q *fakeQuerier) GetLicenseByID(_ context.Context, id int32) (database.License, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, license := range q.licenses {
		if license.ID == id {
			return license, nil
		}
	}
	return database.License{}, sql.ErrNoRows
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
	if err := validateDatabaseType(params); err != nil {
		return database.UserLink{}, err
	}

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
	if err := validateDatabaseType(params); err != nil {
		return database.UserLink{}, err
	}

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
	if err := validateDatabaseType(params); err != nil {
		return database.UserLink{}, err
	}

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

func (q *fakeQuerier) GetGroupByID(_ context.Context, id uuid.UUID) (database.Group, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, group := range q.groups {
		if group.ID == id {
			return group, nil
		}
	}

	return database.Group{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetGroupByOrgAndName(_ context.Context, arg database.GetGroupByOrgAndNameParams) (database.Group, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Group{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, group := range q.groups {
		if group.OrganizationID == arg.OrganizationID &&
			group.Name == arg.Name {
			return group, nil
		}
	}

	return database.Group{}, sql.ErrNoRows
}

func (q *fakeQuerier) InsertAllUsersGroup(ctx context.Context, orgID uuid.UUID) (database.Group, error) {
	return q.InsertGroup(ctx, database.InsertGroupParams{
		ID:             orgID,
		Name:           database.AllUsersGroup,
		OrganizationID: orgID,
	})
}

func (q *fakeQuerier) InsertGroup(_ context.Context, arg database.InsertGroupParams) (database.Group, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Group{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, group := range q.groups {
		if group.OrganizationID == arg.OrganizationID &&
			group.Name == arg.Name {
			return database.Group{}, errDuplicateKey
		}
	}

	//nolint:gosimple
	group := database.Group{
		ID:             arg.ID,
		Name:           arg.Name,
		OrganizationID: arg.OrganizationID,
		AvatarURL:      arg.AvatarURL,
		QuotaAllowance: arg.QuotaAllowance,
	}

	q.groups = append(q.groups, group)

	return group, nil
}

func (q *fakeQuerier) GetGroupMembers(_ context.Context, groupID uuid.UUID) ([]database.User, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var members []database.GroupMember
	for _, member := range q.groupMembers {
		if member.GroupID == groupID {
			members = append(members, member)
		}
	}

	users := make([]database.User, 0, len(members))

	for _, member := range members {
		for _, user := range q.users {
			if user.ID == member.UserID && user.Status == database.UserStatusActive && !user.Deleted {
				users = append(users, user)
				break
			}
		}
	}

	return users, nil
}

func (q *fakeQuerier) GetGroupsByOrganizationID(_ context.Context, organizationID uuid.UUID) ([]database.Group, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var groups []database.Group
	for _, group := range q.groups {
		// Omit the allUsers group.
		if group.OrganizationID == organizationID && group.ID != organizationID {
			groups = append(groups, group)
		}
	}

	return groups, nil
}

func (q *fakeQuerier) DeleteGroupByID(_ context.Context, id uuid.UUID) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, group := range q.groups {
		if group.ID == id {
			q.groups = append(q.groups[:i], q.groups[i+1:]...)
			return nil
		}
	}

	return sql.ErrNoRows
}

func (q *fakeQuerier) DeleteReplicasUpdatedBefore(_ context.Context, before time.Time) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, replica := range q.replicas {
		if replica.UpdatedAt.Before(before) {
			q.replicas = append(q.replicas[:i], q.replicas[i+1:]...)
		}
	}

	return nil
}

func (q *fakeQuerier) InsertReplica(_ context.Context, arg database.InsertReplicaParams) (database.Replica, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Replica{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	replica := database.Replica{
		ID:              arg.ID,
		CreatedAt:       arg.CreatedAt,
		StartedAt:       arg.StartedAt,
		UpdatedAt:       arg.UpdatedAt,
		Hostname:        arg.Hostname,
		RegionID:        arg.RegionID,
		RelayAddress:    arg.RelayAddress,
		Version:         arg.Version,
		DatabaseLatency: arg.DatabaseLatency,
	}
	q.replicas = append(q.replicas, replica)
	return replica, nil
}

func (q *fakeQuerier) UpdateReplica(_ context.Context, arg database.UpdateReplicaParams) (database.Replica, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Replica{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, replica := range q.replicas {
		if replica.ID != arg.ID {
			continue
		}
		replica.Hostname = arg.Hostname
		replica.StartedAt = arg.StartedAt
		replica.StoppedAt = arg.StoppedAt
		replica.UpdatedAt = arg.UpdatedAt
		replica.RelayAddress = arg.RelayAddress
		replica.RegionID = arg.RegionID
		replica.Version = arg.Version
		replica.Error = arg.Error
		replica.DatabaseLatency = arg.DatabaseLatency
		q.replicas[index] = replica
		return replica, nil
	}
	return database.Replica{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetReplicasUpdatedAfter(_ context.Context, updatedAt time.Time) ([]database.Replica, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	replicas := make([]database.Replica, 0)
	for _, replica := range q.replicas {
		if replica.UpdatedAt.After(updatedAt) && !replica.StoppedAt.Valid {
			replicas = append(replicas, replica)
		}
	}
	return replicas, nil
}

func (q *fakeQuerier) GetGitAuthLink(_ context.Context, arg database.GetGitAuthLinkParams) (database.GitAuthLink, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.GitAuthLink{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()
	for _, gitAuthLink := range q.gitAuthLinks {
		if arg.UserID != gitAuthLink.UserID {
			continue
		}
		if arg.ProviderID != gitAuthLink.ProviderID {
			continue
		}
		return gitAuthLink, nil
	}
	return database.GitAuthLink{}, sql.ErrNoRows
}

func (q *fakeQuerier) InsertGitAuthLink(_ context.Context, arg database.InsertGitAuthLinkParams) (database.GitAuthLink, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.GitAuthLink{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()
	// nolint:gosimple
	gitAuthLink := database.GitAuthLink{
		ProviderID:        arg.ProviderID,
		UserID:            arg.UserID,
		CreatedAt:         arg.CreatedAt,
		UpdatedAt:         arg.UpdatedAt,
		OAuthAccessToken:  arg.OAuthAccessToken,
		OAuthRefreshToken: arg.OAuthRefreshToken,
		OAuthExpiry:       arg.OAuthExpiry,
	}
	q.gitAuthLinks = append(q.gitAuthLinks, gitAuthLink)
	return gitAuthLink, nil
}

func (q *fakeQuerier) UpdateGitAuthLink(_ context.Context, arg database.UpdateGitAuthLinkParams) (database.GitAuthLink, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.GitAuthLink{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()
	for index, gitAuthLink := range q.gitAuthLinks {
		if gitAuthLink.ProviderID != arg.ProviderID {
			continue
		}
		if gitAuthLink.UserID != arg.UserID {
			continue
		}
		gitAuthLink.UpdatedAt = arg.UpdatedAt
		gitAuthLink.OAuthAccessToken = arg.OAuthAccessToken
		gitAuthLink.OAuthRefreshToken = arg.OAuthRefreshToken
		gitAuthLink.OAuthExpiry = arg.OAuthExpiry
		q.gitAuthLinks[index] = gitAuthLink

		return gitAuthLink, nil
	}
	return database.GitAuthLink{}, sql.ErrNoRows
}

func (q *fakeQuerier) GetQuotaAllowanceForUser(_ context.Context, userID uuid.UUID) (int64, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	var sum int64
	for _, member := range q.groupMembers {
		if member.UserID != userID {
			continue
		}
		for _, group := range q.groups {
			if group.ID == member.GroupID {
				sum += int64(group.QuotaAllowance)
			}
		}
	}
	return sum, nil
}

func (q *fakeQuerier) GetQuotaConsumedForUser(_ context.Context, userID uuid.UUID) (int64, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	var sum int64
	for _, workspace := range q.workspaces {
		if workspace.OwnerID != userID {
			continue
		}
		if workspace.Deleted {
			continue
		}

		var lastBuild database.WorkspaceBuild
		for _, build := range q.workspaceBuilds {
			if build.WorkspaceID != workspace.ID {
				continue
			}
			if build.CreatedAt.After(lastBuild.CreatedAt) {
				lastBuild = build
			}
		}
		sum += int64(lastBuild.DailyCost)
	}
	return sum, nil
}

func (q *fakeQuerier) UpdateWorkspaceAgentLifecycleStateByID(_ context.Context, arg database.UpdateWorkspaceAgentLifecycleStateByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()
	for i, agent := range q.workspaceAgents {
		if agent.ID == arg.ID {
			agent.LifecycleState = arg.LifecycleState
			q.workspaceAgents[i] = agent
			return nil
		}
	}
	return sql.ErrNoRows
}
