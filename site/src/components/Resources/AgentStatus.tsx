import Tooltip from "@material-ui/core/Tooltip"
import { makeStyles } from "@material-ui/core/styles"
import { combineClasses } from "util/combineClasses"
import { WorkspaceAgent } from "api/typesGenerated"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { useTranslation } from "react-i18next"
import WarningRounded from "@material-ui/icons/WarningRounded"
import {
  HelpPopover,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/Tooltips/HelpTooltip"
import { useRef, useState } from "react"
import Link from "@material-ui/core/Link"

// If we think in the agent status and lifecycle into a single enum/state I’d
// say we would have: connecting, timeout, disconnected, connected:created,
// connected:starting, connected:start_timeout, connected:start_error,
// connected:ready, connected:shutting_down, connected:shutdown_timeout,
// connected:shutdown_error, connected:off.

const ReadyLifeCycle: React.FC = () => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <div
      role="status"
      aria-label={t("agentStatus.connected.ready")}
      className={combineClasses([styles.status, styles.connected])}
    />
  )
}

const StartingLifecycle: React.FC = () => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <Tooltip title={t("agentStatus.connected.starting")}>
      <div
        role="status"
        aria-label={t("agentStatus.connected.starting")}
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
  const { t } = useTranslation("workspacePage")

  return (
    <Tooltip title={t("agentStatus.connected.shuttingDown")}>
      <div
        role="status"
        aria-label={t("agentStatus.connected.shuttingDown")}
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

const OffLifeCycle: React.FC = () => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <Tooltip title={t("agentStatus.connected.off")}>
      <div
        role="status"
        aria-label={t("agentStatus.connected.off")}
        className={combineClasses([styles.status, styles.disconnected])}
      />
    </Tooltip>
  )
}

const ConnectedStatus: React.FC<{
  agent: WorkspaceAgent
}> = ({ agent }) => {
  // NOTE(mafredri): Keep this behind feature flag for the time-being,
  // if login_before_ready is false, the user has updated to
  // terraform-provider-coder v0.6.10 and opted in to the functionality.
  //
  // Remove check once documentation is in place and we do a breaking
  // release indicating startup script behavior has changed.
  // https://github.com/coder/coder/issues/5749
  if (agent.login_before_ready) {
    return <ReadyLifeCycle />
  }
  return (
    <ChooseOne>
      <Cond condition={agent.lifecycle_state === "ready"}>
        <ReadyLifeCycle />
      </Cond>
      <Cond condition={agent.lifecycle_state === "start_timeout"}>
        <StartTimeoutLifecycle agent={agent} />
      </Cond>
      <Cond condition={agent.lifecycle_state === "start_error"}>
        <StartErrorLifecycle agent={agent} />
      </Cond>
      <Cond condition={agent.lifecycle_state === "shutting_down"}>
        <ShuttingDownLifecycle agent={agent} />
      </Cond>
      <Cond condition={agent.lifecycle_state === "shutdown_timeout"}>
        <ShutdownTimeoutLifecycle agent={agent} />
      </Cond>
      <Cond condition={agent.lifecycle_state === "shutdown_error"}>
        <ShutdownErrorLifecycle agent={agent} />
      </Cond>
      <Cond condition={agent.lifecycle_state === "off"}>
        <OffLifeCycle />
      </Cond>
      <Cond>
        <StartingLifecycle />
      </Cond>
    </ChooseOne>
  )
}

const DisconnectedStatus: React.FC = () => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <Tooltip title={t("agentStatus.disconnected")}>
      <div
        role="status"
        aria-label={t("agentStatus.disconnected")}
        className={combineClasses([styles.status, styles.disconnected])}
      />
    </Tooltip>
  )
}

const ConnectingStatus: React.FC = () => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <Tooltip title={t("agentStatus.connecting")}>
      <div
        role="status"
        aria-label={t("agentStatus.connecting")}
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
    width: theme.spacing(2.5),
    height: theme.spacing(2.5),
    position: "relative",
    top: theme.spacing(1),
  },

  errorWarning: {
    color: theme.palette.error.main,
    width: theme.spacing(2.5),
    height: theme.spacing(2.5),
    position: "relative",
    top: theme.spacing(1),
  },
}))
