import AlertTitle from "@mui/material/AlertTitle";
import { workspaceResolveAutostart } from "api/queries/workspaceQuota";
import { TemplateVersion, Workspace } from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { FC } from "react";
import { useQuery } from "react-query";

type WorkspaceNotificationsProps = {
  workspace: Workspace;
  latestVersion?: TemplateVersion;
};

export const WorkspaceNotifications: FC<WorkspaceNotificationsProps> = (
  props,
) => {
  const { workspace, latestVersion } = props;

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
    </>
  );
};
