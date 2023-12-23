import { type FC } from "react";
import type { WorkspaceAgent } from "api/typesGenerated";
import { agentVersionStatus, getDisplayVersionStatus } from "utils/workspace";
import { AgentOutdatedTooltip } from "./AgentOutdatedTooltip";

export const AgentVersion: FC<{
  agent: WorkspaceAgent;
  serverVersion: string;
  serverAPIVersion: string;
  onUpdate: () => void;
}> = ({ agent, serverVersion, serverAPIVersion, onUpdate }) => {
  const { status } = getDisplayVersionStatus(
    agent.version,
    serverVersion,
    agent.api_version,
    serverAPIVersion,
  );

  if (status === agentVersionStatus.Updated) {
    return <span>Updated</span>;
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
