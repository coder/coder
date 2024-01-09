import { formatDistanceToNow } from "date-fns";
import { ReactNode, type FC } from "react";
import type { Workspace } from "api/typesGenerated";
import { useIsWorkspaceActionsEnabled } from "components/Dashboard/DashboardProvider";
import { Alert } from "components/Alert/Alert";
import { useLocalStorage } from "hooks";

interface DormantWorkspaceBannerProps {
  workspace: Workspace;
}

export const DormantWorkspaceBanner: FC<DormantWorkspaceBannerProps> = ({
  workspace,
}) => {
  const experimentEnabled = useIsWorkspaceActionsEnabled();
  const { saveLocal, getLocal } = useLocalStorage();
  const shouldRedisplayBanner = getLocal("dismissedWorkspace") !== workspace.id;

  if (
    // Only show this if the experiment is included.
    !experimentEnabled ||
    !workspace.dormant_at ||
    // Banners should be redisplayed after dismissal when additional workspaces are newly scheduled for deletion
    !shouldRedisplayBanner
  ) {
    return null;
  }

  const formatDate = (dateStr: string, timestamp: boolean): string => {
    const date = new Date(dateStr);
    return date.toLocaleDateString(undefined, {
      month: "long",
      day: "numeric",
      year: "numeric",
      ...(timestamp ? { hour: "numeric", minute: "numeric" } : {}),
    });
  };

  const alertText = (): ReactNode => {
    if (workspace.deleting_at && workspace.dormant_at) {
      return (
        <>
          This workspace has not been used for{" "}
          {formatDistanceToNow(Date.parse(workspace.last_used_at))} and was
          marked dormant on {formatDate(workspace.dormant_at, false)}. It is
          scheduled to be deleted on {formatDate(workspace.deleting_at, true)}.
          To keep it you must activate the workspace.
        </>
      );
    } else if (workspace.dormant_at) {
      return (
        <>
          This workspace has not been used for{" "}
          {formatDistanceToNow(Date.parse(workspace.last_used_at))} and was
          marked dormant on {formatDate(workspace.dormant_at, false)}. It is not
          scheduled for auto-deletion but will become a candidate if
          auto-deletion is enabled on this template. To keep it you must
          activate the workspace.
        </>
      );
    }
    return "";
  };

  return (
    <Alert
      severity="warning"
      onDismiss={() => {
        saveLocal("dismissedWorkspace", workspace.id);
      }}
      dismissible
    >
      {alertText()}
    </Alert>
  );
};
