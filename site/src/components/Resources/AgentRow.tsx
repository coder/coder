import Popover from "@mui/material/Popover";
import { makeStyles, useTheme } from "@mui/styles";
import Skeleton from "@mui/material/Skeleton";
import { useMachine } from "@xstate/react";
import CodeOutlined from "@mui/icons-material/CodeOutlined";
import {
  CloseDropdown,
  OpenDropdown,
} from "components/DropdownArrows/DropdownArrows";
import {
  LogLine,
  logLineHeight,
} from "components/WorkspaceBuildLogs/Logs/Logs";
import { PortForwardButton } from "./PortForwardButton";
import { VSCodeDesktopButton } from "components/Resources/VSCodeDesktopButton/VSCodeDesktopButton";
import {
  FC,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { darcula } from "react-syntax-highlighter/dist/cjs/styles/prism";
import AutoSizer from "react-virtualized-auto-sizer";
import { FixedSizeList as List, ListOnScrollProps } from "react-window";
import { colors } from "theme/colors";
import { combineClasses } from "utils/combineClasses";
import {
  LineWithID,
  workspaceAgentLogsMachine,
} from "xServices/workspaceAgentLogs/workspaceAgentLogsXService";
import {
  Workspace,
  WorkspaceAgent,
  WorkspaceAgentMetadata,
} from "../../api/typesGenerated";
import { AppLink } from "./AppLink/AppLink";
import { SSHButton } from "./SSHButton/SSHButton";
import { Stack } from "../Stack/Stack";
import { TerminalLink } from "./TerminalLink/TerminalLink";
import { AgentLatency } from "./AgentLatency";
import { AgentMetadata } from "./AgentMetadata";
import { AgentVersion } from "./AgentVersion";
import { AgentStatus } from "./AgentStatus";
import Collapse from "@mui/material/Collapse";
import { useProxy } from "contexts/ProxyContext";

export interface AgentRowProps {
  agent: WorkspaceAgent;
  workspace: Workspace;
  showApps: boolean;
  showBuiltinApps?: boolean;
  sshPrefix?: string;
  hideSSHButton?: boolean;
  hideVSCodeDesktopButton?: boolean;
  serverVersion: string;
  onUpdateAgent: () => void;
  storybookLogs?: LineWithID[];
  storybookAgentMetadata?: WorkspaceAgentMetadata[];
}

export const AgentRow: FC<AgentRowProps> = ({
  agent,
  workspace,
  showApps,
  showBuiltinApps = true,
  hideSSHButton,
  hideVSCodeDesktopButton,
  serverVersion,
  onUpdateAgent,
  storybookLogs,
  storybookAgentMetadata,
  sshPrefix,
}) => {
  const styles = useStyles();
  const [logsMachine, sendLogsEvent] = useMachine(workspaceAgentLogsMachine, {
    context: { agentID: agent.id },
    services: process.env.STORYBOOK
      ? {
          getLogs: async () => {
            return storybookLogs || [];
          },
          streamLogs: () => async () => {
            // noop
          },
        }
      : undefined,
  });
  const theme = useTheme();
  const startupScriptAnchorRef = useRef<HTMLButtonElement>(null);
  const [startupScriptOpen, setStartupScriptOpen] = useState(false);
  const hasAppsToDisplay = !hideVSCodeDesktopButton || agent.apps.length > 0;
  const shouldDisplayApps =
    showApps &&
    ((agent.status === "connected" && hasAppsToDisplay) ||
      agent.status === "connecting");
  const hasStartupFeatures =
    Boolean(agent.logs_length) || Boolean(logsMachine.context.logs?.length);
  const { proxy } = useProxy();

  const [showLogs, setShowLogs] = useState(
    ["starting", "start_timeout"].includes(agent.lifecycle_state) &&
      hasStartupFeatures,
  );
  useEffect(() => {
    setShowLogs(agent.lifecycle_state !== "ready" && hasStartupFeatures);
  }, [agent.lifecycle_state, hasStartupFeatures]);
  // External applications can provide startup logs for an agent during it's spawn.
  // These could be Kubernetes logs, or other logs that are useful to the user.
  // For this reason, we want to fetch these logs when the agent is starting.
  useEffect(() => {
    if (agent.lifecycle_state === "starting") {
      sendLogsEvent("FETCH_LOGS");
    }
  }, [sendLogsEvent, agent.lifecycle_state]);
  useEffect(() => {
    // We only want to fetch logs when they are actually shown,
    // otherwise we can make a lot of requests that aren't necessary.
    if (showLogs && logsMachine.can("FETCH_LOGS")) {
      sendLogsEvent("FETCH_LOGS");
    }
  }, [logsMachine, sendLogsEvent, showLogs]);
  const logListRef = useRef<List>(null);
  const logListDivRef = useRef<HTMLDivElement>(null);
  const startupLogs = useMemo(() => {
    const allLogs = logsMachine.context.logs || [];

    const logs = [...allLogs];
    if (agent.logs_overflowed) {
      logs.push({
        id: -1,
        level: "error",
        output: "Startup logs exceeded the max size of 1MB!",
        time: new Date().toISOString(),
      });
    }
    return logs;
  }, [logsMachine.context.logs, agent.logs_overflowed]);
  const [bottomOfLogs, setBottomOfLogs] = useState(true);
  // This is a layout effect to remove flicker when we're scrolling to the bottom.
  useLayoutEffect(() => {
    // If we're currently watching the bottom, we always want to stay at the bottom.
    if (bottomOfLogs && logListRef.current) {
      logListRef.current.scrollToItem(startupLogs.length - 1, "end");
    }
  }, [showLogs, startupLogs, logListRef, bottomOfLogs]);

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
        return;
      }
      // The parent holds the height of the list!
      const parent = logListDivRef.current.parentElement;
      if (!parent) {
        return;
      }
      const distanceFromBottom =
        logListDivRef.current.scrollHeight -
        (props.scrollOffset + parent.clientHeight);
      setBottomOfLogs(distanceFromBottom < logLineHeight);
    },
    [logListDivRef],
  );

  return (
    <Stack
      key={agent.id}
      direction="column"
      spacing={0}
      className={combineClasses([
        styles.agentRow,
        styles[`agentRow-${agent.status}`],
        styles[`agentRow-lifecycle-${agent.lifecycle_state}`],
      ])}
    >
      <div className={styles.agentInfo}>
        <div className={styles.agentNameAndStatus}>
          <div className={styles.agentNameAndInfo}>
            <AgentStatus agent={agent} />
            <div className={styles.agentName}>{agent.name}</div>
            <Stack
              direction="row"
              spacing={2}
              alignItems="baseline"
              className={styles.agentDescription}
            >
              {agent.status === "connected" && (
                <>
                  <span className={styles.agentOS}>
                    {agent.operating_system}
                  </span>
                  <AgentVersion
                    agent={agent}
                    serverVersion={serverVersion}
                    onUpdate={onUpdateAgent}
                  />
                  <AgentLatency agent={agent} />
                </>
              )}
              {agent.status === "connecting" && (
                <>
                  <Skeleton width={160} variant="text" />
                  <Skeleton width={36} variant="text" />
                </>
              )}
            </Stack>
          </div>
        </div>

        {agent.status === "connected" && (
          <div className={styles.agentButtons}>
            {shouldDisplayApps && (
              <>
                {(agent.display_apps.includes("vscode") ||
                  agent.display_apps.includes("vscode_insiders")) &&
                  !hideVSCodeDesktopButton && (
                    <VSCodeDesktopButton
                      userName={workspace.owner_name}
                      workspaceName={workspace.name}
                      agentName={agent.name}
                      folderPath={agent.expanded_directory}
                      displayApps={agent.display_apps}
                    />
                  )}
                {agent.apps.map((app) => (
                  <AppLink
                    key={app.slug}
                    app={app}
                    agent={agent}
                    workspace={workspace}
                  />
                ))}
              </>
            )}

            {showBuiltinApps && (
              <>
                {agent.display_apps.includes("web_terminal") && (
                  <TerminalLink
                    workspaceName={workspace.name}
                    agentName={agent.name}
                    userName={workspace.owner_name}
                  />
                )}
                {!hideSSHButton &&
                  agent.display_apps.includes("ssh_helper") && (
                    <SSHButton
                      workspaceName={workspace.name}
                      agentName={agent.name}
                      sshPrefix={sshPrefix}
                    />
                  )}
                {proxy.preferredWildcardHostname &&
                  proxy.preferredWildcardHostname !== "" &&
                  agent.display_apps.includes("port_forwarding_helper") && (
                    <PortForwardButton
                      host={proxy.preferredWildcardHostname}
                      workspaceName={workspace.name}
                      agent={agent}
                      username={workspace.owner_name}
                    />
                  )}
              </>
            )}
          </div>
        )}

        {agent.status === "connecting" && (
          <div className={styles.agentButtons}>
            <Skeleton
              width={80}
              height={32}
              variant="rectangular"
              className={styles.buttonSkeleton}
            />
            <Skeleton
              width={110}
              height={32}
              variant="rectangular"
              className={styles.buttonSkeleton}
            />
          </div>
        )}
      </div>

      <AgentMetadata storybookMetadata={storybookAgentMetadata} agent={agent} />

      {hasStartupFeatures && (
        <div className={styles.logsPanel}>
          <Collapse in={showLogs}>
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
          </Collapse>

          <div className={styles.logsPanelButtons}>
            {showLogs ? (
              <button
                className={combineClasses([
                  styles.logsPanelButton,
                  styles.toggleLogsButton,
                ])}
                onClick={() => {
                  setShowLogs((v) => !v);
                }}
              >
                <CloseDropdown />
                Hide startup logs
              </button>
            ) : (
              <button
                className={combineClasses([
                  styles.logsPanelButton,
                  styles.toggleLogsButton,
                ])}
                onClick={() => {
                  setShowLogs((v) => !v);
                }}
              >
                <OpenDropdown />
                Show startup logs
              </button>
            )}

            <button
              className={combineClasses([
                styles.logsPanelButton,
                styles.scriptButton,
              ])}
              ref={startupScriptAnchorRef}
              onClick={() => {
                setStartupScriptOpen(!startupScriptOpen);
              }}
            >
              <CodeOutlined />
              Startup script
            </button>

            <Popover
              classes={{
                paper: styles.startupScriptPopover,
              }}
              open={startupScriptOpen}
              onClose={() => setStartupScriptOpen(false)}
              anchorEl={startupScriptAnchorRef.current}
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
          </div>
        </div>
      )}
    </Stack>
  );
};

const useStyles = makeStyles((theme) => ({
  agentRow: {
    backgroundColor: theme.palette.background.paperLight,
    fontSize: 16,
    borderLeft: `2px solid ${theme.palette.text.secondary}`,

    "&:not(:first-of-type)": {
      borderTop: `2px solid ${theme.palette.divider}`,
    },
  },

  "agentRow-connected": {
    borderLeftColor: theme.palette.success.light,
  },

  "agentRow-disconnected": {
    borderLeftColor: theme.palette.text.secondary,
  },

  "agentRow-connecting": {
    borderLeftColor: theme.palette.info.light,
  },

  "agentRow-timeout": {
    borderLeftColor: theme.palette.warning.light,
  },

  "agentRow-lifecycle-created": {},

  "agentRow-lifecycle-starting": {
    borderLeftColor: theme.palette.info.light,
  },

  "agentRow-lifecycle-ready": {
    borderLeftColor: theme.palette.success.light,
  },

  "agentRow-lifecycle-start_timeout": {
    borderLeftColor: theme.palette.warning.light,
  },

  "agentRow-lifecycle-start_error": {
    borderLeftColor: theme.palette.error.light,
  },

  "agentRow-lifecycle-shutting_down": {
    borderLeftColor: theme.palette.info.light,
  },

  "agentRow-lifecycle-shutdown_timeout": {
    borderLeftColor: theme.palette.warning.light,
  },

  "agentRow-lifecycle-shutdown_error": {
    borderLeftColor: theme.palette.error.light,
  },

  "agentRow-lifecycle-off": {
    borderLeftColor: theme.palette.text.secondary,
  },

  agentInfo: {
    padding: theme.spacing(2, 4),
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(6),
    flexWrap: "wrap",

    [theme.breakpoints.down("md")]: {
      gap: theme.spacing(2),
    },
  },

  agentNameAndInfo: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(3),
    flexWrap: "wrap",

    [theme.breakpoints.down("md")]: {
      gap: theme.spacing(1.5),
    },
  },

  agentButtons: {
    display: "flex",
    gap: theme.spacing(1),
    justifyContent: "flex-end",
    flexWrap: "wrap",
    flex: 1,

    [theme.breakpoints.down("md")]: {
      marginLeft: 0,
      justifyContent: "flex-start",
    },
  },

  agentDescription: {
    fontSize: 14,
    color: theme.palette.text.secondary,
  },

  startupLogs: {
    maxHeight: 256,
    borderBottom: `1px solid ${theme.palette.divider}`,
    backgroundColor: theme.palette.background.paper,
    paddingTop: theme.spacing(2),

    // We need this to be able to apply the padding top from startupLogs
    "& > div": {
      position: "relative",
    },
  },

  startupScriptPopover: {
    backgroundColor: theme.palette.background.default,
  },

  agentNameAndStatus: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(4),

    [theme.breakpoints.down("md")]: {
      width: "100%",
    },
  },

  agentName: {
    whiteSpace: "nowrap",
    overflow: "hidden",
    textOverflow: "ellipsis",
    maxWidth: 260,
    fontWeight: 600,
    fontSize: theme.spacing(2),
    flexShrink: 0,
    width: "fit-content",

    [theme.breakpoints.down("md")]: {
      overflow: "unset",
    },
  },

  agentDataGroup: {
    display: "flex",
    alignItems: "baseline",
    gap: theme.spacing(6),
  },

  agentData: {
    display: "flex",
    flexDirection: "column",
    fontSize: 12,

    "& > *:first-of-type": {
      fontWeight: 500,
      color: theme.palette.text.secondary,
    },
  },

  logsPanel: {
    borderTop: `1px solid ${theme.palette.divider}`,
  },

  logsPanelButtons: {
    display: "flex",
  },

  logsPanelButton: {
    textAlign: "left",
    background: "transparent",
    border: 0,
    fontFamily: "inherit",
    padding: theme.spacing(1.5, 4),
    color: theme.palette.text.secondary,
    cursor: "pointer",
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1),
    whiteSpace: "nowrap",

    "&:hover": {
      color: theme.palette.text.primary,
      backgroundColor: colors.gray[14],
    },

    "& svg": {
      color: "inherit",
    },
  },

  toggleLogsButton: {
    width: "100%",
  },

  buttonSkeleton: {
    borderRadius: 4,
  },

  agentErrorMessage: {
    fontSize: 12,
    fontWeight: 400,
    marginTop: theme.spacing(0.5),
    color: theme.palette.warning.light,
  },

  scriptButton: {
    "& svg": {
      width: theme.spacing(2),
      height: theme.spacing(2),
    },
  },

  agentOS: {
    textTransform: "capitalize",
  },
}));
