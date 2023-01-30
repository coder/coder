import { DeploymentConfig, DeploymentDAUsResponse } from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { DAUChart } from "components/DAUChart/DAUChart"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import { Stack } from "components/Stack/Stack"

export type GeneralSettingsPageViewProps = {
  deploymentConfig: Pick<DeploymentConfig, "access_url" | "wildcard_access_url">
  deploymentDAUs?: DeploymentDAUsResponse
  getDeploymentDAUsError: unknown
}
export const GeneralSettingsPageView = ({
  deploymentConfig,
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
          <AlertBanner error={getDeploymentDAUsError} severity="error" />
        )}
        {deploymentDAUs && <DAUChart daus={deploymentDAUs} />}
        <OptionsTable
          options={{
            access_url: deploymentConfig.access_url,
            wildcard_access_url: deploymentConfig.wildcard_access_url,
          }}
        />
      </Stack>
    </>
  )
}
