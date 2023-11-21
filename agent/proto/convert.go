package proto

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
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

func SDKAgentMetadataDescriptionsFromProto(descriptions []*WorkspaceAgentMetadata_Description) []codersdk.WorkspaceAgentMetadataDescription {
	ret := make([]codersdk.WorkspaceAgentMetadataDescription, len(descriptions))
	for i, description := range descriptions {
		ret[i] = SDKAgentMetadataDescriptionFromProto(description)
	}
	return ret
}

func SDKAgentMetadataDescriptionFromProto(description *WorkspaceAgentMetadata_Description) codersdk.WorkspaceAgentMetadataDescription {
	return codersdk.WorkspaceAgentMetadataDescription{
		DisplayName: description.DisplayName,
		Key:         description.Key,
		Script:      description.Script,
		Interval:    int64(description.Interval.AsDuration()),
		Timeout:     int64(description.Timeout.AsDuration()),
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

func SDKAgentScriptsFromProto(protoScripts []*WorkspaceAgentScript) ([]codersdk.WorkspaceAgentScript, error) {
	ret := make([]codersdk.WorkspaceAgentScript, len(protoScripts))
	for i, protoScript := range protoScripts {
		app, err := SDKAgentScriptFromProto(protoScript)
		if err != nil {
			return nil, xerrors.Errorf("parse script %v: %w", i, err)
		}
		ret[i] = app
	}
	return ret, nil
}

func SDKAgentScriptFromProto(protoScript *WorkspaceAgentScript) (codersdk.WorkspaceAgentScript, error) {
	id, err := uuid.FromBytes(protoScript.LogSourceId)
	if err != nil {
		return codersdk.WorkspaceAgentScript{}, xerrors.Errorf("parse id: %w", err)
	}

	return codersdk.WorkspaceAgentScript{
		LogSourceID:      id,
		LogPath:          protoScript.LogPath,
		Script:           protoScript.Script,
		Cron:             protoScript.Cron,
		RunOnStart:       protoScript.RunOnStart,
		RunOnStop:        protoScript.RunOnStop,
		StartBlocksLogin: protoScript.StartBlocksLogin,
		Timeout:          protoScript.Timeout.AsDuration(),
	}, nil
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
			Interval:  durationpb.New(time.Duration(dbApp.HealthcheckInterval) * time.Second),
			Threshold: dbApp.HealthcheckThreshold,
		},
		Health: health,
	}
}

func SDKAppsFromProto(protoApps []*WorkspaceApp) ([]codersdk.WorkspaceApp, error) {
	ret := make([]codersdk.WorkspaceApp, len(protoApps))
	for i, protoApp := range protoApps {
		app, err := SDKAppFromProto(protoApp)
		if err != nil {
			return nil, xerrors.Errorf("parse app %v (%q): %w", i, protoApp.Slug, err)
		}
		ret[i] = app
	}
	return ret, nil
}

func SDKAppFromProto(protoApp *WorkspaceApp) (codersdk.WorkspaceApp, error) {
	id, err := uuid.FromBytes(protoApp.Id)
	if err != nil {
		return codersdk.WorkspaceApp{}, xerrors.Errorf("parse id: %w", err)
	}

	var sharingLevel codersdk.WorkspaceAppSharingLevel
	switch protoApp.SharingLevel {
	case WorkspaceApp_OWNER:
		sharingLevel = codersdk.WorkspaceAppSharingLevelOwner
	case WorkspaceApp_AUTHENTICATED:
		sharingLevel = codersdk.WorkspaceAppSharingLevelAuthenticated
	case WorkspaceApp_PUBLIC:
		sharingLevel = codersdk.WorkspaceAppSharingLevelPublic
	default:
		return codersdk.WorkspaceApp{}, xerrors.Errorf("unknown sharing level: %v", protoApp.SharingLevel)
	}

	var health codersdk.WorkspaceAppHealth
	switch protoApp.Health {
	case WorkspaceApp_DISABLED:
		health = codersdk.WorkspaceAppHealthDisabled
	case WorkspaceApp_INITIALIZING:
		health = codersdk.WorkspaceAppHealthInitializing
	case WorkspaceApp_HEALTHY:
		health = codersdk.WorkspaceAppHealthHealthy
	case WorkspaceApp_UNHEALTHY:
		health = codersdk.WorkspaceAppHealthUnhealthy
	default:
		return codersdk.WorkspaceApp{}, xerrors.Errorf("unknown health: %v", protoApp.Health)
	}

	return codersdk.WorkspaceApp{
		ID:            id,
		URL:           protoApp.Url,
		External:      protoApp.External,
		Slug:          protoApp.Slug,
		DisplayName:   protoApp.DisplayName,
		Command:       protoApp.Command,
		Icon:          protoApp.Icon,
		Subdomain:     protoApp.Subdomain,
		SubdomainName: protoApp.SubdomainName,
		SharingLevel:  sharingLevel,
		Healthcheck: codersdk.Healthcheck{
			URL:       protoApp.Healthcheck.Url,
			Interval:  int32(protoApp.Healthcheck.Interval.AsDuration().Seconds()),
			Threshold: protoApp.Healthcheck.Threshold,
		},
		Health: health,
	}, nil
}
