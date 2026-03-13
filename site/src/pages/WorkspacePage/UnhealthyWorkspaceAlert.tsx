import type * as TypesGen from "api/typesGenerated";
import type { WorkspaceAgentStatus } from "api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "components/Alert/Alert";
import { Link } from "components/Link/Link";
import type { FC } from "react";

interface UnhealthyWorkspaceAlertProps {
	workspace: TypesGen.Workspace;
	troubleshootingURL: string | undefined;
}

export const UnhealthyWorkspaceAlert: FC<UnhealthyWorkspaceAlertProps> = ({
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

	const agentLabel = failingAgentCount > 1 ? "agents" : "agent";

	let title: string;
	let subtitle: string;
	let message: string;

	if (statusSet.has("disconnected")) {
		title = `Workspace ${agentLabel} disconnected`;
		subtitle =
			failingAgentCount > 1
				? `${failingAgentCount} ${agentLabel} have lost connection.`
				: `the ${agentLabel} has lost connection.`;
		message =
			"Continue to wait and check the log output of your workspace for any errors. If the agent does not reconnect, restarting the workspace can be used to try again.";
	} else if (statusSet.has("timeout")) {
		title = "Your workspace is starting, but the agent has not yet connected";
		subtitle =
			failingAgentCount > 1
				? `${failingAgentCount} ${agentLabel} have not connected yet.`
				: `the ${agentLabel} has not connected yet.`;
		message =
			"The agent is taking longer than expected to connect. Continue to wait and check the log output of your workspace for any errors. If the agent does not connect, restarting the workspace can be used to try again.";
	} else if (hasShuttingDown) {
		title = `Workspace ${agentLabel} shutting down`;
		subtitle =
			failingAgentCount > 1
				? `${failingAgentCount} ${agentLabel} are shutting down.`
				: `the ${agentLabel} is shutting down.`;
		message = "The workspace agent is in the process of shutting down.";
	} else if (hasStartError) {
		title = "Startup script failed";
		subtitle =
			failingAgentCount > 1
				? `${failingAgentCount} ${agentLabel} have startup script errors.`
				: "a startup script exited with an error.";
		message =
			"Your workspace is running but a startup script exited with an error. Check the agent logs for more details. You can edit the startup script in your template to fix the issue.";
	} else {
		title = `Workspace ${agentLabel} not connected`;
		subtitle =
			failingAgentCount > 1
				? `${failingAgentCount} ${agentLabel} have not connected yet.`
				: `the ${agentLabel} has not connected yet.`;
		message =
			"Your workspace cannot be used until an agent connects. Continue to wait and check the log output of your workspace for any errors.";
	}

	return (
		<Alert severity="warning" prominent>
			<AlertTitle>{title}</AlertTitle>
			<AlertDescription>
				<p>Your workspace is running but {subtitle}</p>
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
