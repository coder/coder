package cliui

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jedib0t/go-pretty/v6/table"
	"golang.org/x/mod/semver"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
)

var (
	pipeMid = "├"
	pipeEnd = "└"
)

type WorkspaceResourcesOptions struct {
	WorkspaceName  string
	HideAgentState bool
	HideAccess     bool
	Title          string
	ServerVersion  string
	ListeningPorts map[uuid.UUID]codersdk.WorkspaceAgentListeningPortsResponse
	Devcontainers  map[uuid.UUID]codersdk.WorkspaceAgentListContainersResponse
}

// WorkspaceResources displays the connection status and tree-view of provided resources.
// ┌────────────────────────────────────────────────────────────────────────────┐
// │ RESOURCE                     STATUS               ACCESS                   │
// ├────────────────────────────────────────────────────────────────────────────┤
// │ google_compute_disk.root                                                   │
// ├────────────────────────────────────────────────────────────────────────────┤
// │ google_compute_instance.dev                                                │
// │ └─ dev (linux, amd64)        ⦾ connecting [10s]    coder ssh dev.dev       │
// ├────────────────────────────────────────────────────────────────────────────┤
// │ kubernetes_pod.dev                                                         │
// │ ├─ go (linux, amd64)         ⦿ connected           coder ssh dev.go        │
// │ └─ postgres (linux, amd64)   ⦾ disconnected [4s]   coder ssh dev.postgres  │
// └────────────────────────────────────────────────────────────────────────────┘
func WorkspaceResources(writer io.Writer, resources []codersdk.WorkspaceResource, options WorkspaceResourcesOptions) error {
	// Sort resources by type for consistent output.
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Type < resources[j].Type
	})

	tableWriter := table.NewWriter()
	if options.Title != "" {
		tableWriter.SetTitle(options.Title)
	}
	tableWriter.SetStyle(table.StyleLight)
	tableWriter.Style().Options.SeparateColumns = false
	row := table.Row{"Resource"}
	if !options.HideAgentState {
		row = append(row, "Status")
		row = append(row, "Health")
		row = append(row, "Version")
	}
	if !options.HideAccess {
		row = append(row, "Access")
	}
	tableWriter.AppendHeader(row)

	totalAgents := 0
	for _, resource := range resources {
		totalAgents += len(resource.Agents)
	}

	for _, resource := range resources {
		if resource.Type == "random_string" {
			// Hide resources that aren't substantial to a user!
			// This is an unfortunate case, and we should allow
			// callers to hide resources eventually.
			continue
		}
		resourceAddress := resource.Type + "." + resource.Name

		// Sort agents by name for consistent output.
		sort.Slice(resource.Agents, func(i, j int) bool {
			return resource.Agents[i].Name < resource.Agents[j].Name
		})

		// Display a line for the resource.
		tableWriter.AppendRow(table.Row{
			Bold(resourceAddress),
			"",
			"",
			"",
		})
		// Display all agents associated with the resource.
		for index, agent := range resource.Agents {
			tableWriter.AppendRow(renderAgentRow(agent, index, totalAgents, options))
			for _, row := range renderListeningPorts(options, agent.ID, index, totalAgents) {
				tableWriter.AppendRow(row)
			}
			for _, row := range renderDevcontainers(options, agent.ID, index, totalAgents) {
				tableWriter.AppendRow(row)
			}
		}
		tableWriter.AppendSeparator()
	}
	_, err := fmt.Fprintln(writer, tableWriter.Render())
	return err
}

func renderAgentRow(agent codersdk.WorkspaceAgent, index, totalAgents int, options WorkspaceResourcesOptions) table.Row {
	row := table.Row{
		// These tree from a resource!
		fmt.Sprintf("%s─ %s (%s, %s)", renderPipe(index, totalAgents), agent.Name, agent.OperatingSystem, agent.Architecture),
	}
	if !options.HideAgentState {
		var agentStatus, agentHealth, agentVersion string
		if !options.HideAgentState {
			agentStatus = renderAgentStatus(agent)
			agentHealth = renderAgentHealth(agent)
			agentVersion = renderAgentVersion(agent.Version, options.ServerVersion)
		}
		row = append(row, agentStatus, agentHealth, agentVersion)
	}
	if !options.HideAccess {
		sshCommand := "coder ssh " + options.WorkspaceName
		if totalAgents > 1 {
			sshCommand += "." + agent.Name
		}
		sshCommand = pretty.Sprint(DefaultStyles.Code, sshCommand)
		row = append(row, sshCommand)
	}
	return row
}

func renderListeningPorts(wro WorkspaceResourcesOptions, agentID uuid.UUID, idx, total int) []table.Row {
	var rows []table.Row
	if wro.ListeningPorts == nil {
		return []table.Row{}
	}
	lp, ok := wro.ListeningPorts[agentID]
	if !ok || len(lp.Ports) == 0 {
		return []table.Row{}
	}
	rows = append(rows, table.Row{
		fmt.Sprintf("   %s─ Open Ports", renderPipe(idx, total)),
	})
	for idx, port := range lp.Ports {
		rows = append(rows, renderPortRow(port, idx, len(lp.Ports)))
	}
	return rows
}

func renderPortRow(port codersdk.WorkspaceAgentListeningPort, idx, total int) table.Row {
	var sb strings.Builder
	_, _ = sb.WriteString("      ")
	_, _ = sb.WriteString(renderPipe(idx, total))
	_, _ = sb.WriteString("─ ")
	_, _ = sb.WriteString(pretty.Sprintf(DefaultStyles.Code, "%5d/%s", port.Port, port.Network))
	if port.ProcessName != "" {
		_, _ = sb.WriteString(pretty.Sprintf(DefaultStyles.Keyword, " [%s]", port.ProcessName))
	}
	return table.Row{sb.String()}
}

func renderDevcontainers(wro WorkspaceResourcesOptions, agentID uuid.UUID, index, totalAgents int) []table.Row {
	var rows []table.Row
	if wro.Devcontainers == nil {
		return []table.Row{}
	}
	dc, ok := wro.Devcontainers[agentID]
	if !ok || len(dc.Containers) == 0 {
		return []table.Row{}
	}
	rows = append(rows, table.Row{
		fmt.Sprintf("   %s─ %s", renderPipe(index, totalAgents), "Devcontainers"),
	})
	for idx, container := range dc.Containers {
		rows = append(rows, renderDevcontainerRow(container, idx, len(dc.Containers)))
	}
	return rows
}

func renderDevcontainerRow(container codersdk.WorkspaceAgentDevcontainer, index, total int) table.Row {
	var row table.Row
	var sb strings.Builder
	_, _ = sb.WriteString("      ")
	_, _ = sb.WriteString(renderPipe(index, total))
	_, _ = sb.WriteString("─ ")
	_, _ = sb.WriteString(pretty.Sprintf(DefaultStyles.Code, "%s", container.FriendlyName))
	row = append(row, sb.String())
	sb.Reset()
	if container.Running {
		_, _ = sb.WriteString(pretty.Sprintf(DefaultStyles.Keyword, "(%s)", container.Status))
	} else {
		_, _ = sb.WriteString(pretty.Sprintf(DefaultStyles.Error, "(%s)", container.Status))
	}
	row = append(row, sb.String())
	sb.Reset()
	// "health" is not applicable here.
	row = append(row, sb.String())
	_, _ = sb.WriteString(container.Image)
	row = append(row, sb.String())
	return row
}

func renderAgentStatus(agent codersdk.WorkspaceAgent) string {
	switch agent.Status {
	case codersdk.WorkspaceAgentConnecting:
		since := dbtime.Now().Sub(agent.CreatedAt)
		return pretty.Sprint(DefaultStyles.Warn, "⦾ connecting") + " " +
			pretty.Sprint(DefaultStyles.Placeholder, "["+strconv.Itoa(int(since.Seconds()))+"s]")
	case codersdk.WorkspaceAgentDisconnected:
		since := dbtime.Now().Sub(*agent.DisconnectedAt)
		return pretty.Sprint(DefaultStyles.Error, "⦾ disconnected") + " " +
			pretty.Sprint(DefaultStyles.Placeholder, "["+strconv.Itoa(int(since.Seconds()))+"s]")
	case codersdk.WorkspaceAgentTimeout:
		since := dbtime.Now().Sub(agent.CreatedAt)
		return fmt.Sprintf(
			"%s %s",
			pretty.Sprint(DefaultStyles.Warn, "⦾ timeout"),
			pretty.Sprint(DefaultStyles.Placeholder, "["+strconv.Itoa(int(since.Seconds()))+"s]"),
		)
	case codersdk.WorkspaceAgentConnected:
		return pretty.Sprint(DefaultStyles.Keyword, "⦿ connected")
	default:
		return pretty.Sprint(DefaultStyles.Warn, "○ unknown")
	}
}

func renderAgentHealth(agent codersdk.WorkspaceAgent) string {
	if agent.Health.Healthy {
		return pretty.Sprint(DefaultStyles.Keyword, "✔ healthy")
	}
	return pretty.Sprint(DefaultStyles.Error, "✘ "+agent.Health.Reason)
}

func renderAgentVersion(agentVersion, serverVersion string) string {
	if agentVersion == "" {
		agentVersion = "(unknown)"
	}
	if !semver.IsValid(serverVersion) || !semver.IsValid(agentVersion) {
		return pretty.Sprint(DefaultStyles.Placeholder, agentVersion)
	}
	outdated := semver.Compare(agentVersion, serverVersion) < 0
	if outdated {
		return pretty.Sprint(DefaultStyles.Warn, agentVersion+" (outdated)")
	}
	return pretty.Sprint(DefaultStyles.Keyword, agentVersion)
}

func renderPipe(idx, total int) string {
	if idx == total-1 {
		return pipeEnd
	}
	return pipeMid
}
