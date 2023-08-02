import { Workspace } from "api/typesGenerated"
import { isWorkspaceActionsEnabled } from "components/Dashboard/DashboardProvider"
import { Pill } from "components/Pill/Pill"
import LockIcon from "@mui/icons-material/Lock"

export const LockedBadge = ({
  workspace,
}: {
  workspace: Workspace
}): JSX.Element | null => {
  if (!workspace.locked_at || !isWorkspaceActionsEnabled()) {
    return null
  }

  return <Pill icon={<LockIcon />} text="Locked" type="error" />
}
