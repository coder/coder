import { TemplateDAUsResponse } from "api/typesGenerated"
import { DAUChart } from "components/DAUChart/DAUChart"
import { Stack } from "components/Stack/Stack"

interface MetricsPageViewProps {
  deploymentDAUs?: TemplateDAUsResponse
}

export const MetricsPageView = ({ deploymentDAUs }: MetricsPageViewProps): JSX.Element => {
  return (
    <Stack>
      {deploymentDAUs && <DAUChart daus={deploymentDAUs} />}
    </Stack>
  )
}
