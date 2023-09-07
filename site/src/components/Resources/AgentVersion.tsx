import { useRef, useState, FC } from "react";
import { makeStyles } from "@mui/styles";
import { WorkspaceAgent } from "api/typesGenerated";
import { getDisplayVersionStatus } from "utils/workspace";
import { AgentOutdatedTooltip } from "./AgentOutdatedTooltip";

export const AgentVersion: FC<{
  agent: WorkspaceAgent;
  serverVersion: string;
  onUpdate: () => void;
}> = ({ agent, serverVersion, onUpdate }) => {
  const styles = useStyles();
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
        className={styles.trigger}
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

const useStyles = makeStyles(() => ({
  trigger: {
    cursor: "pointer",
  },
}));
