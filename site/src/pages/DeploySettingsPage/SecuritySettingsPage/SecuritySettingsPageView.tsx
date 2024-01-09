import { type FC } from "react";
import type { ClibaseOption } from "api/typesGenerated";
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
  EnterpriseBadge,
} from "components/Badges/Badges";
import { Stack } from "components/Stack/Stack";
import {
  deploymentGroupHasParent,
  useDeploymentOptions,
} from "utils/deployOptions";
import { docs } from "utils/docs";
import { Header } from "../Header";
import OptionsTable from "../OptionsTable";

export type SecuritySettingsPageViewProps = {
  options: ClibaseOption[];
  featureBrowserOnlyEnabled: boolean;
};

export const SecuritySettingsPageView: FC<SecuritySettingsPageViewProps> = ({
  options: options,
  featureBrowserOnlyEnabled,
}) => {
  const tlsOptions = options.filter((o) =>
    deploymentGroupHasParent(o.group, "TLS"),
  );

  return (
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
  );
};
