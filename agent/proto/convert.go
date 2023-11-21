package proto

import (
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
)

func DBAgentMetadataToProtoDescription(metadata []database.WorkspaceAgentMetadatum) []*WorkspaceAgentMetadata_Description {
	ret := make([]*WorkspaceAgentMetadata_Description, len(metadata))
	for i, metadatum := range metadata {
		ret[i] = DBAgentMetadatumToProtoDescription(metadatum)
	}
	return ret
}

func DBAgentMetadatumToProtoDescription(metadatum database.WorkspaceAgentMetadatum) *WorkspaceAgentMetadata_Description {
	return &WorkspaceAgentMetadata_Description{
		DisplayName: metadatum.DisplayName,
		Key:         metadatum.Key,
		Script:      metadatum.Script,
		Interval:    durationpb.New(time.Duration(metadatum.Interval)),
		Timeout:     durationpb.New(time.Duration(metadatum.Timeout)),
	}
}

func DBAgentScriptsToProto(scripts []database.WorkspaceAgentScript) []*WorkspaceAgentScript {
	ret := make([]*WorkspaceAgentScript, len(scripts))
	for i, script := range scripts {
		ret[i] = DBAgentScriptToProto(script)
	}
	return ret
}

func DBAgentScriptToProto(script database.WorkspaceAgentScript) *WorkspaceAgentScript {
	return &WorkspaceAgentScript{
		LogSourceId:      script.LogSourceID[:],
		LogPath:          script.LogPath,
		Script:           script.Script,
		Cron:             script.Cron,
		RunOnStart:       script.RunOnStart,
		RunOnStop:        script.RunOnStop,
		StartBlocksLogin: script.StartBlocksLogin,
		Timeout:          durationpb.New(time.Duration(script.TimeoutSeconds) * time.Second),
	}
}

func DBAppsToProto(dbApps []database.WorkspaceApp, agent database.WorkspaceAgent, ownerName string, workspace database.Workspace) []*WorkspaceApp {
	ret := make([]*WorkspaceApp, len(dbApps))
	for i, dbApp := range dbApps {
		ret[i] = DBAppToProto(dbApp, agent, ownerName, workspace)
	}
	return ret
}

func DBAppToProto(dbApp database.WorkspaceApp, agent database.WorkspaceAgent, ownerName string, workspace database.Workspace) *WorkspaceApp {
	var subdomainName string
	if dbApp.Subdomain && agent.Name != "" && ownerName != "" && workspace.Name != "" {
		appSlug := dbApp.Slug
		if appSlug == "" {
			appSlug = dbApp.DisplayName
		}
		subdomainName = httpapi.ApplicationURL{
			// We never generate URLs with a prefix. We only allow prefixes
			// when parsing URLs from the hostname. Users that want this
			// feature can write out their own URLs.
			Prefix:        "",
			AppSlugOrPort: appSlug,
			AgentName:     agent.Name,
			WorkspaceName: workspace.Name,
			Username:      ownerName,
		}.String()
	}

	sharingLevel := WorkspaceApp_SHARING_LEVEL_UNSPECIFIED
	switch dbApp.SharingLevel {
	case database.AppSharingLevelOwner:
		sharingLevel = WorkspaceApp_OWNER
	case database.AppSharingLevelAuthenticated:
		sharingLevel = WorkspaceApp_AUTHENTICATED
	case database.AppSharingLevelPublic:
		sharingLevel = WorkspaceApp_PUBLIC
	}

	health := WorkspaceApp_HEALTH_UNSPECIFIED
	switch dbApp.Health {
	case database.WorkspaceAppHealthDisabled:
		health = WorkspaceApp_DISABLED
	case database.WorkspaceAppHealthInitializing:
		health = WorkspaceApp_INITIALIZING
	case database.WorkspaceAppHealthHealthy:
		health = WorkspaceApp_HEALTHY
	case database.WorkspaceAppHealthUnhealthy:
		health = WorkspaceApp_UNHEALTHY
	}

	return &WorkspaceApp{
		Id:            dbApp.ID[:],
		Url:           dbApp.Url.String,
		External:      dbApp.External,
		Slug:          dbApp.Slug,
		DisplayName:   dbApp.DisplayName,
		Command:       dbApp.Command.String,
		Icon:          dbApp.Icon,
		Subdomain:     dbApp.Subdomain,
		SubdomainName: subdomainName,
		SharingLevel:  sharingLevel,
		Healthcheck: &WorkspaceApp_Healthcheck{
			Url:       dbApp.HealthcheckUrl,
			Interval:  durationpb.New(time.Duration(dbApp.HealthcheckInterval)),
			Threshold: dbApp.HealthcheckThreshold,
		},
		Health: health,
	}
}
