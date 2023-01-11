import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { NetworkSettingsPageView } from "./NetworkSettingsPageView"

const NetworkSettingsPage: FC = () => {
  const { deploymentConfig: deploymentConfig } = useDeploySettings()

  return (
    <>
      <Helmet>
        <title>{pageTitle("Network Settings")}</title>
      </Helmet>

      <NetworkSettingsPageView deploymentConfig={deploymentConfig} />
    </>
  )
}

export default NetworkSettingsPage
