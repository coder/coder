import type * as TypesGen from "api/typesGenerated";
import type { WorkspaceAgentStatus } from "api/typesGenerated";
import type { FC } from "react";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";

interface WorkspaceAlertProps {
	workspace: TypesGen.Workspace;
	troubleshootingURL: string | undefined;
}

export const WorkspaceAlert: FC<WorkspaceAlertProps> = ({
	workspace,
	troubleshootingURL,
}) => {
	const failingAgentCount = workspace.health.failing_agents.length;
	const statusSet = new Set<WorkspaceAgentStatus>();
	let hasStartError = false;
	let hasShuttingDown = false;

	for (const resource of workspace.latest_build.resources) {
		for (const agent of resource.agents ?? []) {
			statusSet.add(agent.status);
			if (agent.lifecycle_state === "start_error") {
				hasStartError = true;
			}
			if (
				agent.lifecycle_state === "shutting_down" ||
				agent.lifecycle_state === "shutdown_error" ||
				agent.lifecycle_state === "shutdown_timeout"
			) {
				hasShuttingDown = true;
			}
		}
	}

	const plural = failingAgentCount > 1;

	let title: string;
	let message: string;
	let severity: "info" | "warning" = "warning";
	let prominent = true;

	if (statusSet.has("disconnected")) {
		title = plural
			? `${failingAgentCount} workspace agents have disconnected`
			: "Workspace agent has disconnected";
		message =
			"Check the log output for errors. If the agent does not reconnect, try restarting the workspace.";
	} else if (statusSet.has("timeout")) {
		title = plural
			? `${failingAgentCount} agents are taking longer than expected to connect`
			: "Agent is taking longer than expected to connect";
		message =
			"Continue to wait and check the log output for errors. If the agent does not connect, try restarting the workspace.";
		severity = "warning";
		prominent = false;
	} else if (hasShuttingDown) {
		title = plural
			? `${failingAgentCount} workspace agents are shutting down`
			: "Workspace agent is shutting down";
		message = "The workspace is not available while the agent shuts down.";
		severity = "info";
		prominent = false;
	} else if (hasStartError) {
		title = plural
			? `Startup scripts failed on ${failingAgentCount} agents`
			: "Startup script failed";
		message =
			"The workspace is running but a startup script exited with an error. Check the agent logs for details.";
	} else {
		title = plural
			? `${failingAgentCount} workspace agents are still connecting`
			: "Workspace agent is still connecting";
		message =
			"The workspace agent is still connecting. Check the log output if the connection does not complete.";
		severity = "info";
		prominent = false;
	}

	return (
		<Alert severity={severity} prominent={prominent}>
			<AlertTitle>{title}</AlertTitle>
			<AlertDescription>
				<p>{message}</p>
				<p>
					{troubleshootingURL && (
						<Link href={troubleshootingURL} target="_blank">
							View docs to troubleshoot
						</Link>
					)}
				</p>
			</AlertDescription>
		</Alert>
	);
};
