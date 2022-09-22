import { useActor } from "@xstate/react"
import {
  DeploySettingsLayout,
  SettingsHeader,
  SettingsItem,
  SettingsList,
} from "components/DeploySettingsLayout/DeploySettingsLayout"
import React, { useContext, useEffect } from "react"
import { XServiceContext } from "../../xServices/StateContext"

export const OIDCSettingsPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  useEffect(() => {
    authSend({ type: "GET_AUTH_METHODS" })
  }, [authSend])

  return (
    <DeploySettingsLayout>
      <SettingsHeader
        isEnabled={authState.context.methods?.oidc}
        isEnterprise
        title="OpenID Connect"
        description="Configure external authentication to sign in to Coder. Use the command-line options in our documentation."
        docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#openid-connect-with-google"
      />
      <SettingsList>
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
