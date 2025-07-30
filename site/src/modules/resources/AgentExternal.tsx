import type { Interpolation, Theme } from "@emotion/react";
import { API } from "api/api";
import type { Workspace, WorkspaceAgent } from "api/typesGenerated";
import isChromatic from "chromatic/isChromatic";
import { CodeExample } from "components/CodeExample/CodeExample";
import { useEffect, useState, type FC } from "react";

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
	const [externalAgentToken, setExternalAgentToken] = useState<string | null>(null);

	const origin = isChromatic() ? "https://example.com" : window.location.origin;
	let initScriptURL = `${origin}/api/v2/init-script`;
	if (agent.operating_system !== "linux" || agent.architecture !== "amd64") {
		initScriptURL = `${initScriptURL}?os=${agent.operating_system}&arch=${agent.architecture}`;
	}

	useEffect(() => {
		if (isExternalAgent && (agent.status === "timeout" || agent.status === "connecting")) {
			API.getWorkspaceAgentCredentials(workspace.id, agent.name).then((res) => {
				setExternalAgentToken(res.agent_token);
			});
		}
	}, [isExternalAgent, agent.status, workspace.id, agent.name]);

	return <section css={styles.externalAgentSection}>
		<p>
			Please run the following command to attach an agent to the {workspace.name} workspace:
		</p>
		<CodeExample
			code={`CODER_AGENT_TOKEN="${externalAgentToken}" curl -fsSL "${initScriptURL}" | sh`}
			secret={false}
			redactPattern={/CODER_AGENT_TOKEN="([^"]+)"/g}
			redactReplacement={`CODER_AGENT_TOKEN="********"`}
			redactShowButton={true}
		/>
	</section>;
};

const styles = {
	externalAgentSection: (theme) => ({
		fontSize: 16,
		color: theme.palette.text.secondary,
		paddingBottom: 8,
		lineHeight: 1.4,
	}),
} satisfies Record<string, Interpolation<Theme>>;
