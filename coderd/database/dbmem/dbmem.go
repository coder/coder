package dbmem

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/regosql"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

var validProxyByHostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

var errForeignKeyConstraint = &pq.Error{
	Code:    "23503",
	Message: "update or delete on table violates foreign key constraint",
}

var errDuplicateKey = &pq.Error{
	Code:    "23505",
	Message: "duplicate key value violates unique constraint",
}

// New returns an in-memory fake of the database.
func New() database.Store {
	q := &FakeQuerier{
		mutex: &sync.RWMutex{},
		data: &data{
			apiKeys:                   make([]database.APIKey, 0),
			organizationMembers:       make([]database.OrganizationMember, 0),
			organizations:             make([]database.Organization, 0),
			users:                     make([]database.User, 0),
			dbcryptKeys:               make([]database.DBCryptKey, 0),
			externalAuthLinks:         make([]database.ExternalAuthLink, 0),
			groups:                    make([]database.Group, 0),
			groupMembers:              make([]database.GroupMember, 0),
			auditLogs:                 make([]database.AuditLog, 0),
			files:                     make([]database.File, 0),
			gitSSHKey:                 make([]database.GitSSHKey, 0),
			parameterSchemas:          make([]database.ParameterSchema, 0),
			provisionerDaemons:        make([]database.ProvisionerDaemon, 0),
			workspaceAgents:           make([]database.WorkspaceAgent, 0),
			provisionerJobLogs:        make([]database.ProvisionerJobLog, 0),
			workspaceResources:        make([]database.WorkspaceResource, 0),
			workspaceResourceMetadata: make([]database.WorkspaceResourceMetadatum, 0),
			provisionerJobs:           make([]database.ProvisionerJob, 0),
			templateVersions:          make([]database.TemplateVersionTable, 0),
			templates:                 make([]database.TemplateTable, 0),
			workspaceAgentStats:       make([]database.WorkspaceAgentStat, 0),
			workspaceAgentLogs:        make([]database.WorkspaceAgentLog, 0),
			workspaceBuilds:           make([]database.WorkspaceBuildTable, 0),
			workspaceApps:             make([]database.WorkspaceApp, 0),
			workspaces:                make([]database.Workspace, 0),
			licenses:                  make([]database.License, 0),
			workspaceProxies:          make([]database.WorkspaceProxy, 0),
			locks:                     map[int64]struct{}{},
		},
	}
	q.defaultProxyDisplayName = "Default"
	q.defaultProxyIconURL = "/emojis/1f3e1.png"
	return q
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

// FakeQuerier replicates database functionality to enable quick testing.  It's an exported type so that our test code
// can do type checks.
type FakeQuerier struct {
	mutex rwMutex
	*data
}

func (*FakeQuerier) Wrappers() []string {
	return []string{}
}

type fakeTx struct {
	*FakeQuerier
	locks map[int64]struct{}
}

type data struct {
	// Legacy tables
	apiKeys             []database.APIKey
	organizations       []database.Organization
	organizationMembers []database.OrganizationMember
	users               []database.User
	userLinks           []database.UserLink

	// New tables
	workspaceAgentStats           []database.WorkspaceAgentStat
	auditLogs                     []database.AuditLog
	dbcryptKeys                   []database.DBCryptKey
	files                         []database.File
	externalAuthLinks             []database.ExternalAuthLink
	gitSSHKey                     []database.GitSSHKey
	groupMembers                  []database.GroupMember
	groups                        []database.Group
	jfrogXRayScans                []database.JfrogXrayScan
	licenses                      []database.License
	oauth2ProviderApps            []database.OAuth2ProviderApp
	oauth2ProviderAppSecrets      []database.OAuth2ProviderAppSecret
	parameterSchemas              []database.ParameterSchema
	provisionerDaemons            []database.ProvisionerDaemon
	provisionerJobLogs            []database.ProvisionerJobLog
	provisionerJobs               []database.ProvisionerJob
	replicas                      []database.Replica
	templateVersions              []database.TemplateVersionTable
	templateVersionParameters     []database.TemplateVersionParameter
	templateVersionVariables      []database.TemplateVersionVariable
	templates                     []database.TemplateTable
	workspaceAgents               []database.WorkspaceAgent
	workspaceAgentMetadata        []database.WorkspaceAgentMetadatum
	workspaceAgentLogs            []database.WorkspaceAgentLog
	workspaceAgentLogSources      []database.WorkspaceAgentLogSource
	workspaceAgentScripts         []database.WorkspaceAgentScript
	workspaceAgentPortShares      []database.WorkspaceAgentPortShare
	workspaceApps                 []database.WorkspaceApp
	workspaceAppStatsLastInsertID int64
	workspaceAppStats             []database.WorkspaceAppStat
	workspaceBuilds               []database.WorkspaceBuildTable
	workspaceBuildParameters      []database.WorkspaceBuildParameter
	workspaceResourceMetadata     []database.WorkspaceResourceMetadatum
	workspaceResources            []database.WorkspaceResource
	workspaces                    []database.Workspace
	workspaceProxies              []database.WorkspaceProxy
	// Locks is a map of lock names. Any keys within the map are currently
	// locked.
	locks                   map[int64]struct{}
	deploymentID            string
	derpMeshKey             string
	lastUpdateCheck         []byte
	serviceBanner           []byte
	healthSettings          []byte
	applicationName         string
	logoURL                 string
	appSecurityKey          string
	oauthSigningKey         string
	lastLicenseID           int32
	defaultProxyDisplayName string
	defaultProxyIconURL     string
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

func (*FakeQuerier) Ping(_ context.Context) (time.Duration, error) {
	return 0, nil
}

func (tx *fakeTx) AcquireLock(_ context.Context, id int64) error {
	if _, ok := tx.FakeQuerier.locks[id]; ok {
		return xerrors.Errorf("cannot acquire lock %d: already held", id)
	}
	tx.FakeQuerier.locks[id] = struct{}{}
	tx.locks[id] = struct{}{}
	return nil
}

func (tx *fakeTx) TryAcquireLock(_ context.Context, id int64) (bool, error) {
	if _, ok := tx.FakeQuerier.locks[id]; ok {
		return false, nil
	}
	tx.FakeQuerier.locks[id] = struct{}{}
	tx.locks[id] = struct{}{}
	return true, nil
}

func (tx *fakeTx) releaseLocks() {
	for id := range tx.locks {
		delete(tx.FakeQuerier.locks, id)
	}
	tx.locks = map[int64]struct{}{}
}

// InTx doesn't rollback data properly for in-memory yet.
func (q *FakeQuerier) InTx(fn func(database.Store) error, _ *sql.TxOptions) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	tx := &fakeTx{
		FakeQuerier: &FakeQuerier{mutex: inTxMutex{}, data: q.data},
		locks:       map[int64]struct{}{},
	}
	defer tx.releaseLocks()

	return fn(tx)
}

// getUserByIDNoLock is used by other functions in the database fake.
func (q *FakeQuerier) getUserByIDNoLock(id uuid.UUID) (database.User, error) {
	for _, user := range q.users {
		if user.ID == id {
			return user, nil
		}
	}
	return database.User{}, sql.ErrNoRows
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

// mapAgentStatus determines the agent status based on different timestamps like created_at, last_connected_at, disconnected_at, etc.
// The function must be in sync with: coderd/workspaceagents.go:convertWorkspaceAgent.
func mapAgentStatus(dbAgent database.WorkspaceAgent, agentInactiveDisconnectTimeoutSeconds int64) string {
	var status string
	connectionTimeout := time.Duration(dbAgent.ConnectionTimeoutSeconds) * time.Second
	switch {
	case !dbAgent.FirstConnectedAt.Valid:
		switch {
		case connectionTimeout > 0 && dbtime.Now().Sub(dbAgent.CreatedAt) > connectionTimeout:
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
	case dbtime.Now().Sub(dbAgent.LastConnectedAt.Time) > time.Duration(agentInactiveDisconnectTimeoutSeconds)*time.Second:
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

func (q *FakeQuerier) convertToWorkspaceRowsNoLock(ctx context.Context, workspaces []database.Workspace, count int64) []database.GetWorkspacesRow {
	rows := make([]database.GetWorkspacesRow, 0, len(workspaces))
	for _, w := range workspaces {
		wr := database.GetWorkspacesRow{
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
			Count:             count,
			AutomaticUpdates:  w.AutomaticUpdates,
			Favorite:          w.Favorite,
		}

		for _, t := range q.templates {
			if t.ID == w.TemplateID {
				wr.TemplateName = t.Name
				break
			}
		}

		if build, err := q.getLatestWorkspaceBuildByWorkspaceIDNoLock(ctx, w.ID); err == nil {
			for _, tv := range q.templateVersions {
				if tv.ID == build.TemplateVersionID {
					wr.TemplateVersionID = tv.ID
					wr.TemplateVersionName = sql.NullString{
						Valid:  true,
						String: tv.Name,
					}
					break
				}
			}
		}

		rows = append(rows, wr)
	}
	return rows
}

func (q *FakeQuerier) getWorkspaceByIDNoLock(_ context.Context, id uuid.UUID) (database.Workspace, error) {
	for _, workspace := range q.workspaces {
		if workspace.ID == id {
			return workspace, nil
		}
	}
	return database.Workspace{}, sql.ErrNoRows
}

func (q *FakeQuerier) getWorkspaceByAgentIDNoLock(_ context.Context, agentID uuid.UUID) (database.Workspace, error) {
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
			build = q.workspaceBuildWithUserNoLock(_build)
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

func (q *FakeQuerier) getWorkspaceBuildByIDNoLock(_ context.Context, id uuid.UUID) (database.WorkspaceBuild, error) {
	for _, build := range q.workspaceBuilds {
		if build.ID == id {
			return q.workspaceBuildWithUserNoLock(build), nil
		}
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *FakeQuerier) getLatestWorkspaceBuildByWorkspaceIDNoLock(_ context.Context, workspaceID uuid.UUID) (database.WorkspaceBuild, error) {
	var row database.WorkspaceBuild
	var buildNum int32 = -1
	for _, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.WorkspaceID == workspaceID && workspaceBuild.BuildNumber > buildNum {
			row = q.workspaceBuildWithUserNoLock(workspaceBuild)
			buildNum = workspaceBuild.BuildNumber
		}
	}
	if buildNum == -1 {
		return database.WorkspaceBuild{}, sql.ErrNoRows
	}
	return row, nil
}

func (q *FakeQuerier) getTemplateByIDNoLock(_ context.Context, id uuid.UUID) (database.Template, error) {
	for _, template := range q.templates {
		if template.ID == id {
			return q.templateWithUserNoLock(template), nil
		}
	}
	return database.Template{}, sql.ErrNoRows
}

func (q *FakeQuerier) templatesWithUserNoLock(tpl []database.TemplateTable) []database.Template {
	cpy := make([]database.Template, 0, len(tpl))
	for _, t := range tpl {
		cpy = append(cpy, q.templateWithUserNoLock(t))
	}
	return cpy
}

func (q *FakeQuerier) templateWithUserNoLock(tpl database.TemplateTable) database.Template {
	var user database.User
	for _, _user := range q.users {
		if _user.ID == tpl.CreatedBy {
			user = _user
			break
		}
	}
	var withUser database.Template
	// This is a cheeky way to copy the fields over without explicitly listing them all.
	d, _ := json.Marshal(tpl)
	_ = json.Unmarshal(d, &withUser)
	withUser.CreatedByUsername = user.Username
	withUser.CreatedByAvatarURL = user.AvatarURL
	return withUser
}

func (q *FakeQuerier) templateVersionWithUserNoLock(tpl database.TemplateVersionTable) database.TemplateVersion {
	var user database.User
	for _, _user := range q.users {
		if _user.ID == tpl.CreatedBy {
			user = _user
			break
		}
	}
	var withUser database.TemplateVersion
	// This is a cheeky way to copy the fields over without explicitly listing them all.
	d, _ := json.Marshal(tpl)
	_ = json.Unmarshal(d, &withUser)
	withUser.CreatedByUsername = user.Username
	withUser.CreatedByAvatarURL = user.AvatarURL
	return withUser
}

func (q *FakeQuerier) workspaceBuildWithUserNoLock(tpl database.WorkspaceBuildTable) database.WorkspaceBuild {
	var user database.User
	for _, _user := range q.users {
		if _user.ID == tpl.InitiatorID {
			user = _user
			break
		}
	}
	var withUser database.WorkspaceBuild
	// This is a cheeky way to copy the fields over without explicitly listing them all.
	d, _ := json.Marshal(tpl)
	_ = json.Unmarshal(d, &withUser)
	withUser.InitiatorByUsername = user.Username
	withUser.InitiatorByAvatarUrl = user.AvatarURL
	return withUser
}

func (q *FakeQuerier) getTemplateVersionByIDNoLock(_ context.Context, templateVersionID uuid.UUID) (database.TemplateVersion, error) {
	for _, templateVersion := range q.templateVersions {
		if templateVersion.ID != templateVersionID {
			continue
		}
		return q.templateVersionWithUserNoLock(templateVersion), nil
	}
	return database.TemplateVersion{}, sql.ErrNoRows
}

func (q *FakeQuerier) getWorkspaceAgentByIDNoLock(_ context.Context, id uuid.UUID) (database.WorkspaceAgent, error) {
	// The schema sorts this by created at, so we iterate the array backwards.
	for i := len(q.workspaceAgents) - 1; i >= 0; i-- {
		agent := q.workspaceAgents[i]
		if agent.ID == id {
			return agent, nil
		}
	}
	return database.WorkspaceAgent{}, sql.ErrNoRows
}

func (q *FakeQuerier) getWorkspaceAgentsByResourceIDsNoLock(_ context.Context, resourceIDs []uuid.UUID) ([]database.WorkspaceAgent, error) {
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

func (q *FakeQuerier) getWorkspaceAppByAgentIDAndSlugNoLock(_ context.Context, arg database.GetWorkspaceAppByAgentIDAndSlugParams) (database.WorkspaceApp, error) {
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

func (q *FakeQuerier) getProvisionerJobByIDNoLock(_ context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
	for _, provisionerJob := range q.provisionerJobs {
		if provisionerJob.ID != id {
			continue
		}
		// clone the Tags before returning, since maps are reference types and
		// we don't want the caller to be able to mutate the map we have inside
		// dbmem!
		provisionerJob.Tags = maps.Clone(provisionerJob.Tags)
		return provisionerJob, nil
	}
	return database.ProvisionerJob{}, sql.ErrNoRows
}

func (q *FakeQuerier) getWorkspaceResourcesByJobIDNoLock(_ context.Context, jobID uuid.UUID) ([]database.WorkspaceResource, error) {
	resources := make([]database.WorkspaceResource, 0)
	for _, resource := range q.workspaceResources {
		if resource.JobID != jobID {
			continue
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func (q *FakeQuerier) getGroupByIDNoLock(_ context.Context, id uuid.UUID) (database.Group, error) {
	for _, group := range q.groups {
		if group.ID == id {
			return group, nil
		}
	}

	return database.Group{}, sql.ErrNoRows
}

// ErrUnimplemented is returned by methods only used by the enterprise/tailnet.pgCoord.  This coordinator explicitly
// depends on  postgres triggers that announce changes on the pubsub.  Implementing support for this in the fake
// database would  strongly couple the FakeQuerier to the pubsub, which is undesirable.  Furthermore, it makes little
// sense to directly  test the pgCoord against anything other than postgres.  The FakeQuerier is designed to allow us to
// test the Coderd  API, and for that kind of test, the in-memory, AGPL tailnet coordinator is sufficient.  Therefore,
// these methods  remain unimplemented in the FakeQuerier.
var ErrUnimplemented = xerrors.New("unimplemented")

func uniqueSortedUUIDs(uuids []uuid.UUID) []uuid.UUID {
	set := make(map[uuid.UUID]struct{})
	for _, id := range uuids {
		set[id] = struct{}{}
	}
	unique := make([]uuid.UUID, 0, len(set))
	for id := range set {
		unique = append(unique, id)
	}
	slices.SortFunc(unique, func(a, b uuid.UUID) int {
		return slice.Ascending(a.String(), b.String())
	})
	return unique
}

func (q *FakeQuerier) getOrganizationMemberNoLock(orgID uuid.UUID) []database.OrganizationMember {
	var members []database.OrganizationMember
	for _, member := range q.organizationMembers {
		if member.OrganizationID == orgID {
			members = append(members, member)
		}
	}

	return members
}

// getEveryoneGroupMembersNoLock fetches all the users in an organization.
func (q *FakeQuerier) getEveryoneGroupMembersNoLock(orgID uuid.UUID) []database.User {
	var (
		everyone   []database.User
		orgMembers = q.getOrganizationMemberNoLock(orgID)
	)
	for _, member := range orgMembers {
		user, err := q.getUserByIDNoLock(member.UserID)
		if err != nil {
			return nil
		}
		everyone = append(everyone, user)
	}
	return everyone
}

// isEveryoneGroup returns true if the provided ID matches
// an organization ID.
func (q *FakeQuerier) isEveryoneGroup(id uuid.UUID) bool {
	for _, org := range q.organizations {
		if org.ID == id {
			return true
		}
	}
	return false
}

func (q *FakeQuerier) GetActiveDBCryptKeys(_ context.Context) ([]database.DBCryptKey, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	ks := make([]database.DBCryptKey, 0, len(q.dbcryptKeys))
	for _, k := range q.dbcryptKeys {
		if !k.ActiveKeyDigest.Valid {
			continue
		}
		ks = append([]database.DBCryptKey{}, k)
	}
	return ks, nil
}

func maxTime(t, u time.Time) time.Time {
	if t.After(u) {
		return t
	}
	return u
}

func minTime(t, u time.Time) time.Time {
	if t.Before(u) {
		return t
	}
	return u
}

func provisonerJobStatus(j database.ProvisionerJob) database.ProvisionerJobStatus {
	if isNotNull(j.CompletedAt) {
		if j.Error.String != "" {
			return database.ProvisionerJobStatusFailed
		}
		if isNotNull(j.CanceledAt) {
			return database.ProvisionerJobStatusCanceled
		}
		return database.ProvisionerJobStatusSucceeded
	}

	if isNotNull(j.CanceledAt) {
		return database.ProvisionerJobStatusCanceling
	}
	if isNull(j.StartedAt) {
		return database.ProvisionerJobStatusPending
	}
	return database.ProvisionerJobStatusRunning
}

// isNull is only used in dbmem, so reflect is ok. Use this to make the logic
// look more similar to the postgres.
func isNull(v interface{}) bool {
	return !isNotNull(v)
}

func isNotNull(v interface{}) bool {
	return reflect.ValueOf(v).FieldByName("Valid").Bool()
}

func (*FakeQuerier) AcquireLock(_ context.Context, _ int64) error {
	return xerrors.New("AcquireLock must only be called within a transaction")
}

func (q *FakeQuerier) AcquireProvisionerJob(_ context.Context, arg database.AcquireProvisionerJobParams) (database.ProvisionerJob, error) {
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
		provisionerJob.JobStatus = provisonerJobStatus(provisionerJob)
		q.provisionerJobs[index] = provisionerJob
		// clone the Tags before returning, since maps are reference types and
		// we don't want the caller to be able to mutate the map we have inside
		// dbmem!
		provisionerJob.Tags = maps.Clone(provisionerJob.Tags)
		return provisionerJob, nil
	}
	return database.ProvisionerJob{}, sql.ErrNoRows
}

func (q *FakeQuerier) ActivityBumpWorkspace(ctx context.Context, arg database.ActivityBumpWorkspaceParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	workspace, err := q.getWorkspaceByIDNoLock(ctx, arg.WorkspaceID)
	if err != nil {
		return err
	}
	latestBuild, err := q.getLatestWorkspaceBuildByWorkspaceIDNoLock(ctx, arg.WorkspaceID)
	if err != nil {
		return err
	}

	now := dbtime.Now()
	for i := range q.workspaceBuilds {
		if q.workspaceBuilds[i].BuildNumber != latestBuild.BuildNumber {
			continue
		}
		// If the build is not active, do not bump.
		if q.workspaceBuilds[i].Transition != database.WorkspaceTransitionStart {
			return nil
		}
		// If the provisioner job is not completed, do not bump.
		pj, err := q.getProvisionerJobByIDNoLock(ctx, q.workspaceBuilds[i].JobID)
		if err != nil {
			return err
		}
		if !pj.CompletedAt.Valid {
			return nil
		}
		// Do not bump if the deadline is not set.
		if q.workspaceBuilds[i].Deadline.IsZero() {
			return nil
		}

		// Check the template default TTL.
		template, err := q.getTemplateByIDNoLock(ctx, workspace.TemplateID)
		if err != nil {
			return err
		}
		if template.ActivityBump == 0 {
			return nil
		}
		activityBump := time.Duration(template.ActivityBump)

		var ttlDur time.Duration
		if now.Add(activityBump).After(arg.NextAutostart) && arg.NextAutostart.After(now) {
			// Extend to TTL (NOT activity bump)
			add := arg.NextAutostart.Sub(now)
			if workspace.Ttl.Valid && template.AllowUserAutostop {
				add += time.Duration(workspace.Ttl.Int64)
			} else {
				add += time.Duration(template.DefaultTTL)
			}
			ttlDur = add
		} else {
			// Otherwise, default to regular activity bump duration.
			ttlDur = activityBump
		}

		// Only bump if 5% of the deadline has passed.
		ttlDur95 := ttlDur - (ttlDur / 20)
		minBumpDeadline := q.workspaceBuilds[i].Deadline.Add(-ttlDur95)
		if now.Before(minBumpDeadline) {
			return nil
		}

		// Bump.
		newDeadline := now.Add(ttlDur)
		// Never decrease deadlines from a bump
		newDeadline = maxTime(newDeadline, q.workspaceBuilds[i].Deadline)
		q.workspaceBuilds[i].UpdatedAt = now
		if !q.workspaceBuilds[i].MaxDeadline.IsZero() {
			q.workspaceBuilds[i].Deadline = minTime(newDeadline, q.workspaceBuilds[i].MaxDeadline)
		} else {
			q.workspaceBuilds[i].Deadline = newDeadline
		}
		return nil
	}

	return sql.ErrNoRows
}

func (q *FakeQuerier) AllUserIDs(_ context.Context) ([]uuid.UUID, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	userIDs := make([]uuid.UUID, 0, len(q.users))
	for idx := range q.users {
		userIDs = append(userIDs, q.users[idx].ID)
	}
	return userIDs, nil
}

func (q *FakeQuerier) ArchiveUnusedTemplateVersions(_ context.Context, arg database.ArchiveUnusedTemplateVersionsParams) ([]uuid.UUID, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}
	q.mutex.Lock()
	defer q.mutex.Unlock()
	type latestBuild struct {
		Number  int32
		Version uuid.UUID
	}
	latest := make(map[uuid.UUID]latestBuild)

	for _, b := range q.workspaceBuilds {
		v, ok := latest[b.WorkspaceID]
		if ok || b.BuildNumber < v.Number {
			// Not the latest
			continue
		}
		// Ignore deleted workspaces.
		if b.Transition == database.WorkspaceTransitionDelete {
			continue
		}
		latest[b.WorkspaceID] = latestBuild{
			Number:  b.BuildNumber,
			Version: b.TemplateVersionID,
		}
	}

	usedVersions := make(map[uuid.UUID]bool)
	for _, l := range latest {
		usedVersions[l.Version] = true
	}
	for _, tpl := range q.templates {
		usedVersions[tpl.ActiveVersionID] = true
	}

	var archived []uuid.UUID
	for i, v := range q.templateVersions {
		if arg.TemplateVersionID != uuid.Nil {
			if v.ID != arg.TemplateVersionID {
				continue
			}
		}
		if v.Archived {
			continue
		}

		if _, ok := usedVersions[v.ID]; !ok {
			var job *database.ProvisionerJob
			for i, j := range q.provisionerJobs {
				if v.JobID == j.ID {
					job = &q.provisionerJobs[i]
					break
				}
			}

			if arg.JobStatus.Valid {
				if job.JobStatus != arg.JobStatus.ProvisionerJobStatus {
					continue
				}
			}

			if job.JobStatus == database.ProvisionerJobStatusRunning || job.JobStatus == database.ProvisionerJobStatusPending {
				continue
			}

			v.Archived = true
			q.templateVersions[i] = v
			archived = append(archived, v.ID)
		}
	}

	return archived, nil
}

func (q *FakeQuerier) BatchUpdateWorkspaceLastUsedAt(_ context.Context, arg database.BatchUpdateWorkspaceLastUsedAtParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	// temporary map to avoid O(q.workspaces*arg.workspaceIds)
	m := make(map[uuid.UUID]struct{})
	for _, id := range arg.IDs {
		m[id] = struct{}{}
	}
	n := 0
	for i := 0; i < len(q.workspaces); i++ {
		if _, found := m[q.workspaces[i].ID]; !found {
			continue
		}
		q.workspaces[i].LastUsedAt = arg.LastUsedAt
		n++
	}
	return nil
}

func (*FakeQuerier) CleanTailnetCoordinators(_ context.Context) error {
	return ErrUnimplemented
}

func (*FakeQuerier) CleanTailnetLostPeers(context.Context) error {
	return ErrUnimplemented
}

func (*FakeQuerier) CleanTailnetTunnels(context.Context) error {
	return ErrUnimplemented
}

func (q *FakeQuerier) DeleteAPIKeyByID(_ context.Context, id string) error {
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

func (q *FakeQuerier) DeleteAPIKeysByUserID(_ context.Context, userID uuid.UUID) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i := len(q.apiKeys) - 1; i >= 0; i-- {
		if q.apiKeys[i].UserID == userID {
			q.apiKeys = append(q.apiKeys[:i], q.apiKeys[i+1:]...)
		}
	}

	return nil
}

func (*FakeQuerier) DeleteAllTailnetClientSubscriptions(_ context.Context, arg database.DeleteAllTailnetClientSubscriptionsParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	return ErrUnimplemented
}

func (*FakeQuerier) DeleteAllTailnetTunnels(_ context.Context, arg database.DeleteAllTailnetTunnelsParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	return ErrUnimplemented
}

func (q *FakeQuerier) DeleteApplicationConnectAPIKeysByUserID(_ context.Context, userID uuid.UUID) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i := len(q.apiKeys) - 1; i >= 0; i-- {
		if q.apiKeys[i].UserID == userID && q.apiKeys[i].Scope == database.APIKeyScopeApplicationConnect {
			q.apiKeys = append(q.apiKeys[:i], q.apiKeys[i+1:]...)
		}
	}

	return nil
}

func (*FakeQuerier) DeleteCoordinator(context.Context, uuid.UUID) error {
	return ErrUnimplemented
}

func (q *FakeQuerier) DeleteExternalAuthLink(_ context.Context, arg database.DeleteExternalAuthLinkParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, key := range q.externalAuthLinks {
		if key.UserID != arg.UserID {
			continue
		}
		if key.ProviderID != arg.ProviderID {
			continue
		}
		q.externalAuthLinks[index] = q.externalAuthLinks[len(q.externalAuthLinks)-1]
		q.externalAuthLinks = q.externalAuthLinks[:len(q.externalAuthLinks)-1]
		return nil
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) DeleteGitSSHKey(_ context.Context, userID uuid.UUID) error {
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

func (q *FakeQuerier) DeleteGroupByID(_ context.Context, id uuid.UUID) error {
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

func (q *FakeQuerier) DeleteGroupMemberFromGroup(_ context.Context, arg database.DeleteGroupMemberFromGroupParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, member := range q.groupMembers {
		if member.UserID == arg.UserID && member.GroupID == arg.GroupID {
			q.groupMembers = append(q.groupMembers[:i], q.groupMembers[i+1:]...)
		}
	}
	return nil
}

func (q *FakeQuerier) DeleteGroupMembersByOrgAndUser(_ context.Context, arg database.DeleteGroupMembersByOrgAndUserParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	newMembers := q.groupMembers[:0]
	for _, member := range q.groupMembers {
		if member.UserID != arg.UserID {
			// Do not delete the other members
			newMembers = append(newMembers, member)
		} else if member.UserID == arg.UserID {
			// We only want to delete from groups in the organization in the args.
			for _, group := range q.groups {
				// Find the group that the member is apartof.
				if group.ID == member.GroupID {
					// Only add back the member if the organization ID does not match
					// the arg organization ID. Since the arg is saying which
					// org to delete.
					if group.OrganizationID != arg.OrganizationID {
						newMembers = append(newMembers, member)
					}
					break
				}
			}
		}
	}
	q.groupMembers = newMembers

	return nil
}

func (q *FakeQuerier) DeleteLicense(_ context.Context, id int32) (int32, error) {
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

func (q *FakeQuerier) DeleteOAuth2ProviderAppByID(_ context.Context, id uuid.UUID) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, app := range q.oauth2ProviderApps {
		if app.ID == id {
			q.oauth2ProviderApps[index] = q.oauth2ProviderApps[len(q.oauth2ProviderApps)-1]
			q.oauth2ProviderApps = q.oauth2ProviderApps[:len(q.oauth2ProviderApps)-1]

			secrets := []database.OAuth2ProviderAppSecret{}
			for _, secret := range q.oauth2ProviderAppSecrets {
				if secret.AppID != id {
					secrets = append(secrets, secret)
				}
			}
			q.oauth2ProviderAppSecrets = secrets

			return nil
		}
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) DeleteOAuth2ProviderAppSecretByID(_ context.Context, id uuid.UUID) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, secret := range q.oauth2ProviderAppSecrets {
		if secret.ID == id {
			q.oauth2ProviderAppSecrets[index] = q.oauth2ProviderAppSecrets[len(q.oauth2ProviderAppSecrets)-1]
			q.oauth2ProviderAppSecrets = q.oauth2ProviderAppSecrets[:len(q.oauth2ProviderAppSecrets)-1]
			return nil
		}
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) DeleteOldProvisionerDaemons(_ context.Context) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	now := dbtime.Now()
	weekInterval := 7 * 24 * time.Hour
	weekAgo := now.Add(-weekInterval)

	var validDaemons []database.ProvisionerDaemon
	for _, p := range q.provisionerDaemons {
		if (p.CreatedAt.Before(weekAgo) && !p.LastSeenAt.Valid) || (p.LastSeenAt.Valid && p.LastSeenAt.Time.Before(weekAgo)) {
			continue
		}
		validDaemons = append(validDaemons, p)
	}
	q.provisionerDaemons = validDaemons
	return nil
}

func (q *FakeQuerier) DeleteOldWorkspaceAgentLogs(_ context.Context) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	now := dbtime.Now()
	weekInterval := 7 * 24 * time.Hour
	weekAgo := now.Add(-weekInterval)

	var validLogs []database.WorkspaceAgentLog
	for _, log := range q.workspaceAgentLogs {
		var toBeDeleted bool
		for _, agent := range q.workspaceAgents {
			if agent.ID == log.AgentID && agent.LastConnectedAt.Valid && agent.LastConnectedAt.Time.Before(weekAgo) {
				toBeDeleted = true
				break
			}
		}

		if !toBeDeleted {
			validLogs = append(validLogs, log)
		}
	}
	q.workspaceAgentLogs = validLogs
	return nil
}

func (q *FakeQuerier) DeleteOldWorkspaceAgentStats(_ context.Context) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	now := dbtime.Now()
	sixMonthInterval := 6 * 30 * 24 * time.Hour
	sixMonthsAgo := now.Add(-sixMonthInterval)

	var validStats []database.WorkspaceAgentStat
	for _, stat := range q.workspaceAgentStats {
		if stat.CreatedAt.Before(sixMonthsAgo) {
			continue
		}
		validStats = append(validStats, stat)
	}
	q.workspaceAgentStats = validStats
	return nil
}

func (q *FakeQuerier) DeleteReplicasUpdatedBefore(_ context.Context, before time.Time) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, replica := range q.replicas {
		if replica.UpdatedAt.Before(before) {
			q.replicas = append(q.replicas[:i], q.replicas[i+1:]...)
		}
	}

	return nil
}

func (*FakeQuerier) DeleteTailnetAgent(context.Context, database.DeleteTailnetAgentParams) (database.DeleteTailnetAgentRow, error) {
	return database.DeleteTailnetAgentRow{}, ErrUnimplemented
}

func (*FakeQuerier) DeleteTailnetClient(context.Context, database.DeleteTailnetClientParams) (database.DeleteTailnetClientRow, error) {
	return database.DeleteTailnetClientRow{}, ErrUnimplemented
}

func (*FakeQuerier) DeleteTailnetClientSubscription(context.Context, database.DeleteTailnetClientSubscriptionParams) error {
	return ErrUnimplemented
}

func (*FakeQuerier) DeleteTailnetPeer(_ context.Context, arg database.DeleteTailnetPeerParams) (database.DeleteTailnetPeerRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.DeleteTailnetPeerRow{}, err
	}

	return database.DeleteTailnetPeerRow{}, ErrUnimplemented
}

func (*FakeQuerier) DeleteTailnetTunnel(_ context.Context, arg database.DeleteTailnetTunnelParams) (database.DeleteTailnetTunnelRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.DeleteTailnetTunnelRow{}, err
	}

	return database.DeleteTailnetTunnelRow{}, ErrUnimplemented
}

func (q *FakeQuerier) DeleteWorkspaceAgentPortShare(_ context.Context, arg database.DeleteWorkspaceAgentPortShareParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, share := range q.workspaceAgentPortShares {
		if share.WorkspaceID == arg.WorkspaceID && share.AgentName == arg.AgentName && share.Port == arg.Port {
			q.workspaceAgentPortShares = append(q.workspaceAgentPortShares[:i], q.workspaceAgentPortShares[i+1:]...)
			return nil
		}
	}

	return nil
}

func (q *FakeQuerier) FavoriteWorkspace(_ context.Context, arg uuid.UUID) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i := 0; i < len(q.workspaces); i++ {
		if q.workspaces[i].ID != arg {
			continue
		}
		q.workspaces[i].Favorite = true
		return nil
	}
	return nil
}

func (q *FakeQuerier) GetAPIKeyByID(_ context.Context, id string) (database.APIKey, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, apiKey := range q.apiKeys {
		if apiKey.ID == id {
			return apiKey, nil
		}
	}
	return database.APIKey{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetAPIKeyByName(_ context.Context, params database.GetAPIKeyByNameParams) (database.APIKey, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if params.TokenName == "" {
		return database.APIKey{}, sql.ErrNoRows
	}
	for _, apiKey := range q.apiKeys {
		if params.UserID == apiKey.UserID && params.TokenName == apiKey.TokenName {
			return apiKey, nil
		}
	}
	return database.APIKey{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetAPIKeysByLoginType(_ context.Context, t database.LoginType) ([]database.APIKey, error) {
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

func (q *FakeQuerier) GetAPIKeysByUserID(_ context.Context, params database.GetAPIKeysByUserIDParams) ([]database.APIKey, error) {
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

func (q *FakeQuerier) GetAPIKeysLastUsedAfter(_ context.Context, after time.Time) ([]database.APIKey, error) {
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

func (q *FakeQuerier) GetActiveUserCount(_ context.Context) (int64, error) {
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

func (q *FakeQuerier) GetActiveWorkspaceBuildsByTemplateID(ctx context.Context, templateID uuid.UUID) ([]database.WorkspaceBuild, error) {
	workspaceIDs := func() []uuid.UUID {
		q.mutex.RLock()
		defer q.mutex.RUnlock()

		ids := []uuid.UUID{}
		for _, workspace := range q.workspaces {
			if workspace.TemplateID == templateID {
				ids = append(ids, workspace.ID)
			}
		}
		return ids
	}()

	builds, err := q.GetLatestWorkspaceBuildsByWorkspaceIDs(ctx, workspaceIDs)
	if err != nil {
		return nil, err
	}

	filteredBuilds := []database.WorkspaceBuild{}
	for _, build := range builds {
		if build.Transition == database.WorkspaceTransitionStart {
			filteredBuilds = append(filteredBuilds, build)
		}
	}
	return filteredBuilds, nil
}

func (*FakeQuerier) GetAllTailnetAgents(_ context.Context) ([]database.TailnetAgent, error) {
	return nil, ErrUnimplemented
}

func (*FakeQuerier) GetAllTailnetCoordinators(context.Context) ([]database.TailnetCoordinator, error) {
	return nil, ErrUnimplemented
}

func (*FakeQuerier) GetAllTailnetPeers(context.Context) ([]database.TailnetPeer, error) {
	return nil, ErrUnimplemented
}

func (*FakeQuerier) GetAllTailnetTunnels(context.Context) ([]database.TailnetTunnel, error) {
	return nil, ErrUnimplemented
}

func (q *FakeQuerier) GetAppSecurityKey(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.appSecurityKey, nil
}

func (q *FakeQuerier) GetApplicationName(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if q.applicationName == "" {
		return "", sql.ErrNoRows
	}

	return q.applicationName, nil
}

func (q *FakeQuerier) GetAuditLogsOffset(_ context.Context, arg database.GetAuditLogsOffsetParams) ([]database.GetAuditLogsOffsetRow, error) {
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
			workspaceBuild, err := q.getWorkspaceBuildByIDNoLock(context.Background(), alog.ResourceID)
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

func (q *FakeQuerier) GetAuthorizationUserRoles(_ context.Context, userID uuid.UUID) (database.GetAuthorizationUserRolesRow, error) {
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

func (q *FakeQuerier) GetDBCryptKeys(_ context.Context) ([]database.DBCryptKey, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	ks := make([]database.DBCryptKey, 0)
	ks = append(ks, q.dbcryptKeys...)
	return ks, nil
}

func (q *FakeQuerier) GetDERPMeshKey(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.derpMeshKey, nil
}

func (q *FakeQuerier) GetDefaultProxyConfig(_ context.Context) (database.GetDefaultProxyConfigRow, error) {
	return database.GetDefaultProxyConfigRow{
		DisplayName: q.defaultProxyDisplayName,
		IconUrl:     q.defaultProxyIconURL,
	}, nil
}

func (q *FakeQuerier) GetDeploymentDAUs(_ context.Context, tzOffset int32) ([]database.GetDeploymentDAUsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	seens := make(map[time.Time]map[uuid.UUID]struct{})

	for _, as := range q.workspaceAgentStats {
		if as.ConnectionCount == 0 {
			continue
		}
		date := as.CreatedAt.UTC().Add(time.Duration(tzOffset) * -1 * time.Hour).Truncate(time.Hour * 24)

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

func (q *FakeQuerier) GetDeploymentID(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.deploymentID, nil
}

func (q *FakeQuerier) GetDeploymentWorkspaceAgentStats(_ context.Context, createdAfter time.Time) (database.GetDeploymentWorkspaceAgentStatsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	agentStatsCreatedAfter := make([]database.WorkspaceAgentStat, 0)
	for _, agentStat := range q.workspaceAgentStats {
		if agentStat.CreatedAt.After(createdAfter) {
			agentStatsCreatedAfter = append(agentStatsCreatedAfter, agentStat)
		}
	}

	latestAgentStats := map[uuid.UUID]database.WorkspaceAgentStat{}
	for _, agentStat := range q.workspaceAgentStats {
		if agentStat.CreatedAt.After(createdAfter) {
			latestAgentStats[agentStat.AgentID] = agentStat
		}
	}

	stat := database.GetDeploymentWorkspaceAgentStatsRow{}
	for _, agentStat := range latestAgentStats {
		stat.SessionCountVSCode += agentStat.SessionCountVSCode
		stat.SessionCountJetBrains += agentStat.SessionCountJetBrains
		stat.SessionCountReconnectingPTY += agentStat.SessionCountReconnectingPTY
		stat.SessionCountSSH += agentStat.SessionCountSSH
	}

	latencies := make([]float64, 0)
	for _, agentStat := range agentStatsCreatedAfter {
		if agentStat.ConnectionMedianLatencyMS <= 0 {
			continue
		}
		stat.WorkspaceRxBytes += agentStat.RxBytes
		stat.WorkspaceTxBytes += agentStat.TxBytes
		latencies = append(latencies, agentStat.ConnectionMedianLatencyMS)
	}

	tryPercentile := func(fs []float64, p float64) float64 {
		if len(fs) == 0 {
			return -1
		}
		sort.Float64s(fs)
		return fs[int(float64(len(fs))*p/100)]
	}

	stat.WorkspaceConnectionLatency50 = tryPercentile(latencies, 50)
	stat.WorkspaceConnectionLatency95 = tryPercentile(latencies, 95)

	return stat, nil
}

func (q *FakeQuerier) GetDeploymentWorkspaceStats(ctx context.Context) (database.GetDeploymentWorkspaceStatsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	stat := database.GetDeploymentWorkspaceStatsRow{}
	for _, workspace := range q.workspaces {
		build, err := q.getLatestWorkspaceBuildByWorkspaceIDNoLock(ctx, workspace.ID)
		if err != nil {
			return stat, err
		}
		job, err := q.getProvisionerJobByIDNoLock(ctx, build.JobID)
		if err != nil {
			return stat, err
		}
		if !job.StartedAt.Valid {
			stat.PendingWorkspaces++
			continue
		}
		if job.StartedAt.Valid &&
			!job.CanceledAt.Valid &&
			time.Since(job.UpdatedAt) <= 30*time.Second &&
			!job.CompletedAt.Valid {
			stat.BuildingWorkspaces++
			continue
		}
		if job.CompletedAt.Valid &&
			!job.CanceledAt.Valid &&
			!job.Error.Valid {
			if build.Transition == database.WorkspaceTransitionStart {
				stat.RunningWorkspaces++
			}
			if build.Transition == database.WorkspaceTransitionStop {
				stat.StoppedWorkspaces++
			}
			continue
		}
		if job.CanceledAt.Valid || job.Error.Valid {
			stat.FailedWorkspaces++
			continue
		}
	}
	return stat, nil
}

func (q *FakeQuerier) GetExternalAuthLink(_ context.Context, arg database.GetExternalAuthLinkParams) (database.ExternalAuthLink, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.ExternalAuthLink{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()
	for _, gitAuthLink := range q.externalAuthLinks {
		if arg.UserID != gitAuthLink.UserID {
			continue
		}
		if arg.ProviderID != gitAuthLink.ProviderID {
			continue
		}
		return gitAuthLink, nil
	}
	return database.ExternalAuthLink{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetExternalAuthLinksByUserID(_ context.Context, userID uuid.UUID) ([]database.ExternalAuthLink, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	gals := make([]database.ExternalAuthLink, 0)
	for _, gal := range q.externalAuthLinks {
		if gal.UserID == userID {
			gals = append(gals, gal)
		}
	}
	return gals, nil
}

func (q *FakeQuerier) GetFileByHashAndCreator(_ context.Context, arg database.GetFileByHashAndCreatorParams) (database.File, error) {
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

func (q *FakeQuerier) GetFileByID(_ context.Context, id uuid.UUID) (database.File, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, file := range q.files {
		if file.ID == id {
			return file, nil
		}
	}
	return database.File{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetFileTemplates(_ context.Context, id uuid.UUID) ([]database.GetFileTemplatesRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	rows := make([]database.GetFileTemplatesRow, 0)
	var file database.File
	for _, f := range q.files {
		if f.ID == id {
			file = f
			break
		}
	}
	if file.Hash == "" {
		return rows, nil
	}

	for _, job := range q.provisionerJobs {
		if job.FileID == id {
			for _, version := range q.templateVersions {
				if version.JobID == job.ID {
					for _, template := range q.templates {
						if template.ID == version.TemplateID.UUID {
							rows = append(rows, database.GetFileTemplatesRow{
								FileID:                 file.ID,
								FileCreatedBy:          file.CreatedBy,
								TemplateID:             template.ID,
								TemplateOrganizationID: template.OrganizationID,
								TemplateCreatedBy:      template.CreatedBy,
								UserACL:                template.UserACL,
								GroupACL:               template.GroupACL,
							})
						}
					}
				}
			}
		}
	}

	return rows, nil
}

func (q *FakeQuerier) GetGitSSHKey(_ context.Context, userID uuid.UUID) (database.GitSSHKey, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, key := range q.gitSSHKey {
		if key.UserID == userID {
			return key, nil
		}
	}
	return database.GitSSHKey{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetGroupByID(ctx context.Context, id uuid.UUID) (database.Group, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getGroupByIDNoLock(ctx, id)
}

func (q *FakeQuerier) GetGroupByOrgAndName(_ context.Context, arg database.GetGroupByOrgAndNameParams) (database.Group, error) {
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

func (q *FakeQuerier) GetGroupMembers(_ context.Context, id uuid.UUID) ([]database.User, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if q.isEveryoneGroup(id) {
		return q.getEveryoneGroupMembersNoLock(id), nil
	}

	var members []database.GroupMember
	for _, member := range q.groupMembers {
		if member.GroupID == id {
			members = append(members, member)
		}
	}

	users := make([]database.User, 0, len(members))

	for _, member := range members {
		for _, user := range q.users {
			if user.ID == member.UserID && !user.Deleted {
				users = append(users, user)
				break
			}
		}
	}

	return users, nil
}

func (q *FakeQuerier) GetGroupsByOrganizationID(_ context.Context, id uuid.UUID) ([]database.Group, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	groups := make([]database.Group, 0, len(q.groups))
	for _, group := range q.groups {
		if group.OrganizationID == id {
			groups = append(groups, group)
		}
	}

	return groups, nil
}

func (q *FakeQuerier) GetHealthSettings(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if q.healthSettings == nil {
		return "{}", nil
	}

	return string(q.healthSettings), nil
}

func (q *FakeQuerier) GetHungProvisionerJobs(_ context.Context, hungSince time.Time) ([]database.ProvisionerJob, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	hungJobs := []database.ProvisionerJob{}
	for _, provisionerJob := range q.provisionerJobs {
		if provisionerJob.StartedAt.Valid && !provisionerJob.CompletedAt.Valid && provisionerJob.UpdatedAt.Before(hungSince) {
			// clone the Tags before appending, since maps are reference types and
			// we don't want the caller to be able to mutate the map we have inside
			// dbmem!
			provisionerJob.Tags = maps.Clone(provisionerJob.Tags)
			hungJobs = append(hungJobs, provisionerJob)
		}
	}
	return hungJobs, nil
}

func (q *FakeQuerier) GetJFrogXrayScanByWorkspaceAndAgentID(_ context.Context, arg database.GetJFrogXrayScanByWorkspaceAndAgentIDParams) (database.JfrogXrayScan, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.JfrogXrayScan{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, scan := range q.jfrogXRayScans {
		if scan.AgentID == arg.AgentID && scan.WorkspaceID == arg.WorkspaceID {
			return scan, nil
		}
	}

	return database.JfrogXrayScan{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetLastUpdateCheck(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if q.lastUpdateCheck == nil {
		return "", sql.ErrNoRows
	}
	return string(q.lastUpdateCheck), nil
}

func (q *FakeQuerier) GetLatestWorkspaceBuildByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getLatestWorkspaceBuildByWorkspaceIDNoLock(ctx, workspaceID)
}

func (q *FakeQuerier) GetLatestWorkspaceBuilds(_ context.Context) ([]database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	builds := make(map[uuid.UUID]database.WorkspaceBuild)
	buildNumbers := make(map[uuid.UUID]int32)
	for _, workspaceBuild := range q.workspaceBuilds {
		id := workspaceBuild.WorkspaceID
		if workspaceBuild.BuildNumber > buildNumbers[id] {
			builds[id] = q.workspaceBuildWithUserNoLock(workspaceBuild)
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

func (q *FakeQuerier) GetLatestWorkspaceBuildsByWorkspaceIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	builds := make(map[uuid.UUID]database.WorkspaceBuild)
	buildNumbers := make(map[uuid.UUID]int32)
	for _, workspaceBuild := range q.workspaceBuilds {
		for _, id := range ids {
			if id == workspaceBuild.WorkspaceID && workspaceBuild.BuildNumber > buildNumbers[id] {
				builds[id] = q.workspaceBuildWithUserNoLock(workspaceBuild)
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

func (q *FakeQuerier) GetLicenseByID(_ context.Context, id int32) (database.License, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, license := range q.licenses {
		if license.ID == id {
			return license, nil
		}
	}
	return database.License{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetLicenses(_ context.Context) ([]database.License, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	results := append([]database.License{}, q.licenses...)
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	return results, nil
}

func (q *FakeQuerier) GetLogoURL(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if q.logoURL == "" {
		return "", sql.ErrNoRows
	}

	return q.logoURL, nil
}

func (q *FakeQuerier) GetOAuth2ProviderAppByID(_ context.Context, id uuid.UUID) (database.OAuth2ProviderApp, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, app := range q.oauth2ProviderApps {
		if app.ID == id {
			return app, nil
		}
	}
	return database.OAuth2ProviderApp{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetOAuth2ProviderAppSecretByID(_ context.Context, id uuid.UUID) (database.OAuth2ProviderAppSecret, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, secret := range q.oauth2ProviderAppSecrets {
		if secret.ID == id {
			return secret, nil
		}
	}
	return database.OAuth2ProviderAppSecret{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetOAuth2ProviderAppSecretsByAppID(_ context.Context, appID uuid.UUID) ([]database.OAuth2ProviderAppSecret, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, app := range q.oauth2ProviderApps {
		if app.ID == appID {
			secrets := []database.OAuth2ProviderAppSecret{}
			for _, secret := range q.oauth2ProviderAppSecrets {
				if secret.AppID == appID {
					secrets = append(secrets, secret)
				}
			}

			slices.SortFunc(secrets, func(a, b database.OAuth2ProviderAppSecret) int {
				if a.CreatedAt.Before(b.CreatedAt) {
					return -1
				} else if a.CreatedAt.Equal(b.CreatedAt) {
					return 0
				}
				return 1
			})
			return secrets, nil
		}
	}

	return []database.OAuth2ProviderAppSecret{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetOAuth2ProviderApps(_ context.Context) ([]database.OAuth2ProviderApp, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	slices.SortFunc(q.oauth2ProviderApps, func(a, b database.OAuth2ProviderApp) int {
		return slice.Ascending(a.Name, b.Name)
	})
	return q.oauth2ProviderApps, nil
}

func (q *FakeQuerier) GetOAuthSigningKey(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.oauthSigningKey, nil
}

func (q *FakeQuerier) GetOrganizationByID(_ context.Context, id uuid.UUID) (database.Organization, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, organization := range q.organizations {
		if organization.ID == id {
			return organization, nil
		}
	}
	return database.Organization{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetOrganizationByName(_ context.Context, name string) (database.Organization, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, organization := range q.organizations {
		if organization.Name == name {
			return organization, nil
		}
	}
	return database.Organization{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetOrganizationIDsByMemberIDs(_ context.Context, ids []uuid.UUID) ([]database.GetOrganizationIDsByMemberIDsRow, error) {
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

func (q *FakeQuerier) GetOrganizationMemberByUserID(_ context.Context, arg database.GetOrganizationMemberByUserIDParams) (database.OrganizationMember, error) {
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

func (q *FakeQuerier) GetOrganizationMembershipsByUserID(_ context.Context, userID uuid.UUID) ([]database.OrganizationMember, error) {
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

func (q *FakeQuerier) GetOrganizations(_ context.Context) ([]database.Organization, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if len(q.organizations) == 0 {
		return nil, sql.ErrNoRows
	}
	return q.organizations, nil
}

func (q *FakeQuerier) GetOrganizationsByUserID(_ context.Context, userID uuid.UUID) ([]database.Organization, error) {
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

func (q *FakeQuerier) GetParameterSchemasByJobID(_ context.Context, jobID uuid.UUID) ([]database.ParameterSchema, error) {
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

func (q *FakeQuerier) GetPreviousTemplateVersion(_ context.Context, arg database.GetPreviousTemplateVersionParams) (database.TemplateVersion, error) {
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
		currentTemplateVersion = q.templateVersionWithUserNoLock(templateVersion)
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
			previousTemplateVersions = append(previousTemplateVersions, q.templateVersionWithUserNoLock(templateVersion))
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

func (q *FakeQuerier) GetProvisionerDaemons(_ context.Context) ([]database.ProvisionerDaemon, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if len(q.provisionerDaemons) == 0 {
		return nil, sql.ErrNoRows
	}
	// copy the data so that the caller can't manipulate any data inside dbmem
	// after returning
	out := make([]database.ProvisionerDaemon, len(q.provisionerDaemons))
	copy(out, q.provisionerDaemons)
	for i := range out {
		// maps are reference types, so we need to clone them
		out[i].Tags = maps.Clone(out[i].Tags)
	}
	return out, nil
}

func (q *FakeQuerier) GetProvisionerJobByID(ctx context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getProvisionerJobByIDNoLock(ctx, id)
}

func (q *FakeQuerier) GetProvisionerJobsByIDs(_ context.Context, ids []uuid.UUID) ([]database.ProvisionerJob, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	jobs := make([]database.ProvisionerJob, 0)
	for _, job := range q.provisionerJobs {
		for _, id := range ids {
			if id == job.ID {
				// clone the Tags before appending, since maps are reference types and
				// we don't want the caller to be able to mutate the map we have inside
				// dbmem!
				job.Tags = maps.Clone(job.Tags)
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

func (q *FakeQuerier) GetProvisionerJobsByIDsWithQueuePosition(_ context.Context, ids []uuid.UUID) ([]database.GetProvisionerJobsByIDsWithQueuePositionRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	jobs := make([]database.GetProvisionerJobsByIDsWithQueuePositionRow, 0)
	queuePosition := int64(1)
	for _, job := range q.provisionerJobs {
		for _, id := range ids {
			if id == job.ID {
				// clone the Tags before appending, since maps are reference types and
				// we don't want the caller to be able to mutate the map we have inside
				// dbmem!
				job.Tags = maps.Clone(job.Tags)
				job := database.GetProvisionerJobsByIDsWithQueuePositionRow{
					ProvisionerJob: job,
				}
				if !job.ProvisionerJob.StartedAt.Valid {
					job.QueuePosition = queuePosition
				}
				jobs = append(jobs, job)
				break
			}
		}
		if !job.StartedAt.Valid {
			queuePosition++
		}
	}
	for _, job := range jobs {
		if !job.ProvisionerJob.StartedAt.Valid {
			// Set it to the max position!
			job.QueueSize = queuePosition
		}
	}
	return jobs, nil
}

func (q *FakeQuerier) GetProvisionerJobsCreatedAfter(_ context.Context, after time.Time) ([]database.ProvisionerJob, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	jobs := make([]database.ProvisionerJob, 0)
	for _, job := range q.provisionerJobs {
		if job.CreatedAt.After(after) {
			// clone the Tags before appending, since maps are reference types and
			// we don't want the caller to be able to mutate the map we have inside
			// dbmem!
			job.Tags = maps.Clone(job.Tags)
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

func (q *FakeQuerier) GetProvisionerLogsAfterID(_ context.Context, arg database.GetProvisionerLogsAfterIDParams) ([]database.ProvisionerJobLog, error) {
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
		if jobLog.ID <= arg.CreatedAfter {
			continue
		}
		logs = append(logs, jobLog)
	}
	return logs, nil
}

func (q *FakeQuerier) GetQuotaAllowanceForUser(_ context.Context, userID uuid.UUID) (int64, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var sum int64
	for _, member := range q.groupMembers {
		if member.UserID != userID {
			continue
		}
		for _, group := range q.groups {
			if group.ID == member.GroupID {
				sum += int64(group.QuotaAllowance)
				continue
			}
		}
	}
	// Grab the quota for the Everyone group.
	for _, group := range q.groups {
		if group.ID == group.OrganizationID {
			sum += int64(group.QuotaAllowance)
			break
		}
	}
	return sum, nil
}

func (q *FakeQuerier) GetQuotaConsumedForUser(_ context.Context, userID uuid.UUID) (int64, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var sum int64
	for _, workspace := range q.workspaces {
		if workspace.OwnerID != userID {
			continue
		}
		if workspace.Deleted {
			continue
		}

		var lastBuild database.WorkspaceBuildTable
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

func (q *FakeQuerier) GetReplicaByID(_ context.Context, id uuid.UUID) (database.Replica, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, replica := range q.replicas {
		if replica.ID == id {
			return replica, nil
		}
	}

	return database.Replica{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetReplicasUpdatedAfter(_ context.Context, updatedAt time.Time) ([]database.Replica, error) {
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

func (q *FakeQuerier) GetServiceBanner(_ context.Context) (string, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if q.serviceBanner == nil {
		return "", sql.ErrNoRows
	}

	return string(q.serviceBanner), nil
}

func (*FakeQuerier) GetTailnetAgents(context.Context, uuid.UUID) ([]database.TailnetAgent, error) {
	return nil, ErrUnimplemented
}

func (*FakeQuerier) GetTailnetClientsForAgent(context.Context, uuid.UUID) ([]database.TailnetClient, error) {
	return nil, ErrUnimplemented
}

func (*FakeQuerier) GetTailnetPeers(context.Context, uuid.UUID) ([]database.TailnetPeer, error) {
	return nil, ErrUnimplemented
}

func (*FakeQuerier) GetTailnetTunnelPeerBindings(context.Context, uuid.UUID) ([]database.GetTailnetTunnelPeerBindingsRow, error) {
	return nil, ErrUnimplemented
}

func (*FakeQuerier) GetTailnetTunnelPeerIDs(context.Context, uuid.UUID) ([]database.GetTailnetTunnelPeerIDsRow, error) {
	return nil, ErrUnimplemented
}

func (q *FakeQuerier) GetTemplateAppInsights(ctx context.Context, arg database.GetTemplateAppInsightsParams) ([]database.GetTemplateAppInsightsRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	type appKey struct {
		AccessMethod string
		SlugOrPort   string
		Slug         string
		DisplayName  string
		Icon         string
	}
	type uniqueKey struct {
		TemplateID uuid.UUID
		UserID     uuid.UUID
		AgentID    uuid.UUID
		AppKey     appKey
	}

	appUsageIntervalsByUserAgentApp := make(map[uniqueKey]map[time.Time]int64)
	for _, s := range q.workspaceAppStats {
		// (was.session_started_at >= ts.from_ AND was.session_started_at < ts.to_)
		// OR (was.session_ended_at > ts.from_ AND was.session_ended_at < ts.to_)
		// OR (was.session_started_at < ts.from_ AND was.session_ended_at >= ts.to_)
		if !(((s.SessionStartedAt.After(arg.StartTime) || s.SessionStartedAt.Equal(arg.StartTime)) && s.SessionStartedAt.Before(arg.EndTime)) ||
			(s.SessionEndedAt.After(arg.StartTime) && s.SessionEndedAt.Before(arg.EndTime)) ||
			(s.SessionStartedAt.Before(arg.StartTime) && (s.SessionEndedAt.After(arg.EndTime) || s.SessionEndedAt.Equal(arg.EndTime)))) {
			continue
		}

		w, err := q.getWorkspaceByIDNoLock(ctx, s.WorkspaceID)
		if err != nil {
			return nil, err
		}

		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, w.TemplateID) {
			continue
		}

		app, _ := q.getWorkspaceAppByAgentIDAndSlugNoLock(ctx, database.GetWorkspaceAppByAgentIDAndSlugParams{
			AgentID: s.AgentID,
			Slug:    s.SlugOrPort,
		})

		key := uniqueKey{
			TemplateID: w.TemplateID,
			UserID:     s.UserID,
			AgentID:    s.AgentID,
			AppKey: appKey{
				AccessMethod: s.AccessMethod,
				SlugOrPort:   s.SlugOrPort,
				Slug:         app.Slug,
				DisplayName:  app.DisplayName,
				Icon:         app.Icon,
			},
		}
		if appUsageIntervalsByUserAgentApp[key] == nil {
			appUsageIntervalsByUserAgentApp[key] = make(map[time.Time]int64)
		}

		t := s.SessionStartedAt.Truncate(5 * time.Minute)
		if t.Before(arg.StartTime) {
			t = arg.StartTime
		}
		for t.Before(s.SessionEndedAt) && t.Before(arg.EndTime) {
			appUsageIntervalsByUserAgentApp[key][t] = 60 // 1 minute.
			t = t.Add(1 * time.Minute)
		}
	}

	appUsageTemplateIDs := make(map[appKey]map[uuid.UUID]struct{})
	appUsageUserIDs := make(map[appKey]map[uuid.UUID]struct{})
	appUsage := make(map[appKey]int64)
	for uniqueKey, usage := range appUsageIntervalsByUserAgentApp {
		for _, seconds := range usage {
			if appUsageTemplateIDs[uniqueKey.AppKey] == nil {
				appUsageTemplateIDs[uniqueKey.AppKey] = make(map[uuid.UUID]struct{})
			}
			appUsageTemplateIDs[uniqueKey.AppKey][uniqueKey.TemplateID] = struct{}{}
			if appUsageUserIDs[uniqueKey.AppKey] == nil {
				appUsageUserIDs[uniqueKey.AppKey] = make(map[uuid.UUID]struct{})
			}
			appUsageUserIDs[uniqueKey.AppKey][uniqueKey.UserID] = struct{}{}
			appUsage[uniqueKey.AppKey] += seconds
		}
	}

	var rows []database.GetTemplateAppInsightsRow
	for appKey, usage := range appUsage {
		templateIDs := make([]uuid.UUID, 0, len(appUsageTemplateIDs[appKey]))
		for templateID := range appUsageTemplateIDs[appKey] {
			templateIDs = append(templateIDs, templateID)
		}
		slices.SortFunc(templateIDs, func(a, b uuid.UUID) int {
			return slice.Ascending(a.String(), b.String())
		})
		activeUserIDs := make([]uuid.UUID, 0, len(appUsageUserIDs[appKey]))
		for userID := range appUsageUserIDs[appKey] {
			activeUserIDs = append(activeUserIDs, userID)
		}
		slices.SortFunc(activeUserIDs, func(a, b uuid.UUID) int {
			return slice.Ascending(a.String(), b.String())
		})

		rows = append(rows, database.GetTemplateAppInsightsRow{
			TemplateIDs:   templateIDs,
			ActiveUserIDs: activeUserIDs,
			AccessMethod:  appKey.AccessMethod,
			SlugOrPort:    appKey.SlugOrPort,
			DisplayName:   sql.NullString{String: appKey.DisplayName, Valid: appKey.DisplayName != ""},
			Icon:          sql.NullString{String: appKey.Icon, Valid: appKey.Icon != ""},
			IsApp:         appKey.Slug != "",
			UsageSeconds:  usage,
		})
	}

	// NOTE(mafredri): Add sorting if we decide on how to handle PostgreSQL collations.
	// ORDER BY access_method, slug_or_port, display_name, icon, is_app
	return rows, nil
}

func (q *FakeQuerier) GetTemplateAppInsightsByTemplate(ctx context.Context, arg database.GetTemplateAppInsightsByTemplateParams) ([]database.GetTemplateAppInsightsByTemplateRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	type uniqueKey struct {
		TemplateID  uuid.UUID
		DisplayName string
		Slug        string
	}

	// map (TemplateID + DisplayName + Slug) x time.Time x UserID x <usage>
	usageByTemplateAppUser := map[uniqueKey]map[time.Time]map[uuid.UUID]int64{}

	// Review agent stats in terms of usage
	for _, s := range q.workspaceAppStats {
		// (was.session_started_at >= ts.from_ AND was.session_started_at < ts.to_)
		// OR (was.session_ended_at > ts.from_ AND was.session_ended_at < ts.to_)
		// OR (was.session_started_at < ts.from_ AND was.session_ended_at >= ts.to_)
		if !(((s.SessionStartedAt.After(arg.StartTime) || s.SessionStartedAt.Equal(arg.StartTime)) && s.SessionStartedAt.Before(arg.EndTime)) ||
			(s.SessionEndedAt.After(arg.StartTime) && s.SessionEndedAt.Before(arg.EndTime)) ||
			(s.SessionStartedAt.Before(arg.StartTime) && (s.SessionEndedAt.After(arg.EndTime) || s.SessionEndedAt.Equal(arg.EndTime)))) {
			continue
		}

		w, err := q.getWorkspaceByIDNoLock(ctx, s.WorkspaceID)
		if err != nil {
			return nil, err
		}

		app, _ := q.getWorkspaceAppByAgentIDAndSlugNoLock(ctx, database.GetWorkspaceAppByAgentIDAndSlugParams{
			AgentID: s.AgentID,
			Slug:    s.SlugOrPort,
		})

		key := uniqueKey{
			TemplateID:  w.TemplateID,
			DisplayName: app.DisplayName,
			Slug:        app.Slug,
		}

		t := s.SessionStartedAt.Truncate(time.Minute)
		if t.Before(arg.StartTime) {
			t = arg.StartTime
		}
		for t.Before(s.SessionEndedAt) && t.Before(arg.EndTime) {
			if _, ok := usageByTemplateAppUser[key]; !ok {
				usageByTemplateAppUser[key] = map[time.Time]map[uuid.UUID]int64{}
			}
			if _, ok := usageByTemplateAppUser[key][t]; !ok {
				usageByTemplateAppUser[key][t] = map[uuid.UUID]int64{}
			}
			if _, ok := usageByTemplateAppUser[key][t][s.UserID]; !ok {
				usageByTemplateAppUser[key][t][s.UserID] = 60 // 1 minute
			}
			t = t.Add(1 * time.Minute)
		}
	}

	// Sort usage data
	usageKeys := make([]uniqueKey, len(usageByTemplateAppUser))
	var i int
	for key := range usageByTemplateAppUser {
		usageKeys[i] = key
		i++
	}

	slices.SortFunc(usageKeys, func(a, b uniqueKey) int {
		if a.TemplateID != b.TemplateID {
			return slice.Ascending(a.TemplateID.String(), b.TemplateID.String())
		}
		if a.DisplayName != b.DisplayName {
			return slice.Ascending(a.DisplayName, b.DisplayName)
		}
		return slice.Ascending(a.Slug, b.Slug)
	})

	// Build result
	var result []database.GetTemplateAppInsightsByTemplateRow
	for _, usageKey := range usageKeys {
		r := database.GetTemplateAppInsightsByTemplateRow{
			TemplateID:  usageKey.TemplateID,
			DisplayName: sql.NullString{String: usageKey.DisplayName, Valid: true},
			SlugOrPort:  usageKey.Slug,
		}
		for _, mUserUsage := range usageByTemplateAppUser[usageKey] {
			r.ActiveUsers += int64(len(mUserUsage))
			for _, usage := range mUserUsage {
				r.UsageSeconds += usage
			}
		}
		result = append(result, r)
	}
	return result, nil
}

func (q *FakeQuerier) GetTemplateAverageBuildTime(ctx context.Context, arg database.GetTemplateAverageBuildTimeParams) (database.GetTemplateAverageBuildTimeRow, error) {
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

func (q *FakeQuerier) GetTemplateByID(ctx context.Context, id uuid.UUID) (database.Template, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getTemplateByIDNoLock(ctx, id)
}

func (q *FakeQuerier) GetTemplateByOrganizationAndName(_ context.Context, arg database.GetTemplateByOrganizationAndNameParams) (database.Template, error) {
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
		return q.templateWithUserNoLock(template), nil
	}
	return database.Template{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetTemplateDAUs(_ context.Context, arg database.GetTemplateDAUsParams) ([]database.GetTemplateDAUsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	seens := make(map[time.Time]map[uuid.UUID]struct{})

	for _, as := range q.workspaceAgentStats {
		if as.TemplateID != arg.TemplateID {
			continue
		}
		if as.ConnectionCount == 0 {
			continue
		}

		date := as.CreatedAt.UTC().Add(time.Duration(arg.TzOffset) * time.Hour * -1).Truncate(time.Hour * 24)

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

func (q *FakeQuerier) GetTemplateInsights(_ context.Context, arg database.GetTemplateInsightsParams) (database.GetTemplateInsightsRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.GetTemplateInsightsRow{}, err
	}

	templateIDSet := make(map[uuid.UUID]struct{})
	appUsageIntervalsByUser := make(map[uuid.UUID]map[time.Time]*database.GetTemplateInsightsRow)

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, s := range q.workspaceAgentStats {
		if s.CreatedAt.Before(arg.StartTime) || s.CreatedAt.Equal(arg.EndTime) || s.CreatedAt.After(arg.EndTime) {
			continue
		}
		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, s.TemplateID) {
			continue
		}
		if s.ConnectionCount == 0 {
			continue
		}

		templateIDSet[s.TemplateID] = struct{}{}
		if appUsageIntervalsByUser[s.UserID] == nil {
			appUsageIntervalsByUser[s.UserID] = make(map[time.Time]*database.GetTemplateInsightsRow)
		}
		t := s.CreatedAt.Truncate(time.Minute)
		if _, ok := appUsageIntervalsByUser[s.UserID][t]; !ok {
			appUsageIntervalsByUser[s.UserID][t] = &database.GetTemplateInsightsRow{}
		}

		if s.SessionCountJetBrains > 0 {
			appUsageIntervalsByUser[s.UserID][t].UsageJetbrainsSeconds = 60
		}
		if s.SessionCountVSCode > 0 {
			appUsageIntervalsByUser[s.UserID][t].UsageVscodeSeconds = 60
		}
		if s.SessionCountReconnectingPTY > 0 {
			appUsageIntervalsByUser[s.UserID][t].UsageReconnectingPtySeconds = 60
		}
		if s.SessionCountSSH > 0 {
			appUsageIntervalsByUser[s.UserID][t].UsageSshSeconds = 60
		}
	}

	templateIDs := make([]uuid.UUID, 0, len(templateIDSet))
	for templateID := range templateIDSet {
		templateIDs = append(templateIDs, templateID)
	}
	slices.SortFunc(templateIDs, func(a, b uuid.UUID) int {
		return slice.Ascending(a.String(), b.String())
	})
	activeUserIDs := make([]uuid.UUID, 0, len(appUsageIntervalsByUser))
	for userID := range appUsageIntervalsByUser {
		activeUserIDs = append(activeUserIDs, userID)
	}

	result := database.GetTemplateInsightsRow{
		TemplateIDs:   templateIDs,
		ActiveUserIDs: activeUserIDs,
	}
	for _, intervals := range appUsageIntervalsByUser {
		for _, interval := range intervals {
			result.UsageJetbrainsSeconds += interval.UsageJetbrainsSeconds
			result.UsageVscodeSeconds += interval.UsageVscodeSeconds
			result.UsageReconnectingPtySeconds += interval.UsageReconnectingPtySeconds
			result.UsageSshSeconds += interval.UsageSshSeconds
		}
	}
	return result, nil
}

func (q *FakeQuerier) GetTemplateInsightsByInterval(ctx context.Context, arg database.GetTemplateInsightsByIntervalParams) ([]database.GetTemplateInsightsByIntervalRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	type statByInterval struct {
		startTime, endTime time.Time
		userSet            map[uuid.UUID]struct{}
		templateIDSet      map[uuid.UUID]struct{}
	}

	statsByInterval := []statByInterval{{arg.StartTime, arg.StartTime.AddDate(0, 0, int(arg.IntervalDays)), make(map[uuid.UUID]struct{}), make(map[uuid.UUID]struct{})}}
	for statsByInterval[len(statsByInterval)-1].endTime.Before(arg.EndTime) {
		statsByInterval = append(statsByInterval, statByInterval{statsByInterval[len(statsByInterval)-1].endTime, statsByInterval[len(statsByInterval)-1].endTime.AddDate(0, 0, int(arg.IntervalDays)), make(map[uuid.UUID]struct{}), make(map[uuid.UUID]struct{})})
	}
	if statsByInterval[len(statsByInterval)-1].endTime.After(arg.EndTime) {
		statsByInterval[len(statsByInterval)-1].endTime = arg.EndTime
	}

	for _, s := range q.workspaceAgentStats {
		if s.CreatedAt.Before(arg.StartTime) || s.CreatedAt.Equal(arg.EndTime) || s.CreatedAt.After(arg.EndTime) {
			continue
		}
		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, s.TemplateID) {
			continue
		}
		if s.ConnectionCount == 0 {
			continue
		}

		for _, ds := range statsByInterval {
			if s.CreatedAt.Before(ds.startTime) || s.CreatedAt.Equal(ds.endTime) || s.CreatedAt.After(ds.endTime) {
				continue
			}
			ds.userSet[s.UserID] = struct{}{}
			ds.templateIDSet[s.TemplateID] = struct{}{}
		}
	}

	for _, s := range q.workspaceAppStats {
		w, err := q.getWorkspaceByIDNoLock(ctx, s.WorkspaceID)
		if err != nil {
			return nil, err
		}

		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, w.TemplateID) {
			continue
		}

		for _, ds := range statsByInterval {
			// (was.session_started_at >= ts.from_ AND was.session_started_at < ts.to_)
			// OR (was.session_ended_at > ts.from_ AND was.session_ended_at < ts.to_)
			// OR (was.session_started_at < ts.from_ AND was.session_ended_at >= ts.to_)
			if !(((s.SessionStartedAt.After(ds.startTime) || s.SessionStartedAt.Equal(ds.startTime)) && s.SessionStartedAt.Before(ds.endTime)) ||
				(s.SessionEndedAt.After(ds.startTime) && s.SessionEndedAt.Before(ds.endTime)) ||
				(s.SessionStartedAt.Before(ds.startTime) && (s.SessionEndedAt.After(ds.endTime) || s.SessionEndedAt.Equal(ds.endTime)))) {
				continue
			}

			ds.userSet[s.UserID] = struct{}{}
			ds.templateIDSet[w.TemplateID] = struct{}{}
		}
	}

	var result []database.GetTemplateInsightsByIntervalRow
	for _, ds := range statsByInterval {
		templateIDs := make([]uuid.UUID, 0, len(ds.templateIDSet))
		for templateID := range ds.templateIDSet {
			templateIDs = append(templateIDs, templateID)
		}
		slices.SortFunc(templateIDs, func(a, b uuid.UUID) int {
			return slice.Ascending(a.String(), b.String())
		})
		result = append(result, database.GetTemplateInsightsByIntervalRow{
			StartTime:   ds.startTime,
			EndTime:     ds.endTime,
			TemplateIDs: templateIDs,
			ActiveUsers: int64(len(ds.userSet)),
		})
	}
	return result, nil
}

func (q *FakeQuerier) GetTemplateInsightsByTemplate(_ context.Context, arg database.GetTemplateInsightsByTemplateParams) ([]database.GetTemplateInsightsByTemplateRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// map time.Time x TemplateID x UserID x <usage>
	appUsageByTemplateAndUser := map[time.Time]map[uuid.UUID]map[uuid.UUID]database.GetTemplateInsightsByTemplateRow{}

	// Review agent stats in terms of usage
	templateIDSet := make(map[uuid.UUID]struct{})

	for _, s := range q.workspaceAgentStats {
		if s.CreatedAt.Before(arg.StartTime) || s.CreatedAt.Equal(arg.EndTime) || s.CreatedAt.After(arg.EndTime) {
			continue
		}
		if s.ConnectionCount == 0 {
			continue
		}

		t := s.CreatedAt.Truncate(time.Minute)
		templateIDSet[s.TemplateID] = struct{}{}

		if _, ok := appUsageByTemplateAndUser[t]; !ok {
			appUsageByTemplateAndUser[t] = make(map[uuid.UUID]map[uuid.UUID]database.GetTemplateInsightsByTemplateRow)
		}

		if _, ok := appUsageByTemplateAndUser[t][s.TemplateID]; !ok {
			appUsageByTemplateAndUser[t][s.TemplateID] = make(map[uuid.UUID]database.GetTemplateInsightsByTemplateRow)
		}

		if _, ok := appUsageByTemplateAndUser[t][s.TemplateID][s.UserID]; !ok {
			appUsageByTemplateAndUser[t][s.TemplateID][s.UserID] = database.GetTemplateInsightsByTemplateRow{}
		}

		u := appUsageByTemplateAndUser[t][s.TemplateID][s.UserID]
		if s.SessionCountJetBrains > 0 {
			u.UsageJetbrainsSeconds = 60
		}
		if s.SessionCountVSCode > 0 {
			u.UsageVscodeSeconds = 60
		}
		if s.SessionCountReconnectingPTY > 0 {
			u.UsageReconnectingPtySeconds = 60
		}
		if s.SessionCountSSH > 0 {
			u.UsageSshSeconds = 60
		}
		appUsageByTemplateAndUser[t][s.TemplateID][s.UserID] = u
	}

	// Sort used templates
	templateIDs := make([]uuid.UUID, 0, len(templateIDSet))
	for templateID := range templateIDSet {
		templateIDs = append(templateIDs, templateID)
	}
	slices.SortFunc(templateIDs, func(a, b uuid.UUID) int {
		return slice.Ascending(a.String(), b.String())
	})

	// Build result
	var result []database.GetTemplateInsightsByTemplateRow
	for _, templateID := range templateIDs {
		r := database.GetTemplateInsightsByTemplateRow{
			TemplateID: templateID,
		}

		uniqueUsers := map[uuid.UUID]struct{}{}

		for _, mTemplateUserUsage := range appUsageByTemplateAndUser {
			mUserUsage, ok := mTemplateUserUsage[templateID]
			if !ok {
				continue // template was not used in this time window
			}

			for userID, usage := range mUserUsage {
				uniqueUsers[userID] = struct{}{}

				r.UsageJetbrainsSeconds += usage.UsageJetbrainsSeconds
				r.UsageVscodeSeconds += usage.UsageVscodeSeconds
				r.UsageReconnectingPtySeconds += usage.UsageReconnectingPtySeconds
				r.UsageSshSeconds += usage.UsageSshSeconds
			}
		}

		r.ActiveUsers = int64(len(uniqueUsers))

		result = append(result, r)
	}
	return result, nil
}

func (q *FakeQuerier) GetTemplateParameterInsights(ctx context.Context, arg database.GetTemplateParameterInsightsParams) ([]database.GetTemplateParameterInsightsRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// WITH latest_workspace_builds ...
	latestWorkspaceBuilds := make(map[uuid.UUID]database.WorkspaceBuildTable)
	for _, wb := range q.workspaceBuilds {
		if wb.CreatedAt.Before(arg.StartTime) || wb.CreatedAt.Equal(arg.EndTime) || wb.CreatedAt.After(arg.EndTime) {
			continue
		}
		if latestWorkspaceBuilds[wb.WorkspaceID].BuildNumber < wb.BuildNumber {
			latestWorkspaceBuilds[wb.WorkspaceID] = wb
		}
	}
	if len(arg.TemplateIDs) > 0 {
		for wsID := range latestWorkspaceBuilds {
			ws, err := q.getWorkspaceByIDNoLock(ctx, wsID)
			if err != nil {
				return nil, err
			}
			if slices.Contains(arg.TemplateIDs, ws.TemplateID) {
				delete(latestWorkspaceBuilds, wsID)
			}
		}
	}
	// WITH unique_template_params ...
	num := int64(0)
	uniqueTemplateParams := make(map[string]*database.GetTemplateParameterInsightsRow)
	uniqueTemplateParamWorkspaceBuildIDs := make(map[string][]uuid.UUID)
	for _, wb := range latestWorkspaceBuilds {
		tv, err := q.getTemplateVersionByIDNoLock(ctx, wb.TemplateVersionID)
		if err != nil {
			return nil, err
		}
		for _, tvp := range q.templateVersionParameters {
			if tvp.TemplateVersionID != tv.ID {
				continue
			}
			// GROUP BY tvp.name, tvp.type, tvp.display_name, tvp.description, tvp.options
			key := fmt.Sprintf("%s:%s:%s:%s:%s", tvp.Name, tvp.Type, tvp.DisplayName, tvp.Description, tvp.Options)
			if _, ok := uniqueTemplateParams[key]; !ok {
				num++
				uniqueTemplateParams[key] = &database.GetTemplateParameterInsightsRow{
					Num:         num,
					Name:        tvp.Name,
					Type:        tvp.Type,
					DisplayName: tvp.DisplayName,
					Description: tvp.Description,
					Options:     tvp.Options,
				}
			}
			uniqueTemplateParams[key].TemplateIDs = append(uniqueTemplateParams[key].TemplateIDs, tv.TemplateID.UUID)
			uniqueTemplateParamWorkspaceBuildIDs[key] = append(uniqueTemplateParamWorkspaceBuildIDs[key], wb.ID)
		}
	}
	// SELECT ...
	counts := make(map[string]map[string]int64)
	for key, utp := range uniqueTemplateParams {
		for _, wbp := range q.workspaceBuildParameters {
			if !slices.Contains(uniqueTemplateParamWorkspaceBuildIDs[key], wbp.WorkspaceBuildID) {
				continue
			}
			if wbp.Name != utp.Name {
				continue
			}
			if counts[key] == nil {
				counts[key] = make(map[string]int64)
			}
			counts[key][wbp.Value]++
		}
	}

	var rows []database.GetTemplateParameterInsightsRow
	for key, utp := range uniqueTemplateParams {
		for value, count := range counts[key] {
			rows = append(rows, database.GetTemplateParameterInsightsRow{
				Num:         utp.Num,
				TemplateIDs: uniqueSortedUUIDs(utp.TemplateIDs),
				Name:        utp.Name,
				DisplayName: utp.DisplayName,
				Type:        utp.Type,
				Description: utp.Description,
				Options:     utp.Options,
				Value:       value,
				Count:       count,
			})
		}
	}

	// NOTE(mafredri): Add sorting if we decide on how to handle PostgreSQL collations.
	// ORDER BY utp.name, utp.type, utp.display_name, utp.description, utp.options, wbp.value
	return rows, nil
}

func (q *FakeQuerier) GetTemplateVersionByID(ctx context.Context, templateVersionID uuid.UUID) (database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getTemplateVersionByIDNoLock(ctx, templateVersionID)
}

func (q *FakeQuerier) GetTemplateVersionByJobID(_ context.Context, jobID uuid.UUID) (database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, templateVersion := range q.templateVersions {
		if templateVersion.JobID != jobID {
			continue
		}
		return q.templateVersionWithUserNoLock(templateVersion), nil
	}
	return database.TemplateVersion{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetTemplateVersionByTemplateIDAndName(_ context.Context, arg database.GetTemplateVersionByTemplateIDAndNameParams) (database.TemplateVersion, error) {
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
		return q.templateVersionWithUserNoLock(templateVersion), nil
	}
	return database.TemplateVersion{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetTemplateVersionParameters(_ context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionParameter, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	parameters := make([]database.TemplateVersionParameter, 0)
	for _, param := range q.templateVersionParameters {
		if param.TemplateVersionID != templateVersionID {
			continue
		}
		parameters = append(parameters, param)
	}
	sort.Slice(parameters, func(i, j int) bool {
		if parameters[i].DisplayOrder != parameters[j].DisplayOrder {
			return parameters[i].DisplayOrder < parameters[j].DisplayOrder
		}
		return strings.ToLower(parameters[i].Name) < strings.ToLower(parameters[j].Name)
	})
	return parameters, nil
}

func (q *FakeQuerier) GetTemplateVersionVariables(_ context.Context, templateVersionID uuid.UUID) ([]database.TemplateVersionVariable, error) {
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

func (q *FakeQuerier) GetTemplateVersionsByIDs(_ context.Context, ids []uuid.UUID) ([]database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	versions := make([]database.TemplateVersion, 0)
	for _, version := range q.templateVersions {
		for _, id := range ids {
			if id == version.ID {
				versions = append(versions, q.templateVersionWithUserNoLock(version))
				break
			}
		}
	}
	if len(versions) == 0 {
		return nil, sql.ErrNoRows
	}

	return versions, nil
}

func (q *FakeQuerier) GetTemplateVersionsByTemplateID(_ context.Context, arg database.GetTemplateVersionsByTemplateIDParams) (version []database.TemplateVersion, err error) {
	if err := validateDatabaseType(arg); err != nil {
		return version, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, templateVersion := range q.templateVersions {
		if templateVersion.TemplateID.UUID != arg.TemplateID {
			continue
		}
		if arg.Archived.Valid && arg.Archived.Bool != templateVersion.Archived {
			continue
		}
		version = append(version, q.templateVersionWithUserNoLock(templateVersion))
	}

	// Database orders by created_at
	slices.SortFunc(version, func(a, b database.TemplateVersion) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			// Technically the postgres database also orders by uuid. So match
			// that behavior
			return slice.Ascending(a.ID.String(), b.ID.String())
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
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

func (q *FakeQuerier) GetTemplateVersionsCreatedAfter(_ context.Context, after time.Time) ([]database.TemplateVersion, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	versions := make([]database.TemplateVersion, 0)
	for _, version := range q.templateVersions {
		if version.CreatedAt.After(after) {
			versions = append(versions, q.templateVersionWithUserNoLock(version))
		}
	}
	return versions, nil
}

func (q *FakeQuerier) GetTemplates(_ context.Context) ([]database.Template, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	templates := slices.Clone(q.templates)
	slices.SortFunc(templates, func(a, b database.TemplateTable) int {
		if a.Name != b.Name {
			return slice.Ascending(a.Name, b.Name)
		}
		return slice.Ascending(a.ID.String(), b.ID.String())
	})

	return q.templatesWithUserNoLock(templates), nil
}

func (q *FakeQuerier) GetTemplatesWithFilter(ctx context.Context, arg database.GetTemplatesWithFilterParams) ([]database.Template, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	return q.GetAuthorizedTemplates(ctx, arg, nil)
}

func (q *FakeQuerier) GetUnexpiredLicenses(_ context.Context) ([]database.License, error) {
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

func (q *FakeQuerier) GetUserActivityInsights(ctx context.Context, arg database.GetUserActivityInsightsParams) ([]database.GetUserActivityInsightsRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	type uniqueKey struct {
		TemplateID uuid.UUID
		UserID     uuid.UUID
	}

	combinedStats := make(map[uniqueKey]map[time.Time]int64)

	// Get application stats
	for _, s := range q.workspaceAppStats {
		if !(((s.SessionStartedAt.After(arg.StartTime) || s.SessionStartedAt.Equal(arg.StartTime)) && s.SessionStartedAt.Before(arg.EndTime)) ||
			(s.SessionEndedAt.After(arg.StartTime) && s.SessionEndedAt.Before(arg.EndTime)) ||
			(s.SessionStartedAt.Before(arg.StartTime) && (s.SessionEndedAt.After(arg.EndTime) || s.SessionEndedAt.Equal(arg.EndTime)))) {
			continue
		}

		w, err := q.getWorkspaceByIDNoLock(ctx, s.WorkspaceID)
		if err != nil {
			return nil, err
		}

		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, w.TemplateID) {
			continue
		}

		key := uniqueKey{
			TemplateID: w.TemplateID,
			UserID:     s.UserID,
		}
		if combinedStats[key] == nil {
			combinedStats[key] = make(map[time.Time]int64)
		}

		t := s.SessionStartedAt.Truncate(time.Minute)
		if t.Before(arg.StartTime) {
			t = arg.StartTime
		}
		for t.Before(s.SessionEndedAt) && t.Before(arg.EndTime) {
			combinedStats[key][t] = 60
			t = t.Add(1 * time.Minute)
		}
	}

	// Get session stats
	for _, s := range q.workspaceAgentStats {
		if s.CreatedAt.Before(arg.StartTime) || s.CreatedAt.Equal(arg.EndTime) || s.CreatedAt.After(arg.EndTime) {
			continue
		}
		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, s.TemplateID) {
			continue
		}
		if s.ConnectionCount == 0 {
			continue
		}

		key := uniqueKey{
			TemplateID: s.TemplateID,
			UserID:     s.UserID,
		}

		if combinedStats[key] == nil {
			combinedStats[key] = make(map[time.Time]int64)
		}

		if s.SessionCountJetBrains > 0 || s.SessionCountVSCode > 0 || s.SessionCountReconnectingPTY > 0 || s.SessionCountSSH > 0 {
			t := s.CreatedAt.Truncate(time.Minute)
			combinedStats[key][t] = 60
		}
	}

	// Use temporary maps for aggregation purposes
	mUserIDTemplateIDs := map[uuid.UUID]map[uuid.UUID]struct{}{}
	mUserIDUsageSeconds := map[uuid.UUID]int64{}

	for key, times := range combinedStats {
		if mUserIDTemplateIDs[key.UserID] == nil {
			mUserIDTemplateIDs[key.UserID] = make(map[uuid.UUID]struct{})
			mUserIDUsageSeconds[key.UserID] = 0
		}

		if _, ok := mUserIDTemplateIDs[key.UserID][key.TemplateID]; !ok {
			mUserIDTemplateIDs[key.UserID][key.TemplateID] = struct{}{}
		}

		for _, t := range times {
			mUserIDUsageSeconds[key.UserID] += t
		}
	}

	userIDs := make([]uuid.UUID, 0, len(mUserIDUsageSeconds))
	for userID := range mUserIDUsageSeconds {
		userIDs = append(userIDs, userID)
	}
	sort.Slice(userIDs, func(i, j int) bool {
		return userIDs[i].String() < userIDs[j].String()
	})

	// Finally, select stats
	var rows []database.GetUserActivityInsightsRow

	for _, userID := range userIDs {
		user, err := q.getUserByIDNoLock(userID)
		if err != nil {
			return nil, err
		}

		tids := mUserIDTemplateIDs[userID]
		templateIDs := make([]uuid.UUID, 0, len(tids))
		for key := range tids {
			templateIDs = append(templateIDs, key)
		}
		sort.Slice(templateIDs, func(i, j int) bool {
			return templateIDs[i].String() < templateIDs[j].String()
		})

		row := database.GetUserActivityInsightsRow{
			UserID:       user.ID,
			Username:     user.Username,
			AvatarURL:    user.AvatarURL,
			TemplateIDs:  templateIDs,
			UsageSeconds: mUserIDUsageSeconds[userID],
		}

		rows = append(rows, row)
	}
	return rows, nil
}

func (q *FakeQuerier) GetUserByEmailOrUsername(_ context.Context, arg database.GetUserByEmailOrUsernameParams) (database.User, error) {
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

func (q *FakeQuerier) GetUserByID(_ context.Context, id uuid.UUID) (database.User, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getUserByIDNoLock(id)
}

func (q *FakeQuerier) GetUserCount(_ context.Context) (int64, error) {
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

func (q *FakeQuerier) GetUserLatencyInsights(_ context.Context, arg database.GetUserLatencyInsightsParams) ([]database.GetUserLatencyInsightsRow, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	latenciesByUserID := make(map[uuid.UUID][]float64)
	seenTemplatesByUserID := make(map[uuid.UUID]map[uuid.UUID]struct{})
	for _, s := range q.workspaceAgentStats {
		if len(arg.TemplateIDs) > 0 && !slices.Contains(arg.TemplateIDs, s.TemplateID) {
			continue
		}
		if !arg.StartTime.Equal(s.CreatedAt) && (s.CreatedAt.Before(arg.StartTime) || s.CreatedAt.After(arg.EndTime)) {
			continue
		}
		if s.ConnectionCount == 0 {
			continue
		}
		if s.ConnectionMedianLatencyMS <= 0 {
			continue
		}

		latenciesByUserID[s.UserID] = append(latenciesByUserID[s.UserID], s.ConnectionMedianLatencyMS)
		if seenTemplatesByUserID[s.UserID] == nil {
			seenTemplatesByUserID[s.UserID] = make(map[uuid.UUID]struct{})
		}
		seenTemplatesByUserID[s.UserID][s.TemplateID] = struct{}{}
	}

	tryPercentile := func(fs []float64, p float64) float64 {
		if len(fs) == 0 {
			return -1
		}
		sort.Float64s(fs)
		return fs[int(float64(len(fs))*p/100)]
	}

	var rows []database.GetUserLatencyInsightsRow
	for userID, latencies := range latenciesByUserID {
		sort.Float64s(latencies)
		templateIDSet := seenTemplatesByUserID[userID]
		templateIDs := make([]uuid.UUID, 0, len(templateIDSet))
		for templateID := range templateIDSet {
			templateIDs = append(templateIDs, templateID)
		}
		slices.SortFunc(templateIDs, func(a, b uuid.UUID) int {
			return slice.Ascending(a.String(), b.String())
		})
		user, err := q.getUserByIDNoLock(userID)
		if err != nil {
			return nil, err
		}
		row := database.GetUserLatencyInsightsRow{
			UserID:                       userID,
			Username:                     user.Username,
			AvatarURL:                    user.AvatarURL,
			TemplateIDs:                  templateIDs,
			WorkspaceConnectionLatency50: tryPercentile(latencies, 50),
			WorkspaceConnectionLatency95: tryPercentile(latencies, 95),
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b database.GetUserLatencyInsightsRow) int {
		return slice.Ascending(a.UserID.String(), b.UserID.String())
	})

	return rows, nil
}

func (q *FakeQuerier) GetUserLinkByLinkedID(_ context.Context, id string) (database.UserLink, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, link := range q.userLinks {
		if link.LinkedID == id {
			return link, nil
		}
	}
	return database.UserLink{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetUserLinkByUserIDLoginType(_ context.Context, params database.GetUserLinkByUserIDLoginTypeParams) (database.UserLink, error) {
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

func (q *FakeQuerier) GetUserLinksByUserID(_ context.Context, userID uuid.UUID) ([]database.UserLink, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	uls := make([]database.UserLink, 0)
	for _, ul := range q.userLinks {
		if ul.UserID == userID {
			uls = append(uls, ul)
		}
	}
	return uls, nil
}

func (q *FakeQuerier) GetUserWorkspaceBuildParameters(_ context.Context, params database.GetUserWorkspaceBuildParametersParams) ([]database.GetUserWorkspaceBuildParametersRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	userWorkspaceIDs := make(map[uuid.UUID]struct{})
	for _, ws := range q.workspaces {
		if ws.OwnerID != params.OwnerID {
			continue
		}
		if ws.TemplateID != params.TemplateID {
			continue
		}
		userWorkspaceIDs[ws.ID] = struct{}{}
	}

	userWorkspaceBuilds := make(map[uuid.UUID]struct{})
	for _, wb := range q.workspaceBuilds {
		if _, ok := userWorkspaceIDs[wb.WorkspaceID]; !ok {
			continue
		}
		userWorkspaceBuilds[wb.ID] = struct{}{}
	}

	templateVersions := make(map[uuid.UUID]struct{})
	for _, tv := range q.templateVersions {
		if tv.TemplateID.UUID != params.TemplateID {
			continue
		}
		templateVersions[tv.ID] = struct{}{}
	}

	tvps := make(map[string]struct{})
	for _, tvp := range q.templateVersionParameters {
		if _, ok := templateVersions[tvp.TemplateVersionID]; !ok {
			continue
		}

		if _, ok := tvps[tvp.Name]; !ok && !tvp.Ephemeral {
			tvps[tvp.Name] = struct{}{}
		}
	}

	userWorkspaceBuildParameters := make(map[string]database.GetUserWorkspaceBuildParametersRow)
	for _, wbp := range q.workspaceBuildParameters {
		if _, ok := userWorkspaceBuilds[wbp.WorkspaceBuildID]; !ok {
			continue
		}
		if _, ok := tvps[wbp.Name]; !ok {
			continue
		}
		userWorkspaceBuildParameters[wbp.Name] = database.GetUserWorkspaceBuildParametersRow{
			Name:  wbp.Name,
			Value: wbp.Value,
		}
	}

	return maps.Values(userWorkspaceBuildParameters), nil
}

func (q *FakeQuerier) GetUsers(_ context.Context, params database.GetUsersParams) ([]database.GetUsersRow, error) {
	if err := validateDatabaseType(params); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// Avoid side-effect of sorting.
	users := make([]database.User, len(q.users))
	copy(users, q.users)

	// Database orders by username
	slices.SortFunc(users, func(a, b database.User) int {
		return slice.Ascending(strings.ToLower(a.Username), strings.ToLower(b.Username))
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

	if !params.LastSeenBefore.IsZero() {
		usersFilteredByLastSeen := make([]database.User, 0, len(users))
		for i, user := range users {
			if user.LastSeenAt.Before(params.LastSeenBefore) {
				usersFilteredByLastSeen = append(usersFilteredByLastSeen, users[i])
			}
		}
		users = usersFilteredByLastSeen
	}

	if !params.LastSeenAfter.IsZero() {
		usersFilteredByLastSeen := make([]database.User, 0, len(users))
		for i, user := range users {
			if user.LastSeenAt.After(params.LastSeenAfter) {
				usersFilteredByLastSeen = append(usersFilteredByLastSeen, users[i])
			}
		}
		users = usersFilteredByLastSeen
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

func (q *FakeQuerier) GetUsersByIDs(_ context.Context, ids []uuid.UUID) ([]database.User, error) {
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

func (q *FakeQuerier) GetWorkspaceAgentAndOwnerByAuthToken(_ context.Context, authToken uuid.UUID) (database.GetWorkspaceAgentAndOwnerByAuthTokenRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// map of build number -> row
	rows := make(map[int32]database.GetWorkspaceAgentAndOwnerByAuthTokenRow)

	// We want to return the latest build number
	var latestBuildNumber int32

	for _, agt := range q.workspaceAgents {
		if agt.AuthToken != authToken {
			continue
		}
		// get the related workspace and user
		for _, res := range q.workspaceResources {
			if agt.ResourceID != res.ID {
				continue
			}
			for _, build := range q.workspaceBuilds {
				if build.JobID != res.JobID {
					continue
				}
				for _, ws := range q.workspaces {
					if build.WorkspaceID != ws.ID {
						continue
					}
					if ws.Deleted {
						continue
					}
					var row database.GetWorkspaceAgentAndOwnerByAuthTokenRow
					row.WorkspaceID = ws.ID
					row.TemplateID = ws.TemplateID
					usr, err := q.getUserByIDNoLock(ws.OwnerID)
					if err != nil {
						return database.GetWorkspaceAgentAndOwnerByAuthTokenRow{}, sql.ErrNoRows
					}
					row.OwnerID = usr.ID
					row.OwnerRoles = append(usr.RBACRoles, "member")
					// We also need to get org roles for the user
					row.OwnerName = usr.Username
					row.WorkspaceAgent = agt
					row.TemplateVersionID = build.TemplateVersionID
					for _, mem := range q.organizationMembers {
						if mem.UserID == usr.ID {
							row.OwnerRoles = append(row.OwnerRoles, fmt.Sprintf("organization-member:%s", mem.OrganizationID.String()))
						}
					}
					// And group memberships
					for _, groupMem := range q.groupMembers {
						if groupMem.UserID == usr.ID {
							row.OwnerGroups = append(row.OwnerGroups, groupMem.GroupID.String())
						}
					}

					// Keep track of the latest build number
					rows[build.BuildNumber] = row
					if build.BuildNumber > latestBuildNumber {
						latestBuildNumber = build.BuildNumber
					}
				}
			}
		}
	}

	if len(rows) == 0 {
		return database.GetWorkspaceAgentAndOwnerByAuthTokenRow{}, sql.ErrNoRows
	}

	// Return the row related to the latest build
	return rows[latestBuildNumber], nil
}

func (q *FakeQuerier) GetWorkspaceAgentByID(ctx context.Context, id uuid.UUID) (database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceAgentByIDNoLock(ctx, id)
}

func (q *FakeQuerier) GetWorkspaceAgentByInstanceID(_ context.Context, instanceID string) (database.WorkspaceAgent, error) {
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

func (q *FakeQuerier) GetWorkspaceAgentLifecycleStateByID(ctx context.Context, id uuid.UUID) (database.GetWorkspaceAgentLifecycleStateByIDRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	agent, err := q.getWorkspaceAgentByIDNoLock(ctx, id)
	if err != nil {
		return database.GetWorkspaceAgentLifecycleStateByIDRow{}, err
	}
	return database.GetWorkspaceAgentLifecycleStateByIDRow{
		LifecycleState: agent.LifecycleState,
		StartedAt:      agent.StartedAt,
		ReadyAt:        agent.ReadyAt,
	}, nil
}

func (q *FakeQuerier) GetWorkspaceAgentLogSourcesByAgentIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceAgentLogSource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	logSources := make([]database.WorkspaceAgentLogSource, 0)
	for _, logSource := range q.workspaceAgentLogSources {
		for _, id := range ids {
			if logSource.WorkspaceAgentID == id {
				logSources = append(logSources, logSource)
				break
			}
		}
	}
	return logSources, nil
}

func (q *FakeQuerier) GetWorkspaceAgentLogsAfter(_ context.Context, arg database.GetWorkspaceAgentLogsAfterParams) ([]database.WorkspaceAgentLog, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	logs := []database.WorkspaceAgentLog{}
	for _, log := range q.workspaceAgentLogs {
		if log.AgentID != arg.AgentID {
			continue
		}
		if arg.CreatedAfter != 0 && log.ID <= arg.CreatedAfter {
			continue
		}
		logs = append(logs, log)
	}
	return logs, nil
}

func (q *FakeQuerier) GetWorkspaceAgentMetadata(_ context.Context, arg database.GetWorkspaceAgentMetadataParams) ([]database.WorkspaceAgentMetadatum, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	metadata := make([]database.WorkspaceAgentMetadatum, 0)
	for _, m := range q.workspaceAgentMetadata {
		if m.WorkspaceAgentID == arg.WorkspaceAgentID {
			if len(arg.Keys) > 0 && !slices.Contains(arg.Keys, m.Key) {
				continue
			}
			metadata = append(metadata, m)
		}
	}
	return metadata, nil
}

func (q *FakeQuerier) GetWorkspaceAgentPortShare(_ context.Context, arg database.GetWorkspaceAgentPortShareParams) (database.WorkspaceAgentPortShare, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.WorkspaceAgentPortShare{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, share := range q.workspaceAgentPortShares {
		if share.WorkspaceID == arg.WorkspaceID && share.AgentName == arg.AgentName && share.Port == arg.Port {
			return share, nil
		}
	}

	return database.WorkspaceAgentPortShare{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceAgentScriptsByAgentIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceAgentScript, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	scripts := make([]database.WorkspaceAgentScript, 0)
	for _, script := range q.workspaceAgentScripts {
		for _, id := range ids {
			if script.WorkspaceAgentID == id {
				scripts = append(scripts, script)
				break
			}
		}
	}
	return scripts, nil
}

func (q *FakeQuerier) GetWorkspaceAgentStats(_ context.Context, createdAfter time.Time) ([]database.GetWorkspaceAgentStatsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	agentStatsCreatedAfter := make([]database.WorkspaceAgentStat, 0)
	for _, agentStat := range q.workspaceAgentStats {
		if agentStat.CreatedAt.After(createdAfter) || agentStat.CreatedAt.Equal(createdAfter) {
			agentStatsCreatedAfter = append(agentStatsCreatedAfter, agentStat)
		}
	}

	latestAgentStats := map[uuid.UUID]database.WorkspaceAgentStat{}
	for _, agentStat := range q.workspaceAgentStats {
		if agentStat.CreatedAt.After(createdAfter) || agentStat.CreatedAt.Equal(createdAfter) {
			latestAgentStats[agentStat.AgentID] = agentStat
		}
	}

	statByAgent := map[uuid.UUID]database.GetWorkspaceAgentStatsRow{}
	for agentID, agentStat := range latestAgentStats {
		stat := statByAgent[agentID]
		stat.AgentID = agentStat.AgentID
		stat.TemplateID = agentStat.TemplateID
		stat.UserID = agentStat.UserID
		stat.WorkspaceID = agentStat.WorkspaceID
		stat.SessionCountVSCode += agentStat.SessionCountVSCode
		stat.SessionCountJetBrains += agentStat.SessionCountJetBrains
		stat.SessionCountReconnectingPTY += agentStat.SessionCountReconnectingPTY
		stat.SessionCountSSH += agentStat.SessionCountSSH
		statByAgent[stat.AgentID] = stat
	}

	latenciesByAgent := map[uuid.UUID][]float64{}
	minimumDateByAgent := map[uuid.UUID]time.Time{}
	for _, agentStat := range agentStatsCreatedAfter {
		if agentStat.ConnectionMedianLatencyMS <= 0 {
			continue
		}
		stat := statByAgent[agentStat.AgentID]
		minimumDate := minimumDateByAgent[agentStat.AgentID]
		if agentStat.CreatedAt.Before(minimumDate) || minimumDate.IsZero() {
			minimumDateByAgent[agentStat.AgentID] = agentStat.CreatedAt
		}
		stat.WorkspaceRxBytes += agentStat.RxBytes
		stat.WorkspaceTxBytes += agentStat.TxBytes
		statByAgent[agentStat.AgentID] = stat
		latenciesByAgent[agentStat.AgentID] = append(latenciesByAgent[agentStat.AgentID], agentStat.ConnectionMedianLatencyMS)
	}

	tryPercentile := func(fs []float64, p float64) float64 {
		if len(fs) == 0 {
			return -1
		}
		sort.Float64s(fs)
		return fs[int(float64(len(fs))*p/100)]
	}

	for _, stat := range statByAgent {
		stat.AggregatedFrom = minimumDateByAgent[stat.AgentID]
		statByAgent[stat.AgentID] = stat

		latencies, ok := latenciesByAgent[stat.AgentID]
		if !ok {
			continue
		}
		stat.WorkspaceConnectionLatency50 = tryPercentile(latencies, 50)
		stat.WorkspaceConnectionLatency95 = tryPercentile(latencies, 95)
		statByAgent[stat.AgentID] = stat
	}

	stats := make([]database.GetWorkspaceAgentStatsRow, 0, len(statByAgent))
	for _, agent := range statByAgent {
		stats = append(stats, agent)
	}
	return stats, nil
}

func (q *FakeQuerier) GetWorkspaceAgentStatsAndLabels(ctx context.Context, createdAfter time.Time) ([]database.GetWorkspaceAgentStatsAndLabelsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	agentStatsCreatedAfter := make([]database.WorkspaceAgentStat, 0)
	latestAgentStats := map[uuid.UUID]database.WorkspaceAgentStat{}

	for _, agentStat := range q.workspaceAgentStats {
		if agentStat.CreatedAt.After(createdAfter) {
			agentStatsCreatedAfter = append(agentStatsCreatedAfter, agentStat)
			latestAgentStats[agentStat.AgentID] = agentStat
		}
	}

	statByAgent := map[uuid.UUID]database.GetWorkspaceAgentStatsAndLabelsRow{}

	// Session and connection metrics
	for _, agentStat := range latestAgentStats {
		stat := statByAgent[agentStat.AgentID]
		stat.SessionCountVSCode += agentStat.SessionCountVSCode
		stat.SessionCountJetBrains += agentStat.SessionCountJetBrains
		stat.SessionCountReconnectingPTY += agentStat.SessionCountReconnectingPTY
		stat.SessionCountSSH += agentStat.SessionCountSSH
		stat.ConnectionCount += agentStat.ConnectionCount
		if agentStat.ConnectionMedianLatencyMS >= 0 && stat.ConnectionMedianLatencyMS < agentStat.ConnectionMedianLatencyMS {
			stat.ConnectionMedianLatencyMS = agentStat.ConnectionMedianLatencyMS
		}
		statByAgent[agentStat.AgentID] = stat
	}

	// Tx, Rx metrics
	for _, agentStat := range agentStatsCreatedAfter {
		stat := statByAgent[agentStat.AgentID]
		stat.RxBytes += agentStat.RxBytes
		stat.TxBytes += agentStat.TxBytes
		statByAgent[agentStat.AgentID] = stat
	}

	// Labels
	for _, agentStat := range agentStatsCreatedAfter {
		stat := statByAgent[agentStat.AgentID]

		user, err := q.getUserByIDNoLock(agentStat.UserID)
		if err != nil {
			return nil, err
		}

		stat.Username = user.Username

		workspace, err := q.getWorkspaceByIDNoLock(ctx, agentStat.WorkspaceID)
		if err != nil {
			return nil, err
		}
		stat.WorkspaceName = workspace.Name

		agent, err := q.getWorkspaceAgentByIDNoLock(ctx, agentStat.AgentID)
		if err != nil {
			return nil, err
		}
		stat.AgentName = agent.Name

		statByAgent[agentStat.AgentID] = stat
	}

	stats := make([]database.GetWorkspaceAgentStatsAndLabelsRow, 0, len(statByAgent))
	for _, agent := range statByAgent {
		stats = append(stats, agent)
	}
	return stats, nil
}

func (q *FakeQuerier) GetWorkspaceAgentsByResourceIDs(ctx context.Context, resourceIDs []uuid.UUID) ([]database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceAgentsByResourceIDsNoLock(ctx, resourceIDs)
}

func (q *FakeQuerier) GetWorkspaceAgentsCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceAgent, error) {
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

func (q *FakeQuerier) GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx context.Context, workspaceID uuid.UUID) ([]database.WorkspaceAgent, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// Get latest build for workspace.
	workspaceBuild, err := q.getLatestWorkspaceBuildByWorkspaceIDNoLock(ctx, workspaceID)
	if err != nil {
		return nil, xerrors.Errorf("get latest workspace build: %w", err)
	}

	// Get resources for build.
	resources, err := q.getWorkspaceResourcesByJobIDNoLock(ctx, workspaceBuild.JobID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace resources: %w", err)
	}
	if len(resources) == 0 {
		return []database.WorkspaceAgent{}, nil
	}

	resourceIDs := make([]uuid.UUID, len(resources))
	for i, resource := range resources {
		resourceIDs[i] = resource.ID
	}

	agents, err := q.getWorkspaceAgentsByResourceIDsNoLock(ctx, resourceIDs)
	if err != nil {
		return nil, xerrors.Errorf("get workspace agents: %w", err)
	}

	return agents, nil
}

func (q *FakeQuerier) GetWorkspaceAppByAgentIDAndSlug(ctx context.Context, arg database.GetWorkspaceAppByAgentIDAndSlugParams) (database.WorkspaceApp, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.WorkspaceApp{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceAppByAgentIDAndSlugNoLock(ctx, arg)
}

func (q *FakeQuerier) GetWorkspaceAppsByAgentID(_ context.Context, id uuid.UUID) ([]database.WorkspaceApp, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	apps := make([]database.WorkspaceApp, 0)
	for _, app := range q.workspaceApps {
		if app.AgentID == id {
			apps = append(apps, app)
		}
	}
	return apps, nil
}

func (q *FakeQuerier) GetWorkspaceAppsByAgentIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceApp, error) {
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

func (q *FakeQuerier) GetWorkspaceAppsCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceApp, error) {
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

func (q *FakeQuerier) GetWorkspaceBuildByID(ctx context.Context, id uuid.UUID) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceBuildByIDNoLock(ctx, id)
}

func (q *FakeQuerier) GetWorkspaceBuildByJobID(_ context.Context, jobID uuid.UUID) (database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, build := range q.workspaceBuilds {
		if build.JobID == jobID {
			return q.workspaceBuildWithUserNoLock(build), nil
		}
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceBuildByWorkspaceIDAndBuildNumber(_ context.Context, arg database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams) (database.WorkspaceBuild, error) {
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
		return q.workspaceBuildWithUserNoLock(workspaceBuild), nil
	}
	return database.WorkspaceBuild{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceBuildParameters(_ context.Context, workspaceBuildID uuid.UUID) ([]database.WorkspaceBuildParameter, error) {
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

func (q *FakeQuerier) GetWorkspaceBuildsByWorkspaceID(_ context.Context,
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
			history = append(history, q.workspaceBuildWithUserNoLock(workspaceBuild))
		}
	}

	// Order by build_number
	slices.SortFunc(history, func(a, b database.WorkspaceBuild) int {
		return slice.Descending(a.BuildNumber, b.BuildNumber)
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

func (q *FakeQuerier) GetWorkspaceBuildsCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceBuild, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaceBuilds := make([]database.WorkspaceBuild, 0)
	for _, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.CreatedAt.After(after) {
			workspaceBuilds = append(workspaceBuilds, q.workspaceBuildWithUserNoLock(workspaceBuild))
		}
	}
	return workspaceBuilds, nil
}

func (q *FakeQuerier) GetWorkspaceByAgentID(ctx context.Context, agentID uuid.UUID) (database.GetWorkspaceByAgentIDRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	w, err := q.getWorkspaceByAgentIDNoLock(ctx, agentID)
	if err != nil {
		return database.GetWorkspaceByAgentIDRow{}, err
	}

	tpl, err := q.getTemplateByIDNoLock(ctx, w.TemplateID)
	if err != nil {
		return database.GetWorkspaceByAgentIDRow{}, err
	}

	return database.GetWorkspaceByAgentIDRow{
		Workspace:    w,
		TemplateName: tpl.Name,
	}, nil
}

func (q *FakeQuerier) GetWorkspaceByID(ctx context.Context, id uuid.UUID) (database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceByIDNoLock(ctx, id)
}

func (q *FakeQuerier) GetWorkspaceByOwnerIDAndName(_ context.Context, arg database.GetWorkspaceByOwnerIDAndNameParams) (database.Workspace, error) {
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

func (q *FakeQuerier) GetWorkspaceByWorkspaceAppID(_ context.Context, workspaceAppID uuid.UUID) (database.Workspace, error) {
	if err := validateDatabaseType(workspaceAppID); err != nil {
		return database.Workspace{}, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, workspaceApp := range q.workspaceApps {
		workspaceApp := workspaceApp
		if workspaceApp.ID == workspaceAppID {
			return q.getWorkspaceByAgentIDNoLock(context.Background(), workspaceApp.AgentID)
		}
	}
	return database.Workspace{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceProxies(_ context.Context) ([]database.WorkspaceProxy, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	cpy := make([]database.WorkspaceProxy, 0, len(q.workspaceProxies))

	for _, p := range q.workspaceProxies {
		if !p.Deleted {
			cpy = append(cpy, p)
		}
	}
	return cpy, nil
}

func (q *FakeQuerier) GetWorkspaceProxyByHostname(_ context.Context, params database.GetWorkspaceProxyByHostnameParams) (database.WorkspaceProxy, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	// Return zero rows if this is called with a non-sanitized hostname. The SQL
	// version of this query does the same thing.
	if !validProxyByHostnameRegex.MatchString(params.Hostname) {
		return database.WorkspaceProxy{}, sql.ErrNoRows
	}

	// This regex matches the SQL version.
	accessURLRegex := regexp.MustCompile(`[^:]*://` + regexp.QuoteMeta(params.Hostname) + `([:/]?.)*`)

	for _, proxy := range q.workspaceProxies {
		if proxy.Deleted {
			continue
		}
		if params.AllowAccessUrl && accessURLRegex.MatchString(proxy.Url) {
			return proxy, nil
		}

		// Compile the app hostname regex. This is slow sadly.
		if params.AllowWildcardHostname {
			wildcardRegexp, err := appurl.CompileHostnamePattern(proxy.WildcardHostname)
			if err != nil {
				return database.WorkspaceProxy{}, xerrors.Errorf("compile hostname pattern %q for proxy %q (%s): %w", proxy.WildcardHostname, proxy.Name, proxy.ID.String(), err)
			}
			if _, ok := appurl.ExecuteHostnamePattern(wildcardRegexp, params.Hostname); ok {
				return proxy, nil
			}
		}
	}

	return database.WorkspaceProxy{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceProxyByID(_ context.Context, id uuid.UUID) (database.WorkspaceProxy, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, proxy := range q.workspaceProxies {
		if proxy.ID == id {
			return proxy, nil
		}
	}
	return database.WorkspaceProxy{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceProxyByName(_ context.Context, name string) (database.WorkspaceProxy, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, proxy := range q.workspaceProxies {
		if proxy.Deleted {
			continue
		}
		if proxy.Name == name {
			return proxy, nil
		}
	}
	return database.WorkspaceProxy{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceResourceByID(_ context.Context, id uuid.UUID) (database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for _, resource := range q.workspaceResources {
		if resource.ID == id {
			return resource, nil
		}
	}
	return database.WorkspaceResource{}, sql.ErrNoRows
}

func (q *FakeQuerier) GetWorkspaceResourceMetadataByResourceIDs(_ context.Context, ids []uuid.UUID) ([]database.WorkspaceResourceMetadatum, error) {
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

func (q *FakeQuerier) GetWorkspaceResourceMetadataCreatedAfter(ctx context.Context, after time.Time) ([]database.WorkspaceResourceMetadatum, error) {
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

func (q *FakeQuerier) GetWorkspaceResourcesByJobID(ctx context.Context, jobID uuid.UUID) ([]database.WorkspaceResource, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.getWorkspaceResourcesByJobIDNoLock(ctx, jobID)
}

func (q *FakeQuerier) GetWorkspaceResourcesByJobIDs(_ context.Context, jobIDs []uuid.UUID) ([]database.WorkspaceResource, error) {
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

func (q *FakeQuerier) GetWorkspaceResourcesCreatedAfter(_ context.Context, after time.Time) ([]database.WorkspaceResource, error) {
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

func (q *FakeQuerier) GetWorkspaceUniqueOwnerCountByTemplateIDs(_ context.Context, templateIds []uuid.UUID) ([]database.GetWorkspaceUniqueOwnerCountByTemplateIDsRow, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaceOwners := make(map[uuid.UUID]map[uuid.UUID]struct{})
	for _, workspace := range q.workspaces {
		if workspace.Deleted {
			continue
		}
		if !slices.Contains(templateIds, workspace.TemplateID) {
			continue
		}
		_, ok := workspaceOwners[workspace.TemplateID]
		if !ok {
			workspaceOwners[workspace.TemplateID] = make(map[uuid.UUID]struct{})
		}
		workspaceOwners[workspace.TemplateID][workspace.OwnerID] = struct{}{}
	}
	resp := make([]database.GetWorkspaceUniqueOwnerCountByTemplateIDsRow, 0)
	for _, templateID := range templateIds {
		count := len(workspaceOwners[templateID])
		resp = append(resp, database.GetWorkspaceUniqueOwnerCountByTemplateIDsRow{
			TemplateID:      templateID,
			UniqueOwnersSum: int64(count),
		})
	}

	return resp, nil
}

func (q *FakeQuerier) GetWorkspaces(ctx context.Context, arg database.GetWorkspacesParams) ([]database.GetWorkspacesRow, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	// A nil auth filter means no auth filter.
	workspaceRows, err := q.GetAuthorizedWorkspaces(ctx, arg, nil)
	return workspaceRows, err
}

func (q *FakeQuerier) GetWorkspacesEligibleForTransition(ctx context.Context, now time.Time) ([]database.Workspace, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	workspaces := []database.Workspace{}
	for _, workspace := range q.workspaces {
		build, err := q.getLatestWorkspaceBuildByWorkspaceIDNoLock(ctx, workspace.ID)
		if err != nil {
			return nil, err
		}

		if build.Transition == database.WorkspaceTransitionStart &&
			!build.Deadline.IsZero() &&
			build.Deadline.Before(now) &&
			!workspace.DormantAt.Valid {
			workspaces = append(workspaces, workspace)
			continue
		}

		if build.Transition == database.WorkspaceTransitionStop &&
			workspace.AutostartSchedule.Valid &&
			!workspace.DormantAt.Valid {
			workspaces = append(workspaces, workspace)
			continue
		}

		job, err := q.getProvisionerJobByIDNoLock(ctx, build.JobID)
		if err != nil {
			return nil, xerrors.Errorf("get provisioner job by ID: %w", err)
		}
		if codersdk.ProvisionerJobStatus(job.JobStatus) == codersdk.ProvisionerJobFailed {
			workspaces = append(workspaces, workspace)
			continue
		}

		template, err := q.getTemplateByIDNoLock(ctx, workspace.TemplateID)
		if err != nil {
			return nil, xerrors.Errorf("get template by ID: %w", err)
		}
		if !workspace.DormantAt.Valid && template.TimeTilDormant > 0 {
			workspaces = append(workspaces, workspace)
			continue
		}
		if workspace.DormantAt.Valid && template.TimeTilDormantAutoDelete > 0 {
			workspaces = append(workspaces, workspace)
			continue
		}
	}

	return workspaces, nil
}

func (q *FakeQuerier) InsertAPIKey(_ context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.APIKey{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if arg.LifetimeSeconds == 0 {
		arg.LifetimeSeconds = 86400
	}

	for _, u := range q.users {
		if u.ID == arg.UserID && u.Deleted {
			return database.APIKey{}, xerrors.Errorf("refusing to create APIKey for deleted user")
		}
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
		TokenName:       arg.TokenName,
	}
	q.apiKeys = append(q.apiKeys, key)
	return key, nil
}

func (q *FakeQuerier) InsertAllUsersGroup(ctx context.Context, orgID uuid.UUID) (database.Group, error) {
	return q.InsertGroup(ctx, database.InsertGroupParams{
		ID:             orgID,
		Name:           database.EveryoneGroup,
		DisplayName:    "",
		OrganizationID: orgID,
		AvatarURL:      "",
		QuotaAllowance: 0,
	})
}

func (q *FakeQuerier) InsertAuditLog(_ context.Context, arg database.InsertAuditLogParams) (database.AuditLog, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.AuditLog{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	alog := database.AuditLog(arg)

	q.auditLogs = append(q.auditLogs, alog)
	slices.SortFunc(q.auditLogs, func(a, b database.AuditLog) int {
		if a.Time.Before(b.Time) {
			return -1
		} else if a.Time.Equal(b.Time) {
			return 0
		}
		return 1
	})

	return alog, nil
}

func (q *FakeQuerier) InsertDBCryptKey(_ context.Context, arg database.InsertDBCryptKeyParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	for _, key := range q.dbcryptKeys {
		if key.Number == arg.Number {
			return errDuplicateKey
		}
	}

	q.dbcryptKeys = append(q.dbcryptKeys, database.DBCryptKey{
		Number:          arg.Number,
		ActiveKeyDigest: sql.NullString{String: arg.ActiveKeyDigest, Valid: true},
		Test:            arg.Test,
	})
	return nil
}

func (q *FakeQuerier) InsertDERPMeshKey(_ context.Context, id string) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.derpMeshKey = id
	return nil
}

func (q *FakeQuerier) InsertDeploymentID(_ context.Context, id string) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.deploymentID = id
	return nil
}

func (q *FakeQuerier) InsertExternalAuthLink(_ context.Context, arg database.InsertExternalAuthLinkParams) (database.ExternalAuthLink, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.ExternalAuthLink{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()
	// nolint:gosimple
	gitAuthLink := database.ExternalAuthLink{
		ProviderID:             arg.ProviderID,
		UserID:                 arg.UserID,
		CreatedAt:              arg.CreatedAt,
		UpdatedAt:              arg.UpdatedAt,
		OAuthAccessToken:       arg.OAuthAccessToken,
		OAuthAccessTokenKeyID:  arg.OAuthAccessTokenKeyID,
		OAuthRefreshToken:      arg.OAuthRefreshToken,
		OAuthRefreshTokenKeyID: arg.OAuthRefreshTokenKeyID,
		OAuthExpiry:            arg.OAuthExpiry,
		OAuthExtra:             arg.OAuthExtra,
	}
	q.externalAuthLinks = append(q.externalAuthLinks, gitAuthLink)
	return gitAuthLink, nil
}

func (q *FakeQuerier) InsertFile(_ context.Context, arg database.InsertFileParams) (database.File, error) {
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

func (q *FakeQuerier) InsertGitSSHKey(_ context.Context, arg database.InsertGitSSHKeyParams) (database.GitSSHKey, error) {
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

func (q *FakeQuerier) InsertGroup(_ context.Context, arg database.InsertGroupParams) (database.Group, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Group{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

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
		DisplayName:    arg.DisplayName,
		OrganizationID: arg.OrganizationID,
		AvatarURL:      arg.AvatarURL,
		QuotaAllowance: arg.QuotaAllowance,
		Source:         database.GroupSourceUser,
	}

	q.groups = append(q.groups, group)

	return group, nil
}

func (q *FakeQuerier) InsertGroupMember(_ context.Context, arg database.InsertGroupMemberParams) error {
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

func (q *FakeQuerier) InsertLicense(
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

func (q *FakeQuerier) InsertMissingGroups(_ context.Context, arg database.InsertMissingGroupsParams) ([]database.Group, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	groupNameMap := make(map[string]struct{})
	for _, g := range arg.GroupNames {
		groupNameMap[g] = struct{}{}
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, g := range q.groups {
		if g.OrganizationID != arg.OrganizationID {
			continue
		}
		delete(groupNameMap, g.Name)
	}

	newGroups := make([]database.Group, 0, len(groupNameMap))
	for k := range groupNameMap {
		g := database.Group{
			ID:             uuid.New(),
			Name:           k,
			OrganizationID: arg.OrganizationID,
			AvatarURL:      "",
			QuotaAllowance: 0,
			DisplayName:    "",
			Source:         arg.Source,
		}
		q.groups = append(q.groups, g)
		newGroups = append(newGroups, g)
	}

	return newGroups, nil
}

func (q *FakeQuerier) InsertOAuth2ProviderApp(_ context.Context, arg database.InsertOAuth2ProviderAppParams) (database.OAuth2ProviderApp, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.OAuth2ProviderApp{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, app := range q.oauth2ProviderApps {
		if app.Name == arg.Name {
			return database.OAuth2ProviderApp{}, errDuplicateKey
		}
	}

	//nolint:gosimple // Go wants database.OAuth2ProviderApp(arg), but we cannot be sure the structs will remain identical.
	app := database.OAuth2ProviderApp{
		ID:          arg.ID,
		CreatedAt:   arg.CreatedAt,
		UpdatedAt:   arg.UpdatedAt,
		Name:        arg.Name,
		Icon:        arg.Icon,
		CallbackURL: arg.CallbackURL,
	}
	q.oauth2ProviderApps = append(q.oauth2ProviderApps, app)

	return app, nil
}

func (q *FakeQuerier) InsertOAuth2ProviderAppSecret(_ context.Context, arg database.InsertOAuth2ProviderAppSecretParams) (database.OAuth2ProviderAppSecret, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.OAuth2ProviderAppSecret{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, app := range q.oauth2ProviderApps {
		if app.ID == arg.AppID {
			secret := database.OAuth2ProviderAppSecret{
				ID:            arg.ID,
				CreatedAt:     arg.CreatedAt,
				HashedSecret:  arg.HashedSecret,
				DisplaySecret: arg.DisplaySecret,
				AppID:         arg.AppID,
			}
			q.oauth2ProviderAppSecrets = append(q.oauth2ProviderAppSecrets, secret)
			return secret, nil
		}
	}

	return database.OAuth2ProviderAppSecret{}, sql.ErrNoRows
}

func (q *FakeQuerier) InsertOrganization(_ context.Context, arg database.InsertOrganizationParams) (database.Organization, error) {
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

func (q *FakeQuerier) InsertOrganizationMember(_ context.Context, arg database.InsertOrganizationMemberParams) (database.OrganizationMember, error) {
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

func (q *FakeQuerier) InsertProvisionerJob(_ context.Context, arg database.InsertProvisionerJobParams) (database.ProvisionerJob, error) {
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
		Tags:           maps.Clone(arg.Tags),
		TraceMetadata:  arg.TraceMetadata,
	}
	job.JobStatus = provisonerJobStatus(job)
	q.provisionerJobs = append(q.provisionerJobs, job)
	return job, nil
}

func (q *FakeQuerier) InsertProvisionerJobLogs(_ context.Context, arg database.InsertProvisionerJobLogsParams) ([]database.ProvisionerJobLog, error) {
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

func (q *FakeQuerier) InsertReplica(_ context.Context, arg database.InsertReplicaParams) (database.Replica, error) {
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
		Primary:         arg.Primary,
	}
	q.replicas = append(q.replicas, replica)
	return replica, nil
}

func (q *FakeQuerier) InsertTemplate(_ context.Context, arg database.InsertTemplateParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	template := database.TemplateTable{
		ID:                           arg.ID,
		CreatedAt:                    arg.CreatedAt,
		UpdatedAt:                    arg.UpdatedAt,
		OrganizationID:               arg.OrganizationID,
		Name:                         arg.Name,
		Provisioner:                  arg.Provisioner,
		ActiveVersionID:              arg.ActiveVersionID,
		Description:                  arg.Description,
		CreatedBy:                    arg.CreatedBy,
		UserACL:                      arg.UserACL,
		GroupACL:                     arg.GroupACL,
		DisplayName:                  arg.DisplayName,
		Icon:                         arg.Icon,
		AllowUserCancelWorkspaceJobs: arg.AllowUserCancelWorkspaceJobs,
		AllowUserAutostart:           true,
		AllowUserAutostop:            true,
		MaxPortSharingLevel:          arg.MaxPortSharingLevel,
	}
	q.templates = append(q.templates, template)
	return nil
}

func (q *FakeQuerier) InsertTemplateVersion(_ context.Context, arg database.InsertTemplateVersionParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	if len(arg.Message) > 1048576 {
		return xerrors.New("message too long")
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	version := database.TemplateVersionTable{
		ID:             arg.ID,
		TemplateID:     arg.TemplateID,
		OrganizationID: arg.OrganizationID,
		CreatedAt:      arg.CreatedAt,
		UpdatedAt:      arg.UpdatedAt,
		Name:           arg.Name,
		Message:        arg.Message,
		Readme:         arg.Readme,
		JobID:          arg.JobID,
		CreatedBy:      arg.CreatedBy,
	}
	q.templateVersions = append(q.templateVersions, version)
	return nil
}

func (q *FakeQuerier) InsertTemplateVersionParameter(_ context.Context, arg database.InsertTemplateVersionParameterParams) (database.TemplateVersionParameter, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.TemplateVersionParameter{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	param := database.TemplateVersionParameter{
		TemplateVersionID:   arg.TemplateVersionID,
		Name:                arg.Name,
		DisplayName:         arg.DisplayName,
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
		Required:            arg.Required,
		DisplayOrder:        arg.DisplayOrder,
		Ephemeral:           arg.Ephemeral,
	}
	q.templateVersionParameters = append(q.templateVersionParameters, param)
	return param, nil
}

func (q *FakeQuerier) InsertTemplateVersionVariable(_ context.Context, arg database.InsertTemplateVersionVariableParams) (database.TemplateVersionVariable, error) {
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

func (q *FakeQuerier) InsertUser(_ context.Context, arg database.InsertUserParams) (database.User, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.User{}, err
	}

	// There is a common bug when using dbmem that 2 inserted users have the
	// same created_at time. This causes user order to not be deterministic,
	// which breaks some unit tests.
	// To fix this, we make sure that the created_at time is always greater
	// than the last user's created_at time.
	allUsers, _ := q.GetUsers(context.Background(), database.GetUsersParams{})
	if len(allUsers) > 0 {
		lastUser := allUsers[len(allUsers)-1]
		if arg.CreatedAt.Before(lastUser.CreatedAt) ||
			arg.CreatedAt.Equal(lastUser.CreatedAt) {
			// 1 ms is a good enough buffer.
			arg.CreatedAt = lastUser.CreatedAt.Add(time.Millisecond)
		}
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
		Status:         database.UserStatusDormant,
		RBACRoles:      arg.RBACRoles,
		LoginType:      arg.LoginType,
	}
	q.users = append(q.users, user)
	return user, nil
}

func (q *FakeQuerier) InsertUserGroupsByName(_ context.Context, arg database.InsertUserGroupsByNameParams) error {
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

func (q *FakeQuerier) InsertUserLink(_ context.Context, args database.InsertUserLinkParams) (database.UserLink, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	link := database.UserLink{
		UserID:                 args.UserID,
		LoginType:              args.LoginType,
		LinkedID:               args.LinkedID,
		OAuthAccessToken:       args.OAuthAccessToken,
		OAuthAccessTokenKeyID:  args.OAuthAccessTokenKeyID,
		OAuthRefreshToken:      args.OAuthRefreshToken,
		OAuthRefreshTokenKeyID: args.OAuthRefreshTokenKeyID,
		OAuthExpiry:            args.OAuthExpiry,
		DebugContext:           args.DebugContext,
	}

	q.userLinks = append(q.userLinks, link)

	return link, nil
}

func (q *FakeQuerier) InsertWorkspace(_ context.Context, arg database.InsertWorkspaceParams) (database.Workspace, error) {
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
		LastUsedAt:        arg.LastUsedAt,
		AutomaticUpdates:  arg.AutomaticUpdates,
	}
	q.workspaces = append(q.workspaces, workspace)
	return workspace, nil
}

func (q *FakeQuerier) InsertWorkspaceAgent(_ context.Context, arg database.InsertWorkspaceAgentParams) (database.WorkspaceAgent, error) {
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
		InstanceMetadata:         arg.InstanceMetadata,
		ResourceMetadata:         arg.ResourceMetadata,
		ConnectionTimeoutSeconds: arg.ConnectionTimeoutSeconds,
		TroubleshootingURL:       arg.TroubleshootingURL,
		MOTDFile:                 arg.MOTDFile,
		LifecycleState:           database.WorkspaceAgentLifecycleStateCreated,
		DisplayApps:              arg.DisplayApps,
	}

	q.workspaceAgents = append(q.workspaceAgents, agent)
	return agent, nil
}

func (q *FakeQuerier) InsertWorkspaceAgentLogSources(_ context.Context, arg database.InsertWorkspaceAgentLogSourcesParams) ([]database.WorkspaceAgentLogSource, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	logSources := make([]database.WorkspaceAgentLogSource, 0)
	for index, source := range arg.ID {
		logSource := database.WorkspaceAgentLogSource{
			ID:               source,
			WorkspaceAgentID: arg.WorkspaceAgentID,
			CreatedAt:        arg.CreatedAt,
			DisplayName:      arg.DisplayName[index],
			Icon:             arg.Icon[index],
		}
		logSources = append(logSources, logSource)
	}
	q.workspaceAgentLogSources = append(q.workspaceAgentLogSources, logSources...)
	return logSources, nil
}

func (q *FakeQuerier) InsertWorkspaceAgentLogs(_ context.Context, arg database.InsertWorkspaceAgentLogsParams) ([]database.WorkspaceAgentLog, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	logs := []database.WorkspaceAgentLog{}
	id := int64(0)
	if len(q.workspaceAgentLogs) > 0 {
		id = q.workspaceAgentLogs[len(q.workspaceAgentLogs)-1].ID
	}
	outputLength := int32(0)
	for index, output := range arg.Output {
		id++
		logs = append(logs, database.WorkspaceAgentLog{
			ID:          id,
			AgentID:     arg.AgentID,
			CreatedAt:   arg.CreatedAt,
			Level:       arg.Level[index],
			LogSourceID: arg.LogSourceID,
			Output:      output,
		})
		outputLength += int32(len(output))
	}
	for index, agent := range q.workspaceAgents {
		if agent.ID != arg.AgentID {
			continue
		}
		// Greater than 1MB, same as the PostgreSQL constraint!
		if agent.LogsLength+outputLength > (1 << 20) {
			return nil, &pq.Error{
				Constraint: "max_logs_length",
				Table:      "workspace_agents",
			}
		}
		agent.LogsLength += outputLength
		q.workspaceAgents[index] = agent
		break
	}
	q.workspaceAgentLogs = append(q.workspaceAgentLogs, logs...)
	return logs, nil
}

func (q *FakeQuerier) InsertWorkspaceAgentMetadata(_ context.Context, arg database.InsertWorkspaceAgentMetadataParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	//nolint:gosimple
	metadatum := database.WorkspaceAgentMetadatum{
		WorkspaceAgentID: arg.WorkspaceAgentID,
		Script:           arg.Script,
		DisplayName:      arg.DisplayName,
		Key:              arg.Key,
		Timeout:          arg.Timeout,
		Interval:         arg.Interval,
		DisplayOrder:     arg.DisplayOrder,
	}

	q.workspaceAgentMetadata = append(q.workspaceAgentMetadata, metadatum)
	return nil
}

func (q *FakeQuerier) InsertWorkspaceAgentScripts(_ context.Context, arg database.InsertWorkspaceAgentScriptsParams) ([]database.WorkspaceAgentScript, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return nil, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	scripts := make([]database.WorkspaceAgentScript, 0)
	for index, source := range arg.LogSourceID {
		script := database.WorkspaceAgentScript{
			LogSourceID:      source,
			WorkspaceAgentID: arg.WorkspaceAgentID,
			LogPath:          arg.LogPath[index],
			Script:           arg.Script[index],
			Cron:             arg.Cron[index],
			StartBlocksLogin: arg.StartBlocksLogin[index],
			RunOnStart:       arg.RunOnStart[index],
			RunOnStop:        arg.RunOnStop[index],
			TimeoutSeconds:   arg.TimeoutSeconds[index],
			CreatedAt:        arg.CreatedAt,
		}
		scripts = append(scripts, script)
	}
	q.workspaceAgentScripts = append(q.workspaceAgentScripts, scripts...)
	return scripts, nil
}

func (q *FakeQuerier) InsertWorkspaceAgentStat(_ context.Context, p database.InsertWorkspaceAgentStatParams) (database.WorkspaceAgentStat, error) {
	if err := validateDatabaseType(p); err != nil {
		return database.WorkspaceAgentStat{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	stat := database.WorkspaceAgentStat{
		ID:                          p.ID,
		CreatedAt:                   p.CreatedAt,
		WorkspaceID:                 p.WorkspaceID,
		AgentID:                     p.AgentID,
		UserID:                      p.UserID,
		ConnectionsByProto:          p.ConnectionsByProto,
		ConnectionCount:             p.ConnectionCount,
		RxPackets:                   p.RxPackets,
		RxBytes:                     p.RxBytes,
		TxPackets:                   p.TxPackets,
		TxBytes:                     p.TxBytes,
		TemplateID:                  p.TemplateID,
		SessionCountVSCode:          p.SessionCountVSCode,
		SessionCountJetBrains:       p.SessionCountJetBrains,
		SessionCountReconnectingPTY: p.SessionCountReconnectingPTY,
		SessionCountSSH:             p.SessionCountSSH,
		ConnectionMedianLatencyMS:   p.ConnectionMedianLatencyMS,
	}
	q.workspaceAgentStats = append(q.workspaceAgentStats, stat)
	return stat, nil
}

func (q *FakeQuerier) InsertWorkspaceAgentStats(_ context.Context, arg database.InsertWorkspaceAgentStatsParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	var connectionsByProto []map[string]int64
	if err := json.Unmarshal(arg.ConnectionsByProto, &connectionsByProto); err != nil {
		return err
	}
	for i := 0; i < len(arg.ID); i++ {
		cbp, err := json.Marshal(connectionsByProto[i])
		if err != nil {
			return xerrors.Errorf("failed to marshal connections_by_proto: %w", err)
		}
		stat := database.WorkspaceAgentStat{
			ID:                          arg.ID[i],
			CreatedAt:                   arg.CreatedAt[i],
			WorkspaceID:                 arg.WorkspaceID[i],
			AgentID:                     arg.AgentID[i],
			UserID:                      arg.UserID[i],
			ConnectionsByProto:          cbp,
			ConnectionCount:             arg.ConnectionCount[i],
			RxPackets:                   arg.RxPackets[i],
			RxBytes:                     arg.RxBytes[i],
			TxPackets:                   arg.TxPackets[i],
			TxBytes:                     arg.TxBytes[i],
			TemplateID:                  arg.TemplateID[i],
			SessionCountVSCode:          arg.SessionCountVSCode[i],
			SessionCountJetBrains:       arg.SessionCountJetBrains[i],
			SessionCountReconnectingPTY: arg.SessionCountReconnectingPTY[i],
			SessionCountSSH:             arg.SessionCountSSH[i],
			ConnectionMedianLatencyMS:   arg.ConnectionMedianLatencyMS[i],
		}
		q.workspaceAgentStats = append(q.workspaceAgentStats, stat)
	}

	return nil
}

func (q *FakeQuerier) InsertWorkspaceApp(_ context.Context, arg database.InsertWorkspaceAppParams) (database.WorkspaceApp, error) {
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
		DisplayOrder:         arg.DisplayOrder,
	}
	q.workspaceApps = append(q.workspaceApps, workspaceApp)
	return workspaceApp, nil
}

func (q *FakeQuerier) InsertWorkspaceAppStats(_ context.Context, arg database.InsertWorkspaceAppStatsParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

InsertWorkspaceAppStatsLoop:
	for i := 0; i < len(arg.UserID); i++ {
		stat := database.WorkspaceAppStat{
			ID:               q.workspaceAppStatsLastInsertID + 1,
			UserID:           arg.UserID[i],
			WorkspaceID:      arg.WorkspaceID[i],
			AgentID:          arg.AgentID[i],
			AccessMethod:     arg.AccessMethod[i],
			SlugOrPort:       arg.SlugOrPort[i],
			SessionID:        arg.SessionID[i],
			SessionStartedAt: arg.SessionStartedAt[i],
			SessionEndedAt:   arg.SessionEndedAt[i],
			Requests:         arg.Requests[i],
		}
		for j, s := range q.workspaceAppStats {
			// Check unique constraint for upsert.
			if s.UserID == stat.UserID && s.AgentID == stat.AgentID && s.SessionID == stat.SessionID {
				q.workspaceAppStats[j].SessionEndedAt = stat.SessionEndedAt
				q.workspaceAppStats[j].Requests = stat.Requests
				continue InsertWorkspaceAppStatsLoop
			}
		}
		q.workspaceAppStats = append(q.workspaceAppStats, stat)
		q.workspaceAppStatsLastInsertID++
	}

	return nil
}

func (q *FakeQuerier) InsertWorkspaceBuild(_ context.Context, arg database.InsertWorkspaceBuildParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	workspaceBuild := database.WorkspaceBuildTable{
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
		MaxDeadline:       arg.MaxDeadline,
		Reason:            arg.Reason,
	}
	q.workspaceBuilds = append(q.workspaceBuilds, workspaceBuild)
	return nil
}

func (q *FakeQuerier) InsertWorkspaceBuildParameters(_ context.Context, arg database.InsertWorkspaceBuildParametersParams) error {
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

func (q *FakeQuerier) InsertWorkspaceProxy(_ context.Context, arg database.InsertWorkspaceProxyParams) (database.WorkspaceProxy, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	lastRegionID := int32(0)
	for _, p := range q.workspaceProxies {
		if !p.Deleted && p.Name == arg.Name {
			return database.WorkspaceProxy{}, errDuplicateKey
		}
		if p.RegionID > lastRegionID {
			lastRegionID = p.RegionID
		}
	}

	p := database.WorkspaceProxy{
		ID:                arg.ID,
		Name:              arg.Name,
		DisplayName:       arg.DisplayName,
		Icon:              arg.Icon,
		DerpEnabled:       arg.DerpEnabled,
		DerpOnly:          arg.DerpOnly,
		TokenHashedSecret: arg.TokenHashedSecret,
		RegionID:          lastRegionID + 1,
		CreatedAt:         arg.CreatedAt,
		UpdatedAt:         arg.UpdatedAt,
		Deleted:           false,
	}
	q.workspaceProxies = append(q.workspaceProxies, p)
	return p, nil
}

func (q *FakeQuerier) InsertWorkspaceResource(_ context.Context, arg database.InsertWorkspaceResourceParams) (database.WorkspaceResource, error) {
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

func (q *FakeQuerier) InsertWorkspaceResourceMetadata(_ context.Context, arg database.InsertWorkspaceResourceMetadataParams) ([]database.WorkspaceResourceMetadatum, error) {
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

func (q *FakeQuerier) ListWorkspaceAgentPortShares(_ context.Context, workspaceID uuid.UUID) ([]database.WorkspaceAgentPortShare, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	shares := []database.WorkspaceAgentPortShare{}
	for _, share := range q.workspaceAgentPortShares {
		if share.WorkspaceID == workspaceID {
			shares = append(shares, share)
		}
	}

	return shares, nil
}

func (q *FakeQuerier) RegisterWorkspaceProxy(_ context.Context, arg database.RegisterWorkspaceProxyParams) (database.WorkspaceProxy, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, p := range q.workspaceProxies {
		if p.ID == arg.ID {
			p.Url = arg.Url
			p.WildcardHostname = arg.WildcardHostname
			p.DerpEnabled = arg.DerpEnabled
			p.DerpOnly = arg.DerpOnly
			p.Version = arg.Version
			p.UpdatedAt = dbtime.Now()
			q.workspaceProxies[i] = p
			return p, nil
		}
	}
	return database.WorkspaceProxy{}, sql.ErrNoRows
}

func (q *FakeQuerier) RevokeDBCryptKey(_ context.Context, activeKeyDigest string) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i := range q.dbcryptKeys {
		key := q.dbcryptKeys[i]

		// Is the key already revoked?
		if !key.ActiveKeyDigest.Valid {
			continue
		}

		if key.ActiveKeyDigest.String != activeKeyDigest {
			continue
		}

		// Check for foreign key constraints.
		for _, ul := range q.userLinks {
			if (ul.OAuthAccessTokenKeyID.Valid && ul.OAuthAccessTokenKeyID.String == activeKeyDigest) ||
				(ul.OAuthRefreshTokenKeyID.Valid && ul.OAuthRefreshTokenKeyID.String == activeKeyDigest) {
				return errForeignKeyConstraint
			}
		}
		for _, gal := range q.externalAuthLinks {
			if (gal.OAuthAccessTokenKeyID.Valid && gal.OAuthAccessTokenKeyID.String == activeKeyDigest) ||
				(gal.OAuthRefreshTokenKeyID.Valid && gal.OAuthRefreshTokenKeyID.String == activeKeyDigest) {
				return errForeignKeyConstraint
			}
		}

		// Revoke the key.
		q.dbcryptKeys[i].RevokedAt = sql.NullTime{Time: dbtime.Now(), Valid: true}
		q.dbcryptKeys[i].RevokedKeyDigest = sql.NullString{String: key.ActiveKeyDigest.String, Valid: true}
		q.dbcryptKeys[i].ActiveKeyDigest = sql.NullString{}
		return nil
	}

	return sql.ErrNoRows
}

func (*FakeQuerier) TryAcquireLock(_ context.Context, _ int64) (bool, error) {
	return false, xerrors.New("TryAcquireLock must only be called within a transaction")
}

func (q *FakeQuerier) UnarchiveTemplateVersion(_ context.Context, arg database.UnarchiveTemplateVersionParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, v := range q.data.templateVersions {
		if v.ID == arg.TemplateVersionID {
			v.Archived = false
			v.UpdatedAt = arg.UpdatedAt
			q.data.templateVersions[i] = v
			return nil
		}
	}

	return sql.ErrNoRows
}

func (q *FakeQuerier) UnfavoriteWorkspace(_ context.Context, arg uuid.UUID) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i := 0; i < len(q.workspaces); i++ {
		if q.workspaces[i].ID != arg {
			continue
		}
		q.workspaces[i].Favorite = false
		return nil
	}

	return nil
}

func (q *FakeQuerier) UpdateAPIKeyByID(_ context.Context, arg database.UpdateAPIKeyByIDParams) error {
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

func (q *FakeQuerier) UpdateExternalAuthLink(_ context.Context, arg database.UpdateExternalAuthLinkParams) (database.ExternalAuthLink, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.ExternalAuthLink{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()
	for index, gitAuthLink := range q.externalAuthLinks {
		if gitAuthLink.ProviderID != arg.ProviderID {
			continue
		}
		if gitAuthLink.UserID != arg.UserID {
			continue
		}
		gitAuthLink.UpdatedAt = arg.UpdatedAt
		gitAuthLink.OAuthAccessToken = arg.OAuthAccessToken
		gitAuthLink.OAuthAccessTokenKeyID = arg.OAuthAccessTokenKeyID
		gitAuthLink.OAuthRefreshToken = arg.OAuthRefreshToken
		gitAuthLink.OAuthRefreshTokenKeyID = arg.OAuthRefreshTokenKeyID
		gitAuthLink.OAuthExpiry = arg.OAuthExpiry
		gitAuthLink.OAuthExtra = arg.OAuthExtra
		q.externalAuthLinks[index] = gitAuthLink

		return gitAuthLink, nil
	}
	return database.ExternalAuthLink{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateGitSSHKey(_ context.Context, arg database.UpdateGitSSHKeyParams) (database.GitSSHKey, error) {
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

func (q *FakeQuerier) UpdateGroupByID(_ context.Context, arg database.UpdateGroupByIDParams) (database.Group, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Group{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, group := range q.groups {
		if group.ID == arg.ID {
			group.DisplayName = arg.DisplayName
			group.Name = arg.Name
			group.AvatarURL = arg.AvatarURL
			group.QuotaAllowance = arg.QuotaAllowance
			q.groups[i] = group
			return group, nil
		}
	}
	return database.Group{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateInactiveUsersToDormant(_ context.Context, params database.UpdateInactiveUsersToDormantParams) ([]database.UpdateInactiveUsersToDormantRow, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	var updated []database.UpdateInactiveUsersToDormantRow
	for index, user := range q.users {
		if user.Status == database.UserStatusActive && user.LastSeenAt.Before(params.LastSeenAfter) {
			q.users[index].Status = database.UserStatusDormant
			q.users[index].UpdatedAt = params.UpdatedAt
			updated = append(updated, database.UpdateInactiveUsersToDormantRow{
				ID:         user.ID,
				Email:      user.Email,
				LastSeenAt: user.LastSeenAt,
			})
		}
	}

	if len(updated) == 0 {
		return nil, sql.ErrNoRows
	}
	return updated, nil
}

func (q *FakeQuerier) UpdateMemberRoles(_ context.Context, arg database.UpdateMemberRolesParams) (database.OrganizationMember, error) {
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

func (q *FakeQuerier) UpdateOAuth2ProviderAppByID(_ context.Context, arg database.UpdateOAuth2ProviderAppByIDParams) (database.OAuth2ProviderApp, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.OAuth2ProviderApp{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, app := range q.oauth2ProviderApps {
		if app.Name == arg.Name && app.ID != arg.ID {
			return database.OAuth2ProviderApp{}, errDuplicateKey
		}
	}

	for index, app := range q.oauth2ProviderApps {
		if app.ID == arg.ID {
			newApp := database.OAuth2ProviderApp{
				ID:          arg.ID,
				CreatedAt:   app.CreatedAt,
				UpdatedAt:   arg.UpdatedAt,
				Name:        arg.Name,
				Icon:        arg.Icon,
				CallbackURL: arg.CallbackURL,
			}
			q.oauth2ProviderApps[index] = newApp
			return newApp, nil
		}
	}
	return database.OAuth2ProviderApp{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateOAuth2ProviderAppSecretByID(_ context.Context, arg database.UpdateOAuth2ProviderAppSecretByIDParams) (database.OAuth2ProviderAppSecret, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.OAuth2ProviderAppSecret{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, secret := range q.oauth2ProviderAppSecrets {
		if secret.ID == arg.ID {
			newSecret := database.OAuth2ProviderAppSecret{
				ID:            arg.ID,
				CreatedAt:     secret.CreatedAt,
				HashedSecret:  secret.HashedSecret,
				DisplaySecret: secret.DisplaySecret,
				AppID:         secret.AppID,
				LastUsedAt:    arg.LastUsedAt,
			}
			q.oauth2ProviderAppSecrets[index] = newSecret
			return newSecret, nil
		}
	}
	return database.OAuth2ProviderAppSecret{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateProvisionerDaemonLastSeenAt(_ context.Context, arg database.UpdateProvisionerDaemonLastSeenAtParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for idx := range q.provisionerDaemons {
		if q.provisionerDaemons[idx].ID != arg.ID {
			continue
		}
		if q.provisionerDaemons[idx].LastSeenAt.Time.After(arg.LastSeenAt.Time) {
			continue
		}
		q.provisionerDaemons[idx].LastSeenAt = arg.LastSeenAt
		return nil
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateProvisionerJobByID(_ context.Context, arg database.UpdateProvisionerJobByIDParams) error {
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
		job.JobStatus = provisonerJobStatus(job)
		q.provisionerJobs[index] = job
		return nil
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateProvisionerJobWithCancelByID(_ context.Context, arg database.UpdateProvisionerJobWithCancelByIDParams) error {
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
		job.JobStatus = provisonerJobStatus(job)
		q.provisionerJobs[index] = job
		return nil
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateProvisionerJobWithCompleteByID(_ context.Context, arg database.UpdateProvisionerJobWithCompleteByIDParams) error {
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
		job.ErrorCode = arg.ErrorCode
		job.JobStatus = provisonerJobStatus(job)
		q.provisionerJobs[index] = job
		return nil
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateReplica(_ context.Context, arg database.UpdateReplicaParams) (database.Replica, error) {
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
		replica.Primary = arg.Primary
		q.replicas[index] = replica
		return replica, nil
	}
	return database.Replica{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateTemplateACLByID(_ context.Context, arg database.UpdateTemplateACLByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, template := range q.templates {
		if template.ID == arg.ID {
			template.GroupACL = arg.GroupACL
			template.UserACL = arg.UserACL

			q.templates[i] = template
			return nil
		}
	}

	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateTemplateAccessControlByID(_ context.Context, arg database.UpdateTemplateAccessControlByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for idx, tpl := range q.templates {
		if tpl.ID != arg.ID {
			continue
		}
		q.templates[idx].RequireActiveVersion = arg.RequireActiveVersion
		q.templates[idx].Deprecated = arg.Deprecated
		return nil
	}

	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateTemplateActiveVersionByID(_ context.Context, arg database.UpdateTemplateActiveVersionByIDParams) error {
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

func (q *FakeQuerier) UpdateTemplateDeletedByID(_ context.Context, arg database.UpdateTemplateDeletedByIDParams) error {
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

func (q *FakeQuerier) UpdateTemplateMetaByID(_ context.Context, arg database.UpdateTemplateMetaByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for idx, tpl := range q.templates {
		if tpl.ID != arg.ID {
			continue
		}
		tpl.UpdatedAt = dbtime.Now()
		tpl.Name = arg.Name
		tpl.DisplayName = arg.DisplayName
		tpl.Description = arg.Description
		tpl.Icon = arg.Icon
		tpl.GroupACL = arg.GroupACL
		tpl.AllowUserCancelWorkspaceJobs = arg.AllowUserCancelWorkspaceJobs
		tpl.MaxPortSharingLevel = arg.MaxPortSharingLevel
		q.templates[idx] = tpl
		return nil
	}

	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateTemplateScheduleByID(_ context.Context, arg database.UpdateTemplateScheduleByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for idx, tpl := range q.templates {
		if tpl.ID != arg.ID {
			continue
		}
		tpl.AllowUserAutostart = arg.AllowUserAutostart
		tpl.AllowUserAutostop = arg.AllowUserAutostop
		tpl.UpdatedAt = dbtime.Now()
		tpl.DefaultTTL = arg.DefaultTTL
		tpl.ActivityBump = arg.ActivityBump
		tpl.UseMaxTtl = arg.UseMaxTtl
		tpl.MaxTTL = arg.MaxTTL
		tpl.AutostopRequirementDaysOfWeek = arg.AutostopRequirementDaysOfWeek
		tpl.AutostopRequirementWeeks = arg.AutostopRequirementWeeks
		tpl.AutostartBlockDaysOfWeek = arg.AutostartBlockDaysOfWeek
		tpl.FailureTTL = arg.FailureTTL
		tpl.TimeTilDormant = arg.TimeTilDormant
		tpl.TimeTilDormantAutoDelete = arg.TimeTilDormantAutoDelete
		q.templates[idx] = tpl
		return nil
	}

	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateTemplateVersionByID(_ context.Context, arg database.UpdateTemplateVersionByIDParams) error {
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
		templateVersion.Name = arg.Name
		templateVersion.Message = arg.Message
		q.templateVersions[index] = templateVersion
		return nil
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateTemplateVersionDescriptionByJobID(_ context.Context, arg database.UpdateTemplateVersionDescriptionByJobIDParams) error {
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

func (q *FakeQuerier) UpdateTemplateVersionExternalAuthProvidersByJobID(_ context.Context, arg database.UpdateTemplateVersionExternalAuthProvidersByJobIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, templateVersion := range q.templateVersions {
		if templateVersion.JobID != arg.JobID {
			continue
		}
		templateVersion.ExternalAuthProviders = arg.ExternalAuthProviders
		templateVersion.UpdatedAt = arg.UpdatedAt
		q.templateVersions[index] = templateVersion
		return nil
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateTemplateWorkspacesLastUsedAt(_ context.Context, arg database.UpdateTemplateWorkspacesLastUsedAtParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, ws := range q.workspaces {
		if ws.TemplateID != arg.TemplateID {
			continue
		}
		ws.LastUsedAt = arg.LastUsedAt
		q.workspaces[i] = ws
	}

	return nil
}

func (q *FakeQuerier) UpdateUserAppearanceSettings(_ context.Context, arg database.UpdateUserAppearanceSettingsParams) (database.User, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.User{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, user := range q.users {
		if user.ID != arg.ID {
			continue
		}
		user.ThemePreference = arg.ThemePreference
		q.users[index] = user
		return user, nil
	}
	return database.User{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateUserDeletedByID(_ context.Context, params database.UpdateUserDeletedByIDParams) error {
	if err := validateDatabaseType(params); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, u := range q.users {
		if u.ID == params.ID {
			u.Deleted = params.Deleted
			q.users[i] = u
			// NOTE: In the real world, this is done by a trigger.
			i := 0
			for {
				if i >= len(q.apiKeys) {
					break
				}
				k := q.apiKeys[i]
				if k.UserID == u.ID {
					q.apiKeys[i] = q.apiKeys[len(q.apiKeys)-1]
					q.apiKeys = q.apiKeys[:len(q.apiKeys)-1]
					// We removed an element, so decrement
					i--
				}
				i++
			}
			return nil
		}
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateUserHashedPassword(_ context.Context, arg database.UpdateUserHashedPasswordParams) error {
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

func (q *FakeQuerier) UpdateUserLastSeenAt(_ context.Context, arg database.UpdateUserLastSeenAtParams) (database.User, error) {
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

func (q *FakeQuerier) UpdateUserLink(_ context.Context, params database.UpdateUserLinkParams) (database.UserLink, error) {
	if err := validateDatabaseType(params); err != nil {
		return database.UserLink{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, link := range q.userLinks {
		if link.UserID == params.UserID && link.LoginType == params.LoginType {
			link.OAuthAccessToken = params.OAuthAccessToken
			link.OAuthAccessTokenKeyID = params.OAuthAccessTokenKeyID
			link.OAuthRefreshToken = params.OAuthRefreshToken
			link.OAuthRefreshTokenKeyID = params.OAuthRefreshTokenKeyID
			link.OAuthExpiry = params.OAuthExpiry
			link.DebugContext = params.DebugContext

			q.userLinks[i] = link
			return link, nil
		}
	}

	return database.UserLink{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateUserLinkedID(_ context.Context, params database.UpdateUserLinkedIDParams) (database.UserLink, error) {
	if err := validateDatabaseType(params); err != nil {
		return database.UserLink{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, link := range q.userLinks {
		if link.UserID == params.UserID && link.LoginType == params.LoginType {
			link.LinkedID = params.LinkedID

			q.userLinks[i] = link
			return link, nil
		}
	}

	return database.UserLink{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateUserLoginType(_ context.Context, arg database.UpdateUserLoginTypeParams) (database.User, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.User{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, u := range q.users {
		if u.ID == arg.UserID {
			u.LoginType = arg.NewLoginType
			if arg.NewLoginType != database.LoginTypePassword {
				u.HashedPassword = []byte{}
			}
			q.users[i] = u
			return u, nil
		}
	}
	return database.User{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateUserProfile(_ context.Context, arg database.UpdateUserProfileParams) (database.User, error) {
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
		user.Name = arg.Name
		q.users[index] = user
		return user, nil
	}
	return database.User{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateUserQuietHoursSchedule(_ context.Context, arg database.UpdateUserQuietHoursScheduleParams) (database.User, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.User{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, user := range q.users {
		if user.ID != arg.ID {
			continue
		}
		user.QuietHoursSchedule = arg.QuietHoursSchedule
		q.users[index] = user
		return user, nil
	}
	return database.User{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateUserRoles(_ context.Context, arg database.UpdateUserRolesParams) (database.User, error) {
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

func (q *FakeQuerier) UpdateUserStatus(_ context.Context, arg database.UpdateUserStatusParams) (database.User, error) {
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

func (q *FakeQuerier) UpdateWorkspace(_ context.Context, arg database.UpdateWorkspaceParams) (database.Workspace, error) {
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

func (q *FakeQuerier) UpdateWorkspaceAgentConnectionByID(_ context.Context, arg database.UpdateWorkspaceAgentConnectionByIDParams) error {
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
		agent.LastConnectedReplicaID = arg.LastConnectedReplicaID
		q.workspaceAgents[index] = agent
		return nil
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateWorkspaceAgentLifecycleStateByID(_ context.Context, arg database.UpdateWorkspaceAgentLifecycleStateByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()
	for i, agent := range q.workspaceAgents {
		if agent.ID == arg.ID {
			agent.LifecycleState = arg.LifecycleState
			agent.StartedAt = arg.StartedAt
			agent.ReadyAt = arg.ReadyAt
			q.workspaceAgents[i] = agent
			return nil
		}
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateWorkspaceAgentLogOverflowByID(_ context.Context, arg database.UpdateWorkspaceAgentLogOverflowByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()
	for i, agent := range q.workspaceAgents {
		if agent.ID == arg.ID {
			agent.LogsOverflowed = arg.LogsOverflowed
			q.workspaceAgents[i] = agent
			return nil
		}
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateWorkspaceAgentMetadata(_ context.Context, arg database.UpdateWorkspaceAgentMetadataParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, m := range q.workspaceAgentMetadata {
		if m.WorkspaceAgentID != arg.WorkspaceAgentID {
			continue
		}
		for j := 0; j < len(arg.Key); j++ {
			if m.Key == arg.Key[j] {
				q.workspaceAgentMetadata[i].Value = arg.Value[j]
				q.workspaceAgentMetadata[i].Error = arg.Error[j]
				q.workspaceAgentMetadata[i].CollectedAt = arg.CollectedAt[j]
				return nil
			}
		}
	}

	return nil
}

func (q *FakeQuerier) UpdateWorkspaceAgentStartupByID(_ context.Context, arg database.UpdateWorkspaceAgentStartupByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	if len(arg.Subsystems) > 0 {
		seen := map[database.WorkspaceAgentSubsystem]struct{}{
			arg.Subsystems[0]: {},
		}
		for i := 1; i < len(arg.Subsystems); i++ {
			s := arg.Subsystems[i]
			if _, ok := seen[s]; ok {
				return xerrors.Errorf("duplicate subsystem %q", s)
			}
			seen[s] = struct{}{}

			if arg.Subsystems[i-1] > arg.Subsystems[i] {
				return xerrors.Errorf("subsystems not sorted: %q > %q", arg.Subsystems[i-1], arg.Subsystems[i])
			}
		}
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, agent := range q.workspaceAgents {
		if agent.ID != arg.ID {
			continue
		}

		agent.Version = arg.Version
		agent.APIVersion = arg.APIVersion
		agent.ExpandedDirectory = arg.ExpandedDirectory
		agent.Subsystems = arg.Subsystems
		q.workspaceAgents[index] = agent
		return nil
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateWorkspaceAppHealthByID(_ context.Context, arg database.UpdateWorkspaceAppHealthByIDParams) error {
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

func (q *FakeQuerier) UpdateWorkspaceAutomaticUpdates(_ context.Context, arg database.UpdateWorkspaceAutomaticUpdatesParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, workspace := range q.workspaces {
		if workspace.ID != arg.ID {
			continue
		}
		workspace.AutomaticUpdates = arg.AutomaticUpdates
		q.workspaces[index] = workspace
		return nil
	}

	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateWorkspaceAutostart(_ context.Context, arg database.UpdateWorkspaceAutostartParams) error {
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

func (q *FakeQuerier) UpdateWorkspaceBuildCostByID(_ context.Context, arg database.UpdateWorkspaceBuildCostByIDParams) error {
	if err := validateDatabaseType(arg); err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for index, workspaceBuild := range q.workspaceBuilds {
		if workspaceBuild.ID != arg.ID {
			continue
		}
		workspaceBuild.DailyCost = arg.DailyCost
		q.workspaceBuilds[index] = workspaceBuild
		return nil
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateWorkspaceBuildDeadlineByID(_ context.Context, arg database.UpdateWorkspaceBuildDeadlineByIDParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for idx, build := range q.workspaceBuilds {
		if build.ID != arg.ID {
			continue
		}
		build.Deadline = arg.Deadline
		build.MaxDeadline = arg.MaxDeadline
		build.UpdatedAt = arg.UpdatedAt
		q.workspaceBuilds[idx] = build
		return nil
	}

	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateWorkspaceBuildProvisionerStateByID(_ context.Context, arg database.UpdateWorkspaceBuildProvisionerStateByIDParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for idx, build := range q.workspaceBuilds {
		if build.ID != arg.ID {
			continue
		}
		build.ProvisionerState = arg.ProvisionerState
		build.UpdatedAt = arg.UpdatedAt
		q.workspaceBuilds[idx] = build
		return nil
	}

	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateWorkspaceDeletedByID(_ context.Context, arg database.UpdateWorkspaceDeletedByIDParams) error {
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

func (q *FakeQuerier) UpdateWorkspaceDormantDeletingAt(_ context.Context, arg database.UpdateWorkspaceDormantDeletingAtParams) (database.Workspace, error) {
	if err := validateDatabaseType(arg); err != nil {
		return database.Workspace{}, err
	}
	q.mutex.Lock()
	defer q.mutex.Unlock()
	for index, workspace := range q.workspaces {
		if workspace.ID != arg.ID {
			continue
		}
		workspace.DormantAt = arg.DormantAt
		if workspace.DormantAt.Time.IsZero() {
			workspace.LastUsedAt = dbtime.Now()
			workspace.DeletingAt = sql.NullTime{}
		}
		if !workspace.DormantAt.Time.IsZero() {
			var template database.TemplateTable
			for _, t := range q.templates {
				if t.ID == workspace.TemplateID {
					template = t
					break
				}
			}
			if template.ID == uuid.Nil {
				return database.Workspace{}, xerrors.Errorf("unable to find workspace template")
			}
			if template.TimeTilDormantAutoDelete > 0 {
				workspace.DeletingAt = sql.NullTime{
					Valid: true,
					Time:  workspace.DormantAt.Time.Add(time.Duration(template.TimeTilDormantAutoDelete)),
				}
			}
		}
		q.workspaces[index] = workspace
		return workspace, nil
	}
	return database.Workspace{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateWorkspaceLastUsedAt(_ context.Context, arg database.UpdateWorkspaceLastUsedAtParams) error {
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

func (q *FakeQuerier) UpdateWorkspaceProxy(_ context.Context, arg database.UpdateWorkspaceProxyParams) (database.WorkspaceProxy, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for _, p := range q.workspaceProxies {
		if p.Name == arg.Name && p.ID != arg.ID {
			return database.WorkspaceProxy{}, errDuplicateKey
		}
	}

	for i, p := range q.workspaceProxies {
		if p.ID == arg.ID {
			p.Name = arg.Name
			p.DisplayName = arg.DisplayName
			p.Icon = arg.Icon
			if len(p.TokenHashedSecret) > 0 {
				p.TokenHashedSecret = arg.TokenHashedSecret
			}
			q.workspaceProxies[i] = p
			return p, nil
		}
	}
	return database.WorkspaceProxy{}, sql.ErrNoRows
}

func (q *FakeQuerier) UpdateWorkspaceProxyDeleted(_ context.Context, arg database.UpdateWorkspaceProxyDeletedParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, p := range q.workspaceProxies {
		if p.ID == arg.ID {
			p.Deleted = arg.Deleted
			p.UpdatedAt = dbtime.Now()
			q.workspaceProxies[i] = p
			return nil
		}
	}
	return sql.ErrNoRows
}

func (q *FakeQuerier) UpdateWorkspaceTTL(_ context.Context, arg database.UpdateWorkspaceTTLParams) error {
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

func (q *FakeQuerier) UpdateWorkspacesDormantDeletingAtByTemplateID(_ context.Context, arg database.UpdateWorkspacesDormantDeletingAtByTemplateIDParams) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	for i, ws := range q.workspaces {
		if ws.TemplateID != arg.TemplateID {
			continue
		}

		if ws.DormantAt.Time.IsZero() {
			continue
		}

		if !arg.DormantAt.IsZero() {
			ws.DormantAt = sql.NullTime{
				Valid: true,
				Time:  arg.DormantAt,
			}
		}

		deletingAt := sql.NullTime{
			Valid: arg.TimeTilDormantAutodeleteMs > 0,
		}
		if arg.TimeTilDormantAutodeleteMs > 0 {
			deletingAt.Time = ws.DormantAt.Time.Add(time.Duration(arg.TimeTilDormantAutodeleteMs) * time.Millisecond)
		}
		ws.DeletingAt = deletingAt
		q.workspaces[i] = ws
	}

	return nil
}

func (q *FakeQuerier) UpsertAppSecurityKey(_ context.Context, data string) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.appSecurityKey = data
	return nil
}

func (q *FakeQuerier) UpsertApplicationName(_ context.Context, data string) error {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	q.applicationName = data
	return nil
}

func (q *FakeQuerier) UpsertDefaultProxy(_ context.Context, arg database.UpsertDefaultProxyParams) error {
	q.defaultProxyDisplayName = arg.DisplayName
	q.defaultProxyIconURL = arg.IconUrl
	return nil
}

func (q *FakeQuerier) UpsertHealthSettings(_ context.Context, data string) error {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	q.healthSettings = []byte(data)
	return nil
}

func (q *FakeQuerier) UpsertJFrogXrayScanByWorkspaceAndAgentID(_ context.Context, arg database.UpsertJFrogXrayScanByWorkspaceAndAgentIDParams) error {
	err := validateDatabaseType(arg)
	if err != nil {
		return err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, scan := range q.jfrogXRayScans {
		if scan.AgentID == arg.AgentID && scan.WorkspaceID == arg.WorkspaceID {
			scan.Critical = arg.Critical
			scan.High = arg.High
			scan.Medium = arg.Medium
			scan.ResultsUrl = arg.ResultsUrl
			q.jfrogXRayScans[i] = scan
			return nil
		}
	}

	//nolint:gosimple
	q.jfrogXRayScans = append(q.jfrogXRayScans, database.JfrogXrayScan{
		WorkspaceID: arg.WorkspaceID,
		AgentID:     arg.AgentID,
		Critical:    arg.Critical,
		High:        arg.High,
		Medium:      arg.Medium,
		ResultsUrl:  arg.ResultsUrl,
	})

	return nil
}

func (q *FakeQuerier) UpsertLastUpdateCheck(_ context.Context, data string) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.lastUpdateCheck = []byte(data)
	return nil
}

func (q *FakeQuerier) UpsertLogoURL(_ context.Context, data string) error {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	q.logoURL = data
	return nil
}

func (q *FakeQuerier) UpsertOAuthSigningKey(_ context.Context, value string) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.oauthSigningKey = value
	return nil
}

func (q *FakeQuerier) UpsertProvisionerDaemon(_ context.Context, arg database.UpsertProvisionerDaemonParams) (database.ProvisionerDaemon, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.ProvisionerDaemon{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()
	for _, d := range q.provisionerDaemons {
		if d.Name == arg.Name {
			if d.Tags[provisionersdk.TagScope] == provisionersdk.ScopeOrganization && arg.Tags[provisionersdk.TagOwner] != "" {
				continue
			}
			if d.Tags[provisionersdk.TagScope] == provisionersdk.ScopeUser && arg.Tags[provisionersdk.TagOwner] != d.Tags[provisionersdk.TagOwner] {
				continue
			}
			d.Provisioners = arg.Provisioners
			d.Tags = maps.Clone(arg.Tags)
			d.Version = arg.Version
			d.LastSeenAt = arg.LastSeenAt
			return d, nil
		}
	}
	d := database.ProvisionerDaemon{
		ID:           uuid.New(),
		CreatedAt:    arg.CreatedAt,
		Name:         arg.Name,
		Provisioners: arg.Provisioners,
		Tags:         maps.Clone(arg.Tags),
		ReplicaID:    uuid.NullUUID{},
		LastSeenAt:   arg.LastSeenAt,
		Version:      arg.Version,
		APIVersion:   arg.APIVersion,
	}
	q.provisionerDaemons = append(q.provisionerDaemons, d)
	return d, nil
}

func (q *FakeQuerier) UpsertServiceBanner(_ context.Context, data string) error {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	q.serviceBanner = []byte(data)
	return nil
}

func (*FakeQuerier) UpsertTailnetAgent(context.Context, database.UpsertTailnetAgentParams) (database.TailnetAgent, error) {
	return database.TailnetAgent{}, ErrUnimplemented
}

func (*FakeQuerier) UpsertTailnetClient(context.Context, database.UpsertTailnetClientParams) (database.TailnetClient, error) {
	return database.TailnetClient{}, ErrUnimplemented
}

func (*FakeQuerier) UpsertTailnetClientSubscription(context.Context, database.UpsertTailnetClientSubscriptionParams) error {
	return ErrUnimplemented
}

func (*FakeQuerier) UpsertTailnetCoordinator(context.Context, uuid.UUID) (database.TailnetCoordinator, error) {
	return database.TailnetCoordinator{}, ErrUnimplemented
}

func (*FakeQuerier) UpsertTailnetPeer(_ context.Context, arg database.UpsertTailnetPeerParams) (database.TailnetPeer, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.TailnetPeer{}, err
	}

	return database.TailnetPeer{}, ErrUnimplemented
}

func (*FakeQuerier) UpsertTailnetTunnel(_ context.Context, arg database.UpsertTailnetTunnelParams) (database.TailnetTunnel, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.TailnetTunnel{}, err
	}

	return database.TailnetTunnel{}, ErrUnimplemented
}

func (q *FakeQuerier) UpsertWorkspaceAgentPortShare(_ context.Context, arg database.UpsertWorkspaceAgentPortShareParams) (database.WorkspaceAgentPortShare, error) {
	err := validateDatabaseType(arg)
	if err != nil {
		return database.WorkspaceAgentPortShare{}, err
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	for i, share := range q.workspaceAgentPortShares {
		if share.WorkspaceID == arg.WorkspaceID && share.Port == arg.Port && share.AgentName == arg.AgentName {
			share.ShareLevel = arg.ShareLevel
			q.workspaceAgentPortShares[i] = share
			return share, nil
		}
	}

	//nolint:gosimple // casts are not a simplification
	psl := database.WorkspaceAgentPortShare{
		WorkspaceID: arg.WorkspaceID,
		AgentName:   arg.AgentName,
		Port:        arg.Port,
		ShareLevel:  arg.ShareLevel,
	}
	q.workspaceAgentPortShares = append(q.workspaceAgentPortShares, psl)

	return psl, nil
}

func (q *FakeQuerier) GetAuthorizedTemplates(ctx context.Context, arg database.GetTemplatesWithFilterParams, prepared rbac.PreparedAuthorized) ([]database.Template, error) {
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
	for _, templateTable := range q.templates {
		template := q.templateWithUserNoLock(templateTable)
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
		if arg.Deprecated.Valid && arg.Deprecated.Bool == (template.Deprecated != "") {
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
		slices.SortFunc(templates, func(a, b database.Template) int {
			if a.Name != b.Name {
				return slice.Ascending(a.Name, b.Name)
			}
			return slice.Ascending(a.ID.String(), b.ID.String())
		})
		return templates, nil
	}

	return nil, sql.ErrNoRows
}

func (q *FakeQuerier) GetTemplateGroupRoles(_ context.Context, id uuid.UUID) ([]database.TemplateGroup, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var template database.TemplateTable
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
		group, err := q.getGroupByIDNoLock(context.Background(), uuid.MustParse(k))
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

func (q *FakeQuerier) GetTemplateUserRoles(_ context.Context, id uuid.UUID) ([]database.TemplateUser, error) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	var template database.TemplateTable
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

func (q *FakeQuerier) GetAuthorizedWorkspaces(ctx context.Context, arg database.GetWorkspacesParams, prepared rbac.PreparedAuthorized) ([]database.GetWorkspacesRow, error) {
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

		if arg.UsingActive.Valid {
			build, err := q.getLatestWorkspaceBuildByWorkspaceIDNoLock(ctx, workspace.ID)
			if err != nil {
				return nil, xerrors.Errorf("get latest build: %w", err)
			}

			template, err := q.getTemplateByIDNoLock(ctx, workspace.TemplateID)
			if err != nil {
				return nil, xerrors.Errorf("get template: %w", err)
			}

			updated := build.TemplateVersionID == template.ActiveVersionID
			if arg.UsingActive.Bool != updated {
				continue
			}
		}

		if !arg.Deleted && workspace.Deleted {
			continue
		}

		if arg.Name != "" && !strings.Contains(strings.ToLower(workspace.Name), strings.ToLower(arg.Name)) {
			continue
		}

		if !arg.LastUsedBefore.IsZero() {
			if workspace.LastUsedAt.After(arg.LastUsedBefore) {
				continue
			}
		}

		if !arg.LastUsedAfter.IsZero() {
			if workspace.LastUsedAt.Before(arg.LastUsedAfter) {
				continue
			}
		}

		if arg.Status != "" {
			build, err := q.getLatestWorkspaceBuildByWorkspaceIDNoLock(ctx, workspace.ID)
			if err != nil {
				return nil, xerrors.Errorf("get latest build: %w", err)
			}

			job, err := q.getProvisionerJobByIDNoLock(ctx, build.JobID)
			if err != nil {
				return nil, xerrors.Errorf("get provisioner job: %w", err)
			}

			// This logic should match the logic in the workspace.sql file.
			var statusMatch bool
			switch database.WorkspaceStatus(arg.Status) {
			case database.WorkspaceStatusStarting:
				statusMatch = job.JobStatus == database.ProvisionerJobStatusRunning &&
					build.Transition == database.WorkspaceTransitionStart
			case database.WorkspaceStatusStopping:
				statusMatch = job.JobStatus == database.ProvisionerJobStatusRunning &&
					build.Transition == database.WorkspaceTransitionStop
			case database.WorkspaceStatusDeleting:
				statusMatch = job.JobStatus == database.ProvisionerJobStatusRunning &&
					build.Transition == database.WorkspaceTransitionDelete

			case "started":
				statusMatch = job.JobStatus == database.ProvisionerJobStatusSucceeded &&
					build.Transition == database.WorkspaceTransitionStart
			case database.WorkspaceStatusDeleted:
				statusMatch = job.JobStatus == database.ProvisionerJobStatusSucceeded &&
					build.Transition == database.WorkspaceTransitionDelete
			case database.WorkspaceStatusStopped:
				statusMatch = job.JobStatus == database.ProvisionerJobStatusSucceeded &&
					build.Transition == database.WorkspaceTransitionStop
			case database.WorkspaceStatusRunning:
				statusMatch = job.JobStatus == database.ProvisionerJobStatusSucceeded &&
					build.Transition == database.WorkspaceTransitionStart
			default:
				statusMatch = job.JobStatus == database.ProvisionerJobStatus(arg.Status)
			}
			if !statusMatch {
				continue
			}
		}

		if arg.HasAgent != "" {
			build, err := q.getLatestWorkspaceBuildByWorkspaceIDNoLock(ctx, workspace.ID)
			if err != nil {
				return nil, xerrors.Errorf("get latest build: %w", err)
			}

			job, err := q.getProvisionerJobByIDNoLock(ctx, build.JobID)
			if err != nil {
				return nil, xerrors.Errorf("get provisioner job: %w", err)
			}

			workspaceResources, err := q.getWorkspaceResourcesByJobIDNoLock(ctx, job.ID)
			if err != nil {
				return nil, xerrors.Errorf("get workspace resources: %w", err)
			}

			var workspaceResourceIDs []uuid.UUID
			for _, wr := range workspaceResources {
				workspaceResourceIDs = append(workspaceResourceIDs, wr.ID)
			}

			workspaceAgents, err := q.getWorkspaceAgentsByResourceIDsNoLock(ctx, workspaceResourceIDs)
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

		if arg.Dormant && !workspace.DormantAt.Valid {
			continue
		}

		if len(arg.TemplateIDs) > 0 {
			match := false
			for _, id := range arg.TemplateIDs {
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

	// Sort workspaces (ORDER BY)
	isRunning := func(build database.WorkspaceBuild, job database.ProvisionerJob) bool {
		return job.CompletedAt.Valid && !job.CanceledAt.Valid && !job.Error.Valid && build.Transition == database.WorkspaceTransitionStart
	}

	preloadedWorkspaceBuilds := map[uuid.UUID]database.WorkspaceBuild{}
	preloadedProvisionerJobs := map[uuid.UUID]database.ProvisionerJob{}
	preloadedUsers := map[uuid.UUID]database.User{}

	for _, w := range workspaces {
		build, err := q.getLatestWorkspaceBuildByWorkspaceIDNoLock(ctx, w.ID)
		if err == nil {
			preloadedWorkspaceBuilds[w.ID] = build
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get latest build: %w", err)
		}

		job, err := q.getProvisionerJobByIDNoLock(ctx, build.JobID)
		if err == nil {
			preloadedProvisionerJobs[w.ID] = job
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get provisioner job: %w", err)
		}

		user, err := q.getUserByIDNoLock(w.OwnerID)
		if err == nil {
			preloadedUsers[w.ID] = user
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get user: %w", err)
		}
	}

	sort.Slice(workspaces, func(i, j int) bool {
		w1 := workspaces[i]
		w2 := workspaces[j]

		// Order by: favorite first
		if arg.RequesterID == w1.OwnerID && w1.Favorite {
			return true
		}
		if arg.RequesterID == w2.OwnerID && w2.Favorite {
			return false
		}

		// Order by: running
		w1IsRunning := isRunning(preloadedWorkspaceBuilds[w1.ID], preloadedProvisionerJobs[w1.ID])
		w2IsRunning := isRunning(preloadedWorkspaceBuilds[w2.ID], preloadedProvisionerJobs[w2.ID])

		if w1IsRunning && !w2IsRunning {
			return true
		}

		if !w1IsRunning && w2IsRunning {
			return false
		}

		// Order by: usernames
		if strings.Compare(preloadedUsers[w1.ID].Username, preloadedUsers[w2.ID].Username) < 0 {
			return true
		}

		// Order by: workspace names
		return strings.Compare(w1.Name, w2.Name) < 0
	})

	beforePageCount := len(workspaces)

	if arg.Offset > 0 {
		if int(arg.Offset) > len(workspaces) {
			return []database.GetWorkspacesRow{}, nil
		}
		workspaces = workspaces[arg.Offset:]
	}
	if arg.Limit > 0 {
		if int(arg.Limit) > len(workspaces) {
			return q.convertToWorkspaceRowsNoLock(ctx, workspaces, int64(beforePageCount)), nil
		}
		workspaces = workspaces[:arg.Limit]
	}

	return q.convertToWorkspaceRowsNoLock(ctx, workspaces, int64(beforePageCount)), nil
}

func (q *FakeQuerier) GetAuthorizedUsers(ctx context.Context, arg database.GetUsersParams, prepared rbac.PreparedAuthorized) ([]database.GetUsersRow, error) {
	if err := validateDatabaseType(arg); err != nil {
		return nil, err
	}

	// Call this to match the same function calls as the SQL implementation.
	if prepared != nil {
		_, err := prepared.CompileToSQL(ctx, regosql.ConvertConfig{
			VariableConverter: regosql.UserConverter(),
		})
		if err != nil {
			return nil, err
		}
	}

	users, err := q.GetUsers(ctx, arg)
	if err != nil {
		return nil, err
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	filteredUsers := make([]database.GetUsersRow, 0, len(users))
	for _, user := range users {
		// If the filter exists, ensure the object is authorized.
		if prepared != nil && prepared.Authorize(ctx, user.RBACObject()) != nil {
			continue
		}

		filteredUsers = append(filteredUsers, user)
	}
	return filteredUsers, nil
}
