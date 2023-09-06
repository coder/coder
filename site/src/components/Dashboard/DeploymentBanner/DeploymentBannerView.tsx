import { DeploymentStats, WorkspaceStatus } from "api/typesGenerated";
import { FC, useMemo, useEffect, useState } from "react";
import prettyBytes from "pretty-bytes";
import BuildingIcon from "@mui/icons-material/Build";
import { makeStyles } from "@mui/styles";
import { RocketIcon } from "components/Icons/RocketIcon";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import Tooltip from "@mui/material/Tooltip";
import { Link as RouterLink } from "react-router-dom";
import Link from "@mui/material/Link";
import { VSCodeIcon } from "components/Icons/VSCodeIcon";
import DownloadIcon from "@mui/icons-material/CloudDownload";
import UploadIcon from "@mui/icons-material/CloudUpload";
import LatencyIcon from "@mui/icons-material/SettingsEthernet";
import WebTerminalIcon from "@mui/icons-material/WebAsset";
import { TerminalIcon } from "components/Icons/TerminalIcon";
import dayjs from "dayjs";
import CollectedIcon from "@mui/icons-material/Compare";
import RefreshIcon from "@mui/icons-material/Refresh";
import Button from "@mui/material/Button";
import { getDisplayWorkspaceStatus } from "utils/workspace";

export const bannerHeight = 36;

export interface DeploymentBannerViewProps {
  fetchStats?: () => void;
  stats?: DeploymentStats;
}

export const DeploymentBannerView: FC<DeploymentBannerViewProps> = ({
  stats,
  fetchStats,
}) => {
  const styles = useStyles();
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

  return (
    <div className={styles.container}>
      <Tooltip title="Status of your Coder deployment. Only visible for admins!">
        <div className={styles.rocket}>
          <RocketIcon />
        </div>
      </Tooltip>
      <div className={styles.group}>
        <div className={styles.category}>Workspaces</div>
        <div className={styles.values}>
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
      <div className={styles.group}>
        <Tooltip title={`Activity in the last ~${aggregatedMinutes} minutes`}>
          <div className={styles.category}>Transmission</div>
        </Tooltip>

        <div className={styles.values}>
          <Tooltip title="Data sent to workspaces">
            <div className={styles.value}>
              <DownloadIcon />
              {stats ? prettyBytes(stats.workspaces.rx_bytes) : "-"}
            </div>
          </Tooltip>
          <ValueSeparator />
          <Tooltip title="Data sent from workspaces">
            <div className={styles.value}>
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
            <div className={styles.value}>
              <LatencyIcon />
              {displayLatency > 0 ? displayLatency?.toFixed(2) + " ms" : "-"}
            </div>
          </Tooltip>
        </div>
      </div>
      <div className={styles.group}>
        <div className={styles.category}>Active Connections</div>

        <div className={styles.values}>
          <Tooltip title="VS Code Editors with the Coder Remote Extension">
            <div className={styles.value}>
              <VSCodeIcon className={styles.iconStripColor} />
              {typeof stats?.session_count.vscode === "undefined"
                ? "-"
                : stats?.session_count.vscode}
            </div>
          </Tooltip>
          <ValueSeparator />
          <Tooltip title="SSH Sessions">
            <div className={styles.value}>
              <TerminalIcon />
              {typeof stats?.session_count.ssh === "undefined"
                ? "-"
                : stats?.session_count.ssh}
            </div>
          </Tooltip>
          <ValueSeparator />
          <Tooltip title="Web Terminal Sessions">
            <div className={styles.value}>
              <WebTerminalIcon />
              {typeof stats?.session_count.reconnecting_pty === "undefined"
                ? "-"
                : stats?.session_count.reconnecting_pty}
            </div>
          </Tooltip>
        </div>
      </div>
      <div className={styles.refresh}>
        <Tooltip title="The last time stats were aggregated. Workspaces report statistics periodically, so it may take a bit for these to update!">
          <div className={styles.value}>
            <CollectedIcon />
            {lastAggregated}
          </div>
        </Tooltip>

        <Tooltip title="A countdown until stats are fetched again. Click to refresh!">
          <Button
            className={`${styles.value} ${styles.refreshButton}`}
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
  const styles = useStyles();
  return <div className={styles.valueSeparator}>/</div>;
};

const WorkspaceBuildValue: FC<{
  status: WorkspaceStatus;
  count?: number;
}> = ({ status, count }) => {
  const styles = useStyles();
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
        <div className={styles.value}>
          {icon}
          {typeof count === "undefined" ? "-" : count}
        </div>
      </Link>
    </Tooltip>
  );
};

const useStyles = makeStyles((theme) => ({
  rocket: {
    display: "flex",
    alignItems: "center",

    "& svg": {
      width: 16,
      height: 16,
    },

    [theme.breakpoints.down("lg")]: {
      display: "none",
    },
  },
  container: {
    position: "sticky",
    height: bannerHeight,
    bottom: 0,
    zIndex: 1,
    padding: theme.spacing(0, 2),
    backgroundColor: theme.palette.background.paper,
    display: "flex",
    alignItems: "center",
    fontFamily: MONOSPACE_FONT_FAMILY,
    fontSize: 12,
    gap: theme.spacing(4),
    borderTop: `1px solid ${theme.palette.divider}`,
    overflowX: "auto",
    whiteSpace: "nowrap",
  },
  group: {
    display: "flex",
    alignItems: "center",
  },
  category: {
    marginRight: theme.spacing(2),
    color: theme.palette.text.primary,
  },
  values: {
    display: "flex",
    gap: theme.spacing(1),
    color: theme.palette.text.secondary,
  },
  valueSeparator: {
    color: theme.palette.text.disabled,
  },
  value: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(0.5),

    "& svg": {
      width: 12,
      height: 12,
    },
  },
  iconStripColor: {
    "& *": {
      fill: "currentColor",
    },
  },
  refresh: {
    color: theme.palette.text.primary,
    marginLeft: "auto",
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(2),
  },
  refreshButton: {
    margin: 0,
    padding: "0px 8px",
    height: "unset",
    minHeight: "unset",
    fontSize: "unset",
    color: "unset",
    border: 0,
    minWidth: "unset",
    fontFamily: "inherit",

    "& svg": {
      marginRight: theme.spacing(0.5),
    },
  },
}));
