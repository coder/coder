import { DeploymentConfig } from "api/typesGenerated"
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
  EnterpriseBadge,
} from "components/DeploySettingsLayout/Badges"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import { Stack } from "components/Stack/Stack"

export type SecuritySettingsPageViewProps = {
  deploymentConfig: Pick<
    DeploymentConfig,
    "tls" | "ssh_keygen_algorithm" | "secure_auth_cookie"
  >
  featureAuditLogEnabled: boolean
  featureBrowserOnlyEnabled: boolean
}
export const SecuritySettingsPageView = ({
  deploymentConfig,
  featureAuditLogEnabled,
  featureBrowserOnlyEnabled,
}: SecuritySettingsPageViewProps): JSX.Element => (
  <>
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
          {featureAuditLogEnabled ? <EnabledBadge /> : <DisabledBadge />}
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
          {featureBrowserOnlyEnabled ? <EnabledBadge /> : <DisabledBadge />}
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
            tls_enable: deploymentConfig.tls.enable,
            tls_cert_files: deploymentConfig.tls.cert_file,
            tls_key_files: deploymentConfig.tls.key_file,
            tls_min_version: deploymentConfig.tls.min_version,
          }}
        />
      </div>
    </Stack>
  </>
)
