package agentsdk

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
)

func ManifestFromProto(manifest *proto.Manifest) (Manifest, error) {
	apps, err := AppsFromProto(manifest.Apps)
	if err != nil {
		return Manifest{}, xerrors.Errorf("error converting workspace agent apps: %w", err)
	}
	scripts, err := AgentScriptsFromProto(manifest.Scripts)
	if err != nil {
		return Manifest{}, xerrors.Errorf("error converting workspace agent scripts: %w", err)
	}
	agentID, err := uuid.FromBytes(manifest.AgentId)
	if err != nil {
		return Manifest{}, xerrors.Errorf("error converting workspace agent ID: %w", err)
	}
	workspaceID, err := uuid.FromBytes(manifest.WorkspaceId)
	if err != nil {
		return Manifest{}, xerrors.Errorf("error converting workspace ID: %w", err)
	}
	return Manifest{
		AgentID:                  agentID,
		AgentName:                manifest.AgentName,
		OwnerName:                manifest.OwnerUsername,
		WorkspaceID:              workspaceID,
		WorkspaceName:            manifest.WorkspaceName,
		Apps:                     apps,
		Scripts:                  scripts,
		DERPMap:                  tailnet.DERPMapFromProto(manifest.DerpMap),
		DERPForceWebSockets:      manifest.DerpForceWebsockets,
		GitAuthConfigs:           int(manifest.GitAuthConfigs),
		EnvironmentVariables:     manifest.EnvironmentVariables,
		Directory:                manifest.Directory,
		VSCodePortProxyURI:       manifest.VsCodePortProxyUri,
		MOTDFile:                 manifest.MotdPath,
		DisableDirectConnections: manifest.DisableDirectConnections,
		Metadata:                 MetadataDescriptionsFromProto(manifest.Metadata),
	}, nil
}

func ProtoFromManifest(manifest Manifest) (*proto.Manifest, error) {
	apps, err := ProtoFromApps(manifest.Apps)
	if err != nil {
		return nil, xerrors.Errorf("convert workspace apps: %w", err)
	}
	return &proto.Manifest{
		AgentId:                  manifest.AgentID[:],
		AgentName:                manifest.AgentName,
		OwnerUsername:            manifest.OwnerName,
		WorkspaceId:              manifest.WorkspaceID[:],
		WorkspaceName:            manifest.WorkspaceName,
		GitAuthConfigs:           uint32(manifest.GitAuthConfigs),
		EnvironmentVariables:     manifest.EnvironmentVariables,
		Directory:                manifest.Directory,
		VsCodePortProxyUri:       manifest.VSCodePortProxyURI,
		MotdPath:                 manifest.MOTDFile,
		DisableDirectConnections: manifest.DisableDirectConnections,
		DerpForceWebsockets:      manifest.DERPForceWebSockets,
		DerpMap:                  tailnet.DERPMapToProto(manifest.DERPMap),
		Scripts:                  ProtoFromScripts(manifest.Scripts),
		Apps:                     apps,
		Metadata:                 ProtoFromMetadataDescriptions(manifest.Metadata),
	}, nil
}

func MetadataDescriptionsFromProto(descriptions []*proto.WorkspaceAgentMetadata_Description) []codersdk.WorkspaceAgentMetadataDescription {
	ret := make([]codersdk.WorkspaceAgentMetadataDescription, len(descriptions))
	for i, description := range descriptions {
		ret[i] = MetadataDescriptionFromProto(description)
	}
	return ret
}

func ProtoFromMetadataDescriptions(descriptions []codersdk.WorkspaceAgentMetadataDescription) []*proto.WorkspaceAgentMetadata_Description {
	ret := make([]*proto.WorkspaceAgentMetadata_Description, len(descriptions))
	for i, d := range descriptions {
		ret[i] = ProtoFromMetadataDescription(d)
	}
	return ret
}

func MetadataDescriptionFromProto(description *proto.WorkspaceAgentMetadata_Description) codersdk.WorkspaceAgentMetadataDescription {
	return codersdk.WorkspaceAgentMetadataDescription{
		DisplayName: description.DisplayName,
		Key:         description.Key,
		Script:      description.Script,
		Interval:    int64(description.Interval.AsDuration()),
		Timeout:     int64(description.Timeout.AsDuration()),
	}
}

func ProtoFromMetadataDescription(d codersdk.WorkspaceAgentMetadataDescription) *proto.WorkspaceAgentMetadata_Description {
	return &proto.WorkspaceAgentMetadata_Description{
		DisplayName: d.DisplayName,
		Key:         d.Key,
		Script:      d.Script,
		Interval:    durationpb.New(time.Duration(d.Interval)),
		Timeout:     durationpb.New(time.Duration(d.Timeout)),
	}
}

func ProtoFromMetadataResult(r codersdk.WorkspaceAgentMetadataResult) *proto.WorkspaceAgentMetadata_Result {
	return &proto.WorkspaceAgentMetadata_Result{
		CollectedAt: timestamppb.New(r.CollectedAt),
		Age:         r.Age,
		Value:       r.Value,
		Error:       r.Error,
	}
}

func MetadataResultFromProto(r *proto.WorkspaceAgentMetadata_Result) codersdk.WorkspaceAgentMetadataResult {
	return codersdk.WorkspaceAgentMetadataResult{
		CollectedAt: r.GetCollectedAt().AsTime(),
		Age:         r.GetAge(),
		Value:       r.GetValue(),
		Error:       r.GetError(),
	}
}

func MetadataFromProto(m *proto.Metadata) Metadata {
	return Metadata{
		Key:                          m.GetKey(),
		WorkspaceAgentMetadataResult: MetadataResultFromProto(m.GetResult()),
	}
}

func AgentScriptsFromProto(protoScripts []*proto.WorkspaceAgentScript) ([]codersdk.WorkspaceAgentScript, error) {
	ret := make([]codersdk.WorkspaceAgentScript, len(protoScripts))
	for i, protoScript := range protoScripts {
		app, err := AgentScriptFromProto(protoScript)
		if err != nil {
			return nil, xerrors.Errorf("parse script %v: %w", i, err)
		}
		ret[i] = app
	}
	return ret, nil
}

func ProtoFromScripts(scripts []codersdk.WorkspaceAgentScript) []*proto.WorkspaceAgentScript {
	ret := make([]*proto.WorkspaceAgentScript, len(scripts))
	for i, script := range scripts {
		ret[i] = ProtoFromScript(script)
	}
	return ret
}

func AgentScriptFromProto(protoScript *proto.WorkspaceAgentScript) (codersdk.WorkspaceAgentScript, error) {
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

func ProtoFromScript(s codersdk.WorkspaceAgentScript) *proto.WorkspaceAgentScript {
	return &proto.WorkspaceAgentScript{
		LogSourceId:      s.LogSourceID[:],
		LogPath:          s.LogPath,
		Script:           s.Script,
		Cron:             s.Cron,
		RunOnStart:       s.RunOnStart,
		RunOnStop:        s.RunOnStop,
		StartBlocksLogin: s.StartBlocksLogin,
		Timeout:          durationpb.New(s.Timeout),
	}
}

func AppsFromProto(protoApps []*proto.WorkspaceApp) ([]codersdk.WorkspaceApp, error) {
	ret := make([]codersdk.WorkspaceApp, len(protoApps))
	for i, protoApp := range protoApps {
		app, err := AppFromProto(protoApp)
		if err != nil {
			return nil, xerrors.Errorf("parse app %v (%q): %w", i, protoApp.Slug, err)
		}
		ret[i] = app
	}
	return ret, nil
}

func ProtoFromApps(apps []codersdk.WorkspaceApp) ([]*proto.WorkspaceApp, error) {
	ret := make([]*proto.WorkspaceApp, len(apps))
	var err error
	for i, a := range apps {
		ret[i], err = ProtoFromApp(a)
		if err != nil {
			return nil, err
		}
	}
	return ret, nil
}

func AppFromProto(protoApp *proto.WorkspaceApp) (codersdk.WorkspaceApp, error) {
	id, err := uuid.FromBytes(protoApp.Id)
	if err != nil {
		return codersdk.WorkspaceApp{}, xerrors.Errorf("parse id: %w", err)
	}

	sharingLevel := codersdk.WorkspaceAppSharingLevel(strings.ToLower(protoApp.SharingLevel.String()))
	if _, ok := codersdk.MapWorkspaceAppSharingLevels[sharingLevel]; !ok {
		return codersdk.WorkspaceApp{}, xerrors.Errorf("unknown app sharing level: %v (%q)", protoApp.SharingLevel, protoApp.SharingLevel.String())
	}

	health := codersdk.WorkspaceAppHealth(strings.ToLower(protoApp.Health.String()))
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

func ProtoFromApp(a codersdk.WorkspaceApp) (*proto.WorkspaceApp, error) {
	sharingLevel, ok := proto.WorkspaceApp_SharingLevel_value[strings.ToUpper(string(a.SharingLevel))]
	if !ok {
		return nil, xerrors.Errorf("unknown sharing level %s", a.SharingLevel)
	}
	health, ok := proto.WorkspaceApp_Health_value[strings.ToUpper(string(a.Health))]
	if !ok {
		return nil, xerrors.Errorf("unknown health %s", a.Health)
	}
	return &proto.WorkspaceApp{
		Id:            a.ID[:],
		Url:           a.URL,
		External:      a.External,
		Slug:          a.Slug,
		DisplayName:   a.DisplayName,
		Command:       a.Command,
		Icon:          a.Icon,
		Subdomain:     a.Subdomain,
		SubdomainName: a.SubdomainName,
		SharingLevel:  proto.WorkspaceApp_SharingLevel(sharingLevel),
		Healthcheck: &proto.WorkspaceApp_Healthcheck{
			Url:       a.Healthcheck.URL,
			Interval:  durationpb.New(time.Duration(a.Healthcheck.Interval) * time.Second),
			Threshold: a.Healthcheck.Threshold,
		},
		Health: proto.WorkspaceApp_Health(health),
	}, nil
}

func ServiceBannerFromProto(sbp *proto.ServiceBanner) codersdk.BannerConfig {
	return codersdk.BannerConfig{
		Enabled:         sbp.GetEnabled(),
		Message:         sbp.GetMessage(),
		BackgroundColor: sbp.GetBackgroundColor(),
	}
}

func ProtoFromServiceBanner(sb codersdk.BannerConfig) *proto.ServiceBanner {
	return &proto.ServiceBanner{
		Enabled:         sb.Enabled,
		Message:         sb.Message,
		BackgroundColor: sb.BackgroundColor,
	}
}

func BannerConfigFromProto(sbp *proto.BannerConfig) codersdk.BannerConfig {
	return codersdk.BannerConfig{
		Enabled:         sbp.GetEnabled(),
		Message:         sbp.GetMessage(),
		BackgroundColor: sbp.GetBackgroundColor(),
	}
}

func ProtoFromBannerConfig(sb codersdk.BannerConfig) *proto.BannerConfig {
	return &proto.BannerConfig{
		Enabled:         sb.Enabled,
		Message:         sb.Message,
		BackgroundColor: sb.BackgroundColor,
	}
}

func ProtoFromSubsystems(ss []codersdk.AgentSubsystem) ([]proto.Startup_Subsystem, error) {
	ret := make([]proto.Startup_Subsystem, len(ss))
	for i, s := range ss {
		pi, ok := proto.Startup_Subsystem_value[strings.ToUpper(string(s))]
		if !ok {
			return nil, xerrors.Errorf("unknown subsystem: %s", s)
		}
		ret[i] = proto.Startup_Subsystem(pi)
	}
	return ret, nil
}

func ProtoFromAppHealthsRequest(req PostAppHealthsRequest) (*proto.BatchUpdateAppHealthRequest, error) {
	pReq := &proto.BatchUpdateAppHealthRequest{}
	for id, h := range req.Healths {
		hp, ok := proto.AppHealth_value[strings.ToUpper(string(h))]
		if !ok {
			return nil, xerrors.Errorf("unknown app health: %s", h)
		}

		// Copy the ID, otherwise all updates will have the same ID (the last
		// one in the list).
		var idCopy uuid.UUID
		copy(idCopy[:], id[:])
		pReq.Updates = append(pReq.Updates, &proto.BatchUpdateAppHealthRequest_HealthUpdate{
			Id:     idCopy[:],
			Health: proto.AppHealth(hp),
		})
	}
	return pReq, nil
}

func ProtoFromLog(log Log) (*proto.Log, error) {
	lvl, ok := proto.Log_Level_value[strings.ToUpper(string(log.Level))]
	if !ok {
		return nil, xerrors.Errorf("unknown log level: %s", log.Level)
	}
	return &proto.Log{
		CreatedAt: timestamppb.New(log.CreatedAt),
		Output:    strings.ToValidUTF8(log.Output, "❌"),
		Level:     proto.Log_Level(lvl),
	}, nil
}

func ProtoFromLifecycle(req PostLifecycleRequest) (*proto.Lifecycle, error) {
	s, ok := proto.Lifecycle_State_value[strings.ToUpper(string(req.State))]
	if !ok {
		return nil, xerrors.Errorf("unknown lifecycle state: %s", req.State)
	}
	return &proto.Lifecycle{
		State:     proto.Lifecycle_State(s),
		ChangedAt: timestamppb.New(req.ChangedAt),
	}, nil
}

func LifecycleStateFromProto(s proto.Lifecycle_State) (codersdk.WorkspaceAgentLifecycle, error) {
	caps, ok := proto.Lifecycle_State_name[int32(s)]
	if !ok {
		return "", xerrors.Errorf("unknown lifecycle state: %d", s)
	}
	return codersdk.WorkspaceAgentLifecycle(strings.ToLower(caps)), nil
}
