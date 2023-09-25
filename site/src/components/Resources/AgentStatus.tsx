import Tooltip from "@mui/material/Tooltip";
import { makeStyles } from "@mui/styles";
import { combineClasses } from "utils/combineClasses";
import { WorkspaceAgent } from "api/typesGenerated";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import WarningRounded from "@mui/icons-material/WarningRounded";
import {
  HelpPopover,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import { useRef, useState } from "react";
import Link from "@mui/material/Link";

// If we think in the agent status and lifecycle into a single enum/state I’d
// say we would have: connecting, timeout, disconnected, connected:created,
// connected:starting, connected:start_timeout, connected:start_error,
// connected:ready, connected:shutting_down, connected:shutdown_timeout,
// connected:shutdown_error, connected:off.

const ReadyLifecycle = () => {
  const styles = useStyles();

  return (
    <div
      role="status"
      data-testid="agent-status-ready"
      aria-label="Ready"
      className={combineClasses([styles.status, styles.connected])}
    />
  );
};

const StartingLifecycle: React.FC = () => {
  const styles = useStyles();

  return (
    <Tooltip title="Starting...">
      <div
        role="status"
        aria-label="Starting..."
        className={combineClasses([styles.status, styles.connecting])}
      />
    </Tooltip>
  );
};

const StartTimeoutLifecycle: React.FC<{
  agent: WorkspaceAgent;
}> = ({ agent }) => {
  const styles = useStyles();
  const anchorRef = useRef<SVGSVGElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const id = isOpen ? "timeout-popover" : undefined;

  return (
    <>
      <WarningRounded
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        onMouseLeave={() => setIsOpen(false)}
        role="status"
        aria-label="Start timeout"
        className={styles.timeoutWarning}
      />
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipTitle>Agent is taking too long to start</HelpTooltipTitle>
        <HelpTooltipText>
          We noticed this agent is taking longer than expected to start.{" "}
          <Link
            target="_blank"
            rel="noreferrer"
            href={agent.troubleshooting_url}
          >
            Troubleshoot
          </Link>
          .
        </HelpTooltipText>
      </HelpPopover>
    </>
  );
};

const StartErrorLifecycle: React.FC<{
  agent: WorkspaceAgent;
}> = ({ agent }) => {
  const styles = useStyles();
  const anchorRef = useRef<SVGSVGElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const id = isOpen ? "timeout-popover" : undefined;

  return (
    <>
      <WarningRounded
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        onMouseLeave={() => setIsOpen(false)}
        role="status"
        aria-label="Start error"
        className={styles.errorWarning}
      />
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipTitle>Error starting the agent</HelpTooltipTitle>
        <HelpTooltipText>
          Something went wrong during the agent startup.{" "}
          <Link
            target="_blank"
            rel="noreferrer"
            href={agent.troubleshooting_url}
          >
            Troubleshoot
          </Link>
          .
        </HelpTooltipText>
      </HelpPopover>
    </>
  );
};

const ShuttingDownLifecycle: React.FC = () => {
  const styles = useStyles();

  return (
    <Tooltip title="Stopping...">
      <div
        role="status"
        aria-label="Stopping..."
        className={combineClasses([styles.status, styles.connecting])}
      />
    </Tooltip>
  );
};

const ShutdownTimeoutLifecycle: React.FC<{
  agent: WorkspaceAgent;
}> = ({ agent }) => {
  const styles = useStyles();
  const anchorRef = useRef<SVGSVGElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const id = isOpen ? "timeout-popover" : undefined;

  return (
    <>
      <WarningRounded
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        onMouseLeave={() => setIsOpen(false)}
        role="status"
        aria-label="Stop timeout"
        className={styles.timeoutWarning}
      />
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipTitle>Agent is taking too long to stop</HelpTooltipTitle>
        <HelpTooltipText>
          We noticed this agent is taking longer than expected to stop.{" "}
          <Link
            target="_blank"
            rel="noreferrer"
            href={agent.troubleshooting_url}
          >
            Troubleshoot
          </Link>
          .
        </HelpTooltipText>
      </HelpPopover>
    </>
  );
};

const ShutdownErrorLifecycle: React.FC<{
  agent: WorkspaceAgent;
}> = ({ agent }) => {
  const styles = useStyles();
  const anchorRef = useRef<SVGSVGElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const id = isOpen ? "timeout-popover" : undefined;

  return (
    <>
      <WarningRounded
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        onMouseLeave={() => setIsOpen(false)}
        role="status"
        aria-label="Stop error"
        className={styles.errorWarning}
      />
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipTitle>Error stopping the agent</HelpTooltipTitle>
        <HelpTooltipText>
          Something went wrong while trying to stop the agent.{" "}
          <Link
            target="_blank"
            rel="noreferrer"
            href={agent.troubleshooting_url}
          >
            Troubleshoot
          </Link>
          .
        </HelpTooltipText>
      </HelpPopover>
    </>
  );
};

const OffLifecycle: React.FC = () => {
  const styles = useStyles();

  return (
    <Tooltip title="Stopped">
      <div
        role="status"
        aria-label="Stopped"
        className={combineClasses([styles.status, styles.disconnected])}
      />
    </Tooltip>
  );
};

const ConnectedStatus: React.FC<{
  agent: WorkspaceAgent;
}> = ({ agent }) => {
  // This is to support legacy agents that do not support
  // reporting the lifecycle_state field.
  if (agent.scripts.length === 0) {
    return <ReadyLifecycle />;
  }
  return (
    <ChooseOne>
      <Cond condition={agent.lifecycle_state === "ready"}>
        <ReadyLifecycle />
      </Cond>
      <Cond condition={agent.lifecycle_state === "start_timeout"}>
        <StartTimeoutLifecycle agent={agent} />
      </Cond>
      <Cond condition={agent.lifecycle_state === "start_error"}>
        <StartErrorLifecycle agent={agent} />
      </Cond>
      <Cond condition={agent.lifecycle_state === "shutting_down"}>
        <ShuttingDownLifecycle />
      </Cond>
      <Cond condition={agent.lifecycle_state === "shutdown_timeout"}>
        <ShutdownTimeoutLifecycle agent={agent} />
      </Cond>
      <Cond condition={agent.lifecycle_state === "shutdown_error"}>
        <ShutdownErrorLifecycle agent={agent} />
      </Cond>
      <Cond condition={agent.lifecycle_state === "off"}>
        <OffLifecycle />
      </Cond>
      <Cond>
        <StartingLifecycle />
      </Cond>
    </ChooseOne>
  );
};

const DisconnectedStatus: React.FC = () => {
  const styles = useStyles();

  return (
    <Tooltip title="Disconnected">
      <div
        role="status"
        aria-label="Disconnected"
        className={combineClasses([styles.status, styles.disconnected])}
      />
    </Tooltip>
  );
};

const ConnectingStatus: React.FC = () => {
  const styles = useStyles();

  return (
    <Tooltip title="Connecting...">
      <div
        role="status"
        aria-label="Connecting..."
        className={combineClasses([styles.status, styles.connecting])}
      />
    </Tooltip>
  );
};

const TimeoutStatus: React.FC<{
  agent: WorkspaceAgent;
}> = ({ agent }) => {
  const styles = useStyles();
  const anchorRef = useRef<SVGSVGElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const id = isOpen ? "timeout-popover" : undefined;

  return (
    <>
      <WarningRounded
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        onMouseLeave={() => setIsOpen(false)}
        role="status"
        aria-label="Timeout"
        className={styles.timeoutWarning}
      />
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipTitle>Agent is taking too long to connect</HelpTooltipTitle>
        <HelpTooltipText>
          We noticed this agent is taking longer than expected to connect.{" "}
          <Link
            target="_blank"
            rel="noreferrer"
            href={agent.troubleshooting_url}
          >
            Troubleshoot
          </Link>
          .
        </HelpTooltipText>
      </HelpPopover>
    </>
  );
};

export const AgentStatus: React.FC<{
  agent: WorkspaceAgent;
}> = ({ agent }) => {
  return (
    <ChooseOne>
      <Cond condition={agent.status === "connected"}>
        <ConnectedStatus agent={agent} />
      </Cond>
      <Cond condition={agent.status === "disconnected"}>
        <DisconnectedStatus />
      </Cond>
      <Cond condition={agent.status === "timeout"}>
        <TimeoutStatus agent={agent} />
      </Cond>
      <Cond>
        <ConnectingStatus />
      </Cond>
    </ChooseOne>
  );
};

const useStyles = makeStyles((theme) => ({
  status: {
    width: theme.spacing(1),
    height: theme.spacing(1),
    borderRadius: "100%",
    flexShrink: 0,
  },

  connected: {
    backgroundColor: theme.palette.success.light,
    boxShadow: `0 0 12px 0 ${theme.palette.success.light}`,
  },

  disconnected: {
    backgroundColor: theme.palette.text.secondary,
  },

  "@keyframes pulse": {
    "0%": {
      opacity: 1,
    },
    "50%": {
      opacity: 0.4,
    },
    "100%": {
      opacity: 1,
    },
  },

  connecting: {
    backgroundColor: theme.palette.info.light,
    animation: "$pulse 1.5s 0.5s ease-in-out forwards infinite",
  },

  timeoutWarning: {
    color: theme.palette.warning.light,
    width: theme.spacing(2),
    height: theme.spacing(2),
    position: "relative",
  },

  errorWarning: {
    color: theme.palette.error.main,
    width: theme.spacing(2),
    height: theme.spacing(2),
    position: "relative",
  },
}));
