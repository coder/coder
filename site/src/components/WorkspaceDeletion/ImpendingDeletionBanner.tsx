import { Workspace } from "api/typesGenerated"
import { useIsWorkspaceActionsEnabled } from "components/Dashboard/DashboardProvider"
import { Alert } from "components/Alert/Alert"
import { formatDistanceToNow } from "date-fns"
import Link from "@mui/material/Link"
import { Link as RouterLink } from "react-router-dom"

export enum Count {
  Singular,
  Multiple,
}

export const LockedWorkspaceBanner = ({
  workspaces,
  onDismiss,
  shouldRedisplayBanner,
  count = Count.Singular,
}: {
  workspaces?: Workspace[]
  onDismiss: () => void
  shouldRedisplayBanner: boolean
  count?: Count
}): JSX.Element | null => {
  const experimentEnabled = useIsWorkspaceActionsEnabled()

  if (!workspaces) {
    return null
  }

  const hasLockedWorkspaces = workspaces.find(
    (workspace) => workspace.locked_at,
  )

  const hasDeletionScheduledWorkspaces = workspaces.find(
    (workspace) => workspace.deleting_at,
  )

  if (
    // Only show this if the experiment is included.
    !experimentEnabled ||
    !hasLockedWorkspaces ||
    // Banners should be redisplayed after dismissal when additional workspaces are newly scheduled for deletion
    !shouldRedisplayBanner
  ) {
    return null
  }

  const formatDate = (dateStr: string): string => {
    const date = new Date(dateStr)
    return date.toLocaleDateString(undefined, {
      month: "long",
      day: "numeric",
      year: "numeric",
    })
  }

  const alertText = (): string => {
    if (workspaces.length === 1) {
      if (
        hasDeletionScheduledWorkspaces &&
        hasDeletionScheduledWorkspaces.deleting_at &&
        hasDeletionScheduledWorkspaces.locked_at
      ) {
        return `This workspace has been locked for ${formatDistanceToNow(
          Date.parse(hasDeletionScheduledWorkspaces.locked_at),
        )} and is scheduled to be deleted on ${formatDate(
          hasDeletionScheduledWorkspaces.deleting_at,
        )} . To keep it you must unlock the workspace.`
      } else if (hasLockedWorkspaces && hasLockedWorkspaces.locked_at) {
        return `This workspace has been locked for ${formatDate(
          hasLockedWorkspaces.locked_at,
        )}
        and cannot be interacted
		with. Locked workspaces are eligible for
		permanent deletion. To prevent deletion, unlock
		the workspace.`
      }
    }
    return ""
  }

  return (
    <Alert severity="warning" onDismiss={onDismiss} dismissible>
      {count === Count.Singular ? (
        alertText()
      ) : (
        <>
          <span>There are</span>{" "}
          <Link
            component={RouterLink}
            to="/workspaces?filter=locked_at:1970-01-01"
          >
            workspaces
          </Link>{" "}
          that may be deleted soon due to inactivity. Unlock the workspaces you
          wish to retain.
        </>
      )}
    </Alert>
  )
}
