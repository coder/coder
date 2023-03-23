import Link from "@material-ui/core/Link"
import Popover from "@material-ui/core/Popover"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import PlayCircleOutlined from "@material-ui/icons/PlayCircleFilledOutlined"
import VisibilityOffOutlined from "@material-ui/icons/VisibilityOffOutlined"
import VisibilityOutlined from "@material-ui/icons/VisibilityOutlined"
import { Skeleton } from "@material-ui/lab"
import { useMachine } from "@xstate/react"
import { AppLinkSkeleton } from "components/AppLink/AppLinkSkeleton"
import { Maybe } from "components/Conditionals/Maybe"
import { LogLine, logLineHeight } from "components/Logs/Logs"
import { PortForwardButton } from "components/PortForwardButton/PortForwardButton"
import { VSCodeDesktopButton } from "components/VSCodeDesktopButton/VSCodeDesktopButton"
import {
  FC,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react"
import { useTranslation } from "react-i18next"
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter"
import { darcula } from "react-syntax-highlighter/dist/cjs/styles/prism"
import AutoSizer from "react-virtualized-auto-sizer"
import { FixedSizeList as List, ListOnScrollProps } from "react-window"
import {
  LineWithID,
  workspaceAgentLogsMachine,
} from "xServices/workspaceAgentLogs/workspaceAgentLogsXService"
import { Workspace, WorkspaceAgent } from "../../api/typesGenerated"
import { AppLink } from "../AppLink/AppLink"
import { SSHButton } from "../SSHButton/SSHButton"
import { Stack } from "../Stack/Stack"
import { TerminalLink } from "../TerminalLink/TerminalLink"
import { AgentLatency } from "./AgentLatency"
import { AgentStatus } from "./AgentStatus"
import { AgentVersion } from "./AgentVersion"

export interface AgentRowProps {
  agent: WorkspaceAgent
  workspace: Workspace
  applicationsHost: string | undefined
  showApps: boolean
  hideSSHButton?: boolean
  hideVSCodeDesktopButton?: boolean
  serverVersion: string
  onUpdateAgent: () => void

  storybookStartupLogs?: LineWithID[]
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
  storybookStartupLogs,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("agent")

  const [logsMachine, sendLogsEvent] = useMachine(workspaceAgentLogsMachine, {
    context: { agentID: agent.id },
    services: process.env.STORYBOOK
      ? {
          getStartupLogs: async () => {
            return storybookStartupLogs || []
          },
          streamStartupLogs: () => async () => {
            // noop
          },
        }
      : undefined,
  })
  const theme = useTheme()
  const startupScriptAnchorRef = useRef<HTMLLinkElement>(null)
  const [startupScriptOpen, setStartupScriptOpen] = useState(false)

  const hasStartupFeatures =
    Boolean(agent.startup_logs_length) ||
    Boolean(logsMachine.context.startupLogs?.length)

  const [showStartupLogs, setShowStartupLogs] = useState(
    agent.lifecycle_state !== "ready" && hasStartupFeatures,
  )
  useEffect(() => {
    setShowStartupLogs(agent.lifecycle_state !== "ready" && hasStartupFeatures)
  }, [agent.lifecycle_state, hasStartupFeatures])
  // External applications can provide startup logs for an agent during it's spawn.
  // These could be Kubernetes logs, or other logs that are useful to the user.
  // For this reason, we want to fetch these logs when the agent is starting.
  useEffect(() => {
    if (agent.lifecycle_state === "starting") {
      sendLogsEvent("FETCH_STARTUP_LOGS")
    }
  }, [sendLogsEvent, agent.lifecycle_state])
  useEffect(() => {
    // We only want to fetch logs when they are actually shown,
    // otherwise we can make a lot of requests that aren't necessary.
    if (showStartupLogs) {
      sendLogsEvent("FETCH_STARTUP_LOGS")
    }
  }, [sendLogsEvent, showStartupLogs])
  const logListRef = useRef<List>(null)
  const logListDivRef = useRef<HTMLDivElement>(null)
  const startupLogs = useMemo(() => {
    const allLogs = logsMachine.context.startupLogs || []

    const logs = [...allLogs]
    if (agent.startup_logs_overflowed) {
      logs.push({
        id: -1,
        level: "error",
        output: "Startup logs exceeded the max size of 1MB!",
        time: new Date().toISOString(),
      })
    }
    return logs
  }, [logsMachine.context.startupLogs, agent.startup_logs_overflowed])
  const [bottomOfLogs, setBottomOfLogs] = useState(true)
  // This is a layout effect to remove flicker when we're scrolling to the bottom.
  useLayoutEffect(() => {
    // If we're currently watching the bottom, we always want to stay at the bottom.
    if (bottomOfLogs && logListRef.current) {
      logListRef.current.scrollToItem(startupLogs.length - 1, "end")
    }
  }, [showStartupLogs, startupLogs, logListRef, bottomOfLogs])

  // This is a bit of a hack on the react-window API to get the scroll position.
  // If we're scrolled to the bottom, we want to keep the list scrolled to the bottom.
  // This makes it feel similar to a terminal that auto-scrolls downwards!
  const handleLogScroll = useCallback(
    (props: ListOnScrollProps) => {
      if (
        props.scrollOffset === 0 ||
        props.scrollUpdateWasRequested ||
        !logListDivRef.current
      ) {
        return
      }
      // The parent holds the height of the list!
      const parent = logListDivRef.current.parentElement
      if (!parent) {
        return
      }
      const distanceFromBottom =
        logListDivRef.current.scrollHeight -
        (props.scrollOffset + parent.clientHeight)
      setBottomOfLogs(distanceFromBottom < logLineHeight)
    },
    [logListDivRef],
  )

  return (
    <Stack
      direction="column"
      key={agent.id}
      spacing={0}
      className={styles.agentWrapper}
    >
      <Stack
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

            {hasStartupFeatures && (
              <Stack
                direction="row"
                alignItems="baseline"
                spacing={1}
                className={styles.startupLinks}
              >
                <Link
                  className={styles.startupLink}
                  variant="body2"
                  onClick={() => {
                    setShowStartupLogs(!showStartupLogs)
                  }}
                >
                  {showStartupLogs ? (
                    <VisibilityOffOutlined />
                  ) : (
                    <VisibilityOutlined />
                  )}
                  {showStartupLogs ? "Hide" : "Show"} Startup Logs
                </Link>

                {agent.startup_script && (
                  <Link
                    className={styles.startupLink}
                    variant="body2"
                    ref={startupScriptAnchorRef}
                    onClick={() => {
                      setStartupScriptOpen(!startupScriptOpen)
                    }}
                  >
                    <PlayCircleOutlined />
                    View Startup Script
                  </Link>
                )}

                <Popover
                  classes={{
                    paper: styles.startupScriptPopover,
                  }}
                  open={startupScriptOpen}
                  onClose={() => setStartupScriptOpen(false)}
                  anchorEl={startupScriptAnchorRef.current}
                  anchorOrigin={{
                    vertical: "bottom",
                    horizontal: "left",
                  }}
                  transformOrigin={{
                    vertical: "top",
                    horizontal: "left",
                  }}
                >
                  <div>
                    <SyntaxHighlighter
                      style={darcula}
                      language="shell"
                      showLineNumbers
                      // Use inline styles does not work correctly
                      // https://github.com/react-syntax-highlighter/react-syntax-highlighter/issues/329
                      codeTagProps={{ style: {} }}
                      customStyle={{
                        background: theme.palette.background.default,
                        maxWidth: 600,
                        margin: 0,
                      }}
                    >
                      {agent.startup_script || ""}
                    </SyntaxHighlighter>
                  </div>
                </Popover>
              </Stack>
            )}
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

      {showStartupLogs && (
        <AutoSizer disableHeight>
          {({ width }) => (
            <List
              ref={logListRef}
              innerRef={logListDivRef}
              height={256}
              itemCount={startupLogs.length}
              itemSize={logLineHeight}
              width={width}
              className={styles.startupLogs}
              onScroll={handleLogScroll}
            >
              {({ index, style }) => (
                <LogLine
                  line={startupLogs[index]}
                  number={index + 1}
                  style={style}
                />
              )}
            </List>
          )}
        </AutoSizer>
      )}
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  agentWrapper: {
    "&:not(:last-child)": {
      borderBottom: `1px solid ${theme.palette.divider}`,
    },
  },

  agentRow: {
    padding: theme.spacing(3, 4),
    backgroundColor: theme.palette.background.paperLight,
    fontSize: 16,
  },

  startupLinks: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(2),
    marginTop: theme.spacing(0.5),
  },

  startupLink: {
    cursor: "pointer",
    display: "flex",
    gap: 4,
    alignItems: "center",
    userSelect: "none",
    whiteSpace: "nowrap",

    "& svg": {
      width: 12,
      height: 12,
    },
  },

  startupLogs: {
    maxHeight: 256,
    background: theme.palette.background.default,
  },

  startupScriptPopover: {
    backgroundColor: theme.palette.background.default,
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

  agentStartupLogs: {
    maxHeight: 200,
    display: "flex",
    flexDirection: "column-reverse",
  },
}))
