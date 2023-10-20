import { Workspace } from "api/typesGenerated";
import { useIsWorkspaceActionsEnabled } from "components/Dashboard/DashboardProvider";
import { Alert } from "components/Alert/Alert";
import { formatDistanceToNow } from "date-fns";
import Link from "@mui/material/Link";
import { Link as RouterLink } from "react-router-dom";

export enum Count {
  Singular,
  Multiple,
}

export const DormantWorkspaceBanner = ({
  workspaces,
  onDismiss,
  shouldRedisplayBanner,
  count = Count.Singular,
}: {
  workspaces?: Workspace[];
  onDismiss: () => void;
  shouldRedisplayBanner: boolean;
  count?: Count;
}): JSX.Element | null => {
  const experimentEnabled = useIsWorkspaceActionsEnabled();

  if (!workspaces) {
    return null;
  }

  const hasDormantWorkspaces = workspaces.find(
    (workspace) => workspace.dormant_at,
  );

  const hasDeletionScheduledWorkspaces = workspaces.find(
    (workspace) => workspace.deleting_at,
  );

  if (
    // Only show this if the experiment is included.
    !experimentEnabled ||
    !hasDormantWorkspaces ||
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

  const alertText = (): string => {
    if (workspaces.length === 1) {
      if (
        hasDeletionScheduledWorkspaces &&
        hasDeletionScheduledWorkspaces.deleting_at &&
        hasDeletionScheduledWorkspaces.dormant_at
      ) {
        return `This workspace has been dormant for ${formatDistanceToNow(
          Date.parse(hasDeletionScheduledWorkspaces.dormant_at),
        )} and is scheduled to be deleted on ${formatDate(
          hasDeletionScheduledWorkspaces.deleting_at,
        )} . To keep it you must activate the workspace.`;
      } else if (hasDormantWorkspaces && hasDormantWorkspaces.dormant_at) {
        return `This workspace has been dormant for ${formatDistanceToNow(
          Date.parse(hasDormantWorkspaces.dormant_at),
        )}
        and cannot be interacted
		with. Dormant workspaces are eligible for
		permanent deletion. To prevent deletion, activate
		the workspace.`;
      }
    }
    return "";
  };

  return (
    <Alert severity="warning" onDismiss={onDismiss} dismissible>
      {count === Count.Singular ? (
        alertText()
      ) : (
        <>
          <span>There are</span>{" "}
          <Link component={RouterLink} to="/workspaces?filter=is-dormant:true">
            workspaces
          </Link>{" "}
          that may be deleted soon due to inactivity. Activate the workspaces
          you wish to retain.
        </>
      )}
    </Alert>
  );
};
