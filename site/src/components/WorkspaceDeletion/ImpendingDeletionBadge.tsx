import { Workspace } from "api/typesGenerated"
import { displayLockedWorkspace } from "./utils"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import { Pill } from "components/Pill/Pill"
import LockIcon from "@mui/icons-material/Lock"

export const LockedBadge = ({
  workspace,
}: {
  workspace: Workspace
}): JSX.Element | null => {
  const { entitlements, experiments } = useDashboard()
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled
  // This check can be removed when https://github.com/coder/coder/milestone/19
  // is merged up
  const allowWorkspaceActions = experiments.includes("workspace_actions")
  // return null

  if (
    !displayLockedWorkspace(
      workspace,
      allowAdvancedScheduling,
      allowWorkspaceActions,
    )
  ) {
    return null
  }

  return <Pill icon={<LockIcon />} text="Locked" type="error" />
}
