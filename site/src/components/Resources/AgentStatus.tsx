import Tooltip from "@material-ui/core/Tooltip"
import { makeStyles } from "@material-ui/core/styles"
import { combineClasses } from "util/combineClasses"
import { WorkspaceAgent } from "api/typesGenerated"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { useTranslation } from "react-i18next"

const ConnectedStatus: React.FC = () => {
  const styles = useStyles()
  const { t } = useTranslation("workspacePage")

  return (
    <Tooltip title={t("agentStatus.connected")}>
      <div
        role="status"
        aria-label={t("agentStatus.connected")}
        className={combineClasses([styles.status, styles.connected])}
      />
    </Tooltip>
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

export const AgentStatus: React.FC<{ agent: WorkspaceAgent }> = ({ agent }) => {
  return (
    <ChooseOne>
      <Cond condition={agent.status === "connected"}>
        <ConnectedStatus />
      </Cond>
      <Cond condition={agent.status === "disconnected"}>
        <DisconnectedStatus />
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
}))
