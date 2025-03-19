package agentapi

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"
	"tailscale.com/tailcfg"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
)

type ManifestAPI struct {
	AccessURL                *url.URL
	AppHostname              string
	ExternalAuthConfigs      []*externalauth.Config
	DisableDirectConnections bool
	DerpForceWebSockets      bool
	WorkspaceID              uuid.UUID

	AgentFn   func(context.Context) (database.WorkspaceAgent, error)
	Database  database.Store
	DerpMapFn func() *tailcfg.DERPMap
}

func (a *ManifestAPI) GetManifest(ctx context.Context, _ *agentproto.GetManifestRequest) (*agentproto.Manifest, error) {
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}
	var (
		dbApps        []database.WorkspaceApp
		scripts       []database.WorkspaceAgentScript
		metadata      []database.WorkspaceAgentMetadatum
		workspace     database.Workspace
		owner         database.User
		devcontainers []database.WorkspaceAgentDevcontainer
	)

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
		owner, err = a.Database.GetUserByID(ctx, workspace.OwnerID)
		if err != nil {
			return xerrors.Errorf("getting workspace owner by id: %w", err)
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

	appSlug := appurl.ApplicationURL{
		AppSlugOrPort: "{{port}}",
		AgentName:     workspaceAgent.Name,
		WorkspaceName: workspace.Name,
		Username:      owner.Username,
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

	apps, err := dbAppsToProto(dbApps, workspaceAgent, owner.Username, workspace)
	if err != nil {
		return nil, xerrors.Errorf("converting workspace apps: %w", err)
	}

	return &agentproto.Manifest{
		AgentId:                  workspaceAgent.ID[:],
		AgentName:                workspaceAgent.Name,
		OwnerUsername:            owner.Username,
		WorkspaceId:              workspace.ID[:],
		WorkspaceName:            workspace.Name,
		GitAuthConfigs:           gitAuthConfigs,
		EnvironmentVariables:     envs,
		Directory:                workspaceAgent.Directory,
		VsCodePortProxyUri:       vscodeProxyURI,
		MotdPath:                 workspaceAgent.MOTDFile,
		DisableDirectConnections: a.DisableDirectConnections,
		DerpForceWebsockets:      a.DerpForceWebSockets,

		DerpMap:       tailnet.DERPMapToProto(a.DerpMapFn()),
		Scripts:       dbAgentScriptsToProto(scripts),
		Apps:          apps,
		Metadata:      dbAgentMetadataToProtoDescription(metadata),
		Devcontainers: dbAgentDevcontainersToProto(devcontainers),
	}, nil
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

func dbAgentScriptsToProto(scripts []database.WorkspaceAgentScript) []*agentproto.WorkspaceAgentScript {
	ret := make([]*agentproto.WorkspaceAgentScript, len(scripts))
	for i, script := range scripts {
		ret[i] = dbAgentScriptToProto(script)
	}
	return ret
}

func dbAgentScriptToProto(script database.WorkspaceAgentScript) *agentproto.WorkspaceAgentScript {
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

func dbAppsToProto(dbApps []database.WorkspaceApp, agent database.WorkspaceAgent, ownerName string, workspace database.Workspace) ([]*agentproto.WorkspaceApp, error) {
	ret := make([]*agentproto.WorkspaceApp, len(dbApps))
	for i, dbApp := range dbApps {
		var err error
		ret[i], err = dbAppToProto(dbApp, agent, ownerName, workspace)
		if err != nil {
			return nil, xerrors.Errorf("parse app %v (%q): %w", i, dbApp.Slug, err)
		}
	}
	return ret, nil
}

func dbAppToProto(dbApp database.WorkspaceApp, agent database.WorkspaceAgent, ownerName string, workspace database.Workspace) (*agentproto.WorkspaceApp, error) {
	sharingLevelRaw, ok := agentproto.WorkspaceApp_SharingLevel_value[strings.ToUpper(string(dbApp.SharingLevel))]
	if !ok {
		return nil, xerrors.Errorf("unknown app sharing level: %q", dbApp.SharingLevel)
	}

	healthRaw, ok := agentproto.WorkspaceApp_Health_value[strings.ToUpper(string(dbApp.Health))]
	if !ok {
		return nil, xerrors.Errorf("unknown app health: %q", dbApp.SharingLevel)
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
		SubdomainName: db2sdk.AppSubdomain(dbApp, agent.Name, workspace.Name, ownerName),
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
		ret[i] = &agentproto.WorkspaceAgentDevcontainer{
			Id:              dc.ID[:],
			WorkspaceFolder: dc.WorkspaceFolder,
			ConfigPath:      dc.ConfigPath,
		}
	}
	return ret
}
