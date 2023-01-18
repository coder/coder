import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { MetricsPageView } from "./MetricsPageView"

const MetricsPage: FC = () => {
  const { deploymentDAUs } = useDeploySettings()
  return (
    <>
      <Helmet>
        <title>{pageTitle("General Settings")}</title>
      </Helmet>
      <MetricsPageView deploymentDAUs={deploymentDAUs} />
    </>
  )
}

export default MetricsPage
