import Box from "@mui/material/Box";
import { DAUsResponse } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { DAUChart, DAUTitle } from "components/DAUChart/DAUChart";
import { Header } from "components/DeploySettingsLayout/Header";
import OptionsTable from "components/DeploySettingsLayout/OptionsTable";
import { Stack } from "components/Stack/Stack";
import { ChartSection } from "./ChartSection";
import { useDeploymentOptions } from "utils/deployOptions";
import { docs } from "utils/docs";
import { DeploymentOption } from "api/api";

export type GeneralSettingsPageViewProps = {
  deploymentOptions: DeploymentOption[];
  deploymentDAUs?: DAUsResponse;
  deploymentDAUsError: unknown;
};
export const GeneralSettingsPageView = ({
  deploymentOptions,
  deploymentDAUs,
  deploymentDAUsError,
}: GeneralSettingsPageViewProps): JSX.Element => {
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
          <Box height={200} sx={{ mb: 3 }}>
            <ChartSection title={<DAUTitle />}>
              <DAUChart daus={deploymentDAUs} />
            </ChartSection>
          </Box>
        )}
        <OptionsTable
          options={useDeploymentOptions(
            deploymentOptions,
            "Access URL",
            "Wildcard Access URL",
            "Experiments",
          )}
        />
      </Stack>
    </>
  );
};
