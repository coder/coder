import {
  DeploySettingsLayout,
  SettingsHeader,
  SettingsItem,
  SettingsList,
} from "components/DeploySettingsLayout/DeploySettingsLayout"
import React from "react"

export const OIDCSettingsPage: React.FC = () => {
  return (
    <DeploySettingsLayout>
      <SettingsHeader
        title="OIDC"
        description="Configure these options at your deployment level with environment variables or command-line flags."
        docsHref="https://coder.com/docs/coder-oss/latest"
        isEnterprise
      />

      <SettingsList>
        <SettingsItem
          title="Access URL"
          description="Specifies the external URL to access Coder."
          values={[
            { label: "Value", value: "https://www.dev.coder.com" },
            { label: "Flag", value: "--access-url" },
            { label: "Env. Variable", value: "CODER_ACCESS_URL" },
          ]}
        />

        <SettingsItem
          title="Address"
          description="The address to serve the API and dashboard."
          values={[
            { label: "Value", value: "127.0.0.1:3000" },
            { label: "Flag", value: "--address" },
            { label: "Env. Variable", value: "CODER_ADDRESS" },
          ]}
        />
      </SettingsList>
    </DeploySettingsLayout>
  )
}
