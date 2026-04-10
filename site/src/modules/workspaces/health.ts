import type { WorkspaceAgent } from "#/api/typesGenerated";

/**
 * Canonical messages for startup and shutdown script issues.
 * Used by the per-agent-row tooltips in AgentStatus; the
 * start-related entries are also shared with per-agent health
 * classification in getAgentHealthIssue.
 */
export const agentScriptMessages = {
	start_error: {
		title: "Startup script failed",
		detail:
			"A startup script exited with an error. Check the agent logs for details.",
	},
	start_timeout: {
		title: "Startup script is taking longer than expected",
		detail:
			"A startup script has exceeded the expected time. Check the agent logs for details.",
	},
	shutdown_error: {
		title: "Shutdown script failed",
		detail:
			"A shutdown script exited with an error. Check the agent logs for details.",
	},
	shutdown_timeout: {
		title: "Shutdown script is taking longer than expected",
		detail:
			"A shutdown script has exceeded the expected time. Check the agent logs for details.",
	},
} as const;

/**
 * Canonical messages for agent connection issues (the agent
 * process connecting to the Coder control plane).
 */
export const agentConnectionMessages = {
	connecting: {
		title: "Workspace agent is connecting",
		detail:
			"The workspace agent has not connected yet. Wait for it to connect or check the logs if it does not.",
	},
	timeout: {
		title: "Agent is taking longer than expected to connect",
		detail:
			"Continue to wait and check the log output for errors. If agents do not connect, try restarting the workspace.",
	},
	disconnected: {
		title: "Workspace agent has disconnected",
		detail:
			"Check the log output for errors. If agents do not reconnect, try restarting the workspace.",
	},
} as const;

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
 * Classifies the health issue for a single failing top-level agent.
 * This avoids workspace-wide aggregation so alerts describe one
 * concrete agent state at a time.
 */
export function getAgentHealthIssue(
	agent: WorkspaceAgent,
): AgentHealthIssue | undefined {
	if (agent.status === "disconnected") {
		return {
			title: agentConnectionMessages.disconnected.title,
			detail: agentConnectionMessages.disconnected.detail,
			severity: "warning",
			prominent: false,
		};
	}

	if (agent.status === "timeout") {
		return {
			title: agentConnectionMessages.timeout.title,
			detail: agentConnectionMessages.timeout.detail,
			severity: "warning",
			prominent: false,
		};
	}

	if (
		agent.lifecycle_state === "shutting_down" ||
		agent.lifecycle_state === "shutdown_error" ||
		agent.lifecycle_state === "shutdown_timeout"
	) {
		return {
			title: "Workspace agent is shutting down",
			detail: "The workspace is not available while agents shut down.",
			severity: "info",
			prominent: false,
		};
	}

	if (agent.lifecycle_state === "start_error") {
		return {
			title: agentScriptMessages.start_error.title,
			detail: agentScriptMessages.start_error.detail,
			severity: "warning",
			prominent: true,
		};
	}

	if (agent.lifecycle_state === "start_timeout") {
		return {
			title: agentScriptMessages.start_timeout.title,
			detail: agentScriptMessages.start_timeout.detail,
			severity: "warning",
			prominent: false,
		};
	}

	if (agent.status === "connecting") {
		return {
			title: agentConnectionMessages.connecting.title,
			detail: agentConnectionMessages.connecting.detail,
			severity: "info",
			prominent: false,
		};
	}

	return undefined;
}
