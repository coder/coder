package agentapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
)

type ManifestAPI struct {
	AccessURL                       *url.URL
	AppHostname                     string
	AgentInactiveDisconnectTimeout  time.Duration
	AgentFallbackTroubleshootingURL string
	ExternalAuthConfigs             []*externalauth.Config
	DisableDirectConnections        bool
	DerpForceWebSockets             bool

	AgentFn            func(context.Context) (database.WorkspaceAgent, error)
	Database           database.Store
	DerpMapFn          func() *tailcfg.DERPMap
	TailnetCoordinator *atomic.Pointer[tailnet.Coordinator]
}

func (a *ManifestAPI) GetManifest(ctx context.Context, _ *agentproto.GetManifestRequest) (*agentproto.Manifest, error) {
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}

	apiAgent, err := db2sdk.WorkspaceAgent(
		a.DerpMapFn(), *a.TailnetCoordinator.Load(), workspaceAgent, nil, nil, nil, a.AgentInactiveDisconnectTimeout,
		a.AgentFallbackTroubleshootingURL,
	)
	if err != nil {
		return nil, xerrors.Errorf("converting workspace agent: %w", err)
	}

	var (
		dbApps    []database.WorkspaceApp
		scripts   []database.WorkspaceAgentScript
		metadata  []database.WorkspaceAgentMetadatum
		resource  database.WorkspaceResource
		build     database.WorkspaceBuild
		workspace database.Workspace
		owner     database.User
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
			Keys:             nil,
		})
		return err
	})
	eg.Go(func() (err error) {
		resource, err = a.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
		if err != nil {
			return xerrors.Errorf("getting resource by id: %w", err)
		}
		build, err = a.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
		if err != nil {
			return xerrors.Errorf("getting workspace build by job id: %w", err)
		}
		workspace, err = a.Database.GetWorkspaceByID(ctx, build.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("getting workspace by id: %w", err)
		}
		owner, err = a.Database.GetUserByID(ctx, workspace.OwnerID)
		if err != nil {
			return xerrors.Errorf("getting workspace owner by id: %w", err)
		}
		return err
	})
	err = eg.Wait()
	if err != nil {
		return nil, xerrors.Errorf("fetching workspace agent data: %w", err)
	}

	appHost := httpapi.ApplicationURL{
		AppSlugOrPort: "{{port}}",
		AgentName:     workspaceAgent.Name,
		WorkspaceName: workspace.Name,
		Username:      owner.Username,
	}
	vscodeProxyURI := a.AccessURL.Scheme + "://" + strings.ReplaceAll(a.AppHostname, "*", appHost.String())
	if a.AppHostname == "" {
		vscodeProxyURI += a.AccessURL.Hostname()
	}
	if a.AccessURL.Port() != "" {
		vscodeProxyURI += fmt.Sprintf(":%s", a.AccessURL.Port())
	}

	var gitAuthConfigs uint32
	for _, cfg := range a.ExternalAuthConfigs {
		if codersdk.EnhancedExternalAuthProvider(cfg.Type).Git() {
			gitAuthConfigs++
		}
	}

	apps, err := agentproto.DBAppsToProto(dbApps, workspaceAgent, owner.Username, workspace)
	if err != nil {
		return nil, xerrors.Errorf("converting workspace apps: %w", err)
	}

	return &agentproto.Manifest{
		AgentId:                  workspaceAgent.ID[:],
		OwnerUsername:            owner.Username,
		WorkspaceId:              workspace.ID[:],
		GitAuthConfigs:           gitAuthConfigs,
		EnvironmentVariables:     apiAgent.EnvironmentVariables,
		Directory:                apiAgent.Directory,
		VsCodePortProxyUri:       vscodeProxyURI,
		MotdPath:                 workspaceAgent.MOTDFile,
		DisableDirectConnections: a.DisableDirectConnections,
		DerpForceWebsockets:      a.DerpForceWebSockets,

		DerpMap:  tailnet.DERPMapToProto(a.DerpMapFn()),
		Scripts:  agentproto.DBAgentScriptsToProto(scripts),
		Apps:     apps,
		Metadata: agentproto.DBAgentMetadataToProtoDescription(metadata),
	}, nil
}
