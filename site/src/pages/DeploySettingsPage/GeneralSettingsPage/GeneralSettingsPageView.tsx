import { DeploymentOption } from "api/types"
import { DeploymentDAUsResponse } from "api/typesGenerated"
import { ErrorAlert } from "components/Alert/ErrorAlert"
import { DAUChart } from "components/DAUChart/DAUChart"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import { Stack } from "components/Stack/Stack"
import { useDeploymentOptions } from "utils/deployOptions"

export type GeneralSettingsPageViewProps = {
  deploymentOptions: DeploymentOption[]
  deploymentDAUs?: DeploymentDAUsResponse
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
        docsHref="https://coder.com/docs/coder-oss/latest/admin/configure"
      />
      <Stack spacing={4}>
        {Boolean(getDeploymentDAUsError) && (
          <ErrorAlert error={getDeploymentDAUsError} />
        )}
        {deploymentDAUs && <DAUChart daus={deploymentDAUs} />}
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
