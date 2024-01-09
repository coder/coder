import AlertTitle from "@mui/material/AlertTitle";
import Button from "@mui/material/Button";
import { workspaceResolveAutostart } from "api/queries/workspaceQuota";
import { TemplateVersion, Workspace } from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { FC, useEffect, useState } from "react";
import { useQuery } from "react-query";
import { WorkspacePermissions } from "./permissions";
import { DormantWorkspaceBanner } from "./DormantWorkspaceBanner";
import dayjs from "dayjs";

type WorkspaceNotificationsProps = {
  workspace: Workspace;
  permissions: WorkspacePermissions;
  onRestartWorkspace: () => void;
  latestVersion?: TemplateVersion;
};

export const WorkspaceNotifications: FC<WorkspaceNotificationsProps> = (
  props,
) => {
  const { workspace, latestVersion, permissions, onRestartWorkspace } = props;

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

  return (
    <>
      {workspace.outdated &&
        latestVersion &&
        (requiresManualUpdate ? (
          <Alert severity="warning">
            <AlertTitle>
              Autostart has been disabled for your workspace.
            </AlertTitle>
            <AlertDetail>
              Autostart is unable to automatically update your workspace.
              Manually update your workspace to reenable Autostart.
            </AlertDetail>
          </Alert>
        ) : (
          <Alert severity="info">
            <AlertTitle>An update is available for your workspace</AlertTitle>
            <AlertDetail>{latestVersion.message}</AlertDetail>
          </Alert>
        ))}

      {workspace.latest_build.status === "running" &&
        !workspace.health.healthy && (
          <Alert
            severity="warning"
            actions={
              permissions.updateWorkspace && (
                <Button
                  variant="text"
                  size="small"
                  onClick={onRestartWorkspace}
                >
                  Restart
                </Button>
              )
            }
          >
            <AlertTitle>Workspace is unhealthy</AlertTitle>
            <AlertDetail>
              Your workspace is running but{" "}
              {workspace.health.failing_agents.length > 1
                ? `${workspace.health.failing_agents.length} agents are unhealthy`
                : `1 agent is unhealthy`}
              .
            </AlertDetail>
          </Alert>
        )}

      <DormantWorkspaceBanner workspace={workspace} />

      {showAlertPendingInQueue && (
        <Alert severity="info">
          <AlertTitle>Workspace build is pending</AlertTitle>
          <AlertDetail>
            <div css={{ marginBottom: 12 }}>
              This workspace build job is waiting for a provisioner to become
              available. If you have been waiting for an extended period of
              time, please contact your administrator for assistance.
            </div>
            <div>
              Position in queue:{" "}
              <strong>{workspace.latest_build.job.queue_position}</strong>
            </div>
          </AlertDetail>
        </Alert>
      )}
    </>
  );
};
