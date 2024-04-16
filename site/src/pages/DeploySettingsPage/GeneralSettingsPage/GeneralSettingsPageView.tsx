import AlertTitle from "@mui/material/AlertTitle";
import type { FC } from "react";
import type {
  SerpentOption,
  DAUsResponse,
  Entitlements,
  Experiments,
} from "api/typesGenerated";
import {
  ActiveUserChart,
  ActiveUsersTitle,
} from "components/ActiveUserChart/ActiveUserChart";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Stack } from "components/Stack/Stack";
import { useDeploymentOptions } from "utils/deployOptions";
import { docs } from "utils/docs";
import { Alert } from "../../../components/Alert/Alert";
import { Header } from "../Header";
import OptionsTable from "../OptionsTable";
import { ChartSection } from "./ChartSection";

export type GeneralSettingsPageViewProps = {
  deploymentOptions: SerpentOption[];
  deploymentDAUs?: DAUsResponse;
  deploymentDAUsError: unknown;
  entitlements: Entitlements | undefined;
  enabledExperiments: Experiments;
  safeExperiments: Experiments;
};

export const GeneralSettingsPageView: FC<GeneralSettingsPageViewProps> = ({
  deploymentOptions,
  deploymentDAUs,
  deploymentDAUsError,
  entitlements,
  enabledExperiments,
  safeExperiments,
}) => {
  const safe: string[] = [];
  const invalid: string[] = [];

  enabledExperiments.forEach(function (exp) {
    let found = false;
    safeExperiments.forEach(function (name) {
      if (exp === name) {
        found = true;
        return;
      }
    })

    if (found) {
      safe.push(exp);
    } else {
      invalid.push(exp);
    }
  });

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
        {invalid.length > 0 && (
          <Alert severity="warning">
            <AlertTitle>Invalid experiments in use:</AlertTitle>
            <ul>
              {invalid.map((it) => (
                <li key={it}>
                  <pre>{it}</pre>
                </li>
              ))}
            </ul>
            It is recommended that you remove these experiments from your
            configuration as they have no effect. See <a href="https://coder.com/docs/v2/latest/cli/server#--experiments" target="_blank">the documentation</a> for more details.
          </Alert>
        )}
        <OptionsTable
          options={useDeploymentOptions(
            deploymentOptions,
            "Access URL",
            "Wildcard Access URL",
            "Experiments",
          )}
          additionalValues={safe}
        />
      </Stack>
    </>
  );
};
