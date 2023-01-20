import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { MetricsPageView } from "./MetricsPageView"

const MetricsPage: FC = () => {
  const { deploymentDAUs, getDeploymentDAUsError } = useDeploySettings()
  return (
    <>
      <Helmet>
        <title>{pageTitle("Metrics")}</title>
      </Helmet>
      <MetricsPageView
        deploymentDAUs={deploymentDAUs}
        getDeploymentDAUsError={getDeploymentDAUsError}
      />
    </>
  )
}

export default MetricsPage
