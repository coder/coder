package proto

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
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

func DBAppsToProto(dbApps []database.WorkspaceApp, agent database.WorkspaceAgent, ownerName string, workspace database.Workspace) ([]*WorkspaceApp, error) {
	ret := make([]*WorkspaceApp, len(dbApps))
	for i, dbApp := range dbApps {
		var err error
		ret[i], err = DBAppToProto(dbApp, agent, ownerName, workspace)
		if err != nil {
			return nil, xerrors.Errorf("parse app %v (%q): %w", i, dbApp.Slug, err)
		}
	}
	return ret, nil
}

func DBAppToProto(dbApp database.WorkspaceApp, agent database.WorkspaceAgent, ownerName string, workspace database.Workspace) (*WorkspaceApp, error) {
	sharingLevelRaw, ok := WorkspaceApp_SharingLevel_value[strings.ToUpper(string(dbApp.SharingLevel))]
	if !ok {
		return nil, xerrors.Errorf("unknown app sharing level: %q", dbApp.SharingLevel)
	}

	healthRaw, ok := WorkspaceApp_Health_value[strings.ToUpper(string(dbApp.Health))]
	if !ok {
		return nil, xerrors.Errorf("unknown app health: %q", dbApp.SharingLevel)
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
		SubdomainName: db2sdk.AppSubdomain(dbApp, agent.Name, workspace.Name, ownerName),
		SharingLevel:  WorkspaceApp_SharingLevel(sharingLevelRaw),
		Healthcheck: &WorkspaceApp_Healthcheck{
			Url:       dbApp.HealthcheckUrl,
			Interval:  durationpb.New(time.Duration(dbApp.HealthcheckInterval) * time.Second),
			Threshold: dbApp.HealthcheckThreshold,
		},
		Health: WorkspaceApp_Health(healthRaw),
	}, nil
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

	var sharingLevel codersdk.WorkspaceAppSharingLevel = codersdk.WorkspaceAppSharingLevel(strings.ToLower(protoApp.SharingLevel.String()))
	if _, ok := codersdk.MapWorkspaceAppSharingLevels[sharingLevel]; !ok {
		return codersdk.WorkspaceApp{}, xerrors.Errorf("unknown app sharing level: %v (%q)", protoApp.SharingLevel, protoApp.SharingLevel.String())
	}

	var health codersdk.WorkspaceAppHealth = codersdk.WorkspaceAppHealth(strings.ToLower(protoApp.Health.String()))
	if _, ok := codersdk.MapWorkspaceAppHealths[health]; !ok {
		return codersdk.WorkspaceApp{}, xerrors.Errorf("unknown app health: %v (%q)", protoApp.Health, protoApp.Health.String())
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
