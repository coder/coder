import { type FC, useRef, useState } from "react";
import type { WorkspaceAgent } from "api/typesGenerated";
import { getDisplayVersionStatus } from "utils/workspace";
import { AgentOutdatedTooltip } from "./AgentOutdatedTooltip";

export const AgentVersion: FC<{
  agent: WorkspaceAgent;
  serverVersion: string;
  onUpdate: () => void;
}> = ({ agent, serverVersion, onUpdate }) => {
  const anchorRef = useRef<HTMLButtonElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const id = isOpen ? "version-outdated-popover" : undefined;
  const { outdated } = getDisplayVersionStatus(agent.version, serverVersion);

  if (!outdated) {
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
        Outdated
      </span>
      <AgentOutdatedTooltip
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
        agent={agent}
        serverVersion={serverVersion}
        onUpdate={onUpdate}
      />
    </>
  );
};
