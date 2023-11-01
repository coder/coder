import { ClibaseOption } from "api/typesGenerated";
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
  EnterpriseBadge,
} from "components/DeploySettingsLayout/Badges";
import { Header } from "components/DeploySettingsLayout/Header";
import OptionsTable from "components/DeploySettingsLayout/OptionsTable";
import { Stack } from "components/Stack/Stack";
import { deploymentGroupHasParent } from "utils/deployOptions";
import { docs } from "utils/docs";

export type ObservabilitySettingsPageViewProps = {
  options: ClibaseOption[];
  featureAuditLogEnabled: boolean;
};
export const ObservabilitySettingsPageView = ({
  options: options,
  featureAuditLogEnabled,
}: ObservabilitySettingsPageViewProps): JSX.Element => {
  return (
    <>
      <Stack direction="column" spacing={6}>
        <div>
          <Header title="Observability" />
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
            title="Monitoring"
            secondary
            description="Monitoring your Coder application with logs and metrics."
          />

          <OptionsTable
            options={options.filter((o) =>
              deploymentGroupHasParent(o.group, "Introspection"),
            )}
          />
        </div>
      </Stack>
    </>
  );
};
