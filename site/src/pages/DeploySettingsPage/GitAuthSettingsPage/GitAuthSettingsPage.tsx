import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import React from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { GitAuthSettingsPageView } from "./GitAuthSettingsPageView"

const GitAuthSettingsPage: React.FC = () => {
  const { deploymentConfig: deploymentConfig } = useDeploySettings()

  return (
    <>
      <Helmet>
        <title>{pageTitle("Git Authentication Settings")}</title>
      </Helmet>

      <GitAuthSettingsPageView deploymentConfig={deploymentConfig} />
    </>
  )
}

export default GitAuthSettingsPage
