package agentcontainers

import (
	"context"

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
	DisplayApps     []codersdk.DisplayApp
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

	resp, err := a.api.CreateSubAgent(ctx, &agentproto.CreateSubAgentRequest{
		Name:            agent.Name,
		Directory:       agent.Directory,
		Architecture:    agent.Architecture,
		OperatingSystem: agent.OperatingSystem,
		DisplayApps:     displayApps,
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
