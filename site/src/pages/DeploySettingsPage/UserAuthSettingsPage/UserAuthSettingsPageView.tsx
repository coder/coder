import { DeploymentOption } from "api/types"
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
} from "components/DeploySettingsLayout/Badges"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import { Stack } from "components/Stack/Stack"
import {
  deploymentGroupHasParent,
  useDeploymentOptions,
} from "util/deployOptions"

export type UserAuthSettingsPageViewProps = {
  options: DeploymentOption[]
}

export const UserAuthSettingsPageView = ({
  options,
}: UserAuthSettingsPageViewProps): JSX.Element => (
  <>
    <Stack direction="column" spacing={6}>
      <div>
        <Header title="User Authentication" />

        <Header
          title="Login with OpenID Connect"
          secondary
          description="Set up authentication to login with OpenID Connect."
          docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#openid-connect-with-google"
        />

        <Badges>
          {useDeploymentOptions(options, "OIDC Client ID")[0].value ? (
            <EnabledBadge />
          ) : (
            <DisabledBadge />
          )}
        </Badges>

        <OptionsTable
          options={options.filter((o) =>
            deploymentGroupHasParent(o.group, "OIDC"),
          )}
        />
      </div>

      <div>
        <Header
          title="Login with GitHub"
          secondary
          description="Set up authentication to login with GitHub."
          docsHref="https://coder.com/docs/coder-oss/latest/admin/auth#github"
        />

        <Badges>
          {useDeploymentOptions(options, "OAuth2 GitHub Client ID")[0].value ? (
            <EnabledBadge />
          ) : (
            <DisabledBadge />
          )}
        </Badges>

        <OptionsTable
          options={options.filter((o) =>
            deploymentGroupHasParent(o.group, "GitHub"),
          )}
        />
      </div>
    </Stack>
  </>
)
