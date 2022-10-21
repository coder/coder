import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import React from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"

const NetworkSettingsPage: React.FC = () => {
  const { deploymentConfig: deploymentConfig } = useDeploySettings()

  return (
    <>
      <Helmet>
        <title>{pageTitle("Network Settings")}</title>
      </Helmet>

      <Header
        title="Network"
        description="Configure your deployment connectivity."
        docsHref="https://coder.com/docs/coder-oss/latest/admin/networking"
      />

      <OptionsTable
        options={{
          derp_server_enabled: deploymentConfig.derp_server_enabled,
          derp_server_region_name: deploymentConfig.derp_server_region_name,
          derp_server_stun_address: deploymentConfig.derp_server_stun_address,
          derp_config_url: deploymentConfig.derp_config_url,
        }}
      />
    </>
  )
}

export default NetworkSettingsPage
