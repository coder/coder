import Tooltip from "@mui/material/Tooltip"
import { makeStyles } from "@mui/styles"
import { combineClasses } from "utils/combineClasses"
import { WorkspaceAgent } from "api/typesGenerated"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { useTranslation } from "react-i18next"
import WarningRounded from "@mui/icons-material/WarningRounded"
import {
  HelpPopover,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip"
import { useRef, useState } from "react"
import Link from "@mui/material/Link"

// If we think in the agent status and lifecycle into a single enum/state I’d
// say we would have: connecting, timeout, disconnected, connected:created,
// connected:starting, connected:start_timeout, connected:start_error,
// connected:ready, connected:shutting_down, connected:shutdown_timeout,
// connected:shutdown_error, connected:off.

const ReadyLifecycle = () => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <div
      role="status"
      data-testid="agent-status-ready"
      aria-label={t("agentStatus.connected.ready") || "Ready"}
      className={combineClasses([styles.status, styles.connected])}
    />
  )
}

const StartingLifecycle: React.FC = () => {
  const styles = useStyles()

  return (
    <Tooltip title="Starting...">
      <div
        role="status"
        aria-label="Starting..."
        className={combineClasses([styles.status, styles.connecting])}
      />
    </Tooltip>
  )
}

const StartTimeoutLifecycle: React.FC<{
  agent: WorkspaceAgent
}> = ({ agent }) => {
  const { t } = useTranslation("agent")
  const styles = useStyles()
  const anchorRef = useRef<SVGSVGElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const id = isOpen ? "timeout-popover" : undefined

  return (
    <>
      <WarningRounded
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        onMouseLeave={() => setIsOpen(false)}
        role="status"
        aria-label={t("status.startTimeout")}
        className={styles.timeoutWarning}
      />
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipTitle>{t("startTimeoutTooltip.title")}</HelpTooltipTitle>
        <HelpTooltipText>
          {t("startTimeoutTooltip.message")}{" "}
          <Link
            target="_blank"
            rel="noreferrer"
            href={agent.troubleshooting_url}
          >
            {t("startTimeoutTooltip.link")}
          </Link>
          .
        </HelpTooltipText>
      </HelpPopover>
    </>
  )
}

const StartErrorLifecycle: React.FC<{
  agent: WorkspaceAgent
}> = ({ agent }) => {
  const { t } = useTranslation("agent")
  const styles = useStyles()
  const anchorRef = useRef<SVGSVGElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const id = isOpen ? "timeout-popover" : undefined

  return (
    <>
      <WarningRounded
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        onMouseLeave={() => setIsOpen(false)}
        role="status"
        aria-label={t("status.error")}
        className={styles.errorWarning}
      />
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipTitle>{t("startErrorTooltip.title")}</HelpTooltipTitle>
        <HelpTooltipText>
          {t("startErrorTooltip.message")}{" "}
          <Link
            target="_blank"
            rel="noreferrer"
            href={agent.troubleshooting_url}
          >
            {t("startErrorTooltip.link")}
          </Link>
          .
        </HelpTooltipText>
      </HelpPopover>
    </>
  )
}

const ShuttingDownLifecycle: React.FC = () => {
  const styles = useStyles()

  return (
    <Tooltip title="Stopping...">
      <div
        role="status"
        aria-label="Stopping..."
        className={combineClasses([styles.status, styles.connecting])}
      />
    </Tooltip>
  )
}

const ShutdownTimeoutLifecycle: React.FC<{
  agent: WorkspaceAgent
}> = ({ agent }) => {
  const { t } = useTranslation("agent")
  const styles = useStyles()
  const anchorRef = useRef<SVGSVGElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const id = isOpen ? "timeout-popover" : undefined

  return (
    <>
      <WarningRounded
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        onMouseLeave={() => setIsOpen(false)}
        role="status"
        aria-label={t("status.shutdownTimeout")}
        className={styles.timeoutWarning}
      />
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipTitle>{t("shutdownTimeoutTooltip.title")}</HelpTooltipTitle>
        <HelpTooltipText>
          {t("shutdownTimeoutTooltip.message")}{" "}
          <Link
            target="_blank"
            rel="noreferrer"
            href={agent.troubleshooting_url}
          >
            {t("shutdownTimeoutTooltip.link")}
          </Link>
          .
        </HelpTooltipText>
      </HelpPopover>
    </>
  )
}

const ShutdownErrorLifecycle: React.FC<{
  agent: WorkspaceAgent
}> = ({ agent }) => {
  const { t } = useTranslation("agent")
  const styles = useStyles()
  const anchorRef = useRef<SVGSVGElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const id = isOpen ? "timeout-popover" : undefined

  return (
    <>
      <WarningRounded
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        onMouseLeave={() => setIsOpen(false)}
        role="status"
        aria-label={t("status.error")}
        className={styles.errorWarning}
      />
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipTitle>{t("shutdownErrorTooltip.title")}</HelpTooltipTitle>
        <HelpTooltipText>
          {t("shutdownErrorTooltip.message")}{" "}
          <Link
            target="_blank"
            rel="noreferrer"
            href={agent.troubleshooting_url}
          >
            {t("shutdownErrorTooltip.link")}
          </Link>
          .
        </HelpTooltipText>
      </HelpPopover>
    </>
  )
}

const OffLifecycle: React.FC = () => {
  const styles = useStyles()

  return (
    <Tooltip title="Stopped">
      <div
        role="status"
        aria-label="Stopped"
        className={combineClasses([styles.status, styles.disconnected])}
      />
    </Tooltip>
  )
}

const ConnectedStatus: React.FC<{
  agent: WorkspaceAgent
}> = ({ agent }) => {
  switch (agent.startup_script_behavior) {
    case "non-blocking":
      return <ReadyLifecycle />
    case "blocking":
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
      )
  }
}

const DisconnectedStatus: React.FC = () => {
  const styles = useStyles()

  return (
    <Tooltip title="Disconnected">
      <div
        role="status"
        aria-label="Disconnected"
        className={combineClasses([styles.status, styles.disconnected])}
      />
    </Tooltip>
  )
}

const ConnectingStatus: React.FC = () => {
  const styles = useStyles()

  return (
    <Tooltip title="Connecting...">
      <div
        role="status"
        aria-label="Connecting..."
        className={combineClasses([styles.status, styles.connecting])}
      />
    </Tooltip>
  )
}

const TimeoutStatus: React.FC<{
  agent: WorkspaceAgent
}> = ({ agent }) => {
  const { t } = useTranslation("agent")
  const styles = useStyles()
  const anchorRef = useRef<SVGSVGElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const id = isOpen ? "timeout-popover" : undefined

  return (
    <>
      <WarningRounded
        ref={anchorRef}
        onMouseEnter={() => setIsOpen(true)}
        onMouseLeave={() => setIsOpen(false)}
        role="status"
        aria-label={t("status.timeout")}
        className={styles.timeoutWarning}
      />
      <HelpPopover
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onOpen={() => setIsOpen(true)}
        onClose={() => setIsOpen(false)}
      >
        <HelpTooltipTitle>{t("timeoutTooltip.title")}</HelpTooltipTitle>
        <HelpTooltipText>
          {t("timeoutTooltip.message")}{" "}
          <Link
            target="_blank"
            rel="noreferrer"
            href={agent.troubleshooting_url}
          >
            {t("timeoutTooltip.link")}
          </Link>
          .
        </HelpTooltipText>
      </HelpPopover>
    </>
  )
}

export const AgentStatus: React.FC<{
  agent: WorkspaceAgent
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
  )
}

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
}))
