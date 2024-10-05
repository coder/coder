package dbgen

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/testutil"
)

// All methods take in a 'seed' object. Any provided fields in the seed will be
// maintained. Any fields omitted will have sensible defaults generated.

// genCtx is to give all generator functions permission if the db is a dbauthz db.
var genCtx = dbauthz.As(context.Background(), rbac.Subject{
	ID:     "owner",
	Roles:  rbac.Roles(must(rbac.RoleIdentifiers{rbac.RoleOwner()}.Expand())),
	Groups: []string{},
	Scope:  rbac.ExpandableScope(rbac.ScopeAll),
})

func AuditLog(t testing.TB, db database.Store, seed database.AuditLog) database.AuditLog {
	log, err := db.InsertAuditLog(genCtx, database.InsertAuditLogParams{
		ID:     takeFirst(seed.ID, uuid.New()),
		Time:   takeFirst(seed.Time, dbtime.Now()),
		UserID: takeFirst(seed.UserID, uuid.New()),
		// Default to the nil uuid. So by default audit logs are not org scoped.
		OrganizationID: takeFirst(seed.OrganizationID),
		Ip: pqtype.Inet{
			IPNet: takeFirstIP(seed.Ip.IPNet, net.IPNet{}),
			Valid: takeFirst(seed.Ip.Valid, false),
		},
		UserAgent: sql.NullString{
			String: takeFirst(seed.UserAgent.String, ""),
			Valid:  takeFirst(seed.UserAgent.Valid, false),
		},
		ResourceType:     takeFirst(seed.ResourceType, database.ResourceTypeOrganization),
		ResourceID:       takeFirst(seed.ResourceID, uuid.New()),
		ResourceTarget:   takeFirst(seed.ResourceTarget, uuid.NewString()),
		Action:           takeFirst(seed.Action, database.AuditActionCreate),
		Diff:             takeFirstSlice(seed.Diff, []byte("{}")),
		StatusCode:       takeFirst(seed.StatusCode, 200),
		AdditionalFields: takeFirstSlice(seed.Diff, []byte("{}")),
		RequestID:        takeFirst(seed.RequestID, uuid.New()),
		ResourceIcon:     takeFirst(seed.ResourceIcon, ""),
	})
	require.NoError(t, err, "insert audit log")
	return log
}

func Template(t testing.TB, db database.Store, seed database.Template) database.Template {
	id := takeFirst(seed.ID, uuid.New())
	if seed.GroupACL == nil {
		// By default, all users in the organization can read the template.
		seed.GroupACL = database.TemplateACL{
			seed.OrganizationID.String(): []policy.Action{policy.ActionRead},
		}
	}
	if seed.UserACL == nil {
		seed.UserACL = database.TemplateACL{}
	}
	err := db.InsertTemplate(genCtx, database.InsertTemplateParams{
		ID:                           id,
		CreatedAt:                    takeFirst(seed.CreatedAt, dbtime.Now()),
		UpdatedAt:                    takeFirst(seed.UpdatedAt, dbtime.Now()),
		OrganizationID:               takeFirst(seed.OrganizationID, uuid.New()),
		Name:                         takeFirst(seed.Name, testutil.GetRandomName(t)),
		Provisioner:                  takeFirst(seed.Provisioner, database.ProvisionerTypeEcho),
		ActiveVersionID:              takeFirst(seed.ActiveVersionID, uuid.New()),
		Description:                  takeFirst(seed.Description, testutil.GetRandomName(t)),
		CreatedBy:                    takeFirst(seed.CreatedBy, uuid.New()),
		Icon:                         takeFirst(seed.Icon, testutil.GetRandomName(t)),
		UserACL:                      seed.UserACL,
		GroupACL:                     seed.GroupACL,
		DisplayName:                  takeFirst(seed.DisplayName, testutil.GetRandomName(t)),
		AllowUserCancelWorkspaceJobs: seed.AllowUserCancelWorkspaceJobs,
		MaxPortSharingLevel:          takeFirst(seed.MaxPortSharingLevel, database.AppSharingLevelOwner),
	})
	require.NoError(t, err, "insert template")

	template, err := db.GetTemplateByID(genCtx, id)
	require.NoError(t, err, "get template")
	return template
}

func APIKey(t testing.TB, db database.Store, seed database.APIKey) (key database.APIKey, token string) {
	id, _ := cryptorand.String(10)
	secret, _ := cryptorand.String(22)
	hashed := sha256.Sum256([]byte(secret))

	ip := seed.IPAddress
	if !ip.Valid {
		ip = pqtype.Inet{
			IPNet: net.IPNet{
				IP:   net.IPv4(127, 0, 0, 1),
				Mask: net.IPv4Mask(255, 255, 255, 255),
			},
			Valid: true,
		}
	}

	key, err := db.InsertAPIKey(genCtx, database.InsertAPIKeyParams{
		ID: takeFirst(seed.ID, id),
		// 0 defaults to 86400 at the db layer
		LifetimeSeconds: takeFirst(seed.LifetimeSeconds, 0),
		HashedSecret:    takeFirstSlice(seed.HashedSecret, hashed[:]),
		IPAddress:       ip,
		UserID:          takeFirst(seed.UserID, uuid.New()),
		LastUsed:        takeFirst(seed.LastUsed, dbtime.Now()),
		ExpiresAt:       takeFirst(seed.ExpiresAt, dbtime.Now().Add(time.Hour)),
		CreatedAt:       takeFirst(seed.CreatedAt, dbtime.Now()),
		UpdatedAt:       takeFirst(seed.UpdatedAt, dbtime.Now()),
		LoginType:       takeFirst(seed.LoginType, database.LoginTypePassword),
		Scope:           takeFirst(seed.Scope, database.APIKeyScopeAll),
		TokenName:       takeFirst(seed.TokenName),
	})
	require.NoError(t, err, "insert api key")
	return key, fmt.Sprintf("%s-%s", key.ID, secret)
}

func WorkspaceAgentPortShare(t testing.TB, db database.Store, orig database.WorkspaceAgentPortShare) database.WorkspaceAgentPortShare {
	ps, err := db.UpsertWorkspaceAgentPortShare(genCtx, database.UpsertWorkspaceAgentPortShareParams{
		WorkspaceID: takeFirst(orig.WorkspaceID, uuid.New()),
		AgentName:   takeFirst(orig.AgentName, testutil.GetRandomName(t)),
		Port:        takeFirst(orig.Port, 8080),
		ShareLevel:  takeFirst(orig.ShareLevel, database.AppSharingLevelPublic),
		Protocol:    takeFirst(orig.Protocol, database.PortShareProtocolHttp),
	})
	require.NoError(t, err, "insert workspace agent")
	return ps
}

func WorkspaceAgent(t testing.TB, db database.Store, orig database.WorkspaceAgent) database.WorkspaceAgent {
	agt, err := db.InsertWorkspaceAgent(genCtx, database.InsertWorkspaceAgentParams{
		ID:         takeFirst(orig.ID, uuid.New()),
		CreatedAt:  takeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:  takeFirst(orig.UpdatedAt, dbtime.Now()),
		Name:       takeFirst(orig.Name, testutil.GetRandomName(t)),
		ResourceID: takeFirst(orig.ResourceID, uuid.New()),
		AuthToken:  takeFirst(orig.AuthToken, uuid.New()),
		AuthInstanceID: sql.NullString{
			String: takeFirst(orig.AuthInstanceID.String, testutil.GetRandomName(t)),
			Valid:  takeFirst(orig.AuthInstanceID.Valid, true),
		},
		Architecture: takeFirst(orig.Architecture, "amd64"),
		EnvironmentVariables: pqtype.NullRawMessage{
			RawMessage: takeFirstSlice(orig.EnvironmentVariables.RawMessage, []byte("{}")),
			Valid:      takeFirst(orig.EnvironmentVariables.Valid, false),
		},
		OperatingSystem: takeFirst(orig.OperatingSystem, "linux"),
		Directory:       takeFirst(orig.Directory, ""),
		InstanceMetadata: pqtype.NullRawMessage{
			RawMessage: takeFirstSlice(orig.ResourceMetadata.RawMessage, []byte("{}")),
			Valid:      takeFirst(orig.ResourceMetadata.Valid, false),
		},
		ResourceMetadata: pqtype.NullRawMessage{
			RawMessage: takeFirstSlice(orig.ResourceMetadata.RawMessage, []byte("{}")),
			Valid:      takeFirst(orig.ResourceMetadata.Valid, false),
		},
		ConnectionTimeoutSeconds: takeFirst(orig.ConnectionTimeoutSeconds, 3600),
		TroubleshootingURL:       takeFirst(orig.TroubleshootingURL, "https://example.com"),
		MOTDFile:                 takeFirst(orig.TroubleshootingURL, ""),
		DisplayApps:              append([]database.DisplayApp{}, orig.DisplayApps...),
		DisplayOrder:             takeFirst(orig.DisplayOrder, 1),
	})
	require.NoError(t, err, "insert workspace agent")
	return agt
}

func Workspace(t testing.TB, db database.Store, orig database.Workspace) database.Workspace {
	t.Helper()

	workspace, err := db.InsertWorkspace(genCtx, database.InsertWorkspaceParams{
		ID:                takeFirst(orig.ID, uuid.New()),
		OwnerID:           takeFirst(orig.OwnerID, uuid.New()),
		CreatedAt:         takeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:         takeFirst(orig.UpdatedAt, dbtime.Now()),
		OrganizationID:    takeFirst(orig.OrganizationID, uuid.New()),
		TemplateID:        takeFirst(orig.TemplateID, uuid.New()),
		LastUsedAt:        takeFirst(orig.LastUsedAt, dbtime.Now()),
		Name:              takeFirst(orig.Name, testutil.GetRandomName(t)),
		AutostartSchedule: orig.AutostartSchedule,
		Ttl:               orig.Ttl,
		AutomaticUpdates:  takeFirst(orig.AutomaticUpdates, database.AutomaticUpdatesNever),
	})
	require.NoError(t, err, "insert workspace")
	return workspace
}

func WorkspaceAgentLogSource(t testing.TB, db database.Store, orig database.WorkspaceAgentLogSource) database.WorkspaceAgentLogSource {
	sources, err := db.InsertWorkspaceAgentLogSources(genCtx, database.InsertWorkspaceAgentLogSourcesParams{
		WorkspaceAgentID: takeFirst(orig.WorkspaceAgentID, uuid.New()),
		ID:               []uuid.UUID{takeFirst(orig.ID, uuid.New())},
		CreatedAt:        takeFirst(orig.CreatedAt, dbtime.Now()),
		DisplayName:      []string{takeFirst(orig.DisplayName, testutil.GetRandomName(t))},
		Icon:             []string{takeFirst(orig.Icon, testutil.GetRandomName(t))},
	})
	require.NoError(t, err, "insert workspace agent log source")
	return sources[0]
}

func WorkspaceBuild(t testing.TB, db database.Store, orig database.WorkspaceBuild) database.WorkspaceBuild {
	t.Helper()

	buildID := takeFirst(orig.ID, uuid.New())
	var build database.WorkspaceBuild
	err := db.InTx(func(db database.Store) error {
		err := db.InsertWorkspaceBuild(genCtx, database.InsertWorkspaceBuildParams{
			ID:                buildID,
			CreatedAt:         takeFirst(orig.CreatedAt, dbtime.Now()),
			UpdatedAt:         takeFirst(orig.UpdatedAt, dbtime.Now()),
			WorkspaceID:       takeFirst(orig.WorkspaceID, uuid.New()),
			TemplateVersionID: takeFirst(orig.TemplateVersionID, uuid.New()),
			BuildNumber:       takeFirst(orig.BuildNumber, 1),
			Transition:        takeFirst(orig.Transition, database.WorkspaceTransitionStart),
			InitiatorID:       takeFirst(orig.InitiatorID, uuid.New()),
			JobID:             takeFirst(orig.JobID, uuid.New()),
			ProvisionerState:  takeFirstSlice(orig.ProvisionerState, []byte{}),
			Deadline:          takeFirst(orig.Deadline, dbtime.Now().Add(time.Hour)),
			MaxDeadline:       takeFirst(orig.MaxDeadline, time.Time{}),
			Reason:            takeFirst(orig.Reason, database.BuildReasonInitiator),
		})
		if err != nil {
			return err
		}
		build, err = db.GetWorkspaceBuildByID(genCtx, buildID)
		if err != nil {
			return err
		}
		return nil
	}, nil)
	require.NoError(t, err, "insert workspace build")

	return build
}

func WorkspaceBuildParameters(t testing.TB, db database.Store, orig []database.WorkspaceBuildParameter) []database.WorkspaceBuildParameter {
	if len(orig) == 0 {
		return nil
	}

	var (
		names  = make([]string, 0, len(orig))
		values = make([]string, 0, len(orig))
		params []database.WorkspaceBuildParameter
	)
	for _, param := range orig {
		names = append(names, param.Name)
		values = append(values, param.Value)
	}
	err := db.InTx(func(tx database.Store) error {
		id := takeFirst(orig[0].WorkspaceBuildID, uuid.New())
		err := tx.InsertWorkspaceBuildParameters(genCtx, database.InsertWorkspaceBuildParametersParams{
			WorkspaceBuildID: id,
			Name:             names,
			Value:            values,
		})
		if err != nil {
			return err
		}

		params, err = tx.GetWorkspaceBuildParameters(genCtx, id)
		return err
	}, nil)
	require.NoError(t, err)
	return params
}

func User(t testing.TB, db database.Store, orig database.User) database.User {
	user, err := db.InsertUser(genCtx, database.InsertUserParams{
		ID:             takeFirst(orig.ID, uuid.New()),
		Email:          takeFirst(orig.Email, testutil.GetRandomName(t)),
		Username:       takeFirst(orig.Username, testutil.GetRandomName(t)),
		Name:           takeFirst(orig.Name, testutil.GetRandomName(t)),
		HashedPassword: takeFirstSlice(orig.HashedPassword, []byte(must(cryptorand.String(32)))),
		CreatedAt:      takeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:      takeFirst(orig.UpdatedAt, dbtime.Now()),
		RBACRoles:      takeFirstSlice(orig.RBACRoles, []string{}),
		LoginType:      takeFirst(orig.LoginType, database.LoginTypePassword),
	})
	require.NoError(t, err, "insert user")

	user, err = db.UpdateUserStatus(genCtx, database.UpdateUserStatusParams{
		ID:        user.ID,
		Status:    takeFirst(orig.Status, database.UserStatusActive),
		UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err, "insert user")

	if !orig.LastSeenAt.IsZero() {
		user, err = db.UpdateUserLastSeenAt(genCtx, database.UpdateUserLastSeenAtParams{
			ID:         user.ID,
			LastSeenAt: orig.LastSeenAt,
			UpdatedAt:  user.UpdatedAt,
		})
		require.NoError(t, err, "user last seen")
	}

	if orig.Deleted {
		err = db.UpdateUserDeletedByID(genCtx, user.ID)
		require.NoError(t, err, "set user as deleted")
	}
	return user
}

func GitSSHKey(t testing.TB, db database.Store, orig database.GitSSHKey) database.GitSSHKey {
	key, err := db.InsertGitSSHKey(genCtx, database.InsertGitSSHKeyParams{
		UserID:     takeFirst(orig.UserID, uuid.New()),
		CreatedAt:  takeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:  takeFirst(orig.UpdatedAt, dbtime.Now()),
		PrivateKey: takeFirst(orig.PrivateKey, ""),
		PublicKey:  takeFirst(orig.PublicKey, ""),
	})
	require.NoError(t, err, "insert ssh key")
	return key
}

func Organization(t testing.TB, db database.Store, orig database.Organization) database.Organization {
	org, err := db.InsertOrganization(genCtx, database.InsertOrganizationParams{
		ID:          takeFirst(orig.ID, uuid.New()),
		Name:        takeFirst(orig.Name, testutil.GetRandomName(t)),
		DisplayName: takeFirst(orig.Name, testutil.GetRandomName(t)),
		Description: takeFirst(orig.Description, testutil.GetRandomName(t)),
		Icon:        takeFirst(orig.Icon, ""),
		CreatedAt:   takeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:   takeFirst(orig.UpdatedAt, dbtime.Now()),
	})
	require.NoError(t, err, "insert organization")
	return org
}

func OrganizationMember(t testing.TB, db database.Store, orig database.OrganizationMember) database.OrganizationMember {
	mem, err := db.InsertOrganizationMember(genCtx, database.InsertOrganizationMemberParams{
		OrganizationID: takeFirst(orig.OrganizationID, uuid.New()),
		UserID:         takeFirst(orig.UserID, uuid.New()),
		CreatedAt:      takeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:      takeFirst(orig.UpdatedAt, dbtime.Now()),
		Roles:          takeFirstSlice(orig.Roles, []string{}),
	})
	require.NoError(t, err, "insert organization")
	return mem
}

func Group(t testing.TB, db database.Store, orig database.Group) database.Group {
	name := takeFirst(orig.Name, testutil.GetRandomName(t))
	group, err := db.InsertGroup(genCtx, database.InsertGroupParams{
		ID:             takeFirst(orig.ID, uuid.New()),
		Name:           name,
		DisplayName:    takeFirst(orig.DisplayName, name),
		OrganizationID: takeFirst(orig.OrganizationID, uuid.New()),
		AvatarURL:      takeFirst(orig.AvatarURL, "https://logo.example.com"),
		QuotaAllowance: takeFirst(orig.QuotaAllowance, 0),
	})
	require.NoError(t, err, "insert group")
	return group
}

// GroupMember requires a user + group to already exist.
// Example for creating a group member for a random group + user.
//
//	GroupMember(t, db, database.GroupMemberTable{
//	  UserID:  User(t, db, database.User{}).ID,
//	  GroupID: Group(t, db, database.Group{
//	    OrganizationID: must(db.GetDefaultOrganization(genCtx)).ID,
//	  }).ID,
//	})
func GroupMember(t testing.TB, db database.Store, member database.GroupMemberTable) database.GroupMember {
	require.NotEqual(t, member.UserID, uuid.Nil, "A user id is required to use 'dbgen.GroupMember', use 'dbgen.User'.")
	require.NotEqual(t, member.GroupID, uuid.Nil, "A group id is required to use 'dbgen.GroupMember', use 'dbgen.Group'.")

	//nolint:gosimple
	err := db.InsertGroupMember(genCtx, database.InsertGroupMemberParams{
		UserID:  member.UserID,
		GroupID: member.GroupID,
	})
	require.NoError(t, err, "insert group member")

	user, err := db.GetUserByID(genCtx, member.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		require.NoErrorf(t, err, "'dbgen.GroupMember' failed as the user with id %s does not exist. A user is required to use this function, use 'dbgen.User'.", member.UserID)
	}
	require.NoError(t, err, "get user by id")

	group, err := db.GetGroupByID(genCtx, member.GroupID)
	if errors.Is(err, sql.ErrNoRows) {
		require.NoErrorf(t, err, "'dbgen.GroupMember' failed as the group with id %s does not exist. A group is required to use this function, use 'dbgen.Group'.", member.GroupID)
	}
	require.NoError(t, err, "get group by id")

	groupMember := database.GroupMember{
		UserID:                 user.ID,
		UserEmail:              user.Email,
		UserUsername:           user.Username,
		UserHashedPassword:     user.HashedPassword,
		UserCreatedAt:          user.CreatedAt,
		UserUpdatedAt:          user.UpdatedAt,
		UserStatus:             user.Status,
		UserRbacRoles:          user.RBACRoles,
		UserLoginType:          user.LoginType,
		UserAvatarUrl:          user.AvatarURL,
		UserDeleted:            user.Deleted,
		UserLastSeenAt:         user.LastSeenAt,
		UserQuietHoursSchedule: user.QuietHoursSchedule,
		UserThemePreference:    user.ThemePreference,
		UserName:               user.Name,
		UserGithubComUserID:    user.GithubComUserID,
		OrganizationID:         group.OrganizationID,
		GroupName:              group.Name,
		GroupID:                group.ID,
	}

	return groupMember
}

// ProvisionerJob is a bit more involved to get the values such as "completedAt", "startedAt", "cancelledAt" set.  ps
// can be set to nil if you are SURE that you don't require a provisionerdaemon to acquire the job in your test.
func ProvisionerJob(t testing.TB, db database.Store, ps pubsub.Pubsub, orig database.ProvisionerJob) database.ProvisionerJob {
	t.Helper()

	var defOrgID uuid.UUID
	if orig.OrganizationID == uuid.Nil {
		defOrg, _ := db.GetDefaultOrganization(genCtx)
		defOrgID = defOrg.ID
	}

	jobID := takeFirst(orig.ID, uuid.New())
	// Always set some tags to prevent Acquire from grabbing jobs it should not.
	if !orig.StartedAt.Time.IsZero() {
		if orig.Tags == nil {
			orig.Tags = make(database.StringMap)
		}
		// Make sure when we acquire the job, we only get this one.
		orig.Tags[jobID.String()] = "true"
	}

	job, err := db.InsertProvisionerJob(genCtx, database.InsertProvisionerJobParams{
		ID:             jobID,
		CreatedAt:      takeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:      takeFirst(orig.UpdatedAt, dbtime.Now()),
		OrganizationID: takeFirst(orig.OrganizationID, defOrgID, uuid.New()),
		InitiatorID:    takeFirst(orig.InitiatorID, uuid.New()),
		Provisioner:    takeFirst(orig.Provisioner, database.ProvisionerTypeEcho),
		StorageMethod:  takeFirst(orig.StorageMethod, database.ProvisionerStorageMethodFile),
		FileID:         takeFirst(orig.FileID, uuid.New()),
		Type:           takeFirst(orig.Type, database.ProvisionerJobTypeWorkspaceBuild),
		Input:          takeFirstSlice(orig.Input, []byte("{}")),
		Tags:           orig.Tags,
		TraceMetadata:  pqtype.NullRawMessage{},
	})
	require.NoError(t, err, "insert job")
	if ps != nil {
		err = provisionerjobs.PostJob(ps, job)
		require.NoError(t, err, "post job to pubsub")
	}
	if !orig.StartedAt.Time.IsZero() {
		job, err = db.AcquireProvisionerJob(genCtx, database.AcquireProvisionerJobParams{
			StartedAt:      orig.StartedAt,
			OrganizationID: job.OrganizationID,
			Types:          []database.ProvisionerType{database.ProvisionerTypeEcho},
			Tags:           must(json.Marshal(orig.Tags)),
			WorkerID:       uuid.NullUUID{},
		})
		require.NoError(t, err)
		// There is no easy way to make sure we acquire the correct job.
		require.Equal(t, jobID, job.ID, "acquired incorrect job")
	}

	if !orig.CompletedAt.Time.IsZero() || orig.Error.String != "" {
		err := db.UpdateProvisionerJobWithCompleteByID(genCtx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:          jobID,
			UpdatedAt:   job.UpdatedAt,
			CompletedAt: orig.CompletedAt,
			Error:       orig.Error,
			ErrorCode:   orig.ErrorCode,
		})
		require.NoError(t, err)
	}
	if !orig.CanceledAt.Time.IsZero() {
		err := db.UpdateProvisionerJobWithCancelByID(genCtx, database.UpdateProvisionerJobWithCancelByIDParams{
			ID:          jobID,
			CanceledAt:  orig.CanceledAt,
			CompletedAt: orig.CompletedAt,
		})
		require.NoError(t, err)
	}

	job, err = db.GetProvisionerJobByID(genCtx, jobID)
	require.NoError(t, err)

	return job
}

func ProvisionerKey(t testing.TB, db database.Store, orig database.ProvisionerKey) database.ProvisionerKey {
	key, err := db.InsertProvisionerKey(genCtx, database.InsertProvisionerKeyParams{
		ID:             takeFirst(orig.ID, uuid.New()),
		CreatedAt:      takeFirst(orig.CreatedAt, dbtime.Now()),
		OrganizationID: takeFirst(orig.OrganizationID, uuid.New()),
		Name:           takeFirst(orig.Name, testutil.GetRandomName(t)),
		HashedSecret:   orig.HashedSecret,
		Tags:           orig.Tags,
	})
	require.NoError(t, err, "insert provisioner key")
	return key
}

func WorkspaceApp(t testing.TB, db database.Store, orig database.WorkspaceApp) database.WorkspaceApp {
	resource, err := db.InsertWorkspaceApp(genCtx, database.InsertWorkspaceAppParams{
		ID:          takeFirst(orig.ID, uuid.New()),
		CreatedAt:   takeFirst(orig.CreatedAt, dbtime.Now()),
		AgentID:     takeFirst(orig.AgentID, uuid.New()),
		Slug:        takeFirst(orig.Slug, testutil.GetRandomName(t)),
		DisplayName: takeFirst(orig.DisplayName, testutil.GetRandomName(t)),
		Icon:        takeFirst(orig.Icon, testutil.GetRandomName(t)),
		Command: sql.NullString{
			String: takeFirst(orig.Command.String, "ls"),
			Valid:  orig.Command.Valid,
		},
		Url: sql.NullString{
			String: takeFirst(orig.Url.String),
			Valid:  orig.Url.Valid,
		},
		External:             orig.External,
		Subdomain:            orig.Subdomain,
		SharingLevel:         takeFirst(orig.SharingLevel, database.AppSharingLevelOwner),
		HealthcheckUrl:       takeFirst(orig.HealthcheckUrl, "https://localhost:8000"),
		HealthcheckInterval:  takeFirst(orig.HealthcheckInterval, 60),
		HealthcheckThreshold: takeFirst(orig.HealthcheckThreshold, 60),
		Health:               takeFirst(orig.Health, database.WorkspaceAppHealthHealthy),
		DisplayOrder:         takeFirst(orig.DisplayOrder, 1),
		Hidden:               orig.Hidden,
	})
	require.NoError(t, err, "insert app")
	return resource
}

func WorkspaceAppStat(t testing.TB, db database.Store, orig database.WorkspaceAppStat) database.WorkspaceAppStat {
	// This is not going to be correct, but our query doesn't return the ID.
	id, err := cryptorand.Int63()
	require.NoError(t, err, "generate id")

	scheme := database.WorkspaceAppStat{
		ID:               takeFirst(orig.ID, id),
		UserID:           takeFirst(orig.UserID, uuid.New()),
		WorkspaceID:      takeFirst(orig.WorkspaceID, uuid.New()),
		AgentID:          takeFirst(orig.AgentID, uuid.New()),
		AccessMethod:     takeFirst(orig.AccessMethod, ""),
		SlugOrPort:       takeFirst(orig.SlugOrPort, ""),
		SessionID:        takeFirst(orig.SessionID, uuid.New()),
		SessionStartedAt: takeFirst(orig.SessionStartedAt, dbtime.Now().Add(-time.Minute)),
		SessionEndedAt:   takeFirst(orig.SessionEndedAt, dbtime.Now()),
		Requests:         takeFirst(orig.Requests, 1),
	}
	err = db.InsertWorkspaceAppStats(genCtx, database.InsertWorkspaceAppStatsParams{
		UserID:           []uuid.UUID{scheme.UserID},
		WorkspaceID:      []uuid.UUID{scheme.WorkspaceID},
		AgentID:          []uuid.UUID{scheme.AgentID},
		AccessMethod:     []string{scheme.AccessMethod},
		SlugOrPort:       []string{scheme.SlugOrPort},
		SessionID:        []uuid.UUID{scheme.SessionID},
		SessionStartedAt: []time.Time{scheme.SessionStartedAt},
		SessionEndedAt:   []time.Time{scheme.SessionEndedAt},
		Requests:         []int32{scheme.Requests},
	})
	require.NoError(t, err, "insert workspace agent stat")
	return scheme
}

func WorkspaceResource(t testing.TB, db database.Store, orig database.WorkspaceResource) database.WorkspaceResource {
	resource, err := db.InsertWorkspaceResource(genCtx, database.InsertWorkspaceResourceParams{
		ID:         takeFirst(orig.ID, uuid.New()),
		CreatedAt:  takeFirst(orig.CreatedAt, dbtime.Now()),
		JobID:      takeFirst(orig.JobID, uuid.New()),
		Transition: takeFirst(orig.Transition, database.WorkspaceTransitionStart),
		Type:       takeFirst(orig.Type, "fake_resource"),
		Name:       takeFirst(orig.Name, testutil.GetRandomName(t)),
		Hide:       takeFirst(orig.Hide, false),
		Icon:       takeFirst(orig.Icon, ""),
		InstanceType: sql.NullString{
			String: takeFirst(orig.InstanceType.String, ""),
			Valid:  takeFirst(orig.InstanceType.Valid, false),
		},
		DailyCost: takeFirst(orig.DailyCost, 0),
	})
	require.NoError(t, err, "insert resource")
	return resource
}

func WorkspaceResourceMetadatums(t testing.TB, db database.Store, seed database.WorkspaceResourceMetadatum) []database.WorkspaceResourceMetadatum {
	meta, err := db.InsertWorkspaceResourceMetadata(genCtx, database.InsertWorkspaceResourceMetadataParams{
		WorkspaceResourceID: takeFirst(seed.WorkspaceResourceID, uuid.New()),
		Key:                 []string{takeFirst(seed.Key, testutil.GetRandomName(t))},
		Value:               []string{takeFirst(seed.Value.String, testutil.GetRandomName(t))},
		Sensitive:           []bool{takeFirst(seed.Sensitive, false)},
	})
	require.NoError(t, err, "insert meta data")
	return meta
}

func WorkspaceProxy(t testing.TB, db database.Store, orig database.WorkspaceProxy) (database.WorkspaceProxy, string) {
	secret, err := cryptorand.HexString(64)
	require.NoError(t, err, "generate secret")
	hashedSecret := sha256.Sum256([]byte(secret))

	proxy, err := db.InsertWorkspaceProxy(genCtx, database.InsertWorkspaceProxyParams{
		ID:                takeFirst(orig.ID, uuid.New()),
		Name:              takeFirst(orig.Name, testutil.GetRandomName(t)),
		DisplayName:       takeFirst(orig.DisplayName, testutil.GetRandomName(t)),
		Icon:              takeFirst(orig.Icon, testutil.GetRandomName(t)),
		TokenHashedSecret: hashedSecret[:],
		CreatedAt:         takeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:         takeFirst(orig.UpdatedAt, dbtime.Now()),
		DerpEnabled:       takeFirst(orig.DerpEnabled, false),
		DerpOnly:          takeFirst(orig.DerpEnabled, false),
	})
	require.NoError(t, err, "insert proxy")

	// Also set these fields if the caller wants them.
	if orig.Url != "" || orig.WildcardHostname != "" {
		proxy, err = db.RegisterWorkspaceProxy(genCtx, database.RegisterWorkspaceProxyParams{
			Url:              orig.Url,
			WildcardHostname: orig.WildcardHostname,
			ID:               proxy.ID,
		})
		require.NoError(t, err, "update proxy")
	}
	return proxy, secret
}

func File(t testing.TB, db database.Store, orig database.File) database.File {
	file, err := db.InsertFile(genCtx, database.InsertFileParams{
		ID:        takeFirst(orig.ID, uuid.New()),
		Hash:      takeFirst(orig.Hash, hex.EncodeToString(make([]byte, 32))),
		CreatedAt: takeFirst(orig.CreatedAt, dbtime.Now()),
		CreatedBy: takeFirst(orig.CreatedBy, uuid.New()),
		Mimetype:  takeFirst(orig.Mimetype, "application/x-tar"),
		Data:      takeFirstSlice(orig.Data, []byte{}),
	})
	require.NoError(t, err, "insert file")
	return file
}

func UserLink(t testing.TB, db database.Store, orig database.UserLink) database.UserLink {
	link, err := db.InsertUserLink(genCtx, database.InsertUserLinkParams{
		UserID:                 takeFirst(orig.UserID, uuid.New()),
		LoginType:              takeFirst(orig.LoginType, database.LoginTypeGithub),
		LinkedID:               takeFirst(orig.LinkedID),
		OAuthAccessToken:       takeFirst(orig.OAuthAccessToken, uuid.NewString()),
		OAuthAccessTokenKeyID:  takeFirst(orig.OAuthAccessTokenKeyID, sql.NullString{}),
		OAuthRefreshToken:      takeFirst(orig.OAuthRefreshToken, uuid.NewString()),
		OAuthRefreshTokenKeyID: takeFirst(orig.OAuthRefreshTokenKeyID, sql.NullString{}),
		OAuthExpiry:            takeFirst(orig.OAuthExpiry, dbtime.Now().Add(time.Hour*24)),
		DebugContext:           takeFirstSlice(orig.DebugContext, json.RawMessage("{}")),
	})

	require.NoError(t, err, "insert link")
	return link
}

func ExternalAuthLink(t testing.TB, db database.Store, orig database.ExternalAuthLink) database.ExternalAuthLink {
	msg := takeFirst(&orig.OAuthExtra, &pqtype.NullRawMessage{})
	link, err := db.InsertExternalAuthLink(genCtx, database.InsertExternalAuthLinkParams{
		ProviderID:             takeFirst(orig.ProviderID, uuid.New().String()),
		UserID:                 takeFirst(orig.UserID, uuid.New()),
		OAuthAccessToken:       takeFirst(orig.OAuthAccessToken, uuid.NewString()),
		OAuthAccessTokenKeyID:  takeFirst(orig.OAuthAccessTokenKeyID, sql.NullString{}),
		OAuthRefreshToken:      takeFirst(orig.OAuthRefreshToken, uuid.NewString()),
		OAuthRefreshTokenKeyID: takeFirst(orig.OAuthRefreshTokenKeyID, sql.NullString{}),
		OAuthExpiry:            takeFirst(orig.OAuthExpiry, dbtime.Now().Add(time.Hour*24)),
		CreatedAt:              takeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:              takeFirst(orig.UpdatedAt, dbtime.Now()),
		OAuthExtra:             *msg,
	})

	require.NoError(t, err, "insert external auth link")
	return link
}

func TemplateVersion(t testing.TB, db database.Store, orig database.TemplateVersion) database.TemplateVersion {
	var version database.TemplateVersion
	err := db.InTx(func(db database.Store) error {
		versionID := takeFirst(orig.ID, uuid.New())
		err := db.InsertTemplateVersion(genCtx, database.InsertTemplateVersionParams{
			ID:             versionID,
			TemplateID:     takeFirst(orig.TemplateID, uuid.NullUUID{}),
			OrganizationID: takeFirst(orig.OrganizationID, uuid.New()),
			CreatedAt:      takeFirst(orig.CreatedAt, dbtime.Now()),
			UpdatedAt:      takeFirst(orig.UpdatedAt, dbtime.Now()),
			Name:           takeFirst(orig.Name, testutil.GetRandomName(t)),
			Message:        orig.Message,
			Readme:         takeFirst(orig.Readme, testutil.GetRandomName(t)),
			JobID:          takeFirst(orig.JobID, uuid.New()),
			CreatedBy:      takeFirst(orig.CreatedBy, uuid.New()),
		})
		if err != nil {
			return err
		}

		version, err = db.GetTemplateVersionByID(genCtx, versionID)
		if err != nil {
			return err
		}
		return nil
	}, nil)
	require.NoError(t, err, "insert template version")

	return version
}

func TemplateVersionVariable(t testing.TB, db database.Store, orig database.TemplateVersionVariable) database.TemplateVersionVariable {
	version, err := db.InsertTemplateVersionVariable(genCtx, database.InsertTemplateVersionVariableParams{
		TemplateVersionID: takeFirst(orig.TemplateVersionID, uuid.New()),
		Name:              takeFirst(orig.Name, testutil.GetRandomName(t)),
		Description:       takeFirst(orig.Description, testutil.GetRandomName(t)),
		Type:              takeFirst(orig.Type, "string"),
		Value:             takeFirst(orig.Value, ""),
		DefaultValue:      takeFirst(orig.DefaultValue, testutil.GetRandomName(t)),
		Required:          takeFirst(orig.Required, false),
		Sensitive:         takeFirst(orig.Sensitive, false),
	})
	require.NoError(t, err, "insert template version variable")
	return version
}

func TemplateVersionWorkspaceTag(t testing.TB, db database.Store, orig database.TemplateVersionWorkspaceTag) database.TemplateVersionWorkspaceTag {
	workspaceTag, err := db.InsertTemplateVersionWorkspaceTag(genCtx, database.InsertTemplateVersionWorkspaceTagParams{
		TemplateVersionID: takeFirst(orig.TemplateVersionID, uuid.New()),
		Key:               takeFirst(orig.Key, testutil.GetRandomName(t)),
		Value:             takeFirst(orig.Value, testutil.GetRandomName(t)),
	})
	require.NoError(t, err, "insert template version workspace tag")
	return workspaceTag
}

func TemplateVersionParameter(t testing.TB, db database.Store, orig database.TemplateVersionParameter) database.TemplateVersionParameter {
	t.Helper()

	version, err := db.InsertTemplateVersionParameter(genCtx, database.InsertTemplateVersionParameterParams{
		TemplateVersionID:   takeFirst(orig.TemplateVersionID, uuid.New()),
		Name:                takeFirst(orig.Name, testutil.GetRandomName(t)),
		Description:         takeFirst(orig.Description, testutil.GetRandomName(t)),
		Type:                takeFirst(orig.Type, "string"),
		Mutable:             takeFirst(orig.Mutable, false),
		DefaultValue:        takeFirst(orig.DefaultValue, testutil.GetRandomName(t)),
		Icon:                takeFirst(orig.Icon, testutil.GetRandomName(t)),
		Options:             takeFirstSlice(orig.Options, []byte("[]")),
		ValidationRegex:     takeFirst(orig.ValidationRegex, ""),
		ValidationMin:       takeFirst(orig.ValidationMin, sql.NullInt32{}),
		ValidationMax:       takeFirst(orig.ValidationMax, sql.NullInt32{}),
		ValidationError:     takeFirst(orig.ValidationError, ""),
		ValidationMonotonic: takeFirst(orig.ValidationMonotonic, ""),
		Required:            takeFirst(orig.Required, false),
		DisplayName:         takeFirst(orig.DisplayName, testutil.GetRandomName(t)),
		DisplayOrder:        takeFirst(orig.DisplayOrder, 0),
		Ephemeral:           takeFirst(orig.Ephemeral, false),
	})
	require.NoError(t, err, "insert template version parameter")
	return version
}

func WorkspaceAgentStat(t testing.TB, db database.Store, orig database.WorkspaceAgentStat) database.WorkspaceAgentStat {
	if orig.ConnectionsByProto == nil {
		orig.ConnectionsByProto = json.RawMessage([]byte("{}"))
	}
	jsonProto := []byte(fmt.Sprintf("[%s]", orig.ConnectionsByProto))

	params := database.InsertWorkspaceAgentStatsParams{
		ID:                          []uuid.UUID{takeFirst(orig.ID, uuid.New())},
		CreatedAt:                   []time.Time{takeFirst(orig.CreatedAt, dbtime.Now())},
		UserID:                      []uuid.UUID{takeFirst(orig.UserID, uuid.New())},
		TemplateID:                  []uuid.UUID{takeFirst(orig.TemplateID, uuid.New())},
		WorkspaceID:                 []uuid.UUID{takeFirst(orig.WorkspaceID, uuid.New())},
		AgentID:                     []uuid.UUID{takeFirst(orig.AgentID, uuid.New())},
		ConnectionsByProto:          jsonProto,
		ConnectionCount:             []int64{takeFirst(orig.ConnectionCount, 0)},
		RxPackets:                   []int64{takeFirst(orig.RxPackets, 0)},
		RxBytes:                     []int64{takeFirst(orig.RxBytes, 0)},
		TxPackets:                   []int64{takeFirst(orig.TxPackets, 0)},
		TxBytes:                     []int64{takeFirst(orig.TxBytes, 0)},
		SessionCountVSCode:          []int64{takeFirst(orig.SessionCountVSCode, 0)},
		SessionCountJetBrains:       []int64{takeFirst(orig.SessionCountJetBrains, 0)},
		SessionCountReconnectingPTY: []int64{takeFirst(orig.SessionCountReconnectingPTY, 0)},
		SessionCountSSH:             []int64{takeFirst(orig.SessionCountSSH, 0)},
		ConnectionMedianLatencyMS:   []float64{takeFirst(orig.ConnectionMedianLatencyMS, 0)},
		Usage:                       []bool{takeFirst(orig.Usage, false)},
	}
	err := db.InsertWorkspaceAgentStats(genCtx, params)
	require.NoError(t, err, "insert workspace agent stat")

	return database.WorkspaceAgentStat{
		ID:                          params.ID[0],
		CreatedAt:                   params.CreatedAt[0],
		UserID:                      params.UserID[0],
		AgentID:                     params.AgentID[0],
		WorkspaceID:                 params.WorkspaceID[0],
		TemplateID:                  params.TemplateID[0],
		ConnectionsByProto:          orig.ConnectionsByProto,
		ConnectionCount:             params.ConnectionCount[0],
		RxPackets:                   params.RxPackets[0],
		RxBytes:                     params.RxBytes[0],
		TxPackets:                   params.TxPackets[0],
		TxBytes:                     params.TxBytes[0],
		ConnectionMedianLatencyMS:   params.ConnectionMedianLatencyMS[0],
		SessionCountVSCode:          params.SessionCountVSCode[0],
		SessionCountJetBrains:       params.SessionCountJetBrains[0],
		SessionCountReconnectingPTY: params.SessionCountReconnectingPTY[0],
		SessionCountSSH:             params.SessionCountSSH[0],
		Usage:                       params.Usage[0],
	}
}

func OAuth2ProviderApp(t testing.TB, db database.Store, seed database.OAuth2ProviderApp) database.OAuth2ProviderApp {
	app, err := db.InsertOAuth2ProviderApp(genCtx, database.InsertOAuth2ProviderAppParams{
		ID:          takeFirst(seed.ID, uuid.New()),
		Name:        takeFirst(seed.Name, testutil.GetRandomName(t)),
		CreatedAt:   takeFirst(seed.CreatedAt, dbtime.Now()),
		UpdatedAt:   takeFirst(seed.UpdatedAt, dbtime.Now()),
		Icon:        takeFirst(seed.Icon, ""),
		CallbackURL: takeFirst(seed.CallbackURL, "http://localhost"),
	})
	require.NoError(t, err, "insert oauth2 app")
	return app
}

func OAuth2ProviderAppSecret(t testing.TB, db database.Store, seed database.OAuth2ProviderAppSecret) database.OAuth2ProviderAppSecret {
	app, err := db.InsertOAuth2ProviderAppSecret(genCtx, database.InsertOAuth2ProviderAppSecretParams{
		ID:            takeFirst(seed.ID, uuid.New()),
		CreatedAt:     takeFirst(seed.CreatedAt, dbtime.Now()),
		SecretPrefix:  takeFirstSlice(seed.SecretPrefix, []byte("prefix")),
		HashedSecret:  takeFirstSlice(seed.HashedSecret, []byte("hashed-secret")),
		DisplaySecret: takeFirst(seed.DisplaySecret, "secret"),
		AppID:         takeFirst(seed.AppID, uuid.New()),
	})
	require.NoError(t, err, "insert oauth2 app secret")
	return app
}

func OAuth2ProviderAppCode(t testing.TB, db database.Store, seed database.OAuth2ProviderAppCode) database.OAuth2ProviderAppCode {
	code, err := db.InsertOAuth2ProviderAppCode(genCtx, database.InsertOAuth2ProviderAppCodeParams{
		ID:           takeFirst(seed.ID, uuid.New()),
		CreatedAt:    takeFirst(seed.CreatedAt, dbtime.Now()),
		ExpiresAt:    takeFirst(seed.CreatedAt, dbtime.Now()),
		SecretPrefix: takeFirstSlice(seed.SecretPrefix, []byte("prefix")),
		HashedSecret: takeFirstSlice(seed.HashedSecret, []byte("hashed-secret")),
		AppID:        takeFirst(seed.AppID, uuid.New()),
		UserID:       takeFirst(seed.UserID, uuid.New()),
	})
	require.NoError(t, err, "insert oauth2 app code")
	return code
}

func OAuth2ProviderAppToken(t testing.TB, db database.Store, seed database.OAuth2ProviderAppToken) database.OAuth2ProviderAppToken {
	token, err := db.InsertOAuth2ProviderAppToken(genCtx, database.InsertOAuth2ProviderAppTokenParams{
		ID:          takeFirst(seed.ID, uuid.New()),
		CreatedAt:   takeFirst(seed.CreatedAt, dbtime.Now()),
		ExpiresAt:   takeFirst(seed.CreatedAt, dbtime.Now()),
		HashPrefix:  takeFirstSlice(seed.HashPrefix, []byte("prefix")),
		RefreshHash: takeFirstSlice(seed.RefreshHash, []byte("hashed-secret")),
		AppSecretID: takeFirst(seed.AppSecretID, uuid.New()),
		APIKeyID:    takeFirst(seed.APIKeyID, uuid.New().String()),
	})
	require.NoError(t, err, "insert oauth2 app token")
	return token
}

func CustomRole(t testing.TB, db database.Store, seed database.CustomRole) database.CustomRole {
	role, err := db.InsertCustomRole(genCtx, database.InsertCustomRoleParams{
		Name:            takeFirst(seed.Name, strings.ToLower(testutil.GetRandomName(t))),
		DisplayName:     testutil.GetRandomName(t),
		OrganizationID:  seed.OrganizationID,
		SitePermissions: takeFirstSlice(seed.SitePermissions, []database.CustomRolePermission{}),
		OrgPermissions:  takeFirstSlice(seed.SitePermissions, []database.CustomRolePermission{}),
		UserPermissions: takeFirstSlice(seed.SitePermissions, []database.CustomRolePermission{}),
	})
	require.NoError(t, err, "insert custom role")
	return role
}

func CryptoKey(t testing.TB, db database.Store, seed database.CryptoKey) database.CryptoKey {
	t.Helper()

	seed.Feature = takeFirst(seed.Feature, database.CryptoKeyFeatureWorkspaceApps)

	// An empty string for the secret is interpreted as
	// a caller wanting a new secret to be generated.
	// To generate a key with a NULL secret set Valid=false
	// and String to a non-empty string.
	if seed.Secret.String == "" {
		secret, err := newCryptoKeySecret(seed.Feature)
		require.NoError(t, err, "generate secret")
		seed.Secret = sql.NullString{
			String: secret,
			Valid:  true,
		}
	}

	key, err := db.InsertCryptoKey(genCtx, database.InsertCryptoKeyParams{
		Sequence:    takeFirst(seed.Sequence, 123),
		Secret:      seed.Secret,
		SecretKeyID: takeFirst(seed.SecretKeyID, sql.NullString{}),
		Feature:     seed.Feature,
		StartsAt:    takeFirst(seed.StartsAt, dbtime.Now()),
	})
	require.NoError(t, err, "insert crypto key")

	if seed.DeletesAt.Valid {
		key, err = db.UpdateCryptoKeyDeletesAt(genCtx, database.UpdateCryptoKeyDeletesAtParams{
			Feature:   key.Feature,
			Sequence:  key.Sequence,
			DeletesAt: sql.NullTime{Time: seed.DeletesAt.Time, Valid: true},
		})
		require.NoError(t, err, "update crypto key deletes_at")
	}
	return key
}

func ProvisionerJobTimings(t testing.TB, db database.Store, seed database.InsertProvisionerJobTimingsParams) []database.ProvisionerJobTiming {
	timings, err := db.InsertProvisionerJobTimings(genCtx, seed)
	require.NoError(t, err, "insert provisioner job timings")
	return timings
}

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

func takeFirstIP(values ...net.IPNet) net.IPNet {
	return takeFirstF(values, func(v net.IPNet) bool {
		return len(v.IP) != 0 && len(v.Mask) != 0
	})
}

// takeFirstSlice implements takeFirst for []any.
// []any is not a comparable type.
func takeFirstSlice[T any](values ...[]T) []T {
	return takeFirstF(values, func(v []T) bool {
		return len(v) != 0
	})
}

// takeFirstF takes the first value that returns true
func takeFirstF[Value any](values []Value, take func(v Value) bool) Value {
	for _, v := range values {
		if take(v) {
			return v
		}
	}
	// If all empty, return the last element
	if len(values) > 0 {
		return values[len(values)-1]
	}
	var empty Value
	return empty
}

// takeFirst will take the first non-empty value.
func takeFirst[Value comparable](values ...Value) Value {
	var empty Value
	return takeFirstF(values, func(v Value) bool {
		return v != empty
	})
}

func newCryptoKeySecret(feature database.CryptoKeyFeature) (string, error) {
	switch feature {
	case database.CryptoKeyFeatureWorkspaceApps:
		return generateCryptoKey(32)
	case database.CryptoKeyFeatureOidcConvert:
		return generateCryptoKey(64)
	case database.CryptoKeyFeatureTailnetResume:
		return generateCryptoKey(64)
	}
	return "", xerrors.Errorf("unknown feature: %s", feature)
}

func generateCryptoKey(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", xerrors.Errorf("rand read: %w", err)
	}
	return hex.EncodeToString(b), nil
}
