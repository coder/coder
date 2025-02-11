package cli

import (
	"sort"
	"sync"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) show() *serpent.Command {
	client := new(codersdk.Client)
	return &serpent.Command{
		Use:   "show <workspace>",
		Short: "Display details of a workspace's resources and agents",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			buildInfo, err := client.BuildInfo(inv.Context())
			if err != nil {
				return xerrors.Errorf("get server version: %w", err)
			}
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			options := cliui.WorkspaceResourcesOptions{
				WorkspaceName: workspace.Name,
				ServerVersion: buildInfo.Version,
			}
			if workspace.LatestBuild.Status == codersdk.WorkspaceStatusRunning {
				// Get listening ports for each agent.
				ports, devcontainers := fetchRuntimeResources(inv, client, workspace.LatestBuild.Resources...)
				options.ListeningPorts = ports
				options.Devcontainers = devcontainers
			}
			return cliui.WorkspaceResources(inv.Stdout, workspace.LatestBuild.Resources, options)
		},
	}
}

func fetchRuntimeResources(inv *serpent.Invocation, client *codersdk.Client, resources ...codersdk.WorkspaceResource) (map[uuid.UUID]codersdk.WorkspaceAgentListeningPortsResponse, map[uuid.UUID]codersdk.WorkspaceAgentListContainersResponse) {
	ports := make(map[uuid.UUID]codersdk.WorkspaceAgentListeningPortsResponse)
	devcontainers := make(map[uuid.UUID]codersdk.WorkspaceAgentListContainersResponse)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, res := range resources {
		for _, agent := range res.Agents {
			wg.Add(1)
			go func() {
				defer wg.Done()
				lp, err := client.WorkspaceAgentListeningPorts(inv.Context(), agent.ID)
				if err != nil {
					cliui.Warnf(inv.Stderr, "Failed to get listening ports for agent %s: %v", agent.Name, err)
				}
				sort.Slice(lp.Ports, func(i, j int) bool {
					return lp.Ports[i].Port < lp.Ports[j].Port
				})
				mu.Lock()
				ports[agent.ID] = lp
				mu.Unlock()
			}()
			wg.Add(1)
			go func() {
				defer wg.Done()
				dc, err := client.WorkspaceAgentListContainers(inv.Context(), agent.ID, map[string]string{
					// Labels set by VSCode Remote Containers and @devcontainers/cli.
					"devcontainer.config_file":  "",
					"devcontainer.local_folder": "",
				})
				if err != nil {
					cliui.Warnf(inv.Stderr, "Failed to get devcontainers for agent %s: %v", agent.Name, err)
				}
				mu.Lock()
				devcontainers[agent.ID] = dc
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	return ports, devcontainers
}
