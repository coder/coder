import { makeStyles } from "@material-ui/core/styles"
import { Skeleton } from "@material-ui/lab"
import { PortForwardButton } from "components/PortForwardButton/PortForwardButton"
import { FC } from "react"
import { Workspace, WorkspaceAgent } from "../../api/typesGenerated"
import { AppLink } from "../AppLink/AppLink"
import { SSHButton } from "../SSHButton/SSHButton"
import { Stack } from "../Stack/Stack"
import { TerminalLink } from "../TerminalLink/TerminalLink"
import { AgentLatency } from "./AgentLatency"
import { AgentVersion } from "./AgentVersion"
import { Maybe } from "components/Conditionals/Maybe"
import { AgentStatus } from "./AgentStatus"
import { AppLinkSkeleton } from "components/AppLink/AppLinkSkeleton"
import { useTranslation } from "react-i18next"
import { VSCodeDesktopButton } from "components/VSCodeDesktopButton/VSCodeDesktopButton"

export interface AgentRowProps {
  agent: WorkspaceAgent
  workspace: Workspace
  applicationsHost: string | undefined
  showApps: boolean
  hideSSHButton?: boolean
  hideVSCodeDesktopButton?: boolean
  serverVersion: string
  onUpdateAgent: () => void
}

export const AgentRow: FC<AgentRowProps> = ({
  agent,
  workspace,
  applicationsHost,
  showApps,
  hideSSHButton,
  hideVSCodeDesktopButton,
  serverVersion,
  onUpdateAgent,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("agent")

  return (
    <Stack
      key={agent.id}
      direction="row"
      alignItems="center"
      justifyContent="space-between"
      className={styles.agentRow}
      spacing={4}
    >
      <Stack direction="row" alignItems="baseline">
        <div className={styles.agentStatusWrapper}>
          <AgentStatus agent={agent} />
        </div>
        <div>
          <div className={styles.agentName}>{agent.name}</div>
          <Stack
            direction="row"
            alignItems="baseline"
            className={styles.agentData}
            spacing={1}
          >
            <span className={styles.agentOS}>{agent.operating_system}</span>

            <Maybe condition={agent.status === "connected"}>
              <AgentVersion
                agent={agent}
                serverVersion={serverVersion}
                onUpdate={onUpdateAgent}
              />
            </Maybe>

            <AgentLatency agent={agent} />

            <Maybe condition={agent.status === "connecting"}>
              <Skeleton width={160} variant="text" />
              <Skeleton width={36} variant="text" />
            </Maybe>

            <Maybe condition={agent.status === "timeout"}>
              {t("unableToConnect")}
            </Maybe>
          </Stack>
        </div>
      </Stack>

      <Stack
        direction="row"
        alignItems="center"
        spacing={0.5}
        wrap="wrap"
        maxWidth="750px"
      >
        {showApps && agent.status === "connected" && (
          <>
            {agent.apps.map((app) => (
              <AppLink
                key={app.slug}
                appsHost={applicationsHost}
                app={app}
                agent={agent}
                workspace={workspace}
              />
            ))}

            <TerminalLink
              workspaceName={workspace.name}
              agentName={agent.name}
              userName={workspace.owner_name}
            />
            {!hideSSHButton && (
              <SSHButton
                workspaceName={workspace.name}
                agentName={agent.name}
              />
            )}
            {!hideVSCodeDesktopButton && (
              <VSCodeDesktopButton
                userName={workspace.owner_name}
                workspaceName={workspace.name}
                agentName={agent.name}
                folderPath={agent.expanded_directory}
              />
            )}
            {applicationsHost !== undefined && applicationsHost !== "" && (
              <PortForwardButton
                host={applicationsHost}
                workspaceName={workspace.name}
                agentId={agent.id}
                agentName={agent.name}
                username={workspace.owner_name}
              />
            )}
          </>
        )}
        {showApps && agent.status === "connecting" && (
          <>
            <AppLinkSkeleton width={84} />
            <AppLinkSkeleton width={112} />
          </>
        )}
      </Stack>
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  agentRow: {
    padding: theme.spacing(3, 4),
    backgroundColor: theme.palette.background.paperLight,
    fontSize: 16,

    "&:not(:last-child)": {
      borderBottom: `1px solid ${theme.palette.divider}`,
    },
  },

  agentStatusWrapper: {
    width: theme.spacing(4.5),
    display: "flex",
    justifyContent: "center",
  },

  agentName: {
    fontWeight: 600,
  },

  agentOS: {
    textTransform: "capitalize",
  },

  agentData: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.5),
  },
}))
