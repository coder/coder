import { useMachine } from "@xstate/react"
import { usePermissions } from "hooks/usePermissions"
import { DeploymentBannerView } from "./DeploymentBannerView"
import { deploymentStatsMachine } from "xServices/deploymentStats/deploymentStatsMachine"

export const DeploymentBanner: React.FC = () => {
  const permissions = usePermissions()
  const [state, sendEvent] = useMachine(deploymentStatsMachine)

  if (!permissions.viewDeploymentValues || !state.context.deploymentStats) {
    return null
  }

  return (
    <DeploymentBannerView
      stats={state.context.deploymentStats}
      fetchStats={() => sendEvent("RELOAD")}
    />
  )
}
