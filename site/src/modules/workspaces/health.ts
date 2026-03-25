import type { Workspace, WorkspaceAgentStatus } from "#/api/typesGenerated";

interface AgentHealthIssue {
	title: string;
	detail: string;
	severity: "info" | "warning";
	// Whether the alert should be visually prominent. Usually true for
	// warnings, but connection timeout and startup timeout are
	// exceptions (warning severity without prominent styling).
	prominent: boolean;
}

/**
 * Classifies the health issue affecting a workspace based on agent
 * status and lifecycle state. Returns a title and detail message
 * that accurately describes the root cause rather than using a
 * generic "unhealthy" label.
 */
export function getAgentHealthIssue(workspace: Workspace): AgentHealthIssue {
	const failingAgentCount = workspace.health.failing_agents.length;
	const statusSet = new Set<WorkspaceAgentStatus>();
	let hasStartError = false;
	let hasStartTimeout = false;
	let hasShutdownState = false;

	for (const resource of workspace.latest_build.resources) {
		for (const agent of resource.agents ?? []) {
			// Skip sub-agents (devcontainer agents) to match the
			// backend health calculation which excludes them.
			if (agent.parent_id !== null) {
				continue;
			}
			statusSet.add(agent.status);
			if (agent.lifecycle_state === "start_error") {
				hasStartError = true;
			}
			if (agent.lifecycle_state === "start_timeout") {
				hasStartTimeout = true;
			}
			if (
				agent.lifecycle_state === "shutting_down" ||
				agent.lifecycle_state === "shutdown_error" ||
				agent.lifecycle_state === "shutdown_timeout"
			) {
				hasShutdownState = true;
			}
		}
	}

	const plural = failingAgentCount > 1;

	if (statusSet.has("disconnected")) {
		return {
			title: plural
				? `${failingAgentCount} workspace agents have disconnected`
				: "Workspace agent has disconnected",
			detail:
				"Check the log output for errors. If agents do not reconnect, try restarting the workspace.",
			severity: "warning",
			prominent: true,
		};
	}

	if (statusSet.has("timeout")) {
		return {
			title: plural
				? `${failingAgentCount} agents are taking longer than expected to connect`
				: "Agent is taking longer than expected to connect",
			detail:
				"Continue to wait and check the log output for errors. If agents do not connect, try restarting the workspace.",
			severity: "warning",
			prominent: false,
		};
	}

	if (hasShutdownState) {
		return {
			title: plural
				? `${failingAgentCount} workspace agents are shutting down`
				: "Workspace agent is shutting down",
			detail: "The workspace is not available while agents shut down.",
			severity: "info",
			prominent: false,
		};
	}

	if (hasStartError) {
		return {
			title: plural
				? `Startup scripts failed on ${failingAgentCount} agents`
				: "Startup script failed",
			detail:
				"A startup script exited with an error. Check the agent logs for details.",
			severity: "warning",
			prominent: true,
		};
	}

	// The backend does not mark start_timeout agents as unhealthy on
	// their own (it treats it as a soft issue). This branch is only
	// reachable in multi-agent workspaces where a different agent
	// triggered the unhealthy flag but none of the higher-priority
	// branches matched.
	if (hasStartTimeout) {
		return {
			title: plural
				? `Startup scripts are taking longer than expected on ${failingAgentCount} agents`
				: "Startup script is taking longer than expected",
			detail:
				"A startup script has exceeded the expected time. Check the agent logs for details.",
			severity: "warning",
			prominent: false,
		};
	}

	return {
		title: plural
			? `${failingAgentCount} workspace agents are still connecting`
			: "Workspace agent is still connecting",
		detail: "Check the log output if the connection does not complete.",
		severity: "info",
		prominent: false,
	};
}
