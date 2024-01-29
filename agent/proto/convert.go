package proto

import (
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

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

func SDKServiceBannerFromProto(sbp *ServiceBanner) codersdk.ServiceBannerConfig {
	return codersdk.ServiceBannerConfig{
		Enabled:         sbp.GetEnabled(),
		Message:         sbp.GetMessage(),
		BackgroundColor: sbp.GetBackgroundColor(),
	}
}

func ServiceBannerFromSDK(sb codersdk.ServiceBannerConfig) *ServiceBanner {
	return &ServiceBanner{
		Enabled:         sb.Enabled,
		Message:         sb.Message,
		BackgroundColor: sb.BackgroundColor,
	}
}
