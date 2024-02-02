import { type FC } from "react";
import type {
  ClibaseOption,
  DAUsResponse,
  Entitlements,
  Experiments,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
  ActiveUserChart,
  ActiveUsersTitle,
} from "components/ActiveUserChart/ActiveUserChart";
import { Stack } from "components/Stack/Stack";
import { Header } from "../Header";
import OptionsTable from "../OptionsTable";
import { ChartSection } from "./ChartSection";
import { useDeploymentOptions } from "utils/deployOptions";
import { docs } from "utils/docs";

export type GeneralSettingsPageViewProps = {
  deploymentOptions: ClibaseOption[];
  deploymentDAUs?: DAUsResponse;
  deploymentDAUsError: unknown;
  entitlements: Entitlements | undefined;
  safeExperiments: Experiments | undefined;
};

export const GeneralSettingsPageView: FC<GeneralSettingsPageViewProps> = ({
  deploymentOptions,
  deploymentDAUs,
  deploymentDAUsError,
  entitlements,
  safeExperiments,
}) => {
  return (
    <>
      <Header
        title="General"
        description="Information about your Coder deployment."
        docsHref={docs("/admin/configure")}
      />
      <Stack spacing={4}>
        {Boolean(deploymentDAUsError) && (
          <ErrorAlert error={deploymentDAUsError} />
        )}
        {deploymentDAUs && (
          <div css={{ marginBottom: 24, height: 200 }}>
            <ChartSection title={<ActiveUsersTitle />}>
              <ActiveUserChart
                data={deploymentDAUs.entries}
                interval="day"
                userLimit={
                  entitlements?.features.user_limit.enabled
                    ? entitlements?.features.user_limit.limit
                    : undefined
                }
              />
            </ChartSection>
          </div>
        )}
        <OptionsTable
          options={useDeploymentOptions(
            deploymentOptions,
            "Access URL",
            "Wildcard Access URL",
            "Experiments",
          )}
          additionalValues={safeExperiments}
        />
      </Stack>
    </>
  );
};
