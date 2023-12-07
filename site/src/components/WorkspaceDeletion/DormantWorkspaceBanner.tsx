import { formatDistanceToNow } from "date-fns";
import { ReactNode, type FC } from "react";
import type { Workspace } from "api/typesGenerated";
import { useIsWorkspaceActionsEnabled } from "components/Dashboard/DashboardProvider";
import { Alert } from "components/Alert/Alert";

export enum Count {
  Singular,
  Multiple,
}

interface DormantWorkspaceBannerProps {
  workspace: Workspace;
  onDismiss: () => void;
  shouldRedisplayBanner: boolean;
}

export const DormantWorkspaceBanner: FC<DormantWorkspaceBannerProps> = ({
  workspace,
  onDismiss,
  shouldRedisplayBanner,
}) => {
  const experimentEnabled = useIsWorkspaceActionsEnabled();

  if (
    // Only show this if the experiment is included.
    !experimentEnabled ||
    !workspace.dormant_at ||
    // Banners should be redisplayed after dismissal when additional workspaces are newly scheduled for deletion
    !shouldRedisplayBanner
  ) {
    return null;
  }

  const formatDate = (dateStr: string): string => {
    const date = new Date(dateStr);
    return date.toLocaleDateString(undefined, {
      month: "long",
      day: "numeric",
      year: "numeric",
    });
  };

  const alertText = (): ReactNode => {
    if (workspace.deleting_at) {
      return (
        <>
          This workspace has been dormant for $
          {formatDistanceToNow(Date.parse(workspace.last_used_at))} and is
          scheduled to be deleted on ${formatDate(workspace.deleting_at)}. To
          keep it you must activate the workspace.
        </>
      );
    } else if (workspace.dormant_at) {
      return (
        <>
          This workspace has been dormant for $
          {formatDistanceToNow(Date.parse(workspace.dormant_at))}
          and cannot be interacted with. Dormant workspaces are eligible for
          permanent deletion. To prevent deletion, activate the workspace.`;
        </>
      );
    }
    return "";
  };

  return (
    <Alert severity="warning" onDismiss={onDismiss} dismissible>
      {alertText()}
    </Alert>
  );
};
