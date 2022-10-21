import { useActor } from "@xstate/react"
import { FeatureNames } from "api/types"
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
  EnterpriseBadge,
} from "components/DeploySettingsLayout/Badges"
import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import { Stack } from "components/Stack/Stack"
import React, { useContext } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { XServiceContext } from "xServices/StateContext"

const SecuritySettingsPage: React.FC = () => {
  const { deploymentConfig: deploymentConfig } = useDeploySettings()
  const xServices = useContext(XServiceContext)
  const [entitlementsState] = useActor(xServices.entitlementsXService)

  return (
    <>
      <Helmet>
        <title>{pageTitle("Security Settings")}</title>
      </Helmet>
      <Stack direction="column" spacing={6}>
        <div>
          <Header
            title="Security"
            description="Ensure your Coder deployment is secure."
          />

          <OptionsTable
            options={{
              ssh_keygen_algorithm: deploymentConfig.ssh_keygen_algorithm,
              secure_auth_cookie: deploymentConfig.secure_auth_cookie,
            }}
          />
        </div>

        <div>
          <Header
            title="Audit Logging"
            secondary
            description="Allow auditors to monitor user operations in your deployment."
            docsHref="https://coder.com/docs/coder-oss/latest/admin/audit-logs"
          />

          <Badges>
            {entitlementsState.context.entitlements.features[
              FeatureNames.AuditLog
            ].enabled ? (
              <EnabledBadge />
            ) : (
              <DisabledBadge />
            )}
            <EnterpriseBadge />
          </Badges>
        </div>

        <div>
          <Header
            title="Browser Only Connections"
            secondary
            description="Block all workspace access via SSH, port forward, and other non-browser connections."
            docsHref="https://coder.com/docs/coder-oss/latest/networking#browser-only-connections-enterprise"
          />

          <Badges>
            {entitlementsState.context.entitlements.features[
              FeatureNames.BrowserOnly
            ].enabled ? (
              <EnabledBadge />
            ) : (
              <DisabledBadge />
            )}
            <EnterpriseBadge />
          </Badges>
        </div>

        <div>
          <Header
            title="TLS"
            secondary
            description="Ensure TLS is properly configured for your Coder deployment."
          />

          <OptionsTable
            options={{
              tls_enable: deploymentConfig.tls_enable,
              tls_cert_files: deploymentConfig.tls_cert_files,
              tls_key_files: deploymentConfig.tls_key_files,
              tls_min_version: deploymentConfig.tls_min_version,
            }}
          />
        </div>
      </Stack>
    </>
  )
}

export default SecuritySettingsPage
