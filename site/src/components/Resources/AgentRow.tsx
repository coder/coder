import Collapse from "@mui/material/Collapse";
import Skeleton from "@mui/material/Skeleton";
import Tooltip from "@mui/material/Tooltip";
import { type Interpolation, type Theme } from "@emotion/react";
import * as API from "api/api";
import type {
  Workspace,
  WorkspaceAgent,
  WorkspaceAgentLogSource,
  WorkspaceAgentMetadata,
} from "api/typesGenerated";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { displayError } from "components/GlobalSnackbar/utils";
import { VSCodeDesktopButton } from "components/Resources/VSCodeDesktopButton/VSCodeDesktopButton";
import {
  Line,
  LogLine,
  logLineHeight,
} from "components/WorkspaceBuildLogs/Logs";
import { useProxy } from "contexts/ProxyContext";
import {
  FC,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import AutoSizer from "react-virtualized-auto-sizer";
import { FixedSizeList as List, ListOnScrollProps } from "react-window";
import { colors } from "theme/colors";
import { Stack } from "../Stack/Stack";
import { AgentLatency } from "./AgentLatency";
import { AgentMetadata } from "./AgentMetadata";
import { AgentStatus } from "./AgentStatus";
import { AgentVersion } from "./AgentVersion";
import { AppLink } from "./AppLink/AppLink";
import { PortForwardButton } from "./PortForwardButton";
import { SSHButton } from "./SSHButton/SSHButton";
import { TerminalLink } from "./TerminalLink/TerminalLink";

// Logs are stored as the Line interface to make rendering
// much more efficient. Instead of mapping objects each time, we're
// able to just pass the array of logs to the component.
export interface LineWithID extends Line {
  id: number;
}

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
  storybookAgentMetadata,
  sshPrefix,
  storybookLogs,
}) => {
  const hasAppsToDisplay = !hideVSCodeDesktopButton || agent.apps.length > 0;
  const shouldDisplayApps =
    showApps &&
    ((agent.status === "connected" && hasAppsToDisplay) ||
      agent.status === "connecting");
  const logSourceByID = useMemo(() => {
    const sources: { [id: string]: WorkspaceAgentLogSource } = {};
    for (const source of agent.log_sources) {
      sources[source.id] = source;
    }
    return sources;
  }, [agent.log_sources]);
  const hasStartupFeatures = Boolean(agent.logs_length);
  const { proxy } = useProxy();
  const [showLogs, setShowLogs] = useState(
    ["starting", "start_timeout"].includes(agent.lifecycle_state) &&
      hasStartupFeatures,
  );
  const agentLogs = useAgentLogs(agent.id, {
    enabled: showLogs,
    initialData: process.env.STORYBOOK ? storybookLogs || [] : undefined,
  });
  const logListRef = useRef<List>(null);
  const logListDivRef = useRef<HTMLDivElement>(null);
  const startupLogs = useMemo(() => {
    const allLogs = agentLogs || [];

    const logs = [...allLogs];
    if (agent.logs_overflowed) {
      logs.push({
        id: -1,
        level: "error",
        output: "Startup logs exceeded the max size of 1MB!",
        time: new Date().toISOString(),
        source_id: "",
      });
    }
    return logs;
  }, [agentLogs, agent.logs_overflowed]);
  const [bottomOfLogs, setBottomOfLogs] = useState(true);

  useEffect(() => {
    setShowLogs(agent.lifecycle_state !== "ready" && hasStartupFeatures);
  }, [agent.lifecycle_state, hasStartupFeatures]);

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
      css={[
        styles.agentRow,
        styles[`agentRow-${agent.status}`],
        styles[`agentRow-lifecycle-${agent.lifecycle_state}`],
      ]}
    >
      <div css={styles.agentInfo}>
        <div css={styles.agentNameAndStatus}>
          <div css={styles.agentNameAndInfo}>
            <AgentStatus agent={agent} />
            <div css={styles.agentName}>{agent.name}</div>
            <Stack
              direction="row"
              spacing={2}
              alignItems="baseline"
              css={styles.agentDescription}
            >
              {agent.status === "connected" && (
                <>
                  <span css={styles.agentOS}>{agent.operating_system}</span>
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
          <div css={styles.agentButtons}>
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
          <div css={styles.agentButtons}>
            <Skeleton
              width={80}
              height={32}
              variant="rectangular"
              css={styles.buttonSkeleton}
            />
            <Skeleton
              width={110}
              height={32}
              variant="rectangular"
              css={styles.buttonSkeleton}
            />
          </div>
        )}
      </div>

      <AgentMetadata storybookMetadata={storybookAgentMetadata} agent={agent} />

      {hasStartupFeatures && (
        <div css={styles.logsPanel}>
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
                  css={styles.startupLogs}
                  onScroll={handleLogScroll}
                >
                  {({ index, style }) => {
                    const log = startupLogs[index];
                    // getLogSource always returns a valid log source.
                    // This is necessary to support deployments before `coder_script`.
                    // Existed that haven't restarted their agents.
                    const getLogSource = (
                      id: string,
                    ): WorkspaceAgentLogSource => {
                      return (
                        logSourceByID[id] || {
                          created_at: "",
                          display_name: "Logs",
                          icon: "",
                          id: "00000000-0000-0000-0000-000000000000",
                          workspace_agent_id: "",
                        }
                      );
                    };
                    const logSource = getLogSource(log.source_id);

                    let assignedIcon = false;
                    let icon: JSX.Element;
                    // If no icon is specified, we show a deterministic
                    // colored circle to identify unique scripts.
                    if (logSource.icon) {
                      icon = (
                        <img
                          src={logSource.icon}
                          alt=""
                          width={16}
                          height={16}
                          css={{
                            marginRight: 8,
                          }}
                        />
                      );
                    } else {
                      icon = (
                        <div
                          css={{
                            width: 16,
                            height: 16,
                            marginRight: 8,
                            background: determineScriptDisplayColor(
                              logSource.display_name,
                            ),
                            borderRadius: "100%",
                          }}
                        />
                      );
                      assignedIcon = true;
                    }

                    let nextChangesSource = false;
                    if (index < startupLogs.length - 1) {
                      nextChangesSource =
                        getLogSource(startupLogs[index + 1].source_id).id !==
                        log.source_id;
                    }
                    // We don't want every line to repeat the icon, because
                    // that is ugly and repetitive. This removes the icon
                    // for subsequent lines of the same source and shows a
                    // line instead, visually indicating they are from the
                    // same source.
                    if (
                      index > 0 &&
                      getLogSource(startupLogs[index - 1].source_id).id ===
                        log.source_id
                    ) {
                      icon = (
                        <div
                          css={{
                            minWidth: 16,
                            width: 16,
                            height: 16,
                            marginRight: 8,
                            display: "flex",
                            justifyContent: "center",
                            position: "relative",
                          }}
                        >
                          <div
                            css={{
                              height: nextChangesSource ? "50%" : "100%",
                              width: 4,
                              background: "hsl(222, 31%, 25%)",
                              borderRadius: 2,
                            }}
                          />
                          {nextChangesSource && (
                            <div
                              css={{
                                height: 4,
                                width: "50%",
                                top: "calc(50% - 2px)",
                                left: "calc(50% - 1px)",
                                background: "hsl(222, 31%, 25%)",
                                borderRadius: 2,
                                position: "absolute",
                              }}
                            />
                          )}
                        </div>
                      );
                    }

                    return (
                      <LogLine
                        line={startupLogs[index]}
                        number={index + 1}
                        maxNumber={startupLogs.length}
                        style={style}
                        sourceIcon={
                          <Tooltip
                            title={
                              <>
                                {logSource.display_name}
                                {assignedIcon && (
                                  <i>
                                    <br />
                                    No icon specified!
                                  </i>
                                )}
                              </>
                            }
                          >
                            {icon}
                          </Tooltip>
                        }
                      />
                    );
                  }}
                </List>
              )}
            </AutoSizer>
          </Collapse>

          <div css={styles.logsPanelButtons}>
            {showLogs ? (
              <button
                css={[styles.logsPanelButton, styles.toggleLogsButton]}
                onClick={() => {
                  setShowLogs((v) => !v);
                }}
              >
                <DropdownArrow close />
                Hide logs
              </button>
            ) : (
              <button
                css={[styles.logsPanelButton, styles.toggleLogsButton]}
                onClick={() => {
                  setShowLogs((v) => !v);
                }}
              >
                <DropdownArrow />
                Show logs
              </button>
            )}
          </div>
        </div>
      )}
    </Stack>
  );
};

const useAgentLogs = (
  agentId: string,
  { enabled, initialData }: { enabled: boolean; initialData?: LineWithID[] },
) => {
  const [logs, setLogs] = useState<LineWithID[] | undefined>(initialData);
  const socket = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (!enabled) {
      socket.current?.close();
      setLogs([]);
      return;
    }

    socket.current = API.watchWorkspaceAgentLogs(agentId, {
      // Get all logs
      after: 0,
      onMessage: (logs) => {
        setLogs((previousLogs) => {
          const newLogs: LineWithID[] = logs.map((log) => ({
            id: log.id,
            level: log.level || "info",
            output: log.output,
            time: log.created_at,
            source_id: log.source_id,
          }));

          if (!previousLogs) {
            return newLogs;
          }

          return [...previousLogs, ...newLogs];
        });
      },
      onError: () => {
        displayError("Error on getting agent logs");
      },
    });

    return () => {
      socket.current?.close();
    };
  }, [agentId, enabled]);

  return logs;
};

const styles = {
  agentRow: (theme) => ({
    backgroundColor: theme.palette.background.paperLight,
    fontSize: 16,
    borderLeft: `2px solid ${theme.palette.text.secondary}`,

    "&:not(:first-of-type)": {
      borderTop: `2px solid ${theme.palette.divider}`,
    },
  }),

  "agentRow-connected": (theme) => ({
    borderLeftColor: theme.palette.success.light,
  }),

  "agentRow-disconnected": (theme) => ({
    borderLeftColor: theme.palette.text.secondary,
  }),

  "agentRow-connecting": (theme) => ({
    borderLeftColor: theme.palette.info.light,
  }),

  "agentRow-timeout": (theme) => ({
    borderLeftColor: theme.palette.warning.light,
  }),

  "agentRow-lifecycle-created": {},

  "agentRow-lifecycle-starting": (theme) => ({
    borderLeftColor: theme.palette.info.light,
  }),

  "agentRow-lifecycle-ready": (theme) => ({
    borderLeftColor: theme.palette.success.light,
  }),

  "agentRow-lifecycle-start_timeout": (theme) => ({
    borderLeftColor: theme.palette.warning.light,
  }),

  "agentRow-lifecycle-start_error": (theme) => ({
    borderLeftColor: theme.palette.error.light,
  }),

  "agentRow-lifecycle-shutting_down": (theme) => ({
    borderLeftColor: theme.palette.info.light,
  }),

  "agentRow-lifecycle-shutdown_timeout": (theme) => ({
    borderLeftColor: theme.palette.warning.light,
  }),

  "agentRow-lifecycle-shutdown_error": (theme) => ({
    borderLeftColor: theme.palette.error.light,
  }),

  "agentRow-lifecycle-off": (theme) => ({
    borderLeftColor: theme.palette.text.secondary,
  }),

  agentInfo: (theme) => ({
    padding: theme.spacing(2, 4),
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(6),
    flexWrap: "wrap",

    [theme.breakpoints.down("md")]: {
      gap: theme.spacing(2),
    },
  }),

  agentNameAndInfo: (theme) => ({
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(3),
    flexWrap: "wrap",

    [theme.breakpoints.down("md")]: {
      gap: theme.spacing(1.5),
    },
  }),

  agentButtons: (theme) => ({
    display: "flex",
    gap: theme.spacing(1),
    justifyContent: "flex-end",
    flexWrap: "wrap",
    flex: 1,

    [theme.breakpoints.down("md")]: {
      marginLeft: 0,
      justifyContent: "flex-start",
    },
  }),

  agentDescription: (theme) => ({
    fontSize: 14,
    color: theme.palette.text.secondary,
  }),

  startupLogs: (theme) => ({
    maxHeight: 256,
    borderBottom: `1px solid ${theme.palette.divider}`,
    backgroundColor: theme.palette.background.paper,
    paddingTop: theme.spacing(2),

    // We need this to be able to apply the padding top from startupLogs
    "& > div": {
      position: "relative",
    },
  }),

  agentNameAndStatus: (theme) => ({
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(4),

    [theme.breakpoints.down("md")]: {
      width: "100%",
    },
  }),

  agentName: (theme) => ({
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
  }),

  agentDataGroup: (theme) => ({
    display: "flex",
    alignItems: "baseline",
    gap: theme.spacing(6),
  }),

  agentData: (theme) => ({
    display: "flex",
    flexDirection: "column",
    fontSize: 12,

    "& > *:first-of-type": {
      fontWeight: 500,
      color: theme.palette.text.secondary,
    },
  }),

  logsPanel: (theme) => ({
    borderTop: `1px solid ${theme.palette.divider}`,
  }),

  logsPanelButtons: {
    display: "flex",
  },

  logsPanelButton: (theme) => ({
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
  }),

  toggleLogsButton: {
    width: "100%",
  },

  buttonSkeleton: {
    borderRadius: 4,
  },

  agentErrorMessage: (theme) => ({
    fontSize: 12,
    fontWeight: 400,
    marginTop: theme.spacing(0.5),
    color: theme.palette.warning.light,
  }),

  agentOS: {
    textTransform: "capitalize",
  },
} satisfies Record<string, Interpolation<Theme>>;

// These colors were picked at random. Feel free
// to add more, adjust, or change! Users will not
// depend on these colors.
const scriptDisplayColors = [
  "#85A3B2",
  "#A37EB2",
  "#C29FDE",
  "#90B3D7",
  "#829AC7",
  "#728B8E",
  "#506080",
  "#5654B0",
  "#6B56D6",
  "#7847CC",
];

const determineScriptDisplayColor = (displayName: string): string => {
  const hash = displayName.split("").reduce((hash, char) => {
    return (hash << 5) + hash + char.charCodeAt(0); // bit-shift and add for our simple hash
  }, 0);
  return scriptDisplayColors[Math.abs(hash) % scriptDisplayColors.length];
};
