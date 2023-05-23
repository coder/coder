import { Workspace } from "api/typesGenerated"
import { displayImpendingDeletion } from "./utils"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import { Maybe } from "components/Conditionals/Maybe"
import { Alert } from "components/Alert/Alert"

export const ImpendingDeletionBanner = ({
  workspace,
  onDismiss,
  displayImpendingDeletionBanner,
}: {
  workspace?: Workspace
  onDismiss: () => void
  displayImpendingDeletionBanner: boolean
}): JSX.Element | null => {
  const { entitlements, experiments } = useDashboard()
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled
  // This check can be removed when https://github.com/coder/coder/milestone/19
  // is merged up
  const allowWorkspaceActions = experiments.includes("workspace_actions")

  return (
    <Maybe
      condition={Boolean(
        workspace &&
          displayImpendingDeletion(
            workspace,
            allowAdvancedScheduling,
            allowWorkspaceActions,
          ) &&
          displayImpendingDeletionBanner,
      )}
    >
      <Alert severity="info" onDismiss={onDismiss} dismissible>
        You have workspaces that will be deleted soon.
      </Alert>
    </Maybe>
  )
}
