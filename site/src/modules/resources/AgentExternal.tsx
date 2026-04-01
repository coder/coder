import { workspaceAgentCredentials } from "api/queries/workspaces";
import type { Workspace, WorkspaceAgent } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { CodeExample } from "components/CodeExample/CodeExample";
import { Loader } from "components/Loader/Loader";
import type { FC } from "react";
import { useQuery } from "react-query";

interface AgentExternalProps {
	agent: WorkspaceAgent;
	workspace: Workspace;
}

export const AgentExternal: FC<AgentExternalProps> = ({ agent, workspace }) => {
	const {
		data: credentials,
		error,
		isLoading,
		isError,
	} = useQuery(workspaceAgentCredentials(workspace.id, agent.name));

	if (isLoading) {
		return <Loader />;
	}

	if (isError) {
		return <ErrorAlert error={error} />;
	}

	return (
		<section className="text-base text-muted-foreground pb-2 leading-relaxed">
			<p>
				Please run the following command to attach an agent to the{" "}
				{workspace.name} workspace:
			</p>
			<CodeExample
				code={credentials?.command ?? ""}
				secret={false}
				redactPattern={/CODER_AGENT_TOKEN="([^"]+)"/g}
				redactReplacement={`CODER_AGENT_TOKEN="********"`}
				showRevealButton={true}
			/>
		</section>
	);
};
