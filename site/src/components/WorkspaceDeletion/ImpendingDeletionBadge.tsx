import { Workspace } from "api/typesGenerated"
import { useIsWorkspaceActionsEnabled } from "components/Dashboard/DashboardProvider"
import { Pill } from "components/Pill/Pill"
import LockIcon from "@mui/icons-material/Lock"

export const LockedBadge = ({
  workspace,
}: {
  workspace: Workspace
}): JSX.Element | null => {
  const experimentEnabled = useIsWorkspaceActionsEnabled()
  if (!workspace.locked_at || !experimentEnabled) {
    return null
  }

  return <Pill icon={<LockIcon />} text="Locked" type="error" />
}
