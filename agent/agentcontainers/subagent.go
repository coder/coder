package agentcontainers

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
)

// SubAgent represents an agent running in a dev container.
type SubAgent struct {
	ID              uuid.UUID
	Name            string
	AuthToken       uuid.UUID
	Directory       string
	Architecture    string
	OperatingSystem string
	Apps            []SubAgentApp
	DisplayApps     []codersdk.DisplayApp
}

// CloneConfig makes a copy of SubAgent using configuration from the
// devcontainer. The ID is inherited from dc.SubagentID if present, and
// the name is inherited from the devcontainer. AuthToken is not copied.
func (s SubAgent) CloneConfig(dc codersdk.WorkspaceAgentDevcontainer) SubAgent {
	return SubAgent{
		ID:              dc.SubagentID.UUID,
		Name:            dc.Name,
		Directory:       s.Directory,
		Architecture:    s.Architecture,
		OperatingSystem: s.OperatingSystem,
		DisplayApps:     slices.Clone(s.DisplayApps),
		Apps:            slices.Clone(s.Apps),
	}
}

func (s SubAgent) EqualConfig(other SubAgent) bool {
	return s.Name == other.Name &&
		s.Directory == other.Directory &&
		s.Architecture == other.Architecture &&
		s.OperatingSystem == other.OperatingSystem &&
		slices.Equal(s.DisplayApps, other.DisplayApps) &&
		slices.Equal(s.Apps, other.Apps)
}

type SubAgentApp struct {
	Slug        string                            `json:"slug"`
	Command     string                            `json:"command"`
	DisplayName string                            `json:"displayName"`
	External    bool                              `json:"external"`
	Group       string                            `json:"group"`
	HealthCheck SubAgentHealthCheck               `json:"healthCheck"`
	Hidden      bool                              `json:"hidden"`
	Icon        string                            `json:"icon"`
	OpenIn      codersdk.WorkspaceAppOpenIn       `json:"openIn"`
	Order       int32                             `json:"order"`
	Share       codersdk.WorkspaceAppSharingLevel `json:"share"`
	Subdomain   bool                              `json:"subdomain"`
	URL         string                            `json:"url"`
}

func (app SubAgentApp) ToProtoApp() (*agentproto.CreateSubAgentRequest_App, error) {
	proto := agentproto.CreateSubAgentRequest_App{
		Slug:      app.Slug,
		External:  &app.External,
		Hidden:    &app.Hidden,
		Order:     &app.Order,
		Subdomain: &app.Subdomain,
	}

	if app.Command != "" {
		proto.Command = &app.Command
	}
	if app.DisplayName != "" {
		proto.DisplayName = &app.DisplayName
	}
	if app.Group != "" {
		proto.Group = &app.Group
	}
	if app.Icon != "" {
		proto.Icon = &app.Icon
	}
	if app.URL != "" {
		proto.Url = &app.URL
	}

	if app.HealthCheck.URL != "" {
		proto.Healthcheck = &agentproto.CreateSubAgentRequest_App_Healthcheck{
			Interval:  app.HealthCheck.Interval,
			Threshold: app.HealthCheck.Threshold,
			Url:       app.HealthCheck.URL,
		}
	}

	if app.OpenIn != "" {
		switch app.OpenIn {
		case codersdk.WorkspaceAppOpenInSlimWindow:
			proto.OpenIn = agentproto.CreateSubAgentRequest_App_SLIM_WINDOW.Enum()
		case codersdk.WorkspaceAppOpenInTab:
			proto.OpenIn = agentproto.CreateSubAgentRequest_App_TAB.Enum()
		default:
			return nil, xerrors.Errorf("unexpected codersdk.WorkspaceAppOpenIn: %#v", app.OpenIn)
		}
	}

	if app.Share != "" {
		switch app.Share {
		case codersdk.WorkspaceAppSharingLevelAuthenticated:
			proto.Share = agentproto.CreateSubAgentRequest_App_AUTHENTICATED.Enum()
		case codersdk.WorkspaceAppSharingLevelOwner:
			proto.Share = agentproto.CreateSubAgentRequest_App_OWNER.Enum()
		case codersdk.WorkspaceAppSharingLevelPublic:
			proto.Share = agentproto.CreateSubAgentRequest_App_PUBLIC.Enum()
		case codersdk.WorkspaceAppSharingLevelOrganization:
			proto.Share = agentproto.CreateSubAgentRequest_App_ORGANIZATION.Enum()
		default:
			return nil, xerrors.Errorf("unexpected codersdk.WorkspaceAppSharingLevel: %#v", app.Share)
		}
	}

	return &proto, nil
}

type SubAgentHealthCheck struct {
	Interval  int32  `json:"interval"`
	Threshold int32  `json:"threshold"`
	URL       string `json:"url"`
}

// SubAgentClient is an interface for managing sub agents and allows
// changing the implementation without having to deal with the
// agentproto package directly.
type SubAgentClient interface {
	// List returns a list of all agents.
	List(ctx context.Context) ([]SubAgent, error)
	// Create adds a new agent.
	Create(ctx context.Context, agent SubAgent) (SubAgent, error)
	// Delete removes an agent by its ID.
	Delete(ctx context.Context, id uuid.UUID) error
}

// NewSubAgentClient returns a SubAgentClient that uses the provided
// agent API client.
type subAgentAPIClient struct {
	logger slog.Logger
	api    agentproto.DRPCAgentClient28
}

var _ SubAgentClient = (*subAgentAPIClient)(nil)

func NewSubAgentClientFromAPI(logger slog.Logger, agentAPI agentproto.DRPCAgentClient28) SubAgentClient {
	if agentAPI == nil {
		panic("developer error: agentAPI cannot be nil")
	}
	return &subAgentAPIClient{
		logger: logger.Named("subagentclient"),
		api:    agentAPI,
	}
}

func (a *subAgentAPIClient) List(ctx context.Context) ([]SubAgent, error) {
	a.logger.Debug(ctx, "listing sub agents")
	resp, err := a.api.ListSubAgents(ctx, &agentproto.ListSubAgentsRequest{})
	if err != nil {
		return nil, err
	}

	agents := make([]SubAgent, len(resp.Agents))
	for i, agent := range resp.Agents {
		id, err := uuid.FromBytes(agent.GetId())
		if err != nil {
			return nil, err
		}
		authToken, err := uuid.FromBytes(agent.GetAuthToken())
		if err != nil {
			return nil, err
		}
		agents[i] = SubAgent{
			ID:        id,
			Name:      agent.GetName(),
			AuthToken: authToken,
		}
	}
	return agents, nil
}

func (a *subAgentAPIClient) Create(ctx context.Context, agent SubAgent) (_ SubAgent, err error) {
	a.logger.Debug(ctx, "creating sub agent", slog.F("name", agent.Name), slog.F("directory", agent.Directory))

	var id []byte
	if agent.ID != uuid.Nil {
		id = agent.ID[:]
	}

	displayApps := make([]agentproto.CreateSubAgentRequest_DisplayApp, 0, len(agent.DisplayApps))
	for _, displayApp := range agent.DisplayApps {
		var app agentproto.CreateSubAgentRequest_DisplayApp
		switch displayApp {
		case codersdk.DisplayAppPortForward:
			app = agentproto.CreateSubAgentRequest_PORT_FORWARDING_HELPER
		case codersdk.DisplayAppSSH:
			app = agentproto.CreateSubAgentRequest_SSH_HELPER
		case codersdk.DisplayAppVSCodeDesktop:
			app = agentproto.CreateSubAgentRequest_VSCODE
		case codersdk.DisplayAppVSCodeInsiders:
			app = agentproto.CreateSubAgentRequest_VSCODE_INSIDERS
		case codersdk.DisplayAppWebTerminal:
			app = agentproto.CreateSubAgentRequest_WEB_TERMINAL
		default:
			return SubAgent{}, xerrors.Errorf("unexpected codersdk.DisplayApp: %#v", displayApp)
		}

		displayApps = append(displayApps, app)
	}

	apps := make([]*agentproto.CreateSubAgentRequest_App, 0, len(agent.Apps))
	for _, app := range agent.Apps {
		protoApp, err := app.ToProtoApp()
		if err != nil {
			return SubAgent{}, xerrors.Errorf("convert app: %w", err)
		}

		apps = append(apps, protoApp)
	}

	resp, err := a.api.CreateSubAgent(ctx, &agentproto.CreateSubAgentRequest{
		Name:            agent.Name,
		Directory:       agent.Directory,
		Architecture:    agent.Architecture,
		OperatingSystem: agent.OperatingSystem,
		DisplayApps:     displayApps,
		Apps:            apps,
		Id:              id,
	})
	if err != nil {
		return SubAgent{}, err
	}
	defer func() {
		if err != nil {
			// Best effort.
			_, _ = a.api.DeleteSubAgent(ctx, &agentproto.DeleteSubAgentRequest{
				Id: resp.GetAgent().GetId(),
			})
		}
	}()

	agent.Name = resp.GetAgent().GetName()
	agent.ID, err = uuid.FromBytes(resp.GetAgent().GetId())
	if err != nil {
		return SubAgent{}, err
	}
	agent.AuthToken, err = uuid.FromBytes(resp.GetAgent().GetAuthToken())
	if err != nil {
		return SubAgent{}, err
	}

	for _, appError := range resp.GetAppCreationErrors() {
		app := apps[appError.GetIndex()]

		a.logger.Warn(ctx, "unable to create app",
			slog.F("agent_name", agent.Name),
			slog.F("agent_id", agent.ID),
			slog.F("directory", agent.Directory),
			slog.F("app_slug", app.Slug),
			slog.F("field", appError.GetField()),
			slog.F("error", appError.GetError()),
		)
	}

	return agent, nil
}

func (a *subAgentAPIClient) Delete(ctx context.Context, id uuid.UUID) error {
	a.logger.Debug(ctx, "deleting sub agent", slog.F("id", id.String()))
	_, err := a.api.DeleteSubAgent(ctx, &agentproto.DeleteSubAgentRequest{
		Id: id[:],
	})
	return err
}

// noopSubAgentClient is a SubAgentClient that does nothing.
type noopSubAgentClient struct{}

var _ SubAgentClient = noopSubAgentClient{}

func (noopSubAgentClient) List(_ context.Context) ([]SubAgent, error) {
	return nil, nil
}

func (noopSubAgentClient) Create(_ context.Context, _ SubAgent) (SubAgent, error) {
	return SubAgent{}, xerrors.New("noopSubAgentClient does not support creating sub agents")
}

func (noopSubAgentClient) Delete(_ context.Context, _ uuid.UUID) error {
	return xerrors.New("noopSubAgentClient does not support deleting sub agents")
}
