package dbgen

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/cryptorand"
)

// All methods take in a 'seed' object. Any provided fields in the seed will be
// maintained. Any fields omitted will have sensible defaults generated.

// Ctx is to give all generator functions permission if the db is a dbauthz db.
var Ctx = dbauthz.As(context.Background(), rbac.Subject{
	ID:     "owner",
	Roles:  rbac.Roles(must(rbac.RoleNames{rbac.RoleOwner()}.Expand())),
	Groups: []string{},
	Scope:  rbac.ExpandableScope(rbac.ScopeAll),
})

func AuditLog(t testing.TB, db database.Store, seed database.AuditLog) database.AuditLog {
	log, err := db.InsertAuditLog(Ctx, database.InsertAuditLogParams{
		ID:             TakeFirst(seed.ID, uuid.New()),
		Time:           TakeFirst(seed.Time, dbtime.Now()),
		UserID:         TakeFirst(seed.UserID, uuid.New()),
		OrganizationID: TakeFirst(seed.OrganizationID, uuid.New()),
		Ip: pqtype.Inet{
			IPNet: takeFirstIP(seed.Ip.IPNet, net.IPNet{}),
			Valid: TakeFirst(seed.Ip.Valid, false),
		},
		UserAgent: sql.NullString{
			String: TakeFirst(seed.UserAgent.String, ""),
			Valid:  TakeFirst(seed.UserAgent.Valid, false),
		},
		ResourceType:     TakeFirst(seed.ResourceType, database.ResourceTypeOrganization),
		ResourceID:       TakeFirst(seed.ResourceID, uuid.New()),
		ResourceTarget:   TakeFirst(seed.ResourceTarget, uuid.NewString()),
		Action:           TakeFirst(seed.Action, database.AuditActionCreate),
		Diff:             TakeFirstSlice(seed.Diff, []byte("{}")),
		StatusCode:       TakeFirst(seed.StatusCode, 200),
		AdditionalFields: TakeFirstSlice(seed.Diff, []byte("{}")),
		RequestID:        TakeFirst(seed.RequestID, uuid.New()),
		ResourceIcon:     TakeFirst(seed.ResourceIcon, ""),
	})
	require.NoError(t, err, "insert audit log")
	return log
}

func Template(t testing.TB, db database.Store, seed database.Template) database.Template {
	id := TakeFirst(seed.ID, uuid.New())
	if seed.GroupACL == nil {
		// By default, all users in the organization can read the template.
		seed.GroupACL = database.TemplateACL{
			seed.OrganizationID.String(): []rbac.Action{rbac.ActionRead},
		}
	}
	err := db.InsertTemplate(Ctx, database.InsertTemplateParams{
		ID:                           id,
		CreatedAt:                    TakeFirst(seed.CreatedAt, dbtime.Now()),
		UpdatedAt:                    TakeFirst(seed.UpdatedAt, dbtime.Now()),
		OrganizationID:               TakeFirst(seed.OrganizationID, uuid.New()),
		Name:                         TakeFirst(seed.Name, namesgenerator.GetRandomName(1)),
		Provisioner:                  TakeFirst(seed.Provisioner, database.ProvisionerTypeEcho),
		ActiveVersionID:              TakeFirst(seed.ActiveVersionID, uuid.New()),
		Description:                  TakeFirst(seed.Description, namesgenerator.GetRandomName(1)),
		CreatedBy:                    TakeFirst(seed.CreatedBy, uuid.New()),
		Icon:                         TakeFirst(seed.Icon, namesgenerator.GetRandomName(1)),
		UserACL:                      seed.UserACL,
		GroupACL:                     seed.GroupACL,
		DisplayName:                  TakeFirst(seed.DisplayName, namesgenerator.GetRandomName(1)),
		AllowUserCancelWorkspaceJobs: seed.AllowUserCancelWorkspaceJobs,
	})
	require.NoError(t, err, "insert template")

	template, err := db.GetTemplateByID(Ctx, id)
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

	key, err := db.InsertAPIKey(Ctx, database.InsertAPIKeyParams{
		ID: TakeFirst(seed.ID, id),
		// 0 defaults to 86400 at the db layer
		LifetimeSeconds: TakeFirst(seed.LifetimeSeconds, 0),
		HashedSecret:    TakeFirstSlice(seed.HashedSecret, hashed[:]),
		IPAddress:       ip,
		UserID:          TakeFirst(seed.UserID, uuid.New()),
		LastUsed:        TakeFirst(seed.LastUsed, dbtime.Now()),
		ExpiresAt:       TakeFirst(seed.ExpiresAt, dbtime.Now().Add(time.Hour)),
		CreatedAt:       TakeFirst(seed.CreatedAt, dbtime.Now()),
		UpdatedAt:       TakeFirst(seed.UpdatedAt, dbtime.Now()),
		LoginType:       TakeFirst(seed.LoginType, database.LoginTypePassword),
		Scope:           TakeFirst(seed.Scope, database.APIKeyScopeAll),
		TokenName:       TakeFirst(seed.TokenName),
	})
	require.NoError(t, err, "insert api key")
	return key, fmt.Sprintf("%s-%s", key.ID, secret)
}

func WorkspaceAgent(t testing.TB, db database.Store, orig database.WorkspaceAgent) database.WorkspaceAgent {
	agt, err := db.InsertWorkspaceAgent(Ctx, database.InsertWorkspaceAgentParams{
		ID:         TakeFirst(orig.ID, uuid.New()),
		CreatedAt:  TakeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:  TakeFirst(orig.UpdatedAt, dbtime.Now()),
		Name:       TakeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		ResourceID: TakeFirst(orig.ResourceID, uuid.New()),
		AuthToken:  TakeFirst(orig.AuthToken, uuid.New()),
		AuthInstanceID: sql.NullString{
			String: TakeFirst(orig.AuthInstanceID.String, namesgenerator.GetRandomName(1)),
			Valid:  TakeFirst(orig.AuthInstanceID.Valid, true),
		},
		Architecture: TakeFirst(orig.Architecture, "amd64"),
		EnvironmentVariables: pqtype.NullRawMessage{
			RawMessage: TakeFirstSlice(orig.EnvironmentVariables.RawMessage, []byte("{}")),
			Valid:      TakeFirst(orig.EnvironmentVariables.Valid, false),
		},
		OperatingSystem: TakeFirst(orig.OperatingSystem, "linux"),
		Directory:       TakeFirst(orig.Directory, ""),
		InstanceMetadata: pqtype.NullRawMessage{
			RawMessage: TakeFirstSlice(orig.ResourceMetadata.RawMessage, []byte("{}")),
			Valid:      TakeFirst(orig.ResourceMetadata.Valid, false),
		},
		ResourceMetadata: pqtype.NullRawMessage{
			RawMessage: TakeFirstSlice(orig.ResourceMetadata.RawMessage, []byte("{}")),
			Valid:      TakeFirst(orig.ResourceMetadata.Valid, false),
		},
		ConnectionTimeoutSeconds: TakeFirst(orig.ConnectionTimeoutSeconds, 3600),
		TroubleshootingURL:       TakeFirst(orig.TroubleshootingURL, "https://example.com"),
		MOTDFile:                 TakeFirst(orig.TroubleshootingURL, ""),
		DisplayApps:              append([]database.DisplayApp{}, orig.DisplayApps...),
	})
	require.NoError(t, err, "insert workspace agent")
	return agt
}

func Workspace(t testing.TB, db database.Store, orig database.Workspace) database.Workspace {
	workspace, err := db.InsertWorkspace(Ctx, database.InsertWorkspaceParams{
		ID:                TakeFirst(orig.ID, uuid.New()),
		OwnerID:           TakeFirst(orig.OwnerID, uuid.New()),
		CreatedAt:         TakeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:         TakeFirst(orig.UpdatedAt, dbtime.Now()),
		OrganizationID:    TakeFirst(orig.OrganizationID, uuid.New()),
		TemplateID:        TakeFirst(orig.TemplateID, uuid.New()),
		LastUsedAt:        TakeFirst(orig.LastUsedAt, dbtime.Now()),
		Name:              TakeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		AutostartSchedule: orig.AutostartSchedule,
		Ttl:               orig.Ttl,
		AutomaticUpdates:  TakeFirst(orig.AutomaticUpdates, database.AutomaticUpdatesNever),
	})
	require.NoError(t, err, "insert workspace")
	return workspace
}

func WorkspaceAgentLogSource(t testing.TB, db database.Store, orig database.WorkspaceAgentLogSource) database.WorkspaceAgentLogSource {
	sources, err := db.InsertWorkspaceAgentLogSources(Ctx, database.InsertWorkspaceAgentLogSourcesParams{
		WorkspaceAgentID: TakeFirst(orig.WorkspaceAgentID, uuid.New()),
		ID:               []uuid.UUID{TakeFirst(orig.ID, uuid.New())},
		CreatedAt:        TakeFirst(orig.CreatedAt, dbtime.Now()),
		DisplayName:      []string{TakeFirst(orig.DisplayName, namesgenerator.GetRandomName(1))},
		Icon:             []string{TakeFirst(orig.Icon, namesgenerator.GetRandomName(1))},
	})
	require.NoError(t, err, "insert workspace agent log source")
	return sources[0]
}

func WorkspaceBuild(t testing.TB, db database.Store, orig database.WorkspaceBuild) database.WorkspaceBuild {
	buildID := TakeFirst(orig.ID, uuid.New())
	var build database.WorkspaceBuild
	err := db.InTx(func(db database.Store) error {
		err := db.InsertWorkspaceBuild(Ctx, database.InsertWorkspaceBuildParams{
			ID:                buildID,
			CreatedAt:         TakeFirst(orig.CreatedAt, dbtime.Now()),
			UpdatedAt:         TakeFirst(orig.UpdatedAt, dbtime.Now()),
			WorkspaceID:       TakeFirst(orig.WorkspaceID, uuid.New()),
			TemplateVersionID: TakeFirst(orig.TemplateVersionID, uuid.New()),
			BuildNumber:       TakeFirst(orig.BuildNumber, 1),
			Transition:        TakeFirst(orig.Transition, database.WorkspaceTransitionStart),
			InitiatorID:       TakeFirst(orig.InitiatorID, uuid.New()),
			JobID:             TakeFirst(orig.JobID, uuid.New()),
			ProvisionerState:  TakeFirstSlice(orig.ProvisionerState, []byte{}),
			Deadline:          TakeFirst(orig.Deadline, dbtime.Now().Add(time.Hour)),
			MaxDeadline:       TakeFirst(orig.MaxDeadline, time.Time{}),
			Reason:            TakeFirst(orig.Reason, database.BuildReasonInitiator),
		})
		if err != nil {
			return err
		}
		build, err = db.GetWorkspaceBuildByID(Ctx, buildID)
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
		id := TakeFirst(orig[0].WorkspaceBuildID, uuid.New())
		err := tx.InsertWorkspaceBuildParameters(Ctx, database.InsertWorkspaceBuildParametersParams{
			WorkspaceBuildID: id,
			Name:             names,
			Value:            values,
		})
		if err != nil {
			return err
		}

		params, err = tx.GetWorkspaceBuildParameters(Ctx, id)
		if err != nil {
			return err
		}
		return err
	}, nil)
	require.NoError(t, err)
	return params
}

func User(t testing.TB, db database.Store, orig database.User) database.User {
	user, err := db.InsertUser(Ctx, database.InsertUserParams{
		ID:             TakeFirst(orig.ID, uuid.New()),
		Email:          TakeFirst(orig.Email, namesgenerator.GetRandomName(1)),
		Username:       TakeFirst(orig.Username, namesgenerator.GetRandomName(1)),
		HashedPassword: TakeFirstSlice(orig.HashedPassword, []byte(must(cryptorand.String(32)))),
		CreatedAt:      TakeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:      TakeFirst(orig.UpdatedAt, dbtime.Now()),
		RBACRoles:      TakeFirstSlice(orig.RBACRoles, []string{}),
		LoginType:      TakeFirst(orig.LoginType, database.LoginTypePassword),
	})
	require.NoError(t, err, "insert user")

	user, err = db.UpdateUserStatus(Ctx, database.UpdateUserStatusParams{
		ID:        user.ID,
		Status:    TakeFirst(orig.Status, database.UserStatusActive),
		UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err, "insert user")

	if !orig.LastSeenAt.IsZero() {
		user, err = db.UpdateUserLastSeenAt(Ctx, database.UpdateUserLastSeenAtParams{
			ID:         user.ID,
			LastSeenAt: orig.LastSeenAt,
			UpdatedAt:  user.UpdatedAt,
		})
		require.NoError(t, err, "user last seen")
	}

	if orig.Deleted {
		err = db.UpdateUserDeletedByID(Ctx, database.UpdateUserDeletedByIDParams{
			ID:      user.ID,
			Deleted: orig.Deleted,
		})
		require.NoError(t, err, "set user as deleted")
	}
	return user
}

func GitSSHKey(t testing.TB, db database.Store, orig database.GitSSHKey) database.GitSSHKey {
	key, err := db.InsertGitSSHKey(Ctx, database.InsertGitSSHKeyParams{
		UserID:     TakeFirst(orig.UserID, uuid.New()),
		CreatedAt:  TakeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:  TakeFirst(orig.UpdatedAt, dbtime.Now()),
		PrivateKey: TakeFirst(orig.PrivateKey, ""),
		PublicKey:  TakeFirst(orig.PublicKey, ""),
	})
	require.NoError(t, err, "insert ssh key")
	return key
}

func Organization(t testing.TB, db database.Store, orig database.Organization) database.Organization {
	org, err := db.InsertOrganization(Ctx, database.InsertOrganizationParams{
		ID:          TakeFirst(orig.ID, uuid.New()),
		Name:        TakeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		Description: TakeFirst(orig.Description, namesgenerator.GetRandomName(1)),
		CreatedAt:   TakeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:   TakeFirst(orig.UpdatedAt, dbtime.Now()),
	})
	require.NoError(t, err, "insert organization")
	return org
}

func OrganizationMember(t testing.TB, db database.Store, orig database.OrganizationMember) database.OrganizationMember {
	mem, err := db.InsertOrganizationMember(Ctx, database.InsertOrganizationMemberParams{
		OrganizationID: TakeFirst(orig.OrganizationID, uuid.New()),
		UserID:         TakeFirst(orig.UserID, uuid.New()),
		CreatedAt:      TakeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:      TakeFirst(orig.UpdatedAt, dbtime.Now()),
		Roles:          TakeFirstSlice(orig.Roles, []string{}),
	})
	require.NoError(t, err, "insert organization")
	return mem
}

func Group(t testing.TB, db database.Store, orig database.Group) database.Group {
	name := TakeFirst(orig.Name, namesgenerator.GetRandomName(1))
	group, err := db.InsertGroup(Ctx, database.InsertGroupParams{
		ID:             TakeFirst(orig.ID, uuid.New()),
		Name:           name,
		DisplayName:    TakeFirst(orig.DisplayName, name),
		OrganizationID: TakeFirst(orig.OrganizationID, uuid.New()),
		AvatarURL:      TakeFirst(orig.AvatarURL, "https://logo.example.com"),
		QuotaAllowance: TakeFirst(orig.QuotaAllowance, 0),
	})
	require.NoError(t, err, "insert group")
	return group
}

func GroupMember(t testing.TB, db database.Store, orig database.GroupMember) database.GroupMember {
	member := database.GroupMember{
		UserID:  TakeFirst(orig.UserID, uuid.New()),
		GroupID: TakeFirst(orig.GroupID, uuid.New()),
	}
	//nolint:gosimple
	err := db.InsertGroupMember(Ctx, database.InsertGroupMemberParams{
		UserID:  member.UserID,
		GroupID: member.GroupID,
	})
	require.NoError(t, err, "insert group member")
	return member
}

// ProvisionerJob is a bit more involved to get the values such as "completedAt", "startedAt", "cancelledAt" set.  ps
// can be set to nil if you are SURE that you don't require a provisionerdaemon to acquire the job in your test.
func ProvisionerJob(t testing.TB, db database.Store, ps pubsub.Pubsub, orig database.ProvisionerJob) database.ProvisionerJob {
	t.Helper()

	jobID := TakeFirst(orig.ID, uuid.New())
	// Always set some tags to prevent Acquire from grabbing jobs it should not.
	if !orig.StartedAt.Time.IsZero() {
		if orig.Tags == nil {
			orig.Tags = make(database.StringMap)
		}
		// Make sure when we acquire the job, we only get this one.
		orig.Tags[jobID.String()] = "true"
	}

	job, err := db.InsertProvisionerJob(Ctx, database.InsertProvisionerJobParams{
		ID:             jobID,
		CreatedAt:      TakeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:      TakeFirst(orig.UpdatedAt, dbtime.Now()),
		OrganizationID: TakeFirst(orig.OrganizationID, uuid.New()),
		InitiatorID:    TakeFirst(orig.InitiatorID, uuid.New()),
		Provisioner:    TakeFirst(orig.Provisioner, database.ProvisionerTypeEcho),
		StorageMethod:  TakeFirst(orig.StorageMethod, database.ProvisionerStorageMethodFile),
		FileID:         TakeFirst(orig.FileID, uuid.New()),
		Type:           TakeFirst(orig.Type, database.ProvisionerJobTypeWorkspaceBuild),
		Input:          TakeFirstSlice(orig.Input, []byte("{}")),
		Tags:           orig.Tags,
		TraceMetadata:  pqtype.NullRawMessage{},
	})
	require.NoError(t, err, "insert job")
	if ps != nil {
		err = provisionerjobs.PostJob(ps, job)
		require.NoError(t, err, "post job to pubsub")
	}
	if !orig.StartedAt.Time.IsZero() {
		job, err = db.AcquireProvisionerJob(Ctx, database.AcquireProvisionerJobParams{
			StartedAt: orig.StartedAt,
			Types:     []database.ProvisionerType{database.ProvisionerTypeEcho},
			Tags:      must(json.Marshal(orig.Tags)),
			WorkerID:  uuid.NullUUID{},
		})
		require.NoError(t, err)
		// There is no easy way to make sure we acquire the correct job.
		require.Equal(t, jobID, job.ID, "acquired incorrect job")
	}

	if !orig.CompletedAt.Time.IsZero() || orig.Error.String != "" {
		err := db.UpdateProvisionerJobWithCompleteByID(Ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:          jobID,
			UpdatedAt:   job.UpdatedAt,
			CompletedAt: orig.CompletedAt,
			Error:       orig.Error,
			ErrorCode:   orig.ErrorCode,
		})
		require.NoError(t, err)
	}
	if !orig.CanceledAt.Time.IsZero() {
		err := db.UpdateProvisionerJobWithCancelByID(Ctx, database.UpdateProvisionerJobWithCancelByIDParams{
			ID:          jobID,
			CanceledAt:  orig.CanceledAt,
			CompletedAt: orig.CompletedAt,
		})
		require.NoError(t, err)
	}

	job, err = db.GetProvisionerJobByID(Ctx, jobID)
	require.NoError(t, err)

	return job
}

func WorkspaceApp(t testing.TB, db database.Store, orig database.WorkspaceApp) database.WorkspaceApp {
	resource, err := db.InsertWorkspaceApp(Ctx, database.InsertWorkspaceAppParams{
		ID:          TakeFirst(orig.ID, uuid.New()),
		CreatedAt:   TakeFirst(orig.CreatedAt, dbtime.Now()),
		AgentID:     TakeFirst(orig.AgentID, uuid.New()),
		Slug:        TakeFirst(orig.Slug, namesgenerator.GetRandomName(1)),
		DisplayName: TakeFirst(orig.DisplayName, namesgenerator.GetRandomName(1)),
		Icon:        TakeFirst(orig.Icon, namesgenerator.GetRandomName(1)),
		Command: sql.NullString{
			String: TakeFirst(orig.Command.String, "ls"),
			Valid:  orig.Command.Valid,
		},
		Url: sql.NullString{
			String: TakeFirst(orig.Url.String),
			Valid:  orig.Url.Valid,
		},
		External:             orig.External,
		Subdomain:            orig.Subdomain,
		SharingLevel:         TakeFirst(orig.SharingLevel, database.AppSharingLevelOwner),
		HealthcheckUrl:       TakeFirst(orig.HealthcheckUrl, "https://localhost:8000"),
		HealthcheckInterval:  TakeFirst(orig.HealthcheckInterval, 60),
		HealthcheckThreshold: TakeFirst(orig.HealthcheckThreshold, 60),
		Health:               TakeFirst(orig.Health, database.WorkspaceAppHealthHealthy),
	})
	require.NoError(t, err, "insert app")
	return resource
}

func WorkspaceResource(t testing.TB, db database.Store, orig database.WorkspaceResource) database.WorkspaceResource {
	resource, err := db.InsertWorkspaceResource(Ctx, database.InsertWorkspaceResourceParams{
		ID:         TakeFirst(orig.ID, uuid.New()),
		CreatedAt:  TakeFirst(orig.CreatedAt, dbtime.Now()),
		JobID:      TakeFirst(orig.JobID, uuid.New()),
		Transition: TakeFirst(orig.Transition, database.WorkspaceTransitionStart),
		Type:       TakeFirst(orig.Type, "fake_resource"),
		Name:       TakeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		Hide:       TakeFirst(orig.Hide, false),
		Icon:       TakeFirst(orig.Icon, ""),
		InstanceType: sql.NullString{
			String: TakeFirst(orig.InstanceType.String, ""),
			Valid:  TakeFirst(orig.InstanceType.Valid, false),
		},
		DailyCost: TakeFirst(orig.DailyCost, 0),
	})
	require.NoError(t, err, "insert resource")
	return resource
}

func WorkspaceResourceMetadatums(t testing.TB, db database.Store, seed database.WorkspaceResourceMetadatum) []database.WorkspaceResourceMetadatum {
	meta, err := db.InsertWorkspaceResourceMetadata(Ctx, database.InsertWorkspaceResourceMetadataParams{
		WorkspaceResourceID: TakeFirst(seed.WorkspaceResourceID, uuid.New()),
		Key:                 []string{TakeFirst(seed.Key, namesgenerator.GetRandomName(1))},
		Value:               []string{TakeFirst(seed.Value.String, namesgenerator.GetRandomName(1))},
		Sensitive:           []bool{TakeFirst(seed.Sensitive, false)},
	})
	require.NoError(t, err, "insert meta data")
	return meta
}

func WorkspaceProxy(t testing.TB, db database.Store, orig database.WorkspaceProxy) (database.WorkspaceProxy, string) {
	secret, err := cryptorand.HexString(64)
	require.NoError(t, err, "generate secret")
	hashedSecret := sha256.Sum256([]byte(secret))

	proxy, err := db.InsertWorkspaceProxy(Ctx, database.InsertWorkspaceProxyParams{
		ID:                TakeFirst(orig.ID, uuid.New()),
		Name:              TakeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		DisplayName:       TakeFirst(orig.DisplayName, namesgenerator.GetRandomName(1)),
		Icon:              TakeFirst(orig.Icon, namesgenerator.GetRandomName(1)),
		TokenHashedSecret: hashedSecret[:],
		CreatedAt:         TakeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:         TakeFirst(orig.UpdatedAt, dbtime.Now()),
		DerpEnabled:       TakeFirst(orig.DerpEnabled, false),
		DerpOnly:          TakeFirst(orig.DerpEnabled, false),
	})
	require.NoError(t, err, "insert proxy")

	// Also set these fields if the caller wants them.
	if orig.Url != "" || orig.WildcardHostname != "" {
		proxy, err = db.RegisterWorkspaceProxy(Ctx, database.RegisterWorkspaceProxyParams{
			Url:              orig.Url,
			WildcardHostname: orig.WildcardHostname,
			ID:               proxy.ID,
		})
		require.NoError(t, err, "update proxy")
	}
	return proxy, secret
}

func File(t testing.TB, db database.Store, orig database.File) database.File {
	file, err := db.InsertFile(Ctx, database.InsertFileParams{
		ID:        TakeFirst(orig.ID, uuid.New()),
		Hash:      TakeFirst(orig.Hash, hex.EncodeToString(make([]byte, 32))),
		CreatedAt: TakeFirst(orig.CreatedAt, dbtime.Now()),
		CreatedBy: TakeFirst(orig.CreatedBy, uuid.New()),
		Mimetype:  TakeFirst(orig.Mimetype, "application/x-tar"),
		Data:      TakeFirstSlice(orig.Data, []byte{}),
	})
	require.NoError(t, err, "insert file")
	return file
}

func UserLink(t testing.TB, db database.Store, orig database.UserLink) database.UserLink {
	link, err := db.InsertUserLink(Ctx, database.InsertUserLinkParams{
		UserID:                 TakeFirst(orig.UserID, uuid.New()),
		LoginType:              TakeFirst(orig.LoginType, database.LoginTypeGithub),
		LinkedID:               TakeFirst(orig.LinkedID),
		OAuthAccessToken:       TakeFirst(orig.OAuthAccessToken, uuid.NewString()),
		OAuthAccessTokenKeyID:  TakeFirst(orig.OAuthAccessTokenKeyID, sql.NullString{}),
		OAuthRefreshToken:      TakeFirst(orig.OAuthRefreshToken, uuid.NewString()),
		OAuthRefreshTokenKeyID: TakeFirst(orig.OAuthRefreshTokenKeyID, sql.NullString{}),
		OAuthExpiry:            TakeFirst(orig.OAuthExpiry, dbtime.Now().Add(time.Hour*24)),
		DebugContext:           TakeFirstSlice(orig.DebugContext, json.RawMessage("{}")),
	})

	require.NoError(t, err, "insert link")
	return link
}

func ExternalAuthLink(t testing.TB, db database.Store, orig database.ExternalAuthLink) database.ExternalAuthLink {
	msg := TakeFirst(&orig.OAuthExtra, &pqtype.NullRawMessage{})
	link, err := db.InsertExternalAuthLink(Ctx, database.InsertExternalAuthLinkParams{
		ProviderID:             TakeFirst(orig.ProviderID, uuid.New().String()),
		UserID:                 TakeFirst(orig.UserID, uuid.New()),
		OAuthAccessToken:       TakeFirst(orig.OAuthAccessToken, uuid.NewString()),
		OAuthAccessTokenKeyID:  TakeFirst(orig.OAuthAccessTokenKeyID, sql.NullString{}),
		OAuthRefreshToken:      TakeFirst(orig.OAuthRefreshToken, uuid.NewString()),
		OAuthRefreshTokenKeyID: TakeFirst(orig.OAuthRefreshTokenKeyID, sql.NullString{}),
		OAuthExpiry:            TakeFirst(orig.OAuthExpiry, dbtime.Now().Add(time.Hour*24)),
		CreatedAt:              TakeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:              TakeFirst(orig.UpdatedAt, dbtime.Now()),
		OAuthExtra:             *msg,
	})

	require.NoError(t, err, "insert external auth link")
	return link
}

func TemplateVersion(t testing.TB, db database.Store, orig database.TemplateVersion) database.TemplateVersion {
	var version database.TemplateVersion
	err := db.InTx(func(db database.Store) error {
		versionID := TakeFirst(orig.ID, uuid.New())
		err := db.InsertTemplateVersion(Ctx, database.InsertTemplateVersionParams{
			ID:             versionID,
			TemplateID:     TakeFirst(orig.TemplateID, uuid.NullUUID{}),
			OrganizationID: TakeFirst(orig.OrganizationID, uuid.New()),
			CreatedAt:      TakeFirst(orig.CreatedAt, dbtime.Now()),
			UpdatedAt:      TakeFirst(orig.UpdatedAt, dbtime.Now()),
			Name:           TakeFirst(orig.Name, namesgenerator.GetRandomName(1)),
			Message:        orig.Message,
			Readme:         TakeFirst(orig.Readme, namesgenerator.GetRandomName(1)),
			JobID:          TakeFirst(orig.JobID, uuid.New()),
			CreatedBy:      TakeFirst(orig.CreatedBy, uuid.New()),
		})
		if err != nil {
			return err
		}

		version, err = db.GetTemplateVersionByID(Ctx, versionID)
		if err != nil {
			return err
		}
		return nil
	}, nil)
	require.NoError(t, err, "insert template version")

	return version
}

func TemplateVersionVariable(t testing.TB, db database.Store, orig database.TemplateVersionVariable) database.TemplateVersionVariable {
	version, err := db.InsertTemplateVersionVariable(Ctx, database.InsertTemplateVersionVariableParams{
		TemplateVersionID: TakeFirst(orig.TemplateVersionID, uuid.New()),
		Name:              TakeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		Description:       TakeFirst(orig.Description, namesgenerator.GetRandomName(1)),
		Type:              TakeFirst(orig.Type, "string"),
		Value:             TakeFirst(orig.Value, ""),
		DefaultValue:      TakeFirst(orig.DefaultValue, namesgenerator.GetRandomName(1)),
		Required:          TakeFirst(orig.Required, false),
		Sensitive:         TakeFirst(orig.Sensitive, false),
	})
	require.NoError(t, err, "insert template version variable")
	return version
}

func TemplateVersionParameter(t testing.TB, db database.Store, orig database.TemplateVersionParameter) database.TemplateVersionParameter {
	t.Helper()

	version, err := db.InsertTemplateVersionParameter(Ctx, database.InsertTemplateVersionParameterParams{
		TemplateVersionID:   TakeFirst(orig.TemplateVersionID, uuid.New()),
		Name:                TakeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		Description:         TakeFirst(orig.Description, namesgenerator.GetRandomName(1)),
		Type:                TakeFirst(orig.Type, "string"),
		Mutable:             TakeFirst(orig.Mutable, false),
		DefaultValue:        TakeFirst(orig.DefaultValue, namesgenerator.GetRandomName(1)),
		Icon:                TakeFirst(orig.Icon, namesgenerator.GetRandomName(1)),
		Options:             TakeFirstSlice(orig.Options, []byte("[]")),
		ValidationRegex:     TakeFirst(orig.ValidationRegex, ""),
		ValidationMin:       TakeFirst(orig.ValidationMin, sql.NullInt32{}),
		ValidationMax:       TakeFirst(orig.ValidationMax, sql.NullInt32{}),
		ValidationError:     TakeFirst(orig.ValidationError, ""),
		ValidationMonotonic: TakeFirst(orig.ValidationMonotonic, ""),
		Required:            TakeFirst(orig.Required, false),
		DisplayName:         TakeFirst(orig.DisplayName, namesgenerator.GetRandomName(1)),
		DisplayOrder:        TakeFirst(orig.DisplayOrder, 0),
		Ephemeral:           TakeFirst(orig.Ephemeral, false),
	})
	require.NoError(t, err, "insert template version parameter")
	return version
}

func WorkspaceAgentStat(t testing.TB, db database.Store, orig database.WorkspaceAgentStat) database.WorkspaceAgentStat {
	if orig.ConnectionsByProto == nil {
		orig.ConnectionsByProto = json.RawMessage([]byte("{}"))
	}
	scheme, err := db.InsertWorkspaceAgentStat(Ctx, database.InsertWorkspaceAgentStatParams{
		ID:                          TakeFirst(orig.ID, uuid.New()),
		CreatedAt:                   TakeFirst(orig.CreatedAt, dbtime.Now()),
		UserID:                      TakeFirst(orig.UserID, uuid.New()),
		TemplateID:                  TakeFirst(orig.TemplateID, uuid.New()),
		WorkspaceID:                 TakeFirst(orig.WorkspaceID, uuid.New()),
		AgentID:                     TakeFirst(orig.AgentID, uuid.New()),
		ConnectionsByProto:          orig.ConnectionsByProto,
		ConnectionCount:             TakeFirst(orig.ConnectionCount, 0),
		RxPackets:                   TakeFirst(orig.RxPackets, 0),
		RxBytes:                     TakeFirst(orig.RxBytes, 0),
		TxPackets:                   TakeFirst(orig.TxPackets, 0),
		TxBytes:                     TakeFirst(orig.TxBytes, 0),
		SessionCountVSCode:          TakeFirst(orig.SessionCountVSCode, 0),
		SessionCountJetBrains:       TakeFirst(orig.SessionCountJetBrains, 0),
		SessionCountReconnectingPTY: TakeFirst(orig.SessionCountReconnectingPTY, 0),
		SessionCountSSH:             TakeFirst(orig.SessionCountSSH, 0),
		ConnectionMedianLatencyMS:   TakeFirst(orig.ConnectionMedianLatencyMS, 0),
	})
	require.NoError(t, err, "insert workspace agent stat")
	return scheme
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

// TakeFirstSlice implements takeFirst for []any.
// []any is not a comparable type.
func TakeFirstSlice[T any](values ...[]T) []T {
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

// TakeFirst will take the first non-empty value.
func TakeFirst[Value comparable](values ...Value) Value {
	var empty Value
	return takeFirstF(values, func(v Value) bool {
		return v != empty
	})
}
