import AlertTitle from "@mui/material/AlertTitle";
import type { FC } from "react";
import type {
  SerpentOption,
  DAUsResponse,
  Entitlements,
  ExperimentDetail,
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
  experiments?: ExperimentDetail[];
};

export const GeneralSettingsPageView: FC<GeneralSettingsPageViewProps> = ({
  deploymentOptions,
  deploymentDAUs,
  deploymentDAUsError,
  entitlements,
  experiments,
}) => {
  const safe: string[] = [];
  const invalid: string[] = [];

  if (experiments) {
    experiments?.forEach(function (value) {
      if (value.invalid) {
        invalid.push(value.name);
      } else {
        safe.push(value.name);
      }
    });
  }

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
            configuration as they have no effect.
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
