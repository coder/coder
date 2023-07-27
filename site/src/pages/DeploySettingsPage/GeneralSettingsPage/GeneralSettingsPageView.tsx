import Box from "@mui/material/Box"
import { DeploymentOption } from "api/types"
import { DAUsResponse } from "api/typesGenerated"
import { ErrorAlert } from "components/Alert/ErrorAlert"
import { DAUChart, DAUTitle } from "components/DAUChart/DAUChart"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import { Stack } from "components/Stack/Stack"
import { WorkspaceSection } from "components/WorkspaceSection/WorkspaceSection"
import { useDeploymentOptions } from "utils/deployOptions"
import { docs } from "utils/docs"

export type GeneralSettingsPageViewProps = {
  deploymentOptions: DeploymentOption[]
  deploymentDAUs?: DAUsResponse
  getDeploymentDAUsError: unknown
}
export const GeneralSettingsPageView = ({
  deploymentOptions,
  deploymentDAUs,
  getDeploymentDAUsError,
}: GeneralSettingsPageViewProps): JSX.Element => {
  return (
    <>
      <Header
        title="General"
        description="Information about your Coder deployment."
        docsHref={docs("/admin/configure")}
      />
      <Stack spacing={4}>
        {Boolean(getDeploymentDAUsError) && (
          <ErrorAlert error={getDeploymentDAUsError} />
        )}
        {deploymentDAUs && (
          <Box height={200} sx={{ mb: 3 }}>
            <WorkspaceSection title={<DAUTitle />}>
              <DAUChart daus={deploymentDAUs} />
            </WorkspaceSection>
          </Box>
        )}
        <OptionsTable
          options={useDeploymentOptions(
            deploymentOptions,
            "Access URL",
            "Wildcard Access URL",
          )}
        />
      </Stack>
    </>
  )
}
