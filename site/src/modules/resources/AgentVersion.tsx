import type { WorkspaceAgent } from "api/typesGenerated";
import type { FC } from "react";
import { agentVersionStatus, getDisplayVersionStatus } from "utils/workspace";
import { AgentOutdatedTooltip } from "./AgentOutdatedTooltip";
import { buildInfo } from "api/queries/buildInfo";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useQuery } from "react-query";

interface AgentVersionProps {
	agent: WorkspaceAgent;
	onUpdate: () => void;
}

export const AgentVersion: FC<AgentVersionProps> = ({ agent, onUpdate }) => {
	const { metadata } = useEmbeddedMetadata();
	const { data: build } = useQuery(buildInfo(metadata["build-info"]));
	const serverVersion = build?.version ?? "";
	const apiServerVersion = build?.agent_api_version ?? "";

	const { status } = getDisplayVersionStatus(
		agent.version,
		serverVersion,
		agent.api_version,
		apiServerVersion,
	);

	if (status === agentVersionStatus.Updated) {
		return null;
	}

	return (
		<AgentOutdatedTooltip
			agent={agent}
			serverVersion={serverVersion}
			status={status}
			onUpdate={onUpdate}
		/>
	);
};
