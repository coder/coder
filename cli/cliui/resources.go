package cliui

import (
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/jedib0t/go-pretty/v6/table"
	"golang.org/x/mod/semver"

	"github.com/coder/coder/coderd/database"

	"github.com/coder/coder/codersdk"
)

type WorkspaceResourcesOptions struct {
	WorkspaceName  string
	HideAgentState bool
	HideAccess     bool
	Title          string
	ServerVersion  string
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
			Styles.Bold.Render(resourceAddress),
			"",
			"",
		})
		// Display all agents associated with the resource.
		for index, agent := range resource.Agents {
			pipe := "├"
			if index == len(resource.Agents)-1 {
				pipe = "└"
			}
			row := table.Row{
				// These tree from a resource!
				fmt.Sprintf("%s─ %s (%s, %s)", pipe, agent.Name, agent.OperatingSystem, agent.Architecture),
			}
			if !options.HideAgentState {
				var agentStatus string
				var agentVersion string
				if !options.HideAgentState {
					agentStatus = renderAgentStatus(agent)
					agentVersion = renderAgentVersion(agent.Version, options.ServerVersion)
				}
				row = append(row, agentStatus, agentVersion)
			}
			if !options.HideAccess {
				sshCommand := "coder ssh " + options.WorkspaceName
				if totalAgents > 1 {
					sshCommand += "." + agent.Name
				}
				sshCommand = Styles.Code.Render(sshCommand)
				row = append(row, sshCommand)
			}
			tableWriter.AppendRow(row)
		}
		tableWriter.AppendSeparator()
	}
	_, err := fmt.Fprintln(writer, tableWriter.Render())
	return err
}

func renderAgentStatus(agent codersdk.WorkspaceAgent) string {
	switch agent.Status {
	case codersdk.WorkspaceAgentConnecting:
		since := database.Now().Sub(agent.CreatedAt)
		return Styles.Warn.Render("⦾ connecting") + " " +
			Styles.Placeholder.Render("["+strconv.Itoa(int(since.Seconds()))+"s]")
	case codersdk.WorkspaceAgentDisconnected:
		since := database.Now().Sub(*agent.DisconnectedAt)
		return Styles.Error.Render("⦾ disconnected") + " " +
			Styles.Placeholder.Render("["+strconv.Itoa(int(since.Seconds()))+"s]")
	case codersdk.WorkspaceAgentTimeout:
		since := database.Now().Sub(agent.CreatedAt)
		return fmt.Sprintf(
			"%s %s",
			Styles.Warn.Render("⦾ timeout"),
			Styles.Placeholder.Render("["+strconv.Itoa(int(since.Seconds()))+"s]"),
		)
	case codersdk.WorkspaceAgentConnected:
		return Styles.Keyword.Render("⦿ connected")
	default:
		return Styles.Warn.Render("○ unknown")
	}
}

func renderAgentVersion(agentVersion, serverVersion string) string {
	if agentVersion == "" {
		agentVersion = "(unknown)"
	}
	if !semver.IsValid(serverVersion) || !semver.IsValid(agentVersion) {
		return Styles.Placeholder.Render(agentVersion)
	}
	outdated := semver.Compare(agentVersion, serverVersion) < 0
	if outdated {
		return Styles.Warn.Render(agentVersion + " (outdated)")
	}
	return Styles.Keyword.Render(agentVersion)
}
