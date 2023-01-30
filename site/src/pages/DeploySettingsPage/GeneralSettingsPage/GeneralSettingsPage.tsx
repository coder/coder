import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { GeneralSettingsPageView } from "./GeneralSettingsPageView"

const GeneralSettingsPage: FC = () => {
  const { deploymentConfig, deploymentDAUs, getDeploymentDAUsError } =
    useDeploySettings()

  return (
    <>
      <Helmet>
        <title>{pageTitle("General Settings")}</title>
      </Helmet>
      <GeneralSettingsPageView
        deploymentConfig={deploymentConfig}
        deploymentDAUs={deploymentDAUs}
        getDeploymentDAUsError={getDeploymentDAUsError}
      />
    </>
  )
}

export default GeneralSettingsPage
