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
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";
import { Interpolation, Theme, useTheme } from "@emotion/react";
import Button, { ButtonProps } from "@mui/material/Button";
import { ThemeRole } from "theme/experimental";
import WarningRounded from "@mui/icons-material/WarningRounded";

type Notification = {
  title: string;
  severity: AlertProps["severity"];
  detail?: ReactNode;
  actions?: ReactNode;
};

type WorkspaceNotificationsProps = {
  workspace: Workspace;
  template: Template;
  permissions: WorkspacePermissions;
  onRestartWorkspace: () => void;
  onUpdateWorkspace: () => void;
  onActivateWorkspace: () => void;
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
    onUpdateWorkspace,
    onActivateWorkspace,
  } = props;
  const notifications: Notification[] = [];

  // Outdated
  const canAutostartQuery = useQuery(workspaceResolveAutostart(workspace.id));
  const isParameterMismatch =
    canAutostartQuery.data?.parameter_mismatch ?? false;
  const canAutostart = !isParameterMismatch;
  const updateRequired =
    (workspace.template_require_active_version ||
      workspace.automatic_updates === "always") &&
    workspace.outdated;
  const autoStartFailing = workspace.autostart_schedule && !canAutostart;
  const requiresManualUpdate = updateRequired && autoStartFailing;

  if (workspace.outdated && latestVersion) {
    const actions = (
      <NotificationActionButton onClick={onUpdateWorkspace}>
        Update
      </NotificationActionButton>
    );
    if (requiresManualUpdate) {
      notifications.push({
        title: "Autostart has been disabled for your workspace.",
        severity: "warning",
        detail:
          "Autostart is unable to automatically update your workspace. Manually update your workspace to reenable Autostart.",

        actions,
      });
    } else {
      notifications.push({
        title: "An update is available for your workspace",
        severity: "info",
        detail: latestVersion.message,
        actions,
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
      actions: permissions.updateWorkspace ? (
        <NotificationActionButton onClick={onRestartWorkspace}>
          Restart
        </NotificationActionButton>
      ) : undefined,
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
    const actions = (
      <NotificationActionButton onClick={onActivateWorkspace}>
        Activate
      </NotificationActionButton>
    );
    notifications.push({
      actions,
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

  const infoNotifications = notifications.filter((n) => n.severity === "info");
  const warningNotifications = notifications.filter(
    (n) => n.severity === "warning",
  );

  return (
    <div
      css={{
        display: "flex",
        alignItems: "center",
        gap: 12,
        position: "fixed",
        bottom: 48,
        right: 48,
        zIndex: 10,
      }}
    >
      {infoNotifications.length > 0 && (
        <NotificationPill
          notifications={infoNotifications}
          type="info"
          icon={<InfoOutlined />}
        />
      )}

      {warningNotifications.length > 0 && (
        <NotificationPill
          notifications={warningNotifications}
          type="warning"
          icon={<WarningRounded />}
        />
      )}
    </div>
  );
};

type NotificationPillProps = {
  notifications: Notification[];
  type: ThemeRole;
  icon: ReactNode;
};

const NotificationPill: FC<NotificationPillProps> = (props) => {
  const { notifications, type, icon } = props;
  const theme = useTheme();

  return (
    <Popover mode="hover">
      <PopoverTrigger>
        <div css={[styles.pillContainer]}>
          <Pill type={type} icon={icon}>
            {notifications.length}
          </Pill>
        </div>
      </PopoverTrigger>
      <PopoverContent
        transformOrigin={{
          horizontal: "right",
          vertical: "bottom",
        }}
        anchorOrigin={{ horizontal: "right", vertical: "top" }}
        css={{
          "& .MuiPaper-root": {
            borderColor: theme.experimental.roles[type].outline,
            maxWidth: 400,
          },
        }}
      >
        {notifications.map((n) => (
          <NotificationItem notification={n} key={n.title} />
        ))}
      </PopoverContent>
    </Popover>
  );
};

const NotificationItem: FC<{ notification: Notification }> = (props) => {
  const { notification } = props;
  const theme = useTheme();

  return (
    <article css={{ padding: 16 }}>
      <h4 css={{ margin: 0, fontWeight: 500 }}>{notification.title}</h4>
      {notification.detail && (
        <p
          css={{
            margin: 0,
            color: theme.palette.text.secondary,
            lineHeight: 1.6,
          }}
        >
          {notification.detail}
        </p>
      )}
      <div css={{ marginTop: 8 }}>{notification.actions}</div>
    </article>
  );
};

const NotificationActionButton: FC<ButtonProps> = (props) => {
  return (
    <Button
      variant="text"
      css={{
        textDecoration: "underline",
        padding: 0,
        height: "auto",
        minWidth: "auto",
        "&:hover": { background: "none", textDecoration: "underline" },
      }}
      {...props}
    />
  );
};

const styles = {
  // Adds some spacing from the popover content
  pillContainer: {
    paddingTop: 8,
  },
} satisfies Record<string, Interpolation<Theme>>;
