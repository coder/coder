import { DeploymentConfig } from "api/typesGenerated"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"

export type NetworkSettingsPageViewProps = {
  deploymentConfig: Pick<DeploymentConfig, "derp">
}

export const NetworkSettingsPageView = ({
  deploymentConfig,
}: NetworkSettingsPageViewProps): JSX.Element => (
  <>
    <Header
      title="Network"
      description="Configure your deployment connectivity."
      docsHref="https://coder.com/docs/coder-oss/latest/networking"
    />

    <OptionsTable
      options={{
        derp_server_enable: deploymentConfig.derp.server.enable,
        derp_server_region_name: deploymentConfig.derp.server.region_name,
        derp_server_stun_addresses: deploymentConfig.derp.server.stun_addresses,
        derp_config_url: deploymentConfig.derp.config.url,
      }}
    />
  </>
)
