import Tooltip from "@material-ui/core/Tooltip"
import { makeStyles } from "@material-ui/core/styles"
import { combineClasses } from "util/combineClasses"
import { WorkspaceAgent } from "api/typesGenerated"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"

const ConnectedStatus: React.FC = () => {
  const styles = useStyles()

  return (
    <Tooltip title="Connected">
      <div
        role="status"
        className={combineClasses([styles.status, styles.connected])}
      />
    </Tooltip>
  )
}

const DisconnectedStatus: React.FC = () => {
  const styles = useStyles()

  return (
    <Tooltip title="Disconnected">
      <div
        role="status"
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
  },

  disconnected: {
    backgroundColor: theme.palette.text.secondary,
  },

  "@keyframes pulse": {
    "0%": {
      opacity: 0.25,
    },
    "50%": {
      opacity: 1,
    },
    "100%": {
      opacity: 0.25,
    },
  },

  connecting: {
    backgroundColor: theme.palette.info.light,
    animation: "$pulse 1s ease-in-out forwards infinite",
  },
}))
