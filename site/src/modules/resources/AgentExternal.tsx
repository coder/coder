import { API } from "api/api";
import { getErrorMessage } from "api/errors";
import type { Workspace, WorkspaceAgent } from "api/typesGenerated";
import isChromatic from "chromatic/isChromatic";
import { CodeExample } from "components/CodeExample/CodeExample";
import { displayError } from "components/GlobalSnackbar/utils";
import { type FC, useEffect, useState } from "react";

interface AgentExternalProps {
	isExternalAgent: boolean;
	agent: WorkspaceAgent;
	workspace: Workspace;
}

export const AgentExternal: FC<AgentExternalProps> = ({
	isExternalAgent,
	agent,
	workspace,
}) => {
	const [externalAgentToken, setExternalAgentToken] = useState<string | null>(
		null,
	);
	const [command, setCommand] = useState<string | null>(null);

	const origin = isChromatic() ? "https://example.com" : window.location.origin;
	useEffect(() => {
		if (
			isExternalAgent &&
			(agent.status === "timeout" || agent.status === "connecting")
		) {
			API.getWorkspaceAgentCredentials(workspace.id, agent.name)
				.then((res) => {
					setExternalAgentToken(res.agent_token);
					setCommand(res.command);
				})
				.catch((err) => {
					displayError(
						getErrorMessage(err, "Failed to get external agent credentials"),
					);
				});
		}
	}, [isExternalAgent, agent.status, workspace.id, agent.name]);

	return (
		<section className="text-base text-muted-foreground pb-2 leading-relaxed">
			<p>
				Please run the following command to attach an agent to the{" "}
				{workspace.name} workspace:
			</p>
			<CodeExample
				code={command ?? ""}
				secret={false}
				redactPattern={/CODER_AGENT_TOKEN="([^"]+)"/g}
				redactReplacement={`CODER_AGENT_TOKEN="********"`}
				showRevealButton={true}
			/>
		</section>
	);
};
