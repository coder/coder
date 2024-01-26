import Collapse from "@mui/material/Collapse";
import Skeleton from "@mui/material/Skeleton";
import Tooltip from "@mui/material/Tooltip";
import { type Interpolation, type Theme } from "@emotion/react";
import {
  type FC,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import AutoSizer from "react-virtualized-auto-sizer";
import { FixedSizeList as List, ListOnScrollProps } from "react-window";
import * as API from "api/api";
import type {
  Workspace,
  WorkspaceAgent,
  WorkspaceAgentLogSource,
  WorkspaceAgentMetadata,
} from "api/typesGenerated";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import {
  Line,
  LogLine,
  logLineHeight,
} from "components/WorkspaceBuildLogs/Logs";
import { useProxy } from "contexts/ProxyContext";
import { Stack } from "components/Stack/Stack";
import { AgentLatency } from "./AgentLatency";
import { AgentMetadata } from "./AgentMetadata";
import { AgentStatus } from "./AgentStatus";
import { AgentVersion } from "./AgentVersion";
import { AppLink } from "./AppLink/AppLink";
import { PortForwardButton } from "./PortForwardButton";
import { SSHButton } from "./SSHButton/SSHButton";
import { TerminalLink } from "./TerminalLink/TerminalLink";
import { VSCodeDesktopButton } from "./VSCodeDesktopButton/VSCodeDesktopButton";

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
  serverAPIVersion: string;
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
  serverAPIVersion,
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
  const hasVSCodeApp =
    agent.display_apps.includes("vscode") ||
    agent.display_apps.includes("vscode_insiders");
  const showVSCode = hasVSCodeApp && !hideVSCodeDesktopButton;

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
      <header css={styles.header}>
        <div css={styles.agentInfo}>
          <div css={styles.agentNameAndStatus}>
            <AgentStatus agent={agent} />
            <span css={styles.agentName}>{agent.name}</span>
          </div>
          {agent.status === "connected" && (
            <>
              <AgentVersion
                agent={agent}
                serverVersion={serverVersion}
                serverAPIVersion={serverAPIVersion}
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
        </div>

        {showBuiltinApps && (
          <div css={{ display: "flex" }}>
            {!hideSSHButton && agent.display_apps.includes("ssh_helper") && (
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
          </div>
        )}
      </header>

      <div css={styles.content}>
        {agent.status === "connected" && (
          <section css={styles.apps}>
            {shouldDisplayApps && (
              <>
                {showVSCode && (
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

            {showBuiltinApps && agent.display_apps.includes("web_terminal") && (
              <TerminalLink
                workspaceName={workspace.name}
                agentName={agent.name}
                userName={workspace.owner_name}
              />
            )}
          </section>
        )}

        {agent.status === "connecting" && (
          <section css={styles.apps}>
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
          </section>
        )}

        <AgentMetadata
          storybookMetadata={storybookAgentMetadata}
          agent={agent}
        />
      </div>

      {hasStartupFeatures && (
        <section
          css={(theme) => ({ borderTop: `1px solid ${theme.palette.divider}` })}
        >
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
                          width={14}
                          height={14}
                          css={{
                            marginRight: 8,
                            flexShrink: 0,
                          }}
                        />
                      );
                    } else {
                      icon = (
                        <div
                          css={{
                            width: 14,
                            height: 14,
                            marginRight: 8,
                            flexShrink: 0,
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
                            width: 14,
                            height: 14,
                            marginRight: 8,
                            display: "flex",
                            justifyContent: "center",
                            position: "relative",
                            flexShrink: 0,
                          }}
                        >
                          <div
                            className="dashed-line"
                            css={(theme) => ({
                              height: nextChangesSource ? "50%" : "100%",
                              width: 2,
                              background: theme.experimental.l1.outline,
                              borderRadius: 2,
                            })}
                          />
                          {nextChangesSource && (
                            <div
                              className="dashed-line"
                              css={(theme) => ({
                                height: 2,
                                width: "50%",
                                top: "calc(50% - 2px)",
                                left: "calc(50% - 1px)",
                                background: theme.experimental.l1.outline,
                                borderRadius: 2,
                                position: "absolute",
                              })}
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

          <button
            css={styles.logsPanelButton}
            onClick={() => setShowLogs((v) => !v)}
          >
            <DropdownArrow close={showLogs} margin={false} />
            {showLogs ? "Hide" : "Show"} logs
          </button>
        </section>
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
        // Prevent new logs getting added when a connection is not open
        if (socket.current?.readyState !== WebSocket.OPEN) {
          return;
        }

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
      onError: (error) => {
        // For some reason Firefox and Safari throw an error when a websocket
        // connection is close in the middle of a message and because of that we
        // can't safely show to the users an error message since most of the
        // time they are just internal stuff. This does not happen to Chrome at
        // all and I tried to find better way to "soft close" a WS connection on
        // those browsers without success.
        console.error(error);
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
    fontSize: 14,
    border: `1px solid ${theme.palette.text.secondary}`,
    backgroundColor: theme.palette.background.default,
    borderRadius: 8,
    boxShadow: theme.shadows[3],
  }),

  "agentRow-connected": (theme) => ({
    borderColor: theme.palette.success.light,
  }),

  "agentRow-disconnected": (theme) => ({
    borderColor: theme.palette.divider,
  }),

  "agentRow-connecting": (theme) => ({
    borderColor: theme.palette.info.light,
  }),

  "agentRow-timeout": (theme) => ({
    borderColor: theme.palette.warning.light,
  }),

  "agentRow-lifecycle-created": {},

  "agentRow-lifecycle-starting": (theme) => ({
    borderColor: theme.palette.info.light,
  }),

  "agentRow-lifecycle-ready": (theme) => ({
    borderColor: theme.palette.success.light,
  }),

  "agentRow-lifecycle-start_timeout": (theme) => ({
    borderColor: theme.palette.warning.light,
  }),

  "agentRow-lifecycle-start_error": (theme) => ({
    borderColor: theme.palette.error.light,
  }),

  "agentRow-lifecycle-shutting_down": (theme) => ({
    borderColor: theme.palette.info.light,
  }),

  "agentRow-lifecycle-shutdown_timeout": (theme) => ({
    borderColor: theme.palette.warning.light,
  }),

  "agentRow-lifecycle-shutdown_error": (theme) => ({
    borderColor: theme.palette.error.light,
  }),

  "agentRow-lifecycle-off": (theme) => ({
    borderColor: theme.palette.divider,
  }),

  header: (theme) => ({
    padding: "16px 16px 0 32px",
    display: "flex",
    gap: 24,
    alignItems: "center",
    justifyContent: "space-between",
    flexWrap: "wrap",
    lineHeight: "1.5",

    [theme.breakpoints.down("md")]: {
      gap: 16,
    },
  }),

  agentInfo: (theme) => ({
    display: "flex",
    alignItems: "center",
    gap: 24,
    color: theme.palette.text.secondary,
    fontSize: 14,
  }),

  agentNameAndInfo: (theme) => ({
    display: "flex",
    alignItems: "center",
    gap: 24,
    flexWrap: "wrap",

    [theme.breakpoints.down("md")]: {
      gap: 12,
    },
  }),

  content: {
    padding: 32,
    display: "flex",
    flexDirection: "column",
    gap: 32,
  },

  apps: (theme) => ({
    display: "flex",
    gap: 16,
    flexWrap: "wrap",

    "&:empty": {
      display: "none",
    },

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
    paddingTop: 16,

    // We need this to be able to apply the padding top from startupLogs
    "& > div": {
      position: "relative",
    },
  }),

  agentNameAndStatus: (theme) => ({
    display: "flex",
    alignItems: "center",
    gap: 16,

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
    flexShrink: 0,
    width: "fit-content",
    fontSize: 16,
    color: theme.palette.text.primary,

    [theme.breakpoints.down("md")]: {
      overflow: "unset",
    },
  }),

  agentDataGroup: {
    display: "flex",
    alignItems: "baseline",
    gap: 48,
  },

  agentData: (theme) => ({
    display: "flex",
    flexDirection: "column",
    fontSize: 12,

    "& > *:first-of-type": {
      fontWeight: 500,
      color: theme.palette.text.secondary,
    },
  }),

  logsPanelButton: (theme) => ({
    textAlign: "left",
    background: "transparent",
    border: 0,
    fontFamily: "inherit",
    padding: "16px 32px",
    color: theme.palette.text.secondary,
    cursor: "pointer",
    display: "flex",
    alignItems: "center",
    gap: 8,
    whiteSpace: "nowrap",
    width: "100%",
    borderBottomLeftRadius: 8,
    borderBottomRightRadius: 8,

    "&:hover": {
      color: theme.palette.text.primary,
      backgroundColor: theme.experimental.l2.hover.background,
    },

    "& svg": {
      color: "inherit",
    },
  }),

  buttonSkeleton: {
    borderRadius: 4,
  },

  agentErrorMessage: (theme) => ({
    fontSize: 12,
    fontWeight: 400,
    marginTop: 4,
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
