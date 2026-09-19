package agentapi

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"
	"tailscale.com/tailcfg"

	googleproto "google.golang.org/protobuf/proto"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/quartz"
)

type ManifestAPI struct {
	AccessURL                 *url.URL
	AppHostname               string
	ExternalAuthConfigs       []*externalauth.Config
	DisableDirectConnections  bool
	DerpForceWebSockets       bool
	DisableUserSecretFilePath bool
	WorkspaceID               uuid.UUID

	AgentFn   func(ctx context.Context) (database.WorkspaceAgent, error)
	Database  database.Store
	DerpMapFn func() *tailcfg.DERPMap
	Clock     quartz.Clock
	Pubsub    pubsub.Pubsub
}

func (a *ManifestAPI) GetManifest(ctx context.Context, _ *agentproto.GetManifestRequest) (*agentproto.Manifest, error) {
	var (
		dbApps        []database.WorkspaceApp
		scripts       []database.GetWorkspaceAgentScriptsByAgentIDsRow
		metadata      []database.WorkspaceAgentMetadatum
		workspace     database.Workspace
		devcontainers []database.WorkspaceAgentDevcontainer
	)

	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, xerrors.Errorf("getting workspace agent: %w", err)
	}

	var eg errgroup.Group
	eg.Go(func() (err error) {
		dbApps, err = a.Database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	eg.Go(func() (err error) {
		// nolint:gocritic // This is necessary to fetch agent scripts!
		scripts, err = a.Database.GetWorkspaceAgentScriptsByAgentIDs(dbauthz.AsSystemRestricted(ctx), []uuid.UUID{workspaceAgent.ID})
		return err
	})
	eg.Go(func() (err error) {
		metadata, err = a.Database.GetWorkspaceAgentMetadata(ctx, database.GetWorkspaceAgentMetadataParams{
			WorkspaceAgentID: workspaceAgent.ID,
			Keys:             nil, // all
		})
		return err
	})
	eg.Go(func() (err error) {
		workspace, err = a.Database.GetWorkspaceByID(ctx, a.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("getting workspace by id: %w", err)
		}
		return err
	})
	eg.Go(func() (err error) {
		devcontainers, err = a.Database.GetWorkspaceAgentDevcontainersByAgentID(ctx, workspaceAgent.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	err = eg.Wait()
	if err != nil {
		return nil, xerrors.Errorf("fetching workspace agent data: %w", err)
	}

	// Fetch user secrets for injection into the agent manifest.
	// This runs after the errgroup because it needs workspace.OwnerID.
	//nolint:gocritic // System context needed to read secrets for the workspace owner.
	userSecrets, err := a.Database.ListUserSecretsWithValues(dbauthz.AsSystemRestricted(ctx), workspace.OwnerID)
	if err != nil {
		return nil, xerrors.Errorf("getting user secrets: %w", err)
	}

	derpMap := a.DerpMapFn()
	egress, err := a.egressConfig(ctx, workspace.TemplateID, derpMap)
	if err != nil {
		return nil, xerrors.Errorf("getting egress config: %w", err)
	}

	appSlug := appurl.ApplicationURL{
		AppSlugOrPort: "{{port}}",
		AgentName:     workspaceAgent.Name,
		WorkspaceName: workspace.Name,
		Username:      workspace.OwnerUsername,
	}

	vscodeProxyURI := vscodeProxyURI(appSlug, a.AccessURL, a.AppHostname)

	envs, err := db2sdk.WorkspaceAgentEnvironment(workspaceAgent)
	if err != nil {
		return nil, err
	}

	var gitAuthConfigs uint32
	for _, cfg := range a.ExternalAuthConfigs {
		if codersdk.EnhancedExternalAuthProvider(cfg.Type).Git() {
			gitAuthConfigs++
		}
	}

	apps, err := dbAppsToProto(dbApps, workspaceAgent, workspace.OwnerUsername, workspace, a.AppHostname)
	if err != nil {
		return nil, xerrors.Errorf("converting workspace apps: %w", err)
	}

	var parentID []byte
	if workspaceAgent.ParentID.Valid {
		parentID = workspaceAgent.ParentID.UUID[:]
	}

	secretFilePathPolicy := userSecretFilePathAllowed
	if a.DisableUserSecretFilePath {
		secretFilePathPolicy = userSecretFilePathBlocked
	}

	return &agentproto.Manifest{
		AgentId:                  workspaceAgent.ID[:],
		AgentName:                workspaceAgent.Name,
		OwnerUsername:            workspace.OwnerUsername,
		WorkspaceId:              workspace.ID[:],
		WorkspaceName:            workspace.Name,
		GitAuthConfigs:           gitAuthConfigs,
		EnvironmentVariables:     envs,
		Directory:                workspaceAgent.Directory,
		VsCodePortProxyUri:       vscodeProxyURI,
		MotdPath:                 workspaceAgent.MOTDFile,
		DisableDirectConnections: a.DisableDirectConnections,
		DerpForceWebsockets:      a.DerpForceWebSockets,
		ParentId:                 parentID,

		DerpMap:       tailnet.DERPMapToProto(derpMap),
		Scripts:       dbAgentScriptsToProto(scripts),
		Apps:          apps,
		Metadata:      dbAgentMetadataToProtoDescription(metadata),
		Devcontainers: dbAgentDevcontainersToProto(devcontainers),
		Secrets:       dbUserSecretsToProto(userSecrets, secretFilePathPolicy),
		Egress:        egress,
	}, nil
}

// egressConfig returns the egress configuration for a workspace built from
// templateID, or nil when the template is not bound to an exit node.
func (a *ManifestAPI) egressConfig(ctx context.Context, templateID uuid.UUID, derpMap *tailcfg.DERPMap) (*agentproto.EgressConfig, error) {
	//nolint:gocritic // Exit node configuration is deployment configuration.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	template, err := a.Database.GetTemplateByID(systemCtx, templateID)
	if err != nil {
		return nil, xerrors.Errorf("get template: %w", err)
	}
	rows, err := a.Database.GetTemplateExitNodeReplicas(systemCtx, database.GetTemplateExitNodeReplicasParams{
		TemplateID:   templateID,
		UpdatedAfter: a.now().Add(-codersdk.ExitNodeReplicaStaleAfter),
	})
	if err != nil {
		return nil, xerrors.Errorf("get template exit node replicas: %w", err)
	}
	return buildEgressConfig(rows, template, derpMap, a.AccessURL), nil
}

func (a *ManifestAPI) now() time.Time {
	if a.Clock == nil {
		return time.Now()
	}
	return a.Clock.Now("egress_config")
}

func buildEgressConfig(rows []database.GetTemplateExitNodeReplicasRow, template database.Template, derpMap *tailcfg.DERPMap, accessURL *url.URL) *agentproto.EgressConfig {
	if len(rows) == 0 {
		return nil
	}

	exitNodes := make([]*agentproto.EgressExitNode, 0)
	positions := make(map[uuid.UUID]int)
	var wireguardEndpoints []string
	for _, row := range rows {
		index, ok := positions[row.ExitNodeID]
		if !ok {
			index = len(exitNodes)
			positions[row.ExitNodeID] = index
			exitNodes = append(exitNodes, &agentproto.EgressExitNode{Id: row.ExitNodeID[:]})
		}
		if row.ReplicaID.Valid {
			exitNodes[index].ReplicaIds = append(exitNodes[index].ReplicaIds, row.ReplicaID.UUID[:])
			wireguardEndpoints = append(wireguardEndpoints, row.WireguardEndpoints...)
		}
	}

	return &agentproto.EgressConfig{
		ExitNodes:         exitNodes,
		ExitNodePort:      codersdk.ExitNodeTailnetPort,
		Enforce:           template.ExitNodeEnforce,
		ControlPlaneHosts: controlPlaneHosts(accessURL, derpMap, wireguardEndpoints),
	}
}

func (a *ManifestAPI) StreamEgressConfig(_ *agentproto.StreamEgressConfigRequest, stream agentproto.DRPCAgent_StreamEgressConfigStream) error {
	ctx := stream.Context()
	workspace, err := a.Database.GetWorkspaceByID(ctx, a.WorkspaceID)
	if err != nil {
		return xerrors.Errorf("get workspace: %w", err)
	}

	updates := make(chan uuid.UUID, 1)
	cancel, err := a.Pubsub.Subscribe(codersdk.ExitNodeReplicasPubsubChannel, func(_ context.Context, payload []byte) {
		id, err := uuid.ParseBytes(payload)
		if err != nil {
			return
		}
		select {
		case updates <- id:
		default:
		}
	})
	if err != nil {
		return xerrors.Errorf("subscribe to exit node replica updates: %w", err)
	}
	defer cancel()

	ticker := a.Clock.NewTicker(codersdk.ExitNodeReplicaStaleAfter, "stream_egress_config")
	defer ticker.Stop()
	var last *agentproto.EgressConfig
	bound := make(map[uuid.UUID]struct{})
	for {
		cfg, err := a.egressConfig(ctx, workspace.TemplateID, a.DerpMapFn())
		if err != nil {
			return err
		}
		if cfg == nil {
			cfg = &agentproto.EgressConfig{}
		}
		clear(bound)
		for _, exitNode := range cfg.ExitNodes {
			id, err := uuid.FromBytes(exitNode.Id)
			if err == nil {
				bound[id] = struct{}{}
			}
		}
		if last == nil || !googleproto.Equal(last, cfg) {
			if err := stream.Send(cfg); err != nil {
				return xerrors.Errorf("send egress config: %w", err)
			}
			last = &agentproto.EgressConfig{}
			googleproto.Merge(last, cfg)
		}

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				goto recompute
			case id := <-updates:
				if _, ok := bound[id]; ok {
					goto recompute
				}
			}
		}
	recompute:
	}
}

// controlPlaneHosts lists the protocol and port specific destinations an agent
// must keep reaching directly when egress enforcement is on. The result is
// deduplicated and sorted so the manifest is stable.
func controlPlaneHosts(accessURL *url.URL, derpMap *tailcfg.DERPMap, wireguardEndpoints []string) []string {
	hosts := make(map[string]struct{})
	add := func(proto, host string, port int) {
		if host == "" || port <= 0 || port > 65535 {
			return
		}
		hosts[proto+"/"+net.JoinHostPort(host, strconv.Itoa(port))] = struct{}{}
	}

	if accessURL != nil {
		port := accessURL.Port()
		if port == "" {
			switch strings.ToLower(accessURL.Scheme) {
			case "https":
				port = "443"
			case "http":
				port = "80"
			}
		}
		parsedPort, _ := strconv.Atoi(port)
		add("tcp", accessURL.Hostname(), parsedPort)
	}
	if derpMap != nil {
		for _, region := range derpMap.Regions {
			if region == nil {
				continue
			}
			for _, node := range region.Nodes {
				if node == nil {
					continue
				}
				derpPort := node.DERPPort
				if derpPort == 0 {
					if node.ForceHTTP {
						derpPort = 80
					} else {
						derpPort = 443
					}
				}
				stunPort := node.STUNPort
				if stunPort == 0 {
					stunPort = 3478
				}
				addresses := []string{node.HostName}
				for _, ip := range []string{node.IPv4, node.IPv6} {
					if ip != "" && ip != "none" {
						addresses = append(addresses, ip)
					}
				}
				for _, host := range addresses {
					if !node.STUNOnly {
						add("tcp", host, derpPort)
					}
					if node.STUNPort >= 0 {
						add("udp", host, stunPort)
					}
				}
			}
		}
	}
	for _, endpoint := range wireguardEndpoints {
		host, portString, err := net.SplitHostPort(endpoint)
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(portString)
		if err != nil {
			continue
		}
		add("udp", host, port)
	}
	return slices.Sorted(maps.Keys(hosts))
}

func vscodeProxyURI(app appurl.ApplicationURL, accessURL *url.URL, appHost string) string {
	// Proxying by port only works for subdomains. If subdomain support is not
	// available, return an empty string.
	if appHost == "" {
		return ""
	}

	// This will handle the ports from the accessURL or appHost.
	appHost = appurl.SubdomainAppHost(appHost, accessURL)
	// Return the url with a scheme and any wildcards replaced with the app slug.
	return accessURL.Scheme + "://" + strings.ReplaceAll(appHost, "*", app.String())
}

func dbAgentMetadataToProtoDescription(metadata []database.WorkspaceAgentMetadatum) []*agentproto.WorkspaceAgentMetadata_Description {
	ret := make([]*agentproto.WorkspaceAgentMetadata_Description, len(metadata))
	for i, metadatum := range metadata {
		ret[i] = dbAgentMetadatumToProtoDescription(metadatum)
	}
	return ret
}

func dbAgentMetadatumToProtoDescription(metadatum database.WorkspaceAgentMetadatum) *agentproto.WorkspaceAgentMetadata_Description {
	return &agentproto.WorkspaceAgentMetadata_Description{
		DisplayName: metadatum.DisplayName,
		Key:         metadatum.Key,
		Script:      metadatum.Script,
		Interval:    durationpb.New(time.Duration(metadatum.Interval)),
		Timeout:     durationpb.New(time.Duration(metadatum.Timeout)),
	}
}

func dbAgentScriptsToProto(scripts []database.GetWorkspaceAgentScriptsByAgentIDsRow) []*agentproto.WorkspaceAgentScript {
	ret := make([]*agentproto.WorkspaceAgentScript, len(scripts))
	for i, script := range scripts {
		ret[i] = dbAgentScriptToProto(script)
	}
	return ret
}

func dbAgentScriptToProto(script database.GetWorkspaceAgentScriptsByAgentIDsRow) *agentproto.WorkspaceAgentScript {
	return &agentproto.WorkspaceAgentScript{
		Id:               script.ID[:],
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

func dbAppsToProto(dbApps []database.WorkspaceApp, agent database.WorkspaceAgent, ownerName string, workspace database.Workspace, appHostname string) ([]*agentproto.WorkspaceApp, error) {
	ret := make([]*agentproto.WorkspaceApp, len(dbApps))
	for i, dbApp := range dbApps {
		var err error
		ret[i], err = dbAppToProto(dbApp, agent, ownerName, workspace, appHostname)
		if err != nil {
			return nil, xerrors.Errorf("parse app %v (%q): %w", i, dbApp.Slug, err)
		}
	}
	return ret, nil
}

func dbAppToProto(dbApp database.WorkspaceApp, agent database.WorkspaceAgent, ownerName string, workspace database.Workspace, appHostname string) (*agentproto.WorkspaceApp, error) {
	sharingLevelRaw, ok := agentproto.WorkspaceApp_SharingLevel_value[strings.ToUpper(string(dbApp.SharingLevel))]
	if !ok {
		return nil, xerrors.Errorf("unknown app sharing level: %q", dbApp.SharingLevel)
	}

	healthRaw, ok := agentproto.WorkspaceApp_Health_value[strings.ToUpper(string(dbApp.Health))]
	if !ok {
		return nil, xerrors.Errorf("unknown app health: %q", dbApp.SharingLevel)
	}

	// SubdomainName should be empty if AppHostname is not configured
	subdomainName := ""
	if appHostname != "" {
		subdomainName = db2sdk.AppSubdomain(dbApp, agent.Name, workspace.Name, ownerName)
	}

	return &agentproto.WorkspaceApp{
		Id:            dbApp.ID[:],
		Url:           dbApp.Url.String,
		External:      dbApp.External,
		Slug:          dbApp.Slug,
		DisplayName:   dbApp.DisplayName,
		Command:       dbApp.Command.String,
		Icon:          dbApp.Icon,
		Subdomain:     dbApp.Subdomain,
		SubdomainName: subdomainName,
		SharingLevel:  agentproto.WorkspaceApp_SharingLevel(sharingLevelRaw),
		Healthcheck: &agentproto.WorkspaceApp_Healthcheck{
			Url:       dbApp.HealthcheckUrl,
			Interval:  durationpb.New(time.Duration(dbApp.HealthcheckInterval) * time.Second),
			Threshold: dbApp.HealthcheckThreshold,
		},
		Health: agentproto.WorkspaceApp_Health(healthRaw),
		Hidden: dbApp.Hidden,
	}, nil
}

func dbAgentDevcontainersToProto(devcontainers []database.WorkspaceAgentDevcontainer) []*agentproto.WorkspaceAgentDevcontainer {
	ret := make([]*agentproto.WorkspaceAgentDevcontainer, len(devcontainers))
	for i, dc := range devcontainers {
		var subagentID []byte
		if dc.SubagentID.Valid {
			subagentID = dc.SubagentID.UUID[:]
		}

		ret[i] = &agentproto.WorkspaceAgentDevcontainer{
			Id:              dc.ID[:],
			Name:            dc.Name,
			WorkspaceFolder: dc.WorkspaceFolder,
			ConfigPath:      dc.ConfigPath,
			SubagentId:      subagentID,
		}
	}
	return ret
}

// userSecretFilePathPolicy is a named type rather than a bool parameter to
// satisfy revive's flag-parameter rule.
type userSecretFilePathPolicy int

const (
	userSecretFilePathAllowed userSecretFilePathPolicy = iota
	userSecretFilePathBlocked
)

func dbUserSecretsToProto(secrets []database.UserSecret, policy userSecretFilePathPolicy) []*agentproto.WorkspaceSecret {
	ret := make([]*agentproto.WorkspaceSecret, 0, len(secrets))
	for _, s := range secrets {
		// Skip disabled secrets so they are not injected as env vars or
		// written to secret files. The API guarantees every enabled
		// secret has at least one of env_name or file_path set, so we
		// don't need to filter both-empty rows separately here.
		if !s.Enabled {
			continue
		}
		filePath := s.FilePath
		if policy == userSecretFilePathBlocked {
			if s.EnvName == "" {
				continue
			}
			filePath = ""
		}
		ret = append(ret, &agentproto.WorkspaceSecret{
			EnvName:  s.EnvName,
			FilePath: filePath,
			Value:    []byte(s.Value),
		})
	}
	return ret
}
