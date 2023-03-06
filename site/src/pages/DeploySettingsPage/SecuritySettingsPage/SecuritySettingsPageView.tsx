import { DeploymentOption } from "api/types"
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
  EnterpriseBadge,
} from "components/DeploySettingsLayout/Badges"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import { Stack } from "components/Stack/Stack"
import {
  deploymentGroupHasParent,
  useDeploymentOptions,
} from "util/deployOptions"

export type SecuritySettingsPageViewProps = {
  options: DeploymentOption[]
  featureAuditLogEnabled: boolean
  featureBrowserOnlyEnabled: boolean
}
export const SecuritySettingsPageView = ({
  options: options,
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
          options={useDeploymentOptions(
            options,
            "SSH Keygen Algorithm",
            "Secure Auth Cookie",
          )}
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
          options={options.filter((o) =>
            deploymentGroupHasParent(o.group, "TLS"),
          )}
        />
      </div>
    </Stack>
  </>
)
