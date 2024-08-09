import type { FC } from "react";
import type { SerpentOption } from "api/typesGenerated";
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
  EnterpriseBadge,
} from "components/Badges/Badges";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { deploymentGroupHasParent } from "utils/deployOptions";
import { docs } from "utils/docs";
import OptionsTable from "../OptionsTable";

export type ObservabilitySettingsPageViewProps = {
  options: SerpentOption[];
  featureAuditLogEnabled: boolean;
};

export const ObservabilitySettingsPageView: FC<
  ObservabilitySettingsPageViewProps
> = ({ options: options, featureAuditLogEnabled }) => {
  return (
    <>
      <Stack direction="column" spacing={6}>
        <div>
          <SettingsHeader title="Observability" />
          <SettingsHeader
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
          <SettingsHeader
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
