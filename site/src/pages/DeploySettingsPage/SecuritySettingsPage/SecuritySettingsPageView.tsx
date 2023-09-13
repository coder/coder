import { DeploymentOption } from "api/api";
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
  EnterpriseBadge,
} from "components/DeploySettingsLayout/Badges";
import { Header } from "components/DeploySettingsLayout/Header";
import OptionsTable from "components/DeploySettingsLayout/OptionsTable";
import { Stack } from "components/Stack/Stack";
import {
  deploymentGroupHasParent,
  useDeploymentOptions,
} from "utils/deployOptions";
import { docs } from "utils/docs";

export type SecuritySettingsPageViewProps = {
  options: DeploymentOption[];
  featureAuditLogEnabled: boolean;
  featureBrowserOnlyEnabled: boolean;
};
export const SecuritySettingsPageView = ({
  options: options,
  featureAuditLogEnabled,
  featureBrowserOnlyEnabled,
}: SecuritySettingsPageViewProps): JSX.Element => {
  const tlsOptions = options.filter((o) =>
    deploymentGroupHasParent(o.group, "TLS"),
  );

  return (
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
              "Disable Owner Workspace Access",
            )}
          />
        </div>

        <div>
          <Header
            title="Audit Logging"
            secondary
            description="Allow auditors to monitor user operations in your deployment."
            docsHref={docs("/admin/audit-logs")}
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
            docsHref={docs("/networking#browser-only-connections-enterprise")}
          />

          <Badges>
            {featureBrowserOnlyEnabled ? <EnabledBadge /> : <DisabledBadge />}
            <EnterpriseBadge />
          </Badges>
        </div>

        {tlsOptions.length > 0 && (
          <div>
            <Header
              title="TLS"
              secondary
              description="Ensure TLS is properly configured for your Coder deployment."
            />

            <OptionsTable options={tlsOptions} />
          </div>
        )}
      </Stack>
    </>
  );
};
