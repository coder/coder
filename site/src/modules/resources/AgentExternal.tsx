import type { Interpolation, Theme } from "@emotion/react";
import { API } from "api/api";
import type { Workspace, WorkspaceAgent } from "api/typesGenerated";
import isChromatic from "chromatic/isChromatic";
import { CodeExample } from "components/CodeExample/CodeExample";
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

	const origin = isChromatic() ? "https://example.com" : window.location.origin;
	const initScriptURL = `${origin}/api/v2/init-script/${agent.operating_system}/${agent.architecture}`;
	useEffect(() => {
		if (
			isExternalAgent &&
			(agent.status === "timeout" || agent.status === "connecting")
		) {
			API.getWorkspaceAgentCredentials(workspace.id, agent.name).then((res) => {
				setExternalAgentToken(res.agent_token);
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
				code={`CODER_AGENT_TOKEN="${externalAgentToken}" curl -fsSL "${initScriptURL}" | sh`}
				secret={false}
				redactPattern={/CODER_AGENT_TOKEN="([^"]+)"/g}
				redactReplacement={`CODER_AGENT_TOKEN="********"`}
				showRevealButton={true}
			/>
		</section>
	);
};
