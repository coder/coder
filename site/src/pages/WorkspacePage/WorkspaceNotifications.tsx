import { workspaceResolveAutostart } from "api/queries/workspaceQuota";
import { Template, TemplateVersion, Workspace } from "api/typesGenerated";
import { AlertProps } from "components/Alert/Alert";
import { FC, ReactNode, useEffect, useState } from "react";
import { useQuery } from "react-query";
import { WorkspacePermissions } from "./permissions";
import dayjs from "dayjs";
import { useIsWorkspaceActionsEnabled } from "components/Dashboard/DashboardProvider";
import formatDistanceToNow from "date-fns/formatDistanceToNow";
import { Pill } from "components/Pill/Pill";
import InfoOutlined from "@mui/icons-material/InfoOutlined";
import ErrorOutline from "@mui/icons-material/ErrorOutline";

type Notification = {
  title: string;
  severity: AlertProps["severity"];
  detail?: ReactNode;
  actions?: { label: string; onClick: () => void }[];
};

type WorkspaceNotificationsProps = {
  workspace: Workspace;
  template: Template;
  permissions: WorkspacePermissions;
  onRestartWorkspace: () => void;
  latestVersion?: TemplateVersion;
};

export const WorkspaceNotifications: FC<WorkspaceNotificationsProps> = (
  props,
) => {
  const {
    workspace,
    template,
    latestVersion,
    permissions,
    onRestartWorkspace,
  } = props;
  const notifications: Notification[] = [];

  // Outdated
  const canAutostartResponse = useQuery(
    workspaceResolveAutostart(workspace.id),
  );
  const canAutostart = !canAutostartResponse.data?.parameter_mismatch ?? false;
  const updateRequired =
    (workspace.template_require_active_version ||
      workspace.automatic_updates === "always") &&
    workspace.outdated;
  const autoStartFailing = workspace.autostart_schedule && !canAutostart;
  const requiresManualUpdate = updateRequired && autoStartFailing;

  if (workspace.outdated && latestVersion) {
    if (requiresManualUpdate) {
      notifications.push({
        title: "Autostart has been disabled for your workspace.",
        severity: "warning",
        detail:
          "Autostart is unable to automatically update your workspace. Manually update your workspace to reenable Autostart.",
      });
    } else {
      notifications.push({
        title: "An update is available for your workspace",
        severity: "info",
        detail: latestVersion.message,
      });
    }
  }

  // Unhealthy
  if (
    workspace.latest_build.status === "running" &&
    !workspace.health.healthy
  ) {
    notifications.push({
      title: "Workspace is unhealthy",
      severity: "warning",
      detail: (
        <>
          Your workspace is running but{" "}
          {workspace.health.failing_agents.length > 1
            ? `${workspace.health.failing_agents.length} agents are unhealthy`
            : `1 agent is unhealthy`}
          .
        </>
      ),
      actions: permissions.updateWorkspace
        ? [
            {
              label: "Restart",
              onClick: onRestartWorkspace,
            },
          ]
        : undefined,
    });
  }

  // Dormant
  const areActionsEnabled = useIsWorkspaceActionsEnabled();
  if (areActionsEnabled && workspace.dormant_at) {
    const formatDate = (dateStr: string, timestamp: boolean): string => {
      const date = new Date(dateStr);
      return date.toLocaleDateString(undefined, {
        month: "long",
        day: "numeric",
        year: "numeric",
        ...(timestamp ? { hour: "numeric", minute: "numeric" } : {}),
      });
    };
    notifications.push({
      title: "Workspace is dormant",
      severity: "warning",
      detail: workspace.deleting_at ? (
        <>
          This workspace has not been used for{" "}
          {formatDistanceToNow(Date.parse(workspace.last_used_at))} and was
          marked dormant on {formatDate(workspace.dormant_at, false)}. It is
          scheduled to be deleted on {formatDate(workspace.deleting_at, true)}.
          To keep it you must activate the workspace.
        </>
      ) : (
        <>
          This workspace has not been used for{" "}
          {formatDistanceToNow(Date.parse(workspace.last_used_at))} and was
          marked dormant on {formatDate(workspace.dormant_at, false)}. It is not
          scheduled for auto-deletion but will become a candidate if
          auto-deletion is enabled on this template. To keep it you must
          activate the workspace.
        </>
      ),
    });
  }

  // Pending in Queue
  const [showAlertPendingInQueue, setShowAlertPendingInQueue] = useState(false);
  // 2023-11-15 - MES - This effect will be called every single render because
  // "now" will always change and invalidate the dependency array. Need to
  // figure out if this effect really should run every render (possibly meaning
  // no dependency array at all), or how to get the array stabilized (ideal)
  const now = dayjs();
  useEffect(() => {
    if (
      workspace.latest_build.status !== "pending" ||
      workspace.latest_build.job.queue_size === 0
    ) {
      if (!showAlertPendingInQueue) {
        return;
      }

      const hideTimer = setTimeout(() => {
        setShowAlertPendingInQueue(false);
      }, 250);
      return () => {
        clearTimeout(hideTimer);
      };
    }

    const t = Math.max(
      0,
      5000 - dayjs().diff(dayjs(workspace.latest_build.created_at)),
    );
    const showTimer = setTimeout(() => {
      setShowAlertPendingInQueue(true);
    }, t);

    return () => {
      clearTimeout(showTimer);
    };
  }, [workspace, now, showAlertPendingInQueue]);

  if (showAlertPendingInQueue) {
    notifications.push({
      title: "Workspace build is pending",
      severity: "info",
      detail: (
        <>
          <div css={{ marginBottom: 12 }}>
            This workspace build job is waiting for a provisioner to become
            available. If you have been waiting for an extended period of time,
            please contact your administrator for assistance.
          </div>
          <div>
            Position in queue:{" "}
            <strong>{workspace.latest_build.job.queue_position}</strong>
          </div>
        </>
      ),
    });
  }

  // Deprecated
  if (template.deprecated) {
    notifications.push({
      title: "Workspace using deprecated template",
      severity: "warning",
      detail: template.deprecation_message,
    });
  }

  return (
    <div
      css={{
        display: "flex",
        alignItems: "center",
        gap: 8,
        position: "fixed",
        bottom: 48,
        right: 48,
        zIndex: 10,
      }}
    >
      <Pill type="info" icon={<InfoOutlined />}>
        2
      </Pill>
      <Pill type="warning" icon={<ErrorOutline />}>
        4
      </Pill>
    </div>
  );
};
