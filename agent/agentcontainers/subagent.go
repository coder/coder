package agentcontainers

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

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

// CloneConfig makes a copy of SubAgent without ID and AuthToken. The
// name is inherited from the devcontainer.
func (s SubAgent) CloneConfig(dc codersdk.WorkspaceAgentDevcontainer) SubAgent {
	return SubAgent{
		Name:            dc.Name,
		Directory:       s.Directory,
		Architecture:    s.Architecture,
		OperatingSystem: s.OperatingSystem,
		DisplayApps:     slices.Clone(s.DisplayApps),
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
	Slug        string                             `json:"slug"`
	Command     *string                            `json:"command"`
	DisplayName *string                            `json:"displayName"`
	External    *bool                              `json:"external"`
	Group       *string                            `json:"group"`
	HealthCheck *SubAgentHealthCheck               `json:"healthCheck"`
	Hidden      *bool                              `json:"hidden"`
	Icon        *string                            `json:"icon"`
	OpenIn      *codersdk.WorkspaceAppOpenIn       `json:"openIn"`
	Order       *int32                             `json:"order"`
	Share       *codersdk.WorkspaceAppSharingLevel `json:"share"`
	Subdomain   *bool                              `json:"subdomain"`
	URL         *string                            `json:"url"`
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
	api    agentproto.DRPCAgentClient26
}

var _ SubAgentClient = (*subAgentAPIClient)(nil)

func NewSubAgentClientFromAPI(logger slog.Logger, agentAPI agentproto.DRPCAgentClient26) SubAgentClient {
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

func (a *subAgentAPIClient) Create(ctx context.Context, agent SubAgent) (SubAgent, error) {
	a.logger.Debug(ctx, "creating sub agent", slog.F("name", agent.Name), slog.F("directory", agent.Directory))

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
		var healthCheck *agentproto.CreateSubAgentRequest_App_Healthcheck
		if app.HealthCheck != nil {
			healthCheck = &agentproto.CreateSubAgentRequest_App_Healthcheck{
				Interval:  app.HealthCheck.Interval,
				Threshold: app.HealthCheck.Threshold,
				Url:       app.HealthCheck.URL,
			}
		}

		var openIn *agentproto.CreateSubAgentRequest_App_OpenIn
		if app.OpenIn != nil {
			switch *app.OpenIn {
			case codersdk.WorkspaceAppOpenInSlimWindow:
				openIn = agentproto.CreateSubAgentRequest_App_SLIM_WINDOW.Enum()
			case codersdk.WorkspaceAppOpenInTab:
				openIn = agentproto.CreateSubAgentRequest_App_TAB.Enum()
			default:
				return SubAgent{}, xerrors.Errorf("unexpected codersdk.WorkspaceAppOpenIn: %#v", app.OpenIn)
			}
		}

		var share *agentproto.CreateSubAgentRequest_App_SharingLevel
		if app.Share != nil {
			switch *app.Share {
			case codersdk.WorkspaceAppSharingLevelAuthenticated:
				share = agentproto.CreateSubAgentRequest_App_AUTHENTICATED.Enum()
			case codersdk.WorkspaceAppSharingLevelOwner:
				share = agentproto.CreateSubAgentRequest_App_OWNER.Enum()
			case codersdk.WorkspaceAppSharingLevelPublic:
				share = agentproto.CreateSubAgentRequest_App_PUBLIC.Enum()
			case codersdk.WorkspaceAppSharingLevelOrganization:
				share = agentproto.CreateSubAgentRequest_App_ORGANIZATION.Enum()
			default:
				return SubAgent{}, xerrors.Errorf("unexpected codersdk.WorkspaceAppSharingLevel: %#v", app.Share)
			}
		}

		apps = append(apps, &agentproto.CreateSubAgentRequest_App{
			Slug:        app.Slug,
			Command:     app.Command,
			DisplayName: app.DisplayName,
			External:    app.External,
			Group:       app.Group,
			Healthcheck: healthCheck,
			Hidden:      app.Hidden,
			Icon:        app.Icon,
			OpenIn:      openIn,
			Order:       app.Order,
			Share:       share,
			Subdomain:   app.Subdomain,
			Url:         app.URL,
		})
	}

	resp, err := a.api.CreateSubAgent(ctx, &agentproto.CreateSubAgentRequest{
		Name:            agent.Name,
		Directory:       agent.Directory,
		Architecture:    agent.Architecture,
		OperatingSystem: agent.OperatingSystem,
		DisplayApps:     displayApps,
		Apps:            apps,
	})
	if err != nil {
		return SubAgent{}, err
	}

	agent.Name = resp.Agent.Name
	agent.ID, err = uuid.FromBytes(resp.Agent.Id)
	if err != nil {
		return agent, err
	}
	agent.AuthToken, err = uuid.FromBytes(resp.Agent.AuthToken)
	if err != nil {
		return agent, err
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
