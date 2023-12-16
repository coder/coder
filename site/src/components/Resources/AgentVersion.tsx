import { type FC, useRef, useState } from "react";
import type { WorkspaceAgent } from "api/typesGenerated";
import { agentVersionStatus, getDisplayVersionStatus } from "utils/workspace";
import { AgentOutdatedTooltip } from "./AgentOutdatedTooltip";

export const AgentVersion: FC<{
  agent: WorkspaceAgent;
  serverVersion: string;
  serverAPIVersion: string;
  onUpdate: () => void;
}> = ({ agent, serverVersion, serverAPIVersion, onUpdate }) => {
  const anchorRef = useRef<HTMLButtonElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const id = isOpen ? "version-outdated-popover" : undefined;
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
    <>
      <span
        role="presentation"
        aria-label="latency"
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        onMouseLeave={() => setIsOpen(false)}
        css={{ cursor: "pointer" }}
      >
        {status === agentVersionStatus.Outdated ? "Outdated" : "Deprecated"}
      </span>
      <AgentOutdatedTooltip
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
        agent={agent}
        serverVersion={serverVersion}
        status={status}
        onUpdate={onUpdate}
      />
    </>
  );
};
