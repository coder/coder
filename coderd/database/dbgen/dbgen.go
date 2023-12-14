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

// genCtx is to give all generator functions permission if the db is a dbauthz db.
var genCtx = dbauthz.As(context.Background(), rbac.Subject{
	ID:     "owner",
	Roles:  rbac.Roles(must(rbac.RoleNames{rbac.RoleOwner()}.Expand())),
	Groups: []string{},
	Scope:  rbac.ExpandableScope(rbac.ScopeAll),
})

func AuditLog(t testing.TB, db database.Store, seed database.AuditLog) database.AuditLog {
	log, err := db.InsertAuditLog(genCtx, database.InsertAuditLogParams{
		ID:             takeFirst(seed.ID, uuid.New()),
		Time:           takeFirst(seed.Time, dbtime.Now()),
		UserID:         takeFirst(seed.UserID, uuid.New()),
		OrganizationID: takeFirst(seed.OrganizationID, uuid.New()),
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
			seed.OrganizationID.String(): []rbac.Action{rbac.ActionRead},
		}
	}
	err := db.InsertTemplate(genCtx, database.InsertTemplateParams{
		ID:                           id,
		CreatedAt:                    takeFirst(seed.CreatedAt, dbtime.Now()),
		UpdatedAt:                    takeFirst(seed.UpdatedAt, dbtime.Now()),
		OrganizationID:               takeFirst(seed.OrganizationID, uuid.New()),
		Name:                         takeFirst(seed.Name, namesgenerator.GetRandomName(1)),
		Provisioner:                  takeFirst(seed.Provisioner, database.ProvisionerTypeEcho),
		ActiveVersionID:              takeFirst(seed.ActiveVersionID, uuid.New()),
		Description:                  takeFirst(seed.Description, namesgenerator.GetRandomName(1)),
		CreatedBy:                    takeFirst(seed.CreatedBy, uuid.New()),
		Icon:                         takeFirst(seed.Icon, namesgenerator.GetRandomName(1)),
		UserACL:                      seed.UserACL,
		GroupACL:                     seed.GroupACL,
		DisplayName:                  takeFirst(seed.DisplayName, namesgenerator.GetRandomName(1)),
		AllowUserCancelWorkspaceJobs: seed.AllowUserCancelWorkspaceJobs,
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

func WorkspaceAgent(t testing.TB, db database.Store, orig database.WorkspaceAgent) database.WorkspaceAgent {
	agt, err := db.InsertWorkspaceAgent(genCtx, database.InsertWorkspaceAgentParams{
		ID:         takeFirst(orig.ID, uuid.New()),
		CreatedAt:  takeFirst(orig.CreatedAt, dbtime.Now()),
		UpdatedAt:  takeFirst(orig.UpdatedAt, dbtime.Now()),
		Name:       takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		ResourceID: takeFirst(orig.ResourceID, uuid.New()),
		AuthToken:  takeFirst(orig.AuthToken, uuid.New()),
		AuthInstanceID: sql.NullString{
			String: takeFirst(orig.AuthInstanceID.String, namesgenerator.GetRandomName(1)),
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
		Name:              takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
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
		DisplayName:      []string{takeFirst(orig.DisplayName, namesgenerator.GetRandomName(1))},
		Icon:             []string{takeFirst(orig.Icon, namesgenerator.GetRandomName(1))},
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
		Email:          takeFirst(orig.Email, namesgenerator.GetRandomName(1)),
		Username:       takeFirst(orig.Username, namesgenerator.GetRandomName(1)),
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
		err = db.UpdateUserDeletedByID(genCtx, database.UpdateUserDeletedByIDParams{
			ID:      user.ID,
			Deleted: orig.Deleted,
		})
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
		Name:        takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		Description: takeFirst(orig.Description, namesgenerator.GetRandomName(1)),
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
	name := takeFirst(orig.Name, namesgenerator.GetRandomName(1))
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

func GroupMember(t testing.TB, db database.Store, orig database.GroupMember) database.GroupMember {
	member := database.GroupMember{
		UserID:  takeFirst(orig.UserID, uuid.New()),
		GroupID: takeFirst(orig.GroupID, uuid.New()),
	}
	//nolint:gosimple
	err := db.InsertGroupMember(genCtx, database.InsertGroupMemberParams{
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
		OrganizationID: takeFirst(orig.OrganizationID, uuid.New()),
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

func WorkspaceApp(t testing.TB, db database.Store, orig database.WorkspaceApp) database.WorkspaceApp {
	resource, err := db.InsertWorkspaceApp(genCtx, database.InsertWorkspaceAppParams{
		ID:          takeFirst(orig.ID, uuid.New()),
		CreatedAt:   takeFirst(orig.CreatedAt, dbtime.Now()),
		AgentID:     takeFirst(orig.AgentID, uuid.New()),
		Slug:        takeFirst(orig.Slug, namesgenerator.GetRandomName(1)),
		DisplayName: takeFirst(orig.DisplayName, namesgenerator.GetRandomName(1)),
		Icon:        takeFirst(orig.Icon, namesgenerator.GetRandomName(1)),
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
	})
	require.NoError(t, err, "insert app")
	return resource
}

func WorkspaceResource(t testing.TB, db database.Store, orig database.WorkspaceResource) database.WorkspaceResource {
	resource, err := db.InsertWorkspaceResource(genCtx, database.InsertWorkspaceResourceParams{
		ID:         takeFirst(orig.ID, uuid.New()),
		CreatedAt:  takeFirst(orig.CreatedAt, dbtime.Now()),
		JobID:      takeFirst(orig.JobID, uuid.New()),
		Transition: takeFirst(orig.Transition, database.WorkspaceTransitionStart),
		Type:       takeFirst(orig.Type, "fake_resource"),
		Name:       takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
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
		Key:                 []string{takeFirst(seed.Key, namesgenerator.GetRandomName(1))},
		Value:               []string{takeFirst(seed.Value.String, namesgenerator.GetRandomName(1))},
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
		Name:              takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		DisplayName:       takeFirst(orig.DisplayName, namesgenerator.GetRandomName(1)),
		Icon:              takeFirst(orig.Icon, namesgenerator.GetRandomName(1)),
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
			Name:           takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
			Message:        orig.Message,
			Readme:         takeFirst(orig.Readme, namesgenerator.GetRandomName(1)),
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
		Name:              takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		Description:       takeFirst(orig.Description, namesgenerator.GetRandomName(1)),
		Type:              takeFirst(orig.Type, "string"),
		Value:             takeFirst(orig.Value, ""),
		DefaultValue:      takeFirst(orig.DefaultValue, namesgenerator.GetRandomName(1)),
		Required:          takeFirst(orig.Required, false),
		Sensitive:         takeFirst(orig.Sensitive, false),
	})
	require.NoError(t, err, "insert template version variable")
	return version
}

func TemplateVersionParameter(t testing.TB, db database.Store, orig database.TemplateVersionParameter) database.TemplateVersionParameter {
	t.Helper()

	version, err := db.InsertTemplateVersionParameter(genCtx, database.InsertTemplateVersionParameterParams{
		TemplateVersionID:   takeFirst(orig.TemplateVersionID, uuid.New()),
		Name:                takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		Description:         takeFirst(orig.Description, namesgenerator.GetRandomName(1)),
		Type:                takeFirst(orig.Type, "string"),
		Mutable:             takeFirst(orig.Mutable, false),
		DefaultValue:        takeFirst(orig.DefaultValue, namesgenerator.GetRandomName(1)),
		Icon:                takeFirst(orig.Icon, namesgenerator.GetRandomName(1)),
		Options:             takeFirstSlice(orig.Options, []byte("[]")),
		ValidationRegex:     takeFirst(orig.ValidationRegex, ""),
		ValidationMin:       takeFirst(orig.ValidationMin, sql.NullInt32{}),
		ValidationMax:       takeFirst(orig.ValidationMax, sql.NullInt32{}),
		ValidationError:     takeFirst(orig.ValidationError, ""),
		ValidationMonotonic: takeFirst(orig.ValidationMonotonic, ""),
		Required:            takeFirst(orig.Required, false),
		DisplayName:         takeFirst(orig.DisplayName, namesgenerator.GetRandomName(1)),
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
	scheme, err := db.InsertWorkspaceAgentStat(genCtx, database.InsertWorkspaceAgentStatParams{
		ID:                          takeFirst(orig.ID, uuid.New()),
		CreatedAt:                   takeFirst(orig.CreatedAt, dbtime.Now()),
		UserID:                      takeFirst(orig.UserID, uuid.New()),
		TemplateID:                  takeFirst(orig.TemplateID, uuid.New()),
		WorkspaceID:                 takeFirst(orig.WorkspaceID, uuid.New()),
		AgentID:                     takeFirst(orig.AgentID, uuid.New()),
		ConnectionsByProto:          orig.ConnectionsByProto,
		ConnectionCount:             takeFirst(orig.ConnectionCount, 0),
		RxPackets:                   takeFirst(orig.RxPackets, 0),
		RxBytes:                     takeFirst(orig.RxBytes, 0),
		TxPackets:                   takeFirst(orig.TxPackets, 0),
		TxBytes:                     takeFirst(orig.TxBytes, 0),
		SessionCountVSCode:          takeFirst(orig.SessionCountVSCode, 0),
		SessionCountJetBrains:       takeFirst(orig.SessionCountJetBrains, 0),
		SessionCountReconnectingPTY: takeFirst(orig.SessionCountReconnectingPTY, 0),
		SessionCountSSH:             takeFirst(orig.SessionCountSSH, 0),
		ConnectionMedianLatencyMS:   takeFirst(orig.ConnectionMedianLatencyMS, 0),
	})
	require.NoError(t, err, "insert workspace agent stat")
	return scheme
}

func OAuth2ProviderApp(t testing.TB, db database.Store, seed database.OAuth2ProviderApp) database.OAuth2ProviderApp {
	app, err := db.InsertOAuth2ProviderApp(genCtx, database.InsertOAuth2ProviderAppParams{
		ID:          takeFirst(seed.ID, uuid.New()),
		Name:        takeFirst(seed.Name, namesgenerator.GetRandomName(1)),
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
		HashedSecret:  takeFirstSlice(seed.HashedSecret, []byte("hashed-secret")),
		DisplaySecret: takeFirst(seed.DisplaySecret, "secret"),
		AppID:         takeFirst(seed.AppID, uuid.New()),
	})
	require.NoError(t, err, "insert oauth2 app secret")
	return app
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
