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
	"github.com/coder/coder/v2/coderd/util/slice"
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
	ShowDetails    bool
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
		for _, agent := range resource.Agents {
			if !agent.ParentID.Valid {
				totalAgents++
			}
		}
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
		agents := slice.Filter(resource.Agents, func(agent codersdk.WorkspaceAgent) bool {
			return !agent.ParentID.Valid
		})
		for index, agent := range agents {
			tableWriter.AppendRow(renderAgentRow(agent, index, totalAgents, options))
			for _, row := range renderListeningPorts(options, agent.ID, index, totalAgents) {
				tableWriter.AppendRow(row)
			}
			for _, row := range renderDevcontainers(resources, options, agent.ID, index, totalAgents) {
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
		if totalAgents > 1 || len(options.Devcontainers) > 0 {
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

func renderDevcontainers(resources []codersdk.WorkspaceResource, wro WorkspaceResourcesOptions, agentID uuid.UUID, index, totalAgents int) []table.Row {
	var rows []table.Row
	if wro.Devcontainers == nil {
		return []table.Row{}
	}
	dc, ok := wro.Devcontainers[agentID]
	if !ok || len(dc.Devcontainers) == 0 {
		return []table.Row{}
	}
	rows = append(rows, table.Row{
		fmt.Sprintf("   %s─ %s", renderPipe(index, totalAgents), "Devcontainers"),
	})
	for idx, devcontainer := range dc.Devcontainers {
		rows = append(rows, renderDevcontainerRow(resources, devcontainer, idx, len(dc.Devcontainers), wro)...)
	}
	return rows
}

func renderDevcontainerRow(resources []codersdk.WorkspaceResource, devcontainer codersdk.WorkspaceAgentDevcontainer, index, total int, wro WorkspaceResourcesOptions) []table.Row {
	var rows []table.Row

	// If the devcontainer is running and has an associated agent, we want to
	// display the agent's details. Otherwise, we just display the devcontainer
	// name and status.
	var subAgent *codersdk.WorkspaceAgent
	displayName := devcontainer.Name
	if devcontainer.Agent != nil && devcontainer.Status == codersdk.WorkspaceAgentDevcontainerStatusRunning {
		for _, resource := range resources {
			if agent, found := slice.Find(resource.Agents, func(agent codersdk.WorkspaceAgent) bool {
				return agent.ID == devcontainer.Agent.ID
			}); found {
				subAgent = &agent
				break
			}
		}
		if subAgent != nil {
			displayName = subAgent.Name
			displayName += fmt.Sprintf(" (%s, %s)", subAgent.OperatingSystem, subAgent.Architecture)
		}
	}

	if devcontainer.Container != nil {
		displayName += " " + pretty.Sprint(DefaultStyles.Keyword, "["+devcontainer.Container.FriendlyName+"]")
	}

	// Build the main row.
	row := table.Row{
		fmt.Sprintf("      %s─ %s", renderPipe(index, total), displayName),
	}

	// Add status, health, and version columns.
	if !wro.HideAgentState {
		if subAgent != nil {
			row = append(row, renderAgentStatus(*subAgent))
			row = append(row, renderAgentHealth(*subAgent))
			row = append(row, renderAgentVersion(subAgent.Version, wro.ServerVersion))
		} else {
			row = append(row, renderDevcontainerStatus(devcontainer.Status))
			row = append(row, "") // No health for devcontainer without agent.
			row = append(row, "") // No version for devcontainer without agent.
		}
	}

	// Add access column.
	if !wro.HideAccess {
		if subAgent != nil {
			accessString := fmt.Sprintf("coder ssh %s.%s", wro.WorkspaceName, subAgent.Name)
			row = append(row, pretty.Sprint(DefaultStyles.Code, accessString))
		} else {
			row = append(row, "") // No access for devcontainers without agent.
		}
	}

	rows = append(rows, row)

	// Add error message if present.
	if errorMessage := devcontainer.Error; errorMessage != "" {
		// Cap error message length for display.
		if !wro.ShowDetails && len(errorMessage) > 80 {
			errorMessage = errorMessage[:79] + "…"
		}
		errorRow := table.Row{
			"         × " + pretty.Sprint(DefaultStyles.Error, errorMessage),
			"",
			"",
			"",
		}
		if !wro.HideAccess {
			errorRow = append(errorRow, "")
		}
		rows = append(rows, errorRow)
	}

	// Add listening ports for the devcontainer agent.
	if subAgent != nil {
		portRows := renderListeningPorts(wro, subAgent.ID, index, total)
		for _, portRow := range portRows {
			// Adjust indentation for ports under devcontainer agent.
			if len(portRow) > 0 {
				if str, ok := portRow[0].(string); ok {
					portRow[0] = "      " + str // Add extra indentation.
				}
			}
			rows = append(rows, portRow)
		}
	}

	return rows
}

func renderDevcontainerStatus(status codersdk.WorkspaceAgentDevcontainerStatus) string {
	switch status {
	case codersdk.WorkspaceAgentDevcontainerStatusRunning:
		return pretty.Sprint(DefaultStyles.Keyword, "▶ running")
	case codersdk.WorkspaceAgentDevcontainerStatusStopped:
		return pretty.Sprint(DefaultStyles.Placeholder, "⏹ stopped")
	case codersdk.WorkspaceAgentDevcontainerStatusStarting:
		return pretty.Sprint(DefaultStyles.Warn, "⧗ starting")
	case codersdk.WorkspaceAgentDevcontainerStatusError:
		return pretty.Sprint(DefaultStyles.Error, "✘ error")
	default:
		return pretty.Sprint(DefaultStyles.Placeholder, "○ "+string(status))
	}
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
