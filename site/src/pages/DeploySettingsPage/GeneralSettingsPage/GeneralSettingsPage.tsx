import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import React from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { GeneralSettingsPageView } from "./GeneralSettingsPageView"

const GeneralSettingsPage: React.FC = () => {
  const { deploymentConfig: deploymentConfig } = useDeploySettings()

  return (
    <>
      <Helmet>
        <title>{pageTitle("General Settings")}</title>
      </Helmet>
      <GeneralSettingsPageView deploymentConfig={deploymentConfig} />
    </>
  )
}

export default GeneralSettingsPage
