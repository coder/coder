import type { Health } from "api/api";
import type { DeploymentStats, WorkspaceStatus } from "api/typesGenerated";
import {
  type FC,
  useMemo,
  useEffect,
  useState,
  PropsWithChildren,
} from "react";
import prettyBytes from "pretty-bytes";
import BuildingIcon from "@mui/icons-material/Build";
import Tooltip from "@mui/material/Tooltip";
import { Link as RouterLink } from "react-router-dom";
import Link from "@mui/material/Link";
import { VSCodeIcon } from "components/Icons/VSCodeIcon";
import DownloadIcon from "@mui/icons-material/CloudDownload";
import UploadIcon from "@mui/icons-material/CloudUpload";
import LatencyIcon from "@mui/icons-material/SettingsEthernet";
import WebTerminalIcon from "@mui/icons-material/WebAsset";
import CollectedIcon from "@mui/icons-material/Compare";
import RefreshIcon from "@mui/icons-material/Refresh";
import Button from "@mui/material/Button";
import { css as className } from "@emotion/css";
import {
  css,
  type CSSObject,
  type Theme,
  type Interpolation,
  useTheme,
} from "@emotion/react";
import dayjs from "dayjs";
import { TerminalIcon } from "components/Icons/TerminalIcon";
import { RocketIcon } from "components/Icons/RocketIcon";
import ErrorIcon from "@mui/icons-material/ErrorOutline";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { getDisplayWorkspaceStatus } from "utils/workspace";
import { colors } from "theme/colors";
import { HelpTooltipTitle } from "components/HelpTooltip/HelpTooltip";
import { Stack } from "components/Stack/Stack";

export const bannerHeight = 36;

const styles = {
  group: css`
    display: flex;
    align-items: center;
  `,
  category: (theme) => ({
    marginRight: theme.spacing(2),
    color: theme.palette.text.primary,
  }),
  values: (theme) => ({
    display: "flex",
    gap: theme.spacing(1),
    color: theme.palette.text.secondary,
  }),
  value: (theme) => css`
    display: flex;
    align-items: center;
    gap: ${theme.spacing(0.5)};

    & svg {
      width: 12px;
      height: 12px;
    }
  `,
} satisfies Record<string, Interpolation<Theme>>;

export interface DeploymentBannerViewProps {
  health?: Health;
  stats?: DeploymentStats;
  fetchStats?: () => void;
}

export const DeploymentBannerView: FC<DeploymentBannerViewProps> = (props) => {
  const { health, stats, fetchStats } = props;
  const theme = useTheme();
  const aggregatedMinutes = useMemo(() => {
    if (!stats) {
      return;
    }
    return dayjs(stats.collected_at).diff(stats.aggregated_from, "minutes");
  }, [stats]);
  const displayLatency = stats?.workspaces.connection_latency_ms.P50 || -1;
  const [timeUntilRefresh, setTimeUntilRefresh] = useState(0);
  useEffect(() => {
    if (!stats || !fetchStats) {
      return;
    }

    let timeUntilRefresh = dayjs(stats.next_update_at).diff(
      stats.collected_at,
      "seconds",
    );
    setTimeUntilRefresh(timeUntilRefresh);
    let canceled = false;
    const loop = () => {
      if (canceled) {
        return undefined;
      }
      setTimeUntilRefresh(timeUntilRefresh--);
      if (timeUntilRefresh > 0) {
        return window.setTimeout(loop, 1000);
      }
      fetchStats();
    };
    const timeout = setTimeout(loop, 1000);
    return () => {
      canceled = true;
      clearTimeout(timeout);
    };
  }, [fetchStats, stats]);
  const lastAggregated = useMemo(() => {
    if (!stats) {
      return;
    }
    if (!fetchStats) {
      // Storybook!
      return "just now";
    }
    return dayjs().to(dayjs(stats.collected_at));
    // eslint-disable-next-line react-hooks/exhaustive-deps -- We want this to periodically update!
  }, [timeUntilRefresh, stats]);

  const unhealthy = health && !health.healthy;

  const statusBadgeStyle = css`
    display: flex;
    align-items: center;
    justify-content: center;
    background-color: ${unhealthy ? colors.red[10] : undefined};
    padding: ${theme.spacing(0, 1.5)};
    height: ${bannerHeight}px;
    color: #fff;

    & svg {
      width: 16px;
      height: 16px;
    }
  `;

  const statusSummaryStyle = className`
    ${theme.typography.body2 as CSSObject}

    margin: ${theme.spacing(0, 0, 0.5, 1.5)};
    width: ${theme.spacing(50)};
    padding: ${theme.spacing(2)};
    color: ${theme.palette.text.primary};
    background-color: ${theme.palette.background.paper};
    border: 1px solid ${theme.palette.divider};
    pointer-events: none;
  `;

  return (
    <div
      css={{
        position: "sticky",
        height: bannerHeight,
        bottom: 0,
        zIndex: 1,
        paddingRight: theme.spacing(2),
        backgroundColor: theme.palette.background.paper,
        display: "flex",
        alignItems: "center",
        fontFamily: MONOSPACE_FONT_FAMILY,
        fontSize: 12,
        gap: theme.spacing(4),
        borderTop: `1px solid ${theme.palette.divider}`,
        overflowX: "auto",
        whiteSpace: "nowrap",
      }}
    >
      <Tooltip
        classes={{ tooltip: statusSummaryStyle }}
        title={
          unhealthy ? (
            <>
              <HelpTooltipTitle>
                We have detected problems with your Coder deployment.
              </HelpTooltipTitle>
              <Stack spacing={1}>
                {!health.access_url.healthy && (
                  <HealthIssue>
                    Your access URL may be configured incorrectly.
                  </HealthIssue>
                )}
                {!health.database.healthy && (
                  <HealthIssue>Your database is unhealthy.</HealthIssue>
                )}
                {!health.derp.healthy && (
                  <HealthIssue>
                    We&apos;re noticing DERP proxy issues.
                  </HealthIssue>
                )}
                {!health.websocket.healthy && (
                  <HealthIssue>
                    We&apos;re noticing websocket issues.
                  </HealthIssue>
                )}
              </Stack>
            </>
          ) : (
            <>Status of your Coder deployment. Only visible for admins!</>
          )
        }
        open={process.env.STORYBOOK === "true" ? true : undefined}
        css={{ marginRight: theme.spacing(-2) }}
      >
        {unhealthy ? (
          <Link component={RouterLink} to="/health" css={statusBadgeStyle}>
            <ErrorIcon />
          </Link>
        ) : (
          <div css={statusBadgeStyle}>
            <RocketIcon />
          </div>
        )}
      </Tooltip>
      <div css={styles.group}>
        <div css={styles.category}>Workspaces</div>
        <div css={styles.values}>
          <WorkspaceBuildValue
            status="pending"
            count={stats?.workspaces.pending}
          />
          <ValueSeparator />
          <WorkspaceBuildValue
            status="starting"
            count={stats?.workspaces.building}
          />
          <ValueSeparator />
          <WorkspaceBuildValue
            status="running"
            count={stats?.workspaces.running}
          />
          <ValueSeparator />
          <WorkspaceBuildValue
            status="stopped"
            count={stats?.workspaces.stopped}
          />
          <ValueSeparator />
          <WorkspaceBuildValue
            status="failed"
            count={stats?.workspaces.failed}
          />
        </div>
      </div>
      <div css={styles.group}>
        <Tooltip title={`Activity in the last ~${aggregatedMinutes} minutes`}>
          <div css={styles.category}>Transmission</div>
        </Tooltip>

        <div css={styles.values}>
          <Tooltip title="Data sent to workspaces">
            <div css={styles.value}>
              <DownloadIcon />
              {stats ? prettyBytes(stats.workspaces.rx_bytes) : "-"}
            </div>
          </Tooltip>
          <ValueSeparator />
          <Tooltip title="Data sent from workspaces">
            <div css={styles.value}>
              <UploadIcon />
              {stats ? prettyBytes(stats.workspaces.tx_bytes) : "-"}
            </div>
          </Tooltip>
          <ValueSeparator />
          <Tooltip
            title={
              displayLatency < 0
                ? "No recent workspace connections have been made"
                : "The average latency of user connections to workspaces"
            }
          >
            <div css={styles.value}>
              <LatencyIcon />
              {displayLatency > 0 ? displayLatency?.toFixed(2) + " ms" : "-"}
            </div>
          </Tooltip>
        </div>
      </div>
      <div css={styles.group}>
        <div css={styles.category}>Active Connections</div>

        <div css={styles.values}>
          <Tooltip title="VS Code Editors with the Coder Remote Extension">
            <div css={styles.value}>
              <VSCodeIcon
                css={css`
                  & * {
                    fill: currentColor;
                  }
                `}
              />
              {typeof stats?.session_count.vscode === "undefined"
                ? "-"
                : stats?.session_count.vscode}
            </div>
          </Tooltip>
          <ValueSeparator />
          <Tooltip title="SSH Sessions">
            <div css={styles.value}>
              <TerminalIcon />
              {typeof stats?.session_count.ssh === "undefined"
                ? "-"
                : stats?.session_count.ssh}
            </div>
          </Tooltip>
          <ValueSeparator />
          <Tooltip title="Web Terminal Sessions">
            <div css={styles.value}>
              <WebTerminalIcon />
              {typeof stats?.session_count.reconnecting_pty === "undefined"
                ? "-"
                : stats?.session_count.reconnecting_pty}
            </div>
          </Tooltip>
        </div>
      </div>
      <div
        css={{
          color: theme.palette.text.primary,
          marginLeft: "auto",
          display: "flex",
          alignItems: "center",
          gap: theme.spacing(2),
        }}
      >
        <Tooltip title="The last time stats were aggregated. Workspaces report statistics periodically, so it may take a bit for these to update!">
          <div css={styles.value}>
            <CollectedIcon />
            {lastAggregated}
          </div>
        </Tooltip>

        <Tooltip title="A countdown until stats are fetched again. Click to refresh!">
          <Button
            css={css`
              ${styles.value(theme)}

              margin: 0;
              padding: 0 8px;
              height: unset;
              min-height: unset;
              font-size: unset;
              color: unset;
              border: 0;
              min-width: unset;
              font-family: inherit;

              & svg {
                margin-right: ${theme.spacing(0.5)};
              }
            `}
            onClick={() => {
              if (fetchStats) {
                fetchStats();
              }
            }}
            variant="text"
          >
            <RefreshIcon />
            {timeUntilRefresh}s
          </Button>
        </Tooltip>
      </div>
    </div>
  );
};

const ValueSeparator: FC = () => {
  const theme = useTheme();
  const separatorStyles = css`
    color: ${theme.palette.text.disabled};
  `;

  return <div css={separatorStyles}>/</div>;
};

const WorkspaceBuildValue: FC<{
  status: WorkspaceStatus;
  count?: number;
}> = ({ status, count }) => {
  const displayStatus = getDisplayWorkspaceStatus(status);
  let statusText = displayStatus.text;
  let icon = displayStatus.icon;
  if (status === "starting") {
    icon = <BuildingIcon />;
    statusText = "Building";
  }

  return (
    <Tooltip title={`${statusText} Workspaces`}>
      <Link
        component={RouterLink}
        to={`/workspaces?filter=${encodeURIComponent("status:" + status)}`}
      >
        <div css={styles.value}>
          {icon}
          {typeof count === "undefined" ? "-" : count}
        </div>
      </Link>
    </Tooltip>
  );
};

const HealthIssue: FC<PropsWithChildren> = ({ children }) => {
  return (
    <Stack direction="row" spacing={1} alignItems="center">
      <ErrorIcon css={{ width: 16, height: 16 }} htmlColor={colors.red[10]} />
      {children}
    </Stack>
  );
};
