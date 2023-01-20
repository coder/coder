import { DeploymentDAUsResponse } from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { DAUChart } from "components/DAUChart/DAUChart"
import { Stack } from "components/Stack/Stack"

export interface MetricsPageViewProps {
  deploymentDAUs?: DeploymentDAUsResponse
  getDeploymentDAUsError?: unknown
}

export const MetricsPageView = ({
  deploymentDAUs,
  getDeploymentDAUsError,
}: MetricsPageViewProps): JSX.Element => {
  return (
    <Stack>
      {Boolean(getDeploymentDAUsError) && (
        <AlertBanner error={getDeploymentDAUsError} severity="error" />
      )}
      {deploymentDAUs && <DAUChart daus={deploymentDAUs} />}
    </Stack>
  )
}
