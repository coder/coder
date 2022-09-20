import Box from "@material-ui/core/Box"
import Typography from "@material-ui/core/Typography"
import CheckIcon from "@material-ui/icons/Check"
import ErrorIcon from "@material-ui/icons/Error"
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
        title="OpenID Connect"
        description="Configure external authentication to sign in to Coder."
        docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#openid-connect-with-google"
      />
      <Box display="flex" alignItems="center">
        {authState.context.methods?.oidc ? (
          <>
            <CheckIcon color="primary" /> <Typography color="primary">Enabled</Typography>
          </>
        ) : (
          <>
            <ErrorIcon color="secondary" /> <Typography color="secondary">Disabled</Typography>
          </>
        )}
      </Box>
      <Typography>
        Configure OpenID connect using command-line options in our documentation.
      </Typography>
      <SettingsList>
        <SettingsItem
          title="Allow Signups"
          description="Whether new users can sign up with OIDC."
          values={[
            { label: "Value", value: "true" },
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
