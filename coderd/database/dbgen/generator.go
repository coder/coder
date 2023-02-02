package dbgen

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/require"
	"github.com/tabbed/pqtype"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/cryptorand"
)

// All methods take in a 'seed' object. Any provided fields in the seed will be
// maintained. Any fields omitted will have sensible defaults generated.

func AuditLog(t *testing.T, db database.Store, seed database.AuditLog) database.AuditLog {
	log, err := db.InsertAuditLog(context.Background(), database.InsertAuditLogParams{
		ID:             takeFirst(seed.ID, uuid.New()),
		Time:           takeFirst(seed.Time, time.Now()),
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
		Diff:             takeFirstBytes(seed.Diff, []byte("{}")),
		StatusCode:       takeFirst(seed.StatusCode, 200),
		AdditionalFields: takeFirstBytes(seed.Diff, []byte("{}")),
		RequestID:        takeFirst(seed.RequestID, uuid.New()),
		ResourceIcon:     takeFirst(seed.ResourceIcon, ""),
	})
	require.NoError(t, err, "insert audit log")
	return log
}

func Template(t *testing.T, db database.Store, seed database.Template) database.Template {
	template, err := db.InsertTemplate(context.Background(), database.InsertTemplateParams{
		ID:                           takeFirst(seed.ID, uuid.New()),
		CreatedAt:                    takeFirst(seed.CreatedAt, time.Now()),
		UpdatedAt:                    takeFirst(seed.UpdatedAt, time.Now()),
		OrganizationID:               takeFirst(seed.OrganizationID, uuid.New()),
		Name:                         takeFirst(seed.Name, namesgenerator.GetRandomName(1)),
		Provisioner:                  takeFirst(seed.Provisioner, database.ProvisionerTypeEcho),
		ActiveVersionID:              takeFirst(seed.ActiveVersionID, uuid.New()),
		Description:                  takeFirst(seed.Description, namesgenerator.GetRandomName(1)),
		DefaultTTL:                   takeFirst(seed.DefaultTTL, 3600),
		CreatedBy:                    takeFirst(seed.CreatedBy, uuid.New()),
		Icon:                         takeFirst(seed.Icon, namesgenerator.GetRandomName(1)),
		UserACL:                      seed.UserACL,
		GroupACL:                     seed.GroupACL,
		DisplayName:                  takeFirst(seed.DisplayName, namesgenerator.GetRandomName(1)),
		AllowUserCancelWorkspaceJobs: takeFirst(seed.AllowUserCancelWorkspaceJobs, true),
	})
	require.NoError(t, err, "insert template")
	return template
}

func APIKey(t *testing.T, db database.Store, seed database.APIKey) (key database.APIKey, token string) {
	id, _ := cryptorand.String(10)
	secret, _ := cryptorand.String(22)
	hashed := sha256.Sum256([]byte(secret))

	key, err := db.InsertAPIKey(context.Background(), database.InsertAPIKeyParams{
		ID: takeFirst(seed.ID, id),
		// 0 defaults to 86400 at the db layer
		LifetimeSeconds: takeFirst(seed.LifetimeSeconds, 0),
		HashedSecret:    takeFirstBytes(seed.HashedSecret, hashed[:]),
		IPAddress:       pqtype.Inet{},
		UserID:          takeFirst(seed.UserID, uuid.New()),
		LastUsed:        takeFirst(seed.LastUsed, time.Now()),
		ExpiresAt:       takeFirst(seed.ExpiresAt, time.Now().Add(time.Hour)),
		CreatedAt:       takeFirst(seed.CreatedAt, time.Now()),
		UpdatedAt:       takeFirst(seed.UpdatedAt, time.Now()),
		LoginType:       takeFirst(seed.LoginType, database.LoginTypePassword),
		Scope:           takeFirst(seed.Scope, database.APIKeyScopeAll),
	})
	require.NoError(t, err, "insert api key")
	return key, fmt.Sprintf("%s-%s", key.ID, secret)
}

func WorkspaceAgent(t *testing.T, db database.Store, orig database.WorkspaceAgent) database.WorkspaceAgent {
	workspace, err := db.InsertWorkspaceAgent(context.Background(), database.InsertWorkspaceAgentParams{
		ID:         takeFirst(orig.ID, uuid.New()),
		CreatedAt:  takeFirst(orig.CreatedAt, time.Now()),
		UpdatedAt:  takeFirst(orig.UpdatedAt, time.Now()),
		Name:       takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		ResourceID: takeFirst(orig.ResourceID, uuid.New()),
		AuthToken:  takeFirst(orig.AuthToken, uuid.New()),
		AuthInstanceID: sql.NullString{
			String: takeFirst(orig.AuthInstanceID.String, namesgenerator.GetRandomName(1)),
			Valid:  takeFirst(orig.AuthInstanceID.Valid, true),
		},
		Architecture: takeFirst(orig.Architecture, "amd64"),
		EnvironmentVariables: pqtype.NullRawMessage{
			RawMessage: takeFirstBytes(orig.EnvironmentVariables.RawMessage, []byte("{}")),
			Valid:      takeFirst(orig.EnvironmentVariables.Valid, false),
		},
		OperatingSystem: takeFirst(orig.OperatingSystem, "linux"),
		StartupScript: sql.NullString{
			String: takeFirst(orig.StartupScript.String, ""),
			Valid:  takeFirst(orig.StartupScript.Valid, false),
		},
		Directory: takeFirst(orig.Directory, ""),
		InstanceMetadata: pqtype.NullRawMessage{
			RawMessage: takeFirstBytes(orig.ResourceMetadata.RawMessage, []byte("{}")),
			Valid:      takeFirst(orig.ResourceMetadata.Valid, false),
		},
		ResourceMetadata: pqtype.NullRawMessage{
			RawMessage: takeFirstBytes(orig.ResourceMetadata.RawMessage, []byte("{}")),
			Valid:      takeFirst(orig.ResourceMetadata.Valid, false),
		},
		ConnectionTimeoutSeconds:    takeFirst(orig.ConnectionTimeoutSeconds, 3600),
		TroubleshootingURL:          takeFirst(orig.TroubleshootingURL, "https://example.com"),
		MOTDFile:                    takeFirst(orig.TroubleshootingURL, ""),
		LoginBeforeReady:            takeFirst(orig.LoginBeforeReady, false),
		StartupScriptTimeoutSeconds: takeFirst(orig.StartupScriptTimeoutSeconds, 3600),
	})
	require.NoError(t, err, "insert workspace agent")
	return workspace
}

func Workspace(t *testing.T, db database.Store, orig database.Workspace) database.Workspace {
	workspace, err := db.InsertWorkspace(context.Background(), database.InsertWorkspaceParams{
		ID:                takeFirst(orig.ID, uuid.New()),
		OwnerID:           takeFirst(orig.OwnerID, uuid.New()),
		CreatedAt:         takeFirst(orig.CreatedAt, time.Now()),
		UpdatedAt:         takeFirst(orig.UpdatedAt, time.Now()),
		OrganizationID:    takeFirst(orig.OrganizationID, uuid.New()),
		TemplateID:        takeFirst(orig.TemplateID, uuid.New()),
		Name:              takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		AutostartSchedule: orig.AutostartSchedule,
		Ttl:               orig.Ttl,
	})
	require.NoError(t, err, "insert workspace")
	return workspace
}

func WorkspaceBuild(t *testing.T, db database.Store, orig database.WorkspaceBuild) database.WorkspaceBuild {
	build, err := db.InsertWorkspaceBuild(context.Background(), database.InsertWorkspaceBuildParams{
		ID:                takeFirst(orig.ID, uuid.New()),
		CreatedAt:         takeFirst(orig.CreatedAt, time.Now()),
		UpdatedAt:         takeFirst(orig.UpdatedAt, time.Now()),
		WorkspaceID:       takeFirst(orig.WorkspaceID, uuid.New()),
		TemplateVersionID: takeFirst(orig.TemplateVersionID, uuid.New()),
		BuildNumber:       takeFirst(orig.BuildNumber, 1),
		Transition:        takeFirst(orig.Transition, database.WorkspaceTransitionStart),
		InitiatorID:       takeFirst(orig.InitiatorID, uuid.New()),
		JobID:             takeFirst(orig.JobID, uuid.New()),
		ProvisionerState:  takeFirstBytes(orig.ProvisionerState, []byte{}),
		Deadline:          takeFirst(orig.Deadline, time.Now().Add(time.Hour)),
		Reason:            takeFirst(orig.Reason, database.BuildReasonInitiator),
	})
	require.NoError(t, err, "insert workspace build")
	return build
}

func User(t *testing.T, db database.Store, orig database.User) database.User {
	user, err := db.InsertUser(context.Background(), database.InsertUserParams{
		ID:             takeFirst(orig.ID, uuid.New()),
		Email:          takeFirst(orig.Email, namesgenerator.GetRandomName(1)),
		Username:       takeFirst(orig.Username, namesgenerator.GetRandomName(1)),
		HashedPassword: takeFirstBytes(orig.HashedPassword, []byte{}),
		CreatedAt:      takeFirst(orig.CreatedAt, time.Now()),
		UpdatedAt:      takeFirst(orig.UpdatedAt, time.Now()),
		RBACRoles:      []string{},
		LoginType:      takeFirst(orig.LoginType, database.LoginTypePassword),
	})
	require.NoError(t, err, "insert user")
	return user
}

func Organization(t *testing.T, db database.Store, orig database.Organization) database.Organization {
	org, err := db.InsertOrganization(context.Background(), database.InsertOrganizationParams{
		ID:          takeFirst(orig.ID, uuid.New()),
		Name:        takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		Description: takeFirst(orig.Description, namesgenerator.GetRandomName(1)),
		CreatedAt:   takeFirst(orig.CreatedAt, time.Now()),
		UpdatedAt:   takeFirst(orig.UpdatedAt, time.Now()),
	})
	require.NoError(t, err, "insert organization")
	return org
}

func Group(t *testing.T, db database.Store, orig database.Group) database.Group {
	group, err := db.InsertGroup(context.Background(), database.InsertGroupParams{
		ID:             takeFirst(orig.ID, uuid.New()),
		Name:           takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		OrganizationID: takeFirst(orig.OrganizationID, uuid.New()),
		AvatarURL:      takeFirst(orig.AvatarURL, "https://logo.example.com"),
		QuotaAllowance: takeFirst(orig.QuotaAllowance, 0),
	})
	require.NoError(t, err, "insert group")
	return group
}

func ProvisionerJob(t *testing.T, db database.Store, orig database.ProvisionerJob) database.ProvisionerJob {
	job, err := db.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
		ID:             takeFirst(orig.ID, uuid.New()),
		CreatedAt:      takeFirst(orig.CreatedAt, time.Now()),
		UpdatedAt:      takeFirst(orig.UpdatedAt, time.Now()),
		OrganizationID: takeFirst(orig.OrganizationID, uuid.New()),
		InitiatorID:    takeFirst(orig.InitiatorID, uuid.New()),
		Provisioner:    takeFirst(orig.Provisioner, database.ProvisionerTypeEcho),
		StorageMethod:  takeFirst(orig.StorageMethod, database.ProvisionerStorageMethodFile),
		FileID:         takeFirst(orig.FileID, uuid.New()),
		Type:           takeFirst(orig.Type, database.ProvisionerJobTypeWorkspaceBuild),
		Input:          takeFirstBytes(orig.Input, []byte("{}")),
		Tags:           orig.Tags,
	})
	require.NoError(t, err, "insert job")
	return job
}

func WorkspaceResource(t *testing.T, db database.Store, orig database.WorkspaceResource) database.WorkspaceResource {
	resource, err := db.InsertWorkspaceResource(context.Background(), database.InsertWorkspaceResourceParams{
		ID:         takeFirst(orig.ID, uuid.New()),
		CreatedAt:  takeFirst(orig.CreatedAt, time.Now()),
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

func File(t *testing.T, db database.Store, orig database.File) database.File {
	file, err := db.InsertFile(context.Background(), database.InsertFileParams{
		ID:        takeFirst(orig.ID, uuid.New()),
		Hash:      takeFirst(orig.Hash, hex.EncodeToString(make([]byte, 32))),
		CreatedAt: takeFirst(orig.CreatedAt, time.Now()),
		CreatedBy: takeFirst(orig.CreatedBy, uuid.New()),
		Mimetype:  takeFirst(orig.Mimetype, "application/x-tar"),
		Data:      takeFirstBytes(orig.Data, []byte{}),
	})
	require.NoError(t, err, "insert file")
	return file
}

func UserLink(t *testing.T, db database.Store, orig database.UserLink) database.UserLink {
	link, err := db.InsertUserLink(context.Background(), database.InsertUserLinkParams{
		UserID:            takeFirst(orig.UserID, uuid.New()),
		LoginType:         takeFirst(orig.LoginType, database.LoginTypeGithub),
		LinkedID:          takeFirst(orig.LinkedID),
		OAuthAccessToken:  takeFirst(orig.OAuthAccessToken, uuid.NewString()),
		OAuthRefreshToken: takeFirst(orig.OAuthAccessToken, uuid.NewString()),
		OAuthExpiry:       takeFirst(orig.OAuthExpiry, time.Now().Add(time.Hour*24)),
	})

	require.NoError(t, err, "insert link")
	return link
}

func TemplateVersion(t *testing.T, db database.Store, orig database.TemplateVersion) database.TemplateVersion {
	version, err := db.InsertTemplateVersion(context.Background(), database.InsertTemplateVersionParams{
		ID: takeFirst(orig.ID, uuid.New()),
		TemplateID: uuid.NullUUID{
			UUID:  takeFirst(orig.TemplateID.UUID, uuid.New()),
			Valid: takeFirst(orig.TemplateID.Valid, true),
		},
		OrganizationID: takeFirst(orig.OrganizationID, uuid.New()),
		CreatedAt:      takeFirst(orig.CreatedAt, time.Now()),
		UpdatedAt:      takeFirst(orig.UpdatedAt, time.Now()),
		Name:           takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
		Readme:         takeFirst(orig.Readme, namesgenerator.GetRandomName(1)),
		JobID:          takeFirst(orig.JobID, uuid.New()),
		CreatedBy:      takeFirst(orig.CreatedBy, uuid.New()),
	})
	require.NoError(t, err, "insert template version")
	return version
}
