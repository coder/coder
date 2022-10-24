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
          derp_server_enable: deploymentConfig.derp.server.enable,
          derp_server_region_name: deploymentConfig.derp.server.region_name,
          derp_server_stun_addresses:
            deploymentConfig.derp.server.stun_addresses,
          derp_config_url: deploymentConfig.derp.config.url,
        }}
      />
    </>
  )
}

export default NetworkSettingsPage
