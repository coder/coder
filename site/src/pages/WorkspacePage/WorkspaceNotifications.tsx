import AlertTitle from "@mui/material/AlertTitle";
import Button from "@mui/material/Button";
import { workspaceResolveAutostart } from "api/queries/workspaceQuota";
import { TemplateVersion, Workspace } from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { FC } from "react";
import { useQuery } from "react-query";
import { WorkspacePermissions } from "./permissions";
import { DormantWorkspaceBanner } from "./DormantWorkspaceBanner";

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
    </>
  );
};
